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

// This file is the ledger's Huma adoption of the balance resource (Wave 2,
// money-read + routing). It mirrors the asset exemplar (asset_handler_huma.go);
// see that file's header for the full conventions. Balance-specific notes:
//
//  1. balance_id / account_id are UUID-validated by ParseUUIDPathParameters
//     (they are in cn.UUIDPathParameters); alias / code are NOT, so they pass
//     through as raw path strings (no format tag, no native 422) — matching the
//     Fiber handlers that read c.Params("alias") / c.Params("code") directly.
//  2. The two history ops carry `date` as a query param with NO validation tag,
//     so Huma never emits a native 422. The imperative parseBalanceHistoryDate
//     core (balance.go) is the sole date validator across both transports.
//  3. The three write ops (PATCH update, POST create-additional, DELETE) are
//     MONEY-adjacent: the migration is transport-only; the command use cases are
//     untouched. RawBody + SkipValidateBody keeps http.DecodeAndValidate the sole
//     body validator (never a native Huma 422).
//  4. Errors go through the shared pkgHTTP.HumaProblem; auth stays the Fiber guard
//     chain (auth.Authorize("midaz","balances",verb) + tenant + ParseUUID) attached
//     in the unified server BEFORE the Huma terminal — the per-op Security metadata
//     below is SPEC-ONLY.

// --- GET /balances (list) -----------------------------------------------------

