//go:build fuzz

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// safeMiddlewareTest creates a Fiber app with the given CORS config, sends the
// given request, and returns any panic message. If no panic occurs, returns "".
// This helper isolates Fiber's cors.New() panic behavior so fuzz tests can
// report panics as t.Error instead of crashing the fuzz process.
func safeMiddlewareTest(cfg CORSConfig, req *http.Request) (panicMsg string, statusCode int, testErr error) {
	defer func() {
		if r := recover(); r != nil {
			panicMsg = fmt.Sprintf("%v", r)
		}
	}()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Use(CORSMiddleware(cfg))

	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	resp, err := app.Test(req)
	if err != nil {
		return "", 0, err
	}

	defer resp.Body.Close()

	return "", resp.StatusCode, nil
}

// FuzzCORSMiddleware_Origins fuzz tests the CORSMiddleware with random origin
// configuration values. The middleware wraps Fiber's cors.New() and must never
// panic regardless of the AllowedOrigins string content.
//
// KNOWN CRASH: Fiber v2.52.11 cors.New() panics on empty origin strings
// produced by splitting comma-separated values with leading/trailing/consecutive
// commas (e.g., "," -> ["", ""]). This is tracked as a production safety issue.
func FuzzCORSMiddleware_Origins(f *testing.F) {
	// Seed corpus: 5 categories per Ring fuzz standards
	// Category 1: Valid inputs
	f.Add("https://app.example.com", "https://app.example.com")
	f.Add("https://a.com,https://b.com", "https://a.com")
	f.Add("http://localhost:3000", "http://localhost:3000")

	// Category 2: Empty/boundary values
	f.Add("", "https://any.com")
	f.Add(",", "https://any.com")
	f.Add(",,,,", "https://any.com")
	f.Add(strings.Repeat("https://a.com,", 50), "https://a.com")

	// Category 3: Unicode
	f.Add("https://\u65e5\u672c\u8a9e.example.com", "https://\u65e5\u672c\u8a9e.example.com")
	f.Add("https://\u00e9xample.com", "https://\u00e9xample.com")

	// Category 4: Invalid formats
	f.Add("*", "https://any.com")
	f.Add("not-a-url", "not-a-url")
	f.Add("://missing-scheme", "://missing-scheme")

	// Category 5: Security payloads
	f.Add("<script>alert('xss')</script>", "<script>alert('xss')</script>")
	f.Add("https://evil.com\r\nX-Injected: header", "https://evil.com")
	f.Add("' OR 1=1 --", "' OR 1=1 --")

	f.Fuzz(func(t *testing.T, allowedOrigins, requestOrigin string) {
		// Bound inputs to prevent resource exhaustion
		if len(allowedOrigins) > 4096 {
			allowedOrigins = allowedOrigins[:4096]
		}

		if len(requestOrigin) > 1024 {
			requestOrigin = requestOrigin[:1024]
		}

		cfg := CORSConfig{
			AllowedOrigins: allowedOrigins,
			AllowedMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
			AllowedHeaders: "Origin,Content-Type,Accept,Authorization",
		}

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		if requestOrigin != "" {
			req.Header.Set("Origin", requestOrigin)
		}

		panicMsg, statusCode, err := safeMiddlewareTest(cfg, req)

		// A panic in the middleware is a crash -- report as test failure
		if panicMsg != "" {
			t.Errorf("CORSMiddleware panicked for origins=%q request=%q: %s",
				allowedOrigins, requestOrigin, panicMsg)
			return
		}

		if err != nil {
			// Network/framework errors are acceptable during fuzzing
			return
		}

		// Invariant: response status must always be valid HTTP
		if statusCode < 100 || statusCode >= 600 {
			t.Errorf("invalid HTTP status code %d for origins=%q request=%q",
				statusCode, allowedOrigins, requestOrigin)
		}
	})
}

