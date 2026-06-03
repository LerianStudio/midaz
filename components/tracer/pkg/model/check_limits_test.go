// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model_test

import (
	"encoding/json"

	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

func TestNewCheckLimitsInput_Valid(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	fixedTime := testutil.FixedTime()

	input, err := model.NewCheckLimitsInput(decimal.RequireFromString("100"), "BRL", accountID, nil, nil, nil, nil, nil, fixedTime)

	require.NoError(t, err)
	assert.True(t, decimal.RequireFromString("100").Equal(input.Amount))
	assert.Equal(t, "BRL", input.Currency)
	assert.Equal(t, accountID, input.AccountID)
}

func TestNewCheckLimitsInput_NormalizeCurrency(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	fixedTime := testutil.FixedTime()

	input, err := model.NewCheckLimitsInput(decimal.RequireFromString("100"), "brl", accountID, nil, nil, nil, nil, nil, fixedTime)

	require.NoError(t, err)
	assert.Equal(t, "BRL", input.Currency)
}

func TestNewCheckLimitsInput_InvalidAmount(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	fixedTime := testutil.FixedTime()

	tests := []struct {
		name   string
		amount decimal.Decimal
	}{
		{"zero amount", decimal.RequireFromString("0")},
		{"negative amount", decimal.RequireFromString("-1")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := model.NewCheckLimitsInput(tt.amount, "BRL", accountID, nil, nil, nil, nil, nil, fixedTime)

			require.Error(t, err)
			assert.ErrorIs(t, err, constant.ErrCheckLimitsInvalidAmount)
		})
	}
}

func TestNewCheckLimitsInput_InvalidCurrency(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	fixedTime := testutil.FixedTime()

	tests := []struct {
		name     string
		currency string
	}{
		{"empty currency", ""},
		{"too short", "BR"},
		{"too long", "BRLL"},
		{"two chars lowercase", "br"},
		{"numeric", "123"},
		{"special chars", "BR$"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := model.NewCheckLimitsInput(decimal.RequireFromString("100"), tt.currency, accountID, nil, nil, nil, nil, nil, fixedTime)

			require.Error(t, err)
			assert.ErrorIs(t, err, constant.ErrCheckLimitsInvalidCurrency)
		})
	}
}

func TestNewCheckLimitsInput_InvalidAccountID(t *testing.T) {
	t.Parallel()

	fixedTime := testutil.FixedTime()

	_, err := model.NewCheckLimitsInput(decimal.RequireFromString("100"), "BRL", uuid.Nil, nil, nil, nil, nil, nil, fixedTime)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrCheckLimitsInvalidAccountID)
}

func TestNewCheckLimitsInput_InvalidSegmentID(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	fixedTime := testutil.FixedTime()
	zeroID := uuid.Nil

	_, err := model.NewCheckLimitsInput(decimal.RequireFromString("100"), "BRL", accountID, &zeroID, nil, nil, nil, nil, fixedTime)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrCheckLimitsInvalidSegmentID)
}

func TestNewCheckLimitsInput_InvalidPortfolioID(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	fixedTime := testutil.FixedTime()
	zeroID := uuid.Nil

	_, err := model.NewCheckLimitsInput(decimal.RequireFromString("100"), "BRL", accountID, nil, &zeroID, nil, nil, nil, fixedTime)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrCheckLimitsInvalidPortfolioID)
}

func TestNewCheckLimitsInput_InvalidMerchantID(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	fixedTime := testutil.FixedTime()
	zeroID := uuid.Nil

	_, err := model.NewCheckLimitsInput(decimal.RequireFromString("100"), "BRL", accountID, nil, nil, &zeroID, nil, nil, fixedTime)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrCheckLimitsInvalidMerchantID)
}

func TestCheckLimitsInput_Validate(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	fixedTime := testutil.FixedTime()

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		AccountID:            accountID,
		TransactionTimestamp: fixedTime,
	}

	err := input.Validate()

	assert.NoError(t, err)
}

func TestCheckLimitsInput_Validate_NilReceiver(t *testing.T) {
	t.Parallel()

	var input *model.CheckLimitsInput

	err := input.Validate()

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrCheckLimitsNilInput)
}

func TestNewCheckLimitsInput_ZeroTimestamp(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)

	_, err := model.NewCheckLimitsInput(decimal.RequireFromString("100"), "BRL", accountID, nil, nil, nil, nil, nil, time.Time{})

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrCheckLimitsInvalidTimestamp)
}

