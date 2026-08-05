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

// routeTableMinRows is a FLOOR on the served surface, not a count of it — the golden
// bytes pin the exact set. It exists because -update-route-table writes whatever the
// harness produced, so a harness that mounted almost nothing (a registrar list that lost
// its contents, a mount closure that returned early) would regenerate cleanly and pass
// forever after. The value sits below today's row count with room for routes to be
// retired, and far above anything a broken harness yields.
const routeTableMinRows = 200

// routeTableGoldenHeader prefixes the golden so a reader who opens the file knows what
// the third column means, what it does and does not prove, and how to regenerate. It is
// part of the compared bytes, so it cannot drift away from the rows it describes.
const routeTableGoldenHeader = `# Unified server Fiber route table: METHOD<TAB>RAW PATH<TAB>HANDLER COUNT.
# app.Use middleware is excluded: it is fanned across every HTTP method, so it yields rows
# for methods the API never serves and encodes nothing about any route's guard.
# Rows sharing a method and a path are in registration order, and the first of them is
# the guard chain — every guard chain here carries at least two handlers, every Huma
# terminal exactly one.
# The count does NOT pin the authorize tuple: swapping namespace, resource or action
# changes neither path nor count. Nor does a COLLAPSED row (one row whose count already
# sums a guard and its terminal) distinguish guard-first from terminal-first registration.
# Both gaps are covered by TestFullSurfaceRoutes_RejectTokenlessRequests, which asserts
# behavior instead of shape.
# Generated — do not hand-edit. Regenerate with:
#   go test ./components/ledger/internal/bootstrap/ -run TestRouteTableGolden -update-route-table
`

// unguardedPublicRoutes is the LOCKED set of routes the unified server mounts OUTSIDE the
// authorized API surface. Each is a lone one-handler route — the exact shape every other
// route is asserted not to have — and each must be justified inline, because adding an
// entry here is how a genuinely unguarded endpoint would be hidden from both the shape
// invariants and the tokenless-request test.
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

// fullSurfaceRouteOptions is the non-nil ProtectedRouteOptions every registrar in the
// harness receives. It MUST stay non-nil, for two reasons.
//
// Realism: every deployment supplies post-auth middleware here (tenant resolution in
// multi-tenant mode), so nil options build a chain no deployment serves, and a registrar
// that silently stops threading its options through is then indistinguishable from one
// that threads them.
//
// Separability: with nil options, the guard on a route that has no UUID parameter is a
// ONE-handler chain, and the Huma terminal it protects is also one handler. Two rows of one
// handler each are byte-identical, so the table cannot say which came first — and only the
// guard-first order guards. One marker handler puts every guard chain at two or more, which
// is what makes the HANDLER COUNT column separate guard from terminal.
func fullSurfaceRouteOptions() *pkgHTTP.ProtectedRouteOptions {
	return &pkgHTTP.ProtectedRouteOptions{
		PostAuthMiddlewares: []fiber.Handler{func(c fiber.Ctx) error { return c.Next() }},
	}
}

