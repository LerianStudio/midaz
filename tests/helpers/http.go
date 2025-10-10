// Package helpers provides reusable utilities and setup functions to streamline
// integration and end-to-end tests.
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

// HTTPClient is a wrapper around the standard http.Client that simplifies making
// requests to a specific base URL.
type HTTPClient struct {
	base   string
	client *http.Client
}

// NewHTTPClient creates a new HTTPClient with a specified base URL and request timeout.
func NewHTTPClient(base string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		base: base,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// RequestFull executes an HTTP request and returns the full response, including
// the status code, body, and headers.
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

// Request is a simplified version of RequestFull that executes an HTTP request
// and returns only the status code and response body.
func (c *HTTPClient) Request(ctx context.Context, method, path string, headers map[string]string, body any) (int, []byte, error) {
	code, b, _, err := c.RequestFull(ctx, method, path, headers, body)
	return code, b, err
}

// RequestRaw sends a request with an arbitrary raw byte slice as the body and an
// explicit Content-Type header.
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

// RequestWithHeaderValues executes an HTTP request with explicit header values,
// allowing for duplicate headers.
// The `headers` map uses a string slice to support multiple values for the same key.
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

// RequestFullWithRetry performs a request with a simple retry mechanism for transient errors.
// It retries on 429, 502, 503, 504 status codes, or network errors, with an
// exponential backoff strategy.
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
