package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/brettbedarf/webfs"
	"github.com/brettbedarf/webfs/internal/util"
)

type HTTPMethod string

const (
	HTTPMethodGet    HTTPMethod = "GET"
	HTTPMethodHead   HTTPMethod = "HEAD"
	HTTPMethodPost   HTTPMethod = "POST"
	HTTPMethodPut    HTTPMethod = "PUT"
	HTTPMethodPatch  HTTPMethod = "PATCH"
	HTTPMethodDelete HTTPMethod = "DELETE"
)

// validHTTPMethods is a set of valid HTTP methods for O(1) lookup
var validHTTPMethods = map[HTTPMethod]struct{}{
	HTTPMethodGet:    {},
	HTTPMethodHead:   {},
	HTTPMethodPost:   {},
	HTTPMethodPut:    {},
	HTTPMethodPatch:  {},
	HTTPMethodDelete: {},
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// HTTPSource contains http-specific source request fields
type HTTPSource struct {
	URL     string            `json:"url"`
	Method  HTTPMethod        `json:"method,omitempty"` // Default is GET
	Headers map[string]string `json:"headers,omitempty"`

	// TODO: Timeout and MaxRedirects are client-specific so prob would want to
	// implement context-based logic to keep reusing common client with custom request
	//
	// Timeout      *int              `json:"timeout,omitempty"` // Timeout in seconds
	// MaxRedirects *int              `json:"maxRedirects,omitempty"`
}

func RegisterHTTP(r *Registry) {
	provider := &HTTPProvider{
		client: http.DefaultClient,
	}
	r.Register("http", provider)
}

// HTTPProvider implements webfs.AdapterProvider for HTTP sources
type HTTPProvider struct {
	client HTTPClient
}

// NewAdapter creates a new HTTPAdapter from raw JSON configuration
func (p *HTTPProvider) NewAdapter(raw []byte) (webfs.FileAdapter, error) {
	log := util.GetLogger("http-adapter")
	var cfg HTTPSource
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if cfg.Method == "" {
		cfg.Method = HTTPMethodGet
	}

	// Config validation
	if _, ok := validHTTPMethods[cfg.Method]; !ok {
		return nil, fmt.Errorf("http adapter: invalid HTTP method: %s", cfg.Method)
	}

	// TODO: Check escaping (not sure if client does this)
	rawURL := strings.TrimSpace(cfg.URL)
	if rawURL == "" {
		return nil, fmt.Errorf("http adapter: URL is required")
	}

	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return nil, fmt.Errorf("http adapter: invalid URL: %w", err)
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("http adapter: URL is missing scheme (must be http:// or https://): %s", rawURL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("http adapter: unsupported URL scheme: %s", u.Scheme)
	}
	if u.User != nil {
		return nil, fmt.Errorf("http adapter: username not supported: %s", rawURL)
	}
	// Only warning if domain is not fully qualified
	if !strings.Contains(u.Host, ".") && u.Host != "localhost" {
		log.Warn().Str("url", rawURL).Msg("URL is missing domain and may fail to resolve")
	}

	cfg.URL = u.String()

	return NewHTTPAdapter(&cfg, p.client)
}

// HTTPAdapter implements [webfs.FileAdapter] for HTTP sources
type HTTPAdapter struct {
	cfg     *HTTPSource
	client  HTTPClient
	baseReq *http.Request
	log     util.Logger
}

func NewHTTPAdapter(cfg *HTTPSource, client HTTPClient) (*HTTPAdapter, error) {
	adp := &HTTPAdapter{cfg: cfg, client: client, log: util.GetLogger("http-adapter")}
	if req, err := http.NewRequest(string(cfg.Method), cfg.URL, nil); err != nil {
		return nil, fmt.Errorf("http adapter: invalid request for %s: %w", cfg.URL, err)
	} else {
		// Add custom headers
		for k, v := range cfg.Headers {
			req.Header.Set(k, v)
		}
		adp.baseReq = req
	}

	return adp, nil
}

func (h *HTTPAdapter) Open(ctx context.Context) (io.ReadCloser, error) {
	req := h.baseReq.Clone(ctx)
	resp, err := h.client.Do(req)
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
	req := h.baseReq.Clone(ctx)
	// Add Range header: "bytes=start-end" (end is inclusive)
	rangeHeader := fmt.Sprintf("bytes=%d-%d", offset, offset+size-1)
	req.Header.Set("Range", rangeHeader)

	resp, err := h.client.Do(req)
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
	req := h.baseReq.Clone(ctx)

	resp, err := h.client.Do(req)
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

func (h *HTTPAdapter) closeResp(resp *http.Response) {
	if resp == nil {
		return
	}
	if err := resp.Body.Close(); err != nil {
		h.log.Error().Err(err).Interface("resp", resp).Interface("cfg", h.cfg).
			Msg("Failed to close response body")
	}
}

// Compile-time interface check
var _ webfs.FileAdapter = (*HTTPAdapter)(nil)
