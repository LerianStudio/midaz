// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
)

// RuleStatus represents the lifecycle status of a rule
type RuleStatus string

const (
	RuleStatusDraft    RuleStatus = "DRAFT"
	RuleStatusActive   RuleStatus = "ACTIVE"
	RuleStatusInactive RuleStatus = "INACTIVE"
	RuleStatusDeleted  RuleStatus = "DELETED"
)

// IsValid checks if the RuleStatus is a valid enum value.
func (s RuleStatus) IsValid() bool {
	switch s {
	case RuleStatusDraft, RuleStatusActive, RuleStatusInactive, RuleStatusDeleted:
		return true
	default:
		return false
	}
}

// String returns the string representation of the rule status
func (s RuleStatus) String() string {
	return string(s)
}

// Rule represents a validation rule with CEL expression.
// Note: priority field removed from MVP (TRD v1.2.4) - all rules evaluated, DENY takes precedence.
type Rule struct {
	ID            uuid.UUID  `json:"ruleId" swaggertype:"string" format:"uuid"`
	Name          string     `json:"name"`
	Description   *string    `json:"description,omitempty"`
	Expression    string     `json:"expression"`
	Action        Decision   `json:"action"`
	Scopes        []Scope    `json:"scopes"`
	Status        RuleStatus `json:"status"`
	CreatedAt     time.Time  `json:"createdAt" format:"date-time"`
	UpdatedAt     time.Time  `json:"updatedAt" format:"date-time"`
	ActivatedAt   *time.Time `json:"activatedAt,omitempty" format:"date-time"`
	DeactivatedAt *time.Time `json:"deactivatedAt,omitempty" format:"date-time"`
	DeletedAt     *time.Time `json:"deletedAt,omitempty" format:"date-time"`

	// CompiledProgram holds the pre-compiled CEL expression program.
	// Transient field — not persisted, not serialized. Used to pass
	// compiled programs from cache to evaluator, avoiding recompilation
	// on the hot evaluation path.
	CompiledProgram any `json:"-"`
}

// MaxRuleNameLength defines the maximum length for rule names (aligned with VARCHAR(255) in database)
const MaxRuleNameLength = 255

// MaxRuleExpressionLength defines the maximum length for CEL expressions
const MaxRuleExpressionLength = 5000

// NewRule creates a new Rule entity with validation.
// Name is trimmed of leading/trailing whitespace before validation and storage.
// Scopes ordering is preserved: the returned Rule.Scopes maintains the same order as the input.
// The rule is created in DRAFT status with CreatedAt and UpdatedAt set to the provided createdAt time.
func NewRule(name, expression string, action Decision, scopes []Scope, description *string, createdAt time.Time) (*Rule, error) {
	// Normalize textual inputs
	normalizedName := strings.TrimSpace(name)
	normalizedExpression := strings.TrimSpace(expression)

	var normalizedDescription *string

	if description != nil {
		trimmed := strings.TrimSpace(*description)
		normalizedDescription = &trimmed
	}

	// Validate name
	if normalizedName == "" {
		return nil, constant.ErrRuleNameRequired
	}

	if len(normalizedName) > MaxRuleNameLength {
		return nil, constant.ErrRuleNameTooLong
	}

	// Validate expression
	if normalizedExpression == "" {
		return nil, constant.ErrRuleExpressionRequired
	}

	if len(normalizedExpression) > MaxRuleExpressionLength {
		return nil, constant.ErrRuleExpressionTooLong
	}

	// Validate action
	if !action.IsValid() {
		return nil, constant.ErrRuleInvalidAction
	}

	// Validate description length if provided
	if normalizedDescription != nil && len(*normalizedDescription) > MaxDescriptionLength {
		return nil, constant.ErrRuleDescriptionTooLong
	}

	// Defensive deep copy of scopes with validation
	// Always use empty slice instead of nil to ensure proper JSON serialization
	// Deep copy UUID pointers to prevent external mutations from affecting rule
	// SubType is normalized to trimmed lowercase canonical form so DB state is
	// symmetric with runtime case-insensitive matching.
	scopesCopy := make([]Scope, 0, len(scopes))
	for _, scope := range scopes {
		if scope.IsEmpty() {
			return nil, constant.ErrRuleInvalidScope
		}

		scopesCopy = append(scopesCopy, cloneAndNormalizeScope(scope))
	}

	return &Rule{
		ID:          uuid.New(),
		Name:        normalizedName,
		Description: normalizedDescription,
		Expression:  normalizedExpression,
		Action:      action,
		Scopes:      scopesCopy,
		Status:      RuleStatusDraft,
		CreatedAt:   createdAt.UTC(),
		UpdatedAt:   createdAt.UTC(),
	}, nil
}

