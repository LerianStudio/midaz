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

// LimitDeactivatedDefinition is the routing contract for limit.deactivated.
// Subject (ce-subject) is the limit ID.
var LimitDeactivatedDefinition = Definition{
	ResourceType:  "limit",
	EventType:     "deactivated",
	SchemaVersion: "1.0.0",
}

// LimitDeactivatedPayload is the minimal wire payload for limit.deactivated.
// model.Limit has no DeactivatedAt field, so the payload carries only
// identity, status, and the update time.
type LimitDeactivatedPayload struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
}

// NewLimitDeactivated maps a limit into the limit.deactivated wire payload.
func NewLimitDeactivated(limit *model.Limit) LimitDeactivatedPayload {
	return LimitDeactivatedPayload{
		ID:        limit.ID.String(),
		Status:    string(limit.Status),
		UpdatedAt: limit.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest; ts is typically the
// persisted UpdatedAt for this event.
func (p LimitDeactivatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", LimitDeactivatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: LimitDeactivatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
