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

// This file migrates the two TransactionValidationHandler operations (Get, List)
// to Huma, following the reference pattern established in rule_handler_huma.go
// (read that file's header for the full rationale). The conventions carried
// verbatim:
//
//   - Path/query params carry NO validation struct tag (only doc:) — the
//     imperative uuid.Parse / ListTransactionValidationsInput.Validate() is the
//     sole validator, so a bad value yields the canonical Midaz 400, never a
//     native Huma 422.
//   - Handler funcs delegate to the transport-agnostic cores on
//     *TransactionValidationHandler (getTransactionValidation /
//     listTransactionValidations in transaction_validation_handler.go).
//   - Errors flow through the package-level humaProblem (defined in
//     rule_handler_huma.go) — reused verbatim, NOT redefined here.
//
// Transaction validations are immutable (SOX/GLBA) — only read operations exist,
// so there is no RawBody/SkipValidateBody envelope here.

// GetTransactionValidationInputHuma is the Huma request envelope for
// GET /v1/validations/{id}. The path param carries NO format:"uuid": uuid.Parse
// in the core is the sole validator (canonical 400/0065, never a native 422).
type GetTransactionValidationInputHuma struct {
	ID string `path:"id" doc:"Transaction Validation ID (UUID)"`
}

// GetTransactionValidationOutputHuma is the Huma response envelope for
// GET /v1/validations/{id}.
type GetTransactionValidationOutputHuma struct {
	Status int
	Body   *model.TransactionValidation
}

