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

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p OrganizationDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", OrganizationDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: OrganizationDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
