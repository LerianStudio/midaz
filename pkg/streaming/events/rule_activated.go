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

// formatOptionalRFC3339 formats an optional timestamp as an RFC3339 *string,
// returning nil when the input is nil so the wire serializes JSON null. Used
// by rule status-transition payloads whose transition timestamp fields are
// optional but must always be present as a key (never omitempty).
func formatOptionalRFC3339(ts *time.Time) *string {
	if ts == nil {
		return nil
	}

	s := ts.Format(time.RFC3339)

	return &s
}

// RuleActivatedDefinition is the routing contract for rule.activated.
// Subject (ce-subject) is the rule ID.
var RuleActivatedDefinition = Definition{
	ResourceType:  "rule",
	EventType:     "activated",
	SchemaVersion: "1.0.0",
}

// RuleActivatedPayload is the wire payload for rule.activated. ActivatedAt is
// a *string so a defensive nil serializes as JSON null; the key is always
// present (NOT omitempty) per the locked contract.
type RuleActivatedPayload struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	ActivatedAt *string `json:"activatedAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

// NewRuleActivated maps a rule into the rule.activated wire payload. After a
// real ACTIVE transition rule.ActivatedAt is non-nil, but the mapping is
// nil-guarded defensively.
func NewRuleActivated(rule *model.Rule) RuleActivatedPayload {
	return RuleActivatedPayload{
		ID:          rule.ID.String(),
		Status:      rule.Status.String(),
		ActivatedAt: formatOptionalRFC3339(rule.ActivatedAt),
		UpdatedAt:   rule.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest; ts is typically the
// persisted UpdatedAt for this event.
func (p RuleActivatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", RuleActivatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: RuleActivatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
