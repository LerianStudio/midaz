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

// LedgerUpdatedDefinition is the routing contract for ledger.updated.
// Emission anchor: components/ledger/internal/services/command/update_ledger.go,
// immediately after LedgerRepo.Update succeeds and before UpdateOnboardingMetadata.
// IMPORTANT posture: emit failures MUST NOT fail the request.
//
// NOTE: ledger-settings updates (update_ledger_settings.go) are EXPLICITLY NOT
// covered by this event; settings changes are intentionally out of the v1
// wire contract.
var LedgerUpdatedDefinition = Definition{
	ResourceType:  "ledger",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// LedgerUpdatedPayload is the wire payload for ledger.updated.
type LedgerUpdatedPayload struct {
	ID             string              `json:"id"`
	OrganizationID string              `json:"organizationId"`
	Name           string              `json:"name"`
	Status         LedgerStatusPayload `json:"status"`
	UpdatedAt      string              `json:"updatedAt"`
}

// NewLedgerUpdated maps a persisted ledger into the wire payload.
func NewLedgerUpdated(led *mmodel.Ledger) LedgerUpdatedPayload {
	return LedgerUpdatedPayload{
		ID:             led.ID,
		OrganizationID: led.OrganizationID,
		Name:           led.Name,
		Status:         newLedgerStatusPayload(led.Status),
		UpdatedAt:      led.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p LedgerUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", LedgerUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: LedgerUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
