// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// InstrumentUpdatedDefinition is the routing contract for instrument.updated.
// IMPORTANT posture: emit failures MUST NOT fail the request; durability is
// owned by PG + (follow-up task) the outbox subsystem.
var InstrumentUpdatedDefinition = Definition{
	ResourceType:  "instrument",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// InstrumentUpdatedPayload is the wire payload for instrument.updated. It
// mirrors the instrument.created contract exactly (same key set, same PII
// exclusions); the distinction is the event key and that the emit site stamps
// UpdatedAt as the event timestamp. See InstrumentCreatedPayload for the
// PII-exclusion rationale.
//
// Fields are typed independently of mmodel.Instrument so domain evolution does
// not silently shift the wire contract.
type InstrumentUpdatedPayload struct {
	ID             string `json:"id"`
	HolderID       string `json:"holderId"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	AccountID      string `json:"accountId"`

	Type string `json:"type"`

	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`

	RelatedParties []InstrumentRelatedPartyPayload `json:"relatedParties"`
}

// NewInstrumentUpdated maps a persisted instrument into the wire payload.
// organizationID is supplied explicitly by the emit site. PII is dropped here
// (document, banking details, regulatory fields, related-party
// document/name/dates).
func NewInstrumentUpdated(i *mmodel.Instrument, organizationID string) InstrumentUpdatedPayload {
	return InstrumentUpdatedPayload{
		ID:             derefUUIDString(i.ID),
		HolderID:       derefUUIDString(i.HolderID),
		OrganizationID: organizationID,
		LedgerID:       derefString(i.LedgerID),
		AccountID:      derefString(i.AccountID),
		Type:           derefString(i.Type),
		CreatedAt:      i.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      i.UpdatedAt.Format(time.RFC3339),
		RelatedParties: mapInstrumentRelatedParties(i.RelatedParties),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter. ts
// is typically the persisted UpdatedAt for "updated" events. Subject is the
// instrument ID (the aggregate).
//
// Returns a wrapped json.Marshal error so callers can decide whether to log
// Warn (IMPORTANT posture) or fail the request (CRITICAL posture).
func (p InstrumentUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", InstrumentUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: InstrumentUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
