// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewEnvironment tests that CEL environment is created with all required variables.
func TestNewEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		expectErr   bool
		expectEnv   bool
		description string
	}{
		{
			name:        "Success - creates CEL environment with all required variables",
			expectErr:   false,
			expectEnv:   true,
			description: "Environment should be created with transactionType, subType, amount, currency, account, merchant, segment, portfolio, metadata, transactionTimestamp variables",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env, err := NewEnvironment()

			if tc.expectErr {
				assert.Error(t, err, "Expected error but got none")
				assert.Nil(t, env, "Environment should be nil when error occurs")
			} else {
				require.NoError(t, err, "NewEnvironment should not return error")
				assert.NotNil(t, env, "Environment should not be nil")
			}
		})
	}
}

// TestNewEnvironment_CompileValidExpression tests that valid CEL expressions compile successfully.
func TestNewEnvironment_CompileValidExpression(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		expectErr   bool
		description string
	}{
		{
			name:        "Success - compile expression with transactionType",
			expression:  `transactionType == "CARD"`,
			expectErr:   false,
			description: "Expression using transactionType string variable should compile",
		},
		{
			name:        "Success - compile expression with amount comparison",
			expression:  `amount > 100`,
			expectErr:   false,
			description: "Expression using amount double variable should compile with cross-type numeric comparison",
		},
		{
			name:        "Success - compile expression with decimal amount literal",
			expression:  `amount > 12.34`,
			expectErr:   false,
			description: "Expression using amount with decimal literal should compile",
		},
		{
			name:        "Success - compile expression with currency check",
			expression:  `currency == "USD"`,
			expectErr:   false,
			description: "Expression using currency string variable should compile",
		},
		{
			name:        "Success - compile expression with account map access",
			expression:  `account.status == "active"`,
			expectErr:   false,
			description: "Expression using account map variable should compile",
		},
		{
			name:        "Success - compile expression with merchant map access",
			expression:  `merchant.category == "5411"`,
			expectErr:   false,
			description: "Expression using merchant map variable should compile",
		},
		{
			name:        "Success - compile expression with segment map access",
			expression:  `size(segment) > 0`,
			expectErr:   false,
			description: "Expression using segment map variable should compile",
		},
		{
			name:        "Success - compile expression with portfolio map access",
			expression:  `size(portfolio) > 0`,
			expectErr:   false,
			description: "Expression using portfolio map variable should compile",
		},
		{
			name:        "Success - compile expression with metadata map",
			expression:  `"key" in metadata`,
			expectErr:   false,
			description: "Expression using metadata map variable should compile",
		},
		{
			name:        "Success - compile expression with transactionTimestamp",
			expression:  `transactionTimestamp > 0`,
			expectErr:   false,
			description: "Expression using transactionTimestamp int variable should compile",
		},
		{
			name:        "Success - compile expression with optional subType",
			expression:  `subType == "debit"`,
			expectErr:   false,
			description: "Expression using optional subType string variable should compile",
		},
		{
			name:        "Success - compile complex expression",
			expression:  `transactionType == "CARD" && amount > 100 && account.status == "active"`,
			expectErr:   false,
			description: "Complex expression using multiple variables should compile",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env, err := NewEnvironment()
			require.NoError(t, err, "NewEnvironment should not return error")

			_, err = env.Compile(tc.expression)

			if tc.expectErr {
				assert.Error(t, err, "Expected compilation error for expression: %s", tc.expression)
			} else {
				assert.NoError(t, err, "Expected successful compilation for expression: %s", tc.expression)
			}
		})
	}
}

// TestNewEnvironment_RejectInvalidVariable tests that expressions with unknown variables are rejected.
func TestNewEnvironment_RejectInvalidVariable(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		expectErr   bool
		description string
	}{
		{
			name:        "Error - reject expression with unknown variable",
			expression:  `unknownVariable == "test"`,
			expectErr:   true,
			description: "Expression with undefined variable should fail compilation",
		},
		{
			name:        "Error - reject expression with typo in variable name",
			expression:  `transactiontype == "CARD"`,
			expectErr:   true,
			description: "Expression with typo in variable name should fail compilation",
		},
		{
			name:        "Success - compile expression with dynamic account field access",
			expression:  `account.invalidField == "test"`,
			expectErr:   false,
			description: "CEL dynamic maps allow any field access at compile time; field validation occurs at runtime",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env, err := NewEnvironment()
			require.NoError(t, err, "NewEnvironment should not return error")

			_, err = env.Compile(tc.expression)

			if tc.expectErr {
				assert.Error(t, err, "Expected compilation error for invalid expression: %s", tc.expression)
			} else {
				assert.NoError(t, err, "Unexpected compilation error for expression: %s", tc.expression)
			}
		})
	}
}

