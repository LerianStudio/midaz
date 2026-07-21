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

// InstrumentCreatedDefinition is the routing contract for instrument.created.
// IMPORTANT posture: emit failures MUST NOT fail the request; durability is
// owned by PG + (follow-up task) the outbox subsystem.
var InstrumentCreatedDefinition = Definition{
	ResourceType:  "instrument",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// InstrumentRelatedPartyPayload is the wire shape for a single related party
// carried on instrument.created / instrument.updated. It mirrors
// mmodel.RelatedParty explicitly and DELIBERATELY carries only the stable
// identifier and the non-PII role.
//
// mmodel.RelatedParty additionally holds document, name, and relationship dates
// (startDate/endDate); all of those are PII and are never read into this
// payload. The JSONShape tests lock both the present keys and the absence of
// every PII key inside the array element.
type InstrumentRelatedPartyPayload struct {
	RelatedPartyID string `json:"relatedPartyId"`
	Role           string `json:"role"`
}

// InstrumentCreatedPayload is the wire payload for instrument.created. This
// struct is the canonical contract; consumers and tests read it as the source
// of truth.
//
// Instrument is a regulated entity: document (CPF/CNPJ), banking details (IBAN,
// branch, account), and the regulatory-fields sub-object are PII/sensitive and
// are DELIBERATELY ABSENT from this payload. Only stable identifiers, the
// organization/holder/ledger/account scope, the type classification,
// timestamps, and the reduced related-party list cross the wire. The JSONShape
// test locks both the present key set AND the absence of every PII key.
//
// Fields are typed independently of mmodel.Instrument so domain evolution does
// not silently shift the wire contract.
type InstrumentCreatedPayload struct {
	ID             string `json:"id"`
	HolderID       string `json:"holderId"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	AccountID      string `json:"accountId"`

	// Type is the instrument classification (e.g. NATURAL_PERSON /
	// LEGAL_PERSON). Not PII — a non-identifying category.
	Type string `json:"type"`

	// RFC3339-formatted timestamps.
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`

	// RelatedParties carries only {relatedPartyId, role} per entry. Encoded as
	// JSON null when the instrument has no related parties.
	RelatedParties []InstrumentRelatedPartyPayload `json:"relatedParties"`
}

// NewInstrumentCreated maps a persisted instrument into the wire payload.
// Instrument carries no organization scope on the domain model, so
// organizationID is supplied explicitly by the emit site (the use-case method
// parameter).
//
// PII is dropped here, not downstream: document, banking details, regulatory
// fields, and each related party's document/name/dates are never read.
//
// The instrument scalars are *-typed on mmodel.Instrument; a persisted
// instrument always has them set. They are dereferenced through nil-safe
// helpers so a partially-built instrument maps to empty strings rather than
// panicking.
func NewInstrumentCreated(i *mmodel.Instrument, organizationID string) InstrumentCreatedPayload {
	return InstrumentCreatedPayload{
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

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter.
// tenantID comes from pkgStreaming.ResolveTenantID(ctx); ts is the timestamp
// lib-streaming stamps on the ce-time header — typically the persisted
// CreatedAt for "created" events. Subject is the instrument ID (the aggregate).
//
// Returns a wrapped json.Marshal error so callers can decide whether to log
// Warn (IMPORTANT posture) or fail the request (CRITICAL posture).
func (p InstrumentCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", InstrumentCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: InstrumentCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}

// mapInstrumentRelatedParties reduces the domain related-party slice to the
// wire shape, carrying only the stable ID and the non-PII role. A nil slice
// maps to nil (JSON null); nil entries are skipped so a malformed slice cannot
// panic.
func mapInstrumentRelatedParties(parties []*mmodel.RelatedParty) []InstrumentRelatedPartyPayload {
	if len(parties) == 0 {
		return nil
	}

	out := make([]InstrumentRelatedPartyPayload, 0, len(parties))

	for _, rp := range parties {
		if rp == nil {
			continue
		}

		out = append(out, InstrumentRelatedPartyPayload{
			RelatedPartyID: derefUUIDString(rp.ID),
			Role:           rp.Role,
		})
	}

	return out
}
