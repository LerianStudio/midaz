// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the account-type resource. It mirrors
// the asset exemplar (asset_handler.go) — the DE-RISK proof for the ledger
// fan-out — adapted to account-type's five ops (no HEAD-count) and its CURSOR
// pagination (GetAllAccountType returns a cursor the envelope carries via SetCursor).
// See asset_handler.go's header for the full convention rationale; the shared
// helpers (parseOrgLedger, parsePathUUID) and the shared error projection
// (pkgHTTP.HumaProblem) are reused verbatim.
//
// AUTH NOTE: account-types authorizes under the "midaz" appName — the per-op
// (midaz, account-types, verb) authz tuples are attached by the Fiber guard chain in
// RegisterAccountTypeRoutesToApp. The Security metadata below is SPEC-ONLY (bearer OR
// api-key for the generated OAS).

// secAccountTypeBearer advertises that each account-type op accepts EITHER a
// JWT bearer token. SPEC metadata only; runtime auth is the Fiber guard chain. The
// scheme name is declared once on the shared Huma API.
var secAccountTypeBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /account-types ------------------------------------------------------

// CreateAccountTypeRequest is the Huma request envelope for POST. RawBody keeps the
// body out of Huma's validator (see asset header); the org+ledger path params are
// validated by the Fiber ParseUUIDPathParameters middleware, not by a format tag.
type CreateAccountTypeRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateAccountTypeResponse pins 201 (matching http.Created).
type CreateAccountTypeResponse struct {
	Status int
	Body   *mmodel.AccountType
}

// CreateAccountType decodes+validates the raw body imperatively then delegates to
// the shared createAccountType core.
func (handler *AccountTypeHandler) CreateAccountType(ctx context.Context, in *CreateAccountTypeRequest) (*CreateAccountTypeResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreateAccountTypeInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	accountType, err := handler.createAccountType(ctx, orgID, ledgerID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateAccountTypeResponse{Status: http.StatusCreated, Body: accountType}, nil
}

// --- GET /account-types (list) ------------------------------------------------

// ListAccountTypesRequest advertises the list query params in the spec (doc-only,
// no validation tags) and captures the raw query via Resolve for the imperative
// http.ValidateParameters binder.
type ListAccountTypesRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Metadata       string `query:"metadata" doc:"JSON string to filter account types by metadata fields"`
	KeyValue       string `query:"key_value" doc:"Filter account types by key value"`
	Limit          string `query:"limit" doc:"Max items per page (1-100, default 10)"`
	Page           string `query:"page" doc:"Page number (default 1)"`
	Cursor         string `query:"cursor" doc:"Cursor for cursor-based pagination"`
	StartDate      string `query:"start_date" doc:"Filter account types created on/after this date (YYYY-MM-DD)"`
	EndDate        string `query:"end_date" doc:"Filter account types created on/before this date (YYYY-MM-DD)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in http.ValidateParameters.
func (in *ListAccountTypesRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key, present-but-empty
// keys included).
func (in *ListAccountTypesRequest) queries() map[string]string {
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

// ListAccountTypesResponse carries the pagination envelope verbatim.
type ListAccountTypesResponse struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListAccountTypes binds the query imperatively then delegates to
// getAllAccountTypes.
func (handler *AccountTypeHandler) ListAccountTypes(ctx context.Context, in *ListAccountTypesRequest) (*ListAccountTypesResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllAccountTypes(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListAccountTypesResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /account-types/{id} --------------------------------------------------

// GetAccountTypeRequest is the by-id request envelope. The id path param carries no
// format tag (ParseUUIDPathParameters is the sole validator). The path tag is "id"
// (matching the Fiber route's :id param), NOT "account_type_id".
type GetAccountTypeRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Account Type ID (UUID)"`
}

// GetAccountTypeResponse carries the account type verbatim.
type GetAccountTypeResponse struct {
	Status int
	Body   *mmodel.AccountType
}