// Test UUIDs for environment tests
var (
	envTestAccountID1   = uuid.MustParse("550e8400-e29b-41d4-a716-446655440040")
	envTestAccountID2   = uuid.MustParse("550e8400-e29b-41d4-a716-446655440041")
	envTestAccountID3   = uuid.MustParse("550e8400-e29b-41d4-a716-446655440042")
	envTestAccountID4   = uuid.MustParse("550e8400-e29b-41d4-a716-446655440043")
	envTestMerchantID1  = uuid.MustParse("550e8400-e29b-41d4-a716-446655440044")
	envTestSegmentID1   = uuid.MustParse("550e8400-e29b-41d4-a716-446655440045")
	envTestPortfolioID1 = uuid.MustParse("550e8400-e29b-41d4-a716-446655440046")
)

// TestBuildActivation_FullRequest tests building activation map from a complete ValidationRequest.
func TestBuildActivation_FullRequest(t *testing.T) {
	subType := "debit"
	merchantName := "Test Merchant"
	merchantCategory := "5411"
	merchantCountry := "US"

	tests := []struct {
		name               string
		request            *model.ValidationRequest
		expectedTransType  string
		expectedSubType    string
		expectedAmount     float64
		expectedCurrency   string
		expectedAccountID  string
		expectedMerchantID string
		expectMerchantNil  bool
		expectSegment      bool
		expectPortfolio    bool
		description        string
	}{
		{
			name: "Success - build activation from full request",
			request: &model.ValidationRequest{
				RequestID:            uuid.New(),
				TransactionType:      model.TransactionTypeCard,
				SubType:              &subType,
				Amount:               decimal.RequireFromString("100.75"),
				Currency:             "USD",
				TransactionTimestamp: time.Now(),
				Account: model.AccountContext{
					ID:     envTestAccountID1,
					Type:   "checking",
					Status: "active",
					Metadata: map[string]any{
						"tier": "premium",
					},
				},
				Merchant: &model.MerchantContext{
					ID:       envTestMerchantID1,
					Name:     merchantName,
					Category: merchantCategory,
					Country:  merchantCountry,
					Metadata: map[string]any{
						"risk": "low",
					},
				},
				Segment:   &model.SegmentContext{ID: envTestSegmentID1, Name: "retail"},
				Portfolio: &model.PortfolioContext{ID: envTestPortfolioID1, Name: "premium"},
				Metadata: map[string]any{
					"channel": "mobile",
				},
			},
			expectedTransType:  "CARD",
			expectedSubType:    "debit",
			expectedAmount:     float64(100.75),
			expectedCurrency:   "USD",
			expectedAccountID:  envTestAccountID1.String(),
			expectedMerchantID: envTestMerchantID1.String(),
			expectMerchantNil:  false,
			expectSegment:      true,
			expectPortfolio:    true,
			description:        "Activation should contain all fields from complete request",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewEnvironment()
			require.NoError(t, err, "NewEnvironment should not return error")

			activation, err := BuildActivation(tc.request)
			require.NoError(t, err, "BuildActivation should not return error for valid request")
			require.NotNil(t, activation, "Activation should not be nil")

			// Verify transactionType
			transType, found := activation["transactionType"]
			assert.True(t, found, "Activation should contain transactionType")
			assert.Equal(t, tc.expectedTransType, transType, "transactionType should match")

			// Verify subType
			subTypeVal, found := activation["subType"]
			assert.True(t, found, "Activation should contain subType")
			assert.Equal(t, tc.expectedSubType, subTypeVal, "subType should match")

			// Verify amount
			amountVal, found := activation["amount"]
			assert.True(t, found, "Activation should contain amount")
			assert.Equal(t, tc.expectedAmount, amountVal, "amount should match")

			// Verify currency
			currencyVal, found := activation["currency"]
			assert.True(t, found, "Activation should contain currency")
			assert.Equal(t, tc.expectedCurrency, currencyVal, "currency should match")

			// Verify account
			accountVal, found := activation["account"]
			assert.True(t, found, "Activation should contain account")
			accountMap, ok := accountVal.(map[string]any)
			require.True(t, ok, "account should be a map")
			assert.Equal(t, tc.expectedAccountID, accountMap["accountId"], "account.accountId should match")

			// Verify merchant
			if !tc.expectMerchantNil {
				merchantVal, found := activation["merchant"]
				assert.True(t, found, "Activation should contain merchant")
				merchantMap, ok := merchantVal.(map[string]any)
				require.True(t, ok, "merchant should be a map")
				assert.Equal(t, tc.expectedMerchantID, merchantMap["merchantId"], "merchant.merchantId should match")
			}

			// Verify segment
			segmentVal, found := activation["segment"]
			assert.True(t, found, "Activation should contain segment")
			segmentMap, ok := segmentVal.(map[string]any)
			require.True(t, ok, "segment should be a map")
			if tc.expectSegment {
				assert.NotEmpty(t, segmentMap["segmentId"], "segment.segmentId should not be empty")
			}

			// Verify portfolio
			portfolioVal, found := activation["portfolio"]
			assert.True(t, found, "Activation should contain portfolio")
			portfolioMap, ok := portfolioVal.(map[string]any)
			require.True(t, ok, "portfolio should be a map")
			if tc.expectPortfolio {
				assert.NotEmpty(t, portfolioMap["portfolioId"], "portfolio.portfolioId should not be empty")
			}

			// Verify metadata
			metadataVal, found := activation["metadata"]
			assert.True(t, found, "Activation should contain metadata")
			_, ok = metadataVal.(map[string]any)
			require.True(t, ok, "metadata should be a map")

			// Verify timestamp
			timestampVal, found := activation["transactionTimestamp"]
			assert.True(t, found, "Activation should contain timestamp")
			_, ok = timestampVal.(int64)
			require.True(t, ok, "timestamp should be int64")
		})
	}
}

