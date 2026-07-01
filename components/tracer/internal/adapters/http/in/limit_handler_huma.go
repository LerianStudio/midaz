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

// This file migrates the nine LimitHandler operations to Huma, following the
// reference pattern established in rule_handler_huma.go (read that file's header
// for the full rationale). The conventions carried verbatim:
//
//   - In structs take path/query params + a RawBody []byte (contentType JSON) so
//     malformed JSON and imperative Validate() still yield the canonical Midaz
//     error, never a native Huma 400/422. Path/query params carry NO validation
//     struct tag (only doc:) — the imperative uuid.Parse / Validate() is the sole
//     validator.
//   - Handler funcs delegate to the transport-agnostic cores on *LimitHandler
//     (createLimit/getLimit/listLimits/updateLimit/... in limit_handler.go).
//   - Errors flow through the package-level humaProblem (defined in
//     rule_handler_huma.go) — reused verbatim, NOT redefined here.

// CreateLimitInputHuma is the Huma request envelope for POST /v1/limits.
type CreateLimitInputHuma struct {
	RawBody []byte `contentType:"application/json"`
}

// CreateLimitOutputHuma is the Huma response envelope for POST /v1/limits.
type CreateLimitOutputHuma struct {
	Status int
	Body   *model.Limit
}

// GetLimitInputHuma is the Huma request envelope for GET /v1/limits/{id}.
type GetLimitInputHuma struct {
	ID string `path:"id" doc:"Limit ID (UUID)"`
}

// GetLimitOutputHuma is the Huma response envelope for GET /v1/limits/{id}.
type GetLimitOutputHuma struct {
	Status int
	Body   *model.Limit
}

// UpdateLimitInputHuma is the Huma request envelope for PATCH /v1/limits/{id}.
// The body is taken raw so the core's immutable-field map-probe +
// UpdateLimitInput.Validate()/IsEmpty() stay the sole validators.
type UpdateLimitInputHuma struct {
	ID      string `path:"id" doc:"Limit ID (UUID)"`
	RawBody []byte `contentType:"application/json"`
}

// UpdateLimitOutputHuma is the Huma response envelope for PATCH /v1/limits/{id}.
type UpdateLimitOutputHuma struct {
	Status int
	Body   *model.Limit
}

