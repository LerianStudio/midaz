// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestRuleEvaluator_Evaluate(t *testing.T) {
	testRuleID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	testAccountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	testRequestID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
	testRequestNoMatchID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440003")
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

	testRule := &model.Rule{
		ID:         testRuleID,
		Name:       "High amount fraud rule",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusActive,
		Scopes:     []model.Scope{},
		CreatedAt:  now.Add(-24 * time.Hour),
		UpdatedAt:  now.Add(-1 * time.Hour),
	}

	testRequest := &model.ValidationRequest{
		RequestID:            testRequestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("1500"), // $1500.00, should trigger rule
		Currency:             "USD",
		TransactionTimestamp: now,
		Account: model.AccountContext{
			ID: testAccountID,
		},
		Metadata: map[string]any{},
	}

	testRequestNoMatch := &model.ValidationRequest{
		RequestID:            testRequestNoMatchID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("50"), // $50.00, should NOT trigger rule
		Currency:             "USD",
		TransactionTimestamp: now,
		Account: model.AccountContext{
			ID: testAccountID,
		},
		Metadata: map[string]any{},
	}

	mockCompiledProgram := &cel.CompiledProgram{
		ExpressionHash:   "test-hash",
		SourceExpression: testRule.Expression,
		CompiledAt:       now,
		CompileTimeMs:    1,
	}

	tests := []struct {
		name           string
		rule           *model.Rule
		request        *model.ValidationRequest
		mockSetup      func(ctrl *gomock.Controller) *MockExpressionEvaluator
		expectedResult bool
		wantErr        bool
		expectedErr    error
		expectedErrMsg string
	}{
		{
			name:    "rule matches when expression evaluates to true",
			rule:    testRule,
			request: testRequest,
			mockSetup: func(ctrl *gomock.Controller) *MockExpressionEvaluator {
				mockEval := NewMockExpressionEvaluator(ctrl)
				mockEval.EXPECT().
					Compile(gomock.Any(), testRule.Expression).
					Return(mockCompiledProgram, nil)
				mockEval.EXPECT().
					Evaluate(gomock.Any(), mockCompiledProgram, testRequest).
					Return(true, nil)
				return mockEval
			},
			expectedResult: true,
			wantErr:        false,
		},
		{
			name:    "rule does not match when expression evaluates to false",
			rule:    testRule,
			request: testRequestNoMatch,
			mockSetup: func(ctrl *gomock.Controller) *MockExpressionEvaluator {
				mockEval := NewMockExpressionEvaluator(ctrl)
				mockEval.EXPECT().
					Compile(gomock.Any(), testRule.Expression).
					Return(mockCompiledProgram, nil)
				mockEval.EXPECT().
					Evaluate(gomock.Any(), mockCompiledProgram, testRequestNoMatch).
					Return(false, nil)
				return mockEval
			},
			expectedResult: false,
			wantErr:        false,
		},
		{
			name:    "returns error when CEL compilation fails",
			rule:    testRule,
			request: testRequest,
			mockSetup: func(ctrl *gomock.Controller) *MockExpressionEvaluator {
				mockEval := NewMockExpressionEvaluator(ctrl)
				mockEval.EXPECT().
					Compile(gomock.Any(), testRule.Expression).
					Return(nil, errors.New("syntax error in expression"))
				return mockEval
			},
			expectedResult: false,
			wantErr:        true,
			expectedErrMsg: "failed to compile expression",
		},
		{
			name:    "returns error when CEL evaluation fails",
			rule:    testRule,
			request: testRequest,
			mockSetup: func(ctrl *gomock.Controller) *MockExpressionEvaluator {
				mockEval := NewMockExpressionEvaluator(ctrl)
				mockEval.EXPECT().
					Compile(gomock.Any(), testRule.Expression).
					Return(mockCompiledProgram, nil)
				mockEval.EXPECT().
					Evaluate(gomock.Any(), mockCompiledProgram, testRequest).
					Return(false, errors.New("evaluation error"))
				return mockEval
			},
			expectedResult: false,
			wantErr:        true,
			expectedErrMsg: "failed to evaluate expression",
		},
		{
			name:    "returns error when rule is nil",
			rule:    nil,
			request: testRequest,
			mockSetup: func(ctrl *gomock.Controller) *MockExpressionEvaluator {
				return NewMockExpressionEvaluator(ctrl)
			},
			expectedResult: false,
			wantErr:        true,
			expectedErr:    ErrNilRule,
		},
		{
			name:    "returns error when request is nil",
			rule:    testRule,
			request: nil,
			mockSetup: func(ctrl *gomock.Controller) *MockExpressionEvaluator {
				return NewMockExpressionEvaluator(ctrl)
			},
			expectedResult: false,
			wantErr:        true,
			expectedErr:    ErrNilRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupTestTracing(t)

			ctrl := gomock.NewController(t)

			mockEval := tt.mockSetup(ctrl)

			evaluator, err := NewRuleEvaluator(mockEval)
			require.NoError(t, err)

			ctx := context.Background()
			result, err := evaluator.Evaluate(ctx, tt.rule, tt.request)

			if tt.wantErr {
				require.Error(t, err)
				if tt.expectedErr != nil {
					assert.ErrorIs(t, err, tt.expectedErr)
				} else {
					assert.Contains(t, err.Error(), tt.expectedErrMsg)
				}
				assert.False(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestNewRuleEvaluator_NilExpressionEvaluator(t *testing.T) {
	evaluator, err := NewRuleEvaluator(nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilExpressionEvaluator)
	assert.Nil(t, evaluator)
}

// TestRuleEvaluator_ScopeMismatch verifies that rules with scopes that don't match
// the transaction return false without evaluating the expression.
func TestRuleEvaluator_ScopeMismatch(t *testing.T) {
	testutil.SetupTestTracing(t)

	ruleID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	differentAccountID := testutil.MustDeterministicUUID(3)
	requestID := testutil.MustDeterministicUUID(4)
	now := testutil.FixedTime()

	// Rule with specific account scope
	ruleWithScope := &model.Rule{
		ID:         ruleID,
		Name:       "Account specific rule",
		Expression: "amount > 10",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusActive,
		Scopes:     []model.Scope{{AccountID: &differentAccountID}}, // Different account
		CreatedAt:  now.Add(-24 * time.Hour),
		UpdatedAt:  now.Add(-1 * time.Hour),
	}

	// Request from a different account
	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("1500"),
		Currency:             "USD",
		TransactionTimestamp: now,
		Account: model.AccountContext{
			ID: accountID, // Different account than rule scope
		},
	}

	ctrl := gomock.NewController(t)
	// Mock should NOT be called because scope doesn't match
	mockEval := NewMockExpressionEvaluator(ctrl)

	evaluator, err := NewRuleEvaluator(mockEval)
	require.NoError(t, err)

	// Act
	result, err := evaluator.Evaluate(context.Background(), ruleWithScope, request)

	// Assert - rule didn't match because scope mismatched, no error
	require.NoError(t, err)
	assert.False(t, result, "Rule should not match when scopes don't match")
}

// TestRuleEvaluator_ScopeMatch verifies that rules with matching scopes
// do evaluate their expression.
func TestRuleEvaluator_ScopeMatch(t *testing.T) {
	testutil.SetupTestTracing(t)

	ruleID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	requestID := testutil.MustDeterministicUUID(3)
	now := testutil.FixedTime()

	// Rule with specific account scope
	ruleWithScope := &model.Rule{
		ID:         ruleID,
		Name:       "Account specific rule",
		Expression: "amount > 10",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusActive,
		Scopes:     []model.Scope{{AccountID: &accountID}}, // Same account
		CreatedAt:  now.Add(-24 * time.Hour),
		UpdatedAt:  now.Add(-1 * time.Hour),
	}

	// Request from the same account
	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("1500"),
		Currency:             "USD",
		TransactionTimestamp: now,
		Account: model.AccountContext{
			ID: accountID, // Same account as rule scope
		},
	}

	mockCompiledProgram := &cel.CompiledProgram{
		ExpressionHash:   "test-hash",
		SourceExpression: ruleWithScope.Expression,
		CompiledAt:       now,
		CompileTimeMs:    1,
	}

	ctrl := gomock.NewController(t)
	mockEval := NewMockExpressionEvaluator(ctrl)
	// Expect Compile and Evaluate to be called since scope matches
	mockEval.EXPECT().Compile(gomock.Any(), ruleWithScope.Expression).Return(mockCompiledProgram, nil)
	mockEval.EXPECT().Evaluate(gomock.Any(), mockCompiledProgram, request).Return(true, nil)

	evaluator, err := NewRuleEvaluator(mockEval)
	require.NoError(t, err)

	// Act
	result, err := evaluator.Evaluate(context.Background(), ruleWithScope, request)

	// Assert - rule should match because scope matched and expression evaluated to true
	require.NoError(t, err)
	assert.True(t, result, "Rule should match when scopes match and expression is true")
}

// TestRuleEvaluator_EmptyScopesMatchAny verifies that a rule with an empty Scopes
// slice is treated as a global rule and matches any account.
func TestRuleEvaluator_EmptyScopesMatchAny(t *testing.T) {
	testutil.SetupTestTracing(t)

	ruleID := testutil.MustDeterministicUUID(10)
	accountID := testutil.MustDeterministicUUID(11)
	requestID := testutil.MustDeterministicUUID(12)
	now := testutil.FixedTime()

	// Rule with empty scopes = global rule
	globalRule := &model.Rule{
		ID:         ruleID,
		Name:       "Global rule",
		Expression: "amount > 10",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusActive,
		Scopes:     []model.Scope{},
		CreatedAt:  now.Add(-24 * time.Hour),
		UpdatedAt:  now.Add(-1 * time.Hour),
	}

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("1500"),
		Currency:             "USD",
		TransactionTimestamp: now,
		Account: model.AccountContext{
			ID: accountID,
		},
	}

	mockCompiledProgram := &cel.CompiledProgram{
		ExpressionHash:   "test-hash",
		SourceExpression: globalRule.Expression,
		CompiledAt:       now,
		CompileTimeMs:    1,
	}

	ctrl := gomock.NewController(t)
	mockEval := NewMockExpressionEvaluator(ctrl)
	// Expect Compile and Evaluate since empty scopes match any account
	mockEval.EXPECT().Compile(gomock.Any(), globalRule.Expression).Return(mockCompiledProgram, nil)
	mockEval.EXPECT().Evaluate(gomock.Any(), mockCompiledProgram, request).Return(true, nil)

	evaluator, err := NewRuleEvaluator(mockEval)
	require.NoError(t, err)

	// Act
	result, err := evaluator.Evaluate(context.Background(), globalRule, request)

	// Assert - global rule should evaluate expression for any account
	require.NoError(t, err)
	assert.True(t, result, "Global rule (empty scopes) should match any account")
}

// TestRuleEvaluator_NilScopesMatchAny verifies that a rule with nil Scopes
// is treated identically to empty scopes (global rule matching any account).
func TestRuleEvaluator_NilScopesMatchAny(t *testing.T) {
	testutil.SetupTestTracing(t)

	ruleID := testutil.MustDeterministicUUID(40)
	accountID := testutil.MustDeterministicUUID(41)
	requestID := testutil.MustDeterministicUUID(42)
	now := testutil.FixedTime()

	// Rule with nil scopes = global rule (same as empty slice)
	globalRule := &model.Rule{
		ID:         ruleID,
		Name:       "Global rule nil scopes",
		Expression: "amount > 10",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusActive,
		Scopes:     nil,
		CreatedAt:  now.Add(-24 * time.Hour),
		UpdatedAt:  now.Add(-1 * time.Hour),
	}

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("1500"),
		Currency:             "USD",
		TransactionTimestamp: now,
		Account: model.AccountContext{
			ID: accountID,
		},
	}

	mockCompiledProgram := &cel.CompiledProgram{
		ExpressionHash:   "test-hash",
		SourceExpression: globalRule.Expression,
		CompiledAt:       now,
		CompileTimeMs:    1,
	}

	ctrl := gomock.NewController(t)
	mockEval := NewMockExpressionEvaluator(ctrl)
	mockEval.EXPECT().Compile(gomock.Any(), globalRule.Expression).Return(mockCompiledProgram, nil)
	mockEval.EXPECT().Evaluate(gomock.Any(), mockCompiledProgram, request).Return(true, nil)

	evaluator, err := NewRuleEvaluator(mockEval)
	require.NoError(t, err)

	// Act
	result, err := evaluator.Evaluate(context.Background(), globalRule, request)

	// Assert - nil scopes should behave identically to empty scopes
	require.NoError(t, err)
	assert.True(t, result, "Global rule (nil scopes) should match any account")
}

