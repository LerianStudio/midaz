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

// This file is the ledger's Huma adoption of the operation-route resource. It
// mirrors the asset exemplar (asset_handler.go); see that file's header for the
// full conventions. Operation-route-specific notes:
//
//  1. AUTH is the "midaz" appName, resource "operation-routes". The Fiber guard
//     chain is Bearer-only (no X-API-Key), so the per-op Security metadata here is
//     Bearer-only too — this is SPEC metadata only; runtime auth stays the Fiber
//     guard chain (auth.Authorize("midaz","operation-routes",verb) + tenant +
//     ParseUUIDPathParameters("operation_route")) attached in the unified server
//     BEFORE the Huma terminal.
//  2. MERGE-PATCH landmine: the PATCH core (updateOperationRoute in
//     operation_route.go) implements RFC 7396 JSON Merge Patch. It re-derives
//     AccountingEntriesRaw from the raw request bytes to tell accountingEntries
//     FIELD-ABSENT (keep existing) from accountingEntries:null (clear all) — a
//     distinction Go's typed decode collapses. The handler MUST feed in.RawBody to
//     the core unaltered, or the PATCH breaks silently. POST carries the same
//     rawBody for the accountingEntries unknown-key probe. Both keep RawBody +
//     SkipValidateBody so http.DecodeAndValidate is the sole body validator (never
//     a native Huma 422).
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

// CreateOperationRouteRequest is the Huma request envelope for POST. RawBody keeps
// the body out of Huma's validator and feeds the imperative decode + the
// accountingEntries unknown-key probe in the shared core.
type CreateOperationRouteRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateOperationRouteResponse pins 201 (matching http.Created).
type CreateOperationRouteResponse struct {
	Status int
	Body   *mmodel.OperationRoute
}

// CreateOperationRoute decodes+validates the raw body imperatively then delegates
// to the shared createOperationRoute core (feeding in.RawBody for the unknown-key probe).
func (handler *OperationRouteHandler) CreateOperationRoute(ctx context.Context, in *CreateOperationRouteRequest) (*CreateOperationRouteResponse, error) {
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

	return &CreateOperationRouteResponse{Status: http.StatusCreated, Body: operationRoute}, nil
}

// --- GET /operation-routes (list) ---------------------------------------------

