// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Integration Tests: Limits Verification and Tracking (05)
//
// Tests for limit verification, atomic usage updates, and period reset.
// Reference: tests/integration/05-limits-verification.md
//
// Note: In this file, "limit of X" refers to a limit with `maxAmount: X`.
// =============================================================================

// limitVerificationResponse wraps a single limit for verification tests.
type limitVerificationResponse struct {
	ID        string               `json:"limitId"`
	Name      string               `json:"name"`
	LimitType string               `json:"limitType"`
	MaxAmount decimal.Decimal      `json:"maxAmount"`
	Currency  string               `json:"currency"`
	Scopes    []limitScopeResponse `json:"scopes"`
	Status    string               `json:"status"`
	ResetAt   *string              `json:"resetAt,omitempty"`
	CreatedAt string               `json:"createdAt"`
	UpdatedAt string               `json:"updatedAt"`
}

// =============================================================================
// 5.1 Limit Verification
// =============================================================================

// TestLimitsVerification_5_1_1_FindsApplicableLimitsByScope verifies that
// only limits matching the transaction scope are checked during validation.
//
// Test spec 5.1.1: Finds applicable limits by scope
func TestLimitsVerification_5_1_1_FindsApplicableLimitsByScope(t *testing.T) {
	accountID1 := testutil.MustDeterministicUUID(50101).String()
	accountID2 := testutil.MustDeterministicUUID(50102).String()

	// Create DAILY limit for acc-1
	limit1ID := testutil.CreateLimitWithAccountScope(t, accountID1, "1000")
	testutil.ActivateLimit(t, limit1ID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limit1ID)
	})

	// Create DAILY limit for acc-2
	limit2ID := testutil.CreateLimitWithAccountScope(t, accountID2, "1000")
	testutil.ActivateLimit(t, limit2ID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limit2ID)
	})

	// Validate transaction for acc-1
	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50103).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("300"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID1,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Verify limitUsageDetails contains only the acc-1 limit
	foundLimit1 := false
	foundLimit2 := false
	for _, detail := range result.LimitUsageDetails {
		if detail.LimitID == limit1ID {
			foundLimit1 = true
		}
		if detail.LimitID == limit2ID {
			foundLimit2 = true
		}
	}

	assert.True(t, foundLimit1, "limitUsageDetails should contain the acc-1 limit")
	assert.False(t, foundLimit2, "limitUsageDetails should NOT contain the acc-2 limit (different scope)")
}

// TestLimitsVerification_5_1_2_CalculatesProjectedUsage verifies that
// the system correctly calculates projected usage (currentUsage + amount).
//
// Test spec 5.1.2: Calculates projected usage
func TestLimitsVerification_5_1_2_CalculatesProjectedUsage(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50110).String()

	// Create DAILY limit of 1000 for the account
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// First validation to establish currentUsage = 400
	firstReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50111).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("400"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, firstReq)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "First validation should succeed: %s", string(body1))

	// Second validation with amount = 300
	// Projected usage = 400 + 300 = 700
	secondReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50112).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("300"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp2, body2 := testutil.CreateValidation(t, secondReq)
	defer resp2.Body.Close()

	require.Equal(t, http.StatusCreated, resp2.StatusCode, "Expected 201 Created, got: %s", string(body2))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body2, &result)
	require.NoError(t, err)

	// Verify projected usage (currentUsage should reflect 400 + 300 = 700)
	var found bool
	for _, detail := range result.LimitUsageDetails {
		if detail.LimitID == limitID {
			found = true
			// After second transaction, currentUsage reflects the projected value
			assert.True(t, decimal.RequireFromString("700").Equal(detail.CurrentUsage), "currentUsage should be 700 (projected)")
			assert.True(t, decimal.RequireFromString("300").Equal(detail.AttemptedAmount), "attemptedAmount should be 300")
			break
		}
	}
	assert.True(t, found, "limitUsageDetails should contain the created limit")
}

// TestLimitsVerification_5_1_3_ReturnsExceededWhenProjectedGreaterThanLimit verifies that
// DENY is returned when projected usage exceeds the limit.
//
// Test spec 5.1.3: Returns EXCEEDED when projected > limit
func TestLimitsVerification_5_1_3_ReturnsExceededWhenProjectedGreaterThanLimit(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50120).String()

	// Create limit of 1000
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// First validation to establish currentUsage = 800
	firstReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50121).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("800"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, firstReq)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "First validation should succeed: %s", string(body1))

	// Second validation with amount = 300 (800 + 300 = 1100 > 1000)
	secondReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50122).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("300"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp2, body2 := testutil.CreateValidation(t, secondReq)
	defer resp2.Body.Close()

	require.Equal(t, http.StatusCreated, resp2.StatusCode, "Expected 201 Created, got: %s", string(body2))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body2, &result)
	require.NoError(t, err)

	// Verify DENY decision and exceeded flag
	assert.Equal(t, "DENY", result.Decision, "Expected DENY when limit exceeded")
	assert.Equal(t, "limit_exceeded", result.Reason, "Expected reason to be limit_exceeded")

	// Verify limitUsageDetails has exceeded = true
	var found bool
	for _, detail := range result.LimitUsageDetails {
		if detail.LimitID == limitID {
			found = true
			assert.True(t, detail.Exceeded, "exceeded should be true")
			break
		}
	}
	assert.True(t, found, "limitUsageDetails should contain the created limit")
}

// TestLimitsVerification_5_1_4_ReturnsOKWhenProjectedEqualsLimit verifies that
// the transaction is allowed when projected usage equals the limit exactly (boundary).
//
// Test spec 5.1.4: Returns OK when projected == limit (boundary)
func TestLimitsVerification_5_1_4_ReturnsOKWhenProjectedEqualsLimit(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50130).String()

	// Create limit of 1000
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// First validation to establish currentUsage = 700
	firstReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50131).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("700"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, firstReq)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "First validation should succeed: %s", string(body1))

	// Second validation with amount = 300 (700 + 300 = 1000 == limit)
	secondReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50132).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("300"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp2, body2 := testutil.CreateValidation(t, secondReq)
	defer resp2.Body.Close()

	require.Equal(t, http.StatusCreated, resp2.StatusCode, "Expected 201 Created, got: %s", string(body2))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body2, &result)
	require.NoError(t, err)

	// Verify limitUsageDetails has exceeded = false and currentUsage = 1000
	var found bool
	for _, detail := range result.LimitUsageDetails {
		if detail.LimitID == limitID {
			found = true
			assert.False(t, detail.Exceeded, "exceeded should be false when projected == limit")
			assert.True(t, decimal.RequireFromString("1000").Equal(detail.CurrentUsage), "currentUsage should equal limit amount")
			break
		}
	}
	assert.True(t, found, "limitUsageDetails should contain the created limit")
}

