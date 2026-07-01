// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the REFERENCE Huma adoption pattern for the tracer component.
// The other 28 handlers (across rule/limit/validation/reservation/audit) copy
// this shape in the Phase-2b fan-out. Conventions established here:
//
//  1. In/Out structs: request path/query params + a RawBody []byte (NOT a typed
//     Body). RawBody keeps Huma from parsing+validating the body, so malformed
//     JSON and field validation still flow through the handler's imperative
//     json.Unmarshal + Input.Validate() and produce the canonical Midaz error —
//     same code/status/type as the Fiber path, no new native Huma 400/422. The
//     `contentType:"application/json"` tag keeps the generated spec advertising
//     JSON rather than octet-stream.
//     Path/query params carry NO validation struct tag (no `format:"uuid"`,
//     etc.): unlike the body they can't be SkipValidate'd, so a format tag would
//     make Huma reject a bad value with a native 422 before the handler, bypassing
//     the imperative uuid.Parse that yields the canonical 400 / code 0065. Only
//     `doc:` for the spec. Out carries Body (the model type, serialized from the
//     SAME model.Rule the Fiber path serializes because openapi.New strips Huma's
//     SchemaLinkTransformer, so no `$schema`/`$ref` leaks) + Status (the success
//     code). NOTE: the success body is field-identical to Fiber, not byte-
//     identical — Huma encodes via json.NewEncoder(w).Encode (trailing '\n') with
//     SetEscapeHTML(false), while Fiber defaults to SetEscapeHTML(true) and no
//     trailing newline. Both decode to the identical map, so any JSON-parsing
//     consumer (including the generated SDK) sees identical data; only a raw-byte
//     / hash / ETag consumer would observe a difference, and this API has none.
//     Do NOT align the encoders — it fights the framework for zero functional
//     gain.
//
//  2. Handler funcs: func(ctx, *In) (*Out, error). They delegate to the
//     transport-agnostic core on *Handler (createRule/getRule), which owns the
//     span, imperative validation, the service call, and the success log. The
//     Huma ctx is the humafiber v2 adapter's copy of c.UserContext() — so the
//     tenant/DB the tenant middleware injected (c.SetUserContext) reaches the
//     service through this ctx with NO bridge. That is the whole reason the API
//     is mounted on the SAME /v1 group that carries the tenant middleware.
//
//  3. Errors: the core returns the canonical Midaz error; the func converts it
//     to *pkgHTTP.Detail via humaProblem and returns it as the error. *Detail
//     satisfies huma.StatusError (GetStatus/Error) and ContentTypeFilter
//     (application/problem+json) via the embedded huma.ErrorModel, so Huma
//     serializes the frozen RFC 9457 envelope built by the shared
//     pkgHTTP.ProblemDetail — field/status/code/type/entityType-identical to the
//     Fiber http.WithError path (the two transports share ProblemDetail, so the
//     decoded envelopes match exactly, guarded by the money-path golden net; the
//     raw bytes differ by the same encoder trailing-'\n' + HTML-escaping noted in
//     point 1, invisible to any JSON parser).
//
//  4. Auth stays a Fiber middleware on the route (guard.With(...)), registered
//     in routes.go alongside the Huma registration — NOT a Huma per-op Security
//     scheme (that is spec-only and deferred to Epic 2.3).
//
//  5. RegisterXxxRoutes(api huma.API, h *Handler) is the per-file registration
//     seam NewRoutes calls, enabling the parallel per-file fan-out in 2b.

// CreateRuleInputHuma is the Huma request envelope for POST /v1/rules. The body
// is taken raw (see file header rationale) so the handler's imperative
// CreateRuleInput.Validate() remains the sole validator.
type CreateRuleInputHuma struct {
	RawBody []byte `contentType:"application/json"`
}

// CreateRuleOutputHuma is the Huma response envelope for POST /v1/rules. Body is
// the created rule serialized verbatim; Status pins 201 to match http.Created.
type CreateRuleOutputHuma struct {
	Status int
	Body   *model.Rule
}

