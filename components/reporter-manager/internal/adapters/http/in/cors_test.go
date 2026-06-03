// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCORSMiddleware_UsesExplicitOrigins verifies that the CORS middleware
// configures allowed origins from the provided configuration instead of
// using wildcard defaults.
func TestCORSMiddleware_UsesExplicitOrigins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		allowedOrigins string
		requestOrigin  string
		expectAllowed  bool
		expectedOrigin string
	}{
		{
			name:           "Success - allowed origin gets CORS headers",
			allowedOrigins: "https://app.example.com,https://admin.example.com",
			requestOrigin:  "https://app.example.com",
			expectAllowed:  true,
			expectedOrigin: "https://app.example.com",
		},
		{
			name:           "Success - second allowed origin gets CORS headers",
			allowedOrigins: "https://app.example.com,https://admin.example.com",
			requestOrigin:  "https://admin.example.com",
			expectAllowed:  true,
			expectedOrigin: "https://admin.example.com",
		},
		{
			name:           "Error - disallowed origin gets no CORS headers",
			allowedOrigins: "https://app.example.com",
			requestOrigin:  "https://evil.example.com",
			expectAllowed:  false,
			expectedOrigin: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{
				DisableStartupMessage: true,
			})

			// CORSMiddleware must accept explicit origins from config.
			// This function does not exist yet -- test MUST fail.
			app.Use(CORSMiddleware(CORSConfig{
				AllowedOrigins: tt.allowedOrigins,
				AllowedMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
				AllowedHeaders: "Origin,Content-Type,Accept,Authorization,X-Request-ID",
			}))

			app.Get("/test", func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Origin", tt.requestOrigin)

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			corsHeader := resp.Header.Get("Access-Control-Allow-Origin")

			if tt.expectAllowed {
				assert.Equal(t, tt.expectedOrigin, corsHeader,
					"Expected Access-Control-Allow-Origin to be %q, got %q",
					tt.expectedOrigin, corsHeader)
			} else {
				assert.Empty(t, corsHeader,
					"Expected no Access-Control-Allow-Origin header for disallowed origin")
			}
		})
	}
}

// TestCORSMiddleware_PreflightRequest verifies that OPTIONS preflight requests
// are handled correctly with the configured CORS headers.
func TestCORSMiddleware_PreflightRequest(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// CORSMiddleware must accept explicit config.
	// This function does not exist yet -- test MUST fail.
	app.Use(CORSMiddleware(CORSConfig{
		AllowedOrigins: "https://app.example.com",
		AllowedMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowedHeaders: "Origin,Content-Type,Accept,Authorization,X-Request-ID",
	}))

	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type,Authorization")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	// Preflight should return 204 No Content
	assert.Equal(t, http.StatusNoContent, resp.StatusCode,
		"Preflight OPTIONS request should return 204")

	assert.Equal(t, "https://app.example.com",
		resp.Header.Get("Access-Control-Allow-Origin"),
		"Preflight must include Access-Control-Allow-Origin")

	assert.NotEmpty(t, resp.Header.Get("Access-Control-Allow-Methods"),
		"Preflight must include Access-Control-Allow-Methods")

	assert.NotEmpty(t, resp.Header.Get("Access-Control-Allow-Headers"),
		"Preflight must include Access-Control-Allow-Headers")
}

// TestCORSMiddleware_AllowedMethodsConfigured verifies that the CORS middleware
// returns the configured allowed methods in preflight responses.
func TestCORSMiddleware_AllowedMethodsConfigured(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// CORSMiddleware must accept explicit config.
	// This function does not exist yet -- test MUST fail.
	app.Use(CORSMiddleware(CORSConfig{
		AllowedOrigins: "https://app.example.com",
		AllowedMethods: "GET,POST,DELETE",
		AllowedHeaders: "Origin,Content-Type,Accept",
	}))

	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	allowedMethods := resp.Header.Get("Access-Control-Allow-Methods")
	assert.Contains(t, allowedMethods, "GET",
		"Allow-Methods must include GET")
	assert.Contains(t, allowedMethods, "POST",
		"Allow-Methods must include POST")
	assert.Contains(t, allowedMethods, "DELETE",
		"Allow-Methods must include DELETE")
}

// TestCORSConfig_Struct verifies that the CORSConfig struct exists with
// the expected fields for explicit CORS configuration.
func TestCORSConfig_Struct(t *testing.T) {
	t.Parallel()

	cfg := CORSConfig{
		AllowedOrigins: "https://app.example.com",
		AllowedMethods: "GET,POST",
		AllowedHeaders: "Origin,Content-Type",
	}

	assert.Equal(t, "https://app.example.com", cfg.AllowedOrigins)
	assert.Equal(t, "GET,POST", cfg.AllowedMethods)
	assert.Equal(t, "Origin,Content-Type", cfg.AllowedHeaders)
}

// TestCORSMiddleware_SkipsHealthAndSwaggerPaths verifies that CORS headers
// are not added to health, readiness, version, and swagger endpoints.
// These infrastructure paths do not serve cross-origin browser requests.
func TestCORSMiddleware_SkipsHealthAndSwaggerPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{name: "health endpoint skipped", path: "/health"},
		{name: "readyz endpoint skipped", path: "/readyz"},
		{name: "version endpoint skipped", path: "/version"},
		{name: "swagger root skipped", path: "/swagger/index.html"},
		{name: "swagger wildcard skipped", path: "/swagger/doc.json"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{
				DisableStartupMessage: true,
			})

			app.Use(CORSMiddleware(CORSConfig{
				AllowedOrigins: "https://app.example.com",
				AllowedMethods: "GET,POST",
				AllowedHeaders: "Origin,Content-Type",
			}))

			app.Get(tt.path, func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Origin", "https://app.example.com")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			corsHeader := resp.Header.Get("Access-Control-Allow-Origin")
			assert.Empty(t, corsHeader,
				"CORS headers should not be set for %s", tt.path)
		})
	}
}
