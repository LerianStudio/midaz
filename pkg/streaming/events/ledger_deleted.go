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

// LedgerDeletedDefinition is the routing contract for ledger.deleted.
// Emission anchor: components/ledger/internal/services/command/delete_ledger.go,
// immediately after LedgerRepo.Delete succeeds. IMPORTANT posture: emit
// failures MUST NOT fail the request.
var LedgerDeletedDefinition = Definition{
	ResourceType:  "ledger",
	EventType:     "deleted",
	SchemaVersion: "1.0.0",
}

// LedgerDeletedPayload is the wire payload for ledger.deleted.
type LedgerDeletedPayload struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	DeletedAt      string `json:"deletedAt"`
}

// NewLedgerDeleted maps the ledger identity and post-delete timestamp.
// The use case does not return the persisted struct on delete, so the
// caller captures deletedAt at the emit site.
func NewLedgerDeleted(id, organizationID string, deletedAt time.Time) LedgerDeletedPayload {
	return LedgerDeletedPayload{
		ID:             id,
		OrganizationID: organizationID,
		DeletedAt:      deletedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p LedgerDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", LedgerDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: LedgerDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
