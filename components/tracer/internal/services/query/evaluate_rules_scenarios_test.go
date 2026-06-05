// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// Scenario tests verify the complete EvaluateRulesQuery flow with realistic business scenarios.
// These are mock-based tests (not true integration tests against real dependencies).
// They focus on specific business rules:
// 1. DENY precedence - DENY always wins regardless of other matches
// 2. Scope filtering - only scope-matched rules are evaluated
// 3. Default decision fallback - when no rules match
// 4. MaxRulesPerRequest enforcement - truncates rule set

func TestEvaluateRulesIntegration_DenyPrecedence_AllThreeRuleTypesMatch(t *testing.T) {
	// Scenario: Transaction matches DENY, ALLOW, and REVIEW rules
	// Expected: DENY takes precedence over all other decisions
	ctrl := gomock.NewController(t)

	testutil.SetupTestTracing(t)

	denyRuleID := testutil.MustDeterministicUUID(1)
	allowRuleID := testutil.MustDeterministicUUID(2)
	reviewRuleID := testutil.MustDeterministicUUID(3)

	denyRule := &model.Rule{
		ID:         denyRuleID,
		Name:       "Block High Value Transactions",
		Expression: "amount > 100",
		Action:     model.DecisionDeny,
		Scopes:     []model.Scope{},
	}
	allowRule := &model.Rule{
		ID:         allowRuleID,
		Name:       "Allow All Card Transactions",
		Expression: "transaction_type == 'CARD'",
		Action:     model.DecisionAllow,
		Scopes:     []model.Scope{},
	}
	reviewRule := &model.Rule{
		ID:         reviewRuleID,
		Name:       "Review Large Transactions",
		Expression: "amount > 50",
		Action:     model.DecisionReview,
		Scopes:     []model.Scope{},
	}

	testReq := &model.ValidationRequest{
		RequestID:       testutil.MustDeterministicUUID(100),
		TransactionType: model.TransactionTypeCard,
		Amount:          decimal.RequireFromString("150"), // Matches all 3 rules
		Currency:        "USD",
		Account:         model.AccountContext{ID: testutil.MustDeterministicUUID(200), Type: "checking"},
	}

	mockGetActive := NewMockGetActiveRulesExecutor(ctrl)
	mockEvaluator := NewMockCompleteRuleEvaluator(ctrl)

	// All 3 rules returned
	mockGetActive.EXPECT().Execute(gomock.Any(), gomock.Any()).Return([]*model.Rule{denyRule, allowRule, reviewRule}, nil)

	// All 3 rules match
	mockEvaluator.EXPECT().EvaluateAll(gomock.Any(), gomock.Any(), gomock.Any()).Return(&EvaluationCollector{
		DenyRuleIDs:      []uuid.UUID{denyRuleID},
		AllowRuleIDs:     []uuid.UUID{allowRuleID},
		ReviewRuleIDs:    []uuid.UUID{reviewRuleID},
		EvaluatedRuleIDs: []uuid.UUID{denyRuleID, allowRuleID, reviewRuleID},
	}, nil)

	config := &EvaluationConfig{
		DefaultDecisionWhenNoMatch: model.DecisionAllow,
		MaxRulesPerRequest:         1000,
	}

	query, err := NewEvaluateRulesQuery(mockGetActive, mockEvaluator, config)
	require.NoError(t, err)

	result, err := query.Execute(context.Background(), testReq)

	require.NoError(t, err)
	assert.Equal(t, model.DecisionDeny, result.Decision, "DENY should take precedence over ALLOW and REVIEW")
	assert.Len(t, result.EvaluatedRuleIDs, 3, "All 3 rules should be evaluated")
	assert.Len(t, result.MatchedRuleIDs, 3, "All 3 rules should match")
	assert.Contains(t, result.MatchedRuleIDs, denyRuleID, "DENY rule should be in matched rules")
	assert.Contains(t, result.MatchedRuleIDs, allowRuleID, "ALLOW rule should be in matched rules")
	assert.Contains(t, result.MatchedRuleIDs, reviewRuleID, "REVIEW rule should be in matched rules")
	assert.Contains(t, result.Reason, "DENY", "Reason should indicate DENY")
}

