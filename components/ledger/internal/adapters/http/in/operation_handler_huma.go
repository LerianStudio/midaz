// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the operation resource (Wave 2,
// money-read + routing). It mirrors the asset exemplar (asset_handler_huma.go)
// and the balance sibling (balance_handler_huma.go); see those headers for the
// full conventions. Operation-specific notes:
//
//  1. Only the two READ ops migrate here: GetAllOperationsByAccount (cursor-
//     paginated list under an account) and GetOperationByAccount (by-id under an
//     account). UpdateOperation (PATCH, MONEY-adjacent write on the transaction
//     path) is NOT migrated in this wave — its Fiber wrapper is unchanged.
//  2. All four path params (org, ledger, account_id, operation_id) are in
//     cn.UUIDPathParameters, so ParseUUIDPathParameters("operation") is the sole
//     UUID validator across both transports — the In structs carry them as plain
//     strings with only `doc:` (no format tag => no native Huma 422).
//  3. The list op carries the raw query (via Resolve) and rebuilds the
//     map[string]string that http.ValidateParameters consumes, byte-identical to
//     the Fiber c.Queries() path. The metadata-vs-default branch stays in the
//     transport-agnostic core (see operation.go).
//  4. Errors go through the shared pkgHTTP.HumaProblem; auth stays the Fiber guard
//     chain (auth.Authorize("midaz","operations","get") + tenant + ParseUUID)
//     attached in the unified server BEFORE the Huma terminal — the per-op Security
//     metadata below is SPEC-ONLY.

// --- GET /accounts/{account_id}/operations (list) -----------------------------

// ListOperationsByAccountInputHuma advertises the list query params (doc-only) and
// captures the raw query via Resolve for the imperative http.ValidateParameters
// binder. account_id is UUID-validated by ParseUUIDPathParameters (no format tag).
type ListOperationsByAccountInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	AccountID      string `path:"account_id" doc:"Account ID (UUID)"`
	Metadata       string `query:"metadata" doc:"JSON string to filter operations by metadata fields"`
	Limit          string `query:"limit" doc:"Max items per page (max 100, default 10)"`
	StartDate      string `query:"start_date" doc:"Filter operations created on/after this date (YYYY-MM-DD)"`
	EndDate        string `query:"end_date" doc:"Filter operations created on/before this date (YYYY-MM-DD)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`
	Cursor         string `query:"cursor" doc:"Opaque cursor token for pagination"`
	Type           string `query:"type" doc:"Filter by operation type (DEBIT, CREDIT)"`
	Direction      string `query:"direction" doc:"Filter by direction (debit, credit)"`
	RouteID        string `query:"route_id" doc:"Filter by operation route ID (UUID)"`
	RouteCode      string `query:"route_code" doc:"Filter by operation route code"`

	rawQuery url.Values
}

// Resolve captures the raw query before the handler (no validation; canonical
// rejection stays in http.ValidateParameters).
func (in *ListOperationsByAccountInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key). Inlined per
// the pattern (the query binder is copied, not a shared helper).
func (in *ListOperationsByAccountInputHuma) queries() map[string]string {
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

// ListOperationsOutputHuma carries the pagination envelope verbatim.
type ListOperationsOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// GetAllOperationsByAccountHuma binds the query imperatively then delegates to the
// shared getAllOperationsByAccount core.
func (handler *OperationHandler) GetAllOperationsByAccountHuma(ctx context.Context, in *ListOperationsByAccountInputHuma) (*ListOperationsOutputHuma, error) {
	organizationID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	accountID, err := parsePathUUID(in.AccountID, "account_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllOperationsByAccount(ctx, organizationID, ledgerID, accountID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListOperationsOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /accounts/{account_id}/operations/{operation_id} ---------------------

// GetOperationByAccountInputHuma is the by-id request envelope (all path params
// UUID-validated by ParseUUIDPathParameters — no format tags).
type GetOperationByAccountInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	AccountID      string `path:"account_id" doc:"Account ID (UUID)"`
	OperationID    string `path:"operation_id" doc:"Operation ID (UUID)"`
}

// GetOperationOutputHuma carries the operation verbatim.
type GetOperationOutputHuma struct {
	Status int
	Body   *operation.Operation
}

// GetOperationByAccountHuma delegates to the shared getOperationByAccount core.
func (handler *OperationHandler) GetOperationByAccountHuma(ctx context.Context, in *GetOperationByAccountInputHuma) (*GetOperationOutputHuma, error) {
	organizationID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	accountID, err := parsePathUUID(in.AccountID, "account_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	operationID, err := parsePathUUID(in.OperationID, "operation_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	op, err := handler.getOperationByAccount(ctx, organizationID, ledgerID, accountID, operationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetOperationOutputHuma{Status: http.StatusOK, Body: op}, nil
}

// RegisterOperationRoutesToApp registers the two migrated operation read ops on the
// shared Huma API. It is the per-file seam the unified server calls; the auth
// (auth.Authorize("midaz","operations","get")) + tenant + ParseUUIDPathParameters
// ("operation") chain for these routes is attached in the unified server (Fiber
// level) BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE (the /v1
// prefix rides the OpenAPI servers entry).
func RegisterOperationRoutesToApp(api huma.API, h *OperationHandler) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations"
		idPath   = listPath + "/{operation_id}"
		tag      = "Operations"
	)

	huma.Register(api, huma.Operation{
		OperationID: "getAllOperationsByAccount",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all Operations by account",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
	}, h.GetAllOperationsByAccountHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getOperationByAccount",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get Operation",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
	}, h.GetOperationByAccountHuma)
}
