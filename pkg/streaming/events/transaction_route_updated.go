// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// TransactionRouteUpdatedDefinition is the routing contract for transaction-route.updated.
// Emission anchor: components/ledger/internal/services/command/update_transaction_route.go,
// immediately after TransactionRouteRepo.Update succeeds (and the
// post-update operation-route hydration completes) and before the
// metadata-write call.
//
// IMPORTANT posture: emit failures MUST NOT fail the request.
//
// Idempotency hint for consumers: `id + updatedAt` is unique per
// mutation; consumers safe-deduping on that pair can replay this event
// without effect.
var TransactionRouteUpdatedDefinition = Definition{
	ResourceType:  "transaction-route",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// TransactionRouteUpdatedPayload is the wire payload for transaction-route.updated.
// The payload carries the full mutable surface (title, description,
// operationRouteIds) so consumers don't need to join against
// transaction-route.created to render the row. CreatedAt is
// intentionally omitted — pinned at create time and not part of the
// update fact.
//
// OperationRouteIDs is the POST-UPDATE list of links, not a diff.
// Consumers replace their cached join-table set with this slice on
// every update event.
type TransactionRouteUpdatedPayload struct {
	ID                string   `json:"id"`
	OrganizationID    string   `json:"organizationId"`
	LedgerID          string   `json:"ledgerId"`
	Title             string   `json:"title"`
	Description       string   `json:"description,omitempty"`
	OperationRouteIDs []string `json:"operationRouteIds,omitempty"`
	UpdatedAt         string   `json:"updatedAt"`
}

// NewTransactionRouteUpdated maps the post-update transaction route
// record into the wire payload.
//
// Caller invariant: tr must be the value returned by
// TransactionRouteRepo.Update (post-commit) with its OperationRoutes
// field re-populated by the use case to reflect the FINAL post-update
// link set (after toAdd / toRemove). The squirrel + RETURNING repo
// refactor guarantees identity + timestamps; the use case is
// responsible for re-hydrating OperationRoutes via FindByIDs because
// the relations live in a separate join table not covered by the
// RETURNING clause.
func NewTransactionRouteUpdated(tr *mmodel.TransactionRoute) TransactionRouteUpdatedPayload {
	operationRouteIDs := make([]string, 0, len(tr.OperationRoutes))
	for _, o := range tr.OperationRoutes {
		operationRouteIDs = append(operationRouteIDs, o.ID.String())
	}

	return TransactionRouteUpdatedPayload{
		ID:                tr.ID.String(),
		OrganizationID:    tr.OrganizationID.String(),
		LedgerID:          tr.LedgerID.String(),
		Title:             tr.Title,
		Description:       tr.Description,
		OperationRouteIDs: operationRouteIDs,
		UpdatedAt:         tr.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p TransactionRouteUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", TransactionRouteUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: TransactionRouteUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
