// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
)

func TestNewRule(t *testing.T) {
	t.Parallel()

	validScopes := []Scope{
		{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1))},
	}
	validDescription := "Test description"

	t.Run("Success - creates rule with all required fields", func(t *testing.T) {
		fixedTime := testutil.FixedTime()
		rule, err := NewRule(
			"Test Rule",
			"amount > 1000",
			DecisionDeny,
			validScopes,
			&validDescription,
			fixedTime,
		)

		require.NoError(t, err)
		require.NotNil(t, rule)
		assert.NotEqual(t, uuid.Nil, rule.ID, "ID should be generated")
		assert.Equal(t, "Test Rule", rule.Name, "Name should be normalized")
		assert.Equal(t, "amount > 1000", rule.Expression)
		assert.Equal(t, DecisionDeny, rule.Action)
		assert.Equal(t, validScopes, rule.Scopes)
		require.NotNil(t, rule.Description)
		assert.Equal(t, validDescription, *rule.Description)
		assert.Equal(t, RuleStatusDraft, rule.Status, "New rules start in DRAFT status")
		assert.Equal(t, fixedTime, rule.CreatedAt)
		assert.Equal(t, fixedTime, rule.UpdatedAt)
		assert.Nil(t, rule.ActivatedAt)
		assert.Nil(t, rule.DeactivatedAt)
		assert.Nil(t, rule.DeletedAt)
	})

	t.Run("Success - creates rule with nil description", func(t *testing.T) {
		rule, err := NewRule(
			"Test Rule",
			"amount > 1000",
			DecisionAllow,
			validScopes,
			nil,
			testutil.FixedTime(),
		)

		require.NoError(t, err)
		require.NotNil(t, rule)
		assert.Nil(t, rule.Description, "Description should be nil when not provided")
	})

	t.Run("Success - creates rule with empty scopes", func(t *testing.T) {
		rule, err := NewRule(
			"Global Rule",
			"amount > 5000",
			DecisionReview,
			[]Scope{},
			nil,
			testutil.FixedTime(),
		)

		require.NoError(t, err)
		require.NotNil(t, rule)
		assert.Empty(t, rule.Scopes, "Scopes should be empty array (not nil)")
		assert.NotNil(t, rule.Scopes, "Scopes should be initialized as empty array")
	})

	t.Run("Success - creates rule with nil scopes", func(t *testing.T) {
		rule, err := NewRule(
			"Global Rule",
			"amount > 5000",
			DecisionReview,
			nil,
			nil,
			testutil.FixedTime(),
		)

		require.NoError(t, err)
		require.NotNil(t, rule)
		assert.Empty(t, rule.Scopes, "Nil scopes should become empty array")
		assert.NotNil(t, rule.Scopes, "Scopes should be initialized as empty array")
	})

	t.Run("Error - scope with all nil fields", func(t *testing.T) {
		emptyScope := Scope{} // All fields nil

		rule, err := NewRule(
			"Test Rule",
			"amount > 1000",
			DecisionAllow,
			[]Scope{emptyScope},
			nil,
			testutil.FixedTime(),
		)

		require.Error(t, err)
		assert.Nil(t, rule)
		assert.ErrorIs(t, err, constant.ErrRuleInvalidScope)
	})

	t.Run("Success - trims whitespace from name", func(t *testing.T) {
		rule, err := NewRule(
			"  Test Rule  ",
			"amount > 1000",
			DecisionAllow,
			validScopes,
			nil,
			testutil.FixedTime(),
		)

		require.NoError(t, err)
		assert.Equal(t, "Test Rule", rule.Name, "Leading/trailing whitespace should be trimmed")
	})

	t.Run("Success - trims whitespace from expression", func(t *testing.T) {
		rule, err := NewRule(
			"Test Rule",
			"  amount > 1000  ",
			DecisionAllow,
			validScopes,
			nil,
			testutil.FixedTime(),
		)

		require.NoError(t, err)
		assert.Equal(t, "amount > 1000", rule.Expression, "Leading/trailing whitespace should be trimmed")
	})

	t.Run("Success - trims whitespace from description", func(t *testing.T) {
		desc := "  Test description  "
		rule, err := NewRule(
			"Test Rule",
			"amount > 1000",
			DecisionAllow,
			validScopes,
			&desc,
			testutil.FixedTime(),
		)

		require.NoError(t, err)
		require.NotNil(t, rule.Description)
		assert.Equal(t, "Test description", *rule.Description, "Description whitespace should be trimmed")
	})

	t.Run("Success - whitespace-only description becomes empty string", func(t *testing.T) {
		desc := "   "
		rule, err := NewRule(
			"Test Rule",
			"amount > 1000",
			DecisionAllow,
			validScopes,
			&desc,
			testutil.FixedTime(),
		)

		require.NoError(t, err)
		require.NotNil(t, rule)
		require.NotNil(t, rule.Description, "Whitespace-only description becomes pointer to empty string, not nil")
		assert.Equal(t, "", *rule.Description, "Whitespace should be trimmed to empty string")
		assert.Len(t, *rule.Description, 0, "Description length should be zero after trim")
	})

	t.Run("Error - empty name after trim", func(t *testing.T) {
		rule, err := NewRule(
			"   ",
			"amount > 1000",
			DecisionAllow,
			validScopes,
			nil,
			testutil.FixedTime(),
		)

		require.Error(t, err)
		assert.Nil(t, rule)
		assert.ErrorIs(t, err, constant.ErrRuleNameRequired)
	})

	t.Run("Error - name exceeds max length", func(t *testing.T) {
		longName := strings.Repeat("a", MaxRuleNameLength+1)
		rule, err := NewRule(
			longName,
			"amount > 1000",
			DecisionAllow,
			validScopes,
			nil,
			testutil.FixedTime(),
		)

		require.Error(t, err)
		assert.Nil(t, rule)
		assert.ErrorIs(t, err, constant.ErrRuleNameTooLong)
	})

	t.Run("Success - name at max length boundary", func(t *testing.T) {
		nameAtMax := strings.Repeat("a", MaxRuleNameLength)
		rule, err := NewRule(
			nameAtMax,
			"amount > 1000",
			DecisionAllow,
			validScopes,
			nil,
			testutil.FixedTime(),
		)

		require.NoError(t, err)
		require.NotNil(t, rule)
		assert.Len(t, rule.Name, MaxRuleNameLength)
	})

	t.Run("Error - empty expression after trim", func(t *testing.T) {
		rule, err := NewRule(
			"Test Rule",
			"   ",
			DecisionAllow,
			validScopes,
			nil,
			testutil.FixedTime(),
		)

		require.Error(t, err)
		assert.Nil(t, rule)
		assert.ErrorIs(t, err, constant.ErrRuleExpressionRequired)
	})

	t.Run("Error - expression exceeds max length", func(t *testing.T) {
		longExpression := strings.Repeat("a", MaxRuleExpressionLength+1)
		rule, err := NewRule(
			"Test Rule",
			longExpression,
			DecisionAllow,
			validScopes,
			nil,
			testutil.FixedTime(),
		)

		require.Error(t, err)
		assert.Nil(t, rule)
		assert.ErrorIs(t, err, constant.ErrRuleExpressionTooLong)
	})

	t.Run("Success - expression at max length boundary", func(t *testing.T) {
		exprAtMax := strings.Repeat("a", MaxRuleExpressionLength)
		rule, err := NewRule(
			"Test Rule",
			exprAtMax,
			DecisionAllow,
			validScopes,
			nil,
			testutil.FixedTime(),
		)

		require.NoError(t, err)
		require.NotNil(t, rule)
		assert.Len(t, rule.Expression, MaxRuleExpressionLength)
	})

	t.Run("Error - invalid action", func(t *testing.T) {
		rule, err := NewRule(
			"Test Rule",
			"amount > 1000",
			Decision("INVALID"),
			validScopes,
			nil,
			testutil.FixedTime(),
		)

		require.Error(t, err)
		assert.Nil(t, rule)
		assert.ErrorIs(t, err, constant.ErrRuleInvalidAction)
	})

	t.Run("Error - description exceeds max length", func(t *testing.T) {
		longDesc := strings.Repeat("a", MaxDescriptionLength+1)
		rule, err := NewRule(
			"Test Rule",
			"amount > 1000",
			DecisionAllow,
			validScopes,
			&longDesc,
			testutil.FixedTime(),
		)

		require.Error(t, err)
		assert.Nil(t, rule)
		assert.ErrorIs(t, err, constant.ErrRuleDescriptionTooLong)
	})

	t.Run("Success - description at max length boundary", func(t *testing.T) {
		descAtMax := strings.Repeat("a", MaxDescriptionLength)
		rule, err := NewRule(
			"Test Rule",
			"amount > 1000",
			DecisionAllow,
			validScopes,
			&descAtMax,
			testutil.FixedTime(),
		)

		require.NoError(t, err)
		require.NotNil(t, rule)
		assert.Len(t, *rule.Description, MaxDescriptionLength)
	})

	t.Run("Defensive copy - external scope mutation doesn't affect rule", func(t *testing.T) {
		originalAccountID := testutil.MustDeterministicUUID(1)
		externalScopes := []Scope{
			{AccountID: testutil.UUIDPtr(originalAccountID)},
		}

		rule, err := NewRule(
			"Test Rule",
			"amount > 1000",
			DecisionAllow,
			externalScopes,
			nil,
			testutil.FixedTime(),
		)

		require.NoError(t, err)

		// Mutate the UUID value through the external pointer (tests deep copy semantics)
		newAccountID := testutil.MustDeterministicUUID(2)
		*externalScopes[0].AccountID = newAccountID

		// Verify rule's scopes are unaffected (should still have original value)
		assert.Equal(t, originalAccountID, *rule.Scopes[0].AccountID, "Rule scopes should not be affected by external mutation")
		assert.NotEqual(t, newAccountID, *rule.Scopes[0].AccountID)
	})

	t.Run("All decisions are valid", func(t *testing.T) {
		decisions := []Decision{DecisionAllow, DecisionDeny, DecisionReview}

		for _, decision := range decisions {
			t.Run(string(decision), func(t *testing.T) {
				rule, err := NewRule(
					"Test Rule",
					"amount > 1000",
					decision,
					validScopes,
					nil,
					testutil.FixedTime(),
				)

				require.NoError(t, err)
				require.NotNil(t, rule)
				assert.Equal(t, decision, rule.Action)
			})
		}
	})
}
