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

// LimitActivatedDefinition is the routing contract for limit.activated.
// Subject (ce-subject) is the limit ID.
var LimitActivatedDefinition = Definition{
	ResourceType:  "limit",
	EventType:     "activated",
	SchemaVersion: "1.0.0",
}

// LimitActivatedPayload is the minimal wire payload for limit.activated.
// model.Limit has no ActivatedAt/DeactivatedAt fields (unlike model.Rule), so
// the transition events carry only identity, status, and the update time.
type LimitActivatedPayload struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
}

// NewLimitActivated maps a limit into the limit.activated wire payload. Status
// is mapped via a plain string conversion (LimitStatus has no String()).
func NewLimitActivated(limit *model.Limit) LimitActivatedPayload {
	return LimitActivatedPayload{
		ID:        limit.ID.String(),
		Status:    string(limit.Status),
		UpdatedAt: limit.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest; ts is typically the
// persisted UpdatedAt for this event.
func (p LimitActivatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", LimitActivatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: LimitActivatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
