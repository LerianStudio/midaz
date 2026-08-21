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

const midazName = "midaz"

// SettingsMaxPayloadSize defines the maximum payload size for settings endpoints (64KB).
const SettingsMaxPayloadSize = 64 * 1024

// RegisterAssetRoutesToApp wires the Huma-migrated asset surface onto the /v1
// contract. See registerAssetRoutesToApp for what it attaches.
func RegisterAssetRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ih *AssetHandler, routeOptions *http.ProtectedRouteOptions) {
	registerAssetRoutesToApp(group, api, auth, ih, routeOptions, routeOpSuffixV1)
}

// RegisterAssetV2RoutesToApp wires the same asset surface onto the /v2 contract: same paths,
// same handlers, same authz tuples and tenant chain, differing only in the operation IDs the
// contract publishes. It is additive — /v1 keeps serving assets in parallel — and introduces
// no new policy surface.
func RegisterAssetV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ih *AssetHandler, routeOptions *http.ProtectedRouteOptions) {
	registerAssetRoutesToApp(group, api, auth, ih, routeOptions, routeOpSuffixV2)
}

// registerAssetRoutesToApp is the single description of the asset route surface, shared by
// every versioned contract that serves it. For each of the six ops it attaches the Fiber auth
// chain — auth.Authorize("midaz","assets",verb) + tenant PostAuthMiddlewares +
// ParseUUIDPathParameters("asset") — as MIDDLEWARE ONLY (no terminal) on the VERSIONED GROUP
// with GROUP-RELATIVE paths, then registers the Huma terminals via RegisterAssetRoutes on the
// SAME group's Huma API. This preserves the pre-Huma ("midaz","assets",verb) authz tuples and
// tenant resolution BYTE-FOR-BYTE on whichever version group it is mounted on; no asset route
// becomes public.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's — see
// routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface reaches
// every version it is mounted on.
func registerAssetRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ih *AssetHandler, routeOptions *http.ProtectedRouteOptions, opSuffix string) {
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

	RegisterAssetRoutes(api, ih, opSuffix)
}

// RegisterBalanceRoutesToApp wires the Huma-migrated balance surface onto the /v1
// contract. See registerBalanceRoutesToApp for what it attaches.
func RegisterBalanceRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, bh *BalanceHandler, routeOptions *http.ProtectedRouteOptions) {
	registerBalanceRoutesToApp(group, api, auth, bh, routeOptions, routeOpSuffixV1)
}

// RegisterBalanceV2RoutesToApp wires the same balance surface onto the /v2 contract: same
// paths, same handlers, same authz tuples and tenant chain, differing only in the operation
// IDs the contract publishes. It is additive — /v1 keeps serving balances in parallel — and
// introduces no new policy surface.
func RegisterBalanceV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, bh *BalanceHandler, routeOptions *http.ProtectedRouteOptions) {
	registerBalanceRoutesToApp(group, api, auth, bh, routeOptions, routeOpSuffixV2)
}

// registerBalanceRoutesToApp is the single description of the balance route surface, shared by
// every versioned contract that serves it. It attaches the Fiber auth chain —
// auth.Authorize("midaz","balances",verb) + tenant PostAuthMiddlewares +
// ParseUUIDPathParameters("balance") — as MIDDLEWARE ONLY (group-relative paths, no terminal)
// on the VERSIONED GROUP, then registers the Huma terminals via RegisterBalanceRoutes on the
// SAME group's Huma API. The alias/code path segments are NOT UUIDs;
// ParseUUIDPathParameters("balance") only validates org/ledger/balance_id/account_id, so those
// routes pass alias/code through raw (identical to the pre-Huma Fiber path). This preserves the
// ("midaz","balances",verb) authz tuples and tenant resolution BYTE-FOR-BYTE on whichever
// version group it is mounted on.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's — see
// routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface reaches
// every version it is mounted on.
func registerBalanceRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, bh *BalanceHandler, routeOptions *http.ProtectedRouteOptions, opSuffix string) {
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

	RegisterBalanceRoutes(api, bh, opSuffix)
}