// buildFullSurfaceServer builds a UnifiedServer that mounts BOTH contract instances
// with the complete production registrar set, so the golden covers the whole served
// surface rather than one version of it. Handlers are zero-value structs: route
// registration stores handler funcs and never calls them, and the conditional CRM
// handlers (holder-accounts, encryption, audit) are non-nil so their routes mount.
//
// Absolute handler counts stay lower than a running ledger's, because the harness
// supplies one post-auth marker where a deployment supplies its per-module tenant
// middleware. What the counts have to support is the guard/terminal distinction, and one
// marker is enough for that.
func buildFullSurfaceServer(t *testing.T) *UnifiedServer {
	t.Helper()

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

// collectRouteRows reads the unified server's routes, grouped by method and path.
//
// GetRoutes(true) filters the app.Use registrations. Fiber fans those across every HTTP
// method, so they yield rows on path "/" for methods the API never serves; nothing about a
// route's guard is encoded there, and any middleware reshuffle churns all of them. Every
// guard chain on this surface is registered through the router's Add rather than Use, so
// the filter cannot remove one.
//
// GetRoutes promises no ordering across the per-method stacks it walks, so grouping by
// key is what makes the bytes stable. The sort is STABLE and the handler count is
// deliberately outside the key: rows sharing a key keep the order Fiber holds them in,
// which is the order they were registered, and that order is load-bearing — a guard only
// reaches its terminal by sitting ahead of it.
func collectRouteRows(app *fiber.App) []routeRow {
	routes := app.GetRoutes(true)

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

// assertRouteTableInvariants asserts the properties that must hold of ANY correct route
// table, independently of the committed bytes. It runs BEFORE the byte comparison and
// before -update-route-table can write, so a careless regeneration cannot bake a broken
// surface into the golden and turn the file green over it.
//
// These are shape properties. They cannot see the authorize tuple, and they cannot see
// registration order inside a collapsed row — TestFullSurfaceRoutes_RejectTokenlessRequests
// covers both.
func assertRouteTableInvariants(t *testing.T, rows []routeRow) {
	t.Helper()

	require.GreaterOrEqualf(t, len(rows), routeTableMinRows,
		"route table holds %d rows, under the %d floor: the harness is not mounting the full surface, "+
			"so neither the golden bytes nor these invariants are asserting anything", len(rows), routeTableMinRows)

	seen := make(map[string]bool, len(rows))

	for _, group := range groupRouteRows(rows) {
		seen[group.key] = true

		if len(group.rows) == 1 {
			// A lone row is only safe if it is a guard chain long enough to be one. A lone
			// one-handler row is a terminal with no guard registered on its path.
			if group.rows[0].handlers < 2 {
				assert.Truef(t, unguardedPublicRoutes[group.key],
					"%s is a single one-handler route: no guard chain on its path and no sibling row that could be one, "+
						"so it answers without auth. If that is intended, add it to unguardedPublicRoutes with a justification",
					group.display())
			}

			continue
		}

		assert.Greaterf(t, group.rows[0].handlers, 1,
			"%s registers %d rows and the FIRST one carries a single handler: the terminal is registered ahead of its "+
				"guard chain, so the guard never runs", group.display(), len(group.rows))

		// Equal counts inside a group make the two registration orders byte-identical, so
		// the golden would absorb a guard/terminal swap without moving. Guard chains carry
		// at least two handlers and Huma terminals exactly one, so equality means something
		// changed about that shape.
		counts := make(map[int]bool, len(group.rows))

		for _, r := range group.rows {
			assert.Falsef(t, counts[r.handlers],
				"%s has two rows carrying %d handlers each: with equal counts the golden bytes are identical whichever "+
					"row was registered first, so it can no longer show a guard that slipped behind its terminal",
				group.display(), r.handlers)

			counts[r.handlers] = true
		}
	}

	// A carve-out for a route that is no longer mounted is dead, and dead carve-outs are
	// how a real one gets waved through later.
	for key := range unguardedPublicRoutes {
		assert.Truef(t, seen[key], "unguardedPublicRoutes carves out %q, which the server does not mount: remove the entry",
			strings.ReplaceAll(key, "\t", " "))
	}
}

// TestRouteTableGolden locks the SHAPE of the unified server's route table: which
// (method, raw path) pairs are registered, how many rows each pair holds, and how many
// handlers each row carries.
//
// What the HANDLER COUNT proves. Auth on this surface is not app.Use middleware: a guard
// chain and the Huma terminal it protects are two separate registrations on the same
// method and the same RAW path, and equality of that raw path is the whole mechanism
// binding them. A terminal that lands on a raw path its guard does not share therefore
// moves the bytes — the guard row loses its partner, or a collapsed row loses handlers —
// and a guard chain that stops being built at all drops a count to the bare terminal's one.
//
// What it does NOT prove, and must not be read as proving:
//   - The authorize tuple. protectedMidaz and protectedRouting build chains of the same
//     length, and ("transactions","post") is the same length as ("transactions","get"), so
//     a namespace, resource or action swap moves neither path nor count.
//   - Registration order inside a COLLAPSED row. Fiber merges an adjacent same-path
//     registration into the row before it and the merged count is the sum either way, so a
//     row that already sums a guard and its terminal reads identically whichever was
//     registered first — and only guard-first actually guards.
//
// TestFullSurfaceRoutes_RejectTokenlessRequests is the gate for both, because it asserts
// what the surface DOES rather than how its rows are shaped.
func TestRouteTableGolden(t *testing.T) {
	// The docs surface is gated on OPENAPI_DOCS_ENABLED and the golden pins the
	// default deployment posture, so the variable must be genuinely absent.
	// unsetDocsGate uses t.Setenv, which precludes t.Parallel here.
	unsetDocsGate(t)

	server := buildFullSurfaceServer(t)

	rows := collectRouteRows(server.app)

	assertRouteTableInvariants(t, rows)

	got := serializeRouteTable(rows)

	if *updateRouteTable {
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
		"unified server route table drifted from %s. Check the HANDLER COUNT column first: a count that dropped, or a "+
			"guard row that lost its terminal, means that route may now answer without auth. The column does not pin the "+
			"authorize tuple, so an unmoved table is not evidence the namespace/resource/action is unchanged",
		routeTableGoldenPath)

	t.Logf("route table matches golden: %d rows", len(rows))
}