// TestRuleEvaluator_MultipleScopesOneMatch verifies that a rule with multiple scopes
// where only one matches the request's account still triggers expression evaluation (OR logic).
func TestRuleEvaluator_MultipleScopesOneMatch(t *testing.T) {
	testutil.SetupTestTracing(t)

	ruleID := testutil.MustDeterministicUUID(20)
	accountID := testutil.MustDeterministicUUID(21)
	otherAccountID1 := testutil.MustDeterministicUUID(22)
	otherAccountID2 := testutil.MustDeterministicUUID(23)
	requestID := testutil.MustDeterministicUUID(24)
	now := testutil.FixedTime()

	// Rule with multiple scopes, only the second matches the request account
	ruleWithScopes := &model.Rule{
		ID:         ruleID,
		Name:       "Multi-scope rule",
		Expression: "amount > 10",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusActive,
		Scopes: []model.Scope{
			{AccountID: &otherAccountID1},
			{AccountID: &accountID}, // This one matches
			{AccountID: &otherAccountID2},
		},
		CreatedAt: now.Add(-24 * time.Hour),
		UpdatedAt: now.Add(-1 * time.Hour),
	}

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("1500"),
		Currency:             "USD",
		TransactionTimestamp: now,
		Account: model.AccountContext{
			ID: accountID,
		},
	}

	mockCompiledProgram := &cel.CompiledProgram{
		ExpressionHash:   "test-hash",
		SourceExpression: ruleWithScopes.Expression,
		CompiledAt:       now,
		CompileTimeMs:    1,
	}

	ctrl := gomock.NewController(t)
	mockEval := NewMockExpressionEvaluator(ctrl)
	// Expect Compile and Evaluate since one scope matches (OR logic)
	mockEval.EXPECT().Compile(gomock.Any(), ruleWithScopes.Expression).Return(mockCompiledProgram, nil)
	mockEval.EXPECT().Evaluate(gomock.Any(), mockCompiledProgram, request).Return(true, nil)

	evaluator, err := NewRuleEvaluator(mockEval)
	require.NoError(t, err)

	// Act
	result, err := evaluator.Evaluate(context.Background(), ruleWithScopes, request)

	// Assert - rule should match because at least one scope matches (OR logic)
	require.NoError(t, err)
	assert.True(t, result, "Rule should match when any scope in the list matches the request account")
}

