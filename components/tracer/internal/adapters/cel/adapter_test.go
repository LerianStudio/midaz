// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewAdapter_Success tests successful adapter creation.
func TestNewAdapter_Success(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()
	cfg := AdapterConfig{
		CostLimit: 5000,
	}

	adapter, err := NewAdapter(cfg, logger)

	require.NoError(t, err, "NewAdapter should not return error")
	assert.NotNil(t, adapter, "Adapter should not be nil")
	assert.Equal(t, uint64(5000), adapter.costLimit)
}

// TestNewAdapter_DefaultValues tests that zero values use defaults.
func TestNewAdapter_DefaultValues(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()
	cfg := AdapterConfig{} // Zero values

	adapter, err := NewAdapter(cfg, logger)

	require.NoError(t, err)
	assert.Equal(t, DefaultCostLimit, adapter.costLimit)
}

// TestNewAdapter_NilLogger tests error for nil logger.
func TestNewAdapter_NilLogger(t *testing.T) {
	t.Parallel()

	cfg := AdapterConfig{}

	adapter, err := NewAdapter(cfg, nil)

	assert.Nil(t, adapter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "logger")
}

// TestAdapter_Compile_Success tests successful compilation.
func TestAdapter_Compile_Success(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t)
	ctx := context.Background()

	program, err := adapter.Compile(ctx, "amount > 1000")

	require.NoError(t, err)
	assert.NotNil(t, program)
	assert.NotEmpty(t, program.ExpressionHash)
	assert.Equal(t, "amount > 1000", program.SourceExpression)
	assert.NotNil(t, program.Program)
}

// TestAdapter_Compile_SyntaxError tests syntax error handling.
func TestAdapter_Compile_SyntaxError(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t)
	ctx := context.Background()

	program, err := adapter.Compile(ctx, "invalid syntax !!!")

	assert.Nil(t, program)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrExpressionSyntax.Error())
}

// TestAdapter_Compile_EmptyExpression tests empty expression error.
func TestAdapter_Compile_EmptyExpression(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t)
	ctx := context.Background()

	program, err := adapter.Compile(ctx, "")

	assert.Nil(t, program)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrExpressionSyntax.Error())
}

// TestAdapter_Compile_TypeError tests non-boolean expression error.
func TestAdapter_Compile_TypeError(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t)
	ctx := context.Background()

	// Expression that returns string, not bool
	program, err := adapter.Compile(ctx, "transactionType")

	assert.Nil(t, program)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrExpressionType.Error())
}

// TestAdapter_Evaluate_Success tests successful evaluation.
func TestAdapter_Evaluate_Success(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t)
	ctx := context.Background()

	program, err := adapter.Compile(ctx, "amount > 1000")
	require.NoError(t, err)

	req := newTestRequest()
	result, err := adapter.Evaluate(ctx, program, req)

	require.NoError(t, err)
	assert.True(t, result, "1500 > 1000 should be true")
}

// TestAdapter_Evaluate_False tests evaluation returning false.
func TestAdapter_Evaluate_False(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t)
	ctx := context.Background()

	program, err := adapter.Compile(ctx, "amount > 2000")
	require.NoError(t, err)

	req := newTestRequest()
	result, err := adapter.Evaluate(ctx, program, req)

	require.NoError(t, err)
	assert.False(t, result, "1500 > 2000 should be false")
}

// TestAdapter_Evaluate_ComplexExpression tests complex expression evaluation.
func TestAdapter_Evaluate_ComplexExpression(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t)
	ctx := context.Background()

	program, err := adapter.Compile(ctx, `transactionType == "PIX" && amount > 1000 && account["status"] == "active"`)
	require.NoError(t, err)

	req := newTestRequest()
	result, err := adapter.Evaluate(ctx, program, req)

	require.NoError(t, err)
	assert.True(t, result)
}

// TestAdapter_Evaluate_NilProgram tests error for nil program.
func TestAdapter_Evaluate_NilProgram(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t)
	ctx := context.Background()

	result, err := adapter.Evaluate(ctx, nil, newTestRequest())

	assert.False(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "program")
}

// TestAdapter_Evaluate_NilRequest tests error for nil request.
func TestAdapter_Evaluate_NilRequest(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t)
	ctx := context.Background()

	program, err := adapter.Compile(ctx, "amount > 1000")
	require.NoError(t, err)

	result, err := adapter.Evaluate(ctx, program, nil)

	assert.False(t, result)
	assert.Error(t, err)
}

// TestAdapter_TracingSpans tests that spans are created.
func TestAdapter_TracingSpans(t *testing.T) {
	t.Parallel()

	tt := testutil.SetupTestTracing(t)

	ctx := context.Background()

	// Create adapter and perform operations
	adapter := newTestAdapter(t)

	program, err := adapter.Compile(ctx, "amount > 1000")
	require.NoError(t, err)

	_, err = adapter.Evaluate(ctx, program, newTestRequest())
	require.NoError(t, err)

	// Verify spans
	spans := tt.GetSpans()

	var compileSpan, evalSpan bool

	for _, s := range spans {
		if s.Name == "adapter.cel.compile" {
			compileSpan = true
		}

		if s.Name == "adapter.cel.evaluate" {
			evalSpan = true
		}
	}

	assert.True(t, compileSpan, "Should have adapter.cel.compile span")
	assert.True(t, evalSpan, "Should have adapter.cel.evaluate span")
}

// TestAdapter_Evaluate_FractionalAmount tests evaluation with fractional decimal amounts.
func TestAdapter_Evaluate_FractionalAmount(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t)
	ctx := context.Background()

	program, err := adapter.Compile(ctx, "amount > 12.34")
	require.NoError(t, err)

	req := newTestRequest()
	req.Amount = decimal.RequireFromString("15.50")
	result, err := adapter.Evaluate(ctx, program, req)

	require.NoError(t, err)
	assert.True(t, result, "15.50 > 12.34 should be true")
}

// TestAdapter_Evaluate_AmountEquality tests equality operators with int and double literals.
// DynType allows cross-type equality (amount == intLiteral) that DoubleType did not support.
func TestAdapter_Evaluate_AmountEquality(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		expression string
		amount     decimal.Decimal
		expected   bool
	}{
		{
			name:       "equals int literal - true",
			expression: "amount == 1500",
			amount:     decimal.RequireFromString("1500"),
			expected:   true,
		},
		{
			name:       "equals int literal - false",
			expression: "amount == 1500",
			amount:     decimal.RequireFromString("999.99"),
			expected:   false,
		},
		{
			name:       "equals double literal - true",
			expression: "amount == 1500.0",
			amount:     decimal.RequireFromString("1500"),
			expected:   true,
		},
		{
			name:       "not equals int literal - true",
			expression: "amount != 1000",
			amount:     decimal.RequireFromString("1500"),
			expected:   true,
		},
		{
			name:       "not equals int literal - false",
			expression: "amount != 1500",
			amount:     decimal.RequireFromString("1500"),
			expected:   false,
		},
		{
			name:       "equals fractional double - true",
			expression: "amount == 99.99",
			amount:     decimal.RequireFromString("99.99"),
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			adapter := newTestAdapter(t)
			ctx := context.Background()

			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := newTestRequest()
			req.Amount = tc.amount
			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAdapter_ImplementsInterface tests that Adapter implements ExpressionEngine.
func TestAdapter_ImplementsInterface(t *testing.T) {
	t.Parallel()

	var _ ExpressionEngine = (*Adapter)(nil)
}
