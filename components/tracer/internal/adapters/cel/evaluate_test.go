// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// compileForEval is a helper to compile an expression for evaluation testing.
func compileForEval(t *testing.T, adapter *Adapter, expression string) *CompiledProgram {
	t.Helper()

	ctx := context.Background()

	program, err := adapter.Compile(ctx, expression)
	require.NoError(t, err, "Compile should not return error for: %s", expression)

	return program
}

// Test UUIDs for evaluator tests
var (
	evalTestAccountID  = uuid.MustParse("550e8400-e29b-41d4-a716-446655440020")
	evalTestMerchantID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440021")
)

// newEvalTestRequest creates a ValidationRequest for evaluation testing.
func newEvalTestRequest() *model.ValidationRequest {
	subType := "instant"

	return &model.ValidationRequest{
		TransactionType: "PIX",
		SubType:         &subType,
		Amount:          decimal.RequireFromString("1500"),
		Currency:        "BRL",
		Account: model.AccountContext{
			ID:     evalTestAccountID,
			Type:   "checking",
			Status: "active",
		},
		Merchant: &model.MerchantContext{
			ID:       evalTestMerchantID,
			Name:     "Test Store",
			Category: "5411",
			Country:  "BR",
		},
		Segment:   &model.SegmentContext{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"), Name: "retail"},
		Portfolio: &model.PortfolioContext{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440002"), Name: "premium"},
		Metadata:  map[string]any{"channel": "mobile", "risk_score": 75},
	}
}

// TestEvaluate_Success tests successful expression evaluation.
func TestEvaluate_Success(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		expected    bool
		description string
	}{
		{
			name:        "Success - amount comparison true",
			expression:  "amount > 1000",
			expected:    true,
			description: "1500 > 1000 should be true",
		},
		{
			name:        "Success - amount comparison false",
			expression:  "amount > 2000",
			expected:    false,
			description: "1500 > 2000 should be false",
		},
		{
			name:        "Success - transaction type check",
			expression:  `transactionType == "PIX"`,
			expected:    true,
			description: "Transaction type is PIX",
		},
		{
			name:        "Success - complex expression",
			expression:  `transactionType == "PIX" && amount > 1000`,
			expected:    true,
			description: "PIX transaction with amount > 1000",
		},
		{
			name:        "Success - currency check",
			expression:  `currency == "BRL"`,
			expected:    true,
			description: "Currency is BRL",
		},
		{
			name:        "Success - account status check",
			expression:  `account["status"] == "active"`,
			expected:    true,
			description: "Account status is active",
		},
		{
			name:        "Success - merchant category check",
			expression:  `merchant["category"] == "5411"`,
			expected:    true,
			description: "Merchant category is 5411",
		},
		{
			name:        "Success - segment check with bracket notation",
			expression:  `segment["name"] == "retail"`,
			expected:    true,
			description: "Segment name is retail (bracket notation)",
		},
		{
			name:        "Success - segment check with dot notation",
			expression:  `segment.segmentId == "550e8400-e29b-41d4-a716-446655440001"`,
			expected:    true,
			description: "Segment ID matches (dot notation)",
		},
		{
			name:        "Success - metadata access",
			expression:  `metadata["channel"] == "mobile"`,
			expected:    true,
			description: "Metadata channel is mobile",
		},
		{
			name:        "Success - decimal amount comparison",
			expression:  "amount > 12.34",
			expected:    true,
			description: "1500 > 12.34 should be true (cross-type numeric comparison)",
		},
		{
			name:        "Success - fractional threshold boundary true",
			expression:  "amount > 1499.99",
			expected:    true,
			description: "1500 > 1499.99 should be true (fractional boundary)",
		},
		{
			name:        "Success - fractional threshold boundary false",
			expression:  "amount > 1500.01",
			expected:    false,
			description: "1500 > 1500.01 should be false (fractional boundary)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			adapter := newTestAdapter(t)
			program := compileForEval(t, adapter, tc.expression)
			req := newEvalTestRequest()

			ctx := context.Background()
			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err, "Evaluate should not return error")
			assert.Equal(t, tc.expected, result, tc.description)
		})
	}
}

// TestEvaluate_WithTracing tests that evaluation creates spans.
func TestEvaluate_WithTracing(t *testing.T) {
	tt := testutil.SetupTestTracing(t)

	ctx := context.Background()

	adapter := newTestAdapter(t)
	program := compileForEval(t, adapter, "amount > 1000")
	req := newEvalTestRequest()

	result, err := adapter.Evaluate(ctx, program, req)

	require.NoError(t, err)
	assert.True(t, result)

	// Verify spans
	spans := tt.GetSpans()
	require.GreaterOrEqual(t, len(spans), 1, "Should have at least one span")

	// Find the evaluate span
	var evalSpan testutil.SpanStub

	for _, s := range spans {
		if s.Name == "adapter.cel.evaluate" {
			evalSpan = s
			break
		}
	}

	require.NotEmpty(t, evalSpan.Name, "Should have adapter.cel.evaluate span")

	// Check attributes - lib-commons v4 flattens struct attributes into dotted keys
	// e.g. "evaluate_result.duration_ms", "evaluate_result.result"
	attrs := attributesToMap(evalSpan.Attributes)
	assert.Contains(t, attrs, "evaluate_result.duration_ms", "Should have evaluate_result.duration_ms attribute")
	assert.Contains(t, attrs, "evaluate_result.result", "Should have evaluate_result.result attribute")
	assert.Equal(t, true, attrs["evaluate_result.result"], "Result should be true")
}

