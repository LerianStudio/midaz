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
	"github.com/stretchr/testify/require"

	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
)

// updateRouteTable, when set, rewrites the committed route-table golden instead of
// asserting against it:
// `go test ./components/ledger/internal/bootstrap/ -run TestRouteTableGolden -update-route-table`.
// Without it the test compares the freshly-serialized table byte-for-byte and fails
// on any drift.
var updateRouteTable = flag.Bool("update-route-table", false, "rewrite the committed route-table golden")

// routeTableGoldenPath is the committed serialization of the unified server's Fiber
// route table: one line per registered route, carrying method, raw path and HANDLER
// COUNT.
const routeTableGoldenPath = "testdata/route_table.golden"

// routeTableGoldenHeader prefixes the golden so a reader who opens the file knows
// what the third column means and how to regenerate. It is part of the compared
// bytes, so it cannot drift away from the rows it describes.
const routeTableGoldenHeader = `# Unified server Fiber route table: METHOD<TAB>RAW PATH<TAB>HANDLER COUNT.
# Rows sharing a method and a path are in registration order: the guard chain comes
# before the terminal it hands off to.
# Generated — do not hand-edit. Regenerate with:
#   go test ./components/ledger/internal/bootstrap/ -run TestRouteTableGolden -update-route-table
`

// buildFullSurfaceServer builds a UnifiedServer that mounts BOTH contract instances
// with the complete production registrar set, so the golden covers the whole served
// surface rather than one version of it. Handlers are zero-value structs: route
// registration stores handler funcs and never calls them, and the conditional CRM
// handlers (holder-accounts, encryption, audit) are non-nil so their routes mount.
//
// routeOptions is nil on every registrar, so each guard chain carries the authorize
// handler plus the UUID path parser and none of the per-module tenant middleware the
// binary injects. Absolute handler counts are therefore lower than a running ledger's,
// which costs nothing: a guarded route still counts above one, and losing a guard is a
// drop from that count.
func buildFullSurfaceServer(t *testing.T) *UnifiedServer {
	t.Helper()

	logger := newTestLogger()
	telemetry := &libOpentelemetry.Telemetry{}
	auth := &middleware.AuthClient{Enabled: false}

	transactionHandler := &httpin.TransactionHandler{}
	metadataIndexHandler := &httpin.MetadataIndexHandler{}

	humaMount := func(group fiber.Router, api huma.API) {
		httpin.RegisterOrganizationRoutesToApp(group, api, auth, &httpin.OrganizationHandler{}, nil)
		httpin.RegisterLedgerRoutesToApp(group, api, auth, &httpin.LedgerHandler{}, nil)
		httpin.RegisterPortfolioRoutesToApp(group, api, auth, &httpin.PortfolioHandler{}, nil)
		httpin.RegisterSegmentRoutesToApp(group, api, auth, &httpin.SegmentHandler{}, nil)
		httpin.RegisterAccountRoutesToApp(group, api, auth, &httpin.AccountHandler{}, nil)
		httpin.RegisterAccountTypeRoutesToApp(group, api, auth, &httpin.AccountTypeHandler{}, nil)
		httpin.RegisterMetadataIndexRoutesToApp(group, api, auth, metadataIndexHandler, nil)
		httpin.RegisterAssetRoutesToApp(group, api, auth, &httpin.AssetHandler{}, nil)
		httpin.RegisterAssetRateRoutesToApp(group, api, auth, &httpin.AssetRateHandler{}, nil)

		httpin.RegisterBalanceRoutesToApp(group, api, auth, &httpin.BalanceHandler{}, nil)
		httpin.RegisterOperationRoutesToApp(group, api, auth, &httpin.OperationHandler{}, nil)
		httpin.RegisterCountTransactionRoutesToApp(group, api, auth, transactionHandler, nil)
		httpin.RegisterOperationRouteRoutesToApp(group, api, auth, &httpin.OperationRouteHandler{}, nil)
		httpin.RegisterTransactionRouteRoutesToApp(group, api, auth, &httpin.TransactionRouteHandler{}, nil)

		httpin.RegisterTransactionHumaRoutesToApp(group, api, auth, transactionHandler, nil)

		httpin.RegisterCRMRoutesToApp(group, api, auth,
			&httpin.HolderHandler{}, &httpin.InstrumentHandler{}, &httpin.HolderAccountsHandler{},
			&httpin.EncryptionHandler{}, &httpin.AuditHandler{}, nil)
		httpin.RegisterFeesRoutesToApp(group, api, auth,
			&httpin.PackageHandler{}, &httpin.FeeHandler{},
			&httpin.BillingPackageHandler{}, &httpin.BillingCalculateHandler{}, nil)
		httpin.RegisterCompositionRoutesToApp(group, api, auth, &httpin.CompositionHandler{}, nil)
	}

	humaMountV2 := func(group fiber.Router, api huma.API) {
		httpin.RegisterTransactionV2RoutesToApp(group, api, auth, transactionHandler, nil)
		httpin.RegisterCRMV2RoutesToApp(group, api, auth,
			&httpin.HolderHandler{}, &httpin.InstrumentHandler{}, &httpin.HolderAccountsHandler{},
			&httpin.EncryptionHandler{}, &httpin.AuditHandler{}, nil)
		httpin.RegisterFeesV2RoutesToApp(group, api, auth,
			&httpin.PackageHandler{}, &httpin.FeeHandler{},
			&httpin.BillingPackageHandler{}, &httpin.BillingCalculateHandler{}, nil)
	}

	onboardingRouteRegistrar := func(router fiber.Router) {
		httpin.RegisterOnboardingRoutesToApp(router, auth,
			&httpin.AccountHandler{}, &httpin.PortfolioHandler{}, &httpin.LedgerHandler{},
			&httpin.OrganizationHandler{}, &httpin.SegmentHandler{}, &httpin.AccountTypeHandler{}, nil)
	}
	ledgerRouteRegistrar := httpin.CreateRouteRegistrar(auth, metadataIndexHandler, nil)

	readyzHandler := NewReadyzHandler(ReadyzHandlerConfig{Logger: logger, Version: "test-version"})

	server := NewUnifiedServer(":0", "test-version", logger, telemetry, readyzHandler,
		humaMount, humaMountV2, onboardingRouteRegistrar, ledgerRouteRegistrar)
	require.NotNil(t, server, "NewUnifiedServer should return a non-nil server")
	require.NotNil(t, server.app, "server should hold a Fiber app")

	return server
}

