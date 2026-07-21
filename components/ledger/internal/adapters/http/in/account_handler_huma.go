// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the account resource, following the
// asset exemplar (asset_handler_huma.go) verbatim: shared parseOrgLedger /
// parsePathUUID / HumaProblem / DecodeAndValidate helpers, path params as plain
// strings (ParseUUIDPathParameters is the sole UUID validator — no format tag), raw
// body bytes decoded imperatively (no native Huma 422), and the query bound via the
// same ValidateParameters path. Auth stays the Fiber middleware chain attached in
// RegisterAccountRoutesToApp; the per-op Security metadata is SPEC-ONLY.
//
// Account differs from the asset exemplar in two ways, both absorbed by the cores:
//  1. TWO extra by-key reads — GET .../accounts/alias/{alias} and
//     GET .../accounts/external/{code} — whose path params are NOT UUIDs.
//     ParseUUIDPathParameters only UUID-parses params in cn.UUIDPathParameters
//     ("id","organization_id","ledger_id",...); "alias" and "code" fall through as
//     raw string locals, so no format tag is needed and no native 422 can fire.
//  2. The RecordAccountCreated metric and the status-enum / portfolio+segment filter
//     logic live in the shared cores (account.go), so both transports match.

// secAccountBearerOrAPIKey advertises that each account operation accepts EITHER a
// JWT bearer token OR an X-API-Key (SPEC metadata only; runtime auth is the Fiber
// guard chain). Scheme names are declared once on the shared Huma API.
var secAccountBearerOrAPIKey = []map[string][]string{
	{"BearerAuth": {}},
	{"ApiKeyAuth": {}},
}

// --- POST /accounts -----------------------------------------------------------

// CreateAccountInputHuma is the Huma request envelope for POST. RawBody keeps the
// body out of Huma's validator; the org+ledger path params are validated by the
// Fiber middleware, not by a format tag.
type CreateAccountInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Authorization  string `header:"Authorization" doc:"Bearer token (forwarded to the service)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateAccountOutputHuma pins 201 (matching http.Created).
type CreateAccountOutputHuma struct {
	Status int
	Body   *mmodel.Account
}

// CreateAccountHuma decodes+validates the raw body imperatively then delegates to
// the shared createAccount core.
func (handler *AccountHandler) CreateAccountHuma(ctx context.Context, in *CreateAccountInputHuma) (*CreateAccountOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreateAccountInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	account, err := handler.createAccount(ctx, orgID, ledgerID, payload, in.Authorization)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateAccountOutputHuma{Status: http.StatusCreated, Body: account}, nil
}

// --- GET /accounts (list) -----------------------------------------------------

// ListAccountsInputHuma advertises the list query params in the spec (doc-only, no
// validation tags) and captures the raw query via Resolve for the imperative
// http.ValidateParameters binder.
type ListAccountsInputHuma struct {
	OrganizationID  string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID        string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Metadata        string `query:"metadata" doc:"JSON string to filter accounts by metadata fields"`
	Limit           string `query:"limit" doc:"Max items per page (1-100, default 10)"`
	Page            string `query:"page" doc:"Page number (default 1)"`
	StartDate       string `query:"start_date" doc:"Filter accounts created on/after this date (YYYY-MM-DD)"`
	EndDate         string `query:"end_date" doc:"Filter accounts created on/before this date (YYYY-MM-DD)"`
	SortOrder       string `query:"sort_order" doc:"Sort direction (asc, desc)"`
	PortfolioID     string `query:"portfolio_id" doc:"Filter accounts by portfolio ID (UUID)"`
	SegmentID       string `query:"segment_id" doc:"Filter accounts by segment ID (UUID)"`
	Status          string `query:"status" doc:"Filter accounts by status (ACTIVE, INACTIVE, BLOCKED)"`
	Type            string `query:"type" doc:"Filter accounts by type (e.g., deposit, savings, external)"`
	AssetCode       string `query:"asset_code" doc:"Filter accounts by asset code (e.g., USD, BRL, EUR)"`
	EntityID        string `query:"entity_id" doc:"Filter accounts by entity ID"`
	Blocked         string `query:"blocked" doc:"Filter accounts by blocked status (true, false)"`
	ParentAccountID string `query:"parent_account_id" doc:"Filter accounts by parent account ID (UUID)"`
	Name            string `query:"name" doc:"Filter accounts by name (case-insensitive, prefix match)"`
	Alias           string `query:"alias" doc:"Filter accounts by alias (case-insensitive, prefix match)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in http.ValidateParameters.
