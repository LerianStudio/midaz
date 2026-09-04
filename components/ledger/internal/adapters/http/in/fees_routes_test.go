// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"sort"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// feesV2Scope is the /v2-mounted ledger scope every fee and billing route hangs off,
// in Fiber path syntax.
const feesV2Scope = "/v2/organizations/:organization_id/ledgers/:ledger_id"

// feesV2FullRoutes is the route surface the four fee registrars mount: the twelve
// organization-scoped fee and billing operations, re-scoped to a ledger. POST /fees is
// absent for the reason it is absent from /v1.
var feesV2FullRoutes = []string{
	"POST:" + feesV2Scope + "/packages",
	"GET:" + feesV2Scope + "/packages",
	"GET:" + feesV2Scope + "/packages/:id",
	"PATCH:" + feesV2Scope + "/packages/:id",
	"DELETE:" + feesV2Scope + "/packages/:id",
	"POST:" + feesV2Scope + "/estimates",
	"POST:" + feesV2Scope + "/billing-packages",
	"GET:" + feesV2Scope + "/billing-packages",
	"GET:" + feesV2Scope + "/billing-packages/:id",
	"PATCH:" + feesV2Scope + "/billing-packages/:id",
	"DELETE:" + feesV2Scope + "/billing-packages/:id",
	"POST:" + feesV2Scope + "/billing/calculate",
}

// feesV2OperationIDs is the operation ID each published v2 fee operation must carry,
// keyed by "METHOD path" in OpenAPI template syntax (group-relative: the op paths as
// registered, without the /v2 prefix the shared document adds). Every ID repeats its
// /v1 counterpart with the version suffix appended, which keeps the IDs unique within
// the shared document and across the ledger↔tracer hub-spec join — see v2OpSuffix.
var feesV2OperationIDs = map[string]string{
	"POST /organizations/{organization_id}/ledgers/{ledger_id}/packages":                "createPackageV2",
	"GET /organizations/{organization_id}/ledgers/{ledger_id}/packages":                 "getAllPackagesV2",
	"GET /organizations/{organization_id}/ledgers/{ledger_id}/packages/{id}":            "getPackageByIDV2",
	"PATCH /organizations/{organization_id}/ledgers/{ledger_id}/packages/{id}":          "updatePackageV2",
	"DELETE /organizations/{organization_id}/ledgers/{ledger_id}/packages/{id}":         "deletePackageV2",
	"POST /organizations/{organization_id}/ledgers/{ledger_id}/estimates":               "estimateFeeCalculationV2",
	"POST /organizations/{organization_id}/ledgers/{ledger_id}/billing-packages":        "createBillingPackageV2",
	"GET /organizations/{organization_id}/ledgers/{ledger_id}/billing-packages":         "getAllBillingPackagesV2",
	"GET /organizations/{organization_id}/ledgers/{ledger_id}/billing-packages/{id}":    "getBillingPackageByIDV2",
	"PATCH /organizations/{organization_id}/ledgers/{ledger_id}/billing-packages/{id}":  "updateBillingPackageV2",
	"DELETE /organizations/{organization_id}/ledgers/{ledger_id}/billing-packages/{id}": "deleteBillingPackageV2",
	"POST /organizations/{organization_id}/ledgers/{ledger_id}/billing/calculate":       "calculateBillingV2",
}

