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

// This file is the ledger's Huma adoption of the ledger resource, mirroring the
// proven asset exemplar (asset_handler_huma.go) adapted to the ledger's two-level
// path (org/ledger), its status/name-filtered offset pagination, its HEAD-count +
// DELETE-204 shapes, and its two /settings sub-resources. Conventions (identical to
// the asset exemplar — see its header for the full rationale):
//
//  1. Path params are plain strings with ONLY `doc:` (no `format:"uuid"`): the
//     ParseUUIDPathParameters Fiber middleware (attached before the Huma terminal)
//     is the sole UUID validator, yielding the canonical 400 / 0065.
//  2. Body ops carry RawBody []byte + SkipValidateBody so imperative
//     http.DecodeAndValidate stays the sole body validator — never a native 422.
//  3. List captures the raw query (via Resolve) and rebuilds the map[string]string
//     that the getAllLedgers core feeds to http.ValidateParameters, byte-identical
//     to the Fiber c.Queries() path (status allowlist + name-filter exclusion live
//     in the core).
//  4. Errors go through the shared pkgHTTP.HumaProblem (RFC 9457 problem+json,
//     field/status/code-identical to the Fiber http.WithError path).
//  5. Auth stays a Fiber middleware chain (auth.Authorize("midaz","ledgers",verb) +
//     tenant PostAuthMiddlewares + ParseUUIDPathParameters("ledger")) attached
//     BEFORE the Huma registration — NOT a Huma Security scheme. The per-op Security
//     metadata below is SPEC-ONLY (for the generated OAS/SDK).

// secLedgerBearerOrAPIKey advertises that each ledger operation accepts EITHER a JWT
// bearer token OR an X-API-Key (two entries = OR). SPEC metadata only; runtime auth
// is the Fiber guard chain.
var secLedgerBearerOrAPIKey = []map[string][]string{
	{"BearerAuth": {}},
	{"ApiKeyAuth": {}},
}

// parseOrg resolves the org path string to a UUID. On the wired path the
// ParseUUIDPathParameters middleware has already validated it, so this never
// errors; the canonical 0065 is returned defensively if it somehow does. (Reuses
// parsePathUUID from the asset exemplar — same package.)
func parseOrg(orgStr string) (orgID uuid.UUID, err error) {
	return parsePathUUID(orgStr, "organization_id")
}

// --- POST /ledgers ------------------------------------------------------------

// CreateLedgerInputHuma is the Huma request envelope for POST. RawBody keeps the
// body out of Huma's validator; the org path param is validated by the Fiber
// middleware, not by a format tag.
type CreateLedgerInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateLedgerOutputHuma pins 201 (matching http.Created).
type CreateLedgerOutputHuma struct {
	Status int
	Body   *mmodel.Ledger
}

