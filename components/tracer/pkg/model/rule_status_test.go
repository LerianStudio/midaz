// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
)

func TestRuleStatus_CanTransitionTo_Activate(t *testing.T) {
	// DRAFT → ACTIVE (allowed)
	assert.True(t, RuleStatusDraft.CanTransitionTo(RuleStatusActive))
}

func TestRuleStatus_CanTransitionTo_Deactivate(t *testing.T) {
	// ACTIVE → INACTIVE (allowed)
	assert.True(t, RuleStatusActive.CanTransitionTo(RuleStatusInactive))
}

func TestRuleStatus_CanTransitionTo_DeleteFromDraft(t *testing.T) {
	// DRAFT → DELETED (allowed - skip activation for unwanted drafts)
	assert.True(t, RuleStatusDraft.CanTransitionTo(RuleStatusDeleted))
}

func TestRuleStatus_CanTransitionTo_DraftToInactive_NotAllowed(t *testing.T) {
	// DRAFT → INACTIVE is NOT allowed (must go through ACTIVE first)
	assert.False(t, RuleStatusDraft.CanTransitionTo(RuleStatusInactive))
}

func TestRuleStatus_CanTransitionTo_Recovery(t *testing.T) {
	// INACTIVE → DRAFT (allowed)
	assert.True(t, RuleStatusInactive.CanTransitionTo(RuleStatusDraft))
}

func TestRuleStatus_CanTransitionTo_Reactivate(t *testing.T) {
	// INACTIVE → ACTIVE (allowed)
	assert.True(t, RuleStatusInactive.CanTransitionTo(RuleStatusActive))
}

func TestRuleStatus_CanTransitionTo_Delete(t *testing.T) {
	// INACTIVE → DELETED (allowed)
	assert.True(t, RuleStatusInactive.CanTransitionTo(RuleStatusDeleted))
}

func TestRuleStatus_CanTransitionTo_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from RuleStatus
		to   RuleStatus
	}{
		{"ACTIVE → DRAFT", RuleStatusActive, RuleStatusDraft},
		{"ACTIVE → DELETED", RuleStatusActive, RuleStatusDeleted},
		{"DRAFT → INACTIVE", RuleStatusDraft, RuleStatusInactive},
		{"DELETED → DRAFT", RuleStatusDeleted, RuleStatusDraft},
		{"DELETED → ACTIVE", RuleStatusDeleted, RuleStatusActive},
		{"DELETED → INACTIVE", RuleStatusDeleted, RuleStatusInactive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, tt.from.CanTransitionTo(tt.to))
		})
	}
}

func TestInvalidTransitionError(t *testing.T) {
	from := RuleStatusActive
	to := RuleStatusDraft

	err := NewInvalidTransitionError(from, to)

	require.Error(t, err)
	assert.Equal(t, "invalid status transition from ACTIVE to DRAFT", err.Error())
}

func TestRuleStatus_CanTransitionTo_InvalidSourceStatus(t *testing.T) {
	// Edge case: source status not in validTransitions map should always return false
	invalidStatus := RuleStatus("INVALID_STATUS")

	tests := []struct {
		name   string
		target RuleStatus
	}{
		{"INVALID → DRAFT", RuleStatusDraft},
		{"INVALID → ACTIVE", RuleStatusActive},
		{"INVALID → INACTIVE", RuleStatusInactive},
		{"INVALID → DELETED", RuleStatusDeleted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, invalidStatus.CanTransitionTo(tt.target),
				"Invalid source status should not be able to transition to any target")
		})
	}
}

func TestRuleStatus_CanTransitionTo_InvalidTargetStatus(t *testing.T) {
	// Edge case: valid source status transitioning to invalid target should return false
	invalidTarget := RuleStatus("INVALID_TARGET")

	tests := []struct {
		name   string
		source RuleStatus
	}{
		{"DRAFT → INVALID", RuleStatusDraft},
		{"ACTIVE → INVALID", RuleStatusActive},
		{"INACTIVE → INVALID", RuleStatusInactive},
		{"DELETED → INVALID", RuleStatusDeleted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, tt.source.CanTransitionTo(invalidTarget),
				"Valid source status should not be able to transition to invalid target")
		})
	}
}