// ListTransactionValidationsInputHuma is the Huma request envelope for
// GET /v1/validations. Every query param carries only doc: (no min/max/enum/
// format tag) — the moment a param is typed or validated, Huma rejects a bad
// value with a native 422 before the handler. Validation stays imperative in
// ListTransactionValidationsInput.Validate(). See ListRulesInputHuma for the full
// present-vs-absent + last-wins rationale; here only `limit` (a *int) needs the
// present-vs-absent distinction since every other field is a plain string.
type ListTransactionValidationsInputHuma struct {
	Limit           string `query:"limit" doc:"Max items per page (1-1000, default: 100)"`
	Cursor          string `query:"cursor" doc:"Pagination cursor from previous response"`
	SortBy          string `query:"sort_by" doc:"Field to sort by (created_at, processing_time_ms; default: created_at)"`
	SortOrder       string `query:"sort_order" doc:"Sort direction (ASC, DESC; default: DESC)"`
	StartDate       string `query:"start_date" doc:"Filter from this date (RFC3339)"`
	EndDate         string `query:"end_date" doc:"Filter to this date (RFC3339)"`
	Decision        string `query:"decision" doc:"Filter by decision (ALLOW, DENY, REVIEW)"`
	AccountID       string `query:"account_id" doc:"Filter by account ID (UUID)"`
	MatchedRuleID   string `query:"matched_rule_id" doc:"Filter by matched rule ID (UUID)"`
	ExceededLimitID string `query:"exceeded_limit_id" doc:"Filter by exceeded limit ID (UUID)"`
	SegmentID       string `query:"segment_id" doc:"Filter by segment ID (UUID)"`
	PortfolioID     string `query:"portfolio_id" doc:"Filter by portfolio ID (UUID)"`
	TransactionType string `query:"transaction_type" doc:"Filter by transaction type (CARD, WIRE, PIX, CRYPTO)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the
	// binding source (NOT the struct-tag fields above), so present-but-empty keys
	// survive to bindListTransactionValidationsInput. Unexported => absent from
	// the generated spec.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler runs. It implements
// huma.Resolver purely to reach the request URL that the plain context.Context
// reaching the handler no longer carries. It performs NO validation and NEVER
// returns an error — an error here would route through Huma's native WriteErr (a
// 422 the money-path forbids). All canonical rejection stays imperative in
// ListTransactionValidationsInput.Validate(), invoked by the shared core.
func (in *ListTransactionValidationsInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// bindListTransactionValidationsInput copies the query params into a
// *ListTransactionValidationsInput in the exact field shape Fiber's
// c.QueryParser produces, reading in.rawQuery so both present-vs-absent (for the
// *int limit) AND repeated-key (last-wins) resolution match Fiber. The
// last()/optStr helpers are copied inline (NOT shared package-level) per the
// migration convention. See ListRulesInputHuma.bindListRulesInput for the full
// contract.
func (in *ListTransactionValidationsInputHuma) bindListTransactionValidationsInput(target any) error {
	out, ok := target.(*ListTransactionValidationsInput)
	if !ok {
		// Unreachable: listTransactionValidations always passes a
		// *ListTransactionValidationsInput.
		return nil
	}

	q := in.rawQuery

	// last returns the LAST value of a repeated query key, matching Fiber's
	// c.QueryParser (gorilla/schema binds the last value into a scalar target),
	// NOT url.Values.Get which returns the FIRST. For a single or absent key it is
	// identical to Get; the divergence only appears on repeated keys, where using
	// the first value would flip the status/code vs. the Fiber path.
	last := func(key string) string {
		vs := q[key]
		if len(vs) == 0 {
			return ""
		}

		return vs[len(vs)-1]
	}

	// Every non-limit field is a plain string on the input: Fiber's QueryParser
	// binds a present-but-empty OR absent key to "", so last() reproduces both
	// (an absent key yields "", a repeated key yields its LAST value).
	out.Cursor = last("cursor")
	out.SortBy = last("sort_by")
	out.SortOrder = last("sort_order")
	out.StartDate = last("start_date")
	out.EndDate = last("end_date")
	out.Decision = last("decision")
	out.AccountID = last("account_id")
	out.MatchedRuleID = last("matched_rule_id")
	out.ExceededLimitID = last("exceeded_limit_id")
	out.SegmentID = last("segment_id")
	out.PortfolioID = last("portfolio_id")
	out.TransactionType = last("transaction_type")

	// limit is a *int: a present key (even empty) yields a non-nil pointer; an
	// absent key leaves it nil (SetDefaults fills the default). Empty limit binds
	// to 0 (gorilla decodes "" as 0), NOT an error — Validate then rejects 0
	// (0331). Only a non-numeric LAST limit is a bind error (0082).
	if q.Has("limit") {
		v := last("limit")

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

// ListTransactionValidationsOutputHuma is the Huma response envelope for
// GET /v1/validations. Body is the ListTransactionValidationsResponse cursor DTO
// serialized verbatim.
type ListTransactionValidationsOutputHuma struct {
	Status int
	Body   *ListTransactionValidationsResponse
}

// GetTransactionValidationHuma is the Huma handler for GET /v1/validations/{id}.
// It delegates to the shared core and, on success, returns 200 with the record.
func (h *TransactionValidationHandler) GetTransactionValidationHuma(ctx context.Context, in *GetTransactionValidationInputHuma) (*GetTransactionValidationOutputHuma, error) {
	result, err := h.getTransactionValidation(ctx, in.ID)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &GetTransactionValidationOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// ListTransactionValidationsHuma is the Huma handler for GET /v1/validations. It
// hands the shared core its own string->typed query binder; the core owns
// Validate/SetDefaults/filters/service/response so the result is identical to
// Fiber.
func (h *TransactionValidationHandler) ListTransactionValidationsHuma(ctx context.Context, in *ListTransactionValidationsInputHuma) (*ListTransactionValidationsOutputHuma, error) {
	result, err := h.listTransactionValidations(ctx, in.bindListTransactionValidationsInput)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &ListTransactionValidationsOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// RegisterTransactionValidationRoutes registers the migrated transaction
// validation operations on the shared Huma API. It is the per-file seam
// NewRoutes calls; the auth middleware for these routes is attached in routes.go
// (Fiber-level), not here. Paths are GROUP-RELATIVE to the /v1 Fiber group.
func RegisterTransactionValidationRoutes(api huma.API, h *TransactionValidationHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "getValidation",
		Method:      http.MethodGet,
		Path:        "/validations/{id}",
		Summary:     "Get a transaction validation record by ID",
		Tags:        []string{"Validations"},
	}, h.GetTransactionValidationHuma)

	huma.Register(api, huma.Operation{
		OperationID: "listValidations",
		Method:      http.MethodGet,
		Path:        "/validations",
		Summary:     "List transaction validation records",
		Tags:        []string{"Validations"},
	}, h.ListTransactionValidationsHuma)
}
