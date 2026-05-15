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

// OrganizationDeletedDefinition is the routing contract for organization.deleted.
// Emission anchor: components/ledger/internal/services/command/delete_organization.go,
// immediately after OrganizationRepo.Delete succeeds. IMPORTANT posture: emit
// failures MUST NOT fail the request.
var OrganizationDeletedDefinition = Definition{
	ResourceType:  "organization",
	EventType:     "deleted",
	SchemaVersion: "1.0.0",
}

// OrganizationDeletedPayload is the wire payload for organization.deleted.
type OrganizationDeletedPayload struct {
	ID        string `json:"id"`
	DeletedAt string `json:"deletedAt"`
}

// NewOrganizationDeleted maps the organization identity and post-delete timestamp.
func NewOrganizationDeleted(id string, deletedAt time.Time) OrganizationDeletedPayload {
	return OrganizationDeletedPayload{
		ID:        id,
		DeletedAt: deletedAt.Format(time.RFC3339),
	}
}

// ToEvent assembles a libStreaming.Event ready for the Emitter.
func (p OrganizationDeletedPayload) ToEvent(tenantID, source string, ts time.Time) (libStreaming.Event, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.Event{}, fmt.Errorf("marshal %s payload: %w", OrganizationDeletedDefinition.Key(), err)
	}

	return libStreaming.Event{
		TenantID:      tenantID,
		Source:        source,
		ResourceType:  OrganizationDeletedDefinition.ResourceType,
		EventType:     OrganizationDeletedDefinition.EventType,
		SchemaVersion: OrganizationDeletedDefinition.SchemaVersion,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
