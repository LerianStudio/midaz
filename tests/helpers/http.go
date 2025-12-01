// Package helpers provides HTTP client utilities for Midaz integration tests.
//
// # Purpose
//
// This file provides a wrapper HTTP client with convenience methods for testing
// API endpoints. It handles JSON serialization, header management, and retry logic.
//
// # Features
//
// Request methods:
//   - Request: Simple request returning status and body
//   - RequestFull: Full request returning status, body, and headers
//   - RequestRaw: Send raw bytes with explicit content type
//   - RequestWithHeaderValues: Support for multi-value headers
//   - RequestFullWithRetry: Automatic retry with exponential backoff
//   - RequestMultipart: Multipart form data with files (in multipart.go)
//
// Automatic behaviors:
//   - JSON marshaling of request bodies
//   - Content-Type: application/json default for JSON bodies
//   - Base URL concatenation with paths
//
// # Usage
//
//	client := helpers.NewHTTPClient("http://localhost:3000", 20*time.Second)
//
//	// Simple request
//	status, body, err := client.Request(ctx, "GET", "/api/v1/users", headers, nil)
//
//	// Request with retry
//	status, body, hdrs, err := client.RequestFullWithRetry(ctx, "POST", "/api/v1/users",
//	    headers, payload, 3, 200*time.Millisecond)
//
// # Thread Safety
//
// HTTPClient wraps http.Client which is safe for concurrent use.
// Multiple goroutines can call methods on the same HTTPClient.
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
//
// # Fields
//
//   - base: Base URL prepended to all request paths
//   - client: Underlying http.Client with configured timeout
//
// # Thread Safety
//
// HTTPClient is safe for concurrent use. The underlying http.Client
// handles connection pooling and concurrent requests.
type HTTPClient struct {
	base   string
	client *http.Client
}

// NewHTTPClient constructs a client with given base URL and timeout.
//
// # Parameters
//
//   - base: Base URL for all requests (e.g., "http://localhost:3000")
//   - timeout: HTTP client timeout for all requests
//
// # Returns
//
//   - *HTTPClient: Configured client ready for use
func NewHTTPClient(base string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		base: base,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// RequestFull executes an HTTP request and returns status, body, and headers.
//
// # Parameters
//
//   - ctx: Context for request cancellation
//   - method: HTTP method (GET, POST, PUT, DELETE, etc.)
//   - path: URL path appended to base URL
//   - headers: Request headers (nil for none)
//   - body: Request body (will be JSON marshaled, nil for no body)
//
// # Process
//
//	Step 1: If body provided, marshal to JSON and set Content-Type
//	Step 2: Create request with context
//	Step 3: Set all provided headers
//	Step 4: Execute request
//	Step 5: Read and return response body
//
// # Returns
//
//   - int: HTTP status code
//   - []byte: Response body
//   - http.Header: Response headers
//   - error: Request or response error
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
//
// This is a convenience wrapper around RequestFull that discards response headers.
//
// # Parameters
//
//   - ctx: Context for request cancellation
//   - method: HTTP method
//   - path: URL path appended to base URL
//   - headers: Request headers
//   - body: Request body (JSON marshaled)
//
// # Returns
//
//   - int: HTTP status code
//   - []byte: Response body
//   - error: Request error
func (c *HTTPClient) Request(ctx context.Context, method, path string, headers map[string]string, body any) (int, []byte, error) {
	code, b, _, err := c.RequestFull(ctx, method, path, headers, body)
	return code, b, err
}

// RequestRaw sends a request with an arbitrary raw body and explicit Content-Type.
//
// Use this when sending non-JSON payloads like XML, form data, or binary content.
//
// # Parameters
//
//   - ctx: Context for request cancellation
//   - method: HTTP method
//   - path: URL path appended to base URL
//   - headers: Request headers (Content-Type will be overwritten)
//   - contentType: Content-Type header value
//   - raw: Raw request body bytes
//
// # Returns
//
//   - int: HTTP status code
//   - []byte: Response body
//   - http.Header: Response headers
//   - error: Request error
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
//
// Use this when you need to send multiple values for the same header (e.g., multiple
// Cookie headers). Values are added using Header.Add, not Header.Set.
//
// # Parameters
//
//   - ctx: Context for request cancellation
//   - method: HTTP method
//   - path: URL path appended to base URL
//   - headers: Map of header name to list of values
//   - body: Request body (JSON marshaled)
//
// # Note
//
// Content-Type is NOT auto-set. You must include it in headers if needed.
//
// # Returns
//
//   - int: HTTP status code
//   - []byte: Response body
//   - http.Header: Response headers
//   - error: Request error
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
//
// This method automatically retries requests that fail due to transient errors or
// receive certain HTTP status codes indicating temporary unavailability.
//
// # Retry Conditions
//
// Retries on:
//   - Network errors
//   - HTTP 429 (Too Many Requests)
//   - HTTP 502 (Bad Gateway)
//   - HTTP 503 (Service Unavailable)
//   - HTTP 504 (Gateway Timeout)
//
// # Parameters
//
//   - ctx: Context for request cancellation
//   - method: HTTP method
//   - path: URL path appended to base URL
//   - headers: Request headers
//   - body: Request body (JSON marshaled)
//   - attempts: Maximum number of attempts (minimum 1)
//   - baseBackoff: Base backoff duration (exponential: 1x, 2x, 4x, ...)
//
// # Backoff Strategy
//
// Exponential backoff: baseBackoff * 2^attempt
// Example with baseBackoff=200ms: 200ms, 400ms, 800ms, ...
//
// # Returns
//
//   - int: HTTP status code from last attempt
//   - []byte: Response body from last attempt
//   - http.Header: Response headers from last attempt
//   - error: Error from last attempt (nil if successful)
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
