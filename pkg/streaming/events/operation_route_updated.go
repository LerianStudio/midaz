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

// OperationRouteUpdatedDefinition is the routing contract for operation-route.updated.
// Emission anchor: components/ledger/internal/services/command/update_operation_route.go,
// immediately after OperationRouteRepo.Update succeeds and before the
// metadata-write call.
//
// IMPORTANT posture: emit failures MUST NOT fail the request.
//
// Idempotency hint for consumers: `id + updatedAt` is unique per
// mutation; consumers safe-deduping on that pair can replay this event
// without effect.
var OperationRouteUpdatedDefinition = Definition{
	ResourceType:  "operation-route",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// OperationRouteUpdatedPayload is the wire payload for operation-route.updated.
// The payload carries the full mutable surface (title, description,
// code, account, accountingEntries) plus the immutable operationType so
// consumers don't need to join against operation-route.created to
// render the row. CreatedAt is intentionally omitted — pinned at create
// time and not part of the update fact.
type OperationRouteUpdatedPayload struct {
	ID                string                    `json:"id"`
	OrganizationID    string                    `json:"organizationId"`
	LedgerID          string                    `json:"ledgerId"`
	Title             string                    `json:"title"`
	Description       string                    `json:"description,omitempty"`
	Code              string                    `json:"code,omitempty"`
	OperationType     string                    `json:"operationType"`
	Account           *mmodel.AccountRule       `json:"account,omitempty"`
	AccountingEntries *mmodel.AccountingEntries `json:"accountingEntries,omitempty"`
	UpdatedAt         string                    `json:"updatedAt"`
}

// NewOperationRouteUpdated maps the post-update operation route record
// into the wire payload.
//
// Caller invariant: o must be the value returned by
// OperationRouteRepo.Update (post-commit), not the input struct.
// Specifically o.UpdatedAt must reflect the persisted timestamp and the
// other fields must carry the post-RETURNING state (the squirrel +
// RETURNING repo guarantees this).
func NewOperationRouteUpdated(o *mmodel.OperationRoute) OperationRouteUpdatedPayload {
	return OperationRouteUpdatedPayload{
		ID:                o.ID.String(),
		OrganizationID:    o.OrganizationID.String(),
		LedgerID:          o.LedgerID.String(),
		Title:             o.Title,
		Description:       o.Description,
		Code:              o.Code, //nolint:staticcheck // legacy Code field emitted for backward compatibility
		OperationType:     o.OperationType,
		Account:           o.Account,
		AccountingEntries: o.AccountingEntries,
		UpdatedAt:         o.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p OperationRouteUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", OperationRouteUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: OperationRouteUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
