// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the account-exception resource. It mirrors the
// operation-route exemplar (operation_route_handler.go); see that file's header for the full
// conventions. Account-exception-specific notes:
//
//  1. AUTH is the "midaz" appName, resource "account-exceptions". The per-op Security metadata
//     here is SPEC metadata only; runtime auth stays the Fiber guard chain
//     (auth.Authorize("midaz","account-exceptions",verb) + tenant +
//     ParseUUIDPathParameters("account_exception")) attached BEFORE the Huma terminal.
//  2. POST/PATCH carry RawBody + SkipValidateBody so http.DecodeAndValidate is the sole body
//     validator (never a native Huma 422).
//  3. List is page-based (limit/offset). The raw query is captured via Resolve and fed to the
//     imperative http.ValidateParameters binder.
//  4. Errors go through the shared pkgHTTP.HumaProblem.

// secAccountExceptionBearer advertises that each account-exception operation accepts a JWT
// bearer token (Bearer-only, matching the Fiber guard chain). SPEC metadata only.
var secAccountExceptionBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /accounts/{account_id}/exceptions -----------------------------------

// CreateAccountExceptionRequest is the Huma request envelope for POST. RawBody keeps the body
// out of Huma's validator and feeds the imperative decode in the handler.
type CreateAccountExceptionRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	AccountID      string `path:"account_id" doc:"Account ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateAccountExceptionResponse pins 201 (matching http.Created).
type CreateAccountExceptionResponse struct {
	Status int
	Body   *mmodel.AccountException
}

// CreateAccountException decodes+validates the raw body imperatively then delegates to the
// shared createAccountException core.
func (handler *AccountExceptionHandler) CreateAccountException(ctx context.Context, in *CreateAccountExceptionRequest) (*CreateAccountExceptionResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	accountID, err := parsePathUUID(in.AccountID, "account_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreateAccountExceptionInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	exception, err := handler.createAccountException(ctx, orgID, ledgerID, accountID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateAccountExceptionResponse{Status: http.StatusCreated, Body: exception}, nil
}

// --- GET /accounts/{account_id}/exceptions (list) -----------------------------

// ListAccountExceptionsRequest advertises the page-list query params (doc-only) and captures
// the raw query via Resolve for the imperative binder.
type ListAccountExceptionsRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	AccountID      string `path:"account_id" doc:"Account ID (UUID)"`
	Limit          string `query:"limit" doc:"Max items per page (default 10)"`
	Page           string `query:"page" doc:"Page number, 1-based"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`
	StartDate      string `query:"start_date" doc:"Filter created on/after this date (YYYY-MM-DD)"`
	EndDate        string `query:"end_date" doc:"Filter created on/before this date (YYYY-MM-DD)"`

	rawQuery url.Values
}

// Resolve captures the raw query before the handler (no validation; canonical rejection stays
// in http.ValidateParameters).
func (in *ListAccountExceptionsRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes, matching
// Fiber's c.Queries() (last value wins for a repeated key).
func (in *ListAccountExceptionsRequest) queries() map[string]string {
	return queriesFromValues(in.rawQuery)
}

// ListAccountExceptionsResponse carries the pagination envelope verbatim.
type ListAccountExceptionsResponse struct {
	Status int
	Body   pkgHTTP.Pagination
}

// GetAllAccountExceptions binds the query imperatively then delegates to getAllAccountExceptions.
func (handler *AccountExceptionHandler) GetAllAccountExceptions(ctx context.Context, in *ListAccountExceptionsRequest) (*ListAccountExceptionsResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	accountID, err := parsePathUUID(in.AccountID, "account_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllAccountExceptions(ctx, orgID, ledgerID, accountID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListAccountExceptionsResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /accounts/{account_id}/exceptions/{exception_id} ----------------------

// GetAccountExceptionRequest is the by-id request envelope. exception_id carries no format tag
// (parsePathUUID is the validator, mirroring operation-route's id param).
type GetAccountExceptionRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	AccountID      string `path:"account_id" doc:"Account ID (UUID)"`
	ExceptionID    string `path:"exception_id" doc:"Account Exception ID (UUID)"`
}

// GetAccountExceptionResponse carries the account exception verbatim.
type GetAccountExceptionResponse struct {
	Status int
	Body   *mmodel.AccountException
}

// GetAccountExceptionByID delegates to getAccountExceptionByID.
func (handler *AccountExceptionHandler) GetAccountExceptionByID(ctx context.Context, in *GetAccountExceptionRequest) (*GetAccountExceptionResponse, error) {
	orgID, ledgerID, accountID, id, err := parseAccountExceptionScope(in.OrganizationID, in.LedgerID, in.AccountID, in.ExceptionID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	exception, err := handler.getAccountExceptionByID(ctx, orgID, ledgerID, accountID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetAccountExceptionResponse{Status: http.StatusOK, Body: exception}, nil
}

// --- PATCH /accounts/{account_id}/exceptions/{exception_id} ---------------------

// UpdateAccountExceptionRequest is the update request envelope. RawBody keeps the body out of
// Huma's validator and feeds the imperative decode in the handler.
type UpdateAccountExceptionRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	AccountID      string `path:"account_id" doc:"Account ID (UUID)"`
	ExceptionID    string `path:"exception_id" doc:"Account Exception ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateAccountExceptionResponse carries the updated exception (200, matching http.OK).
type UpdateAccountExceptionResponse struct {
	Status int
	Body   *mmodel.AccountException
}

// UpdateAccountException decodes+validates the raw body imperatively then delegates to the
// shared updateAccountException core.
func (handler *AccountExceptionHandler) UpdateAccountException(ctx context.Context, in *UpdateAccountExceptionRequest) (*UpdateAccountExceptionResponse, error) {
	orgID, ledgerID, accountID, id, err := parseAccountExceptionScope(in.OrganizationID, in.LedgerID, in.AccountID, in.ExceptionID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdateAccountExceptionInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	exception, err := handler.updateAccountException(ctx, orgID, ledgerID, accountID, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateAccountExceptionResponse{Status: http.StatusOK, Body: exception}, nil
}

// --- DELETE /accounts/{account_id}/exceptions/{exception_id} --------------------

// DeleteAccountExceptionResponse has NO Body field: paired with DefaultStatus 204 it makes Huma
// emit a bodiless 204.
type DeleteAccountExceptionResponse struct{}

// DeleteAccountExceptionByID delegates to deleteAccountException; returns a bodiless 204 on
// success.
func (handler *AccountExceptionHandler) DeleteAccountExceptionByID(ctx context.Context, in *GetAccountExceptionRequest) (*DeleteAccountExceptionResponse, error) {
	orgID, ledgerID, accountID, id, err := parseAccountExceptionScope(in.OrganizationID, in.LedgerID, in.AccountID, in.ExceptionID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteAccountException(ctx, orgID, ledgerID, accountID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteAccountExceptionResponse{}, nil
}

// parseAccountExceptionScope resolves the org+ledger+account+exception path strings to UUIDs.
// The four-UUID scope is shared by the three by-id ops, so it is factored here to keep the
// canonical 0065 handling in one place.
func parseAccountExceptionScope(orgStr, ledgerStr, accountStr, exceptionStr string) (orgID, ledgerID, accountID, exceptionID uuid.UUID, err error) {
	orgID, ledgerID, err = parseOrgLedger(orgStr, ledgerStr)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, err
	}

	accountID, err = parsePathUUID(accountStr, "account_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, err
	}

	exceptionID, err = parsePathUUID(exceptionStr, "exception_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, err
	}

	return orgID, ledgerID, accountID, exceptionID, nil
}
