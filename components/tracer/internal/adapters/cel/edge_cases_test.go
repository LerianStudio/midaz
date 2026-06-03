// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test UUIDs for edge case tests
var (
	edgeTestAccountID  = uuid.MustParse("550e8400-e29b-41d4-a716-446655440010")
	edgeTestMerchantID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440011")
)

// TestEdgeCase_NilMerchant tests evaluation when merchant is nil.
func TestEdgeCase_NilMerchant(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "amount check with nil merchant",
			expression: "amount > 1000",
			expected:   true,
		},
		{
			name:       "transaction type with nil merchant",
			expression: `transactionType == "PIX"`,
			expected:   true,
		},
		{
			name:       "account status with nil merchant",
			expression: `account["status"] == "active"`,
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := &model.ValidationRequest{
				TransactionType: "PIX",
				Amount:          decimal.RequireFromString("1500"),
				Currency:        "BRL",
				Account: model.AccountContext{
					ID:     edgeTestAccountID,
					Status: "active",
				},
				Merchant: nil, // nil merchant
			}

			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestEdgeCase_NilSegmentPortfolio tests evaluation with nil segment and portfolio.
func TestEdgeCase_NilSegmentPortfolio(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "segment is empty when nil",
			expression: "size(segment) == 0",
			expected:   true,
		},
		{
			name:       "portfolio is empty when nil",
			expression: "size(portfolio) == 0",
			expected:   true,
		},
		{
			name:       "amount check ignoring segment/portfolio",
			expression: "amount > 1000",
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := &model.ValidationRequest{
				TransactionType: "PIX",
				Amount:          decimal.RequireFromString("1500"),
				Currency:        "BRL",
				Account: model.AccountContext{
					ID:     edgeTestAccountID,
					Status: "active",
				},
				Segment:   nil, // no segment
				Portfolio: nil, // no portfolio
			}

			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestEdgeCase_EmptyMetadata tests evaluation with empty/nil metadata.
func TestEdgeCase_EmptyMetadata(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		metadata   map[string]any
		expected   bool
	}{
		{
			name:       "nil metadata - amount check",
			expression: "amount > 1000",
			metadata:   nil,
			expected:   true,
		},
		{
			name:       "empty metadata - amount check",
			expression: "amount > 1000",
			metadata:   map[string]any{},
			expected:   true,
		},
		{
			name:       "nil metadata - key not in check",
			expression: `!("channel" in metadata)`,
			metadata:   nil,
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := &model.ValidationRequest{
				TransactionType: "PIX",
				Amount:          decimal.RequireFromString("1500"),
				Currency:        "BRL",
				Account: model.AccountContext{
					ID:     edgeTestAccountID,
					Status: "active",
				},
				Metadata: tc.metadata,
			}

			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestEdgeCase_NilSubType tests evaluation when subType is nil.
func TestEdgeCase_NilSubType(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	program, err := adapter.Compile(ctx, "amount > 1000")
	require.NoError(t, err)

	req := &model.ValidationRequest{
		TransactionType: "PIX",
		SubType:         nil, // nil subType
		Amount:          decimal.RequireFromString("1500"),
		Currency:        "BRL",
		Account: model.AccountContext{
			ID:     edgeTestAccountID,
			Status: "active",
		},
	}

	result, err := adapter.Evaluate(ctx, program, req)

	require.NoError(t, err)
	assert.True(t, result)
}

// TestEdgeCase_MinimalRequest tests evaluation with minimal required fields.
func TestEdgeCase_MinimalRequest(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	program, err := adapter.Compile(ctx, "amount > 1000")
	require.NoError(t, err)

	// Minimal request - only required fields
	req := &model.ValidationRequest{
		TransactionType: "PIX",
		Amount:          decimal.RequireFromString("1500"),
		Currency:        "BRL",
		Account: model.AccountContext{
			ID:     edgeTestAccountID,
			Status: "active",
		},
		// All optional fields are nil/empty
		SubType:   nil,
		Merchant:  nil,
		Segment:   nil,
		Portfolio: nil,
		Metadata:  nil,
	}

	result, err := adapter.Evaluate(ctx, program, req)

	require.NoError(t, err)
	assert.True(t, result)
}

// TestEdgeCase_ZeroAmount tests evaluation with zero amount.
func TestEdgeCase_ZeroAmount(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "zero amount equals zero (int literal)",
			expression: "amount == 0",
			expected:   true,
		},
		{
			name:       "zero amount equals zero (double literal)",
			expression: "amount == 0.0",
			expected:   true,
		},
		{
			name:       "zero amount not greater than zero",
			expression: "amount > 0",
			expected:   false,
		},
		{
			name:       "zero amount greater or equal",
			expression: "amount >= 0",
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := &model.ValidationRequest{
				TransactionType: "PIX",
				Amount:          decimal.RequireFromString("0"), // zero amount
				Currency:        "BRL",
				Account: model.AccountContext{
					ID:     edgeTestAccountID,
					Status: "active",
				},
			}

			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestEdgeCase_EmptyStrings tests evaluation with empty string fields.
func TestEdgeCase_EmptyStrings(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "empty currency check",
			expression: `currency == ""`,
			expected:   true,
		},
		{
			name:       "empty account status",
			expression: `account["status"] == ""`,
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := &model.ValidationRequest{
				TransactionType: "PIX",
				Amount:          decimal.RequireFromString("1500"),
				Currency:        "", // empty string
				Account: model.AccountContext{
					ID:     edgeTestAccountID,
					Status: "", // empty string
				},
			}

			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestEdgeCase_NilAccountMetadata tests account with nil metadata.
func TestEdgeCase_NilAccountMetadata(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	program, err := adapter.Compile(ctx, `account["status"] == "active"`)
	require.NoError(t, err)

	req := &model.ValidationRequest{
		TransactionType: "PIX",
		Amount:          decimal.RequireFromString("1500"),
		Currency:        "BRL",
		Account: model.AccountContext{
			ID:       edgeTestAccountID,
			Status:   "active",
			Metadata: nil, // nil account metadata
		},
	}

	result, err := adapter.Evaluate(ctx, program, req)

	require.NoError(t, err)
	assert.True(t, result)
}

// TestEdgeCase_MerchantWithNilMetadata tests merchant with nil metadata.
func TestEdgeCase_MerchantWithNilMetadata(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	program, err := adapter.Compile(ctx, `merchant["category"] == "5411"`)
	require.NoError(t, err)

	req := &model.ValidationRequest{
		TransactionType: "PIX",
		Amount:          decimal.RequireFromString("1500"),
		Currency:        "BRL",
		Account: model.AccountContext{
			ID:     edgeTestAccountID,
			Status: "active",
		},
		Merchant: &model.MerchantContext{
			ID:       edgeTestMerchantID,
			Name:     "Test Store",
			Category: "5411",
			Country:  "BR",
			Metadata: nil, // nil merchant metadata
		},
	}

	result, err := adapter.Evaluate(ctx, program, req)

	require.NoError(t, err)
	assert.True(t, result)
}

// TestEdgeCase_FractionalAmount tests evaluation with fractional (decimal) amounts.
func TestEdgeCase_FractionalAmount(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		amount     decimal.Decimal
		expected   bool
	}{
		{
			name:       "fractional greater than - true",
			expression: "amount > 10.50",
			amount:     decimal.RequireFromString("10.75"),
			expected:   true,
		},
		{
			name:       "fractional exact match",
			expression: "amount == 99.99",
			amount:     decimal.RequireFromString("99.99"),
			expected:   true,
		},
		{
			name:       "fractional less than small value",
			expression: "amount < 0.01",
			amount:     decimal.RequireFromString("0.005"),
			expected:   true,
		},
		{
			name:       "half unit amount",
			expression: "amount == 0.5",
			amount:     decimal.RequireFromString("0.5"),
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := &model.ValidationRequest{
				TransactionType: "PIX",
				Amount:          tc.amount,
				Currency:        "BRL",
				Account: model.AccountContext{
					ID:     edgeTestAccountID,
					Status: "active",
				},
			}

			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestEdgeCase_NegativeAmount tests evaluation with negative amount.
func TestEdgeCase_NegativeAmount(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "negative amount less than zero",
			expression: "amount < 0",
			expected:   true,
		},
		{
			name:       "negative amount equals negative value (int literal)",
			expression: "amount == -500",
			expected:   true,
		},
		{
			name:       "negative amount equals negative value (double literal)",
			expression: "amount == -500.0",
			expected:   true,
		},
		{
			name:       "negative fractional amount less than zero",
			expression: "amount < -0.01",
			expected:   true,
		},
		{
			name:       "negative fractional amount greater than negative threshold",
			expression: "amount > -500.01",
			expected:   true,
		},
	}

	// Also test with a fractional negative amount
	t.Run("negative fractional amount equality", func(t *testing.T) {
		program, err := adapter.Compile(ctx, "amount == -499.99")
		require.NoError(t, err)

		req := &model.ValidationRequest{
			TransactionType: "REFUND",
			Amount:          decimal.RequireFromString("-499.99"),
			Currency:        "BRL",
			Account: model.AccountContext{
				ID:     edgeTestAccountID,
				Status: "active",
			},
		}

		result, err := adapter.Evaluate(ctx, program, req)

		require.NoError(t, err)
		assert.True(t, result, "-499.99 == -499.99 should be true")
	})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := &model.ValidationRequest{
				TransactionType: "REFUND",
				Amount:          decimal.RequireFromString("-500"), // negative amount (refund)
				Currency:        "BRL",
				Account: model.AccountContext{
					ID:     edgeTestAccountID,
					Status: "active",
				},
			}

			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
