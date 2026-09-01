// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import "fmt"

// validTransitions defines allowed status transitions:
// From DRAFT: can go to ACTIVE or DELETED
// From ACTIVE: can go to INACTIVE only (cannot be deleted directly)
// From INACTIVE: can go to DRAFT, ACTIVE, or DELETED
// From DELETED: terminal state, no further transitions
var validTransitions = map[RuleStatus][]RuleStatus{
	RuleStatusDraft:    {RuleStatusActive, RuleStatusDeleted},
	RuleStatusActive:   {RuleStatusInactive},
	RuleStatusInactive: {RuleStatusDraft, RuleStatusActive, RuleStatusDeleted},
	RuleStatusDeleted:  {},
}

// CanTransitionTo checks if transition from current status to target is valid
func (s RuleStatus) CanTransitionTo(target RuleStatus) bool {
	allowed, exists := validTransitions[s]
	if !exists {
		return false
	}

	for _, status := range allowed {
		if status == target {
			return true
		}
	}

	return false
}

// InvalidTransitionError represents an invalid state transition attempt
type InvalidTransitionError struct {
	From RuleStatus
	To   RuleStatus
}

func (e *InvalidTransitionError) Error() string {
	return fmt.Sprintf("invalid status transition from %s to %s", e.From, e.To)
}

// NewInvalidTransitionError creates a new InvalidTransitionError
func NewInvalidTransitionError(from, to RuleStatus) error {
	return &InvalidTransitionError{From: from, To: to}
}