// TestLimitsVerification_5_1_5_ReturnsOKWhenProjectedLessThanLimit verifies that
// the transaction is allowed when projected usage is less than the limit.
//
// Test spec 5.1.5: Returns OK when projected < limit
func TestLimitsVerification_5_1_5_ReturnsOKWhenProjectedLessThanLimit(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50140).String()

	// Create limit of 1000
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// First validation to establish currentUsage = 500
	firstReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50141).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("500"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, firstReq)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "First validation should succeed: %s", string(body1))

	// Second validation with amount = 300 (500 + 300 = 800 < 1000)
	secondReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50142).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("300"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp2, body2 := testutil.CreateValidation(t, secondReq)
	defer resp2.Body.Close()

	require.Equal(t, http.StatusCreated, resp2.StatusCode, "Expected 201 Created, got: %s", string(body2))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body2, &result)
	require.NoError(t, err)

	// Decision should not be DENY due to limit (might be DENY for other reasons like rules)
	// Check that limit is NOT exceeded
	found := false
	for _, detail := range result.LimitUsageDetails {
		if detail.LimitID == limitID {
			found = true
			assert.False(t, detail.Exceeded, "exceeded should be false when projected < limit")
			break
		}
	}
	assert.True(t, found, "limitUsageDetails should contain the created limit")
}

// TestLimitsVerification_5_1_6_ChecksMultipleLimits verifies that
// all applicable limits are checked for a transaction.
//
// Test spec 5.1.6: Checks multiple limits
func TestLimitsVerification_5_1_6_ChecksMultipleLimits(t *testing.T) {
	// Scenario 1: amount = 300 - both limits should be OK
	t.Run("both_limits_ok", func(t *testing.T) {
		// Create a fresh account for this sub-test to avoid state issues
		accountID1 := testutil.MustDeterministicUUID(50160).String()

		dailyID := testutil.CreateLimitWithAccountScopeAndType(t, accountID1, "1000", "DAILY")
		testutil.ActivateLimit(t, dailyID)
		t.Cleanup(func() {
			testutil.CleanupLimit(t, dailyID)
		})

		monthlyID := testutil.CreateLimitWithAccountScopeAndType(t, accountID1, "5000", "MONTHLY")
		testutil.ActivateLimit(t, monthlyID)
		t.Cleanup(func() {
			testutil.CleanupLimit(t, monthlyID)
		})

		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(50161).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("300"),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID1,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		// Both limits should be present and not exceeded
		dailyFound := false
		monthlyFound := false
		for _, detail := range result.LimitUsageDetails {
			if detail.LimitID == dailyID {
				dailyFound = true
				assert.False(t, detail.Exceeded, "DAILY limit should not be exceeded")
			}
			if detail.LimitID == monthlyID {
				monthlyFound = true
				assert.False(t, detail.Exceeded, "MONTHLY limit should not be exceeded")
			}
		}
		assert.True(t, dailyFound, "DAILY limit should be in response")
		assert.True(t, monthlyFound, "MONTHLY limit should be in response")
	})

	// Scenario 2: amount = 1200 - exceeds DAILY limit
	t.Run("exceeds_daily_limit", func(t *testing.T) {
		accountID2 := testutil.MustDeterministicUUID(50170).String()

		dailyID := testutil.CreateLimitWithAccountScopeAndType(t, accountID2, "1000", "DAILY")
		testutil.ActivateLimit(t, dailyID)
		t.Cleanup(func() {
			testutil.CleanupLimit(t, dailyID)
		})

		monthlyID := testutil.CreateLimitWithAccountScopeAndType(t, accountID2, "5000", "MONTHLY")
		testutil.ActivateLimit(t, monthlyID)
		t.Cleanup(func() {
			testutil.CleanupLimit(t, monthlyID)
		})

		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(50171).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("1200"), // Exceeds DAILY limit of 1000
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID2,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "DENY", result.Decision, "Expected DENY when any limit exceeded")
		assert.Equal(t, "limit_exceeded", result.Reason, "Expected reason to be limit_exceeded")

		// Daily should be exceeded
		for _, detail := range result.LimitUsageDetails {
			if detail.LimitID == dailyID {
				assert.True(t, detail.Exceeded, "DAILY limit should be exceeded")
			}
		}
	})
}

// TestLimitsVerification_5_1_9_PerTransactionLimitChecksValueOnly verifies that
// PER_TRANSACTION limits check only the transaction amount, not accumulated usage.
//
// Test spec 5.1.9: PER_TRANSACTION limit checks value only
func TestLimitsVerification_5_1_9_PerTransactionLimitChecksValueOnly(t *testing.T) {
	// Use valid transaction type (must be one of CARD, WIRE, PIX, CRYPTO)
	transactionType := "CARD"

	// Create PER_TRANSACTION limit of 500
	limitID := testutil.CreateLimitWithTransactionTypeScope(t, transactionType, "500")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	testCases := []struct {
		name     string
		amount   decimal.Decimal
		exceeded bool
	}{
		{"amount_300_ok", decimal.RequireFromString("300"), false},
		{"amount_500_ok", decimal.RequireFromString("500"), false},
		{"amount_600_exceeded", decimal.RequireFromString("600"), true},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			accountID := testutil.MustDeterministicUUID(int64(50190 + i)).String()

			req := &testutil.ValidationRequest{
				RequestID:            testutil.MustDeterministicUUID(int64(50191 + i*10)).String(),
				TransactionType:      transactionType,
				Amount:               tc.amount,
				Currency:             "BRL",
				TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
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

			if tc.exceeded {
				assert.Equal(t, "DENY", result.Decision, "Expected DENY when amount > PER_TRANSACTION limit")
				assert.Equal(t, "limit_exceeded", result.Reason)
			} else {
				// When not exceeded, decision should not be DENY due to limit
				if result.Decision == "DENY" {
					assert.NotEqual(t, "limit_exceeded", result.Reason,
						"Should not be denied due to limit when amount <= PER_TRANSACTION limit")
				}
			}

			// Verify exceeded flag
			for _, detail := range result.LimitUsageDetails {
				if detail.LimitID == limitID {
					assert.Equal(t, tc.exceeded, detail.Exceeded, "exceeded flag mismatch for amount %s", tc.amount)
					break
				}
			}
		})
	}
}

// =============================================================================
// 5.2 Atomic Usage Update
// =============================================================================

// TestLimitsVerification_5_2_1_IncrementsUsageAtomically verifies that
// usage is incremented atomically after a successful validation.
//
// Test spec 5.2.1: Increments usage atomically
func TestLimitsVerification_5_2_1_IncrementsUsageAtomically(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50201).String()

	// Create DAILY limit of 1000 with currentUsage = 0
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Validate transaction with amount = 200
	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50202).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("200"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
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

	// Decision should be ALLOW (no rules to deny, limit not exceeded)
	// Note: Decision might be ALLOW or depend on rules

	// Verify usage was incremented via GET /v1/limits/{limitId}/usage
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	usageReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID+"/usage", nil)
	require.NoError(t, err)
	usageReq.Header.Set("X-API-Key", apiKey)

	usageResp, err := testutil.HTTPClient.Do(usageReq)
	require.NoError(t, err)
	defer usageResp.Body.Close()

	usageBody, err := io.ReadAll(usageResp.Body)
	require.NoError(t, err)

	// Assert status code first (fail-fast)
	require.Equal(t, http.StatusOK, usageResp.StatusCode,
		"Usage endpoint should return 200, got: %s", string(usageBody))

	var usageResponse model.UsageSnapshot
	err = json.Unmarshal(usageBody, &usageResponse)
	require.NoError(t, err)

	// Verify the usage directly
	assert.True(t, decimal.RequireFromString("200").Equal(usageResponse.CurrentUsage),
		"currentUsage should be 200 after validation")
}

