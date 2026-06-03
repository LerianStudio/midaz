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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test 1.1.1: Validation with complete payload
func TestValidation_CompletePayload(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(100).String()
	segmentID := testutil.MustDeterministicUUID(101).String()
	portfolioID := testutil.MustDeterministicUUID(102).String()
	merchantID := testutil.MustDeterministicUUID(103).String()
	requestID := testutil.MustDeterministicUUID(104).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		SubType:              "credit",
		Amount:               decimal.RequireFromString("1500"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		Segment: &testutil.SegmentContext{
			ID: segmentID,
		},
		Portfolio: &testutil.PortfolioContext{
			ID: portfolioID,
		},
		Merchant: &testutil.MerchantContext{
			ID:   merchantID,
			MCC:  "5411",
			Name: "Test Merchant",
		},
		Metadata: map[string]interface{}{
			"channel":  "mobile",
			"deviceId": "device-123",
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Verify response contains required fields
	assert.Equal(t, requestID, result.RequestID, "requestId should match request")
	assert.Contains(t, []string{"ALLOW", "DENY", "REVIEW"}, result.Decision, "decision should be valid")
	assert.NotNil(t, result.MatchedRuleIDs, "matchedRuleIds should be present")
	assert.NotNil(t, result.EvaluatedRuleIDs, "evaluatedRuleIds should be present")
	assert.NotNil(t, result.LimitUsageDetails, "limitUsageDetails should be present")
	assert.GreaterOrEqual(t, result.ProcessingTimeMs, float64(0), "processingTimeMs should be >= 0")
	assert.NotEmpty(t, result.Reason, "reason should be present and not empty")

	// Verify validationId is a valid UUID format (server-generated)
	require.NotEmpty(t, result.ValidationID, "validationId should be present")
	_, uuidErr := uuid.Parse(result.ValidationID)
	require.NoError(t, uuidErr, "validationId should be valid UUID format: %s", result.ValidationID)
	assert.Len(t, result.ValidationID, 36, "validationId should be 36 characters (8-4-4-4-12 format)")
	assert.NotEqual(t, requestID, result.ValidationID, "validationId should be different from requestId (server-generated)")
}

// Test 1.1.2: Validation returns ALLOW without DENY rules
func TestValidation_ReturnsAllowWithoutDenyRules(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(110).String()
	requestID := testutil.MustDeterministicUUID(111).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
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

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body, &result)
	require.NoError(t, err)

	assert.Equal(t, "ALLOW", result.Decision, "Expected ALLOW when no DENY rules match")
	assert.Empty(t, result.MatchedRuleIDs, "matchedRuleIds should be empty when no DENY rules match")
	assert.NotEmpty(t, result.Reason, "reason should be present and not empty")
}

// Test 1.1.3: Validation returns DENY when DENY rule matches
func TestValidation_ReturnsDenyWhenRuleMatches(t *testing.T) {
	// Create and activate DENY rule: "transactionType == 'CARD' && amount > 5000"
	ruleName := "deny-high-value-card-" + testutil.MustDeterministicUUID(1001).String()[:8]
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "transactionType == 'CARD' && amount > 5000", "DENY")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	accountID := testutil.MustDeterministicUUID(120).String()
	requestID := testutil.MustDeterministicUUID(121).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("6000"), // Above 5000 threshold
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

	assert.Equal(t, "DENY", result.Decision, "Expected DENY when DENY rule matches")
	assert.Contains(t, result.MatchedRuleIDs, ruleID, "matchedRuleIds should contain the DENY rule ID")
	assert.NotEmpty(t, result.Reason, "reason should be present when DENY")
}

// Test 1.1.4: Validation returns REVIEW when REVIEW rule matches
func TestValidation_ReturnsReviewWhenRuleMatches(t *testing.T) {
	// Create and activate REVIEW rule: "amount > 1000 && amount <= 5000"
	ruleName := "review-medium-value-" + testutil.MustDeterministicUUID(1002).String()[:8]
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "amount > 1000 && amount <= 5000", "REVIEW")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	accountID := testutil.MustDeterministicUUID(130).String()
	requestID := testutil.MustDeterministicUUID(131).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "WIRE",
		Amount:               decimal.RequireFromString("2000"), // Between 1000 and 5000
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

	assert.Equal(t, "REVIEW", result.Decision, "Expected REVIEW when REVIEW rule matches")
	assert.Contains(t, result.MatchedRuleIDs, ruleID, "matchedRuleIds should contain the REVIEW rule ID")
}

// Test 1.1.5: Validation returns DENY when limit is exceeded
func TestValidation_ReturnsDenyWhenLimitExceeded(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(200).String()

	// Create and activate DAILY limit with amount 1000 for accountId
	limitID := createTestLimitWithAccountScope(t, accountID, "1000")
	activateTestLimit(t, limitID)
	t.Cleanup(func() {
		cleanupTestLimit(t, limitID)
	})

	// Consume 900 of the limit with a first validation
	firstReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(201).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("900"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, firstReq)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "First validation should succeed: %s", string(body1))

	// Now try to exceed the limit with amount 200 (900 + 200 > 1000)
	secondReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(202).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("200"),
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

	assert.Equal(t, "DENY", result.Decision, "Expected DENY when limit is exceeded")
	assert.Equal(t, "limit_exceeded", result.Reason, "Expected reason to be limit_exceeded")

	// Verify limitUsageDetails contains entry with exceeded == true
	require.NotEmpty(t, result.LimitUsageDetails, "limitUsageDetails should not be empty")
	hasExceeded := false
	for _, detail := range result.LimitUsageDetails {
		if detail.Exceeded {
			hasExceeded = true
			break
		}
	}
	assert.True(t, hasExceeded, "limitUsageDetails should contain entry with exceeded == true")

	// Verify complete LimitUsageDetails structure per spec
	var foundLimit bool
	for _, detail := range result.LimitUsageDetails {
		if detail.LimitID == limitID {
			foundLimit = true
			// Validate limitId format
			_, uuidErr := uuid.Parse(detail.LimitID)
			assert.NoError(t, uuidErr, "limitId should be valid UUID format")

			// Validate scope format: "{scopeType}:{scopeValue}"
			assert.Contains(t, detail.Scope, ":", "scope should be in format scopeType:scopeValue")
			assert.Contains(t, detail.Scope, accountID, "scope should contain account_id")

			// Validate period
			assert.Equal(t, "DAILY", detail.Period, "period should be DAILY")

			// Validate amounts
			assert.True(t, decimal.RequireFromString("1000").Equal(detail.LimitAmount), "limitAmount should be 1000")
			assert.True(t, detail.CurrentUsage.GreaterThanOrEqual(decimal.RequireFromString("900")), "currentUsage should be at least 900")
			assert.True(t, decimal.RequireFromString("200").Equal(detail.AttemptedAmount), "attemptedAmount should be 200")

			// Validate exceeded flag
			assert.True(t, detail.Exceeded, "exceeded should be true")
			break
		}
	}
	assert.True(t, foundLimit, "limitUsageDetails should contain the created limit")
}

// Test 1.1.6: Validation with correct precedence: DENY > LIMIT_EXCEEDED > REVIEW > ALLOW
func TestValidation_DecisionPrecedence(t *testing.T) {
	t.Run("DENY rule takes precedence over REVIEW rule", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(210).String()

		// Create DENY rule
		denyRuleName := "deny-precedence-test-" + testutil.MustDeterministicUUID(1003).String()[:8]
		denyRuleID := testutil.CreateTestRuleWithExpression(t, denyRuleName, "transactionType == 'CARD' && amount > 100", "DENY")
		testutil.ActivateRule(t, denyRuleID)

		// Create REVIEW rule
		reviewRuleName := "review-precedence-test-" + testutil.MustDeterministicUUID(1004).String()[:8]
		reviewRuleID := testutil.CreateTestRuleWithExpression(t, reviewRuleName, "transactionType == 'CARD' && amount > 50", "REVIEW")
		testutil.ActivateRule(t, reviewRuleID)

		t.Cleanup(func() {
			testutil.CleanupRule(t, denyRuleID)
			testutil.CleanupRule(t, reviewRuleID)
		})

		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(211).String(),
			TransactionType:      "CARD",
			Amount:               decimal.RequireFromString("150"), // Matches both DENY and REVIEW rules
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

		assert.Equal(t, "DENY", result.Decision, "DENY should take precedence over REVIEW")
		assert.Contains(t, result.MatchedRuleIDs, denyRuleID, "matchedRuleIds should contain the DENY rule ID")
	})

	t.Run("Limit exceeded takes precedence over REVIEW rule", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(220).String()

		// Create a limit with low amount
		limitID := createTestLimitWithAccountScope(t, accountID, "50")
		activateTestLimit(t, limitID)

		// Create REVIEW rule
		reviewRuleName := "review-limit-precedence-" + testutil.MustDeterministicUUID(1005).String()[:8]
		reviewRuleID := testutil.CreateTestRuleWithExpression(t, reviewRuleName, "transactionType == 'PIX'", "REVIEW")
		testutil.ActivateRule(t, reviewRuleID)

		t.Cleanup(func() {
			cleanupTestLimit(t, limitID)
			testutil.CleanupRule(t, reviewRuleID)
		})

		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(221).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("100"), // Exceeds limit of 50
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

		assert.Equal(t, "DENY", result.Decision, "Limit exceeded should result in DENY")
	})

	t.Run("REVIEW rule only results in REVIEW decision", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(230).String()

		// Create only REVIEW rule
		reviewRuleName := "review-only-" + testutil.MustDeterministicUUID(1006).String()[:8]
		reviewRuleID := testutil.CreateTestRuleWithExpression(t, reviewRuleName, "transactionType == 'WIRE' && amount > 500", "REVIEW")
		testutil.ActivateRule(t, reviewRuleID)

		t.Cleanup(func() {
			testutil.CleanupRule(t, reviewRuleID)
		})

		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(231).String(),
			TransactionType:      "WIRE",
			Amount:               decimal.RequireFromString("750"), // Matches REVIEW rule
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

		assert.Equal(t, "REVIEW", result.Decision, "Should return REVIEW when only REVIEW rule matches")
		assert.Contains(t, result.MatchedRuleIDs, reviewRuleID, "matchedRuleIds should contain the REVIEW rule ID")
	})

	t.Run("ALLOW only results in ALLOW decision", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(400).String()

		// Create only ALLOW rule
		allowRuleName := "allow-only-test-" + testutil.MustDeterministicUUID(1007).String()[:8]
		allowRuleID := testutil.CreateTestRuleWithExpression(t, allowRuleName, "transactionType == 'PIX'", "ALLOW")
		testutil.ActivateRule(t, allowRuleID)

		t.Cleanup(func() {
			testutil.CleanupRule(t, allowRuleID)
		})

		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(401).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("50"),
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

		assert.Equal(t, "ALLOW", result.Decision, "ALLOW only should result in ALLOW")
		assert.Contains(t, result.MatchedRuleIDs, allowRuleID, "matchedRuleIds should contain the ALLOW rule")
		// Verify reason pattern per spec
		assert.NotEmpty(t, result.Reason, "reason should be present")
		reasonLower := strings.ToLower(result.Reason)
		assert.True(t, strings.Contains(reasonLower, "allow") || strings.Contains(reasonLower, "approved"),
			"reason should contain 'allow' or 'approved', got: %s", result.Reason)
	})

	t.Run("No rules results in default ALLOW", func(t *testing.T) {
		// Use unique identifiers to ensure no rules match
		accountID := testutil.MustDeterministicUUID(402).String()
		segmentID := testutil.MustDeterministicUUID(403).String()

		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(404).String(),
			TransactionType:      "CRYPTO",                          // Uncommon type unlikely to match any rules
			Amount:               decimal.RequireFromString("9.99"), // Uncommon amount
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
			Segment: &testutil.SegmentContext{
				ID: segmentID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created, got: %s", string(body))

		var result testutil.ValidationResponse
		err := json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Equal(t, "ALLOW", result.Decision, "No rules should result in default ALLOW")
		assert.Empty(t, result.MatchedRuleIDs, "matchedRuleIds should be empty")
		// Verify reason pattern per spec
		assert.NotEmpty(t, result.Reason, "reason should be present")
		reasonLower := strings.ToLower(result.Reason)
		assert.True(t,
			(strings.Contains(reasonLower, "no") && strings.Contains(reasonLower, "rule")) ||
				strings.Contains(reasonLower, "default"),
			"reason should contain 'no'+'rule' or 'default', got: %s", result.Reason)
	})
}

// Test 1.1.7: Validation without rules returns configurable default (ALLOW by default)
func TestValidation_DefaultDecisionWithoutRules(t *testing.T) {
	// Use unique accountId and transactionType to ensure no rules match
	accountID := testutil.MustDeterministicUUID(250).String()
	requestID := testutil.MustDeterministicUUID(251).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CRYPTO", // Uncommon transaction type
		Amount:               decimal.RequireFromString("500"),
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

	assert.Equal(t, "ALLOW", result.Decision, "Default decision should be ALLOW when no rules match")
	assert.Empty(t, result.MatchedRuleIDs, "matchedRuleIds should be empty when no rules match")
}

// createTestLimitWithAccountScope creates a DAILY limit with the specified account scope and max amount.
// Returns the limit ID.
func createTestLimitWithAccountScope(t *testing.T, accountID string, maxAmount string) string {
	t.Helper()

	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	type limitScopeInput struct {
		AccountID *string `json:"accountId,omitempty"`
	}

	type createLimitRequest struct {
		Name      string            `json:"name"`
		LimitType string            `json:"limitType"`
		MaxAmount string            `json:"maxAmount"`
		Currency  string            `json:"currency"`
		Scopes    []limitScopeInput `json:"scopes"`
	}

	type limitResponse struct {
		ID string `json:"limitId"`
	}

	uniqueName := "Test Limit " + testutil.RandomSuffix()
	reqBody := createLimitRequest{
		Name:      uniqueName,
		LimitType: "DAILY",
		MaxAmount: maxAmount,
		Currency:  "BRL",
		Scopes: []limitScopeInput{
			{AccountID: &accountID},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Failed to create test limit: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	return limit.ID
}

// cleanupTestLimit deletes a limit. Called in t.Cleanup() to clean up test data.
func cleanupTestLimit(t *testing.T, limitID string) {
	t.Helper()

	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodDelete, baseURL+"/v1/limits/"+limitID, nil)
	if err != nil {
		t.Logf("Cleanup: failed to create delete request for limit %s: %v", limitID, err)
		return
	}
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	if err != nil {
		t.Logf("Cleanup: failed to delete limit %s: %v", limitID, err)
		return
	}
	resp.Body.Close()
}

// activateTestLimit activates a limit by ID.
// Limits are created in DRAFT status and must be activated to be enforced.
func activateTestLimit(t *testing.T, limitID string) {
	t.Helper()

	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to activate limit: %s", string(respBody))
}

// Test 1.1.8: Validation with multiple matching rules
func TestValidation_1_1_8_MultipleMatchingRules(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(300).String()
	requestID := testutil.MustDeterministicUUID(301).String()

	// Create 3 ALLOW rules that all match the same payload
	// Rule 1: matches transactionType == 'PIX'
	rule1Name := "allow-pix-rule1-" + testutil.MustDeterministicUUID(302).String()[:8]
	rule1ID := testutil.CreateTestRuleWithExpression(t, rule1Name, "transactionType == 'PIX'", "ALLOW")
	testutil.ActivateRule(t, rule1ID)

	// Rule 2: matches amount > 50
	rule2Name := "allow-amount-rule2-" + testutil.MustDeterministicUUID(303).String()[:8]
	rule2ID := testutil.CreateTestRuleWithExpression(t, rule2Name, "amount > 50", "ALLOW")
	testutil.ActivateRule(t, rule2ID)

	// Rule 3: matches currency == 'BRL'
	rule3Name := "allow-currency-rule3-" + testutil.MustDeterministicUUID(304).String()[:8]
	rule3ID := testutil.CreateTestRuleWithExpression(t, rule3Name, "currency == 'BRL'", "ALLOW")
	testutil.ActivateRule(t, rule3ID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, rule1ID)
		testutil.CleanupRule(t, rule2ID)
		testutil.CleanupRule(t, rule3ID)
	})

	// Create validation request that matches all 3 rules
	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "PIX",                            // matches rule1
		Amount:               decimal.RequireFromString("100"), // matches rule2
		Currency:             "BRL",                            // matches rule3
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

	// Verify matchedRuleIds contains exactly 3 IDs
	assert.Len(t, result.MatchedRuleIDs, 3, "matchedRuleIds should contain exactly 3 IDs")
	assert.Contains(t, result.MatchedRuleIDs, rule1ID, "matchedRuleIds should contain rule1")
	assert.Contains(t, result.MatchedRuleIDs, rule2ID, "matchedRuleIds should contain rule2")
	assert.Contains(t, result.MatchedRuleIDs, rule3ID, "matchedRuleIds should contain rule3")

	// Verify evaluatedRuleIds contains at least 3 IDs
	assert.GreaterOrEqual(t, len(result.EvaluatedRuleIDs), 3, "evaluatedRuleIds should contain at least 3 IDs")
}

