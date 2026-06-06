// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	midazHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		method         string
		path           string
		expectedHeader string
		expectedValue  string
	}{
		{
			name:           "X-Content-Type-Options header is set to nosniff",
			method:         http.MethodGet,
			path:           "/health",
			expectedHeader: "X-Content-Type-Options",
			expectedValue:  "nosniff",
		},
		{
			name:           "X-Frame-Options header is set to DENY",
			method:         http.MethodGet,
			path:           "/health",
			expectedHeader: "X-Frame-Options",
			expectedValue:  "DENY",
		},
		{
			name:           "X-XSS-Protection header is set to 0",
			method:         http.MethodGet,
			path:           "/health",
			expectedHeader: "X-XSS-Protection",
			expectedValue:  "0",
		},
		{
			name:           "Strict-Transport-Security header is set with max-age and includeSubDomains",
			method:         http.MethodGet,
			path:           "/health",
			expectedHeader: "Strict-Transport-Security",
			expectedValue:  "max-age=31536000; includeSubDomains",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{
				DisableStartupMessage: true,
			})

			// Apply SecurityHeaders middleware (does not exist yet - test must FAIL)
			app.Use(SecurityHeaders())

			app.Get("/health", func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			})

			req := httptest.NewRequest(tt.method, tt.path, nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			headerValue := resp.Header.Get(tt.expectedHeader)
			assert.Equal(t, tt.expectedValue, headerValue,
				"Expected header %s to be %q, got %q",
				tt.expectedHeader, tt.expectedValue, headerValue,
			)
		})
	}
}

func TestSecurityHeaders_AllHeadersOnSingleResponse(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// Apply SecurityHeaders middleware (does not exist yet - test must FAIL)
	app.Use(SecurityHeaders())

	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// All four security headers must be present on a single response
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"),
		"X-Content-Type-Options must be nosniff")
	assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"),
		"X-Frame-Options must be DENY")
	assert.Equal(t, "0", resp.Header.Get("X-XSS-Protection"),
		"X-XSS-Protection must be 0")
	assert.Equal(t, "max-age=31536000; includeSubDomains", resp.Header.Get("Strict-Transport-Security"),
		"Strict-Transport-Security must enforce HSTS with includeSubDomains")
}