// TestBuildActivation_NilOptionalFields tests building activation when optional fields are nil.
func TestBuildActivation_NilOptionalFields(t *testing.T) {
	tests := []struct {
		name        string
		request     *model.ValidationRequest
		description string
	}{
		{
			name: "Success - build activation with nil merchant",
			request: &model.ValidationRequest{
				RequestID:            uuid.New(),
				TransactionType:      model.TransactionTypeWire,
				SubType:              nil,
				Amount:               decimal.RequireFromString("50"),
				Currency:             "BRL",
				TransactionTimestamp: time.Now(),
				Account: model.AccountContext{
					ID:     envTestAccountID2,
					Type:   "savings",
					Status: "active",
				},
				Merchant:  nil,
				Segment:   nil,
				Portfolio: nil,
				Metadata:  nil,
			},
			description: "Activation should handle nil merchant gracefully",
		},
		{
			name: "Success - build activation with nil subType",
			request: &model.ValidationRequest{
				RequestID:            uuid.New(),
				TransactionType:      model.TransactionTypePix,
				SubType:              nil,
				Amount:               decimal.RequireFromString("10"),
				Currency:             "BRL",
				TransactionTimestamp: time.Now(),
				Account: model.AccountContext{
					ID:     envTestAccountID3,
					Type:   "checking",
					Status: "active",
				},
				Merchant:  nil,
				Segment:   nil,
				Portfolio: nil,
				Metadata:  map[string]any{},
			},
			description: "Activation should handle nil subType gracefully",
		},
		{
			name: "Success - build activation with nil segment and portfolio",
			request: &model.ValidationRequest{
				RequestID:            uuid.New(),
				TransactionType:      model.TransactionTypeCrypto,
				SubType:              nil,
				Amount:               decimal.RequireFromString("1000"),
				Currency:             "USD",
				TransactionTimestamp: time.Now(),
				Account: model.AccountContext{
					ID:     envTestAccountID4,
					Type:   "credit",
					Status: "suspended",
				},
				Merchant:  nil,
				Segment:   nil,
				Portfolio: nil,
				Metadata:  nil,
			},
			description: "Activation should handle nil segment and portfolio gracefully",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewEnvironment()
			require.NoError(t, err, "NewEnvironment should not return error")

			activation, err := BuildActivation(tc.request)
			require.NoError(t, err, "BuildActivation should not return error for valid request with nil fields")
			require.NotNil(t, activation, "Activation should not be nil")

			// Verify required fields are present
			_, found := activation["transactionType"]
			assert.True(t, found, "Activation should contain transactionType")

			_, found = activation["amount"]
			assert.True(t, found, "Activation should contain amount")

			_, found = activation["currency"]
			assert.True(t, found, "Activation should contain currency")

			_, found = activation["account"]
			assert.True(t, found, "Activation should contain account")

			_, found = activation["transactionTimestamp"]
			assert.True(t, found, "Activation should contain timestamp")

			// Verify optional fields have appropriate defaults or empty values
			subTypeVal, found := activation["subType"]
			assert.True(t, found, "Activation should contain subType (even if empty)")
			if tc.request.SubType == nil {
				assert.Equal(t, "", subTypeVal, "subType should be empty string when nil")
			}

			merchantVal, found := activation["merchant"]
			assert.True(t, found, "Activation should contain merchant (even if empty)")
			if tc.request.Merchant == nil {
				merchantMap, ok := merchantVal.(map[string]any)
				require.True(t, ok, "merchant should be a map")
				assert.Empty(t, merchantMap, "merchant should be empty map when nil")
			}

			segmentVal, found := activation["segment"]
			assert.True(t, found, "Activation should contain segment")
			segmentMap, ok := segmentVal.(map[string]any)
			require.True(t, ok, "segment should be a map")
			if tc.request.Segment == nil {
				assert.Empty(t, segmentMap, "segment should be empty map when nil")
			}

			portfolioVal, found := activation["portfolio"]
			assert.True(t, found, "Activation should contain portfolio")
			portfolioMap, ok := portfolioVal.(map[string]any)
			require.True(t, ok, "portfolio should be a map")
			if tc.request.Portfolio == nil {
				assert.Empty(t, portfolioMap, "portfolio should be empty map when nil")
			}

			metadataVal, found := activation["metadata"]
			assert.True(t, found, "Activation should contain metadata")
			metadataMap, ok := metadataVal.(map[string]any)
			require.True(t, ok, "metadata should be a map")
			if tc.request.Metadata == nil {
				assert.Empty(t, metadataMap, "metadata should be empty map when nil")
			}
		})
	}
}

