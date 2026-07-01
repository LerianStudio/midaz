// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the CRM instrument resource. It mirrors
// the asset exemplar (asset_handler_huma.go) and the holder sibling
// (holder_handler_huma.go); see those headers for the full conventions. Instrument-
// specific notes:
//
//  1. AUTH is appName "midaz" (crm_routes.go ApplicationName), resource "instruments".
//     The swaggo @Security on the Fiber wrappers is BearerAuth ONLY, so the per-op
//     Security metadata here is Bearer-only too — SPEC metadata only; runtime auth
//     stays the Fiber guard chain (auth.Authorize("midaz","instruments",verb) + tenant
//     + ParseUUIDPathParameters) attached BEFORE the Huma terminal.
//  2. Instruments are HOLDER-SCOPED for create/get/patch/delete (org+holder+instrument
//     in the path); the list is ORG-SCOPED (org only, holder is a query filter). The
//     related-party delete adds a fourth path segment (related_party_id) and is guarded
//     by ParseUUIDPathParameters("related-parties") in crm_routes.go — REPLICATED in the
//     test harness so its path-UUID validation matches production exactly.
//  3. POST carries the same idempotency dance as holder: the shell resolves the client
//     key + TTL from headers and projects the replayed flag the shared createInstrument
//     core returns onto the X-Idempotency-Replayed response header.
//  4. PATCH is RFC 7396 merge-patch: the shell derives fieldsToRemove from the parsed
//     body via http.FindNilFields (the same derivation http.WithBody feeds into the
//     Fiber patchRemove local) and passes it to the shared updateInstrument core.
//  5. GET-by-id, list and delete read include_deleted / hard_delete from the query,
//     matching the Fiber http.GetBooleanParam reads.
//  6. Body ops carry RawBody + SkipValidateBody so http.DecodeAndValidate is the sole
//     body validator (never a native Huma 422). Errors go through pkgHTTP.HumaProblem.

// secInstrumentBearer advertises that each instrument operation accepts a JWT bearer
// token (Bearer-only, matching the Fiber swaggo @Security BearerAuth). SPEC metadata
// only; runtime auth is the Fiber guard chain.
var secInstrumentBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /holders/{holder_id}/instruments ------------------------------------

// CreateInstrumentInputHuma is the Huma request envelope for POST. RawBody keeps the
// body out of Huma's validator; the idempotency headers are read so the shell can run
// the same claim the Fiber wrapper does.
type CreateInstrumentInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	HolderID       string `path:"holder_id" doc:"Holder ID (UUID)"`
	IdempotencyKey string `header:"X-Idempotency" doc:"Idempotency key to safely retry the create; an identical retry returns the original instrument"`
	IdempotencyTTL string `header:"X-TTL" doc:"Idempotency slot TTL in seconds (default 300)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateInstrumentOutputHuma pins 201 (matching http.Created) and carries the
// X-Idempotency-Replayed response header (parity with the Fiber c.Set).
type CreateInstrumentOutputHuma struct {
	Status              int
	IdempotencyReplayed string `header:"X-Idempotency-Replayed"`
	Body                *mmodel.Instrument
}

// CreateInstrumentHuma decodes+validates the raw body imperatively then delegates to
// the shared createInstrument core, resolving the idempotency key/TTL from headers and
// projecting the replayed flag onto the X-Idempotency-Replayed response header.
func (handler *InstrumentHandler) CreateInstrumentHuma(ctx context.Context, in *CreateInstrumentInputHuma) (*CreateInstrumentOutputHuma, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	holderID, err := parsePathUUID(in.HolderID, "holder_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreateInstrumentInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	ttl := pkgHTTP.ParseIdempotencyTTL(in.IdempotencyTTL)

	instrument, replayed, err := handler.createInstrument(ctx, orgID, holderID, payload, in.IdempotencyKey, ttl)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	replayedHeader := "false"
	if replayed {
		replayedHeader = "true"
	}

	return &CreateInstrumentOutputHuma{Status: http.StatusCreated, IdempotencyReplayed: replayedHeader, Body: instrument}, nil
}

// --- GET /holders/{holder_id}/instruments/{instrument_id} ---------------------

// GetInstrumentInputHuma is the by-id request envelope. The path params carry no
// format tag (ParseUUIDPathParameters is the sole validator); include_deleted is read
// via the query, matching the Fiber http.GetBooleanParam read.
type GetInstrumentInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	HolderID       string `path:"holder_id" doc:"Holder ID (UUID)"`
	InstrumentID   string `path:"instrument_id" doc:"Instrument ID (UUID)"`
	IncludeDeleted string `query:"include_deleted" doc:"Returns the instrument even if it was logically deleted (true,false)"`
}

// GetInstrumentOutputHuma carries the instrument verbatim.
type GetInstrumentOutputHuma struct {
	Status int
	Body   *mmodel.Instrument
}

// GetInstrumentByIDHuma delegates to getInstrumentByID.
func (handler *InstrumentHandler) GetInstrumentByIDHuma(ctx context.Context, in *GetInstrumentInputHuma) (*GetInstrumentOutputHuma, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	holderID, err := parsePathUUID(in.HolderID, "holder_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.InstrumentID, "instrument_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	instrument, err := handler.getInstrumentByID(ctx, orgID, holderID, id, strings.EqualFold(in.IncludeDeleted, "true"))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetInstrumentOutputHuma{Status: http.StatusOK, Body: instrument}, nil
}

// --- PATCH /holders/{holder_id}/instruments/{instrument_id} -------------------

// UpdateInstrumentInputHuma is the update request envelope (RawBody, see Create). The
// raw body is the sole source of the RFC 7396 null-field paths derived below.
type UpdateInstrumentInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	HolderID       string `path:"holder_id" doc:"Holder ID (UUID)"`
	InstrumentID   string `path:"instrument_id" doc:"Instrument ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdateInstrumentOutputHuma carries the updated instrument (200, matching http.OK).
