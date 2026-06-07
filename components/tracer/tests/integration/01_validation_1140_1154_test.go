// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Tests 1.1.40-1.1.54: Limits and Advanced Scenarios
// ============================================================================

// Test 1.1.40: Validation with MONTHLY limit type
func TestValidation_1_1_40_MonthlyLimitType(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1000).String()

	// Create and activate MONTHLY limit of 10000
	limitID := testutil.CreateLimitWithAccountScopeAndType(t, accountID, "10000", "MONTHLY")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Consume 9000 of the limit with a first validation
	firstReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1001).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("9000"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, firstReq)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "First validation should succeed: %s", string(body1))

	var firstResult testutil.ValidationResponse
	err := json.Unmarshal(body1, &firstResult)
	require.NoError(t, err)
	require.Equal(t, "ALLOW", firstResult.Decision, "First validation should ALLOW")

	// Now try to exceed the limit with amount 1500 (9000 + 1500 > 10000)
	secondReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1002).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("1500"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp2, body2 := testutil.CreateValidation(t, secondReq)
	defer resp2.Body.Close()

	require.Equal(t, http.StatusCreated, resp2.StatusCode, "Expected 201 Created, got: %s", string(body2))

	var result testutil.ValidationResponse
	err = json.Unmarshal(body2, &result)
	require.NoError(t, err)

	assert.Equal(t, "DENY", result.Decision, "Expected DENY when monthly limit is exceeded")
	assert.Equal(t, "limit_exceeded", result.Reason, "Expected reason to be limit_exceeded")

	// Verify limitUsageDetails contains entry with period == "MONTHLY" and exceeded == true
	require.NotEmpty(t, result.LimitUsageDetails, "limitUsageDetails should not be empty")
	var foundLimit *testutil.LimitUsageDetail
	for i := range result.LimitUsageDetails {
		if result.LimitUsageDetails[i].LimitID == limitID {
			foundLimit = &result.LimitUsageDetails[i]
			break
		}
	}
	require.NotNil(t, foundLimit, "limitUsageDetails should contain our limit")
	assert.Equal(t, "MONTHLY", foundLimit.Period, "period should be MONTHLY")
	assert.True(t, foundLimit.Exceeded, "exceeded should be true")
}