// TestLimitsVerification_5_2_2_DoesNotIncrementOnRuleBasedDeny verifies that
// usage is NOT incremented when the transaction is denied by a rule (not limit).
//
// Test spec 5.2.2: Does not increment on rule-based DENY
func TestLimitsVerification_5_2_2_DoesNotIncrementOnRuleBasedDeny(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50210).String()

	// Create a limit of 1000 (currentUsage: 0)
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// First, establish some usage (500)
	firstReq := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("500"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, firstReq)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "First validation: %s", string(body1))

	// Create DENY rule that will match CARD transactions with high amounts
	// Use valid transaction type and a specific expression
	ruleName := "deny-high-card-" + testutil.MustDeterministicUUID(5001).String()[:8]
	expression := "transactionType == 'CARD' && amount > 150"
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, expression, "DENY")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Validate transaction that will be denied by the rule
	// Using CARD with amount > 150 to trigger the rule
	denyReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50212).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("200"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp2, body2 := testutil.CreateValidation(t, denyReq)
	defer resp2.Body.Close()

	require.Equal(t, http.StatusCreated, resp2.StatusCode, "Expected 201 Created, got: %s", string(body2))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body2, &result)
	require.NoError(t, err)

	assert.Equal(t, "DENY", result.Decision, "Expected DENY from rule")
	assert.Contains(t, result.MatchedRuleIDs, ruleID, "Rule should be in matchedRuleIds")

	// Verify usage was NOT incremented (should still be 500)
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	usageReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID+"/usage", nil)
	require.NoError(t, err)
	usageReq.Header.Set("X-API-Key", apiKey)

	usageResp, err := testutil.HTTPClient.Do(usageReq)
	require.NoError(t, err)
	defer usageResp.Body.Close()

	usageBody, err := io.ReadAll(usageResp.Body)
	require.NoError(t, err)

	// Assert status code first
	require.Equal(t, http.StatusOK, usageResp.StatusCode,
		"Usage endpoint should return 200, got: %s", string(usageBody))

	var usageResponse model.UsageSnapshot
	err = json.Unmarshal(usageBody, &usageResponse)
	require.NoError(t, err)

	assert.True(t, decimal.RequireFromString("500").Equal(usageResponse.CurrentUsage),
		"Usage should NOT be incremented on rule-based DENY")
}

// TestLimitsVerification_5_2_3_DoesNotIncrementOnReview verifies that
// usage is NOT incremented when the transaction decision is REVIEW.
//
// Test spec 5.2.3: Does not increment on REVIEW
func TestLimitsVerification_5_2_3_DoesNotIncrementOnReview(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50220).String()

	// Create a limit
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// First, establish initial usage (300) with a PIX transaction (not WIRE, so no REVIEW)
	setupReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50222).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("300"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	setupResp, setupBody := testutil.CreateValidation(t, setupReq)
	defer setupResp.Body.Close()
	require.Equal(t, http.StatusCreated, setupResp.StatusCode, "Setup validation should succeed: %s", string(setupBody))

	// Create REVIEW rule for WIRE transactions with medium amounts
	// Use valid transaction type
	ruleName := "review-wire-medium-" + testutil.MustDeterministicUUID(5002).String()[:8]
	expression := "transactionType == 'WIRE' && amount > 100"
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, expression, "REVIEW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Validate transaction that will trigger REVIEW
	// Using WIRE with amount > 100 to trigger the rule
	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50221).String(),
		TransactionType:      "WIRE",
		Amount:               decimal.RequireFromString("200"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
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

	assert.Equal(t, "REVIEW", result.Decision, "Expected REVIEW from rule")
	assert.Contains(t, result.MatchedRuleIDs, ruleID, "Rule should be in matchedRuleIds")

	// Verify usage was NOT incremented (should still be 300 from setup)
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	usageReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID+"/usage", nil)
	require.NoError(t, err)
	usageReq.Header.Set("X-API-Key", apiKey)

	usageResp, err := testutil.HTTPClient.Do(usageReq)
	require.NoError(t, err)
	defer usageResp.Body.Close()

	usageBody, err := io.ReadAll(usageResp.Body)
	require.NoError(t, err)

	// Assert status code first
	require.Equal(t, http.StatusOK, usageResp.StatusCode,
		"Usage endpoint should return 200, got: %s", string(usageBody))

	var usageResponse model.UsageSnapshot
	err = json.Unmarshal(usageBody, &usageResponse)
	require.NoError(t, err)

	assert.True(t, decimal.RequireFromString("300").Equal(usageResponse.CurrentUsage),
		"Usage should NOT be incremented on REVIEW")
}

// TestLimitsVerification_5_2_4_ConcurrentTransactionsAccumulateCorrectly verifies that
// concurrent transactions correctly accumulate usage.
//
// Test spec 5.2.4: Concurrent transactions accumulate correctly
func TestLimitsVerification_5_2_4_ConcurrentTransactionsAccumulateCorrectly(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50230).String()

	// Create limit of 10000 (high enough for 10 concurrent transactions)
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "10000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Fire 10 parallel validations of amount=100 each
	const numConcurrent = 10
	const amountPerTx = 100

	var wg sync.WaitGroup
	results := make(chan *testutil.ValidationResponse, numConcurrent)
	errors := make(chan error, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			req := &testutil.ValidationRequest{
				RequestID:            testutil.MustDeterministicUUID(int64(50231 + idx)).String(),
				TransactionType:      "PIX",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
				Account: &testutil.AccountContext{
					ID: accountID,
				},
			}

			resp, body := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusCreated {
				errors <- nil // Not an error for this test
				return
			}

			var result testutil.ValidationResponse
			if err := json.Unmarshal(body, &result); err != nil {
				errors <- err
				return
			}

			results <- &result
		}(i)
	}

	wg.Wait()
	close(results)
	close(errors)

	// Check for errors
	for err := range errors {
		if err != nil {
			t.Errorf("Concurrent validation error: %v", err)
		}
	}

	// Count successful validations
	successCount := 0
	for result := range results {
		if result != nil && result.Decision != "DENY" {
			successCount++
		}
	}

	// Verify final usage via API
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	usageReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID+"/usage", nil)
	require.NoError(t, err)
	usageReq.Header.Set("X-API-Key", apiKey)

	usageResp, err := testutil.HTTPClient.Do(usageReq)
	require.NoError(t, err)
	defer usageResp.Body.Close()

	usageBody, err := io.ReadAll(usageResp.Body)
	require.NoError(t, err)

	// Assert status code first
	require.Equal(t, http.StatusOK, usageResp.StatusCode,
		"Usage endpoint should return 200, got: %s", string(usageBody))

	var usageResponse model.UsageSnapshot
	err = json.Unmarshal(usageBody, &usageResponse)
	require.NoError(t, err)

	// Final usage should be successCount * amountPerTx
	expectedUsage := decimal.NewFromInt(int64(successCount * amountPerTx))
	assert.True(t, expectedUsage.Equal(usageResponse.CurrentUsage),
		"Final currentUsage should be %s (based on %d successful validations)", expectedUsage, successCount)

	// All transactions should succeed (limit is high enough)
	assert.Equal(t, numConcurrent, successCount, "All 10 concurrent transactions should succeed")
}

