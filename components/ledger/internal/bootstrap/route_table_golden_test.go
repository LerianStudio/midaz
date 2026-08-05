// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// updateRouteTable, when set, rewrites the committed route-table golden instead of
// asserting against it:
// `go test ./components/ledger/internal/bootstrap/ -run TestRouteTableGolden -update-route-table`.
// Without it the test compares the freshly-serialized table byte-for-byte and fails
// on any drift.
//
// The name is deliberately NOT the conventional `-update`: the docs generator
// (postman/generator/generate-docs.sh) passes a package-scoped `-update` to the
// http/in package, and a shared spelling would make an unrelated docs regeneration
// rewrite this golden as a side effect the moment either invocation widened its
// package scope.
var updateRouteTable = flag.Bool("update-route-table", false, "rewrite the committed route-table golden")

// routeTableGoldenPath is the committed serialization of the unified server's Fiber
// route table: one line per registered route, carrying method, raw path and HANDLER
// COUNT.
const routeTableGoldenPath = "testdata/route_table.golden"

// routeTableMinRows is a FLOOR on the served surface, not a count of it — the golden bytes pin
// the exact set. It exists because -update-route-table writes whatever the harness produced, so
// a harness that mounted almost nothing would regenerate cleanly and pass forever after. It
// sits just under today's row count, so losing a registrar trips it.
//
// It catches REMOVALS only. A registrar mounted in production but never added to this harness is
// invisible to it, and to every other gate in this package.
const routeTableMinRows = 235

// routeTableGoldenHeader prefixes the golden so a reader who opens the file knows what the
// third column means and how to regenerate. It is part of the compared bytes, so it cannot
// drift away from the rows it describes.
const routeTableGoldenHeader = `# Unified server Fiber route table: METHOD<TAB>RAW PATH<TAB>HANDLER COUNT.
# app.Use middleware is excluded: it is fanned across every HTTP method, so it yields rows
# for methods the API never serves and encodes nothing about any route's guard.
# Rows sharing a method and a path are in registration order.
# GET /health, GET /version and GET /readyz are neither guard chains nor Huma terminals: they
# are mounted outside the authorized surface and carved out in unguardedPublicRoutes.
#
# This file pins WHICH (method, raw path, handler count) triples are registered. What the
# count does not say:
#   - which handler on a chain is the authorizer: a chain of the same length built from
#     non-auth middleware reads identically.
#   - the authorize tuple: swapping namespace, resource or action moves neither path nor
#     count, and no gate in this repository covers that tuple.
#   - registration order inside a COLLAPSED row (one row whose count already sums a guard and
#     its terminal): the merged count is the sum whichever was registered first.
# Generated — do not hand-edit. Regenerate with:
#   go test ./components/ledger/internal/bootstrap/ -run TestRouteTableGolden -update-route-table
`

// unguardedPublicRouteCount pins the size of unguardedPublicRoutes as a literal. Adding a key
// is how a real endpoint would be carved out of the tokenless-request probe, and that probe
// reconciles against this constant rather than against the map, so a carve-out takes a second
// deliberate edit here.
const unguardedPublicRouteCount = 3

// unguardedPublicRoutes is the LOCKED set of routes the unified server mounts OUTSIDE the
// authorized API surface. Each must be justified inline, because adding an entry here is how a
// genuinely unguarded endpoint would be skipped by the tokenless-request probe.
//
// It mirrors the excludedPaths carve-out the contract-spec gate in
// components/ledger/internal/adapters/http/in uses for the same three probes.
//
// Keys are "METHOD\tPATH", matching routeRow.key.
var unguardedPublicRoutes = map[string]bool{
	// Liveness ping. Must answer for a load balancer that holds no credentials.
	"GET\t/health": true,
	// Build metadata. Carries no tenant or account data.
	"GET\t/version": true,
	// Readiness probe. Must answer while the auth service itself is unreachable,
	// which is precisely when putting it behind auth would take the pod down.
	"GET\t/readyz": true,
}

// fullSurfaceAuthClient is the auth client every full-surface harness mounts. Auth is
// ENABLED so the guard chains are the ones a deployment runs, and the address is a name
// that does not resolve so no test can reach the network: a credential-less request is
// refused before the address is used, and route registration never uses it at all.
func fullSurfaceAuthClient() *middleware.AuthClient {
	return &middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"}
}

// fullSurfaceMarkerRan records whether the harness post-auth marker executed. A credential-less
// request must be refused by the FIRST handler on the chain, so a marker sitting behind that
// handler must never run. Package-level state is safe because every test in this package is
// sequential: goleak_test.go verifies the whole package.
var fullSurfaceMarkerRan atomic.Bool