// Test 1.1.41: Validation with PER_TRANSACTION limit type
func TestValidation_1_1_41_PerTransactionLimitType(t *testing.T) {
	// Create and activate PER_TRANSACTION limit of 1000 for CARD transactions
	transactionType := "CARD"
	limitID := testutil.CreateLimitWithTransactionTypeScope(t, transactionType, "1000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	accountID := testutil.MustDeterministicUUID(1010).String()

	// Test 1: Amount within limit - expect ALLOW
	t.Run("Within limit", func(t *testing.T) {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(1011).String(),
			TransactionType:      "CARD",
			Amount:               decimal.RequireFromString("500"),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "ALLOW", result.Decision, "Expected ALLOW for amount within PER_TRANSACTION limit")

		// Verify limitUsageDetails contains entry with period == "PER_TRANSACTION" and currentUsage == 0
		require.NotEmpty(t, result.LimitUsageDetails, "limitUsageDetails should not be empty")
		var foundLimit *testutil.LimitUsageDetail
		for i := range result.LimitUsageDetails {
			if result.LimitUsageDetails[i].LimitID == limitID {
				foundLimit = &result.LimitUsageDetails[i]
				break
			}
		}
		require.NotNil(t, foundLimit, "limitUsageDetails should contain our limit")
		assert.Equal(t, "PER_TRANSACTION", foundLimit.Period, "period should be PER_TRANSACTION")
		assert.True(t, foundLimit.CurrentUsage.IsZero(), "currentUsage should always be 0 for PER_TRANSACTION")
		assert.False(t, foundLimit.Exceeded, "exceeded should be false")
	})

	// Test 2: Amount exceeds limit - expect DENY
	t.Run("Exceeds limit", func(t *testing.T) {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(1012).String(),
			TransactionType:      "CARD",
			Amount:               decimal.RequireFromString("1500"),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "DENY", result.Decision, "Expected DENY for amount exceeding PER_TRANSACTION limit")

		// Verify limitUsageDetails
		require.NotEmpty(t, result.LimitUsageDetails, "limitUsageDetails should not be empty")
		var foundLimit *testutil.LimitUsageDetail
		for i := range result.LimitUsageDetails {
			if result.LimitUsageDetails[i].LimitID == limitID {
				foundLimit = &result.LimitUsageDetails[i]
				break
			}
		}
		require.NotNil(t, foundLimit, "limitUsageDetails should contain our limit")
		assert.Equal(t, "PER_TRANSACTION", foundLimit.Period, "period should be PER_TRANSACTION")
		assert.True(t, foundLimit.CurrentUsage.IsZero(), "currentUsage should always be 0 for PER_TRANSACTION")
		assert.True(t, foundLimit.Exceeded, "exceeded should be true")
	})

	// Test 3: Amount exactly at limit - expect ALLOW
	t.Run("Exactly at limit", func(t *testing.T) {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(1013).String(),
			TransactionType:      "CARD",
			Amount:               decimal.RequireFromString("1000"), // Exactly at limit
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "ALLOW", result.Decision, "Expected ALLOW for amount exactly at PER_TRANSACTION limit")

		// Verify limitUsageDetails
		require.NotEmpty(t, result.LimitUsageDetails, "limitUsageDetails should not be empty")
		var foundLimit *testutil.LimitUsageDetail
		for i := range result.LimitUsageDetails {
			if result.LimitUsageDetails[i].LimitID == limitID {
				foundLimit = &result.LimitUsageDetails[i]
				break
			}
		}
		require.NotNil(t, foundLimit, "limitUsageDetails should contain our limit")
		assert.Equal(t, "PER_TRANSACTION", foundLimit.Period, "period should be PER_TRANSACTION")
		assert.True(t, foundLimit.CurrentUsage.IsZero(), "currentUsage should always be 0 for PER_TRANSACTION")
		assert.False(t, foundLimit.Exceeded, "exceeded should be false at exact boundary")
	})
}

// Test 1.1.42: Validation with scope-matching rules (segment scope)
func TestValidation_1_1_42_ScopeMatchingRulesSegment(t *testing.T) {
	matchingSegmentID := testutil.MustDeterministicUUID(1020).String()
	nonMatchingSegmentID := testutil.MustDeterministicUUID(1021).String()
	accountID := testutil.MustDeterministicUUID(1022).String()

	// Create and activate DENY rule with segment scope
	ruleName := "deny-segment-scope-" + testutil.MustDeterministicUUID(1201).String()[:8]
	ruleID := testutil.CreateRuleWithScope(t, ruleName, "amount > 0", "DENY", []testutil.ScopeInput{
		{SegmentID: &matchingSegmentID},
	})
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Test 1: Matching segment - expect DENY
	t.Run("Matching segment", func(t *testing.T) {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(1023).String(),
			TransactionType:      "CARD",
			Amount:               decimal.RequireFromString("100"),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
			Segment: &testutil.SegmentContext{
				ID: matchingSegmentID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "DENY", result.Decision, "Expected DENY when segment matches rule scope")
		assert.Contains(t, result.MatchedRuleIDs, ruleID, "matchedRuleIds should contain the segment-scoped rule")
	})

	// Test 2: Non-matching segment - expect ALLOW
	t.Run("Non-matching segment", func(t *testing.T) {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(1024).String(),
			TransactionType:      "CARD",
			Amount:               decimal.RequireFromString("100"),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
			Segment: &testutil.SegmentContext{
				ID: nonMatchingSegmentID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "ALLOW", result.Decision, "Expected ALLOW when segment does not match rule scope")
		assert.NotContains(t, result.MatchedRuleIDs, ruleID, "matchedRuleIds should NOT contain the segment-scoped rule")
	})
}

