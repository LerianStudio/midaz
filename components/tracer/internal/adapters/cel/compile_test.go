// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TestCompile_Success tests successful compilation of valid CEL expressions.
func TestCompile_Success(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		description string
	}{
		{
			name:        "Success - simple amount comparison",
			expression:  "amount > 100",
			description: "Compiles simple numeric comparison expression",
		},
		{
			name:        "Success - transaction type check",
			expression:  `transactionType == "CARD"`,
			description: "Compiles string equality expression",
		},
		{
			name:        "Success - complex multi-condition expression",
			expression:  `transactionType == "CARD" && amount > 100 && account.status == "active"`,
			description: "Compiles complex expression with multiple conditions",
		},
		{
			name:        "Success - expression with currency check",
			expression:  `currency == "USD" || currency == "BRL"`,
			description: "Compiles expression with OR conditions",
		},
		{
			name:        "Success - expression with metadata access",
			expression:  `"channel" in metadata`,
			description: "Compiles expression accessing metadata map",
		},
		{
			name:        "Success - decimal amount literal",
			expression:  "amount > 12.34",
			description: "Compiles expression with decimal literal for amount comparison",
		},
		{
			name:        "Success - amount equality with integer literal",
			expression:  "amount == 1500",
			description: "DynType amount supports equality with int literals",
		},
		{
			name:        "Success - amount equality with double literal",
			expression:  "amount == 1500.0",
			description: "DynType amount supports equality with double literals",
		},
		{
			name:        "Success - amount inequality with integer literal",
			expression:  "amount != 1000",
			description: "DynType amount supports inequality with int literals",
		},
		{
			name:        "Success - amount inequality with double literal",
			expression:  "amount != 999.99",
			description: "DynType amount supports inequality with double literals",
		},
		{
			name:        "Success - amount greater or equal with integer literal",
			expression:  "amount >= 1500",
			description: "Compiles greater-or-equal with int literal",
		},
		{
			name:        "Success - amount greater or equal with double literal",
			expression:  "amount >= 1499.99",
			description: "Compiles greater-or-equal with double literal",
		},
		{
			name:        "Success - amount less than with integer literal",
			expression:  "amount < 2000",
			description: "Compiles less-than with int literal",
		},
		{
			name:        "Success - amount less than with double literal",
			expression:  "amount < 2000.50",
			description: "Compiles less-than with double literal",
		},
		{
			name:        "Success - amount less or equal with integer literal",
			expression:  "amount <= 1500",
			description: "Compiles less-or-equal with int literal",
		},
		{
			name:        "Success - amount less or equal with double literal",
			expression:  "amount <= 1500.99",
			description: "Compiles less-or-equal with double literal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			adapter := newTestAdapter(t)

			ctx := context.Background()
			result, err := adapter.Compile(ctx, tc.expression)

			require.NoError(t, err, "Compile should not return error for valid expression")
			require.NotNil(t, result, "CompiledProgram should not be nil")

			// Verify CompiledProgram fields
			assert.NotEmpty(t, result.ExpressionHash, "ExpressionHash should be set")
			assert.Equal(t, tc.expression, result.SourceExpression, "SourceExpression should match input")
			assert.NotNil(t, result.Program, "Program should be set")
			assert.False(t, result.CompiledAt.IsZero(), "CompiledAt should be set")
			assert.GreaterOrEqual(t, result.CompileTimeMs, int64(0), "CompileTimeMs should be non-negative")

			// Verify hash is correct
			expectedHash := HashExpression(tc.expression)
			assert.Equal(t, expectedHash, result.ExpressionHash, "ExpressionHash should match SHA-256 of expression")
		})
	}
}

// TestCompile_InvalidExpression tests compilation failures for invalid expressions.
func TestCompile_InvalidExpression(t *testing.T) {
	tests := []struct {
		name           string
		expression     string
		expectedErrMsg string
		description    string
	}{
		{
			name:           "Error - syntax error",
			expression:     "invalid syntax !!!",
			expectedErrMsg: "TRC-0083",
			description:    "Should fail on malformed CEL syntax",
		},
		{
			name:           "Error - undefined variable",
			expression:     "undefinedVariable > 100",
			expectedErrMsg: "undeclared reference",
			description:    "Should fail when using undefined variable",
		},
		{
			name:           "Error - type mismatch",
			expression:     `transactionTimestamp == "string"`,
			expectedErrMsg: "TRC-0084",
			description:    "Should fail on type mismatch (timestamp int vs string)",
		},
		{
			name:           "Error - empty expression",
			expression:     "",
			expectedErrMsg: "expression",
			description:    "Should fail on empty expression",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			adapter := newTestAdapter(t)

			ctx := context.Background()
			result, err := adapter.Compile(ctx, tc.expression)

			assert.Nil(t, result, "CompiledProgram should be nil for invalid expression")
			assert.Error(t, err, "Compile should return error for invalid expression")
			assert.Contains(t, err.Error(), tc.expectedErrMsg,
				"Error message should contain expected substring")
		})
	}
}

