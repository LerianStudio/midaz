//go:build fuzz

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fuzzy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/reporter/utils"
)

// FuzzBlocksConfig_MalformedRequest fuzzes the GET /v1/templates/blocks-config endpoint
// with various malformed HTTP headers, query parameters, and content types.
// Verifies: no 5xx errors or panics from arbitrary request variations.
func FuzzBlocksConfig_MalformedRequest(f *testing.F) {
	// Seed corpus: various malformed header/query combinations
	// Category: valid inputs
	f.Add("", "application/json", "GET")
	// Category: empty/boundary
	f.Add("", "", "GET")
	// Category: invalid content types
	f.Add("", "text/xml; charset=utf-8", "GET")
	f.Add("", "multipart/form-data; boundary=----", "GET")
	// Category: unicode/special characters in Accept header
	f.Add("", "\u0000\u0001\u0002", "GET")
	// Category: security payloads in query string
	f.Add("?<script>alert('xss')</script>", "application/json", "GET")
	f.Add("?' OR 1=1 --", "application/json", "GET")
	// Category: oversized query string
	f.Add("?"+strings.Repeat("a=b&", 500), "application/json", "GET")
	// Category: wrong HTTP methods
	f.Add("", "application/json", "POST")
	f.Add("", "application/json", "DELETE")
	f.Add("", "application/json", "PATCH")
	// Category: boundary values
	f.Add("?limit=-1&offset=-1", "application/json", "GET")
	f.Add("?"+strings.Repeat("x", 8192), "application/json", "GET")

	env := h.LoadEnvironment()
	h.RequireReachable(f, env.ManagerURL)
	ctx := context.Background()

	f.Fuzz(func(t *testing.T, queryString string, contentType string, method string) {
		// Bound inputs to prevent resource exhaustion
		if len(queryString) > 8192 {
			queryString = queryString[:8192]
		}
		if len(contentType) > 512 {
			contentType = contentType[:512]
		}
		if len(method) > 16 {
			method = method[:16]
		}

		// Only test valid HTTP methods to avoid client-side errors
		validMethods := map[string]bool{
			"GET": true, "POST": true, "PUT": true, "PATCH": true,
			"DELETE": true, "HEAD": true, "OPTIONS": true,
		}
		method = strings.ToUpper(strings.TrimSpace(method))
		if !validMethods[method] {
			method = "GET"
		}

		url := env.ManagerURL + "/v1/templates/blocks-config" + queryString
		req, err := http.NewRequestWithContext(ctx, method, url, nil)
		if err != nil {
			// Invalid URL from fuzzed query string is acceptable
			return
		}

		req.Header.Set("Authorization", "Bearer test")
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}

		client := &http.Client{Timeout: env.HTTPTimeout}
		resp, err := client.Do(req)
		if err != nil {
			// Connection errors are acceptable during fuzzing
			return
		}
		defer resp.Body.Close()

		// Drain body to prevent connection leaks
		_, _ = io.ReadAll(resp.Body)

		// Server must NEVER crash (5xx)
		if resp.StatusCode >= 500 {
			t.Fatalf("SERVER ERROR on blocks-config: code=%d method=%s query=%q contentType=%q",
				resp.StatusCode, method, queryString, contentType)
		}
	})
}