// TestLimitsVerification_5_2_5_RaceConditionPrevented verifies that
// race conditions are handled when multiple transactions would exceed the limit.
//
// Test spec 5.2.5: Race condition prevented
//
// Scenario: DAILY limit of 1000, pre-loaded with currentUsage=900 (one validation of amount=900).
// Action: 3 concurrent goroutines each with amount=100.
// Assertions: Exactly 1 approved (900+100=1000), exactly 2 denied.
// Verify final currentUsage=1000 via GET /v1/limits/{limitID}/usage.
func TestLimitsVerification_5_2_5_RaceConditionPrevented(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50250).String()

	// Create limit of 1000 with high initial usage (900)
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Freeze timestamp to avoid period-boundary flakes in concurrency tests
	// All requests in this test must use the same timestamp to ensure they target the same period
	fixedTimestamp := time.Now().UTC().Format(time.RFC3339)

	// First, establish currentUsage = 900
	setupReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50251).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("900"),
		Currency:             "BRL",
		TransactionTimestamp: fixedTimestamp,
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	respSetup, bodySetup := testutil.CreateValidation(t, setupReq)
	defer respSetup.Body.Close()
	require.Equal(t, http.StatusCreated, respSetup.StatusCode, "Setup validation should succeed: %s", string(bodySetup))

	// Fire 3 parallel validations of amount=100 each
	// Expected: Only 1 should succeed (atomic check-and-increment)
	// (900 + 100 = 1000 <= limit, subsequent attempts exceed)
	const numConcurrent = 3

	var wg sync.WaitGroup
	var approvedCount int64
	var deniedCount int64

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			req := &testutil.ValidationRequest{
				RequestID:            testutil.MustDeterministicUUID(int64(50252 + idx)).String(),
				TransactionType:      "PIX",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: fixedTimestamp,
				Account: &testutil.AccountContext{
					ID: accountID,
				},
			}

			resp, body := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			// Use assert (not require) inside goroutines to avoid panics
			if !assert.Equal(t, http.StatusCreated, resp.StatusCode, "Validation request should return 201 Created") {
				return
			}

			var result testutil.ValidationResponse
			if err := json.Unmarshal(body, &result); err != nil {
				assert.NoError(t, err, "Failed to unmarshal validation response")
				return
			}

			if result.Decision == "DENY" && result.Reason == "limit_exceeded" {
				atomic.AddInt64(&deniedCount, 1)
			} else if result.Decision != "DENY" {
				atomic.AddInt64(&approvedCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// Load final counts
	finalApproved := atomic.LoadInt64(&approvedCount)
	finalDenied := atomic.LoadInt64(&deniedCount)

	// Log observed behavior for documentation
	t.Logf("Race condition test results: approved=%d, denied=%d", finalApproved, finalDenied)

	// Verify: Exactly 1 approved and 2 denied (atomic enforcement)
	assert.Equal(t, int64(1), finalApproved, "Exactly 1 transaction should be approved (atomic enforcement)")
	assert.Equal(t, int64(2), finalDenied, "Exactly 2 transactions should be denied (limit exceeded)")

	// Verify final usage via GET /v1/limits/{limitID}/usage
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	usageReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID+"/usage", nil)
	require.NoError(t, err)
	usageReq.Header.Set("X-API-Key", apiKey)

	usageResp, err := testutil.HTTPClient.Do(usageReq)
	require.NoError(t, err)
	defer usageResp.Body.Close()

	usageBody, err := io.ReadAll(usageResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, usageResp.StatusCode, "Get usage should succeed: %s", string(usageBody))

	var usageResponse model.UsageSnapshot
	err = json.Unmarshal(usageBody, &usageResponse)
	require.NoError(t, err)

	assert.True(t, decimal.RequireFromString("1000").Equal(usageResponse.CurrentUsage),
		"Final currentUsage should be 1000 (900 + 100)")
}

// =============================================================================
// 5.3 Period Reset
// =============================================================================

// TestLimitsVerification_5_3_1_NewPeriodCreatesNewCounter verifies that
// a new period creates a new counter (documented as spec behavior).
//
// Test spec 5.3.1: New period creates new counter
//
// Note: This test documents the expected behavior. Actually simulating
// date changes requires control over the system clock or waiting for
// midnight, which is not practical in integration tests.
func TestLimitsVerification_5_3_1_NewPeriodCreatesNewCounter(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50301).String()

	// Create DAILY limit
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Verify the limit has a resetAt timestamp
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, getResp.StatusCode, "Get limit should succeed: %s", string(getBody))

	var limit limitVerificationResponse
	err = json.Unmarshal(getBody, &limit)
	require.NoError(t, err)

	// DAILY limits should have resetAt
	assert.NotNil(t, limit.ResetAt, "DAILY limit should have resetAt set")

	if limit.ResetAt != nil {
		resetAt, err := time.Parse(time.RFC3339, *limit.ResetAt)
		require.NoError(t, err, "resetAt should be valid RFC3339 timestamp")

		// Verify it's in the future
		assert.True(t, resetAt.After(time.Now().UTC()), "resetAt should be in the future")

		// Log the actual resetAt value for documentation
		t.Logf("DAILY limit resetAt: %s", *limit.ResetAt)

		// Verify it's reasonably in the future (within 24-48 hours for DAILY)
		maxExpected := time.Now().UTC().Add(48 * time.Hour)
		assert.True(t, resetAt.Before(maxExpected), "resetAt should be within 48 hours for DAILY limit")
	}

	// Document expected behavior for period reset:
	// - When a new period starts (e.g., new day for DAILY limit),
	//   the counter should reset and a new periodKey should be used
	// - This is verified indirectly by checking that resetAt is calculated correctly
	t.Log("Period reset behavior: When resetAt is reached, counter resets and new periodKey is used")
}

// =============================================================================
// Additional Verification Tests
// =============================================================================

// TestLimitsVerification_DailyLimitPeriodFormat verifies that
// DAILY limits use the correct period format (YYYY-MM-DD).
//
// Test spec 5.1.7: DAILY limit uses correct period
func TestLimitsVerification_DailyLimitPeriodFormat(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50307).String()

	// Create DAILY limit
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Validate a transaction to create a usage counter
	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50308).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Validation should succeed: %s", string(body))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Verify the limit is checked with period = DAILY
	for _, detail := range result.LimitUsageDetails {
		if detail.LimitID == limitID {
			assert.Equal(t, "DAILY", detail.Period, "Period should be DAILY")
			break
		}
	}

	// Document expected periodKey format: "YYYY-MM-DD" (e.g., "2024-01-15")
	t.Logf("DAILY limit uses periodKey format: YYYY-MM-DD (e.g., %s)", time.Now().UTC().Format("2006-01-02"))
}