// CreateLedgerHuma decodes+validates the raw body imperatively then delegates to the
// shared createLedger core.
func (handler *LedgerHandler) CreateLedgerHuma(ctx context.Context, in *CreateLedgerInputHuma) (*CreateLedgerOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreateLedgerInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	ledger, err := handler.createLedger(ctx, orgID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateLedgerOutputHuma{Status: http.StatusCreated, Body: ledger}, nil
}

// --- GET /ledgers (list) ------------------------------------------------------

// ListLedgersInputHuma advertises the list query params in the spec (doc-only, no
// validation tags) and captures the raw query via Resolve for the imperative binder
// in the getAllLedgers core.
type ListLedgersInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	Metadata       string `query:"metadata" doc:"JSON string to filter ledgers by metadata fields"`
	Limit          string `query:"limit" doc:"Max items per page (1-100, default 10)"`
	Page           string `query:"page" doc:"Page number (default 1)"`
	StartDate      string `query:"start_date" doc:"Filter ledgers created on/after this date (YYYY-MM-DD)"`
	EndDate        string `query:"end_date" doc:"Filter ledgers created on/before this date (YYYY-MM-DD)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`
	Name           string `query:"name" doc:"Filter ledgers by name (case-insensitive, prefix match)"`
	Status         string `query:"status" doc:"Filter ledgers by status (ACTIVE, INACTIVE)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in the getAllLedgers core.
func (in *ListLedgersInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string the getAllLedgers core feeds to
// http.ValidateParameters, matching Fiber's c.Queries() (last value wins for a
// repeated key, present-but-empty keys included).
func (in *ListLedgersInputHuma) queries() map[string]string {
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

// ListLedgersOutputHuma carries the pagination envelope verbatim.
type ListLedgersOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListLedgersHuma delegates to getAllLedgers (which binds+validates the query,
// enforces the status allowlist and the metadata/name-filter exclusion).
func (handler *LedgerHandler) ListLedgersHuma(ctx context.Context, in *ListLedgersInputHuma) (*ListLedgersOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllLedgers(ctx, orgID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListLedgersOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /ledgers/{ledger_id} -------------------------------------------------

// GetLedgerInputHuma is the by-id request envelope. The ledger_id path param carries
// no format tag (ParseUUIDPathParameters is the sole validator).
type GetLedgerInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
}

// GetLedgerOutputHuma carries the ledger verbatim.
type GetLedgerOutputHuma struct {
	Status int
	Body   *mmodel.Ledger
}

// GetLedgerByIDHuma delegates to getLedgerByID.
func (handler *LedgerHandler) GetLedgerByIDHuma(ctx context.Context, in *GetLedgerInputHuma) (*GetLedgerOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.LedgerID, "ledger_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	ledger, err := handler.getLedgerByID(ctx, orgID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetLedgerOutputHuma{Status: http.StatusOK, Body: ledger}, nil
}

// --- PATCH /ledgers/{ledger_id} -----------------------------------------------

// UpdateLedgerInputHuma is the update request envelope (RawBody, see Create).
type UpdateLedgerInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateLedgerOutputHuma carries the updated ledger (200, matching http.OK).
type UpdateLedgerOutputHuma struct {
	Status int
	Body   *mmodel.Ledger
}

// UpdateLedgerHuma decodes+validates the raw body imperatively then delegates to the
// shared updateLedger core.
func (handler *LedgerHandler) UpdateLedgerHuma(ctx context.Context, in *UpdateLedgerInputHuma) (*UpdateLedgerOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.LedgerID, "ledger_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdateLedgerInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	ledger, err := handler.updateLedger(ctx, orgID, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateLedgerOutputHuma{Status: http.StatusOK, Body: ledger}, nil
}

// --- DELETE /ledgers/{ledger_id} ----------------------------------------------

// DeleteLedgerOutputHuma has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteLedgerOutputHuma struct{}

// DeleteLedgerByIDHuma delegates to deleteLedger (which enforces the production-env
// guard); returns a bodiless 204 on success.
func (handler *LedgerHandler) DeleteLedgerByIDHuma(ctx context.Context, in *GetLedgerInputHuma) (*DeleteLedgerOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.LedgerID, "ledger_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteLedger(ctx, orgID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteLedgerOutputHuma{}, nil
}

// --- HEAD /ledgers/metrics/count ----------------------------------------------

// CountLedgersInputHuma is the HEAD-count request envelope (org only).
type CountLedgersInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
}

// CountLedgersOutputHuma replicates the Fiber HEAD-count response manually: the
// X-Total-Count header carries the count, Content-Length is pinned to 0, and the
// body is empty at status 204.
type CountLedgersOutputHuma struct {
	TotalCount    string `header:"X-Total-Count"`
	ContentLength string `header:"Content-Length"`
}

// CountLedgersHuma delegates to countLedgers and sets the count headers.
func (handler *LedgerHandler) CountLedgersHuma(ctx context.Context, in *CountLedgersInputHuma) (*CountLedgersOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	count, err := handler.countLedgers(ctx, orgID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CountLedgersOutputHuma{
		TotalCount:    strconv.FormatInt(count, 10),
		ContentLength: "0",
	}, nil
}

// --- GET /ledgers/{ledger_id}/settings ----------------------------------------

// GetLedgerSettingsOutputHuma carries the parsed settings (200, matching http.OK).
type GetLedgerSettingsOutputHuma struct {
	Status int
	Body   mmodel.LedgerSettings
}

// GetLedgerSettingsHuma delegates to getLedgerSettings.
func (handler *LedgerHandler) GetLedgerSettingsHuma(ctx context.Context, in *GetLedgerInputHuma) (*GetLedgerSettingsOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.LedgerID, "ledger_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	settings, err := handler.getLedgerSettings(ctx, orgID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetLedgerSettingsOutputHuma{Status: http.StatusOK, Body: settings}, nil
}

// --- PATCH /ledgers/{ledger_id}/settings --------------------------------------

// UpdateLedgerSettingsInputHuma is the settings merge-patch request envelope. The
// body is a free-form JSON object (map[string]any), NOT a validated struct — the
// allowlist enforcement (unknown field -> 0147, wrong type -> 0148) lives in the
// updateLedgerSettings core. RawBody keeps it out of Huma's validator; the imperative
// pipeline preserves the null-byte/depth/key-count guards.
type UpdateLedgerSettingsInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateLedgerSettingsOutputHuma carries the parsed merged settings (200).
type UpdateLedgerSettingsOutputHuma struct {
	Status int
	Body   mmodel.LedgerSettings
}

// UpdateLedgerSettingsHuma preserves the Fiber settings landmine byte-for-byte:
// the 64KB body-limit guard (ErrPayloadTooLarge / 0143, mirroring the
// WithBodyLimit(SettingsMaxPayloadSize) middleware), the shared imperative decode
// pipeline into a map[string]any (null-byte/depth/key-count guards), then the
// allowlist merge-patch in the core. Every rejection is a canonical 400 rendered by
// HumaProblem — never a native Huma 422.
func (handler *LedgerHandler) UpdateLedgerSettingsHuma(ctx context.Context, in *UpdateLedgerSettingsInputHuma) (*UpdateLedgerSettingsOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.LedgerID, "ledger_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	// Mirror WithBodyLimit(SettingsMaxPayloadSize): reject oversized settings bodies
	// with the same canonical 0143 the Fiber middleware raised (400).
	if len(in.RawBody) > SettingsMaxPayloadSize {
		return nil, pkgHTTP.HumaProblem(pkg.ValidateBusinessError(constant.ErrPayloadTooLarge, "request"))
	}

	// Decode into a map through the shared pipeline so the null-byte/depth/key-count
	// guards match the Fiber WithBody(new(map[string]any)) path. ValidateStruct is a
	// no-op for a map (only those guards run); the allowlist itself is in the core.
	settings := make(map[string]any)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, &settings); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	updatedSettings, err := handler.updateLedgerSettings(ctx, orgID, id, settings)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateLedgerSettingsOutputHuma{Status: http.StatusOK, Body: updatedSettings}, nil
}

// RegisterLedgerRoutes registers the eight migrated ledger operations on the shared
// Huma API. It is the per-file seam the unified server calls (via the deferred
// RegisterLedgerRoutesToApp integration wiring); the auth + tenant +
// ParseUUIDPathParameters middleware chain for these routes is attached at the
// Fiber level BEFORE the Huma terminal, not here.
//
// Paths are GROUP-RELATIVE: the Huma API is bound to the /v1 Fiber group, so Fiber
// prepends /v1. The /v1 prefix rides the OpenAPI `servers` entry, keeping op paths
// relative.
func RegisterLedgerRoutes(api huma.API, h *LedgerHandler) {
	const (
		listPath     = "/organizations/{organization_id}/ledgers"
		idPath       = listPath + "/{ledger_id}"
		countPath    = listPath + "/metrics/count"
		settingsPath = idPath + "/settings"
		tag          = "Ledgers"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createLedger",
		Method:           http.MethodPost,
		Path:             listPath,
		Summary:          "Create a new ledger",
		Tags:             []string{tag},
		Security:         secLedgerBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively (http.DecodeAndValidate).
	}, h.CreateLedgerHuma)

	huma.Register(api, huma.Operation{
		OperationID: "listLedgers",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all ledgers",
		Tags:        []string{tag},
		Security:    secLedgerBearerOrAPIKey,
	}, h.ListLedgersHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getLedgerByID",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific ledger",
		Tags:        []string{tag},
		Security:    secLedgerBearerOrAPIKey,
	}, h.GetLedgerByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateLedger",
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an existing ledger",
		Tags:             []string{tag},
		Security:         secLedgerBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively — see createLedger.
	}, h.UpdateLedgerHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteLedger",
		Method:        http.MethodDelete,
		Path:          idPath,
		Summary:       "Delete a ledger",
		Tags:          []string{tag},
		Security:      secLedgerBearerOrAPIKey,
		DefaultStatus: http.StatusNoContent, // Out has no Body field => bodiless 204.
	}, h.DeleteLedgerByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "countLedgers",
		Method:        http.MethodHead,
		Path:          countPath,
		Summary:       "Count total ledgers",
		Tags:          []string{tag},
		Security:      secLedgerBearerOrAPIKey,
		DefaultStatus: http.StatusNoContent, // X-Total-Count header + empty 204 body.
	}, h.CountLedgersHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getLedgerSettings",
		Method:      http.MethodGet,
		Path:        settingsPath,
		Summary:     "Get ledger settings",
		Tags:        []string{tag},
		Security:    secLedgerBearerOrAPIKey,
	}, h.GetLedgerSettingsHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateLedgerSettings",
		Method:           http.MethodPatch,
		Path:             settingsPath,
		Summary:          "Update ledger settings",
		Tags:             []string{tag},
		Security:         secLedgerBearerOrAPIKey,
		SkipValidateBody: true, // free-form map; allowlist enforced imperatively in the core.
	}, h.UpdateLedgerSettingsHuma)
}