// TestEvaluate_NilFields tests evaluation with nil optional fields.
func TestEvaluate_NilFields(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		req         *model.ValidationRequest
		expected    bool
		expectError bool
		description string
	}{
		{
			name:       "Success - nil merchant",
			expression: "amount > 1000",
			req: &model.ValidationRequest{
				TransactionType: "PIX",
				Amount:          decimal.RequireFromString("1500"),
				Currency:        "BRL",
				Account: model.AccountContext{
					ID:     evalTestAccountID,
					Status: "active",
				},
				Merchant: nil,
			},
			expected:    true,
			expectError: false,
			description: "Should work with nil merchant",
		},
		{
			name:       "Success - nil subType",
			expression: `transactionType == "PIX"`,
			req: &model.ValidationRequest{
				TransactionType: "PIX",
				SubType:         nil,
				Amount:          decimal.RequireFromString("1500"),
				Currency:        "BRL",
			},
			expected:    true,
			expectError: false,
			description: "Should work with nil subType",
		},
		{
			name:       "Success - empty metadata",
			expression: "amount > 1000",
			req: &model.ValidationRequest{
				TransactionType: "PIX",
				Amount:          decimal.RequireFromString("1500"),
				Currency:        "BRL",
				Metadata:        nil,
			},
			expected:    true,
			expectError: false,
			description: "Should work with nil metadata",
		},
		{
			name:       "Success - fractional amount with nil merchant",
			expression: "amount > 10.50",
			req: &model.ValidationRequest{
				TransactionType: "PIX",
				Amount:          decimal.RequireFromString("15.75"),
				Currency:        "BRL",
				Account: model.AccountContext{
					ID:     evalTestAccountID,
					Status: "active",
				},
				Merchant: nil,
			},
			expected:    true,
			expectError: false,
			description: "Should work with fractional amount and nil merchant",
		},
		{
			name:       "Success - cross-type equality with int literal",
			expression: "amount == 1500",
			req: &model.ValidationRequest{
				TransactionType: "PIX",
				Amount:          decimal.RequireFromString("1500"),
				Currency:        "BRL",
				Account: model.AccountContext{
					ID:     evalTestAccountID,
					Status: "active",
				},
			},
			expected:    true,
			expectError: false,
			description: "DynType amount supports equality with int literals",
		},
		{
			name:        "Error - nil request",
			expression:  "amount == 0",
			req:         nil,
			expected:    false,
			expectError: true,
			description: "Nil request should return error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			adapter := newTestAdapter(t)
			program := compileForEval(t, adapter, tc.expression)

			ctx := context.Background()
			result, err := adapter.Evaluate(ctx, program, tc.req)

			if tc.expectError {
				assert.Error(t, err, "Should return error")
				assert.Equal(t, tc.expected, result, tc.description)
			} else {
				require.NoError(t, err, "Evaluate should not return error")
				assert.Equal(t, tc.expected, result, tc.description)
			}
		})
	}
}

// TestEvaluate_NilProgram tests error handling for nil program.
func TestEvaluate_NilProgram(t *testing.T) {
	adapter := newTestAdapter(t)
	req := newEvalTestRequest()

	ctx := context.Background()
	result, err := adapter.Evaluate(ctx, nil, req)

	assert.False(t, result, "Result should be false for nil program")
	assert.Error(t, err, "Should return error for nil program")
	assert.Contains(t, err.Error(), "program", "Error should mention program")
}

// TestEvaluate_NilCompiledProgram tests error handling for nil compiled program.
func TestEvaluate_NilCompiledProgram(t *testing.T) {
	adapter := newTestAdapter(t)

	program := &CompiledProgram{
		ExpressionHash:   "test-hash",
		SourceExpression: "amount > 100",
		Program:          nil, // nil compiled program
		CompiledAt:       time.Now(),
	}

	req := newEvalTestRequest()

	ctx := context.Background()
	result, err := adapter.Evaluate(ctx, program, req)

	assert.False(t, result, "Result should be false for nil compiled program")
	assert.Error(t, err, "Should return error for nil compiled program")
	assert.Contains(t, err.Error(), "nil", "Error should mention nil")
}
