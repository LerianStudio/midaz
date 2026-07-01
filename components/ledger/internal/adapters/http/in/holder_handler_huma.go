// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the CRM holder resource (holders +
// the holder-scoped account listing). It mirrors the asset exemplar
// (asset_handler_huma.go); see that file's header for the full conventions.
// Holder-specific notes:
//
//  1. AUTH is appName "midaz" (crm_routes.go ApplicationName), resource "holders".
//     The swaggo @Security on the Fiber wrappers is BearerAuth ONLY, so the per-op
//     Security metadata here is Bearer-only too — SPEC metadata only; runtime auth
//     stays the Fiber guard chain (auth.Authorize("midaz","holders",verb) + tenant
//     + ParseUUIDPathParameters("holder")) attached BEFORE the Huma terminal.
//  2. Holders are ORG-SCOPED (no ledger in the path), so the shells resolve only
//     organization_id (+ id) via the shared parsePathUUID; there is no parseOrgLedger.
//  3. POST carries an idempotency dance: the shell resolves the client key + TTL
//     from request headers and sets the X-Idempotency-Replayed response header from
//     the replayed flag the shared createHolder core returns — the transport-neutral
//     mirror of the Fiber wrapper's GetIdempotencyKeyAndTTL + c.Set.
//  4. PATCH is RFC 7396 merge-patch: the shell derives fieldsToRemove from the parsed
//     body via http.FindNilFields (the same derivation http.WithBody feeds into the
//     Fiber patchRemove local) and passes it to the shared updateHolder core.
//  5. GET-by-id, list and delete read include_deleted / hard_delete from the query,
//     matching the Fiber http.GetBooleanParam("...") reads (ValidateParameters does
//     not bind those flags, so the shells read them from the raw query directly).
//  6. Body ops carry RawBody + SkipValidateBody so http.DecodeAndValidate is the
//     sole body validator (never a native Huma 422). Errors go through pkgHTTP.HumaProblem.

// secHolderBearer advertises that each holder operation accepts a JWT bearer token
// (Bearer-only, matching the Fiber swaggo @Security BearerAuth). SPEC metadata only;
// runtime auth is the Fiber guard chain.
var secHolderBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// queryBool mirrors http.GetBooleanParam: a query value equal to "true"
// (case-insensitive) is true, everything else (absent, empty, "false") is false.
func queryBool(q url.Values, key string) bool {
	return strings.EqualFold(q.Get(key), "true")
}

// queriesFromValues rebuilds the map[string]string that http.ValidateParameters
// consumes, matching Fiber's c.Queries() (last value wins for a repeated key,
// present-but-empty keys included).
func queriesFromValues(q url.Values) map[string]string {
	out := make(map[string]string, len(q))
	for k, vs := range q {
		if len(vs) == 0 {
			out[k] = ""
			continue
		}

		out[k] = vs[len(vs)-1]
	}

	return out
}

// --- POST /holders ------------------------------------------------------------

