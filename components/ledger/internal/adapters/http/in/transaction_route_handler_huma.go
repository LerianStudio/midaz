// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the transaction-route resource (Wave 2,
// money-read + routing). It mirrors the operation-route exemplar
// (operation_route_handler_huma.go); see the asset exemplar's header for the full
// conventions. Transaction-route-specific notes:
//
//  1. AUTH is the "routing" appName, resource "transaction-routes" (protectedRouting
//     in routes.go), NOT "midaz". The swaggo @Security on the Fiber wrappers is
//     BearerAuth ONLY, so the per-op Security metadata here is Bearer-only too —
//     SPEC metadata only; runtime auth stays the Fiber guard chain
//     (auth.Authorize("routing","transaction-routes",verb) + tenant +
//     ParseUUIDPathParameters("transaction_route")) attached in the unified server
//     BEFORE the Huma terminal.
//  2. NO merge-patch landmine. Unlike operation-route, transaction-route uses a
//     NORMAL typed body (no accountingEntries, no RFC 7396 field-absent-vs-null
//     distinction), so the cores take the decoded *Input only — no rawBody. Both
//     POST and PATCH keep RawBody + SkipValidateBody so http.DecodeAndValidate is
//     the sole body validator (never a native Huma 422).
//  3. The Create/Update/Delete cores own the side-effects the Fiber methods used to
//     inline (accounting-route cache write on Create/Update, cache delete on Delete,
//     the created metric) so BOTH transports behave identically.
//  4. List is cursor-based (no offset page, no HEAD-count). The raw query is captured
//     via Resolve and fed to the imperative http.ValidateParameters binder.
//  5. Errors go through the shared pkgHTTP.HumaProblem.

// secTransactionRouteBearer advertises that each transaction-route operation accepts
// a JWT bearer token (Bearer-only, matching the Fiber swaggo @Security BearerAuth).
// SPEC metadata only; runtime auth is the Fiber guard chain.
var secTransactionRouteBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /transaction-routes -------------------------------------------------

// CreateTransactionRouteInputHuma is the Huma request envelope for POST. RawBody
// keeps the body out of Huma's validator and feeds the imperative decode.
type CreateTransactionRouteInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateTransactionRouteOutputHuma pins 201 (matching http.Created).
type CreateTransactionRouteOutputHuma struct {
	Status int
	Body   *mmodel.TransactionRoute
}

// CreateTransactionRouteHuma decodes+validates the raw body imperatively then delegates
// to the shared createTransactionRoute core.
func (handler *TransactionRouteHandler) CreateTransactionRouteHuma(ctx context.Context, in *CreateTransactionRouteInputHuma) (*CreateTransactionRouteOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreateTransactionRouteInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionRoute, err := handler.createTransactionRoute(ctx, orgID, ledgerID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateTransactionRouteOutputHuma{Status: http.StatusCreated, Body: transactionRoute}, nil
}

// --- GET /transaction-routes (list) -------------------------------------------

// ListTransactionRoutesInputHuma advertises the cursor-list query params (doc-only)
// and captures the raw query via Resolve for the imperative binder.
type ListTransactionRoutesInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Limit          string `query:"limit" doc:"Max items per page (default 10)"`
	StartDate      string `query:"start_date" doc:"Filter created on/after this date (YYYY-MM-DD)"`
	EndDate        string `query:"end_date" doc:"Filter created on/before this date (YYYY-MM-DD)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`
	Cursor         string `query:"cursor" doc:"Opaque cursor token for pagination"`

	rawQuery url.Values
}

// Resolve captures the raw query before the handler (no validation; canonical
// rejection stays in http.ValidateParameters).
func (in *ListTransactionRoutesInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key). Inlined per
// the pattern (the query binder is copied, not a shared helper).
func (in *ListTransactionRoutesInputHuma) queries() map[string]string {
	out := make(map[string]string, len(in.rawQuery))
	for k, vs := range in.rawQuery {
		if len(vs) == 0 {
			out[k] = ""
			continue
		}

		out[k] = vs[len(vs)-1]
	}

	return out
}

