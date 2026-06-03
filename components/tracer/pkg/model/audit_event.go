// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
)

// ActorType represents the type of actor that performed an action.
type ActorType string

const (
	ActorTypeUser   ActorType = "user"
	ActorTypeAPIKey ActorType = "api_key"
	ActorTypeSystem ActorType = "system"
)

// IsValid checks if the ActorType is a valid enum value.
func (a ActorType) IsValid() bool {
	switch a {
	case ActorTypeUser, ActorTypeAPIKey, ActorTypeSystem:
		return true
	default:
		return false
	}
}

// AuditEventType represents the type of audit event.
type AuditEventType string

const (
	// Transaction validation events
	AuditEventTransactionValidated AuditEventType = "TRANSACTION_VALIDATED"

	// Rule lifecycle events
	AuditEventRuleCreated     AuditEventType = "RULE_CREATED"
	AuditEventRuleUpdated     AuditEventType = "RULE_UPDATED"
	AuditEventRuleActivated   AuditEventType = "RULE_ACTIVATED"
	AuditEventRuleDeactivated AuditEventType = "RULE_DEACTIVATED"
	AuditEventRuleDrafted     AuditEventType = "RULE_DRAFTED"
	AuditEventRuleDeleted     AuditEventType = "RULE_DELETED"

	// Limit lifecycle events
	AuditEventLimitCreated     AuditEventType = "LIMIT_CREATED"
	AuditEventLimitUpdated     AuditEventType = "LIMIT_UPDATED"
	AuditEventLimitDeleted     AuditEventType = "LIMIT_DELETED"
	AuditEventLimitActivated   AuditEventType = "LIMIT_ACTIVATED"
	AuditEventLimitDeactivated AuditEventType = "LIMIT_DEACTIVATED"
	AuditEventLimitDrafted     AuditEventType = "LIMIT_DRAFTED"
)

// IsValid checks if the AuditEventType is a valid enum value.
func (t AuditEventType) IsValid() bool {
	switch t {
	case AuditEventTransactionValidated,
		AuditEventRuleCreated, AuditEventRuleUpdated, AuditEventRuleActivated, AuditEventRuleDeactivated, AuditEventRuleDrafted, AuditEventRuleDeleted,
		AuditEventLimitCreated, AuditEventLimitUpdated, AuditEventLimitDeleted, AuditEventLimitActivated, AuditEventLimitDeactivated, AuditEventLimitDrafted:
		return true
	default:
		return false
	}
}

// AuditAction represents the action performed.
type AuditAction string

const (
	AuditActionValidate   AuditAction = "VALIDATE"
	AuditActionCreate     AuditAction = "CREATE"
	AuditActionUpdate     AuditAction = "UPDATE"
	AuditActionDelete     AuditAction = "DELETE"
	AuditActionActivate   AuditAction = "ACTIVATE"
	AuditActionDeactivate AuditAction = "DEACTIVATE"
	AuditActionDraft      AuditAction = "DRAFT"
)

// IsValid checks if the AuditAction is a valid enum value.
func (a AuditAction) IsValid() bool {
	switch a {
	case AuditActionValidate, AuditActionCreate, AuditActionUpdate, AuditActionDelete, AuditActionActivate, AuditActionDeactivate, AuditActionDraft:
		return true
	default:
		return false
	}
}

// AuditResult represents the result of an action (unified field).
// For validations: ALLOW, DENY, REVIEW
// For CRUD operations: SUCCESS, FAILED
type AuditResult string

const (
	// CRUD operation results
	AuditResultSuccess AuditResult = "SUCCESS"
	AuditResultFailed  AuditResult = "FAILED"

	// Validation results (same as Decision enum)
	AuditResultAllow  AuditResult = "ALLOW"
	AuditResultDeny   AuditResult = "DENY"
	AuditResultReview AuditResult = "REVIEW"
)

// IsValid checks if the AuditResult is a valid enum value.
func (r AuditResult) IsValid() bool {
	switch r {
	case AuditResultSuccess, AuditResultFailed, AuditResultAllow, AuditResultDeny, AuditResultReview:
		return true
	default:
		return false
	}
}

// ResourceType represents the type of resource affected.
type ResourceType string

const (
	ResourceTypeTransaction ResourceType = "transaction"
	ResourceTypeRule        ResourceType = "rule"
	ResourceTypeLimit       ResourceType = "limit"
)

// IsValid checks if the ResourceType is a valid enum value.
func (r ResourceType) IsValid() bool {
	switch r {
	case ResourceTypeTransaction, ResourceTypeRule, ResourceTypeLimit:
		return true
	default:
		return false
	}
}

// Actor represents who performed the action.
type Actor struct {
	ActorType ActorType `json:"actorType"`
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role,omitempty"`
	IPAddress string    `json:"ipAddress"`
}

