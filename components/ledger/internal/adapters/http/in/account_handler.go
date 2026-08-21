// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file holds the account resource's Huma transport, following the asset
// exemplar (asset_handler.go): shared parseOrgLedger / parsePathUUID / HumaProblem /
// DecodeAndValidate helpers, path params as plain strings (ParseUUIDPathParameters is
// the sole UUID validator — no format tag), raw body bytes decoded imperatively (no
// native Huma 422), and the query bound via ValidateParameters. Auth is the Fiber
// middleware chain attached in RegisterAccountRoutesToApp; the per-op Security
// metadata is SPEC-ONLY.
//
// Account differs from the asset exemplar in one way: TWO extra by-key reads — GET
// .../accounts/alias/{alias} and GET .../accounts/external/{code} — whose path params
// are NOT UUIDs. ParseUUIDPathParameters only UUID-parses params in
// cn.UUIDPathParameters ("id","organization_id","ledger_id",...); "alias" and "code"
// fall through as raw string locals, so no format tag is needed and no native 422 can
// fire.

// secAccountBearerOrAPIKey advertises that each account operation accepts EITHER a
// JWT bearer token OR an X-API-Key (SPEC metadata only; runtime auth is the Fiber
// guard chain). Scheme names are declared once on the shared Huma API.
var secAccountBearerOrAPIKey = []map[string][]string{
	{"BearerAuth": {}},
	{"ApiKeyAuth": {}},
}

// --- POST /accounts -----------------------------------------------------------

// CreateAccountRequest is the Huma request envelope for POST. RawBody keeps the
// body out of Huma's validator; the org+ledger path params are validated by the
// Fiber middleware, not by a format tag.
type CreateAccountRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Authorization  string `header:"Authorization" doc:"Bearer token (forwarded to the service)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateAccountResponse pins 201 (matching http.Created).
type CreateAccountResponse struct {
	Status int
	Body   *mmodel.Account
}

// CreateAccount decodes+validates the raw body imperatively then delegates to
// the shared createAccount core.
func (handler *AccountHandler) CreateAccount(ctx context.Context, in *CreateAccountRequest) (*CreateAccountResponse, error) {
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

	return &CreateAccountResponse{Status: http.StatusCreated, Body: account}, nil
}

// --- GET /accounts (list) -----------------------------------------------------

// ListAccountsRequest advertises the list query params in the spec (doc-only, no
// validation tags) and captures the raw query via Resolve for the imperative
// http.ValidateParameters binder.
type ListAccountsRequest struct {
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
func (in *ListAccountsRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key, present-but-
// empty keys included).
func (in *ListAccountsRequest) queries() map[string]string {
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

// ListAccountsResponse carries the pagination envelope verbatim.
type ListAccountsResponse struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListAccounts binds the query imperatively then delegates to getAllAccounts.
func (handler *AccountHandler) ListAccounts(ctx context.Context, in *ListAccountsRequest) (*ListAccountsResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllAccounts(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListAccountsResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /accounts/{id} -------------------------------------------------------

// GetAccountRequest is the by-id request envelope. The id path param carries no
// format tag (ParseUUIDPathParameters is the sole validator).
type GetAccountRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Account ID (UUID)"`
}

// GetAccountResponse carries the account verbatim.
type GetAccountResponse struct {
	Status int
	Body   *mmodel.Account
}

// GetAccountByID delegates to getAccountByID.
func (handler *AccountHandler) GetAccountByID(ctx context.Context, in *GetAccountRequest) (*GetAccountResponse, error) {
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

	return &GetAccountResponse{Status: http.StatusOK, Body: account}, nil
}

// --- GET /accounts/alias/{alias} ----------------------------------------------

// GetAccountByAliasRequest is the by-alias request envelope. alias is NOT a UUID;
// it rides through ParseUUIDPathParameters as a raw string local, so it carries no
// format tag and can never trigger a native Huma 422.
type GetAccountByAliasRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Alias          string `path:"alias" doc:"Account alias (e.g. @person1)"`
}

// GetAccountByAlias delegates to the shared getAccountByAlias core.
func (handler *AccountHandler) GetAccountByAlias(ctx context.Context, in *GetAccountByAliasRequest) (*GetAccountResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	account, err := handler.getAccountByAlias(ctx, "handler.get_account_by_alias", orgID, ledgerID, in.Alias)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetAccountResponse{Status: http.StatusOK, Body: account}, nil
}

