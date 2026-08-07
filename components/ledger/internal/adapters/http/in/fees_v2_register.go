// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the v2 fee/billing contract seam (filename-suffix versioning — the v1
// files are left untouched). It mounts the twelve fee and billing operations at
// ledger scope on the /v2 version group of the shared Huma contract, and attaches the SAME Fiber
// guard chain the organization-scoped routes carry: auth.Authorize("plugin-fees",
// resource, verb) with the same (resource, verb) tuples, the same fees-scoped tenant
// PostAuthMiddlewares, and the same ParseUUIDPathParameters labels. No new policy
// surface: the fees namespace stays "plugin-fees".
//
// The registrations are written out here rather than routed through the four
// organization-scoped registrars, even though those take a base path. A ledger-scoped
// path carries a parameter the organization-scoped input structs do not declare, and
// Huma does not object: it registers the operation, publishes only the parameters the
// struct declares, and the terminal never sees the ledger. Reusing them would
// therefore produce a contract whose path template names a parameter the operation
// does not document, served by a handler acting at the wrong scope.
//
// POST /fees is absent here for the reason it is absent from /v1: in the unified
// binary fees run in-process via the transaction seam, so only the dry-run estimate is
// exposed over HTTP.

// feeBasePathV2 is the scope every v2 fee and billing resource hangs off, in OpenAPI
// template syntax. Each registration appends its own segments to it, and feeChainPath
// restates it in Fiber syntax for the guard chain.
const feeBasePathV2 = "/organizations/{organization_id}/ledgers/{ledger_id}"

// feeOpSuffixV2 is the operation-ID suffix the v2 fee contract carries. The ledger
// serves both fee versions on a single OpenAPI document, and huma.OpenAPI.AddOperation
// scans the whole document and panics on a duplicate operation ID, so a v2 op MUST NOT
// repeat the ID of its v1 twin or the ledger panics at boot. The V2 suffix makes that
// disjunction a boot invariant; it secondarily keeps IDs unique across the ledger↔tracer
// hub-spec join. The organization-scoped contract publishes its IDs unsuffixed: they are
// what already published SDKs bind to, so they are frozen rather than versioned.
const feeOpSuffixV2 = "V2"

// RegisterFeesV2Routes registers the twelve ledger-scoped fee and billing operations
// on the /v2 version group of the shared Huma API. Auth is the Fiber guard chain attached in
// RegisterFeesV2RoutesToApp BEFORE these terminals — the per-op Security metadata is
// SPEC-ONLY. Paths are GROUP-RELATIVE (the group's PrefixModifier writes the /v2 prefix
// into each op's op.Path, not into a servers entry).
func RegisterFeesV2Routes(api huma.API, ph *PackageHandler, fh *FeeHandler, bph *BillingPackageHandler, bch *BillingCalculateHandler) {
	registerPackageV2Routes(api, ph)
	registerFeeEstimateV2Routes(api, fh)
	registerBillingPackageV2Routes(api, bph)
	registerBillingCalculateV2Routes(api, bch)
}

