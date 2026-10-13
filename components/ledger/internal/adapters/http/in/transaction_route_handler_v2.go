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

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the Huma transport of the organization-level transaction-route surface
// (/v2 only). Transaction routes belong to the organization: these shells resolve only
// organization_id (+ id) and reach the same cores as the ledger-level /v2 shells in
// transaction_route_contract_v2.go, so both paths publish one TransactionRouteV2 schema. A
// route created here has no ledger.

// CreateOrganizationTransactionRouteRequest is the organization-level POST envelope.
type CreateOrganizationTransactionRouteRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// ListOrganizationTransactionRoutesRequest advertises the cursor-list query params
// (doc-only) and captures the raw query via Resolve for the imperative binder.
type ListOrganizationTransactionRoutesRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	Limit          string `query:"limit" doc:"Max items per page (default 10)"`
	StartDate      string `query:"start_date" doc:"Filter created on/after this date (YYYY-MM-DD)"`
	EndDate        string `query:"end_date" doc:"Filter created on/before this date (YYYY-MM-DD)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`
	Cursor         string `query:"cursor" doc:"Opaque cursor token for pagination"`

	rawQuery url.Values
}

// GetOrganizationTransactionRouteRequest is the organization-level by-id envelope.
type GetOrganizationTransactionRouteRequest struct {
	OrganizationID     string `path:"organization_id" doc:"Organization ID (UUID)"`
	TransactionRouteID string `path:"transaction_route_id" doc:"Transaction Route ID (UUID)"`
}

// UpdateOrganizationTransactionRouteRequest is the organization-level update envelope.
type UpdateOrganizationTransactionRouteRequest struct {
	OrganizationID     string `path:"organization_id" doc:"Organization ID (UUID)"`
	TransactionRouteID string `path:"transaction_route_id" doc:"Transaction Route ID (UUID)"`
	RawBody            []byte `contentType:"application/json"`
}

// Resolve captures the raw query before the handler (no validation; canonical
// rejection stays in http.ValidateParameters).
func (in *ListOrganizationTransactionRoutesRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// CreateOrganizationTransactionRoute creates a transaction route with no ledger.
func (handler *TransactionRouteHandler) CreateOrganizationTransactionRoute(ctx context.Context, in *CreateOrganizationTransactionRouteRequest) (*TransactionRouteV2Response, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return handler.createTransactionRouteV2(ctx, orgID, nil, in.RawBody)
}

// GetAllOrganizationTransactionRoutes lists every transaction route of the organization.
func (handler *TransactionRouteHandler) GetAllOrganizationTransactionRoutes(ctx context.Context, in *ListOrganizationTransactionRoutesRequest) (*ListTransactionRoutesResponse, error) {
	orgID, err := parsePathUUID(in.OrganizationID, "organization_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllTransactionRoutes(ctx, orgID, queriesFromValues(in.rawQuery))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListTransactionRoutesResponse{Status: http.StatusOK, Body: newTransactionRouteV2Items(pagination)}, nil
}

// GetOrganizationTransactionRouteByID retrieves a transaction route of the organization.
func (handler *TransactionRouteHandler) GetOrganizationTransactionRouteByID(ctx context.Context, in *GetOrganizationTransactionRouteRequest) (*TransactionRouteV2Response, error) {
	orgID, id, err := parseOrganizationTransactionRoute(in.OrganizationID, in.TransactionRouteID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionRoute, err := handler.getTransactionRouteByID(ctx, orgID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &TransactionRouteV2Response{Status: http.StatusOK, Body: newTransactionRouteV2(transactionRoute)}, nil
}

// UpdateOrganizationTransactionRoute updates a transaction route of the organization.
func (handler *TransactionRouteHandler) UpdateOrganizationTransactionRoute(ctx context.Context, in *UpdateOrganizationTransactionRouteRequest) (*TransactionRouteV2Response, error) {
	orgID, id, err := parseOrganizationTransactionRoute(in.OrganizationID, in.TransactionRouteID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return handler.updateTransactionRouteV2(ctx, orgID, id, in.RawBody)
}

// DeleteOrganizationTransactionRouteByID deletes a transaction route of the
// organization; returns a bodiless 204 on success.
func (handler *TransactionRouteHandler) DeleteOrganizationTransactionRouteByID(ctx context.Context, in *GetOrganizationTransactionRouteRequest) (*DeleteTransactionRouteResponse, error) {
	orgID, id, err := parseOrganizationTransactionRoute(in.OrganizationID, in.TransactionRouteID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteTransactionRouteByID(ctx, orgID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteTransactionRouteResponse{}, nil
}

// parseOrganizationTransactionRoute resolves the organization and transaction-route
// path strings to UUIDs.
func parseOrganizationTransactionRoute(organizationID, transactionRouteID string) (uuid.UUID, uuid.UUID, error) {
	orgID, err := parsePathUUID(organizationID, "organization_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	id, err := parsePathUUID(transactionRouteID, "transaction_route_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return orgID, id, nil
}