// TestRuleEvaluator_MultipleScopesNoneMatch verifies that a rule with multiple scopes
// where none matches the request's account short-circuits without compiling/evaluating.
func TestRuleEvaluator_MultipleScopesNoneMatch(t *testing.T) {
	testutil.SetupTestTracing(t)

	ruleID := testutil.MustDeterministicUUID(30)
	accountID := testutil.MustDeterministicUUID(31)
	otherAccountID1 := testutil.MustDeterministicUUID(32)
	otherAccountID2 := testutil.MustDeterministicUUID(33)
	otherAccountID3 := testutil.MustDeterministicUUID(34)
	requestID := testutil.MustDeterministicUUID(35)
	now := testutil.FixedTime()

	// Rule with multiple scopes, none matching the request account
	ruleWithScopes := &model.Rule{
		ID:         ruleID,
		Name:       "Multi-scope rule no match",
		Expression: "amount > 10",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusActive,
		Scopes: []model.Scope{
			{AccountID: &otherAccountID1},
			{AccountID: &otherAccountID2},
			{AccountID: &otherAccountID3},
		},
		CreatedAt: now.Add(-24 * time.Hour),
		UpdatedAt: now.Add(-1 * time.Hour),
	}

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("1500"),
		Currency:             "USD",
		TransactionTimestamp: now,
		Account: model.AccountContext{
			ID: accountID,
		},
	}

	ctrl := gomock.NewController(t)
	mockEval := NewMockExpressionEvaluator(ctrl)
	// No EXPECT() calls: test fails if Compile or Evaluate is invoked

	evaluator, err := NewRuleEvaluator(mockEval)
	require.NoError(t, err)

	// Act
	result, err := evaluator.Evaluate(context.Background(), ruleWithScopes, request)

	// Assert - no scope matches, evaluator should short-circuit
	require.NoError(t, err)
	assert.False(t, result, "Rule should not match when no scope in the list matches the request account")
}

