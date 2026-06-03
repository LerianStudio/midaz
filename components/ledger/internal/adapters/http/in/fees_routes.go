// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	feehttp "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/gofiber/fiber/v2"
)

// feesApplicationName is the auth resource namespace for fee/billing routes. It
// is preserved verbatim from the standalone plugin-fees service: tenant-manager
// RBAC policies key on this string, so it MUST NOT be renamed (R9).
const feesApplicationName = "plugin-fees"

// RegisterFeesRoutesToApp mounts the fee/billing CRUD surface on an existing
// Fiber router. It is the fee analogue of RegisterCRMRoutesToApp: routes are
// protected by the ledger ProtectedRouteChain (auth -> route-scoped post-auth
// middleware via routeOptions -> handlers) and carry the plugin-fees auth
// namespace verbatim.
//
// The fee calculate endpoint (POST /v1/fees) is intentionally NOT mounted: in
// the unified binary fees run in-process via the transaction seam, so only the
// dry-run estimate (POST /v1/estimates) is exposed over HTTP.
func RegisterFeesRoutesToApp(
	f fiber.Router,
	auth *middleware.AuthClient,
	ph *PackageHandler,
	fh *FeeHandler,
	bph *BillingPackageHandler,
	bch *BillingCalculateHandler,
	routeOptions *http.ProtectedRouteOptions,
) {
	// Packages
	f.Post("/v1/packages", http.ProtectedRouteChain(auth.Authorize(feesApplicationName, "packages", "post"), routeOptions, parseFeeHeaderParameters, feehttp.WithBodyTracing(new(model.CreatePackageInput), ph.CreatePackage))...)
	f.Get("/v1/packages", http.ProtectedRouteChain(auth.Authorize(feesApplicationName, "packages", "get"), routeOptions, parseFeeHeaderParameters, ph.GetAllPackages)...)
	f.Get("/v1/packages/:id", http.ProtectedRouteChain(auth.Authorize(feesApplicationName, "packages", "get"), routeOptions, parseFeeHeaderParameters, parseFeePathParameters, ph.GetPackageByID)...)
	f.Patch("/v1/packages/:id", http.ProtectedRouteChain(auth.Authorize(feesApplicationName, "packages", "patch"), routeOptions, parseFeeHeaderParameters, parseFeePathParameters, feehttp.WithBodyTracing(new(model.UpdatePackageInput), ph.UpdatePackageByID))...)
	f.Delete("/v1/packages/:id", http.ProtectedRouteChain(auth.Authorize(feesApplicationName, "packages", "delete"), routeOptions, parseFeeHeaderParameters, parseFeePathParameters, ph.DeletePackageByID)...)

	// Fee estimate (dry-run). POST /v1/fees is NOT mounted — fees run in-process via the seam.
	f.Post("/v1/estimates", http.ProtectedRouteChain(auth.Authorize(feesApplicationName, "estimates", "post"), routeOptions, parseFeeHeaderParameters, feehttp.WithBodyTracing(new(model.FeeEstimate), fh.EstimateFeeCalculation))...)

	// Billing packages
	f.Post("/v1/billing-packages", http.ProtectedRouteChain(auth.Authorize(feesApplicationName, "billing-packages", "post"), routeOptions, parseFeeHeaderParameters, feehttp.WithBodyTracing(new(model.BillingPackage), bph.CreateBillingPackage))...)
	f.Get("/v1/billing-packages", http.ProtectedRouteChain(auth.Authorize(feesApplicationName, "billing-packages", "get"), routeOptions, parseFeeHeaderParameters, bph.GetAllBillingPackages)...)
	f.Get("/v1/billing-packages/:id", http.ProtectedRouteChain(auth.Authorize(feesApplicationName, "billing-packages", "get"), routeOptions, parseFeeHeaderParameters, parseFeePathParameters, bph.GetBillingPackageByID)...)
	f.Patch("/v1/billing-packages/:id", http.ProtectedRouteChain(auth.Authorize(feesApplicationName, "billing-packages", "patch"), routeOptions, parseFeeHeaderParameters, parseFeePathParameters, feehttp.WithBodyTracing(new(model.BillingPackageUpdate), bph.UpdateBillingPackage))...)
	f.Delete("/v1/billing-packages/:id", http.ProtectedRouteChain(auth.Authorize(feesApplicationName, "billing-packages", "delete"), routeOptions, parseFeeHeaderParameters, parseFeePathParameters, bph.DeleteBillingPackage)...)

	// Billing calculate
	f.Post("/v1/billing/calculate", http.ProtectedRouteChain(auth.Authorize(feesApplicationName, "billing-calculate", "post"), routeOptions, parseFeeHeaderParameters, feehttp.WithBodyTracing(new(model.BillingCalculateRequest), bch.CalculateBilling))...)
}

// CreateFeesRouteRegistrar returns a registrar that mounts the fee/billing routes
// on the unified ledger server. The routeOptions carries the fees-scoped tenant
// middleware (built in the ledger composition root) so it applies ONLY to fee
// routes.
func CreateFeesRouteRegistrar(
	auth *middleware.AuthClient,
	ph *PackageHandler,
	fh *FeeHandler,
	bph *BillingPackageHandler,
	bch *BillingCalculateHandler,
	routeOptions *http.ProtectedRouteOptions,
) func(fiber.Router) {
	return func(router fiber.Router) {
		RegisterFeesRoutesToApp(router, auth, ph, fh, bph, bch, routeOptions)
	}
}
