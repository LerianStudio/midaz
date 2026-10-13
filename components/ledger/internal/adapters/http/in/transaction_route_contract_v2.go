// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the /v2 wire shape of a transaction route. /v2 links operation routes in two
// lists, operationRoutes (required) and optionalOperationRoutes (a transaction may leave them
// unused). mmodel keeps the /v1 shape, where operationRoutes is every link, so the /v1 contract
// does not change; the /v2 types below wrap it.

// patchLinksDocV2 describes how a /v2 PATCH applies the two link lists.
const patchLinksDocV2 = "operationRoutes (required links) and optionalOperationRoutes (links a transaction may leave unused) " +
	"are each applied as a JSON merge patch: an absent or null list keeps the stored links, a present list replaces them " +
	"and an empty list removes them. The resulting links are validated as a create: a route in both lists, fewer than " +
	"two links, or no required source or destination is refused."

// CreateTransactionRouteInputV2 is the /v2 create body: the /v1 body plus the optional links.
type CreateTransactionRouteInputV2 struct {
	mmodel.CreateTransactionRouteInput

	// Operation Route IDs a transaction may leave unused. A route cannot be in both lists.
	OptionalOperationRoutes *[]uuid.UUID `json:"optionalOperationRoutes,omitempty" nullable:"true" format:"uuid"`
}

// toCommand is the use-case input the body carries.
func (in *CreateTransactionRouteInputV2) toCommand() *mmodel.CreateTransactionRouteInput {
	payload := in.CreateTransactionRouteInput
	if in.OptionalOperationRoutes != nil {
		payload.OptionalOperationRoutes = *in.OptionalOperationRoutes
	}

	return &payload
}

// UpdateTransactionRouteInputV2 is the /v2 update body. Each link list is a JSON merge patch of
// its own: absent or null keeps the stored list, present replaces it, empty removes it.
type UpdateTransactionRouteInputV2 struct {
	mmodel.UpdateTransactionRouteInput

	// Operation Route IDs every transaction must use. Omit or send null to keep the current
	// required links; a list replaces them. It shadows the embedded field so the /v2 contract
	// can declare null without changing the /v1 one.
	OperationRoutes *[]uuid.UUID `json:"operationRoutes,omitempty" nullable:"true" format:"uuid"`
	// Operation Route IDs a transaction may leave unused. Omit or send null to keep the current
	// optional links; an empty list removes them. A route cannot be in both lists.
	OptionalOperationRoutes *[]uuid.UUID `json:"optionalOperationRoutes,omitempty" nullable:"true" format:"uuid"`
}

// toCommand is the use-case input the body carries.
func (in *UpdateTransactionRouteInputV2) toCommand() *mmodel.UpdateTransactionRouteInput {
	payload := in.UpdateTransactionRouteInput
	payload.OperationRoutes = in.OperationRoutes
	payload.OptionalOperationRoutes = in.OptionalOperationRoutes

	return &payload
}

// TransactionRouteV2 is the /v2 wire projection of a transaction route: operationRoutes lists
// the required links and optionalOperationRoutes the optional ones, matching the request.
//
// OperationRoutes shadows the embedded field of the same JSON name (a field at depth 0 wins
// over a promoted one), so the embedded every-link list never reaches the wire.
type TransactionRouteV2 struct {
	*mmodel.TransactionRoute

	// Operation routes every transaction on this route must use.
	OperationRoutes []mmodel.OperationRoute `json:"operationRoutes,omitempty"`
	// Operation routes a transaction on this route may leave unused.
	OptionalOperationRoutes []mmodel.OperationRoute `json:"optionalOperationRoutes"`
}

// newTransactionRouteV2 splits a transaction route's links into the /v2 lists. A nil route
// stays nil.
func newTransactionRouteV2(tr *mmodel.TransactionRoute) *TransactionRouteV2 {
	if tr == nil {
		return nil
	}

	out := &TransactionRouteV2{
		TransactionRoute:        tr,
		OperationRoutes:         make([]mmodel.OperationRoute, 0, len(tr.OperationRoutes)),
		OptionalOperationRoutes: make([]mmodel.OperationRoute, 0, len(tr.OptionalOperationRouteIDs)),
	}

	for _, operationRoute := range tr.OperationRoutes {
		if tr.IsOptional(operationRoute.ID) {
			out.OptionalOperationRoutes = append(out.OptionalOperationRoutes, operationRoute)
		} else {
			out.OperationRoutes = append(out.OperationRoutes, operationRoute)
		}
	}

	return out
}

