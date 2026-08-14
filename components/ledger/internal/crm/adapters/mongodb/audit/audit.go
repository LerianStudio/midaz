// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package audit

import (
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
)

// AuditEventModel is the MongoDB representation of ProtectionAuditEvent.
// It MUST NOT hold plaintext, keysets, wrapped keysets, DEK/KEK material,
// credentials, balances, financial values, or PII.
type AuditEventModel struct {
	ID             string             `bson:"_id"`
	TenantID       string             `bson:"tenant_id,omitempty"`
	OrganizationID string             `bson:"organization_id"`
	EventType      string             `bson:"event_type"`
	Action         string             `bson:"action"`
	Outcome        string             `bson:"outcome"`
	ActorID        string             `bson:"actor_id"`
	ActorType      string             `bson:"actor_type"`
	Reason         string             `bson:"reason"`
	Timestamp      time.Time          `bson:"timestamp"`
	RequestID      string             `bson:"request_id"`
	ExpiresAt      *time.Time         `bson:"expires_at,omitempty"`
	Details        *AuditDetailsModel `bson:"details,omitempty"`
}

// AuditDetailsModel is the MongoDB representation of mmodel.AuditDetails.
type AuditDetailsModel struct {
	PreviousStatus    string   `bson:"previous_status"`
	NewStatus         string   `bson:"new_status"`
	AffectedKeyIDs    []uint32 `bson:"affected_key_ids"`
	ProviderReference string   `bson:"provider_reference"`
	ErrorCode         string   `bson:"error_code"`
}

// FromEntity converts a domain ProtectionAuditEvent to its MongoDB model.
// It leaves ExpiresAt nil (TTL is applied by a later task), maps the entity ID
// to the _id string, and preserves a nil Details as a nil embedded document.
func FromEntity(e *mmodel.ProtectionAuditEvent) *AuditEventModel {
	if e == nil {
		return nil
	}

	return &AuditEventModel{
		ID:             e.ID.String(),
		TenantID:       e.TenantID,
		OrganizationID: e.OrganizationID,
		EventType:      string(e.EventType),
		Action:         string(e.Action),
		Outcome:        string(e.Outcome),
		ActorID:        e.ActorID,
		ActorType:      e.ActorType,
		Reason:         e.Reason,
		Timestamp:      e.Timestamp,
		RequestID:      e.RequestID,
		ExpiresAt:      nil,
		Details:        detailsFromEntity(e.Details),
	}
}

// ToEntity converts the MongoDB model back to a domain ProtectionAuditEvent.
// An unparseable _id maps to uuid.Nil; a nil Details document round-trips as nil.
func (m *AuditEventModel) ToEntity() *mmodel.ProtectionAuditEvent {
	if m == nil {
		return nil
	}

	id, err := uuid.Parse(m.ID)
	if err != nil {
		id = uuid.Nil
	}

	return &mmodel.ProtectionAuditEvent{
		ID:             id,
		TenantID:       m.TenantID,
		OrganizationID: m.OrganizationID,
		EventType:      mmodel.AuditEventType(m.EventType),
		Action:         mmodel.AuditAction(m.Action),
		Outcome:        mmodel.AuditOutcome(m.Outcome),
		ActorID:        m.ActorID,
		ActorType:      m.ActorType,
		Reason:         m.Reason,
		Timestamp:      m.Timestamp,
		RequestID:      m.RequestID,
		Details:        m.Details.toEntity(),
	}
}

// detailsFromEntity maps domain AuditDetails to its MongoDB model, preserving
// a nil pointer and the nil-vs-empty distinction of AffectedKeyIDs.
func detailsFromEntity(d *mmodel.AuditDetails) *AuditDetailsModel {
	if d == nil {
		return nil
	}

	return &AuditDetailsModel{
		PreviousStatus:    d.PreviousStatus,
		NewStatus:         d.NewStatus,
		AffectedKeyIDs:    cloneKeyIDs(d.AffectedKeyIDs),
		ProviderReference: d.ProviderReference,
		ErrorCode:         d.ErrorCode,
	}
}

// toEntity maps the MongoDB details model back to domain AuditDetails, preserving
// a nil pointer and the nil-vs-empty distinction of AffectedKeyIDs.
func (m *AuditDetailsModel) toEntity() *mmodel.AuditDetails {
	if m == nil {
		return nil
	}

	return &mmodel.AuditDetails{
		PreviousStatus:    m.PreviousStatus,
		NewStatus:         m.NewStatus,
		AffectedKeyIDs:    cloneKeyIDs(m.AffectedKeyIDs),
		ProviderReference: m.ProviderReference,
		ErrorCode:         m.ErrorCode,
	}
}

// cloneKeyIDs copies the slice while preserving the nil-vs-empty distinction so
// the conversion round-trip is faithful.
func cloneKeyIDs(in []uint32) []uint32 {
	if in == nil {
		return nil
	}

	out := make([]uint32, len(in))
	copy(out, in)

	return out
}
