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

// LimitDeletedDefinition is the routing contract for limit.deleted.
// Subject (ce-subject) is the limit ID.
var LimitDeletedDefinition = Definition{
	ResourceType:  "limit",
	EventType:     "deleted",
	SchemaVersion: "1.0.0",
}

// LimitDeletedPayload is the leanest limit wire payload: just the identity and
// the soft-delete timestamp. No status — deletion is terminal.
type LimitDeletedPayload struct {
	ID        string `json:"id"`
	DeletedAt string `json:"deletedAt"`
}

// NewLimitDeleted maps a soft-deleted limit into the wire payload. A nil
// DeletedAt falls back to an empty deletedAt rather than panicking.
func NewLimitDeleted(limit *model.Limit) LimitDeletedPayload {
	deletedAt := ""
	if limit.DeletedAt != nil {
		deletedAt = limit.DeletedAt.Format(time.RFC3339)
	}

	return LimitDeletedPayload{
		ID:        limit.ID.String(),
		DeletedAt: deletedAt,
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest; ts is the limit's delete
// timestamp.
func (p LimitDeletedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", LimitDeletedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: LimitDeletedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
