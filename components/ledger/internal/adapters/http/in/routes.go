// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/pkg/net/http"
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
// (the group's PrefixModifier writes "/v1" into each op's op.Path, not into a servers
// entry, so the Huma absolute path matches the Fiber chain's raw path byte-for-byte).
// Param names
// (:organization_id/:ledger_id/:id) match the Huma path tags exactly.
func RegisterAssetRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ih *AssetHandler, routeOptions *http.ProtectedRouteOptions) {
	const (
		listPath  = "/organizations/:organization_id/ledgers/:ledger_id/assets"
		idPath    = listPath + "/:id"
		countPath = listPath + "/metrics/count"
	)

	parse := http.ParseUUIDPathParameters("asset")

	routePost(group, listPath, protectedMidaz(auth, "assets", "post", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "assets", "patch", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "assets", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "assets", "get", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "assets", "delete", routeOptions, parse))
	routeHead(group, countPath, protectedMidaz(auth, "assets", "head", routeOptions, parse))

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

	routeGet(group, balancesPath, protectedMidaz(auth, "balances", "get", routeOptions, parse))
	routeGet(group, balanceIDPath, protectedMidaz(auth, "balances", "get", routeOptions, parse))
	routePatch(group, balanceIDPath, protectedMidaz(auth, "balances", "patch", routeOptions, parse))
	routeDelete(group, balanceIDPath, protectedMidaz(auth, "balances", "delete", routeOptions, parse))
	routeGet(group, balanceHistory, protectedMidaz(auth, "balances", "get", routeOptions, parse))
	routeGet(group, acctBalances, protectedMidaz(auth, "balances", "get", routeOptions, parse))
	routePost(group, acctBalances, protectedMidaz(auth, "balances", "post", routeOptions, parse))
	routeGet(group, acctHistory, protectedMidaz(auth, "balances", "get", routeOptions, parse))
	routeGet(group, aliasBalances, protectedMidaz(auth, "balances", "get", routeOptions, parse))
	routeGet(group, codeBalances, protectedMidaz(auth, "balances", "get", routeOptions, parse))

	RegisterBalanceRoutes(api, bh)
}

// RegisterOperationRoutesToApp wires the three Huma-migrated operation ops: two READ
// (GET, on the account path) plus the PATCH (UpdateOperation, on the transaction path —
// a money-write LEG of the double-entry). Auth is
// auth.Authorize("midaz","operations",verb) + tenant +
// ParseUUIDPathParameters("operation"), attached as middleware-only on the /v1 group
// before the Huma terminals — the SAME (appName, resource, verb) tuples the inline Fiber
// routes carried, preserved byte-for-byte.
func RegisterOperationRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, oh *OperationHandler, routeOptions *http.ProtectedRouteOptions) {
	const (
		listPath  = "/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations"
		idPath    = listPath + "/:operation_id"
		patchPath = "/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id"
	)

	parse := http.ParseUUIDPathParameters("operation")

	// Two READ ops — ("operations","get").
	routeGet(group, listPath, protectedMidaz(auth, "operations", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "operations", "get", routeOptions, parse))

	// PATCH (money-write leg) — ("operations","patch").
	routePatch(group, patchPath, protectedMidaz(auth, "operations", "patch", routeOptions, parse))

	RegisterOperationRoutes(api, oh)
}

// RegisterCountTransactionRoutesToApp wires the Huma-migrated transaction-count HEAD
// op. Auth is auth.Authorize("midaz","transactions","head") + tenant +
// ParseUUIDPathParameters("transaction"), attached as middleware-only on the /v1
// group before the Huma terminal.
func RegisterCountTransactionRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *http.ProtectedRouteOptions) {
	const countPath = "/organizations/:organization_id/ledgers/:ledger_id/transactions/metrics/count"

	parse := http.ParseUUIDPathParameters("transaction")

	routeHead(group, countPath, protectedMidaz(auth, "transactions", "head", routeOptions, parse))

	RegisterCountTransactionRoutes(api, th)
}