// TestRuleEvaluator_PreCompiledProgram verifies that when a rule has a pre-compiled
// program (set by CacheAdapter), the evaluator uses it directly without calling Compile().
func TestRuleEvaluator_PreCompiledProgram(t *testing.T) {
	testutil.SetupTestTracing(t)

	ruleID := testutil.MustDeterministicUUID(50)
	accountID := testutil.MustDeterministicUUID(51)
	requestID := testutil.MustDeterministicUUID(52)
	now := testutil.FixedTime()

	preCompiled := &cel.CompiledProgram{
		ExpressionHash:   "pre-compiled-hash",
		SourceExpression: "amount > 10",
		CompiledAt:       now,
		CompileTimeMs:    1,
	}

	rule := &model.Rule{
		ID:              ruleID,
		Name:            "Pre-compiled rule",
		Expression:      "amount > 10",
		Action:          model.DecisionDeny,
		Status:          model.RuleStatusActive,
		Scopes:          []model.Scope{},
		CreatedAt:       now.Add(-24 * time.Hour),
		UpdatedAt:       now.Add(-1 * time.Hour),
		CompiledProgram: preCompiled, // Set by CacheAdapter
	}

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("1500"),
		Currency:             "USD",
		TransactionTimestamp: now,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)
	mockEval := NewMockExpressionEvaluator(ctrl)
	// Compile should NOT be called — pre-compiled program is used
	mockEval.EXPECT().Evaluate(gomock.Any(), preCompiled, request).Return(true, nil)

	evaluator, err := NewRuleEvaluator(mockEval)
	require.NoError(t, err)

	result, err := evaluator.Evaluate(context.Background(), rule, request)

	require.NoError(t, err)
	assert.True(t, result, "Should use pre-compiled program and evaluate to true")
}