// TestCompile_CreatesSpan tests that compilation creates an OpenTelemetry span.
func TestCompile_CreatesSpan(t *testing.T) {
	tests := []struct {
		name             string
		expression       string
		expectedSpanName string
		description      string
	}{
		{
			name:             "Success - span created for compilation",
			expression:       "amount > 100",
			expectedSpanName: "adapter.cel.compile",
			description:      "Compile should create a span with correct name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := testutil.SetupTestTracing(t)
			adapter := newTestAdapter(t)

			ctx := context.Background()

			// Execute compilation
			_, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err, "Compile should not return error")

			// Verify span was created
			spans := tt.GetSpans()
			require.NotEmpty(t, spans, "At least one span should be created")

			// Find the compile span
			var found bool
			for _, s := range spans {
				if s.Name == tc.expectedSpanName {
					found = true
					break
				}
			}

			require.True(t, found, "Span with name %q should exist", tc.expectedSpanName)
		})
	}
}

// TestCompile_SpanAttributes tests that spans have correct attributes.
func TestCompile_SpanAttributes(t *testing.T) {
	tests := []struct {
		name          string
		expression    string
		expectedAttrs map[string]any
		description   string
	}{
		{
			name:       "Success - span has expression_hash attribute",
			expression: "amount > 100",
			expectedAttrs: map[string]any{
				"compile_input.expression_hash":   HashExpression("amount > 100"),
				"compile_input.expression_length": len("amount > 100"),
			},
			description: "Span should have expression_hash and expression_length attributes",
		},
		{
			name:       "Success - long expression has correct length",
			expression: `transactionType == "CARD" && amount > 100 && account.status == "active" && currency == "USD"`,
			expectedAttrs: map[string]any{
				"compile_input.expression_length": len(`transactionType == "CARD" && amount > 100 && account.status == "active" && currency == "USD"`),
			},
			description: "Span should have correct expression_length for longer expressions",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := testutil.SetupTestTracing(t)
			adapter := newTestAdapter(t)

			ctx := context.Background()

			// Execute compilation
			_, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err, "Compile should not return error")

			// Find the compile span
			spans := tt.GetSpans()
			var compileSpan *testutil.SpanStub
			for i := range spans {
				if spans[i].Name == "adapter.cel.compile" {
					compileSpan = &spans[i]
					break
				}
			}

			require.NotNil(t, compileSpan, "Compile span should exist")

			// Verify attributes - lib-commons v4 flattens struct attributes into dotted keys
			// e.g. "compile_input.expression_hash", "compile_input.expression_length"
			attrs := attributesToMap(compileSpan.Attributes)

			for expectedKey, expectedVal := range tc.expectedAttrs {
				assert.Contains(t, attrs, expectedKey, "Should have %s attribute", expectedKey)

				actualVal, exists := attrs[expectedKey]
				if !exists {
					continue
				}

				switch ev := expectedVal.(type) {
				case int:
					assert.Equal(t, int64(ev), actualVal,
						"%s value should match", expectedKey)
				case string:
					assert.Equal(t, ev, actualVal,
						"%s value should match", expectedKey)
				default:
					assert.Equal(t, expectedVal, actualVal,
						"%s value should match", expectedKey)
				}
			}
		})
	}
}