func TestNewCheckLimitsInput_WithTransactionType(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	fixedTime := testutil.FixedTime()
	txType := model.TransactionTypeCard

	input, err := model.NewCheckLimitsInput(decimal.RequireFromString("100"), "BRL", accountID, nil, nil, nil, &txType, nil, fixedTime)

	require.NoError(t, err)
	require.NotNil(t, input.TransactionType)
	assert.Equal(t, txType, *input.TransactionType)
	assert.Nil(t, input.SubType)
}

func TestNewCheckLimitsInput_WithSubType(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	fixedTime := testutil.FixedTime()
	subType := "debit"

	input, err := model.NewCheckLimitsInput(decimal.RequireFromString("100"), "BRL", accountID, nil, nil, nil, nil, &subType, fixedTime)

	require.NoError(t, err)
	assert.Nil(t, input.TransactionType)
	require.NotNil(t, input.SubType)
	assert.Equal(t, subType, *input.SubType)
}

func TestNewCheckLimitsInput_WithAllOptionalFields(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	segmentID := testutil.MustDeterministicUUID(2)
	portfolioID := testutil.MustDeterministicUUID(3)
	// Seed 20 for merchant ID keeps it deterministically distinguishable from
	// accountID (seed 1), segmentID (seed 2), and portfolioID (seed 3) within
	// this test — so the final assertion on `input.MerchantID` can pinpoint
	// that the MerchantID field received the right value (and not, say, a
	// copy of AccountID/SegmentID/PortfolioID from a constructor argument
	// order bug). The assertion verifies that NewCheckLimitsInput propagates
	// MerchantID into the returned struct.
	merchantID := testutil.MustDeterministicUUID(20)
	fixedTime := testutil.FixedTime()
	txType := model.TransactionTypePix
	subType := "instant"

	input, err := model.NewCheckLimitsInput(decimal.RequireFromString("100"), "BRL", accountID, &segmentID, &portfolioID, &merchantID, &txType, &subType, fixedTime)

	require.NoError(t, err)
	assert.Equal(t, accountID, input.AccountID)
	require.NotNil(t, input.SegmentID)
	assert.Equal(t, segmentID, *input.SegmentID)
	require.NotNil(t, input.PortfolioID)
	assert.Equal(t, portfolioID, *input.PortfolioID)
	require.NotNil(t, input.MerchantID)
	assert.Equal(t, merchantID, *input.MerchantID)
	require.NotNil(t, input.TransactionType)
	assert.Equal(t, txType, *input.TransactionType)
	require.NotNil(t, input.SubType)
	assert.Equal(t, subType, *input.SubType)
}

