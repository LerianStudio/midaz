// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the operation-route resource (Wave 2,
// money-read + routing). It mirrors the asset exemplar (asset_handler_huma.go); see
// that file's header for the full conventions. Operation-route-specific notes:
//
//  1. AUTH is the "routing" appName, resource "operation-routes" (protectedRouting
//     in routes.go), NOT "midaz". The Fiber guard chain is Bearer-only (no
//     X-API-Key), so the per-op Security metadata here is
//     Bearer-only too — this is SPEC metadata only; runtime auth stays the Fiber
//     guard chain (auth.Authorize("routing","operation-routes",verb) + tenant +
//     ParseUUIDPathParameters("operation_route")) attached in the unified server
//     BEFORE the Huma terminal.
//  2. MERGE-PATCH landmine: the PATCH core (updateOperationRoute in
//     operation_route.go) implements RFC 7396 JSON Merge Patch. It re-derives
//     AccountingEntriesRaw from the raw request bytes to tell accountingEntries
//     FIELD-ABSENT (keep existing) from accountingEntries:null (clear all) — a
//     distinction Go's typed decode collapses. The Huma shell MUST feed in.RawBody
//     to the core exactly as the Fiber wrapper feeds c.Body(), or the PATCH breaks
//     silently. POST carries the same rawBody for the accountingEntries unknown-key
//     probe. Both keep RawBody + SkipValidateBody so http.DecodeAndValidate is the
//     sole body validator (never a native Huma 422).
//  3. List is cursor-based (no offset page, no HEAD-count). The raw query is
//     captured via Resolve and fed to the imperative http.ValidateParameters binder.
//  4. Errors go through the shared pkgHTTP.HumaProblem.

// secOperationRouteBearer advertises that each operation-route operation accepts a
// JWT bearer token (Bearer-only, matching the Fiber guard chain).
// SPEC metadata only; runtime auth is the Fiber guard chain.
var secOperationRouteBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /operation-routes ---------------------------------------------------

// CreateOperationRouteInputHuma is the Huma request envelope for POST. RawBody keeps
// the body out of Huma's validator and feeds the imperative decode + the
// accountingEntries unknown-key probe in the shared core.
type CreateOperationRouteInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateOperationRouteOutputHuma pins 201 (matching http.Created).
type CreateOperationRouteOutputHuma struct {
	Status int
	Body   *mmodel.OperationRoute
}

// CreateOperationRouteHuma decodes+validates the raw body imperatively then delegates
// to the shared createOperationRoute core (feeding in.RawBody for the unknown-key probe).
func (handler *OperationRouteHandler) CreateOperationRouteHuma(ctx context.Context, in *CreateOperationRouteInputHuma) (*CreateOperationRouteOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreateOperationRouteInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	operationRoute, err := handler.createOperationRoute(ctx, orgID, ledgerID, payload, in.RawBody)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateOperationRouteOutputHuma{Status: http.StatusCreated, Body: operationRoute}, nil
}

// --- GET /operation-routes (list) ---------------------------------------------

// ListOperationRoutesInputHuma advertises the cursor-list query params (doc-only)
// and captures the raw query via Resolve for the imperative binder.
type ListOperationRoutesInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Limit          string `query:"limit" doc:"Max items per page (default 10)"`
	StartDate      string `query:"start_date" doc:"Filter created on/after this date (YYYY-MM-DD)"`
	EndDate        string `query:"end_date" doc:"Filter created on/before this date (YYYY-MM-DD)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`
	Cursor         string `query:"cursor" doc:"Opaque cursor token for pagination"`

	rawQuery url.Values
}