func TestRuleStatus_String(t *testing.T) {
	tests := []struct {
		name     string
		status   RuleStatus
		expected string
	}{
		{"DRAFT returns DRAFT string", RuleStatusDraft, "DRAFT"},
		{"ACTIVE returns ACTIVE string", RuleStatusActive, "ACTIVE"},
		{"INACTIVE returns INACTIVE string", RuleStatusInactive, "INACTIVE"},
		{"DELETED returns DELETED string", RuleStatusDeleted, "DELETED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestRule_SetStatus_InvalidStatus(t *testing.T) {
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rule, err := NewRule("Test", "amount > 100", DecisionAllow, nil, nil, fixedTime)
	require.NoError(t, err)

	// Test with invalid status value
	err = rule.SetStatus(RuleStatus("INVALID"), fixedTime)
	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrRuleInvalidStatus, "should return ErrRuleInvalidStatus for invalid status value")
}

func TestRule_SetStatus_InvalidTransition(t *testing.T) {
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rule, err := NewRule("Test", "amount > 100", DecisionAllow, nil, nil, fixedTime)
	require.NoError(t, err)
	require.Equal(t, RuleStatusDraft, rule.Status)

	// Try invalid transition: DRAFT → INACTIVE (not allowed)
	err = rule.SetStatus(RuleStatusInactive, fixedTime)
	require.Error(t, err)

	// Verify it's an InvalidTransitionError with correct from/to
	var transitionErr *InvalidTransitionError
	assert.True(t, errors.As(err, &transitionErr), "should return InvalidTransitionError for disallowed transition")
	if transitionErr != nil {
		assert.Equal(t, RuleStatusDraft, transitionErr.From)
		assert.Equal(t, RuleStatusInactive, transitionErr.To)
	}

	assert.Equal(t, RuleStatusDraft, rule.Status, "status should not change on invalid transition")
}

func TestRule_SetStatus_ClearsDeletedAtWhenNotDeleted(t *testing.T) {
	t.Parallel()

	staleTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	fixedTime := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	// Create rule in INACTIVE state with simulated stale DeletedAt
	// (This could happen if rule was previously DELETED and somehow has stale timestamp)
	rule := &Rule{
		Status:    RuleStatusInactive,
		DeletedAt: &staleTime,
	}

	t.Run("INACTIVE → ACTIVE clears DeletedAt", func(t *testing.T) {
		err := rule.SetStatus(RuleStatusActive, fixedTime)
		require.NoError(t, err)

		assert.Nil(t, rule.DeletedAt, "DeletedAt should be cleared when transitioning to ACTIVE")
		assert.NotNil(t, rule.ActivatedAt, "ActivatedAt should be set")
	})

	// Reset rule to INACTIVE with stale DeletedAt
	rule.Status = RuleStatusInactive
	rule.DeletedAt = &staleTime

	t.Run("INACTIVE → DRAFT clears DeletedAt", func(t *testing.T) {
		err := rule.SetStatus(RuleStatusDraft, fixedTime)
		require.NoError(t, err)

		assert.Nil(t, rule.DeletedAt, "DeletedAt should be cleared when transitioning to DRAFT")
		assert.Nil(t, rule.ActivatedAt, "ActivatedAt should be cleared for DRAFT")
	})

	// Reset rule to INACTIVE
	rule.Status = RuleStatusInactive

	t.Run("INACTIVE → INACTIVE (idempotent) preserves DeletedAt = nil", func(t *testing.T) {
		rule.DeletedAt = nil // Clean state

		err := rule.SetStatus(RuleStatusInactive, fixedTime)
		require.NoError(t, err)

		assert.Nil(t, rule.DeletedAt, "DeletedAt should remain nil for INACTIVE")
	})
}