// Test 1.1.43: Validation with scope-matching rules (portfolio scope)
func TestValidation_1_1_43_ScopeMatchingRulesPortfolio(t *testing.T) {
	matchingPortfolioID := testutil.MustDeterministicUUID(1030).String()
	nonMatchingPortfolioID := testutil.MustDeterministicUUID(1031).String()
	accountID := testutil.MustDeterministicUUID(1032).String()

	// Create and activate REVIEW rule with portfolio scope
	ruleName := "review-portfolio-scope-" + testutil.MustDeterministicUUID(1202).String()[:8]
	ruleID := testutil.CreateRuleWithScope(t, ruleName, "transactionType == 'WIRE'", "REVIEW", []testutil.ScopeInput{
		{PortfolioID: &matchingPortfolioID},
	})
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Test 1: Matching portfolio - expect REVIEW
	t.Run("Matching portfolio", func(t *testing.T) {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(1033).String(),
			TransactionType:      "WIRE",
			Amount:               decimal.RequireFromString("1000"),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
			Portfolio: &testutil.PortfolioContext{
				ID: matchingPortfolioID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "REVIEW", result.Decision, "Expected REVIEW when portfolio matches rule scope")
		assert.Contains(t, result.MatchedRuleIDs, ruleID, "matchedRuleIds should contain the portfolio-scoped rule")
	})

	// Test 2: Non-matching portfolio - expect ALLOW
	t.Run("Non-matching portfolio", func(t *testing.T) {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(1034).String(),
			TransactionType:      "WIRE",
			Amount:               decimal.RequireFromString("1000"),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
			Portfolio: &testutil.PortfolioContext{
				ID: nonMatchingPortfolioID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "ALLOW", result.Decision, "Expected ALLOW when portfolio does not match rule scope")
		assert.NotContains(t, result.MatchedRuleIDs, ruleID, "matchedRuleIds should NOT contain the portfolio-scoped rule")
	})
}

// Test 1.1.44: Validation with scope-matching rules (transactionType scope)
func TestValidation_1_1_44_ScopeMatchingRulesTransactionType(t *testing.T) {
	cryptoType := "CRYPTO"
	accountID := testutil.MustDeterministicUUID(1040).String()

	// Create and activate DENY rule with transactionType=CRYPTO scope
	ruleName := "deny-crypto-scope-" + testutil.MustDeterministicUUID(1203).String()[:8]
	ruleID := testutil.CreateRuleWithScope(t, ruleName, "amount > 0", "DENY", []testutil.ScopeInput{
		{TransactionType: &cryptoType},
	})
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Test 1: CRYPTO transaction - expect DENY
	t.Run("Matching transaction type CRYPTO", func(t *testing.T) {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(1041).String(),
			TransactionType:      "CRYPTO",
			Amount:               decimal.RequireFromString("100"),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "DENY", result.Decision, "Expected DENY for CRYPTO transaction")
		assert.Contains(t, result.MatchedRuleIDs, ruleID, "matchedRuleIds should contain the transactionType-scoped rule")
	})

	// Test 2: PIX transaction - expect ALLOW
	t.Run("Non-matching transaction type PIX", func(t *testing.T) {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(1042).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("100"),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "ALLOW", result.Decision, "Expected ALLOW for PIX transaction (rule scoped to CRYPTO)")
		assert.NotContains(t, result.MatchedRuleIDs, ruleID, "matchedRuleIds should NOT contain the transactionType-scoped rule")
	})
}