// ListBalancesInputHuma advertises the list query params (doc-only) and captures
// the raw query via Resolve for the imperative http.ValidateParameters binder.
type ListBalancesInputHuma struct {
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
func (in *ListBalancesInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key). Inlined per
// the pattern (the query binder is copied, not a shared helper).
func (in *ListBalancesInputHuma) queries() map[string]string {
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

// ListBalancesOutputHuma carries the pagination envelope verbatim.
type ListBalancesOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// GetAllBalancesHuma binds the query imperatively then delegates to getAllBalances.
func (handler *BalanceHandler) GetAllBalancesHuma(ctx context.Context, in *ListBalancesInputHuma) (*ListBalancesOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllBalances(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListBalancesOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /accounts/{account_id}/balances (list) -------------------------------

// ListAccountBalancesInputHuma is the by-account list envelope.
type ListAccountBalancesInputHuma struct {
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

func (in *ListAccountBalancesInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

func (in *ListAccountBalancesInputHuma) queries() map[string]string {
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

// GetAllBalancesByAccountIDHuma binds the query imperatively then delegates.
func (handler *BalanceHandler) GetAllBalancesByAccountIDHuma(ctx context.Context, in *ListAccountBalancesInputHuma) (*ListBalancesOutputHuma, error) {
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

	return &ListBalancesOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /balances/{balance_id} -----------------------------------------------

// GetBalanceInputHuma is the by-id request envelope.
type GetBalanceInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	BalanceID      string `path:"balance_id" doc:"Balance ID (UUID)"`
}

// GetBalanceOutputHuma carries the balance verbatim.
type GetBalanceOutputHuma struct {
	Status int
	Body   *mmodel.Balance
}

// GetBalanceByIDHuma delegates to getBalanceByID.
func (handler *BalanceHandler) GetBalanceByIDHuma(ctx context.Context, in *GetBalanceInputHuma) (*GetBalanceOutputHuma, error) {
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

	return &GetBalanceOutputHuma{Status: http.StatusOK, Body: balance}, nil
}

// --- PATCH /balances/{balance_id} (MONEY-adjacent) ----------------------------

// UpdateBalanceInputHuma is the update envelope (RawBody, see asset Create).
type UpdateBalanceInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	BalanceID      string `path:"balance_id" doc:"Balance ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateBalanceOutputHuma carries the updated balance (200).
type UpdateBalanceOutputHuma struct {
	Status int
	Body   *mmodel.Balance
}

// UpdateBalanceHuma decodes+validates the raw body imperatively then delegates to
// the shared updateBalance core (command use case untouched).
func (handler *BalanceHandler) UpdateBalanceHuma(ctx context.Context, in *UpdateBalanceInputHuma) (*UpdateBalanceOutputHuma, error) {
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

	return &UpdateBalanceOutputHuma{Status: http.StatusOK, Body: balance}, nil
}

// --- DELETE /balances/{balance_id} (MONEY-adjacent) ---------------------------

// DeleteBalanceOutputHuma has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteBalanceOutputHuma struct{}

// DeleteBalanceByIDHuma delegates to deleteBalance; returns a bodiless 204.
func (handler *BalanceHandler) DeleteBalanceByIDHuma(ctx context.Context, in *GetBalanceInputHuma) (*DeleteBalanceOutputHuma, error) {
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

	return &DeleteBalanceOutputHuma{}, nil
}

// --- POST /accounts/{account_id}/balances (MONEY-adjacent) --------------------

// CreateAdditionalBalanceInputHuma is the create-additional envelope (RawBody).
type CreateAdditionalBalanceInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	AccountID      string `path:"account_id" doc:"Account ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateAdditionalBalanceOutputHuma pins 201.
type CreateAdditionalBalanceOutputHuma struct {
	Status int
	Body   *mmodel.Balance
}

// CreateAdditionalBalanceHuma decodes+validates imperatively then delegates to the
// shared createAdditionalBalance core (command use case untouched).
func (handler *BalanceHandler) CreateAdditionalBalanceHuma(ctx context.Context, in *CreateAdditionalBalanceInputHuma) (*CreateAdditionalBalanceOutputHuma, error) {
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

	return &CreateAdditionalBalanceOutputHuma{Status: http.StatusCreated, Body: balance}, nil
}

// --- GET /accounts/alias/{alias}/balances -------------------------------------

// GetBalancesByAliasInputHuma carries the raw alias path string (no UUID parse).
type GetBalancesByAliasInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Alias          string `path:"alias" doc:"Alias (e.g. @person1)"`
}

// GetBalancesByAliasHuma delegates to getBalancesByAlias.
func (handler *BalanceHandler) GetBalancesByAliasHuma(ctx context.Context, in *GetBalancesByAliasInputHuma) (*ListBalancesOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getBalancesByAlias(ctx, orgID, ledgerID, in.Alias)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListBalancesOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /accounts/external/{code}/balances -----------------------------------

// GetBalancesExternalByCodeInputHuma carries the raw code path string (no UUID parse).
type GetBalancesExternalByCodeInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Code           string `path:"code" doc:"Code (e.g. BRL)"`
}

// GetBalancesExternalByCodeHuma delegates to getBalancesExternalByCode.
func (handler *BalanceHandler) GetBalancesExternalByCodeHuma(ctx context.Context, in *GetBalancesExternalByCodeInputHuma) (*ListBalancesOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getBalancesExternalByCode(ctx, orgID, ledgerID, in.Code)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListBalancesOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /balances/{balance_id}/history ---------------------------------------

// GetBalanceHistoryInputHuma carries the date query param with NO validation tag
// (the imperative core is the sole date validator — see file header).
type GetBalanceHistoryInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	BalanceID      string `path:"balance_id" doc:"Balance ID (UUID)"`
	Date           string `query:"date" doc:"Point in time (format: yyyy-mm-dd hh:mm:ss)"`
}

// GetBalanceHistoryOutputHuma carries the balance history snapshot.
type GetBalanceHistoryOutputHuma struct {
	Status int
	Body   *mmodel.BalanceHistory
}

// GetBalanceAtTimestampHuma validates the date imperatively (in the core) then
// delegates to getBalanceAtTimestamp.
func (handler *BalanceHandler) GetBalanceAtTimestampHuma(ctx context.Context, in *GetBalanceHistoryInputHuma) (*GetBalanceHistoryOutputHuma, error) {
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

	return &GetBalanceHistoryOutputHuma{Status: http.StatusOK, Body: history}, nil
}

// --- GET /accounts/{account_id}/balances/history ------------------------------

// GetAccountBalanceHistoryInputHuma carries the date query param (no validation tag).
type GetAccountBalanceHistoryInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	AccountID      string `path:"account_id" doc:"Account ID (UUID)"`
	Date           string `query:"date" doc:"Point in time (format: yyyy-mm-dd hh:mm:ss)"`
}

// GetAccountBalanceHistoryOutputHuma carries the list of history snapshots.
type GetAccountBalanceHistoryOutputHuma struct {
	Status int
	Body   []*mmodel.BalanceHistory
}

// GetAccountBalancesAtTimestampHuma validates the date imperatively (in the core)
// then delegates to getAccountBalancesAtTimestamp.
func (handler *BalanceHandler) GetAccountBalancesAtTimestampHuma(ctx context.Context, in *GetAccountBalanceHistoryInputHuma) (*GetAccountBalanceHistoryOutputHuma, error) {
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

	return &GetAccountBalanceHistoryOutputHuma{Status: http.StatusOK, Body: history}, nil
}

// RegisterBalanceRoutesToApp registers the ten migrated balance operations on the
// shared Huma API. It is the per-file seam the unified server calls; the auth
// (auth.Authorize("midaz","balances",verb)) + tenant + ParseUUIDPathParameters
// ("balance") chain for these routes is attached in the unified server (Fiber
// level) BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE (the /v1
// prefix rides the OpenAPI servers entry).
func RegisterBalanceRoutesToApp(api huma.API, h *BalanceHandler) {
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
		OperationID: "getAllBalances",
		Method:      http.MethodGet,
		Path:        balancesPath,
		Summary:     "Get all balances",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
	}, h.GetAllBalancesHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getBalanceByID",
		Method:      http.MethodGet,
		Path:        balanceIDPath,
		Summary:     "Get Balance by id",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
	}, h.GetBalanceByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getBalanceAtTimestamp",
		Method:      http.MethodGet,
		Path:        balanceHistory,
		Summary:     "Get Balance history at date",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
	}, h.GetBalanceAtTimestampHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getAllBalancesByAccountID",
		Method:      http.MethodGet,
		Path:        acctBalances,
		Summary:     "Get all balances by account id",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
	}, h.GetAllBalancesByAccountIDHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountBalancesAtTimestamp",
		Method:      http.MethodGet,
		Path:        acctHistory,
		Summary:     "Get Account Balances history at date",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
	}, h.GetAccountBalancesAtTimestampHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getBalancesByAlias",
		Method:      http.MethodGet,
		Path:        aliasBalances,
		Summary:     "Get Balances using Alias",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
	}, h.GetBalancesByAliasHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getBalancesExternalByCode",
		Method:      http.MethodGet,
		Path:        codeBalances,
		Summary:     "Get External balances using code",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
	}, h.GetBalancesExternalByCodeHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateBalance",
		Method:           http.MethodPatch,
		Path:             balanceIDPath,
		Summary:          "Update Balance",
		Tags:             []string{tag},
		Security:         secAssetBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively — see file header.
	}, h.UpdateBalanceHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "createAdditionalBalance",
		Method:           http.MethodPost,
		Path:             acctBalances,
		Summary:          "Create Additional Balance",
		Tags:             []string{tag},
		Security:         secAssetBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively — see file header.
	}, h.CreateAdditionalBalanceHuma)

	huma.Register(api, huma.Operation{
		OperationID: "deleteBalance",
		Method:      http.MethodDelete,
		Path:        balanceIDPath,
		Summary:     "Delete Balance by account",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteBalanceByIDHuma)
}
