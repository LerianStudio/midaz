// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the ledger resource, mirroring the
// proven asset exemplar (asset_handler.go) adapted to the ledger's two-level
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

// CreateLedgerRequest is the Huma request envelope for POST. RawBody keeps the
// body out of Huma's validator; the org path param is validated by the Fiber
// middleware, not by a format tag.
type CreateLedgerRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateLedgerResponse pins 201 (matching http.Created).
type CreateLedgerResponse struct {
	Status int
	Body   *mmodel.Ledger
}

// CreateLedger decodes+validates the raw body imperatively then delegates to the
// shared createLedger core.
func (handler *LedgerHandler) CreateLedger(ctx context.Context, in *CreateLedgerRequest) (*CreateLedgerResponse, error) {
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

	return &CreateLedgerResponse{Status: http.StatusCreated, Body: ledger}, nil
}

// --- GET /ledgers (list) ------------------------------------------------------

// ListLedgersRequest advertises the list query params in the spec (doc-only, no
// validation tags) and captures the raw query via Resolve for the imperative binder
// in the getAllLedgers core.
type ListLedgersRequest struct {
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
func (in *ListLedgersRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string the getAllLedgers core feeds to
// http.ValidateParameters, matching Fiber's c.Queries() (last value wins for a
// repeated key, present-but-empty keys included).
func (in *ListLedgersRequest) queries() map[string]string {
	return queriesFromValues(in.rawQuery)
}

// ListLedgersResponse carries the pagination envelope verbatim.
type ListLedgersResponse struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListLedgers delegates to getAllLedgers (which binds+validates the query,
// enforces the status allowlist and the metadata/name-filter exclusion).
func (handler *LedgerHandler) ListLedgers(ctx context.Context, in *ListLedgersRequest) (*ListLedgersResponse, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllLedgers(ctx, orgID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListLedgersResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /ledgers/{ledger_id} -------------------------------------------------

// GetLedgerRequest is the by-id request envelope. The ledger_id path param carries
// no format tag (ParseUUIDPathParameters is the sole validator).
type GetLedgerRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
}

// GetLedgerResponse carries the ledger verbatim.
type GetLedgerResponse struct {
	Status int
	Body   *mmodel.Ledger
}

// GetLedgerByID delegates to getLedgerByID.
func (handler *LedgerHandler) GetLedgerByID(ctx context.Context, in *GetLedgerRequest) (*GetLedgerResponse, error) {
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

	return &GetLedgerResponse{Status: http.StatusOK, Body: ledger}, nil
}

// --- PATCH /ledgers/{ledger_id} -----------------------------------------------

// UpdateLedgerRequest is the update request envelope (RawBody, see Create).
type UpdateLedgerRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateLedgerResponse carries the updated ledger (200, matching http.OK).
type UpdateLedgerResponse struct {
	Status int
	Body   *mmodel.Ledger
}

// UpdateLedger decodes+validates the raw body imperatively then delegates to the
// shared updateLedger core.
func (handler *LedgerHandler) UpdateLedger(ctx context.Context, in *UpdateLedgerRequest) (*UpdateLedgerResponse, error) {
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

	return &UpdateLedgerResponse{Status: http.StatusOK, Body: ledger}, nil
}

// --- DELETE /ledgers/{ledger_id} ----------------------------------------------

// DeleteLedgerResponse has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteLedgerResponse struct{}

// DeleteLedgerByID delegates to deleteLedger (which enforces the production-env
// guard); returns a bodiless 204 on success.
func (handler *LedgerHandler) DeleteLedgerByID(ctx context.Context, in *GetLedgerRequest) (*DeleteLedgerResponse, error) {
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

	return &DeleteLedgerResponse{}, nil
}

// --- HEAD /ledgers/metrics/count ----------------------------------------------

// CountLedgersRequest is the HEAD-count request envelope (org only).
type CountLedgersRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
}

// CountLedgersResponse replicates the Fiber HEAD-count response manually: the
// X-Total-Count header carries the count, Content-Length is pinned to 0, and the
// body is empty at status 204.
type CountLedgersResponse struct {
	TotalCount    string `header:"X-Total-Count"`
	ContentLength string `header:"Content-Length"`
}

// CountLedgers delegates to countLedgers and sets the count headers.
func (handler *LedgerHandler) CountLedgers(ctx context.Context, in *CountLedgersRequest) (*CountLedgersResponse, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	count, err := handler.countLedgers(ctx, orgID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CountLedgersResponse{
		TotalCount:    strconv.FormatInt(count, 10),
		ContentLength: "0",
	}, nil
}

// --- GET /ledgers/{ledger_id}/settings ----------------------------------------

// GetLedgerSettingsResponse carries the parsed settings (200, matching http.OK).
type GetLedgerSettingsResponse struct {
	Status int
	Body   mmodel.LedgerSettings
}

// GetLedgerSettings delegates to getLedgerSettings.
func (handler *LedgerHandler) GetLedgerSettings(ctx context.Context, in *GetLedgerRequest) (*GetLedgerSettingsResponse, error) {
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

	return &GetLedgerSettingsResponse{Status: http.StatusOK, Body: settings}, nil
}

// --- PATCH /ledgers/{ledger_id}/settings --------------------------------------

// UpdateLedgerSettingsRequest is the settings merge-patch request envelope. The
// body is a free-form JSON object (map[string]any), NOT a validated struct — the
// allowlist enforcement (unknown field -> 0147, wrong type -> 0148) lives in the
// updateLedgerSettings core. RawBody keeps it out of Huma's validator; the imperative
// pipeline preserves the null-byte/depth/key-count guards.
type UpdateLedgerSettingsRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateLedgerSettingsResponse carries the parsed merged settings (200).
type UpdateLedgerSettingsResponse struct {
	Status int
	Body   mmodel.LedgerSettings
}

// UpdateLedgerSettings preserves the Fiber settings landmine byte-for-byte:
// the 64KB body-limit guard (ErrPayloadTooLarge / 0143, mirroring the
// WithBodyLimit(SettingsMaxPayloadSize) middleware), the shared imperative decode
// pipeline into a map[string]any (null-byte/depth/key-count guards), then the
// allowlist merge-patch in the core. Every rejection is a canonical 400 rendered by
// HumaProblem — never a native Huma 422.
func (handler *LedgerHandler) UpdateLedgerSettings(ctx context.Context, in *UpdateLedgerSettingsRequest) (*UpdateLedgerSettingsResponse, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.LedgerID, "ledger_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	// Preserves the canonical 0143 (400) the Fiber WithBodyLimit middleware raised
	// for oversized settings bodies. This is response-contract parity only, NOT
	// pre-buffering protection: NewUnifiedServer leaves Fiber's default body limit
	// in place and Huma has already buffered RawBody by the time this runs.
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

	return &UpdateLedgerSettingsResponse{Status: http.StatusOK, Body: updatedSettings}, nil
}

// RegisterLedgerRoutes registers the eight ledger operations on the shared Huma API.
// It is the per-file seam registerLedgerRoutesToApp calls; the auth + tenant +
// ParseUUIDPathParameters middleware chain for these routes is attached at the Fiber
// level BEFORE the Huma terminal, not here.
//
// Paths are GROUP-RELATIVE: the Huma API is bound to a versioned Fiber group, so the
// humafiber adapter registers on that group and Fiber prepends the version prefix.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. A straight v1/v2 mirror reuses the same handler methods and the
// same input/output types, so only the operation IDs differ between the twins.
func RegisterLedgerRoutes(api huma.API, h *LedgerHandler, opSuffix string) {
	const (
		listPath     = "/organizations/{organization_id}/ledgers"
		idPath       = listPath + "/{ledger_id}"
		countPath    = listPath + "/metrics/count"
		settingsPath = idPath + "/settings"
		tag          = "Ledgers"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createLedger" + opSuffix,
		Method:           http.MethodPost,
		Path:             listPath,
		Summary:          "Create a new ledger",
		Tags:             []string{tag},
		Security:         secLedgerBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively (http.DecodeAndValidate).
		DefaultStatus:    http.StatusCreated,
	}, h.CreateLedger)
	attachTypedRequestBody[mmodel.CreateLedgerInput](api, "createLedger"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listLedgers" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all ledgers",
		Tags:        []string{tag},
		Security:    secLedgerBearerOrAPIKey,
	}, h.ListLedgers)

	huma.Register(api, huma.Operation{
		OperationID: "getLedgerByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific ledger",
		Tags:        []string{tag},
		Security:    secLedgerBearerOrAPIKey,
	}, h.GetLedgerByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updateLedger" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an existing ledger",
		Tags:             []string{tag},
		Security:         secLedgerBearerOrAPIKey,
		SkipValidateBody: true, // body validated imperatively — see createLedger.
	}, h.UpdateLedger)
	attachTypedRequestBody[mmodel.UpdateLedgerInput](api, "updateLedger"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteLedger" + opSuffix,
		Method:        http.MethodDelete,
		Path:          idPath,
		Summary:       "Delete a ledger",
		Tags:          []string{tag},
		Security:      secLedgerBearerOrAPIKey,
		DefaultStatus: http.StatusNoContent, // Out has no Body field => bodiless 204.
	}, h.DeleteLedgerByID)

	huma.Register(api, huma.Operation{
		OperationID:   "countLedgers" + opSuffix,
		Method:        http.MethodHead,
		Path:          countPath,
		Summary:       "Count total ledgers",
		Tags:          []string{tag},
		Security:      secLedgerBearerOrAPIKey,
		DefaultStatus: http.StatusNoContent, // X-Total-Count header + empty 204 body.
	}, h.CountLedgers)

	huma.Register(api, huma.Operation{
		OperationID: "getLedgerSettings" + opSuffix,
		Method:      http.MethodGet,
		Path:        settingsPath,
		Summary:     "Get ledger settings",
		Tags:        []string{tag},
		Security:    secLedgerBearerOrAPIKey,
	}, h.GetLedgerSettings)

	huma.Register(api, huma.Operation{
		OperationID:      "updateLedgerSettings" + opSuffix,
		Method:           http.MethodPatch,
		Path:             settingsPath,
		Summary:          "Update ledger settings",
		Tags:             []string{tag},
		Security:         secLedgerBearerOrAPIKey,
		SkipValidateBody: true, // free-form map; allowlist enforced imperatively in the core.
	}, h.UpdateLedgerSettings)
	// updateLedgerSettings decodes into a free-form map[string]any (allowlist enforced
	// in the core), so the published schema is a structured object, not a $ref component.
	attachTypedRequestBody[map[string]any](api, "updateLedgerSettings"+opSuffix)
}

