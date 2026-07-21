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

// secAssetBearerOrAPIKey advertises that each asset operation accepts EITHER a JWT
// bearer token OR an X-API-Key (two entries = OR). SPEC metadata only; runtime auth
// is the Fiber guard chain. The scheme names are declared once on the shared Huma
// API in unified-server.go (BearerAuth via openapi.DeclareBearerAuth, ApiKeyAuth
// via the local nil-guarded assignment).
var secAssetBearerOrAPIKey = []map[string][]string{
	{"BearerAuth": {}},
	{"ApiKeyAuth": {}},
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
// / 0065) so a bad path param yields the canonical 400 identical to the Fiber path.
func parsePathUUID(value, key string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", key)
	}

	return id, nil
}

// --- POST /assets -------------------------------------------------------------

// CreateAssetInputHuma is the Huma request envelope for POST. RawBody keeps the
// body out of Huma's validator (see file header); the org+ledger path params are
// validated by the Fiber middleware, not by a format tag.
type CreateAssetInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Authorization  string `header:"Authorization" doc:"Bearer token (forwarded to the service)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateAssetOutputHuma pins 201 (matching http.Created).
type CreateAssetOutputHuma struct {
	Status int
	Body   *mmodel.Asset
}

// CreateAssetHuma decodes+validates the raw body imperatively then delegates to the
// shared createAsset core.
func (handler *AssetHandler) CreateAssetHuma(ctx context.Context, in *CreateAssetInputHuma) (*CreateAssetOutputHuma, error) {
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

	return &CreateAssetOutputHuma{Status: http.StatusCreated, Body: asset}, nil
}

// --- GET /assets (list) -------------------------------------------------------

// ListAssetsInputHuma advertises the list query params in the spec (doc-only, no
// validation tags) and captures the raw query via Resolve for the imperative
// http.ValidateParameters binder.
type ListAssetsInputHuma struct {
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
func (in *ListAssetsInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key, present-but-
// empty keys included).
func (in *ListAssetsInputHuma) queries() map[string]string {
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

// ListAssetsOutputHuma carries the pagination envelope verbatim.
type ListAssetsOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListAssetsHuma binds the query imperatively then delegates to getAllAssets.
func (handler *AssetHandler) ListAssetsHuma(ctx context.Context, in *ListAssetsInputHuma) (*ListAssetsOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllAssets(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListAssetsOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /assets/{id} ---------------------------------------------------------

// GetAssetInputHuma is the by-id request envelope. The id path param carries no
// format tag (ParseUUIDPathParameters is the sole validator).
type GetAssetInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Asset ID (UUID)"`
}

// GetAssetOutputHuma carries the asset verbatim.
type GetAssetOutputHuma struct {
	Status int
	Body   *mmodel.Asset
}

// GetAssetByIDHuma delegates to getAssetByID.
func (handler *AssetHandler) GetAssetByIDHuma(ctx context.Context, in *GetAssetInputHuma) (*GetAssetOutputHuma, error) {
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

	return &GetAssetOutputHuma{Status: http.StatusOK, Body: asset}, nil
}

// --- PATCH /assets/{id} -------------------------------------------------------

// UpdateAssetInputHuma is the update request envelope (RawBody, see Create).
type UpdateAssetInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Asset ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateAssetOutputHuma carries the updated asset (200, matching http.OK).
type UpdateAssetOutputHuma struct {
	Status int
	Body   *mmodel.Asset
}

// UpdateAssetHuma decodes+validates the raw body imperatively then delegates to the
// shared updateAsset core.
func (handler *AssetHandler) UpdateAssetHuma(ctx context.Context, in *UpdateAssetInputHuma) (*UpdateAssetOutputHuma, error) {
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

	return &UpdateAssetOutputHuma{Status: http.StatusOK, Body: asset}, nil
}

// --- DELETE /assets/{id} ------------------------------------------------------

// DeleteAssetOutputHuma has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteAssetOutputHuma struct{}

// DeleteAssetByIDHuma delegates to deleteAsset; returns a bodiless 204 on success.
func (handler *AssetHandler) DeleteAssetByIDHuma(ctx context.Context, in *GetAssetInputHuma) (*DeleteAssetOutputHuma, error) {
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

	return &DeleteAssetOutputHuma{}, nil
}

// --- HEAD /assets/metrics/count -----------------------------------------------

// CountAssetsInputHuma is the HEAD-count request envelope (org+ledger only).
type CountAssetsInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
}

// CountAssetsOutputHuma replicates the Fiber HEAD-count response manually: the
// X-Total-Count header carries the count, Content-Length is pinned to 0, and the
// body is empty at status 204. Huma serializes the header from the struct tag; the
// DefaultStatus 204 + no Body field yields the bodiless response.
type CountAssetsOutputHuma struct {
	TotalCount    string `header:"X-Total-Count"`
	ContentLength string `header:"Content-Length"`
}

// CountAssetsHuma delegates to countAssets and sets the count headers.
func (handler *AssetHandler) CountAssetsHuma(ctx context.Context, in *CountAssetsInputHuma) (*CountAssetsOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	count, err := handler.countAssets(ctx, orgID, ledgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CountAssetsOutputHuma{
		TotalCount:    strconv.FormatInt(count, 10),
		ContentLength: "0",
	}, nil
}

// RegisterAssetRoutes registers the six migrated asset operations on the shared
// Huma API. It is the per-file seam unified-server.go calls; the auth + tenant +
// ParseUUIDPathParameters middleware chain for these routes is attached in
// unified-server.go (Fiber-level) BEFORE the Huma terminal, not here.
//
// Paths are GROUP-RELATIVE: the Huma API is bound to the /v1 Fiber group, so the
// humafiber adapter registers on that group and Fiber prepends /v1. The /v1 prefix
// rides the OpenAPI `servers` entry (openapi.New Config), keeping op paths relative.
func RegisterAssetRoutes(api huma.API, h *AssetHandler) {
	const (
		listPath  = "/organizations/{organization_id}/ledgers/{ledger_id}/assets"
		idPath    = listPath + "/{id}"
		countPath = listPath + "/metrics/count"
		tag       = "Assets"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createAsset",
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create a new asset",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
	}, h.CreateAssetHuma)

	huma.Register(api, huma.Operation{
		OperationID: "listAssets",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all assets",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
	}, h.ListAssetsHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getAssetByID",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific asset",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
	}, h.GetAssetByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateAsset",
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an asset",
		Tags:             []string{tag},
		Security:         secAssetBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively — see createAsset.
	}, h.UpdateAssetHuma)

	huma.Register(api, huma.Operation{
		OperationID: "deleteAsset",
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete an asset",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteAssetByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID: "countAssets",
		Method:      http.MethodHead,
		Path:        countPath,
		Summary:     "Count total assets",
		Tags:        []string{tag},
		Security:    secAssetBearerOrAPIKey,
		// HEAD count: X-Total-Count header + empty 204 body (Content-Length 0 set
		// on the Out struct), matching the Fiber http.NoContent + header path.
		DefaultStatus: http.StatusNoContent,
	}, h.CountAssetsHuma)
}
