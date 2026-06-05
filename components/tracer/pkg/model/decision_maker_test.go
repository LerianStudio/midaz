// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
)

func TestDecisionMaker_MakeDecision(t *testing.T) {
	tests := []struct {
		name             string
		denyRuleIDs      []uuid.UUID
		allowRuleIDs     []uuid.UUID
		reviewRuleIDs    []uuid.UUID
		evaluatedRuleIDs []uuid.UUID
		defaultDecision  Decision
		expectedDecision Decision
		expectedReason   string
		wantErr          bool
		expectedErr      error
	}{
		{
			name:             "DENY takes precedence over all (DENY + ALLOW + REVIEW → DENY)",
			denyRuleIDs:      testutil.MustDeterministicUUIDs(1, 100),
			allowRuleIDs:     testutil.MustDeterministicUUIDs(1, 200),
			reviewRuleIDs:    testutil.MustDeterministicUUIDs(1, 300),
			evaluatedRuleIDs: []uuid.UUID{testutil.MustDeterministicUUID(100), testutil.MustDeterministicUUID(200), testutil.MustDeterministicUUID(300)},
			defaultDecision:  DecisionAllow,
			expectedDecision: DecisionDeny,
			expectedReason:   "Rule matched with DENY action",
		},
		{
			name:             "REVIEW takes precedence over ALLOW (no DENY, REVIEW + ALLOW → REVIEW)",
			denyRuleIDs:      []uuid.UUID{},
			allowRuleIDs:     testutil.MustDeterministicUUIDs(2, 200),
			reviewRuleIDs:    testutil.MustDeterministicUUIDs(1, 300),
			evaluatedRuleIDs: []uuid.UUID{testutil.MustDeterministicUUID(200), testutil.MustDeterministicUUID(201), testutil.MustDeterministicUUID(300)},
			defaultDecision:  DecisionAllow,
			expectedDecision: DecisionReview,
			expectedReason:   "Rule matched with REVIEW action",
		},
		{
			name:             "Only ALLOW returns ALLOW",
			denyRuleIDs:      []uuid.UUID{},
			allowRuleIDs:     testutil.MustDeterministicUUIDs(3, 200),
			reviewRuleIDs:    []uuid.UUID{},
			evaluatedRuleIDs: testutil.MustDeterministicUUIDs(3, 200),
			defaultDecision:  DecisionDeny,
			expectedDecision: DecisionAllow,
			expectedReason:   "Rule matched with ALLOW action",
		},
		{
			name:             "No match uses default ALLOW",
			denyRuleIDs:      []uuid.UUID{},
			allowRuleIDs:     []uuid.UUID{},
			reviewRuleIDs:    []uuid.UUID{},
			evaluatedRuleIDs: testutil.MustDeterministicUUIDs(5, 400),
			defaultDecision:  DecisionAllow,
			expectedDecision: DecisionAllow,
			expectedReason:   "No matching rules found",
		},
		{
			name:             "No match uses default DENY",
			denyRuleIDs:      []uuid.UUID{},
			allowRuleIDs:     []uuid.UUID{},
			reviewRuleIDs:    []uuid.UUID{},
			evaluatedRuleIDs: testutil.MustDeterministicUUIDs(5, 500),
			defaultDecision:  DecisionDeny,
			expectedDecision: DecisionDeny,
			expectedReason:   "No matching rules found",
		},
		{
			name:             "No match uses default REVIEW",
			denyRuleIDs:      []uuid.UUID{},
			allowRuleIDs:     []uuid.UUID{},
			reviewRuleIDs:    []uuid.UUID{},
			evaluatedRuleIDs: testutil.MustDeterministicUUIDs(5, 700),
			defaultDecision:  DecisionReview,
			expectedDecision: DecisionReview,
			expectedReason:   "No matching rules found",
		},
		{
			name:             "Invalid default decision returns error",
			denyRuleIDs:      []uuid.UUID{},
			allowRuleIDs:     []uuid.UUID{},
			reviewRuleIDs:    []uuid.UUID{},
			evaluatedRuleIDs: testutil.MustDeterministicUUIDs(5, 600),
			defaultDecision:  Decision("INVALID"),
			wantErr:          true,
			expectedErr:      constant.ErrInvalidDefaultDecision,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create DecisionMaker
			dm := NewDecisionMaker()

			// Execute
			result, err := dm.MakeDecision(
				tt.denyRuleIDs,
				tt.allowRuleIDs,
				tt.reviewRuleIDs,
				tt.evaluatedRuleIDs,
				tt.defaultDecision,
			)

			// Assert
			if tt.wantErr {
				require.Error(t, err, "expected error for test case: %s", tt.name)
				assert.ErrorIs(t, err, tt.expectedErr, "error should match expected")
				assert.Nil(t, result, "result should be nil when error occurs")

				return
			}

			require.NoError(t, err, "expected no error for test case: %s", tt.name)
			require.NotNil(t, result, "result should not be nil")
			assert.Equal(t, tt.expectedDecision, result.Decision, "decision should match expected")
			assert.Equal(t, tt.expectedReason, result.Reason, "reason should match expected")
			assert.Equal(t, tt.evaluatedRuleIDs, result.EvaluatedRuleIDs, "evaluated rule IDs should be preserved")

			// When all input rule slices are empty, matched rule IDs should also be empty (default decision case)
			if len(tt.denyRuleIDs) == 0 && len(tt.allowRuleIDs) == 0 && len(tt.reviewRuleIDs) == 0 {
				assert.Empty(t, result.MatchedRuleIDs, "matched rule IDs should be empty when default decision is used")

				return
			}

			// Verify matched rule IDs contains ALL matched rules (DENY + REVIEW + ALLOW), not just the winning category.
			// Per API design: matchedRuleIds contains all rules that matched, regardless of action.
			expectedMatched := make([]uuid.UUID, 0, len(tt.denyRuleIDs)+len(tt.reviewRuleIDs)+len(tt.allowRuleIDs))
			expectedMatched = append(expectedMatched, tt.denyRuleIDs...)
			expectedMatched = append(expectedMatched, tt.reviewRuleIDs...)
			expectedMatched = append(expectedMatched, tt.allowRuleIDs...)
			assert.Equal(t, expectedMatched, result.MatchedRuleIDs, "matched rule IDs should contain ALL matched rules")
		})
	}
}

