// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the billing-calculate (compute) op. It
// mirrors the fee-estimate sibling (fees_handler_huma.go) and the asset exemplar
// (asset_handler_huma.go); see the asset header for the full conventions. Billing-
// calculate-specific notes:
//
//  1. AUTH is appName "plugin-fees" (fees_routes.go feesApplicationName — the LEGACY
//     RBAC namespace preserved verbatim), resource "billing-calculate", verb "post".
//     The Fiber guard chain is Bearer-only, so the per-op
//     Security metadata here is Bearer-only too — SPEC metadata only; runtime auth
//     stays the Fiber guard chain (auth.Authorize("plugin-fees","billing-calculate",
//     "post") + tenant + ParseUUIDPathParameters("billing-calculate")) attached BEFORE
//     the Huma terminal.
//  2. ORG-SCOPED (no ledger in the path): the shell resolves only organization_id via
//     the shared parseOrg helper.
//  3. LANDMINE — WithBodyTracing: the Fiber route decodes the body via the fee-package
//     feehttp.WithBodyTracing, NOT the standard http.WithBody. This shell PRESERVES
//     BOTH the fee validator and the replicated "middleware.body_parsing" span by
//     decoding through decodeFeeBodyInSpan (defined in fees_handler_huma.go). It does
//     NOT use pkgHTTP.DecodeAndValidate — that would swap the validator instance. The
//     handler-level validateBillingCalculateRequest (ledgerId/period/type semantics)
//     stays in the shared calculateBilling core, so the Huma path is byte-identical.
//  4. 200 is intentional: this is a compute/RPC-style endpoint that persists nothing.
//     Unlike the fee-estimate op (whose response embeds the transaction tree and forces
//     a raw-[]byte escape hatch), BillingCalculateResponse is a flat Results+Summary
//     struct with no time.Time-alias schema-gen landmine, so it serializes as a normal
//     typed Body.
//  5. Errors go through the shared pkgHTTP.HumaProblem.

// --- POST /billing/calculate --------------------------------------------------

// CalculateBillingInputHuma is the Huma request envelope for POST. RawBody keeps the
// body out of Huma's validator (see file header); the org path param is validated by
// the Fiber middleware, not by a format tag.
type CalculateBillingInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CalculateBillingOutputHuma carries the calculation envelope at 200.
type CalculateBillingOutputHuma struct {
	Status int
	Body   *model.BillingCalculateResponse
}

// CalculateBillingHuma decodes+validates the raw body imperatively (fee validator,
// inside the replicated body-parsing span) then delegates to the shared
// calculateBilling core.
func (handler *BillingCalculateHandler) CalculateBillingHuma(ctx context.Context, in *CalculateBillingInputHuma) (*CalculateBillingOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.BillingCalculateRequest)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	result, err := handler.calculateBilling(ctx, orgID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CalculateBillingOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// RegisterBillingCalculateRoutes registers the migrated billing-calculate operation on
// the shared Huma API. It is the per-file seam the unified server calls; the auth
// ("plugin-fees","billing-calculate","post") + tenant +
// ParseUUIDPathParameters("billing-calculate") middleware chain is attached on the /v1
// group (Fiber-level) BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE
// (see asset_handler_huma.go's RegisterAssetRoutes header for the /v1 rationale).
func RegisterBillingCalculateRoutes(api huma.API, h *BillingCalculateHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "calculateBilling",
		Method:      http.MethodPost,
		Path:        "/organizations/{organization_id}/billing/calculate",
		Summary:     "Calculate billing",
		Tags:        []string{"Billing Calculate"},
		Security:    secBillingBearer,
		// Body validated imperatively (feehttp.DecodeValidateBody) — see file header.
		SkipValidateBody: true,
	}, h.CalculateBillingHuma)
}
