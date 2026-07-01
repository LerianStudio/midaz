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

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/assetrate"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the asset-rate resource. It mirrors
// the proven asset exemplar (asset_handler_huma.go), adapted to asset-rate's three
// operations: a PUT create-or-upsert, a GET by external id, and a cursor-paginated
// GET list keyed by a free-form asset code. assetrate is MONEY-adjacent (exchange
// rates), so every op stays code+status byte-identical to the pre-migration Fiber
// path. The same five conventions the asset exemplar documents apply here:
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
// asset_handler_huma.go (same package) — reused here, not redefined.

// secAssetRateBearerOrAPIKey advertises that each asset-rate operation accepts EITHER
// a JWT bearer token OR an X-API-Key (two entries = OR). SPEC metadata only; runtime
// auth is the Fiber guard chain.
var secAssetRateBearerOrAPIKey = []map[string][]string{
	{"BearerAuth": {}},
	{"ApiKeyAuth": {}},
}

// --- PUT /asset-rates ---------------------------------------------------------

// CreateAssetRateInputHuma is the Huma request envelope for PUT. RawBody keeps the
// body out of Huma's validator; org+ledger are validated by the Fiber middleware.
type CreateAssetRateInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateAssetRateOutputHuma pins 201 (matching http.Created — the Fiber path returns
// 201 for both create and upsert).
type CreateAssetRateOutputHuma struct {
	Status int
	Body   *assetrate.AssetRate
}

// CreateOrUpdateAssetRateHuma decodes+validates the raw body imperatively then
// delegates to the shared createOrUpdateAssetRate core.
func (handler *AssetRateHandler) CreateOrUpdateAssetRateHuma(ctx context.Context, in *CreateAssetRateInputHuma) (*CreateAssetRateOutputHuma, error) {
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

	return &CreateAssetRateOutputHuma{Status: http.StatusCreated, Body: assetRate}, nil
}

// --- GET /asset-rates/{external_id} -------------------------------------------

// GetAssetRateByExternalIDInputHuma is the by-external-id request envelope. The
// external_id path param carries no format tag (ParseUUIDPathParameters is the sole
// validator — external_id IS in constant.UUIDPathParameters).
type GetAssetRateByExternalIDInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ExternalID     string `path:"external_id" doc:"External ID (UUID)"`
}

// GetAssetRateByExternalIDOutputHuma carries the asset rate verbatim.
type GetAssetRateByExternalIDOutputHuma struct {
	Status int
	Body   *assetrate.AssetRate
}

// GetAssetRateByExternalIDHuma delegates to getAssetRateByExternalID.
func (handler *AssetRateHandler) GetAssetRateByExternalIDHuma(ctx context.Context, in *GetAssetRateByExternalIDInputHuma) (*GetAssetRateByExternalIDOutputHuma, error) {
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

	return &GetAssetRateByExternalIDOutputHuma{Status: http.StatusOK, Body: assetRate}, nil
}

// --- GET /asset-rates/from/{asset_code} (list) --------------------------------

// ListAssetRatesByAssetCodeInputHuma advertises the list query params in the spec
// (doc-only, no validation tags) and captures the raw query via Resolve for the
// imperative http.ValidateParameters binder. asset_code is a free-form string path
// segment (NOT a UUID).
type ListAssetRatesByAssetCodeInputHuma struct {
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
func (in *ListAssetRatesByAssetCodeInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key, present-but-
// empty keys included).
func (in *ListAssetRatesByAssetCodeInputHuma) queries() map[string]string {
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

// ListAssetRatesByAssetCodeOutputHuma carries the pagination envelope verbatim.
type ListAssetRatesByAssetCodeOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListAssetRatesByAssetCodeHuma binds the query imperatively then delegates to
// getAllAssetRatesByAssetCode.
func (handler *AssetRateHandler) ListAssetRatesByAssetCodeHuma(ctx context.Context, in *ListAssetRatesByAssetCodeInputHuma) (*ListAssetRatesByAssetCodeOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllAssetRatesByAssetCode(ctx, orgID, ledgerID, in.AssetCode, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListAssetRatesByAssetCodeOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// RegisterAssetRateRoutes registers the three migrated asset-rate operations on the
// shared Huma API. It is the per-file seam the routes wiring calls; the auth +
// tenant + ParseUUIDPathParameters middleware chain for these routes is attached at
// the Fiber level BEFORE the Huma terminal, not here.
//
// Paths are GROUP-RELATIVE: the Huma API is bound to the /v1 Fiber group, so the
// humafiber adapter registers on that group and Fiber prepends /v1.
func RegisterAssetRateRoutes(api huma.API, h *AssetRateHandler) {
	const (
		basePath     = "/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates"
		externalPath = basePath + "/{external_id}"
		fromPath     = basePath + "/from/{asset_code}"
		tag          = "Asset Rates"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createOrUpdateAssetRate",
		Method:      http.MethodPut,
		Path:        basePath,
		Summary:     "Create or Update an AssetRate",
		Tags:        []string{tag},
		Security:    secAssetRateBearerOrAPIKey,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
	}, h.CreateOrUpdateAssetRateHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getAssetRateByExternalID",
		Method:      http.MethodGet,
		Path:        externalPath,
		Summary:     "Get an AssetRate by External ID",
		Tags:        []string{tag},
		Security:    secAssetRateBearerOrAPIKey,
	}, h.GetAssetRateByExternalIDHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getAllAssetRatesByAssetCode",
		Method:      http.MethodGet,
		Path:        fromPath,
		Summary:     "Get an AssetRate by the Asset Code",
		Tags:        []string{tag},
		Security:    secAssetRateBearerOrAPIKey,
	}, h.ListAssetRatesByAssetCodeHuma)
}

// RegisterAssetRateRoutesToApp wires the Huma-migrated asset-rate resource. For each
// of the three ops it attaches the Fiber auth chain — protectedMidaz(auth,"asset-rates",
// verb) (= auth.Authorize("midaz","asset-rates",verb) + tenant PostAuthMiddlewares) +
// ParseUUIDPathParameters("asset-rate") — as MIDDLEWARE ONLY (no terminal) on the /v1
// GROUP with GROUP-RELATIVE paths, then registers the Huma terminals via
// RegisterAssetRateRoutes on the SAME group's Huma API. This preserves the pre-Huma
// ("asset-rates", verb) authz tuples and tenant resolution BYTE-FOR-BYTE (the plural
// resource + the "asset-rate" entity-name for ParseUUIDPathParameters, exactly as in
// the pre-migration routes.go) — no asset-rate route becomes public. asset-rate is
// MONEY-adjacent (exchange rates). Mirrors RegisterSegmentRoutesToApp; the integration
// task calls this from the unified server's humaMount seam.
func RegisterAssetRateRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AssetRateHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	const (
		basePath     = "/organizations/:organization_id/ledgers/:ledger_id/asset-rates"
		externalPath = basePath + "/:external_id"
		fromPath     = basePath + "/from/:asset_code"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("asset-rate")

	group.Put(basePath, protectedMidaz(auth, "asset-rates", "put", routeOptions, parse)...)
	group.Get(externalPath, protectedMidaz(auth, "asset-rates", "get", routeOptions, parse)...)
	group.Get(fromPath, protectedMidaz(auth, "asset-rates", "get", routeOptions, parse)...)

	RegisterAssetRateRoutes(api, h)
}
