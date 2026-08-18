// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	libStreaming "github.com/LerianStudio/lib-streaming/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// newManifestTestHandler builds a real catalog-only lib-streaming manifest
// handler over a tiny fixed catalog so the route test exercises the actual lib
// hardening (headers, method allowlist) rather than a stub.
func newManifestTestHandler(t *testing.T) nethttp.Handler {
	t.Helper()

	catalog, err := libStreaming.NewCatalog(libStreaming.EventDefinition{
		Key:           "account.created",
		ResourceType:  "account",
		EventType:     "created",
		SchemaVersion: "1.0.0",
	})
	require.NoError(t, err, "manifest test catalog must build")

	handler, err := libStreaming.NewStreamingHandler(libStreaming.PublisherDescriptor{
		ServiceName: "ledger",
		SourceBase:  "ledger",
		RoutePath:   pkgStreaming.ManifestRoutePath,
	}, catalog)
	require.NoError(t, err, "manifest test handler must build")

	return handler
}

func newManifestTestApp(auth *middleware.AuthClient, handler nethttp.Handler) *fiber.App {
	app := fiber.New()
	RegisterStreamingManifestRouteToApp(app, auth, nil, handler)

	return app
}

func TestRegisterStreamingManifestRouteToApp_GetServesJSONWithHardening(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: false}
	app := newManifestTestApp(auth, newManifestTestHandler(t))

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

func TestRegisterStreamingManifestRouteToApp_PostReturns405(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: false}
	app := newManifestTestApp(auth, newManifestTestHandler(t))

	req := httptest.NewRequest(nethttp.MethodPost, pkgStreaming.ManifestRoutePath, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, nethttp.StatusMethodNotAllowed, resp.StatusCode, "POST manifest must return 405")
}

func TestRegisterStreamingManifestRouteToApp_NilHandlerNotMounted(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: false}
	app := newManifestTestApp(auth, nil)

	req := httptest.NewRequest(nethttp.MethodGet, pkgStreaming.ManifestRoutePath, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, nethttp.StatusNotFound, resp.StatusCode, "a nil handler must leave the route unmounted (404)")
}