// mountFeesV2Routes wires the four fee registrars on a /v2 group, mirroring the
// production humaMountV2 seam: problem.Install() before any huma.Register, the Huma
// API built with openapi.New over the /v2 group, and the registrar attaching the Fiber
// guard chain plus the Huma terminals on that group.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError
// hook and Huma validation uses process-global sync.Pools.
func mountFeesV2Routes(app *fiber.App, auth *middleware.AuthClient, routeOptions *pkgHTTP.ProtectedRouteOptions) huma.API {
	libProblem.Install()
	apiV2 := app.Group("/v2")
	hAPI := openapi.New(app, apiV2, openapi.Config{Title: "ledger-fees-v2", Version: "test", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(hAPI)

	RegisterPackageV2RoutesToApp(apiV2, hAPI, auth, &PackageHandler{}, routeOptions)
	RegisterFeeEstimateV2RoutesToApp(apiV2, hAPI, auth, &FeeHandler{}, routeOptions)
	RegisterBillingPackageV2RoutesToApp(apiV2, hAPI, auth, &BillingPackageHandler{}, routeOptions)
	RegisterBillingCalculateV2RoutesToApp(apiV2, hAPI, auth, &BillingCalculateHandler{}, routeOptions)

	return hAPI
}

// TestFeesV2RoutesMountedOnGroup asserts the twelve ledger-scoped fee and billing ops
// are both SERVED on the /v2 Fiber group and PUBLISHED on the /v2 Huma document under
// the suffixed operation IDs. A missing route means the guard-chain attach or the Huma
// registration regressed; a missing or renamed operation ID means the two contracts
// would collide when the hub spec joins them.
func TestFeesV2RoutesMountedOnGroup(t *testing.T) {
	// NOT parallel: huma registration mutates process-global state.
	app := fiber.New()

	hAPI := mountFeesV2Routes(app, &middleware.AuthClient{Enabled: false}, nil)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	for _, w := range feesV2FullRoutes {
		assert.Truef(t, routeSet[w], "expected mounted route %q", w)
	}

	publishedIDs := make(map[string]string)

	for path, item := range hAPI.OpenAPI().Paths {
		for method, op := range humaPathOperations(item) {
			if op != nil {
				publishedIDs[method+" "+path] = op.OperationID
			}
		}
	}

	assert.Len(t, publishedIDs, len(feesV2OperationIDs),
		"the v2 fee contract must publish exactly the twelve operations")

	for where, wantID := range feesV2OperationIDs {
		assert.Equalf(t, wantID, publishedIDs[where], "operation ID published for %s", where)
	}
}

// TestFeesV2Routes_DoNotMountFeeCalculate asserts the fee-calculate
// endpoint stays unmounted on /v2 as it is on /v1: in the unified binary fees run
// in-process via the transaction seam, so only the dry-run estimate is exposed. Both
// the organization-scoped and the ledger-scoped spellings are checked, since the v2
// surface could plausibly have introduced either.
func TestFeesV2Routes_DoNotMountFeeCalculate(t *testing.T) {
	// NOT parallel: mutates process-global huma state.
	app := fiber.New()

	hAPI := mountFeesV2Routes(app, &middleware.AuthClient{Enabled: false}, nil)

	forbidden := []string{
		fiber.MethodPost + ":/v2/fees",
		fiber.MethodPost + ":/v2/organizations/:organization_id/fees",
		fiber.MethodPost + ":" + feesV2Scope + "/fees",
	}

	for _, r := range app.GetRoutes() {
		for _, f := range forbidden {
			assert.NotEqualf(t, f, r.Method+":"+r.Path,
				"%s must NOT be mounted — fees run in-process via the seam", f)
		}
	}

	for path := range hAPI.OpenAPI().Paths {
		assert.NotContainsf(t, path, "/fees",
			"the v2 contract must not publish a fee-calculate path, found %q", path)
	}
}

// TestFeesV2RoutesParameterNamesAgree is TestFeesRoutesParameterNamesAgree for the
// ledger-scoped surface, and it is what makes the new {ledger_id} segment safe. The
// spec-vs-routes gate collapses every parameter to a positional token, so it reads a
// chain declaring ":ledger" and a contract declaring "{ledger_id}" as the same route.
// The mismatch has a runtime cost: ParseUUIDPathParameters keys on the name the FIBER
// route declares and parses only the names constant.UUIDPathParameters lists, so a
// parameter spelled off-list reaches the handler as an unvalidated string while the
// request still routes and the chain still runs.
//
// Both halves are asserted per path: the two surfaces name their parameters
// identically, and every name they use is one the UUID validator recognizes.
func TestFeesV2RoutesParameterNamesAgree(t *testing.T) {
	// NOT parallel: huma registration mutates process-global state.
	app := fiber.New()

	api := mountFeesV2Routes(app, &middleware.AuthClient{Enabled: false}, nil)

	// Both surfaces land in the Fiber router — the guard chain directly, the Huma
	// terminal through the adapter — so a path structure that carries two different
	// parameter spellings shows up as two entries here. Keying by canonical structure
	// and collecting every spelling seen is what makes the disagreement visible.
	//
	// The key set comes from the document this registrar just wrote, so no fee path can
	// be silently left out of the sweep.
	spellings := make(map[string]map[string][]string)

	for path := range api.OpenAPI().Paths {
		spellings[canonicalizePath("/v2"+path)] = make(map[string][]string)
	}

	require.Len(t, spellings, 6, "the v2 fee surface publishes six distinct path structures")

	for _, r := range app.GetRoutes() {
		path := canonicalizePath(r.Path)
		if seen, ok := spellings[path]; ok {
			seen[strings.Join(r.Params, ",")] = r.Params
		}
	}

	for path, seen := range spellings {
		t.Run(strings.TrimPrefix(path, "/"), func(t *testing.T) {
			names := make([]string, 0, len(seen))
			for joined := range seen {
				names = append(names, joined)
			}

			sort.Strings(names)

			require.Lenf(t, names, 1,
				"%s: the guard chain and the contract must name their parameters identically, saw %v",
				path, names)

			for _, name := range seen[names[0]] {
				assert.Containsf(t, constant.UUIDPathParameters, name,
					"%s: parameter %q is not UUID-validated by ParseUUIDPathParameters", path, name)
			}
		})
	}
}

// TestFeesV2RoutesRunTheGuardChain asserts every v2 fee route actually EXECUTES the
// fees-scoped guard chain, which is the property the spec-vs-routes gate cannot see:
// that gate compares paths and methods and never inspects a route's middleware, so it
// stays green on a route mounted with no chain at all.
//
// The probe rides in PostAuthMiddlewares — the slot the production feesRouteOptions
// fills with the fee tenant chain — and records the path it ran on. A route whose
// terminal answers without the probe having run was registered on the Huma API without
// the Fiber chain in front of it.
func TestFeesV2RoutesRunTheGuardChain(t *testing.T) {
	// NOT parallel: huma registration mutates process-global state.
	var ran []string

	probe := func(c fiber.Ctx) error {
		ran = append(ran, c.Method()+":"+c.Route().Path)

		return c.Next()
	}

	app, stubs := buildFeesV2AppWithOptions(t,
		&pkgHTTP.ProtectedRouteOptions{PostAuthMiddlewares: []fiber.Handler{probe}})
	seedFeesV2Results(stubs)

	for _, route := range feesV2FullRoutes {
		method, path, ok := strings.Cut(route, ":")
		require.True(t, ok)

		ran = nil
		driveFeeV2Probe(t, app, method, path)

		assert.Containsf(t, ran, route,
			"%s must run the fees post-auth chain before its terminal", route)
	}
}
