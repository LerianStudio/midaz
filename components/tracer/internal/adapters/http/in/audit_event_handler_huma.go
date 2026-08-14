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
)

// This file migrates the three AuditEventHandler operations to Huma, following
// the reference pattern established in rule_handler_huma.go (read that file's
// header for the full rationale). The conventions carried verbatim:
//
//   - In structs take path/query params so malformed input still flows through
//     the handler's imperative uuid.Parse / ListAuditEventsInput.Validate() and
//     yields the canonical Midaz error, never a native Huma 400/422. Path/query
//     params carry NO validation struct tag (only doc:) — a format/enum/min/max
//     tag would make Huma reject a bad value with a native 422 before the handler.
//   - Handler funcs delegate to the transport-agnostic cores on *AuditEventHandler
//     (listAuditEvents/getAuditEvent/verifyHashChain in audit_event_handler.go).
//   - Errors flow through the package-level humaProblem (defined in
//     rule_handler_huma.go) — reused verbatim, NOT redefined here.
//
// ListAuditEvents is the heaviest of the 2b handlers: 18 query params, several of
// them TYPED pointers (*model.AuditEventType/*AuditAction/*AuditResult/
// *ResourceType/*ActorType). Fiber's c.QueryParser (gorilla/schema) binds a
// present key into the typed pointer as &Type(value) — even for an empty value —
// then ListAuditEventsInput.Validate() runs the go-playground enum validators
// (auditeventtype/etc.) imperatively. The binder below reproduces that string->
// typed conversion exactly, so a bad value (event_type=INVALID) reaches Validate
// and produces the canonical 400 (formatValidationError default arm -> 0009), NOT
// a native Huma 422.