// ListTransactionRoutesOutputHuma carries the pagination envelope verbatim.
type ListTransactionRoutesOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// GetAllTransactionRoutesHuma binds the query imperatively then delegates to
// getAllTransactionRoutes.
func (handler *TransactionRouteHandler) GetAllTransactionRoutesHuma(ctx context.Context, in *ListTransactionRoutesInputHuma) (*ListTransactionRoutesOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllTransactionRoutes(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListTransactionRoutesOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /transaction-routes/{transaction_route_id} ---------------------------

// GetTransactionRouteInputHuma is the by-id request envelope. The id path param
// carries no format tag (ParseUUIDPathParameters is the sole validator).
type GetTransactionRouteInputHuma struct {
	OrganizationID     string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID           string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	TransactionRouteID string `path:"transaction_route_id" doc:"Transaction Route ID (UUID)"`
}

// GetTransactionRouteOutputHuma carries the transaction route verbatim.
type GetTransactionRouteOutputHuma struct {
	Status int
	Body   *mmodel.TransactionRoute
}

// GetTransactionRouteByIDHuma delegates to getTransactionRouteByID.
func (handler *TransactionRouteHandler) GetTransactionRouteByIDHuma(ctx context.Context, in *GetTransactionRouteInputHuma) (*GetTransactionRouteOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.TransactionRouteID, "transaction_route_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionRoute, err := handler.getTransactionRouteByID(ctx, orgID, ledgerID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetTransactionRouteOutputHuma{Status: http.StatusOK, Body: transactionRoute}, nil
}

// --- PATCH /transaction-routes/{transaction_route_id} -------------------------

// UpdateTransactionRouteInputHuma is the update request envelope (RawBody, see Create).
type UpdateTransactionRouteInputHuma struct {
	OrganizationID     string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID           string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	TransactionRouteID string `path:"transaction_route_id" doc:"Transaction Route ID (UUID)"`
	RawBody            []byte `contentType:"application/json"`
}

// UpdateTransactionRouteOutputHuma carries the updated route (200, matching http.OK).
type UpdateTransactionRouteOutputHuma struct {
	Status int
	Body   *mmodel.TransactionRoute
}

// UpdateTransactionRouteHuma decodes+validates the raw body imperatively then delegates
// to the shared updateTransactionRoute core.
func (handler *TransactionRouteHandler) UpdateTransactionRouteHuma(ctx context.Context, in *UpdateTransactionRouteInputHuma) (*UpdateTransactionRouteOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.TransactionRouteID, "transaction_route_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdateTransactionRouteInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionRoute, err := handler.updateTransactionRoute(ctx, orgID, ledgerID, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateTransactionRouteOutputHuma{Status: http.StatusOK, Body: transactionRoute}, nil
}

// --- DELETE /transaction-routes/{transaction_route_id} ------------------------

// DeleteTransactionRouteOutputHuma has NO Body field: paired with DefaultStatus 204
// it makes Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteTransactionRouteOutputHuma struct{}

// DeleteTransactionRouteByIDHuma delegates to deleteTransactionRouteByID; returns a
// bodiless 204 on success.
func (handler *TransactionRouteHandler) DeleteTransactionRouteByIDHuma(ctx context.Context, in *GetTransactionRouteInputHuma) (*DeleteTransactionRouteOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.TransactionRouteID, "transaction_route_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteTransactionRouteByID(ctx, orgID, ledgerID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteTransactionRouteOutputHuma{}, nil
}

// RegisterTransactionRouteRoutes registers the five migrated transaction-route
// operations on the shared Huma API. It is the per-file seam the unified server
// calls; the auth ("routing","transaction-routes",verb) + tenant +
// ParseUUIDPathParameters("transaction_route") middleware chain is attached on the
// /v1 group (Fiber-level) BEFORE the Huma terminal, not here. Paths are
// GROUP-RELATIVE (see asset_handler_huma.go's RegisterAssetRoutes header for the
// /v1 rationale).
func RegisterTransactionRouteRoutes(api huma.API, h *TransactionRouteHandler) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes"
		idPath   = listPath + "/{transaction_route_id}"
		tag      = "Transaction Routes"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createTransactionRoute",
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create Transaction Route",
		Tags:        []string{tag},
		Security:    secTransactionRouteBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
	}, h.CreateTransactionRouteHuma)

	huma.Register(api, huma.Operation{
		OperationID: "listTransactionRoutes",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all Transaction Routes",
		Tags:        []string{tag},
		Security:    secTransactionRouteBearer,
	}, h.GetAllTransactionRoutesHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getTransactionRouteByID",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get Transaction Route by ID",
		Tags:        []string{tag},
		Security:    secTransactionRouteBearer,
	}, h.GetTransactionRouteByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateTransactionRoute",
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update Transaction Route",
		Tags:             []string{tag},
		Security:         secTransactionRouteBearer,
		SkipValidateBody: true, // body validated imperatively — see file header.
	}, h.UpdateTransactionRouteHuma)

	huma.Register(api, huma.Operation{
		OperationID: "deleteTransactionRoute",
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete Transaction Route by ID",
		Tags:        []string{tag},
		Security:    secTransactionRouteBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteTransactionRouteByIDHuma)
}