// Test 1.1.45: DENY rule takes precedence over limit exceeded
func TestValidation_1_1_45_DenyRulePrecedenceOverLimitExceeded(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1050).String()

	// Create and activate DENY rule
	ruleName := "deny-high-value-precedence-" + testutil.MustDeterministicUUID(1204).String()[:8]
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "amount > 500", "DENY")
	testutil.ActivateRule(t, ruleID)

	// Create and activate a DAILY limit that would also be exceeded
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
		testutil.CleanupLimit(t, limitID)
	})

	// Consume 500 of the limit first (amount <= 500 to avoid DENY rule)
	firstReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1051).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("500"), // <= 500 to avoid DENY rule, will be ALLOWED
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, firstReq)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "First validation should succeed: %s", string(body1))

	var firstResult testutil.ValidationResponse
	err := json.Unmarshal(body1, &firstResult)
	require.NoError(t, err)
	require.Equal(t, "ALLOW", firstResult.Decision, "First validation must be ALLOWED to consume limit usage")

	// Now send a transaction that exceeds both the rule threshold AND would exceed the limit
	secondReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1052).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("600"), // > 500 (triggers DENY rule) AND 500+600 > 1000 (exceeds limit)
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp2, body2 := testutil.CreateValidation(t, secondReq)
	defer resp2.Body.Close()

	require.Equal(t, http.StatusCreated, resp2.StatusCode, "Expected 201 Created, got: %s", string(body2))

	var result testutil.ValidationResponse
	err = json.Unmarshal(body2, &result)
	require.NoError(t, err)

	assert.Equal(t, "DENY", result.Decision, "Expected DENY")
	// DENY rule should take precedence, so reason should indicate rule match (not limit_exceeded)
	assert.Equal(t, "Rule matched with DENY action", result.Reason, "DENY rule should take precedence over limit exceeded")
	assert.Contains(t, result.MatchedRuleIDs, ruleID, "matchedRuleIds should contain the DENY rule ID")
}

// Test 1.1.46: Validation updates limit usage only on ALLOW
func TestValidation_1_1_46_LimitUsageUpdatedOnlyOnAllow(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1060).String()

	// Create and activate a DAILY limit
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)

	// Create and activate DENY rule for CARD transactions
	ruleName := "deny-all-card-usage-test-" + testutil.MustDeterministicUUID(1205).String()[:8]
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "transactionType == 'CARD'", "DENY")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
		testutil.CleanupLimit(t, limitID)
	})

	// First, create a PIX validation that will be ALLOWED to establish baseline usage
	pixReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1061).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("300"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	pixResp, pixBody := testutil.CreateValidation(t, pixReq)
	defer pixResp.Body.Close()
	require.Equal(t, http.StatusCreated, pixResp.StatusCode, "PIX validation should succeed: %s", string(pixBody))

	var pixResult testutil.ValidationResponse
	err := json.Unmarshal(pixBody, &pixResult)
	require.NoError(t, err)
	require.Equal(t, "ALLOW", pixResult.Decision, "PIX validation should be ALLOW")

	// Get the current usage after PIX (should be 300)
	initialUsage := decimal.RequireFromString("300")

	// Now send a CARD transaction that will be DENIED
	cardReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1062).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("500"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	cardResp, cardBody := testutil.CreateValidation(t, cardReq)
	defer cardResp.Body.Close()

	require.Equal(t, http.StatusCreated, cardResp.StatusCode, "Expected 201 Created, got: %s", string(cardBody))

	var cardResult testutil.ValidationResponse
	err = json.Unmarshal(cardBody, &cardResult)
	require.NoError(t, err)

	assert.Equal(t, "DENY", cardResult.Decision, "CARD validation should be DENY")

	// Now send another PIX validation to check if usage was updated
	checkReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1063).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("10"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	checkResp, checkBody := testutil.CreateValidation(t, checkReq)
	defer checkResp.Body.Close()

	require.Equal(t, http.StatusCreated, checkResp.StatusCode, "Check validation should succeed: %s", string(checkBody))

	var checkResult testutil.ValidationResponse
	err = json.Unmarshal(checkBody, &checkResult)
	require.NoError(t, err)

	// Find our limit in the details
	var foundLimit *testutil.LimitUsageDetail
	for i := range checkResult.LimitUsageDetails {
		if checkResult.LimitUsageDetails[i].LimitID == limitID {
			foundLimit = &checkResult.LimitUsageDetails[i]
			break
		}
	}

	require.NotNil(t, foundLimit, "limitUsageDetails should contain our limit")
	// The key check is that the CARD transaction's 500 was NOT added
	expectedUsage := initialUsage.Add(decimal.RequireFromString("10")) // PIX (300) + check request (10)
	assert.True(t, foundLimit.CurrentUsage.Equal(expectedUsage),
		"Usage should exclude the DENIED CARD transaction and include the PIX check; expected %s, got %s",
		expectedUsage, foundLimit.CurrentUsage)
}