// Test 1.1.9: Validation rejects payload >100KB
func TestValidation_1_1_9_PayloadTooLarge(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(310).String()
	requestID := testutil.MustDeterministicUUID(311).String()

	// Create a large string > 100KB (100 * 1024 = 102400 bytes)
	largeString := make([]byte, 110*1024) // 110KB to ensure we exceed 100KB
	for i := range largeString {
		largeString[i] = 'x'
	}

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		Metadata: map[string]interface{}{
			"largeField": string(largeString),
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode,
		"Payload >100KB should return 413 Payload Too Large")

	// Verify error response structure
	errorResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "TRC-0011", errorResp.Code, "Error code should be TRC-0011 for payload too large")
	assert.Equal(t, "Payload Too Large", errorResp.Title, "Error title should be Payload Too Large")
	assert.Equal(t, "payload too large: exceeds 100KB limit", errorResp.Message, "Error message should indicate payload size limit")
}

// Test 1.1.10: Required fields validation
func TestValidation_RequiredFieldsValidation(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(140).String()
	requestID := testutil.MustDeterministicUUID(141).String()
	timestamp := testutil.FixedTime().Format(time.RFC3339)

	tests := []struct {
		name            string
		req             *testutil.ValidationRequest
		missing         string
		expectedCode    string
		expectedTitle   string
		expectedMessage string
	}{
		{
			name: "missing requestId",
			req: &testutil.ValidationRequest{
				RequestID:            "", // Missing requestId
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: timestamp,
				Account:              &testutil.AccountContext{ID: accountID},
			},
			missing:         "requestId",
			expectedCode:    "TRC-0220",
			expectedTitle:   "Validation Error",
			expectedMessage: "requestId is required",
		},
		{
			name: "missing transactionType",
			req: &testutil.ValidationRequest{
				RequestID:            requestID,
				TransactionType:      "", // Missing
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: timestamp,
				Account:              &testutil.AccountContext{ID: accountID},
			},
			missing:         "transactionType",
			expectedCode:    "TRC-0221",
			expectedTitle:   "Validation Error",
			expectedMessage: "transactionType must be one of [CARD, WIRE, PIX, CRYPTO]",
		},
		{
			name: "missing amount (zero value)",
			req: &testutil.ValidationRequest{
				RequestID:            testutil.MustDeterministicUUID(142).String(),
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("0"), // Missing/zero
				Currency:             "BRL",
				TransactionTimestamp: timestamp,
				Account:              &testutil.AccountContext{ID: accountID},
			},
			missing:         "amount",
			expectedCode:    "TRC-0222",
			expectedTitle:   "Validation Error",
			expectedMessage: "amount must be positive",
		},
		{
			name: "missing currency",
			req: &testutil.ValidationRequest{
				RequestID:            testutil.MustDeterministicUUID(143).String(),
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "", // Missing
				TransactionTimestamp: timestamp,
				Account:              &testutil.AccountContext{ID: accountID},
			},
			missing:         "currency",
			expectedCode:    "TRC-0223",
			expectedTitle:   "Validation Error",
			expectedMessage: "currency is required",
		},
		{
			name: "missing timestamp",
			req: &testutil.ValidationRequest{
				RequestID:            testutil.MustDeterministicUUID(144).String(),
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: "", // Missing
				Account:              &testutil.AccountContext{ID: accountID},
			},
			missing:         "timestamp",
			expectedCode:    "TRC-0225",
			expectedTitle:   "Validation Error",
			expectedMessage: "transactionTimestamp is required",
		},
		{
			name: "missing account",
			req: &testutil.ValidationRequest{
				RequestID:            testutil.MustDeterministicUUID(145).String(),
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: timestamp,
				Account:              nil, // Missing
			},
			missing:         "account",
			expectedCode:    "TRC-0227",
			expectedTitle:   "Validation Error",
			expectedMessage: "account is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := testutil.CreateValidation(t, tc.req)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"Missing %s should return 400 Bad Request", tc.missing)

			// Verify error response structure per spec
			errorResp := testutil.ParseErrorResponse(t, body)

			assert.Equal(t, tc.expectedCode, errorResp.Code, "Error code should be %s for missing %s", tc.expectedCode, tc.missing)
			assert.Equal(t, tc.expectedTitle, errorResp.Title, "Error title should be %s", tc.expectedTitle)
			assert.Equal(t, tc.expectedMessage, errorResp.Message, "Error message should be %s", tc.expectedMessage)
		})
	}
}

// Test 1.1.11: Validation rejects invalid transactionType
func TestValidation_InvalidTransactionType(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(150).String()
	requestID := testutil.MustDeterministicUUID(151).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "INVALID_TYPE",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Invalid transactionType should return 400")

	// Verify error response structure per spec
	errorResp := testutil.ParseErrorResponse(t, body)

	assert.Equal(t, "TRC-0221", errorResp.Code, "Error code should be TRC-0221 for invalid transactionType")
	assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "transactionType must be one of [CARD, WIRE, PIX, CRYPTO]", errorResp.Message, "Error message should indicate valid transactionTypes")
}

// Test 1.1.12: Validation rejects invalid amount
func TestValidation_InvalidAmount(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(160).String()
	timestamp := testutil.FixedTime().Format(time.RFC3339)

	// Test invalid amounts that should be rejected
	invalidTests := []struct {
		name   string
		amount decimal.Decimal
	}{
		{
			name:   "zero amount",
			amount: decimal.Zero,
		},
		{
			name:   "negative amount",
			amount: decimal.RequireFromString("-1"),
		},
	}

	for i, tc := range invalidTests {
		t.Run(tc.name, func(t *testing.T) {
			req := &testutil.ValidationRequest{
				RequestID:            testutil.MustDeterministicUUID(int64(161 + i)).String(),
				TransactionType:      "CARD",
				Amount:               tc.amount,
				Currency:             "BRL",
				TransactionTimestamp: timestamp,
				Account: &testutil.AccountContext{
					ID: accountID,
				},
			}

			resp, body := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Amount %s should return 400", tc.amount)

			// Verify error response structure per spec
			errorResp := testutil.ParseErrorResponse(t, body)

			assert.Equal(t, "TRC-0222", errorResp.Code, "Error code should be TRC-0222 for non-positive amount")
			assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
			assert.Equal(t, "amount must be positive", errorResp.Message, "Error message should indicate amount must be positive")
		})
	}

	// Test boundary: amount=0.01 should be valid (minimum positive amount)
	t.Run("minimum valid amount (0.01)", func(t *testing.T) {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(163).String(),
			TransactionType:      "CARD",
			Amount:               decimal.RequireFromString("0.01"), // Minimum valid amount
			Currency:             "BRL",
			TransactionTimestamp: timestamp,
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode,
			"Amount 0.01 (minimum valid) should return 201 Created: %s", string(body))
	})
}

// Test 1.1.13: Validation rejects invalid currency
func TestValidation_InvalidCurrency(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(170).String()
	timestamp := testutil.FixedTime().Format(time.RFC3339)

	tests := []struct {
		name     string
		currency string
	}{
		{
			name:     "invalid currency code",
			currency: "INVALID",
		},
		{
			name:     "too short currency code",
			currency: "US",
		},
		{
			name:     "too long currency code",
			currency: "BRLL",
		},
		{
			name:     "numeric currency code",
			currency: "123",
		},
		{
			name:     "empty currency",
			currency: "",
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &testutil.ValidationRequest{
				RequestID:            testutil.MustDeterministicUUID(int64(171 + i)).String(),
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("100"),
				Currency:             tc.currency,
				TransactionTimestamp: timestamp,
				Account: &testutil.AccountContext{
					ID: accountID,
				},
			}

			resp, body := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Currency '%s' should return 400", tc.currency)

			// Verify error response structure per spec
			errorResp := testutil.ParseErrorResponse(t, body)

			// Empty currency returns TRC-0223 (currency required), others return TRC-0224 (invalid currency)
			if tc.currency == "" {
				assert.Equal(t, "TRC-0223", errorResp.Code, "Error code should be TRC-0223 for missing currency")
				assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
				assert.Equal(t, "currency is required", errorResp.Message, "Error message should indicate currency is required")
			} else {
				assert.Equal(t, "TRC-0224", errorResp.Code, "Error code should be TRC-0224 for invalid currency")
				assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
				assert.Equal(t, "currency must be valid ISO 4217 code (e.g., BRL, USD)", errorResp.Message, "Error message should indicate valid currency format")
			}
		})
	}
}

// Test 1.1.14: Validation rejects future timestamp
func TestValidation_1_1_14_FutureTimestamp(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(320).String()
	requestID := testutil.MustDeterministicUUID(321).String()

	// Use a dynamic future timestamp (1 year from now) instead of hardcoded date
	futureTimestamp := time.Now().UTC().Add(365 * 24 * time.Hour).Format(time.RFC3339)

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: futureTimestamp, // Dynamic future timestamp
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Future timestamp should return 400 Bad Request")

	// Verify error response structure
	errorResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "TRC-0226", errorResp.Code, "Error code should be TRC-0226 for future timestamp")
	assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "transactionTimestamp cannot be in the future", errorResp.Message, "Error message should indicate timestamp cannot be in the future")
}

// Test 1.1.15: Validation requires authentication
func TestValidation_RequiresAuthentication(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(180).String()
	requestID := testutil.MustDeterministicUUID(181).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, _ := testutil.CreateValidationWithoutAuth(t, req)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Request without X-API-Key should return 401")
}

// Test 1.1.16: Validation with optional merchant
func TestValidation_OptionalMerchant(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(190).String()
	requestID := testutil.MustDeterministicUUID(191).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		// Merchant is intentionally omitted (nil)
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "Request without merchant should return 201 Created: %s", string(body))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Validation should process normally
	assert.Contains(t, []string{"ALLOW", "DENY", "REVIEW"}, result.Decision)
}

// Test 1.1.17: Validation with scopes array
func TestValidation_1_1_17_ScopesArray(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(330).String()
	segmentID := testutil.MustDeterministicUUID(331).String()
	portfolioID := testutil.MustDeterministicUUID(332).String()
	requestID := testutil.MustDeterministicUUID(333).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "WIRE",
		Amount:               decimal.RequireFromString("250"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		Segment: &testutil.SegmentContext{
			ID: segmentID,
		},
		Portfolio: &testutil.PortfolioContext{
			ID: portfolioID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Validation with multiple scopes should return 201 Created: %s", string(body))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Validation should process normally with valid decision
	assert.Contains(t, []string{"ALLOW", "DENY", "REVIEW"}, result.Decision,
		"Decision should be valid")
	// Rules with scope matching should be evaluated (evaluatedRuleIds may be empty if no rules exist)
	assert.NotNil(t, result.EvaluatedRuleIDs, "evaluatedRuleIds should be present")
}

// Test 1.1.18: Validation with custom metadata
func TestValidation_1_1_18_CustomMetadata(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(340).String()
	requestID := testutil.MustDeterministicUUID(341).String()

	// Create metadata with 50 entries (maximum allowed)
	metadata := make(map[string]interface{})
	for i := 1; i <= 50; i++ {
		key := fmt.Sprintf("field%02d", i)
		metadata[key] = fmt.Sprintf("value%02d", i)
	}

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("150"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		Metadata: metadata,
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Validation with 50 metadata entries should return 201 Created: %s", string(body))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Validation should process normally
	assert.Contains(t, []string{"ALLOW", "DENY", "REVIEW"}, result.Decision,
		"Decision should be valid")

	// Follow-up: GET the validation to verify metadata is persisted
	getResp, getBody := testutil.GetValidation(t, result.ValidationID)
	defer getResp.Body.Close()

	require.Equal(t, http.StatusOK, getResp.StatusCode,
		"GET validation should succeed: %s", string(getBody))

	var detailResult testutil.ValidationDetailResponse
	err = json.Unmarshal(getBody, &detailResult)
	require.NoError(t, err)

	// Verify metadata is present and contains all 50 entries
	require.NotNil(t, detailResult.Metadata, "metadata should be present in GET response")
	assert.Len(t, detailResult.Metadata, 50, "metadata should contain all 50 entries")

	// Verify a few key-value pairs are preserved exactly
	assert.Equal(t, "value01", detailResult.Metadata["field01"], "metadata.field01 should be preserved")
	assert.Equal(t, "value25", detailResult.Metadata["field25"], "metadata.field25 should be preserved")
	assert.Equal(t, "value50", detailResult.Metadata["field50"], "metadata.field50 should be preserved")
}

// Test 1.1.19: Rejects metadata with >50 entries
func TestValidation_1_1_19_RejectsMetadataOverLimit(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(350).String()
	requestID := testutil.MustDeterministicUUID(351).String()

	// Create metadata with 51 entries (exceeds maximum of 50)
	metadata := make(map[string]interface{})
	for i := 1; i <= 51; i++ {
		key := fmt.Sprintf("field%02d", i)
		metadata[key] = fmt.Sprintf("value%02d", i)
	}

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("150"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		Metadata: metadata,
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Metadata with >50 entries should return 400 Bad Request")

	// Verify error response structure
	errorResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "TRC-0063", errorResp.Code, "Error code should be TRC-0063 for metadata entries exceeded")
	assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "metadata exceeds maximum of 50 entries", errorResp.Message, "Error message should indicate metadata entry limit")
}

// Test 1.1.20: Rejects metadata key >64 characters
func TestValidation_1_1_20_RejectsLongMetadataKey(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(352).String()
	requestID := testutil.MustDeterministicUUID(353).String()

	// Create a metadata key with 65 characters (exceeds maximum of 64)
	longKey := "this_key_is_way_too_long_and_exceeds_the_64_character_limit_for_k"

	metadata := map[string]interface{}{
		longKey: "value",
	}

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("150"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		Metadata: metadata,
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Metadata key >64 characters should return 400 Bad Request")

	// Verify error response structure
	errorResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "TRC-0060", errorResp.Code, "Error code should be TRC-0060 for metadata key length exceeded")
	assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "metadata key exceeds maximum length of 64 characters", errorResp.Message, "Error message should indicate key length limit")
}

// Test 1.1.21: Rejects metadata key with invalid characters
func TestValidation_1_1_21_RejectsInvalidMetadataKeyChars(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(354).String()
	timestamp := testutil.FixedTime().Format(time.RFC3339)

	// Test various invalid key patterns (only alphanumeric and underscores allowed)
	tests := []struct {
		name string
		key  string
	}{
		{
			name: "key with hyphen",
			key:  "invalid-key",
		},
		{
			name: "key with dot",
			key:  "invalid.key",
		},
		{
			name: "key with space",
			key:  "invalid key",
		},
		{
			name: "key with at sign",
			key:  "invalid@key",
		},
		{
			name: "key with hash",
			key:  "invalid#key",
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requestID := testutil.MustDeterministicUUID(int64(355 + i)).String()

			metadata := map[string]interface{}{
				tc.key: "value",
			}

			req := &testutil.ValidationRequest{
				RequestID:            requestID,
				TransactionType:      "PIX",
				Amount:               decimal.RequireFromString("150"),
				Currency:             "BRL",
				TransactionTimestamp: timestamp,
				Account: &testutil.AccountContext{
					ID: accountID,
				},
				Metadata: metadata,
			}

			resp, body := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"Metadata key '%s' with invalid characters should return 400 Bad Request", tc.key)

			// Verify error response structure
			errorResp := testutil.ParseErrorResponse(t, body)
			assert.Equal(t, "TRC-0064", errorResp.Code, "Error code should be TRC-0064 for invalid metadata key characters")
			assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
			assert.Equal(t, "metadata key contains invalid characters (only alphanumeric and underscore allowed)", errorResp.Message, "Error message should indicate valid key characters")
		})
	}
}

// Test 1.1.22: Rejects requestId with invalid UUID format
func TestValidation_1_1_22_RejectsInvalidRequestIdUUID(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(360).String()

	tests := []struct {
		name            string
		requestID       string
		expectedCode    string
		expectedTitle   string
		expectedMessage string
	}{
		{
			name:            "invalid UUID format",
			requestID:       "invalid-uuid",
			expectedCode:    "TRC-0003",
			expectedTitle:   "Bad Request",
			expectedMessage: "invalid UUID format in request (check requestId, account.id, segment.id, portfolio.id, merchant.id)",
		},
		{
			name:            "numeric string",
			requestID:       "123",
			expectedCode:    "TRC-0003",
			expectedTitle:   "Bad Request",
			expectedMessage: "invalid UUID format in request (check requestId, account.id, segment.id, portfolio.id, merchant.id)",
		},
		{
			name:            "empty string",
			requestID:       "",
			expectedCode:    "TRC-0220",
			expectedTitle:   "Validation Error",
			expectedMessage: "requestId is required",
		},
		{
			name:            "partial UUID",
			requestID:       "550e8400-e29b-41d4",
			expectedCode:    "TRC-0003",
			expectedTitle:   "Bad Request",
			expectedMessage: "invalid UUID format in request (check requestId, account.id, segment.id, portfolio.id, merchant.id)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &testutil.ValidationRequest{
				RequestID:            tc.requestID,
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

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"Invalid requestId '%s' should return 400 Bad Request", tc.requestID)

			// Verify error response structure per spec
			errorResp := testutil.ParseErrorResponse(t, body)

			assert.Equal(t, tc.expectedCode, errorResp.Code, "Error code should be %s", tc.expectedCode)
			assert.Equal(t, tc.expectedTitle, errorResp.Title, "Error title should be %s", tc.expectedTitle)
			assert.Equal(t, tc.expectedMessage, errorResp.Message, "Error message should be %s", tc.expectedMessage)
		})
	}
}