// serializeRouteTable renders every route Fiber holds as one "METHOD\tpath\tcount"
// line, grouped by method and path. GetRoutes promises no ordering across the
// per-method stacks it walks, so grouping by that key is what makes the bytes stable.
//
// The sort is STABLE and the count is deliberately outside the key: rows that share a
// method and a path keep the order Fiber holds them in, which is the order they were
// registered. That order is load-bearing — a guard only reaches its terminal by sitting
// ahead of it — so pinning it means a guard that slips behind its own terminal shows up
// as two swapped lines rather than as no change at all.
func serializeRouteTable(app *fiber.App) string {
	routes := app.GetRoutes()

	type row struct {
		key  string
		line string
	}

	rows := make([]row, 0, len(routes))
	for _, r := range routes {
		key := r.Method + "\t" + r.Path
		rows = append(rows, row{key: key, line: fmt.Sprintf("%s\t%d", key, len(r.Handlers))})
	}

	sort.SliceStable(rows, func(i, j int) bool { return rows[i].key < rows[j].key })

	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, r.line)
	}

	return routeTableGoldenHeader + strings.Join(lines, "\n") + "\n"
}

// TestRouteTableGolden locks the SHAPE of the unified server's route table, which no
// other test asserts on: the route tests prove a path answers, not that it answers
// behind its guard.
//
// The handler count is the field carrying the weight. Auth on this surface is not
// app.Use middleware: a guard chain and the Huma terminal it protects are two separate
// route registrations on the same method and the same RAW path, and equality of that
// raw path is the whole mechanism binding them. The table shows the pair in one of two
// shapes — collapsed into a single row whose count includes both, or left as two
// adjacent rows the router walks in order — and in both the count is what distinguishes
// a guarded terminal from a bare one.
//
// So a terminal that lands on a raw path its guard does not share is exactly what this
// golden is here to catch: it would serve unauthenticated, and the table cannot absorb
// it quietly — a collapsed row loses handlers, a pair loses its partner, and either way
// the bytes move.
//
// With -update-route-table the golden is rewritten; without it, any drift fails.
func TestRouteTableGolden(t *testing.T) {
	// The docs surface is gated on OPENAPI_DOCS_ENABLED and the golden pins the
	// default deployment posture, so the variable must be genuinely absent.
	// unsetDocsGate uses t.Setenv, which precludes t.Parallel here.
	unsetDocsGate(t)

	server := buildFullSurfaceServer(t)

	got := serializeRouteTable(server.app)

	if *updateRouteTable {
		require.NoError(t, os.MkdirAll(filepath.Dir(routeTableGoldenPath), 0o755),
			"create golden directory for %s", routeTableGoldenPath)
		require.NoError(t, os.WriteFile(routeTableGoldenPath, []byte(got), 0o644),
			"write golden route table %s", routeTableGoldenPath)
		t.Logf("wrote golden route table %s (%d routes)", routeTableGoldenPath, len(server.app.GetRoutes()))

		return
	}

	want, err := os.ReadFile(routeTableGoldenPath)
	require.NoErrorf(t, err, "read golden route table %s (run with -update-route-table to generate)", routeTableGoldenPath)
	require.Equalf(t, string(want), got,
		"unified server route table drifted from %s. Check the HANDLER COUNT column first: a count that dropped, or a "+
			"guard row that lost its terminal, means that route may now answer without auth. "+
			"If the change is intended, regenerate with `go test ./components/ledger/internal/bootstrap/ -run TestRouteTableGolden -update-route-table`",
		routeTableGoldenPath)

	t.Logf("route table matches golden: %d routes", len(server.app.GetRoutes()))
}