// --- GET /accounts/external/{code} --------------------------------------------

// GetAccountExternalByCodeRequest is the external-by-code request envelope. code
// is NOT a UUID (see the alias envelope note).
type GetAccountExternalByCodeRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Code           string `path:"code" doc:"Account external code (e.g. BRL)"`
}

// GetAccountExternalByCode resolves the external alias then delegates to the
// shared getAccountByAlias core.
func (handler *AccountHandler) GetAccountExternalByCode(ctx context.Context, in *GetAccountExternalByCodeRequest) (*GetAccountResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	alias := constant.DefaultExternalAccountAliasPrefix + in.Code

	account, err := handler.getAccountByAlias(ctx, "handler.get_account_external_by_code", orgID, ledgerID, alias)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetAccountResponse{Status: http.StatusOK, Body: account}, nil
}

// --- PATCH /accounts/{id} -----------------------------------------------------

// UpdateAccountRequest is the update request envelope (RawBody, see Create).
type UpdateAccountRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Account ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateAccountResponse carries the updated account (200, matching http.OK).
type UpdateAccountResponse struct {
	Status int
	Body   *mmodel.Account
}

// UpdateAccount decodes+validates the raw body imperatively then delegates to
// the shared updateAccount core.
func (handler *AccountHandler) UpdateAccount(ctx context.Context, in *UpdateAccountRequest) (*UpdateAccountResponse, error) {
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

	return &UpdateAccountResponse{Status: http.StatusOK, Body: account}, nil
}

// --- DELETE /accounts/{id} ----------------------------------------------------

// DeleteAccountRequest is the delete request envelope. Authorization is forwarded
// to the service.
type DeleteAccountRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Account ID (UUID)"`
	Authorization  string `header:"Authorization" doc:"Bearer token (forwarded to the service)"`
}

// DeleteAccountResponse has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204.
type DeleteAccountResponse struct{}

// DeleteAccountByID delegates to deleteAccount; returns a bodiless 204.
func (handler *AccountHandler) DeleteAccountByID(ctx context.Context, in *DeleteAccountRequest) (*DeleteAccountResponse, error) {
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

	return &DeleteAccountResponse{}, nil
}

// --- HEAD /accounts/metrics/count ---------------------------------------------

// CountAccountsRequest is the HEAD-count request envelope (org+ledger only).
type CountAccountsRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
}

// CountAccountsResponse replicates the Fiber HEAD-count response manually: the
// X-Total-Count header carries the count, Content-Length is pinned to 0, and the
// body is empty at status 204.
type CountAccountsResponse struct {
	TotalCount    string `header:"X-Total-Count"`
	ContentLength string `header:"Content-Length"`
}