// Test 1.1.23: Rejects invalid accountId in account context
func TestValidation_1_1_23_RejectsInvalidAccountIdUUID(t *testing.T) {
	requestID := testutil.MustDeterministicUUID(361).String()

	tests := []struct {
		name            string
		accountID       string
		expectedCode    string
		expectedTitle   string
		expectedMessage string
	}{
		{
			name:            "invalid UUID format",
			accountID:       "invalid-uuid",
			expectedCode:    "TRC-0003",
			expectedTitle:   "Bad Request",
			expectedMessage: "invalid UUID format in request (check requestId, account.id, segment.id, portfolio.id, merchant.id)",
		},
		{
			name:            "numeric string",
			accountID:       "12345",
			expectedCode:    "TRC-0003",
			expectedTitle:   "Bad Request",
			expectedMessage: "invalid UUID format in request (check requestId, account.id, segment.id, portfolio.id, merchant.id)",
		},
		{
			name:            "empty string",
			accountID:       "",
			expectedCode:    "TRC-0227",
			expectedTitle:   "Validation Error",
			expectedMessage: "account is required",
		},
		{
			name:            "malformed UUID",
			accountID:       "not-a-valid-uuid-format",
			expectedCode:    "TRC-0003",
			expectedTitle:   "Bad Request",
			expectedMessage: "invalid UUID format in request (check requestId, account.id, segment.id, portfolio.id, merchant.id)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &testutil.ValidationRequest{
				RequestID:            requestID,
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
				Account: &testutil.AccountContext{
					ID: tc.accountID,
				},
			}

			resp, body := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"Invalid accountId '%s' should return 400 Bad Request", tc.accountID)

			// Verify error response structure per spec
			errorResp := testutil.ParseErrorResponse(t, body)

			assert.Equal(t, tc.expectedCode, errorResp.Code, "Error code should be %s", tc.expectedCode)
			assert.Equal(t, tc.expectedTitle, errorResp.Title, "Error title should be %s", tc.expectedTitle)
			assert.Equal(t, tc.expectedMessage, errorResp.Message, "Error message should be %s", tc.expectedMessage)
		})
	}
}

// Test 1.1.24: Rejects invalid segmentId in account context
func TestValidation_1_1_24_RejectsInvalidSegmentIdUUID(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(362).String()
	requestID := testutil.MustDeterministicUUID(363).String()

	tests := []struct {
		name      string
		segmentID string
	}{
		{
			name:      "invalid UUID format",
			segmentID: "invalid-uuid",
		},
		{
			name:      "numeric string",
			segmentID: "67890",
		},
		{
			name:      "malformed UUID",
			segmentID: "not-valid-segment-id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &testutil.ValidationRequest{
				RequestID:            requestID,
				TransactionType:      "WIRE",
				Amount:               decimal.RequireFromString("150"),
				Currency:             "BRL",
				TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
				Account: &testutil.AccountContext{
					ID: accountID,
				},
				Segment: &testutil.SegmentContext{
					ID: tc.segmentID,
				},
			}

			resp, body := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"Invalid segmentId '%s' should return 400 Bad Request", tc.segmentID)

			// Verify error response structure per spec
			errorResp := testutil.ParseErrorResponse(t, body)

			assert.Equal(t, "TRC-0003", errorResp.Code, "Error code should be TRC-0003 for invalid UUID format")
			assert.Equal(t, "Bad Request", errorResp.Title, "Error title should be Bad Request")
			assert.Equal(t, "invalid UUID format in request (check requestId, account.id, segment.id, portfolio.id, merchant.id)", errorResp.Message, "Error message should match expected format")
		})
	}
}

// Test 1.1.25: Rejects invalid portfolioId in account context
func TestValidation_1_1_25_RejectsInvalidPortfolioIdUUID(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(364).String()
	requestID := testutil.MustDeterministicUUID(365).String()

	tests := []struct {
		name        string
		portfolioID string
	}{
		{
			name:        "invalid UUID format",
			portfolioID: "invalid-uuid",
		},
		{
			name:        "numeric string",
			portfolioID: "11111",
		},
		{
			name:        "malformed UUID",
			portfolioID: "bad-portfolio-id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &testutil.ValidationRequest{
				RequestID:            requestID,
				TransactionType:      "PIX",
				Amount:               decimal.RequireFromString("200"),
				Currency:             "BRL",
				TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
				Account: &testutil.AccountContext{
					ID: accountID,
				},
				Portfolio: &testutil.PortfolioContext{
					ID: tc.portfolioID,
				},
			}

			resp, body := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"Invalid portfolioId '%s' should return 400 Bad Request", tc.portfolioID)

			// Verify error response structure per spec
			errorResp := testutil.ParseErrorResponse(t, body)

			assert.Equal(t, "TRC-0003", errorResp.Code, "Error code should be TRC-0003 for invalid UUID format")
			assert.Equal(t, "Bad Request", errorResp.Title, "Error title should be Bad Request")
			assert.Equal(t, "invalid UUID format in request (check requestId, account.id, segment.id, portfolio.id, merchant.id)", errorResp.Message, "Error message should match expected format")
		})
	}
}

// Test 1.1.26: Rejects invalid merchantId in merchant context
func TestValidation_1_1_26_RejectsInvalidMerchantIdUUID(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(366).String()
	requestID := testutil.MustDeterministicUUID(367).String()

	tests := []struct {
		name       string
		merchantID string
	}{
		{
			name:       "invalid UUID format",
			merchantID: "invalid-uuid",
		},
		{
			name:       "numeric string",
			merchantID: "99999",
		},
		{
			name:       "malformed UUID",
			merchantID: "bad-merchant-id-format",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &testutil.ValidationRequest{
				RequestID:            requestID,
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("250"),
				Currency:             "BRL",
				TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
				Account: &testutil.AccountContext{
					ID: accountID,
				},
				Merchant: &testutil.MerchantContext{
					ID:   tc.merchantID,
					MCC:  "5411",
					Name: "Test Merchant",
				},
			}

			resp, body := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"Invalid merchantId '%s' should return 400 Bad Request", tc.merchantID)

			// Verify error response structure per spec
			errorResp := testutil.ParseErrorResponse(t, body)

			assert.Equal(t, "TRC-0003", errorResp.Code, "Error code should be TRC-0003 for invalid UUID format")
			assert.Equal(t, "Bad Request", errorResp.Title, "Error title should be Bad Request")
			assert.Equal(t, "invalid UUID format in request (check requestId, account.id, segment.id, portfolio.id, merchant.id)", errorResp.Message, "Error message should match expected format")
		})
	}
}

// Test 1.1.27: Validation returns 504 on processing timeout
// Uses fault injection middleware to simulate timeout scenario.
func TestValidation_1_1_27_Returns504OnProcessingTimeout(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1127).String()
	requestID := testutil.MustDeterministicUUID(11271).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	// Use fault injection to simulate timeout
	resp, body := testutil.CreateValidationWithFaultInjection(t, req, testutil.FaultTimeout)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode,
		"Processing timeout should return 504 Gateway Timeout, got: %s", string(body))

	// Verify error response structure
	errorResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "TRC-0229", errorResp.Code, "Error code should be TRC-0229 (DeadlineExceeded)")
	assert.Equal(t, "Gateway Timeout", errorResp.Title, "Error title should be Gateway Timeout")
	assert.Equal(t, "validation timeout", errorResp.Message, "Error message should indicate validation timeout")
}

// Test 1.1.28: Validation returns 503 on service unavailable
// Uses fault injection middleware to simulate service unavailable scenario.
func TestValidation_1_1_28_Returns503OnServiceUnavailable(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1128).String()
	requestID := testutil.MustDeterministicUUID(11281).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	// Use fault injection to simulate service unavailable
	resp, body := testutil.CreateValidationWithFaultInjection(t, req, testutil.FaultUnavailable)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
		"Service unavailable should return 503, got: %s", string(body))

	// Verify error response structure
	errorResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "TRC-0012", errorResp.Code, "Error code should be TRC-0012 (ServiceUnavailable)")
	assert.Equal(t, "Service Unavailable", errorResp.Title, "Error title should be Service Unavailable")
	assert.Equal(t, "service temporarily unavailable", errorResp.Message, "Error message should indicate service unavailable")
}

// Test 1.1.29: Validation accepts decimal amount values
func TestValidation_1_1_29_AcceptsDecimalAmount(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1129).String()
	requestID := testutil.MustDeterministicUUID(11291).String()

	// Send raw JSON with decimal amount (1.50)
	jsonPayload := fmt.Sprintf(`{
		"requestId": "%s",
		"transactionType": "CARD",
		"amount": "1.50",
		"currency": "BRL",
		"transactionTimestamp": "%s",
		"account": {"accountId": "%s"},
		"scopes": [{"accountId": "%s"}]
	}`, requestID, testutil.FixedTime().Format(time.RFC3339), accountID, accountID)

	resp, body := testutil.CreateValidationRaw(t, []byte(jsonPayload))
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode,
		"Decimal amount should return 201 Created (decimal.Decimal accepts fractional values), body: %s", string(body))
}

// Test 1.1.30: Validation rejects timestamp without timezone
func TestValidation_1_1_30_RejectsTimestampWithoutTimezone(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1130).String()
	timestamp := testutil.FixedTime().Format(time.RFC3339)

	tests := []struct {
		name      string
		timestamp string
		expected  int
	}{
		{
			name:      "no timezone",
			timestamp: "2024-01-15T10:30:00",
			expected:  http.StatusBadRequest,
		},
		{
			name:      "date only",
			timestamp: "2024-01-15",
			expected:  http.StatusBadRequest,
		},
		{
			name:      "valid UTC timezone",
			timestamp: timestamp,
			expected:  http.StatusCreated,
		},
		{
			name:      "valid local timezone with offset",
			timestamp: testutil.FixedTime().In(time.FixedZone("BRT", -3*3600)).Format(time.RFC3339),
			expected:  http.StatusCreated,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requestID := testutil.MustDeterministicUUID(int64(11301 + i)).String()

			req := &testutil.ValidationRequest{
				RequestID:            requestID,
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: tc.timestamp,
				Account: &testutil.AccountContext{
					ID: accountID,
				},
			}

			resp, body := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			assert.Equal(t, tc.expected, resp.StatusCode,
				"Timestamp '%s' should return %d", tc.timestamp, tc.expected)

			// Verify error response for invalid timestamps
			if tc.expected == http.StatusBadRequest {
				errorResp := testutil.ParseErrorResponse(t, body)
				assert.Equal(t, "TRC-0003", errorResp.Code, "Error code should be TRC-0003 for invalid timestamp format")
				assert.Equal(t, "Bad Request", errorResp.Title, "Error title should be Bad Request")
				assert.Equal(t, "timestamp: invalid format (expected RFC3339)", errorResp.Message, "Error message should indicate RFC3339 format required")
			}
		})
	}
}

// Test 1.1.31: Validation with invalid API Key format returns 401
func TestValidation_1_1_31_InvalidAPIKeyFormat(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1131).String()
	requestID := testutil.MustDeterministicUUID(11311).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	tests := []struct {
		name   string
		apiKey string
	}{
		{
			name:   "invalid key format",
			apiKey: "invalid-key-format",
		},
		{
			name:   "empty string",
			apiKey: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := testutil.CreateValidationWithAPIKey(t, req, tc.apiKey)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"API Key '%s' should return 401 Unauthorized", tc.apiKey)

			// Verify error response structure
			errorResp := testutil.ParseErrorResponse(t, body)
			assert.Equal(t, "Unauthenticated", errorResp.Code, "Error code should be Unauthenticated for invalid API key")
			assert.Equal(t, "Unauthorized", errorResp.Title, "Error title should be Unauthorized")
			assert.Equal(t, "API Key missing or invalid", errorResp.Message, "Error message should indicate API key issue")
		})
	}
}

// Test 1.1.32: Validation rejects invalid account.type value
func TestValidation_1_1_32_RejectsInvalidAccountType(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1132).String()
	timestamp := testutil.FixedTime().Format(time.RFC3339)

	tests := []struct {
		name        string
		accountType string
		expected    int
	}{
		{
			name:        "valid checking",
			accountType: "checking",
			expected:    http.StatusCreated,
		},
		{
			name:        "valid savings",
			accountType: "savings",
			expected:    http.StatusCreated,
		},
		{
			name:        "valid credit",
			accountType: "credit",
			expected:    http.StatusCreated,
		},
		{
			name:        "invalid INVALID",
			accountType: "INVALID",
			expected:    http.StatusBadRequest,
		},
		{
			name:        "invalid debit",
			accountType: "debit",
			expected:    http.StatusBadRequest,
		},
		{
			name:        "omitted (empty string with omitempty)",
			accountType: "",
			expected:    http.StatusCreated, // Empty string with omitempty = field not sent = optional field OK
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requestID := testutil.MustDeterministicUUID(int64(11321 + i)).String()

			req := &testutil.ValidationRequest{
				RequestID:            requestID,
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: timestamp,
				Account: &testutil.AccountContext{
					ID:   accountID,
					Type: tc.accountType,
				},
			}

			resp, body := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			assert.Equal(t, tc.expected, resp.StatusCode,
				"account.type '%s' should return %d", tc.accountType, tc.expected)

			// Verify error response structure for invalid cases
			if tc.expected == http.StatusBadRequest {
				errorResp := testutil.ParseErrorResponse(t, body)

				assert.Equal(t, "TRC-0233", errorResp.Code, "Error code should be TRC-0233 for invalid account type")
				assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
				assert.Equal(t, "account.type must be one of: checking, savings, credit", errorResp.Message, "Error message should indicate valid account types")
			}
		})
	}
}

// Test 1.1.33: Validation rejects invalid account.status value
func TestValidation_1_1_33_RejectsInvalidAccountStatus(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1133).String()
	timestamp := testutil.FixedTime().Format(time.RFC3339)

	tests := []struct {
		name          string
		accountStatus string
		expected      int
	}{
		{
			name:          "valid active",
			accountStatus: "active",
			expected:      http.StatusCreated,
		},
		{
			name:          "valid suspended",
			accountStatus: "suspended",
			expected:      http.StatusCreated,
		},
		{
			name:          "valid closed",
			accountStatus: "closed",
			expected:      http.StatusCreated,
		},
		{
			name:          "invalid INVALID",
			accountStatus: "INVALID",
			expected:      http.StatusBadRequest,
		},
		{
			name:          "invalid blocked",
			accountStatus: "blocked",
			expected:      http.StatusBadRequest,
		},
		{
			name:          "omitted (empty string with omitempty)",
			accountStatus: "",
			expected:      http.StatusCreated, // Empty string with omitempty = field not sent = optional field OK
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requestID := testutil.MustDeterministicUUID(int64(11331 + i)).String()

			req := &testutil.ValidationRequest{
				RequestID:            requestID,
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: timestamp,
				Account: &testutil.AccountContext{
					ID:     accountID,
					Status: tc.accountStatus,
				},
			}

			resp, body := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			assert.Equal(t, tc.expected, resp.StatusCode,
				"account.status '%s' should return %d", tc.accountStatus, tc.expected)

			// Verify error response structure for invalid cases
			if tc.expected == http.StatusBadRequest {
				errorResp := testutil.ParseErrorResponse(t, body)

				assert.Equal(t, "TRC-0234", errorResp.Code, "Error code should be TRC-0234 for invalid account status")
				assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
				assert.Equal(t, "account.status must be one of: active, suspended, closed", errorResp.Message, "Error message should indicate valid account statuses")
			}
		})
	}
}

// Test 1.1.34: Validation rejects invalid merchant.category (MCC)
func TestValidation_1_1_34_RejectsInvalidMerchantCategory(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1134).String()
	merchantID := testutil.MustDeterministicUUID(11341).String()
	timestamp := testutil.FixedTime().Format(time.RFC3339)

	tests := []struct {
		name     string
		category string
		expected int
	}{
		{
			name:     "valid MCC 5411",
			category: "5411",
			expected: http.StatusCreated,
		},
		{
			name:     "valid MCC 5812",
			category: "5812",
			expected: http.StatusCreated,
		},
		{
			name:     "invalid non-numeric",
			category: "ABCD",
			expected: http.StatusBadRequest,
		},
		{
			name:     "invalid too short",
			category: "123",
			expected: http.StatusBadRequest,
		},
		{
			name:     "invalid too long",
			category: "12345",
			expected: http.StatusBadRequest,
		},
		{
			name:     "omitted (empty string with omitempty)",
			category: "",
			expected: http.StatusCreated, // Empty string with omitempty = field not sent = optional field OK
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requestID := testutil.MustDeterministicUUID(int64(11342 + i)).String()

			req := &testutil.ValidationRequest{
				RequestID:            requestID,
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: timestamp,
				Account: &testutil.AccountContext{
					ID: accountID,
				},
				Merchant: &testutil.MerchantContext{
					ID:       merchantID,
					Category: tc.category,
				},
			}

			resp, body := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			assert.Equal(t, tc.expected, resp.StatusCode,
				"merchant.category '%s' should return %d", tc.category, tc.expected)

			// Verify error response structure for invalid cases
			if tc.expected == http.StatusBadRequest {
				errorResp := testutil.ParseErrorResponse(t, body)

				assert.Equal(t, "TRC-0235", errorResp.Code, "Error code should be TRC-0235 for invalid merchant category")
				assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
				assert.Equal(t, "merchant.category must be a 4-digit MCC code", errorResp.Message, "Error message should indicate valid MCC format")
			}
		})
	}
}

