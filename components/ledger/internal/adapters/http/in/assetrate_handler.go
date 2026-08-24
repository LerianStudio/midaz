// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/assetrate"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file serves the asset-rate resource over Huma. It mirrors the asset exemplar
// (asset_handler.go), adapted to asset-rate's three operations: a PUT
// create-or-upsert, a GET by external id, and a cursor-paginated GET list keyed by a
// free-form asset code. assetrate is MONEY-adjacent (exchange rates). The same five
// conventions the asset exemplar documents apply here:
//
//  1. Path params carry ONLY `doc:` (no `format:"uuid"`) — the ParseUUIDPathParameters
//     Fiber middleware is the sole UUID validator, yielding the canonical 400 / 0065.
//     The asset_code segment is NOT a UUID (not in constant.UUIDPathParameters) and
//     is passed through as a free-form string.
//  2. The body op carries RawBody []byte + SkipValidateBody so the imperative
//     http.DecodeAndValidate stays the sole body validator — never a native Huma 422.
//  3. List captures the raw query (via Resolve) and rebuilds the map[string]string
//     that http.ValidateParameters consumes, byte-identical to c.Queries().
//  4. Errors go through the shared pkgHTTP.HumaProblem (RFC 9457 problem+json).
//  5. Auth stays the Fiber middleware chain (protectedMidaz("midaz","asset-rates",verb)
//     + tenant + ParseUUIDPathParameters("asset-rate")) attached in the routes wiring
//     BEFORE the Huma terminal — NOT a Huma Security scheme. The per-op Security
//     metadata below is SPEC-ONLY.
//
// parseOrgLedger / parsePathUUID are the shared helpers defined in
// asset_handler.go (same package) — reused here, not redefined.

// secAssetRateBearer advertises that each asset-rate operation accepts a JWT bearer
// token. SPEC metadata only; runtime auth is the Fiber guard chain.
var secAssetRateBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- PUT /asset-rates ---------------------------------------------------------

// CreateAssetRateRequest is the Huma request envelope for PUT. RawBody keeps the
// body out of Huma's validator; org+ledger are validated by the Fiber middleware.
type CreateAssetRateRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateAssetRateResponse pins 201, which the op returns for both create and
// upsert.
type CreateAssetRateResponse struct {
	Status int
	Body   *assetrate.AssetRate
}

// CreateOrUpdateAssetRate decodes+validates the raw body imperatively then
// delegates to the shared createOrUpdateAssetRate core.
func (handler *AssetRateHandler) CreateOrUpdateAssetRate(ctx context.Context, in *CreateAssetRateRequest) (*CreateAssetRateResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(assetrate.CreateAssetRateInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	assetRate, err := handler.createOrUpdateAssetRate(ctx, orgID, ledgerID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateAssetRateResponse{Status: http.StatusCreated, Body: assetRate}, nil
}

// --- GET /asset-rates/{external_id} -------------------------------------------

// GetAssetRateByExternalIDRequest is the by-external-id request envelope. The
// external_id path param carries no format tag (ParseUUIDPathParameters is the sole
// validator — external_id IS in constant.UUIDPathParameters).
type GetAssetRateByExternalIDRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ExternalID     string `path:"external_id" doc:"External ID (UUID)"`
}

// GetAssetRateByExternalIDResponse carries the asset rate verbatim.
type GetAssetRateByExternalIDResponse struct {
	Status int
	Body   *assetrate.AssetRate
}

// GetAssetRateByExternalID delegates to getAssetRateByExternalID.
func (handler *AssetRateHandler) GetAssetRateByExternalID(ctx context.Context, in *GetAssetRateByExternalIDRequest) (*GetAssetRateByExternalIDResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	externalID, err := parsePathUUID(in.ExternalID, "external_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	assetRate, err := handler.getAssetRateByExternalID(ctx, orgID, ledgerID, externalID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetAssetRateByExternalIDResponse{Status: http.StatusOK, Body: assetRate}, nil
}

// --- GET /asset-rates/from/{asset_code} (list) --------------------------------

// ListAssetRatesByAssetCodeRequest advertises the list query params in the spec
// (doc-only, no validation tags) and captures the raw query via Resolve for the
// imperative http.ValidateParameters binder. asset_code is a free-form string path
// segment (NOT a UUID).
type ListAssetRatesByAssetCodeRequest struct {
	OrganizationID string   `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string   `path:"ledger_id" doc:"Ledger ID (UUID)"`
	AssetCode      string   `path:"asset_code" doc:"Source asset code"`
	To             []string `query:"to" doc:"Filter by destination asset codes"`
	Limit          string   `query:"limit" doc:"Max items per page (1-100, default 10)"`
	StartDate      string   `query:"start_date" doc:"Filter asset rates created on/after this date (YYYY-MM-DD)"`
	EndDate        string   `query:"end_date" doc:"Filter asset rates created on/before this date (YYYY-MM-DD)"`
	SortOrder      string   `query:"sort_order" doc:"Sort direction (asc, desc)"`
	Cursor         string   `query:"cursor" doc:"Opaque cursor token for pagination"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in http.ValidateParameters.
func (in *ListAssetRatesByAssetCodeRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key, present-but-
// empty keys included).
func (in *ListAssetRatesByAssetCodeRequest) queries() map[string]string {
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

// ListAssetRatesByAssetCodeResponse carries the pagination envelope verbatim.
type ListAssetRatesByAssetCodeResponse struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListAssetRatesByAssetCode binds the query imperatively then delegates to
// getAllAssetRatesByAssetCode.
func (handler *AssetRateHandler) ListAssetRatesByAssetCode(ctx context.Context, in *ListAssetRatesByAssetCodeRequest) (*ListAssetRatesByAssetCodeResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllAssetRatesByAssetCode(ctx, orgID, ledgerID, in.AssetCode, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListAssetRatesByAssetCodeResponse{Status: http.StatusOK, Body: pagination}, nil
}
