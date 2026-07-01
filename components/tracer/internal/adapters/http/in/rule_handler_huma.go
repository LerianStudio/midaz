// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

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

// UpdateRuleInputHuma is the Huma request envelope for PATCH /v1/rules/{id}.
// Like Create, the body is taken raw + SkipValidateBody so the imperative
// UpdateRuleInput.Validate()/IsEmpty() in updateRule stay the sole validators;
// the path param carries NO `format:"uuid"` (uuid.Parse is the sole path
// validator, yielding the canonical 400/0065 rather than a native Huma 422).
type UpdateRuleInputHuma struct {
	ID      string `path:"id" doc:"Rule ID (UUID)"`
	RawBody []byte `contentType:"application/json"`
}

// UpdateRuleOutputHuma is the Huma response envelope for PATCH /v1/rules/{id}.
type UpdateRuleOutputHuma struct {
	Status int
	Body   *model.Rule
}

// ListRulesInputHuma is the Huma request envelope for GET /v1/rules. Every query
// param carries only `doc:` (no min/max/enum/required/format tag): the moment a
// param is typed or validated, Huma rejects a bad value with a native 422 BEFORE
// the handler — the query-param analogue of the Phase-2a format:"uuid" bug. So
// Huma never rejects the value; listRules binds it into ListRulesInput and runs
// the imperative Validate()/SetDefaults(), producing the canonical 400 identical
// to the Fiber QueryParser+Validate path.
//
// The struct-tag fields exist ONLY to advertise the params in the generated spec.
// They are NOT the binding source: Huma drops a present-but-empty query value
// (`?status=`) before the handler — its request layer treats value=="" as unset
// and never sets the field (huma.go: `if value == "" { return }`), collapsing
// `?status=` and an absent `status` to the same empty string. Fiber's QueryParser
// does the OPPOSITE — a present-but-empty key yields a NON-nil pointer
// (`?status=` -> &"", `?limit=` -> &0), which downstream Validate() rejects
// (`RuleStatus("").IsValid()` false -> 0082; `*limit<1` -> 0331). Preserving that
// money-path distinction requires the raw query, which the resolver captures.
//
// rawQuery is populated by Resolve (huma.Resolver) from the request URL before
// the handler runs; bindListRulesInput reads it with url.Values so present-vs-
// absent matches Fiber byte-for-byte. Resolve NEVER returns an error (that would
// trigger Huma's native 422 path) — all rejection stays imperative in Validate().
type ListRulesInputHuma struct {
	Name            string `query:"name" doc:"Filter by name (case-insensitive partial match)"`
	Status          string `query:"status" doc:"Filter by status (DRAFT, ACTIVE, INACTIVE; DELETED not allowed)"`
	Action          string `query:"action" doc:"Filter by action (ALLOW, DENY, REVIEW)"`
	AccountID       string `query:"account_id" doc:"Filter by scope account_id (UUID)"`
	SegmentID       string `query:"segment_id" doc:"Filter by scope segment_id (UUID)"`
	PortfolioID     string `query:"portfolio_id" doc:"Filter by scope portfolio_id (UUID)"`
	MerchantID      string `query:"merchant_id" doc:"Filter by scope merchant_id (UUID)"`
	TransactionType string `query:"transaction_type" doc:"Filter by scope transaction_type (CARD, WIRE, PIX, CRYPTO)"`
	SubType         string `query:"sub_type" doc:"Filter by scope sub_type (case-insensitive; max 50 chars)"`
	Limit           string `query:"limit" doc:"Max items per page (1-100, default: 10)"`
	Cursor          string `query:"cursor" doc:"Pagination cursor (empty for first page)"`
	SortBy          string `query:"sort_by" doc:"Sort field (created_at, updated_at, name, status)"`
	SortOrder       string `query:"sort_order" doc:"Sort direction (ASC, DESC)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the
	// binding source (NOT the struct-tag fields above), so present-but-empty keys
	// survive to bindListRulesInput. Unexported => absent from the generated spec.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler runs. It implements
// huma.Resolver purely to reach the request URL (huma.Context) that the plain
// context.Context reaching the handler no longer carries. It performs NO
// validation and NEVER returns an error — an error here would route through
// Huma's native WriteErr (a 422 the money-path forbids). All canonical rejection
// stays imperative in ListRulesInput.Validate(), invoked by the shared core.
func (in *ListRulesInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// bindListRulesInput copies the query params into a *ListRulesInput in the exact
// field shape Fiber's c.QueryParser produces, reading in.rawQuery so both
// present-vs-absent AND repeated-key resolution match Fiber:
//   - key absent            -> pointer stays nil (SetDefaults()/no-filter applies)
//   - key present-but-empty -> non-nil pointer to "" (or 0 for limit), exactly as
//     Fiber's gorilla-schema decoder yields; downstream Validate() then rejects
//     ?status= /?action= (0082) and ?limit= (limit 0 -> 0331), same as Fiber.
//   - key repeated          -> the LAST value binds, matching Fiber's gorilla-
//     schema decode into a scalar target (via the local last() helper, NOT
//     url.Values.Get which returns the first). This subsumes present-but-empty:
//     ?status=A&status= binds "" (rejected 0082); ?status=&status=A binds "A".
//
// The only value that can fail here is limit: a non-numeric LAST limit (e.g.
// ?limit=abc, or ?limit=25&limit=abc where the last value is abc)
// returns an error, which listRules canonicalizes to ErrInvalidQueryParameter
// (0082) — the SAME code Fiber's QueryParser-failure arm produced. An empty limit
// (?limit=) is NOT an error: it binds to 0 (matching gorilla), and Validate()
// rejects 0 with ErrPaginationLimitInvalid (0331), again as Fiber.
func (in *ListRulesInputHuma) bindListRulesInput(target any) error {
	out, ok := target.(*ListRulesInput)
	if !ok {
		// Unreachable: listRules always passes a *ListRulesInput.
		return nil
	}

	q := in.rawQuery

	// last returns the LAST value of a repeated query key, matching Fiber's
	// c.QueryParser (gorilla/schema binds the last value into a scalar target),
	// NOT url.Values.Get which returns the FIRST. For a single or absent key it is
	// identical to Get; the divergence only appears on repeated keys, where using
	// the first value would flip the status/code vs. the Fiber path. nil-safe:
	// callers gate on q.Has(key), and an empty slice returns "".
	last := func(key string) string {
		vs := q[key]
		if len(vs) == 0 {
			return ""
		}

		return vs[len(vs)-1]
	}

	// optStr mirrors Fiber's QueryParser for *string fields: a present key (even
	// empty) yields a non-nil pointer holding the LAST value; an absent key leaves
	// it nil.
	optStr := func(key string) *string {
		if !q.Has(key) {
			return nil
		}

		s := last(key)

		return &s
	}

	out.Name = optStr("name")
	out.AccountID = optStr("account_id")
	out.SegmentID = optStr("segment_id")
	out.PortfolioID = optStr("portfolio_id")
	out.MerchantID = optStr("merchant_id")
	out.TransactionType = optStr("transaction_type")
	out.SubType = optStr("sub_type")
	out.Cursor = last("cursor")
	out.SortBy = last("sort_by")
	out.SortOrder = last("sort_order")

	if q.Has("status") {
		s := model.RuleStatus(last("status"))
		out.Status = &s
	}

	if q.Has("action") {
		a := model.Decision(last("action"))
		out.Action = &a
	}

	if q.Has("limit") {
		v := last("limit")

		// Empty limit binds to 0 (gorilla decodes "" as 0), NOT an error — Validate
		// then rejects 0 (0331). Only a non-numeric limit is a bind error (0082).
		n := 0

		if v != "" {
			parsed, err := strconv.Atoi(v)
			if err != nil {
				return err
			}

			n = parsed
		}

		out.Limit = &n
	}

	return nil
}

// ListRulesOutputHuma is the Huma response envelope for GET /v1/rules. Body is
// the ListRulesResponse cursor DTO serialized verbatim.
type ListRulesOutputHuma struct {
	Status int
	Body   *ListRulesResponse
}

// RuleIDInputHuma is the shared Huma request envelope for the id-only,
// body-less lifecycle ops (activate/deactivate/draft). No RawBody, no
// SkipValidateBody. Path param carries no format tag (uuid.Parse is the sole
// validator — canonical 400/0065, never a native 422).
type RuleIDInputHuma struct {
	ID string `path:"id" doc:"Rule ID (UUID)"`
}

// RuleOutputHuma is the shared 200 response envelope for the lifecycle ops.
type RuleOutputHuma struct {
	Status int
	Body   *model.Rule
}

// DeleteRuleOutputHuma is the Huma response envelope for DELETE /v1/rules/{id}.
// It has NO Body field: paired with huma.Operation{DefaultStatus: 204} it makes
// Huma emit a bodiless 204, matching the Fiber http.NoContent path exactly.
type DeleteRuleOutputHuma struct{}

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

// UpdateRuleHuma is the Huma handler for PATCH /v1/rules/{id}. It delegates to
// the shared core and, on success, returns 200 with the updated rule.
func (h *Handler) UpdateRuleHuma(ctx context.Context, in *UpdateRuleInputHuma) (*UpdateRuleOutputHuma, error) {
	result, err := h.updateRule(ctx, in.ID, in.RawBody)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &UpdateRuleOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// ListRulesHuma is the Huma handler for GET /v1/rules. It hands the shared core
// its own string->typed query binder (bindListRulesInput); the core owns
// Validate/SetDefaults/service/response so the result is identical to Fiber.
func (h *Handler) ListRulesHuma(ctx context.Context, in *ListRulesInputHuma) (*ListRulesOutputHuma, error) {
	result, err := h.listRules(ctx, in.bindListRulesInput)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &ListRulesOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// ActivateRuleHuma is the Huma handler for POST /v1/rules/{id}/activate.
func (h *Handler) ActivateRuleHuma(ctx context.Context, in *RuleIDInputHuma) (*RuleOutputHuma, error) {
	result, err := h.activateRule(ctx, in.ID)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &RuleOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// DeactivateRuleHuma is the Huma handler for POST /v1/rules/{id}/deactivate.
func (h *Handler) DeactivateRuleHuma(ctx context.Context, in *RuleIDInputHuma) (*RuleOutputHuma, error) {
	result, err := h.deactivateRule(ctx, in.ID)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &RuleOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// DraftRuleHuma is the Huma handler for POST /v1/rules/{id}/draft.
func (h *Handler) DraftRuleHuma(ctx context.Context, in *RuleIDInputHuma) (*RuleOutputHuma, error) {
	result, err := h.draftRule(ctx, in.ID)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &RuleOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// DeleteRuleHuma is the Huma handler for DELETE /v1/rules/{id}. On success it
// returns an empty DeleteRuleOutputHuma; paired with DefaultStatus:204 Huma
// emits a bodiless 204, matching the Fiber http.NoContent path.
func (h *Handler) DeleteRuleHuma(ctx context.Context, in *RuleIDInputHuma) (*DeleteRuleOutputHuma, error) {
	if err := h.deleteRule(ctx, in.ID); err != nil {
		return nil, humaProblem(err)
	}

	return &DeleteRuleOutputHuma{}, nil
}

// RegisterRuleRoutes registers the migrated rule operations on the shared Huma
// API. It is the per-file seam NewRoutes calls; the auth middleware for these
// routes is attached in routes.go (Fiber-level), not here. As of Phase 2b-1 all
// eight rule operations are Huma-registered.
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

	huma.Register(api, huma.Operation{
		OperationID: "listRules",
		Method:      http.MethodGet,
		Path:        "/rules",
		Summary:     "List fraud rules",
		Tags:        []string{"Rules"},
	}, h.ListRulesHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateRule",
		Method:           http.MethodPatch,
		Path:             "/rules/{id}",
		Summary:          "Partially update an existing fraud rule",
		Tags:             []string{"Rules"},
		SkipValidateBody: true, // body validated imperatively — see CreateRule.
	}, h.UpdateRuleHuma)

	huma.Register(api, huma.Operation{
		OperationID: "activateRule",
		Method:      http.MethodPost,
		Path:        "/rules/{id}/activate",
		Summary:     "Activate a fraud rule",
		Tags:        []string{"Rules"},
	}, h.ActivateRuleHuma)

	huma.Register(api, huma.Operation{
		OperationID: "deactivateRule",
		Method:      http.MethodPost,
		Path:        "/rules/{id}/deactivate",
		Summary:     "Deactivate a fraud rule",
		Tags:        []string{"Rules"},
	}, h.DeactivateRuleHuma)

	huma.Register(api, huma.Operation{
		OperationID: "draftRule",
		Method:      http.MethodPost,
		Path:        "/rules/{id}/draft",
		Summary:     "Transition a rule back to draft",
		Tags:        []string{"Rules"},
	}, h.DraftRuleHuma)

	huma.Register(api, huma.Operation{
		OperationID: "deleteRule",
		Method:      http.MethodDelete,
		Path:        "/rules/{id}",
		Summary:     "Delete a fraud rule",
		Tags:        []string{"Rules"},
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204,
		// matching the Fiber http.NoContent path.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteRuleHuma)
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
