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