// FuzzFiltersConfig_MalformedRequest fuzzes the GET /v1/templates/filters endpoint
// with various malformed HTTP headers, query parameters, and content types.
// Verifies: no 5xx errors or panics from arbitrary request variations.
func FuzzFiltersConfig_MalformedRequest(f *testing.F) {
	// Seed corpus: various malformed header/query combinations
	// Category: valid inputs
	f.Add("", "application/json", "GET")
	// Category: empty/boundary
	f.Add("", "", "GET")
	// Category: invalid content types
	f.Add("", "text/xml; charset=utf-8", "GET")
	f.Add("", "application/x-www-form-urlencoded", "GET")
	// Category: unicode/special characters
	f.Add("", "\xef\xbb\xbfapplication/json", "GET")
	// Category: security payloads
	f.Add("?<img src=x onerror=alert(1)>", "application/json", "GET")
	f.Add("?'; DROP TABLE filters; --", "application/json", "GET")
	// Category: oversized query
	f.Add("?"+strings.Repeat("key=value&", 500), "application/json", "GET")
	// Category: wrong HTTP methods
	f.Add("", "application/json", "POST")
	f.Add("", "application/json", "PUT")
	f.Add("", "application/json", "DELETE")
	// Category: boundary values
	f.Add("?page=999999999&size=999999999", "application/json", "GET")

	env := h.LoadEnvironment()
	h.RequireReachable(f, env.ManagerURL)
	ctx := context.Background()

	f.Fuzz(func(t *testing.T, queryString string, contentType string, method string) {
		// Bound inputs to prevent resource exhaustion
		if len(queryString) > 8192 {
			queryString = queryString[:8192]
		}
		if len(contentType) > 512 {
			contentType = contentType[:512]
		}
		if len(method) > 16 {
			method = method[:16]
		}

		validMethods := map[string]bool{
			"GET": true, "POST": true, "PUT": true, "PATCH": true,
			"DELETE": true, "HEAD": true, "OPTIONS": true,
		}
		method = strings.ToUpper(strings.TrimSpace(method))
		if !validMethods[method] {
			method = "GET"
		}

		url := env.ManagerURL + "/v1/templates/filters" + queryString
		req, err := http.NewRequestWithContext(ctx, method, url, nil)
		if err != nil {
			return
		}

		req.Header.Set("Authorization", "Bearer test")
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}

		client := &http.Client{Timeout: env.HTTPTimeout}
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()

		_, _ = io.ReadAll(resp.Body)

		// Server must NEVER crash (5xx)
		if resp.StatusCode >= 500 {
			t.Fatalf("SERVER ERROR on filters-config: code=%d method=%s query=%q contentType=%q",
				resp.StatusCode, method, queryString, contentType)
		}
	})
}

// FuzzBlocksConfig_MalformedBody fuzzes the blocks-config endpoint by sending
// unexpected request bodies to a GET endpoint. Some frameworks parse bodies even
// on GET requests, which could cause panics or unexpected behavior.
// Verifies: no 5xx errors from arbitrary body content on GET endpoints.
func FuzzBlocksConfig_MalformedBody(f *testing.F) {
	// Seed corpus: various body payloads sent to a GET endpoint
	// Category: valid JSON
	f.Add(`{"key": "value"}`)
	// Category: empty
	f.Add("")
	f.Add(`{}`)
	// Category: invalid JSON
	f.Add(`{invalid json}`)
	f.Add(`{"unclosed": "string`)
	// Category: unicode/special
	f.Add("\u0000\u0001\u0002\u0003")
	f.Add(strings.Repeat("{", 1000))
	// Category: security payloads
	f.Add(`{"__proto__": {"admin": true}}`)
	f.Add(`<xml><script>alert(1)</script></xml>`)
	f.Add(`' OR '1'='1`)
	// Category: boundary - oversized
	f.Add(strings.Repeat("x", 10000))
	// Category: various content types
	f.Add(`null`)
	f.Add(`[]`)
	f.Add(`""`)

	env := h.LoadEnvironment()
	h.RequireReachable(f, env.ManagerURL)
	ctx := context.Background()

	f.Fuzz(func(t *testing.T, bodyContent string) {
		// Bound input to prevent OOM
		if len(bodyContent) > 100000 {
			bodyContent = bodyContent[:100000]
		}

		url := env.ManagerURL + "/v1/templates/blocks-config"
		req, err := http.NewRequestWithContext(ctx, "GET", url, bytes.NewReader([]byte(bodyContent)))
		if err != nil {
			return
		}

		req.Header.Set("Authorization", "Bearer test")
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: env.HTTPTimeout}
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		// Server must NEVER crash (5xx)
		if resp.StatusCode >= 500 {
			t.Fatalf("SERVER ERROR on blocks-config with body: code=%d body=%s payload=%q",
				resp.StatusCode, string(respBody), bodyContent)
		}

		// If response is 200, verify it returns valid JSON
		if resp.StatusCode == 200 {
			var result json.RawMessage
			if err := json.Unmarshal(respBody, &result); err != nil {
				t.Fatalf("blocks-config returned invalid JSON: err=%v body=%s", err, string(respBody))
			}
		}
	})
}

