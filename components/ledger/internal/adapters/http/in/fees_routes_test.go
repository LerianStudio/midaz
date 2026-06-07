// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// feeRoutes is the canonical fee/billing route table mounted on the unified
// ledger server. POST /v1/fees is intentionally absent: in the unified binary
// fees run in-process via the transaction seam, so only POST /v1/estimates
// (dry-run) is exposed. Organization is path-scoped: every route carries
// :organization_id, validated as a UUID by ParseUUIDPathParameters.
var feeRoutes = []struct {
	method string
	path   string
}{
	{fiber.MethodPost, "/v1/organizations/:organization_id/packages"},
	{fiber.MethodGet, "/v1/organizations/:organization_id/packages"},
	{fiber.MethodGet, "/v1/organizations/:organization_id/packages/:id"},
	{fiber.MethodPatch, "/v1/organizations/:organization_id/packages/:id"},
	{fiber.MethodDelete, "/v1/organizations/:organization_id/packages/:id"},
	{fiber.MethodPost, "/v1/organizations/:organization_id/estimates"},
	{fiber.MethodPost, "/v1/organizations/:organization_id/billing-packages"},
	{fiber.MethodGet, "/v1/organizations/:organization_id/billing-packages"},
	{fiber.MethodGet, "/v1/organizations/:organization_id/billing-packages/:id"},
	{fiber.MethodPatch, "/v1/organizations/:organization_id/billing-packages/:id"},
	{fiber.MethodDelete, "/v1/organizations/:organization_id/billing-packages/:id"},
	{fiber.MethodPost, "/v1/organizations/:organization_id/billing/calculate"},
}

// concreteFeePath substitutes fixed UUID literals for the path parameters so a
// route-pattern entry becomes a requestable URL.
func concreteFeePath(path string) string {
	path = strings.ReplaceAll(path, ":organization_id", "11111111-1111-1111-1111-111111111111")
	path = strings.ReplaceAll(path, ":id", "22222222-2222-2222-2222-222222222222")

	return path
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
// path instead of the disabled passthrough. Path params use valid UUIDs so the
// test asserts exactly one thing: auth ordering, not path validation.
func TestRegisterFeesRoutesToApp_RoutesAreAuthProtected(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"}
	ph, fh, bph, bch := newFeesTestHandlers()

	RegisterFeesRoutesToApp(app, auth, ph, fh, bph, bch, nil)

	for _, route := range feeRoutes {
		route := route

		t.Run(route.method+" "+route.path, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(route.method, concreteFeePath(route.path), nil)
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

// TestRegisterFeesRoutesToApp_PathValidation drives the REAL registered chain
// (auth disabled passthrough) and pins the path-validation contract:
// a malformed UUID segment is rejected with the canonical midaz
// ErrInvalidPathParameter envelope, replacing the former FEE-shim codes
// (FEE-0019/FEE-0020). A genuinely MISSING org can no longer be expressed —
// the route does not match and Fiber returns 404 — so the former FEE-0020
// "missing header" semantics become "malformed segment → canonical 400".
func TestRegisterFeesRoutesToApp_PathValidation(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}
	ph, fh, bph, bch := newFeesTestHandlers()

	RegisterFeesRoutesToApp(app, auth, ph, fh, bph, bch, nil)

	const validOrg = "11111111-1111-1111-1111-111111111111"

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "non-UUID organization_id on packages list returns canonical 400",
			method: fiber.MethodGet,
			path:   "/v1/organizations/not-a-uuid/packages",
		},
		{
			name:   "non-UUID organization_id on billing-packages list returns canonical 400",
			method: fiber.MethodGet,
			path:   "/v1/organizations/not-a-uuid/billing-packages",
		},
		{
			name:   "non-UUID package id returns canonical 400",
			method: fiber.MethodGet,
			path:   "/v1/organizations/" + validOrg + "/packages/not-a-uuid",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tc.method, tc.path, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var errResp map[string]any
			require.NoError(t, json.Unmarshal(body, &errResp))
			assert.Equal(t, cn.ErrInvalidPathParameter.Error(), errResp["code"],
				"path validation must emit the canonical midaz code, not a FEE-shim code")
		})
	}

	t.Run("valid organization_id passes path validation and reaches body binding", func(t *testing.T) {
		t.Parallel()

		// Empty body on a create: the chain must get PAST ParseUUIDPathParameters
		// and fail inside the body binder instead — proving a valid org admits
		// the request. The binder's code (whatever it is) must not be the
		// path-validation code.
		req := httptest.NewRequest(fiber.MethodPost, "/v1/organizations/"+validOrg+"/packages", nil)
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, -1)
		require.NoError(t, err)

		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var errResp map[string]any
		require.NoError(t, json.Unmarshal(body, &errResp))
		assert.NotEqual(t, cn.ErrInvalidPathParameter.Error(), errResp["code"],
			"a valid org segment must pass path validation (failure here means the request died at the path validator)")
	})
}
