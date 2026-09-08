// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// RegisterTransactionRouteRoutes registers the five transaction-route
// operations on the shared Huma API. It is the per-file seam the unified server
// calls; the auth ("midaz","transaction-routes",verb) + tenant +
// ParseUUIDPathParameters("transaction_route") middleware chain is attached on the
// /v1 group (Fiber-level) BEFORE the Huma terminal, not here. Paths are
// GROUP-RELATIVE (see asset_handler.go's RegisterAssetRoutes header for the
// /v1 rationale).
//
// opSuffix distinguishes the operation IDs one version group publishes from another's (empty
// for /v1, "V2" for /v2 — see v1OpSuffix). The v2 twin is a straight mirror: same handler
// methods, same paths, same input/output types, differing only in the suffixed operation IDs so
// the two twins do not collide as a duplicate operationId in the one shared document.
func RegisterTransactionRouteRoutes(api huma.API, h *TransactionRouteHandler, opSuffix string) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes"
		idPath   = listPath + "/{transaction_route_id}"
		tag      = "Transaction Routes"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createTransactionRoute" + opSuffix,
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create Transaction Route",
		Tags:        []string{tag},
		Security:    secTransactionRouteBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionRoute)
	attachTypedRequestBody[mmodel.CreateTransactionRouteInput](api, "createTransactionRoute"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listTransactionRoutes" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all Transaction Routes",
		Tags:        []string{tag},
		Security:    secTransactionRouteBearer,
	}, h.GetAllTransactionRoutes)

	huma.Register(api, huma.Operation{
		OperationID: "getTransactionRouteByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get Transaction Route by ID",
		Tags:        []string{tag},
		Security:    secTransactionRouteBearer,
	}, h.GetTransactionRouteByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updateTransactionRoute" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update Transaction Route",
		Tags:             []string{tag},
		Security:         secTransactionRouteBearer,
		SkipValidateBody: true, // body validated imperatively — see file header.
	}, h.UpdateTransactionRoute)
	attachTypedRequestBody[mmodel.UpdateTransactionRouteInput](api, "updateTransactionRoute"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "deleteTransactionRoute" + opSuffix,
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete Transaction Route by ID",
		Tags:        []string{tag},
		Security:    secTransactionRouteBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteTransactionRouteByID)
}

// RegisterTransactionRouteRoutesToApp wires the transaction-route surface onto
// the /v1 contract. See registerTransactionRouteRoutesToApp for what it attaches.
func RegisterTransactionRouteRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, trh *TransactionRouteHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerTransactionRouteRoutesToApp(group, api, auth, trh, routeOptions, v1OpSuffix)
}

// RegisterTransactionRouteV2RoutesToApp wires the same transaction-route surface onto the /v2
// contract: same paths, same handlers, same authz tuples and tenant chain, differing only in
// the operation IDs the contract publishes. It is additive — /v1 keeps serving transaction
// routes in parallel — and introduces no new policy surface.
func RegisterTransactionRouteV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, trh *TransactionRouteHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerTransactionRouteRoutesToApp(group, api, auth, trh, routeOptions, v2OpSuffix)
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
// v1OpSuffix. Nothing else varies between contracts, so a change to the surface reaches
// every version it is mounted on.
func registerTransactionRouteRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, trh *TransactionRouteHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath = "/organizations/:organization_id/ledgers/:ledger_id/transaction-routes"
		idPath   = listPath + "/:transaction_route_id"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("transaction_route")

	routePost(group, listPath, protectedMidaz(auth, "transaction-routes", "post", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "transaction-routes", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "transaction-routes", "get", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "transaction-routes", "patch", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "transaction-routes", "delete", routeOptions, parse))

	RegisterTransactionRouteRoutes(api, trh, opSuffix)
}
