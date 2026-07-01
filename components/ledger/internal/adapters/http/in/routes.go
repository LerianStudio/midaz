// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
)

const (
	midazName   = "midaz"
	routingName = "routing"
)

// SettingsMaxPayloadSize defines the maximum payload size for settings endpoints (64KB).
const SettingsMaxPayloadSize = 64 * 1024

// RegisterMetadataRoutesToApp registers ledger routes (metadata indexes) to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
//
// Wave-1 MIGRATED TO HUMA: the metadata-index routes no longer register inline here.
// Their terminal handlers live on the shared Huma API and their auth + tenant
// middleware chain (authz resource "settings", NOT "metadata-indexes") is attached on
// the /v1 group by RegisterMetadataIndexRoutesToApp, called from the unified server's
// humaMount. The (resource, verb) authz tuples are preserved byte-for-byte there.
//
// The parameters are retained on this signature (blanked for now) because
// CreateRouteRegistrar and the contract-spec test still construct and pass them.
func RegisterMetadataRoutesToApp(_ fiber.Router, _ *middleware.AuthClient, _ *MetadataIndexHandler, _ *http.ProtectedRouteOptions) {
}

// CreateRouteRegistrar returns a function that registers ledger routes to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
func CreateRouteRegistrar(auth *middleware.AuthClient, mdi *MetadataIndexHandler, routeOptions *http.ProtectedRouteOptions) func(fiber.Router) {
	return func(fiberRouter fiber.Router) {
		RegisterMetadataRoutesToApp(fiberRouter, auth, mdi, routeOptions)
	}
}

// RegisterOnboardingRoutesToApp registers onboarding routes to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
// The app should already have middleware configured (telemetry, cors, logging).
//
// Wave-1 MIGRATED TO HUMA: organizations, ledgers, portfolios, segments, accounts,
// and account-types no longer register inline here. Their terminal handlers live on
// the shared Huma API and their auth + tenant + ParseUUIDPathParameters middleware
// chains are attached on the /v1 group by the per-resource RegisterXxxRoutesToApp
// wrappers (RegisterOrganizationRoutesToApp, RegisterLedgerRoutesToApp,
// RegisterPortfolioRoutesToApp, RegisterSegmentRoutesToApp, RegisterAccountRoutesToApp,
// RegisterAccountTypeRoutesToApp), all called from the unified server's humaMount.
// The (resource, verb) authz tuples are preserved byte-for-byte in those wrappers.
//
// The handler parameters are retained on this signature (blanked for now) because the
// unified server and contract-spec test still construct and pass them, and the
// non-migrated Wave 3/4 onboarding routes will re-attach here as they land.
func RegisterOnboardingRoutesToApp(_ fiber.Router, _ *middleware.AuthClient, _ *AccountHandler, _ *PortfolioHandler, _ *LedgerHandler, _ *OrganizationHandler, _ *SegmentHandler, _ *AccountTypeHandler, _ *http.ProtectedRouteOptions) {
}

// RegisterAssetRoutesToApp wires the Huma-migrated asset resource. For each of the
// six ops it attaches the Fiber auth chain — auth.Authorize("midaz","assets",verb)
// + the tenant PostAuthMiddlewares + ParseUUIDPathParameters("asset") — as
// MIDDLEWARE ONLY (no terminal handler) on the /v1 GROUP with GROUP-RELATIVE paths,
// then registers the Huma terminals via RegisterAssetRoutes on the SAME group's
// Huma API. Fiber runs the middleware chain first; its final ParseUUIDPathParameters
// calls c.Next(), advancing into the Huma terminal. This preserves the pre-Huma
// (resource, verb) authz tuples and tenant resolution BYTE-FOR-BYTE — no asset
// route becomes public — while the Huma terminal owns request/response shaping.
//
// The group-relative middleware paths (e.g. "/organizations/:organization_id/.../assets")
// resolve to the same absolute "/v1/organizations/.../assets" the Huma op paths do
// (Huma advertises the "/v1" server prefix and registers relative). Param names
// (:organization_id/:ledger_id/:id) match the Huma path tags exactly.
func RegisterAssetRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ih *AssetHandler, routeOptions *http.ProtectedRouteOptions) {
	const (
		listPath  = "/organizations/:organization_id/ledgers/:ledger_id/assets"
		idPath    = listPath + "/:id"
		countPath = listPath + "/metrics/count"
	)

	parse := http.ParseUUIDPathParameters("asset")

	group.Post(listPath, protectedMidaz(auth, "assets", "post", routeOptions, parse)...)
	group.Patch(idPath, protectedMidaz(auth, "assets", "patch", routeOptions, parse)...)
	group.Get(listPath, protectedMidaz(auth, "assets", "get", routeOptions, parse)...)
	group.Get(idPath, protectedMidaz(auth, "assets", "get", routeOptions, parse)...)
	group.Delete(idPath, protectedMidaz(auth, "assets", "delete", routeOptions, parse)...)
	group.Head(countPath, protectedMidaz(auth, "assets", "head", routeOptions, parse)...)

	RegisterAssetRoutes(api, ih)
}

