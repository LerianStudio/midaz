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

// RegisterBillingPackageRoutes registers the five billing-package operations on the
// given Huma API. It is the per-file seam the unified server calls; the auth
// ("midaz","billing-packages",verb) + tenant +
// ParseUUIDPathParameters("billing-packages") middleware chain is attached on the
// versioned Fiber group BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE.
//
// opSuffix is appended to every operation ID — see v2OpSuffix.
func RegisterBillingPackageRoutes(api huma.API, h *BillingPackageHandler, opSuffix string) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/billing-packages"
		idPath   = listPath + "/{id}"
		tag      = "Billing Packages"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createBillingPackage" + opSuffix,
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create a BillingPackage",
		Tags:        []string{tag},
		Security:    secBillingBearer,
		// Body validated imperatively (feehttp.DecodeValidateBody) — see file header.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateBillingPackageV2)
	attachTypedRequestBody[model.CreateBillingPackageInput](api, "createBillingPackage"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "getAllBillingPackages" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all billing packages of a ledger",
		Tags:        []string{tag},
		Security:    secBillingBearer,
	}, h.GetAllBillingPackagesV2)

	huma.Register(api, huma.Operation{
		OperationID: "getBillingPackageByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get billing package",
		Tags:        []string{tag},
		Security:    secBillingBearer,
	}, h.GetBillingPackageByIDV2)

	huma.Register(api, huma.Operation{
		OperationID:      "updateBillingPackage" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a billing package",
		Tags:             []string{tag},
		Security:         secBillingBearer,
		SkipValidateBody: true, // body validated imperatively — see createBillingPackage.
	}, h.UpdateBillingPackageV2)
	attachTypedRequestBody[model.BillingPackageUpdate](api, "updateBillingPackage"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "deleteBillingPackage" + opSuffix,
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "SoftDelete a BillingPackage by ID",
		Tags:        []string{tag},
		Security:    secBillingBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteBillingPackageV2)
}

// RegisterBillingPackageV2RoutesToApp wires the billing-package surface onto the /v2
// contract, which is the ONLY version group that serves it. See
// registerBillingPackageRoutesToApp for the auth chain and tenant options it attaches.
func RegisterBillingPackageV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *BillingPackageHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerBillingPackageRoutesToApp(group, api, auth, h, routeOptions, v2OpSuffix)
}

// registerBillingPackageRoutesToApp is the single description of the billing-package
// route surface, shared by every versioned contract that serves it, mirroring
// registerPackageRoutesToApp. For each of the five ops it attaches the Fiber auth chain
// — auth.Authorize("midaz","billing-packages",verb) + the fees-scoped tenant
// PostAuthMiddlewares (routeOptions) + ParseUUIDPathParameters("billing-packages") — as
// MIDDLEWARE ONLY (no terminal handler, and no body binder, because the Huma terminal
// decodes and validates the body imperatively) on the VERSIONED GROUP with
// GROUP-RELATIVE paths, then registers the Huma terminals via
// RegisterBillingPackageRoutes on the SAME group's Huma API.
//
// The ParseUUIDPathParameters label is the span-attribute name; the middleware validates
// every UUID path param regardless of label. See registerPackageRoutesToApp on why the
// Fiber and OpenAPI parameter names must agree.
func registerBillingPackageRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *BillingPackageHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		billingPackagesPath  = "/organizations/:organization_id/ledgers/:ledger_id/billing-packages"
		billingPackageIDPath = billingPackagesPath + "/:id"
	)

	billingPackageParse := pkgHTTP.ParseUUIDPathParameters("billing-packages")

	routePost(group, billingPackagesPath, protectedMidaz(auth, "billing-packages", "post", routeOptions, billingPackageParse))
	routeGet(group, billingPackagesPath, protectedMidaz(auth, "billing-packages", "get", routeOptions, billingPackageParse))
	routeGet(group, billingPackageIDPath, protectedMidaz(auth, "billing-packages", "get", routeOptions, billingPackageParse))
	routePatch(group, billingPackageIDPath, protectedMidaz(auth, "billing-packages", "patch", routeOptions, billingPackageParse))
	routeDelete(group, billingPackageIDPath, protectedMidaz(auth, "billing-packages", "delete", routeOptions, billingPackageParse))

	RegisterBillingPackageRoutes(api, h, opSuffix)
}