// RegisterOperationRoutesToApp wires the Huma-migrated operation surface onto the /v1
// contract. See registerOperationRoutesToApp for what it attaches.
func RegisterOperationRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, oh *OperationHandler, routeOptions *http.ProtectedRouteOptions) {
	registerOperationRoutesToApp(group, api, auth, oh, routeOptions, routeOpSuffixV1)
}

// RegisterOperationV2RoutesToApp wires the same operation surface onto the /v2 contract: same
// paths, same handlers, same authz tuples and tenant chain, differing only in the operation IDs
// the contract publishes. It is additive — /v1 keeps serving operations in parallel — and
// introduces no new policy surface.
func RegisterOperationV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, oh *OperationHandler, routeOptions *http.ProtectedRouteOptions) {
	registerOperationRoutesToApp(group, api, auth, oh, routeOptions, routeOpSuffixV2)
}

// registerOperationRoutesToApp is the single description of the operation route surface, shared
// by every versioned contract that serves it. It attaches the Fiber auth chain for the three
// ops — two READ (GET, on the account path) plus the PATCH (UpdateOperation, on the transaction
// path — a money-write LEG of the double-entry) —
// auth.Authorize("midaz","operations",verb) + tenant PostAuthMiddlewares +
// ParseUUIDPathParameters("operation") — as MIDDLEWARE ONLY (group-relative paths, no terminal)
// on the VERSIONED GROUP, then registers the Huma terminals via RegisterOperationRoutes on the
// SAME group's Huma API. This preserves the pre-Huma ("midaz","operations",verb) authz tuples
// and tenant resolution BYTE-FOR-BYTE on whichever version group it is mounted on.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's — see
// routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface reaches
// every version it is mounted on.
func registerOperationRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, oh *OperationHandler, routeOptions *http.ProtectedRouteOptions, opSuffix string) {
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

	RegisterOperationRoutes(api, oh, opSuffix)
}

// RegisterCountTransactionRoutesToApp wires the Huma-migrated transaction-count HEAD
// op onto the /v1 contract. See registerCountTransactionRoutesToApp for what it attaches.
func RegisterCountTransactionRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *http.ProtectedRouteOptions) {
	registerCountTransactionRoutesToApp(group, api, auth, th, routeOptions, routeOpSuffixV1)
}

// RegisterCountTransactionV2RoutesToApp wires the same transaction-count HEAD op onto the /v2
// contract: same path, same handler, same authz tuple and tenant chain, differing only in the
// operation ID the contract publishes. It is additive — /v1 keeps serving the count in parallel
// — and introduces no new policy surface.
func RegisterCountTransactionV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *http.ProtectedRouteOptions) {
	registerCountTransactionRoutesToApp(group, api, auth, th, routeOptions, routeOpSuffixV2)
}

// registerCountTransactionRoutesToApp is the single description of the transaction-count route
// surface, shared by every versioned contract that serves it. It attaches the Fiber auth chain
// — auth.Authorize("midaz","transactions","head") + tenant PostAuthMiddlewares +
// ParseUUIDPathParameters("transaction") — as MIDDLEWARE ONLY (group-relative path, no terminal)
// on the VERSIONED GROUP, then registers the Huma terminal via RegisterCountTransactionRoutes on
// the SAME group's Huma API. This preserves the ("midaz","transactions","head") authz tuple and
// tenant resolution BYTE-FOR-BYTE on whichever version group it is mounted on.
//
// opSuffix distinguishes the operation ID one version group publishes from another's — see
// routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface reaches
// every version it is mounted on.
func registerCountTransactionRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *http.ProtectedRouteOptions, opSuffix string) {
	const countPath = "/organizations/:organization_id/ledgers/:ledger_id/transactions/metrics/count"

	parse := http.ParseUUIDPathParameters("transaction")

	routeHead(group, countPath, protectedMidaz(auth, "transactions", "head", routeOptions, parse))

	RegisterCountTransactionRoutes(api, th, opSuffix)
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

// RegisterOperationRouteRoutesToApp wires the Huma-migrated operation-route surface onto
// the /v1 contract. See registerOperationRouteRoutesToApp for what it attaches.
func RegisterOperationRouteRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, orh *OperationRouteHandler, routeOptions *http.ProtectedRouteOptions) {
	registerOperationRouteRoutesToApp(group, api, auth, orh, routeOptions, routeOpSuffixV1)
}

