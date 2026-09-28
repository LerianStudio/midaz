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

// RegisterOrganizationTransactionRouteRoutes registers the five organization-level
// transaction-route operations on the given Huma API. Paths are GROUP-RELATIVE; the
// Fiber guard chain is attached by registerOrganizationTransactionRouteRoutesToApp
// BEFORE the Huma terminal, not here.
//
// opSuffix is appended to every operation ID — see v2OpSuffix.
func RegisterOrganizationTransactionRouteRoutes(api huma.API, h *TransactionRouteHandler, opSuffix string) {
	const (
		listPath = "/organizations/{organization_id}/transaction-routes"
		idPath   = listPath + "/{transaction_route_id}"
		tag      = "Transaction Routes"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createOrganizationTransactionRoute" + opSuffix,
		Method:           http.MethodPost,
		Path:             listPath,
		Summary:          "Create an organization Transaction Route",
		Description:      "Creates a transaction route that belongs to the organization and validates in every ledger of it. The route has no ledgerId.",
		Tags:             []string{tag},
		Security:         secTransactionRouteBearer,
		SkipValidateBody: true, // body validated imperatively (http.DecodeAndValidate).
		DefaultStatus:    http.StatusCreated,
	}, h.CreateOrganizationTransactionRoute)
	attachTypedRequestBody[mmodel.CreateTransactionRouteInput](api, "createOrganizationTransactionRoute"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listOrganizationTransactionRoutes" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List the organization Transaction Routes",
		Description: "Lists every transaction route of the organization, whichever ledger it was created under, including routes created at organization level.",
		Tags:        []string{tag},
		Security:    secTransactionRouteBearer,
	}, h.GetAllOrganizationTransactionRoutes)

	huma.Register(api, huma.Operation{
		OperationID: "getOrganizationTransactionRouteByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get an organization Transaction Route by ID",
		Tags:        []string{tag},
		Security:    secTransactionRouteBearer,
	}, h.GetOrganizationTransactionRouteByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updateOrganizationTransactionRoute" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an organization Transaction Route",
		Tags:             []string{tag},
		Security:         secTransactionRouteBearer,
		SkipValidateBody: true, // body validated imperatively (http.DecodeAndValidate).
	}, h.UpdateOrganizationTransactionRoute)
	attachTypedRequestBody[mmodel.UpdateTransactionRouteInput](api, "updateOrganizationTransactionRoute"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "deleteOrganizationTransactionRoute" + opSuffix,
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete an organization Transaction Route by ID",
		Tags:        []string{tag},
		Security:    secTransactionRouteBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteOrganizationTransactionRouteByID)
}

// RegisterOrganizationTransactionRouteV2RoutesToApp wires the organization-level
// transaction-route surface onto the /v2 contract, the ONLY version group that serves
// it. It carries the same ("midaz","transaction-routes",verb) tuples and the same
// route options as the ledger-level surface, so it introduces no new policy.
func RegisterOrganizationTransactionRouteV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, trh *TransactionRouteHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerOrganizationTransactionRouteRoutesToApp(group, api, auth, trh, routeOptions, v2OpSuffix)
}

// registerOrganizationTransactionRouteRoutesToApp attaches, for each of the five ops,
// auth.Authorize("midaz","transaction-routes",verb) + the route options +
// ParseUUIDPathParameters("transaction_route") as MIDDLEWARE ONLY on the versioned
// group, then registers the Huma terminals on the SAME group's Huma API.
func registerOrganizationTransactionRouteRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, trh *TransactionRouteHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath = "/organizations/:organization_id/transaction-routes"
		idPath   = listPath + "/:transaction_route_id"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("transaction_route")

	routePost(group, listPath, protectedMidaz(auth, "transaction-routes", "post", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "transaction-routes", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "transaction-routes", "get", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "transaction-routes", "patch", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "transaction-routes", "delete", routeOptions, parse))

	RegisterOrganizationTransactionRouteRoutes(api, trh, opSuffix)
}