// Test 1.1.47: Validation rejects amount exceeding CEL float64 safe precision (2^53).
// The CEL expression engine converts decimal amounts to float64, which loses precision
// beyond 2^53. The precision guard rejects such amounts to prevent silent rounding errors.
// A rule must be active so the amount goes through CEL evaluation (no rules → no CEL).
// Returns HTTP 400 Bad Request with error code TRC-0089.
func TestValidation_1_1_47_RejectsAmountExceedingCELPrecision(t *testing.T) {
	// Create and activate a rule so the validation path invokes CEL.
	// The expression "amount > 0" matches any positive amount, forcing CEL evaluation.
	ruleName := "cel-precision-guard-test-" + testutil.MustDeterministicUUID(1209).String()[:8]
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "amount > 0", "DENY")
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })
	testutil.ActivateRule(t, ruleID)

	accountID := testutil.MustDeterministicUUID(1070).String()

	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// 9223372036854775808 is int64 max + 1, which also exceeds 2^53 (9007199254740992),
	// the maximum safe integer for float64. The CEL precision guard rejects this.
	rawJSON := fmt.Sprintf(`{
		"requestId": "%s",
		"transactionType": "CARD",
		"amount": "9223372036854775808",
		"currency": "BRL",
		"transactionTimestamp": "%s",
		"account": {"accountId": "%s"}
	}`, testutil.MustDeterministicUUID(1071).String(), testutil.FixedTime().UTC().Format(time.RFC3339), accountID)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader([]byte(rawJSON)))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Amount exceeding CEL precision (2^53) should return 400: %s", string(body))

	var errResp map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &errResp), "Response body should be valid JSON")
	assert.Equal(t, "0346", errResp["code"], "Error code should be TRC-0089 (amount exceeds precision)")
	assert.Equal(t, "Bad Request", errResp["title"])
}

// Test 1.1.47b: Validation accepts the maximum safe amount for CEL evaluation (2^53).
// 9007199254740992 is exactly 2^53, the boundary for float64 integer precision.
// Amounts at or below this threshold are safe for CEL's float64 arithmetic.
func TestValidation_1_1_47b_AcceptsMaxSafeCELAmount(t *testing.T) {
	// Create and activate a rule so the validation path invokes CEL and the precision guard.
	// Without this, the request would be ALLOW'd without ever reaching CEL evaluation.
	ruleName := "cel-precision-boundary-test-" + testutil.MustDeterministicUUID(1210).String()[:8]
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "amount > 0", "DENY")
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })
	testutil.ActivateRule(t, ruleID)

	accountID := testutil.MustDeterministicUUID(1072).String()

	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// 2^53 = 9007199254740992 — the largest integer that float64 represents exactly.
	rawJSON := fmt.Sprintf(`{
		"requestId": "%s",
		"transactionType": "CARD",
		"amount": "9007199254740992",
		"currency": "BRL",
		"transactionTimestamp": "%s",
		"account": {"accountId": "%s"}
	}`, testutil.MustDeterministicUUID(1073).String(), testutil.FixedTime().UTC().Format(time.RFC3339), accountID)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader([]byte(rawJSON)))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Amount at CEL precision boundary (2^53) should pass the precision guard: %s", string(body))

	// Verify CEL actually evaluated the expression (decision DENY proves the amount
	// went through CEL evaluation, not just a no-rules pass-through).
	var result testutil.ValidationResponse
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "DENY", result.Decision,
		"Rule 'amount > 0' should match and DENY, proving CEL evaluated the boundary amount")
	assert.Contains(t, result.MatchedRuleIDs, ruleID,
		"matchedRuleIds should contain the CEL precision boundary rule")
}

// Test 1.1.48: Validation with empty metadata object
func TestValidation_1_1_48_EmptyMetadataObject(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1080).String()
	requestID := testutil.MustDeterministicUUID(1081).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		Metadata: map[string]any{}, // Empty metadata object
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Empty metadata should be accepted: %s", string(body))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body, &result)
	require.NoError(t, err)

	assert.Contains(t, []string{"ALLOW", "DENY", "REVIEW"}, result.Decision, "Decision should be valid")
}

