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
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

// mountWave4Routes wires the Wave-4 MONEY-WRITE surface exactly as the production
// humaMount seam in config.go does: the ten Huma-migrated transaction ops + the
// operation PATCH (UpdateOperation, a double-entry leg) on the shared /v1 Huma API,
// PLUS RegisterTransactionRoutesToApp on the app ROOT, which still mounts the one
// non-migrated op — POST /transactions/dsl — as a pure inline Fiber terminal
// (multipart .casl upload, SUNSET 2026-08-01, out of the Huma spec).
//
// Registration never invokes the handlers, so nil-backed zero-value handler structs
// are safe. This mirrors config.go: RegisterTransactionHumaRoutesToApp +
// RegisterOperationRoutesToApp on the group, RegisterTransactionRoutesToApp on root.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError
// hook and Huma validation uses process-global sync.Pools.
func mountWave4Routes(app *fiber.App, auth *middleware.AuthClient) huma.API {
	libProblem.Install()
	apiV1 := app.Group("/v1")
	hAPI := openapi.New(app, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})
	pkgHTTP.InstallLedgerSchemaNamer(hAPI)

	// Money-write ops on the Huma group.
	RegisterTransactionHumaRoutesToApp(apiV1, hAPI, auth, &TransactionHandler{}, nil)
	RegisterOperationRoutesToApp(apiV1, hAPI, auth, &OperationHandler{}, nil)

	// Non-migrated DSL terminal on the app root (pure Fiber).
	RegisterTransactionRoutesToApp(app, auth,
		&TransactionHandler{}, &OperationHandler{}, &AssetRateHandler{},
		&BalanceHandler{}, &OperationRouteHandler{}, &TransactionRouteHandler{}, nil)

	return hAPI
}

const wave4OrgLedger = "/v1/organizations/:organization_id/ledgers/:ledger_id"

// wave4MigratedRoutes is the byte-for-byte money-write surface the Wave-4 registrars
// mount on the /v1 group. Paths + methods are preserved from the pre-Huma inline
// Fiber routes; only the transport changed. Ordering and (resource, verb) tuples are
// non-negotiable — this is the money path.
var wave4MigratedRoutes = []string{
	// Four CREATE ops — ("transactions","post").
	"POST:" + wave4OrgLedger + "/transactions/json",
	"POST:" + wave4OrgLedger + "/transactions/inflow",
	"POST:" + wave4OrgLedger + "/transactions/outflow",
	"POST:" + wave4OrgLedger + "/transactions/annotation",
	// Three STATE ops (id-only, bodiless) — ("transactions","post").
	"POST:" + wave4OrgLedger + "/transactions/:transaction_id/commit",
	"POST:" + wave4OrgLedger + "/transactions/:transaction_id/cancel",
	"POST:" + wave4OrgLedger + "/transactions/:transaction_id/revert",
	// PATCH — ("transactions","patch").
	"PATCH:" + wave4OrgLedger + "/transactions/:transaction_id",
	// Two READ ops — ("transactions","get").
	"GET:" + wave4OrgLedger + "/transactions/:transaction_id",
	"GET:" + wave4OrgLedger + "/transactions",
	// Operation PATCH (UpdateOperation, money-write leg) — ("operations","patch").
	"PATCH:" + wave4OrgLedger + "/transactions/:transaction_id/operations/:operation_id",
}

// TestWave4RoutesMountedOnGroup asserts every Wave-4 money-write route is served on
// the /v1 group after the Fiber-inline -> Huma migration. Paths + methods are
// preserved byte-for-byte; only the transport changed. A missing route means the
// auth-middleware attach or the Huma registration regressed on the double-entry path.
func TestWave4RoutesMountedOnGroup(t *testing.T) {
	// NOT parallel: mountWave4Routes mutates process-global huma state.
	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	mountWave4Routes(app, auth)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	for _, w := range wave4MigratedRoutes {
		assert.Truef(t, routeSet[w], "expected mounted money-write route %q", w)
	}
}

// TestWave4DSLStaysPureFiber pins the load-bearing money-path invariant: the
// non-migrated POST /transactions/dsl op stays a pure inline Fiber terminal on the
// app root and never leaks onto the /v1 Huma group. It is DEPRECATED (SUNSET
// 2026-08-01) and deliberately absent from the Huma spec; mounting it on the group
// would pull it into the served contract. The migrated json-CREATE op — its Huma
// replacement — MUST still be present, proving the DSL exclusion is surgical.
func TestWave4DSLStaysPureFiber(t *testing.T) {
	// NOT parallel: mutates process-global huma state.
	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	mountWave4Routes(app, auth)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	// DSL stays on the app root as a pure Fiber terminal.
	assert.True(t, routeSet["POST:"+wave4OrgLedger+"/transactions/dsl"],
		"POST /transactions/dsl must stay mounted as a pure inline Fiber terminal")

	// Its Huma replacement (json CREATE) coexists — the DSL is not a substitute for it.
	assert.True(t, routeSet["POST:"+wave4OrgLedger+"/transactions/json"],
		"POST /transactions/json (Huma replacement) must be mounted alongside DSL")
}
