// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel/attribute"

	libObservability "github.com/LerianStudio/lib-observability"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feehttp "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
	feeerrors "github.com/LerianStudio/midaz/v4/pkg"
	feeconstant "github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the fee-estimate (dry-run) op. It
// mirrors the asset exemplar (asset_handler_huma.go); see that file's header for the
// full conventions. Fee-specific notes:
//
//  1. AUTH is appName "plugin-fees" (fees_routes.go feesApplicationName — the LEGACY
//     RBAC namespace preserved verbatim), resource "estimates", verb "post". The
//     swaggo @Security on the Fiber wrapper is BearerAuth ONLY, so the per-op Security
//     metadata here is Bearer-only too — SPEC metadata only; runtime auth stays the
//     Fiber guard chain (auth.Authorize("plugin-fees","estimates","post") + tenant +
//     ParseUUIDPathParameters("estimates")) attached BEFORE the Huma terminal.
//  2. ORG-SCOPED (no ledger in the path): the shell resolves only organization_id via
//     the shared parseOrg helper (defined in ledger_handler_huma.go).
//  3. LANDMINE — WithBodyTracing: the Fiber route decodes the body via the fee-package
//     feehttp.WithBodyTracing, NOT the standard http.WithBody. WithBodyTracing (a)
//     wraps the decode in a dedicated "middleware.body_parsing" span, and (b) runs the
//     fee package's OWN decode+validate pipeline (feehttp.DecodeValidateBody — the fee
//     ValidateStruct/findUnknownFields/parseMetadata, a DIFFERENT validator instance
//     from pkg/net/http's). This shell PRESERVES BOTH: it decodes via
//     feehttp.DecodeValidateBody (byte-identical validator) inside a replicated
//     "middleware.body_parsing" span (observability parity). It does NOT use
//     pkgHTTP.DecodeAndValidate — that would silently swap the validator.
//  4. Errors go through the shared pkgHTTP.HumaProblem.

// secFeeBearer advertises that the estimate operation accepts a JWT bearer token
// (Bearer-only, matching the Fiber swaggo @Security BearerAuth). SPEC metadata only;
// runtime auth is the Fiber guard chain.
var secFeeBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// decodeFeeBodyInSpan runs the fee-package decode+validate pipeline
// (feehttp.DecodeValidateBody) inside a "middleware.body_parsing" span, replicating
// the observability of the Fiber feehttp.WithBodyTracing decorator on the Huma path.
// The Fiber-only span attributes (url.path/http.route/method) are transport-specific
// and omitted; request_id and body size — the fields that make the span meaningful —
// are preserved.
func decodeFeeBodyInSpan(ctx context.Context, rawBody []byte, payload any) error {
	_, tracer, reqID, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "middleware.body_parsing")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.Int("http.request.body.size", len(rawBody)),
	)

	if _, err := feehttp.DecodeValidateBody(rawBody, payload); err != nil {
		span.SetAttributes(attribute.String("error.message", err.Error()))

		return err
	}

	return nil
}

// --- POST /estimates ----------------------------------------------------------

// EstimateFeeInputHuma is the Huma request envelope for POST. RawBody keeps the body
// out of Huma's validator (see file header); the org path param is validated by the
// Fiber middleware, not by a format tag.
type EstimateFeeInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// EstimateFeeOutputHuma carries the estimate envelope at 200 (the endpoint is a
// compute/RPC-style calculation that persists nothing).
//
// Body is a pre-serialized []byte, NOT *model.FeeEstimateResponse: the response
// embeds the projected transaction tree (FeeEstimateResult → FeeAdjustedTransaction →
// mtransaction.Send / mtransaction.TransactionDate), and mtransaction.TransactionDate
// is a named `time.Time` alias carrying an `example:"2021-01-01T00:00:00Z"` swaggo tag
// that Huma's schema generator parses as JSON — a bare timestamp is invalid JSON for a
// non-string schema, so schema gen panics. No other migrated ledger resource exposes
// the transaction tree, so this is the fee-estimate-only escape hatch. The raw []byte
// keeps Huma from recursing into that tree (it schema-gens as an opaque string) while
// the wire bytes stay byte-identical to the Fiber commonsHttp.Respond(JSON) path;
// swaggo remains the OAS source of truth for this op (annotations INTACT on the Fiber
// wrapper). ContentType pins application/json so the response header matches Fiber.
type EstimateFeeOutputHuma struct {
	Status int
	Body   []byte `contentType:"application/json"`
}

// EstimateFeeCalculationHuma decodes+validates the raw body imperatively (fee-package
// validator, inside the replicated body-parsing span) then delegates to the shared
// estimateFeeCalculation core and serializes the envelope verbatim.
func (handler *FeeHandler) EstimateFeeCalculationHuma(ctx context.Context, in *EstimateFeeInputHuma) (*EstimateFeeOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.FeeEstimate)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	response, err := handler.estimateFeeCalculation(ctx, orgID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	body, err := json.Marshal(response)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(feeerrors.ValidateInternalError(feeconstant.ErrInternalServer, "Fee"))
	}

	return &EstimateFeeOutputHuma{Status: http.StatusOK, Body: body}, nil
}

// RegisterFeeEstimateRoutes registers the migrated fee-estimate operation on the
// shared Huma API. It is the per-file seam the unified server calls; the auth
// ("plugin-fees","estimates","post") + tenant + ParseUUIDPathParameters("estimates")
// middleware chain is attached on the /v1 group (Fiber-level) BEFORE the Huma
// terminal, not here. Paths are GROUP-RELATIVE (see asset_handler_huma.go's
// RegisterAssetRoutes header for the /v1 rationale).
func RegisterFeeEstimateRoutes(api huma.API, h *FeeHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "estimateFeeCalculation",
		Method:      http.MethodPost,
		Path:        "/organizations/{organization_id}/estimates",
		Summary:     "Create a fee estimate calculation",
		Tags:        []string{"Fees"},
		Security:    secFeeBearer,
		// Body validated imperatively (feehttp.DecodeValidateBody) — see file header.
		SkipValidateBody: true,
	}, h.EstimateFeeCalculationHuma)
}
