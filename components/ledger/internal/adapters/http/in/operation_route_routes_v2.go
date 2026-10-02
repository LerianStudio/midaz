// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// RegisterOrganizationOperationRouteRoutes registers the five organization-level
// operation-route operations on the given Huma API. Paths are GROUP-RELATIVE; the
// Fiber guard chain is attached by registerOrganizationOperationRouteRoutesToApp
// BEFORE the Huma terminal, not here.
//
// opSuffix is appended to every operation ID — see v2OpSuffix.
func RegisterOrganizationOperationRouteRoutes(api huma.API, h *OperationRouteHandler, opSuffix string) {
	const (
		listPath = "/organizations/{organization_id}/operation-routes"
		idPath   = listPath + "/{operation_route_id}"
		tag      = "Operation Routes"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createOrganizationOperationRoute" + opSuffix,
		Method:           http.MethodPost,
		Path:             listPath,
		Summary:          "Create an organization Operation Route",
		Description:      "Creates an operation route that belongs to the organization and validates in every ledger of it. The route has no ledgerId.",
		Tags:             []string{tag},
		Security:         secOperationRouteBearer,
		SkipValidateBody: true, // body validated imperatively (http.DecodeAndValidate).
		DefaultStatus:    http.StatusCreated,
	}, h.CreateOrganizationOperationRoute)
	attachTypedRequestBody[mmodel.CreateOperationRouteInput](api, "createOrganizationOperationRoute"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listOrganizationOperationRoutes" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List the organization Operation Routes",
		Description: "Lists every operation route of the organization, whichever ledger it was created under, including routes created at organization level.",
		Tags:        []string{tag},
		Security:    secOperationRouteBearer,
	}, h.GetAllOrganizationOperationRoutes)

	huma.Register(api, huma.Operation{
		OperationID: "getOrganizationOperationRouteByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get an organization Operation Route by ID",
		Tags:        []string{tag},
		Security:    secOperationRouteBearer,
	}, h.GetOrganizationOperationRouteByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updateOrganizationOperationRoute" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an organization Operation Route",
		Tags:             []string{tag},
		Security:         secOperationRouteBearer,
		SkipValidateBody: true, // body validated imperatively — RFC 7396 merge-patch core.
	}, h.UpdateOrganizationOperationRoute)
	attachTypedRequestBody[mmodel.UpdateOperationRouteInput](api, "updateOrganizationOperationRoute"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "deleteOrganizationOperationRoute" + opSuffix,
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete an organization Operation Route by ID",
		Tags:        []string{tag},
		Security:    secOperationRouteBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteOrganizationOperationRouteByID)
}

// RegisterOrganizationOperationRouteV2RoutesToApp wires the organization-level
// operation-route surface onto the /v2 contract, the ONLY version group that serves
// it. It carries the same ("midaz","operation-routes",verb) tuples and the same
// route options as the ledger-level surface, so it introduces no new policy.
func RegisterOrganizationOperationRouteV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, orh *OperationRouteHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerOrganizationOperationRouteRoutesToApp(group, api, auth, orh, routeOptions, v2OpSuffix)
}

// registerOrganizationOperationRouteRoutesToApp attaches, for each of the five ops,
// auth.Authorize("midaz","operation-routes",verb) + the route options +
// ParseUUIDPathParameters("operation_route") as MIDDLEWARE ONLY on the versioned
// group, then registers the Huma terminals on the SAME group's Huma API.
func registerOrganizationOperationRouteRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, orh *OperationRouteHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath = "/organizations/:organization_id/operation-routes"
		idPath   = listPath + "/:operation_route_id"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("operation_route")

	routePost(group, listPath, protectedMidaz(auth, "operation-routes", "post", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "operation-routes", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "operation-routes", "get", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "operation-routes", "patch", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "operation-routes", "delete", routeOptions, parse))

	RegisterOrganizationOperationRouteRoutes(api, orh, opSuffix)
}
