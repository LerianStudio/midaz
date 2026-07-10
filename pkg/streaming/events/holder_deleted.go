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

// HolderDeletedDefinition is the routing contract for holder.deleted.
// IMPORTANT posture: emit failures MUST NOT fail the request; durability is
// owned by PG + (follow-up task) the outbox subsystem.
var HolderDeletedDefinition = Definition{
	ResourceType:  "holder",
	EventType:     "deleted",
	SchemaVersion: "1.0.0",
}

// HolderDeletedPayload is the wire payload for holder.deleted. Kept minimal:
// identity, organization scope, the deletion type (soft vs hard), and the
// deletion timestamp. No classification type or correlation externalId is
// carried, and no PII ever crosses the wire. The JSONShape test
// (holder_deleted_test.go) locks the present key set AND the absence of every
// PII key.
//
// Idempotency hint for consumers: id + deletedAt is unique per deletion;
// consumers safe-deduping on that pair can replay this event without effect.
type HolderDeletedPayload struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`

	// DeletionType is "soft" or "hard", derived from the hardDelete flag at
	// the emit site.
	DeletionType string `json:"deletionType"`

	// RFC3339-formatted deletion timestamp.
	DeletedAt string `json:"deletedAt"`
}

// NewHolderDeleted builds the wire payload from the identifiers and flags
// available at the emit site. The holder record is not required (and on a hard
// delete may already be gone); id, organizationID, the hardDelete flag, and the
// post-commit deletedAt instant are all the emit site has and all the contract
// needs. deletionType derives from hardDelete.
func NewHolderDeleted(id, organizationID string, hardDelete bool, deletedAt time.Time) HolderDeletedPayload {
	deletionType := deletionTypeSoft
	if hardDelete {
		deletionType = deletionTypeHard
	}

	return HolderDeletedPayload{
		ID:             id,
		OrganizationID: organizationID,
		DeletionType:   deletionType,
		DeletedAt:      deletedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter. ts
// is typically the same wall-clock instant passed into NewHolderDeleted as
// deletedAt.
//
// Returns a wrapped json.Marshal error so callers can decide whether to log
// Warn (IMPORTANT posture) or fail the request (CRITICAL posture).
func (p HolderDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", HolderDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: HolderDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