// TestRuleEvaluator_PreCompiledProgram_WrongType verifies that when CompiledProgram
// is set to a wrong type, the evaluator falls back to Compile() (defense-in-depth).
func TestRuleEvaluator_PreCompiledProgram_WrongType(t *testing.T) {
	testutil.SetupTestTracing(t)

	ruleID := testutil.MustDeterministicUUID(60)
	accountID := testutil.MustDeterministicUUID(61)
	requestID := testutil.MustDeterministicUUID(62)
	now := testutil.FixedTime()

	compiledByFallback := &cel.CompiledProgram{
		ExpressionHash:   "fallback-hash",
		SourceExpression: "amount > 10",
		CompiledAt:       now,
		CompileTimeMs:    1,
	}

	rule := &model.Rule{
		ID:              ruleID,
		Name:            "Wrong type rule",
		Expression:      "amount > 10",
		Action:          model.DecisionDeny,
		Status:          model.RuleStatusActive,
		Scopes:          []model.Scope{},
		CreatedAt:       now.Add(-24 * time.Hour),
		UpdatedAt:       now.Add(-1 * time.Hour),
		CompiledProgram: "not-a-compiled-program", // Wrong type
	}

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("1500"),
		Currency:             "USD",
		TransactionTimestamp: now,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)
	mockEval := NewMockExpressionEvaluator(ctrl)
	// Compile SHOULD be called as fallback since CompiledProgram is wrong type
	mockEval.EXPECT().Compile(gomock.Any(), rule.Expression).Return(compiledByFallback, nil)
	mockEval.EXPECT().Evaluate(gomock.Any(), compiledByFallback, request).Return(true, nil)

	evaluator, err := NewRuleEvaluator(mockEval)
	require.NoError(t, err)

	result, err := evaluator.Evaluate(context.Background(), rule, request)

	require.NoError(t, err)
	assert.True(t, result, "Should fall back to Compile() and evaluate to true")
}

