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

// This file is the ledger's Huma adoption of the balance resource (money-read +
// routing). It mirrors the asset exemplar (asset_handler.go);
// see that file's header for the full conventions. Balance-specific notes:
//
//  1. balance_id / account_id are UUID-validated by ParseUUIDPathParameters
//     (they are in cn.UUIDPathParameters); alias / code are NOT, so they pass
//     through as raw path strings (no format tag, no native 422) — matching the
//     Fiber handlers that read c.Params("alias") / c.Params("code") directly.
//  2. The two history ops carry `date` as a query param with NO validation tag,
//     so Huma never emits a native 422. The imperative parseBalanceHistoryDate
//     core (balance.go) is its sole validator.
//  3. The three write ops (PATCH update, POST create-additional, DELETE) are
//     MONEY-adjacent: the migration is transport-only; the command use cases are
//     untouched. RawBody + SkipValidateBody keeps http.DecodeAndValidate the sole
//     body validator (never a native Huma 422).
//  4. Errors go through the shared pkgHTTP.HumaProblem; auth stays the Fiber guard
//     chain (auth.Authorize("midaz","balances",verb) + tenant + ParseUUID) attached
//     in the unified server BEFORE the Huma terminal — the per-op Security metadata
//     below is SPEC-ONLY.

// secBalanceBearer is the spec-only Security metadata for the balance operations:
// a JWT bearer token. Runtime auth is the Fiber guard chain.
var secBalanceBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- GET /balances (list) -----------------------------------------------------

// ListBalancesRequest advertises the list query params (doc-only) and captures
// the raw query via Resolve for the imperative http.ValidateParameters binder.
type ListBalancesRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Limit          string `query:"limit" doc:"Max items per page (max 100, default 10)"`
	StartDate      string `query:"start_date" doc:"Filter balances created on/after this date (YYYY-MM-DD)"`
	EndDate        string `query:"end_date" doc:"Filter balances created on/before this date (YYYY-MM-DD)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`
	Cursor         string `query:"cursor" doc:"Opaque cursor token for pagination"`

	rawQuery url.Values
}

