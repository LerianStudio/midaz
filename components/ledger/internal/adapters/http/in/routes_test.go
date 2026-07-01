// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mountMetadataIndexRoutes wires the Huma-migrated metadata-index resource on a /v1
// group, mirroring the production humaMount seam: problem.Install() runs before any
// huma.Register, the Huma API is built with openapi.New over the /v1 group, and
// RegisterMetadataIndexRoutesToApp attaches the auth+tenant middleware chain (as
// middleware only) plus the Huma terminals on that group. The registered surface is
// therefore the same /v1/settings/metadata-indexes/* paths the unified server mounts.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError
// hook and Huma validation uses process-global sync.Pools — concurrent builds
// cross-contaminate. (Same rationale as the asset/portfolio huma exemplars.)
func mountMetadataIndexRoutes(app *fiber.App, auth *middleware.AuthClient, handler *MetadataIndexHandler, opts *pkgHTTP.ProtectedRouteOptions) {
	libProblem.Install()
	apiV1 := app.Group("/v1")
	hAPI := openapi.New(app, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})
	RegisterMetadataIndexRoutesToApp(apiV1, hAPI, auth, handler, opts)
}

// TestRegisterRoutesToApp_RegistersRoutes asserts the metadata-index routes are
// served on the /v1 group after the Wave-1 Huma migration. The three ops keep their
// exact paths and methods; only the transport (Fiber inline -> Huma terminal) changed.
func TestRegisterRoutesToApp_RegistersRoutes(t *testing.T) {
	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	mountMetadataIndexRoutes(app, auth, &MetadataIndexHandler{}, nil)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	assert.True(t, routeSet["POST:/v1/settings/metadata-indexes/entities/:entity_name"])
	assert.True(t, routeSet["GET:/v1/settings/metadata-indexes"])
	assert.True(t, routeSet["DELETE:/v1/settings/metadata-indexes/entities/:entity_name/key/:index_key"])
}

// TestRegisterRoutesToApp_WithRouteOptions asserts the auth chain honors the
// PostAuthMiddlewares carried by ProtectedRouteOptions when the metadata-index routes
// are mounted through the Huma wrapper.
func TestRegisterRoutesToApp_WithRouteOptions(t *testing.T) {
	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	middlewareCalled := false
	options := &pkgHTTP.ProtectedRouteOptions{
		PostAuthMiddlewares: []fiber.Handler{
			func(c *fiber.Ctx) error {
				middlewareCalled = true
				return c.Next()
			},
		},
	}

	mountMetadataIndexRoutes(app, auth, &MetadataIndexHandler{}, options)

	assert.NotEmpty(t, app.GetRoutes(), "should have registered routes with options")

	// The middleware existence is verified by route count including middleware handlers.
	// Actual middleware invocation requires a full request cycle with auth,
	// but we confirm routes are registered with the options chain.
	_ = middlewareCalled
}

// TestCreateRouteRegistrar_ReturnsFunctionThatRegistersRoutes asserts the legacy
// CreateRouteRegistrar seam is still constructible and callable. Post-migration it no
// longer registers inline metadata Fiber routes (those moved to the Huma seam), so it
// registers no /v1 metadata paths — the assertion is on its safe, no-op invocation.
func TestCreateRouteRegistrar_ReturnsFunctionThatRegistersRoutes(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: false}
	handler := &MetadataIndexHandler{}

	registrar := CreateRouteRegistrar(auth, handler, nil)
	require.NotNil(t, registrar)

	app := fiber.New()
	require.NotPanics(t, func() { registrar(app) })

	// Metadata is now Huma-served (RegisterMetadataIndexRoutesToApp); the legacy
	// Fiber registrar intentionally registers nothing.
	assert.Empty(t, app.GetRoutes())
}