// TestRuleEvaluator_ScopeWithNilAccountID verifies that a scope with nil AccountID
// acts as a wildcard ("match any") per Scope.Matches semantics, triggering expression evaluation.
func TestRuleEvaluator_ScopeWithNilAccountID(t *testing.T) {
	testutil.SetupTestTracing(t)

	ruleID := testutil.MustDeterministicUUID(30)
	accountID := testutil.MustDeterministicUUID(31)
	requestID := testutil.MustDeterministicUUID(32)
	now := testutil.FixedTime()

	// Rule with a scope where AccountID is nil = wildcard for account field
	ruleWithNilScope := &model.Rule{
		ID:         ruleID,
		Name:       "Wildcard scope rule",
		Expression: "amount > 10",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusActive,
		Scopes:     []model.Scope{{AccountID: nil}},
		CreatedAt:  now.Add(-24 * time.Hour),
		UpdatedAt:  now.Add(-1 * time.Hour),
	}

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("1500"),
		Currency:             "USD",
		TransactionTimestamp: now,
		Account: model.AccountContext{
			ID: accountID,
		},
	}

	mockCompiledProgram := &cel.CompiledProgram{
		ExpressionHash:   "test-hash",
		SourceExpression: ruleWithNilScope.Expression,
		CompiledAt:       now,
		CompileTimeMs:    1,
	}

	ctrl := gomock.NewController(t)
	mockEval := NewMockExpressionEvaluator(ctrl)
	// Expect Compile and Evaluate since nil AccountID means "match any"
	mockEval.EXPECT().Compile(gomock.Any(), ruleWithNilScope.Expression).Return(mockCompiledProgram, nil)
	mockEval.EXPECT().Evaluate(gomock.Any(), mockCompiledProgram, request).Return(true, nil)

	evaluator, err := NewRuleEvaluator(mockEval)
	require.NoError(t, err)

	// Act
	result, err := evaluator.Evaluate(context.Background(), ruleWithNilScope, request)

	// Assert - nil AccountID in scope acts as wildcard, expression should be evaluated
	require.NoError(t, err)
	assert.True(t, result, "Scope with nil AccountID should match any account (wildcard)")
}

