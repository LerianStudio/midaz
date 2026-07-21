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

// InstrumentRelatedPartyDeletedDefinition is the routing contract for
// instrument.related-party-deleted. The EventType uses a HYPHEN: the
// lib-streaming route-key validator rejects underscores, so this must stay
// hyphenated. IMPORTANT posture: emit failures MUST NOT fail the request.
var InstrumentRelatedPartyDeletedDefinition = Definition{
	ResourceType:  "instrument",
	EventType:     "related-party-deleted",
	SchemaVersion: "1.0.0",
}

// InstrumentRelatedPartyDeletedPayload is the wire payload for
// instrument.related-party-deleted. It carries the instrument + holder +
// organization scope and the removed related-party ID. There is NO
// deletionType: removing a related party is always a pointwise removal, not a
// soft/hard distinction. No related-party PII (document, name, role, dates)
// ever crosses the wire.
type InstrumentRelatedPartyDeletedPayload struct {
	InstrumentID   string `json:"instrumentId"`
	HolderID       string `json:"holderId"`
	OrganizationID string `json:"organizationId"`
	RelatedPartyID string `json:"relatedPartyId"`

	// RFC3339-formatted deletion timestamp.
	DeletedAt string `json:"deletedAt"`
}

// NewInstrumentRelatedPartyDeleted builds the wire payload from the identifiers
// available at the emit site. All values are already string IDs; deletedAt is
// the post-commit instant captured at the call site.
func NewInstrumentRelatedPartyDeleted(instrumentID, holderID, organizationID, relatedPartyID string, deletedAt time.Time) InstrumentRelatedPartyDeletedPayload {
	return InstrumentRelatedPartyDeletedPayload{
		InstrumentID:   instrumentID,
		HolderID:       holderID,
		OrganizationID: organizationID,
		RelatedPartyID: relatedPartyID,
		DeletedAt:      deletedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter.
//
// Subject is the INSTRUMENT ID, not the related-party ID: the aggregate is the
// instrument, and ce-subject identifies the aggregate. This differs from the
// other events in this package, where Subject == p.ID.
//
// Returns a wrapped json.Marshal error so callers can decide whether to log
// Warn (IMPORTANT posture) or fail the request (CRITICAL posture).
func (p InstrumentRelatedPartyDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", InstrumentRelatedPartyDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: InstrumentRelatedPartyDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.InstrumentID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
