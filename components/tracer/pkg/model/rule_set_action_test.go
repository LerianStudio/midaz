// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
)

func TestRule_SetAction(t *testing.T) {
	t.Parallel()

	t.Run("Success - sets ALLOW action", func(t *testing.T) {
		t.Parallel()

		rule := createValidRuleForSetAction(t)
		now := time.Date(2026, 2, 4, 12, 0, 0, 0, time.UTC)

		err := rule.SetAction(DecisionAllow, now)

		require.NoError(t, err)
		assert.Equal(t, DecisionAllow, rule.Action)
		assert.Equal(t, now, rule.UpdatedAt)
	})

	t.Run("Success - sets DENY action", func(t *testing.T) {
		t.Parallel()

		rule := createValidRuleForSetAction(t)
		// Change initial action to something different to test actual change
		rule.Action = DecisionAllow
		now := time.Date(2026, 2, 4, 12, 0, 0, 0, time.UTC)

		err := rule.SetAction(DecisionDeny, now)

		require.NoError(t, err)
		assert.Equal(t, DecisionDeny, rule.Action)
		assert.Equal(t, now, rule.UpdatedAt)
	})

	t.Run("Success - sets REVIEW action", func(t *testing.T) {
		t.Parallel()

		rule := createValidRuleForSetAction(t)
		now := time.Date(2026, 2, 4, 12, 0, 0, 0, time.UTC)

		err := rule.SetAction(DecisionReview, now)

		require.NoError(t, err)
		assert.Equal(t, DecisionReview, rule.Action)
		assert.Equal(t, now, rule.UpdatedAt)
	})

	t.Run("Success - updates UpdatedAt with provided timestamp", func(t *testing.T) {
		t.Parallel()

		rule := createValidRuleForSetAction(t)
		originalUpdatedAt := rule.UpdatedAt
		customTime := time.Date(2030, 12, 25, 23, 59, 59, 0, time.UTC)

		err := rule.SetAction(DecisionAllow, customTime)

		require.NoError(t, err)
		assert.NotEqual(t, originalUpdatedAt, rule.UpdatedAt)
		assert.Equal(t, customTime, rule.UpdatedAt)
	})

	t.Run("Error - rejects invalid decision (empty string)", func(t *testing.T) {
		t.Parallel()

		rule := createValidRuleForSetAction(t)
		originalAction := rule.Action
		originalUpdatedAt := rule.UpdatedAt
		now := time.Date(2026, 2, 4, 12, 0, 0, 0, time.UTC)

		err := rule.SetAction(Decision(""), now)

		require.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrRuleInvalidAction)
		// Verify no mutation occurred
		assert.Equal(t, originalAction, rule.Action, "Action should not be mutated on error")
		assert.Equal(t, originalUpdatedAt, rule.UpdatedAt, "UpdatedAt should not be mutated on error")
	})

	t.Run("Error - rejects invalid decision (lowercase)", func(t *testing.T) {
		t.Parallel()

		rule := createValidRuleForSetAction(t)
		originalAction := rule.Action
		originalUpdatedAt := rule.UpdatedAt
		now := time.Date(2026, 2, 4, 12, 0, 0, 0, time.UTC)

		err := rule.SetAction(Decision("allow"), now)

		require.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrRuleInvalidAction)
		assert.Equal(t, originalAction, rule.Action, "Action should not be mutated on error")
		assert.Equal(t, originalUpdatedAt, rule.UpdatedAt, "UpdatedAt should not be mutated on error")
	})

	t.Run("Error - rejects invalid decision (random string)", func(t *testing.T) {
		t.Parallel()

		rule := createValidRuleForSetAction(t)
		originalAction := rule.Action
		originalUpdatedAt := rule.UpdatedAt
		now := time.Date(2026, 2, 4, 12, 0, 0, 0, time.UTC)

		err := rule.SetAction(Decision("INVALID"), now)

		require.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrRuleInvalidAction)
		assert.Equal(t, originalAction, rule.Action, "Action should not be mutated on error")
		assert.Equal(t, originalUpdatedAt, rule.UpdatedAt, "UpdatedAt should not be mutated on error")
	})

	t.Run("Idempotency - setting same action does not update timestamp", func(t *testing.T) {
		t.Parallel()

		rule := createValidRuleForSetAction(t)
		rule.Action = DecisionDeny
		originalUpdatedAt := rule.UpdatedAt
		newTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)

		err := rule.SetAction(DecisionDeny, newTime)

		require.NoError(t, err)
		assert.Equal(t, DecisionDeny, rule.Action)
		assert.Equal(t, originalUpdatedAt, rule.UpdatedAt, "UpdatedAt should NOT be updated when setting same action (idempotency)")
	})
}

// createValidRuleForSetAction creates a valid Rule for SetAction tests.
func createValidRuleForSetAction(t *testing.T) *Rule {
	t.Helper()

	rule, err := NewRule(
		"Test Rule",
		"amount > 1000",
		DecisionDeny,
		[]Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1))}},
		nil,
		testutil.FixedTime(),
	)
	require.NoError(t, err, "createValidRuleForSetAction: NewRule failed")

	return rule
}