type UpdateInstrumentOutputHuma struct {
	Status int
	Body   *mmodel.Instrument
}

// UpdateInstrumentHuma decodes+validates the raw body imperatively, derives the
// merge-patch null-field paths via http.FindNilFields (the same derivation
// http.WithBody feeds the Fiber patchRemove local), then delegates to the shared
// updateInstrument core.
func (handler *InstrumentHandler) UpdateInstrumentHuma(ctx context.Context, in *UpdateInstrumentInputHuma) (*UpdateInstrumentOutputHuma, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	holderID, err := parsePathUUID(in.HolderID, "holder_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.InstrumentID, "instrument_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdateInstrumentInput)

	originalMap, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	instrument, err := handler.updateInstrument(ctx, orgID, holderID, id, payload, pkgHTTP.FindNilFields(originalMap, ""))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateInstrumentOutputHuma{Status: http.StatusOK, Body: instrument}, nil
}

// --- DELETE /holders/{holder_id}/instruments/{instrument_id} ------------------

// DeleteInstrumentInputHuma is the delete request envelope; hard_delete is read from
// the query (matching the Fiber http.GetBooleanParam read).
type DeleteInstrumentInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	HolderID       string `path:"holder_id" doc:"Holder ID (UUID)"`
	InstrumentID   string `path:"instrument_id" doc:"Instrument ID (UUID)"`
	HardDelete     string `query:"hard_delete" doc:"Use only to perform a physical deletion of the data. This action is irreversible. (true,false)"`
}

// DeleteInstrumentOutputHuma has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteInstrumentOutputHuma struct{}

// DeleteInstrumentByIDHuma delegates to deleteInstrument; returns a bodiless 204 on success.
func (handler *InstrumentHandler) DeleteInstrumentByIDHuma(ctx context.Context, in *DeleteInstrumentInputHuma) (*DeleteInstrumentOutputHuma, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	holderID, err := parsePathUUID(in.HolderID, "holder_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.InstrumentID, "instrument_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteInstrument(ctx, orgID, holderID, id, strings.EqualFold(in.HardDelete, "true")); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteInstrumentOutputHuma{}, nil
}

// --- DELETE .../{instrument_id}/related-parties/{related_party_id} ------------

// DeleteRelatedPartyInputHuma is the related-party delete request envelope (four path
// params). Its path-UUID validation is ParseUUIDPathParameters("related-parties") in
// crm_routes.go, NOT "instruments" — the sole per-op UUID validator variance.
type DeleteRelatedPartyInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	HolderID       string `path:"holder_id" doc:"Holder ID (UUID)"`
	InstrumentID   string `path:"instrument_id" doc:"Instrument ID (UUID)"`
	RelatedPartyID string `path:"related_party_id" doc:"Related Party ID (UUID)"`
}

// DeleteRelatedPartyOutputHuma has NO Body field: bodiless 204, matching Fiber http.NoContent.
type DeleteRelatedPartyOutputHuma struct{}

