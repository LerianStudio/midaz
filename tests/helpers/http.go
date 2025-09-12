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