// fullSurfaceRouteOptions is the ProtectedRouteOptions every registrar in the harness
// receives. Its single post-auth handler is an OBSERVER: it records that it ran and passes
// through, so a refusal that reaches it is a refusal that did not come from the authorizer.
//
// It MUST stay a bare passthrough. The production post-auth handler
// pkgHTTP.MarkTrustedAuthAssertion answers 401 on a missing token itself, so substituting it
// here would make the observer indistinguishable from the authorizer.
func fullSurfaceRouteOptions() *pkgHTTP.ProtectedRouteOptions {
	marker := func(c fiber.Ctx) error {
		fullSurfaceMarkerRan.Store(true)

		return c.Next()
	}

	return &pkgHTTP.ProtectedRouteOptions{PostAuthMiddlewares: []fiber.Handler{marker}}
}

// buildFullSurfaceServer builds a UnifiedServer that mounts BOTH contract instances
// with the complete production registrar set, so the golden covers the whole served
// surface rather than one version of it. Handlers are zero-value structs: route
// registration stores handler funcs and never calls them, and the conditional CRM
// handlers (holder-accounts, encryption, audit) are non-nil so their routes mount.
//
// Absolute handler counts stay lower than a running ledger's, because the harness supplies
// one post-auth observer where a multi-tenant deployment supplies its per-module tenant
// middleware, and a single-tenant deployment supplies none at all.
func buildFullSurfaceServer(t *testing.T) *UnifiedServer {
	t.Helper()

	fullSurfaceMarkerRan.Store(false)

	logger := newTestLogger()
	telemetry := &libOpentelemetry.Telemetry{}
	auth := fullSurfaceAuthClient()
	routeOptions := fullSurfaceRouteOptions()

	transactionHandler := &httpin.TransactionHandler{}
	metadataIndexHandler := &httpin.MetadataIndexHandler{}

	humaMount := func(group fiber.Router, api huma.API) {
		httpin.RegisterOrganizationRoutesToApp(group, api, auth, &httpin.OrganizationHandler{}, routeOptions)
		httpin.RegisterLedgerRoutesToApp(group, api, auth, &httpin.LedgerHandler{}, routeOptions)
		httpin.RegisterPortfolioRoutesToApp(group, api, auth, &httpin.PortfolioHandler{}, routeOptions)
		httpin.RegisterSegmentRoutesToApp(group, api, auth, &httpin.SegmentHandler{}, routeOptions)
		httpin.RegisterAccountRoutesToApp(group, api, auth, &httpin.AccountHandler{}, routeOptions)
		httpin.RegisterAccountTypeRoutesToApp(group, api, auth, &httpin.AccountTypeHandler{}, routeOptions)
		httpin.RegisterMetadataIndexRoutesToApp(group, api, auth, metadataIndexHandler, routeOptions)
		httpin.RegisterAssetRoutesToApp(group, api, auth, &httpin.AssetHandler{}, routeOptions)
		httpin.RegisterAssetRateRoutesToApp(group, api, auth, &httpin.AssetRateHandler{}, routeOptions)

		httpin.RegisterBalanceRoutesToApp(group, api, auth, &httpin.BalanceHandler{}, routeOptions)
		httpin.RegisterOperationRoutesToApp(group, api, auth, &httpin.OperationHandler{}, routeOptions)
		httpin.RegisterCountTransactionRoutesToApp(group, api, auth, transactionHandler, routeOptions)
		httpin.RegisterOperationRouteRoutesToApp(group, api, auth, &httpin.OperationRouteHandler{}, routeOptions)
		httpin.RegisterTransactionRouteRoutesToApp(group, api, auth, &httpin.TransactionRouteHandler{}, routeOptions)

		httpin.RegisterTransactionHumaRoutesToApp(group, api, auth, transactionHandler, routeOptions)

		httpin.RegisterCRMRoutesToApp(group, api, auth,
			&httpin.HolderHandler{}, &httpin.InstrumentHandler{}, &httpin.HolderAccountsHandler{},
			&httpin.EncryptionHandler{}, &httpin.AuditHandler{}, routeOptions)
		httpin.RegisterFeesRoutesToApp(group, api, auth,
			&httpin.PackageHandler{}, &httpin.FeeHandler{},
			&httpin.BillingPackageHandler{}, &httpin.BillingCalculateHandler{}, routeOptions)
		httpin.RegisterCompositionRoutesToApp(group, api, auth, &httpin.CompositionHandler{}, routeOptions)
	}

	humaMountV2 := func(group fiber.Router, api huma.API) {
		httpin.RegisterTransactionV2RoutesToApp(group, api, auth, transactionHandler, routeOptions)
		httpin.RegisterCRMV2RoutesToApp(group, api, auth,
			&httpin.HolderHandler{}, &httpin.InstrumentHandler{}, &httpin.HolderAccountsHandler{},
			&httpin.EncryptionHandler{}, &httpin.AuditHandler{}, routeOptions)
		httpin.RegisterFeesV2RoutesToApp(group, api, auth,
			&httpin.PackageHandler{}, &httpin.FeeHandler{},
			&httpin.BillingPackageHandler{}, &httpin.BillingCalculateHandler{}, routeOptions)
	}

	onboardingRouteRegistrar := func(router fiber.Router) {
		httpin.RegisterOnboardingRoutesToApp(router, auth,
			&httpin.AccountHandler{}, &httpin.PortfolioHandler{}, &httpin.LedgerHandler{},
			&httpin.OrganizationHandler{}, &httpin.SegmentHandler{}, &httpin.AccountTypeHandler{}, routeOptions)
	}
	ledgerRouteRegistrar := httpin.CreateRouteRegistrar(auth, metadataIndexHandler, routeOptions)

	readyzHandler := NewReadyzHandler(ReadyzHandlerConfig{Logger: logger, Version: "test-version"})

	server := NewUnifiedServer(":0", "test-version", logger, telemetry, readyzHandler,
		humaMount, humaMountV2, onboardingRouteRegistrar, ledgerRouteRegistrar)
	require.NotNil(t, server, "NewUnifiedServer should return a non-nil server")
	require.NotNil(t, server.app, "server should hold a Fiber app")

	return server
}

