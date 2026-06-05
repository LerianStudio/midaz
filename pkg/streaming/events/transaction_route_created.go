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

// TransactionRouteCreatedDefinition is the routing contract for transaction-route.created.
// Emission anchor: components/ledger/internal/services/command/create_transaction_route.go,
// immediately after TransactionRouteRepo.Create succeeds and before the
// metadata-write call.
//
// IMPORTANT posture: emit failures MUST NOT fail the request.
//
// Note on resource type: the wire format uses the hyphen form
// `transaction-route`. The lib-streaming route-key regex
// `^[a-z0-9][a-z0-9-]*(\.[a-z0-9][a-z0-9-]*)+$` REJECTS underscores, so
// the canonical wire name diverges from the JSON entity name
// (`TransactionRoute`) and the HTTP route segment
// (`transaction-routes`). See the account-type / operation-route
// precedents.
var TransactionRouteCreatedDefinition = Definition{
	ResourceType:  "transaction-route",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// TransactionRouteCreatedPayload is the wire payload for transaction-route.created.
//
// OperationRouteIDs carries the linked operation-route identifiers that
// compose the routing topology — consumers replicate the join table
// via this slice instead of fetching the full operation route objects.
// The slice is always non-nil because the create input requires at
// least one operation route, but it uses omitempty for the (currently
// impossible) zero-link case so a future loosening of the validation
// does not silently push an empty array onto the wire.
//
// Description is optional and omitted from the wire when empty.
type TransactionRouteCreatedPayload struct {
	ID                string   `json:"id"`
	OrganizationID    string   `json:"organizationId"`
	LedgerID          string   `json:"ledgerId"`
	Title             string   `json:"title"`
	Description       string   `json:"description,omitempty"`
	OperationRouteIDs []string `json:"operationRouteIds,omitempty"`
	CreatedAt         string   `json:"createdAt"`
	UpdatedAt         string   `json:"updatedAt"`
}

// NewTransactionRouteCreated maps a persisted transaction route into
// the wire payload.
//
// Caller invariant: tr must be the value returned by the use case
// AFTER the squirrel + RETURNING insert AND the operation-route
// hydration step. Specifically tr.ID, tr.CreatedAt, tr.UpdatedAt must
// reflect the persisted row, and tr.OperationRoutes must carry the
// linked operation routes the join table now references.
func NewTransactionRouteCreated(tr *mmodel.TransactionRoute) TransactionRouteCreatedPayload {
	operationRouteIDs := make([]string, 0, len(tr.OperationRoutes))
	for _, o := range tr.OperationRoutes {
		operationRouteIDs = append(operationRouteIDs, o.ID.String())
	}

	return TransactionRouteCreatedPayload{
		ID:                tr.ID.String(),
		OrganizationID:    tr.OrganizationID.String(),
		LedgerID:          tr.LedgerID.String(),
		Title:             tr.Title,
		Description:       tr.Description,
		OperationRouteIDs: operationRouteIDs,
		CreatedAt:         tr.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         tr.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p TransactionRouteCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", TransactionRouteCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: TransactionRouteCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
