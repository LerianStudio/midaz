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

// HolderCreatedDefinition is the routing contract for holder.created.
// IMPORTANT posture: emit failures MUST NOT fail the request; durability is
// owned by PG + (follow-up task) the outbox subsystem.
var HolderCreatedDefinition = Definition{
	ResourceType:  "holder",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// HolderCreatedPayload is the wire payload for holder.created. This struct is
// the canonical contract; consumers and tests read it as the source of truth.
//
// Holder is a regulated entity: name, document (CPF/CNPJ), contact, addresses,
// and the natural/legal-person sub-objects are PII and are DELIBERATELY ABSENT
// from this payload. Only stable identifiers, the organization scope, the
// person-type classification, the optional client correlation externalId, and
// timestamps cross the wire. The JSONShape test (holder_created_test.go) locks
// both the present key set AND the absence of every PII key.
//
// Fields are typed independently of mmodel.Holder so domain evolution does not
// silently shift the wire contract.
type HolderCreatedPayload struct {
	// Required core identity + scope.
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`

	// Type is the person classification: NATURAL_PERSON or LEGAL_PERSON. Not
	// PII — it is a non-identifying category required for routing/consumer
	// dispatch.
	Type string `json:"type"`

	// ExternalID is an optional client-supplied correlation identifier.
	// Encoded as JSON null when unset.
	ExternalID *string `json:"externalId"`

	// RFC3339-formatted timestamps.
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// NewHolderCreated maps a persisted holder into the wire payload. Holder
// carries no organization scope on the domain model, so organizationID is
// supplied explicitly by the emit site (the use-case method parameter).
//
// PII is dropped here, not downstream: name, document, contact, addresses, and
// the natural/legal-person sub-objects are never read.
//
// h.ID and h.Type are *-typed on mmodel.Holder; a persisted holder always has
// both set. They are dereferenced through nil-safe helpers so a partially-built
// holder maps to empty strings rather than panicking.
func NewHolderCreated(h *mmodel.Holder, organizationID string) HolderCreatedPayload {
	return HolderCreatedPayload{
		ID:             derefUUIDString(h.ID),
		OrganizationID: organizationID,
		Type:           derefString(h.Type),
		ExternalID:     h.ExternalID,
		CreatedAt:      h.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      h.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the Emitter.
// tenantID comes from pkgStreaming.ResolveTenantID(ctx); ts is the timestamp
// lib-streaming stamps on the ce-time header — typically the persisted
// CreatedAt for "created" events.
//
// Returns a wrapped json.Marshal error so callers can decide whether to log
// Warn (IMPORTANT posture) or fail the request (CRITICAL posture).
func (p HolderCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", HolderCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: HolderCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