// TestCompile_SpanErrorOnFailure tests that span records error on compilation failure.
func TestCompile_SpanErrorOnFailure(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		description string
	}{
		{
			name:        "Error - span records compilation error",
			expression:  "invalid syntax !!!",
			description: "Span should record error status when compilation fails",
		},
		{
			name:        "Error - span records undefined variable error",
			expression:  "unknownVariable > 100",
			description: "Span should record error status for undefined variable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := testutil.SetupTestTracing(t)
			adapter := newTestAdapter(t)

			ctx := context.Background()

			// Execute compilation (expecting error)
			_, err := adapter.Compile(ctx, tc.expression)
			assert.Error(t, err, "Compile should return error for invalid expression")

			// Find the compile span
			spans := tt.GetSpans()
			var compileSpan *testutil.SpanStub
			for i := range spans {
				if spans[i].Name == "adapter.cel.compile" {
					compileSpan = &spans[i]
					break
				}
			}

			require.NotNil(t, compileSpan, "Compile span should exist")

			// Verify span kind is internal (default for adapter operations)
			assert.Equal(t, trace.SpanKindInternal, compileSpan.SpanKind, "Span kind should be internal")

			// Check for error event or error status
			hasError := compileSpan.Status.Code != 0 || len(compileSpan.Events) > 0
			assert.True(t, hasError, "Span should have error status or error event recorded")
		})
	}
}

// TestCompile_TypeValidation tests that non-boolean expressions are rejected.
func TestCompile_TypeValidation(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		expectError bool
		description string
	}{
		{
			name:        "Success - boolean expression accepted",
			expression:  "amount > 100",
			expectError: false,
			description: "Expression returning bool should compile",
		},
		{
			name:        "Success - complex boolean expression accepted",
			expression:  `transactionType == "CARD" && amount > 100`,
			expectError: false,
			description: "Complex boolean expression should compile",
		},
		{
			name:        "Error - string expression rejected",
			expression:  "transactionType",
			expectError: true,
			description: "Expression returning string should fail",
		},
		{
			name:        "Error - dyn expression rejected",
			expression:  "amount",
			expectError: true,
			description: "Expression returning dyn should fail",
		},
		{
			name:        "Error - arithmetic expression rejected",
			expression:  "amount + 100",
			expectError: true,
			description: "Arithmetic expression should fail (returns dyn)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			adapter := newTestAdapter(t)

			ctx := context.Background()
			result, err := adapter.Compile(ctx, tc.expression)

			if tc.expectError {
				assert.Nil(t, result, "CompiledProgram should be nil for non-boolean expression")
				assert.Error(t, err, "Compile should return error for non-boolean expression")
				assert.Contains(t, err.Error(), "TRC-0084", "Error should be ErrExpressionType")
			} else {
				require.NoError(t, err, "Compile should not return error for boolean expression")
				assert.NotNil(t, result, "CompiledProgram should not be nil")
			}
		})
	}
}

// TestAdapter_CostLimitParameter tests that cost limit is properly set via parameter.
func TestAdapter_CostLimitParameter(t *testing.T) {
	tests := []struct {
		name          string
		costLimit     uint64
		expectedLimit uint64
		description   string
	}{
		{
			name:          "Success - custom cost limit",
			costLimit:     5000,
			expectedLimit: 5000,
			description:   "Cost limit should be set from parameter",
		},
		{
			name:          "Success - zero uses default",
			costLimit:     0,
			expectedLimit: DefaultCostLimit,
			description:   "Zero cost limit should use default",
		},
		{
			name:          "Success - large cost limit",
			costLimit:     100000,
			expectedLimit: 100000,
			description:   "Large cost limit should be supported",
		},
		{
			name:          "Success - default cost limit constant",
			costLimit:     DefaultCostLimit,
			expectedLimit: DefaultCostLimit,
			description:   "DefaultCostLimit constant should work",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := testutil.NewMockLogger()
			cfg := AdapterConfig{CostLimit: tc.costLimit}

			adapter, err := NewAdapter(cfg, logger)
			require.NoError(t, err, "NewAdapter should not return error")

			assert.Equal(t, tc.expectedLimit, adapter.costLimit, "Cost limit should match expected value")
		})
	}
}

// TestAdapter_CostLimitApplied tests that cost limit is applied to compiled programs.
func TestAdapter_CostLimitApplied(t *testing.T) {
	logger := testutil.NewMockLogger()
	cfg := AdapterConfig{CostLimit: 5000}

	adapter, err := NewAdapter(cfg, logger)
	require.NoError(t, err, "NewAdapter should not return error")

	// Verify the adapter has the correct cost limit
	assert.Equal(t, uint64(5000), adapter.costLimit, "Adapter should have cost limit from parameter")

	// Compile a simple expression - should succeed
	ctx := context.Background()
	result, err := adapter.Compile(ctx, "amount > 100")

	require.NoError(t, err, "Compile should not return error for simple expression")
	assert.NotNil(t, result, "CompiledProgram should not be nil")
}

