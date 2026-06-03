// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"tracer/internal/testutil"
	"tracer/pkg/model"
)

func TestNewEvaluateRulesQuery(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockGetActive := NewMockGetActiveRulesExecutor(ctrl)
	mockEvaluator := NewMockCompleteRuleEvaluator(ctrl)
	validConfig := &EvaluationConfig{DefaultDecisionWhenNoMatch: model.DecisionAllow, MaxRulesPerRequest: 1000}

	tests := []struct {
		name        string
		getActive   GetActiveRulesExecutor
		completeEva CompleteRuleEvaluator
		config      *EvaluationConfig
		expectErr   error
	}{
		{
			name:        "creates query with valid dependencies",
			getActive:   mockGetActive,
			completeEva: mockEvaluator,
			config:      validConfig,
			expectErr:   nil,
		},
		{
			name:        "returns error when getActiveRules is nil",
			getActive:   nil,
			completeEva: mockEvaluator,
			config:      validConfig,
			expectErr:   ErrNilGetActiveRulesQuery,
		},
		{
			name:        "returns error when completeEvaluator is nil",
			getActive:   mockGetActive,
			completeEva: nil,
			config:      validConfig,
			expectErr:   ErrNilCompleteEvaluator,
		},
		{
			name:        "returns error when config is nil",
			getActive:   mockGetActive,
			completeEva: mockEvaluator,
			config:      nil,
			expectErr:   ErrNilEvaluationConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := NewEvaluateRulesQuery(tt.getActive, tt.completeEva, tt.config)

			if tt.expectErr != nil {
				require.ErrorIs(t, err, tt.expectErr)
				assert.Nil(t, query)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, query)
			}
		})
	}
}

