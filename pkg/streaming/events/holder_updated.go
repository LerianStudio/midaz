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

// HolderUpdatedDefinition is the routing contract for holder.updated.
// IMPORTANT posture: emit failures MUST NOT fail the request; durability is
// owned by PG + (follow-up task) the outbox subsystem.
var HolderUpdatedDefinition = Definition{
	ResourceType:  "holder",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// HolderUpdatedPayload is the wire payload for holder.updated. Shares the
// holder.created shape: identity, organization scope, classification type,
// optional correlation externalId, and timestamps — never PII. The JSONShape
// test (holder_updated_test.go) locks the present key set AND the absence of
// every PII key.
//
// Fields are typed independently of mmodel.Holder so domain evolution does not
// silently shift the wire contract.
type HolderUpdatedPayload struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organizationId"`
	Type           string  `json:"type"`
	ExternalID     *string `json:"externalId"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
}

// NewHolderUpdated maps a persisted holder into the wire payload. Holder
// carries no organization scope on the domain model, so organizationID is
// supplied explicitly by the emit site. PII is dropped here, not downstream.
func NewHolderUpdated(h *mmodel.Holder, organizationID string) HolderUpdatedPayload {
	return HolderUpdatedPayload{
		ID:             derefUUIDString(h.ID),
		OrganizationID: organizationID,
		Type:           derefString(h.Type),
		ExternalID:     h.ExternalID,
		CreatedAt:      h.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      h.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter. ts
// is typically the persisted UpdatedAt for "updated" events.
//
// Returns a wrapped json.Marshal error so callers can decide whether to log
// Warn (IMPORTANT posture) or fail the request (CRITICAL posture).
func (p HolderUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", HolderUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: HolderUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