// ValidationResponseContext holds additional validation-specific fields for context.response.
// Used when event_type = TRANSACTION_VALIDATED.
// NOTE: Does NOT embed EvaluationResult to avoid redundancy with AuditEvent.Result field.
// EvaluationResult fields are passed separately to WithValidationContext method.
type ValidationResponseContext struct {
	ProcessingTimeMs  float64            `json:"processingTimeMs"`
	LimitUsageDetails []LimitUsageDetail `json:"limitUsageDetails"`
}

// AuditEvent represents an immutable audit record for compliance (SOX/GLBA).
type AuditEvent struct {
	// Internal fields (system-managed)
	ID           int64  `json:"-"` // Internal sequence, not exposed
	Hash         string `json:"hash,omitempty"`
	PreviousHash string `json:"previousHash,omitempty"`

	// Core fields
	EventID   uuid.UUID      `json:"eventId" swaggertype:"string" format:"uuid"`
	EventType AuditEventType `json:"eventType"`
	CreatedAt time.Time      `json:"createdAt" format:"date-time"` // When event occurred
	Action    AuditAction    `json:"action" swaggertype:"string"`
	Result    AuditResult    `json:"result" swaggertype:"string"` // Unified: ALLOW/DENY/REVIEW for validations, SUCCESS/FAILED for CRUD

	// Resource fields
	ResourceID   string       `json:"resourceId"`
	ResourceType ResourceType `json:"resourceType"`

	// Actor fields
	Actor Actor `json:"actor"`

	// Context (JSONB)
	// Note: accountId, segmentId, portfolioId are in context.request.account (not first-level fields)
	// For validations: { request: {...}, response: {reason, processingTimeMs, matchedRuleIds, ...} }
	// For CRUD: { before: {...}, after: {...}, reason: "..." }
	Context map[string]any `json:"context,omitempty"`

	// Metadata (JSONB) - additional info like ticketId, correlationId
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewAuditEvent creates a new AuditEvent with validation.
// Returns error if:
//   - eventType is not valid → constant.ErrAuditEventInvalidType
//   - action is not valid → constant.ErrAuditEventInvalidAction
//   - result is not valid → constant.ErrAuditEventInvalidResult
//   - resourceID is empty → constant.ErrAuditEventResourceIDRequired
//   - resourceType is not valid → constant.ErrAuditEventInvalidResourceType
//   - actor.ID is empty → constant.ErrAuditEventActorIDRequired
func NewAuditEvent(
	eventType AuditEventType,
	action AuditAction,
	result AuditResult,
	resourceID string,
	resourceType ResourceType,
	actor Actor,
) (*AuditEvent, error) {
	// Normalize textual inputs
	normalizedResourceID := strings.TrimSpace(resourceID)
	normalizedActorID := strings.TrimSpace(actor.ID)

	// Validate eventType
	if !eventType.IsValid() {
		return nil, constant.ErrAuditEventInvalidType
	}

	// Validate action
	if !action.IsValid() {
		return nil, constant.ErrAuditEventInvalidAction
	}

	// Validate result
	if !result.IsValid() {
		return nil, constant.ErrAuditEventInvalidResult
	}

	// Validate resourceID
	if normalizedResourceID == "" {
		return nil, constant.ErrAuditEventResourceIDRequired
	}

	// Validate resourceType
	if !resourceType.IsValid() {
		return nil, constant.ErrAuditEventInvalidResourceType
	}

	// Validate actor.ActorType
	if !actor.ActorType.IsValid() {
		return nil, constant.ErrAuditEventActorTypeInvalid
	}

	// Validate actor.ID
	if normalizedActorID == "" {
		return nil, constant.ErrAuditEventActorIDRequired
	}

	// Update actor with normalized ID
	normalizedActor := actor
	normalizedActor.ID = normalizedActorID

	return &AuditEvent{
		EventID:      uuid.New(),
		EventType:    eventType,
		CreatedAt:    time.Now().UTC(),
		Action:       action,
		Result:       result,
		ResourceID:   normalizedResourceID,
		ResourceType: resourceType,
		Actor:        normalizedActor,
		Context:      make(map[string]any),
		Metadata:     make(map[string]any),
	}, nil
}

// WithContext sets the context data (request/response or before/after).
func (e *AuditEvent) WithContext(context map[string]any) *AuditEvent {
	e.Context = context
	return e
}

// WithValidationContext sets the context for a validation event.
// Structure: { request: requestSnapshot, response: {reason, processingTimeMs, matchedRuleIds, ...} }
// NOTE: EvaluationResult is passed separately (not embedded in ValidationResponseContext)
// to avoid redundancy with AuditEvent.Result which already contains the decision.
func (e *AuditEvent) WithValidationContext(request map[string]any, evalResult EvaluationResult, response ValidationResponseContext) *AuditEvent {
	e.Context = map[string]any{
		"request": request,
		"response": map[string]any{
			// From EvaluationResult (passed separately, NOT embedded in ValidationResponseContext)
			// NOTE: "decision" is included here for complete audit snapshot (SOX/GLBA compliance)
			// AuditEvent.Result also contains decision for efficient filtering/indexing
			"decision":         string(evalResult.Decision),
			"reason":           evalResult.Reason,
			"matchedRuleIds":   evalResult.MatchedRuleIDs,
			"evaluatedRuleIds": evalResult.EvaluatedRuleIDs,
			"totalRulesLoaded": evalResult.TotalRulesLoaded,
			"truncated":        evalResult.Truncated,
			// From ValidationResponseContext (additional fields)
			"processingTimeMs":  response.ProcessingTimeMs,
			"limitUsageDetails": response.LimitUsageDetails,
		},
	}

	return e
}

// WithCRUDContext sets the context for a CRUD event (create/update/delete).
// Structure: { before: {...}, after: {...}, reason: "..." }
func (e *AuditEvent) WithCRUDContext(before, after map[string]any, reason string) *AuditEvent {
	ctx := make(map[string]any)
	if before != nil {
		ctx["before"] = before
	}

	if after != nil {
		ctx["after"] = after
	}

	if reason != "" {
		ctx["reason"] = reason
	}

	e.Context = ctx

	return e
}

// WithMetadata sets additional metadata.
func (e *AuditEvent) WithMetadata(metadata map[string]any) *AuditEvent {
	e.Metadata = metadata
	return e
}

// DecisionToAuditResult converts a validation Decision to AuditResult.
// Returns AuditResultFailed for unknown Decision values (defensive fallback).
func DecisionToAuditResult(decision Decision) AuditResult {
	switch decision {
	case DecisionAllow:
		return AuditResultAllow
	case DecisionDeny:
		return AuditResultDeny
	case DecisionReview:
		return AuditResultReview
	default:
		return AuditResultFailed
	}
}

// --- Helper methods to extract validation-specific fields from context ---

// GetValidationRequest extracts the request snapshot from context.
func (e *AuditEvent) GetValidationRequest() map[string]any {
	if req, ok := e.Context["request"].(map[string]any); ok {
		return req
	}

	return nil
}

// GetValidationResponse extracts the response data from context.
func (e *AuditEvent) GetValidationResponse() map[string]any {
	if resp, ok := e.Context["response"].(map[string]any); ok {
		return resp
	}

	return nil
}

// GetReason extracts reason from context.response (validations) or context.reason (CRUD).
func (e *AuditEvent) GetReason() string {
	// Try context.response.reason (validations)
	if resp := e.GetValidationResponse(); resp != nil {
		if reason, ok := resp["reason"].(string); ok {
			return reason
		}
	}
	// Try context.reason (CRUD)
	if reason, ok := e.Context["reason"].(string); ok {
		return reason
	}

	return ""
}

// GetProcessingTimeMs extracts processingTimeMs from context.response.
func (e *AuditEvent) GetProcessingTimeMs() float64 {
	if resp := e.GetValidationResponse(); resp != nil {
		if pt, ok := resp["processingTimeMs"].(float64); ok {
			return pt
		}

		if pt, ok := resp["processingTimeMs"].(int64); ok {
			return float64(pt)
		}
	}

	return 0
}

// GetMatchedRuleIDs extracts matchedRuleIds from context.response.
func (e *AuditEvent) GetMatchedRuleIDs() []uuid.UUID {
	if resp := e.GetValidationResponse(); resp != nil {
		value := resp["matchedRuleIds"]
		switch v := value.(type) {
		case []uuid.UUID:
			// In-memory events store []uuid.UUID directly
			return v
		case []string:
			// Handle []string by parsing each element
			result := make([]uuid.UUID, 0, len(v))
			for _, s := range v {
				if uid, err := uuid.Parse(s); err == nil {
					result = append(result, uid)
				}
			}

			return result
		case []any:
			// Handle []any with string elements (e.g., from JSON unmarshaling)
			result := make([]uuid.UUID, 0, len(v))
			for _, id := range v {
				if s, ok := id.(string); ok {
					if uid, err := uuid.Parse(s); err == nil {
						result = append(result, uid)
					}
				}
			}

			return result
		}
	}

	return nil
}

// GetEvaluatedRuleIDs extracts evaluatedRuleIds from context.response.
func (e *AuditEvent) GetEvaluatedRuleIDs() []uuid.UUID {
	if resp := e.GetValidationResponse(); resp != nil {
		value := resp["evaluatedRuleIds"]
		switch v := value.(type) {
		case []uuid.UUID:
			// In-memory events store []uuid.UUID directly
			return v
		case []string:
			// Handle []string by parsing each element
			result := make([]uuid.UUID, 0, len(v))
			for _, s := range v {
				if uid, err := uuid.Parse(s); err == nil {
					result = append(result, uid)
				}
			}

			return result
		case []any:
			// Handle []any with string elements (e.g., from JSON unmarshaling)
			result := make([]uuid.UUID, 0, len(v))
			for _, id := range v {
				if s, ok := id.(string); ok {
					if uid, err := uuid.Parse(s); err == nil {
						result = append(result, uid)
					}
				}
			}

			return result
		}
	}

	return nil
}