// TestCompile_CostValidation tests that expressions exceeding cost limit are rejected at compile time.
func TestCompile_CostValidation(t *testing.T) {
	tests := []struct {
		name        string
		costLimit   uint64
		expression  string
		expectError bool
		description string
	}{
		{
			name:        "Success - simple expression within limit",
			costLimit:   1000,
			expression:  "amount > 100",
			expectError: false,
			description: "Simple expression should be within cost limit",
		},
		{
			name:        "Success - moderate expression within limit",
			costLimit:   5000,
			expression:  `transactionType == "CARD" && amount > 100 && currency == "USD"`,
			expectError: false,
			description: "Moderate expression should be within cost limit",
		},
		{
			name:        "Error - expression exceeds very low cost limit",
			costLimit:   1,
			expression:  `transactionType in ["CARD", "PIX", "WIRE"] && amount > 10`,
			expectError: true,
			description: "Expression should exceed very low cost limit. " +
				"Note: costLimit=1 relies on CEL assigning cost > 1 to this expression. " +
				"If CEL's cost estimator changes, this test may need adjustment.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := testutil.NewMockLogger()
			cfg := AdapterConfig{CostLimit: tc.costLimit}

			adapter, err := NewAdapter(cfg, logger)
			require.NoError(t, err, "NewAdapter should not return error")

			ctx := context.Background()
			result, err := adapter.Compile(ctx, tc.expression)

			if tc.expectError {
				assert.Nil(t, result, "CompiledProgram should be nil when cost exceeds limit")
				assert.Error(t, err, "Compile should return error when cost exceeds limit")
				assert.Contains(t, err.Error(), "TRC-0085", "Error should be ErrExpressionCostExceeded")
			} else {
				require.NoError(t, err, "Compile should not return error when within cost limit")
				assert.NotNil(t, result, "CompiledProgram should not be nil")
			}
		})
	}
}

// TestCompile_NoRuntimeCostLimit tests that expressions passing compile-time cost
// validation do NOT fail at runtime due to cost limits.
// The compile-time checker.Cost() validation is the only cost gate.
func TestCompile_NoRuntimeCostLimit(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		request     *model.ValidationRequest
		expected    bool
		description string
	}{
		{
			name:       "Success - simple expression executes without runtime cost error",
			expression: "amount > 100",
			request: &model.ValidationRequest{
				TransactionType: "CARD",
				Amount:          decimal.RequireFromString("500"),
				Currency:        "USD",
				Account:         model.AccountContext{Type: "checking", Status: "active"},
			},
			expected:    true,
			description: "Expression that passes compile-time cost check should never fail at runtime due to cost",
		},
		{
			name:       "Success - moderate expression executes without runtime cost error",
			expression: `transactionType == "CARD" && amount > 100 && currency == "USD"`,
			request: &model.ValidationRequest{
				TransactionType: "CARD",
				Amount:          decimal.RequireFromString("500"),
				Currency:        "USD",
				Account:         model.AccountContext{Type: "checking", Status: "active"},
			},
			expected:    true,
			description: "Multi-condition expression should execute without runtime cost error",
		},
		{
			name:       "Success - expression with list membership check",
			expression: `transactionType in ["CARD", "PIX", "WIRE", "ACH", "SEPA"]`,
			request: &model.ValidationRequest{
				TransactionType: "CARD",
				Amount:          decimal.RequireFromString("500"),
				Currency:        "USD",
				Account:         model.AccountContext{Type: "checking", Status: "active"},
			},
			expected:    true,
			description: "List membership check should execute without runtime cost error",
		},
		{
			name:       "Success - nested property access expression",
			expression: `account.status == "active" && account.type == "checking"`,
			request: &model.ValidationRequest{
				TransactionType: "CARD",
				Amount:          decimal.RequireFromString("500"),
				Currency:        "USD",
				Account:         model.AccountContext{Type: "checking", Status: "active"},
			},
			expected:    true,
			description: "Nested property access should execute without runtime cost error",
		},
		{
			name:       "Success - expression evaluates to false without runtime cost error",
			expression: "amount > 1000",
			request: &model.ValidationRequest{
				TransactionType: "CARD",
				Amount:          decimal.RequireFromString("500"),
				Currency:        "USD",
				Account:         model.AccountContext{Type: "checking", Status: "active"},
			},
			expected:    false,
			description: "Expression evaluating to false should complete without runtime cost error",
		},
		{
			name:       "Success - complex expression evaluates to false without runtime cost error",
			expression: `transactionType == "PIX" && amount > 100`,
			request: &model.ValidationRequest{
				TransactionType: "CARD",
				Amount:          decimal.RequireFromString("500"),
				Currency:        "USD",
				Account:         model.AccountContext{Type: "checking", Status: "active"},
			},
			expected:    false,
			description: "Complex expression evaluating to false should complete without runtime cost error",
		},
		{
			name:       "Success - fractional amount comparison without runtime cost error",
			expression: "amount > 499.99",
			request: &model.ValidationRequest{
				TransactionType: "CARD",
				Amount:          decimal.RequireFromString("500.50"),
				Currency:        "USD",
				Account:         model.AccountContext{Type: "checking", Status: "active"},
			},
			expected:    true,
			description: "Fractional amount comparison should execute without runtime cost error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			adapter := newTestAdapter(t)

			ctx := context.Background()

			// Compile - should pass cost validation
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err, "Compile should succeed for expression within cost limit")
			require.NotNil(t, program, "CompiledProgram should not be nil")

			// Evaluate - should NEVER fail due to runtime cost limit
			// Any cost-related rejection should happen at compile time only
			result, err := adapter.Evaluate(ctx, program, tc.request)

			// The key assertion: no runtime cost-related error
			require.NoError(t, err, "Evaluate should NOT fail with runtime cost error - "+
				"cost validation happens only at compile time")
			assert.Equal(t, tc.expected, result, tc.description)
		})
	}
}

