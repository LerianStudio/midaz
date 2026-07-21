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

// PortfolioCreatedDefinition is the routing contract for portfolio.created.
// Emission anchor: components/ledger/internal/services/command/create_portfolio.go,
// immediately after PortfolioRepo.Create succeeds and before CreateOnboardingMetadata.
// IMPORTANT posture: emit failures MUST NOT fail the request.
var PortfolioCreatedDefinition = Definition{
	ResourceType:  "portfolio",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// PortfolioStatusPayload mirrors mmodel.Status for portfolio events
// without embedding domain types directly into the wire contract.
// Description is optional and omitted when nil.
type PortfolioStatusPayload struct {
	Code        string  `json:"code"`
	Description *string `json:"description,omitempty"`
}

// PortfolioCreatedPayload is the wire payload for portfolio.created.
// EntityID is optional (the request may omit it) and is omitted from
// the wire when empty.
type PortfolioCreatedPayload struct {
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organizationId"`
	LedgerID       string                 `json:"ledgerId"`
	Name           string                 `json:"name"`
	EntityID       string                 `json:"entityId,omitempty"`
	Status         PortfolioStatusPayload `json:"status"`
	CreatedAt      string                 `json:"createdAt"`
	UpdatedAt      string                 `json:"updatedAt"`
}

// NewPortfolioCreated maps a persisted portfolio into the wire payload.
//
// Caller invariant: p must be the value returned by PortfolioRepo.Create
// (post-commit), not the input struct. Specifically p.ID, p.CreatedAt,
// and p.UpdatedAt must reflect the persisted state.
func NewPortfolioCreated(p *mmodel.Portfolio) PortfolioCreatedPayload {
	return PortfolioCreatedPayload{
		ID:             p.ID,
		OrganizationID: p.OrganizationID,
		LedgerID:       p.LedgerID,
		Name:           p.Name,
		EntityID:       p.EntityID,
		Status:         newPortfolioStatusPayload(p.Status),
		CreatedAt:      p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      p.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p PortfolioCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", PortfolioCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: PortfolioCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}

func newPortfolioStatusPayload(status mmodel.Status) PortfolioStatusPayload {
	return PortfolioStatusPayload{
		Code:        status.Code,
		Description: status.Description,
	}
}