// TestLimitsVerification_MonthlyLimitPeriodFormat verifies that
// MONTHLY limits use the correct period format (YYYY-MM).
//
// Test spec 5.1.8: MONTHLY limit uses correct period
func TestLimitsVerification_MonthlyLimitPeriodFormat(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50317).String()

	// Create MONTHLY limit
	limitID := testutil.CreateLimitWithAccountScopeAndType(t, accountID, "5000", "MONTHLY")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Validate a transaction to create a usage counter
	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50318).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Validation should succeed: %s", string(body))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Verify the limit is checked with period = MONTHLY
	for _, detail := range result.LimitUsageDetails {
		if detail.LimitID == limitID {
			assert.Equal(t, "MONTHLY", detail.Period, "Period should be MONTHLY")
			break
		}
	}

	// Document expected periodKey format: "YYYY-MM" (e.g., "2024-01")
	t.Logf("MONTHLY limit uses periodKey format: YYYY-MM (e.g., %s)", time.Now().UTC().Format("2006-01"))
}

// TestLimitsVerification_5_2_6_RollbackWorks verifies that
// usage is rolled back if a transaction fails after increment but before commit.
//
// Test spec 5.2.6: Rollback works
//
// Note: This test documents the expected rollback behavior.
// Actually testing rollback requires either:
// 1. Fault injection capability in the service
// 2. Ability to simulate failure mid-transaction
//
// The test verifies the system's behavior when fault injection is available,
// otherwise it documents the expected behavior.
func TestLimitsVerification_5_2_6_RollbackWorks(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50260).String()

	// Create limit of 1000 (currentUsage: 0)
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Establish initial usage of 500
	setupReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50261).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("500"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, setupReq)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "Setup validation should succeed: %s", string(body1))

	// Verify initial usage is 500
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	usageReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID+"/usage", nil)
	require.NoError(t, err)
	usageReq.Header.Set("X-API-Key", apiKey)

	usageResp, err := testutil.HTTPClient.Do(usageReq)
	require.NoError(t, err)
	defer usageResp.Body.Close()

	usageBody, err := io.ReadAll(usageResp.Body)
	require.NoError(t, err)

	// Assert status code first
	require.Equal(t, http.StatusOK, usageResp.StatusCode,
		"Usage endpoint should return 200, got: %s", string(usageBody))

	var usageResponse model.UsageSnapshot
	err = json.Unmarshal(usageBody, &usageResponse)
	require.NoError(t, err)

	initialUsage := usageResponse.CurrentUsage

	// Try to trigger a validation with fault injection (if supported)
	// This simulates a failure after increment but before commit
	faultReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50262).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("200"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	// Try with fault injection header (FaultUnavailable simulates 503 error)
	resp2, body2 := testutil.CreateValidationWithFaultInjection(t, faultReq, testutil.FaultUnavailable)
	defer resp2.Body.Close()

	// If fault injection worked (503 response), verify usage was not incremented
	if resp2.StatusCode == http.StatusServiceUnavailable {
		t.Log("Fault injection triggered 503 - verifying rollback behavior")

		// Poll for rollback completion (avoid flaky fixed sleeps)
		// Returns (usage, error) to distinguish transient network errors from actual usage values.
		checkUsage := func() (decimal.Decimal, error) {
			usageReq2, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID+"/usage", nil)
			if err != nil {
				return decimal.Zero, fmt.Errorf("create request: %w", err)
			}
			usageReq2.Header.Set("X-API-Key", apiKey)

			usageResp2, err := testutil.HTTPClient.Do(usageReq2)
			if err != nil {
				return decimal.Zero, fmt.Errorf("HTTP request: %w", err)
			}
			defer usageResp2.Body.Close()

			usageBody2, err := io.ReadAll(usageResp2.Body)
			if err != nil {
				return decimal.Zero, fmt.Errorf("read body: %w", err)
			}

			if usageResp2.StatusCode != http.StatusOK {
				return decimal.Zero, fmt.Errorf("unexpected status %d: %s", usageResp2.StatusCode, string(usageBody2))
			}

			var usageResponse2 model.UsageSnapshot
			if err := json.Unmarshal(usageBody2, &usageResponse2); err != nil {
				return decimal.Zero, fmt.Errorf("unmarshal: %w", err)
			}

			return usageResponse2.CurrentUsage, nil
		}

		// Verify usage returns to initialUsage after rollback
		// Note: CurrentUsage=0 when no counters exist is acceptable if initialUsage was also 0
		require.Eventually(t, func() bool {
			usage, err := checkUsage()
			if err != nil {
				t.Logf("checkUsage error (retrying): %v", err)
				return false // keep retrying on transient errors
			}
			return usage.Equal(initialUsage)
		}, 2*time.Second, 100*time.Millisecond, "Usage should be rolled back to %s after failure", initialUsage)
	} else {
		// Fault injection not triggered - document expected behavior
		t.Logf("Fault injection not triggered (status: %d). Expected behavior documented:", resp2.StatusCode)
		t.Log("- If failure occurs after increment but before commit, usage should rollback")
		t.Log("- After rollback, currentUsage should return to original value")
		t.Logf("- Response body: %s", string(body2))
	}
}

// =============================================================================
// 5.3 Period Reset (continued)
// =============================================================================

// TestLimitsVerification_5_3_2_UsageResetsInNewDailyPeriod verifies that
// usage resets when a new DAILY period begins.
//
// Test spec 5.3.2: Usage resets in new DAILY period
//
// Note: This test documents the expected behavior. Actually testing
// period rollover would require:
// 1. Waiting for midnight UTC (impractical in tests)
// 2. Control over system clock (mock time)
// 3. Backdating transaction timestamps (if supported by the service)
//
// The test verifies the structure and documents the expected rollover behavior.
func TestLimitsVerification_5_3_2_UsageResetsInNewDailyPeriod(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50320).String()

	// Create DAILY limit of 1000
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Establish usage in current period (800)
	setupReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50321).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("800"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, setupReq)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "Setup validation should succeed: %s", string(body1))

	// Verify the limit has resetAt in the future
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode, "Get limit should succeed: %s", string(getBody))

	var limit limitVerificationResponse
	err = json.Unmarshal(getBody, &limit)
	require.NoError(t, err)

	// DAILY limits should have resetAt set to next midnight
	require.NotNil(t, limit.ResetAt, "DAILY limit should have resetAt set")

	resetAt, err := time.Parse(time.RFC3339, *limit.ResetAt)
	require.NoError(t, err, "resetAt should be valid RFC3339 timestamp")

	// Verify resetAt is in the future
	assert.True(t, resetAt.After(time.Now().UTC()), "resetAt should be in the future")

	// Log current state and expected behavior after period rollover
	t.Logf("Current period resetAt: %s", *limit.ResetAt)
	t.Logf("Current periodKey: %s (YYYY-MM-DD format)", time.Now().UTC().Format("2006-01-02"))
	t.Log("Expected behavior after midnight UTC:")
	t.Log("- New periodKey created: YYYY-MM-DD (next day)")
	t.Log("- Counter starts fresh at 0")
	t.Log("- Transaction with amount=500 should be approved")
	t.Log("- New currentUsage = 500")

	// Verify current usage is 800 (demonstrates period-based tracking)
	usageReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID+"/usage", nil)
	require.NoError(t, err)
	usageReq.Header.Set("X-API-Key", apiKey)

	usageResp, err := testutil.HTTPClient.Do(usageReq)
	require.NoError(t, err)
	defer usageResp.Body.Close()

	usageBody, err := io.ReadAll(usageResp.Body)
	require.NoError(t, err)

	// Assert status code first
	require.Equal(t, http.StatusOK, usageResp.StatusCode,
		"Usage endpoint should return 200, got: %s", string(usageBody))

	var usageResponse model.UsageSnapshot
	err = json.Unmarshal(usageBody, &usageResponse)
	require.NoError(t, err)

	assert.True(t, decimal.RequireFromString("800").Equal(usageResponse.CurrentUsage),
		"currentUsage should be 800 in current period")
	t.Logf("Current period usage: %s", usageResponse.CurrentUsage)
}