// DeleteRelatedPartyHuma delegates to deleteRelatedParty; returns a bodiless 204 on success.
func (handler *InstrumentHandler) DeleteRelatedPartyHuma(ctx context.Context, in *DeleteRelatedPartyInputHuma) (*DeleteRelatedPartyOutputHuma, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	holderID, err := parsePathUUID(in.HolderID, "holder_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	instrumentID, err := parsePathUUID(in.InstrumentID, "instrument_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	relatedPartyID, err := parsePathUUID(in.RelatedPartyID, "related_party_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteRelatedParty(ctx, orgID, holderID, instrumentID, relatedPartyID); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteRelatedPartyOutputHuma{}, nil
}

// --- GET /instruments (list, org-scoped) --------------------------------------

// ListInstrumentsInputHuma advertises the list query params (doc-only, no validation
// tags) and captures the raw query via Resolve for the imperative binder + the
// include_deleted flag read. holder_id is a QUERY filter here (the list is org-scoped),
// not a path param.
type ListInstrumentsInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	HolderID       string `query:"holder_id" doc:"Filter instruments by holder ID (UUID)"`
	Metadata       string `query:"metadata" doc:"JSON string to filter instruments by metadata fields"`
	Limit          string `query:"limit" doc:"Max items per page (1-100, default 10)"`
	Page           string `query:"page" doc:"Page number (default 1)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`
	IncludeDeleted string `query:"include_deleted" doc:"Return includes logically deleted instruments (true,false)"`
	AccountID      string `query:"account_id" doc:"Filter instrument by accountID"`
	LedgerID       string `query:"ledger_id" doc:"Filter instrument by ledgerID"`
	Document       string `query:"document" doc:"Filter instrument by document"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in http.ValidateParameters.
func (in *ListInstrumentsInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// ListInstrumentsOutputHuma carries the pagination envelope verbatim.
type ListInstrumentsOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// GetAllInstrumentsHuma binds the query imperatively then delegates to getAllInstruments.
func (handler *InstrumentHandler) GetAllInstrumentsHuma(ctx context.Context, in *ListInstrumentsInputHuma) (*ListInstrumentsOutputHuma, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllInstruments(ctx, orgID, queriesFromValues(in.rawQuery), queryBool(in.rawQuery, "include_deleted"))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListInstrumentsOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// RegisterInstrumentRoutes registers the six migrated instrument operations on the
// shared Huma API. It is the per-file seam the unified server calls; the auth
// ("midaz","instruments",verb) + tenant + ParseUUIDPathParameters middleware chain is
// attached on the /v1 group (Fiber-level) BEFORE the Huma terminal, not here. The
// related-party delete uses ParseUUIDPathParameters("related-parties"); all others use
// "instruments" (see crm_routes.go). Paths are GROUP-RELATIVE (see
// asset_handler_huma.go's RegisterAssetRoutes header for the /v1 rationale).
func RegisterInstrumentRoutes(api huma.API, h *InstrumentHandler) {
	const (
		listPath     = "/organizations/{organization_id}/instruments"
		holderScoped = "/organizations/{organization_id}/holders/{holder_id}/instruments"
		idPath       = holderScoped + "/{instrument_id}"
		rpPath       = idPath + "/related-parties/{related_party_id}"
		tag          = "Instruments"
	)

	huma.Register(api, huma.Operation{
		OperationID: "listInstruments",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List Instruments",
		Tags:        []string{tag},
		Security:    secInstrumentBearer,
	}, h.GetAllInstrumentsHuma)

	huma.Register(api, huma.Operation{
		OperationID: "createInstrument",
		Method:      http.MethodPost,
		Path:        holderScoped,
		Summary:     "Create an Instrument Account",
		Tags:        []string{tag},
		Security:    secInstrumentBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
	}, h.CreateInstrumentHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getInstrumentByID",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve Instrument details",
		Tags:        []string{tag},
		Security:    secInstrumentBearer,
	}, h.GetInstrumentByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateInstrument",
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an Instrument",
		Tags:             []string{tag},
		Security:         secInstrumentBearer,
		SkipValidateBody: true, // body validated imperatively — RFC 7396 merge-patch core.
	}, h.UpdateInstrumentHuma)

	huma.Register(api, huma.Operation{
		OperationID: "deleteInstrument",
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete an Instrument",
		Tags:        []string{tag},
		Security:    secInstrumentBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteInstrumentByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteRelatedParty",
		Method:        http.MethodDelete,
		Path:          rpPath,
		Summary:       "Delete a Related Party",
		Tags:          []string{tag},
		Security:      secInstrumentBearer,
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteRelatedPartyHuma)
}