// routeRow is one registered route: its method, its RAW (parameterized) path and how
// many handlers Fiber holds for it.
type routeRow struct {
	method   string
	path     string
	handlers int
}

// key is the identity Fiber's router matches on, and the grouping key of the table:
// rows sharing it are alternative registrations on one endpoint.
func (r routeRow) key() string {
	return r.method + "\t" + r.path
}

// routeGroup is the rows sharing one key, in registration order.
type routeGroup struct {
	key  string
	rows []routeRow
}

// display renders a key for a failure message, where a tab would be invisible.
func (g routeGroup) display() string {
	return strings.ReplaceAll(g.key, "\t", " ")
}

// requireDroppedUseRowsAreRootOnly fails when the GetRoutes(true) filter dropped a row whose
// path is not "/". The filter drops by the route's use flag, not by its path, so a Use mounted on
// a CONCRETE path serves requests while appearing in no route table and in no probe.
//
// The comparison is a multiset on (method, path), so a Use that shadows a path an Add also
// registers still leaves one unmatched row.
func requireDroppedUseRowsAreRootOnly(t *testing.T, all, filtered []fiber.Route) {
	t.Helper()

	remaining := make(map[string]int, len(filtered))
	for _, r := range filtered {
		remaining[r.Method+"\t"+r.Path]++
	}

	dropped := make([]string, 0)

	for _, r := range all {
		key := r.Method + "\t" + r.Path

		if remaining[key] > 0 {
			remaining[key]--

			continue
		}

		if r.Path != "/" {
			dropped = append(dropped, strings.ReplaceAll(key, "\t", " "))
		}
	}

	sort.Strings(dropped)

	require.Emptyf(t, dropped,
		"the app.Use filter dropped these rows on concrete paths, so they serve requests while appearing in neither the "+
			"route table nor the tokenless probe:\n%s", strings.Join(dropped, "\n"))
}

// collectRouteRows reads the unified server's routes, grouped by method and path. What the
// GetRoutes(true) filter dropped is checked first: Fiber fans app.Use across every HTTP method,
// so those rows say nothing about any route's guard and any middleware reshuffle churns all of
// them, but only while they sit on "/".
//
// GetRoutes promises no ordering across the per-method stacks it walks, so grouping by key is
// what makes the bytes stable. The sort is STABLE and the handler count is deliberately outside
// the key: rows sharing a key keep the order Fiber holds them in, which is the order they were
// registered, and that order is load-bearing — a guard only reaches its terminal by sitting
// ahead of it.
func collectRouteRows(t *testing.T, app *fiber.App) []routeRow {
	t.Helper()

	routes := app.GetRoutes(true)

	requireDroppedUseRowsAreRootOnly(t, app.GetRoutes(false), routes)

	rows := make([]routeRow, 0, len(routes))
	for _, r := range routes {
		rows = append(rows, routeRow{method: r.Method, path: r.Path, handlers: len(r.Handlers)})
	}

	sort.SliceStable(rows, func(i, j int) bool { return rows[i].key() < rows[j].key() })

	return rows
}