func TestCheckLimitsInput_Validate_Invalid(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	fixedTime := testutil.FixedTime()

	tests := []struct {
		name        string
		input       model.CheckLimitsInput
		expectedErr error
	}{
		{
			name: "zero amount",
			input: model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("0"),
				Currency:             "BRL",
				AccountID:            accountID,
				TransactionTimestamp: fixedTime,
			},
			expectedErr: constant.ErrCheckLimitsInvalidAmount,
		},
		{
			name: "invalid currency",
			input: model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("100"),
				Currency:             "XX",
				AccountID:            accountID,
				TransactionTimestamp: fixedTime,
			},
			expectedErr: constant.ErrCheckLimitsInvalidCurrency,
		},
		{
			name: "nil account ID",
			input: model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				AccountID:            uuid.Nil,
				TransactionTimestamp: fixedTime,
			},
			expectedErr: constant.ErrCheckLimitsInvalidAccountID,
		},
		{
			name: "zero timestamp",
			input: model.CheckLimitsInput{
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				AccountID:            accountID,
				TransactionTimestamp: time.Time{},
			},
			expectedErr: constant.ErrCheckLimitsInvalidTimestamp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.input.Validate()

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestNewCheckLimitsOutput_Allowed(t *testing.T) {
	t.Parallel()

	fixedTime := testutil.FixedTime()
	output := model.NewCheckLimitsOutput(true, fixedTime)

	assert.True(t, output.Allowed)
	assert.Empty(t, output.ExceededLimitIDs)
	assert.Empty(t, output.LimitUsageDetails)
	assert.Equal(t, fixedTime, output.EvaluatedAt)
}

func TestNewCheckLimitsOutput_Denied(t *testing.T) {
	t.Parallel()

	fixedTime := testutil.FixedTime()
	output := model.NewCheckLimitsOutput(false, fixedTime)

	assert.False(t, output.Allowed)
	assert.Empty(t, output.ExceededLimitIDs)
	assert.Empty(t, output.LimitUsageDetails)
	assert.Equal(t, fixedTime, output.EvaluatedAt)
}

func TestCheckLimitsOutput_WithExceededLimits(t *testing.T) {
	t.Parallel()

	fixedTime := testutil.FixedTime()
	exceededIDs := []uuid.UUID{
		testutil.MustDeterministicUUID(1),
		testutil.MustDeterministicUUID(2),
	}

	output := model.NewCheckLimitsOutput(false, fixedTime).WithExceededLimits(exceededIDs)

	assert.False(t, output.Allowed)
	assert.Equal(t, exceededIDs, output.ExceededLimitIDs)
	assert.Equal(t, fixedTime, output.EvaluatedAt)
}

func TestCheckLimitsOutput_WithLimitUsageDetails(t *testing.T) {
	t.Parallel()

	fixedTime := testutil.FixedTime()
	details := []model.LimitUsageDetail{
		{
			LimitID:      testutil.MustDeterministicUUID(1),
			LimitAmount:  decimal.RequireFromString("1000"),
			CurrentUsage: decimal.RequireFromString("500"),
			Exceeded:     false,
		},
	}

	output := model.NewCheckLimitsOutput(true, fixedTime).WithLimitUsageDetails(details)

	assert.True(t, output.Allowed)
	assert.Equal(t, details, output.LimitUsageDetails)
	assert.Equal(t, fixedTime, output.EvaluatedAt)
}

func TestCheckLimitsOutput_ChainedMethods(t *testing.T) {
	t.Parallel()

	fixedTime := testutil.FixedTime()
	exceededIDs := []uuid.UUID{testutil.MustDeterministicUUID(1)}
	details := []model.LimitUsageDetail{
		{
			LimitID:      testutil.MustDeterministicUUID(1),
			LimitAmount:  decimal.RequireFromString("1000"),
			CurrentUsage: decimal.RequireFromString("1500"),
			Exceeded:     true,
		},
	}

	output := model.NewCheckLimitsOutput(false, fixedTime).
		WithExceededLimits(exceededIDs).
		WithLimitUsageDetails(details)

	assert.False(t, output.Allowed)
	assert.Equal(t, exceededIDs, output.ExceededLimitIDs)
	assert.Equal(t, details, output.LimitUsageDetails)
	assert.Equal(t, fixedTime, output.EvaluatedAt)
}

func TestCheckLimitsOutput_WithNilSlices(t *testing.T) {
	t.Parallel()

	fixedTime := testutil.FixedTime()
	output := model.NewCheckLimitsOutput(true, fixedTime).
		WithExceededLimits(nil).
		WithLimitUsageDetails(nil)

	assert.NotNil(t, output.ExceededLimitIDs, "ExceededLimitIDs should not be nil")
	assert.NotNil(t, output.LimitUsageDetails, "LimitUsageDetails should not be nil")
	assert.Empty(t, output.ExceededLimitIDs)
	assert.Empty(t, output.LimitUsageDetails)
	assert.Equal(t, fixedTime, output.EvaluatedAt)
}

func TestCheckLimitsOutput_JSONSerializesEmptyArrays(t *testing.T) {
	t.Parallel()

	fixedTime := testutil.FixedTime()
	output := model.NewCheckLimitsOutput(true, fixedTime).
		WithExceededLimits(nil).
		WithLimitUsageDetails(nil)

	jsonBytes, err := json.Marshal(output)
	require.NoError(t, err)

	jsonStr := string(jsonBytes)
	assert.Contains(t, jsonStr, `"exceededLimitIds":[]`)
	assert.Contains(t, jsonStr, `"limitUsageDetails":[]`)
	assert.Contains(t, jsonStr, `"evaluatedAt":`)
	assert.NotContains(t, jsonStr, "null")
}

func TestLimitUsageDetail_RemainingAmount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		detail       model.LimitUsageDetail
		expectedRest decimal.Decimal
	}{
		{
			name: "has remaining",
			detail: model.LimitUsageDetail{
				LimitAmount:  decimal.RequireFromString("1000"),
				CurrentUsage: decimal.RequireFromString("300"),
			},
			expectedRest: decimal.RequireFromString("700"),
		},
		{
			name: "exactly at limit",
			detail: model.LimitUsageDetail{
				LimitAmount:  decimal.RequireFromString("1000"),
				CurrentUsage: decimal.RequireFromString("1000"),
			},
			expectedRest: decimal.RequireFromString("0"),
		},
		{
			name: "exceeded - clamped to zero",
			detail: model.LimitUsageDetail{
				LimitAmount:  decimal.RequireFromString("1000"),
				CurrentUsage: decimal.RequireFromString("1500"),
			},
			expectedRest: decimal.RequireFromString("0"), // Cannot be negative
		},
		{
			name: "no usage - returns full limit",
			detail: model.LimitUsageDetail{
				LimitAmount:  decimal.RequireFromString("1000"),
				CurrentUsage: decimal.RequireFromString("0"),
			},
			expectedRest: decimal.RequireFromString("1000"), // Full limit available
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			remaining := tt.detail.RemainingAmount()

			assert.True(t, tt.expectedRest.Equal(remaining), "expected %s, got %s", tt.expectedRest, remaining)
		})
	}
}

