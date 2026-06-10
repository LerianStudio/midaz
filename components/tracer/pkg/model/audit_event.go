// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ActorType represents the type of actor that performed an action.
type ActorType string //	@name	ActorType

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
type AuditEventType string //	@name	AuditEventType

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

	// Reservation lifecycle events (two-phase reservation seam).
	// RESERVED/CONFIRMED/RELEASED/EXPIRED mirror the persisted
	// usage_reservations.status transitions. SKIPPED is audit-only: it records
	// the ledger fail-open decision when the tracer is unreachable, and is NEVER
	// a usage_reservations.status value (no reservation row is written).
	AuditEventReservationReserved  AuditEventType = "RESERVATION_RESERVED"
	AuditEventReservationConfirmed AuditEventType = "RESERVATION_CONFIRMED"
	AuditEventReservationReleased  AuditEventType = "RESERVATION_RELEASED"
	AuditEventReservationExpired   AuditEventType = "RESERVATION_EXPIRED"
	AuditEventReservationSkipped   AuditEventType = "RESERVATION_SKIPPED"
)

// IsValid checks if the AuditEventType is a valid enum value.
func (t AuditEventType) IsValid() bool {
	switch t {
	case AuditEventTransactionValidated,
		AuditEventRuleCreated, AuditEventRuleUpdated, AuditEventRuleActivated, AuditEventRuleDeactivated, AuditEventRuleDrafted, AuditEventRuleDeleted,
		AuditEventLimitCreated, AuditEventLimitUpdated, AuditEventLimitDeleted, AuditEventLimitActivated, AuditEventLimitDeactivated, AuditEventLimitDrafted,
		AuditEventReservationReserved, AuditEventReservationConfirmed, AuditEventReservationReleased, AuditEventReservationExpired, AuditEventReservationSkipped:
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

	// Reservation lifecycle actions (two-phase reservation seam). RESERVE holds
	// capacity, CONFIRM commits it, RELEASE returns it on abort, EXPIRE is the
	// reaper-driven release, and SKIP records a ledger fail-open with no counter
	// move.
	AuditActionReserve AuditAction = "RESERVE"
	AuditActionConfirm AuditAction = "CONFIRM"
	AuditActionRelease AuditAction = "RELEASE"
	AuditActionExpire  AuditAction = "EXPIRE"
	AuditActionSkip    AuditAction = "SKIP"
)

// IsValid checks if the AuditAction is a valid enum value.
func (a AuditAction) IsValid() bool {
	switch a {
	case AuditActionValidate, AuditActionCreate, AuditActionUpdate, AuditActionDelete, AuditActionActivate, AuditActionDeactivate, AuditActionDraft,
		AuditActionReserve, AuditActionConfirm, AuditActionRelease, AuditActionExpire, AuditActionSkip:
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
type ResourceType string //	@name	ResourceType

const (
	ResourceTypeTransaction ResourceType = "transaction"
	ResourceTypeRule        ResourceType = "rule"
	ResourceTypeLimit       ResourceType = "limit"
	ResourceTypeReservation ResourceType = "reservation"
)

// IsValid checks if the ResourceType is a valid enum value.
func (r ResourceType) IsValid() bool {
	switch r {
	case ResourceTypeTransaction, ResourceTypeRule, ResourceTypeLimit, ResourceTypeReservation:
		return true
	default:
		return false
	}
}

// Actor represents who performed the action.
type Actor struct {
	ActorType ActorType `json:"actorType" swaggertype:"string" enums:"user,api_key,system" example:"user"`
	ID        string    `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	Name      string    `json:"name" example:"Jane Doe"`
	Role      string    `json:"role,omitempty" example:"admin"`
	IPAddress string    `json:"ipAddress" example:"203.0.113.42"`
} //	@name	Actor

// ValidationResponseContext holds additional validation-specific fields for context.response.
// Used when event_type = TRANSACTION_VALIDATED.
// NOTE: Does NOT embed EvaluationResult to avoid redundancy with AuditEvent.Result field.
// EvaluationResult fields are passed separately to WithValidationContext method.
type ValidationResponseContext struct {
	ProcessingTimeMs  float64            `json:"processingTimeMs"`
	LimitUsageDetails []LimitUsageDetail `json:"limitUsageDetails"`
}

// AuditEvent represents an immutable audit record for compliance (SOX/GLBA).
//
// swagger:model AuditEvent
//
//	@Description	Immutable audit record for SOX/GLBA compliance. Each event captures who performed what action on which resource, the outcome, and a full context snapshot. Events are hash-chained to detect tampering; the hash covers all core fields.
type AuditEvent struct {
	// Internal fields (system-managed)
	ID int64 `json:"-"` // Internal sequence, not exposed

	// SHA-256 hash of this event's fields for tamper detection
	// example: a3f1e2b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2
	Hash string `json:"hash,omitempty" example:"a3f1e2b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2"`

	// Hash of the preceding event in the chain, empty for the first event
	// example: b4e2f3a5c6d7e8f9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3
	PreviousHash string `json:"previousHash,omitempty" example:"b4e2f3a5c6d7e8f9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3"`

	// Unique identifier for this audit event
	// format: uuid
	EventID uuid.UUID `json:"eventId" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// Type of event that occurred
	// example: TRANSACTION_VALIDATED
	// enums: TRANSACTION_VALIDATED,RULE_CREATED,RULE_UPDATED,RULE_ACTIVATED,RULE_DEACTIVATED,RULE_DRAFTED,RULE_DELETED,LIMIT_CREATED,LIMIT_UPDATED,LIMIT_DELETED,LIMIT_ACTIVATED,LIMIT_DEACTIVATED,LIMIT_DRAFTED,RESERVATION_RESERVED,RESERVATION_CONFIRMED,RESERVATION_RELEASED,RESERVATION_EXPIRED,RESERVATION_SKIPPED
	EventType AuditEventType `json:"eventType" swaggertype:"string" enums:"TRANSACTION_VALIDATED,RULE_CREATED,RULE_UPDATED,RULE_ACTIVATED,RULE_DEACTIVATED,RULE_DRAFTED,RULE_DELETED,LIMIT_CREATED,LIMIT_UPDATED,LIMIT_DELETED,LIMIT_ACTIVATED,LIMIT_DEACTIVATED,LIMIT_DRAFTED,RESERVATION_RESERVED,RESERVATION_CONFIRMED,RESERVATION_RELEASED,RESERVATION_EXPIRED,RESERVATION_SKIPPED" example:"TRANSACTION_VALIDATED"`

	// Timestamp when the event occurred
	// format: date-time
	CreatedAt time.Time `json:"createdAt" format:"date-time" example:"2021-01-01T00:00:00Z"`

	// Action performed
	// example: VALIDATE
	// enums: VALIDATE,CREATE,UPDATE,DELETE,ACTIVATE,DEACTIVATE,DRAFT,RESERVE,CONFIRM,RELEASE,EXPIRE,SKIP
	Action AuditAction `json:"action" swaggertype:"string" enums:"VALIDATE,CREATE,UPDATE,DELETE,ACTIVATE,DEACTIVATE,DRAFT,RESERVE,CONFIRM,RELEASE,EXPIRE,SKIP" example:"VALIDATE"`

	// Outcome: ALLOW/DENY/REVIEW for validations; SUCCESS/FAILED for CRUD operations
	// example: ALLOW
	// enums: ALLOW,DENY,REVIEW,SUCCESS,FAILED
	Result AuditResult `json:"result" swaggertype:"string" enums:"ALLOW,DENY,REVIEW,SUCCESS,FAILED" example:"ALLOW"`

	// ID of the resource affected by this event
	// example: 00000000-0000-0000-0000-000000000000
	ResourceID string `json:"resourceId" example:"00000000-0000-0000-0000-000000000000"`

	// Type of resource affected
	// example: transaction
	// enums: transaction,rule,limit,reservation
	ResourceType ResourceType `json:"resourceType" swaggertype:"string" enums:"transaction,rule,limit,reservation" example:"transaction"`

	// Actor who performed the action
	Actor Actor `json:"actor"`

	// Context (JSONB): for validations: { request: {...}, response: {reason, processingTimeMs, matchedRuleIds, ...} }; for CRUD: { before: {...}, after: {...}, reason: "..." }
	// Note: accountId, segmentId, portfolioId are in context.request.account (not first-level fields)
	Context map[string]any `json:"context,omitempty"`

	// Additional metadata such as ticketId or correlationId
	Metadata map[string]any `json:"metadata,omitempty"`
} //	@name	AuditEvent

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
