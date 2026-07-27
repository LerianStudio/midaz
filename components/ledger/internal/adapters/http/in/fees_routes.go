// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
)

// feesApplicationName is the auth resource namespace for fee/billing routes. It
// is preserved verbatim from the standalone plugin-fees service: tenant-manager
// RBAC policies key on this string, so it MUST NOT be renamed (R9).
const feesApplicationName = "plugin-fees"

// RegisterFeesRoutesToApp wires the Huma-migrated fee/billing surface, mirroring
// RegisterAssetRoutesToApp / RegisterCRMRoutesToApp. For each op it attaches the Fiber
// auth chain — auth.Authorize("plugin-fees",resource,verb) + the fees-scoped tenant
// PostAuthMiddlewares (routeOptions) + ParseUUIDPathParameters — as MIDDLEWARE ONLY (no
// terminal handler, and NO body binder: the Fiber WithBodyTracing decorator the inline
// routes used is replaced by the Huma terminal's imperative DecodeAndValidate) on the
// /v1 GROUP with GROUP-RELATIVE paths, then registers the Huma terminals on the SAME
// group's Huma API. The plugin-fees authz namespace and the (resource, verb) tuples are
// preserved BYTE-FOR-BYTE.
//
// The fee calculate endpoint (POST /v1/fees) is intentionally NOT mounted: in the
// unified binary fees run in-process via the transaction seam, so only the dry-run
// estimate (POST /v1/.../estimates) is exposed over HTTP.
func RegisterFeesRoutesToApp(
	group fiber.Router,
	api huma.API,
	auth *middleware.AuthClient,
	ph *PackageHandler,
	fh *FeeHandler,
	bph *BillingPackageHandler,
	bch *BillingCalculateHandler,
	routeOptions *http.ProtectedRouteOptions,
) {
	const (
		packagesPath   = "/organizations/:organization_id/packages"
		packageIDPath  = packagesPath + "/:id"
		estimatesPath  = "/organizations/:organization_id/estimates"
		billingPkgPath = "/organizations/:organization_id/billing-packages"
		billingPkgID   = billingPkgPath + "/:id"
		billingCalc    = "/organizations/:organization_id/billing/calculate"
	)

	pkgParse := http.ParseUUIDPathParameters("packages")

	// Packages
	routePost(group, packagesPath, protectedFees(auth, "packages", "post", routeOptions, pkgParse))
	routeGet(group, packagesPath, protectedFees(auth, "packages", "get", routeOptions, pkgParse))
	routeGet(group, packageIDPath, protectedFees(auth, "packages", "get", routeOptions, pkgParse))
	routePatch(group, packageIDPath, protectedFees(auth, "packages", "patch", routeOptions, pkgParse))
	routeDelete(group, packageIDPath, protectedFees(auth, "packages", "delete", routeOptions, pkgParse))

	RegisterPackageRoutes(api, ph)

	// Fee estimate (dry-run). POST /v1/fees is NOT mounted — fees run in-process via the seam.
	routePost(group, estimatesPath, protectedFees(auth, "estimates", "post", routeOptions, http.ParseUUIDPathParameters("estimates")))

	RegisterFeeEstimateRoutes(api, fh)

	// Billing packages
	billingParse := http.ParseUUIDPathParameters("billing-packages")
	routePost(group, billingPkgPath, protectedFees(auth, "billing-packages", "post", routeOptions, billingParse))
	routeGet(group, billingPkgPath, protectedFees(auth, "billing-packages", "get", routeOptions, billingParse))
	routeGet(group, billingPkgID, protectedFees(auth, "billing-packages", "get", routeOptions, billingParse))
	routePatch(group, billingPkgID, protectedFees(auth, "billing-packages", "patch", routeOptions, billingParse))
	routeDelete(group, billingPkgID, protectedFees(auth, "billing-packages", "delete", routeOptions, billingParse))

	RegisterBillingPackageRoutes(api, bph)

	// Billing calculate
	routePost(group, billingCalc, protectedFees(auth, "billing-calculate", "post", routeOptions, http.ParseUUIDPathParameters("billing-calculate")))

	RegisterBillingCalculateRoutes(api, bch)
}

// protectedFees is the plugin-fees analogue of protectedMidaz/protectedRouting: it
// builds the auth-attaching Fiber chain under the "plugin-fees" authz appName.
func protectedFees(auth *middleware.AuthClient, resource, action string, routeOptions *http.ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	return http.ProtectedRouteChain(auth.Authorize(feesApplicationName, resource, action), routeOptions, handlers...)
}
