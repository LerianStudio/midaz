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
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the asset resource (the DE-RISK
// exemplar for the ledger fan-out). It mirrors the proven tracer pattern
// (rule_handler_huma.go), adapted to the ledger's three-level path
// (org/ledger/asset), its cursor-less offset pagination, and its HEAD-count +
// DELETE-204 shapes. Conventions:
//
//  1. In structs carry the path params as plain strings with ONLY `doc:` (no
//     `format:"uuid"`): a format tag would make Huma reject a bad value with a
//     native 422 BEFORE the handler. The ParseUUIDPathParameters Fiber middleware
//     (attached in unified-server.go BEFORE the Huma terminal) is the sole UUID
//     validator — it yields the canonical 400 / 0065. The core re-parses the
//     (already-validated) strings; that parse never fails on the wired path but is
//     handled defensively.
//  2. Body ops carry RawBody []byte + SkipValidateBody so the imperative
//     http.DecodeAndValidate (the SAME pipeline the Fiber WithBody decorator runs)
//     stays the sole body validator — never a native Huma 422.
//  3. List carries the raw query (via Resolve) and rebuilds the map[string]string
//     that http.ValidateParameters consumes, so the query binder is byte-identical
//     to the Fiber c.Queries() path.
//  4. Errors go through the shared pkgHTTP.HumaProblem (RFC 9457 problem+json,
//     field/status/code-identical to the Fiber http.WithError path).
//  5. Auth stays a Fiber middleware chain (auth.Authorize("midaz","assets",verb) +
//     tenant PostAuthMiddlewares + ParseUUIDPathParameters) attached in
//     unified-server.go BEFORE the Huma registration — NOT a Huma Security scheme.
//     The per-op Security metadata below is SPEC-ONLY (for the generated OAS/SDK).

// secAssetBearer advertises that each asset operation accepts a JWT bearer token.
// SPEC metadata only; runtime auth is the Fiber guard chain, which authorizes a
// bearer token and nothing else. The scheme name is declared once on the shared
// Huma API (BearerAuth via openapi.DeclareBearerAuth in AssembleHumaContract).
var secAssetBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// Path params are declared FLAT on each Input struct (not via an embedded shared
// struct): Huma v2's request layer does not populate `path:` tags on anonymous
// embedded structs, so embedding silently leaves org/ledger empty and every core
// call 0065s. Flat fields are the proven shape (mirrors the tracer). The org+ledger
// pair is resolved through the shared parseOrgLedger helper to avoid repetition.

// parseOrgLedger resolves the org+ledger path strings to UUIDs. On the wired path
// the ParseUUIDPathParameters middleware has already validated them, so this never
// errors; the canonical 0065 is returned defensively if it somehow does.
func parseOrgLedger(orgStr, ledgerStr string) (orgID, ledgerID uuid.UUID, err error) {
	orgID, err = parsePathUUID(orgStr, "organization_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	ledgerID, err = parsePathUUID(ledgerStr, "ledger_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return orgID, ledgerID, nil
}

// parsePathUUID mirrors GetUUIDFromLocals' failure envelope (ErrInvalidPathParameter
// / 0065) so a bad path param yields the canonical 400.
func parsePathUUID(value, key string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", key)
	}

	return id, nil
}

// --- POST /assets -------------------------------------------------------------

// CreateAssetRequest is the Huma request envelope for POST. RawBody keeps the
// body out of Huma's validator (see file header); the org+ledger path params are
// validated by the Fiber middleware, not by a format tag.
type CreateAssetRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Authorization  string `header:"Authorization" doc:"Bearer token (forwarded to the service)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateAssetResponse pins 201 (matching http.Created).
type CreateAssetResponse struct {
	Status int
	Body   *mmodel.Asset
}

// CreateAsset decodes+validates the raw body imperatively then delegates to the
// shared createAsset core.
func (handler *AssetHandler) CreateAsset(ctx context.Context, in *CreateAssetRequest) (*CreateAssetResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreateAssetInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	asset, err := handler.createAsset(ctx, orgID, ledgerID, payload, in.Authorization)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateAssetResponse{Status: http.StatusCreated, Body: asset}, nil
}

// --- GET /assets (list) -------------------------------------------------------

// ListAssetsRequest advertises the list query params in the spec (doc-only, no
// validation tags) and captures the raw query via Resolve for the imperative
// http.ValidateParameters binder.
type ListAssetsRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Metadata       string `query:"metadata" doc:"JSON string to filter assets by metadata fields"`
	Limit          string `query:"limit" doc:"Max items per page (1-100, default 10)"`
	Page           string `query:"page" doc:"Page number (default 1)"`
	StartDate      string `query:"start_date" doc:"Filter assets created on/after this date (YYYY-MM-DD)"`
	EndDate        string `query:"end_date" doc:"Filter assets created on/before this date (YYYY-MM-DD)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in http.ValidateParameters.
func (in *ListAssetsRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key, present-but-
// empty keys included).
func (in *ListAssetsRequest) queries() map[string]string {
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

// ListAssetsResponse carries the pagination envelope verbatim.
type ListAssetsResponse struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListAssets binds the query imperatively then delegates to getAllAssets.
func (handler *AssetHandler) ListAssets(ctx context.Context, in *ListAssetsRequest) (*ListAssetsResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllAssets(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListAssetsResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /assets/{id} ---------------------------------------------------------

// GetAssetRequest is the by-id request envelope. The id path param carries no
// format tag (ParseUUIDPathParameters is the sole validator).
type GetAssetRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Asset ID (UUID)"`
}

// GetAssetResponse carries the asset verbatim.
type GetAssetResponse struct {
	Status int
	Body   *mmodel.Asset
}

// GetAssetByID delegates to getAssetByID.
func (handler *AssetHandler) GetAssetByID(ctx context.Context, in *GetAssetRequest) (*GetAssetResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	asset, err := handler.getAssetByID(ctx, orgID, ledgerID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetAssetResponse{Status: http.StatusOK, Body: asset}, nil
}

// --- PATCH /assets/{id} -------------------------------------------------------

// UpdateAssetRequest is the update request envelope (RawBody, see Create).
type UpdateAssetRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Asset ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateAssetResponse carries the updated asset (200, matching http.OK).
type UpdateAssetResponse struct {
	Status int
	Body   *mmodel.Asset
}

// UpdateAsset decodes+validates the raw body imperatively then delegates to the
// shared updateAsset core.
func (handler *AssetHandler) UpdateAsset(ctx context.Context, in *UpdateAssetRequest) (*UpdateAssetResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdateAssetInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	asset, err := handler.updateAsset(ctx, orgID, ledgerID, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateAssetResponse{Status: http.StatusOK, Body: asset}, nil
}

// --- DELETE /assets/{id} ------------------------------------------------------

// DeleteAssetResponse has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteAssetResponse struct{}

// DeleteAssetByID delegates to deleteAsset; returns a bodiless 204 on success.
func (handler *AssetHandler) DeleteAssetByID(ctx context.Context, in *GetAssetRequest) (*DeleteAssetResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteAsset(ctx, orgID, ledgerID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteAssetResponse{}, nil
}

// --- HEAD /assets/metrics/count -----------------------------------------------

// CountAssetsRequest is the HEAD-count request envelope (org+ledger only).
type CountAssetsRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
}

// CountAssetsResponse replicates the Fiber HEAD-count response manually: the
// X-Total-Count header carries the count, Content-Length is pinned to 0, and the
// body is empty at status 204. Huma serializes the header from the struct tag; the
// DefaultStatus 204 + no Body field yields the bodiless response.
type CountAssetsResponse struct {
	TotalCount    string `header:"X-Total-Count"`
	ContentLength string `header:"Content-Length"`
}

// CountAssets delegates to countAssets and sets the count headers.
func (handler *AssetHandler) CountAssets(ctx context.Context, in *CountAssetsRequest) (*CountAssetsResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	count, err := handler.countAssets(ctx, orgID, ledgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CountAssetsResponse{
		TotalCount:    strconv.FormatInt(count, 10),
		ContentLength: "0",
	}, nil
}
