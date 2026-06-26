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

// PortfolioUpdatedDefinition is the routing contract for portfolio.updated.
// Emission anchor: components/ledger/internal/services/command/update_portfolio.go,
// immediately after PortfolioRepo.Update succeeds and before UpdateOnboardingMetadata.
// IMPORTANT posture: emit failures MUST NOT fail the request.
//
// Idempotency hint for consumers: `id + updatedAt` is unique per
// mutation; consumers safe-deduping on that pair can replay this event
// without effect.
var PortfolioUpdatedDefinition = Definition{
	ResourceType:  "portfolio",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// PortfolioUpdatedPayload is the wire payload for portfolio.updated. The
// payload carries the full mutable surface (name, entityId, status) so
// consumers don't need to join against portfolio.created to render the
// row. CreatedAt is intentionally omitted — pinned at create time and
// not part of the update fact.
type PortfolioUpdatedPayload struct {
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organizationId"`
	LedgerID       string                 `json:"ledgerId"`
	Name           string                 `json:"name"`
	EntityID       string                 `json:"entityId,omitempty"`
	Status         PortfolioStatusPayload `json:"status"`
	UpdatedAt      string                 `json:"updatedAt"`
}

// NewPortfolioUpdated maps the post-update portfolio record into the
// wire payload.
//
// Caller invariant: p must be the value returned by PortfolioRepo.Update
// (post-commit), not the input struct. Specifically p.UpdatedAt must
// reflect the persisted timestamp.
func NewPortfolioUpdated(p *mmodel.Portfolio) PortfolioUpdatedPayload {
	return PortfolioUpdatedPayload{
		ID:             p.ID,
		OrganizationID: p.OrganizationID,
		LedgerID:       p.LedgerID,
		Name:           p.Name,
		EntityID:       p.EntityID,
		Status:         newPortfolioStatusPayload(p.Status),
		UpdatedAt:      p.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p PortfolioUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", PortfolioUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: PortfolioUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
