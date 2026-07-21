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

// RuleCreatedDefinition is the routing contract for rule.created.
// Subject (ce-subject) is the rule ID. IMPORTANT posture: emit failures
// MUST NOT fail the request.
var RuleCreatedDefinition = Definition{
	ResourceType:  "rule",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// RuleCreatedPayload is the wire payload for rule.created. Fields are typed
// independently of model.Rule so domain evolution does not leak onto the
// wire. The payload fence EXCLUDES name, description, expression, and the
// transient compiledProgram — those never appear on the wire.
type RuleCreatedPayload struct {
	ID        string             `json:"id"`
	Status    string             `json:"status"`
	Action    string             `json:"action"`
	Scopes    []RuleScopePayload `json:"scopes"`
	CreatedAt string             `json:"createdAt"`
	UpdatedAt string             `json:"updatedAt"`
}

// NewRuleCreated maps a persisted rule into the wire payload. Every field
// on RuleCreatedPayload must be assigned here; the JSONShape test locks the
// field count so additions and deletions surface at test time.
func NewRuleCreated(rule *model.Rule) RuleCreatedPayload {
	return RuleCreatedPayload{
		ID:        rule.ID.String(),
		Status:    rule.Status.String(),
		Action:    string(rule.Action),
		Scopes:    newRuleScopePayloads(rule.Scopes),
		CreatedAt: rule.CreatedAt.Format(time.RFC3339),
		UpdatedAt: rule.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest. tenantID comes from
// pkgStreaming.ResolveTenantID(ctx); ts is the ce-time — typically the
// persisted CreatedAt for this event. Returns a wrapped json.Marshal error
// so IMPORTANT-posture callers can log Warn without failing the request.
func (p RuleCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", RuleCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: RuleCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