func TestEvaluateRulesIntegration_ScopeFiltering_OnlyScopeMatchedRulesEvaluated(t *testing.T) {
	// Scenario: Two rules exist, but only one matches transaction scopes
	// Expected: Only the scope-matched rule is evaluated
	ctrl := gomock.NewController(t)

	testutil.SetupTestTracing(t)

	accountID := testutil.MustDeterministicUUID(1)
	otherAccountID := testutil.MustDeterministicUUID(2)

	matchingRuleID := testutil.MustDeterministicUUID(10)
	nonMatchingRuleID := testutil.MustDeterministicUUID(20)

	matchingRule := &model.Rule{
		ID:         matchingRuleID,
		Name:       "Account Specific Rule",
		Expression: "amount > 10",
		Action:     model.DecisionDeny,
		Scopes:     []model.Scope{{AccountID: &accountID}},
	}
	nonMatchingRule := &model.Rule{
		ID:         nonMatchingRuleID,
		Name:       "Other Account Rule",
		Expression: "amount > 10",
		Action:     model.DecisionDeny,
		Scopes:     []model.Scope{{AccountID: &otherAccountID}},
	}

	testReq := &model.ValidationRequest{
		RequestID:       testutil.MustDeterministicUUID(100),
		TransactionType: model.TransactionTypeCard,
		Amount:          decimal.RequireFromString("50"),
		Currency:        "USD",
		Account:         model.AccountContext{ID: accountID, Type: "checking"},
	}

	mockGetActive := NewMockGetActiveRulesExecutor(ctrl)
	mockEvaluator := NewMockCompleteRuleEvaluator(ctrl)

	// Both rules returned from repository
	mockGetActive.EXPECT().Execute(gomock.Any(), gomock.Any()).Return([]*model.Rule{matchingRule, nonMatchingRule}, nil)

	// CompleteEvaluator filters by scope - only matchingRule is evaluated
	mockEvaluator.EXPECT().EvaluateAll(gomock.Any(), gomock.Any(), gomock.Any()).Return(&EvaluationCollector{
		DenyRuleIDs:      []uuid.UUID{matchingRuleID},
		AllowRuleIDs:     []uuid.UUID{},
		ReviewRuleIDs:    []uuid.UUID{},
		EvaluatedRuleIDs: []uuid.UUID{matchingRuleID}, // Only 1 rule evaluated due to scope filtering
	}, nil)

	config := &EvaluationConfig{
		DefaultDecisionWhenNoMatch: model.DecisionAllow,
		MaxRulesPerRequest:         1000,
	}

	query, err := NewEvaluateRulesQuery(mockGetActive, mockEvaluator, config)
	require.NoError(t, err)

	result, err := query.Execute(context.Background(), testReq)

	require.NoError(t, err)
	assert.Equal(t, model.DecisionDeny, result.Decision)
	require.Len(t, result.EvaluatedRuleIDs, 1, "Only scope-matched rule should be evaluated")
	assert.Equal(t, matchingRuleID, result.EvaluatedRuleIDs[0])
}

