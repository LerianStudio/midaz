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

// InstrumentDeletedDefinition is the routing contract for instrument.deleted.
// IMPORTANT posture: emit failures MUST NOT fail the request; durability is
// owned by PG + (follow-up task) the outbox subsystem.
var InstrumentDeletedDefinition = Definition{
	ResourceType:  "instrument",
	EventType:     "deleted",
	SchemaVersion: "1.0.0",
}

// InstrumentDeletedPayload is the wire payload for instrument.deleted. Kept
// minimal: identity, holder + organization scope, the deletion type (soft vs
// hard), and the deletion timestamp. Unlike holder.deleted it carries holderId
// so consumers can attribute the removal to a holder without a lookup. No
// classification, references, or PII ever cross the wire. The JSONShape test
// locks the present key set AND the absence of every PII key.
type InstrumentDeletedPayload struct {
	ID             string `json:"id"`
	HolderID       string `json:"holderId"`
	OrganizationID string `json:"organizationId"`

	// DeletionType is "soft" or "hard", derived from the hardDelete flag at
	// the emit site.
	DeletionType string `json:"deletionType"`

	// RFC3339-formatted deletion timestamp.
	DeletedAt string `json:"deletedAt"`
}

// NewInstrumentDeleted builds the wire payload from the identifiers and flags
// available at the emit site. The instrument record is not required (and on a
// hard delete may already be gone); the ids, the hardDelete flag, and the
// post-commit deletedAt instant are all the emit site has and all the contract
// needs. deletionType derives from hardDelete.
func NewInstrumentDeleted(id, holderID, organizationID string, hardDelete bool, deletedAt time.Time) InstrumentDeletedPayload {
	deletionType := deletionTypeSoft
	if hardDelete {
		deletionType = deletionTypeHard
	}

	return InstrumentDeletedPayload{
		ID:             id,
		HolderID:       holderID,
		OrganizationID: organizationID,
		DeletionType:   deletionType,
		DeletedAt:      deletedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter. ts
// is typically the same wall-clock instant passed into NewInstrumentDeleted as
// deletedAt. Subject is the instrument ID (the aggregate).
//
// Returns a wrapped json.Marshal error so callers can decide whether to log
// Warn (IMPORTANT posture) or fail the request (CRITICAL posture).
func (p InstrumentDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", InstrumentDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: InstrumentDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