func TestEvaluateRulesQuery_Execute(t *testing.T) {
	// Setup deterministic test data
	denyRuleID := testutil.MustDeterministicUUID(1)
	allowRuleID := testutil.MustDeterministicUUID(2)
	reviewRuleID := testutil.MustDeterministicUUID(3)

	denyRule := &model.Rule{
		ID:         denyRuleID,
		Name:       "Deny High Value",
		Expression: "amount > 100",
		Action:     model.DecisionDeny,
		Scopes:     []model.Scope{},
	}
	allowRule := &model.Rule{
		ID:         allowRuleID,
		Name:       "Allow Low Value",
		Expression: "amount <= 100",
		Action:     model.DecisionAllow,
		Scopes:     []model.Scope{},
	}
	reviewRule := &model.Rule{
		ID:         reviewRuleID,
		Name:       "Review Medium Value",
		Expression: "amount > 50",
		Action:     model.DecisionReview,
		Scopes:     []model.Scope{},
	}

	testReq := &model.ValidationRequest{
		RequestID:       testutil.MustDeterministicUUID(100),
		TransactionType: model.TransactionTypeCard,
		Amount:          decimal.RequireFromString("150"),
		Currency:        "USD",
		Account:         model.AccountContext{ID: testutil.MustDeterministicUUID(200), Type: "checking"},
	}

	tests := []struct {
		name                 string
		mockRules            []*model.Rule
		mockRulesErr         error
		mockCollector        *EvaluationCollector
		mockCollectorErr     error
		maxRulesPerRequest   int
		defaultDecision      model.Decision
		expectedDecision     model.Decision
		expectedMatchedLen   int
		expectedEvaluatedLen int
		expectErr            bool
		expectedErrContains  string
	}{
		{
			name:      "DENY rule matches - DENY takes precedence",
			mockRules: []*model.Rule{denyRule, allowRule},
			mockCollector: &EvaluationCollector{
				DenyRuleIDs:      []uuid.UUID{denyRuleID},
				AllowRuleIDs:     []uuid.UUID{allowRuleID},
				ReviewRuleIDs:    []uuid.UUID{},
				EvaluatedRuleIDs: []uuid.UUID{denyRuleID, allowRuleID},
			},
			maxRulesPerRequest:   1000,
			defaultDecision:      model.DecisionAllow,
			expectedDecision:     model.DecisionDeny,
			expectedMatchedLen:   2, // ALL matched rules: DENY + ALLOW
			expectedEvaluatedLen: 2,
		},
		{
			name:      "only ALLOW matches - returns ALLOW",
			mockRules: []*model.Rule{denyRule, allowRule},
			mockCollector: &EvaluationCollector{
				DenyRuleIDs:      []uuid.UUID{},
				AllowRuleIDs:     []uuid.UUID{allowRuleID},
				ReviewRuleIDs:    []uuid.UUID{},
				EvaluatedRuleIDs: []uuid.UUID{denyRuleID, allowRuleID},
			},
			maxRulesPerRequest:   1000,
			defaultDecision:      model.DecisionAllow,
			expectedDecision:     model.DecisionAllow,
			expectedMatchedLen:   1,
			expectedEvaluatedLen: 2,
		},
		{
			name:      "REVIEW matches without DENY - returns REVIEW",
			mockRules: []*model.Rule{allowRule, reviewRule},
			mockCollector: &EvaluationCollector{
				DenyRuleIDs:      []uuid.UUID{},
				AllowRuleIDs:     []uuid.UUID{allowRuleID},
				ReviewRuleIDs:    []uuid.UUID{reviewRuleID},
				EvaluatedRuleIDs: []uuid.UUID{allowRuleID, reviewRuleID},
			},
			maxRulesPerRequest:   1000,
			defaultDecision:      model.DecisionAllow,
			expectedDecision:     model.DecisionReview,
			expectedMatchedLen:   2, // ALL matched rules: REVIEW + ALLOW
			expectedEvaluatedLen: 2,
		},
		{
			name:      "no matches - uses default ALLOW",
			mockRules: []*model.Rule{denyRule},
			mockCollector: &EvaluationCollector{
				DenyRuleIDs:      []uuid.UUID{},
				AllowRuleIDs:     []uuid.UUID{},
				ReviewRuleIDs:    []uuid.UUID{},
				EvaluatedRuleIDs: []uuid.UUID{denyRuleID},
			},
			maxRulesPerRequest:   1000,
			defaultDecision:      model.DecisionAllow,
			expectedDecision:     model.DecisionAllow,
			expectedMatchedLen:   0,
			expectedEvaluatedLen: 1,
		},
		{
			name:      "no matches - uses default DENY",
			mockRules: []*model.Rule{denyRule},
			mockCollector: &EvaluationCollector{
				DenyRuleIDs:      []uuid.UUID{},
				AllowRuleIDs:     []uuid.UUID{},
				ReviewRuleIDs:    []uuid.UUID{},
				EvaluatedRuleIDs: []uuid.UUID{denyRuleID},
			},
			maxRulesPerRequest:   1000,
			defaultDecision:      model.DecisionDeny,
			expectedDecision:     model.DecisionDeny,
			expectedMatchedLen:   0,
			expectedEvaluatedLen: 1,
		},
		{
			name:      "empty rules slice - uses default decision",
			mockRules: []*model.Rule{},
			mockCollector: &EvaluationCollector{
				DenyRuleIDs:      []uuid.UUID{},
				AllowRuleIDs:     []uuid.UUID{},
				ReviewRuleIDs:    []uuid.UUID{},
				EvaluatedRuleIDs: []uuid.UUID{},
			},
			maxRulesPerRequest:   1000,
			defaultDecision:      model.DecisionAllow,
			expectedDecision:     model.DecisionAllow,
			expectedMatchedLen:   0,
			expectedEvaluatedLen: 0,
		},
		{
			name:                "returns error when loading rules fails",
			mockRulesErr:        errors.New("database error"),
			maxRulesPerRequest:  1000,
			defaultDecision:     model.DecisionAllow,
			expectErr:           true,
			expectedErrContains: "failed to load rules",
		},
		{
			name:                "returns error when evaluation fails",
			mockRules:           []*model.Rule{denyRule},
			mockCollectorErr:    errors.New("evaluation error"),
			maxRulesPerRequest:  1000,
			defaultDecision:     model.DecisionAllow,
			expectErr:           true,
			expectedErrContains: "failed to evaluate rules",
		},
		{
			name:      "truncates rules when exceeding MaxRulesPerRequest",
			mockRules: []*model.Rule{denyRule, allowRule, reviewRule},
			mockCollector: &EvaluationCollector{
				DenyRuleIDs:      []uuid.UUID{denyRuleID},
				AllowRuleIDs:     []uuid.UUID{},
				ReviewRuleIDs:    []uuid.UUID{},
				EvaluatedRuleIDs: []uuid.UUID{denyRuleID, allowRuleID},
			},
			maxRulesPerRequest:   2, // Only 2 rules should be evaluated
			defaultDecision:      model.DecisionAllow,
			expectedDecision:     model.DecisionDeny,
			expectedMatchedLen:   1,
			expectedEvaluatedLen: 2,
		},
		{
			name:      "MaxRulesPerRequest=0 means no limit - all rules evaluated",
			mockRules: []*model.Rule{denyRule, allowRule, reviewRule},
			mockCollector: &EvaluationCollector{
				DenyRuleIDs:      []uuid.UUID{denyRuleID},
				AllowRuleIDs:     []uuid.UUID{allowRuleID},
				ReviewRuleIDs:    []uuid.UUID{reviewRuleID},
				EvaluatedRuleIDs: []uuid.UUID{denyRuleID, allowRuleID, reviewRuleID},
			},
			maxRulesPerRequest:   0, // 0 means no limit
			defaultDecision:      model.DecisionAllow,
			expectedDecision:     model.DecisionDeny,
			expectedMatchedLen:   3, // ALL matched rules: DENY + REVIEW + ALLOW
			expectedEvaluatedLen: 3, // All 3 rules evaluated
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			// Setup test tracing
			testutil.SetupTestTracing(t)

			// Create mock GetActiveRulesQuery
			mockGetActive := NewMockGetActiveRulesExecutor(ctrl)
			if tt.mockRulesErr != nil {
				mockGetActive.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, tt.mockRulesErr)
			} else {
				mockGetActive.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(tt.mockRules, nil)
			}

			// Create mock CompleteEvaluator
			mockEvaluator := NewMockCompleteRuleEvaluator(ctrl)
			if tt.mockRulesErr == nil {
				if tt.mockCollectorErr != nil {
					mockEvaluator.EXPECT().EvaluateAll(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, tt.mockCollectorErr)
				} else {
					// Expect the truncated rules if MaxRulesPerRequest is lower
					expectedRulesCount := len(tt.mockRules)
					if tt.maxRulesPerRequest > 0 && tt.maxRulesPerRequest < expectedRulesCount {
						expectedRulesCount = tt.maxRulesPerRequest
					}

					mockEvaluator.EXPECT().EvaluateAll(gomock.Any(), gomock.Len(expectedRulesCount), gomock.Any()).Return(tt.mockCollector, nil)
				}
			}

			config := &EvaluationConfig{
				DefaultDecisionWhenNoMatch: tt.defaultDecision,
				MaxRulesPerRequest:         tt.maxRulesPerRequest,
			}

			query, err := NewEvaluateRulesQuery(mockGetActive, mockEvaluator, config)
			require.NoError(t, err)

			result, err := query.Execute(context.Background(), testReq)

			if tt.expectErr {
				require.Error(t, err)
				if tt.expectedErrContains != "" {
					assert.Contains(t, err.Error(), tt.expectedErrContains)
				}

				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedDecision, result.Decision)
				assert.Len(t, result.MatchedRuleIDs, tt.expectedMatchedLen)
				assert.Len(t, result.EvaluatedRuleIDs, tt.expectedEvaluatedLen)
			}
		})
	}
}

