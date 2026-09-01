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

// RegisterAssetRoutes registers the six asset operations on the shared
// Huma API. It is the per-file seam registerAssetRoutesToApp calls; the auth + tenant +
// ParseUUIDPathParameters middleware chain for these routes is attached in
// registerAssetRoutesToApp (Fiber-level) BEFORE the Huma terminal, not here.
//
// Paths are GROUP-RELATIVE: the Huma API is bound to a versioned Fiber group, so the
// humafiber adapter registers on that group and Fiber prepends the version prefix.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see v1OpSuffix. A straight v1/v2 mirror reuses the same handler methods and the
// same input/output types, so only the operation IDs differ between the twins.
func RegisterAssetRoutes(api huma.API, h *AssetHandler, opSuffix string) {
	const (
		listPath  = "/organizations/{organization_id}/ledgers/{ledger_id}/assets"
		idPath    = listPath + "/{id}"
		countPath = listPath + "/metrics/count"
		tag       = "Assets"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createAsset" + opSuffix,
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create a new asset",
		Tags:        []string{tag},
		Security:    secAssetBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateAsset)
	attachTypedRequestBody[mmodel.CreateAssetInput](api, "createAsset"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listAssets" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all assets",
		Tags:        []string{tag},
		Security:    secAssetBearer,
	}, h.ListAssets)

	huma.Register(api, huma.Operation{
		OperationID: "getAssetByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific asset",
		Tags:        []string{tag},
		Security:    secAssetBearer,
	}, h.GetAssetByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updateAsset" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an asset",
		Tags:             []string{tag},
		Security:         secAssetBearer,
		SkipValidateBody: true, // body validated imperatively — see createAsset.
	}, h.UpdateAsset)
	attachTypedRequestBody[mmodel.UpdateAssetInput](api, "updateAsset"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "deleteAsset" + opSuffix,
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete an asset",
		Tags:        []string{tag},
		Security:    secAssetBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteAssetByID)

	huma.Register(api, huma.Operation{
		OperationID: "countAssets" + opSuffix,
		Method:      http.MethodHead,
		Path:        countPath,
		Summary:     "Count total assets",
		Tags:        []string{tag},
		Security:    secAssetBearer,
		// HEAD count: X-Total-Count header + empty 204 body (Content-Length 0 set
		// on the Out struct), matching the Fiber http.NoContent + header path.
		DefaultStatus: http.StatusNoContent,
	}, h.CountAssets)
}

// RegisterAssetRoutesToApp wires the asset surface onto the /v1
// contract. See registerAssetRoutesToApp for what it attaches.
func RegisterAssetRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ih *AssetHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerAssetRoutesToApp(group, api, auth, ih, routeOptions, v1OpSuffix)
}

// RegisterAssetV2RoutesToApp wires the same asset surface onto the /v2 contract: same paths,
// same handlers, same authz tuples and tenant chain, differing only in the operation IDs the
// contract publishes. It is additive — /v1 keeps serving assets in parallel — and introduces
// no new policy surface.
func RegisterAssetV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ih *AssetHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerAssetRoutesToApp(group, api, auth, ih, routeOptions, v2OpSuffix)
}

// registerAssetRoutesToApp is the single description of the asset route surface, shared by
// every versioned contract that serves it. For each of the six ops it attaches the Fiber auth
// chain — auth.Authorize("midaz","assets",verb) + tenant PostAuthMiddlewares +
// ParseUUIDPathParameters("asset") — as MIDDLEWARE ONLY (no terminal) on the VERSIONED GROUP
// with GROUP-RELATIVE paths, then registers the Huma terminals via RegisterAssetRoutes on the
// SAME group's Huma API. The ("midaz","assets",verb) authz tuples and tenant resolution
// therefore apply on whichever version group it is mounted on; no asset route becomes
// public.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's — see
// v1OpSuffix. Nothing else varies between contracts, so a change to the surface reaches
// every version it is mounted on.
func registerAssetRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ih *AssetHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath  = "/organizations/:organization_id/ledgers/:ledger_id/assets"
		idPath    = listPath + "/:id"
		countPath = listPath + "/metrics/count"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("asset")

	routePost(group, listPath, protectedMidaz(auth, "assets", "post", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "assets", "patch", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "assets", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "assets", "get", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "assets", "delete", routeOptions, parse))
	routeHead(group, countPath, protectedMidaz(auth, "assets", "head", routeOptions, parse))

	RegisterAssetRoutes(api, ih, opSuffix)
}
