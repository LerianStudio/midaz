// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	"go.opentelemetry.io/otel/attribute"

	libObservability "github.com/LerianStudio/lib-observability/v2"

	feehttp "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
)

// This file holds the shared fee-body decode helper, the fee-estimate response envelope,
// and the estimate operation's security metadata. The ledger-scoped estimate handler
// that consumes them lives in fees_v2_handler.go; the auth
// ("plugin-fees","estimates","post") + tenant + ParseUUIDPathParameters("estimates")
// middleware chain is attached on the /v2 Fiber group BEFORE the Huma terminal (see
// fees_v2_register.go), so the Security metadata here is SPEC metadata only.
//
// LANDMINE — the fee body is decoded via the fee-package feehttp.DecodeValidateBody (the
// fee ValidateStruct/findUnknownFields/parseMetadata, a DIFFERENT validator instance
// from pkg/net/http's), inside a replicated "middleware.body_parsing" span. Handlers MUST
// route through decodeFeeBodyInSpan rather than pkgHTTP.DecodeAndValidate, which would
// silently swap the validator.

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
