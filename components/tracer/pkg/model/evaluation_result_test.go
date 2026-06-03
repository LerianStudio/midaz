// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/internal/testutil"
	"tracer/pkg/constant"
)

func TestNewEvaluationResult(t *testing.T) {
	matchedID1 := testutil.MustDeterministicUUID(1)
	matchedID2 := testutil.MustDeterministicUUID(2)
	evaluatedIDs := []uuid.UUID{matchedID1, matchedID2, testutil.MustDeterministicUUID(3)}

	tests := []struct {
		name             string
		decision         Decision
		matchedRuleIDs   []uuid.UUID
		evaluatedRuleIDs []uuid.UUID
		reason           string
		wantErr          error
		validate         func(*testing.T, *EvaluationResult)
	}{
		{
			name:             "multiple matched and evaluated",
			decision:         DecisionDeny,
			matchedRuleIDs:   []uuid.UUID{matchedID1, matchedID2},
			evaluatedRuleIDs: evaluatedIDs,
			reason:           "DENY: 2 rule(s) matched",
			wantErr:          nil,
			validate: func(t *testing.T, r *EvaluationResult) {
				assert.Equal(t, DecisionDeny, r.Decision)
				assert.Len(t, r.MatchedRuleIDs, 2)
				assert.Len(t, r.EvaluatedRuleIDs, 3)
				assert.Equal(t, "DENY: 2 rule(s) matched", r.Reason)
			},
		},
		{
			name:             "single matched rule",
			decision:         DecisionReview,
			matchedRuleIDs:   []uuid.UUID{matchedID1},
			evaluatedRuleIDs: []uuid.UUID{matchedID1},
			reason:           "REVIEW: suspicious transaction",
			wantErr:          nil,
			validate: func(t *testing.T, r *EvaluationResult) {
				assert.Equal(t, DecisionReview, r.Decision)
				assert.Len(t, r.MatchedRuleIDs, 1)
				assert.Equal(t, matchedID1, r.MatchedRuleIDs[0])
			},
		},
		{
			name:             "nil matchedRuleIDs normalized to empty slice",
			decision:         DecisionDeny,
			matchedRuleIDs:   nil,
			evaluatedRuleIDs: evaluatedIDs,
			reason:           "reason",
			wantErr:          nil,
			validate: func(t *testing.T, r *EvaluationResult) {
				assert.NotNil(t, r.MatchedRuleIDs)
				assert.Empty(t, r.MatchedRuleIDs)
				assert.Len(t, r.EvaluatedRuleIDs, 3)
			},
		},
		{
			name:             "nil evaluatedRuleIDs normalized to empty slice",
			decision:         DecisionAllow,
			matchedRuleIDs:   []uuid.UUID{matchedID1},
			evaluatedRuleIDs: nil,
			reason:           "reason",
			wantErr:          nil,
			validate: func(t *testing.T, r *EvaluationResult) {
				assert.NotNil(t, r.EvaluatedRuleIDs)
				assert.Empty(t, r.EvaluatedRuleIDs)
				assert.Len(t, r.MatchedRuleIDs, 1)
			},
		},
		{
			name:             "both nil slices normalized to empty slices",
			decision:         DecisionReview,
			matchedRuleIDs:   nil,
			evaluatedRuleIDs: nil,
			reason:           "reason",
			wantErr:          nil,
			validate: func(t *testing.T, r *EvaluationResult) {
				assert.NotNil(t, r.MatchedRuleIDs)
				assert.NotNil(t, r.EvaluatedRuleIDs)
				assert.Empty(t, r.MatchedRuleIDs)
				assert.Empty(t, r.EvaluatedRuleIDs)
			},
		},
		{
			name:             "invalid decision returns error",
			decision:         Decision("INVALID"),
			matchedRuleIDs:   []uuid.UUID{matchedID1},
			evaluatedRuleIDs: evaluatedIDs,
			reason:           "some reason",
			wantErr:          constant.ErrInvalidDecision,
			validate:         nil,
		},
		{
			name:             "empty decision returns error",
			decision:         Decision(""),
			matchedRuleIDs:   []uuid.UUID{matchedID1},
			evaluatedRuleIDs: evaluatedIDs,
			reason:           "some reason",
			wantErr:          constant.ErrInvalidDecision,
			validate:         nil,
		},
		{
			name:             "empty reason returns error",
			decision:         DecisionAllow,
			matchedRuleIDs:   []uuid.UUID{matchedID1},
			evaluatedRuleIDs: evaluatedIDs,
			reason:           "",
			wantErr:          constant.ErrReasonRequired,
			validate:         nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewEvaluationResult(tt.decision, tt.matchedRuleIDs, tt.evaluatedRuleIDs, tt.reason)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

func TestNewNoMatchResult(t *testing.T) {
	evaluatedIDs := []uuid.UUID{testutil.MustDeterministicUUID(10), testutil.MustDeterministicUUID(11), testutil.MustDeterministicUUID(12)}

	tests := []struct {
		name             string
		defaultDecision  Decision
		evaluatedRuleIDs []uuid.UUID
		wantErr          error
		validate         func(*testing.T, *EvaluationResult)
	}{
		{
			name:             "valid default ALLOW decision",
			defaultDecision:  DecisionAllow,
			evaluatedRuleIDs: evaluatedIDs,
			wantErr:          nil,
			validate: func(t *testing.T, r *EvaluationResult) {
				assert.Equal(t, DecisionAllow, r.Decision)
				assert.Empty(t, r.MatchedRuleIDs)
				assert.Len(t, r.EvaluatedRuleIDs, 3)
				assert.Contains(t, r.Reason, "No matching rules")
			},
		},
		{
			name:             "valid default DENY decision",
			defaultDecision:  DecisionDeny,
			evaluatedRuleIDs: evaluatedIDs,
			wantErr:          nil,
			validate: func(t *testing.T, r *EvaluationResult) {
				assert.Equal(t, DecisionDeny, r.Decision)
				assert.Empty(t, r.MatchedRuleIDs)
				assert.Len(t, r.EvaluatedRuleIDs, 3)
			},
		},
		{
			name:             "valid default REVIEW decision",
			defaultDecision:  DecisionReview,
			evaluatedRuleIDs: evaluatedIDs,
			wantErr:          nil,
			validate: func(t *testing.T, r *EvaluationResult) {
				assert.Equal(t, DecisionReview, r.Decision)
				assert.Empty(t, r.MatchedRuleIDs)
			},
		},
		{
			name:             "empty evaluated rules",
			defaultDecision:  DecisionAllow,
			evaluatedRuleIDs: []uuid.UUID{},
			wantErr:          nil,
			validate: func(t *testing.T, r *EvaluationResult) {
				assert.Equal(t, DecisionAllow, r.Decision)
				assert.Empty(t, r.MatchedRuleIDs)
				assert.Empty(t, r.EvaluatedRuleIDs)
			},
		},
		{
			name:             "nil evaluatedRuleIDs normalized to empty slice",
			defaultDecision:  DecisionDeny,
			evaluatedRuleIDs: nil,
			wantErr:          nil,
			validate: func(t *testing.T, r *EvaluationResult) {
				assert.NotNil(t, r.EvaluatedRuleIDs)
				assert.Empty(t, r.EvaluatedRuleIDs)
				assert.NotNil(t, r.MatchedRuleIDs)
				assert.Empty(t, r.MatchedRuleIDs)
			},
		},
		{
			name:             "invalid default decision returns error",
			defaultDecision:  Decision("INVALID"),
			evaluatedRuleIDs: evaluatedIDs,
			wantErr:          constant.ErrInvalidDefaultDecision,
			validate:         nil,
		},
		{
			name:             "empty default decision returns error",
			defaultDecision:  Decision(""),
			evaluatedRuleIDs: evaluatedIDs,
			wantErr:          constant.ErrInvalidDefaultDecision,
			validate:         nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewNoMatchResult(tt.defaultDecision, tt.evaluatedRuleIDs)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

func TestWithTruncationInfo(t *testing.T) {
	matchedID := testutil.MustDeterministicUUID(1)
	evaluatedIDs := []uuid.UUID{matchedID, testutil.MustDeterministicUUID(2)}

	tests := []struct {
		name             string
		totalRulesLoaded int
		truncated        bool
		wantTotalLoaded  int
		wantTruncated    bool
	}{
		{
			name:             "no truncation",
			totalRulesLoaded: 50,
			truncated:        false,
			wantTotalLoaded:  50,
			wantTruncated:    false,
		},
		{
			name:             "with truncation",
			totalRulesLoaded: 150,
			truncated:        true,
			wantTotalLoaded:  150,
			wantTruncated:    true,
		},
		{
			name:             "zero rules loaded",
			totalRulesLoaded: 0,
			truncated:        false,
			wantTotalLoaded:  0,
			wantTruncated:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewEvaluationResult(DecisionAllow, []uuid.UUID{matchedID}, evaluatedIDs, "test reason")
			require.NoError(t, err)

			// Apply truncation info
			result = result.WithTruncationInfo(tt.totalRulesLoaded, tt.truncated)

			// Verify truncation info was set
			assert.Equal(t, tt.wantTotalLoaded, result.TotalRulesLoaded)
			assert.Equal(t, tt.wantTruncated, result.Truncated)

			// Verify original fields are unchanged
			assert.Equal(t, DecisionAllow, result.Decision)
			assert.Len(t, result.MatchedRuleIDs, 1)
			assert.Len(t, result.EvaluatedRuleIDs, 2)
			assert.Equal(t, "test reason", result.Reason)
		})
	}
}
