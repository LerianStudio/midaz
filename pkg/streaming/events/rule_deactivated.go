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

// RuleDeactivatedDefinition is the routing contract for rule.deactivated.
// Subject (ce-subject) is the rule ID.
var RuleDeactivatedDefinition = Definition{
	ResourceType:  "rule",
	EventType:     "deactivated",
	SchemaVersion: "1.0.0",
}

// RuleDeactivatedPayload is the wire payload for rule.deactivated.
// DeactivatedAt is a *string so a defensive nil serializes as JSON null; the
// key is always present (NOT omitempty) per the locked contract.
type RuleDeactivatedPayload struct {
	ID            string  `json:"id"`
	Status        string  `json:"status"`
	DeactivatedAt *string `json:"deactivatedAt"`
	UpdatedAt     string  `json:"updatedAt"`
}

// NewRuleDeactivated maps a rule into the rule.deactivated wire payload.
// After a real INACTIVE transition rule.DeactivatedAt is non-nil, but the
// mapping is nil-guarded defensively.
func NewRuleDeactivated(rule *model.Rule) RuleDeactivatedPayload {
	return RuleDeactivatedPayload{
		ID:            rule.ID.String(),
		Status:        rule.Status.String(),
		DeactivatedAt: formatOptionalRFC3339(rule.DeactivatedAt),
		UpdatedAt:     rule.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest; ts is typically the
// persisted UpdatedAt for this event.
func (p RuleDeactivatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", RuleDeactivatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: RuleDeactivatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