func (in *ListAccountsInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key, present-but-
// empty keys included).
func (in *ListAccountsInputHuma) queries() map[string]string {
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

// ListAccountsOutputHuma carries the pagination envelope verbatim.
type ListAccountsOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListAccountsHuma binds the query imperatively then delegates to getAllAccounts.
func (handler *AccountHandler) ListAccountsHuma(ctx context.Context, in *ListAccountsInputHuma) (*ListAccountsOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllAccounts(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListAccountsOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /accounts/{id} -------------------------------------------------------

// GetAccountInputHuma is the by-id request envelope. The id path param carries no
// format tag (ParseUUIDPathParameters is the sole validator).
type GetAccountInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Account ID (UUID)"`
}

// GetAccountOutputHuma carries the account verbatim.
type GetAccountOutputHuma struct {
	Status int
	Body   *mmodel.Account
}

// GetAccountByIDHuma delegates to getAccountByID.
func (handler *AccountHandler) GetAccountByIDHuma(ctx context.Context, in *GetAccountInputHuma) (*GetAccountOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	account, err := handler.getAccountByID(ctx, orgID, ledgerID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetAccountOutputHuma{Status: http.StatusOK, Body: account}, nil
}

// --- GET /accounts/alias/{alias} ----------------------------------------------

// GetAccountByAliasInputHuma is the by-alias request envelope. alias is NOT a UUID;
// it rides through ParseUUIDPathParameters as a raw string local, so it carries no
// format tag and can never trigger a native Huma 422.
type GetAccountByAliasInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Alias          string `path:"alias" doc:"Account alias (e.g. @person1)"`
}

// GetAccountByAliasHuma delegates to the shared getAccountByAlias core.
func (handler *AccountHandler) GetAccountByAliasHuma(ctx context.Context, in *GetAccountByAliasInputHuma) (*GetAccountOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	account, err := handler.getAccountByAlias(ctx, "handler.get_account_by_alias", orgID, ledgerID, in.Alias)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetAccountOutputHuma{Status: http.StatusOK, Body: account}, nil
}

// --- GET /accounts/external/{code} --------------------------------------------

// GetAccountExternalByCodeInputHuma is the external-by-code request envelope. code
// is NOT a UUID (see the alias envelope note).
type GetAccountExternalByCodeInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Code           string `path:"code" doc:"Account external code (e.g. BRL)"`
}

// GetAccountExternalByCodeHuma resolves the external alias then delegates to the
// shared getAccountByAlias core, mirroring the Fiber wrapper.
func (handler *AccountHandler) GetAccountExternalByCodeHuma(ctx context.Context, in *GetAccountExternalByCodeInputHuma) (*GetAccountOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	alias := constant.DefaultExternalAccountAliasPrefix + in.Code

	account, err := handler.getAccountByAlias(ctx, "handler.get_account_external_by_code", orgID, ledgerID, alias)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetAccountOutputHuma{Status: http.StatusOK, Body: account}, nil
}

// --- PATCH /accounts/{id} -----------------------------------------------------

// UpdateAccountInputHuma is the update request envelope (RawBody, see Create).
type UpdateAccountInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Account ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateAccountOutputHuma carries the updated account (200, matching http.OK).
type UpdateAccountOutputHuma struct {
	Status int
	Body   *mmodel.Account
}

// UpdateAccountHuma decodes+validates the raw body imperatively then delegates to
// the shared updateAccount core.
func (handler *AccountHandler) UpdateAccountHuma(ctx context.Context, in *UpdateAccountInputHuma) (*UpdateAccountOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdateAccountInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	account, err := handler.updateAccount(ctx, orgID, ledgerID, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateAccountOutputHuma{Status: http.StatusOK, Body: account}, nil
}

// --- DELETE /accounts/{id} ----------------------------------------------------

// DeleteAccountInputHuma is the delete request envelope. Authorization is forwarded
// to the service (the Fiber path passes c.Get("Authorization")).
type DeleteAccountInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Account ID (UUID)"`
	Authorization  string `header:"Authorization" doc:"Bearer token (forwarded to the service)"`
}

// DeleteAccountOutputHuma has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteAccountOutputHuma struct{}

// DeleteAccountByIDHuma delegates to deleteAccount; returns a bodiless 204.
func (handler *AccountHandler) DeleteAccountByIDHuma(ctx context.Context, in *DeleteAccountInputHuma) (*DeleteAccountOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteAccount(ctx, orgID, ledgerID, id, in.Authorization); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteAccountOutputHuma{}, nil
}

// --- HEAD /accounts/metrics/count ---------------------------------------------

// CountAccountsInputHuma is the HEAD-count request envelope (org+ledger only).
type CountAccountsInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
}

// CountAccountsOutputHuma replicates the Fiber HEAD-count response manually: the
// X-Total-Count header carries the count, Content-Length is pinned to 0, and the
// body is empty at status 204.
type CountAccountsOutputHuma struct {
	TotalCount    string `header:"X-Total-Count"`
	ContentLength string `header:"Content-Length"`
}

// CountAccountsHuma delegates to countAccounts and sets the count headers.
func (handler *AccountHandler) CountAccountsHuma(ctx context.Context, in *CountAccountsInputHuma) (*CountAccountsOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	count, err := handler.countAccounts(ctx, orgID, ledgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CountAccountsOutputHuma{
		TotalCount:    strconv.FormatInt(count, 10),
		ContentLength: "0",
	}, nil
}

// RegisterAccountRoutes registers the eight migrated account operations on the
// shared Huma API. Paths are GROUP-RELATIVE (the Huma API is bound to the /v1 Fiber
// group; the /v1 prefix rides the OpenAPI `servers` entry). The auth + tenant +
// ParseUUIDPathParameters chain is attached in RegisterAccountRoutesToApp
// (Fiber-level) BEFORE the Huma terminals, not here.
func RegisterAccountRoutes(api huma.API, h *AccountHandler) {
	const (
		listPath     = "/organizations/{organization_id}/ledgers/{ledger_id}/accounts"
		idPath       = listPath + "/{id}"
		aliasPath    = listPath + "/alias/{alias}"
		externalPath = listPath + "/external/{code}"
		countPath    = listPath + "/metrics/count"
		tag          = "Accounts"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createAccount",
		Method:           http.MethodPost,
		Path:             listPath,
		Summary:          "Create a new account",
		Tags:             []string{tag},
		Security:         secAccountBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively (http.DecodeAndValidate).
	}, h.CreateAccountHuma)

	huma.Register(api, huma.Operation{
		OperationID: "listAccounts",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all accounts",
		Tags:        []string{tag},
		Security:    secAccountBearerOrAPIKey,
	}, h.ListAccountsHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountByID",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific account",
		Tags:        []string{tag},
		Security:    secAccountBearerOrAPIKey,
	}, h.GetAccountByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountByAlias",
		Method:      http.MethodGet,
		Path:        aliasPath,
		Summary:     "Retrieve an account by alias",
		Tags:        []string{tag},
		Security:    secAccountBearerOrAPIKey,
	}, h.GetAccountByAliasHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountExternalByCode",
		Method:      http.MethodGet,
		Path:        externalPath,
		Summary:     "Retrieve an account by external code",
		Tags:        []string{tag},
		Security:    secAccountBearerOrAPIKey,
	}, h.GetAccountExternalByCodeHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateAccount",
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an account",
		Tags:             []string{tag},
		Security:         secAccountBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively.
	}, h.UpdateAccountHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteAccount",
		Method:        http.MethodDelete,
		Path:          idPath,
		Summary:       "Delete an account",
		Tags:          []string{tag},
		Security:      secAccountBearerOrAPIKey,
		DefaultStatus: http.StatusNoContent, // bodiless 204.
	}, h.DeleteAccountByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "countAccounts",
		Method:        http.MethodHead,
		Path:          countPath,
		Summary:       "Count accounts",
		Tags:          []string{tag},
		Security:      secAccountBearerOrAPIKey,
		DefaultStatus: http.StatusNoContent, // X-Total-Count header + empty 204 body.
	}, h.CountAccountsHuma)
}

