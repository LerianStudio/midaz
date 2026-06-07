// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// newTestRule creates a valid Rule for testing purposes.
// Returns a Rule with valid fields and a sample scope.
// Fails the test immediately if NewRule returns an error.
func newTestRule(t *testing.T) *Rule {
	t.Helper()

	rule, err := NewRule(
		"Test Rule",
		"amount > 1000",
		DecisionDeny,
		[]Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1))}},
		nil,
		testutil.FixedTime(),
	)
	require.NoError(t, err, "newTestRule: NewRule failed")

	return rule
}

func TestRule_Update_ScopeValidation(t *testing.T) {
	t.Parallel()

	fixedTime := testutil.FixedTime()

	t.Run("Error - rejects scope with all nil fields (empty scope)", func(t *testing.T) {
		rule := newTestRule(t)
		emptyScope := Scope{} // All fields nil

		scopesWithEmpty := &[]Scope{emptyScope}

		err := rule.Update(nil, nil, nil, scopesWithEmpty, fixedTime)

		require.Error(t, err, "Update should reject empty scope")
		assert.ErrorIs(t, err, constant.ErrRuleInvalidScope)
	})

	t.Run("Error - rejects multiple scopes where one is empty", func(t *testing.T) {
		rule := newTestRule(t)
		validScope := Scope{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(2))}
		emptyScope := Scope{} // All fields nil

		scopesWithOneEmpty := &[]Scope{validScope, emptyScope}

		err := rule.Update(nil, nil, nil, scopesWithOneEmpty, fixedTime)

		require.Error(t, err, "Update should reject when any scope is empty")
		assert.ErrorIs(t, err, constant.ErrRuleInvalidScope)
	})

	t.Run("Error - empty scope in first position", func(t *testing.T) {
		rule := newTestRule(t)
		emptyScope := Scope{}
		validScope := Scope{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(2))}

		scopesWithFirstEmpty := &[]Scope{emptyScope, validScope}

		err := rule.Update(nil, nil, nil, scopesWithFirstEmpty, fixedTime)

		require.Error(t, err, "Update should reject when first scope is empty")
		assert.ErrorIs(t, err, constant.ErrRuleInvalidScope)
	})

	t.Run("Success - accepts valid scopes", func(t *testing.T) {
		rule := newTestRule(t)

		validScopes := &[]Scope{
			{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(3))},
			{PortfolioID: testutil.UUIDPtr(testutil.MustDeterministicUUID(4))},
		}

		err := rule.Update(nil, nil, nil, validScopes, fixedTime)

		require.NoError(t, err)
		assert.Len(t, rule.Scopes, 2)
	})

	t.Run("Success - accepts empty slice (removes all scopes)", func(t *testing.T) {
		rule := newTestRule(t)

		emptySlice := &[]Scope{}

		err := rule.Update(nil, nil, nil, emptySlice, fixedTime)

		require.NoError(t, err)
		assert.Empty(t, rule.Scopes)
	})

	t.Run("Success - nil scopes parameter keeps existing scopes", func(t *testing.T) {
		rule := newTestRule(t)
		originalScopes := make([]Scope, len(rule.Scopes))
		copy(originalScopes, rule.Scopes)

		err := rule.Update(nil, nil, nil, nil, fixedTime)

		require.NoError(t, err)
		assert.Equal(t, originalScopes, rule.Scopes, "Scopes should remain unchanged when nil is passed")
	})

	t.Run("Atomicity - does not mutate scopes on validation failure", func(t *testing.T) {
		rule := newTestRule(t)
		originalScopes := make([]Scope, len(rule.Scopes))
		copy(originalScopes, rule.Scopes)

		emptyScope := Scope{}
		invalidScopes := &[]Scope{emptyScope}

		err := rule.Update(nil, nil, nil, invalidScopes, fixedTime)

		require.Error(t, err)
		assert.Equal(t, originalScopes, rule.Scopes, "Scopes should not be mutated on validation failure")
	})

	t.Run("Deep copy - external scope mutation doesn't affect rule", func(t *testing.T) {
		rule := newTestRule(t)

		// Create scope with UUID pointer
		originalAccountID := testutil.MustDeterministicUUID(5)
		externalScopes := []Scope{
			{AccountID: testutil.UUIDPtr(originalAccountID)},
		}

		// Update rule with scopes
		err := rule.Update(nil, nil, nil, &externalScopes, fixedTime)
		require.NoError(t, err)

		// Mutate the UUID value through the external pointer (tests deep copy semantics)
		newAccountID := testutil.MustDeterministicUUID(6)
		*externalScopes[0].AccountID = newAccountID

		// Verify rule's scopes are unaffected (should still have original value)
		assert.Equal(t, originalAccountID, *rule.Scopes[0].AccountID, "Rule scopes should not be affected by external mutation")
		assert.NotEqual(t, newAccountID, *rule.Scopes[0].AccountID, "Rule should have deep-copied UUID pointer")
	})
}
