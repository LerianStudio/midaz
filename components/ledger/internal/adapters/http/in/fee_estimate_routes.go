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

// RegisterFeeEstimateRoutes registers the fee-estimate operation on the given Huma API.
// It is the per-file seam the unified server calls; the auth
// ("midaz","estimates","post") + tenant + ParseUUIDPathParameters("estimates")
// middleware chain is attached on the versioned Fiber group BEFORE the Huma terminal,
// not here. Paths are GROUP-RELATIVE.
//
// POST /fees — the fee calculation itself — is deliberately absent: in the unified
// binary fees run in-process via the transaction seam, so only the dry-run estimate is
// exposed over HTTP.
//
// opSuffix is appended to the operation ID — see v2OpSuffix.
func RegisterFeeEstimateRoutes(api huma.API, h *FeeHandler, opSuffix string) {
	huma.Register(api, huma.Operation{
		OperationID: "estimateFeeCalculation" + opSuffix,
		Method:      http.MethodPost,
		Path:        "/organizations/{organization_id}/ledgers/{ledger_id}/estimates",
		Summary:     "Create a fee estimate calculation",
		Tags:        []string{"Fees"},
		Security:    secFeeBearer,
		// Body validated imperatively (feehttp.DecodeValidateBody) — see file header.
		SkipValidateBody: true,
	}, h.EstimateFeeCalculationV2)
	attachTypedRequestBody[model.FeeEstimate](api, "estimateFeeCalculation"+opSuffix)
}

// RegisterFeeEstimateV2RoutesToApp wires the fee-estimate surface onto the /v2
// contract, which is the ONLY version group that serves it. See
// registerFeeEstimateRoutesToApp for the auth chain and tenant options it attaches.
func RegisterFeeEstimateV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *FeeHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerFeeEstimateRoutesToApp(group, api, auth, h, routeOptions, v2OpSuffix)
}

// registerFeeEstimateRoutesToApp is the single description of the fee-estimate route
// surface, shared by every versioned contract that serves it, mirroring
// registerPackageRoutesToApp. It attaches the Fiber auth chain —
// auth.Authorize("midaz","estimates","post") + the fees-scoped tenant
// PostAuthMiddlewares (routeOptions) + ParseUUIDPathParameters("estimates") — as
// MIDDLEWARE ONLY (no terminal handler, and no body binder, because the Huma terminal
// decodes and validates the body imperatively) on the VERSIONED GROUP with
// GROUP-RELATIVE paths, then registers the Huma terminal via RegisterFeeEstimateRoutes
// on the SAME group's Huma API.
//
// The ParseUUIDPathParameters label is the span-attribute name; the middleware validates
// every UUID path param regardless of label. See registerPackageRoutesToApp on why the
// Fiber and OpenAPI parameter names must agree.
func registerFeeEstimateRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *FeeHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const estimatesPath = "/organizations/:organization_id/ledgers/:ledger_id/estimates"

	estimateParse := pkgHTTP.ParseUUIDPathParameters("estimates")

	routePost(group, estimatesPath, protectedMidaz(auth, "estimates", "post", routeOptions, estimateParse))

	RegisterFeeEstimateRoutes(api, h, opSuffix)
}