func TestEvaluateRulesIntegration_DefaultDecisionFallback(t *testing.T) {
	// Scenario: No rules match the transaction
	// Expected: Use configured default decision (ALLOW or DENY)
	tests := []struct {
		name            string
		defaultDecision model.Decision
	}{
		{
			name:            "uses default ALLOW when no rules match",
			defaultDecision: model.DecisionAllow,
		},
		{
			name:            "uses default DENY when no rules match",
			defaultDecision: model.DecisionDeny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			testutil.SetupTestTracing(t)

			ruleID := testutil.MustDeterministicUUID(1)

			rule := &model.Rule{
				ID:         ruleID,
				Name:       "Non-matching Rule",
				Expression: "amount > 1000", // Very high threshold
				Action:     model.DecisionDeny,
				Scopes:     []model.Scope{},
			}

			testReq := &model.ValidationRequest{
				RequestID:       testutil.MustDeterministicUUID(100),
				TransactionType: model.TransactionTypeCard,
				Amount:          decimal.RequireFromString("1"), // Low amount doesn't match rule
				Currency:        "USD",
				Account:         model.AccountContext{ID: testutil.MustDeterministicUUID(200), Type: "checking"},
			}

			mockGetActive := NewMockGetActiveRulesExecutor(ctrl)
			mockEvaluator := NewMockCompleteRuleEvaluator(ctrl)

			mockGetActive.EXPECT().Execute(gomock.Any(), gomock.Any()).Return([]*model.Rule{rule}, nil)

			// Rule evaluated but expression returns false - no match
			mockEvaluator.EXPECT().EvaluateAll(gomock.Any(), gomock.Any(), gomock.Any()).Return(&EvaluationCollector{
				DenyRuleIDs:      []uuid.UUID{},
				AllowRuleIDs:     []uuid.UUID{},
				ReviewRuleIDs:    []uuid.UUID{},
				EvaluatedRuleIDs: []uuid.UUID{ruleID},
			}, nil)

			config := &EvaluationConfig{
				DefaultDecisionWhenNoMatch: tt.defaultDecision,
				MaxRulesPerRequest:         1000,
			}

			query, err := NewEvaluateRulesQuery(mockGetActive, mockEvaluator, config)
			require.NoError(t, err)

			result, err := query.Execute(context.Background(), testReq)

			require.NoError(t, err)
			assert.Equal(t, tt.defaultDecision, result.Decision, "Should use configured default decision")
			assert.Empty(t, result.MatchedRuleIDs, "No rules should match")
			assert.Len(t, result.EvaluatedRuleIDs, 1, "Rule should still be evaluated")
		})
	}
}

func TestEvaluateRulesIntegration_MaxRulesPerRequestLimit(t *testing.T) {
	// Scenario: Repository returns 100 rules, MaxRulesPerRequest is 10
	// Expected: Only first 10 rules are evaluated
	ctrl := gomock.NewController(t)

	testutil.SetupTestTracing(t)

	const totalRules = 100
	const maxRules = 10

	rules := make([]*model.Rule, totalRules)
	for i := range totalRules {
		rules[i] = &model.Rule{
			ID:         testutil.MustDeterministicUUID(int64(i)),
			Name:       "Test Rule",
			Expression: "true",
			Action:     model.DecisionAllow,
			Scopes:     []model.Scope{},
		}
	}

	testReq := &model.ValidationRequest{
		RequestID:       testutil.MustDeterministicUUID(1000),
		TransactionType: model.TransactionTypeCard,
		Amount:          decimal.RequireFromString("10"),
		Currency:        "USD",
		Account:         model.AccountContext{ID: testutil.MustDeterministicUUID(2000), Type: "checking"},
	}

	mockGetActive := NewMockGetActiveRulesExecutor(ctrl)
	mockEvaluator := NewMockCompleteRuleEvaluator(ctrl)

	// Returns all 100 rules
	mockGetActive.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(rules, nil)

	// Build expected evaluated IDs (only first 10 due to truncation)
	expectedEvaluatedIDs := make([]uuid.UUID, maxRules)
	expectedAllowIDs := make([]uuid.UUID, maxRules)
	for i := range maxRules {
		expectedEvaluatedIDs[i] = testutil.MustDeterministicUUID(int64(i))
		expectedAllowIDs[i] = testutil.MustDeterministicUUID(int64(i))
	}

	// Expects truncated slice (only 10 rules)
	mockEvaluator.EXPECT().
		EvaluateAll(gomock.Any(), gomock.Len(maxRules), gomock.Any()).
		Return(&EvaluationCollector{
			DenyRuleIDs:      []uuid.UUID{},
			AllowRuleIDs:     expectedAllowIDs,
			ReviewRuleIDs:    []uuid.UUID{},
			EvaluatedRuleIDs: expectedEvaluatedIDs,
		}, nil)

	config := &EvaluationConfig{
		DefaultDecisionWhenNoMatch: model.DecisionAllow,
		MaxRulesPerRequest:         maxRules,
	}

	query, err := NewEvaluateRulesQuery(mockGetActive, mockEvaluator, config)
	require.NoError(t, err)

	result, err := query.Execute(context.Background(), testReq)

	require.NoError(t, err)
	assert.Equal(t, model.DecisionAllow, result.Decision)
	assert.Len(t, result.EvaluatedRuleIDs, maxRules, "Only MaxRulesPerRequest rules should be evaluated")

	// Verify truncation metadata
	assert.Equal(t, totalRules, result.TotalRulesLoaded, "TotalRulesLoaded should reflect original count before truncation")
	assert.True(t, result.Truncated, "Truncated should be true when rules exceed MaxRulesPerRequest")
}

