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

// RegisterHolderRoutes registers the five holder operations on the given
// Huma API. It is the per-file seam the unified server calls; the auth
// ("midaz","holders",verb) + tenant + ParseUUIDPathParameters("holder") middleware
// chain is attached on the versioned Fiber group BEFORE the Huma terminal, not here.
// Paths are GROUP-RELATIVE (see asset_handler.go's RegisterAssetRoutes header
// for the rationale).
//
// opSuffix is appended to every operation ID — see v2OpSuffix.
func RegisterHolderRoutes(api huma.API, h *HolderHandler, opSuffix string) {
	const (
		listPath = "/organizations/{organization_id}/holders"
		idPath   = listPath + "/{id}"
		tag      = "Holders"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createHolder" + opSuffix,
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create a Holder",
		Tags:        []string{tag},
		Security:    secHolderBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateHolder)
	attachTypedRequestBody[mmodel.CreateHolderInput](api, "createHolder"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "getHolderByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve Holder details",
		Tags:        []string{tag},
		Security:    secHolderBearer,
	}, h.GetHolderByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updateHolder" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a Holder",
		Tags:             []string{tag},
		Security:         secHolderBearer,
		SkipValidateBody: true, // body validated imperatively — RFC 7396 merge-patch core.
	}, h.UpdateHolder)
	attachTypedRequestBody[mmodel.UpdateHolderInput](api, "updateHolder"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "deleteHolder" + opSuffix,
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete a Holder",
		Tags:        []string{tag},
		Security:    secHolderBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteHolderByID)

	huma.Register(api, huma.Operation{
		OperationID: "listHolders" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List Holders",
		Tags:        []string{tag},
		Security:    secHolderBearer,
	}, h.GetAllHolders)
}

// RegisterHolderV2RoutesToApp wires the holder surface onto the /v2 contract, which is
// the ONLY version group that serves it. See registerHolderRoutesToApp for the auth
// chain and tenant options it attaches.
func RegisterHolderV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *HolderHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerHolderRoutesToApp(group, api, auth, h, routeOptions, v2OpSuffix)
}

// registerHolderRoutesToApp is the single description of the holder route surface,
// shared by every versioned contract that serves it, mirroring
// registerAccountRoutesToApp. For each of the five ops it attaches the Fiber auth chain
// — auth.Authorize("midaz","holders",verb) + the CRM-scoped tenant PostAuthMiddlewares
// (routeOptions) + ParseUUIDPathParameters("holder") — as MIDDLEWARE ONLY (no terminal
// handler, no body binder) on the VERSIONED GROUP with GROUP-RELATIVE paths, then
// registers the Huma terminals via RegisterHolderRoutes on the SAME group's Huma API.
//
// The ParseUUIDPathParameters label is the span-attribute name; the middleware validates
// every UUID path param regardless of label.
func registerHolderRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *HolderHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		holdersPath  = "/organizations/:organization_id/holders"
		holderIDPath = holdersPath + "/:id"
	)

	holderParse := pkgHTTP.ParseUUIDPathParameters("holder")

	routePost(group, holdersPath, protectedMidaz(auth, "holders", "post", routeOptions, holderParse))
	routeGet(group, holderIDPath, protectedMidaz(auth, "holders", "get", routeOptions, holderParse))
	routePatch(group, holderIDPath, protectedMidaz(auth, "holders", "patch", routeOptions, holderParse))
	routeDelete(group, holderIDPath, protectedMidaz(auth, "holders", "delete", routeOptions, holderParse))
	routeGet(group, holdersPath, protectedMidaz(auth, "holders", "get", routeOptions, holderParse))

	RegisterHolderRoutes(api, h, opSuffix)
}
