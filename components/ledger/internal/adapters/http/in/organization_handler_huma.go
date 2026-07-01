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

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the organization resource, mirroring
// the asset exemplar (asset_handler_huma.go) adapted to organization's FIRST-LEVEL
// path (no org/ledger prefix — only the top-level /organizations collection and a
// single {id} path param). The conventions are identical to the asset exemplar:
//
//  1. Path params carry ONLY `doc:` (no `format:"uuid"`) so Huma never emits a
//     native 422; ParseUUIDPathParameters (wired as a Fiber middleware BEFORE the
//     Huma terminal) is the sole UUID validator, yielding the canonical 400 / 0065.
//     parsePathUUID (shared, defined in asset_handler_huma.go) re-parses defensively.
//  2. Body ops carry RawBody []byte + SkipValidateBody so http.DecodeAndValidate
//     stays the sole body validator — never a native Huma 422.
//  3. List captures the raw query (via Resolve) and rebuilds the map[string]string
//     http.ValidateParameters consumes, byte-identical to Fiber's c.Queries().
//  4. Errors go through the shared pkgHTTP.HumaProblem (RFC 9457 problem+json).
//  5. Auth stays the Fiber middleware chain (auth.Authorize("midaz","organizations",
//     verb) + tenant PostAuthMiddlewares + ParseUUIDPathParameters) attached in
//     RegisterOrganizationRoutesToApp BEFORE the Huma registration — NOT a Huma
//     Security scheme. The per-op Security metadata below is SPEC-ONLY.

// secOrgBearerOrAPIKey advertises that each organization operation accepts EITHER a
// JWT bearer token OR an X-API-Key (two entries = OR). SPEC metadata only; runtime
// auth is the Fiber guard chain. Scheme names are declared on the shared Huma API
// in unified-server.go.
var secOrgBearerOrAPIKey = []map[string][]string{
	{"BearerAuth": {}},
	{"ApiKeyAuth": {}},
}

// --- POST /organizations ------------------------------------------------------

// CreateOrganizationInputHuma is the Huma request envelope for POST. RawBody keeps
// the body out of Huma's validator (see file header).
type CreateOrganizationInputHuma struct {
	Authorization string `header:"Authorization" doc:"Bearer token (forwarded to the service)"`
	RawBody       []byte `contentType:"application/json"`
}

// CreateOrganizationOutputHuma pins 201 (matching http.Created).
type CreateOrganizationOutputHuma struct {
	Status int
	Body   *mmodel.Organization
}

