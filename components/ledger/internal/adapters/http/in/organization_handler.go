// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the organization resource, mirroring
// the asset exemplar (asset_handler.go) adapted to organization's FIRST-LEVEL
// path (no org/ledger prefix — only the top-level /organizations collection and a
// single {id} path param). The conventions are identical to the asset exemplar:
//
//  1. Path params carry ONLY `doc:` (no `format:"uuid"`) so Huma never emits a
//     native 422; ParseUUIDPathParameters (wired as a Fiber middleware BEFORE the
//     Huma terminal) is the sole UUID validator, yielding the canonical 400 / 0065.
//     parsePathUUID (shared, defined in asset_handler.go) re-parses defensively.
//  2. Body ops carry RawBody []byte + SkipValidateBody so http.DecodeAndValidate
//     stays the sole body validator — never a native Huma 422.
//  3. List captures the raw query (via Resolve) and rebuilds the map[string]string
//     http.ValidateParameters consumes, byte-identical to Fiber's c.Queries().
//  4. Errors go through the shared pkgHTTP.HumaProblem (RFC 9457 problem+json).
//  5. Auth stays the Fiber middleware chain (auth.Authorize("midaz","organizations",
//     verb) + tenant PostAuthMiddlewares + ParseUUIDPathParameters) attached in
//     RegisterOrganizationRoutesToApp BEFORE the Huma registration — NOT a Huma
//     Security scheme. The per-op Security metadata below is SPEC-ONLY.
//
// The CREATE shell in this file serves the /v1 contract: it passes
// command.HolderOffV1 so the organization's self-holder is NOT provisioned. Its /v2
// twin is in organization_handler_v2.go. Every other op is version-agnostic — the
// organization wire shape carries no holder field, so the two contracts share one
// response type and the five non-create terminals bind the same handler methods.

// secOrgBearer advertises that each organization operation accepts EITHER a
// JWT bearer token. SPEC metadata only; runtime auth is the Fiber guard chain. The
// scheme name is declared on the shared Huma API.
var secOrgBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /organizations ------------------------------------------------------

// CreateOrganizationRequest is the Huma request envelope for POST. RawBody keeps
// the body out of Huma's validator (see file header).
type CreateOrganizationRequest struct {
	RawBody []byte `contentType:"application/json"`
}

// CreateOrganizationResponse pins 201 (matching http.Created).
type CreateOrganizationResponse struct {
	Status int
	Body   *mmodel.Organization
}

