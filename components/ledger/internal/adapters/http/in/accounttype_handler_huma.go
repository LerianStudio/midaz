// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the account-type resource. It mirrors
// the asset exemplar (asset_handler_huma.go) — the DE-RISK proof for the ledger
// fan-out — adapted to account-type's five ops (no HEAD-count) and its CURSOR
// pagination (GetAllAccountType returns a cursor the envelope carries via SetCursor).
// See asset_handler_huma.go's header for the full convention rationale; the shared
// helpers (parseOrgLedger, parsePathUUID) and the shared error projection
// (pkgHTTP.HumaProblem) are reused verbatim.
//
// AUTH NOTE: account-types runs under appName "routing" (protectedRouting), NOT
// "midaz" — the per-op (routing, account-types, verb) authz tuples are preserved
// BYTE-FOR-BYTE by the Fiber guard chain attached in RegisterAccountTypeRoutesToApp.
// The Security metadata below is SPEC-ONLY (bearer OR api-key for the generated OAS).

// secAccountTypeBearerOrAPIKey advertises that each account-type op accepts EITHER a
// JWT bearer token OR an X-API-Key (two entries = OR). SPEC metadata only; runtime
// auth is the Fiber guard chain. The scheme names are declared once on the shared
// Huma API in unified-server.go.
var secAccountTypeBearerOrAPIKey = []map[string][]string{
	{"BearerAuth": {}},
	{"ApiKeyAuth": {}},
}

// --- POST /account-types ------------------------------------------------------

// CreateAccountTypeInputHuma is the Huma request envelope for POST. RawBody keeps the
// body out of Huma's validator (see asset header); the org+ledger path params are
// validated by the Fiber ParseUUIDPathParameters middleware, not by a format tag.
type CreateAccountTypeInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateAccountTypeOutputHuma pins 201 (matching http.Created).
type CreateAccountTypeOutputHuma struct {
	Status int
	Body   *mmodel.AccountType
}

// CreateAccountTypeHuma decodes+validates the raw body imperatively then delegates to
// the shared createAccountType core.
func (handler *AccountTypeHandler) CreateAccountTypeHuma(ctx context.Context, in *CreateAccountTypeInputHuma) (*CreateAccountTypeOutputHuma, error) {
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

	return &CreateAccountTypeOutputHuma{Status: http.StatusCreated, Body: accountType}, nil
}

// --- GET /account-types (list) ------------------------------------------------

