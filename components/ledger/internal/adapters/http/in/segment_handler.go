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

// This file is the ledger's Huma adoption of the segment resource, cloned from the
// asset DE-RISK exemplar (asset_handler.go). It reuses the package-shared
// helpers parseOrgLedger / parsePathUUID (org+ledger+id path resolution) and
// declares its own secSegmentBearer spec-only Security metadata (bearer-only, as on
// every resource). Conventions (see asset_handler.go header for the full rationale):
//
//  1. Path params are plain strings with ONLY `doc:` (no `format:"uuid"`): the
//     ParseUUIDPathParameters("segment") Fiber middleware (attached in
//     RegisterSegmentRoutesToApp BEFORE the Huma terminal) is the sole UUID validator
//     — it yields the canonical 400 / 0065.
//  2. Body ops carry RawBody []byte + SkipValidateBody so the imperative
//     http.DecodeAndValidate stays the sole body validator — never a native Huma 422.
//  3. List captures the raw query (via Resolve) and rebuilds the map[string]string
//     that http.ValidateParameters consumes, byte-identical to c.Queries().
//  4. Errors go through the shared pkgHTTP.HumaProblem (RFC 9457).
//  5. Auth stays a Fiber middleware chain (protectedMidaz(auth,"segments",verb) +
//     tenant PostAuthMiddlewares + ParseUUIDPathParameters) — NOT a Huma Security
//     scheme. The per-op Security metadata is SPEC-ONLY.

// secSegmentBearer is the spec-only Security metadata for the segment operations:
// a JWT bearer token. Runtime auth is the Fiber guard chain, which authorizes a
// bearer token and nothing else.
var secSegmentBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /segments -----------------------------------------------------------

// CreateSegmentRequest is the Huma request envelope for POST.
type CreateSegmentRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateSegmentResponse pins 201 (matching http.Created).
type CreateSegmentResponse struct {
	Status int
	Body   *mmodel.Segment
}

// CreateSegment decodes+validates the raw body imperatively then delegates to the
// shared createSegment core.
func (handler *SegmentHandler) CreateSegment(ctx context.Context, in *CreateSegmentRequest) (*CreateSegmentResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreateSegmentInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	segment, err := handler.createSegment(ctx, orgID, ledgerID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateSegmentResponse{Status: http.StatusCreated, Body: segment}, nil
}

// --- GET /segments (list) -----------------------------------------------------

// ListSegmentsRequest advertises the list query params in the spec (doc-only) and
// captures the raw query via Resolve for the imperative http.ValidateParameters binder.
type ListSegmentsRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Metadata       string `query:"metadata" doc:"JSON string to filter segments by metadata fields"`
	Limit          string `query:"limit" doc:"Max items per page (1-100, default 10)"`
	Page           string `query:"page" doc:"Page number (default 1)"`
	StartDate      string `query:"start_date" doc:"Filter segments created on/after this date (YYYY-MM-DD)"`
	EndDate        string `query:"end_date" doc:"Filter segments created on/before this date (YYYY-MM-DD)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in http.ValidateParameters.
func (in *ListSegmentsRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key, present-but-empty
// keys included).
func (in *ListSegmentsRequest) queries() map[string]string {
	return queriesFromValues(in.rawQuery)
}

// ListSegmentsResponse carries the pagination envelope verbatim.
type ListSegmentsResponse struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListSegments binds the query imperatively then delegates to getAllSegments.
func (handler *SegmentHandler) ListSegments(ctx context.Context, in *ListSegmentsRequest) (*ListSegmentsResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllSegments(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListSegmentsResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /segments/{id} -------------------------------------------------------

// GetSegmentRequest is the by-id request envelope.
type GetSegmentRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Segment ID (UUID)"`
}

// GetSegmentResponse carries the segment verbatim.
type GetSegmentResponse struct {
	Status int
	Body   *mmodel.Segment
}

// GetSegmentByID delegates to getSegmentByID.
func (handler *SegmentHandler) GetSegmentByID(ctx context.Context, in *GetSegmentRequest) (*GetSegmentResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	segment, err := handler.getSegmentByID(ctx, orgID, ledgerID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetSegmentResponse{Status: http.StatusOK, Body: segment}, nil
}

// --- PATCH /segments/{id} -----------------------------------------------------

// UpdateSegmentRequest is the update request envelope (RawBody, see Create).
type UpdateSegmentRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Segment ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateSegmentResponse carries the updated segment (200, matching http.OK).
type UpdateSegmentResponse struct {
	Status int
	Body   *mmodel.Segment
}

// UpdateSegment decodes+validates the raw body imperatively then delegates to the
// shared updateSegment core.
func (handler *SegmentHandler) UpdateSegment(ctx context.Context, in *UpdateSegmentRequest) (*UpdateSegmentResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdateSegmentInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	segment, err := handler.updateSegment(ctx, orgID, ledgerID, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateSegmentResponse{Status: http.StatusOK, Body: segment}, nil
}

// --- DELETE /segments/{id} ----------------------------------------------------

// DeleteSegmentResponse has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteSegmentResponse struct{}

// DeleteSegmentByID delegates to deleteSegment; returns a bodiless 204 on success.
func (handler *SegmentHandler) DeleteSegmentByID(ctx context.Context, in *GetSegmentRequest) (*DeleteSegmentResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteSegment(ctx, orgID, ledgerID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteSegmentResponse{}, nil
}

// --- HEAD /segments/metrics/count ---------------------------------------------

// CountSegmentsRequest is the HEAD-count request envelope (org+ledger only).
type CountSegmentsRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
}

// CountSegmentsResponse replicates the Fiber HEAD-count response: X-Total-Count
// carries the count, Content-Length is pinned to 0, body empty at status 204.
type CountSegmentsResponse struct {
	TotalCount    string `header:"X-Total-Count"`
	ContentLength string `header:"Content-Length"`
}

// CountSegments delegates to countSegments and sets the count headers.
func (handler *SegmentHandler) CountSegments(ctx context.Context, in *CountSegmentsRequest) (*CountSegmentsResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	count, err := handler.countSegments(ctx, orgID, ledgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CountSegmentsResponse{
		TotalCount:    strconv.FormatInt(count, 10),
		ContentLength: "0",
	}, nil
}