// RegisterBalanceRoutesToApp wires the Huma-migrated balance resource, mirroring
// RegisterAssetRoutesToApp: it attaches the Fiber auth chain —
// auth.Authorize("midaz","balances",verb) + tenant PostAuthMiddlewares +
// ParseUUIDPathParameters("balance") — as MIDDLEWARE ONLY (group-relative paths,
// no terminal) on the /v1 group, then registers the Huma terminals via
// RegisterBalanceRoutes on the SAME group's Huma API. The alias/code path segments
// are NOT UUIDs; ParseUUIDPathParameters("balance") only validates org/ledger/
// balance_id/account_id, so those routes pass alias/code through raw (identical to
// the pre-Huma Fiber path).
func RegisterBalanceRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, bh *BalanceHandler, routeOptions *http.ProtectedRouteOptions) {
	const (
		orgLedger      = "/organizations/:organization_id/ledgers/:ledger_id"
		balancesPath   = orgLedger + "/balances"
		balanceIDPath  = balancesPath + "/:balance_id"
		balanceHistory = balanceIDPath + "/history"
		acctBalances   = orgLedger + "/accounts/:account_id/balances"
		acctHistory    = acctBalances + "/history"
		aliasBalances  = orgLedger + "/accounts/alias/:alias/balances"
		codeBalances   = orgLedger + "/accounts/external/:code/balances"
	)

	parse := http.ParseUUIDPathParameters("balance")

	group.Get(balancesPath, protectedMidaz(auth, "balances", "get", routeOptions, parse)...)
	group.Get(balanceIDPath, protectedMidaz(auth, "balances", "get", routeOptions, parse)...)
	group.Patch(balanceIDPath, protectedMidaz(auth, "balances", "patch", routeOptions, parse)...)
	group.Delete(balanceIDPath, protectedMidaz(auth, "balances", "delete", routeOptions, parse)...)
	group.Get(balanceHistory, protectedMidaz(auth, "balances", "get", routeOptions, parse)...)
	group.Get(acctBalances, protectedMidaz(auth, "balances", "get", routeOptions, parse)...)
	group.Post(acctBalances, protectedMidaz(auth, "balances", "post", routeOptions, parse)...)
	group.Get(acctHistory, protectedMidaz(auth, "balances", "get", routeOptions, parse)...)
	group.Get(aliasBalances, protectedMidaz(auth, "balances", "get", routeOptions, parse)...)
	group.Get(codeBalances, protectedMidaz(auth, "balances", "get", routeOptions, parse)...)

	RegisterBalanceRoutes(api, bh)
}

// RegisterOperationRoutesToApp wires the two Huma-migrated operation READ ops. Auth
// is auth.Authorize("midaz","operations","get") + tenant +
// ParseUUIDPathParameters("operation"), attached as middleware-only on the /v1 group
// before the Huma terminals. The operation PATCH (UpdateOperation) is NOT migrated —
// it stays inline Fiber in RegisterTransactionRoutesToApp.
func RegisterOperationRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, oh *OperationHandler, routeOptions *http.ProtectedRouteOptions) {
	const (
		listPath = "/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations"
		idPath   = listPath + "/:operation_id"
	)

	parse := http.ParseUUIDPathParameters("operation")

	group.Get(listPath, protectedMidaz(auth, "operations", "get", routeOptions, parse)...)
	group.Get(idPath, protectedMidaz(auth, "operations", "get", routeOptions, parse)...)

	RegisterOperationRoutes(api, oh)
}