func TestRecoverMiddleware_PanicDoesNotCrashServer(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// Apply RecoverMiddleware (does not exist yet - test must FAIL).
	// This function should wrap Fiber's recover.New() so we can test
	// that it is wired into the middleware stack.
	app.Use(RecoverMiddleware())

	app.Get("/panic", func(_ *fiber.Ctx) error {
		panic("intentional test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	resp, err := app.Test(req)

	require.NoError(t, err, "Server must not crash on panic when recover middleware is present")
	defer resp.Body.Close()

	// Fiber's recover middleware returns 500 Internal Server Error by default
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode,
		"Recovered panic should return 500, not crash the server")
}

func TestMiddlewareOrdering_AuthBeforeTenant(t *testing.T) {
	t.Parallel()

	// Track execution order of middleware
	var executionOrder []string

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// Simulate auth middleware that records its execution
	fakeAuth := func(c *fiber.Ctx) error {
		executionOrder = append(executionOrder, "auth")
		return c.Next()
	}

	// Simulate tenant middleware that records its execution
	fakeTenant := func(c *fiber.Ctx) error {
		executionOrder = append(executionOrder, "tenant")
		return c.Next()
	}

	// Use WhenEnabled to conditionally apply tenant middleware after auth
	app.Get("/v1/test", fakeAuth, WhenEnabled(fakeTenant), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, executionOrder, 2, "both auth and tenant middleware must execute")
	assert.Equal(t, "auth", executionOrder[0], "auth must run BEFORE tenant")
	assert.Equal(t, "tenant", executionOrder[1], "tenant must run AFTER auth")
}

func TestMiddlewareOrdering_WithTenantNil_OnlyAuth(t *testing.T) {
	t.Parallel()

	var executionOrder []string

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	fakeAuth := func(c *fiber.Ctx) error {
		executionOrder = append(executionOrder, "auth")
		return c.Next()
	}

	// When tenantMiddleware is nil (single-tenant mode), WhenEnabled skips it
	app.Get("/v1/test", fakeAuth, WhenEnabled(nil), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, executionOrder, 1, "only auth middleware should execute")
	assert.Equal(t, "auth", executionOrder[0])
}

func TestRecoverMiddleware_NonPanicRouteUnaffected(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// Apply RecoverMiddleware (does not exist yet - test must FAIL)
	app.Use(RecoverMiddleware())

	app.Get("/ok", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"Non-panicking routes must work normally with recover middleware")
}

// TestProtectedRouteChain_Composition proves the central chain-composition
// helper that every protected reporter-manager route registers through:
// auth runs first and short-circuits unauthenticated requests before any
// handler executes, and on the authenticated path the post-auth tenant
// middleware and the UUID path parser run in order before the business
// handler. This is the no-behavior-change proof for the ProtectedRouteChain
// adoption (Epic 3.1).
func TestProtectedRouteChain_Composition(t *testing.T) {
	t.Parallel()

	t.Run("auth short-circuits before tenant, parse, and handler", func(t *testing.T) {
		t.Parallel()

		var order []string

		denyingAuth := func(c *fiber.Ctx) error {
			order = append(order, "auth")
			return fiber.NewError(http.StatusUnauthorized, "Unauthorized")
		}
		tenant := func(c *fiber.Ctx) error {
			order = append(order, "tenant")
			return c.Next()
		}
		handler := func(c *fiber.Ctx) error {
			order = append(order, "handler")
			return c.SendStatus(http.StatusOK)
		}

		opts := &midazHTTP.ProtectedRouteOptions{
			PostAuthMiddlewares: []fiber.Handler{WhenEnabled(tenant)},
		}
		chain := midazHTTP.ProtectedRouteChain(denyingAuth, opts, ParsePathParametersUUID, handler)

		app := fiber.New(fiber.Config{
			DisableStartupMessage: true,
			ErrorHandler:          legacyFiberErrorHandler,
		})
		app.Get("/v1/templates/:id", chain...)

		req := httptest.NewRequest(http.MethodGet, "/v1/templates/"+uuid.NewString(), nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.Equal(t, []string{"auth"}, order,
			"only auth must run; tenant, UUID parse, and handler must not execute on rejection")
	})

	t.Run("authenticated path runs auth, tenant, parse, handler in order", func(t *testing.T) {
		t.Parallel()

		var order []string

		allowingAuth := func(c *fiber.Ctx) error {
			order = append(order, "auth")
			return c.Next()
		}
		tenant := func(c *fiber.Ctx) error {
			order = append(order, "tenant")
			return c.Next()
		}
		handler := func(c *fiber.Ctx) error {
			order = append(order, "handler")

			parsed, ok := c.Locals("id").(uuid.UUID)
			require.True(t, ok, "UUID path param must be parsed into Locals before the handler")
			assert.NotEqual(t, uuid.Nil, parsed)

			return c.SendStatus(http.StatusOK)
		}

		opts := &midazHTTP.ProtectedRouteOptions{
			PostAuthMiddlewares: []fiber.Handler{WhenEnabled(tenant)},
		}
		chain := midazHTTP.ProtectedRouteChain(allowingAuth, opts, ParsePathParametersUUID, handler)

		app := fiber.New(fiber.Config{
			DisableStartupMessage: true,
			ErrorHandler:          legacyFiberErrorHandler,
		})
		app.Get("/v1/templates/:id", chain...)

		req := httptest.NewRequest(http.MethodGet, "/v1/templates/"+uuid.NewString(), nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, []string{"auth", "tenant", "handler"}, order,
			"auth then tenant then handler; UUID parse runs between tenant and handler")
	})

	t.Run("nil tenant middleware is skipped via WhenEnabled", func(t *testing.T) {
		t.Parallel()

		var order []string

		allowingAuth := func(c *fiber.Ctx) error {
			order = append(order, "auth")
			return c.Next()
		}
		handler := func(c *fiber.Ctx) error {
			order = append(order, "handler")
			return c.SendStatus(http.StatusOK)
		}

		opts := &midazHTTP.ProtectedRouteOptions{
			PostAuthMiddlewares: []fiber.Handler{WhenEnabled(nil)},
		}
		chain := midazHTTP.ProtectedRouteChain(allowingAuth, opts, handler)

		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Get("/v1/metrics", chain...)

		req := httptest.NewRequest(http.MethodGet, "/v1/metrics", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, []string{"auth", "handler"}, order,
			"single-tenant mode: nil tenant middleware is skipped, auth then handler only")
	})
}

func TestManagerHealthHandler_ReturnsAlive(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/health", NewManagerHealthHandler(nil))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.JSONEq(t, `{"status":"alive"}`, string(body))
}

func TestLegacyFiberErrorHandler_RouteNotFound_UsesLegacyShape(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          legacyFiberErrorHandler,
	})

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "Cannot GET /missing", body["error"])
	assert.Len(t, body, 1)
}

func TestLegacyFiberErrorHandler_MethodNotAllowed_UsesLegacyShape(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          legacyFiberErrorHandler,
	})
	app.Get("/health", NewManagerHealthHandler(nil))

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	assert.Equal(t, "Method Not Allowed", body["error"])
	assert.Len(t, body, 1)
}

// TestReadyzHandler_NilDeps_ReturnsUnhealthyWithoutPanic verifies that
// mounting NewManagerReadyzHandler with nil deps does not panic and reports
// 503 unhealthy (because all required connections are missing).
func TestReadyzHandler_NilDeps_ReturnsUnhealthyWithoutPanic(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewManagerReadyzHandler(nil))

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, "unhealthy", body["status"])
}
