// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"testing"
	"time"

	"tracer/pkg/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test UUIDs for advanced edge case tests
var (
	advTestAccountID  = uuid.MustParse("550e8400-e29b-41d4-a716-446655440030")
	advTestMerchantID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440031")
	advTestSegmentID  = uuid.MustParse("550e8400-e29b-41d4-a716-446655440032")
	advTestRequestID  = uuid.MustParse("550e8400-e29b-41d4-a716-446655440034")
	advTestTimestamp  = time.Date(2030, 6, 15, 10, 30, 0, 0, time.UTC)
)

// newAdvancedTestRequest creates a full ValidationRequest for advanced testing.
func newAdvancedTestRequest() *model.ValidationRequest {
	subType := "instant"

	return &model.ValidationRequest{
		RequestID:            advTestRequestID,
		TransactionTimestamp: advTestTimestamp,
		TransactionType:      "PIX",
		SubType:              &subType,
		Amount:               decimal.RequireFromString("1500"),
		Currency:             "BRL",
		Account: model.AccountContext{
			ID:     advTestAccountID,
			Type:   "checking",
			Status: "active",
		},
		Merchant: &model.MerchantContext{
			ID:       advTestMerchantID,
			Name:     "Test Store",
			Category: "5411",
			Country:  "BR",
		},
		Segment:   &model.SegmentContext{ID: advTestSegmentID, Name: "retail"},
		Portfolio: &model.PortfolioContext{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440033"), Name: "premium"},
		Metadata:  map[string]any{"channel": "mobile", "risk_score": 75, "tags": []any{"vip", "high-value"}},
	}
}

// TestAdvancedEdgeCase_InOperator tests the 'in' operator for list membership.
func TestAdvancedEdgeCase_InOperator(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "transaction type in list - match",
			expression: `transactionType in ["PIX", "TED", "DOC"]`,
			expected:   true,
		},
		{
			name:       "transaction type in list - no match",
			expression: `transactionType in ["CARD", "WIRE"]`,
			expected:   false,
		},
		{
			name:       "currency in list - match",
			expression: `currency in ["BRL", "USD", "EUR"]`,
			expected:   true,
		},
		{
			name:       "merchant category in list - match",
			expression: `merchant["category"] in ["5411", "5412", "5499"]`,
			expected:   true,
		},
		{
			name:       "merchant category in list - no match",
			expression: `merchant["category"] in ["7995", "5912"]`,
			expected:   false,
		},
		{
			name:       "key in metadata map - exists",
			expression: `"channel" in metadata`,
			expected:   true,
		},
		{
			name:       "key in metadata map - not exists",
			expression: `"nonexistent" in metadata`,
			expected:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := newAdvancedTestRequest()
			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAdvancedEdgeCase_SizeFunction tests the size() function.
func TestAdvancedEdgeCase_SizeFunction(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "segment size greater than 0",
			expression: "size(segment) > 0",
			expected:   true,
		},
		{
			name:       "portfolio size greater than 0",
			expression: "size(portfolio) > 0",
			expected:   true,
		},
		{
			name:       "metadata size check",
			expression: "size(metadata) >= 2",
			expected:   true,
		},
		{
			name:       "account size check",
			expression: "size(account) > 0",
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := newAdvancedTestRequest()
			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAdvancedEdgeCase_OptionalFieldChecks tests checking for optional fields using size().
// Note: In this CEL environment, optional fields (merchant, segment, portfolio) are always
// present as maps but empty when nil in the source request. Use size() > 0 to check presence.
func TestAdvancedEdgeCase_OptionalFieldChecks(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		modifyReq  func(*model.ValidationRequest)
		expected   bool
	}{
		{
			name:       "merchant present - size > 0",
			expression: "size(merchant) > 0",
			modifyReq:  nil,
			expected:   true,
		},
		{
			name:       "merchant absent - size == 0",
			expression: "size(merchant) == 0",
			modifyReq:  func(r *model.ValidationRequest) { r.Merchant = nil },
			expected:   true,
		},
		{
			name:       "segment present - size > 0",
			expression: "size(segment) > 0",
			modifyReq:  nil,
			expected:   true,
		},
		{
			name:       "segment absent - size == 0",
			expression: "size(segment) == 0",
			modifyReq:  func(r *model.ValidationRequest) { r.Segment = nil },
			expected:   true,
		},
		{
			name:       "portfolio present - size > 0",
			expression: "size(portfolio) > 0",
			modifyReq:  nil,
			expected:   true,
		},
		{
			name:       "portfolio absent - size == 0",
			expression: "size(portfolio) == 0",
			modifyReq:  func(r *model.ValidationRequest) { r.Portfolio = nil },
			expected:   true,
		},
		{
			name:       "safe merchant access with size check",
			expression: `size(merchant) > 0 && merchant["category"] == "5411"`,
			modifyReq:  nil,
			expected:   true,
		},
		{
			name:       "safe merchant access - no merchant",
			expression: `size(merchant) > 0 && merchant["category"] == "5411"`,
			modifyReq:  func(r *model.ValidationRequest) { r.Merchant = nil },
			expected:   false,
		},
		{
			name:       "check key exists in merchant",
			expression: `size(merchant) > 0 && "category" in merchant`,
			modifyReq:  nil,
			expected:   true,
		},
		{
			name:       "check key not exists in merchant",
			expression: `size(merchant) > 0 && "nonexistent" in merchant`,
			modifyReq:  nil,
			expected:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := newAdvancedTestRequest()
			if tc.modifyReq != nil {
				tc.modifyReq(req)
			}

			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAdvancedEdgeCase_LogicalOperators tests AND, OR, NOT operators.
func TestAdvancedEdgeCase_LogicalOperators(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "AND - both true",
			expression: `transactionType == "PIX" && amount > 1000`,
			expected:   true,
		},
		{
			name:       "AND - first false",
			expression: `transactionType == "CARD" && amount > 1000`,
			expected:   false,
		},
		{
			name:       "AND - second false",
			expression: `transactionType == "PIX" && amount > 2000`,
			expected:   false,
		},
		{
			name:       "OR - first true",
			expression: `transactionType == "PIX" || transactionType == "TED"`,
			expected:   true,
		},
		{
			name:       "OR - second true",
			expression: `transactionType == "CARD" || transactionType == "PIX"`,
			expected:   true,
		},
		{
			name:       "OR - both false",
			expression: `transactionType == "CARD" || transactionType == "TED"`,
			expected:   false,
		},
		{
			name:       "NOT - negate true",
			expression: `!(transactionType == "CARD")`,
			expected:   true,
		},
		{
			name:       "NOT - negate false",
			expression: `!(transactionType == "PIX")`,
			expected:   false,
		},
		{
			name:       "complex - AND OR combined",
			expression: `(transactionType == "PIX" || transactionType == "TED") && amount > 1000`,
			expected:   true,
		},
		{
			name:       "complex - nested NOT",
			expression: `!(transactionType == "CARD" && amount < 500)`,
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := newAdvancedTestRequest()
			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAdvancedEdgeCase_ComparisonOperators tests all comparison operators.
func TestAdvancedEdgeCase_ComparisonOperators(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		// Amount is 1500
		{
			name:       "equal - true (int literal)",
			expression: "amount == 1500",
			expected:   true,
		},
		{
			name:       "equal - true (double literal)",
			expression: "amount == 1500.0",
			expected:   true,
		},
		{
			name:       "equal - false",
			expression: "amount == 1000",
			expected:   false,
		},
		{
			name:       "not equal - true (int literal)",
			expression: "amount != 1000",
			expected:   true,
		},
		{
			name:       "not equal - true (double literal)",
			expression: "amount != 1000.0",
			expected:   true,
		},
		{
			name:       "not equal - false",
			expression: "amount != 1500",
			expected:   false,
		},
		{
			name:       "greater than - true",
			expression: "amount > 1000",
			expected:   true,
		},
		{
			name:       "greater than - false (equal)",
			expression: "amount > 1500",
			expected:   false,
		},
		{
			name:       "greater than or equal - true (greater)",
			expression: "amount >= 1000",
			expected:   true,
		},
		{
			name:       "greater than or equal - true (equal)",
			expression: "amount >= 1500",
			expected:   true,
		},
		{
			name:       "greater than or equal - false",
			expression: "amount >= 2000",
			expected:   false,
		},
		{
			name:       "less than - true",
			expression: "amount < 2000",
			expected:   true,
		},
		{
			name:       "less than - false (equal)",
			expression: "amount < 1500",
			expected:   false,
		},
		{
			name:       "less than or equal - true (less)",
			expression: "amount <= 2000",
			expected:   true,
		},
		{
			name:       "less than or equal - true (equal)",
			expression: "amount <= 1500",
			expected:   true,
		},
		{
			name:       "less than or equal - false",
			expression: "amount <= 1000",
			expected:   false,
		},
		// Fractional comparisons (amount is 1500)
		{
			name:       "fractional equal - false",
			expression: "amount == 1500.50",
			expected:   false,
		},
		{
			name:       "fractional greater than - true",
			expression: "amount > 1499.99",
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := newAdvancedTestRequest()
			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAdvancedEdgeCase_TernaryConditional tests ternary operator (? :).
func TestAdvancedEdgeCase_TernaryConditional(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "ternary - condition true",
			expression: `(amount > 1000 ? true : false)`,
			expected:   true,
		},
		{
			name:       "ternary - condition false",
			expression: `(amount > 2000 ? true : false)`,
			expected:   false,
		},
		{
			name:       "ternary - nested check",
			expression: `(transactionType == "PIX" ? amount > 1000 : amount > 500)`,
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := newAdvancedTestRequest()
			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAdvancedEdgeCase_AmountBoundaries tests amount boundary conditions.
func TestAdvancedEdgeCase_AmountBoundaries(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		amount     decimal.Decimal
		expected   bool
		expectErr  bool
	}{
		{
			name:       "max int64 amount",
			expression: "amount > 0",
			amount:     decimal.RequireFromString("9223372036854775807"), // max int64, exceeds float64 safe precision
			expectErr:  true,
		},
		{
			name:       "max safe float64 amount",
			expression: "amount > 0",
			amount:     decimal.RequireFromString("9007199254740992"), // 2^53, at the safe limit
			expected:   true,
		},
		{
			name:       "large amount comparison",
			expression: "amount > 999999999999",
			amount:     decimal.RequireFromString("1000000000000"), // 1 trillion
			expected:   true,
		},
		{
			name:       "amount range check - in range",
			expression: "amount >= 1000 && amount <= 5000",
			amount:     decimal.RequireFromString("2500"),
			expected:   true,
		},
		{
			name:       "amount range check - below range",
			expression: "amount >= 1000 && amount <= 5000",
			amount:     decimal.RequireFromString("500"),
			expected:   false,
		},
		{
			name:       "amount range check - above range",
			expression: "amount >= 1000 && amount <= 5000",
			amount:     decimal.RequireFromString("6000"),
			expected:   false,
		},
		{
			name:       "amount at lower boundary",
			expression: "amount >= 1000 && amount <= 5000",
			amount:     decimal.RequireFromString("1000"),
			expected:   true,
		},
		{
			name:       "amount at upper boundary",
			expression: "amount >= 1000 && amount <= 5000",
			amount:     decimal.RequireFromString("5000"),
			expected:   true,
		},
		{
			name:       "fractional range - in range",
			expression: "amount >= 100.50 && amount <= 500.75",
			amount:     decimal.RequireFromString("250.25"),
			expected:   true,
		},
		{
			name:       "fractional range - at lower boundary",
			expression: "amount >= 100.50 && amount <= 500.75",
			amount:     decimal.RequireFromString("100.50"),
			expected:   true,
		},
		{
			name:       "fractional range - at upper boundary",
			expression: "amount >= 100.50 && amount <= 500.75",
			amount:     decimal.RequireFromString("500.75"),
			expected:   true,
		},
		{
			name:       "fractional range - below lower boundary",
			expression: "amount >= 100.50 && amount <= 500.75",
			amount:     decimal.RequireFromString("100.49"),
			expected:   false,
		},
		{
			name:       "fractional range - above upper boundary",
			expression: "amount >= 100.50 && amount <= 500.75",
			amount:     decimal.RequireFromString("500.76"),
			expected:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := &model.ValidationRequest{
				RequestID:            advTestRequestID,
				TransactionTimestamp: advTestTimestamp,
				TransactionType:      "PIX",
				Amount:               tc.amount,
				Currency:             "BRL",
				Account: model.AccountContext{
					ID:     advTestAccountID,
					Status: "active",
				},
			}

			result, err := adapter.Evaluate(ctx, program, req)

			if tc.expectErr {
				require.Error(t, err, "Expected error for amount %s", tc.amount)
				assert.Contains(t, err.Error(), "exceeds safe precision")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

// TestAdvancedEdgeCase_MetadataNumericValues tests metadata with numeric values.
func TestAdvancedEdgeCase_MetadataNumericValues(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "metadata numeric comparison - greater",
			expression: `"risk_score" in metadata && metadata["risk_score"] > 50`,
			expected:   true, // risk_score is 75
		},
		{
			name:       "metadata numeric comparison - less",
			expression: `"risk_score" in metadata && metadata["risk_score"] < 50`,
			expected:   false,
		},
		{
			name:       "metadata numeric comparison - equal",
			expression: `"risk_score" in metadata && metadata["risk_score"] == 75`,
			expected:   true,
		},
		{
			name:       "metadata numeric range",
			expression: `"risk_score" in metadata && metadata["risk_score"] >= 70 && metadata["risk_score"] <= 80`,
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := newAdvancedTestRequest()
			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAdvancedEdgeCase_StringComparisons tests string comparison operations.
func TestAdvancedEdgeCase_StringComparisons(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "string equality",
			expression: `transactionType == "PIX"`,
			expected:   true,
		},
		{
			name:       "string inequality",
			expression: `transactionType != "CARD"`,
			expected:   true,
		},
		{
			name:       "string lexicographic - less",
			expression: `transactionType < "TED"`, // PIX < TED lexicographically
			expected:   true,
		},
		{
			name:       "string lexicographic - greater",
			expression: `transactionType > "DOC"`, // PIX > DOC lexicographically
			expected:   true,
		},
		{
			name:       "empty string check - not empty",
			expression: `currency != ""`,
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := newAdvancedTestRequest()
			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAdvancedEdgeCase_NestedMapAccess tests deeply nested map access patterns.
func TestAdvancedEdgeCase_NestedMapAccess(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "account ID via bracket notation",
			expression: `account["accountId"] == "550e8400-e29b-41d4-a716-446655440030"`,
			expected:   true,
		},
		{
			name:       "account type via bracket notation",
			expression: `account["type"] == "checking"`,
			expected:   true,
		},
		{
			name:       "merchant name via bracket notation",
			expression: `merchant["name"] == "Test Store"`,
			expected:   true,
		},
		{
			name:       "merchant country via bracket notation",
			expression: `merchant["country"] == "BR"`,
			expected:   true,
		},
		{
			name:       "segment name via dot notation",
			expression: `segment.name == "retail"`,
			expected:   true,
		},
		{
			name:       "portfolio name via dot notation",
			expression: `portfolio.name == "premium"`,
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := newAdvancedTestRequest()
			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAdvancedEdgeCase_ComplexBusinessRules tests complex real-world business rule expressions.
func TestAdvancedEdgeCase_ComplexBusinessRules(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name        string
		expression  string
		modifyReq   func(*model.ValidationRequest)
		expected    bool
		description string
	}{
		{
			name:        "high value PIX from active account",
			expression:  `transactionType == "PIX" && amount > 1000 && account["status"] == "active"`,
			modifyReq:   nil,
			expected:    true,
			description: "Standard high-value PIX transaction",
		},
		{
			name:        "gambling merchant high risk",
			expression:  `size(merchant) > 0 && merchant["category"] in ["7995", "7994"] && amount > 500`,
			modifyReq:   func(r *model.ValidationRequest) { r.Merchant.Category = "7995" },
			expected:    true,
			description: "Gambling merchant with high amount",
		},
		{
			name:        "international transfer check",
			expression:  `size(merchant) > 0 && merchant["country"] != "BR" && amount > 100`,
			modifyReq:   func(r *model.ValidationRequest) { r.Merchant.Country = "US" },
			expected:    true,
			description: "International merchant with significant amount",
		},
		{
			name:        "VIP customer with high risk score",
			expression:  `"risk_score" in metadata && metadata["risk_score"] > 70 && segment.name == "retail"`,
			modifyReq:   nil,
			expected:    true,
			description: "High risk score in retail segment",
		},
		{
			name:        "mobile channel large transaction",
			expression:  `"channel" in metadata && metadata["channel"] == "mobile" && amount > 1000`,
			modifyReq:   nil,
			expected:    true,
			description: "Large mobile transaction",
		},
		{
			name:        "premium portfolio grocery",
			expression:  `portfolio.name == "premium" && size(merchant) > 0 && merchant["category"] == "5411"`,
			modifyReq:   nil,
			expected:    true,
			description: "Premium customer at grocery store",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := newAdvancedTestRequest()
			if tc.modifyReq != nil {
				tc.modifyReq(req)
			}

			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result, tc.description)
		})
	}
}

// TestAdvancedEdgeCase_ShortCircuitEvaluation tests that CEL short-circuits correctly.
func TestAdvancedEdgeCase_ShortCircuitEvaluation(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		modifyReq  func(*model.ValidationRequest)
		expected   bool
	}{
		{
			name:       "AND short-circuit - first false prevents second eval",
			expression: `size(merchant) > 0 && merchant["category"] == "5411"`,
			modifyReq:  func(r *model.ValidationRequest) { r.Merchant = nil },
			expected:   false,
		},
		{
			name:       "OR short-circuit - first true skips second",
			expression: `amount > 1000 || merchant["category"] == "9999"`,
			modifyReq:  nil,
			expected:   true,
		},
		{
			name:       "safe navigation with size - prevents empty map access issue",
			expression: `size(merchant) == 0 || merchant["category"] == "5411"`,
			modifyReq:  func(r *model.ValidationRequest) { r.Merchant = nil },
			expected:   true, // size(merchant) == 0 is true, so whole expression is true
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := newAdvancedTestRequest()
			if tc.modifyReq != nil {
				tc.modifyReq(req)
			}

			result, err := adapter.Evaluate(ctx, program, req)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