// TestCompile_CostValidationIsCompileTimeOnly tests that cost validation occurs
// only at compile time via checker.Cost(), not at runtime.
func TestCompile_CostValidationIsCompileTimeOnly(t *testing.T) {
	t.Run("Cost exceeded error only at compile time", func(t *testing.T) {
		logger := testutil.NewMockLogger()
		// Use a very low cost limit to trigger compile-time rejection
		cfg := AdapterConfig{CostLimit: 1}

		adapter, err := NewAdapter(cfg, logger)
		require.NoError(t, err, "NewAdapter should not return error")

		ctx := context.Background()

		// This expression should exceed the very low cost limit at COMPILE time
		expr := `transactionType in ["CARD", "PIX", "WIRE"] && amount > 10`
		_, err = adapter.Compile(ctx, expr)

		// Verify: cost exceeded error happens at COMPILE time, not runtime
		require.Error(t, err, "Compile should reject expression exceeding cost limit")
		assert.Contains(t, err.Error(), "TRC-0085",
			"Error should be ErrExpressionCostExceeded at compile time")
	})

	t.Run("No runtime cost limit after passing compile-time check", func(t *testing.T) {
		logger := testutil.NewMockLogger()
		// Use default cost limit (high enough for normal expressions)
		cfg := AdapterConfig{CostLimit: DefaultCostLimit}

		adapter, err := NewAdapter(cfg, logger)
		require.NoError(t, err, "NewAdapter should not return error")

		ctx := context.Background()

		// Compile an expression
		program, err := adapter.Compile(ctx, "amount > 100")
		require.NoError(t, err, "Compile should succeed")

		// Create request
		req := &model.ValidationRequest{
			TransactionType: "CARD",
			Amount:          decimal.RequireFromString("500"),
			Currency:        "USD",
		}

		// Evaluate multiple times - should never hit runtime cost limit
		for i := 0; i < 100; i++ {
			result, err := adapter.Evaluate(ctx, program, req)
			require.NoError(t, err, "Evaluate should NEVER fail due to runtime cost (iteration %d)", i)
			assert.True(t, result, "Result should be true")
		}
	})
}

// attributesToMap converts span attributes to a map for easier testing.
func attributesToMap(attrs []attribute.KeyValue) map[string]interface{} {
	result := make(map[string]interface{})
	for _, attr := range attrs {
		key := string(attr.Key)
		switch attr.Value.Type() {
		case attribute.STRING:
			result[key] = attr.Value.AsString()
		case attribute.INT64:
			result[key] = attr.Value.AsInt64()
		case attribute.FLOAT64:
			result[key] = attr.Value.AsFloat64()
		case attribute.BOOL:
			result[key] = attr.Value.AsBool()
		}
	}
	return result
}