// TestRuleEvaluator_MissingMetadataKey covers the missing-metadata-key contract:
// rules whose CEL expression references a missing map key must be treated as
// non-match (not fatal), while other runtime errors must still propagate and
// the happy path must keep matching when the referenced key is present.
//
// Regression: previously, an active rule referencing metadata["channel"] with
// "channel" absent caused the whole validation request to return HTTP 500
// ("validation processing failed").
func TestRuleEvaluator_MissingMetadataKey(t *testing.T) {
	missingKeyErr := fmt.Errorf("%w: %w", constant.ErrExpressionEvaluation, errors.New("no such key: channel"))
	runtimeErr := fmt.Errorf("%w: %w", constant.ErrExpressionEvaluation, errors.New("division by zero"))

	tests := []struct {
		name              string
		expression        string
		metadata          map[string]any
		evaluateReturnRes bool
		evaluateReturnErr error
		expectMatched     bool
		expectError       bool
		expectErrContains string
	}{
		{
			name:              "missing metadata key treated as non-match",
			expression:        `metadata["channel"] == "mobile"`,
			metadata:          nil,
			evaluateReturnRes: false,
			evaluateReturnErr: missingKeyErr,
			expectMatched:     false,
			expectError:       false,
		},
		{
			name:              "other runtime error propagates",
			expression:        `amount / 0 > 1`,
			metadata:          nil,
			evaluateReturnRes: false,
			evaluateReturnErr: runtimeErr,
			expectMatched:     false,
			expectError:       true,
			expectErrContains: "failed to evaluate expression",
		},
		{
			name:              "metadata key present and matches",
			expression:        `metadata["caseId"] == "case-001"`,
			metadata:          map[string]any{"caseId": "case-001"},
			evaluateReturnRes: true,
			evaluateReturnErr: nil,
			expectMatched:     true,
			expectError:       false,
		},
		{
			name:              "metadata key present but does not match",
			expression:        `metadata["caseId"] == "case-001"`,
			metadata:          map[string]any{"caseId": "case-999"},
			evaluateReturnRes: false,
			evaluateReturnErr: nil,
			expectMatched:     false,
			expectError:       false,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testutil.SetupTestTracing(t)

			ruleID := testutil.MustDeterministicUUID(int64(200 + i))
			accountID := testutil.MustDeterministicUUID(int64(300 + i))
			requestID := testutil.MustDeterministicUUID(int64(400 + i))
			now := testutil.FixedTime()

			rule := &model.Rule{
				ID:         ruleID,
				Name:       tc.name,
				Expression: tc.expression,
				Action:     model.DecisionDeny,
				Status:     model.RuleStatusActive,
				Scopes:     []model.Scope{},
				CreatedAt:  now.Add(-24 * time.Hour),
				UpdatedAt:  now.Add(-1 * time.Hour),
			}

			request := &model.ValidationRequest{
				RequestID:            requestID,
				TransactionType:      model.TransactionTypeCard,
				Amount:               decimal.RequireFromString("100"),
				Currency:             "USD",
				TransactionTimestamp: now,
				Account:              model.AccountContext{ID: accountID},
				Metadata:             tc.metadata,
			}

			mockCompiledProgram := &cel.CompiledProgram{
				ExpressionHash:   fmt.Sprintf("test-hash-%d", i),
				SourceExpression: rule.Expression,
				CompiledAt:       now,
				CompileTimeMs:    1,
			}

			ctrl := gomock.NewController(t)
			mockEval := NewMockExpressionEvaluator(ctrl)
			mockEval.EXPECT().Compile(gomock.Any(), rule.Expression).Return(mockCompiledProgram, nil)
			mockEval.EXPECT().Evaluate(gomock.Any(), mockCompiledProgram, request).Return(tc.evaluateReturnRes, tc.evaluateReturnErr)

			evaluator, err := NewRuleEvaluator(mockEval)
			require.NoError(t, err)

			result, err := evaluator.Evaluate(context.Background(), rule, request)

			if tc.expectError {
				require.Error(t, err)

				if tc.expectErrContains != "" {
					assert.Contains(t, err.Error(), tc.expectErrContains)
				}
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tc.expectMatched, result)
		})
	}
}