// registerPackageV2Routes registers the five ledger-scoped fee-package operations.
func registerPackageV2Routes(api huma.API, h *PackageHandler) {
	const tag = "Packages"

	listPath := feeBasePathV2 + "/packages"
	idPath := listPath + "/{id}"

	huma.Register(api, huma.Operation{
		OperationID: "createPackage" + feeOpSuffixV2,
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create a Package",
		Tags:        []string{tag},
		Security:    secPackageBearer,
		// Body validated imperatively (feehttp.DecodeValidateBody) — see fees_v2_handler.go.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreatePackageV2Huma)

	huma.Register(api, huma.Operation{
		OperationID: "getAllPackages" + feeOpSuffixV2,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all packages of a ledger",
		Tags:        []string{tag},
		Security:    secPackageBearer,
	}, h.GetAllPackagesV2Huma)

	huma.Register(api, huma.Operation{
		OperationID: "getPackageByID" + feeOpSuffixV2,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get package",
		Tags:        []string{tag},
		Security:    secPackageBearer,
	}, h.GetPackageByIDV2Huma)

	huma.Register(api, huma.Operation{
		OperationID:      "updatePackage" + feeOpSuffixV2,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a package",
		Tags:             []string{tag},
		Security:         secPackageBearer,
		SkipValidateBody: true, // body validated imperatively — see createPackage.
	}, h.UpdatePackageByIDV2Huma)

	huma.Register(api, huma.Operation{
		OperationID: "deletePackage" + feeOpSuffixV2,
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "SoftDelete a Package by ID",
		Tags:        []string{tag},
		Security:    secPackageBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeletePackageByIDV2Huma)
}

// registerFeeEstimateV2Routes registers the ledger-scoped fee-estimate operation.
func registerFeeEstimateV2Routes(api huma.API, h *FeeHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "estimateFeeCalculation" + feeOpSuffixV2,
		Method:      http.MethodPost,
		Path:        feeBasePathV2 + "/estimates",
		Summary:     "Create a fee estimate calculation",
		Tags:        []string{"Fees"},
		Security:    secFeeBearer,
		// Body validated imperatively (feehttp.DecodeValidateBody) — see fees_v2_handler.go.
		SkipValidateBody: true,
	}, h.EstimateFeeCalculationV2Huma)
}

// registerBillingPackageV2Routes registers the five ledger-scoped billing-package
// operations.
func registerBillingPackageV2Routes(api huma.API, h *BillingPackageHandler) {
	const tag = "Billing Packages"

	listPath := feeBasePathV2 + "/billing-packages"
	idPath := listPath + "/{id}"

	huma.Register(api, huma.Operation{
		OperationID: "createBillingPackage" + feeOpSuffixV2,
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create a BillingPackage",
		Tags:        []string{tag},
		Security:    secBillingBearer,
		// Body validated imperatively (feehttp.DecodeValidateBody) — see fees_v2_handler.go.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateBillingPackageV2Huma)

	huma.Register(api, huma.Operation{
		OperationID: "getAllBillingPackages" + feeOpSuffixV2,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all billing packages of a ledger",
		Tags:        []string{tag},
		Security:    secBillingBearer,
	}, h.GetAllBillingPackagesV2Huma)

	huma.Register(api, huma.Operation{
		OperationID: "getBillingPackageByID" + feeOpSuffixV2,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get billing package",
		Tags:        []string{tag},
		Security:    secBillingBearer,
	}, h.GetBillingPackageByIDV2Huma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateBillingPackage" + feeOpSuffixV2,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a billing package",
		Tags:             []string{tag},
		Security:         secBillingBearer,
		SkipValidateBody: true, // body validated imperatively — see createBillingPackage.
	}, h.UpdateBillingPackageV2Huma)

	huma.Register(api, huma.Operation{
		OperationID: "deleteBillingPackage" + feeOpSuffixV2,
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "SoftDelete a BillingPackage by ID",
		Tags:        []string{tag},
		Security:    secBillingBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteBillingPackageV2Huma)
}

// registerBillingCalculateV2Routes registers the ledger-scoped billing-calculate
// operation.
func registerBillingCalculateV2Routes(api huma.API, h *BillingCalculateHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "calculateBilling" + feeOpSuffixV2,
		Method:      http.MethodPost,
		Path:        feeBasePathV2 + "/billing/calculate",
		Summary:     "Calculate billing",
		Tags:        []string{"Billing Calculate"},
		Security:    secBillingBearer,
		// Body validated imperatively (feehttp.DecodeValidateBody) — see fees_v2_handler.go.
		SkipValidateBody: true,
	}, h.CalculateBillingV2Huma)
}

// RegisterFeesV2RoutesToApp wires the ledger-scoped fee/billing surface end-to-end on
// the /v2 contract: the Fiber guard chain on the /v2 group with group-relative paths,
// then the Huma terminals on that group's Huma API.
//
// The guard chain comes from feeGuardRoutes, the table the organization-scoped surface
// attaches too, so the (resource, verb) tuples of the two scopes cannot drift. Only the
// scope they hang off differs, and it is derived from feeBasePathV2 by feeChainPath
// rather than restated by hand: a Fiber spelling that drifts from the contract's is not
// caught by the contract diff, and a path parameter whose name falls outside the
// identifier allowlist is carried through as an unvalidated string.
// TestFeesV2RoutesParameterNamesAgree pins that every route agrees on its parameter
// names and that every name is one the UUID validator recognizes.
//
// It is additive — /v1 keeps serving the organization-scoped surface in parallel.
func RegisterFeesV2RoutesToApp(
	group fiber.Router,
	api huma.API,
	auth *middleware.AuthClient,
	ph *PackageHandler,
	fh *FeeHandler,
	bph *BillingPackageHandler,
	bch *BillingCalculateHandler,
	routeOptions *pkgHTTP.ProtectedRouteOptions,
) {
	attachFeeGuards(group, auth, routeOptions, feeChainPath(feeBasePathV2))

	RegisterFeesV2Routes(api, ph, fh, bph, bch)
}