// RegisterOperationRouteV2RoutesToApp wires the same operation-route surface onto the /v2
// contract: same paths, same handlers, same authz tuples and tenant chain, differing only in
// the operation IDs the contract publishes. It is additive — /v1 keeps serving operation
// routes in parallel — and introduces no new policy surface.
func RegisterOperationRouteV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, orh *OperationRouteHandler, routeOptions *http.ProtectedRouteOptions) {
	registerOperationRouteRoutesToApp(group, api, auth, orh, routeOptions, routeOpSuffixV2)
}

// registerOperationRouteRoutesToApp is the single description of the operation-route surface,
// shared by every versioned contract that serves it. Auth is the "midaz" appName:
// auth.Authorize("midaz","operation-routes",verb) + tenant +
// ParseUUIDPathParameters("operation_route"), attached as MIDDLEWARE ONLY (group-relative
// paths, no terminal) on the versioned group, then it registers the Huma terminals via
// RegisterOperationRouteRoutes on the SAME group's Huma API. The ("midaz","operation-routes",
// verb) authz tuples and tenant resolution hold on whichever version group it is mounted on.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's — see
// routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface reaches
// every version it is mounted on.
func registerOperationRouteRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, orh *OperationRouteHandler, routeOptions *http.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath = "/organizations/:organization_id/ledgers/:ledger_id/operation-routes"
		idPath   = listPath + "/:operation_route_id"
	)

	parse := http.ParseUUIDPathParameters("operation_route")

	routePost(group, listPath, protectedMidaz(auth, "operation-routes", "post", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "operation-routes", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "operation-routes", "get", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "operation-routes", "patch", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "operation-routes", "delete", routeOptions, parse))

	RegisterOperationRouteRoutes(api, orh, opSuffix)
}

// RegisterTransactionRouteRoutesToApp wires the Huma-migrated transaction-route surface onto
// the /v1 contract. See registerTransactionRouteRoutesToApp for what it attaches.
func RegisterTransactionRouteRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, trh *TransactionRouteHandler, routeOptions *http.ProtectedRouteOptions) {
	registerTransactionRouteRoutesToApp(group, api, auth, trh, routeOptions, routeOpSuffixV1)
}

// RegisterTransactionRouteV2RoutesToApp wires the same transaction-route surface onto the /v2
// contract: same paths, same handlers, same authz tuples and tenant chain, differing only in
// the operation IDs the contract publishes. It is additive — /v1 keeps serving transaction
// routes in parallel — and introduces no new policy surface.
func RegisterTransactionRouteV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, trh *TransactionRouteHandler, routeOptions *http.ProtectedRouteOptions) {
	registerTransactionRouteRoutesToApp(group, api, auth, trh, routeOptions, routeOpSuffixV2)
}

// registerTransactionRouteRoutesToApp is the single description of the transaction-route surface,
// shared by every versioned contract that serves it. Auth is the "midaz" appName:
// auth.Authorize("midaz","transaction-routes",verb) + tenant +
// ParseUUIDPathParameters("transaction_route"), attached as MIDDLEWARE ONLY (group-relative
// paths, no terminal) on the versioned group, then it registers the Huma terminals via
// RegisterTransactionRouteRoutes on the SAME group's Huma API. The ("midaz","transaction-routes",
// verb) authz tuples and tenant resolution hold on whichever version group it is mounted on.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's — see
// routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface reaches
// every version it is mounted on.
func registerTransactionRouteRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, trh *TransactionRouteHandler, routeOptions *http.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath = "/organizations/:organization_id/ledgers/:ledger_id/transaction-routes"
		idPath   = listPath + "/:transaction_route_id"
	)

	parse := http.ParseUUIDPathParameters("transaction_route")

	routePost(group, listPath, protectedMidaz(auth, "transaction-routes", "post", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "transaction-routes", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "transaction-routes", "get", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "transaction-routes", "patch", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "transaction-routes", "delete", routeOptions, parse))

	RegisterTransactionRouteRoutes(api, trh, opSuffix)
}

func protectedMidaz(auth *middleware.AuthClient, resource, action string, routeOptions *http.ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	return http.ProtectedRouteChain(auth.Authorize(midazName, resource, action), routeOptions, handlers...)
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