// RegisterLedgerRoutesToApp wires the ledger surface onto the /v1
// contract. See registerLedgerRoutesToApp for what it attaches.
func RegisterLedgerRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *LedgerHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerLedgerRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV1)
}

// RegisterLedgerV2RoutesToApp wires the same ledger surface onto the /v2 contract: same
// paths, same handlers, same authz tuples and tenant chain, differing only in the operation
// IDs the contract publishes. It is additive — /v1 keeps serving ledgers in parallel — and
// introduces no new policy surface.
func RegisterLedgerV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *LedgerHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerLedgerRoutesToApp(group, api, auth, h, routeOptions, routeOpSuffixV2)
}

// registerLedgerRoutesToApp is the single description of the ledger route surface, shared by
// every versioned contract that serves it, mirroring RegisterAssetRoutesToApp. For each of
// the eight ops it attaches the Fiber auth chain — protectedMidaz(auth,"ledgers",verb) (=
// auth.Authorize("midaz","ledgers",verb) + tenant PostAuthMiddlewares) +
// ParseUUIDPathParameters("ledger") — as MIDDLEWARE ONLY (no terminal) on the VERSIONED
// GROUP with GROUP-RELATIVE paths, then registers the Huma terminals via RegisterLedgerRoutes
// on the SAME group's Huma API. The ("ledgers", verb) authz tuples and tenant resolution
// therefore apply on whichever version group it is mounted on — no ledger route becomes
// public. Every one of the eight ops carries ParseUUIDPathParameters("ledger"). Body
// handling is owned by the Huma terminal, and the
// body limit was never an authz concern.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's — see
// routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface reaches
// every version it is mounted on.
func registerLedgerRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *LedgerHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath     = "/organizations/:organization_id/ledgers"
		idPath       = listPath + "/:ledger_id"
		countPath    = listPath + "/metrics/count"
		settingsPath = idPath + "/settings"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("ledger")

	routePost(group, listPath, protectedMidaz(auth, "ledgers", "post", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "ledgers", "patch", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "ledgers", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "ledgers", "get", routeOptions, parse))
	routeGet(group, settingsPath, protectedMidaz(auth, "ledgers", "get", routeOptions, parse))
	routePatch(group, settingsPath, protectedMidaz(auth, "ledgers", "patch", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "ledgers", "delete", routeOptions, parse))
	routeHead(group, countPath, protectedMidaz(auth, "ledgers", "head", routeOptions, parse))

	RegisterLedgerRoutes(api, h, opSuffix)
}
