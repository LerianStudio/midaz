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

// RuleUpdatedDefinition is the routing contract for rule.updated.
// Subject (ce-subject) is the rule ID.
var RuleUpdatedDefinition = Definition{
	ResourceType:  "rule",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// RuleUpdatedPayload is the wire payload for rule.updated. It shares the
// full rule shape with rule.created and honors the same fence (no name,
// description, expression, or compiledProgram).
type RuleUpdatedPayload struct {
	ID        string             `json:"id"`
	Status    string             `json:"status"`
	Action    string             `json:"action"`
	Scopes    []RuleScopePayload `json:"scopes"`
	CreatedAt string             `json:"createdAt"`
	UpdatedAt string             `json:"updatedAt"`
}

// NewRuleUpdated maps a persisted rule into the rule.updated wire payload.
func NewRuleUpdated(rule *model.Rule) RuleUpdatedPayload {
	return RuleUpdatedPayload{
		ID:        rule.ID.String(),
		Status:    rule.Status.String(),
		Action:    string(rule.Action),
		Scopes:    newRuleScopePayloads(rule.Scopes),
		CreatedAt: rule.CreatedAt.Format(time.RFC3339),
		UpdatedAt: rule.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest; ts is typically the
// persisted UpdatedAt for this event.
func (p RuleUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", RuleUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: RuleUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
