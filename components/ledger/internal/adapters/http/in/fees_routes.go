// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"regexp"

	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
)

// feesApplicationName is the auth resource namespace for fee/billing routes. It
// is preserved verbatim from the standalone plugin-fees service: tenant-manager
// RBAC policies key on this string, so it MUST NOT be renamed (R9).
const feesApplicationName = "plugin-fees"

// feeBasePathV1 is the scope every v1 fee and billing resource hangs off, in OpenAPI
// template syntax. Each resource registrar appends its own segments to it, and
// feeChainPath restates it in Fiber syntax for the guard chain.
const feeBasePathV1 = "/organizations/{organization_id}"

// feeOpSuffixV1 is the operation-ID suffix the v1 fee contract carries. The ledger
// publishes each versioned contract as a separate OpenAPI document and the published
// hub spec joins them; the join makes path keys unique by the version prefix but leaves
// operation IDs alone, so a contract that repeats another's IDs collides there. v1's
// suffix is empty because its IDs are what published SDKs already bind to.
const feeOpSuffixV1 = ""

// feeSpecPathParam matches a single OpenAPI path-parameter segment, "{name}".
var feeSpecPathParam = regexp.MustCompile(`\{([^{}/]+)\}`)

// feeChainPath restates an OpenAPI path template in Fiber's route syntax, carrying the
// parameter names through unchanged, so the guard chain and the Huma terminal are cut
// from one path value instead of two hand-written ones.
//
// The names have to agree, and nothing else in the build reports it when they don't.
// ParseUUIDPathParameters reads the names the FIBER route declares and only parses the
// ones pkg/constant.UUIDPathParameters lists, so a name spelled differently on the chain
// than in the contract is passed through as an unvalidated string. Fiber matches on
// segment structure rather than on names, so the request still routes and the chain
// still runs; the spec-vs-routes gate compares parameter positions, so it stays green
// too.
func feeChainPath(specPath string) string {
	return feeSpecPathParam.ReplaceAllString(specPath, ":$1")
}

// RegisterFeesRoutesToApp wires the Huma-migrated fee/billing surface onto the /v1
// contract. See registerFeesRoutesToApp for what it attaches.
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
	registerFeesRoutesToApp(group, api, auth, ph, fh, bph, bch, routeOptions, feeBasePathV1, feeOpSuffixV1)
}

// registerFeesRoutesToApp is the single description of the fee/billing route surface,
// shared by every versioned contract that serves it. For each op it attaches the Fiber
// auth chain — auth.Authorize("plugin-fees",resource,verb) + the fees-scoped tenant
// PostAuthMiddlewares (routeOptions) + ParseUUIDPathParameters — as MIDDLEWARE ONLY (no
// terminal handler, and NO body binder: the Fiber WithBodyTracing decorator the inline
// routes used is replaced by the Huma terminal's imperative DecodeAndValidate) on the
// VERSIONED GROUP with GROUP-RELATIVE paths, then registers the Huma terminals on the
// SAME group's Huma API. The plugin-fees authz namespace and the (resource, verb) tuples
// are preserved BYTE-FOR-BYTE.
//
// basePath is the scope the resources hang off, in OpenAPI template syntax — see
// feeBasePathV1. opSuffix distinguishes the operation IDs one contract publishes from
// another's — see feeOpSuffixV1. Nothing else varies between contracts, so a change to
// the surface reaches every version it is mounted on.
//
// The fee calculate endpoint (POST /v1/fees) is intentionally NOT mounted: in the
// unified binary fees run in-process via the transaction seam, so only the dry-run
// estimate (POST /v1/.../estimates) is exposed over HTTP.
func registerFeesRoutesToApp(
	group fiber.Router,
	api huma.API,
	auth *middleware.AuthClient,
	ph *PackageHandler,
	fh *FeeHandler,
	bph *BillingPackageHandler,
	bch *BillingCalculateHandler,
	routeOptions *http.ProtectedRouteOptions,
	basePath string,
	opSuffix string,
) {
	chainBase := feeChainPath(basePath)

	packagesPath := chainBase + "/packages"
	packageIDPath := packagesPath + "/:id"
	estimatesPath := chainBase + "/estimates"
	billingPkgPath := chainBase + "/billing-packages"
	billingPkgID := billingPkgPath + "/:id"
	billingCalc := chainBase + "/billing/calculate"

	pkgParse := http.ParseUUIDPathParameters("packages")

	// Packages
	routePost(group, packagesPath, protectedFees(auth, "packages", "post", routeOptions, pkgParse))
	routeGet(group, packagesPath, protectedFees(auth, "packages", "get", routeOptions, pkgParse))
	routeGet(group, packageIDPath, protectedFees(auth, "packages", "get", routeOptions, pkgParse))
	routePatch(group, packageIDPath, protectedFees(auth, "packages", "patch", routeOptions, pkgParse))
	routeDelete(group, packageIDPath, protectedFees(auth, "packages", "delete", routeOptions, pkgParse))

	RegisterPackageRoutes(api, ph, basePath, opSuffix)

	// Fee estimate (dry-run). POST /v1/fees is NOT mounted — fees run in-process via the seam.
	routePost(group, estimatesPath, protectedFees(auth, "estimates", "post", routeOptions, http.ParseUUIDPathParameters("estimates")))

	RegisterFeeEstimateRoutes(api, fh, basePath, opSuffix)

	// Billing packages
	billingParse := http.ParseUUIDPathParameters("billing-packages")
	routePost(group, billingPkgPath, protectedFees(auth, "billing-packages", "post", routeOptions, billingParse))
	routeGet(group, billingPkgPath, protectedFees(auth, "billing-packages", "get", routeOptions, billingParse))
	routeGet(group, billingPkgID, protectedFees(auth, "billing-packages", "get", routeOptions, billingParse))
	routePatch(group, billingPkgID, protectedFees(auth, "billing-packages", "patch", routeOptions, billingParse))
	routeDelete(group, billingPkgID, protectedFees(auth, "billing-packages", "delete", routeOptions, billingParse))

	RegisterBillingPackageRoutes(api, bph, basePath, opSuffix)

	// Billing calculate
	routePost(group, billingCalc, protectedFees(auth, "billing-calculate", "post", routeOptions, http.ParseUUIDPathParameters("billing-calculate")))

	RegisterBillingCalculateRoutes(api, bch, basePath, opSuffix)
}

// protectedFees is the plugin-fees analogue of protectedMidaz/protectedRouting: it
// builds the auth-attaching Fiber chain under the "plugin-fees" authz appName.
func protectedFees(auth *middleware.AuthClient, resource, action string, routeOptions *http.ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	return http.ProtectedRouteChain(auth.Authorize(feesApplicationName, resource, action), routeOptions, handlers...)
}