// ListAccountTypesInputHuma advertises the list query params in the spec (doc-only,
// no validation tags) and captures the raw query via Resolve for the imperative
// http.ValidateParameters binder.
type ListAccountTypesInputHuma struct {
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
func (in *ListAccountTypesInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key, present-but-empty
// keys included).
func (in *ListAccountTypesInputHuma) queries() map[string]string {
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

// ListAccountTypesOutputHuma carries the pagination envelope verbatim.
type ListAccountTypesOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListAccountTypesHuma binds the query imperatively then delegates to
// getAllAccountTypes.
func (handler *AccountTypeHandler) ListAccountTypesHuma(ctx context.Context, in *ListAccountTypesInputHuma) (*ListAccountTypesOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllAccountTypes(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListAccountTypesOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /account-types/{id} --------------------------------------------------

// GetAccountTypeInputHuma is the by-id request envelope. The id path param carries no
// format tag (ParseUUIDPathParameters is the sole validator). The path tag is "id"
// (matching the Fiber route's :id param), NOT "account_type_id".
type GetAccountTypeInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Account Type ID (UUID)"`
}

// GetAccountTypeOutputHuma carries the account type verbatim.
type GetAccountTypeOutputHuma struct {
	Status int
	Body   *mmodel.AccountType
}

// GetAccountTypeByIDHuma delegates to getAccountTypeByID.
func (handler *AccountTypeHandler) GetAccountTypeByIDHuma(ctx context.Context, in *GetAccountTypeInputHuma) (*GetAccountTypeOutputHuma, error) {
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

	return &GetAccountTypeOutputHuma{Status: http.StatusOK, Body: accountType}, nil
}

// --- PATCH /account-types/{id} ------------------------------------------------

// UpdateAccountTypeInputHuma is the update request envelope (RawBody, see Create).
type UpdateAccountTypeInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Account Type ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateAccountTypeOutputHuma carries the updated account type (200, matching http.OK).
type UpdateAccountTypeOutputHuma struct {
	Status int
	Body   *mmodel.AccountType
}

// UpdateAccountTypeHuma decodes+validates the raw body imperatively then delegates to
// the shared updateAccountType core.
func (handler *AccountTypeHandler) UpdateAccountTypeHuma(ctx context.Context, in *UpdateAccountTypeInputHuma) (*UpdateAccountTypeOutputHuma, error) {
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

	return &UpdateAccountTypeOutputHuma{Status: http.StatusOK, Body: accountType}, nil
}

// --- DELETE /account-types/{id} -----------------------------------------------

// DeleteAccountTypeOutputHuma has NO Body field: paired with DefaultStatus 204 it
// makes Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteAccountTypeOutputHuma struct{}

// DeleteAccountTypeByIDHuma delegates to deleteAccountType; returns a bodiless 204 on
// success.
func (handler *AccountTypeHandler) DeleteAccountTypeByIDHuma(ctx context.Context, in *GetAccountTypeInputHuma) (*DeleteAccountTypeOutputHuma, error) {
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

	return &DeleteAccountTypeOutputHuma{}, nil
}

// RegisterAccountTypeRoutes registers the five migrated account-type operations on the
// shared Huma API. It is the per-file seam unified-server.go calls; the auth + tenant +
// ParseUUIDPathParameters middleware chain for these routes is attached in
// RegisterAccountTypeRoutesToApp (Fiber-level) BEFORE the Huma terminal, not here.
//
// Paths are GROUP-RELATIVE: the Huma API is bound to the /v1 Fiber group, so the
// humafiber adapter registers on that group and Fiber prepends /v1. The /v1 prefix
// rides the OpenAPI `servers` entry, keeping op paths relative.
func RegisterAccountTypeRoutes(api huma.API, h *AccountTypeHandler) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/account-types"
		idPath   = listPath + "/{id}"
		tag      = "Account Types"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createAccountType",
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create a new account type",
		Tags:        []string{tag},
		Security:    secAccountTypeBearerOrAPIKey,
		// Body validated imperatively (http.DecodeAndValidate) — see asset header.
		SkipValidateBody: true,
	}, h.CreateAccountTypeHuma)

	huma.Register(api, huma.Operation{
		OperationID: "listAccountTypes",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all account types",
		Tags:        []string{tag},
		Security:    secAccountTypeBearerOrAPIKey,
	}, h.ListAccountTypesHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getAccountTypeByID",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific account type",
		Tags:        []string{tag},
		Security:    secAccountTypeBearerOrAPIKey,
	}, h.GetAccountTypeByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateAccountType",
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an account type",
		Tags:             []string{tag},
		Security:         secAccountTypeBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively — see createAccountType.
	}, h.UpdateAccountTypeHuma)

	huma.Register(api, huma.Operation{
		OperationID: "deleteAccountType",
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete an account type",
		Tags:        []string{tag},
		Security:    secAccountTypeBearerOrAPIKey,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteAccountTypeByIDHuma)
}

// RegisterAccountTypeRoutesToApp wires the Huma-migrated account-type resource,
// mirroring RegisterAssetRoutesToApp. For each of the five ops it attaches the Fiber
// auth chain — protectedRouting(auth,"account-types",verb) (= auth.Authorize(
// "routing","account-types",verb) + tenant PostAuthMiddlewares) +
// ParseUUIDPathParameters("account_type") — as MIDDLEWARE ONLY (no terminal) on the
// /v1 GROUP with GROUP-RELATIVE paths, then registers the Huma terminals via
// RegisterAccountTypeRoutes on the SAME group's Huma API. Unlike the other Wave-1
// resources, account-type authorizes against the "routing" appName (protectedRouting,
// NOT protectedMidaz), exactly as the pre-migration routes.go did — this preserves
// the ("routing","account-types",verb) authz tuples and tenant resolution
// BYTE-FOR-BYTE, no account-type route becomes public. The op order (post, patch,
// get-by-id, list, delete) matches routes.go. Called from the unified server's
// humaMount seam (integration task), NOT from routes.go.
func RegisterAccountTypeRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AccountTypeHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	const (
		listPath = "/organizations/:organization_id/ledgers/:ledger_id/account-types"
		idPath   = listPath + "/:id"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("account_type")

	group.Post(listPath, protectedRouting(auth, "account-types", "post", routeOptions, parse)...)
	group.Patch(idPath, protectedRouting(auth, "account-types", "patch", routeOptions, parse)...)
	group.Get(idPath, protectedRouting(auth, "account-types", "get", routeOptions, parse)...)
	group.Get(listPath, protectedRouting(auth, "account-types", "get", routeOptions, parse)...)
	group.Delete(idPath, protectedRouting(auth, "account-types", "delete", routeOptions, parse)...)

	RegisterAccountTypeRoutes(api, h)
}