func TestLimitUsageDetail_RemainingAmount_NilReceiver(t *testing.T) {
	t.Parallel()

	var detail *model.LimitUsageDetail

	remaining := detail.RemainingAmount()

	assert.True(t, decimal.Zero.Equal(remaining))
}

func TestCalculatePeriodKey(t *testing.T) {
	t.Parallel()

	timestamp := time.Date(2025, 12, 28, 15, 30, 0, 0, time.UTC)

	tests := []struct {
		name      string
		limitType model.LimitType
		expected  string
	}{
		{
			name:      "daily",
			limitType: model.LimitTypeDaily,
			expected:  "2025-12-28",
		},
		{
			name:      "monthly",
			limitType: model.LimitTypeMonthly,
			expected:  "2025-12",
		},
		{
			name:      "per transaction",
			limitType: model.LimitTypePerTransaction,
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			periodKey, err := model.CalculatePeriodKey(tt.limitType, timestamp)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, periodKey)
		})
	}
}

func TestCalculatePeriodKey_UnknownLimitType(t *testing.T) {
	t.Parallel()

	timestamp := time.Date(2025, 12, 28, 15, 30, 0, 0, time.UTC)
	unknownType := model.LimitType("UNKNOWN")

	periodKey, err := model.CalculatePeriodKey(unknownType, timestamp)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrCheckLimitsUnknownLimitType)
	assert.Contains(t, err.Error(), "UNKNOWN")
	assert.Empty(t, periodKey)
}

func TestCalculateScopeKey(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	segmentID := testutil.MustDeterministicUUID(2)
	portfolioID := testutil.MustDeterministicUUID(3)
	merchantID := testutil.MustDeterministicUUID(4)

	tests := []struct {
		name     string
		scope    model.Scope
		expected string
	}{
		{
			name:     "account only",
			scope:    model.Scope{AccountID: &accountID},
			expected: "acct:" + accountID.String(),
		},
		{
			name:     "segment only",
			scope:    model.Scope{SegmentID: &segmentID},
			expected: "seg:" + segmentID.String(),
		},
		{
			name:     "portfolio only",
			scope:    model.Scope{PortfolioID: &portfolioID},
			expected: "port:" + portfolioID.String(),
		},
		{
			name:     "account and segment",
			scope:    model.Scope{AccountID: &accountID, SegmentID: &segmentID},
			expected: "acct:" + accountID.String() + "|seg:" + segmentID.String(),
		},
		{
			name:     "all three",
			scope:    model.Scope{AccountID: &accountID, SegmentID: &segmentID, PortfolioID: &portfolioID},
			expected: "acct:" + accountID.String() + "|port:" + portfolioID.String() + "|seg:" + segmentID.String(),
		},
		{
			name:     "merchant only",
			scope:    model.Scope{MerchantID: &merchantID},
			expected: "merch:" + merchantID.String(),
		},
		{
			name:     "account and merchant",
			scope:    model.Scope{AccountID: &accountID, MerchantID: &merchantID},
			expected: "acct:" + accountID.String() + "|merch:" + merchantID.String(),
		},
		{
			name:     "all four",
			scope:    model.Scope{AccountID: &accountID, SegmentID: &segmentID, PortfolioID: &portfolioID, MerchantID: &merchantID},
			expected: "acct:" + accountID.String() + "|merch:" + merchantID.String() + "|port:" + portfolioID.String() + "|seg:" + segmentID.String(),
		},
		{
			name:     "empty scope",
			scope:    model.Scope{},
			expected: "global",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scopeKey := model.CalculateScopeKey(&tt.scope)

			assert.Equal(t, tt.expected, scopeKey)
		})
	}

	t.Run("nil scope", func(t *testing.T) {
		t.Parallel()

		scopeKey := model.CalculateScopeKey(nil)

		assert.Equal(t, "global", scopeKey)
	})
}

func TestCheckLimitsInput_Validate_InvalidSegmentID(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	zero := uuid.Nil
	fixedTime := testutil.FixedTime()

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		AccountID:            accountID,
		SegmentID:            &zero,
		TransactionTimestamp: fixedTime,
	}

	err := input.Validate()

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrCheckLimitsInvalidSegmentID)
}

