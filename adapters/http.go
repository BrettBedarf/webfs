package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/brettbedarf/webfs"
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

func RegisterHTTP() {
	Register("http", func(raw []byte) (webfs.AdapterProvider, error) {
		var config HTTPSource
		if err := json.Unmarshal(raw, &config); err != nil {
			return nil, err
		}
		return &config, nil
	})
}

func (h *HTTPSource) Adapter() webfs.FileAdapter {
	return &HTTPAdapter{config: h}
}

// HTTPAdapter implements [webfs.FileAdapter] for HTTP sources
type HTTPAdapter struct {
	config *HTTPSource
}

func (h *HTTPAdapter) newRequest(ctx context.Context, method HTTPMethod) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, h.config.URL, nil)
	if err != nil {
		return nil, err
	}

	// Add custom headers
	for k, v := range h.config.Headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

func (h *HTTPAdapter) Stat(ctx context.Context) error {
	return fmt.Errorf("Stat not implemented")
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

func (h *HTTPAdapter) Read(ctx context.Context, offset int64, p []byte) (int, error) {
	// TODO: Implement range request logic
	return 0, fmt.Errorf("Read not implemented")
}

func (h *HTTPAdapter) Write(ctx context.Context, offset int64, p []byte) (int, error) {
	// HTTP sources are read-only to start
	return 0, fmt.Errorf("HTTP sources are read-only")
}

func (h *HTTPAdapter) Size(ctx context.Context) (int64, error) {
	req, err := h.newRequest(ctx, "HEAD")
	if err != nil {
		return 0, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	return resp.ContentLength, nil
}

func (h *HTTPAdapter) Exists(ctx context.Context) (bool, error) {
	req, err := h.newRequest(ctx, "HEAD")
	if err != nil {
		return false, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}

func (h *HTTPAdapter) getMethod() HTTPMethod {
	if h.config.Method != nil {
		return *h.config.Method
	}
	return HTTPMethodGet
}
