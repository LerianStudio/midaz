// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

func TestCompleteEvaluator_EvaluateAll(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	testAccountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	// Sentinel errors for ErrorIs checking
	errCELEvaluation := errors.New("CEL evaluation error")

	// Create test request for all test cases
	testRequest := &model.ValidationRequest{
		RequestID:            uuid.MustParse("550e8400-e29b-41d4-a716-446655440002"),
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("1500"), // $1500.00
		Currency:             "USD",
		TransactionTimestamp: now,
		Account: model.AccountContext{
			ID: testAccountID,
		},
		Metadata: map[string]any{},
	}

	// Create test rules with different actions
	denyRuleID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	secondDenyRuleID := uuid.MustParse("11111111-1111-1111-1111-111111111112")
	allowRuleID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	reviewRuleID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	nonMatchingRuleID := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	denyRule := &model.Rule{
		ID:         denyRuleID,
		Name:       "Deny high amount",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusActive,
		Scopes:     []model.Scope{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	allowRule := &model.Rule{
		ID:         allowRuleID,
		Name:       "Allow standard",
		Expression: "amount > 0",
		Action:     model.DecisionAllow,
		Status:     model.RuleStatusActive,
		Scopes:     []model.Scope{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	reviewRule := &model.Rule{
		ID:         reviewRuleID,
		Name:       "Review medium",
		Expression: "amount > 500",
		Action:     model.DecisionReview,
		Status:     model.RuleStatusActive,
		Scopes:     []model.Scope{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	nonMatchingRule := &model.Rule{
		ID:         nonMatchingRuleID,
		Name:       "Never matches",
		Expression: "amount < 0",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusActive,
		Scopes:     []model.Scope{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	secondDenyRule := &model.Rule{
		ID:         secondDenyRuleID,
		Name:       "Second deny rule",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusActive,
		Scopes:     []model.Scope{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	tests := []struct {
		name                  string
		rules                 []*model.Rule
		request               *model.ValidationRequest
		mockSetup             func(ctrl *gomock.Controller) SingleRuleEvaluator
		ctxSetup              func() context.Context // optional: custom context setup
		expectedDenyRuleIDs   []uuid.UUID
		expectedAllowRuleIDs  []uuid.UUID
		expectedReviewRuleIDs []uuid.UUID
		expectedEvaluatedIDs  []uuid.UUID
		wantErr               bool
		expectedErr           error  // for ErrorIs checking (e.g., context.Canceled)
		expectedErrMsg        string // for substring matching (when expectedErr is nil)
	}{
		{
			name:    "all rules evaluated with mixed matches (DENY, REVIEW, ALLOW)",
			rules:   []*model.Rule{denyRule, allowRule, reviewRule, nonMatchingRule},
			request: testRequest,
			mockSetup: func(ctrl *gomock.Controller) SingleRuleEvaluator {
				mockEval := NewMockSingleRuleEvaluator(ctrl)
				// Successful matches do not short-circuit; all rules should be evaluated
				mockEval.EXPECT().
					Evaluate(gomock.Any(), denyRule, testRequest).
					Return(true, nil)
				mockEval.EXPECT().
					Evaluate(gomock.Any(), allowRule, testRequest).
					Return(true, nil)
				mockEval.EXPECT().
					Evaluate(gomock.Any(), reviewRule, testRequest).
					Return(true, nil)
				mockEval.EXPECT().
					Evaluate(gomock.Any(), nonMatchingRule, testRequest).
					Return(false, nil)

				return mockEval
			},
			expectedDenyRuleIDs:   []uuid.UUID{denyRuleID},
			expectedAllowRuleIDs:  []uuid.UUID{allowRuleID},
			expectedReviewRuleIDs: []uuid.UUID{reviewRuleID},
			expectedEvaluatedIDs:  []uuid.UUID{denyRuleID, allowRuleID, reviewRuleID, nonMatchingRuleID},
			wantErr:               false,
		},
		{
			name:    "no matches - all rules evaluated but none match",
			rules:   []*model.Rule{nonMatchingRule},
			request: testRequest,
			mockSetup: func(ctrl *gomock.Controller) SingleRuleEvaluator {
				mockEval := NewMockSingleRuleEvaluator(ctrl)
				mockEval.EXPECT().
					Evaluate(gomock.Any(), nonMatchingRule, testRequest).
					Return(false, nil)

				return mockEval
			},
			expectedDenyRuleIDs:   []uuid.UUID{},
			expectedAllowRuleIDs:  []uuid.UUID{},
			expectedReviewRuleIDs: []uuid.UUID{},
			expectedEvaluatedIDs:  []uuid.UUID{nonMatchingRuleID},
			wantErr:               false,
		},
		{
			name:    "returns error when rule evaluation fails",
			rules:   []*model.Rule{denyRule, allowRule},
			request: testRequest,
			mockSetup: func(ctrl *gomock.Controller) SingleRuleEvaluator {
				mockEval := NewMockSingleRuleEvaluator(ctrl)
				// Evaluation stops on error (fail-fast) so subsequent Evaluate calls should not occur
				mockEval.EXPECT().
					Evaluate(gomock.Any(), denyRule, testRequest).
					Return(false, errCELEvaluation)
				// No expectation for allowRule - fail-fast behavior

				return mockEval
			},
			expectedDenyRuleIDs:   nil,
			expectedAllowRuleIDs:  nil,
			expectedReviewRuleIDs: nil,
			expectedEvaluatedIDs:  nil,
			wantErr:               true,
			expectedErr:           errCELEvaluation,
		},
		{
			name:    "empty rules list returns empty collector",
			rules:   []*model.Rule{},
			request: testRequest,
			mockSetup: func(ctrl *gomock.Controller) SingleRuleEvaluator {
				mockEval := NewMockSingleRuleEvaluator(ctrl)
				// No expectations - no rules to evaluate

				return mockEval
			},
			expectedDenyRuleIDs:   []uuid.UUID{},
			expectedAllowRuleIDs:  []uuid.UUID{},
			expectedReviewRuleIDs: []uuid.UUID{},
			expectedEvaluatedIDs:  []uuid.UUID{},
			wantErr:               false,
		},
		{
			name:    "multiple rules of same action type",
			rules:   []*model.Rule{denyRule, secondDenyRule},
			request: testRequest,
			mockSetup: func(ctrl *gomock.Controller) SingleRuleEvaluator {
				mockEval := NewMockSingleRuleEvaluator(ctrl)
				// Both DENY rules match - explicit expectations for each rule
				mockEval.EXPECT().
					Evaluate(gomock.Any(), gomock.Eq(denyRule), testRequest).
					Return(true, nil)
				mockEval.EXPECT().
					Evaluate(gomock.Any(), gomock.Eq(secondDenyRule), testRequest).
					Return(true, nil)

				return mockEval
			},
			expectedDenyRuleIDs:   []uuid.UUID{denyRuleID, secondDenyRuleID},
			expectedAllowRuleIDs:  []uuid.UUID{},
			expectedReviewRuleIDs: []uuid.UUID{},
			expectedEvaluatedIDs:  []uuid.UUID{denyRuleID, secondDenyRuleID},
			wantErr:               false,
		},
		{
			name:    "context cancellation stops evaluation",
			rules:   []*model.Rule{denyRule, allowRule},
			request: testRequest,
			mockSetup: func(ctrl *gomock.Controller) SingleRuleEvaluator {
				mockEval := NewMockSingleRuleEvaluator(ctrl)
				// No EXPECT() calls - context is cancelled before any rule evaluation
				return mockEval
			},
			ctxSetup: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately before evaluation
				return ctx
			},
			wantErr:     true,
			expectedErr: context.Canceled,
		},
		{
			name:    "nil request returns error",
			rules:   []*model.Rule{denyRule, allowRule},
			request: nil,
			mockSetup: func(ctrl *gomock.Controller) SingleRuleEvaluator {
				mockEval := NewMockSingleRuleEvaluator(ctrl)
				// No EXPECT() calls - request validation fails before rule evaluation
				return mockEval
			},
			wantErr:     true,
			expectedErr: ErrNilRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test tracing to ensure lib-commons works correctly
			testutil.SetupTestTracing(t)

			ctrl := gomock.NewController(t)

			mockEval := tt.mockSetup(ctrl)

			evaluator, err := NewCompleteEvaluator(mockEval)
			require.NoError(t, err, "NewCompleteEvaluator should not return error with valid evaluator")

			ctx := context.Background()
			if tt.ctxSetup != nil {
				ctx = tt.ctxSetup()
			}
			result, err := evaluator.EvaluateAll(ctx, tt.rules, tt.request)

			if tt.wantErr {
				require.Error(t, err, "expected error from EvaluateAll for test case: %s", tt.name)
				if tt.expectedErr != nil {
					require.ErrorIs(t, err, tt.expectedErr)
				} else {
					assert.Contains(t, err.Error(), tt.expectedErrMsg)
				}
				assert.Nil(t, result, "expected nil result when error occurs")
			} else {
				require.NoError(t, err, "expected no error from CompleteEvaluator.EvaluateAll")
				require.NotNil(t, result, "expected non-nil result from CompleteEvaluator.EvaluateAll")

				assert.Equal(t, tt.expectedDenyRuleIDs, result.DenyRuleIDs)
				assert.Equal(t, tt.expectedAllowRuleIDs, result.AllowRuleIDs)
				assert.Equal(t, tt.expectedReviewRuleIDs, result.ReviewRuleIDs)
				assert.Equal(t, tt.expectedEvaluatedIDs, result.EvaluatedRuleIDs)
			}
		})
	}
}

func TestNewCompleteEvaluator(t *testing.T) {
	testutil.SetupTestTracing(t)

	t.Run("returns evaluator with valid input", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockEval := NewMockSingleRuleEvaluator(ctrl)

		evaluator, err := NewCompleteEvaluator(mockEval)

		require.NoError(t, err, "NewCompleteEvaluator should not return error with valid evaluator")
		require.NotNil(t, evaluator, "NewCompleteEvaluator should return non-nil evaluator")
	})

	t.Run("returns error when underlying evaluator is nil", func(t *testing.T) {
		evaluator, err := NewCompleteEvaluator(nil)

		require.Error(t, err, "NewCompleteEvaluator should return error when evaluator is nil")
		require.ErrorIs(t, err, ErrNilSingleRuleEvaluator, "should return sentinel error for nil evaluator")
		assert.Nil(t, evaluator, "NewCompleteEvaluator should return nil evaluator when input is nil")
	})
}

func TestCompleteEvaluator_EvaluateAll_UnknownAction(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)
	mockEval := NewMockSingleRuleEvaluator(ctrl)

	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	testAccountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	testRequest := &model.ValidationRequest{
		RequestID:            uuid.MustParse("550e8400-e29b-41d4-a716-446655440003"),
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("1500"),
		Currency:             "USD",
		TransactionTimestamp: now,
		Account: model.AccountContext{
			ID: testAccountID,
		},
		Metadata: map[string]any{},
	}

	// Rule with unknown action type (simulating future action type)
	unknownActionRule := &model.Rule{
		ID:         uuid.MustParse("55555555-5555-5555-5555-555555555555"),
		Name:       "Unknown action rule",
		Expression: "amount > 0",
		Action:     model.Decision("UNKNOWN_ACTION"), // Unknown action type
		Status:     model.RuleStatusActive,
		Scopes:     []model.Scope{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Mock: rule matches
	mockEval.EXPECT().
		Evaluate(gomock.Any(), unknownActionRule, testRequest).
		Return(true, nil)

	evaluator, err := NewCompleteEvaluator(mockEval)
	require.NoError(t, err, "NewCompleteEvaluator should not return error")

	ctx := context.Background()

	result, err := evaluator.EvaluateAll(ctx, []*model.Rule{unknownActionRule}, testRequest)

	require.NoError(t, err, "expected no error when evaluating rule with unknown action")
	require.NotNil(t, result, "expected non-nil result from EvaluateAll")

	// Unknown action should NOT be added to any category
	assert.Empty(t, result.DenyRuleIDs, "unknown action should not be in deny")
	assert.Empty(t, result.AllowRuleIDs, "unknown action should not be in allow")
	assert.Empty(t, result.ReviewRuleIDs, "unknown action should not be in review")

	// But it should still be tracked as evaluated
	assert.Len(t, result.EvaluatedRuleIDs, 1, "rule should still be evaluated")
	assert.Equal(t, unknownActionRule.ID, result.EvaluatedRuleIDs[0])
}

func TestCompleteEvaluator_EvaluateAll_NilRuleSkipped(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)
	mockEval := NewMockSingleRuleEvaluator(ctrl)

	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	testAccountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	testRequest := &model.ValidationRequest{
		RequestID:            uuid.MustParse("550e8400-e29b-41d4-a716-446655440004"),
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("1500"),
		Currency:             "USD",
		TransactionTimestamp: now,
		Account: model.AccountContext{
			ID: testAccountID,
		},
		Metadata: map[string]any{},
	}

	validRule := &model.Rule{
		ID:         uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Name:       "Valid rule",
		Expression: "amount > 0",
		Action:     model.DecisionAllow,
		Status:     model.RuleStatusActive,
		Scopes:     []model.Scope{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Rules slice with nil entry - should be skipped without panic
	rulesWithNil := []*model.Rule{nil, validRule, nil}

	// Only the valid rule should be evaluated
	mockEval.EXPECT().
		Evaluate(gomock.Any(), validRule, testRequest).
		Return(true, nil)

	evaluator, err := NewCompleteEvaluator(mockEval)
	require.NoError(t, err, "NewCompleteEvaluator should not return error")

	ctx := context.Background()

	result, err := evaluator.EvaluateAll(ctx, rulesWithNil, testRequest)

	require.NoError(t, err, "expected no error when evaluating rules with nil entries")
	require.NotNil(t, result, "expected non-nil result from EvaluateAll")

	// Only the valid rule should be in results
	assert.Len(t, result.EvaluatedRuleIDs, 1, "only valid rule should be evaluated")
	assert.Equal(t, validRule.ID, result.EvaluatedRuleIDs[0])
	assert.Len(t, result.AllowRuleIDs, 1, "valid rule should be in allow")
	assert.Empty(t, result.DenyRuleIDs, "no deny rules")
	assert.Empty(t, result.ReviewRuleIDs, "no review rules")
}
