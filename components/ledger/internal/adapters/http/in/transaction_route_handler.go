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

// This file is the ledger's Huma adoption of the transaction-route resource
// (money-read + routing). It mirrors the operation-route exemplar
// (operation_route_handler.go); see the asset exemplar's header for the full
// conventions. Transaction-route-specific notes:
//
//  1. AUTH is the "midaz" appName, resource "transaction-routes". The Fiber guard
//     chain is Bearer-only, so the per-op Security metadata here is Bearer-only too —
//     SPEC metadata only; runtime auth stays the Fiber guard chain
//     (auth.Authorize("midaz","transaction-routes",verb) + tenant +
//     ParseUUIDPathParameters("transaction_route")) attached in the unified server
//     BEFORE the Huma terminal.
//  2. NO merge-patch landmine. Unlike operation-route, transaction-route uses a
//     NORMAL typed body (no accountingEntries, no RFC 7396 field-absent-vs-null
//     distinction), so the cores take the decoded *Input only — no rawBody. Both
//     POST and PATCH keep RawBody + SkipValidateBody so http.DecodeAndValidate is
//     the sole body validator (never a native Huma 422).
//  3. The Create/Update/Delete cores own the side-effects (accounting-route cache
//     write on Create/Update, cache delete on Delete, the created metric).
//  4. List is cursor-based (no offset page, no HEAD-count). The raw query is captured
//     via Resolve and fed to the imperative http.ValidateParameters binder.
//  5. Errors go through the shared pkgHTTP.HumaProblem.

// secTransactionRouteBearer advertises that each transaction-route operation accepts
// a JWT bearer token (Bearer-only, matching the Fiber guard chain).
// SPEC metadata only; runtime auth is the Fiber guard chain.
var secTransactionRouteBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /transaction-routes -------------------------------------------------

// CreateTransactionRouteRequest is the Huma request envelope for POST. RawBody
// keeps the body out of Huma's validator and feeds the imperative decode.
type CreateTransactionRouteRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateTransactionRouteResponse pins 201 (matching http.Created).
type CreateTransactionRouteResponse struct {
	Status int
	Body   *mmodel.TransactionRoute
}

// CreateTransactionRoute decodes+validates the raw body imperatively then delegates
// to the shared createTransactionRoute core.
func (handler *TransactionRouteHandler) CreateTransactionRoute(ctx context.Context, in *CreateTransactionRouteRequest) (*CreateTransactionRouteResponse, error) {
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

	return &CreateTransactionRouteResponse{Status: http.StatusCreated, Body: transactionRoute}, nil
}

// --- GET /transaction-routes (list) -------------------------------------------

// ListTransactionRoutesRequest advertises the cursor-list query params (doc-only)
// and captures the raw query via Resolve for the imperative binder.
type ListTransactionRoutesRequest struct {
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
func (in *ListTransactionRoutesRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key). Inlined per
// the pattern (the query binder is copied, not a shared helper).
func (in *ListTransactionRoutesRequest) queries() map[string]string {
	return queriesFromValues(in.rawQuery)
}

// ListTransactionRoutesResponse carries the pagination envelope verbatim.
type ListTransactionRoutesResponse struct {
	Status int
	Body   pkgHTTP.Pagination
}

// GetAllTransactionRoutes binds the query imperatively then delegates to
// getAllTransactionRoutes.
func (handler *TransactionRouteHandler) GetAllTransactionRoutes(ctx context.Context, in *ListTransactionRoutesRequest) (*ListTransactionRoutesResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllTransactionRoutes(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListTransactionRoutesResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /transaction-routes/{transaction_route_id} ---------------------------

// GetTransactionRouteRequest is the by-id request envelope. The id path param
// carries no format tag (ParseUUIDPathParameters is the sole validator).
type GetTransactionRouteRequest struct {
	OrganizationID     string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID           string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	TransactionRouteID string `path:"transaction_route_id" doc:"Transaction Route ID (UUID)"`
}

// GetTransactionRouteResponse carries the transaction route verbatim.
type GetTransactionRouteResponse struct {
	Status int
	Body   *mmodel.TransactionRoute
}

// GetTransactionRouteByID delegates to getTransactionRouteByID.
func (handler *TransactionRouteHandler) GetTransactionRouteByID(ctx context.Context, in *GetTransactionRouteRequest) (*GetTransactionRouteResponse, error) {
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

	return &GetTransactionRouteResponse{Status: http.StatusOK, Body: transactionRoute}, nil
}

// --- PATCH /transaction-routes/{transaction_route_id} -------------------------

// UpdateTransactionRouteRequest is the update request envelope (RawBody, see Create).
type UpdateTransactionRouteRequest struct {
	OrganizationID     string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID           string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	TransactionRouteID string `path:"transaction_route_id" doc:"Transaction Route ID (UUID)"`
	RawBody            []byte `contentType:"application/json"`
}

// UpdateTransactionRouteResponse carries the updated route (200, matching http.OK).
type UpdateTransactionRouteResponse struct {
	Status int
	Body   *mmodel.TransactionRoute
}

// UpdateTransactionRoute decodes+validates the raw body imperatively then delegates
// to the shared updateTransactionRoute core.
func (handler *TransactionRouteHandler) UpdateTransactionRoute(ctx context.Context, in *UpdateTransactionRouteRequest) (*UpdateTransactionRouteResponse, error) {
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

	return &UpdateTransactionRouteResponse{Status: http.StatusOK, Body: transactionRoute}, nil
}

// --- DELETE /transaction-routes/{transaction_route_id} ------------------------

// DeleteTransactionRouteResponse has NO Body field: paired with DefaultStatus 204
// it makes Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteTransactionRouteResponse struct{}

// DeleteTransactionRouteByID delegates to deleteTransactionRouteByID; returns a
// bodiless 204 on success.
func (handler *TransactionRouteHandler) DeleteTransactionRouteByID(ctx context.Context, in *GetTransactionRouteRequest) (*DeleteTransactionRouteResponse, error) {
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

	return &DeleteTransactionRouteResponse{}, nil
}
