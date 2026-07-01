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

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the segment resource, cloned from the
// asset DE-RISK exemplar (asset_handler_huma.go). It reuses the package-shared
// helpers parseOrgLedger / parsePathUUID (org+ledger+id path resolution) and the
// secAssetBearerOrAPIKey spec-only Security metadata (the same Bearer-OR-ApiKey OR
// applies to every resource). Conventions (see asset_handler_huma.go header for the
// full rationale):
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

// --- POST /segments -----------------------------------------------------------

// CreateSegmentInputHuma is the Huma request envelope for POST.
type CreateSegmentInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateSegmentOutputHuma pins 201 (matching http.Created).
type CreateSegmentOutputHuma struct {
	Status int
	Body   *mmodel.Segment
}

// CreateSegmentHuma decodes+validates the raw body imperatively then delegates to the
// shared createSegment core.
func (handler *SegmentHandler) CreateSegmentHuma(ctx context.Context, in *CreateSegmentInputHuma) (*CreateSegmentOutputHuma, error) {
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

	return &CreateSegmentOutputHuma{Status: http.StatusCreated, Body: segment}, nil
}

// --- GET /segments (list) -----------------------------------------------------

// ListSegmentsInputHuma advertises the list query params in the spec (doc-only) and
// captures the raw query via Resolve for the imperative http.ValidateParameters binder.
type ListSegmentsInputHuma struct {
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
func (in *ListSegmentsInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key, present-but-empty
// keys included).
func (in *ListSegmentsInputHuma) queries() map[string]string {
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

// ListSegmentsOutputHuma carries the pagination envelope verbatim.
type ListSegmentsOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListSegmentsHuma binds the query imperatively then delegates to getAllSegments.
func (handler *SegmentHandler) ListSegmentsHuma(ctx context.Context, in *ListSegmentsInputHuma) (*ListSegmentsOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllSegments(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListSegmentsOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /segments/{id} -------------------------------------------------------

// GetSegmentInputHuma is the by-id request envelope.
type GetSegmentInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Segment ID (UUID)"`
}

// GetSegmentOutputHuma carries the segment verbatim.
type GetSegmentOutputHuma struct {
	Status int
	Body   *mmodel.Segment
}

// GetSegmentByIDHuma delegates to getSegmentByID.
func (handler *SegmentHandler) GetSegmentByIDHuma(ctx context.Context, in *GetSegmentInputHuma) (*GetSegmentOutputHuma, error) {
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

	return &GetSegmentOutputHuma{Status: http.StatusOK, Body: segment}, nil
}

// --- PATCH /segments/{id} -----------------------------------------------------

// UpdateSegmentInputHuma is the update request envelope (RawBody, see Create).
type UpdateSegmentInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Segment ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateSegmentOutputHuma carries the updated segment (200, matching http.OK).
type UpdateSegmentOutputHuma struct {
	Status int
	Body   *mmodel.Segment
}

// UpdateSegmentHuma decodes+validates the raw body imperatively then delegates to the
// shared updateSegment core.
func (handler *SegmentHandler) UpdateSegmentHuma(ctx context.Context, in *UpdateSegmentInputHuma) (*UpdateSegmentOutputHuma, error) {
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

	return &UpdateSegmentOutputHuma{Status: http.StatusOK, Body: segment}, nil
}

// --- DELETE /segments/{id} ----------------------------------------------------

// DeleteSegmentOutputHuma has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteSegmentOutputHuma struct{}

// DeleteSegmentByIDHuma delegates to deleteSegment; returns a bodiless 204 on success.
func (handler *SegmentHandler) DeleteSegmentByIDHuma(ctx context.Context, in *GetSegmentInputHuma) (*DeleteSegmentOutputHuma, error) {
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

	return &DeleteSegmentOutputHuma{}, nil
}

// --- HEAD /segments/metrics/count ---------------------------------------------

// CountSegmentsInputHuma is the HEAD-count request envelope (org+ledger only).
type CountSegmentsInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
}

// CountSegmentsOutputHuma replicates the Fiber HEAD-count response: X-Total-Count
// carries the count, Content-Length is pinned to 0, body empty at status 204.
type CountSegmentsOutputHuma struct {
	TotalCount    string `header:"X-Total-Count"`
	ContentLength string `header:"Content-Length"`
}

// CountSegmentsHuma delegates to countSegments and sets the count headers.
func (handler *SegmentHandler) CountSegmentsHuma(ctx context.Context, in *CountSegmentsInputHuma) (*CountSegmentsOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	count, err := handler.countSegments(ctx, orgID, ledgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CountSegmentsOutputHuma{
		TotalCount:    strconv.FormatInt(count, 10),
		ContentLength: "0",
	}, nil
}

// RegisterSegmentRoutes registers the six migrated segment operations on the shared
// Huma API. Paths are GROUP-RELATIVE (the Huma API is bound to the /v1 Fiber group;
// the /v1 prefix rides the OpenAPI servers entry). Mirrors RegisterAssetRoutes.
func RegisterSegmentRoutes(api huma.API, h *SegmentHandler) {
	const (
		listPath  = "/organizations/{organization_id}/ledgers/{ledger_id}/segments"
		idPath    = listPath + "/{id}"
		countPath = listPath + "/metrics/count"
		tag       = "Segments"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createSegment",
		Method:           http.MethodPost,
		Path:             listPath,
		Summary:          "Create a new segment",
		Tags:             []string{tag},
		Security:         secAssetBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively — see file header.
	}, h.CreateSegmentHuma)

	huma.Register(api, huma.Operation{
		OperationID: "listSegments",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all segments",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
	}, h.ListSegmentsHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getSegmentByID",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific segment",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
	}, h.GetSegmentByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateSegment",
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a segment",
		Tags:             []string{tag},
		Security:         secAssetBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively — see file header.
	}, h.UpdateSegmentHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteSegment",
		Method:        http.MethodDelete,
		Path:          idPath,
		Summary:       "Delete a segment",
		Tags:          []string{tag},
		Security:      secAssetBearerOrAPIKey,
		DefaultStatus: http.StatusNoContent, // Out struct with no Body field => bodiless 204.
	}, h.DeleteSegmentByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "countSegments",
		Method:        http.MethodHead,
		Path:          countPath,
		Summary:       "Count total segments",
		Tags:          []string{tag},
		Security:      secAssetBearerOrAPIKey,
		DefaultStatus: http.StatusNoContent, // X-Total-Count header + empty 204 body.
	}, h.CountSegmentsHuma)
}

