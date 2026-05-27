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

// OrganizationUpdatedDefinition is the routing contract for organization.updated.
// Emission anchor: components/ledger/internal/services/command/update_organization.go,
// immediately after OrganizationRepo.Update succeeds and before UpdateOnboardingMetadata.
// IMPORTANT posture: emit failures MUST NOT fail the request.
var OrganizationUpdatedDefinition = Definition{
	ResourceType:  "organization",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// OrganizationUpdatedPayload is the wire payload for organization.updated.
type OrganizationUpdatedPayload struct {
	ID                   string                     `json:"id"`
	ParentOrganizationID *string                    `json:"parentOrganizationId"`
	LegalName            string                     `json:"legalName"`
	DoingBusinessAs      *string                    `json:"doingBusinessAs"`
	Address              OrganizationAddressPayload `json:"address"`
	Status               OrganizationStatusPayload  `json:"status"`
	UpdatedAt            string                     `json:"updatedAt"`
}

// NewOrganizationUpdated maps a persisted organization into the wire payload.
func NewOrganizationUpdated(org *mmodel.Organization) OrganizationUpdatedPayload {
	return OrganizationUpdatedPayload{
		ID:                   org.ID,
		ParentOrganizationID: org.ParentOrganizationID,
		LegalName:            org.LegalName,
		DoingBusinessAs:      org.DoingBusinessAs,
		Address:              newOrganizationAddressPayload(org.Address),
		Status:               newOrganizationStatusPayload(org.Status),
		UpdatedAt:            org.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p OrganizationUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", OrganizationUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: OrganizationUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
