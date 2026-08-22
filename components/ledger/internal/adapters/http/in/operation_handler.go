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

// This file is the ledger's Huma adoption of the operation resource. It mirrors the
// asset exemplar (asset_handler.go) and the balance sibling (balance_handler.go); see
// those headers for the full conventions. Operation-specific notes:
//
//  1. Three ops: GetAllOperationsByAccount (cursor-paginated list under an account),
//     GetOperationByAccount (by-id under an account) and UpdateOperation (PATCH, a
//     money-write LEG of the double-entry, on the transaction path).
//  2. All four path params (org, ledger, account_id, operation_id) are in
//     cn.UUIDPathParameters, so ParseUUIDPathParameters("operation") is the sole
//     UUID validator — the request structs carry them as plain strings with only
//     `doc:` (no format tag => no native Huma 422).
//  3. The list op carries the raw query (via Resolve) and rebuilds the
//     map[string]string that http.ValidateParameters consumes. The
//     metadata-vs-default branch stays in the transport-agnostic core (operation.go).
//  4. Errors go through the shared pkgHTTP.HumaProblem; auth is the Fiber guard chain
//     (auth.Authorize("midaz","operations",verb) + tenant + ParseUUID) attached in the
//     unified server BEFORE the Huma terminal — the per-op Security metadata below is
//     SPEC-ONLY.

// --- GET /accounts/{account_id}/operations (list) -----------------------------

// ListOperationsByAccountRequest advertises the list query params (doc-only) and
// captures the raw query via Resolve for the imperative http.ValidateParameters
// binder. account_id is UUID-validated by ParseUUIDPathParameters (no format tag).
type ListOperationsByAccountRequest struct {
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
func (in *ListOperationsByAccountRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key). Inlined per
// the pattern (the query binder is copied, not a shared helper).
func (in *ListOperationsByAccountRequest) queries() map[string]string {
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

// ListOperationsResponse carries the pagination envelope verbatim.
type ListOperationsResponse struct {
	Status int
	Body   pkgHTTP.Pagination
}

// GetAllOperationsByAccount binds the query imperatively then delegates to the
// shared getAllOperationsByAccount core.
func (handler *OperationHandler) GetAllOperationsByAccount(ctx context.Context, in *ListOperationsByAccountRequest) (*ListOperationsResponse, error) {
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

	return &ListOperationsResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /accounts/{account_id}/operations/{operation_id} ---------------------

// GetOperationByAccountRequest is the by-id request envelope (all path params
// UUID-validated by ParseUUIDPathParameters — no format tags).
type GetOperationByAccountRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	AccountID      string `path:"account_id" doc:"Account ID (UUID)"`
	OperationID    string `path:"operation_id" doc:"Operation ID (UUID)"`
}

// GetOperationResponse carries the operation verbatim.
type GetOperationResponse struct {
	Status int
	Body   *operation.Operation
}

// GetOperationByAccount delegates to the shared getOperationByAccount core.
func (handler *OperationHandler) GetOperationByAccount(ctx context.Context, in *GetOperationByAccountRequest) (*GetOperationResponse, error) {
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

	return &GetOperationResponse{Status: http.StatusOK, Body: op}, nil
}

// --- PATCH /transactions/{transaction_id}/operations/{operation_id} -----------

// UpdateOperationRequest is the PATCH request envelope. The op is a LEG of the
// double-entry (money-write) but a plain metadata/description update, NOT merge-patch:
// the command builds a fresh operation.Operation{Description} and merges Metadata, with
// no null-field derivation via FindNilFields. RawBody keeps the body out of Huma's
// validator (SkipValidateBody); http.DecodeAndValidate runs the canonical decode+Validate
// so malformed/invalid bodies stay canonical 400 (no native Huma 422). The four path
// params are UUID-validated by ParseUUIDPathParameters("operation") — plain-string
// doc-only fields, no format tag.
type UpdateOperationRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	TransactionID  string `path:"transaction_id" doc:"Transaction ID (UUID)"`
	OperationID    string `path:"operation_id" doc:"Operation ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateOperationResponse carries the updated operation verbatim (200).
type UpdateOperationResponse struct {
	Status int
	Body   *operation.Operation
}

// UpdateOperation resolves the path UUIDs, decodes+validates the raw body
// imperatively, then delegates to the shared updateOperation core
// (command.UpdateOperation + query.GetOperationByID).
func (handler *OperationHandler) UpdateOperation(ctx context.Context, in *UpdateOperationRequest) (*UpdateOperationResponse, error) {
	organizationID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionID, err := parsePathUUID(in.TransactionID, "transaction_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	operationID, err := parsePathUUID(in.OperationID, "operation_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(operation.UpdateOperationInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	op, err := handler.updateOperation(ctx, organizationID, ledgerID, transactionID, operationID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateOperationResponse{Status: http.StatusOK, Body: op}, nil
}

// RegisterOperationRoutes registers the two operation read ops plus the PATCH
// (money-write leg) on the shared Huma API. It is the per-file seam the unified
// server calls; the auth (auth.Authorize("midaz","operations",verb)) + tenant +
// ParseUUIDPathParameters("operation") chain for these routes is attached in the unified
// server (Fiber level) BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE (the
// group's PrefixModifier writes the version into each op's op.Path, not into a servers entry).
//
// opSuffix distinguishes the operation IDs one version group publishes from another's — see
// routeOpSuffixV1. The v1 group passes the empty suffix so its IDs stay exactly what published
// SDKs bind to; the v2 group passes "V2" so its twins do not collide in the one document.
func RegisterOperationRoutes(api huma.API, h *OperationHandler, opSuffix string) {
	const (
		listPath  = "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations"
		idPath    = listPath + "/{operation_id}"
		patchPath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/operations/{operation_id}"
		tag       = "Operations"
	)

	huma.Register(api, huma.Operation{
		OperationID: "getAllOperationsByAccount" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all Operations by account",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
	}, h.GetAllOperationsByAccount)

	huma.Register(api, huma.Operation{
		OperationID: "getOperationByAccount" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get Operation",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
	}, h.GetOperationByAccount)

	huma.Register(api, huma.Operation{
		OperationID:      "updateOperation" + opSuffix,
		Method:           http.MethodPatch,
		Path:             patchPath,
		Summary:          "Update an Operation",
		Tags:             []string{tag},
		Security:         secTransactionBearer, // BearerAuth (Bearer-only), matching the Fiber guard chain on the transaction-path PATCH.
		SkipValidateBody: true,                 // body validated imperatively (http.DecodeAndValidate) — plain decode, not merge-patch.
	}, h.UpdateOperation)
	attachTypedRequestBody[operation.UpdateOperationInput](api, "updateOperation"+opSuffix)
}