// TestBuildActivation_AmountPrecisionValidation tests that BuildActivation rejects amounts
// that exceed float64 safe precision range.
func TestBuildActivation_AmountPrecisionValidation(t *testing.T) {
	tests := []struct {
		name      string
		amount    string
		expectErr bool
	}{
		{
			name:      "Success - normal monetary amount",
			amount:    "1000.50",
			expectErr: false,
		},
		{
			name:      "Success - large but safe amount",
			amount:    "999999999999999",
			expectErr: false,
		},
		{
			name:      "Error - amount exceeds float64 safe precision",
			amount:    "9007199254740993",
			expectErr: true,
		},
		{
			name:      "Error - negative amount exceeds float64 safe precision",
			amount:    "-9007199254740993",
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &model.ValidationRequest{
				RequestID:            uuid.New(),
				TransactionType:      model.TransactionTypePix,
				Amount:               decimal.RequireFromString(tc.amount),
				Currency:             "BRL",
				TransactionTimestamp: time.Now(),
				Account: model.AccountContext{
					ID:     envTestAccountID1,
					Type:   "checking",
					Status: "active",
				},
			}

			activation, err := BuildActivation(req)

			if tc.expectErr {
				require.Error(t, err, "Expected error for amount %s", tc.amount)
				assert.Nil(t, activation, "Activation should be nil on error")
				assert.Contains(t, err.Error(), "exceeds safe precision")
			} else {
				require.NoError(t, err, "Unexpected error for amount %s", tc.amount)
				assert.NotNil(t, activation, "Activation should not be nil")
			}
		})
	}
}

// TestBuildActivation_NilRequest tests that BuildActivation returns error for nil request.
func TestBuildActivation_NilRequest(t *testing.T) {
	activation, err := BuildActivation(nil)

	assert.Nil(t, activation, "activation should be nil for nil request")
	assert.Error(t, err, "should return error for nil request")
	assert.Contains(t, err.Error(), "validation request is required")
}
