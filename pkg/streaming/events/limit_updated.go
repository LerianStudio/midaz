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

// LimitUpdatedDefinition is the routing contract for limit.updated.
// Subject (ce-subject) is the limit ID.
var LimitUpdatedDefinition = Definition{
	ResourceType:  "limit",
	EventType:     "updated",
	SchemaVersion: "1.0.0",
}

// LimitUpdatedPayload is the wire payload for limit.updated. It carries the
// same twelve-field shape as LimitCreatedPayload; the fence EXCLUDES name,
// description, and maxAmount.
type LimitUpdatedPayload struct {
	ID              string             `json:"id"`
	Status          string             `json:"status"`
	LimitType       string             `json:"limitType"`
	Currency        string             `json:"currency"`
	Scopes          []RuleScopePayload `json:"scopes"`
	ActiveTimeStart *string            `json:"activeTimeStart"`
	ActiveTimeEnd   *string            `json:"activeTimeEnd"`
	CustomStartDate *string            `json:"customStartDate"`
	CustomEndDate   *string            `json:"customEndDate"`
	ResetAt         *string            `json:"resetAt"`
	CreatedAt       string             `json:"createdAt"`
	UpdatedAt       string             `json:"updatedAt"`
}

// NewLimitUpdated maps a persisted limit into the wire payload. Status and
// LimitType are mapped via a plain string conversion.
func NewLimitUpdated(limit *model.Limit) LimitUpdatedPayload {
	return LimitUpdatedPayload{
		ID:              limit.ID.String(),
		Status:          string(limit.Status),
		LimitType:       string(limit.LimitType),
		Currency:        limit.Currency,
		Scopes:          newRuleScopePayloads(limit.Scopes),
		ActiveTimeStart: formatOptionalTimeOfDay(limit.ActiveTimeStart),
		ActiveTimeEnd:   formatOptionalTimeOfDay(limit.ActiveTimeEnd),
		CustomStartDate: formatOptionalRFC3339(limit.CustomStartDate),
		CustomEndDate:   formatOptionalRFC3339(limit.CustomEndDate),
		ResetAt:         formatOptionalRFC3339(limit.ResetAt),
		CreatedAt:       limit.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       limit.UpdatedAt.Format(time.RFC3339),
	}
}

// ToEmitRequest assembles a libStreaming.EmitRequest; ts is typically the
// persisted UpdatedAt for this event.
func (p LimitUpdatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", LimitUpdatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: LimitUpdatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