func TestEvaluateRulesIntegration_ReviewWithoutDeny_ReturnsReview(t *testing.T) {
	// Scenario: ALLOW and REVIEW match, but no DENY
	// Expected: REVIEW takes precedence over ALLOW
	ctrl := gomock.NewController(t)

	testutil.SetupTestTracing(t)

	allowRuleID := testutil.MustDeterministicUUID(1)
	reviewRuleID := testutil.MustDeterministicUUID(2)

	allowRule := &model.Rule{
		ID:         allowRuleID,
		Name:       "Allow Low Value",
		Expression: "amount < 100",
		Action:     model.DecisionAllow,
		Scopes:     []model.Scope{},
	}
	reviewRule := &model.Rule{
		ID:         reviewRuleID,
		Name:       "Review Medium Value",
		Expression: "amount > 10",
		Action:     model.DecisionReview,
		Scopes:     []model.Scope{},
	}

	testReq := &model.ValidationRequest{
		RequestID:       testutil.MustDeterministicUUID(100),
		TransactionType: model.TransactionTypeCard,
		Amount:          decimal.RequireFromString("50"), // Matches both rules
		Currency:        "USD",
		Account:         model.AccountContext{ID: testutil.MustDeterministicUUID(200), Type: "checking"},
	}

	mockGetActive := NewMockGetActiveRulesExecutor(ctrl)
	mockEvaluator := NewMockCompleteRuleEvaluator(ctrl)

	mockGetActive.EXPECT().Execute(gomock.Any(), gomock.Any()).Return([]*model.Rule{allowRule, reviewRule}, nil)

	mockEvaluator.EXPECT().EvaluateAll(gomock.Any(), gomock.Any(), gomock.Any()).Return(&EvaluationCollector{
		DenyRuleIDs:      []uuid.UUID{},
		AllowRuleIDs:     []uuid.UUID{allowRuleID},
		ReviewRuleIDs:    []uuid.UUID{reviewRuleID},
		EvaluatedRuleIDs: []uuid.UUID{allowRuleID, reviewRuleID},
	}, nil)

	config := &EvaluationConfig{
		DefaultDecisionWhenNoMatch: model.DecisionAllow,
		MaxRulesPerRequest:         1000,
	}

	query, err := NewEvaluateRulesQuery(mockGetActive, mockEvaluator, config)
	require.NoError(t, err)

	result, err := query.Execute(context.Background(), testReq)

	require.NoError(t, err)
	assert.Equal(t, model.DecisionReview, result.Decision, "REVIEW should take precedence over ALLOW when no DENY")
	assert.Len(t, result.EvaluatedRuleIDs, 2)
	assert.Len(t, result.MatchedRuleIDs, 2, "Both rules should match")
	assert.Contains(t, result.MatchedRuleIDs, allowRuleID, "ALLOW rule should be in matched rules")
	assert.Contains(t, result.MatchedRuleIDs, reviewRuleID, "REVIEW rule should be in matched rules")
}