// FuzzCORSMiddleware_Methods fuzz tests the CORSMiddleware with random method
// configuration strings. The middleware must handle any AllowedMethods value
// without panicking.
func FuzzCORSMiddleware_Methods(f *testing.F) {
	// Seed corpus: 5 categories per Ring fuzz standards
	// Category 1: Valid inputs
	f.Add("GET,POST,PUT,PATCH,DELETE,OPTIONS")
	f.Add("GET")
	f.Add("POST,DELETE")

	// Category 2: Empty/boundary values
	f.Add("")
	f.Add(",")
	f.Add(strings.Repeat("GET,", 200))

	// Category 3: Unicode
	f.Add("\u65e5\u672c\u8a9e")
	f.Add("GET,\u00e9xtra")

	// Category 4: Invalid formats
	f.Add("INVALID_METHOD")
	f.Add("GET POST PUT")
	f.Add("GET;POST;PUT")

	// Category 5: Security payloads
	f.Add("<script>alert(1)</script>")
	f.Add("GET\r\nX-Injected: true")
	f.Add("' OR 1=1 --")

	f.Fuzz(func(t *testing.T, allowedMethods string) {
		// Bound input to prevent resource exhaustion
		if len(allowedMethods) > 4096 {
			allowedMethods = allowedMethods[:4096]
		}

		cfg := CORSConfig{
			AllowedOrigins: "https://app.example.com",
			AllowedMethods: allowedMethods,
			AllowedHeaders: "Origin,Content-Type,Accept",
		}

		req := httptest.NewRequest(http.MethodOptions, "/test", nil)
		req.Header.Set("Origin", "https://app.example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")

		panicMsg, statusCode, err := safeMiddlewareTest(cfg, req)

		if panicMsg != "" {
			t.Errorf("CORSMiddleware panicked for methods=%q: %s",
				allowedMethods, panicMsg)
			return
		}

		if err != nil {
			return
		}

		// Invariant: response status must always be valid HTTP
		if statusCode < 100 || statusCode >= 600 {
			t.Errorf("invalid HTTP status code %d for methods=%q",
				statusCode, allowedMethods)
		}
	})
}

// FuzzCORSMiddleware_Headers fuzz tests the CORSMiddleware with random header
// configuration strings. The middleware must handle any AllowedHeaders value
// without panicking.
func FuzzCORSMiddleware_Headers(f *testing.F) {
	// Seed corpus: 5 categories per Ring fuzz standards
	// Category 1: Valid inputs
	f.Add("Origin,Content-Type,Accept,Authorization,X-Request-ID")
	f.Add("Content-Type")
	f.Add("Authorization,X-Custom-Header")

	// Category 2: Empty/boundary values
	f.Add("")
	f.Add(",")
	f.Add(strings.Repeat("X-Header,", 200))

	// Category 3: Unicode
	f.Add("X-\u65e5\u672c\u8a9e-Header")
	f.Add("\u00c9ncoding")

	// Category 4: Invalid formats
	f.Add("not a valid header name!!!")
	f.Add("Header\x00Name")
	f.Add("Header\tWith\tTabs")

	// Category 5: Security payloads
	f.Add("<script>alert(1)</script>")
	f.Add("Header\r\nX-Injected: true")
	f.Add("' OR 1=1 --")

	f.Fuzz(func(t *testing.T, allowedHeaders string) {
		// Bound input to prevent resource exhaustion
		if len(allowedHeaders) > 4096 {
			allowedHeaders = allowedHeaders[:4096]
		}

		cfg := CORSConfig{
			AllowedOrigins: "https://app.example.com",
			AllowedMethods: "GET,POST",
			AllowedHeaders: allowedHeaders,
		}

		req := httptest.NewRequest(http.MethodOptions, "/test", nil)
		req.Header.Set("Origin", "https://app.example.com")
		req.Header.Set("Access-Control-Request-Method", "GET")
		req.Header.Set("Access-Control-Request-Headers", "Content-Type")

		panicMsg, statusCode, err := safeMiddlewareTest(cfg, req)

		if panicMsg != "" {
			t.Errorf("CORSMiddleware panicked for headers=%q: %s",
				allowedHeaders, panicMsg)
			return
		}

		if err != nil {
			return
		}

		// Invariant: response status must always be valid HTTP
		if statusCode < 100 || statusCode >= 600 {
			t.Errorf("invalid HTTP status code %d for headers=%q",
				statusCode, allowedHeaders)
		}
	})
}