// FuzzFiltersConfig_MalformedBody fuzzes the filters endpoint by sending
// unexpected request bodies to a GET endpoint.
// Verifies: no 5xx errors from arbitrary body content on GET endpoints.
func FuzzFiltersConfig_MalformedBody(f *testing.F) {
	// Seed corpus: various body payloads
	// Category: valid JSON
	f.Add(`{"filter": "test"}`)
	// Category: empty
	f.Add("")
	f.Add(`{}`)
	// Category: invalid JSON
	f.Add(`not json at all`)
	f.Add(`{"broken":`)
	// Category: unicode/special
	f.Add("\xff\xfe")
	f.Add(strings.Repeat("[", 1000))
	// Category: security payloads
	f.Add(`{"$where": "function() { return true; }"}`)
	f.Add(`<!--#exec cmd="ls"-->`)
	f.Add(`%00%01%02`)
	// Category: boundary - oversized
	f.Add(strings.Repeat("a", 10000))
	// Category: null/array/primitives
	f.Add(`null`)
	f.Add(`[1,2,3]`)

	env := h.LoadEnvironment()
	h.RequireReachable(f, env.ManagerURL)
	ctx := context.Background()

	f.Fuzz(func(t *testing.T, bodyContent string) {
		if len(bodyContent) > 100000 {
			bodyContent = bodyContent[:100000]
		}

		url := env.ManagerURL + "/v1/templates/filters"
		req, err := http.NewRequestWithContext(ctx, "GET", url, bytes.NewReader([]byte(bodyContent)))
		if err != nil {
			return
		}

		req.Header.Set("Authorization", "Bearer test")
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: env.HTTPTimeout}
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		// Server must NEVER crash (5xx)
		if resp.StatusCode >= 500 {
			t.Fatalf("SERVER ERROR on filters with body: code=%d body=%s payload=%q",
				resp.StatusCode, string(respBody), bodyContent)
		}

		// If response is 200, verify it returns valid JSON
		if resp.StatusCode == 200 {
			var result json.RawMessage
			if err := json.Unmarshal(respBody, &result); err != nil {
				t.Fatalf("filters returned invalid JSON: err=%v body=%s", err, string(respBody))
			}
		}
	})
}

// FuzzBlocksConfig_HeaderInjection fuzzes the blocks-config endpoint with malformed
// HTTP headers to test for header injection vulnerabilities or server panics.
// Verifies: no 5xx errors from arbitrary header values.
func FuzzBlocksConfig_HeaderInjection(f *testing.F) {
	// Seed corpus: various Authorization and custom header values
	// Category: valid
	f.Add("Bearer valid-token-123", "application/json")
	// Category: empty
	f.Add("", "")
	// Category: boundary
	f.Add("Bearer "+strings.Repeat("x", 4096), "application/json")
	// Category: security - header injection
	f.Add("Bearer test\r\nX-Injected: true", "application/json")
	f.Add("Bearer test\nHost: evil.com", "application/json")
	// Category: unicode/special
	f.Add("\u0000Bearer\u0000test", "\u0000application/json")
	f.Add("Bearer \xef\xbb\xbftest", "application/json")
	// Category: SQL/NoSQL injection in auth header
	f.Add("Bearer ' OR '1'='1", "application/json")
	f.Add("Bearer {\"$gt\": \"\"}", "application/json")
	// Category: oversized
	f.Add(strings.Repeat("A", 65536), strings.Repeat("B", 1024))

	env := h.LoadEnvironment()
	h.RequireReachable(f, env.ManagerURL)
	ctx := context.Background()

	f.Fuzz(func(t *testing.T, authHeader string, acceptHeader string) {
		// Bound inputs
		if len(authHeader) > 65536 {
			authHeader = authHeader[:65536]
		}
		if len(acceptHeader) > 1024 {
			acceptHeader = acceptHeader[:1024]
		}

		url := env.ManagerURL + "/v1/templates/blocks-config"
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return
		}

		// Set fuzzed headers - Go's http library may reject truly invalid headers
		func() {
			defer func() {
				// Recover from panics in header setting (invalid chars)
				recover()
			}()
			if authHeader != "" {
				req.Header.Set("Authorization", authHeader)
			}
			if acceptHeader != "" {
				req.Header.Set("Accept", acceptHeader)
			}
		}()

		client := &http.Client{Timeout: env.HTTPTimeout}
		resp, err := client.Do(req)
		if err != nil {
			// Connection/header errors are acceptable during fuzzing
			return
		}
		defer resp.Body.Close()

		_, _ = io.ReadAll(resp.Body)

		// Server must NEVER crash (5xx)
		if resp.StatusCode >= 500 {
			t.Fatalf("SERVER ERROR on blocks-config header injection: code=%d auth=%q accept=%q",
				resp.StatusCode, authHeader, acceptHeader)
		}
	})
}