// Test 1.1.35: Validation rejects invalid merchant.country format
func TestValidation_1_1_35_RejectsInvalidMerchantCountry(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1135).String()
	merchantID := testutil.MustDeterministicUUID(11351).String()
	timestamp := testutil.FixedTime().Format(time.RFC3339)

	tests := []struct {
		name     string
		country  string
		expected int
	}{
		{
			name:     "valid BR",
			country:  "BR",
			expected: http.StatusCreated,
		},
		{
			name:     "valid US",
			country:  "US",
			expected: http.StatusCreated,
		},
		{
			name:     "invalid alpha-3",
			country:  "BRA",
			expected: http.StatusBadRequest,
		},
		{
			name:     "invalid too short",
			country:  "B",
			expected: http.StatusBadRequest,
		},
		{
			name:     "invalid full name",
			country:  "BRAZIL",
			expected: http.StatusBadRequest,
		},
		{
			name:     "invalid numeric",
			country:  "12",
			expected: http.StatusBadRequest,
		},
		{
			name:     "invalid lowercase",
			country:  "br",
			expected: http.StatusBadRequest, // Must be uppercase per ISO 3166-1 alpha-2
		},
		{
			name:     "omitted (empty string with omitempty)",
			country:  "",
			expected: http.StatusCreated, // Empty string with omitempty = field not sent = optional field OK
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requestID := testutil.MustDeterministicUUID(int64(11352 + i)).String()

			req := &testutil.ValidationRequest{
				RequestID:            requestID,
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: timestamp,
				Account: &testutil.AccountContext{
					ID: accountID,
				},
				Merchant: &testutil.MerchantContext{
					ID:      merchantID,
					Country: tc.country,
				},
			}

			resp, body := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			assert.Equal(t, tc.expected, resp.StatusCode,
				"merchant.country '%s' should return %d", tc.country, tc.expected)

			// Verify error response structure for invalid cases
			if tc.expected == http.StatusBadRequest {
				errorResp := testutil.ParseErrorResponse(t, body)

				assert.Equal(t, "TRC-0236", errorResp.Code, "Error code should be TRC-0236 for invalid merchant country")
				assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
				assert.Equal(t, "merchant.country must be ISO 3166-1 alpha-2 code (e.g., BR, US)", errorResp.Message, "Error message should indicate valid country format")
			}
		})
	}
}

// Test 1.1.36: Validation with timestamp at exact clock skew boundary
func TestValidation_1_1_36_TimestampClockSkewBoundary(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1136).String()
	now := time.Now().UTC()

	tests := []struct {
		name     string
		offset   time.Duration
		expected int
	}{
		{
			name:     "+55 seconds safely within tolerance",
			offset:   55 * time.Second,
			expected: http.StatusCreated,
		},
		{
			name:     "+65 seconds safely beyond tolerance",
			offset:   65 * time.Second,
			expected: http.StatusBadRequest,
		},
		{
			name:     "+90 seconds beyond tolerance",
			offset:   90 * time.Second,
			expected: http.StatusBadRequest,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requestID := testutil.MustDeterministicUUID(int64(11361 + i)).String()
			futureTimestamp := now.Add(tc.offset).Format(time.RFC3339)

			req := &testutil.ValidationRequest{
				RequestID:            requestID,
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: futureTimestamp,
				Account: &testutil.AccountContext{
					ID: accountID,
				},
			}

			resp, _ := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			assert.Equal(t, tc.expected, resp.StatusCode,
				"Timestamp %s (offset %v) should return %d", futureTimestamp, tc.offset, tc.expected)
		})
	}
}

// Test 1.1.37: Validation accepts all valid transactionType values
func TestValidation_1_1_37_ValidTransactionTypes(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1137).String()
	timestamp := testutil.FixedTime().Format(time.RFC3339)

	tests := []struct {
		name            string
		transactionType string
		expected        int
	}{
		{
			name:            "valid CARD",
			transactionType: "CARD",
			expected:        http.StatusCreated,
		},
		{
			name:            "valid WIRE",
			transactionType: "WIRE",
			expected:        http.StatusCreated,
		},
		{
			name:            "valid PIX",
			transactionType: "PIX",
			expected:        http.StatusCreated,
		},
		{
			name:            "valid CRYPTO",
			transactionType: "CRYPTO",
			expected:        http.StatusCreated,
		},
		{
			name:            "invalid lowercase card",
			transactionType: "card",
			expected:        http.StatusBadRequest,
		},
		{
			name:            "invalid mixed case Card",
			transactionType: "Card",
			expected:        http.StatusBadRequest,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requestID := testutil.MustDeterministicUUID(int64(11371 + i)).String()

			req := &testutil.ValidationRequest{
				RequestID:            requestID,
				TransactionType:      tc.transactionType,
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: timestamp,
				Account: &testutil.AccountContext{
					ID: accountID,
				},
			}

			resp, _ := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			assert.Equal(t, tc.expected, resp.StatusCode,
				"transactionType '%s' should return %d", tc.transactionType, tc.expected)
		})
	}
}

// Test 1.1.38: Validation with subType field
func TestValidation_1_1_38_SubTypeField(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(1138).String()
	timestamp := testutil.FixedTime().Format(time.RFC3339)

	// Create a string longer than 50 characters
	longSubType := "this_subtype_is_way_too_long_and_exceeds_the_50_character_limit_defined"

	tests := []struct {
		name     string
		subType  string
		expected int
	}{
		{
			name:     "valid credit",
			subType:  "credit",
			expected: http.StatusCreated,
		},
		{
			name:     "valid debit",
			subType:  "debit",
			expected: http.StatusCreated,
		},
		{
			name:     "valid domestic",
			subType:  "domestic",
			expected: http.StatusCreated,
		},
		{
			name:     "valid international",
			subType:  "international",
			expected: http.StatusCreated,
		},
		{
			name:     "invalid too long",
			subType:  longSubType,
			expected: http.StatusBadRequest,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requestID := testutil.MustDeterministicUUID(int64(11381 + i)).String()

			req := &testutil.ValidationRequest{
				RequestID:            requestID,
				TransactionType:      "CARD",
				SubType:              tc.subType,
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: timestamp,
				Account: &testutil.AccountContext{
					ID: accountID,
				},
			}

			resp, _ := testutil.CreateValidation(t, req)
			defer resp.Body.Close()

			assert.Equal(t, tc.expected, resp.StatusCode,
				"subType '%s' should return %d", tc.subType, tc.expected)
		})
	}
}

// Test 1.1.39: Validation rejects missing accountId in account context
func TestValidation_1_1_39_RejectsMissingAccountId(t *testing.T) {
	requestID := testutil.MustDeterministicUUID(1139).String()
	timestamp := testutil.FixedTime().Format(time.RFC3339)

	// Send raw JSON with account context but no accountId
	jsonPayload := fmt.Sprintf(`{
		"requestId": "%s",
		"transactionType": "CARD",
		"amount": "100.00",
		"currency": "BRL",
		"transactionTimestamp": "%s",
		"account": {
			"type": "checking",
			"status": "active"
		}
	}`, requestID, timestamp)

	resp, body := testutil.CreateValidationRaw(t, []byte(jsonPayload))
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Missing accountId in account context should return 400 Bad Request")

	// Verify error response structure per spec
	errorResp := testutil.ParseErrorResponse(t, body)

	assert.Equal(t, "TRC-0227", errorResp.Code, "Error code should be TRC-0227 for missing accountId")
	assert.Equal(t, "Validation Error", errorResp.Title, "Error title should be Validation Error")
	assert.Equal(t, "account is required", errorResp.Message, "Error message should indicate account is required")
}

// Test 1.1.55: Deactivated rule is not evaluated
// Verifies that deactivating a rule removes it from validation evaluation immediately.
// Type: Integration (Cross-endpoint: Rules + Validations)
func TestValidation_1_1_55_DeactivatedRuleNotEvaluated(t *testing.T) {
	// Preconditions: Create and activate DENY rule
	ruleName := "deny-specific-amount-" + testutil.MustDeterministicUUID(1009).String()[:8]
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "amount == 2500", "DENY")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	accountID := testutil.MustDeterministicUUID(1550).String()

	// Step 1 (Rule ACTIVE): Validate with amount=2500, expect DENY
	requestID1 := testutil.MustDeterministicUUID(1551).String()
	req1 := &testutil.ValidationRequest{
		RequestID:            requestID1,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("2500"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, req1)
	defer resp1.Body.Close()

	require.Equal(t, http.StatusCreated, resp1.StatusCode, "Expected 201 Created, got: %s", string(body1))

	var result1 testutil.ValidationResponse
	err := json.Unmarshal(body1, &result1)
	require.NoError(t, err)

	assert.NotEmpty(t, result1.ValidationID, "ValidationID should be set")
	assert.Equal(t, requestID1, result1.RequestID, "RequestID should match the request")
	assert.Equal(t, "DENY", result1.Decision, "With active rule, decision should be DENY")
	assert.Equal(t, []string{ruleID}, result1.MatchedRuleIDs, "matchedRuleIds should contain exactly the rule ID when active")
	assert.Equal(t, []string{ruleID}, result1.EvaluatedRuleIDs, "evaluatedRuleIds should contain exactly the rule ID when active")
	assert.NotEmpty(t, result1.Reason, "Reason should be set when rule matches")

	// Step 2: Deactivate the rule
	testutil.DeactivateRule(t, ruleID)

	// Step 3 (Rule INACTIVE): Validate with same amount, different requestId, expect ALLOW
	requestID2 := testutil.MustDeterministicUUID(1552).String()
	req2 := &testutil.ValidationRequest{
		RequestID:            requestID2,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("2500"), // Same amount as before
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp2, body2 := testutil.CreateValidation(t, req2)
	defer resp2.Body.Close()

	require.Equal(t, http.StatusCreated, resp2.StatusCode, "Expected 201 Created, got: %s", string(body2))

	var result2 testutil.ValidationResponse
	err = json.Unmarshal(body2, &result2)
	require.NoError(t, err)

	assert.NotEmpty(t, result2.ValidationID, "ValidationID should be set")
	assert.Equal(t, requestID2, result2.RequestID, "RequestID should match the request")
	assert.Equal(t, "ALLOW", result2.Decision, "With inactive rule, decision should be ALLOW")
	assert.NotContains(t, result2.MatchedRuleIDs, ruleID, "matchedRuleIds should NOT contain the deactivated rule ID")
	assert.NotContains(t, result2.EvaluatedRuleIDs, ruleID, "evaluatedRuleIds should NOT contain the deactivated rule ID")
}

// ============================================================================
// 1.2 GET /v1/validations/{validationId} - Validation Query
// ============================================================================

// Test 1.2.1: Retrieves validation by ID
func TestValidation_1_2_1_RetrievesValidationByID(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(400).String()
	// Seed 12001 avoids collision with seed 104 used in TestValidation_CompletePayload,
	// which shares the same request_id column now enforced as UNIQUE by migration 000009.
	requestID := testutil.MustDeterministicUUID(12001).String()

	// First create a validation
	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("500"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	var createResult testutil.ValidationResponse
	err := json.Unmarshal(body, &createResult)
	require.NoError(t, err)

	// Use validationId as the validation identifier (server-generated unique ID)
	validationID := createResult.ValidationID

	// Now retrieve the validation
	getResp, getBody := testutil.GetValidation(t, validationID)
	defer getResp.Body.Close()

	require.Equal(t, http.StatusOK, getResp.StatusCode,
		"Expected 200 OK from GET /v1/validations/{id}, got: %s", string(getBody))

	var result testutil.ValidationDetailResponse
	err = json.Unmarshal(getBody, &result)
	require.NoError(t, err)

	// Verify ValidationDetail fields per API design (Section 4.1.2)
	// Required fields from the original request
	assert.Equal(t, validationID, result.ID, "validationId should match")
	assert.Equal(t, requestID, result.RequestID, "requestId should be echoed")
	assert.Equal(t, "PIX", result.TransactionType, "transactionType should be preserved")
	assert.True(t, decimal.RequireFromString("500").Equal(result.Amount), "amount should be preserved")
	assert.Equal(t, "BRL", result.Currency, "currency should be preserved")
	assert.NotEmpty(t, result.TransactionTimestamp, "transactionTimestamp should be present")

	// Account context should be present
	require.NotNil(t, result.Account, "account should be present")
	assert.Equal(t, accountID, result.Account["accountId"], "account.accountId should be preserved")

	// Verify response contains decision, reason, matchedRuleIds, evaluatedRuleIds
	assert.Contains(t, []string{"ALLOW", "DENY", "REVIEW"}, result.Decision,
		"decision should be valid")
	assert.NotEmpty(t, result.Reason, "reason should be present")
	assert.NotNil(t, result.MatchedRuleIDs, "matchedRuleIds should be present")
	assert.NotNil(t, result.EvaluatedRuleIDs, "evaluatedRuleIds should be present")

	// Verify response contains limitUsageDetails, processingTimeMs, createdAt
	assert.NotNil(t, result.LimitUsageDetails, "limitUsageDetails should be present")
	assert.GreaterOrEqual(t, result.ProcessingTimeMs, float64(0), "processingTimeMs should be >= 0")
	assert.NotEmpty(t, result.CreatedAt, "createdAt should be present")
}

// Test 1.2.2: Returns 404 for non-existent ID
func TestValidation_1_2_2_Returns404ForNonExistentID(t *testing.T) {
	// Use a UUID that doesn't exist
	nonExistentID := testutil.MustDeterministicUUID(410).String()

	resp, _ := testutil.GetValidation(t, nonExistentID)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"GET /v1/validations/{non-existent-id} should return 404 Not Found")
}

// Test 1.2.3: Returns 400 for invalid UUID
func TestValidation_1_2_3_Returns400ForInvalidUUID(t *testing.T) {
	// Use an invalid UUID format
	invalidUUID := "invalid-uuid"

	resp, _ := testutil.GetValidation(t, invalidUUID)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"GET /v1/validations/{invalid-uuid} should return 400 Bad Request")
}

// Test 1.2.4: Requires authentication
func TestValidation_1_2_4_RequiresAuthentication(t *testing.T) {
	// First create a validation to get a valid ID
	accountID := testutil.MustDeterministicUUID(420).String()
	requestID := testutil.MustDeterministicUUID(421).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
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

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	var createResult testutil.ValidationResponse
	err := json.Unmarshal(body, &createResult)
	require.NoError(t, err)

	validationID := createResult.ValidationID

	// Try to get validation without authentication
	getResp, _ := testutil.GetValidationWithoutAuth(t, validationID)
	defer getResp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, getResp.StatusCode,
		"GET /v1/validations/{id} without X-API-Key should return 401 Unauthorized")
}