// RegisterCountTransactionRoutesToApp wires the Huma-migrated transaction-count HEAD
// op. Auth is auth.Authorize("midaz","transactions","head") + tenant +
// ParseUUIDPathParameters("transaction"), attached as middleware-only on the /v1
// group before the Huma terminal.
func RegisterCountTransactionRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *http.ProtectedRouteOptions) {
	const countPath = "/organizations/:organization_id/ledgers/:ledger_id/transactions/metrics/count"

	parse := http.ParseUUIDPathParameters("transaction")

	group.Head(countPath, protectedMidaz(auth, "transactions", "head", routeOptions, parse)...)

	RegisterCountTransactionRoutes(api, th)
}

// RegisterOperationRouteRoutesToApp wires the five Huma-migrated operation-route ops.
// Auth is the "routing" appName: auth.Authorize("routing","operation-routes",verb) +
// tenant + ParseUUIDPathParameters("operation_route"), attached as middleware-only on
// the /v1 group before the Huma terminals.
func RegisterOperationRouteRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, orh *OperationRouteHandler, routeOptions *http.ProtectedRouteOptions) {
	const (
		listPath = "/organizations/:organization_id/ledgers/:ledger_id/operation-routes"
		idPath   = listPath + "/:operation_route_id"
	)

	parse := http.ParseUUIDPathParameters("operation_route")

	group.Post(listPath, protectedRouting(auth, "operation-routes", "post", routeOptions, parse)...)
	group.Get(listPath, protectedRouting(auth, "operation-routes", "get", routeOptions, parse)...)
	group.Get(idPath, protectedRouting(auth, "operation-routes", "get", routeOptions, parse)...)
	group.Patch(idPath, protectedRouting(auth, "operation-routes", "patch", routeOptions, parse)...)
	group.Delete(idPath, protectedRouting(auth, "operation-routes", "delete", routeOptions, parse)...)

	RegisterOperationRouteRoutes(api, orh)
}

// RegisterTransactionRouteRoutesToApp wires the five Huma-migrated transaction-route
// ops. Auth is the "routing" appName: auth.Authorize("routing","transaction-routes",
// verb) + tenant + ParseUUIDPathParameters("transaction_route"), attached as
// middleware-only on the /v1 group before the Huma terminals.
func RegisterTransactionRouteRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, trh *TransactionRouteHandler, routeOptions *http.ProtectedRouteOptions) {
	const (
		listPath = "/organizations/:organization_id/ledgers/:ledger_id/transaction-routes"
		idPath   = listPath + "/:transaction_route_id"
	)

	parse := http.ParseUUIDPathParameters("transaction_route")

	group.Post(listPath, protectedRouting(auth, "transaction-routes", "post", routeOptions, parse)...)
	group.Get(listPath, protectedRouting(auth, "transaction-routes", "get", routeOptions, parse)...)
	group.Get(idPath, protectedRouting(auth, "transaction-routes", "get", routeOptions, parse)...)
	group.Patch(idPath, protectedRouting(auth, "transaction-routes", "patch", routeOptions, parse)...)
	group.Delete(idPath, protectedRouting(auth, "transaction-routes", "delete", routeOptions, parse)...)

	RegisterTransactionRouteRoutes(api, trh)
}

