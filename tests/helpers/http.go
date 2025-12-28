package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	httpRetryStatusTooManyRequests = 429
	httpRetryStatusBadGateway      = 502
	httpRetryStatusServiceUnavail  = 503
	httpRetryStatusGatewayTimeout  = 504
	httpDefaultRetryAttempts       = 1
	httpDefaultBackoff             = 200 * time.Millisecond
	randHexRequestIDLength         = 16

	// HTTP transport connection pool configuration
	transportMaxIdleConns        = 100              // Total idle connections across all hosts
	transportMaxIdleConnsPerHost = 50               // Idle connections per host (default: 2)
	transportMaxConnsPerHost     = 100              // Active connections per host
	transportIdleConnTimeout     = 90 * time.Second // Idle connection timeout
	transportTLSHandshakeTimeout = 10 * time.Second // TLS handshake timeout
	transportExpectContinue      = 1 * time.Second  // Expect continue timeout
)

// HTTPClient wraps a standard http.Client with base URL handling.
type HTTPClient struct {
	base   string
	client *http.Client
}

// NewHTTPClient constructs a client with given base URL and timeout.
// Configures a custom Transport optimized for high-concurrency test scenarios
// to prevent connection pool starvation under parallel test load.
func NewHTTPClient(base string, timeout time.Duration) *HTTPClient {
	// Configure transport for high-concurrency test scenarios.
	// Default MaxIdleConnsPerHost=2 causes connection pool starvation
	// when running 30+ parallel tests with 40-110 concurrent requests each.
	transport := &http.Transport{
		// Connection pooling - critical for parallel tests
		MaxIdleConns:        transportMaxIdleConns,
		MaxIdleConnsPerHost: transportMaxIdleConnsPerHost,
		MaxConnsPerHost:     transportMaxConnsPerHost,

		// Timeouts for connection lifecycle
		IdleConnTimeout:       transportIdleConnTimeout,
		TLSHandshakeTimeout:   transportTLSHandshakeTimeout,
		ExpectContinueTimeout: transportExpectContinue,

		// Performance settings
		DisableKeepAlives:  false, // Keep connections alive for reuse
		DisableCompression: false,
		ForceAttemptHTTP2:  false, // Stay on HTTP/1.1 for simplicity
	}

	return &HTTPClient{
		base: base,
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
	}
}

// RequestFull executes an HTTP request and returns status, body, and headers.
func (c *HTTPClient) RequestFull(ctx context.Context, method, path string, headers map[string]string, body any) (int, []byte, http.Header, error) {
	var rdr io.Reader

	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			//nolint:wrapcheck // Error already wrapped with context for test helpers
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
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return 0, nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return 0, nil, nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return resp.StatusCode, nil, resp.Header, fmt.Errorf("failed to read response body: %w", err)
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
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return 0, nil, nil, fmt.Errorf("failed to create raw request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return 0, nil, nil, fmt.Errorf("failed to execute raw request: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return resp.StatusCode, nil, resp.Header, fmt.Errorf("failed to read raw response body: %w", err)
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
			//nolint:wrapcheck // Error already wrapped with context for test helpers
			return 0, nil, nil, fmt.Errorf("marshal body: %w", err)
		}

		rdr = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return 0, nil, nil, fmt.Errorf("failed to create request with header values: %w", err)
	}

	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return 0, nil, nil, fmt.Errorf("failed to execute request with header values: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return resp.StatusCode, nil, resp.Header, fmt.Errorf("failed to read response body with header values: %w", err)
	}

	return resp.StatusCode, data, resp.Header, nil
}

// isRetryableStatus returns true if the HTTP status code should trigger a retry
func isRetryableStatus(code int) bool {
	return code == httpRetryStatusTooManyRequests ||
		code == httpRetryStatusBadGateway ||
		code == httpRetryStatusServiceUnavail ||
		code == httpRetryStatusGatewayTimeout
}

// isRetryableError returns true if the error represents a transient connection issue.
// These errors typically occur under high load or during server restarts:
// - EOF: Connection closed prematurely by server
// - connection reset: TCP reset due to server overload
// - connection refused: Server temporarily unavailable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return strings.Contains(errStr, "EOF") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "connection refused")
}

// prepareRetryHeaders creates headers for a retry attempt
func prepareRetryHeaders(headers map[string]string, isRetry bool) map[string]string {
	retryHeaders := make(map[string]string)
	for k, v := range headers {
		retryHeaders[k] = v
	}

	if isRetry {
		retryHeaders["X-Request-Id"] = "retry-" + RandHex(randHexRequestIDLength)
	}

	return retryHeaders
}

// RequestFullWithRetry performs RequestFull with simple retry/backoff for transient statuses.
// Retries on 429, 502, 503, 504 or network errors (EOF, connection reset/refused) up to attempts with exponential backoff.
func (c *HTTPClient) RequestFullWithRetry(ctx context.Context, method, path string, headers map[string]string, body any, attempts int, baseBackoff time.Duration) (int, []byte, http.Header, error) {
	if attempts <= 0 {
		attempts = httpDefaultRetryAttempts
	}

	if baseBackoff <= 0 {
		baseBackoff = httpDefaultBackoff
	}

	var (
		lastCode int
		lastBody []byte
		lastHdr  http.Header
		lastErr  error
	)

	for i := 0; i < attempts; i++ {
		retryHeaders := prepareRetryHeaders(headers, i > 0)
		code, b, hdr, err := c.RequestFull(ctx, method, path, retryHeaders, body)

		lastCode, lastBody, lastHdr, lastErr = code, b, hdr, err

		// Success: no error and non-retryable status code
		if err == nil && !isRetryableStatus(code) {
			return code, b, hdr, nil
		}

		// Retry on connection errors (EOF, reset, refused) - common under high test parallelism
		if err != nil && isRetryableError(err) && i < attempts-1 {
			time.Sleep(time.Duration(1<<i) * baseBackoff)
			continue
		}

		// Retry on specific HTTP status codes (429, 502, 503, 504)
		if err == nil && isRetryableStatus(code) && i < attempts-1 {
			time.Sleep(time.Duration(1<<i) * baseBackoff)
			continue
		}

		// If we get here and it's not retryable, return immediately
		if err != nil && !isRetryableError(err) {
			return code, b, hdr, err
		}

		if err == nil && !isRetryableStatus(code) {
			return code, b, hdr, nil
		}
	}

	return lastCode, lastBody, lastHdr, lastErr
}
