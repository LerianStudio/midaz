// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/assetrate"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// RegisterAssetRateRoutes registers the three asset-rate operations on the
// shared Huma API. It is the per-file seam registerAssetRateRoutesToApp calls; the auth +
// tenant + ParseUUIDPathParameters middleware chain for these routes is attached at
// the Fiber level BEFORE the Huma terminal, not here.
//
// Paths are GROUP-RELATIVE: the Huma API is bound to a versioned Fiber group, so the
// humafiber adapter registers on that group and Fiber prepends the version prefix.
//
// opSuffix is appended to each operation ID (see v1OpSuffix), keeping the parameter
// shape identical to the sibling dual-version registrars. Asset-rate is served on /v1 only
// and has no v2 twin, so a single suffix is in play; the PUT upsert carries it exactly like
// the two GETs.
func RegisterAssetRateRoutes(api huma.API, h *AssetRateHandler, opSuffix string) {
	const (
		basePath     = "/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates"
		externalPath = basePath + "/{external_id}"
		fromPath     = basePath + "/from/{asset_code}"
		tag          = "Asset Rates"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createOrUpdateAssetRate" + opSuffix,
		Method:      http.MethodPut,
		Path:        basePath,
		Summary:     "Create or Update an AssetRate",
		Tags:        []string{tag},
		Security:    secAssetRateBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateOrUpdateAssetRate)
	attachTypedRequestBody[assetrate.CreateAssetRateInput](api, "createOrUpdateAssetRate"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "getAssetRateByExternalID" + opSuffix,
		Method:      http.MethodGet,
		Path:        externalPath,
		Summary:     "Get an AssetRate by External ID",
		Tags:        []string{tag},
		Security:    secAssetRateBearer,
	}, h.GetAssetRateByExternalID)

	huma.Register(api, huma.Operation{
		OperationID: "getAllAssetRatesByAssetCode" + opSuffix,
		Method:      http.MethodGet,
		Path:        fromPath,
		Summary:     "Get an AssetRate by the Asset Code",
		Tags:        []string{tag},
		Security:    secAssetRateBearer,
	}, h.ListAssetRatesByAssetCode)
}

// RegisterAssetRateRoutesToApp wires the asset-rate surface onto the /v1
// contract. See registerAssetRateRoutesToApp for what it attaches.
func RegisterAssetRateRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AssetRateHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerAssetRateRoutesToApp(group, api, auth, h, routeOptions, v1OpSuffix)
}

// registerAssetRateRoutesToApp is the single description of the asset-rate route surface,
// mounted on the /v1 contract. For each of the three ops it attaches
// the Fiber auth chain — protectedMidaz(auth,"asset-rates",verb) (=
// auth.Authorize("midaz","asset-rates",verb) + tenant PostAuthMiddlewares) +
// ParseUUIDPathParameters("asset-rate") — as MIDDLEWARE ONLY (no terminal) on the VERSIONED
// GROUP with GROUP-RELATIVE paths, then registers the Huma terminals via
// RegisterAssetRateRoutes on the SAME group's Huma API. The authz tuples are
// ("asset-rates", verb), with the "asset-rate" entity-name for
// ParseUUIDPathParameters — no asset-rate route is public on the /v1 group. asset-rate is MONEY-adjacent (exchange rates).
//
// opSuffix is appended to each operation ID (see v1OpSuffix). It keeps this registrar's
// signature identical to the sibling dual-version registrars; asset-rate mounts on /v1 only,
// so a single suffix is in play.
func registerAssetRateRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AssetRateHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		basePath     = "/organizations/:organization_id/ledgers/:ledger_id/asset-rates"
		externalPath = basePath + "/:external_id"
		fromPath     = basePath + "/from/:asset_code"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("asset-rate")

	routePut(group, basePath, protectedMidaz(auth, "asset-rates", "put", routeOptions, parse))
	routeGet(group, externalPath, protectedMidaz(auth, "asset-rates", "get", routeOptions, parse))
	routeGet(group, fromPath, protectedMidaz(auth, "asset-rates", "get", routeOptions, parse))

	RegisterAssetRateRoutes(api, h, opSuffix)
}
