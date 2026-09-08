// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// RegisterPackageRoutes registers the five fee-package operations on the given Huma
// API. It is the per-file seam the unified server calls; the auth
// ("midaz","packages",verb) + tenant + ParseUUIDPathParameters("packages") middleware
// chain is attached on the versioned Fiber group BEFORE the Huma terminal, not here.
// Paths are GROUP-RELATIVE (see asset_handler.go's RegisterAssetRoutes header for the
// rationale).
//
// opSuffix is appended to every operation ID — see v2OpSuffix.
func RegisterPackageRoutes(api huma.API, h *PackageHandler, opSuffix string) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/packages"
		idPath   = listPath + "/{id}"
		tag      = "Packages"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createPackage" + opSuffix,
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create a Package",
		Tags:        []string{tag},
		Security:    secPackageBearer,
		// Body validated imperatively (feehttp.DecodeValidateBody) — see file header.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreatePackageV2)
	attachTypedRequestBody[model.CreatePackageInput](api, "createPackage"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "getAllPackages" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all packages of a ledger",
		Tags:        []string{tag},
		Security:    secPackageBearer,
	}, h.GetAllPackagesV2)

	huma.Register(api, huma.Operation{
		OperationID: "getPackageByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get package",
		Tags:        []string{tag},
		Security:    secPackageBearer,
	}, h.GetPackageByIDV2)

	huma.Register(api, huma.Operation{
		OperationID:      "updatePackage" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a package",
		Tags:             []string{tag},
		Security:         secPackageBearer,
		SkipValidateBody: true, // body validated imperatively — see createPackage.
	}, h.UpdatePackageByIDV2)
	attachTypedRequestBody[model.UpdatePackageInput](api, "updatePackage"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "deletePackage" + opSuffix,
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "SoftDelete a Package by ID",
		Tags:        []string{tag},
		Security:    secPackageBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeletePackageByIDV2)
}

// RegisterPackageV2RoutesToApp wires the fee-package surface onto the /v2 contract,
// which is the ONLY version group that serves it. See registerPackageRoutesToApp for
// the auth chain and tenant options it attaches.
func RegisterPackageV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *PackageHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerPackageRoutesToApp(group, api, auth, h, routeOptions, v2OpSuffix)
}

// registerPackageRoutesToApp is the single description of the fee-package route
// surface, shared by every versioned contract that serves it, mirroring
// registerHolderRoutesToApp. For each of the five ops it attaches the Fiber auth chain
// — auth.Authorize("midaz","packages",verb) + the fees-scoped tenant PostAuthMiddlewares
// (routeOptions) + ParseUUIDPathParameters("packages") — as MIDDLEWARE ONLY (no terminal
// handler, and no body binder, because the Huma terminal decodes and validates the body
// imperatively) on the VERSIONED GROUP with GROUP-RELATIVE paths, then registers the
// Huma terminals via RegisterPackageRoutes on the SAME group's Huma API.
//
// The ParseUUIDPathParameters label is the span-attribute name; the middleware validates
// every UUID path param regardless of label. The Fiber and OpenAPI spellings of the path
// must name their parameters identically — TestFeesV2RoutesParameterNamesAgree pins it,
// because ParseUUIDPathParameters keys on the name the FIBER route declares and passes a
// name outside constant.UUIDPathParameters through as an unvalidated string.
func registerPackageRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *PackageHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		packagesPath  = "/organizations/:organization_id/ledgers/:ledger_id/packages"
		packageIDPath = packagesPath + "/:id"
	)

	packageParse := pkgHTTP.ParseUUIDPathParameters("packages")

	routePost(group, packagesPath, protectedMidaz(auth, "packages", "post", routeOptions, packageParse))
	routeGet(group, packagesPath, protectedMidaz(auth, "packages", "get", routeOptions, packageParse))
	routeGet(group, packageIDPath, protectedMidaz(auth, "packages", "get", routeOptions, packageParse))
	routePatch(group, packageIDPath, protectedMidaz(auth, "packages", "patch", routeOptions, packageParse))
	routeDelete(group, packageIDPath, protectedMidaz(auth, "packages", "delete", routeOptions, packageParse))

	RegisterPackageRoutes(api, h, opSuffix)
}
