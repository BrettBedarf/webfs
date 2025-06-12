package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/brettbedarf/webfs/fs"
)

type HttpMethod = string

const (
	HttpMethodGet    HttpMethod = "GET"
	HttpMethodHead   HttpMethod = "HEAD"
	HttpMethodPost   HttpMethod = "POST"
	HttpMethodPut    HttpMethod = "PUT"
	HttpMethodPatch  HttpMethod = "PATCH"
	HttpMethodDelete HttpMethod = "DELETE"
)

// HTTP-specific source request
type HttpSourceConfig struct {
	URL     string            `json:"url"`
	Method  *HttpMethod       `json:"method,omitempty"` // Default is GET
	Headers map[string]string `json:"headers,omitempty"`

	// TODO: Timeout and MaxRedirects are client-specific so prob would want to
	// implement context-based logic to keep reusing common client with custom request
	//
	// Timeout      *int              `json:"timeout,omitempty"` // Timeout in seconds
	// MaxRedirects *int              `json:"maxRedirects,omitempty"`
}

func RegisterHttp() {
	Register("http", func(raw []byte) (fs.AdapterProvider, error) {
		var config HttpSourceConfig
		if err := json.Unmarshal(raw, &config); err != nil {
			return nil, err
		}
		return &config, nil
	})
}

func (h *HttpSourceConfig) Adapter() fs.FileAdapter {
	return &HttpAdapter{config: h}
}

// HttpAdapter implements [fs.FileAdapter]for HTTP sources
type HttpAdapter struct {
	config *HttpSourceConfig
}

func (h *HttpAdapter) newRequest(ctx context.Context, method HttpMethod) (*http.Request, error) {
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

func (h *HttpAdapter) Open(ctx context.Context) (io.ReadCloser, error) {
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

func (h *HttpAdapter) Read(ctx context.Context, offset int64, p []byte) (int, error) {
	// TODO: Implement range request logic
	return 0, fmt.Errorf("Read not implemented")
}

func (h *HttpAdapter) Write(ctx context.Context, offset int64, p []byte) (int, error) {
	// HTTP sources are read-only to start
	return 0, fmt.Errorf("HTTP sources are read-only")
}

func (h *HttpAdapter) Size(ctx context.Context) (int64, error) {
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

func (h *HttpAdapter) Exists(ctx context.Context) (bool, error) {
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

func (h *HttpAdapter) getMethod() HttpMethod {
	if h.config.Method != nil {
		return *h.config.Method
	}
	return HttpMethodGet
}
