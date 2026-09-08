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

// RegisterBillingCalculateRoutes registers the billing-calculate operation on the given
// Huma API. It is the per-file seam the unified server calls; the auth
// ("midaz","billing-calculate","post") + tenant +
// ParseUUIDPathParameters("billing-calculate") middleware chain is attached on the
// versioned Fiber group BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE.
//
// opSuffix is appended to the operation ID — see v2OpSuffix.
func RegisterBillingCalculateRoutes(api huma.API, h *BillingCalculateHandler, opSuffix string) {
	huma.Register(api, huma.Operation{
		OperationID: "calculateBilling" + opSuffix,
		Method:      http.MethodPost,
		Path:        "/organizations/{organization_id}/ledgers/{ledger_id}/billing/calculate",
		Summary:     "Calculate billing",
		Tags:        []string{"Billing Calculate"},
		Security:    secBillingBearer,
		// Body validated imperatively (feehttp.DecodeValidateBody) — see file header.
		SkipValidateBody: true,
	}, h.CalculateBillingV2)
	attachTypedRequestBody[model.BillingCalculateRequest](api, "calculateBilling"+opSuffix)
}

// RegisterBillingCalculateV2RoutesToApp wires the billing-calculate surface onto the
// /v2 contract, which is the ONLY version group that serves it. See
// registerBillingCalculateRoutesToApp for the auth chain and tenant options it
// attaches.
func RegisterBillingCalculateV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *BillingCalculateHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerBillingCalculateRoutesToApp(group, api, auth, h, routeOptions, v2OpSuffix)
}

// registerBillingCalculateRoutesToApp is the single description of the
// billing-calculate route surface, shared by every versioned contract that serves it,
// mirroring registerPackageRoutesToApp. It attaches the Fiber auth chain —
// auth.Authorize("midaz","billing-calculate","post") + the fees-scoped tenant
// PostAuthMiddlewares (routeOptions) + ParseUUIDPathParameters("billing-calculate") —
// as MIDDLEWARE ONLY (no terminal handler, and no body binder, because the Huma
// terminal decodes and validates the body imperatively) on the VERSIONED GROUP with
// GROUP-RELATIVE paths, then registers the Huma terminal via
// RegisterBillingCalculateRoutes on the SAME group's Huma API.
//
// The ParseUUIDPathParameters label is the span-attribute name; the middleware validates
// every UUID path param regardless of label. See registerPackageRoutesToApp on why the
// Fiber and OpenAPI parameter names must agree.
func registerBillingCalculateRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *BillingCalculateHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const billingCalculatePath = "/organizations/:organization_id/ledgers/:ledger_id/billing/calculate"

	billingCalculateParse := pkgHTTP.ParseUUIDPathParameters("billing-calculate")

	routePost(group, billingCalculatePath, protectedMidaz(auth, "billing-calculate", "post", routeOptions, billingCalculateParse))

	RegisterBillingCalculateRoutes(api, h, opSuffix)
}
