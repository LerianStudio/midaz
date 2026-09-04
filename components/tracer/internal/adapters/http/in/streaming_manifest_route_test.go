// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	authMiddleware "github.com/LerianStudio/lib-auth/v4/auth/middleware"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// newManifestTestHandler builds a real catalog-only lib-streaming manifest
// handler over a tiny fixed catalog so the route test exercises the actual lib
// hardening (headers, method allowlist) rather than a stub.
func newManifestTestHandler(t *testing.T) nethttp.Handler {
	t.Helper()

	catalog, err := libStreaming.NewCatalog(libStreaming.EventDefinition{
		Key:           "rule.created",
		ResourceType:  "rule",
		EventType:     "created",
		SchemaVersion: "1.0.0",
	})
	require.NoError(t, err, "manifest test catalog must build")

	handler, err := libStreaming.NewStreamingHandler(libStreaming.PublisherDescriptor{
		ServiceName: "tracer",
		Source:      "tracer",
		RoutePath:   pkgStreaming.ManifestRoutePath,
	}, catalog)
	require.NoError(t, err, "manifest test handler must build")

	return handler
}

// newManifestTestGuard builds an AuthGuard from the given config over a disabled
// plugin-auth client. With PluginAuthEnabled=false the guard falls back to the
// API-key middleware, whose enforcement is driven by APIKeyEnabled.
func newManifestTestGuard(t *testing.T, cfg middleware.AuthGuardConfig) *middleware.AuthGuard {
	t.Helper()

	authLogger := libLog.NewNop()
	authClient := authMiddleware.NewAuthClient("", cfg.PluginAuthEnabled, authLogger)
	guard := middleware.NewAuthGuard(cfg, authClient)
	require.NotNil(t, guard, "auth guard must build")

	return guard
}

// newManifestTestApp mounts the manifest route inside a /v1 group, mirroring the
// production mount in NewRoutes (the route lives under the protected /v1 group,
// never in the public probe block).
func newManifestTestApp(guard *middleware.AuthGuard, handler nethttp.Handler) *fiber.App {
	app := fiber.New()
	api := app.Group("/v1")
	RegisterStreamingManifestRoute(api, guard, handler)

	return app
}

func TestRegisterStreamingManifestRoute_GetServesJSONWithHardening(t *testing.T) {
	t.Parallel()

	guard := newManifestTestGuard(t, middleware.AuthGuardConfig{AppName: "tracer"})
	app := newManifestTestApp(guard, newManifestTestHandler(t))

	req := httptest.NewRequest(nethttp.MethodGet, pkgStreaming.ManifestRoutePath, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, nethttp.StatusOK, resp.StatusCode, "GET manifest must return 200")
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
}

func TestRegisterStreamingManifestRoute_PostReturns405(t *testing.T) {
	t.Parallel()

	guard := newManifestTestGuard(t, middleware.AuthGuardConfig{AppName: "tracer"})
	app := newManifestTestApp(guard, newManifestTestHandler(t))

	req := httptest.NewRequest(nethttp.MethodPost, pkgStreaming.ManifestRoutePath, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, nethttp.StatusMethodNotAllowed, resp.StatusCode, "POST manifest must return 405")
}

// TestRegisterStreamingManifestRoute_ProtectedByGuard proves the route lives
// under the protected /v1 group and is NOT in the public probe block: with the
// API key enforced, a request that omits the key is rejected with 401 before the
// manifest handler runs.
func TestRegisterStreamingManifestRoute_ProtectedByGuard(t *testing.T) {
	t.Parallel()

	guard := newManifestTestGuard(t, middleware.AuthGuardConfig{
		APIKey:        "test-secret-key-32-characters-long",
		APIKeyEnabled: true,
		AppName:       "tracer",
	})
	app := newManifestTestApp(guard, newManifestTestHandler(t))

	req := httptest.NewRequest(nethttp.MethodGet, pkgStreaming.ManifestRoutePath, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, nethttp.StatusUnauthorized, resp.StatusCode,
		"manifest route must be protected (401 without the API key), proving it is not in the public block")
}

func TestRegisterStreamingManifestRoute_NilHandlerNotMounted(t *testing.T) {
	t.Parallel()

	guard := newManifestTestGuard(t, middleware.AuthGuardConfig{AppName: "tracer"})
	app := newManifestTestApp(guard, nil)

	req := httptest.NewRequest(nethttp.MethodGet, pkgStreaming.ManifestRoutePath, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, nethttp.StatusNotFound, resp.StatusCode, "a nil handler must leave the route unmounted (404)")
}
