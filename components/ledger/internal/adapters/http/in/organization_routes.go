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

// RegisterOrganizationRoutes registers the six organization operations on
// the shared Huma API. It is the per-file seam RegisterOrganizationRoutesToApp calls;
// the auth + tenant + ParseUUIDPathParameters middleware chain for these routes is
// attached in RegisterOrganizationRoutesToApp (Fiber-level) BEFORE the Huma terminal,
// not here.
//
// Paths are GROUP-RELATIVE: the Huma API is bound to a versioned Fiber group, so the
// humafiber adapter registers on that group and Fiber prepends the version prefix.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. A straight v1/v2 mirror reuses the same handler methods and the
// same input/output types, so only the operation IDs differ between the twins.
func RegisterOrganizationRoutes(api huma.API, h *OrganizationHandler, opSuffix string) {
	const (
		listPath  = "/organizations"
		idPath    = listPath + "/{id}"
		countPath = listPath + "/metrics/count"
		tag       = "Organizations"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createOrganization" + opSuffix,
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create a new organization",
		Tags:        []string{tag},
		Security:    secOrgBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateOrganization)
	attachTypedRequestBody[mmodel.CreateOrganizationInput](api, "createOrganization"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listOrganizations" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all organizations",
		Tags:        []string{tag},
		Security:    secOrgBearer,
	}, h.ListOrganizations)

	huma.Register(api, huma.Operation{
		OperationID: "getOrganizationByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific organization",
		Tags:        []string{tag},
		Security:    secOrgBearer,
	}, h.GetOrganizationByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updateOrganization" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an existing organization",
		Tags:             []string{tag},
		Security:         secOrgBearer,
		SkipValidateBody: true, // body validated imperatively — see createOrganization.
	}, h.UpdateOrganization)
	attachTypedRequestBody[mmodel.UpdateOrganizationInput](api, "updateOrganization"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "deleteOrganization" + opSuffix,
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete an organization",
		Tags:        []string{tag},
		Security:    secOrgBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteOrganizationByID)

	huma.Register(api, huma.Operation{
		OperationID: "countOrganizations" + opSuffix,
		Method:      http.MethodHead,
		Path:        countPath,
		Summary:     "Count total organizations",
		Tags:        []string{tag},
		Security:    secOrgBearer,
		// HEAD count: X-Total-Count header + empty 204 body (Content-Length 0 set on
		// the Out struct), matching the Fiber http.NoContent + header path.
		DefaultStatus: http.StatusNoContent,
	}, h.CountOrganizations)
}

// RegisterOrganizationRoutesToApp wires the organization surface onto the
// /v1 contract. See registerOrganizationRoutesToApp for what it attaches.
func RegisterOrganizationRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *OrganizationHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerOrganizationRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV1)
}

// RegisterOrganizationV2RoutesToApp wires the same organization surface onto the /v2
// contract: same paths, same handlers, same authz tuples and tenant chain, differing only
// in the operation IDs the contract publishes. It is additive — /v1 keeps serving
// organizations in parallel — and introduces no new policy surface.
func RegisterOrganizationV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *OrganizationHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerOrganizationRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV2)
}

// registerOrganizationRoutesToApp is the single description of the organization route
// surface, shared by every versioned contract that serves it, mirroring
// RegisterAssetRoutesToApp. For each of the six ops it attaches the Fiber auth chain —
// protectedMidaz(auth,"organizations",verb) (= auth.Authorize("midaz","organizations",
// verb) + tenant PostAuthMiddlewares) — as MIDDLEWARE ONLY (no terminal) on the VERSIONED
// GROUP with GROUP-RELATIVE paths, then registers the Huma terminals via
// RegisterOrganizationRoutes on the SAME group's Huma API. The ("organizations", verb)
// authz tuples and tenant resolution therefore apply on whichever version group it is
// mounted on — no organization route becomes public.
//
// ParseUUIDPathParameters("organization") is attached ONLY on the three ":id" ops
// (patch/get-by-id/delete); create, list and count carry no path UUID, so none needs it.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface
// reaches every version it is mounted on.
func registerOrganizationRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *OrganizationHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath  = "/organizations"
		idPath    = listPath + "/:id"
		countPath = listPath + "/metrics/count"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("organization")

	routePost(group, listPath, protectedMidaz(auth, "organizations", "post", routeOptions))
	routePatch(group, idPath, protectedMidaz(auth, "organizations", "patch", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "organizations", "get", routeOptions))
	routeGet(group, idPath, protectedMidaz(auth, "organizations", "get", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "organizations", "delete", routeOptions, parse))
	routeHead(group, countPath, protectedMidaz(auth, "organizations", "head", routeOptions))

	RegisterOrganizationRoutes(api, h, opSuffix)
}