// groupRouteRows collapses a key-sorted row slice into one group per key, preserving
// registration order within each group.
func groupRouteRows(rows []routeRow) []routeGroup {
	groups := make([]routeGroup, 0, len(rows))

	for _, r := range rows {
		if n := len(groups); n > 0 && groups[n-1].key == r.key() {
			groups[n-1].rows = append(groups[n-1].rows, r)

			continue
		}

		groups = append(groups, routeGroup{key: r.key(), rows: []routeRow{r}})
	}

	return groups
}

// serializeRouteTable renders rows as the golden's "METHOD\tpath\tcount" lines under the
// golden header.
func serializeRouteTable(rows []routeRow) string {
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("%s\t%d", r.key(), r.handlers))
	}

	return routeTableGoldenHeader + strings.Join(lines, "\n") + "\n"
}

// assertRouteTableInvariants asserts the properties that protect the HARNESS and its carve-out
// rather than the surface: that a plausible surface was mounted, that the carve-out is the size
// it is pinned to, and that no entry in it is dead. It runs BEFORE the byte comparison and
// before -update-route-table can write.
func assertRouteTableInvariants(t *testing.T, rows []routeRow) {
	t.Helper()

	require.GreaterOrEqualf(t, len(rows), routeTableMinRows,
		"route table holds %d rows, under the %d floor: the harness is not mounting the full surface, "+
			"so the golden bytes are not pinning it", len(rows), routeTableMinRows)

	assert.Lenf(t, unguardedPublicRoutes, unguardedPublicRouteCount,
		"unguardedPublicRoutes holds %d entries against the %d pinned in unguardedPublicRouteCount: a carve-out has to be "+
			"added in both places", len(unguardedPublicRoutes), unguardedPublicRouteCount)

	seen := make(map[string]bool, len(rows))
	for _, r := range rows {
		seen[r.key()] = true
	}

	// A carve-out for a route that is no longer mounted is dead, and dead carve-outs are
	// how a real one gets waved through later.
	for key := range unguardedPublicRoutes {
		assert.Truef(t, seen[key], "unguardedPublicRoutes carves out %q, which the server does not mount: remove the entry",
			strings.ReplaceAll(key, "\t", " "))
	}
}

// TestRouteTableGolden pins which (method, raw path, handler count) triples the unified server
// registers, so a change to the served surface has to move the committed bytes and show up in
// review. routeTableGoldenHeader above enumerates what the HANDLER COUNT does NOT say, and is
// serialized into the file so the enumeration reaches whoever opens it.
func TestRouteTableGolden(t *testing.T) {
	// The docs surface is gated on OPENAPI_DOCS_ENABLED and the golden pins the
	// default deployment posture, so the variable must be genuinely absent.
	// unsetDocsGate uses t.Setenv, which precludes t.Parallel here.
	unsetDocsGate(t)

	server := buildFullSurfaceServer(t)

	rows := collectRouteRows(t, server.app)

	assertRouteTableInvariants(t, rows)

	got := serializeRouteTable(rows)

	if *updateRouteTable {
		// The invariants above are assert-based, so a failure marks the test and execution
		// continues to here. Writing then bakes the broken surface into the golden and turns
		// the file green over it.
		if t.Failed() {
			return
		}

		require.NoError(t, os.MkdirAll(filepath.Dir(routeTableGoldenPath), 0o755),
			"create golden directory for %s", routeTableGoldenPath)
		require.NoError(t, os.WriteFile(routeTableGoldenPath, []byte(got), 0o644),
			"write golden route table %s", routeTableGoldenPath)
		t.Logf("wrote golden route table %s (%d rows)", routeTableGoldenPath, len(rows))

		return
	}

	want, err := os.ReadFile(routeTableGoldenPath)
	require.NoErrorf(t, err, "read golden route table %s (run with -update-route-table to generate)", routeTableGoldenPath)
	require.Equalf(t, string(want), got,
		"unified server route table drifted from %s: the served surface changed. The column does not pin the authorize "+
			"tuple, so an unmoved table is not evidence the namespace/resource/action is unchanged either",
		routeTableGoldenPath)

	t.Logf("route table matches golden: %d rows", len(rows))
}
