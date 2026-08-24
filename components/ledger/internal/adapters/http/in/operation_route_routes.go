// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// RegisterOperationRouteRoutes registers the five operation-route operations on the
// shared Huma API. It is the per-file seam the unified server calls; the auth
// ("midaz","operation-routes",verb) + tenant +
// ParseUUIDPathParameters("operation_route") middleware chain is attached on the
// version group (Fiber-level) BEFORE the Huma terminal, not here. Paths are
// GROUP-RELATIVE (see asset_handler.go's RegisterAssetRoutes header for the
// rationale).
//
// opSuffix is appended to every operation ID so the same surface can be published on
// more than one version group of the one document without colliding — see
// v1OpSuffix.
func RegisterOperationRouteRoutes(api huma.API, h *OperationRouteHandler, opSuffix string) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/operation-routes"
		idPath   = listPath + "/{operation_route_id}"
		tag      = "Operation Routes"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createOperationRoute" + opSuffix,
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create Operation Route",
		Tags:        []string{tag},
		Security:    secOperationRouteBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateOperationRoute)
	attachTypedRequestBody[mmodel.CreateOperationRouteInput](api, "createOperationRoute"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listOperationRoutes" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Retrieve all operation routes",
		Tags:        []string{tag},
		Security:    secOperationRouteBearer,
	}, h.GetAllOperationRoutes)

	huma.Register(api, huma.Operation{
		OperationID: "getOperationRouteByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific operation route",
		Tags:        []string{tag},
		Security:    secOperationRouteBearer,
	}, h.GetOperationRouteByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updateOperationRoute" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an operation route",
		Tags:             []string{tag},
		Security:         secOperationRouteBearer,
		SkipValidateBody: true, // body validated imperatively — RFC 7396 merge-patch core.
	}, h.UpdateOperationRoute)
	attachTypedRequestBody[mmodel.UpdateOperationRouteInput](api, "updateOperationRoute"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "deleteOperationRoute" + opSuffix,
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete an operation route",
		Tags:        []string{tag},
		Security:    secOperationRouteBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteOperationRouteByID)
}

// RegisterOperationRouteRoutesToApp wires the operation-route surface onto
// the /v1 contract. See registerOperationRouteRoutesToApp for what it attaches.
func RegisterOperationRouteRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, orh *OperationRouteHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerOperationRouteRoutesToApp(group, api, auth, orh, routeOptions, v1OpSuffix)
}

// RegisterOperationRouteV2RoutesToApp wires the same operation-route surface onto the /v2
// contract: same paths, same handlers, same authz tuples and tenant chain, differing only in
// the operation IDs the contract publishes. It is additive — /v1 keeps serving operation
// routes in parallel — and introduces no new policy surface.
func RegisterOperationRouteV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, orh *OperationRouteHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerOperationRouteRoutesToApp(group, api, auth, orh, routeOptions, v2OpSuffix)
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
// v1OpSuffix. Nothing else varies between contracts, so a change to the surface reaches
// every version it is mounted on.
func registerOperationRouteRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, orh *OperationRouteHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath = "/organizations/:organization_id/ledgers/:ledger_id/operation-routes"
		idPath   = listPath + "/:operation_route_id"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("operation_route")

	routePost(group, listPath, protectedMidaz(auth, "operation-routes", "post", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "operation-routes", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "operation-routes", "get", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "operation-routes", "patch", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "operation-routes", "delete", routeOptions, parse))

	RegisterOperationRouteRoutes(api, orh, opSuffix)
}