// CountAccounts delegates to countAccounts and sets the count headers.
func (handler *AccountHandler) CountAccounts(ctx context.Context, in *CountAccountsRequest) (*CountAccountsResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	count, err := handler.countAccounts(ctx, orgID, ledgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CountAccountsResponse{
		TotalCount:    strconv.FormatInt(count, 10),
		ContentLength: "0",
	}, nil
}

// RegisterAccountRoutes registers the eight account operations on the
// shared Huma API. Paths are GROUP-RELATIVE (the Huma API is bound to a versioned Fiber
// group, so the humafiber adapter registers on that group and Fiber prepends the version
// prefix). The auth + tenant + ParseUUIDPathParameters chain is attached in
// registerAccountRoutesToApp (Fiber-level) BEFORE the Huma terminals, not here.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. A straight v1/v2 mirror reuses the same handler methods and the
// same input/output types, so only the operation IDs differ between the twins.
func RegisterAccountRoutes(api huma.API, h *AccountHandler, opSuffix string) {
	const (
		listPath     = "/organizations/{organization_id}/ledgers/{ledger_id}/accounts"
		idPath       = listPath + "/{id}"
		aliasPath    = listPath + "/alias/{alias}"
		externalPath = listPath + "/external/{code}"
		countPath    = listPath + "/metrics/count"
		tag          = "Accounts"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createAccount" + opSuffix,
		Method:           http.MethodPost,
		Path:             listPath,
		Summary:          "Create a new account",
		Tags:             []string{tag},
		Security:         secAccountBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively (http.DecodeAndValidate).
		DefaultStatus:    http.StatusCreated,
	}, h.CreateAccount)
	attachTypedRequestBody[mmodel.CreateAccountInput](api, "createAccount"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listAccounts" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all accounts",
		Tags:        []string{tag},
		Security:    secAccountBearerOrAPIKey,
	}, h.ListAccounts)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific account",
		Tags:        []string{tag},
		Security:    secAccountBearerOrAPIKey,
	}, h.GetAccountByID)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountByAlias" + opSuffix,
		Method:      http.MethodGet,
		Path:        aliasPath,
		Summary:     "Retrieve an account by alias",
		Tags:        []string{tag},
		Security:    secAccountBearerOrAPIKey,
	}, h.GetAccountByAlias)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountExternalByCode" + opSuffix,
		Method:      http.MethodGet,
		Path:        externalPath,
		Summary:     "Retrieve an account by external code",
		Tags:        []string{tag},
		Security:    secAccountBearerOrAPIKey,
	}, h.GetAccountExternalByCode)

	huma.Register(api, huma.Operation{
		OperationID:      "updateAccount" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an account",
		Tags:             []string{tag},
		Security:         secAccountBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively.
	}, h.UpdateAccount)
	attachTypedRequestBody[mmodel.UpdateAccountInput](api, "updateAccount"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteAccount" + opSuffix,
		Method:        http.MethodDelete,
		Path:          idPath,
		Summary:       "Delete an account",
		Tags:          []string{tag},
		Security:      secAccountBearerOrAPIKey,
		DefaultStatus: http.StatusNoContent, // bodiless 204.
	}, h.DeleteAccountByID)

	huma.Register(api, huma.Operation{
		OperationID:   "countAccounts" + opSuffix,
		Method:        http.MethodHead,
		Path:          countPath,
		Summary:       "Count accounts",
		Tags:          []string{tag},
		Security:      secAccountBearerOrAPIKey,
		DefaultStatus: http.StatusNoContent, // X-Total-Count header + empty 204 body.
	}, h.CountAccounts)
}

// RegisterAccountRoutesToApp wires the account surface onto the /v1
// contract. See registerAccountRoutesToApp for what it attaches.
func RegisterAccountRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerAccountRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV1)
}

// RegisterAccountV2RoutesToApp wires the same account surface onto the /v2 contract: same
// paths, same handlers, same authz tuples and tenant chain, differing only in the operation
// IDs the contract publishes. It is additive — /v1 keeps serving accounts in parallel — and
// introduces no new policy surface.
func RegisterAccountV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerAccountRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV2)
}

// registerAccountRoutesToApp is the single description of the account route surface, shared
// by every versioned contract that serves it, mirroring RegisterAssetRoutesToApp /
// RegisterPortfolioRoutesToApp. For each of the eight ops it attaches the Fiber auth chain —
// protectedMidaz(auth,"accounts",verb) (= auth.Authorize("midaz","accounts",verb) + tenant
// PostAuthMiddlewares) + ParseUUIDPathParameters("account") — as MIDDLEWARE ONLY (no
// terminal) on the VERSIONED GROUP with GROUP-RELATIVE paths, then registers the Huma
// terminals via RegisterAccountRoutes on the SAME group's Huma API. The (accounts, verb)
// authz tuples and tenant resolution therefore apply on whichever version group it is
// mounted on — no account route becomes public.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface
// reaches every version it is mounted on.
func registerAccountRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath     = "/organizations/:organization_id/ledgers/:ledger_id/accounts"
		idPath       = listPath + "/:id"
		aliasPath    = listPath + "/alias/:alias"
		externalPath = listPath + "/external/:code"
		countPath    = listPath + "/metrics/count"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("account")

	routePost(group, listPath, protectedMidaz(auth, "accounts", "post", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "accounts", "patch", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse))
	routeGet(group, aliasPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse))
	routeGet(group, externalPath, protectedMidaz(auth, "accounts", "get", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "accounts", "delete", routeOptions, parse))
	routeHead(group, countPath, protectedMidaz(auth, "accounts", "head", routeOptions, parse))

	RegisterAccountRoutes(api, h, opSuffix)
}
