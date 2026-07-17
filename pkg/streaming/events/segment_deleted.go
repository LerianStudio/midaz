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

// SegmentDeletedDefinition is the routing contract for segment.deleted.
// Emission anchor: components/ledger/internal/services/command/delete_segment.go,
// immediately after SegmentRepo.Delete succeeds (post-commit).
//
// IMPORTANT posture: emit failures MUST NOT fail the request.
var SegmentDeletedDefinition = Definition{
	ResourceType:  "segment",
	EventType:     "deleted",
	SchemaVersion: "1.0.0",
}

// SegmentDeletedPayload is the wire payload for segment.deleted.
// Kept intentionally minimal: identity, tenant scope (org/ledger), and
// the soft-delete timestamp.
//
// Idempotency hint for consumers: `id + deletedAt` is unique per
// soft-delete; consumers safe-deduping on that pair can replay this
// event without effect.
type SegmentDeletedPayload struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	DeletedAt      string `json:"deletedAt"`
}

// NewSegmentDeleted maps the segment identity and post-commit deletedAt
// timestamp into the wire payload. The use case does not return the
// persisted struct on delete, so the caller captures deletedAt at the
// emit site.
func NewSegmentDeleted(id, organizationID, ledgerID string, deletedAt time.Time) SegmentDeletedPayload {
	return SegmentDeletedPayload{
		ID:             id,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		DeletedAt:      deletedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest ready for the
// Emitter. Source, ResourceType, EventType, and SchemaVersion live in
// the Catalog under DefinitionKey.
func (p SegmentDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", SegmentDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: SegmentDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