// RegisterTransactionHumaRoutesToApp wires the twelve Wave-4 Huma-migrated transaction
// ops (six CREATE — json/inflow/outflow/annotation/block/unblock, three id-only STATE,
// one PATCH, two READ). Auth is
// auth.Authorize("midaz","transactions",verb) + tenant + ParseUUIDPathParameters
// ("transaction"), attached as middleware-only on the /v1 group BEFORE the Huma terminals
// — the SAME (appName, resource, verb) tuples the inline Fiber routes carried, preserved
// byte-for-byte. Paths are relative to the /v1 group; the Huma terminals are attached by
// RegisterTransactionRoutes.
func RegisterTransactionHumaRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *http.ProtectedRouteOptions) {
	const (
		listPath = "/organizations/:organization_id/ledgers/:ledger_id/transactions"
		idPath   = listPath + "/:transaction_id"
	)

	parse := http.ParseUUIDPathParameters("transaction")

	// Six CREATE ops — ("transactions","post").
	routePost(group, listPath+"/json", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, listPath+"/inflow", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, listPath+"/outflow", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, listPath+"/annotation", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, listPath+"/block", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, listPath+"/unblock", protectedMidaz(auth, "transactions", "post", routeOptions, parse))

	// Three STATE ops (id-only, bodiless) — ("transactions","post").
	routePost(group, idPath+"/commit", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, idPath+"/cancel", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, idPath+"/revert", protectedMidaz(auth, "transactions", "post", routeOptions, parse))

	// PATCH — ("transactions","patch").
	routePatch(group, idPath, protectedMidaz(auth, "transactions", "patch", routeOptions, parse))

	// Two READ ops — ("transactions","get").
	routeGet(group, idPath, protectedMidaz(auth, "transactions", "get", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "transactions", "get", routeOptions, parse))

	RegisterTransactionRoutes(api, th)
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

	routePost(group, listPath, protectedRouting(auth, "operation-routes", "post", routeOptions, parse))
	routeGet(group, listPath, protectedRouting(auth, "operation-routes", "get", routeOptions, parse))
	routeGet(group, idPath, protectedRouting(auth, "operation-routes", "get", routeOptions, parse))
	routePatch(group, idPath, protectedRouting(auth, "operation-routes", "patch", routeOptions, parse))
	routeDelete(group, idPath, protectedRouting(auth, "operation-routes", "delete", routeOptions, parse))

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

	routePost(group, listPath, protectedRouting(auth, "transaction-routes", "post", routeOptions, parse))
	routeGet(group, listPath, protectedRouting(auth, "transaction-routes", "get", routeOptions, parse))
	routeGet(group, idPath, protectedRouting(auth, "transaction-routes", "get", routeOptions, parse))
	routePatch(group, idPath, protectedRouting(auth, "transaction-routes", "patch", routeOptions, parse))
	routeDelete(group, idPath, protectedRouting(auth, "transaction-routes", "delete", routeOptions, parse))

	RegisterTransactionRouteRoutes(api, trh)
}

func protectedMidaz(auth *middleware.AuthClient, resource, action string, routeOptions *http.ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	return http.ProtectedRouteChain(auth.Authorize(midazName, resource, action), routeOptions, handlers...)
}

func protectedRouting(auth *middleware.AuthClient, resource, action string, routeOptions *http.ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	return http.ProtectedRouteChain(auth.Authorize(routingName, resource, action), routeOptions, handlers...)
}

// registerRoute registers a protected handler chain on a Fiber v3 router. Fiber
// v3's route methods take (handler any, handlers ...any) and a []fiber.Handler
// cannot be spread into ...any, so the chain is split across the fixed first
// handler and the variadic tail. The chain always carries at least the auth
// handler, so index 0 is safe.
func registerRoute(r fiber.Router, method, path string, chain []fiber.Handler) {
	tail := make([]any, len(chain)-1)
	for i, h := range chain[1:] {
		tail[i] = h
	}

	r.Add([]string{method}, path, chain[0], tail...)
}

func routePost(r fiber.Router, path string, chain []fiber.Handler) {
	registerRoute(r, fiber.MethodPost, path, chain)
}

func routeGet(r fiber.Router, path string, chain []fiber.Handler) {
	registerRoute(r, fiber.MethodGet, path, chain)
}

func routePatch(r fiber.Router, path string, chain []fiber.Handler) {
	registerRoute(r, fiber.MethodPatch, path, chain)
}

func routePut(r fiber.Router, path string, chain []fiber.Handler) {
	registerRoute(r, fiber.MethodPut, path, chain)
}

func routeDelete(r fiber.Router, path string, chain []fiber.Handler) {
	registerRoute(r, fiber.MethodDelete, path, chain)
}

func routeHead(r fiber.Router, path string, chain []fiber.Handler) {
	registerRoute(r, fiber.MethodHead, path, chain)
}