// Test 1.2.5: Complete snapshot preserved (AccountContext verification)
func TestValidation_1_2_5_CompleteSnapshotPreserved(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(430).String()
	segmentID := testutil.MustDeterministicUUID(431).String()
	requestID := testutil.MustDeterministicUUID(432).String()
	subType := "credit"

	// Create validation with subType, scopes, and complete account context
	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		SubType:              subType,
		Amount:               decimal.RequireFromString("750"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID:     accountID,
			Type:   "checking", // Added for AccountContext verification
			Status: "active",   // Added for AccountContext verification
		},
		Segment: &testutil.SegmentContext{
			ID: segmentID,
		},
		Metadata: map[string]interface{}{
			"channel": "mobile",
			"testKey": "testValue",
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	var createResult testutil.ValidationResponse
	err := json.Unmarshal(body, &createResult)
	require.NoError(t, err)

	validationID := createResult.ValidationID

	// Retrieve the validation
	getResp, getBody := testutil.GetValidation(t, validationID)
	defer getResp.Body.Close()

	require.Equal(t, http.StatusOK, getResp.StatusCode,
		"Expected 200 OK from GET, got: %s", string(getBody))

	var result testutil.ValidationDetailResponse
	err = json.Unmarshal(getBody, &result)
	require.NoError(t, err)

	// Verify ValidationDetail fields per API design (Section 4.1.2)
	// All original payload fields should be preserved at top level
	assert.Equal(t, "CARD", result.TransactionType, "transactionType should be preserved")
	assert.True(t, decimal.RequireFromString("750").Equal(result.Amount), "amount should be preserved")
	assert.Equal(t, "BRL", result.Currency, "currency should be preserved")

	// Verify subType is preserved
	require.NotNil(t, result.SubType, "subType should be present")
	assert.Equal(t, subType, *result.SubType, "subType should be preserved")

	// Verify account context is preserved (per API design Section 6.11 AccountContext)
	require.NotNil(t, result.Account, "account should be present")
	assert.Equal(t, accountID, result.Account["accountId"], "account.accountId should be preserved")
	assert.Equal(t, "checking", result.Account["type"], "account.type should be preserved")
	assert.Equal(t, "active", result.Account["status"], "account.status should be preserved")

	// Verify segment context is preserved
	require.NotNil(t, result.Segment, "segment should be present")
	assert.Equal(t, segmentID, result.Segment["segmentId"], "segment.segmentId should be preserved")

	// Verify metadata is preserved
	require.NotNil(t, result.Metadata, "metadata should be present")
	assert.Equal(t, "mobile", result.Metadata["channel"], "metadata.channel should be preserved")
	assert.Equal(t, "testValue", result.Metadata["testKey"], "metadata.testKey should be preserved")
}

// Test 1.2.6: Complete response snapshot (MerchantContext verification)
func TestValidation_1_2_6_CompleteResponseSnapshot(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(440).String()
	merchantID := testutil.MustDeterministicUUID(441).String()
	requestID := testutil.MustDeterministicUUID(442).String()

	// Create validation with complete merchant context
	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("300"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		Merchant: &testutil.MerchantContext{
			ID:       merchantID,
			Name:     "Test Store",
			MCC:      "5411", // Grocery Stores
			Category: "5411",
			Country:  "BR",
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	var createResult testutil.ValidationResponse
	err := json.Unmarshal(body, &createResult)
	require.NoError(t, err)

	validationID := createResult.ValidationID

	// Retrieve the validation
	getResp, getBody := testutil.GetValidation(t, validationID)
	defer getResp.Body.Close()

	require.Equal(t, http.StatusOK, getResp.StatusCode,
		"Expected 200 OK from GET, got: %s", string(getBody))

	var result testutil.ValidationDetailResponse
	err = json.Unmarshal(getBody, &result)
	require.NoError(t, err)

	// Verify ValidationDetail response fields per API design (Section 4.1.2)
	// Decision and reason should be at top level
	assert.Contains(t, []string{"ALLOW", "DENY", "REVIEW"}, result.Decision,
		"decision should be valid")
	assert.NotEmpty(t, result.Reason, "reason should be present")

	// Verify UUID arrays are preserved correctly at top level
	assert.NotNil(t, result.MatchedRuleIDs, "matchedRuleIds should be present as array")
	assert.NotNil(t, result.EvaluatedRuleIDs, "evaluatedRuleIds should be present as array")
	assert.NotNil(t, result.LimitUsageDetails, "limitUsageDetails should be present as array")

	// Verify processing info
	assert.GreaterOrEqual(t, result.ProcessingTimeMs, float64(0), "processingTimeMs should be >= 0")
	assert.NotEmpty(t, result.CreatedAt, "createdAt should be present")

	// Verify merchant context is preserved (per API design Section 6.15 MerchantContext)
	require.NotNil(t, result.Merchant, "merchant should be present")
	assert.Equal(t, merchantID, result.Merchant["merchantId"], "merchant.merchantId should be preserved")
	assert.Equal(t, "Test Store", result.Merchant["name"], "merchant.name should be preserved")
	assert.Equal(t, "5411", result.Merchant["category"], "merchant.category should be preserved")
	assert.Equal(t, "BR", result.Merchant["country"], "merchant.country should be preserved")
}

// Test 1.2.7: Complete data preserved with segment and portfolio (segment.name/portfolio.name assertions)
func TestValidation_1_2_7_CompleteDataWithSegmentPortfolio(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(450).String()
	segmentID := testutil.MustDeterministicUUID(451).String()
	portfolioID := testutil.MustDeterministicUUID(452).String()
	requestID := testutil.MustDeterministicUUID(453).String()

	// Create validation with segment name and portfolio name included
	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		SubType:              "debit",
		Amount:               decimal.RequireFromString("850"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		Segment: &testutil.SegmentContext{
			ID:   segmentID,
			Name: "Premium", // Added for segment.name assertion
		},
		Portfolio: &testutil.PortfolioContext{
			ID:   portfolioID,
			Name: "Corporate", // Added for portfolio.name assertion
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	var createResult testutil.ValidationResponse
	err := json.Unmarshal(body, &createResult)
	require.NoError(t, err)

	validationID := createResult.ValidationID

	// Retrieve the validation by ID
	getResp, getBody := testutil.GetValidation(t, validationID)
	defer getResp.Body.Close()

	require.Equal(t, http.StatusOK, getResp.StatusCode,
		"Expected 200 OK from GET, got: %s", string(getBody))

	var result testutil.ValidationDetailResponse
	err = json.Unmarshal(getBody, &result)
	require.NoError(t, err)

	// Verify ValidationDetail fields per API design (Section 4.1.2)
	// Transaction data should be at top level
	assert.Equal(t, "CARD", result.TransactionType, "transactionType should be preserved")
	assert.True(t, decimal.RequireFromString("850").Equal(result.Amount), "amount should be preserved")

	// Verify subType is preserved
	require.NotNil(t, result.SubType, "subType should be present")
	assert.Equal(t, "debit", *result.SubType, "subType should be preserved")

	// Verify account context is preserved
	require.NotNil(t, result.Account, "account should be present")
	assert.Equal(t, accountID, result.Account["accountId"], "account.accountId should be preserved")

	// Verify segment context is preserved (per API design: SegmentContext at top level)
	require.NotNil(t, result.Segment, "segment should be present")
	assert.Equal(t, segmentID, result.Segment["segmentId"], "segment.segmentId should be preserved")
	assert.Equal(t, "Premium", result.Segment["name"], "segment.name should be preserved")

	// Verify portfolio context is preserved (per API design: PortfolioContext at top level)
	require.NotNil(t, result.Portfolio, "portfolio should be present")
	assert.Equal(t, portfolioID, result.Portfolio["portfolioId"], "portfolio.portfolioId should be preserved")
	assert.Equal(t, "Corporate", result.Portfolio["name"], "portfolio.name should be preserved")

	// Verify decision and arrays are present
	assert.Contains(t, []string{"ALLOW", "DENY", "REVIEW"}, result.Decision, "decision should be valid")
	assert.NotNil(t, result.MatchedRuleIDs, "matchedRuleIds should be present")
	assert.NotNil(t, result.EvaluatedRuleIDs, "evaluatedRuleIds should be present")
}

// Test 1.2.8: Response data preserved with complete LimitUsage (scope format verification)
func TestValidation_1_2_8_CompleteLimitUsagePreserved(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(460).String()
	requestID := testutil.MustDeterministicUUID(461).String()

	// Create a limit for the account with a specific maxAmount
	maxAmount := "2000"
	limitID := createTestLimitWithAccountScope(t, accountID, maxAmount)
	activateTestLimit(t, limitID)
	t.Cleanup(func() {
		cleanupTestLimit(t, limitID)
	})

	// Create validation that consumes part of the limit
	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("500"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	var createResult testutil.ValidationResponse
	err := json.Unmarshal(body, &createResult)
	require.NoError(t, err)

	validationID := createResult.ValidationID

	// Retrieve the validation by ID
	getResp, getBody := testutil.GetValidation(t, validationID)
	defer getResp.Body.Close()

	require.Equal(t, http.StatusOK, getResp.StatusCode,
		"Expected 200 OK from GET, got: %s", string(getBody))

	var result testutil.ValidationDetailResponse
	err = json.Unmarshal(getBody, &result)
	require.NoError(t, err)

	// Verify limitUsageDetails contains the expected structure
	assert.NotNil(t, result.LimitUsageDetails, "limitUsageDetails should be present")
	require.NotEmpty(t, result.LimitUsageDetails, "limitUsageDetails should not be empty")

	// Find our limit in the details
	var foundLimit *testutil.LimitUsageDetail
	for i := range result.LimitUsageDetails {
		if result.LimitUsageDetails[i].LimitID == limitID {
			foundLimit = &result.LimitUsageDetails[i]
			break
		}
	}

	require.NotNil(t, foundLimit, "limitUsageDetails should contain our limit with ID %s", limitID)

	// Verify LimitUsage fields are present and correct
	assert.Equal(t, limitID, foundLimit.LimitID, "limitId should match the created limit")
	assert.True(t, decimal.RequireFromString("2000").Equal(foundLimit.LimitAmount), "limitAmount should be the configured max amount")
	assert.True(t, decimal.RequireFromString("500").Equal(foundLimit.CurrentUsage), "currentUsage should reflect the consumed amount")
	assert.False(t, foundLimit.Exceeded, "exceeded should be false for this non-exceeding transaction")

	// Verify scope format (per API design Section 4.1.1: format "{scopeType}:{scopeValue}")
	// Example: "accountId:550e8400-e29b-41d4-a716-446655440000"
	assert.NotEmpty(t, foundLimit.Scope, "scope should be present")
	assert.Contains(t, foundLimit.Scope, ":", "scope should have format 'scopeType:scopeValue'")
	// Scope should contain the accountId since we created an account-scoped limit
	assert.Contains(t, foundLimit.Scope, accountID, "scope should contain the account_id")

	// Verify period is present and valid
	assert.NotEmpty(t, foundLimit.Period, "period should be present")
	assert.Contains(t, []string{"DAILY", "MONTHLY", "PER_TRANSACTION"}, foundLimit.Period,
		"period should be one of DAILY, MONTHLY, PER_TRANSACTION")
}

// Test 1.2.9: Validates createdAt is ISO 8601 timestamp
func TestValidation_1_2_9_CreatedAtIsISO8601(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(470).String()
	requestID := testutil.MustDeterministicUUID(471).String()

	// Record time before creating validation
	beforeCreate := time.Now().UTC()

	// Create validation
	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "WIRE",
		Amount:               decimal.RequireFromString("400"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	var createResult testutil.ValidationResponse
	err := json.Unmarshal(body, &createResult)
	require.NoError(t, err)

	validationID := createResult.ValidationID

	// Record time after creating validation
	afterCreate := time.Now().UTC()

	// Retrieve the validation by ID
	getResp, getBody := testutil.GetValidation(t, validationID)
	defer getResp.Body.Close()

	require.Equal(t, http.StatusOK, getResp.StatusCode,
		"Expected 200 OK from GET, got: %s", string(getBody))

	var result testutil.ValidationDetailResponse
	err = json.Unmarshal(getBody, &result)
	require.NoError(t, err)

	// Verify createdAt is present and not empty
	assert.NotEmpty(t, result.CreatedAt, "createdAt should be present and not empty")

	// Verify createdAt is a valid ISO 8601 timestamp (RFC3339 format)
	parsedTime, err := time.Parse(time.RFC3339, result.CreatedAt)
	require.NoError(t, err, "createdAt should be a valid ISO 8601 (RFC3339) timestamp, got: %s", result.CreatedAt)

	// Verify createdAt is not in the future
	assert.True(t, !parsedTime.After(time.Now().UTC().Add(time.Minute)),
		"createdAt should not be in the future")

	// Verify createdAt is within reasonable range of when validation was created
	// Allow 1 minute tolerance for clock differences
	assert.True(t, parsedTime.After(beforeCreate.Add(-time.Minute)),
		"createdAt should be after test start time (with 1 minute tolerance)")
	assert.True(t, parsedTime.Before(afterCreate.Add(time.Minute)),
		"createdAt should be before test end time (with 1 minute tolerance)")
}

// ============================================================================
// 1.3 GET /v1/validations - Validation History
// ============================================================================

// Test 1.3.1: Lists validations without filters
func TestValidation_1_3_1_ListsValidationsWithoutFilters(t *testing.T) {
	// First create a validation to ensure there's at least one record
	accountID := testutil.MustDeterministicUUID(500).String()
	requestID := testutil.MustDeterministicUUID(501).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("250"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	// Now list validations without any filters
	listResp, listBody := testutil.ListValidations(t, "")
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err := json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify response contains items array
	assert.NotNil(t, result.TransactionValidations, "items should be present")

	// Verify items are ordered by createdAt DESC (most recent first)
	if len(result.TransactionValidations) >= 2 {
		for i := 1; i < len(result.TransactionValidations); i++ {
			// Parse timestamps to compare
			prevTime, err1 := time.Parse(time.RFC3339, result.TransactionValidations[i-1].CreatedAt)
			currTime, err2 := time.Parse(time.RFC3339, result.TransactionValidations[i].CreatedAt)
			if err1 == nil && err2 == nil {
				assert.True(t, prevTime.After(currTime) || prevTime.Equal(currTime),
					"Items should be ordered by createdAt DESC: %s should be >= %s",
					result.TransactionValidations[i-1].CreatedAt, result.TransactionValidations[i].CreatedAt)
			}
		}
	}
}

// Test 1.3.2: Filters by startDate and endDate
func TestValidation_1_3_2_FiltersByDateRange(t *testing.T) {
	// Create a validation with current timestamp
	accountID := testutil.MustDeterministicUUID(510).String()
	requestID := testutil.MustDeterministicUUID(511).String()
	now := time.Now().UTC()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("350"),
		Currency:             "BRL",
		TransactionTimestamp: now.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	// Query with date range covering now
	startDate := now.Add(-1 * time.Hour).Format(time.RFC3339)
	endDate := now.Add(1 * time.Hour).Format(time.RFC3339)

	listResp, listBody := testutil.ListValidations(t, fmt.Sprintf("start_date=%s&end_date=%s", startDate, endDate))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err := json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify all items have createdAt within the specified period
	startTime := now.Add(-1 * time.Hour)
	endTime := now.Add(1 * time.Hour)
	for _, item := range result.TransactionValidations {
		createdAt, err := time.Parse(time.RFC3339, item.CreatedAt)
		if err == nil {
			assert.True(t, !createdAt.Before(startTime) && !createdAt.After(endTime),
				"Item createdAt %s should be within range [%s, %s]",
				item.CreatedAt, startDate, endDate)
		}
	}
}

// Test 1.3.3: Filters by decision
func TestValidation_1_3_3_FiltersByDecision(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(520).String()

	// Create a DENY rule to ensure we get a DENY decision
	ruleName := "deny-filter-test-" + testutil.MustDeterministicUUID(1010).String()[:8]
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "transactionType == 'CARD' && amount > 1000", "DENY")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Create a validation that will be DENIED
	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(521).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("1500"), // Will trigger DENY rule
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	var createResult testutil.ValidationResponse
	err := json.Unmarshal(body, &createResult)
	require.NoError(t, err)
	require.Equal(t, "DENY", createResult.Decision, "Expected DENY decision")

	// Query with decision=DENY filter
	listResp, listBody := testutil.ListValidations(t, "decision=DENY")
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations?decision=DENY, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err = json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify all items have decision == "DENY"
	for _, item := range result.TransactionValidations {
		assert.Equal(t, "DENY", item.Decision,
			"All items should have decision == DENY when filtering by decision=DENY")
	}
}

// Test 1.3.4: Filters by accountId
func TestValidation_1_3_4_FiltersByAccountID(t *testing.T) {
	// Create a validation with a specific accountId
	accountID := testutil.MustDeterministicUUID(530).String()
	requestID := testutil.MustDeterministicUUID(531).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
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
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	// Query with accountId filter
	listResp, listBody := testutil.ListValidations(t, fmt.Sprintf("account_id=%s", accountID))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations?account_id=..., got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err := json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify all items have matching account.accountId
	for _, item := range result.TransactionValidations {
		assert.Equal(t, accountID, item.AccountID,
			"All items should have account_id == %s when filtering by account_id", accountID)
	}

	// Verify we got at least the validation we just created
	assert.GreaterOrEqual(t, len(result.TransactionValidations), 1,
		"Should have at least 1 validation for this account_id")
}

// Test 1.3.5: Filters by segmentId
func TestValidation_1_3_5_FiltersBySegmentID(t *testing.T) {
	// Create a validation with a specific segmentId
	accountID := testutil.MustDeterministicUUID(540).String()
	segmentID := testutil.MustDeterministicUUID(541).String()
	requestID := testutil.MustDeterministicUUID(542).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "WIRE",
		Amount:               decimal.RequireFromString("450"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		Segment: &testutil.SegmentContext{
			ID: segmentID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	// Query with segmentId filter
	listResp, listBody := testutil.ListValidations(t, fmt.Sprintf("segment_id=%s", segmentID))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations?segment_id=..., got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err := json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify all items belong to specified segment
	for _, item := range result.TransactionValidations {
		assert.Equal(t, segmentID, item.SegmentID,
			"All items should have segmentId == %s when filtering by segmentId", segmentID)
	}

	// Verify we got at least the validation we just created
	assert.GreaterOrEqual(t, len(result.TransactionValidations), 1,
		"Should have at least 1 validation for this segmentId")
}

// Test 1.3.6: Filters by portfolioId
func TestValidation_1_3_6_FiltersByPortfolioID(t *testing.T) {
	// Create a validation with a specific portfolioId
	accountID := testutil.MustDeterministicUUID(550).String()
	portfolioID := testutil.MustDeterministicUUID(551).String()
	requestID := testutil.MustDeterministicUUID(552).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("300"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		Portfolio: &testutil.PortfolioContext{
			ID: portfolioID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	// Query with portfolioId filter
	listResp, listBody := testutil.ListValidations(t, fmt.Sprintf("portfolio_id=%s", portfolioID))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations?portfolio_id=..., got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err := json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify all items belong to specified portfolio
	for _, item := range result.TransactionValidations {
		assert.Equal(t, portfolioID, item.PortfolioID,
			"All items should have portfolioId == %s when filtering by portfolioId", portfolioID)
	}

	// Verify we got at least the validation we just created
	assert.GreaterOrEqual(t, len(result.TransactionValidations), 1,
		"Should have at least 1 validation for this portfolioId")
}

// Test 1.3.7: Filters by transactionType
func TestValidation_1_3_7_FiltersByTransactionType(t *testing.T) {
	// Create a validation with transactionType CARD
	accountID := testutil.MustDeterministicUUID(560).String()
	requestID := testutil.MustDeterministicUUID(561).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("150"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	// Query with transactionType=CARD filter
	listResp, listBody := testutil.ListValidations(t, "transaction_type=CARD")
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations?transaction_type=CARD, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err := json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify all items have transactionType == "CARD"
	for _, item := range result.TransactionValidations {
		assert.Equal(t, "CARD", item.TransactionType,
			"All items should have transactionType == CARD when filtering by transaction_type=CARD")
	}

	// Verify we got at least the validation we just created
	assert.GreaterOrEqual(t, len(result.TransactionValidations), 1,
		"Should have at least 1 validation with transaction_type=CARD")
}

// Test 1.3.8: Filters by ruleId
func TestValidation_1_3_8_FiltersByRuleID(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(570).String()

	// Create a rule that will match
	ruleName := "filter-rule-test-" + testutil.MustDeterministicUUID(1011).String()[:8]
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "transactionType == 'WIRE' && amount > 500", "DENY")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Create a validation that will match this rule
	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(571).String(),
		TransactionType:      "WIRE",
		Amount:               decimal.RequireFromString("750"), // Will trigger the rule
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	var createResult testutil.ValidationResponse
	err := json.Unmarshal(body, &createResult)
	require.NoError(t, err)
	require.Contains(t, createResult.MatchedRuleIDs, ruleID, "Validation should match the created rule")

	// Query with matchedRuleId filter
	listResp, listBody := testutil.ListValidations(t, fmt.Sprintf("matched_rule_id=%s", ruleID))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations?matched_rule_id=..., got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err = json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify all items have matchedRuleIds containing the ruleId
	for _, item := range result.TransactionValidations {
		assert.Contains(t, item.MatchedRuleIDs, ruleID,
			"All items should have matchedRuleIds containing %s when filtering by ruleId", ruleID)
	}

	// Verify we got at least the validation we just created
	assert.GreaterOrEqual(t, len(result.TransactionValidations), 1,
		"Should have at least 1 validation matching this ruleId")
}

// Test 1.3.9: Filters by exceededLimitId
func TestValidation_1_3_9_FiltersByExceededLimitId(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(575).String()

	// Create a limit for the account with a LOW maxAmount to easily exceed
	maxAmount := "100" // Very low limit
	limitID := createTestLimitWithAccountScope(t, accountID, maxAmount)
	activateTestLimit(t, limitID)
	t.Cleanup(func() {
		cleanupTestLimit(t, limitID)
	})

	// Create a validation that EXCEEDS the limit
	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(576).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("200"), // Exceeds limit of 100
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	var createResult testutil.ValidationResponse
	err := json.Unmarshal(body, &createResult)
	require.NoError(t, err)

	// Verify the validation was denied due to limit exceeded
	require.Equal(t, "DENY", createResult.Decision, "Expected DENY when limit is exceeded")
	require.Equal(t, "limit_exceeded", createResult.Reason, "Expected reason to be limit_exceeded")

	// Verify limitUsageDetails contains exceeded entry
	hasExceeded := false
	for _, detail := range createResult.LimitUsageDetails {
		if detail.LimitID == limitID && detail.Exceeded {
			hasExceeded = true
			break
		}
	}
	require.True(t, hasExceeded, "limitUsageDetails should show exceeded == true for the limit")

	// List validations with filter exceededLimitId
	listResp, listBody := testutil.ListValidations(t, fmt.Sprintf("exceeded_limit_id=%s", limitID))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations?exceeded_limit_id=..., got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err = json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify all items have exceededLimitIds containing the limitId
	for _, item := range result.TransactionValidations {
		assert.Contains(t, item.ExceededLimitIDs, limitID,
			"All items should have exceededLimitIds containing %s when filtering by exceededLimitId", limitID)
	}

	// Verify we got at least the validation we just created
	assert.GreaterOrEqual(t, len(result.TransactionValidations), 1,
		"Should have at least 1 validation with exceededLimitId=%s", limitID)
}

// Test 1.3.10: Pagination works correctly
func TestValidation_1_3_10_PaginationWorks(t *testing.T) {
	// Create multiple validations to ensure we have enough data
	baseAccountID := testutil.MustDeterministicUUID(580).String()

	// Create 15 validations
	for i := 0; i < 15; i++ {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(int64(581 + i)).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString(strconv.Itoa(100 + i*10)),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: baseAccountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))
	}

	// Get first page with limit=10
	listResp1, listBody1 := testutil.ListValidations(t, fmt.Sprintf("account_id=%s&limit=10", baseAccountID))
	defer listResp1.Body.Close()

	require.Equal(t, http.StatusOK, listResp1.StatusCode,
		"Expected 200 OK from first page, got: %s", string(listBody1))

	var result1 testutil.ListValidationsResponse
	err := json.Unmarshal(listBody1, &result1)
	require.NoError(t, err)

	// Verify first page returns max 10 items
	assert.LessOrEqual(t, len(result1.TransactionValidations), 10,
		"First page should return at most 10 items")

	// If there's a next page, get it
	if result1.NextCursor != "" {
		listResp2, listBody2 := testutil.ListValidations(t,
			fmt.Sprintf("account_id=%s&limit=10&cursor=%s", baseAccountID, result1.NextCursor))
		defer listResp2.Body.Close()

		require.Equal(t, http.StatusOK, listResp2.StatusCode,
			"Expected 200 OK from second page, got: %s", string(listBody2))

		var result2 testutil.ListValidationsResponse
		err = json.Unmarshal(listBody2, &result2)
		require.NoError(t, err)

		// Verify second page returns different items
		assert.NotEmpty(t, result2.TransactionValidations, "Second page should have items")

		// Verify no duplicates between pages
		firstPageIDs := make(map[string]bool)
		for _, item := range result1.TransactionValidations {
			firstPageIDs[item.ID] = true
		}

		for _, item := range result2.TransactionValidations {
			assert.False(t, firstPageIDs[item.ID],
				"Item %s should not appear in both pages", item.ID)
		}
	}
}

// Test 1.3.11: Sorting works
func TestValidation_1_3_11_SortingWorks(t *testing.T) {
	// Create validations with different processing times
	accountID := testutil.MustDeterministicUUID(600).String()

	for i := 0; i < 5; i++ {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(int64(601 + i)).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString(strconv.Itoa(100 + i*50)),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))
		resp.Body.Close()
	}

	// Query with sort_by=processing_time_ms&sort_order=DESC
	listResp, listBody := testutil.ListValidations(t,
		fmt.Sprintf("account_id=%s&sort_by=processing_time_ms&sort_order=DESC", accountID))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err := json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify items are ordered by processingTimeMs descending
	if len(result.TransactionValidations) >= 2 {
		for i := 1; i < len(result.TransactionValidations); i++ {
			assert.GreaterOrEqual(t, result.TransactionValidations[i-1].ProcessingTimeMs, result.TransactionValidations[i].ProcessingTimeMs,
				"Items should be ordered by processingTimeMs DESC: %d should be >= %d",
				result.TransactionValidations[i-1].ProcessingTimeMs, result.TransactionValidations[i].ProcessingTimeMs)
		}
	}
}

// Test 1.3.12: Combined filters
func TestValidation_1_3_12_CombinedFilters(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(620).String()
	now := time.Now().UTC()

	// Create a DENY rule for CARD transactions with high amount
	ruleName := "combined-filter-deny-" + testutil.MustDeterministicUUID(1012).String()[:8]
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "transactionType == 'CARD' && amount > 800", "DENY")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Create a validation that will match: DENY decision + CARD type + within date range
	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(621).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("1000"), // Will trigger DENY rule
		Currency:             "BRL",
		TransactionTimestamp: now.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	var createResult testutil.ValidationResponse
	err := json.Unmarshal(body, &createResult)
	require.NoError(t, err)
	require.Equal(t, "DENY", createResult.Decision, "Expected DENY decision")

	// Query with combined filters
	startDate := now.Add(-1 * time.Hour).Format(time.RFC3339)

	listResp, listBody := testutil.ListValidations(t,
		fmt.Sprintf("decision=DENY&transaction_type=CARD&start_date=%s&account_id=%s", startDate, accountID))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations with combined filters, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err = json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify all items satisfy ALL filters
	for _, item := range result.TransactionValidations {
		assert.Equal(t, "DENY", item.Decision,
			"All items should have decision == DENY")
		assert.Equal(t, "CARD", item.TransactionType,
			"All items should have transactionType == CARD")
		assert.Equal(t, accountID, item.AccountID,
			"All items should have matching account_id")

		// Verify date is within range
		createdAt, err := time.Parse(time.RFC3339, item.CreatedAt)
		if err == nil {
			startTime := now.Add(-1 * time.Hour)
			assert.True(t, !createdAt.Before(startTime),
				"Item createdAt %s should be after startDate %s", item.CreatedAt, startDate)
		}
	}

	// Verify we got at least the validation we just created
	assert.GreaterOrEqual(t, len(result.TransactionValidations), 1,
		"Should have at least 1 validation matching all filters")
}

// Test 1.3.16: Rejects invalid segmentId UUID in filter
func TestValidation_1_3_16_RejectsInvalidSegmentIdFilter(t *testing.T) {
	tests := []struct {
		name      string
		segmentID string
	}{
		{
			name:      "invalid UUID format",
			segmentID: "invalid-uuid",
		},
		{
			name:      "numeric string",
			segmentID: "12345",
		},
		{
			name:      "malformed UUID",
			segmentID: "not-a-valid-segment-uuid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			listResp, _ := testutil.ListValidations(t, fmt.Sprintf("segment_id=%s", tc.segmentID))
			defer listResp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
				"Invalid segmentId '%s' in filter should return 400 Bad Request", tc.segmentID)
		})
	}
}

// Test 1.3.17: Rejects invalid portfolioId UUID in filter
func TestValidation_1_3_17_RejectsInvalidPortfolioIdFilter(t *testing.T) {
	tests := []struct {
		name        string
		portfolioID string
	}{
		{
			name:        "invalid UUID format",
			portfolioID: "invalid-uuid",
		},
		{
			name:        "numeric string",
			portfolioID: "67890",
		},
		{
			name:        "malformed UUID",
			portfolioID: "bad-portfolio-filter-id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			listResp, _ := testutil.ListValidations(t, fmt.Sprintf("portfolio_id=%s", tc.portfolioID))
			defer listResp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
				"Invalid portfolioId '%s' in filter should return 400 Bad Request", tc.portfolioID)
		})
	}
}

// Test 1.3.18: Rejects invalid accountId UUID in filter
func TestValidation_1_3_18_RejectsInvalidAccountIdFilter(t *testing.T) {
	tests := []struct {
		name      string
		accountID string
	}{
		{
			name:      "invalid UUID format",
			accountID: "invalid-uuid",
		},
		{
			name:      "numeric string",
			accountID: "11111",
		},
		{
			name:      "malformed UUID",
			accountID: "bad-account-id-format",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			listResp, _ := testutil.ListValidations(t, fmt.Sprintf("account_id=%s", tc.accountID))
			defer listResp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
				"Invalid accountId '%s' in filter should return 400 Bad Request", tc.accountID)
		})
	}
}

