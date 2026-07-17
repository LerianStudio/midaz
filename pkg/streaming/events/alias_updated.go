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

// AliasUpdatedDefinition is the routing contract for alias.updated.
// IMPORTANT posture: emit failures MUST NOT fail the request; durability is
// owned by PG + (follow-up task) the outbox subsystem.
var AliasUpdatedDefinition = Definition{
	ResourceType:  "alias",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// AliasUpdatedPayload is the wire payload for alias.updated. It mirrors the
// alias.created contract exactly (same key set, same PII exclusions); the
// distinction is the event key and that the emit site stamps UpdatedAt as the
// event timestamp. See AliasCreatedPayload for the PII-exclusion rationale.
//
// Fields are typed independently of mmodel.Alias so domain evolution does not
// silently shift the wire contract.
type AliasUpdatedPayload struct {
	ID             string `json:"id"`
	HolderID       string `json:"holderId"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	AccountID      string `json:"accountId"`

	Type string `json:"type"`

	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`

	RelatedParties []AliasRelatedPartyPayload `json:"relatedParties"`
}

// NewAliasUpdated maps a persisted alias into the wire payload. organizationID
// is supplied explicitly by the emit site. PII is dropped here (document,
// banking details, regulatory fields, related-party document/name/dates).
func NewAliasUpdated(a *mmodel.Alias, organizationID string) AliasUpdatedPayload {
	return AliasUpdatedPayload{
		ID:             derefUUIDString(a.ID),
		HolderID:       derefUUIDString(a.HolderID),
		OrganizationID: organizationID,
		LedgerID:       derefString(a.LedgerID),
		AccountID:      derefString(a.AccountID),
		Type:           derefString(a.Type),
		CreatedAt:      a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      a.UpdatedAt.Format(time.RFC3339),
		RelatedParties: mapAliasRelatedParties(a.RelatedParties),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter. ts
// is typically the persisted UpdatedAt for "updated" events. Subject is the
// alias ID (the aggregate).
//
// Returns a wrapped json.Marshal error so callers can decide whether to log
// Warn (IMPORTANT posture) or fail the request (CRITICAL posture).
func (p AliasUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", AliasUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: AliasUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
