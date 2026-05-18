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

// LedgerCreatedDefinition is the routing contract for ledger.created.
// Emission anchor: components/ledger/internal/services/command/create_ledger.go,
// immediately after LedgerRepo.Create succeeds and before CreateOnboardingMetadata.
// IMPORTANT posture: emit failures MUST NOT fail the request.
var LedgerCreatedDefinition = Definition{
	ResourceType:  "ledger",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// LedgerStatusPayload mirrors mmodel.Status for ledger events without
// embedding domain types directly into the wire contract. Description is
// optional and omitted when nil.
type LedgerStatusPayload struct {
	Code        string  `json:"code"`
	Description *string `json:"description,omitempty"`
}

// LedgerCreatedPayload is the wire payload for ledger.created.
type LedgerCreatedPayload struct {
	ID             string              `json:"id"`
	OrganizationID string              `json:"organizationId"`
	Name           string              `json:"name"`
	Status         LedgerStatusPayload `json:"status"`
	CreatedAt      string              `json:"createdAt"`
	UpdatedAt      string              `json:"updatedAt"`
}

// NewLedgerCreated maps a persisted ledger into the wire payload.
func NewLedgerCreated(led *mmodel.Ledger) LedgerCreatedPayload {
	return LedgerCreatedPayload{
		ID:             led.ID,
		OrganizationID: led.OrganizationID,
		Name:           led.Name,
		Status:         newLedgerStatusPayload(led.Status),
		CreatedAt:      led.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      led.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEvent assembles a libStreaming.Event ready for the Emitter.
func (p LedgerCreatedPayload) ToEvent(tenantID, source string, ts time.Time) (libStreaming.Event, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.Event{}, fmt.Errorf("marshal %s payload: %w", LedgerCreatedDefinition.Key(), err)
	}

	return libStreaming.Event{
		TenantID:      tenantID,
		Source:        source,
		ResourceType:  LedgerCreatedDefinition.ResourceType,
		EventType:     LedgerCreatedDefinition.EventType,
		SchemaVersion: LedgerCreatedDefinition.SchemaVersion,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}

func newLedgerStatusPayload(status mmodel.Status) LedgerStatusPayload {
	return LedgerStatusPayload{
		Code:        status.Code,
		Description: status.Description,
	}
}
