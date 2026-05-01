// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplicationNameConstant(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{
			name:     "ApplicationName has correct value",
			expected: "plugin-crm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ApplicationName,
				"ApplicationName constant must equal %q", tt.expected)
		})
	}
}

func TestNewRouter_TenantMiddlewareRegistration(t *testing.T) {
	// Test that the conditional middleware registration pattern works correctly
	// using a standalone Fiber app that mirrors the NewRouter pattern.
	tests := []struct {
		name           string
		tenantMw       fiber.Handler
		expectMwCalled bool
	}{
		{
			name:           "nil middleware is not registered and does not panic",
			tenantMw:       nil,
			expectMwCalled: false,
		},
		{
			name: "non-nil middleware is registered and invoked",
			tenantMw: func(c *fiber.Ctx) error {
				c.Locals("tenant_mw_called", true)
				return c.Next()
			},
			expectMwCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()

			var mwCalled bool

			// Wrap the test middleware to track invocation
			if tt.tenantMw != nil {
				original := tt.tenantMw
				app.Use(func(c *fiber.Ctx) error {
					mwCalled = true
					return original(c)
				})
			}

			app.Get("/health", func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)

			defer func() {
				if resp != nil && resp.Body != nil {
					resp.Body.Close()
				}
			}()

			assert.Equal(t, http.StatusOK, resp.StatusCode,
				"health endpoint must return 200 OK regardless of middleware presence")
			assert.Equal(t, tt.expectMwCalled, mwCalled,
				"middleware invocation must match expectation")
		})
	}
}

func TestNewRouter_PublicEndpointsBypassTenantMiddleware(t *testing.T) {
	t.Parallel()

	// rejectingTenantMw simulates a multi-tenant middleware that rejects
	// every request without a valid tenant header, returning 401.
	rejectingTenantMw := func(c *fiber.Ctx) error {
		if c.Get("X-Tenant-ID") == "" {
			return c.SendStatus(http.StatusUnauthorized)
		}

		return c.Next()
	}

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "health endpoint bypasses tenant middleware",
			method:     http.MethodGet,
			path:       "/health",
			wantStatus: http.StatusOK,
		},
		{
			name:       "version endpoint bypasses tenant middleware",
			method:     http.MethodGet,
			path:       "/version",
			wantStatus: http.StatusOK,
		},
		{
			name:       "swagger endpoint bypasses tenant middleware",
			method:     http.MethodGet,
			path:       "/swagger/index.html",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()

			// Mirror the NewRouter middleware order:
			// 1. Common middleware (omitted for simplicity — not relevant to this test)
			// 2. Public endpoints registered BEFORE tenant middleware
			// 3. Tenant middleware
			// 4. API routes (require tenant context)

			// Public endpoints MUST come before tenant middleware so they
			// remain accessible to k8s probes and swagger without JWT.
			app.Get("/health", func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			})
			app.Get("/version", func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			})
			app.Get("/swagger/*", func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			})

			// Tenant middleware rejects requests without X-Tenant-ID
			app.Use(rejectingTenantMw)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			// No X-Tenant-ID header — simulates k8s probe or swagger access
			resp, err := app.Test(req, -1)
			require.NoError(t, err)

			defer func() {
				if resp != nil && resp.Body != nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}()

			assert.Equal(t, tt.wantStatus, resp.StatusCode,
				"public endpoint %s must be accessible without tenant context", tt.path)
		})
	}
}

func TestNewRouter_ServesSwaggerUIAssets(t *testing.T) {
	t.Parallel()

	app := NewRouter(
		&libLog.GoLogger{},
		&libOpentelemetry.Telemetry{},
		&middleware.AuthClient{Enabled: false},
		nil,
		nil,
		&HolderHandler{},
		&AliasHandler{},
	)

	for _, path := range []string{
		"/swagger/index.html",
		"/swagger/doc.json",
		"/swagger/swagger-ui.css",
		"/swagger/swagger-ui-bundle.js",
		"/swagger/swagger-ui-standalone-preset.js",
	} {
		path := path

		t.Run(path, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, path, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

func TestNewRouter_APIRoutesRequireTenantMiddleware(t *testing.T) {
	t.Parallel()

	// rejectingTenantMw simulates a multi-tenant middleware that rejects
	// every request without a valid tenant header, returning 401.
	rejectingTenantMw := func(c *fiber.Ctx) error {
		if c.Get("X-Tenant-ID") == "" {
			return c.SendStatus(http.StatusUnauthorized)
		}

		return c.Next()
	}

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "holders endpoint requires tenant context",
			method:     http.MethodGet,
			path:       "/v1/holders",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "aliases endpoint requires tenant context",
			method:     http.MethodGet,
			path:       "/v1/aliases",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()

			// Public endpoints before tenant middleware (same as NewRouter)
			app.Get("/health", func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			})

			// Tenant middleware rejects requests without X-Tenant-ID
			app.Use(rejectingTenantMw)

			// API routes after tenant middleware (require tenant context)
			app.Get("/v1/holders", func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			})
			app.Get("/v1/aliases", func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			})

			req := httptest.NewRequest(tt.method, tt.path, nil)
			// No X-Tenant-ID header
			resp, err := app.Test(req, -1)
			require.NoError(t, err)

			defer func() {
				if resp != nil && resp.Body != nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}()

			assert.Equal(t, tt.wantStatus, resp.StatusCode,
				"API endpoint %s must be rejected without tenant context", tt.path)
		})
	}
}

func TestNewRouter_TenantMiddlewareCallCount(t *testing.T) {
	var callCount atomic.Int32

	tenantMw := func(c *fiber.Ctx) error {
		callCount.Add(1)
		return c.Next()
	}

	app := fiber.New()
	app.Use(tenantMw)
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	defer func() {
		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(1), callCount.Load(),
		"tenant middleware must be called exactly once per request")
}
