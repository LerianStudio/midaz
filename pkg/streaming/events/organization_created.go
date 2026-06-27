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

// OrganizationCreatedDefinition is the routing contract for organization.created.
// Emission anchor: components/ledger/internal/services/command/create_organization.go,
// immediately after OrganizationRepo.Create succeeds and before CreateOnboardingMetadata.
// IMPORTANT posture: emit failures MUST NOT fail the request.
var OrganizationCreatedDefinition = Definition{
	ResourceType:  "organization",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// OrganizationAddressPayload mirrors mmodel.Address for organization events
// without embedding domain types directly into the wire contract.
type OrganizationAddressPayload struct {
	Line1       string  `json:"line1"`
	Line2       *string `json:"line2"`
	ZipCode     string  `json:"zipCode"`
	City        string  `json:"city"`
	State       string  `json:"state"`
	Country     string  `json:"country"`
	Description *string `json:"description,omitempty"`
}

// OrganizationStatusPayload mirrors mmodel.Status for organization events.
// Description is optional and omitted when nil.
type OrganizationStatusPayload struct {
	Code        string  `json:"code"`
	Description *string `json:"description,omitempty"`
}

// OrganizationCreatedPayload is the wire payload for organization.created.
type OrganizationCreatedPayload struct {
	ID                   string                     `json:"id"`
	ParentOrganizationID *string                    `json:"parentOrganizationId"`
	LegalName            string                     `json:"legalName"`
	DoingBusinessAs      *string                    `json:"doingBusinessAs"`
	LegalDocument        string                     `json:"legalDocument"`
	Address              OrganizationAddressPayload `json:"address"`
	Status               OrganizationStatusPayload  `json:"status"`
	CreatedAt            string                     `json:"createdAt"`
	UpdatedAt            string                     `json:"updatedAt"`
}

// NewOrganizationCreated maps a persisted organization into the wire payload.
func NewOrganizationCreated(org *mmodel.Organization) OrganizationCreatedPayload {
	return OrganizationCreatedPayload{
		ID:                   org.ID,
		ParentOrganizationID: org.ParentOrganizationID,
		LegalName:            org.LegalName,
		DoingBusinessAs:      org.DoingBusinessAs,
		LegalDocument:        org.LegalDocument,
		Address:              newOrganizationAddressPayload(org.Address),
		Status:               newOrganizationStatusPayload(org.Status),
		CreatedAt:            org.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            org.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p OrganizationCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", OrganizationCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: OrganizationCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}

func newOrganizationAddressPayload(address mmodel.Address) OrganizationAddressPayload {
	return OrganizationAddressPayload{
		Line1:       address.Line1,
		Line2:       address.Line2,
		ZipCode:     address.ZipCode,
		City:        address.City,
		State:       address.State,
		Country:     address.Country,
		Description: address.Description,
	}
}

func newOrganizationStatusPayload(status mmodel.Status) OrganizationStatusPayload {
	return OrganizationStatusPayload{
		Code:        status.Code,
		Description: status.Description,
	}
}