// Test 1.3.19: Rejects invalid ruleId UUID in filter
func TestValidation_1_3_19_RejectsInvalidRuleIdFilter(t *testing.T) {
	tests := []struct {
		name   string
		ruleID string
	}{
		{
			name:   "invalid UUID format",
			ruleID: "invalid-uuid",
		},
		{
			name:   "numeric string",
			ruleID: "99999",
		},
		{
			name:   "malformed UUID",
			ruleID: "not-valid-rule-id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			listResp, _ := testutil.ListValidations(t, fmt.Sprintf("matched_rule_id=%s", tc.ruleID))
			defer listResp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
				"Invalid ruleId '%s' in filter should return 400 Bad Request", tc.ruleID)
		})
	}
}

// Test 1.3.20: Rejects invalid transactionType filter
func TestValidation_1_3_20_RejectsInvalidTransactionTypeFilter(t *testing.T) {
	// Query with invalid transactionType
	listResp, _ := testutil.ListValidations(t, "transaction_type=INVALID_TYPE")
	defer listResp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
		"Invalid transaction_type filter should return 400 Bad Request")
}

// Test 1.3.21: Rejects invalid decision filter
func TestValidation_1_3_21_RejectsInvalidDecisionFilter(t *testing.T) {
	// Query with invalid decision
	listResp, _ := testutil.ListValidations(t, "decision=INVALID")
	defer listResp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
		"Invalid decision filter should return 400 Bad Request")
}

// Test 1.3.22: Rejects invalid sortBy
func TestValidation_1_3_22_RejectsInvalidSortBy(t *testing.T) {
	// Query with invalid sortBy field
	listResp, _ := testutil.ListValidations(t, "sort_by=invalid_field")
	defer listResp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
		"Invalid sort_by field should return 400 Bad Request")
}

// Test 1.3.23: Rejects invalid sortOrder
func TestValidation_1_3_23_RejectsInvalidSortOrder(t *testing.T) {
	// Query with invalid sortOrder
	listResp, _ := testutil.ListValidations(t, "sort_order=INVALID")
	defer listResp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
		"Invalid sort_order should return 400 Bad Request")
}

// Test 1.3.24: Rejects limit > 1000
func TestValidation_1_3_24_RejectsLimitOver1000(t *testing.T) {
	// Query with limit > 1000
	listResp, _ := testutil.ListValidations(t, "limit=1001")
	defer listResp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
		"Limit > 1000 should return 400 Bad Request")
}

// Test 1.3.25: Rejects limit < 1
func TestValidation_1_3_25_RejectsLimitUnder1(t *testing.T) {
	// Test with limit=0
	listResp0, _ := testutil.ListValidations(t, "limit=0")
	defer listResp0.Body.Close()

	assert.Equal(t, http.StatusBadRequest, listResp0.StatusCode,
		"Limit 0 should return 400 Bad Request")

	// Test with limit=-1
	listRespNeg, _ := testutil.ListValidations(t, "limit=-1")
	defer listRespNeg.Body.Close()

	assert.Equal(t, http.StatusBadRequest, listRespNeg.StatusCode,
		"Negative limit should return 400 Bad Request")
}

// Test 1.3.26: Rejects startDate after endDate
func TestValidation_1_3_26_RejectsStartDateAfterEndDate(t *testing.T) {
	// Query with startDate after endDate
	listResp, _ := testutil.ListValidations(t, "start_date=2025-01-20T00:00:00Z&end_date=2025-01-10T00:00:00Z")
	defer listResp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
		"startDate after endDate should return 400 Bad Request")
}

// Test 1.3.28: Requires authentication
func TestValidation_1_3_28_RequiresAuthentication(t *testing.T) {
	// Query without authentication
	listResp, _ := testutil.ListValidationsWithoutAuth(t, "")
	defer listResp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, listResp.StatusCode,
		"GET /v1/validations without X-API-Key should return 401 Unauthorized")
}

// Test 1.3.13: Sorting by processingTimeMs ASC
func TestValidation_1_3_13_SortingByProcessingTimeAsc(t *testing.T) {
	// Create validations with different processing times
	accountID := testutil.MustDeterministicUUID(700).String()

	for i := 0; i < 5; i++ {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(int64(701 + i)).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString(strconv.Itoa(100 + i*50)),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))
		resp.Body.Close()
	}

	// Query with sort_by=processing_time_ms&sort_order=ASC
	listResp, listBody := testutil.ListValidations(t,
		fmt.Sprintf("account_id=%s&sort_by=processing_time_ms&sort_order=ASC", accountID))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err := json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify items are ordered by processingTimeMs ascending
	if len(result.TransactionValidations) >= 2 {
		for i := 1; i < len(result.TransactionValidations); i++ {
			assert.LessOrEqual(t, result.TransactionValidations[i-1].ProcessingTimeMs, result.TransactionValidations[i].ProcessingTimeMs,
				"Items should be ordered by processingTimeMs ASC: %d should be <= %d",
				result.TransactionValidations[i-1].ProcessingTimeMs, result.TransactionValidations[i].ProcessingTimeMs)
		}
	}
}

