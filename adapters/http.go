package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/brettbedarf/webfs"
	"github.com/brettbedarf/webfs/internal/util"
)

type HTTPMethod = string

const (
	HTTPMethodGet    HTTPMethod = "GET"
	HTTPMethodHead   HTTPMethod = "HEAD"
	HTTPMethodPost   HTTPMethod = "POST"
	HTTPMethodPut    HTTPMethod = "PUT"
	HTTPMethodPatch  HTTPMethod = "PATCH"
	HTTPMethodDelete HTTPMethod = "DELETE"
)

func RegisterHTTP() {
	provider := &HTTPProvider{
		// Can add shared resources here later (HTTP client, connection pool, etc.)
	}
	Register("http", provider)
}

// HTTPProvider implements webfs.AdapterProvider for HTTP sources
type HTTPProvider struct {
	// Future: shared HTTP client, connection pool, etc.
}

// Adapter creates a new HTTPAdapter from raw JSON configuration
func (p *HTTPProvider) Adapter(raw []byte) (webfs.FileAdapter, error) {
	var config HTTPSource
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	return &HTTPAdapter{cfg: &config, log: util.GetLogger("http-adapter")}, nil
}

// HTTPSource contains http-specific source request fields
type HTTPSource struct {
	URL     string            `json:"url"`
	Method  *HTTPMethod       `json:"method,omitempty"` // Default is GET
	Headers map[string]string `json:"headers,omitempty"`

	// TODO: Timeout and MaxRedirects are client-specific so prob would want to
	// implement context-based logic to keep reusing common client with custom request
	//
	// Timeout      *int              `json:"timeout,omitempty"` // Timeout in seconds
	// MaxRedirects *int              `json:"maxRedirects,omitempty"`
}

// HTTPAdapter implements [webfs.FileAdapter] for HTTP sources
type HTTPAdapter struct {
	webfs.FileAdapter
	cfg *HTTPSource
	log util.Logger
}

func (h *HTTPAdapter) newRequest(ctx context.Context, method HTTPMethod) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, h.cfg.URL, nil)
	if err != nil {
		return nil, err
	}

	// Add custom headers
	for k, v := range h.cfg.Headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

func (h *HTTPAdapter) Open(ctx context.Context) (io.ReadCloser, error) {
	req, err := h.newRequest(ctx, h.getMethod())
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (h *HTTPAdapter) Read(ctx context.Context, offset int64, size int64, buf []byte) (int, error) {
	// Validate buffer size
	if int64(len(buf)) < size {
		return 0, fmt.Errorf("buffer too small: need %d bytes, got %d", size, len(buf))
	}

	// Create request with Range header for partial content
	req, err := h.newRequest(ctx, h.getMethod())
	if err != nil {
		return 0, err
	}

	// Add Range header: "bytes=start-end" (end is inclusive)
	rangeHeader := fmt.Sprintf("bytes=%d-%d", offset, offset+size-1)
	req.Header.Set("Range", rangeHeader)

	h.log.Debug().Str("range", rangeHeader).Str("url", h.cfg.URL).Msg("Making range request")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer h.closeResp(resp)

	// Check for success status codes
	// 206 = Partial Content (range request successful)
	// 200 = OK (server doesn't support ranges, returns full content)
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// If server doesn't support ranges (200 OK), we need to skip to offset
	if resp.StatusCode == http.StatusOK {
		// Server returned full content, discard bytes before offset
		if offset > 0 {
			discarded, err := io.CopyN(io.Discard, resp.Body, offset)
			if err != nil {
				return 0, fmt.Errorf("failed to skip to offset %d: %w", offset, err)
			}
			if discarded != offset {
				return 0, fmt.Errorf("could not skip to offset %d, only skipped %d bytes", offset, discarded)
			}
		}
	}

	// Read the requested data into buffer
	bytesRead, err := io.ReadFull(resp.Body, buf[:size])
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return 0, fmt.Errorf("failed to read data: %w", err)
	}

	h.log.Debug().Int("bytesRead", bytesRead).Int64("requested", size).Msg("Range request completed")

	return bytesRead, nil
}

func (h *HTTPAdapter) Write(ctx context.Context, offset int64, buf []byte) (int, error) {
	// HTTP sources are read-only to start
	return 0, fmt.Errorf("HTTP sources are read-only")
}

func (h *HTTPAdapter) GetMeta(ctx context.Context) (*webfs.FileMetadata, error) {
	req, err := h.newRequest(ctx, "HEAD")
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	h.closeResp(resp)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	meta := &webfs.FileMetadata{
		Size: uint64(resp.ContentLength), // TODO: Check for overflow?
	}

	// Parse Last-Modified header
	if lastMod := resp.Header.Get("Last-Modified"); lastMod != "" {
		if t, err := http.ParseTime(lastMod); err == nil {
			meta.LastModified = &t
		}
	}

	// ETag as version identifier
	if etag := resp.Header.Get("ETag"); etag != "" {
		meta.Version = etag
	}

	return meta, nil
}

func (h *HTTPAdapter) getMethod() HTTPMethod {
	if h.cfg.Method != nil {
		return *h.cfg.Method
	}
	return HTTPMethodGet
}

func (h *HTTPAdapter) closeResp(resp *http.Response) {
	if resp == nil {
		return
	}
	if err := resp.Body.Close(); err != nil {
		h.log.Error().Err(err).Interface("resp", resp).Interface("cfg", h.cfg).
			Msg("Failed to close response body")
	}
}