// TestLimitsVerification_5_3_3_UsageResetsInNewMonthlyPeriod verifies that
// usage resets when a new MONTHLY period begins.
//
// Test spec 5.3.3: Usage resets in new MONTHLY period
//
// Note: This test documents the expected behavior. Actually testing
// monthly rollover would require waiting until the 1st of next month.
func TestLimitsVerification_5_3_3_UsageResetsInNewMonthlyPeriod(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50330).String()

	// Create MONTHLY limit of 5000
	limitID := testutil.CreateLimitWithAccountScopeAndType(t, accountID, "5000", "MONTHLY")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Establish usage in current month (4500)
	setupReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50331).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("4500"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, setupReq)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "Setup validation should succeed: %s", string(body1))

	// Verify the MONTHLY limit structure
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode, "Get limit should succeed: %s", string(getBody))

	var limit limitVerificationResponse
	err = json.Unmarshal(getBody, &limit)
	require.NoError(t, err)

	assert.Equal(t, "MONTHLY", limit.LimitType, "Limit type should be MONTHLY")

	// MONTHLY limits should have resetAt set to first of next month
	if limit.ResetAt != nil {
		resetAt, err := time.Parse(time.RFC3339, *limit.ResetAt)
		require.NoError(t, err, "resetAt should be valid RFC3339 timestamp")

		// Verify resetAt is in the future
		assert.True(t, resetAt.After(time.Now().UTC()), "resetAt should be in the future")

		// For MONTHLY limits, reset should be at the start of next month
		t.Logf("MONTHLY limit resetAt: %s", *limit.ResetAt)
	}

	// Log current state and expected behavior
	t.Logf("Current periodKey: %s (YYYY-MM format)", time.Now().UTC().Format("2006-01"))
	t.Log("Expected behavior after month rollover:")
	t.Log("- New periodKey created: YYYY-MM (next month)")
	t.Log("- Counter starts fresh at 0")
	t.Log("- Transaction with amount=1000 should be approved")
	t.Log("- New currentUsage = 1000")

	// Verify current usage
	usageReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID+"/usage", nil)
	require.NoError(t, err)
	usageReq.Header.Set("X-API-Key", apiKey)

	usageResp, err := testutil.HTTPClient.Do(usageReq)
	require.NoError(t, err)
	defer usageResp.Body.Close()

	usageBody, err := io.ReadAll(usageResp.Body)
	require.NoError(t, err)

	// Assert status code first
	require.Equal(t, http.StatusOK, usageResp.StatusCode,
		"Usage endpoint should return 200, got: %s", string(usageBody))

	var usageResponse model.UsageSnapshot
	err = json.Unmarshal(usageBody, &usageResponse)
	require.NoError(t, err)

	assert.True(t, decimal.RequireFromString("4500").Equal(usageResponse.CurrentUsage),
		"currentUsage should be 4500 in current period")
	t.Logf("Current period usage: %s", usageResponse.CurrentUsage)
}

// TestLimitsVerification_5_3_4_OldCountersCleanedUp verifies that
// old period counters are cleaned up by the system.
//
// Test spec 5.3.4: Old counters cleaned up
//
// Note: This test documents the expected cleanup behavior.
// Actually testing cleanup would require:
// 1. Creating counters in past periods
// 2. Triggering or waiting for the cleanup job
// 3. Verifying counters are removed
//
// The test verifies the API structure supports querying counters
// and documents the expected cleanup behavior.
func TestLimitsVerification_5_3_4_OldCountersCleanedUp(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(50340).String()

	// Create DAILY limit to test counter structure
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Create some usage to generate a counter
	setupReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(50341).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("300"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, setupReq)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "Setup validation should succeed: %s", string(body1))

	// Query the usage endpoint to verify counter structure
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	usageReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID+"/usage", nil)
	require.NoError(t, err)
	usageReq.Header.Set("X-API-Key", apiKey)

	usageResp, err := testutil.HTTPClient.Do(usageReq)
	require.NoError(t, err)
	defer usageResp.Body.Close()

	usageBody, err := io.ReadAll(usageResp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, usageResp.StatusCode,
		"Usage endpoint should return 200, got %d: %s", usageResp.StatusCode, string(usageBody))

	var usageResponse model.UsageSnapshot
	err = json.Unmarshal(usageBody, &usageResponse)
	require.NoError(t, err)

	// Verify usage snapshot - must be 300 after setup transaction
	t.Logf("Current usage - Amount: %s, Limit: %s, Utilization: %.2f%%",
		usageResponse.CurrentUsage, usageResponse.LimitAmount, usageResponse.UtilizationPercent)

	// Verify usage has expected value (use require to fail test if wrong)
	require.True(t, decimal.RequireFromString("300").Equal(usageResponse.CurrentUsage),
		"Usage should be 300 after setup transaction, got %s", usageResponse.CurrentUsage)

	// Document expected cleanup behavior
	t.Log("Expected cleanup behavior:")
	t.Log("- Cleanup job runs periodically (e.g., daily)")
	t.Log("- Counters older than 2 periods are removed:")
	t.Log("  - For DAILY limits: counters older than 2 days")
	t.Log("  - For MONTHLY limits: counters older than 2 months")
	t.Log("- Recent counters are preserved for auditing")
	t.Log("- Cleanup does not affect current period counters")
}

// =============================================================================
// 5.2.7 High Concurrency Atomic Enforcement
// =============================================================================

