// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
)

// newTestTelemetry returns a minimal Telemetry pointer whose LibraryName is
// set so the telemetry middleware can build a tracer without panicking.
func newTestTelemetry() *libOpentelemetry.Telemetry {
	return &libOpentelemetry.Telemetry{
		TelemetryConfig: libOpentelemetry.TelemetryConfig{LibraryName: "test"},
	}
}

// TestNewUnifiedServer_BaseEndpoints verifies that the baked-in routes
// (/health, /version) are registered and reachable on the returned app.
func TestNewUnifiedServer_BaseEndpoints(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	srv := NewUnifiedServer(":0", logger, newTestTelemetry())
	require.NotNil(t, srv)

	tests := []struct {
		name string
		path string
	}{
		{name: "health", path: "/health"},
		{name: "version", path: "/version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, http.NoBody)

			resp, err := srv.app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

// TestNewUnifiedServer_ServerAddress verifies the accessor.
func TestNewUnifiedServer_ServerAddress(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	srv := NewUnifiedServer(":12345", logger, newTestTelemetry())
	require.NotNil(t, srv)

	assert.Equal(t, ":12345", srv.ServerAddress())
}

// TestNewUnifiedServer_RouteRegistrarsInvoked verifies that every non-nil
// RouteRegistrar passed to NewUnifiedServer is invoked exactly once during
// construction, and that a nil registrar is safely skipped.
func TestNewUnifiedServer_RouteRegistrarsInvoked(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	var (
		firstCalls  int32
		secondCalls int32
	)

	first := RouteRegistrar(func(app *fiber.App) {
		atomic.AddInt32(&firstCalls, 1)
		app.Get("/first/ping", func(c *fiber.Ctx) error {
			return c.SendStatus(http.StatusOK)
		})
	})

	second := RouteRegistrar(func(app *fiber.App) {
		atomic.AddInt32(&secondCalls, 1)
		app.Get("/second/ping", func(c *fiber.Ctx) error {
			return c.SendStatus(http.StatusOK)
		})
	})

	// Pass a nil registrar alongside real ones — the server must skip nils
	// rather than panicking.
	srv := NewUnifiedServer(":0", logger, newTestTelemetry(), first, nil, second)
	require.NotNil(t, srv)

	assert.Equal(t, int32(1), atomic.LoadInt32(&firstCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&secondCalls))

	// Prove that both registered routes are reachable.
	for _, path := range []string{"/first/ping", "/second/ping"} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody)

		resp, err := srv.app.Test(req)
		require.NoError(t, err)

		require.NoError(t, resp.Body.Close())
		assert.Equal(t, http.StatusOK, resp.StatusCode, "path %s should be registered", path)
	}
}

// TestNewUnifiedServer_CORSHeadersPresent asserts that CORS middleware is
// attached to the unified server and that preflight OPTIONS requests return
// the expected Access-Control-* response headers. This catches regressions
// where CORS configuration is accidentally dropped.
func TestNewUnifiedServer_CORSHeadersPresent(t *testing.T) {
	// NOTE: no t.Parallel() — we rely on t.Setenv to configure CORS, which
	// mutates process-global state and is incompatible with parallel subtests
	// touching the same env var.
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	// Constrain CORS so we have a deterministic origin to assert against.
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com")

	srv := NewUnifiedServer(":0", logger, newTestTelemetry())
	require.NotNil(t, srv)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/health", http.NoBody)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)

	resp, err := srv.app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	// Fiber's CORS middleware should echo the origin when it matches the
	// allow-list. We don't pin the exact status code — both 200 and 204
	// are acceptable per spec, depending on middleware chain ordering.
	assert.Equal(t, "https://example.com", resp.Header.Get("Access-Control-Allow-Origin"))
}
