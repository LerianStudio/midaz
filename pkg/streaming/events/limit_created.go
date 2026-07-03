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

// formatOptionalTimeOfDay formats an optional TimeOfDay as an "HH:MM" *string,
// returning nil when the input is nil so the wire serializes JSON null. A
// non-nil but zero-value TimeOfDay stringifies to "" (never panics); the domain
// validates windows at construction so that cannot occur post-NewLimit.
func formatOptionalTimeOfDay(t *model.TimeOfDay) *string {
	if t == nil {
		return nil
	}

	s := t.String()

	return &s
}

// LimitCreatedDefinition is the routing contract for limit.created.
// Subject (ce-subject) is the limit ID. IMPORTANT posture: emit failures
// MUST NOT fail the request.
var LimitCreatedDefinition = Definition{
	ResourceType:  "limit",
	EventType:     "created",
	SchemaVersion: "1.0.0",
}

// LimitCreatedPayload is the wire payload for limit.created. Fields are typed
// independently of model.Limit so domain evolution does not leak onto the
// wire. The payload fence EXCLUDES name, description (free text), and maxAmount
// (a financial value — consumers fetch the amount by id). The nested Scopes
// reuse RuleScopePayload since model.Limit.Scopes is the same []model.Scope
// type as model.Rule.Scopes.
type LimitCreatedPayload struct {
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

// NewLimitCreated maps a persisted limit into the wire payload. Status and
// LimitType are mapped via a plain string conversion (LimitStatus/LimitType
// have no String() method, unlike RuleStatus). Every field must be assigned
// here; the JSONShape test locks the field count so drift surfaces at test
// time.
func NewLimitCreated(limit *model.Limit) LimitCreatedPayload {
	return LimitCreatedPayload{
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

// ToEmitRequest assembles a libStreaming.EmitRequest. tenantID comes from
// pkgStreaming.ResolveTenantID(ctx); ts is the ce-time — typically the
// persisted CreatedAt for this event. Returns a wrapped json.Marshal error
// so IMPORTANT-posture callers can log Warn without failing the request.
func (p LimitCreatedPayload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", LimitCreatedDefinition.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: LimitCreatedDefinition.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