// Test 1.1.49: Validation with null optional fields
func TestValidation_1_1_49_NullOptionalFields(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1090).String()

	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	rawJSON := fmt.Sprintf(`{
		"requestId": "%s",
		"transactionType": "CARD",
		"subType": null,
		"amount": "100.00",
		"currency": "BRL",
		"transactionTimestamp": "%s",
		"account": {"accountId": "%s"},
		"segment": null,
		"portfolio": null,
		"merchant": null,
		"metadata": null
	}`, testutil.MustDeterministicUUID(1091).String(), testutil.FixedTime().UTC().Format(time.RFC3339), accountID)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader([]byte(rawJSON)))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Null optional fields should be accepted: %s", string(respBody))

	var result testutil.ValidationResponse
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Contains(t, []string{"ALLOW", "DENY", "REVIEW"}, result.Decision, "Decision should be valid")
}

// Test 1.1.50: Validation with very old timestamp
func TestValidation_1_1_50_VeryOldTimestamp(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1100).String()
	requestID := testutil.MustDeterministicUUID(1101).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: "2020-01-01T00:00:00Z", // Very old timestamp
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Very old timestamp should be rejected (max age 24h): %s", string(body))

	errorResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "0421", errorResp.Code,
		"Error code should be TRC-0228 for timestamp too far in the past")
}

// Test 1.1.51: Validation rejects lowercase currency (ISO 4217 requires uppercase)
func TestValidation_1_1_51_LowercaseCurrencyRejected(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1110).String()
	requestID := testutil.MustDeterministicUUID(1111).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "brl", // Lowercase currency - ISO 4217 specifies uppercase
		TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	// ISO 4217 currency codes are canonically uppercase (USD, EUR, BRL)
	// API enforces strict validation - lowercase is rejected
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Lowercase currency should be rejected (ISO 4217 requires uppercase)")

	// Verify structured error response
	errResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "0417", errResp.Code, "Error response should have invalid currency error code")
	assert.Equal(t, "Validation Error", errResp.Title, "Error response should have validation error title")
	assert.Equal(t, "currency must be valid ISO 4217 code (e.g., BRL, USD)", errResp.Message, "Error message should indicate valid currency format")
}

// Test 1.1.52: Duplicate requestId returns cached response (idempotent behavior)
func TestValidation_1_1_52_UniqueValidationIdPerRequest(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1120).String()
	requestID := testutil.MustDeterministicUUID(1121).String() // Same requestId for both

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	// First request — new validation
	resp1, body1 := testutil.CreateValidation(t, req)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "First request should return 201 Created: %s", string(body1))

	var result1 testutil.ValidationResponse
	err := json.Unmarshal(body1, &result1)
	require.NoError(t, err)

	// Second request with same requestId — duplicate, returns cached response
	resp2, body2 := testutil.CreateValidation(t, req)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusOK, resp2.StatusCode, "Duplicate request should return 200 OK: %s", string(body2))

	var result2 testutil.ValidationResponse
	err = json.Unmarshal(body2, &result2)
	require.NoError(t, err)

	// Verify both have the same requestId
	assert.Equal(t, requestID, result1.RequestID, "First response should echo requestId")
	assert.Equal(t, requestID, result2.RequestID, "Second response should echo requestId")

	// With idempotency, duplicate returns the same validationId (cached response)
	assert.Equal(t, result1.ValidationID, result2.ValidationID,
		"Duplicate request should return the same validationId (cached)")

	// Verify cached response is identical in all stable fields
	assert.Equal(t, result1.Decision, result2.Decision, "Cached decision must match")
	assert.Equal(t, result1.Reason, result2.Reason, "Cached reason must match")
	assert.Equal(t, result1.MatchedRuleIDs, result2.MatchedRuleIDs, "Cached matchedRuleIds must match")
	assert.Equal(t, result1.EvaluatedRuleIDs, result2.EvaluatedRuleIDs, "Cached evaluatedRuleIds must match")
	assert.Equal(t, result1.LimitUsageDetails, result2.LimitUsageDetails, "Cached limitUsageDetails must match")

	// Verify validationId is different from requestId
	assert.NotEqual(t, result1.ValidationID, requestID,
		"validationId should be different from requestId")
}

