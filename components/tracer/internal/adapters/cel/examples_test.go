// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/pkg/model"
)

// compileExampleExpression is a helper to compile an expression for testing.
func compileExampleExpression(t *testing.T, expression string) *CompiledProgram {
	t.Helper()

	adapter := newTestAdapter(t)

	ctx := context.Background()

	program, err := adapter.Compile(ctx, expression)
	require.NoError(t, err, "Compile should not return error for: %s", expression)

	return program
}

// Test UUIDs for example tests
var (
	exampleTestAccountID  = uuid.MustParse("550e8400-e29b-41d4-a716-446655440030")
	exampleTestMerchantID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440031")
)

// newExampleRequest creates a ValidationRequest with all fields populated for example tests.
// Uses 2 scopes to test multiple scope scenarios.
func newExampleRequest() *model.ValidationRequest {
	subType := "instant"

	return &model.ValidationRequest{
		TransactionType: "PIX",
		SubType:         &subType,
		Amount:          decimal.RequireFromString("1500"),
		Currency:        "BRL",
		Account: model.AccountContext{
			ID:     exampleTestAccountID,
			Type:   "checking",
			Status: "active",
		},
		Merchant: &model.MerchantContext{
			ID:       exampleTestMerchantID,
			Name:     "Test Store",
			Category: "5411",
			Country:  "BR",
		},
		Segment:   &model.SegmentContext{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"), Name: "retail"},
		Portfolio: &model.PortfolioContext{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440002"), Name: "premium"},
		Metadata:  map[string]any{"channel": "mobile", "risk_score": 75},
	}
}

// TestAllExpressions_Compile tests that all example expressions compile successfully.
func TestAllExpressions_Compile(t *testing.T) {
	expressions := AllExpressions()

	for _, expr := range expressions {
		t.Run(expr.Name, func(t *testing.T) {
			program := compileExampleExpression(t, expr.Expression)
			assert.NotNil(t, program, "Expression should compile: %s", expr.Description)
		})
	}
}

// TestAmountExpressions tests amount-based expressions.
func TestAmountExpressions(t *testing.T) {
	adapter := newTestAdapter(t)

	tests := []struct {
		name       string
		expression string
		amount     string
		expected   bool
	}{
		{
			name:       "high_value_true",
			expression: "amount > 1000",
			amount:     "1500",
			expected:   true,
		},
		{
			name:       "high_value_false",
			expression: "amount > 1000",
			amount:     "500",
			expected:   false,
		},
		{
			name:       "low_value_true",
			expression: "amount <= 100",
			amount:     "50",
			expected:   true,
		},
		{
			name:       "range_true",
			expression: "amount >= 500 && amount <= 2000",
			amount:     "1000",
			expected:   true,
		},
		{
			name:       "range_false_below",
			expression: "amount >= 500 && amount <= 2000",
			amount:     "300",
			expected:   false,
		},
		{
			name:       "decimal_threshold_true",
			expression: "amount > 12.34",
			amount:     "15.50",
			expected:   true,
		},
		{
			name:       "decimal_threshold_false",
			expression: "amount > 12.34",
			amount:     "10.00",
			expected:   false,
		},
		{
			name:       "exact_decimal_match_true",
			expression: "amount == 99.99",
			amount:     "99.99",
			expected:   true,
		},
		{
			name:       "decimal_range_true",
			expression: "amount >= 1000.50 && amount <= 5000.75",
			amount:     "2500.25",
			expected:   true,
		},
		{
			name:       "boundary_exact_1000",
			expression: "amount > 1000",
			amount:     "1000",
			expected:   false,
		},
		{
			name:       "zero_amount",
			expression: "amount >= 0",
			amount:     "0",
			expected:   true,
		},
		{
			name:       "negative_amount",
			expression: "amount > 0",
			amount:     "-10",
			expected:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newExampleRequest()
			req.Amount = decimal.RequireFromString(tc.amount)

			program := compileExampleExpression(t, tc.expression)
			result, err := adapter.Evaluate(context.Background(), program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestTransactionTypeExpressions tests transaction type expressions.
func TestTransactionTypeExpressions(t *testing.T) {
	adapter := newTestAdapter(t)

	tests := []struct {
		name            string
		expression      string
		transactionType model.TransactionType
		expected        bool
	}{
		{
			name:            "is_pix_true",
			expression:      `transactionType == "PIX"`,
			transactionType: "PIX",
			expected:        true,
		},
		{
			name:            "is_pix_false",
			expression:      `transactionType == "PIX"`,
			transactionType: "WIRE",
			expected:        false,
		},
		{
			name:            "is_wire_true",
			expression:      `transactionType == "WIRE"`,
			transactionType: "WIRE",
			expected:        true,
		},
		{
			name:            "high_risk_types_wire",
			expression:      `transactionType in ["WIRE", "CRYPTO"]`,
			transactionType: "WIRE",
			expected:        true,
		},
		{
			name:            "high_risk_types_pix",
			expression:      `transactionType in ["WIRE", "CRYPTO"]`,
			transactionType: "PIX",
			expected:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newExampleRequest()
			req.TransactionType = tc.transactionType

			program := compileExampleExpression(t, tc.expression)
			result, err := adapter.Evaluate(context.Background(), program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAccountExpressions tests account context expressions.
func TestAccountExpressions(t *testing.T) {
	adapter := newTestAdapter(t)

	tests := []struct {
		name       string
		expression string
		status     string
		accType    string
		expected   bool
	}{
		{
			name:       "active_true",
			expression: `account["status"] == "active"`,
			status:     "active",
			accType:    "checking",
			expected:   true,
		},
		{
			name:       "active_false",
			expression: `account["status"] == "active"`,
			status:     "suspended",
			accType:    "checking",
			expected:   false,
		},
		{
			name:       "checking_true",
			expression: `account["type"] == "checking"`,
			status:     "active",
			accType:    "checking",
			expected:   true,
		},
		{
			name:       "checking_false",
			expression: `account["type"] == "checking"`,
			status:     "active",
			accType:    "savings",
			expected:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newExampleRequest()
			req.Account.Status = tc.status
			req.Account.Type = tc.accType

			program := compileExampleExpression(t, tc.expression)
			result, err := adapter.Evaluate(context.Background(), program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestMerchantExpressions tests merchant context expressions.
func TestMerchantExpressions(t *testing.T) {
	adapter := newTestAdapter(t)

	tests := []struct {
		name       string
		expression string
		category   string
		country    string
		expected   bool
	}{
		{
			name:       "gambling_mcc_true",
			expression: `merchant["category"] == "7995"`,
			category:   "7995",
			country:    "BR",
			expected:   true,
		},
		{
			name:       "gambling_mcc_false",
			expression: `merchant["category"] == "7995"`,
			category:   "5411",
			country:    "BR",
			expected:   false,
		},
		{
			name:       "foreign_true",
			expression: `merchant["country"] != "BR"`,
			category:   "5411",
			country:    "US",
			expected:   true,
		},
		{
			name:       "foreign_false",
			expression: `merchant["country"] != "BR"`,
			category:   "5411",
			country:    "BR",
			expected:   false,
		},
		{
			name:       "high_risk_mcc_true",
			expression: `merchant["category"] in ["5912", "5993", "7995"]`,
			category:   "7995",
			country:    "BR",
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newExampleRequest()
			req.Merchant.Category = tc.category
			req.Merchant.Country = tc.country

			program := compileExampleExpression(t, tc.expression)
			result, err := adapter.Evaluate(context.Background(), program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestSegmentPortfolioExpressions tests segment and portfolio-based expressions.
func TestSegmentPortfolioExpressions(t *testing.T) {
	adapter := newTestAdapter(t)

	tests := []struct {
		name       string
		expression string
		segment    *model.SegmentContext
		portfolio  *model.PortfolioContext
		expected   bool
	}{
		{
			name:       "has_segment_true",
			expression: `size(segment) > 0`,
			segment:    &model.SegmentContext{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"), Name: "retail"},
			portfolio:  nil,
			expected:   true,
		},
		{
			name:       "has_segment_false",
			expression: `size(segment) > 0`,
			segment:    nil,
			portfolio:  nil,
			expected:   false,
		},
		{
			name:       "has_portfolio_true",
			expression: `size(portfolio) > 0`,
			segment:    nil,
			portfolio:  &model.PortfolioContext{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440002"), Name: "premium"},
			expected:   true,
		},
		{
			name:       "has_portfolio_false",
			expression: `size(portfolio) > 0`,
			segment:    nil,
			portfolio:  nil,
			expected:   false,
		},
		{
			name:       "segment_name_check",
			expression: `segment["name"] == "retail"`,
			segment:    &model.SegmentContext{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"), Name: "retail"},
			portfolio:  nil,
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newExampleRequest()
			req.Segment = tc.segment
			req.Portfolio = tc.portfolio

			program := compileExampleExpression(t, tc.expression)
			result, err := adapter.Evaluate(context.Background(), program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestMetadataExpressions tests metadata-based expressions.
func TestMetadataExpressions(t *testing.T) {
	adapter := newTestAdapter(t)

	tests := []struct {
		name       string
		expression string
		metadata   map[string]any
		expected   bool
	}{
		{
			name:       "has_risk_score_true",
			expression: `"risk_score" in metadata`,
			metadata:   map[string]any{"risk_score": 75},
			expected:   true,
		},
		{
			name:       "has_risk_score_false",
			expression: `"risk_score" in metadata`,
			metadata:   map[string]any{"channel": "mobile"},
			expected:   false,
		},
		{
			name:       "mobile_channel_true",
			expression: `metadata["channel"] == "mobile"`,
			metadata:   map[string]any{"channel": "mobile"},
			expected:   true,
		},
		{
			name:       "mobile_channel_false",
			expression: `metadata["channel"] == "mobile"`,
			metadata:   map[string]any{"channel": "web"},
			expected:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newExampleRequest()
			req.Metadata = tc.metadata

			program := compileExampleExpression(t, tc.expression)
			result, err := adapter.Evaluate(context.Background(), program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestCombinedExpressions tests complex combined expressions.
func TestCombinedExpressions(t *testing.T) {
	adapter := newTestAdapter(t)

	tests := []struct {
		name       string
		expression string
		modify     func(*model.ValidationRequest)
		expected   bool
	}{
		{
			name:       "high_value_active_true",
			expression: `amount > 1000 && account["status"] == "active"`,
			modify: func(req *model.ValidationRequest) {
				req.Amount = decimal.RequireFromString("1500")
				req.Account.Status = "active"
			},
			expected: true,
		},
		{
			name:       "high_value_active_false_amount",
			expression: `amount > 1000 && account["status"] == "active"`,
			modify: func(req *model.ValidationRequest) {
				req.Amount = decimal.RequireFromString("500")
				req.Account.Status = "active"
			},
			expected: false,
		},
		{
			name:       "high_value_active_false_status",
			expression: `amount > 1000 && account["status"] == "active"`,
			modify: func(req *model.ValidationRequest) {
				req.Amount = decimal.RequireFromString("1500")
				req.Account.Status = "suspended"
			},
			expected: false,
		},
		{
			name:       "full_validation_true",
			expression: `transactionType == "PIX" && amount > 100 && account["status"] == "active" && currency == "BRL"`,
			modify: func(req *model.ValidationRequest) {
				req.TransactionType = "PIX"
				req.Amount = decimal.RequireFromString("500")
				req.Account.Status = "active"
				req.Currency = "BRL"
			},
			expected: true,
		},
		{
			name:       "full_validation_false_currency",
			expression: `transactionType == "PIX" && amount > 100 && account["status"] == "active" && currency == "BRL"`,
			modify: func(req *model.ValidationRequest) {
				req.TransactionType = "PIX"
				req.Amount = decimal.RequireFromString("500")
				req.Account.Status = "active"
				req.Currency = "USD"
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newExampleRequest()
			tc.modify(req)

			program := compileExampleExpression(t, tc.expression)
			result, err := adapter.Evaluate(context.Background(), program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestExpressions_WithNilMerchant tests that expressions work when merchant is nil.
func TestExpressions_WithNilMerchant(t *testing.T) {
	adapter := newTestAdapter(t)

	// Expression that doesn't use merchant should work
	req := newExampleRequest()
	req.Merchant = nil

	program := compileExampleExpression(t, "amount > 1000")
	result, err := adapter.Evaluate(context.Background(), program, req)

	require.NoError(t, err)
	assert.True(t, result)
}