// RegisterAccountRoutesToApp wires the Huma-migrated account resource, mirroring
// RegisterAssetRoutesToApp / RegisterPortfolioRoutesToApp. For each of the eight ops
// it attaches the Fiber auth chain — protectedMidaz(auth,"accounts",verb) (=
// auth.Authorize("midaz","accounts",verb) + tenant PostAuthMiddlewares) +
// ParseUUIDPathParameters("account") — as MIDDLEWARE ONLY (no terminal) on the /v1
// GROUP with GROUP-RELATIVE paths, then registers the Huma terminals via
// RegisterAccountRoutes on the SAME group's Huma API. This preserves the pre-Huma
// (accounts, verb) authz tuples and tenant resolution BYTE-FOR-BYTE — no account
// route becomes public. Called from the unified server's humaMount seam
// (integration task), NOT from routes.go.
func RegisterAccountRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	const (
		listPath     = "/organizations/:organization_id/ledgers/:ledger_id/accounts"
		idPath       = listPath + "/:id"
		aliasPath    = listPath + "/alias/:alias"
		externalPath = listPath + "/external/:code"
		countPath    = listPath + "/metrics/count"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("account")

	group.Post(listPath, protectedMidaz(auth, "accounts", "post", routeOptions, parse)...)
	group.Patch(idPath, protectedMidaz(auth, "accounts", "patch", routeOptions, parse)...)
	group.Get(listPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse)...)
	group.Get(idPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse)...)
	group.Get(aliasPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse)...)
	group.Get(externalPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse)...)
	group.Delete(idPath, protectedMidaz(auth, "accounts", "delete", routeOptions, parse)...)
	group.Head(countPath, protectedMidaz(auth, "accounts", "head", routeOptions, parse)...)

	RegisterAccountRoutes(api, h)
}