// CreateOrganization decodes+validates the raw body imperatively then delegates
// to the shared createOrganization core under command.HolderOffV1: the /v1 contract
// predates the holder seam, so no CRM self-holder record is written.
func (handler *OrganizationHandler) CreateOrganization(ctx context.Context, in *CreateOrganizationRequest) (*CreateOrganizationResponse, error) {
	payload := new(mmodel.CreateOrganizationInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	organization, err := handler.createOrganization(ctx, payload, command.HolderOffV1)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateOrganizationResponse{Status: http.StatusCreated, Body: organization}, nil
}

// --- GET /organizations (list) ------------------------------------------------

// ListOrganizationsRequest advertises the list query params in the spec (doc-only,
// no validation tags) and captures the raw query via Resolve for the imperative
// http.ValidateParameters binder.
type ListOrganizationsRequest struct {
	Metadata        string `query:"metadata" doc:"JSON string to filter organizations by metadata fields"`
	Limit           string `query:"limit" doc:"Max items per page (1-100, default 10)"`
	Page            string `query:"page" doc:"Page number (default 1)"`
	StartDate       string `query:"start_date" doc:"Filter organizations created on/after this date (YYYY-MM-DD)"`
	EndDate         string `query:"end_date" doc:"Filter organizations created on/before this date (YYYY-MM-DD)"`
	SortOrder       string `query:"sort_order" doc:"Sort direction (asc, desc)"`
	LegalName       string `query:"legal_name" doc:"Filter by legal name (case-insensitive, prefix match)"`
	DoingBusinessAs string `query:"doing_business_as" doc:"Filter by doing business as name (case-insensitive, prefix match)"`
	Status          string `query:"status" doc:"Filter by status"`
	LegalDocument   string `query:"legal_document" doc:"Filter by legal document (exact match)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in the core (ValidateParameters
// + the organization status/name-filter guards).
func (in *ListOrganizationsRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string http.ValidateParameters consumes, matching
// Fiber's c.Queries() (last value wins for a repeated key, present-but-empty keys
// included).
func (in *ListOrganizationsRequest) queries() map[string]string {
	return queriesFromValues(in.rawQuery)
}

// ListOrganizationsResponse carries the pagination envelope verbatim.
type ListOrganizationsResponse struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListOrganizations binds the query imperatively then delegates to the shared
// getAllOrganizations core (which owns the status + name-filter guards).
func (handler *OrganizationHandler) ListOrganizations(ctx context.Context, in *ListOrganizationsRequest) (*ListOrganizationsResponse, error) {
	pagination, err := handler.getAllOrganizations(ctx, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListOrganizationsResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /organizations/{id} --------------------------------------------------

// GetOrganizationRequest is the by-id request envelope. The id path param carries
// no format tag (ParseUUIDPathParameters is the sole validator).
type GetOrganizationRequest struct {
	ID string `path:"id" doc:"Organization ID (UUID)"`
}

// GetOrganizationResponse carries the organization verbatim.
type GetOrganizationResponse struct {
	Status int
	Body   *mmodel.Organization
}

// GetOrganizationByID delegates to the shared getOrganizationByID core.
func (handler *OrganizationHandler) GetOrganizationByID(ctx context.Context, in *GetOrganizationRequest) (*GetOrganizationResponse, error) {
	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	organization, err := handler.getOrganizationByID(ctx, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetOrganizationResponse{Status: http.StatusOK, Body: organization}, nil
}

// --- PATCH /organizations/{id} ------------------------------------------------

// UpdateOrganizationRequest is the update request envelope (RawBody, see Create).
type UpdateOrganizationRequest struct {
	ID      string `path:"id" doc:"Organization ID (UUID)"`
	RawBody []byte `contentType:"application/json"`
}

// UpdateOrganizationResponse carries the updated organization (200, matching http.OK).
type UpdateOrganizationResponse struct {
	Status int
	Body   *mmodel.Organization
}

// UpdateOrganization decodes+validates the raw body imperatively then delegates
// to the shared updateOrganization core.
func (handler *OrganizationHandler) UpdateOrganization(ctx context.Context, in *UpdateOrganizationRequest) (*UpdateOrganizationResponse, error) {
	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdateOrganizationInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	organization, err := handler.updateOrganization(ctx, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateOrganizationResponse{Status: http.StatusOK, Body: organization}, nil
}

// --- DELETE /organizations/{id} -----------------------------------------------

// DeleteOrganizationResponse has NO Body field: paired with DefaultStatus 204 it
// makes Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteOrganizationResponse struct{}

// DeleteOrganizationByID delegates to the shared deleteOrganization core (which
// owns the production-environment guard); returns a bodiless 204 on success.
func (handler *OrganizationHandler) DeleteOrganizationByID(ctx context.Context, in *GetOrganizationRequest) (*DeleteOrganizationResponse, error) {
	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteOrganization(ctx, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteOrganizationResponse{}, nil
}

// --- HEAD /organizations/metrics/count ----------------------------------------

// CountOrganizationsRequest is the HEAD-count request envelope (no path params).
type CountOrganizationsRequest struct{}

// CountOrganizationsResponse replicates the Fiber HEAD-count response manually: the
// X-Total-Count header carries the count, Content-Length is pinned to 0, and the body
// is empty at status 204.
type CountOrganizationsResponse struct {
	TotalCount    string `header:"X-Total-Count"`
	ContentLength string `header:"Content-Length"`
}

// CountOrganizations delegates to the shared countOrganizations core and sets the
// count headers.
func (handler *OrganizationHandler) CountOrganizations(ctx context.Context, _ *CountOrganizationsRequest) (*CountOrganizationsResponse, error) {
	count, err := handler.countOrganizations(ctx)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CountOrganizationsResponse{
		TotalCount:    strconv.FormatInt(count, 10),
		ContentLength: "0",
	}, nil
}