// Test 1.1.53: Idempotent behavior — duplicate requestId returns cached result
func TestValidation_1_1_53_IdempotentBehavior(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1130).String()

	// Create and activate a DAILY limit
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	requestID := testutil.MustDeterministicUUID(1131).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("600"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	// First request - should ALLOW and consume limit
	resp1, body1 := testutil.CreateValidation(t, req)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "First request should return 201 Created: %s", string(body1))

	var result1 testutil.ValidationResponse
	err := json.Unmarshal(body1, &result1)
	require.NoError(t, err)

	assert.Equal(t, "ALLOW", result1.Decision, "First request should be ALLOW")

	// Second request with same requestId — duplicate, returns cached ALLOW (no double-count)
	resp2, body2 := testutil.CreateValidation(t, req)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusOK, resp2.StatusCode, "Duplicate request should return 200 OK: %s", string(body2))

	var result2 testutil.ValidationResponse
	err = json.Unmarshal(body2, &result2)
	require.NoError(t, err)

	assert.Equal(t, "ALLOW", result2.Decision,
		"Duplicate request should return cached ALLOW (idempotent, no double-count)")

	// With idempotency, duplicate returns the same validationId
	assert.Equal(t, result1.ValidationID, result2.ValidationID,
		"Duplicate request should return the same validationId (cached)")

	// Follow-up: prove no double-count by consuming remaining headroom (400 of 1000).
	// If the duplicate had consumed 600 again, this 400 would exceed 1000 and DENY.
	followUpReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1132).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("400"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp3, body3 := testutil.CreateValidation(t, followUpReq)
	defer resp3.Body.Close()
	require.Equal(t, http.StatusCreated, resp3.StatusCode, "Follow-up should return 201: %s", string(body3))

	var result3 testutil.ValidationResponse
	err = json.Unmarshal(body3, &result3)
	require.NoError(t, err)

	assert.Equal(t, "ALLOW", result3.Decision,
		"Follow-up 400 should ALLOW (usage=600+400=1000, proves duplicate did not double-count)")
}

// Test 1.1.54: Validation with all rule actions matching
func TestValidation_1_1_54_AllRuleActionsMatching(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1140).String()

	// Create DENY rule
	denyRuleName := "deny-all-actions-test-" + testutil.MustDeterministicUUID(1206).String()[:8]
	denyRuleID := testutil.CreateTestRuleWithExpression(t, denyRuleName, "amount > 100", "DENY")
	testutil.ActivateRule(t, denyRuleID)

	// Create REVIEW rule
	reviewRuleName := "review-all-actions-test-" + testutil.MustDeterministicUUID(1207).String()[:8]
	reviewRuleID := testutil.CreateTestRuleWithExpression(t, reviewRuleName, "amount > 50", "REVIEW")
	testutil.ActivateRule(t, reviewRuleID)

	// Create ALLOW rule
	allowRuleName := "allow-all-actions-test-" + testutil.MustDeterministicUUID(1208).String()[:8]
	allowRuleID := testutil.CreateTestRuleWithExpression(t, allowRuleName, "amount > 0", "ALLOW")
	testutil.ActivateRule(t, allowRuleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, denyRuleID)
		testutil.CleanupRule(t, reviewRuleID)
		testutil.CleanupRule(t, allowRuleID)
	})

	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1141).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("150"), // Matches all 3 rules
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().UTC().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body, &result)
	require.NoError(t, err)

	// DENY should take precedence
	assert.Equal(t, "DENY", result.Decision,
		"DENY should take precedence over REVIEW and ALLOW")

	// All 3 rules should be in matchedRuleIds
	assert.Contains(t, result.MatchedRuleIDs, denyRuleID,
		"matchedRuleIds should contain DENY rule")
	assert.Contains(t, result.MatchedRuleIDs, reviewRuleID,
		"matchedRuleIds should contain REVIEW rule")
	assert.Contains(t, result.MatchedRuleIDs, allowRuleID,
		"matchedRuleIds should contain ALLOW rule")

	assert.Len(t, result.MatchedRuleIDs, 3,
		"matchedRuleIds should contain exactly 3 rules")
}