// Update modifies rule fields with validation.
// All parameters are optional (use nil to keep current value).
// Validates ALL inputs before mutating ANY (atomicity guarantee).
// Updates UpdatedAt timestamp on successful mutation.
// now parameter allows deterministic timestamps in tests (follows SetAction pattern).
func (r *Rule) Update(
	name *string,
	expression *string,
	description *string,
	scopes *[]Scope,
	now time.Time,
) error {
	updated := false

	// Normalize values once for validation and later mutation
	var normalizedName, normalizedExpression, normalizedDescription string

	// Validate ALL before mutating ANY
	if name != nil {
		normalizedName = strings.TrimSpace(*name)
		if normalizedName == "" {
			return constant.ErrRuleNameRequired
		}

		if len(normalizedName) > MaxRuleNameLength {
			return constant.ErrRuleNameTooLong
		}
	}

	if expression != nil {
		normalizedExpression = strings.TrimSpace(*expression)
		if normalizedExpression == "" {
			return constant.ErrRuleExpressionRequired
		}

		if len(normalizedExpression) > MaxRuleExpressionLength {
			return constant.ErrRuleExpressionTooLong
		}
	}

	if description != nil {
		normalizedDescription = strings.TrimSpace(*description)
		if len(normalizedDescription) > MaxDescriptionLength {
			return constant.ErrRuleDescriptionTooLong
		}
	}

	// Validate scopes - each scope must have at least one field set
	if scopes != nil {
		for _, scope := range *scopes {
			if scope.IsEmpty() {
				return constant.ErrRuleInvalidScope
			}
		}
	}

	// All validations passed - now mutate (reuse normalized values)
	if name != nil {
		r.Name = normalizedName
		updated = true
	}

	if expression != nil {
		r.Expression = normalizedExpression
		updated = true
	}

	if description != nil {
		r.Description = &normalizedDescription
		updated = true
	}

	if scopes != nil {
		// Defensive deep copy of scopes to prevent external mutation
		// Deep copy UUID pointers to prevent external mutations from affecting rule
		// Note: IsEmpty() already validated in the validation phase above
		// SubType is normalized to trimmed lowercase canonical form so DB state is
		// symmetric with runtime case-insensitive matching.
		scopesCopy := make([]Scope, 0, len(*scopes))
		for _, scope := range *scopes {
			scopesCopy = append(scopesCopy, cloneAndNormalizeScope(scope))
		}

		r.Scopes = scopesCopy
		updated = true
	}

	if updated {
		r.UpdatedAt = now.UTC()
	}

	return nil
}

// SetStatus changes the rule status with transition validation.
// Idempotent: same-status transitions are no-ops (return nil without updating timestamp).
// DELETED is a terminal state and cannot be transitioned from.
// Maintains timestamp invariants based on status:
// - RuleStatusActive → sets ActivatedAt, clears DeactivatedAt
// - RuleStatusInactive → sets DeactivatedAt
// - RuleStatusDeleted → sets DeletedAt
// - RuleStatusDraft → clears ActivatedAt and DeactivatedAt
// now parameter allows deterministic timestamps in tests (follows SetAction/Update pattern).
func (r *Rule) SetStatus(status RuleStatus, now time.Time) error {
	if !status.IsValid() {
		return constant.ErrRuleInvalidStatus
	}

	// Idempotency: same status is a no-op
	if r.Status == status {
		return nil
	}

	// Check if transition is allowed
	if !r.Status.CanTransitionTo(status) {
		return NewInvalidTransitionError(r.Status, status)
	}

	// Update status and maintain timestamp invariants
	utcNow := now.UTC()
	r.Status = status
	r.UpdatedAt = utcNow

	switch status {
	case RuleStatusActive:
		r.ActivatedAt = &utcNow
		r.DeactivatedAt = nil
		r.DeletedAt = nil
	case RuleStatusInactive:
		r.DeactivatedAt = &utcNow
		r.DeletedAt = nil
	case RuleStatusDeleted:
		r.DeletedAt = &utcNow
	case RuleStatusDraft:
		r.ActivatedAt = nil
		r.DeactivatedAt = nil
		r.DeletedAt = nil
	}

	return nil
}

// SetAction updates the rule's action/decision with validation.
// Idempotent: same-action assignments are no-ops (return nil without updating timestamp).
// Returns error if action is invalid.
// Updates UpdatedAt timestamp on successful mutation.
func (r *Rule) SetAction(action Decision, now time.Time) error {
	if !action.IsValid() {
		return constant.ErrRuleInvalidAction
	}

	// Idempotency: same action is a no-op
	if r.Action == action {
		return nil
	}

	r.Action = action
	r.UpdatedAt = now.UTC()

	return nil
}

// ListRulesFilter represents the filter criteria for listing rules.
// Uses cursor-based pagination for consistent results during navigation.
type ListRulesFilter struct {
	Name        *string // Filter by name (case-insensitive partial match / contains)
	Status      *RuleStatus
	Action      *Decision
	ScopeFilter *Scope // Optional scope filter for JSONB scope matching
	Limit       int
	Cursor      string // Base64 encoded cursor for pagination
	SortBy      string
	SortOrder   string
}

// ListRulesResult represents the result of listing rules.
// Uses cursor-based pagination per PROJECT_RULES.md.
type ListRulesResult struct {
	Rules      []Rule
	NextCursor string // Base64 encoded cursor for next page (empty if no more results)
	HasMore    bool   // Indicates if there are more results
}