func TestEvaluateRulesQuery_Execute_NilRequest(t *testing.T) {
	ctrl := gomock.NewController(t)

	testutil.SetupTestTracing(t)

	mockGetActive := NewMockGetActiveRulesExecutor(ctrl)
	mockEvaluator := NewMockCompleteRuleEvaluator(ctrl)
	config := &EvaluationConfig{
		DefaultDecisionWhenNoMatch: model.DecisionAllow,
		MaxRulesPerRequest:         1000,
	}

	query, err := NewEvaluateRulesQuery(mockGetActive, mockEvaluator, config)
	require.NoError(t, err)

	result, err := query.Execute(context.Background(), nil)

	require.ErrorIs(t, err, ErrNilValidationRequest)
	assert.Nil(t, result)
}

func TestEvaluateRulesQuery_Execute_ContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)

	testutil.SetupTestTracing(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mockGetActive := NewMockGetActiveRulesExecutor(ctrl)
	mockEvaluator := NewMockCompleteRuleEvaluator(ctrl)

	// No mock expectations - function returns early when context is cancelled

	config := &EvaluationConfig{
		DefaultDecisionWhenNoMatch: model.DecisionAllow,
		MaxRulesPerRequest:         1000,
	}

	testReq := &model.ValidationRequest{
		RequestID:       testutil.MustDeterministicUUID(100),
		TransactionType: model.TransactionTypeCard,
		Amount:          decimal.RequireFromString("10"),
		Currency:        "USD",
		Account:         model.AccountContext{ID: testutil.MustDeterministicUUID(200), Type: "checking"},
	}

	query, err := NewEvaluateRulesQuery(mockGetActive, mockEvaluator, config)
	require.NoError(t, err)

	result, err := query.Execute(ctx, testReq)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
}
