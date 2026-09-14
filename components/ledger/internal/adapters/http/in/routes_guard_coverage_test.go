// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// guardCoverageParam matches one Fiber path parameter segment, including the optional
// '?' and the '+'/'*' greedy forms, so a concrete request path can be built for any route
// the mount produces.
var guardCoverageParam = regexp.MustCompile(`:[A-Za-z_][A-Za-z0-9_]*\??`)

// guardCoverageConcretePath turns a Fiber route pattern into a request path by filling
// every parameter with a UUID. The value never reaches a handler — the authorization
// check refuses first — it only has to be something Fiber routes and the UUID-parsing
// middlewares accept.
func guardCoverageConcretePath(pattern string) string {
	const filler = "00000000-0000-4000-8000-000000000000"

	path := guardCoverageParam.ReplaceAllString(pattern, filler)
	path = strings.ReplaceAll(path, "/*", "/"+filler)
	path = strings.ReplaceAll(path, "/+", "/"+filler)

	return path
}

// guardCoveragePublicRoutes are the paths that are deliberately served WITHOUT the midaz
// authorization check. Every entry is a decision someone made on purpose, which is the
// whole point of listing them here: a route that appears in the mount and is not in this
// list has to be guarded, and a route added to this list is a reviewable line in a diff
// rather than an omission nobody sees.
//
// Empty today: the ledger mount below serves no public path.
var guardCoveragePublicRoutes = map[string]bool{}

// guardCoverageMinimumRoutes is a floor on how many distinct METHOD+path entries the
// mount is expected to produce, well under the count today so ordinary additions and
// removals do not touch it. It exists only to catch the sweep going empty.
const guardCoverageMinimumRoutes = 150

// guardCoverageMinimumScopedRoutes is the same floor for the scoped subset — the routes
// whose path names at least one instance identifier.
const guardCoverageMinimumScopedRoutes = 120

