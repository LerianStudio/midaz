// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel/trace"
)

// TraceReference returns the per-occurrence reference published as the RFC 9457
// `instance` member: the request's trace id, or "" when the request carries no
// recorded trace.
//
// It reads the id through the SAME accessor the loggers use for their trace.id
// field (trace.SpanFromContext(ctx).SpanContext().TraceID().String()), so the
// value a customer quotes from an error body is byte-for-byte the value an
// operator greps for in the logs. Any other source — a fresh uuid, a request id,
// a formatted variant — would hand the customer a reference that finds nothing.
//
// The empty return is deliberate: `instance` carries omitempty, so no trace
// means the member is absent from the body rather than present and blank. A
// blank string reads as an answer, and support would chase it.
func TraceReference(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return ""
	}

	return sc.TraceID().String()
}

// problemInstanceAPI decorates a Huma API so every problem+json body it writes
// carries the per-occurrence reference.
//
// Why a decorator rather than the obvious parameter: the reference needs the
// request context, and the shared classifier that builds the body
// (ProblemDetail) is reached from 378 handler error paths through HumaProblem,
// none of which could supply one without threading a parameter through all of
// them. Huma already transforms every response body — error bodies included —
// through API.Transform with the request context in hand, so overriding that one
// method reaches every path at once and no handler changes.
//
// lib-commons' openapi.New clears huma.Config.Transformers and exposes no hook
// to register one, so the decorator is applied to the API it returns.
type problemInstanceAPI struct {
	huma.API
}

// Transform stamps the reference onto a Midaz problem body on its way out, then
// defers to the wrapped API. A body that already carries one is left alone, and
// a non-problem body (every success response) is untouched.
func (a problemInstanceAPI) Transform(ctx huma.Context, status string, v any) (any, error) {
	if detail, ok := v.(*Detail); ok && detail.Instance == "" {
		detail.Instance = TraceReference(ctx.Context())
	}

	return a.API.Transform(ctx, status, v)
}

// WithProblemInstance wraps api so that every error body it writes carries the
// per-occurrence reference in the RFC 9457 `instance` member.
//
// Apply it to the API returned by openapi.New and register every operation on
// the result: huma.Register captures the API it is handed, and only that API's
// Transform runs when a response is written.
//
//nolint:ireturn // the huma.API interface is the type every caller registers on.
func WithProblemInstance(api huma.API) huma.API {
	if api == nil {
		return nil
	}

	return problemInstanceAPI{API: api}
}
