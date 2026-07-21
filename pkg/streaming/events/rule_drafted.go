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

// RuleDraftedDefinition is the routing contract for rule.drafted.
// Subject (ce-subject) is the rule ID.
var RuleDraftedDefinition = Definition{
	ResourceType:  "rule",
	EventType:     "drafted",
	SchemaVersion: "1.0.0",
}

// RuleDraftedPayload is the minimal wire payload for rule.drafted. It omits
// activatedAt and deactivatedAt because SetStatus(DRAFT) clears both — the
// contract keeps drafted lean rather than emitting null noise.
type RuleDraftedPayload struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
}

// NewRuleDrafted maps a rule into the rule.drafted wire payload.
func NewRuleDrafted(rule *model.Rule) RuleDraftedPayload {
	return RuleDraftedPayload{
		ID:        rule.ID.String(),
		Status:    rule.Status.String(),
		UpdatedAt: rule.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest; ts is typically the
// persisted UpdatedAt for this event.
func (p RuleDraftedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", RuleDraftedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: RuleDraftedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
