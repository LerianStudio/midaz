// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/google/uuid"
)

// RuleDeletedDefinition is the routing contract for rule.deleted.
// Subject (ce-subject) is the rule ID.
var RuleDeletedDefinition = Definition{
	ResourceType:  "rule",
	EventType:     "deleted",
	SchemaVersion: "1.0.0",
}

// RuleDeletedPayload is the leanest rule wire payload: just the identity and
// the soft-delete timestamp. No status — deletion is terminal.
type RuleDeletedPayload struct {
	ID        string `json:"id"`
	DeletedAt string `json:"deletedAt"`
}

// NewRuleDeleted maps the identity and delete timestamp into the wire
// payload.
//
// Deviation from the New<Event>(domain) template: the tracer delete use case
// returns no entity (it soft-deletes by ID and returns only error), so there
// is no post-delete Rule to read DeletedAt from. The constructor therefore
// takes primitives — the caller captures deletedAt at the delete site and
// passes it here.
func NewRuleDeleted(id uuid.UUID, deletedAt time.Time) RuleDeletedPayload {
	return RuleDeletedPayload{
		ID:        id.String(),
		DeletedAt: deletedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest; ts is typically the
// same captured deletedAt passed to the constructor.
func (p RuleDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", RuleDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: RuleDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
