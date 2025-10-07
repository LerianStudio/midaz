// Package helpers provides test utilities and helper functions for integration tests.
// This file contains HTTP client utilities and request/response helpers.
package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient wraps a standard http.Client with base URL handling.
type HTTPClient struct {
	base   string
	client *http.Client
}

// NewHTTPClient constructs a client with given base URL and timeout.
func NewHTTPClient(base string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		base: base,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// RequestFull executes an HTTP request and returns status, body, and headers.
func (c *HTTPClient) RequestFull(ctx context.Context, method, path string, headers map[string]string, body any) (int, []byte, http.Header, error) {
	var rdr io.Reader

	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("marshal body: %w", err)
		}

		rdr = bytes.NewReader(b)

		if headers == nil {
			headers = make(map[string]string)
		}

		if _, ok := headers["Content-Type"]; !ok {
			headers["Content-Type"] = "application/json"
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return 0, nil, nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, resp.Header, err
	}

	return resp.StatusCode, data, resp.Header, nil
}

// Request executes an HTTP request with optional JSON body and returns status code and response body.
func (c *HTTPClient) Request(ctx context.Context, method, path string, headers map[string]string, body any) (int, []byte, error) {
	code, b, _, err := c.RequestFull(ctx, method, path, headers, body)
	return code, b, err
}

// RequestRaw sends a request with an arbitrary raw body and explicit Content-Type.
func (c *HTTPClient) RequestRaw(ctx context.Context, method, path string, headers map[string]string, contentType string, raw []byte) (int, []byte, http.Header, error) {
	if headers == nil {
		headers = map[string]string{}
	}

	if contentType != "" {
		headers["Content-Type"] = contentType
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, resp.Header, err
	}

	return resp.StatusCode, b, resp.Header, nil
}

// RequestWithHeaderValues executes an HTTP request with explicit header values including duplicates.
// The map value is a slice; each value is added using Header.Add in order. Content-Type is NOT auto-set.
func (c *HTTPClient) RequestWithHeaderValues(ctx context.Context, method, path string, headers map[string][]string, body any) (int, []byte, http.Header, error) {
	var rdr io.Reader

	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("marshal body: %w", err)
		}

		rdr = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return 0, nil, nil, err
	}

	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, resp.Header, err
	}

	return resp.StatusCode, data, resp.Header, nil
}

// RequestFullWithRetry performs RequestFull with simple retry/backoff for transient statuses.
// Retries on 429, 502, 503, 504 or network errors up to attempts with exponential backoff.
func (c *HTTPClient) RequestFullWithRetry(ctx context.Context, method, path string, headers map[string]string, body any, attempts int, baseBackoff time.Duration) (int, []byte, http.Header, error) {
	if attempts <= 0 {
		attempts = 1
	}

	if baseBackoff <= 0 {
		baseBackoff = 200 * time.Millisecond
	}

	var (
		lastCode int
		lastBody []byte
		lastHdr  http.Header
		lastErr  error
	)

	for i := 0; i < attempts; i++ {
		code, b, hdr, err := c.RequestFull(ctx, method, path, headers, body)

		lastCode, lastBody, lastHdr, lastErr = code, b, hdr, err
		if err == nil && code != 429 && code != 502 && code != 503 && code != 504 {
			return code, b, hdr, nil
		}
		// back off only if another retry will be attempted
		if i < attempts-1 {
			time.Sleep(time.Duration(1<<i) * baseBackoff)
		}
	}

	return lastCode, lastBody, lastHdr, lastErr
}