// Test 1.3.27: Combined filters with new parameters
func TestValidation_1_3_27_CombinedFiltersWithNewParams(t *testing.T) {
	// Create unique identifiers for this test
	accountID := testutil.MustDeterministicUUID(710).String()
	segmentID := testutil.MustDeterministicUUID(711).String()
	portfolioID := testutil.MustDeterministicUUID(712).String()
	now := time.Now().UTC()

	// Create multiple validations with different characteristics
	// Validation 1: ALLOW, CARD, with segment and portfolio
	req1 := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(713).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("150"),
		Currency:             "BRL",
		TransactionTimestamp: now.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		Segment: &testutil.SegmentContext{
			ID: segmentID,
		},
		Portfolio: &testutil.PortfolioContext{
			ID: portfolioID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, req1)
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "Expected 201 Created from POST, got: %s", string(body1))
	resp1.Body.Close()

	var createResult1 testutil.ValidationResponse
	err := json.Unmarshal(body1, &createResult1)
	require.NoError(t, err)

	// Validation 2: Another with same characteristics to ensure filter works
	req2 := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(714).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("200"),
		Currency:             "BRL",
		TransactionTimestamp: now.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		Segment: &testutil.SegmentContext{
			ID: segmentID,
		},
		Portfolio: &testutil.PortfolioContext{
			ID: portfolioID,
		},
	}

	resp2, body2 := testutil.CreateValidation(t, req2)
	require.Equal(t, http.StatusCreated, resp2.StatusCode, "Expected 201 Created from POST, got: %s", string(body2))
	resp2.Body.Close()

	// Query with combined filters: decision + transactionType + startDate + endDate + accountId + limit
	startDate := now.Add(-1 * time.Hour).Format(time.RFC3339)
	endDate := now.Add(1 * time.Hour).Format(time.RFC3339)
	decision := createResult1.Decision // Use the actual decision from first validation

	queryParams := fmt.Sprintf(
		"decision=%s&transaction_type=CARD&start_date=%s&end_date=%s&account_id=%s&limit=10",
		decision, startDate, endDate, accountID,
	)

	listResp, listBody := testutil.ListValidations(t, queryParams)
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations with combined filters, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err = json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify all returned validations satisfy ALL filters
	for _, item := range result.TransactionValidations {
		assert.Equal(t, decision, item.Decision,
			"All items should have decision == %s", decision)
		assert.Equal(t, "CARD", item.TransactionType,
			"All items should have transactionType == CARD")
		assert.Equal(t, accountID, item.AccountID,
			"All items should have account_id == %s", accountID)

		// Verify date is within range
		createdAt, parseErr := time.Parse(time.RFC3339, item.CreatedAt)
		if parseErr == nil {
			startTime := now.Add(-1 * time.Hour)
			endTime := now.Add(1 * time.Hour)
			assert.True(t, !createdAt.Before(startTime),
				"Item createdAt %s should be after startDate %s", item.CreatedAt, startDate)
			assert.True(t, !createdAt.After(endTime),
				"Item createdAt %s should be before endDate %s", item.CreatedAt, endDate)
		}
	}

	// Verify we got at least our 2 created validations
	assert.GreaterOrEqual(t, len(result.TransactionValidations), 2,
		"Should have at least 2 validations matching all combined filters")

	// Verify the limit was respected
	assert.LessOrEqual(t, len(result.TransactionValidations), 10,
		"Result should respect the limit=10 parameter")
}

// Test 1.2.10: GET validation - validationId echoed correctly
func TestValidation_1_2_10_ValidationIdEchoedCorrectly(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(480).String()
	requestID := testutil.MustDeterministicUUID(481).String()

	// Create a validation
	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("500"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	var createResult testutil.ValidationResponse
	err := json.Unmarshal(body, &createResult)
	require.NoError(t, err)

	validationID := createResult.ValidationID

	// GET the validation and verify ID matches
	getResp, getBody := testutil.GetValidation(t, validationID)
	defer getResp.Body.Close()

	require.Equal(t, http.StatusOK, getResp.StatusCode,
		"Expected 200 OK from GET, got: %s", string(getBody))

	var result testutil.ValidationDetailResponse
	err = json.Unmarshal(getBody, &result)
	require.NoError(t, err)

	// Verify validationId in response exactly matches the path parameter
	assert.Equal(t, validationID, result.ID,
		"validationId in response body should match the path parameter")

	// Verify validationId is valid UUID format (36 characters, 8-4-4-4-12 hyphen format)
	_, uuidErr := uuid.Parse(result.ID)
	assert.NoError(t, uuidErr, "validationId should be valid UUID format")
	assert.Len(t, result.ID, 36, "validationId should be 36 characters (UUID format)")
}

// Test 1.2.11: GET validation - processingTimeMs is non-negative integer
func TestValidation_1_2_11_ProcessingTimeMsNonNegative(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(482).String()
	requestID := testutil.MustDeterministicUUID(483).String()

	// Create a validation
	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("350"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	var createResult testutil.ValidationResponse
	err := json.Unmarshal(body, &createResult)
	require.NoError(t, err)

	validationID := createResult.ValidationID

	// GET the validation
	getResp, getBody := testutil.GetValidation(t, validationID)
	defer getResp.Body.Close()

	require.Equal(t, http.StatusOK, getResp.StatusCode,
		"Expected 200 OK from GET, got: %s", string(getBody))

	var result testutil.ValidationDetailResponse
	err = json.Unmarshal(getBody, &result)
	require.NoError(t, err)

	// Verify processingTimeMs is present and >= 0
	assert.GreaterOrEqual(t, result.ProcessingTimeMs, float64(0),
		"processingTimeMs should be >= 0")

	// Also verify it's a reasonable value (less than 1 hour in ms)
	assert.Less(t, result.ProcessingTimeMs, float64(3600000),
		"processingTimeMs should be a reasonable value (< 1 hour)")
}

// Test 1.2.12: GET validation - returns correct decision enum values
func TestValidation_1_2_12_ReturnsCorrectDecisionEnumValues(t *testing.T) {
	t.Run("ALLOW decision", func(t *testing.T) {
		requestID := testutil.MustDeterministicUUID(485).String()

		// Setup: Create an ALLOW rule for a unique account ID
		// This ensures we get ALLOW decision regardless of other rules in DB
		uniqueAccountID := testutil.MustDeterministicUUID(7111).String()
		ruleName := "allow-decision-test-" + testutil.MustDeterministicUUID(1012).String()[:8]
		ruleID := testutil.CreateTestRuleWithExpression(t, ruleName,
			fmt.Sprintf("account.accountId == '%s' && amount < 50", uniqueAccountID), "ALLOW")
		testutil.ActivateRule(t, ruleID)

		t.Cleanup(func() {
			testutil.CleanupRule(t, ruleID)
		})

		// Create validation that triggers the ALLOW rule
		req := &testutil.ValidationRequest{
			RequestID:            requestID,
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("10"),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: uniqueAccountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResult testutil.ValidationResponse
		err := json.Unmarshal(body, &createResult)
		require.NoError(t, err)

		require.Equal(t, "ALLOW", createResult.Decision, "Should return ALLOW based on our test rule")

		// GET and verify
		getResp, getBody := testutil.GetValidation(t, createResult.ValidationID)
		defer getResp.Body.Close()

		require.Equal(t, http.StatusOK, getResp.StatusCode)

		var result testutil.ValidationDetailResponse
		err = json.Unmarshal(getBody, &result)
		require.NoError(t, err)

		assert.Equal(t, "ALLOW", result.Decision, "decision should be exactly 'ALLOW' (string, exact match)")
		assert.NotEmpty(t, result.Decision, "decision field should never be null/empty")
	})

	t.Run("DENY decision", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(486).String()

		// Create DENY rule
		ruleName := "deny-decision-test-" + testutil.MustDeterministicUUID(1013).String()[:8]
		ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "transactionType == 'CARD' && amount > 2000", "DENY")
		testutil.ActivateRule(t, ruleID)

		t.Cleanup(func() {
			testutil.CleanupRule(t, ruleID)
		})

		// Create validation that triggers DENY
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(487).String(),
			TransactionType:      "CARD",
			Amount:               decimal.RequireFromString("2500"),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResult testutil.ValidationResponse
		err := json.Unmarshal(body, &createResult)
		require.NoError(t, err)
		require.Equal(t, "DENY", createResult.Decision)

		// GET and verify
		getResp, getBody := testutil.GetValidation(t, createResult.ValidationID)
		defer getResp.Body.Close()

		require.Equal(t, http.StatusOK, getResp.StatusCode)

		var result testutil.ValidationDetailResponse
		err = json.Unmarshal(getBody, &result)
		require.NoError(t, err)

		assert.Equal(t, "DENY", result.Decision, "decision should be exactly 'DENY' (string, exact match)")
		assert.NotEmpty(t, result.Decision, "decision field should never be null/empty")
	})

	t.Run("REVIEW decision", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(488).String()

		// Create REVIEW rule
		ruleName := "review-decision-test-" + testutil.MustDeterministicUUID(1014).String()[:8]
		ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "transactionType == 'WIRE' && amount > 1500", "REVIEW")
		testutil.ActivateRule(t, ruleID)

		t.Cleanup(func() {
			testutil.CleanupRule(t, ruleID)
		})

		// Create validation that triggers REVIEW
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(489).String(),
			TransactionType:      "WIRE",
			Amount:               decimal.RequireFromString("1800"),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResult testutil.ValidationResponse
		err := json.Unmarshal(body, &createResult)
		require.NoError(t, err)
		require.Equal(t, "REVIEW", createResult.Decision)

		// GET and verify
		getResp, getBody := testutil.GetValidation(t, createResult.ValidationID)
		defer getResp.Body.Close()

		require.Equal(t, http.StatusOK, getResp.StatusCode)

		var result testutil.ValidationDetailResponse
		err = json.Unmarshal(getBody, &result)
		require.NoError(t, err)

		assert.Equal(t, "REVIEW", result.Decision, "decision should be exactly 'REVIEW' (string, exact match)")
		assert.NotEmpty(t, result.Decision, "decision field should never be null/empty")
	})
}

// Test 1.3.14: Combined filters
func TestValidation_1_3_14_CombinedFiltersAdvanced(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(730).String()

	// Create a DENY rule for CARD transactions with high amount
	ruleName := "combined-adv-filter-deny-" + testutil.MustDeterministicUUID(1015).String()[:8]
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "transactionType == 'CARD' && amount > 900", "DENY")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Create validation that matches: accountId=X, decision=DENY, transactionType=CARD
	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(731).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("1200"), // Will trigger DENY rule
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	var createResult testutil.ValidationResponse
	err := json.Unmarshal(body, &createResult)
	require.NoError(t, err)
	require.Equal(t, "DENY", createResult.Decision, "Expected DENY decision")

	// Query with combined filters: accountId=X&decision=DENY&transaction_type=CARD
	listResp, listBody := testutil.ListValidations(t,
		fmt.Sprintf("account_id=%s&decision=DENY&transaction_type=CARD", accountID))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations with combined filters, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err = json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify all items satisfy ALL filters
	for _, item := range result.TransactionValidations {
		assert.Equal(t, accountID, item.AccountID, "All items should have matching account_id")
		assert.Equal(t, "DENY", item.Decision, "All items should have decision == DENY")
		assert.Equal(t, "CARD", item.TransactionType, "All items should have transactionType == CARD")
	}

	// Verify we got at least the validation we just created
	assert.GreaterOrEqual(t, len(result.TransactionValidations), 1,
		"Should have at least 1 validation matching all filters")
}

// Test 1.3.15: Returns 504 on query timeout
// Uses fault injection middleware to simulate query timeout scenario.
// Note: GET /v1/validations (list) returns TRC-0252, POST /v1/validations returns TRC-0229.
func TestValidation_1_3_15_Returns504OnQueryTimeout(t *testing.T) {
	// Use fault injection to simulate query timeout
	resp, body := testutil.ListValidationsWithFaultInjection(t, "", testutil.FaultTimeout)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode,
		"Query timeout should return 504 Gateway Timeout, got: %s", string(body))

	// Verify error response structure
	errorResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "TRC-0252", errorResp.Code, "Error code should be TRC-0252 (ListValidationsTimeout)")
	assert.Equal(t, "Gateway Timeout", errorResp.Title, "Error title should be Gateway Timeout")
	assert.Equal(t, "query timeout exceeded", errorResp.Message, "Error message should indicate query timeout")
}

// Test 1.3.30: Rejects invalid exceededLimitId UUID
func TestValidation_1_3_30_RejectsInvalidExceededLimitIdFilter(t *testing.T) {
	tests := []struct {
		name    string
		limitID string
	}{
		{
			name:    "invalid UUID format",
			limitID: "invalid-uuid",
		},
		{
			name:    "numeric string",
			limitID: "12345",
		},
		{
			name:    "malformed UUID",
			limitID: "not-a-valid-limit-uuid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			listResp, _ := testutil.ListValidations(t, fmt.Sprintf("exceeded_limit_id=%s", tc.limitID))
			defer listResp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
				"Invalid exceededLimitId '%s' in filter should return 400 Bad Request", tc.limitID)
		})
	}
}

// Test 1.3.31: Empty result set returns valid response structure
func TestValidation_1_3_31_EmptyResultSetValidStructure(t *testing.T) {
	// Use a UUID that won't have any validations
	nonExistentAccountID := testutil.MustDeterministicUUID(9999).String()

	listResp, listBody := testutil.ListValidations(t, fmt.Sprintf("account_id=%s", nonExistentAccountID))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK even for empty result set, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err := json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify valid response structure
	assert.NotNil(t, result.TransactionValidations, "transactionValidations should not be nil")
	assert.Empty(t, result.TransactionValidations, "transactionValidations should be empty array []")
	assert.False(t, result.HasMore, "hasMore should be false for empty result")
}

// Test 1.3.32: Default pagination limit is 100
func TestValidation_1_3_32_DefaultPaginationLimit(t *testing.T) {
	// Create a unique account for this test
	accountID := testutil.MustDeterministicUUID(740).String()

	// Create 105 validations to exceed default limit
	for i := 0; i < 105; i++ {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(int64(741 + i)).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString(strconv.Itoa(10 + i)),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))
		resp.Body.Close()
	}

	// List validations WITHOUT specifying limit
	listResp, listBody := testutil.ListValidations(t, fmt.Sprintf("account_id=%s", accountID))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err := json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify default limit is 100
	assert.LessOrEqual(t, len(result.TransactionValidations), 100,
		"Default pagination limit should be 100, got %d items", len(result.TransactionValidations))

	// With 105 validations and default limit of 100, hasMore should be true
	assert.True(t, result.HasMore, "hasMore should be true when there are more than 100 validations")
}

// Test 1.3.33: Sorting by createdAt ASC
func TestValidation_1_3_33_SortingByCreatedAtAsc(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(850).String()

	// Create multiple validations with slight time gaps
	for i := 0; i < 5; i++ {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(int64(851 + i)).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString(strconv.Itoa(100 + i*10)),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))
		resp.Body.Close()
		time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
	}

	// Query with sort_by=created_at&sort_order=ASC
	listResp, listBody := testutil.ListValidations(t,
		fmt.Sprintf("account_id=%s&sort_by=created_at&sort_order=ASC", accountID))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err := json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify items are ordered by createdAt ascending
	if len(result.TransactionValidations) >= 2 {
		for i := 1; i < len(result.TransactionValidations); i++ {
			prevTime, err1 := time.Parse(time.RFC3339, result.TransactionValidations[i-1].CreatedAt)
			currTime, err2 := time.Parse(time.RFC3339, result.TransactionValidations[i].CreatedAt)
			if err1 == nil && err2 == nil {
				assert.True(t, prevTime.Before(currTime) || prevTime.Equal(currTime),
					"Items should be ordered by createdAt ASC: %s should be <= %s",
					result.TransactionValidations[i-1].CreatedAt, result.TransactionValidations[i].CreatedAt)
			}
		}
	}
}

// Test 1.3.34: Filters by multiple parameters simultaneously
func TestValidation_1_3_34_MultipleParametersSimultaneously(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(860).String()
	now := time.Now().UTC()

	// Create validation with specific characteristics
	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(861).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("250"),
		Currency:             "BRL",
		TransactionTimestamp: now.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	var createResult testutil.ValidationResponse
	err := json.Unmarshal(body, &createResult)
	require.NoError(t, err)

	// Query with multiple parameters: startDate, endDate, decision, accountId
	startDate := now.Add(-1 * time.Hour).Format(time.RFC3339)
	endDate := now.Add(1 * time.Hour).Format(time.RFC3339)

	queryParams := fmt.Sprintf(
		"start_date=%s&end_date=%s&decision=%s&account_id=%s",
		startDate, endDate, createResult.Decision, accountID,
	)

	listResp, listBody := testutil.ListValidations(t, queryParams)
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err = json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify all items satisfy ALL filters
	for _, item := range result.TransactionValidations {
		assert.Equal(t, accountID, item.AccountID, "All items should have matching account_id")
		assert.Equal(t, createResult.Decision, item.Decision, "All items should have matching decision")

		// Verify date is within range
		createdAt, parseErr := time.Parse(time.RFC3339, item.CreatedAt)
		if parseErr == nil {
			startTime := now.Add(-1 * time.Hour)
			endTime := now.Add(1 * time.Hour)
			assert.True(t, !createdAt.Before(startTime) && createdAt.Before(endTime),
				"Item createdAt %s should be within date range", item.CreatedAt)
		}
	}

	// Verify we got at least our created validation
	assert.GreaterOrEqual(t, len(result.TransactionValidations), 1,
		"Should have at least 1 validation matching all filters")
}

// Test 1.3.35: Rejects negative limit value
func TestValidation_1_3_35_RejectsNegativeLimit(t *testing.T) {
	listResp, _ := testutil.ListValidations(t, "limit=-1")
	defer listResp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
		"Negative limit should return 400 Bad Request")
}

