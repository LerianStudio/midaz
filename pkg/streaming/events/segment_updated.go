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

// SegmentUpdatedDefinition is the routing contract for segment.updated.
// Emission anchor: components/ledger/internal/services/command/update_segment.go,
// immediately after SegmentRepo.Update succeeds and before UpdateOnboardingMetadata.
// IMPORTANT posture: emit failures MUST NOT fail the request.
//
// Idempotency hint for consumers: `id + updatedAt` is unique per
// mutation; consumers safe-deduping on that pair can replay this event
// without effect.
var SegmentUpdatedDefinition = Definition{
	ResourceType:  "segment",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// SegmentUpdatedPayload is the wire payload for segment.updated. The
// payload carries the full mutable surface (name, status) so consumers
// don't need to join against segment.created to render the row.
// CreatedAt is intentionally omitted — pinned at create time and not
// part of the update fact.
type SegmentUpdatedPayload struct {
	ID             string               `json:"id"`
	OrganizationID string               `json:"organizationId"`
	LedgerID       string               `json:"ledgerId"`
	Name           string               `json:"name"`
	Status         SegmentStatusPayload `json:"status"`
	UpdatedAt      string               `json:"updatedAt"`
}

// NewSegmentUpdated maps the post-update segment record into the wire
// payload.
//
// Caller invariant: s must be the value returned by SegmentRepo.Update
// (post-commit), not the input struct. Specifically s.UpdatedAt must
// reflect the persisted timestamp.
func NewSegmentUpdated(s *mmodel.Segment) SegmentUpdatedPayload {
	return SegmentUpdatedPayload{
		ID:             s.ID,
		OrganizationID: s.OrganizationID,
		LedgerID:       s.LedgerID,
		Name:           s.Name,
		Status:         newSegmentStatusPayload(s.Status),
		UpdatedAt:      s.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p SegmentUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", SegmentUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: SegmentUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
