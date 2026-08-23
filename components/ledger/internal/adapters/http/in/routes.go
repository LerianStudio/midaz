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

// RegisterCountTransactionRoutesToApp wires the transaction-count HEAD op onto the /v1
// contract. See registerCountTransactionRoutesToApp for what it attaches.
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

// RegisterTransactionHumaRoutesToApp wires the twelve transaction ops (six CREATE —
// json/inflow/outflow/annotation/block/unblock, three id-only STATE, one PATCH, two
// READ). Auth is auth.Authorize("midaz","transactions",verb) + tenant +
// ParseUUIDPathParameters("transaction"), attached as middleware-only on the /v1 group
// BEFORE the Huma terminals, so each op keeps its (appName, resource, verb) tuple. Paths
// are relative to the /v1 group; the Huma terminals are attached by
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

// RegisterOperationRouteRoutesToApp wires the operation-route surface onto
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

// RegisterTransactionRouteRoutesToApp wires the transaction-route surface onto
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