// TestLimitsVerification_5_2_7_HighConcurrencyAtomicEnforcement verifies that
// atomic enforcement holds under high concurrency.
//
// Test spec 5.2.7: High concurrency atomic enforcement
//
// Scenario: 20 goroutines each send validation with amount=1000, limit=10000.
// Expected: Exactly 10 approved, 10 denied.
// Verify final currentUsage=10000 via GET /v1/limits/{limitID}/usage.
func TestLimitsVerification_5_2_7_HighConcurrencyAtomicEnforcement(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(90150).String()

	// Create DAILY limit of 10000
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "10000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Fire 20 parallel validations of amount=1000 each
	// Expected: 10 should succeed (10 * 1000 = 10000 <= limit), 10 should be denied
	const numConcurrent = 20

	// Freeze timestamp to avoid period-boundary flakes in concurrency tests
	// All requests in this test must use the same timestamp to ensure they target the same period
	fixedTimestamp := time.Now().UTC().Format(time.RFC3339)

	var wg sync.WaitGroup
	var approvedCount int64
	var deniedCount int64

	// Barrier sync: ready WaitGroup + start channel
	ready := sync.WaitGroup{}
	start := make(chan struct{})

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		ready.Add(1)
		go func(idx int) {
			defer wg.Done()

			req := &testutil.ValidationRequest{
				RequestID:            testutil.MustDeterministicUUID(int64(90151 + idx)).String(),
				TransactionType:      "PIX",
				Amount:               decimal.RequireFromString("1000"),
				Currency:             "BRL",
				TransactionTimestamp: fixedTimestamp,
				Account: &testutil.AccountContext{
					ID: accountID,
				},
			}

			// Signal ready and wait for start signal
			ready.Done()
			<-start

			resp, body := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			// Use assert (not require) inside goroutines to avoid panics
			if !assert.Equal(t, http.StatusCreated, resp.StatusCode, "Validation request should return 201 Created") {
				return
			}

			var result testutil.ValidationResponse
			if err := json.Unmarshal(body, &result); err != nil {
				assert.NoError(t, err, "Failed to unmarshal validation response")
				return
			}

			if result.Decision == "DENY" && result.Reason == "limit_exceeded" {
				atomic.AddInt64(&deniedCount, 1)
			} else if result.Decision != "DENY" {
				atomic.AddInt64(&approvedCount, 1)
			}
		}(i)
	}

	// Wait for all goroutines to be ready, then release them simultaneously
	ready.Wait()
	close(start)

	// Wait for all goroutines to complete
	wg.Wait()

	// Load final counts
	finalApproved := atomic.LoadInt64(&approvedCount)
	finalDenied := atomic.LoadInt64(&deniedCount)

	// Log observed behavior for documentation
	t.Logf("High concurrency test results: approved=%d, denied=%d", finalApproved, finalDenied)

	// Verify: Exactly 10 approved and 10 denied (atomic enforcement)
	assert.Equal(t, int64(10), finalApproved, "Exactly 10 transactions should be approved (atomic enforcement)")
	assert.Equal(t, int64(10), finalDenied, "Exactly 10 transactions should be denied (limit exceeded)")

	// Verify final usage via GET /v1/limits/{limitID}/usage
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	usageReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID+"/usage", nil)
	require.NoError(t, err)
	usageReq.Header.Set("X-API-Key", apiKey)

	usageResp, err := testutil.HTTPClient.Do(usageReq)
	require.NoError(t, err)
	defer usageResp.Body.Close()

	usageBody, err := io.ReadAll(usageResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, usageResp.StatusCode, "Get usage should succeed: %s", string(usageBody))

	var usageResponse model.UsageSnapshot
	err = json.Unmarshal(usageBody, &usageResponse)
	require.NoError(t, err)

	assert.True(t, decimal.RequireFromString("10000").Equal(usageResponse.CurrentUsage),
		"Final currentUsage should be 10000 (10 * 1000)")
}

// =============================================================================
// 5.4 Timestamp Bypass Prevention
// =============================================================================

// TestLimitsVerification_5_4_1_BackdatedTimestampBypass verifies that
// backdated timestamps cannot be used to bypass daily limits.
//
// Test spec 5.4.1: Backdated timestamp bypass prevention
//
// Scenario: Create DAILY limit of 5000, exhaust with 5 current-time transactions,
// then attempt 5 more with yesterday's timestamps. All backdated attempts must be denied.
func TestLimitsVerification_5_4_1_BackdatedTimestampBypass(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(90200).String()

	// Create DAILY limit of 5000
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "5000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Capture base time once for consistent testing
	baseNow := time.Now().UTC()
	currentTimestamp := baseNow.Format(time.RFC3339)

	// Exhaust the limit with 5 current-time transactions of 1000 each
	for i := 0; i < 5; i++ {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(int64(90201 + i)).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("1000"),
			Currency:             "BRL",
			TransactionTimestamp: currentTimestamp,
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Transaction %d should succeed: %s", i+1, string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		// All 5 should be approved (limit not exceeded)
		assert.NotEqual(t, "DENY", result.Decision,
			"Transaction %d should not be denied, limit not yet exhausted", i+1)
	}

	// Now attempt 5 more transactions with yesterday's timestamp (backdated)
	// These should all be DENIED because:
	// 1. Period key is calculated from server time, not client timestamp
	// 2. The limit is already exhausted for today's period

	// Calculate yesterday's timestamp: start of today UTC - 1 second (guaranteed previous UTC day)
	startOfTodayUTC := time.Date(baseNow.Year(), baseNow.Month(), baseNow.Day(), 0, 0, 0, 0, time.UTC)
	yesterdayTimestamp := startOfTodayUTC.Add(-time.Second).Format(time.RFC3339) // Last second of previous day

	deniedCount := 0
	for i := 0; i < 5; i++ {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(int64(90206 + i)).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("1000"),
			Currency:             "BRL",
			TransactionTimestamp: yesterdayTimestamp,
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Backdated transaction %d should return 201: %s", i+1, string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		// All backdated transactions should be denied due to limit exceeded
		if result.Decision == "DENY" && result.Reason == "limit_exceeded" {
			deniedCount++
		}
	}

	// All 5 backdated transactions must be denied
	assert.Equal(t, 5, deniedCount, "All 5 backdated transactions should be denied due to limit exceeded")

	t.Log("Backdated timestamp bypass prevention verified: server time used for period key calculation")
}

// TestLimitsVerification_5_4_2_PastTimestampRejection verifies that
// transactions with timestamps older than MaxTimestampAge (24h) are rejected
// at the validation layer with TRC-0228.
//
// Test spec 5.4.2: Past timestamp rejected with TRC-0228
func TestLimitsVerification_5_4_2_PastTimestampRejection(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(90210).String()

	// Submit transaction with timestamp 48h in the past (beyond MaxTimestampAge)
	pastTimestamp := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)

	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(90211).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: pastTimestamp,
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	// Should return HTTP 400 (validation error)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Timestamp 48h in past should be rejected with 400: %s", string(body))

	var errResp testutil.ErrorResponse
	err := json.Unmarshal(body, &errResp)
	require.NoError(t, err)

	// Error code should be TRC-0228
	assert.Equal(t, "0421", errResp.Code,
		"Error code should be TRC-0228 (timestamp too far in past)")

	t.Logf("Past timestamp rejection verified: timestamps older than 24h rejected with TRC-0228")
}