// CreateOrganizationHuma decodes+validates the raw body imperatively then delegates
// to the shared createOrganization core.
func (handler *OrganizationHandler) CreateOrganizationHuma(ctx context.Context, in *CreateOrganizationInputHuma) (*CreateOrganizationOutputHuma, error) {
	payload := new(mmodel.CreateOrganizationInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	organization, err := handler.createOrganization(ctx, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateOrganizationOutputHuma{Status: http.StatusCreated, Body: organization}, nil
}

// --- GET /organizations (list) ------------------------------------------------

// ListOrganizationsInputHuma advertises the list query params in the spec (doc-only,
// no validation tags) and captures the raw query via Resolve for the imperative
// http.ValidateParameters binder.
type ListOrganizationsInputHuma struct {
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
func (in *ListOrganizationsInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string http.ValidateParameters consumes, matching
// Fiber's c.Queries() (last value wins for a repeated key, present-but-empty keys
// included).
func (in *ListOrganizationsInputHuma) queries() map[string]string {
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

// ListOrganizationsOutputHuma carries the pagination envelope verbatim.
type ListOrganizationsOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListOrganizationsHuma binds the query imperatively then delegates to the shared
// getAllOrganizations core (which owns the status + name-filter guards).
func (handler *OrganizationHandler) ListOrganizationsHuma(ctx context.Context, in *ListOrganizationsInputHuma) (*ListOrganizationsOutputHuma, error) {
	pagination, err := handler.getAllOrganizations(ctx, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListOrganizationsOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /organizations/{id} --------------------------------------------------

// GetOrganizationInputHuma is the by-id request envelope. The id path param carries
// no format tag (ParseUUIDPathParameters is the sole validator).
type GetOrganizationInputHuma struct {
	ID string `path:"id" doc:"Organization ID (UUID)"`
}

// GetOrganizationOutputHuma carries the organization verbatim.
type GetOrganizationOutputHuma struct {
	Status int
	Body   *mmodel.Organization
}

// GetOrganizationByIDHuma delegates to the shared getOrganizationByID core.
func (handler *OrganizationHandler) GetOrganizationByIDHuma(ctx context.Context, in *GetOrganizationInputHuma) (*GetOrganizationOutputHuma, error) {
	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	organization, err := handler.getOrganizationByID(ctx, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetOrganizationOutputHuma{Status: http.StatusOK, Body: organization}, nil
}

// --- PATCH /organizations/{id} ------------------------------------------------

// UpdateOrganizationInputHuma is the update request envelope (RawBody, see Create).
type UpdateOrganizationInputHuma struct {
	ID      string `path:"id" doc:"Organization ID (UUID)"`
	RawBody []byte `contentType:"application/json"`
}

// UpdateOrganizationOutputHuma carries the updated organization (200, matching http.OK).
type UpdateOrganizationOutputHuma struct {
	Status int
	Body   *mmodel.Organization
}

// UpdateOrganizationHuma decodes+validates the raw body imperatively then delegates
// to the shared updateOrganization core.
func (handler *OrganizationHandler) UpdateOrganizationHuma(ctx context.Context, in *UpdateOrganizationInputHuma) (*UpdateOrganizationOutputHuma, error) {
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

	return &UpdateOrganizationOutputHuma{Status: http.StatusOK, Body: organization}, nil
}

// --- DELETE /organizations/{id} -----------------------------------------------

// DeleteOrganizationOutputHuma has NO Body field: paired with DefaultStatus 204 it
// makes Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteOrganizationOutputHuma struct{}

// DeleteOrganizationByIDHuma delegates to the shared deleteOrganization core (which
// owns the production-environment guard); returns a bodiless 204 on success.
func (handler *OrganizationHandler) DeleteOrganizationByIDHuma(ctx context.Context, in *GetOrganizationInputHuma) (*DeleteOrganizationOutputHuma, error) {
	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteOrganization(ctx, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteOrganizationOutputHuma{}, nil
}

// --- HEAD /organizations/metrics/count ----------------------------------------

// CountOrganizationsInputHuma is the HEAD-count request envelope (no path params).
type CountOrganizationsInputHuma struct{}

// CountOrganizationsOutputHuma replicates the Fiber HEAD-count response manually: the
// X-Total-Count header carries the count, Content-Length is pinned to 0, and the body
// is empty at status 204.
type CountOrganizationsOutputHuma struct {
	TotalCount    string `header:"X-Total-Count"`
	ContentLength string `header:"Content-Length"`
}

// CountOrganizationsHuma delegates to the shared countOrganizations core and sets the
// count headers.
func (handler *OrganizationHandler) CountOrganizationsHuma(ctx context.Context, _ *CountOrganizationsInputHuma) (*CountOrganizationsOutputHuma, error) {
	count, err := handler.countOrganizations(ctx)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CountOrganizationsOutputHuma{
		TotalCount:    strconv.FormatInt(count, 10),
		ContentLength: "0",
	}, nil
}

// RegisterOrganizationRoutes registers the six migrated organization operations on
// the shared Huma API. It is the per-file seam RegisterOrganizationRoutesToApp calls;
// the auth + tenant + ParseUUIDPathParameters middleware chain for these routes is
// attached in RegisterOrganizationRoutesToApp (Fiber-level) BEFORE the Huma terminal,
// not here.
//
// Paths are GROUP-RELATIVE: the Huma API is bound to the /v1 Fiber group, so the
// humafiber adapter registers on that group and Fiber prepends /v1.
func RegisterOrganizationRoutes(api huma.API, h *OrganizationHandler) {
	const (
		listPath  = "/organizations"
		idPath    = listPath + "/{id}"
		countPath = listPath + "/metrics/count"
		tag       = "Organizations"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createOrganization",
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create a new organization",
		Tags:        []string{tag},
		Security:    secOrgBearerOrAPIKey,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
	}, h.CreateOrganizationHuma)

	huma.Register(api, huma.Operation{
		OperationID: "listOrganizations",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all organizations",
		Tags:        []string{tag},
		Security:    secOrgBearerOrAPIKey,
	}, h.ListOrganizationsHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getOrganizationByID",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific organization",
		Tags:        []string{tag},
		Security:    secOrgBearerOrAPIKey,
	}, h.GetOrganizationByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateOrganization",
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an existing organization",
		Tags:             []string{tag},
		Security:         secOrgBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively — see createOrganization.
	}, h.UpdateOrganizationHuma)

	huma.Register(api, huma.Operation{
		OperationID: "deleteOrganization",
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete an organization",
		Tags:        []string{tag},
		Security:    secOrgBearerOrAPIKey,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteOrganizationByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID: "countOrganizations",
		Method:      http.MethodHead,
		Path:        countPath,
		Summary:     "Count total organizations",
		Tags:        []string{tag},
		Security:    secOrgBearerOrAPIKey,
		// HEAD count: X-Total-Count header + empty 204 body (Content-Length 0 set on
		// the Out struct), matching the Fiber http.NoContent + header path.
		DefaultStatus: http.StatusNoContent,
	}, h.CountOrganizationsHuma)
}