func TestCheckLimitsInput_Validate_InvalidPortfolioID(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	zero := uuid.Nil
	fixedTime := testutil.FixedTime()

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		AccountID:            accountID,
		PortfolioID:          &zero,
		TransactionTimestamp: fixedTime,
	}

	err := input.Validate()

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrCheckLimitsInvalidPortfolioID)
}

func TestCheckLimitsInput_Validate_InvalidMerchantID(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	zero := uuid.Nil
	fixedTime := testutil.FixedTime()

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		AccountID:            accountID,
		MerchantID:           &zero,
		TransactionTimestamp: fixedTime,
	}

	err := input.Validate()

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrCheckLimitsInvalidMerchantID)
}

func TestCheckLimitsInput_Validate_InvalidTransactionType(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	fixedTime := testutil.FixedTime()
	invalidTxType := model.TransactionType("INVALID")

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		AccountID:            accountID,
		TransactionTimestamp: fixedTime,
		TransactionType:      &invalidTxType,
	}

	err := input.Validate()

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrCheckLimitsInvalidTransactionType)
}

func TestCheckLimitsInput_Validate_SubTypeTooLong(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	fixedTime := testutil.FixedTime()
	longSubType := "a_very_long_subtype_that_exceeds_the_fifty_character_limit_defined"

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		AccountID:            accountID,
		TransactionTimestamp: fixedTime,
		SubType:              &longSubType,
	}

	err := input.Validate()

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrCheckLimitsInvalidSubType)
}

func TestCheckLimitsInput_Validate_SubTypeExactlyAtLimit(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(1)
	fixedTime := testutil.FixedTime()
	exactSubType := strings.Repeat("a", model.MaxSubTypeLength)

	input := &model.CheckLimitsInput{
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		AccountID:            accountID,
		TransactionTimestamp: fixedTime,
		SubType:              &exactSubType,
	}

	err := input.Validate()

	require.NoError(t, err)
}

// TestCheckLimitsOutput_EvaluatedAt tests that the EvaluatedAt timestamp is:
// 1. Preserved in the output structure
// 2. Serialized to JSON in ISO 8601 UTC format
// Seed range: 11000-11099
func TestCheckLimitsOutput_EvaluatedAt(t *testing.T) {
	t.Parallel()

	// Use deterministic timestamp for reproducible tests
	evaluatedAt := time.Date(2026, 3, 10, 14, 30, 15, 0, time.UTC)

	// Test 1: EvaluatedAt field is preserved in constructor
	output := model.NewCheckLimitsOutput(true, evaluatedAt)

	require.Equal(t, evaluatedAt, output.EvaluatedAt,
		"EvaluatedAt should be preserved in the output")
	assert.True(t, output.Allowed)

	// Test 2: JSON serialization produces ISO 8601 UTC format
	jsonBytes, err := json.Marshal(output)
	require.NoError(t, err, "failed to marshal output to JSON")

	var parsed map[string]any
	err = json.Unmarshal(jsonBytes, &parsed)
	require.NoError(t, err, "failed to unmarshal JSON")

	// Verify evaluatedAt field exists in JSON
	require.Contains(t, parsed, "evaluatedAt",
		"JSON output should contain evaluatedAt field")

	// Verify ISO 8601 format: "2026-03-10T14:30:15Z"
	expectedISO8601 := "2026-03-10T14:30:15Z"
	assert.Equal(t, expectedISO8601, parsed["evaluatedAt"],
		"evaluatedAt should be serialized in ISO 8601 UTC format")
}

// TestCheckLimitsOutput_EvaluatedAt_WithUsageDetails verifies that EvaluatedAt
// is preserved when chaining methods like WithExceededLimits and WithLimitUsageDetails.
func TestCheckLimitsOutput_EvaluatedAt_WithUsageDetails(t *testing.T) {
	t.Parallel()

	evaluatedAt := time.Date(2026, 3, 10, 14, 30, 15, 0, time.UTC)
	limitID := testutil.MustDeterministicUUID(11001)

	details := []model.LimitUsageDetail{
		{
			LimitID:      limitID,
			LimitAmount:  decimal.RequireFromString("1000"),
			CurrentUsage: decimal.RequireFromString("500"),
			Exceeded:     false,
		},
	}

	output := model.NewCheckLimitsOutput(true, evaluatedAt).
		WithLimitUsageDetails(details)

	// EvaluatedAt should be preserved through method chaining
	assert.Equal(t, evaluatedAt, output.EvaluatedAt,
		"EvaluatedAt should be preserved after method chaining")
	assert.Len(t, output.LimitUsageDetails, 1)
}
