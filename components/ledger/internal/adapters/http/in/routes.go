// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
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

	f.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/metrics/count", protectedMidaz(auth, "transactions", "head", routeOptions, http.ParseUUIDPathParameters("transaction"), th.CountTransactionsByFilters)...)

	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", protectedMidaz(auth, "transactions", "get", routeOptions, http.ParseUUIDPathParameters("transaction"), th.GetTransaction)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions", protectedMidaz(auth, "transactions", "get", routeOptions, http.ParseUUIDPathParameters("transaction"), th.GetAllTransactions)...)

	// Operations
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations", protectedMidaz(auth, "operations", "get", routeOptions, http.ParseUUIDPathParameters("operation"), oh.GetAllOperationsByAccount)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations/:operation_id", protectedMidaz(auth, "operations", "get", routeOptions, http.ParseUUIDPathParameters("operation"), oh.GetOperationByAccount)...)
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

	// Balance
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", protectedMidaz(auth, "balances", "patch", routeOptions, http.ParseUUIDPathParameters("balance"), http.WithBody(new(mmodel.UpdateBalance), bh.UpdateBalance))...)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", protectedMidaz(auth, "balances", "delete", routeOptions, http.ParseUUIDPathParameters("balance"), bh.DeleteBalanceByID)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/balances", protectedMidaz(auth, "balances", "get", routeOptions, http.ParseUUIDPathParameters("balance"), bh.GetAllBalances)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", protectedMidaz(auth, "balances", "get", routeOptions, http.ParseUUIDPathParameters("balance"), bh.GetBalanceByID)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id/history", protectedMidaz(auth, "balances", "get", routeOptions, http.ParseUUIDPathParameters("balance"), bh.GetBalanceAtTimestamp)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances", protectedMidaz(auth, "balances", "get", routeOptions, http.ParseUUIDPathParameters("balance"), bh.GetAllBalancesByAccountID)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances/history", protectedMidaz(auth, "balances", "get", routeOptions, http.ParseUUIDPathParameters("balance"), bh.GetAccountBalancesAtTimestamp)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias/balances", protectedMidaz(auth, "balances", "get", routeOptions, http.ParseUUIDPathParameters("balance"), bh.GetBalancesByAlias)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/external/:code/balances", protectedMidaz(auth, "balances", "get", routeOptions, http.ParseUUIDPathParameters("balance"), bh.GetBalancesExternalByCode)...)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances", protectedMidaz(auth, "balances", "post", routeOptions, http.ParseUUIDPathParameters("balance"), http.WithBody(new(mmodel.CreateAdditionalBalance), bh.CreateAdditionalBalance))...)

	// Operation-route
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes", protectedRouting(auth, "operation-routes", "post", routeOptions, http.ParseUUIDPathParameters("operation_route"), http.WithBody(new(mmodel.CreateOperationRouteInput), orh.CreateOperationRoute))...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id", protectedRouting(auth, "operation-routes", "get", routeOptions, http.ParseUUIDPathParameters("operation_route"), orh.GetOperationRouteByID)...)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id", protectedRouting(auth, "operation-routes", "patch", routeOptions, http.ParseUUIDPathParameters("operation_route"), http.WithBody(new(mmodel.UpdateOperationRouteInput), orh.UpdateOperationRoute))...)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id", protectedRouting(auth, "operation-routes", "delete", routeOptions, http.ParseUUIDPathParameters("operation_route"), orh.DeleteOperationRouteByID)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes", protectedRouting(auth, "operation-routes", "get", routeOptions, http.ParseUUIDPathParameters("operation_route"), orh.GetAllOperationRoutes)...)

	// Transaction-route
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes", protectedRouting(auth, "transaction-routes", "post", routeOptions, http.ParseUUIDPathParameters("transaction_route"), http.WithBody(new(mmodel.CreateTransactionRouteInput), trh.CreateTransactionRoute))...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id", protectedRouting(auth, "transaction-routes", "get", routeOptions, http.ParseUUIDPathParameters("transaction_route"), trh.GetTransactionRouteByID)...)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id", protectedRouting(auth, "transaction-routes", "patch", routeOptions, http.ParseUUIDPathParameters("transaction_route"), http.WithBody(new(mmodel.UpdateTransactionRouteInput), trh.UpdateTransactionRoute))...)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id", protectedRouting(auth, "transaction-routes", "delete", routeOptions, http.ParseUUIDPathParameters("transaction_route"), trh.DeleteTransactionRouteByID)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes", protectedRouting(auth, "transaction-routes", "get", routeOptions, http.ParseUUIDPathParameters("transaction_route"), trh.GetAllTransactionRoutes)...)
}

func protectedMidaz(auth *middleware.AuthClient, resource, action string, routeOptions *http.ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	return http.ProtectedRouteChain(auth.Authorize(midazName, resource, action), routeOptions, handlers...)
}

func protectedRouting(auth *middleware.AuthClient, resource, action string, routeOptions *http.ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	return http.ProtectedRouteChain(auth.Authorize(routingName, resource, action), routeOptions, handlers...)
}
