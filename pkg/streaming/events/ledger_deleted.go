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

// ToEvent assembles a libStreaming.Event ready for the Emitter.
func (p LedgerDeletedPayload) ToEvent(tenantID, source string, ts time.Time) (libStreaming.Event, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.Event{}, fmt.Errorf("marshal %s payload: %w", LedgerDeletedDefinition.Key(), err)
	}

	return libStreaming.Event{
		TenantID:      tenantID,
		Source:        source,
		ResourceType:  LedgerDeletedDefinition.ResourceType,
		EventType:     LedgerDeletedDefinition.EventType,
		SchemaVersion: LedgerDeletedDefinition.SchemaVersion,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
