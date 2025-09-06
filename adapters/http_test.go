package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockHTTPClient struct {
	mock.Mock
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	return args.Get(0).(*http.Response), args.Error(1)
}

func TestHTTPProvider_NewAdapter(t *testing.T) {
	provider := &HTTPProvider{&MockHTTPClient{}}

	t.Run("URL validation", func(t *testing.T) {
		tests := []struct {
			url     string
			wantErr bool
			desc    string
		}{
			// Valid cases
			{"http://test.com", false, "basic HTTP URL"},
			{"https://test.com", false, "basic HTTPS URL"},
			{"  http://test.com   ", false, "URL with whitespace"},
			{"http://test.com/path?arg=1&arg2=2", false, "URL with path and query"},
			{"http://test.com:8080", false, "URL with port"},
			{"http://localhost:8080/test", false, "localhost with port"},
			{"http://123.123.123.123/test", false, "IP address"},
			{"http://mylocalnet/test", false, "single label hostname"},

			// Invalid cases
			{"", true, "empty string"},
			{" ", true, "whitespace only"},
			{"_", true, "invalid character"},
			{"ftp://test.com", true, "different scheme rejected"},
			{"test.com", true, "missing scheme"},
			{"http://user@test.com/path", true, "URL with user info"},
		}

		for _, tt := range tests {
			t.Run(tt.desc, func(t *testing.T) {
				config := createCfg(tt.url)
				adapter, err := provider.NewAdapter(config)

				if tt.wantErr {
					assert.Error(t, err)
					assert.Nil(t, adapter)
				} else {
					require.NoError(t, err)
					require.NotNil(t, adapter)
					assert.IsType(t, &HTTPAdapter{}, adapter)
				}
			})
		}
	})

	t.Run("HTTP Method", func(t *testing.T) {
		tests := []struct {
			name    string
			method  HTTPMethod
			wantErr bool
		}{
			{
				"defaults GET",
				(HTTPMethod)(""),
				false,
			},
			{
				"valid method",
				HTTPMethodPost,
				false,
			},
			{
				"invalid method",
				(HTTPMethod)("INVALID"),
				true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cfg := &HTTPSource{URL: "https://test.com/test", Method: tt.method}
				raw, _ := json.Marshal(cfg)
				adapter, err := provider.NewAdapter(raw)

				if tt.wantErr {
					assert.Error(t, err)
					assert.Nil(t, adapter)
				} else {
					require.NoError(t, err)
					require.NotNil(t, adapter)
					assert.IsType(t, &HTTPAdapter{}, adapter)
				}
			})
		}
	})
}

func TestHTTPAdapter_Open(t *testing.T) {
	t.Run("successful request", func(t *testing.T) {
		mockClient := newMockHTTPClient(t)
		// mockClient.Test(t)
		adapter, createErr := NewHTTPAdapter(&HTTPSource{
			URL:    "http://test.com",
			Method: HTTPMethodGet,
			Headers: map[string]string{
				"TestHeader": "test-value",
			},
		}, mockClient)
		require.NoError(t, createErr)

		resp := createMockResponse(200, []byte("test body"), nil)
		mockClient.On("Do", mock.MatchedBy(func(req *http.Request) bool {
			return req.Method == http.MethodGet &&
				req.URL.String() == "http://test.coms" &&
				req.Header.Get("TestHeader") == "test-value"
		})).Return(resp, nil).Once()

		body, err := adapter.Open(context.Background())
		require.NoError(t, err)
		assert.Equal(t, resp.Body, body)
	})

	t.Run("returns error on http error", func(t *testing.T) {
		t.Skip()
		mockClient := newMockHTTPClient(t)
		adapter, createErr := NewHTTPAdapter(&HTTPSource{
			URL:    "http://test.com",
			Method: HTTPMethodGet,
		}, mockClient)
		require.NoError(t, createErr)

		mockClient.On("Do", mock.Anything).Return(nil, errors.New("network error")).Once()
		_, err := adapter.Open(context.Background())
		require.Error(t, err)
	})
}

func TestHTTPAdapter_Read(t *testing.T) {
	t.Run("successful range request", func(t *testing.T) {
		// TODO: Implement test with mock server supporting ranges
		t.Skip("Not implemented yet")
	})

	t.Run("server doesn't support ranges", func(t *testing.T) {
		// TODO: Implement test with server returning 200 OK
		t.Skip("Not implemented yet")
	})

	t.Run("buffer too small", func(t *testing.T) {
		// TODO: Implement test
		t.Skip("Not implemented yet")
	})

	t.Run("HTTP error status", func(t *testing.T) {
		// TODO: Implement test
		t.Skip("Not implemented yet")
	})

	t.Run("offset skip logic", func(t *testing.T) {
		// TODO: Test offset handling when server doesn't support ranges
		t.Skip("Not implemented yet")
	})
}

func TestHTTPAdapter_GetMeta(t *testing.T) {
	t.Run("successful HEAD request", func(t *testing.T) {
		// TODO: Implement test with mock server
		t.Skip("Not implemented yet")
	})

	t.Run("with Last-Modified header", func(t *testing.T) {
		// TODO: Implement test
		t.Skip("Not implemented yet")
	})

	t.Run("with ETag header", func(t *testing.T) {
		// TODO: Implement test
		t.Skip("Not implemented yet")
	})

	t.Run("HTTP error status", func(t *testing.T) {
		// TODO: Implement test
		t.Skip("Not implemented yet")
	})
}

func TestRegisterHTTP(t *testing.T) {
	t.Run("registers http provider", func(t *testing.T) {
		registry := NewRegistry()
		RegisterHTTP(registry)

		provider, err := registry.GetProvider("http")
		require.NoError(t, err)
		require.NotNil(t, provider)
		assert.IsType(t, &HTTPProvider{}, provider)
	})
}

// Test helpers

func createCfg(url string) []byte {
	config := HTTPSource{URL: url}
	data, _ := json.Marshal(config)
	return data
}

func createCfgWithOpts(url string, method HTTPMethod, headers map[string]string) []byte {
	config := HTTPSource{
		URL:     url,
		Method:  method,
		Headers: headers,
	}
	data, _ := json.Marshal(config)
	return data
}

func newMockHTTPClient(t *testing.T) *MockHTTPClient {
	m := &MockHTTPClient{}
	m.Test(t) // Associate with test to prevent panics
	return m
}

func createMockResponse(statusCode int, body []byte, headers map[string]string) *http.Response {
	h := make(http.Header)
	for k, v := range headers {
		h.Set(k, v)
	}
	resp := &http.Response{
		StatusCode: statusCode,
		Status:     fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     h,
	}

	return resp
}