// CreateHolderInputHuma is the Huma request envelope for POST. RawBody keeps the
// body out of Huma's validator; the idempotency headers are read so the shell can
// run the same claim the Fiber wrapper does.
type CreateHolderInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	IdempotencyKey string `header:"X-Idempotency" doc:"Idempotency key to safely retry the create; an identical retry returns the original holder"`
	IdempotencyTTL string `header:"X-TTL" doc:"Idempotency slot TTL in seconds (default 300)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateHolderOutputHuma pins 201 (matching http.Created) and carries the
// X-Idempotency-Replayed response header (parity with the Fiber c.Set).
type CreateHolderOutputHuma struct {
	Status              int
	IdempotencyReplayed string `header:"X-Idempotency-Replayed"`
	Body                *mmodel.Holder
}

// CreateHolderHuma decodes+validates the raw body imperatively then delegates to the
// shared createHolder core, resolving the idempotency key/TTL from headers and
// projecting the replayed flag onto the X-Idempotency-Replayed response header.
func (handler *HolderHandler) CreateHolderHuma(ctx context.Context, in *CreateHolderInputHuma) (*CreateHolderOutputHuma, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreateHolderInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	ttl := pkgHTTP.ParseIdempotencyTTL(in.IdempotencyTTL)

	holder, replayed, err := handler.createHolder(ctx, orgID, payload, in.IdempotencyKey, ttl)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	replayedHeader := "false"
	if replayed {
		replayedHeader = "true"
	}

	return &CreateHolderOutputHuma{Status: http.StatusCreated, IdempotencyReplayed: replayedHeader, Body: holder}, nil
}

// --- GET /holders/{id} --------------------------------------------------------

// GetHolderInputHuma is the by-id request envelope. The path params carry no format
// tag (ParseUUIDPathParameters is the sole validator); include_deleted is read via
// Resolve, matching the Fiber http.GetBooleanParam read.
type GetHolderInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	ID             string `path:"id" doc:"Holder ID (UUID)"`
	IncludeDeleted string `query:"include_deleted" doc:"Returns the holder even if it was logically deleted (true,false)"`
}

// GetHolderOutputHuma carries the holder verbatim.
type GetHolderOutputHuma struct {
	Status int
	Body   *mmodel.Holder
}

// GetHolderByIDHuma delegates to getHolderByID.
func (handler *HolderHandler) GetHolderByIDHuma(ctx context.Context, in *GetHolderInputHuma) (*GetHolderOutputHuma, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	holder, err := handler.getHolderByID(ctx, orgID, id, strings.EqualFold(in.IncludeDeleted, "true"))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetHolderOutputHuma{Status: http.StatusOK, Body: holder}, nil
}

// --- PATCH /holders/{id} ------------------------------------------------------

// UpdateHolderInputHuma is the update request envelope (RawBody, see Create). The
// raw body is the sole source of the RFC 7396 null-field paths derived below.
type UpdateHolderInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	ID             string `path:"id" doc:"Holder ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateHolderOutputHuma carries the updated holder (200, matching http.OK).
type UpdateHolderOutputHuma struct {
	Status int
	Body   *mmodel.Holder
}

// UpdateHolderHuma decodes+validates the raw body imperatively, derives the merge-patch
// null-field paths via http.FindNilFields (the same derivation http.WithBody feeds the
// Fiber patchRemove local), then delegates to the shared updateHolder core.
func (handler *HolderHandler) UpdateHolderHuma(ctx context.Context, in *UpdateHolderInputHuma) (*UpdateHolderOutputHuma, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdateHolderInput)

	originalMap, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	holder, err := handler.updateHolder(ctx, orgID, id, payload, pkgHTTP.FindNilFields(originalMap, ""))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateHolderOutputHuma{Status: http.StatusOK, Body: holder}, nil
}

// --- DELETE /holders/{id} -----------------------------------------------------

// DeleteHolderInputHuma is the delete request envelope; hard_delete is read from the
// query (matching the Fiber http.GetBooleanParam read).
type DeleteHolderInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	ID             string `path:"id" doc:"Holder ID (UUID)"`
	HardDelete     string `query:"hard_delete" doc:"Use only to perform a physical deletion of the data. This action is irreversible. (true,false)"`
}

// DeleteHolderOutputHuma has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteHolderOutputHuma struct{}

// DeleteHolderByIDHuma delegates to deleteHolder; returns a bodiless 204 on success.
func (handler *HolderHandler) DeleteHolderByIDHuma(ctx context.Context, in *DeleteHolderInputHuma) (*DeleteHolderOutputHuma, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteHolder(ctx, orgID, id, strings.EqualFold(in.HardDelete, "true")); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteHolderOutputHuma{}, nil
}

// --- GET /holders (list) ------------------------------------------------------