// buildGuardCoverageApp mounts the WHOLE ledger HTTP surface — both version groups,
// every registrar — through the production seams (HumaMountDeps.MountV1/MountV2), with an
// authorization client pointed at an access manager that refuses everything.
//
// Handlers are zero values. No handler runs: the refusal happens in the guard chain,
// before any of them is reached — which is exactly the property under test.
func buildGuardCoverageApp(t *testing.T, address string) *fiber.App {
	t.Helper()

	libProblem.Install()

	app := fiber.New()
	v1 := app.Group("/v1")
	v2 := app.Group("/v2")

	deps := HumaMountDeps{
		Auth:                  middleware.NewAuthClient(address, true, libLog.NewNop()),
		Organization:          &OrganizationHandler{},
		Ledger:                &LedgerHandler{},
		Portfolio:             &PortfolioHandler{},
		Segment:               &SegmentHandler{},
		Account:               &AccountHandler{},
		AccountType:           &AccountTypeHandler{},
		AccountBlockException: &AccountBlockExceptionHandler{},
		MetadataIndex:         &MetadataIndexHandler{},
		Asset:                 &AssetHandler{},
		AssetRate:             &AssetRateHandler{},
		Balance:               &BalanceHandler{},
		Operation:             &OperationHandler{},
		OperationRoute:        &OperationRouteHandler{},
		TransactionRoute:      &TransactionRouteHandler{},
		Transaction:           &TransactionHandler{},
		Holder:                &HolderHandler{},
		Instrument:            &InstrumentHandler{},
		HolderAccounts:        &HolderAccountsHandler{},
		Encryption:            &EncryptionHandler{},
		Audit:                 &AuditHandler{},
		FeePackage:            &PackageHandler{},
		Fee:                   &FeeHandler{},
		BillingPackage:        &BillingPackageHandler{},
		BillingCalculate:      &BillingCalculateHandler{},
		Composition:           &CompositionHandler{},
	}

	apiV1 := openapi.New(app, v1, openapi.Config{Title: "ledger-guard-coverage-v1", Version: "test", Servers: []string{"/v1"}})
	pkgHTTP.InstallLedgerSchemaNamer(apiV1)
	deps.MountV1(v1, apiV1)

	apiV2 := openapi.New(app, v2, openapi.Config{Title: "ledger-guard-coverage-v2", Version: "test", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(apiV2)
	deps.MountV2(v2, apiV2)

	return app
}

// TestEveryMountedRouteAsksTheAuthorizationService is the guard that keeps the NEXT route
// from being born without one.
//
// It walks every route the mount actually produced and issues one request at it, against
// an access manager that records what it was asked. A route that runs the authorization
// check asks a question; a route mounted straight onto Fiber, bypassing the funnel, asks
// none — and asking none is the failure.
//
// The credential carries no partner on purpose. A partner-bound one is refused locally on
// any route that names no instance, and that refusal also leaves no question behind — so
// sweeping with it would report a correctly guarded route as unguarded.
//
// One limitation, stated because it is a property of HTTP routing rather than of this
// test: a raw route whose path is already matched by a guarded pattern (say
// /organizations/raw under the guarded /organizations/:id) is reached THROUGH that
// pattern's chain, so the check does run for it and the sweep reports it guarded. That is
// the correct answer — such a route is guarded — but it means the sweep catches a raw
// route by its PATH SHAPE being new, not by the call that mounted it being raw.
//
// The evidence is the QUESTION, not the status code. A route with no guard would answer
// some non-2xx of its own (a nil handler, a missing body, a 404 on a middleware-only
// entry), so reading the status would quietly pass the very thing this test exists to
// catch.
func TestEveryMountedRouteAsksTheAuthorizationService(t *testing.T) {
	fake := newFakeAccessManager(t)
	app := buildGuardCoverageApp(t, fake.URL)

	routes := app.GetRoutes()
	require.NotEmpty(t, routes, "the mount produced no routes at all")

	// One request per distinct METHOD+path. Fiber holds two entries per operation —
	// the guard chain and the Huma terminal — and one request exercises both.
	seen := make(map[string]string, len(routes))
	for _, r := range routes {
		seen[r.Method+" "+r.Path] = r.Path
	}

	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	// A floor on the sweep's own size. Without it, a mount that stopped producing routes —
	// or a filter that started excluding all of them — would leave the loop below with
	// nothing to walk and report success, which is the failure mode a coverage test is
	// least able to notice about itself.
	require.GreaterOrEqual(t, len(keys), guardCoverageMinimumRoutes,
		"the sweep walked far fewer routes than the mount serves; it is no longer covering the surface")

	var unguarded []string

	for _, key := range keys {
		method, pattern, _ := strings.Cut(key, " ")

		if guardCoveragePublicRoutes[pattern] {
			continue
		}

		before := len(fake.questions())

		req := httptest.NewRequest(method, guardCoverageConcretePath(pattern), nil)
		req.Header.Set("Authorization", "Bearer "+scopeTestPlainToken(t))

		resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err, "route %s could not be exercised", key)

		_ = resp.Body.Close()

		if len(fake.questions()) == before {
			unguarded = append(unguarded, key)
		}
	}

	assert.Empty(t, unguarded,
		"these routes are mounted outside the midaz authorization funnel: either register them "+
			"through protectedMidaz, or add the path to guardCoveragePublicRoutes with the reason")
}

// TestGuardCoverage_CatchesARawFiberRoute is the control for the test above: it proves the
// detection actually fires. Without it, a coverage test that silently stopped detecting
// anything would go on reporting success forever.
//
// It mounts a raw Fiber route — the exact mistake the guard exists to catch — on the same
// app and asserts the route asks no authorization question and answers its terminal.
func TestGuardCoverage_CatchesARawFiberRoute(t *testing.T) {
	fake := newFakeAccessManager(t)
	app := buildGuardCoverageApp(t, fake.URL)

	app.Get("/v1/raw-unguarded", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	before := len(fake.questions())

	req := httptest.NewRequest(http.MethodGet, "/v1/raw-unguarded", nil)
	req.Header.Set("Authorization", "Bearer "+scopeTestPlainToken(t))

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, fiber.StatusOK, resp.StatusCode,
		"a raw route serves its terminal — this is what an unguarded route looks like")
	assert.Len(t, fake.questions(), before,
		"a raw route asks NO authorization question: that absence is what the coverage test detects")
}

// TestEveryScopedRouteDeclaresWhatItsPathNames closes the gap the path argument opens.
//
// protectedMidaz is told the route's path a second time, next to the path the route is
// registered at. Nothing in the type system says the two agree, and a call site that
// spelled a path WITHOUT an identifier the real route carries would ask a wider question
// than the route serves — a partner scoped to one ledger would reach every ledger under
// its organization, with no error anywhere to show for it.
//
// So this walks the mounted surface with a partner credential and reads back, from the
// access manager, the identifiers each route actually sent. They have to be exactly the
// ones the route's own path names, filled with the values the request carried.
func TestEveryScopedRouteDeclaresWhatItsPathNames(t *testing.T) {
	fake := newFakeAccessManager(t)
	app := buildGuardCoverageApp(t, fake.URL)

	const filler = "00000000-0000-4000-8000-000000000000"

	seen := make(map[string]bool)

	var checked int

	for _, r := range app.GetRoutes() {
		key := r.Method + " " + r.Path
		if seen[key] || guardCoveragePublicRoutes[r.Path] {
			continue
		}

		seen[key] = true

		dims := midazScopeDims(r.Path)
		if len(dims) == 0 {
			continue
		}

		want := make(map[string]string, len(dims))
		for _, d := range dims {
			want[d.Name()] = filler
		}

		before := len(fake.questions())

		req := httptest.NewRequest(r.Method, guardCoverageConcretePath(r.Path), nil)
		req.Header.Set("Authorization", "Bearer "+scopeTestPartnerToken(t))

		resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)

		_ = resp.Body.Close()

		asked := fake.questions()
		require.Greater(t, len(asked), before,
			"route %s names instance identifiers but asked no authorization question", key)

		assert.Equal(t, want, asked[len(asked)-1].Attributes,
			"route %s sent identifiers that are not the ones its path names", key)

		checked++
	}

	require.GreaterOrEqual(t, checked, guardCoverageMinimumScopedRoutes,
		"far fewer scoped routes were checked than the mount serves; the check is no longer covering them")
}
