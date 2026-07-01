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

// mountWave2Routes wires the five Wave-2 Huma-migrated resources (balance,
// operation-read, transaction-count, operation-route, transaction-route) on a /v1
// group, mirroring the production humaMount seam: problem.Install() before any
// huma.Register, the shared Huma API built with openapi.New over the /v1 group, and
// each RegisterXxxRoutesToApp attaching the Fiber auth+tenant middleware chain (as
// middleware only) plus the Huma terminals on that group.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError
// hook and Huma validation uses process-global sync.Pools.
func mountWave2Routes(app *fiber.App, auth *middleware.AuthClient) huma.API {
	libProblem.Install()
	apiV1 := app.Group("/v1")
	hAPI := openapi.New(app, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})
	pkgHTTP.InstallLedgerSchemaNamer(hAPI)

	RegisterBalanceRoutesToApp(apiV1, hAPI, auth, &BalanceHandler{}, nil)
	RegisterOperationRoutesToApp(apiV1, hAPI, auth, &OperationHandler{}, nil)
	RegisterCountTransactionRoutesToApp(apiV1, hAPI, auth, &TransactionHandler{}, nil)
	RegisterOperationRouteRoutesToApp(apiV1, hAPI, auth, &OperationRouteHandler{}, nil)
	RegisterTransactionRouteRoutesToApp(apiV1, hAPI, auth, &TransactionRouteHandler{}, nil)

	return hAPI
}

// TestWave2RoutesMountedOnGroup asserts every Wave-2 migrated route is served on the
// /v1 group after the Fiber-inline -> Huma migration. Paths + methods are preserved
// byte-for-byte; only the transport changed. A missing route means the auth-middleware
// attach or the Huma registration regressed.
func TestWave2RoutesMountedOnGroup(t *testing.T) {
	// NOT parallel: mountWave2Routes mutates process-global huma state.
	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	mountWave2Routes(app, auth)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	orgLedger := "/v1/organizations/:organization_id/ledgers/:ledger_id"

	want := []string{
		// Balance (10)
		"GET:" + orgLedger + "/balances",
		"GET:" + orgLedger + "/balances/:balance_id",
		"PATCH:" + orgLedger + "/balances/:balance_id",
		"DELETE:" + orgLedger + "/balances/:balance_id",
		"GET:" + orgLedger + "/balances/:balance_id/history",
		"GET:" + orgLedger + "/accounts/:account_id/balances",
		"POST:" + orgLedger + "/accounts/:account_id/balances",
		"GET:" + orgLedger + "/accounts/:account_id/balances/history",
		"GET:" + orgLedger + "/accounts/alias/:alias/balances",
		"GET:" + orgLedger + "/accounts/external/:code/balances",
		// Operation read (2)
		"GET:" + orgLedger + "/accounts/:account_id/operations",
		"GET:" + orgLedger + "/accounts/:account_id/operations/:operation_id",
		// Transaction count (1, explicit HEAD)
		"HEAD:" + orgLedger + "/transactions/metrics/count",
		// Operation-route (5)
		"POST:" + orgLedger + "/operation-routes",
		"GET:" + orgLedger + "/operation-routes",
		"GET:" + orgLedger + "/operation-routes/:operation_route_id",
		"PATCH:" + orgLedger + "/operation-routes/:operation_route_id",
		"DELETE:" + orgLedger + "/operation-routes/:operation_route_id",
		// Transaction-route (5)
		"POST:" + orgLedger + "/transaction-routes",
		"GET:" + orgLedger + "/transaction-routes",
		"GET:" + orgLedger + "/transaction-routes/:transaction_route_id",
		"PATCH:" + orgLedger + "/transaction-routes/:transaction_route_id",
		"DELETE:" + orgLedger + "/transaction-routes/:transaction_route_id",
	}

	for _, w := range want {
		assert.True(t, routeSet[w], "expected mounted route %q", w)
	}
}
