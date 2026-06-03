// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// feeRoutes is the canonical fee/billing route table mounted on the unified
// ledger server. POST /v1/fees is intentionally absent: in the unified binary
// fees run in-process via the transaction seam, so only POST /v1/estimates
// (dry-run) is exposed.
var feeRoutes = []struct {
	method string
	path   string
}{
	{fiber.MethodPost, "/v1/packages"},
	{fiber.MethodGet, "/v1/packages"},
	{fiber.MethodGet, "/v1/packages/:id"},
	{fiber.MethodPatch, "/v1/packages/:id"},
	{fiber.MethodDelete, "/v1/packages/:id"},
	{fiber.MethodPost, "/v1/estimates"},
	{fiber.MethodPost, "/v1/billing-packages"},
	{fiber.MethodGet, "/v1/billing-packages"},
	{fiber.MethodGet, "/v1/billing-packages/:id"},
	{fiber.MethodPatch, "/v1/billing-packages/:id"},
	{fiber.MethodDelete, "/v1/billing-packages/:id"},
	{fiber.MethodPost, "/v1/billing/calculate"},
}

func newFeesTestHandlers() (*PackageHandler, *FeeHandler, *BillingPackageHandler, *BillingCalculateHandler) {
	return &PackageHandler{}, &FeeHandler{}, &BillingPackageHandler{}, &BillingCalculateHandler{}
}

func TestRegisterFeesRoutesToApp_RegistersEveryRoute(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}
	ph, fh, bph, bch := newFeesTestHandlers()

	RegisterFeesRoutesToApp(app, auth, ph, fh, bph, bch, nil)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	for _, route := range feeRoutes {
		assert.Truef(t, routeSet[route.method+":"+route.path],
			"fee route %s %s must be registered on the unified app", route.method, route.path)
	}
}

func TestRegisterFeesRoutesToApp_DoesNotMountFeeCalculate(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}
	ph, fh, bph, bch := newFeesTestHandlers()

	RegisterFeesRoutesToApp(app, auth, ph, fh, bph, bch, nil)

	for _, r := range app.GetRoutes() {
		assert.NotEqualf(t, fiber.MethodPost+":/v1/fees", r.Method+":"+r.Path,
			"POST /v1/fees must NOT be mounted — fees run in-process via the seam")
	}
}

func TestCreateFeesRouteRegistrar_RegistersEveryRoute(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: false}
	ph, fh, bph, bch := newFeesTestHandlers()

	registrar := CreateFeesRouteRegistrar(auth, ph, fh, bph, bch, nil)
	require.NotNil(t, registrar)

	app := fiber.New()
	registrar(app)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	for _, route := range feeRoutes {
		assert.Truef(t, routeSet[route.method+":"+route.path],
			"fee route %s %s must be registered via the registrar", route.method, route.path)
	}
}

// TestRegisterFeesRoutesToApp_RoutesAreAuthProtected asserts that every mounted
// fee/billing route runs the auth middleware first: an unauthenticated request
// is blocked with 401 (auth gate), NOT 404 (route absent) and NOT 200 (handler
// reached). Address must be non-empty so the auth client takes the enforcing
// path instead of the disabled passthrough.
func TestRegisterFeesRoutesToApp_RoutesAreAuthProtected(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"}
	ph, fh, bph, bch := newFeesTestHandlers()

	RegisterFeesRoutesToApp(app, auth, ph, fh, bph, bch, nil)

	for _, route := range feeRoutes {
		route := route

		// Use a concrete ID so :id routes resolve to the registered path.
		path := route.path
		if path == "/v1/packages/:id" {
			path = "/v1/packages/00000000-0000-0000-0000-000000000000"
		}

		if path == "/v1/billing-packages/:id" {
			path = "/v1/billing-packages/00000000-0000-0000-0000-000000000000"
		}

		t.Run(route.method+" "+route.path, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(route.method, path, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)

			defer func() {
				if resp != nil && resp.Body != nil {
					resp.Body.Close()
				}
			}()

			assert.Equalf(t, fiber.StatusUnauthorized, resp.StatusCode,
				"fee route %s %s must be auth-protected (401), not unprotected or unmounted (%d)",
				route.method, route.path, resp.StatusCode)
		})
	}
}
