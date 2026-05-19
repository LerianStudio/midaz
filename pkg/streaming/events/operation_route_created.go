// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// OperationRouteCreatedDefinition is the routing contract for operation-route.created.
// Emission anchor: components/ledger/internal/services/command/create_operation_route.go,
// immediately after OperationRouteRepo.Create succeeds and before the
// metadata-write call.
//
// IMPORTANT posture: emit failures MUST NOT fail the request.
//
// Note on resource type: the wire format uses the hyphen form
// `operation-route`. The lib-streaming route-key regex
// `^[a-z0-9][a-z0-9-]*(\.[a-z0-9][a-z0-9-]*)+$` REJECTS underscores, so
// the canonical wire name diverges from the JSON entity name
// (`OperationRoute`) and the HTTP route segment (`operation-routes`).
var OperationRouteCreatedDefinition = Definition{
	ResourceType:  "operation-route",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// OperationRouteCreatedPayload is the wire payload for operation-route.created.
//
// Optional fields (Description, Code, Account, AccountingEntries) use
// omitempty to mirror the HTTP response contract: a route created with
// only title + operationType + account does not leak empty strings or
// nil pointers onto the wire.
//
// Account and AccountingEntries embed the mmodel types directly because
// their JSON tags already produce the canonical nested shape consumers
// expect. The same shape is documented in the OpenAPI spec for
// OperationRoute create responses, so we keep one source of truth.
type OperationRouteCreatedPayload struct {
	ID                string                    `json:"id"`
	OrganizationID    string                    `json:"organizationId"`
	LedgerID          string                    `json:"ledgerId"`
	Title             string                    `json:"title"`
	Description       string                    `json:"description,omitempty"`
	Code              string                    `json:"code,omitempty"`
	OperationType     string                    `json:"operationType"`
	Account           *mmodel.AccountRule       `json:"account,omitempty"`
	AccountingEntries *mmodel.AccountingEntries `json:"accountingEntries,omitempty"`
	CreatedAt         string                    `json:"createdAt"`
	UpdatedAt         string                    `json:"updatedAt"`
}

// NewOperationRouteCreated maps a persisted operation route into the
// wire payload.
//
// Caller invariant: o must be the value returned by
// OperationRouteRepo.Create (post-commit), not the input struct.
// Specifically o.ID, o.CreatedAt, and o.UpdatedAt must reflect the
// persisted state and o.Account/o.AccountingEntries must reflect the
// post-RETURNING state (the squirrel + RETURNING repo guarantees this).
func NewOperationRouteCreated(o *mmodel.OperationRoute) OperationRouteCreatedPayload {
	return OperationRouteCreatedPayload{
		ID:                o.ID.String(),
		OrganizationID:    o.OrganizationID.String(),
		LedgerID:          o.LedgerID.String(),
		Title:             o.Title,
		Description:       o.Description,
		Code:              o.Code,
		OperationType:     o.OperationType,
		Account:           o.Account,
		AccountingEntries: o.AccountingEntries,
		CreatedAt:         o.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         o.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p OperationRouteCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", OperationRouteCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: OperationRouteCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