// Resolve captures the raw query before the handler (no validation; canonical
// rejection stays in http.ValidateParameters).
func (in *ListBalancesRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key). Inlined per
// the pattern (the query binder is copied, not a shared helper).
func (in *ListBalancesRequest) queries() map[string]string {
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

// ListBalancesResponse carries the pagination envelope verbatim.
type ListBalancesResponse struct {
	Status int
	Body   pkgHTTP.Pagination
}

// GetAllBalances binds the query imperatively then delegates to getAllBalances.
func (handler *BalanceHandler) GetAllBalances(ctx context.Context, in *ListBalancesRequest) (*ListBalancesResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllBalances(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListBalancesResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /accounts/{account_id}/balances (list) -------------------------------

// ListAccountBalancesRequest is the by-account list envelope.
type ListAccountBalancesRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	AccountID      string `path:"account_id" doc:"Account ID (UUID)"`
	Limit          string `query:"limit" doc:"Max items per page (max 100, default 10)"`
	StartDate      string `query:"start_date" doc:"Filter balances created on/after this date (YYYY-MM-DD)"`
	EndDate        string `query:"end_date" doc:"Filter balances created on/before this date (YYYY-MM-DD)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`
	Cursor         string `query:"cursor" doc:"Opaque cursor token for pagination"`

	rawQuery url.Values
}

func (in *ListAccountBalancesRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

func (in *ListAccountBalancesRequest) queries() map[string]string {
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

// GetAllBalancesByAccountID binds the query imperatively then delegates.
func (handler *BalanceHandler) GetAllBalancesByAccountID(ctx context.Context, in *ListAccountBalancesRequest) (*ListBalancesResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	accountID, err := parsePathUUID(in.AccountID, "account_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllBalancesByAccountID(ctx, orgID, ledgerID, accountID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListBalancesResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /balances/{balance_id} -----------------------------------------------

// GetBalanceRequest is the by-id request envelope.
type GetBalanceRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	BalanceID      string `path:"balance_id" doc:"Balance ID (UUID)"`
}

// GetBalanceResponse carries the balance verbatim.
type GetBalanceResponse struct {
	Status int
	Body   *mmodel.Balance
}

// GetBalanceByID delegates to getBalanceByID.
func (handler *BalanceHandler) GetBalanceByID(ctx context.Context, in *GetBalanceRequest) (*GetBalanceResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	balanceID, err := parsePathUUID(in.BalanceID, "balance_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	balance, err := handler.getBalanceByID(ctx, orgID, ledgerID, balanceID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetBalanceResponse{Status: http.StatusOK, Body: balance}, nil
}

// --- PATCH /balances/{balance_id} (MONEY-adjacent) ----------------------------

// UpdateBalanceRequest is the update envelope (RawBody, see asset Create).
type UpdateBalanceRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	BalanceID      string `path:"balance_id" doc:"Balance ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateBalanceResponse carries the updated balance (200).
type UpdateBalanceResponse struct {
	Status int
	Body   *mmodel.Balance
}

// UpdateBalance decodes+validates the raw body imperatively then delegates to
// the shared updateBalance core (command use case untouched).
func (handler *BalanceHandler) UpdateBalance(ctx context.Context, in *UpdateBalanceRequest) (*UpdateBalanceResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	balanceID, err := parsePathUUID(in.BalanceID, "balance_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdateBalance)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	balance, err := handler.updateBalance(ctx, orgID, ledgerID, balanceID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateBalanceResponse{Status: http.StatusOK, Body: balance}, nil
}

// --- DELETE /balances/{balance_id} (MONEY-adjacent) ---------------------------

// DeleteBalanceResponse has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteBalanceResponse struct{}

// DeleteBalanceByID delegates to deleteBalance; returns a bodiless 204.
func (handler *BalanceHandler) DeleteBalanceByID(ctx context.Context, in *GetBalanceRequest) (*DeleteBalanceResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	balanceID, err := parsePathUUID(in.BalanceID, "balance_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteBalance(ctx, orgID, ledgerID, balanceID); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteBalanceResponse{}, nil
}

// --- POST /accounts/{account_id}/balances (MONEY-adjacent) --------------------

// CreateAdditionalBalanceRequest is the create-additional envelope (RawBody).
type CreateAdditionalBalanceRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	AccountID      string `path:"account_id" doc:"Account ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateAdditionalBalanceResponse pins 201.
type CreateAdditionalBalanceResponse struct {
	Status int
	Body   *mmodel.Balance
}

// CreateAdditionalBalance decodes+validates imperatively then delegates to the
// shared createAdditionalBalance core (command use case untouched).
func (handler *BalanceHandler) CreateAdditionalBalance(ctx context.Context, in *CreateAdditionalBalanceRequest) (*CreateAdditionalBalanceResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	accountID, err := parsePathUUID(in.AccountID, "account_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreateAdditionalBalance)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	balance, err := handler.createAdditionalBalance(ctx, orgID, ledgerID, accountID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateAdditionalBalanceResponse{Status: http.StatusCreated, Body: balance}, nil
}

// --- GET /accounts/alias/{alias}/balances -------------------------------------

// GetBalancesByAliasRequest carries the raw alias path string (no UUID parse).
type GetBalancesByAliasRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Alias          string `path:"alias" doc:"Alias (e.g. @person1)"`
}

// GetBalancesByAlias delegates to getBalancesByAlias.
func (handler *BalanceHandler) GetBalancesByAlias(ctx context.Context, in *GetBalancesByAliasRequest) (*ListBalancesResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getBalancesByAlias(ctx, orgID, ledgerID, in.Alias)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListBalancesResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /accounts/external/{code}/balances -----------------------------------

// GetBalancesExternalByCodeRequest carries the raw code path string (no UUID parse).
type GetBalancesExternalByCodeRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Code           string `path:"code" doc:"Code (e.g. BRL)"`
}

// GetBalancesExternalByCode delegates to getBalancesExternalByCode.
func (handler *BalanceHandler) GetBalancesExternalByCode(ctx context.Context, in *GetBalancesExternalByCodeRequest) (*ListBalancesResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getBalancesExternalByCode(ctx, orgID, ledgerID, in.Code)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListBalancesResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /balances/{balance_id}/history ---------------------------------------

// GetBalanceHistoryRequest carries the date query param with NO validation tag
// (the imperative core is the sole date validator — see file header).
type GetBalanceHistoryRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	BalanceID      string `path:"balance_id" doc:"Balance ID (UUID)"`
	Date           string `query:"date" doc:"Point in time (format: yyyy-mm-dd hh:mm:ss)"`
}

// GetBalanceHistoryResponse carries the balance history snapshot.
type GetBalanceHistoryResponse struct {
	Status int
	Body   *mmodel.BalanceHistory
}

// GetBalanceAtTimestamp validates the date imperatively (in the core) then
// delegates to getBalanceAtTimestamp.
func (handler *BalanceHandler) GetBalanceAtTimestamp(ctx context.Context, in *GetBalanceHistoryRequest) (*GetBalanceHistoryResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	balanceID, err := parsePathUUID(in.BalanceID, "balance_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	history, err := handler.getBalanceAtTimestamp(ctx, orgID, ledgerID, balanceID, in.Date)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetBalanceHistoryResponse{Status: http.StatusOK, Body: history}, nil
}

// --- GET /accounts/{account_id}/balances/history ------------------------------

// GetAccountBalanceHistoryRequest carries the date query param (no validation tag).
type GetAccountBalanceHistoryRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	AccountID      string `path:"account_id" doc:"Account ID (UUID)"`
	Date           string `query:"date" doc:"Point in time (format: yyyy-mm-dd hh:mm:ss)"`
}

// GetAccountBalanceHistoryResponse carries the list of history snapshots.
type GetAccountBalanceHistoryResponse struct {
	Status int
	Body   []*mmodel.BalanceHistory
}

// GetAccountBalancesAtTimestamp validates the date imperatively (in the core)
// then delegates to getAccountBalancesAtTimestamp.
func (handler *BalanceHandler) GetAccountBalancesAtTimestamp(ctx context.Context, in *GetAccountBalanceHistoryRequest) (*GetAccountBalanceHistoryResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	accountID, err := parsePathUUID(in.AccountID, "account_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	history, err := handler.getAccountBalancesAtTimestamp(ctx, orgID, ledgerID, accountID, in.Date)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetAccountBalanceHistoryResponse{Status: http.StatusOK, Body: history}, nil
}

// RegisterBalanceRoutes registers the ten balance operations on the
// shared Huma API. The auth
// (auth.Authorize("midaz","balances",verb)) + tenant + ParseUUIDPathParameters
// ("balance") chain for these routes is attached in the unified server (Fiber
// level) BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE (the Huma API
// is bound to a versioned Fiber group; the group's PrefixModifier writes the version
// into each op's op.Path, not into a servers entry).
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. A straight v1/v2 mirror reuses the same handler methods and the
// same input/output types, so only the operation IDs differ between the twins.
func RegisterBalanceRoutes(api huma.API, h *BalanceHandler, opSuffix string) {
	const (
		orgLedger      = "/organizations/{organization_id}/ledgers/{ledger_id}"
		balancesPath   = orgLedger + "/balances"
		balanceIDPath  = balancesPath + "/{balance_id}"
		balanceHistory = balanceIDPath + "/history"
		acctBalances   = orgLedger + "/accounts/{account_id}/balances"
		acctHistory    = acctBalances + "/history"
		aliasBalances  = orgLedger + "/accounts/alias/{alias}/balances"
		codeBalances   = orgLedger + "/accounts/external/{code}/balances"
		tag            = "Balances"
	)

	huma.Register(api, huma.Operation{
		OperationID: "getAllBalances" + opSuffix,
		Method:      http.MethodGet,
		Path:        balancesPath,
		Summary:     "Get all balances",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
	}, h.GetAllBalances)

	huma.Register(api, huma.Operation{
		OperationID: "getBalanceByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        balanceIDPath,
		Summary:     "Get Balance by id",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
	}, h.GetBalanceByID)

	huma.Register(api, huma.Operation{
		OperationID: "getBalanceAtTimestamp" + opSuffix,
		Method:      http.MethodGet,
		Path:        balanceHistory,
		Summary:     "Get Balance history at date",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
	}, h.GetBalanceAtTimestamp)

	huma.Register(api, huma.Operation{
		OperationID: "getAllBalancesByAccountID" + opSuffix,
		Method:      http.MethodGet,
		Path:        acctBalances,
		Summary:     "Get all balances by account id",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
	}, h.GetAllBalancesByAccountID)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountBalancesAtTimestamp" + opSuffix,
		Method:      http.MethodGet,
		Path:        acctHistory,
		Summary:     "Get Account Balances history at date",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
	}, h.GetAccountBalancesAtTimestamp)

	huma.Register(api, huma.Operation{
		OperationID: "getBalancesByAlias" + opSuffix,
		Method:      http.MethodGet,
		Path:        aliasBalances,
		Summary:     "Get Balances using Alias",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
	}, h.GetBalancesByAlias)

	huma.Register(api, huma.Operation{
		OperationID: "getBalancesExternalByCode" + opSuffix,
		Method:      http.MethodGet,
		Path:        codeBalances,
		Summary:     "Get External balances using code",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
	}, h.GetBalancesExternalByCode)

	huma.Register(api, huma.Operation{
		OperationID:      "updateBalance" + opSuffix,
		Method:           http.MethodPatch,
		Path:             balanceIDPath,
		Summary:          "Update Balance",
		Tags:             []string{tag},
		Security:         secBalanceBearer,
		SkipValidateBody: true, // body validated imperatively — see file header.
	}, h.UpdateBalance)
	attachTypedRequestBody[mmodel.UpdateBalance](api, "updateBalance"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID:      "createAdditionalBalance" + opSuffix,
		Method:           http.MethodPost,
		Path:             acctBalances,
		Summary:          "Create Additional Balance",
		Tags:             []string{tag},
		Security:         secBalanceBearer,
		SkipValidateBody: true, // body validated imperatively — see file header.
		DefaultStatus:    http.StatusCreated,
	}, h.CreateAdditionalBalance)
	attachTypedRequestBody[mmodel.CreateAdditionalBalance](api, "createAdditionalBalance"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "deleteBalance" + opSuffix,
		Method:      http.MethodDelete,
		Path:        balanceIDPath,
		Summary:     "Delete Balance by account",
		Tags:        []string{tag},
		Security:    secBalanceBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteBalanceByID)
}
