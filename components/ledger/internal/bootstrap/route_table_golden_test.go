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
// a harness that mounted almost nothing would regenerate cleanly and pass forever after.
//
// It sits ONE row below today's count, so it trips when two or more rows disappear and tolerates
// exactly one. That makes it no kind of per-registrar guarantee: a registrar whose guard chain
// and Huma terminal collapse into a single row contributes one row, two of them do today, and
// losing either clears this floor — the golden bytes are what catch that. Pinning it at the exact
// count instead would turn every legitimate route retirement into a two-line edit while adding no
// coverage the bytes do not already give.
//
// It catches REMOVALS only. A registrar mounted in production but never added to this harness is
// invisible to it, and to every other gate in this package.
const routeTableMinRows = 301

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
// handler must never run.
//
// Package-level state cannot be raced between the two TESTS that share it: both open with
// unsetDocsGate, whose t.Setenv makes t.Parallel panic in the test that called it, so neither can
// run concurrently with anything. It says nothing about their subtests — Go's parallel denial is a
// field on the T that called Setenv and subtests get a fresh one — so the reset lives next to the
// read, inside the subtest, and that subtest must stay sequential. Nothing enforces that.
var fullSurfaceMarkerRan atomic.Bool

// fullSurfaceRouteOptions is the ProtectedRouteOptions every registrar in the harness
// receives. Its single post-auth handler is an OBSERVER: it records that it ran and passes
// through, so a refusal that reaches it is a refusal that did not come from the authorizer.
//
// It MUST stay a bare passthrough. A post-auth handler that answered 401 itself — the production
// one, pkgHTTP.MarkTrustedAuthAssertion, does — would leave the marker false on a route whose
// authorizer had gone missing, because the refusal would never reach past it. That class is
// caught instead by the envelope assertion in TestFullSurfaceRoutes_RejectTokenlessRequests, and
// only for substitutes whose 401 renders through the app ErrorHandler.
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

	logger := newTestLogger()
	telemetry := &libOpentelemetry.Telemetry{}
	auth := fullSurfaceAuthClient()
	routeOptions := fullSurfaceRouteOptions()

	transactionHandler := &httpin.TransactionHandler{}
	metadataIndexHandler := &httpin.MetadataIndexHandler{}

	// The full-surface harness threads ONE ProtectedRouteOptions to every role, so all
	// six named option fields carry routeOptions. The handler counts the golden pins are
	// therefore the counts a single post-auth observer produces — below a running
	// deployment's, which supplies per-module tenant middleware (MT) or none (ST).
	// Handlers are non-nil zero-value structs so every route mounts, including the
	// conditional CRM holder-accounts/encryption/audit routes. It mounts through the
	// SAME HumaMountDeps.MountV1/MountV2 production runs, so a registrar added there
	// reaches this golden.
	humaDeps := httpin.HumaMountDeps{
		Auth: auth,

		Organization:  &httpin.OrganizationHandler{},
		Ledger:        &httpin.LedgerHandler{},
		Portfolio:     &httpin.PortfolioHandler{},
		Segment:       &httpin.SegmentHandler{},
		Account:       &httpin.AccountHandler{},
		AccountType:   &httpin.AccountTypeHandler{},
		MetadataIndex: metadataIndexHandler,
		Asset:         &httpin.AssetHandler{},
		AssetRate:     &httpin.AssetRateHandler{},

		Balance:          &httpin.BalanceHandler{},
		Operation:        &httpin.OperationHandler{},
		OperationRoute:   &httpin.OperationRouteHandler{},
		TransactionRoute: &httpin.TransactionRouteHandler{},

		Transaction: transactionHandler,

		Holder:         &httpin.HolderHandler{},
		Instrument:     &httpin.InstrumentHandler{},
		HolderAccounts: &httpin.HolderAccountsHandler{},
		Encryption:     &httpin.EncryptionHandler{},
		Audit:          &httpin.AuditHandler{},

		FeePackage:       &httpin.PackageHandler{},
		Fee:              &httpin.FeeHandler{},
		BillingPackage:   &httpin.BillingPackageHandler{},
		BillingCalculate: &httpin.BillingCalculateHandler{},

		Composition: &httpin.CompositionHandler{},

		OnboardingOptions:  routeOptions,
		LedgerOptions:      routeOptions,
		TransactionOptions: routeOptions,
		CRMOptions:         routeOptions,
		FeesOptions:        routeOptions,
		CompositionOptions: routeOptions,
	}

	onboardingRouteRegistrar := func(router fiber.Router) {
		httpin.RegisterOnboardingRoutesToApp(router, auth,
			&httpin.AccountHandler{}, &httpin.PortfolioHandler{}, &httpin.LedgerHandler{},
			&httpin.OrganizationHandler{}, &httpin.SegmentHandler{}, &httpin.AccountTypeHandler{}, routeOptions)
	}
	ledgerRouteRegistrar := httpin.CreateRouteRegistrar(auth, metadataIndexHandler, routeOptions)

	readyzHandler := NewReadyzHandler(ReadyzHandlerConfig{Logger: logger, Version: "test-version"})

	server := NewUnifiedServer(":0", "test-version", logger, telemetry, readyzHandler,
		humaDeps.MountV1, humaDeps.MountV2, onboardingRouteRegistrar, ledgerRouteRegistrar)
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

// assertRouteTableInvariants asserts what must hold of the table independently of the committed
// bytes: that a plausible surface was mounted, that the carve-out is the size it is pinned to,
// that no entry in it is dead, and that no route is registered as a lone unguarded terminal. It
// runs BEFORE the byte comparison and before -update-route-table can write, so a regeneration
// cannot bake an unguarded route into the golden and turn the file green over it.
func assertRouteTableInvariants(t *testing.T, rows []routeRow) {
	t.Helper()

	require.GreaterOrEqualf(t, len(rows), routeTableMinRows,
		"route table holds %d rows, under the %d floor: the harness is not mounting the full surface, "+
			"so the golden bytes are not pinning it", len(rows), routeTableMinRows)

	assert.Lenf(t, unguardedPublicRoutes, unguardedPublicRouteCount,
		"unguardedPublicRoutes holds %d entries against the %d pinned in unguardedPublicRouteCount: a carve-out has to be "+
			"added in both places", len(unguardedPublicRoutes), unguardedPublicRouteCount)

	// A lone row is safe only if it is a guard chain long enough to be one. A lone ONE-handler row
	// is a terminal with no guard registered on its path — the shape a dropped guard chain leaves
	// behind, and the one shape a collapsed row cannot counterfeit: a collapsed row already sums a
	// guard and its terminal, so it carries two handlers or more and never reaches this branch.
	//
	// It is the only shape invariant here. The two that were scoped to groups of more than one row
	// are covered by the behavioural sweep in route_guard_test.go instead, and THAT subsumption
	// rests on no terminal ever answering 401. The 401 producers on this surface are lib-auth's
	// authorizer at chain position 0 and pkgHTTP.MarkTrustedAuthAssertion behind it, and the only
	// site rendering constant.ErrInvalidToken is CanonicalFiberErrorHandler's 401 arm. Should a
	// terminal ever answer 401, those two become load-bearing again.
	for _, group := range groupRouteRows(rows) {
		if len(group.rows) != 1 || group.rows[0].handlers >= 2 {
			continue
		}

		assert.Truef(t, unguardedPublicRoutes[group.key],
			"%s is a single one-handler route: no guard chain on its path and no sibling row that could be one, so it "+
				"answers without auth. If that is intended, add it to unguardedPublicRoutes with a justification",
			group.display())
	}

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