// ListOperationRoutesRequest advertises the cursor-list query params (doc-only)
// and captures the raw query via Resolve for the imperative binder.
type ListOperationRoutesRequest struct {
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
func (in *ListOperationRoutesRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that http.ValidateParameters consumes,
// matching Fiber's c.Queries() (last value wins for a repeated key). Inlined per
// the pattern (the query binder is copied, not a shared helper).
func (in *ListOperationRoutesRequest) queries() map[string]string {
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

// ListOperationRoutesResponse carries the pagination envelope verbatim.
type ListOperationRoutesResponse struct {
	Status int
	Body   pkgHTTP.Pagination
}

// GetAllOperationRoutes binds the query imperatively then delegates to
// getAllOperationRoutes.
func (handler *OperationRouteHandler) GetAllOperationRoutes(ctx context.Context, in *ListOperationRoutesRequest) (*ListOperationRoutesResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllOperationRoutes(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListOperationRoutesResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /operation-routes/{operation_route_id} -------------------------------

// GetOperationRouteRequest is the by-id request envelope. The id path param
// carries no format tag (ParseUUIDPathParameters is the sole validator).
type GetOperationRouteRequest struct {
	OrganizationID   string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID         string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	OperationRouteID string `path:"operation_route_id" doc:"Operation Route ID (UUID)"`
}

// GetOperationRouteResponse carries the operation route verbatim.
type GetOperationRouteResponse struct {
	Status int
	Body   *mmodel.OperationRoute
}

// GetOperationRouteByID delegates to getOperationRouteByID.
func (handler *OperationRouteHandler) GetOperationRouteByID(ctx context.Context, in *GetOperationRouteRequest) (*GetOperationRouteResponse, error) {
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

	return &GetOperationRouteResponse{Status: http.StatusOK, Body: operationRoute}, nil
}

// --- PATCH /operation-routes/{operation_route_id} -----------------------------

// UpdateOperationRouteRequest is the update request envelope. RawBody is the sole
// source that preserves accountingEntries field-absent vs explicit-null for the
// RFC 7396 merge-patch core (see file header + updateOperationRoute).
type UpdateOperationRouteRequest struct {
	OrganizationID   string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID         string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	OperationRouteID string `path:"operation_route_id" doc:"Operation Route ID (UUID)"`
	RawBody          []byte `contentType:"application/json"`
}

// UpdateOperationRouteResponse carries the updated route (200, matching http.OK).
type UpdateOperationRouteResponse struct {
	Status int
	Body   *mmodel.OperationRoute
}

// UpdateOperationRoute decodes+validates the raw body imperatively then delegates
// to the shared updateOperationRoute core, feeding in.RawBody so the RFC 7396 merge
// distinguishes accountingEntries absent from accountingEntries:null.
func (handler *OperationRouteHandler) UpdateOperationRoute(ctx context.Context, in *UpdateOperationRouteRequest) (*UpdateOperationRouteResponse, error) {
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

	return &UpdateOperationRouteResponse{Status: http.StatusOK, Body: operationRoute}, nil
}

// --- DELETE /operation-routes/{operation_route_id} ----------------------------

// DeleteOperationRouteResponse has NO Body field: paired with DefaultStatus 204 it
// makes Huma emit a bodiless 204.
type DeleteOperationRouteResponse struct{}

// DeleteOperationRouteByID delegates to deleteOperationRouteByID; returns a
// bodiless 204 on success.
func (handler *OperationRouteHandler) DeleteOperationRouteByID(ctx context.Context, in *GetOperationRouteRequest) (*DeleteOperationRouteResponse, error) {
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

	return &DeleteOperationRouteResponse{}, nil
}

// RegisterOperationRouteRoutes registers the five operation-route operations on the
// shared Huma API. It is the per-file seam the unified server calls; the auth
// ("midaz","operation-routes",verb) + tenant +
// ParseUUIDPathParameters("operation_route") middleware chain is attached on the
// version group (Fiber-level) BEFORE the Huma terminal, not here. Paths are
// GROUP-RELATIVE (see asset_handler.go's RegisterAssetRoutes header for the
// rationale).
//
// opSuffix is appended to every operation ID so the same surface can be published on
// more than one version group of the one document without colliding — see
// routeOpSuffixV1.
func RegisterOperationRouteRoutes(api huma.API, h *OperationRouteHandler, opSuffix string) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/operation-routes"
		idPath   = listPath + "/{operation_route_id}"
		tag      = "Operation Routes"
	)

	huma.Register(api, huma.Operation{
		OperationID: "createOperationRoute" + opSuffix,
		Method:      http.MethodPost,
		Path:        listPath,
		Summary:     "Create Operation Route",
		Tags:        []string{tag},
		Security:    secOperationRouteBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateOperationRoute)
	attachTypedRequestBody[mmodel.CreateOperationRouteInput](api, "createOperationRoute"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listOperationRoutes" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Retrieve all operation routes",
		Tags:        []string{tag},
		Security:    secOperationRouteBearer,
	}, h.GetAllOperationRoutes)

	huma.Register(api, huma.Operation{
		OperationID: "getOperationRouteByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific operation route",
		Tags:        []string{tag},
		Security:    secOperationRouteBearer,
	}, h.GetOperationRouteByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updateOperationRoute" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update an operation route",
		Tags:             []string{tag},
		Security:         secOperationRouteBearer,
		SkipValidateBody: true, // body validated imperatively — RFC 7396 merge-patch core.
	}, h.UpdateOperationRoute)
	attachTypedRequestBody[mmodel.UpdateOperationRouteInput](api, "updateOperationRoute"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "deleteOperationRoute" + opSuffix,
		Method:      http.MethodDelete,
		Path:        idPath,
		Summary:     "Delete an operation route",
		Tags:        []string{tag},
		Security:    secOperationRouteBearer,
		// DefaultStatus 204 + an Out struct with no Body field => bodiless 204.
		DefaultStatus: http.StatusNoContent,
	}, h.DeleteOperationRouteByID)
}