// ListAuditEventsInputHuma is the Huma request envelope for GET /v1/audit-events.
// Every query param carries only doc: (no min/max/enum/format/uuid tag) — the
// moment a param is typed or validated, Huma rejects a bad value with a native
// 422 before the handler. Validation stays imperative in
// ListAuditEventsInput.Validate(). See ListRulesInputHuma for the full present-
// vs-absent + last-wins rationale; the same applies here plus the typed-pointer
// conversion the audit filters need.
type ListAuditEventsInputHuma struct {
	StartDate       string `query:"start_date" doc:"Start date (RFC3339 format)"`
	EndDate         string `query:"end_date" doc:"End date (RFC3339 format)"`
	EventType       string `query:"event_type" doc:"Filter by event type (TRANSACTION_VALIDATED, RULE_*, LIMIT_*)"`
	Action          string `query:"action" doc:"Filter by action (VALIDATE, CREATE, UPDATE, DELETE, ACTIVATE, DEACTIVATE, DRAFT)"`
	Result          string `query:"result" doc:"Filter by result (SUCCESS, FAILED, ALLOW, DENY, REVIEW)"`
	ResourceType    string `query:"resource_type" doc:"Filter by resource type (transaction, rule, limit)"`
	ResourceID      string `query:"resource_id" doc:"Filter by resource ID (UUID)"`
	ActorType       string `query:"actor_type" doc:"Filter by actor type (user, system)"`
	ActorID         string `query:"actor_id" doc:"Filter by actor ID"`
	AccountID       string `query:"account_id" doc:"Filter by account ID (UUID)"`
	SegmentID       string `query:"segment_id" doc:"Filter by segment ID (UUID)"`
	PortfolioID     string `query:"portfolio_id" doc:"Filter by portfolio ID (UUID)"`
	TransactionType string `query:"transaction_type" doc:"Filter by transaction type (CARD, WIRE, PIX, CRYPTO)"`
	MatchedRuleID   string `query:"matched_rule_id" doc:"Filter by matched rule ID (UUID)"`
	Limit           string `query:"limit" doc:"Max items per page (1-1000, default: 100)"`
	Cursor          string `query:"cursor" doc:"Pagination token (empty for first page)"`
	SortBy          string `query:"sort_by" doc:"Sort field (created_at, event_type)"`
	SortOrder       string `query:"sort_order" doc:"Sort direction (ASC, DESC)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the
	// binding source (NOT the struct-tag fields above), so present-but-empty keys
	// survive to bindListAuditEventsInput. Unexported => absent from the spec.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler runs (see ListRulesInputHuma
// for why). It performs NO validation and NEVER returns an error — an error here
// would route through Huma's native WriteErr (a 422 the money-path forbids).
func (in *ListAuditEventsInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// bindListAuditEventsInput copies the query params into a *ListAuditEventsInput in
// the exact field shape Fiber's c.QueryParser produces, reading in.rawQuery so
// both present-vs-absent AND repeated-key (last-wins) resolution match Fiber:
//   - key absent            -> pointer stays nil (SetDefaults()/no-filter applies)
//   - key present-but-empty -> non-nil pointer to the empty typed value (or 0 for
//     limit), exactly as Fiber's gorilla-schema decoder yields; downstream
//     Validate() then rejects it (empty enum -> 0009, empty limit -> 0331).
//   - key repeated          -> the LAST value binds, matching Fiber's gorilla-
//     schema decode into a scalar target (via the local last() helper, NOT
//     url.Values.Get which returns the first).
//
// The typed-pointer fields are converted string->typed here (model.AuditEventType,
// etc.); the go-playground validators run against those pointers in Validate(), so
// a bad value produces the canonical 0009 — not a native Huma 422.
//
// The only value that can fail here is limit: a non-numeric LAST limit returns an
// error, which listAuditEvents canonicalizes to ErrInvalidQueryParameter (0082) —
// the SAME code Fiber's QueryParser-failure arm produced. An empty limit (?limit=)
// is NOT an error: it binds to 0 (matching gorilla), and Validate() rejects 0 with
// ErrPaginationLimitInvalid (0331), again as Fiber.
//
// The last()/optStr helpers are copied inline (NOT shared package-level) per the
// migration convention. See ListRulesInputHuma.bindListRulesInput.
func (in *ListAuditEventsInputHuma) bindListAuditEventsInput(target any) error {
	out, ok := target.(*ListAuditEventsInput)
	if !ok {
		// Unreachable: listAuditEvents always passes a *ListAuditEventsInput.
		return nil
	}

	q := in.rawQuery

	// last returns the LAST value of a repeated query key, matching Fiber's
	// c.QueryParser (gorilla/schema binds the last value into a scalar target),
	// NOT url.Values.Get which returns the FIRST.
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

	// Plain string fields: present-but-empty and absent both yield "" (matching
	// gorilla's scalar decode), so last() alone is faithful.
	out.StartDate = last("start_date")
	out.EndDate = last("end_date")
	out.Cursor = last("cursor")
	out.SortBy = last("sort_by")
	out.SortOrder = last("sort_order")

	// Untyped *string filters.
	out.ResourceID = optStr("resource_id")
	out.ActorID = optStr("actor_id")
	out.AccountID = optStr("account_id")
	out.SegmentID = optStr("segment_id")
	out.PortfolioID = optStr("portfolio_id")
	out.TransactionType = optStr("transaction_type")
	out.MatchedRuleID = optStr("matched_rule_id")

	// Typed enum *pointer filters: convert string->typed on present (even empty),
	// mirroring Fiber's gorilla decode into a custom-string-typed pointer target.
	if q.Has("event_type") {
		v := model.AuditEventType(last("event_type"))
		out.EventType = &v
	}

	if q.Has("action") {
		v := model.AuditAction(last("action"))
		out.Action = &v
	}

	if q.Has("result") {
		v := model.AuditResult(last("result"))
		out.Result = &v
	}

	if q.Has("resource_type") {
		v := model.ResourceType(last("resource_type"))
		out.ResourceType = &v
	}

	if q.Has("actor_type") {
		v := model.ActorType(last("actor_type"))
		out.ActorType = &v
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

// ListAuditEventsOutputHuma is the Huma response envelope for GET /v1/audit-events.
type ListAuditEventsOutputHuma struct {
	Status int
	Body   *ListAuditEventsResponse
}

// AuditEventIDInputHuma is the shared Huma request envelope for the by-id
// audit-event ops (get + verify). Path param carries no format tag (uuid.Parse is
// the sole validator — canonical 400/0065, never a native 422).
type AuditEventIDInputHuma struct {
	ID string `path:"id" doc:"Audit event ID (UUID)"`
}

// GetAuditEventOutputHuma is the Huma response envelope for GET
// /v1/audit-events/{id}.
type GetAuditEventOutputHuma struct {
	Status int
	Body   *model.AuditEvent
}

// VerifyHashChainOutputHuma is the Huma response envelope for GET
// /v1/audit-events/{id}/verify.
type VerifyHashChainOutputHuma struct {
	Status int
	Body   *model.HashChainVerificationResult
}

// ListAuditEventsHuma is the Huma handler for GET /v1/audit-events. It hands the
// shared core its own string->typed query binder (bindListAuditEventsInput); the
// core owns Validate/SetDefaults/filters/service/response so the result is
// identical to Fiber.
func (h *AuditEventHandler) ListAuditEventsHuma(ctx context.Context, in *ListAuditEventsInputHuma) (*ListAuditEventsOutputHuma, error) {
	result, err := h.listAuditEvents(ctx, in.bindListAuditEventsInput)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &ListAuditEventsOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// GetAuditEventHuma is the Huma handler for GET /v1/audit-events/{id}.
func (h *AuditEventHandler) GetAuditEventHuma(ctx context.Context, in *AuditEventIDInputHuma) (*GetAuditEventOutputHuma, error) {
	result, err := h.getAuditEvent(ctx, in.ID)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &GetAuditEventOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// VerifyHashChainHuma is the Huma handler for GET /v1/audit-events/{id}/verify.
func (h *AuditEventHandler) VerifyHashChainHuma(ctx context.Context, in *AuditEventIDInputHuma) (*VerifyHashChainOutputHuma, error) {
	result, err := h.verifyHashChain(ctx, in.ID)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &VerifyHashChainOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// RegisterAuditEventRoutes registers the migrated audit-event operations on the
// shared Huma API. It is the per-file seam NewRoutes calls; the auth middleware
// for these routes is attached in routes.go (Fiber-level), not here.
func RegisterAuditEventRoutes(api huma.API, h *AuditEventHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "listAuditEvents",
		Method:      http.MethodGet,
		Path:        "/audit-events",
		Summary:     "List audit events",
		Tags:        []string{"Audit"},
		Security:    secBearerOrAPIKey,
	}, h.ListAuditEventsHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getAuditEvent",
		Method:      http.MethodGet,
		Path:        "/audit-events/{id}",
		Summary:     "Get an audit event by ID",
		Tags:        []string{"Audit"},
		Security:    secBearerOrAPIKey,
	}, h.GetAuditEventHuma)

	huma.Register(api, huma.Operation{
		OperationID: "verifyAuditEvent",
		Method:      http.MethodGet,
		Path:        "/audit-events/{id}/verify",
		Summary:     "Verify audit event hash chain integrity",
		Tags:        []string{"Audit"},
		Security:    secBearerOrAPIKey,
	}, h.VerifyHashChainHuma)
}