// GetRuleInputHuma is the Huma request envelope for GET /v1/rules/{id}. The path
// param carries NO `format:"uuid"`: path params can't be SkipValidate'd, so a
// format tag would make Huma reject a malformed id with a native 422 BEFORE the
// handler — diverging from the canonical 400 / code 0065 (ErrInvalidPathParameter)
// the shared getRule core produces via uuid.Parse. Leaving uuid.Parse as the sole
// path validator is the same principle as RawBody+SkipValidateBody for the body.
// The 2b fan-out must carry NO format/struct-tag validation on path/query params.
type GetRuleInputHuma struct {
	ID string `path:"id" doc:"Rule ID (UUID)"`
}

// GetRuleOutputHuma is the Huma response envelope for GET /v1/rules/{id}.
type GetRuleOutputHuma struct {
	Status int
	Body   *model.Rule
}

// CreateRuleHuma is the Huma handler for POST /v1/rules. It delegates to the
// shared core and, on success, returns 201 with the created rule.
func (h *Handler) CreateRuleHuma(ctx context.Context, in *CreateRuleInputHuma) (*CreateRuleOutputHuma, error) {
	result, err := h.createRule(ctx, in.RawBody)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &CreateRuleOutputHuma{Status: http.StatusCreated, Body: result}, nil
}

// GetRuleHuma is the Huma handler for GET /v1/rules/{id}. It delegates to the
// shared core and, on success, returns 200 with the rule.
func (h *Handler) GetRuleHuma(ctx context.Context, in *GetRuleInputHuma) (*GetRuleOutputHuma, error) {
	result, err := h.getRule(ctx, in.ID)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &GetRuleOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// RegisterRuleRoutes registers the migrated rule operations on the shared Huma
// API. It is the per-file seam NewRoutes calls; the auth middleware for these
// routes is attached in routes.go (Fiber-level), not here. Only the two
// reference operations are Huma-registered in Phase 2a; the remaining rule
// operations stay inline Fiber in routes.go until the 2b fan-out.
func RegisterRuleRoutes(api huma.API, h *Handler) {
	// Paths are GROUP-RELATIVE: the Huma API is bound to the /v1 Fiber group, so
	// the humafiber adapter registers on that group and Fiber prepends /v1. The
	// /v1 prefix rides the OpenAPI `servers` entry (set in openapi.New's Config),
	// keeping operation paths relative and the routes single-prefixed.
	huma.Register(api, huma.Operation{
		OperationID: "createRule",
		Method:      http.MethodPost,
		Path:        "/rules",
		Summary:     "Create a new fraud rule",
		Tags:        []string{"Rules"},
		// SkipValidateBody: the body is taken as RawBody and validated
		// imperatively by CreateRuleInput.Validate() inside the handler, which
		// produces the canonical Midaz error codes. Without this, Huma validates
		// the JSON body against the RawBody string/binary schema and rejects it
		// with a native 422 before the handler runs — exactly the divergence the
		// migration must avoid.
		SkipValidateBody: true,
	}, h.CreateRuleHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getRule",
		Method:      http.MethodGet,
		Path:        "/rules/{id}",
		Summary:     "Get a fraud rule by ID",
		Tags:        []string{"Rules"},
	}, h.GetRuleHuma)
}

// humaProblem converts a canonical Midaz error (already classified + span-
// attributed by the handler core) into the frozen RFC 9457 *pkgHTTP.Detail,
// returned as the huma.StatusError so Huma serializes the problem+json body. It
// shares pkgHTTP.ProblemDetail with the Fiber http.WithError path, so the two
// transports emit field/status/code/type-identical envelopes — the decoded
// bodies match exactly (guarded by the money-path golden net); the raw bytes
// differ only by Huma's encoder trailing-'\n' + HTML-escaping, invisible to any
// JSON parser. This is the reference error seam the 2b fan-out reuses verbatim.
//
// *Detail satisfies huma.StatusError (GetStatus/Error) and ContentTypeFilter
// (application/problem+json) via the embedded huma.ErrorModel, so returning it
// as the error is all Huma needs to render the frozen body + correct status.
func humaProblem(err error) error {
	detail, ok := pkgHTTP.ProblemDetail(err)
	if !ok {
		// Unreachable: ProblemDetail only fails on MapError's non-*Detail return.
		// Fall back to the canonical sanitized 500 shape.
		detail, _ = pkgHTTP.ProblemDetail(nil)
	}

	return &detail
}