func TestDecisionMaker_MakeDecision_NilSlices(t *testing.T) {
	dm := NewDecisionMaker()

	// Test with nil slices - should handle gracefully
	result, err := dm.MakeDecision(nil, nil, nil, nil, DecisionAllow)

	require.NoError(t, err)
	require.NotNil(t, result, "result should not be nil even with nil inputs")
	assert.Equal(t, DecisionAllow, result.Decision, "should return default decision when all slices are nil")
	assert.Equal(t, "No matching rules found", result.Reason, "reason should indicate no match")
	assert.NotNil(t, result.MatchedRuleIDs, "matched rule IDs should not be nil (empty slice)")
	assert.NotNil(t, result.EvaluatedRuleIDs, "evaluated rule IDs should not be nil (empty slice)")
}

func TestDecisionMaker_MakeDecision_PrecedenceWithMultipleRules(t *testing.T) {
	// Create specific UUIDs for verification using deterministic values
	denyIDs := testutil.MustDeterministicUUIDs(2, 100)
	allowIDs := testutil.MustDeterministicUUIDs(3, 200)
	reviewIDs := testutil.MustDeterministicUUIDs(1, 300)

	// All evaluated IDs
	allEvaluated := make([]uuid.UUID, 0, len(denyIDs)+len(allowIDs)+len(reviewIDs))
	allEvaluated = append(allEvaluated, denyIDs...)
	allEvaluated = append(allEvaluated, allowIDs...)
	allEvaluated = append(allEvaluated, reviewIDs...)

	dm := NewDecisionMaker()

	// Even with multiple ALLOW rules, a single DENY should take precedence
	result, err := dm.MakeDecision(denyIDs, allowIDs, reviewIDs, allEvaluated, DecisionAllow)

	require.NoError(t, err)
	assert.Equal(t, DecisionDeny, result.Decision, "DENY should take precedence over multiple ALLOW and REVIEW rules")

	// MatchedRuleIDs should contain ALL matched rules, not just DENY rules
	// Per API design: matchedRuleIds contains all rules that matched, regardless of action.
	expectedMatched := make([]uuid.UUID, 0, len(denyIDs)+len(reviewIDs)+len(allowIDs))
	expectedMatched = append(expectedMatched, denyIDs...)
	expectedMatched = append(expectedMatched, reviewIDs...)
	expectedMatched = append(expectedMatched, allowIDs...)
	assert.Equal(t, expectedMatched, result.MatchedRuleIDs, "matched rule IDs should contain ALL matched rule IDs")
}
