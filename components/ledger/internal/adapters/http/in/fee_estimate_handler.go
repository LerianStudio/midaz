// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"net/http"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feehttp "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
	feeerrors "github.com/LerianStudio/midaz/v4/pkg"
	feeconstant "github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the Huma transport of the fee-estimate resource: the shared fee-body
// decode helper, the response envelope, the operation's security metadata, and the
// shell that decodes a request, calls estimateFeeCalculation in fee_estimate_core.go,
// and renders the envelope.
//
// The shell names the ledger in its path and resolves it via parseFeeV2Path, and the
// body's own ledger must agree with the path — see requireBodyLedgerMatchesPath in
// fee_ledger_scope.go.
//
// LANDMINE — the fee body is decoded via the fee-package feehttp.DecodeValidateBody (the
// fee ValidateStruct/findUnknownFields/parseMetadata, a DIFFERENT validator instance
// from pkg/net/http's), inside a replicated "middleware.body_parsing" span. Handlers MUST
// route through decodeFeeBodyInSpan rather than pkgHTTP.DecodeAndValidate, which would
// silently swap the validator.
//
// AUTH is appName "midaz" (routes.go midazName), resource "estimates". The Fiber guard
// chain — auth.Authorize("midaz","estimates","post") + the fees-scoped tenant
// PostAuthMiddlewares + ParseUUIDPathParameters("estimates") — is attached on the /v2
// group BEFORE this terminal (see fee_estimate_routes.go), so the Security metadata is
// SPEC metadata only.

// secFeeBearer advertises that the estimate operation accepts a JWT bearer token
// (Bearer-only, matching the Fiber guard chain). SPEC metadata only;
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

// EstimateFeeResponse carries the estimate envelope at 200 (the endpoint is a
// compute/RPC-style calculation that persists nothing).
//
// Body is a pre-serialized []byte, NOT *model.FeeEstimateResponse: the response
// embeds the projected transaction tree (FeeEstimateResult → FeeAdjustedTransaction →
// mtransaction.Send / mtransaction.TransactionDate), and mtransaction.TransactionDate
// is a named `time.Time` alias carrying an `example:"2021-01-01T00:00:00Z"` struct tag
// that Huma's schema generator parses as JSON — a bare timestamp is invalid JSON for a
// non-string schema, so schema gen panics. No other migrated ledger resource exposes
// the transaction tree, so this is the fee-estimate-only escape hatch. The raw []byte
// keeps Huma from recursing into that tree (it schema-gens as an opaque string) while
// the wire bytes stay byte-identical to the Fiber commonsHttp.Respond(JSON) path.
// ContentType pins application/json so the response header matches Fiber.
type EstimateFeeResponse struct {
	Status int
	Body   []byte `contentType:"application/json"`
}

// --- POST /ledgers/{ledger_id}/estimates -----------------------------------------

// EstimateFeeV2Request is the ledger-scoped estimate envelope (RawBody, see Create).
type EstimateFeeV2Request struct {
	FeeV2Path
	RawBody []byte `contentType:"application/json"`
}

// EstimateFeeCalculationV2 decodes+validates the raw body imperatively, refuses a
// body ledger that disagrees with the path, then delegates to the shared
// estimateFeeCalculation core and serializes the envelope verbatim.
//
// The response Body stays a pre-serialized []byte for the reason
// EstimateFeeResponse documents: the estimate embeds the projected transaction tree,
// whose time alias makes Huma's schema generator panic. The escape hatch is a property
// of the response type, so it holds identically on both contracts.
func (handler *FeeHandler) EstimateFeeCalculationV2(ctx context.Context, in *EstimateFeeV2Request) (*EstimateFeeResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.FeeEstimate)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := requireLedgerMatchesPath(payload.LedgerID, ledgerID); err != nil {
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

	return &EstimateFeeResponse{Status: http.StatusOK, Body: body}, nil
}
