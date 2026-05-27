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

// SegmentCreatedDefinition is the routing contract for segment.created.
// Emission anchor: components/ledger/internal/services/command/create_segment.go,
// immediately after SegmentRepo.Create succeeds and before CreateOnboardingMetadata.
// IMPORTANT posture: emit failures MUST NOT fail the request.
var SegmentCreatedDefinition = Definition{
	ResourceType:  "segment",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// SegmentStatusPayload mirrors mmodel.Status for segment events without
// embedding domain types directly into the wire contract. Description
// is optional and omitted when nil.
type SegmentStatusPayload struct {
	Code        string  `json:"code"`
	Description *string `json:"description,omitempty"`
}

// SegmentCreatedPayload is the wire payload for segment.created.
type SegmentCreatedPayload struct {
	ID             string               `json:"id"`
	OrganizationID string               `json:"organizationId"`
	LedgerID       string               `json:"ledgerId"`
	Name           string               `json:"name"`
	Status         SegmentStatusPayload `json:"status"`
	CreatedAt      string               `json:"createdAt"`
	UpdatedAt      string               `json:"updatedAt"`
}

// NewSegmentCreated maps a persisted segment into the wire payload.
//
// Caller invariant: s must be the value returned by SegmentRepo.Create
// (post-commit), not the input struct. Specifically s.ID, s.CreatedAt,
// and s.UpdatedAt must reflect the persisted state.
func NewSegmentCreated(s *mmodel.Segment) SegmentCreatedPayload {
	return SegmentCreatedPayload{
		ID:             s.ID,
		OrganizationID: s.OrganizationID,
		LedgerID:       s.LedgerID,
		Name:           s.Name,
		Status:         newSegmentStatusPayload(s.Status),
		CreatedAt:      s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      s.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p SegmentCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", SegmentCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: SegmentCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}

func newSegmentStatusPayload(status mmodel.Status) SegmentStatusPayload {
	return SegmentStatusPayload{
		Code:        status.Code,
		Description: status.Description,
	}
}