// Test 1.3.36: Rejects non-integer limit value
func TestValidation_1_3_36_RejectsNonIntegerLimit(t *testing.T) {
	listResp, _ := testutil.ListValidations(t, "limit=abc")
	defer listResp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
		"Non-integer limit should return 400 Bad Request")
}

// Test 1.3.37: Rejects invalid cursor format
func TestValidation_1_3_37_RejectsInvalidCursorFormat(t *testing.T) {
	listResp, _ := testutil.ListValidations(t, "cursor=invalid-base64")
	defer listResp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
		"Invalid cursor format should return 400 Bad Request")
}

// Test 1.3.38: Rejects invalid date format for startDate
func TestValidation_1_3_38_RejectsInvalidStartDateFormat(t *testing.T) {
	listResp, _ := testutil.ListValidations(t, "start_date=not-a-date")
	defer listResp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
		"Invalid startDate format should return 400 Bad Request")
}

// Test 1.3.39: Rejects invalid date format for endDate
func TestValidation_1_3_39_RejectsInvalidEndDateFormat(t *testing.T) {
	listResp, _ := testutil.ListValidations(t, "end_date=not-a-date")
	defer listResp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, listResp.StatusCode,
		"Invalid endDate format should return 400 Bad Request")
}

// Test 1.3.40: Cursor pagination maintains consistency
func TestValidation_1_3_40_CursorPaginationConsistency(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(870).String()

	// Create 25 validations
	createdIDs := make([]string, 0, 25)
	for i := 0; i < 25; i++ {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(int64(871 + i)).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString(strconv.Itoa(50 + i)),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

		var createResult testutil.ValidationResponse
		err := json.Unmarshal(body, &createResult)
		require.NoError(t, err)
		createdIDs = append(createdIDs, createResult.ValidationID)
		resp.Body.Close()
	}

	// Paginate through all with limit=10
	allRetrievedIDs := make(map[string]bool)
	cursor := ""
	pageCount := 0
	maxPages := 5 // Safety limit

	for pageCount < maxPages {
		queryParams := fmt.Sprintf("account_id=%s&limit=10", accountID)
		if cursor != "" {
			queryParams += "&cursor=" + cursor
		}

		listResp, listBody := testutil.ListValidations(t, queryParams)
		require.Equal(t, http.StatusOK, listResp.StatusCode,
			"Expected 200 OK from page %d, got: %s", pageCount+1, string(listBody))

		var result testutil.ListValidationsResponse
		err := json.Unmarshal(listBody, &result)
		require.NoError(t, err)
		listResp.Body.Close()

		// Collect all IDs from this page
		for _, item := range result.TransactionValidations {
			// Check for duplicates
			assert.False(t, allRetrievedIDs[item.ID],
				"Duplicate validation ID found across pages: %s", item.ID)
			allRetrievedIDs[item.ID] = true
		}

		pageCount++

		if !result.HasMore || result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}

	// Verify we got all 25 validations we created (no missing)
	for _, id := range createdIDs {
		assert.True(t, allRetrievedIDs[id],
			"Validation %s was created but not retrieved during pagination", id)
	}
}

// Test 1.3.41: Filter by matchedRuleId returns only matching validations
func TestValidation_1_3_41_FilterByMatchedRuleId(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(896).String()

	// Create a rule
	ruleName := "matched-rule-filter-test-" + testutil.MustDeterministicUUID(1016).String()[:8]
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "transactionType == 'CARD' && amount > 700", "DENY")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Create validation that matches the rule
	reqMatch := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(897).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("800"), // Will trigger the rule
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, reqMatch)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode)

	var matchResult testutil.ValidationResponse
	err := json.Unmarshal(body1, &matchResult)
	require.NoError(t, err)
	require.Contains(t, matchResult.MatchedRuleIDs, ruleID, "Validation should match the rule")

	// Create validation that does NOT match the rule
	reqNoMatch := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(898).String(),
		TransactionType:      "PIX", // Different type, won't match
		Amount:               decimal.RequireFromString("800"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp2, body2 := testutil.CreateValidation(t, reqNoMatch)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusCreated, resp2.StatusCode)

	var noMatchResult testutil.ValidationResponse
	err = json.Unmarshal(body2, &noMatchResult)
	require.NoError(t, err)

	// Filter by ruleId
	listResp, listBody := testutil.ListValidations(t,
		fmt.Sprintf("account_id=%s&matched_rule_id=%s", accountID, ruleID))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err = json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify only matching validation is returned
	for _, item := range result.TransactionValidations {
		assert.Contains(t, item.MatchedRuleIDs, ruleID,
			"All items should have matchedRuleIds containing %s", ruleID)
	}

	// Verify the matched validation is present and the non-matched is not
	foundMatch := false
	foundNoMatch := false
	for _, item := range result.TransactionValidations {
		if item.ID == matchResult.ValidationID {
			foundMatch = true
		}
		if item.ID == noMatchResult.ValidationID {
			foundNoMatch = true
		}
	}

	assert.True(t, foundMatch, "Matched validation should be in results")
	assert.False(t, foundNoMatch, "Non-matched validation should NOT be in results")
}

// Test 1.3.42: All sortBy and sortOrder combinations work correctly
func TestValidation_1_3_42_AllSortBySortOrderCombinations(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(900).String()

	// Create multiple validations
	for i := 0; i < 5; i++ {
		req := &testutil.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(int64(901 + i)).String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString(strconv.Itoa(100 + i*50)),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))
		resp.Body.Close()
		time.Sleep(10 * time.Millisecond)
	}

	testCases := []struct {
		name      string
		sortBy    string
		sortOrder string
	}{
		{"created_at ASC", "created_at", "ASC"},
		{"created_at DESC", "created_at", "DESC"},
		{"processing_time_ms ASC", "processing_time_ms", "ASC"},
		{"processing_time_ms DESC", "processing_time_ms", "DESC"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			listResp, listBody := testutil.ListValidations(t,
				fmt.Sprintf("account_id=%s&sort_by=%s&sort_order=%s", accountID, tc.sortBy, tc.sortOrder))
			defer listResp.Body.Close()

			require.Equal(t, http.StatusOK, listResp.StatusCode,
				"Expected 200 OK for sortBy=%s&sort_order=%s, got: %s", tc.sortBy, tc.sortOrder, string(listBody))

			var result testutil.ListValidationsResponse
			err := json.Unmarshal(listBody, &result)
			require.NoError(t, err)

			if len(result.TransactionValidations) >= 2 {
				for i := 1; i < len(result.TransactionValidations); i++ {
					prev := result.TransactionValidations[i-1]
					curr := result.TransactionValidations[i]

					if tc.sortBy == "created_at" {
						prevTime, _ := time.Parse(time.RFC3339, prev.CreatedAt)
						currTime, _ := time.Parse(time.RFC3339, curr.CreatedAt)

						if tc.sortOrder == "ASC" {
							assert.True(t, prevTime.Before(currTime) || prevTime.Equal(currTime),
								"createdAt ASC: %s should be <= %s", prev.CreatedAt, curr.CreatedAt)
						} else {
							assert.True(t, prevTime.After(currTime) || prevTime.Equal(currTime),
								"createdAt DESC: %s should be >= %s", prev.CreatedAt, curr.CreatedAt)
						}
					} else if tc.sortBy == "processing_time_ms" {
						if tc.sortOrder == "ASC" {
							assert.LessOrEqual(t, prev.ProcessingTimeMs, curr.ProcessingTimeMs,
								"processingTimeMs ASC: %d should be <= %d", prev.ProcessingTimeMs, curr.ProcessingTimeMs)
						} else {
							assert.GreaterOrEqual(t, prev.ProcessingTimeMs, curr.ProcessingTimeMs,
								"processingTimeMs DESC: %d should be >= %d", prev.ProcessingTimeMs, curr.ProcessingTimeMs)
						}
					}
				}
			}
		})
	}
}

// Test 1.3.43: Date range boundary semantics
func TestValidation_1_3_43_DateRangeBoundarySemantics(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(910).String()

	// Create a validation now
	now := time.Now().UTC()
	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(911).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("150"),
		Currency:             "BRL",
		TransactionTimestamp: now.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, _ := testutil.CreateValidation(t, req)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Test startDate is inclusive
	t.Run("startDate is inclusive", func(t *testing.T) {
		// Use a startDate slightly before now
		startDate := now.Add(-1 * time.Second).Format(time.RFC3339)

		listResp, listBody := testutil.ListValidations(t,
			fmt.Sprintf("account_id=%s&start_date=%s", accountID, startDate))
		defer listResp.Body.Close()

		require.Equal(t, http.StatusOK, listResp.StatusCode)

		var result testutil.ListValidationsResponse
		err := json.Unmarshal(listBody, &result)
		require.NoError(t, err)

		// All items should have createdAt >= startDate
		for _, item := range result.TransactionValidations {
			createdAt, _ := time.Parse(time.RFC3339, item.CreatedAt)
			startTime, _ := time.Parse(time.RFC3339, startDate)
			assert.True(t, !createdAt.Before(startTime),
				"startDate should be inclusive: createdAt %s should be >= %s", item.CreatedAt, startDate)
		}
	})

	// Test endDate is exclusive
	t.Run("endDate is exclusive", func(t *testing.T) {
		// Use an endDate slightly after now
		endDate := now.Add(1 * time.Second).Format(time.RFC3339)

		listResp, listBody := testutil.ListValidations(t,
			fmt.Sprintf("account_id=%s&end_date=%s", accountID, endDate))
		defer listResp.Body.Close()

		require.Equal(t, http.StatusOK, listResp.StatusCode)

		var result testutil.ListValidationsResponse
		err := json.Unmarshal(listBody, &result)
		require.NoError(t, err)

		// All items should have createdAt < endDate (exclusive)
		for _, item := range result.TransactionValidations {
			createdAt, _ := time.Parse(time.RFC3339, item.CreatedAt)
			endTime, _ := time.Parse(time.RFC3339, endDate)
			assert.True(t, createdAt.Before(endTime),
				"endDate should be exclusive: createdAt %s should be < %s", item.CreatedAt, endDate)
		}
	})
}

// Test 1.3.44: Limit parameter exactly 1000 (maximum) is accepted
func TestValidation_1_3_44_LimitMaximum1000Accepted(t *testing.T) {
	listResp, listBody := testutil.ListValidations(t, "limit=1000")
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"limit=1000 should be accepted, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err := json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Just verify we got a valid response
	assert.NotNil(t, result.TransactionValidations, "transactionValidations should be present")
}

// Test 1.3.45: Filter by only startDate (no endDate)
func TestValidation_1_3_45_FilterByOnlyStartDate(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(920).String()
	now := time.Now().UTC()

	// Create a validation
	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(921).String(),
		TransactionType:      "WIRE",
		Amount:               decimal.RequireFromString("400"),
		Currency:             "BRL",
		TransactionTimestamp: now.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, _ := testutil.CreateValidation(t, req)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Query with only startDate
	startDate := now.Add(-1 * time.Hour).Format(time.RFC3339)

	listResp, listBody := testutil.ListValidations(t,
		fmt.Sprintf("account_id=%s&start_date=%s", accountID, startDate))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK with only startDate filter, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err := json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify all results have createdAt >= startDate
	startTime := now.Add(-1 * time.Hour)
	for _, item := range result.TransactionValidations {
		createdAt, parseErr := time.Parse(time.RFC3339, item.CreatedAt)
		if parseErr == nil {
			assert.True(t, !createdAt.Before(startTime),
				"All results should have createdAt >= startDate: %s should be >= %s",
				item.CreatedAt, startDate)
		}
	}

	// Verify we got at least our created validation
	assert.GreaterOrEqual(t, len(result.TransactionValidations), 1,
		"Should have at least 1 validation after startDate")
}

// Test 1.3.46: Filter by only endDate (no startDate)
func TestValidation_1_3_46_FilterByOnlyEndDate(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(930).String()
	now := time.Now().UTC()

	// Create a validation
	req := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(931).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("350"),
		Currency:             "BRL",
		TransactionTimestamp: now.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, _ := testutil.CreateValidation(t, req)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Query with only endDate (in the future to include recent validations)
	endDate := now.Add(1 * time.Hour).Format(time.RFC3339)

	listResp, listBody := testutil.ListValidations(t,
		fmt.Sprintf("account_id=%s&end_date=%s", accountID, endDate))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK with only endDate filter, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err := json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify all results have createdAt < endDate
	endTime := now.Add(1 * time.Hour)
	for _, item := range result.TransactionValidations {
		createdAt, parseErr := time.Parse(time.RFC3339, item.CreatedAt)
		if parseErr == nil {
			assert.True(t, createdAt.Before(endTime),
				"All results should have createdAt < endDate: %s should be < %s",
				item.CreatedAt, endDate)
		}
	}

	// Verify we got at least our created validation
	assert.GreaterOrEqual(t, len(result.TransactionValidations), 1,
		"Should have at least 1 validation before endDate")
}

// Test 1.3.29: ValidationSummary contains all required fields
func TestValidation_1_3_29_ValidationSummaryFields(t *testing.T) {
	// Create a validation to ensure there's at least one record
	accountID := testutil.MustDeterministicUUID(720).String()
	requestID := testutil.MustDeterministicUUID(721).String()

	req := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("550"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created from POST, got: %s", string(body))

	// List validations with limit=1 to get at least one item
	listResp, listBody := testutil.ListValidations(t, fmt.Sprintf("account_id=%s&limit=1", accountID))
	defer listResp.Body.Close()

	require.Equal(t, http.StatusOK, listResp.StatusCode,
		"Expected 200 OK from GET /v1/validations, got: %s", string(listBody))

	var result testutil.ListValidationsResponse
	err := json.Unmarshal(listBody, &result)
	require.NoError(t, err)

	// Verify we have at least one validation
	require.NotEmpty(t, result.TransactionValidations, "transactionValidations array should not be empty")

	// Verify each item in the transactionValidations array contains all required fields
	for i, item := range result.TransactionValidations {
		// Verify id (validationId) - must be non-empty string
		assert.NotEmpty(t, item.ID,
			"Item %d: id (validationId) should be present and not empty", i)

		// Verify requestId - should be a valid UUID format
		_, uuidErr := uuid.Parse(item.ID)
		assert.NoError(t, uuidErr,
			"Item %d: id should be a valid UUID format, got: %s", i, item.ID)

		// Verify transactionType - must be one of valid enum values
		assert.Contains(t, []string{"CARD", "WIRE", "PIX", "CRYPTO"}, item.TransactionType,
			"Item %d: transactionType should be valid enum value, got: %s", i, item.TransactionType)

		// Verify amount - must be present (decimal.Decimal, can be 0 or positive)
		assert.True(t, item.Amount.GreaterThanOrEqual(decimal.Zero),
			"Item %d: amount should be >= 0, got: %s", i, item.Amount)

		// Verify currency - must be 3-character string (ISO 4217)
		assert.Len(t, item.Currency, 3,
			"Item %d: currency should be 3 characters (ISO 4217), got: %s", i, item.Currency)

		// Verify decision - must be one of valid enum values
		assert.Contains(t, []string{"ALLOW", "DENY", "REVIEW"}, item.Decision,
			"Item %d: decision should be valid enum value, got: %s", i, item.Decision)

		// Verify accountId - must be non-empty string and valid UUID
		assert.NotEmpty(t, item.AccountID,
			"Item %d: accountId should be present and not empty", i)
		_, accountUUIDErr := uuid.Parse(item.AccountID)
		assert.NoError(t, accountUUIDErr,
			"Item %d: accountId should be a valid UUID format, got: %s", i, item.AccountID)

		// Verify matchedRuleIds - must be an array (may be empty)
		assert.NotNil(t, item.MatchedRuleIDs,
			"Item %d: matchedRuleIds should be present (array, may be empty)", i)
		// If not empty, verify each element is a valid UUID
		for j, ruleID := range item.MatchedRuleIDs {
			_, ruleUUIDErr := uuid.Parse(ruleID)
			assert.NoError(t, ruleUUIDErr,
				"Item %d: matchedRuleIds[%d] should be a valid UUID, got: %s", i, j, ruleID)
		}

		// Note: evaluatedRuleIds is not present in ValidationSummary according to the struct
		// The roadmap mentions it but the actual struct has exceededLimitIds instead

		// Verify exceededLimitIds - must be an array (may be empty)
		assert.NotNil(t, item.ExceededLimitIDs,
			"Item %d: exceededLimitIds should be present (array, may be empty)", i)

		// Verify processingTimeMs - must be present and >= 0
		assert.GreaterOrEqual(t, item.ProcessingTimeMs, float64(0),
			"Item %d: processingTimeMs should be >= 0, got: %f", i, item.ProcessingTimeMs)

		// Verify createdAt - must be valid ISO 8601 timestamp
		assert.NotEmpty(t, item.CreatedAt,
			"Item %d: createdAt should be present and not empty", i)
		_, timeErr := time.Parse(time.RFC3339, item.CreatedAt)
		assert.NoError(t, timeErr,
			"Item %d: createdAt should be valid ISO 8601 (RFC3339) timestamp, got: %s", i, item.CreatedAt)
	}
}