// RegisterTransactionRoutesToApp registers transaction routes to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
// The app should already have middleware configured (telemetry, cors, logging).
func RegisterTransactionRoutesToApp(f fiber.Router, auth *middleware.AuthClient, th *TransactionHandler, oh *OperationHandler, ah *AssetRateHandler, bh *BalanceHandler, orh *OperationRouteHandler, trh *TransactionRouteHandler, routeOptions *http.ProtectedRouteOptions) {
	// Transactions
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/dsl", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), th.CreateTransactionDSL)...)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/json", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), http.WithBody(new(mtransaction.CreateTransactionInput), th.CreateTransactionJSON))...)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/inflow", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), http.WithBody(new(mtransaction.CreateTransactionInflowInput), th.CreateTransactionInflow))...)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/outflow", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), http.WithBody(new(mtransaction.CreateTransactionOutflowInput), th.CreateTransactionOutflow))...)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/annotation", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), http.WithBody(new(mtransaction.CreateTransactionInput), th.CreateTransactionAnnotation))...)

	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), th.CommitTransaction)...)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/cancel", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), th.CancelTransaction)...)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), th.RevertTransaction)...)

	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", protectedMidaz(auth, "transactions", "patch", routeOptions, http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.UpdateTransactionInput), th.UpdateTransaction))...)

	// Transaction-count HEAD — Wave-2 MIGRATED TO HUMA (see RegisterCountTransactionRoutesToApp).
	// The metrics/count HEAD op no longer registers inline here; its terminal lives on the
	// shared Huma API and its auth ("transactions","head") + tenant + ParseUUIDPathParameters
	// ("transaction") chain is attached on the /v1 group by RegisterCountTransactionRoutesToApp,
	// called from the unified server's humaMount. The tuple is preserved byte-for-byte there.

	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", protectedMidaz(auth, "transactions", "get", routeOptions, http.ParseUUIDPathParameters("transaction"), th.GetTransaction)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions", protectedMidaz(auth, "transactions", "get", routeOptions, http.ParseUUIDPathParameters("transaction"), th.GetAllTransactions)...)

	// Operations — the two read (GET) ops are Wave-2 MIGRATED TO HUMA (see
	// RegisterOperationRoutesToApp): their terminals live on the shared Huma API and their
	// auth ("operations","get") + tenant + ParseUUIDPathParameters("operation") chain is
	// attached on the /v1 group by RegisterOperationRoutesToApp, called from humaMount. The
	// PATCH UpdateOperation op is NOT migrated — it stays inline Fiber below.
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id", protectedMidaz(auth, "operations", "patch", routeOptions, http.ParseUUIDPathParameters("operation"), http.WithBody(new(operation.UpdateOperationInput), oh.UpdateOperation))...)

	// Asset-rate — Wave-1 MIGRATED TO HUMA (see RegisterAssetRateRoutesToApp). The
	// three asset-rate ops no longer register inline here; their terminal handlers
	// live on the shared Huma API and their auth ("asset-rates", verb) + tenant +
	// ParseUUIDPathParameters("asset-rate") chain is attached on the /v1 group by
	// RegisterAssetRateRoutesToApp, called from the unified server's humaMount.
	// asset-rate is MONEY-adjacent; the authz tuples are preserved byte-for-byte
	// there. The ah *AssetRateHandler param is retained on this signature (blanked
	// below) because the unified server and contract-spec test still pass it.
	_ = ah

	// Balance, operation-route, and transaction-route — Wave-2 MIGRATED TO HUMA. The ten
	// balance ops, five operation-route ops, and five transaction-route ops no longer
	// register inline here; their terminal handlers live on the shared Huma API and their
	// auth + tenant + ParseUUIDPathParameters chains are attached on the /v1 group by the
	// per-resource RegisterXxxRoutesToApp wrappers (RegisterBalanceRoutesToApp,
	// RegisterOperationRouteRoutesToApp, RegisterTransactionRouteRoutesToApp), all called
	// from the unified server's humaMount. The (appName, resource, verb) authz tuples are
	// preserved byte-for-byte there — balance under "midaz","balances"; the two route
	// resources under "routing","operation-routes"/"transaction-routes". The bh/orh/trh
	// handler params are retained on this signature (blanked below) because the unified
	// server and contract-spec test still construct and pass them, and the non-migrated
	// transaction write/DSL ops above still use th/oh/ah.
	_, _, _ = bh, orh, trh
}

func protectedMidaz(auth *middleware.AuthClient, resource, action string, routeOptions *http.ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	return http.ProtectedRouteChain(auth.Authorize(midazName, resource, action), routeOptions, handlers...)
}

func protectedRouting(auth *middleware.AuthClient, resource, action string, routeOptions *http.ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	return http.ProtectedRouteChain(auth.Authorize(routingName, resource, action), routeOptions, handlers...)
}