// RegisterSegmentRoutesToApp wires the Huma-migrated segment resource. For each of the
// six ops it attaches the Fiber auth chain — protectedMidaz(auth,"segments",verb) (=
// auth.Authorize("midaz","segments",verb) + tenant PostAuthMiddlewares) +
// ParseUUIDPathParameters("segment") — as MIDDLEWARE ONLY (no terminal) on the /v1
// GROUP with GROUP-RELATIVE paths, then registers the Huma terminals via
// RegisterSegmentRoutes on the SAME group's Huma API. This preserves the pre-Huma
// (segments, verb) authz tuples and tenant resolution BYTE-FOR-BYTE — no segment
// route becomes public. Mirrors RegisterAssetRoutesToApp; the integration task calls
// this from the unified server's humaMount seam.
func RegisterSegmentRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *SegmentHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	const (
		listPath  = "/organizations/:organization_id/ledgers/:ledger_id/segments"
		idPath    = listPath + "/:id"
		countPath = listPath + "/metrics/count"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("segment")

	group.Post(listPath, protectedMidaz(auth, "segments", "post", routeOptions, parse)...)
	group.Patch(idPath, protectedMidaz(auth, "segments", "patch", routeOptions, parse)...)
	group.Get(listPath, protectedMidaz(auth, "segments", "get", routeOptions, parse)...)
	group.Get(idPath, protectedMidaz(auth, "segments", "get", routeOptions, parse)...)
	group.Delete(idPath, protectedMidaz(auth, "segments", "delete", routeOptions, parse)...)
	group.Head(countPath, protectedMidaz(auth, "segments", "head", routeOptions, parse)...)

	RegisterSegmentRoutes(api, h)
}