// ListHoldersInputHuma advertises the list query params (doc-only, no validation
// tags) and captures the raw query via Resolve for the imperative binder + the
// include_deleted flag read.
type ListHoldersInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	Metadata       string `query:"metadata" doc:"JSON string to filter holders by metadata fields"`
	Limit          string `query:"limit" doc:"Max items per page (1-100, default 10)"`
	Page           string `query:"page" doc:"Page number (default 1)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`
	IncludeDeleted string `query:"include_deleted" doc:"Return includes logically deleted holders (true,false)"`
	ExternalID     string `query:"external_id" doc:"Filter holders by externalID"`
	Document       string `query:"document" doc:"Filter holders by document"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in http.ValidateParameters.
func (in *ListHoldersInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// ListHoldersOutputHuma carries the pagination envelope verbatim.
type ListHoldersOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// GetAllHoldersHuma binds the query imperatively then delegates to getAllHolders.
func (handler *HolderHandler) GetAllHoldersHuma(ctx context.Context, in *ListHoldersInputHuma) (*ListHoldersOutputHuma, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllHolders(ctx, orgID, queriesFromValues(in.rawQuery), queryBool(in.rawQuery, "include_deleted"))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListHoldersOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /holders/{id}/accounts -----------------------------------------------

// ListHolderAccountsInputHuma advertises the account-list query params (doc-only)
// and captures the raw query via Resolve for the imperative binder.
type ListHolderAccountsInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	ID             string `path:"id" doc:"Holder ID (UUID)"`
	Limit          string `query:"limit" doc:"Max items per page (1-100, default 10)"`
	Page           string `query:"page" doc:"Page number (default 1)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`

	rawQuery url.Values
}

// Resolve captures the raw query before the handler (no validation; canonical
// rejection stays in http.ValidateParameters).
func (in *ListHolderAccountsInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// ListHolderAccountsOutputHuma carries the pagination envelope verbatim.
type ListHolderAccountsOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// GetAccountsByHolderHuma binds the query imperatively then delegates to
// getAccountsByHolder.
func (handler *HolderAccountsHandler) GetAccountsByHolderHuma(ctx context.Context, in *ListHolderAccountsInputHuma) (*ListHolderAccountsOutputHuma, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	holderID, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAccountsByHolder(ctx, orgID, holderID, queriesFromValues(in.rawQuery))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListHolderAccountsOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// RegisterHolderRoutes registers the five migrated holder operations on the shared
// Huma API. It is the per-file seam the unified server calls; the auth
// ("midaz","holders",verb) + tenant + ParseUUIDPathParameters("holder") middleware
// chain is attached on the /v1 group (Fiber-level) BEFORE the Huma terminal, not here.
// Paths are GROUP-RELATIVE (see asset_handler_huma.go's RegisterAssetRoutes header
// for the /v1 rationale).
func RegisterHolderRoutes(api huma.API, h *HolderHandler) {
	const (
		listPath = "/organizations/{organization_id}/holders"
		idPath   = listPath + "/{id}"
		tag      = "Holders"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createHolder",
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create a Holder",
		Tags:        []string{tag},
		Security:    secHolderBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
	}, h.CreateHolderHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getHolderByID",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve Holder details",
		Tags:        []string{tag},
		Security:    secHolderBearer,
	}, h.GetHolderByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateHolder",
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a Holder",
		Tags:             []string{tag},
		Security:         secHolderBearer,
		SkipValidateBody: true, // body validated imperatively — RFC 7396 merge-patch core.
	}, h.UpdateHolderHuma)

	huma.Register(api, huma.Operation{
		OperationID: "deleteHolder",
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete a Holder",
		Tags:        []string{tag},
		Security:    secHolderBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteHolderByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID: "listHolders",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List Holders",
		Tags:        []string{tag},
		Security:    secHolderBearer,
	}, h.GetAllHoldersHuma)
}

// RegisterHolderAccountsRoutes registers the holder-scoped account listing on the
// shared Huma API. It is a separate seam so the unified server can mount it
// conditionally (only when the ledger account-query backing is wired, matching the
// Fiber `if hah != nil` guard in crm_routes.go). Auth is ("midaz","holders","get")
// + ParseUUIDPathParameters("holder"), attached BEFORE the Huma terminal.
func RegisterHolderAccountsRoutes(api huma.API, h *HolderAccountsHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "listAccountsByHolder",
		Method:      http.MethodGet,
		Path:        "/organizations/{organization_id}/holders/{id}/accounts",
		Summary:     "List Accounts by Holder",
		Tags:        []string{"Holders"},
		Security:    secHolderBearer,
	}, h.GetAccountsByHolderHuma)
}
