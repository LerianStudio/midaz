// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
)

// OperationRouteDeletedDefinition is the routing contract for operation-route.deleted.
// Emission anchor: components/ledger/internal/services/command/delete_operation_route.go,
// immediately after OperationRouteRepo.Delete succeeds (post-commit).
//
// IMPORTANT posture: emit failures MUST NOT fail the request.
var OperationRouteDeletedDefinition = Definition{
	ResourceType:  "operation-route",
	EventType:     "deleted",
	SchemaVersion: "1.0.0",
}

// OperationRouteDeletedPayload is the wire payload for operation-route.deleted.
// Kept intentionally minimal: identity, tenant scope (org/ledger), and
// the soft-delete timestamp.
//
// Idempotency hint for consumers: `id + deletedAt` is unique per
// soft-delete; consumers safe-deduping on that pair can replay this
// event without effect.
type OperationRouteDeletedPayload struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	DeletedAt      string `json:"deletedAt"`
}

// NewOperationRouteDeleted maps the operation route identity and
// post-commit deletedAt timestamp into the wire payload. The use case
// does not return the persisted struct on delete, so the caller
// captures deletedAt at the emit site.
func NewOperationRouteDeleted(id, organizationID, ledgerID string, deletedAt time.Time) OperationRouteDeletedPayload {
	return OperationRouteDeletedPayload{
		ID:             id,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		DeletedAt:      deletedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p OperationRouteDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", OperationRouteDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: OperationRouteDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
