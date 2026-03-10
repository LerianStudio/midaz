// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestActionConstants_Values(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "ActionDirect is direct",
			constant: ActionDirect,
			expected: "direct",
		},
		{
			name:     "ActionHold is hold",
			constant: ActionHold,
			expected: "hold",
		},
		{
			name:     "ActionCommit is commit",
			constant: ActionCommit,
			expected: "commit",
		},
		{
			name:     "ActionCancel is cancel",
			constant: ActionCancel,
			expected: "cancel",
		},
		{
			name:     "ActionRevert is revert",
			constant: ActionRevert,
			expected: "revert",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.constant)
		})
	}
}

func TestValidActions_Count(t *testing.T) {
	t.Parallel()

	assert.Len(t, ValidActions, 5, "ValidActions must contain exactly 5 elements")
}

func TestValidActions_AllLowercase(t *testing.T) {
	t.Parallel()

	for _, action := range ValidActions {
		t.Run(action, func(t *testing.T) {
			t.Parallel()

			for _, c := range action {
				assert.True(t, c >= 'a' && c <= 'z', "action %q contains non-lowercase character %c", action, c)
			}
		})
	}
}

func TestValidActions_MatchesCheckConstraint(t *testing.T) {
	t.Parallel()

	// These must match the CHECK constraint from migration 000023:
	// LOWER(action) IN ('direct', 'hold', 'commit', 'cancel', 'revert')
	expectedActions := map[string]bool{
		"direct": false,
		"hold":   false,
		"commit": false,
		"cancel": false,
		"revert": false,
	}

	for _, action := range ValidActions {
		_, exists := expectedActions[action]
		assert.True(t, exists, "unexpected action %q in ValidActions", action)
		expectedActions[action] = true
	}

	for action, found := range expectedActions {
		assert.True(t, found, "expected action %q not found in ValidActions", action)
	}
}

func TestValidActions_ContainsAllConstants(t *testing.T) {
	t.Parallel()

	constants := []string{
		ActionDirect,
		ActionHold,
		ActionCommit,
		ActionCancel,
		ActionRevert,
	}

	for _, c := range constants {
		assert.Contains(t, ValidActions, c, "ValidActions should contain constant %q", c)
	}
}
