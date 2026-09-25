// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the Huma transport of the organization-level operation-route surface
// (/v2 only). Operation routes belong to the organization: these shells resolve only
// organization_id (+ id) and reach the same cores as the ledger-level shells in
// operation_route_handler.go, whose response envelopes they reuse so both paths
// publish one OperationRoute schema. A route created here has no ledger.

// CreateOrganizationOperationRouteRequest is the organization-level POST envelope.
type CreateOrganizationOperationRouteRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// ListOrganizationOperationRoutesRequest advertises the cursor-list query params
// (doc-only) and captures the raw query via Resolve for the imperative binder.
type ListOrganizationOperationRoutesRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	Limit          string `query:"limit" doc:"Max items per page (default 10)"`
	StartDate      string `query:"start_date" doc:"Filter created on/after this date (YYYY-MM-DD)"`
	EndDate        string `query:"end_date" doc:"Filter created on/before this date (YYYY-MM-DD)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`
	Cursor         string `query:"cursor" doc:"Opaque cursor token for pagination"`

	rawQuery url.Values
}

// GetOrganizationOperationRouteRequest is the organization-level by-id envelope.
type GetOrganizationOperationRouteRequest struct {
	OrganizationID   string `path:"organization_id" doc:"Organization ID (UUID)"`
	OperationRouteID string `path:"operation_route_id" doc:"Operation Route ID (UUID)"`
}

// UpdateOrganizationOperationRouteRequest is the organization-level update envelope.
type UpdateOrganizationOperationRouteRequest struct {
	OrganizationID   string `path:"organization_id" doc:"Organization ID (UUID)"`
	OperationRouteID string `path:"operation_route_id" doc:"Operation Route ID (UUID)"`
	RawBody          []byte `contentType:"application/json"`
}

// Resolve captures the raw query before the handler (no validation; canonical
// rejection stays in http.ValidateParameters).
func (in *ListOrganizationOperationRoutesRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// CreateOrganizationOperationRoute creates an operation route with no ledger.
func (handler *OperationRouteHandler) CreateOrganizationOperationRoute(ctx context.Context, in *CreateOrganizationOperationRouteRequest) (*CreateOperationRouteResponse, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreateOperationRouteInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	operationRoute, err := handler.createOperationRoute(ctx, orgID, nil, payload, in.RawBody)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateOperationRouteResponse{Status: http.StatusCreated, Body: operationRoute}, nil
}

// GetAllOrganizationOperationRoutes lists every operation route of the organization.
func (handler *OperationRouteHandler) GetAllOrganizationOperationRoutes(ctx context.Context, in *ListOrganizationOperationRoutesRequest) (*ListOperationRoutesResponse, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllOperationRoutes(ctx, orgID, queriesFromValues(in.rawQuery))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListOperationRoutesResponse{Status: http.StatusOK, Body: pagination}, nil
}

// GetOrganizationOperationRouteByID retrieves an operation route of the organization.
func (handler *OperationRouteHandler) GetOrganizationOperationRouteByID(ctx context.Context, in *GetOrganizationOperationRouteRequest) (*GetOperationRouteResponse, error) {
	orgID, id, err := parseOrganizationOperationRoute(in.OrganizationID, in.OperationRouteID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	operationRoute, err := handler.getOperationRouteByID(ctx, orgID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetOperationRouteResponse{Status: http.StatusOK, Body: operationRoute}, nil
}

// UpdateOrganizationOperationRoute updates an operation route of the organization.
func (handler *OperationRouteHandler) UpdateOrganizationOperationRoute(ctx context.Context, in *UpdateOrganizationOperationRouteRequest) (*UpdateOperationRouteResponse, error) {
	orgID, id, err := parseOrganizationOperationRoute(in.OrganizationID, in.OperationRouteID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdateOperationRouteInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	operationRoute, err := handler.updateOperationRoute(ctx, orgID, id, payload, in.RawBody)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateOperationRouteResponse{Status: http.StatusOK, Body: operationRoute}, nil
}

// DeleteOrganizationOperationRouteByID deletes an operation route of the
// organization; returns a bodiless 204 on success.
func (handler *OperationRouteHandler) DeleteOrganizationOperationRouteByID(ctx context.Context, in *GetOrganizationOperationRouteRequest) (*DeleteOperationRouteResponse, error) {
	orgID, id, err := parseOrganizationOperationRoute(in.OrganizationID, in.OperationRouteID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteOperationRouteByID(ctx, orgID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteOperationRouteResponse{}, nil
}

// parseOrganizationOperationRoute resolves the organization and operation-route
// path strings to UUIDs.
func parseOrganizationOperationRoute(organizationID, operationRouteID string) (uuid.UUID, uuid.UUID, error) {
	orgID, err := parsePathUUID(organizationID, "organization_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	id, err := parsePathUUID(operationRouteID, "operation_route_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return orgID, id, nil
}