// ListLimitsInputHuma is the Huma request envelope for GET /v1/limits. Every
// query param carries only doc: (no min/max/enum/format tag) — the moment a
// param is typed or validated, Huma rejects a bad value with a native 422 before
// the handler. Validation stays imperative in ListLimitsInput.Validate(). See
// ListRulesInputHuma for the full present-vs-absent + last-wins rationale.
type ListLimitsInputHuma struct {
	Name            string `query:"name" doc:"Filter by name (case-insensitive partial match)"`
	Status          string `query:"status" doc:"Filter by status (DRAFT, ACTIVE, INACTIVE)"`
	LimitType       string `query:"limit_type" doc:"Filter by limit type (DAILY, WEEKLY, MONTHLY, CUSTOM, PER_TRANSACTION)"`
	AccountID       string `query:"account_id" doc:"Filter by scope account_id (UUID)"`
	SegmentID       string `query:"segment_id" doc:"Filter by scope segment_id (UUID)"`
	PortfolioID     string `query:"portfolio_id" doc:"Filter by scope portfolio_id (UUID)"`
	MerchantID      string `query:"merchant_id" doc:"Filter by scope merchant_id (UUID)"`
	TransactionType string `query:"transaction_type" doc:"Filter by scope transaction_type (CARD, WIRE, PIX, CRYPTO)"`
	SubType         string `query:"sub_type" doc:"Filter by scope sub_type (case-insensitive; max 50 chars)"`
	Limit           string `query:"limit" doc:"Max items per page (1-100, default: 10)"`
	Cursor          string `query:"cursor" doc:"Pagination cursor (empty for first page)"`
	SortBy          string `query:"sort_by" doc:"Sort field (created_at, updated_at, name, max_amount)"`
	SortOrder       string `query:"sort_order" doc:"Sort direction (ASC, DESC)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the
	// binding source (NOT the struct-tag fields above), so present-but-empty keys
	// survive to bindListLimitsInput. Unexported => absent from the generated spec.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler runs (see ListRulesInputHuma
// for why). It performs NO validation and NEVER returns an error.
func (in *ListLimitsInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// bindListLimitsInput copies the query params into a *ListLimitsInput in the
// exact field shape Fiber's c.QueryParser produces, reading in.rawQuery so both
// present-vs-absent AND repeated-key (last-wins) resolution match Fiber. The
// last()/optStr helpers are copied inline (NOT shared package-level) per the
// migration convention. See ListRulesInputHuma.bindListRulesInput for the full
// contract.
func (in *ListLimitsInputHuma) bindListLimitsInput(target any) error {
	out, ok := target.(*ListLimitsInput)
	if !ok {
		// Unreachable: listLimits always passes a *ListLimitsInput.
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

	out.Name = optStr("name")
	out.AccountID = optStr("account_id")
	out.SegmentID = optStr("segment_id")
	out.PortfolioID = optStr("portfolio_id")
	out.MerchantID = optStr("merchant_id")
	out.TransactionType = optStr("transaction_type")
	out.SubType = optStr("sub_type")
	out.Cursor = last("cursor")
	out.Status = last("status")
	out.LimitType = last("limit_type")
	out.SortBy = last("sort_by")
	out.SortOrder = last("sort_order")

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

// ListLimitsOutputHuma is the Huma response envelope for GET /v1/limits.
type ListLimitsOutputHuma struct {
	Status int
	Body   *ListLimitsResponse
}

// LimitIDInputHuma is the shared Huma request envelope for the id-only,
// body-less lifecycle ops (activate/deactivate/draft) plus get-usage.
type LimitIDInputHuma struct {
	ID string `path:"id" doc:"Limit ID (UUID)"`
}

// LimitOutputHuma is the shared 200 response envelope for the lifecycle ops.
type LimitOutputHuma struct {
	Status int
	Body   *model.Limit
}

// GetLimitUsageOutputHuma is the Huma response envelope for GET
// /v1/limits/{id}/usage.
type GetLimitUsageOutputHuma struct {
	Status int
	Body   *model.UsageSnapshot
}

// DeleteLimitOutputHuma is the Huma response envelope for DELETE
// /v1/limits/{id}. It has NO Body field: paired with DefaultStatus:204 Huma
// emits a bodiless 204, matching the Fiber http.NoContent path.
type DeleteLimitOutputHuma struct{}

// CreateLimitHuma is the Huma handler for POST /v1/limits.
func (h *LimitHandler) CreateLimitHuma(ctx context.Context, in *CreateLimitInputHuma) (*CreateLimitOutputHuma, error) {
	result, err := h.createLimit(ctx, in.RawBody)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &CreateLimitOutputHuma{Status: http.StatusCreated, Body: result}, nil
}

// GetLimitHuma is the Huma handler for GET /v1/limits/{id}.
func (h *LimitHandler) GetLimitHuma(ctx context.Context, in *GetLimitInputHuma) (*GetLimitOutputHuma, error) {
	result, err := h.getLimit(ctx, in.ID)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &GetLimitOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// UpdateLimitHuma is the Huma handler for PATCH /v1/limits/{id}.
func (h *LimitHandler) UpdateLimitHuma(ctx context.Context, in *UpdateLimitInputHuma) (*UpdateLimitOutputHuma, error) {
	result, err := h.updateLimit(ctx, in.ID, in.RawBody)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &UpdateLimitOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// ListLimitsHuma is the Huma handler for GET /v1/limits.
func (h *LimitHandler) ListLimitsHuma(ctx context.Context, in *ListLimitsInputHuma) (*ListLimitsOutputHuma, error) {
	result, err := h.listLimits(ctx, in.bindListLimitsInput)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &ListLimitsOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// ActivateLimitHuma is the Huma handler for POST /v1/limits/{id}/activate.
func (h *LimitHandler) ActivateLimitHuma(ctx context.Context, in *LimitIDInputHuma) (*LimitOutputHuma, error) {
	result, err := h.activateLimit(ctx, in.ID)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &LimitOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// DeactivateLimitHuma is the Huma handler for POST /v1/limits/{id}/deactivate.
func (h *LimitHandler) DeactivateLimitHuma(ctx context.Context, in *LimitIDInputHuma) (*LimitOutputHuma, error) {
	result, err := h.deactivateLimit(ctx, in.ID)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &LimitOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// DraftLimitHuma is the Huma handler for POST /v1/limits/{id}/draft.
func (h *LimitHandler) DraftLimitHuma(ctx context.Context, in *LimitIDInputHuma) (*LimitOutputHuma, error) {
	result, err := h.draftLimit(ctx, in.ID)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &LimitOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// DeleteLimitHuma is the Huma handler for DELETE /v1/limits/{id}. On success it
// returns an empty DeleteLimitOutputHuma; paired with DefaultStatus:204 Huma
// emits a bodiless 204.
func (h *LimitHandler) DeleteLimitHuma(ctx context.Context, in *LimitIDInputHuma) (*DeleteLimitOutputHuma, error) {
	if err := h.deleteLimit(ctx, in.ID); err != nil {
		return nil, humaProblem(err)
	}

	return &DeleteLimitOutputHuma{}, nil
}

// GetLimitUsageHuma is the Huma handler for GET /v1/limits/{id}/usage.
func (h *LimitHandler) GetLimitUsageHuma(ctx context.Context, in *LimitIDInputHuma) (*GetLimitUsageOutputHuma, error) {
	result, err := h.getLimitUsage(ctx, in.ID)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &GetLimitUsageOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// RegisterLimitRoutes registers the migrated limit operations on the shared Huma
// API. It is the per-file seam NewRoutes calls; the auth middleware for these
// routes is attached in routes.go (Fiber-level), not here.
func RegisterLimitRoutes(api huma.API, h *LimitHandler) {
	huma.Register(api, huma.Operation{
		OperationID:      "createLimit",
		Method:           http.MethodPost,
		Path:             "/limits",
		Summary:          "Create a new spending limit",
		Tags:             []string{"Limits"},
		SkipValidateBody: true, // body validated imperatively — see rule_handler_huma.go.
	}, h.CreateLimitHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getLimit",
		Method:      http.MethodGet,
		Path:        "/limits/{id}",
		Summary:     "Get a spending limit by ID",
		Tags:        []string{"Limits"},
	}, h.GetLimitHuma)

	huma.Register(api, huma.Operation{
		OperationID: "listLimits",
		Method:      http.MethodGet,
		Path:        "/limits",
		Summary:     "List spending limits",
		Tags:        []string{"Limits"},
	}, h.ListLimitsHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateLimit",
		Method:           http.MethodPatch,
		Path:             "/limits/{id}",
		Summary:          "Partially update an existing spending limit",
		Tags:             []string{"Limits"},
		SkipValidateBody: true,
	}, h.UpdateLimitHuma)

	huma.Register(api, huma.Operation{
		OperationID: "activateLimit",
		Method:      http.MethodPost,
		Path:        "/limits/{id}/activate",
		Summary:     "Activate a spending limit",
		Tags:        []string{"Limits"},
	}, h.ActivateLimitHuma)

	huma.Register(api, huma.Operation{
		OperationID: "deactivateLimit",
		Method:      http.MethodPost,
		Path:        "/limits/{id}/deactivate",
		Summary:     "Deactivate a spending limit",
		Tags:        []string{"Limits"},
	}, h.DeactivateLimitHuma)

	huma.Register(api, huma.Operation{
		OperationID: "draftLimit",
		Method:      http.MethodPost,
		Path:        "/limits/{id}/draft",
		Summary:     "Transition a limit back to draft",
		Tags:        []string{"Limits"},
	}, h.DraftLimitHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteLimit",
		Method:        http.MethodDelete,
		Path:          "/limits/{id}",
		Summary:       "Delete a spending limit",
		Tags:          []string{"Limits"},
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteLimitHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getLimitUsage",
		Method:      http.MethodGet,
		Path:        "/limits/{id}/usage",
		Summary:     "Get usage snapshot for a limit",
		Tags:        []string{"Limits"},
	}, h.GetLimitUsageHuma)
}