// newTransactionRouteV2Items re-projects a page of transaction routes onto the /v2 shape. A
// page whose items are not []*mmodel.TransactionRoute is returned untouched.
func newTransactionRouteV2Items(p pkgHTTP.Pagination) pkgHTTP.Pagination {
	src, ok := p.Items.([]*mmodel.TransactionRoute)
	if !ok || src == nil {
		return p
	}

	out := make([]*TransactionRouteV2, 0, len(src))
	for _, tr := range src {
		out = append(out, newTransactionRouteV2(tr))
	}

	p.Items = out

	return p
}

// TransactionRouteV2Response carries one transaction route in the /v2 shape.
type TransactionRouteV2Response struct {
	Status int
	Body   *TransactionRouteV2
}

// CreateTransactionRouteV2 is the /v2 ledger-level create.
func (handler *TransactionRouteHandler) CreateTransactionRouteV2(ctx context.Context, in *CreateTransactionRouteRequest) (*TransactionRouteV2Response, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return handler.createTransactionRouteV2(ctx, orgID, &ledgerID, in.RawBody)
}

// GetAllTransactionRoutesV2 is the /v2 ledger-level list.
func (handler *TransactionRouteHandler) GetAllTransactionRoutesV2(ctx context.Context, in *ListTransactionRoutesRequest) (*ListTransactionRoutesResponse, error) {
	response, err := handler.GetAllTransactionRoutes(ctx, in)
	if err != nil {
		return nil, err
	}

	response.Body = newTransactionRouteV2Items(response.Body)

	return response, nil
}

// GetTransactionRouteByIDV2 is the /v2 ledger-level read by ID.
func (handler *TransactionRouteHandler) GetTransactionRouteByIDV2(ctx context.Context, in *GetTransactionRouteRequest) (*TransactionRouteV2Response, error) {
	response, err := handler.GetTransactionRouteByID(ctx, in)
	if err != nil {
		return nil, err
	}

	return &TransactionRouteV2Response{Status: response.Status, Body: newTransactionRouteV2(response.Body)}, nil
}

// UpdateTransactionRouteV2 is the /v2 ledger-level update.
func (handler *TransactionRouteHandler) UpdateTransactionRouteV2(ctx context.Context, in *UpdateTransactionRouteRequest) (*TransactionRouteV2Response, error) {
	orgID, _, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.TransactionRouteID, "transaction_route_id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return handler.updateTransactionRouteV2(ctx, orgID, id, in.RawBody)
}

// createTransactionRouteV2 decodes a /v2 create body and creates the route; a nil ledgerID
// creates it at organization level.
func (handler *TransactionRouteHandler) createTransactionRouteV2(ctx context.Context, orgID uuid.UUID, ledgerID *uuid.UUID, body []byte) (*TransactionRouteV2Response, error) {
	payload := new(CreateTransactionRouteInputV2)
	if _, err := pkgHTTP.DecodeAndValidate(body, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionRoute, err := handler.createTransactionRoute(ctx, orgID, ledgerID, payload.toCommand())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &TransactionRouteV2Response{Status: http.StatusCreated, Body: newTransactionRouteV2(transactionRoute)}, nil
}

// updateTransactionRouteV2 decodes a /v2 update body and applies it as a merge patch per list.
func (handler *TransactionRouteHandler) updateTransactionRouteV2(ctx context.Context, orgID, id uuid.UUID, body []byte) (*TransactionRouteV2Response, error) {
	payload := new(UpdateTransactionRouteInputV2)
	if _, err := decodePatchBody(body, payload, &payload.Metadata, metadataNullKeepsV2); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionRoute, err := handler.updateTransactionRoute(ctx, orgID, id, payload.toCommand(), command.LinksMergePatchV2)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &TransactionRouteV2Response{Status: http.StatusOK, Body: newTransactionRouteV2(transactionRoute)}, nil
}