// Resolve captures the raw query before the handler (no validation; canonical
// rejection stays in http.ValidateParameters).
func (in *ListOperationRoutesInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key). Inlined per
// the pattern (the query binder is copied, not a shared helper).
func (in *ListOperationRoutesInputHuma) queries() map[string]string {
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

// ListOperationRoutesOutputHuma carries the pagination envelope verbatim.
type ListOperationRoutesOutputHuma struct {
	Status int
	Body   pkgHTTP.Pagination
}

// GetAllOperationRoutesHuma binds the query imperatively then delegates to
// getAllOperationRoutes.
func (handler *OperationRouteHandler) GetAllOperationRoutesHuma(ctx context.Context, in *ListOperationRoutesInputHuma) (*ListOperationRoutesOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllOperationRoutes(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListOperationRoutesOutputHuma{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /operation-routes/{operation_route_id} -------------------------------

// GetOperationRouteInputHuma is the by-id request envelope. The id path param
// carries no format tag (ParseUUIDPathParameters is the sole validator).
type GetOperationRouteInputHuma struct {
	OrganizationID   string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID         string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	OperationRouteID string `path:"operation_route_id" doc:"Operation Route ID (UUID)"`
}

// GetOperationRouteOutputHuma carries the operation route verbatim.
type GetOperationRouteOutputHuma struct {
	Status int
	Body   *mmodel.OperationRoute
}

// GetOperationRouteByIDHuma delegates to getOperationRouteByID.
func (handler *OperationRouteHandler) GetOperationRouteByIDHuma(ctx context.Context, in *GetOperationRouteInputHuma) (*GetOperationRouteOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.OperationRouteID, "operation_route_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	operationRoute, err := handler.getOperationRouteByID(ctx, orgID, ledgerID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetOperationRouteOutputHuma{Status: http.StatusOK, Body: operationRoute}, nil
}

// --- PATCH /operation-routes/{operation_route_id} -----------------------------

// UpdateOperationRouteInputHuma is the update request envelope. RawBody is the sole
// source that preserves accountingEntries field-absent vs explicit-null for the
// RFC 7396 merge-patch core (see file header + updateOperationRoute).
type UpdateOperationRouteInputHuma struct {
	OrganizationID   string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID         string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	OperationRouteID string `path:"operation_route_id" doc:"Operation Route ID (UUID)"`
	RawBody          []byte `contentType:"application/json"`
}

// UpdateOperationRouteOutputHuma carries the updated route (200, matching http.OK).
type UpdateOperationRouteOutputHuma struct {
	Status int
	Body   *mmodel.OperationRoute
}

// UpdateOperationRouteHuma decodes+validates the raw body imperatively then delegates
// to the shared updateOperationRoute core, feeding in.RawBody so the RFC 7396 merge
// distinguishes accountingEntries absent from accountingEntries:null.
func (handler *OperationRouteHandler) UpdateOperationRouteHuma(ctx context.Context, in *UpdateOperationRouteInputHuma) (*UpdateOperationRouteOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.OperationRouteID, "operation_route_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdateOperationRouteInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	operationRoute, err := handler.updateOperationRoute(ctx, orgID, ledgerID, id, payload, in.RawBody)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateOperationRouteOutputHuma{Status: http.StatusOK, Body: operationRoute}, nil
}

// --- DELETE /operation-routes/{operation_route_id} ----------------------------

// DeleteOperationRouteOutputHuma has NO Body field: paired with DefaultStatus 204 it
// makes Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeleteOperationRouteOutputHuma struct{}

// DeleteOperationRouteByIDHuma delegates to deleteOperationRouteByID; returns a
// bodiless 204 on success.
func (handler *OperationRouteHandler) DeleteOperationRouteByIDHuma(ctx context.Context, in *GetOperationRouteInputHuma) (*DeleteOperationRouteOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.OperationRouteID, "operation_route_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteOperationRouteByID(ctx, orgID, ledgerID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteOperationRouteOutputHuma{}, nil
}

// RegisterOperationRouteRoutes registers the five migrated operation-route
// operations on the shared Huma API. It is the per-file seam the unified server
// calls; the auth ("routing","operation-routes",verb) + tenant +
// ParseUUIDPathParameters("operation_route") middleware chain is attached on the /v1
// group (Fiber-level) BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE
// (see asset_handler_huma.go's RegisterAssetRoutes header for the /v1 rationale).
func RegisterOperationRouteRoutes(api huma.API, h *OperationRouteHandler) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/operation-routes"
		idPath   = listPath + "/{operation_route_id}"
		tag      = "Operation Routes"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createOperationRoute",
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create Operation Route",
		Tags:        []string{tag},
		Security:    secOperationRouteBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
	}, h.CreateOperationRouteHuma)

	huma.Register(api, huma.Operation{
		OperationID: "listOperationRoutes",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Retrieve all operation routes",
		Tags:        []string{tag},
		Security:    secOperationRouteBearer,
	}, h.GetAllOperationRoutesHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getOperationRouteByID",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific operation route",
		Tags:        []string{tag},
		Security:    secOperationRouteBearer,
	}, h.GetOperationRouteByIDHuma)

	huma.Register(api, huma.Operation{
		OperationID:      "updateOperationRoute",
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an operation route",
		Tags:             []string{tag},
		Security:         secOperationRouteBearer,
		SkipValidateBody: true, // body validated imperatively — RFC 7396 merge-patch core.
	}, h.UpdateOperationRouteHuma)

	huma.Register(api, huma.Operation{
		OperationID: "deleteOperationRoute",
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete an operation route",
		Tags:        []string{tag},
		Security:    secOperationRouteBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteOperationRouteByIDHuma)
}