// GetAccountTypeByID delegates to getAccountTypeByID.
func (handler *AccountTypeHandler) GetAccountTypeByID(ctx context.Context, in *GetAccountTypeRequest) (*GetAccountTypeResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	accountType, err := handler.getAccountTypeByID(ctx, orgID, ledgerID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetAccountTypeResponse{Status: http.StatusOK, Body: accountType}, nil
}

// --- PATCH /account-types/{id} ------------------------------------------------

// UpdateAccountTypeRequest is the update request envelope (RawBody, see Create).
type UpdateAccountTypeRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Account Type ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateAccountTypeResponse carries the updated account type (200, matching http.OK).
type UpdateAccountTypeResponse struct {
	Status int
	Body   *mmodel.AccountType
}

// UpdateAccountType decodes+validates the raw body imperatively then delegates to
// the shared updateAccountType core.
func (handler *AccountTypeHandler) UpdateAccountType(ctx context.Context, in *UpdateAccountTypeRequest) (*UpdateAccountTypeResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdateAccountTypeInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	accountType, err := handler.updateAccountType(ctx, orgID, ledgerID, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateAccountTypeResponse{Status: http.StatusOK, Body: accountType}, nil
}

// --- DELETE /account-types/{id} -----------------------------------------------

// DeleteAccountTypeResponse has NO Body field: paired with DefaultStatus 204 it
// makes Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteAccountTypeResponse struct{}

// DeleteAccountTypeByID delegates to deleteAccountType; returns a bodiless 204 on
// success.
func (handler *AccountTypeHandler) DeleteAccountTypeByID(ctx context.Context, in *GetAccountTypeRequest) (*DeleteAccountTypeResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteAccountType(ctx, orgID, ledgerID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteAccountTypeResponse{}, nil
}

// RegisterAccountTypeRoutes registers the five account-type operations on the
// shared Huma API. It is the per-file seam registerAccountTypeRoutesToApp calls; the auth +
// tenant + ParseUUIDPathParameters middleware chain for these routes is attached in
// registerAccountTypeRoutesToApp (Fiber-level) BEFORE the Huma terminal, not here.
//
// Paths are GROUP-RELATIVE: the Huma API is bound to a versioned Fiber group, so the
// humafiber adapter registers on that group and Fiber prepends the version prefix.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. A straight v1/v2 mirror reuses the same handler methods and the
// same input/output types, so only the operation IDs differ between the twins.
func RegisterAccountTypeRoutes(api huma.API, h *AccountTypeHandler, opSuffix string) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/account-types"
		idPath   = listPath + "/{id}"
		tag      = "Account Types"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createAccountType" + opSuffix,
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create a new account type",
		Tags:        []string{tag},
		Security:    secAccountTypeBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see asset header.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateAccountType)
	attachTypedRequestBody[mmodel.CreateAccountTypeInput](api, "createAccountType"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listAccountTypes" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all account types",
		Tags:        []string{tag},
		Security:    secAccountTypeBearer,
	}, h.ListAccountTypes)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountTypeByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific account type",
		Tags:        []string{tag},
		Security:    secAccountTypeBearer,
	}, h.GetAccountTypeByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updateAccountType" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an account type",
		Tags:             []string{tag},
		Security:         secAccountTypeBearer,
		SkipValidateBody: true, // body validated imperatively — see createAccountType.
	}, h.UpdateAccountType)
	attachTypedRequestBody[mmodel.UpdateAccountTypeInput](api, "updateAccountType"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "deleteAccountType" + opSuffix,
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete an account type",
		Tags:        []string{tag},
		Security:    secAccountTypeBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteAccountTypeByID)
}

// RegisterAccountTypeRoutesToApp wires the account-type surface onto the
// /v1 contract. See registerAccountTypeRoutesToApp for what it attaches.
func RegisterAccountTypeRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountTypeHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerAccountTypeRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV1)
}

// RegisterAccountTypeV2RoutesToApp wires the same account-type surface onto the /v2
// contract: same paths, same handlers, same authz tuples and tenant chain, differing only
// in the operation IDs the contract publishes. It is additive — /v1 keeps serving
// account-types in parallel — and introduces no new policy surface.
func RegisterAccountTypeV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountTypeHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerAccountTypeRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV2)
}

// registerAccountTypeRoutesToApp is the single description of the account-type route
// surface, shared by every versioned contract that serves it, mirroring
// RegisterAssetRoutesToApp. For each of the five ops it attaches the Fiber auth chain —
// protectedMidaz(auth,"account-types",verb) (= auth.Authorize("midaz","account-types",
// verb) + tenant PostAuthMiddlewares) + ParseUUIDPathParameters("account_type") — as
// MIDDLEWARE ONLY (no terminal) on the VERSIONED GROUP with GROUP-RELATIVE paths, then
// registers the Huma terminals via RegisterAccountTypeRoutes on the SAME group's Huma API.
// The ("midaz","account-types",verb) authz tuples and tenant resolution hold on whichever
// version group it is mounted on; no account-type route becomes public. The op order
// (post, patch, get-by-id, list, delete) matches routes.go.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface
// reaches every version it is mounted on.
func registerAccountTypeRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountTypeHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath = "/organizations/:organization_id/ledgers/:ledger_id/account-types"
		idPath   = listPath + "/:id"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("account_type")

	routePost(group, listPath, protectedMidaz(auth, "account-types", "post", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "account-types", "patch", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "account-types", "get", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "account-types", "get", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "account-types", "delete", routeOptions, parse))

	RegisterAccountTypeRoutes(api, h, opSuffix)
}