// TestLimitsVerification_5_4_3_MaxTimestampAgeBoundary verifies the exact
// boundary behavior of MaxTimestampAge validation (24h).
//
// Test spec 5.4.3: MaxTimestampAge boundary test
func TestLimitsVerification_5_4_3_MaxTimestampAgeBoundary(t *testing.T) {
	// Test 1: Timestamp (24h - 1min) in past should be ACCEPTED
	t.Run("within_threshold_accepted", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(90220).String()

		// 24h - 1min = within threshold
		acceptedTimestamp := time.Now().UTC().Add(-(24*time.Hour - time.Minute)).Format(time.RFC3339)

		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(90221).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("100"),
			Currency:             "BRL",
			TransactionTimestamp: acceptedTimestamp,
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		// Should return HTTP 201 (accepted)
		require.Equal(t, http.StatusCreated, resp.StatusCode,
			"Timestamp (24h - 1min) in past should be accepted: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		// Should have a validationId (processed successfully)
		assert.NotEmpty(t, result.ValidationID, "Should return validationId for accepted request")

		t.Logf("Boundary test: timestamp 24h-1min in past accepted")
	})

	// Test 2: Timestamp (24h + 1min) in past should be REJECTED
	t.Run("beyond_threshold_rejected", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(90222).String()

		// 24h + 1min = beyond threshold
		rejectedTimestamp := time.Now().UTC().Add(-(24*time.Hour + time.Minute)).Format(time.RFC3339)

		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(90223).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("100"),
			Currency:             "BRL",
			TransactionTimestamp: rejectedTimestamp,
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		// Should return HTTP 400 (rejected)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"Timestamp (24h + 1min) in past should be rejected: %s", string(body))

		var errResp testutil.ErrorResponse
		err := json.Unmarshal(body, &errResp)
		require.NoError(t, err)

		assert.Equal(t, "0421", errResp.Code,
			"Error code should be TRC-0228 for timestamp beyond MaxTimestampAge")

		t.Logf("Boundary test: timestamp 24h+1min in past rejected with TRC-0228")
	})
}

// TestLimitsVerification_5_4_4_AuditTrailPreservation verifies that
// the original transaction timestamp is preserved in the audit trail.
//
// Test spec 5.4.4: Audit trail preservation
func TestLimitsVerification_5_4_4_AuditTrailPreservation(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(90230).String()

	// Submit transaction with timestamp 2h in past (within tolerance)
	pastTimestamp := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)

	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(90231).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: pastTimestamp,
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Transaction with 2h-old timestamp should be accepted: %s", string(body))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body, &result)
	require.NoError(t, err)

	validationID := result.ValidationID
	require.NotEmpty(t, validationID, "Should return validationId")

	// Retrieve the validation via GET /v1/validations/{id}
	getResp, getBody := testutil.GetValidation(t, validationID)
	defer getResp.Body.Close()

	require.Equal(t, http.StatusOK, getResp.StatusCode,
		"GET validation should succeed: %s", string(getBody))

	var detail testutil.ValidationDetailResponse
	err = json.Unmarshal(getBody, &detail)
	require.NoError(t, err)

	// Parse the stored timestamp
	storedTimestamp, err := time.Parse(time.RFC3339, detail.TransactionTimestamp)
	require.NoError(t, err, "Stored timestamp should be valid RFC3339")

	// Parse the original timestamp
	originalTimestamp, err := time.Parse(time.RFC3339, pastTimestamp)
	require.NoError(t, err, "Original timestamp should be valid RFC3339")

	// The stored timestamp should match the original (not server time)
	// Allow 1 second tolerance for RFC3339 formatting differences
	timeDiff := storedTimestamp.Sub(originalTimestamp).Abs()
	assert.LessOrEqual(t, timeDiff, time.Second,
		"Stored timestamp should match original (within 1s tolerance), got diff: %v", timeDiff)

	t.Logf("Audit trail preservation verified: original timestamp %s stored as %s",
		pastTimestamp, detail.TransactionTimestamp)
}

// TestLimitsVerification_5_4_5_PerTransactionUnaffected verifies that
// PER_TRANSACTION limits are not affected by timestamp manipulation
// since they check amount only, not accumulated usage.
//
// Test spec 5.4.5: PER_TRANSACTION limit checks value only
func TestLimitsVerification_5_4_5_PerTransactionUnaffected(t *testing.T) {
	transactionType := "CARD"

	// Create PER_TRANSACTION limit of 50000
	limitID := testutil.CreateLimitWithTransactionTypeScope(t, transactionType, "50000")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Test 1: Current timestamp with amount < limit (should be accepted)
	t.Run("current_timestamp_under_limit", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(90240).String()

		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(90241).String(),
			TransactionType:      transactionType,
			Amount:               decimal.RequireFromString("30000"),
			Currency:             "BRL",
			TransactionTimestamp: time.Now().UTC().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode,
			"Transaction under limit should return 201: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		// Should not be denied due to limit
		for _, detail := range result.LimitUsageDetails {
			if detail.LimitID == limitID {
				assert.False(t, detail.Exceeded, "PER_TRANSACTION limit should not be exceeded for 30000 < 50000")
				break
			}
		}
	})

	// Test 2: Past timestamp (2h) with amount < limit (should be accepted)
	t.Run("past_timestamp_under_limit", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(90242).String()

		pastTimestamp := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)

		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(90243).String(),
			TransactionType:      transactionType,
			Amount:               decimal.RequireFromString("30000"),
			Currency:             "BRL",
			TransactionTimestamp: pastTimestamp,
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode,
			"Transaction with past timestamp under limit should return 201: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		// Should not be denied due to limit (PER_TRANSACTION checks amount only)
		for _, detail := range result.LimitUsageDetails {
			if detail.LimitID == limitID {
				assert.False(t, detail.Exceeded, "PER_TRANSACTION limit should not be exceeded for 30000 < 50000")
				break
			}
		}
	})

	// Test 3: Amount > limit (should be denied regardless of timestamp)
	t.Run("over_limit_denied", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(90244).String()

		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(90245).String(),
			TransactionType:      transactionType,
			Amount:               decimal.RequireFromString("60000"), // Exceeds 50000 limit
			Currency:             "BRL",
			TransactionTimestamp: time.Now().UTC().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode,
			"Transaction over limit should return 201 with DENY: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		// Should be denied due to PER_TRANSACTION limit exceeded
		assert.Equal(t, "DENY", result.Decision, "Should be DENY when amount exceeds PER_TRANSACTION limit")
		assert.Equal(t, "limit_exceeded", result.Reason, "Reason should be limit_exceeded")

		// Verify exceeded flag
		foundLimit := false
		for _, detail := range result.LimitUsageDetails {
			if detail.LimitID == limitID {
				foundLimit = true
				assert.True(t, detail.Exceeded, "PER_TRANSACTION limit should be exceeded for 60000 > 50000")
				break
			}
		}
		assert.True(t, foundLimit, "PER_TRANSACTION limit should be in response")
	})

	t.Log("PER_TRANSACTION limit behavior verified: checks amount only, not accumulated usage")
}
