// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// LimitDraftedDefinition is the routing contract for limit.drafted.
// Subject (ce-subject) is the limit ID.
var LimitDraftedDefinition = Definition{
	ResourceType:  "limit",
	EventType:     "drafted",
	SchemaVersion: "1.0.0",
}

// LimitDraftedPayload is the minimal wire payload for limit.drafted. model.Limit
// has no transition-timestamp fields, so the payload carries only identity,
// status, and the update time.
type LimitDraftedPayload struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
}

// NewLimitDrafted maps a limit into the limit.drafted wire payload.
func NewLimitDrafted(limit *model.Limit) LimitDraftedPayload {
	return LimitDraftedPayload{
		ID:        limit.ID.String(),
		Status:    string(limit.Status),
		UpdatedAt: limit.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest; ts is typically the
// persisted UpdatedAt for this event.
func (p LimitDraftedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", LimitDraftedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: LimitDraftedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
