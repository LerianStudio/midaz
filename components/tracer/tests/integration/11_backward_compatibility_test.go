// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	testutil_integration "github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil_integration"
)

// =============================================================================
// Backward Compatibility Tests
//
// These tests validate that the new Time Windows and Custom Periods features
// do not break existing limit functionality.
//
// Critical: All existing DAILY, MONTHLY, WEEKLY limits without time windows
// or custom periods must continue to work exactly as before.
// =============================================================================

// TestBackwardCompatibility_DAILY_NoTimeWindow verifies that existing DAILY
// limits (without time windows) continue to work correctly.
//
// This is the most common use case in production.
func TestBackwardCompatibility_DAILY_NoTimeWindow(t *testing.T) {
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-15T14:30:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	accountID := uuid.New().String()

	// Create traditional DAILY limit (no time window, no custom period)
	limitID := createTraditionalLimit(t, accountID, "DAILY", "1000.00")
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

	// Transaction 1: 400.00
	tx1Time := testutil.TestNow().Add(-30 * time.Second)
	req1 := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("400.00"),
		Currency:             "BRL",
		TransactionTimestamp: tx1Time.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, req1)
	defer resp1.Body.Close()

	require.Equal(t, http.StatusCreated, resp1.StatusCode)

	var result1 testutil.ValidationResponse
	err = json.Unmarshal(body1, &result1)
	require.NoError(t, err)

	require.Len(t, result1.LimitUsageDetails, 1)
	detail1 := result1.LimitUsageDetails[0]

	// CRITICAL: Traditional limit should work as before (always evaluated, never skipped)
	assert.False(t, detail1.Skipped, "Traditional DAILY limit should NEVER be skipped")
	assert.True(t, detail1.CurrentUsage.Equal(decimal.RequireFromString("400.00")),
		"Counter should be incremented to 400")
	assert.False(t, detail1.Exceeded, "400 < 1000, should not be exceeded")

	// Transaction 2: 700.00 (would exceed: 400 + 700 = 1100 > 1000)
	tx2Time := testutil.TestNow().Add(-20 * time.Second)
	req2 := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("700.00"),
		Currency:             "BRL",
		TransactionTimestamp: tx2Time.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp2, body2 := testutil.CreateValidation(t, req2)
	defer resp2.Body.Close()

	require.Equal(t, http.StatusCreated, resp2.StatusCode)

	var result2 testutil.ValidationResponse
	err = json.Unmarshal(body2, &result2)
	require.NoError(t, err)

	require.Len(t, result2.LimitUsageDetails, 1)
	detail2 := result2.LimitUsageDetails[0]

	// CRITICAL: Should detect exceeded
	assert.False(t, detail2.Skipped)
	assert.True(t, detail2.Exceeded, "1100 > 1000, should be exceeded")
	assert.True(t, detail2.CurrentUsage.Equal(decimal.RequireFromString("1100.00")),
		"Counter should be 1100 (400 + 700, transaction incremented counter even when exceeded)")

	t.Logf("✅ Backward Compatibility DAILY: Traditional limit works correctly (400 allowed, 700 blocked)")
}

// TestBackwardCompatibility_MONTHLY_NoTimeWindow verifies that existing MONTHLY
// limits continue to work correctly.
func TestBackwardCompatibility_MONTHLY_NoTimeWindow(t *testing.T) {
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-15T10:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	accountID := uuid.New().String()

	// Create traditional MONTHLY limit
	limitID := createTraditionalLimit(t, accountID, "MONTHLY", "50000.00")
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

	// Multiple transactions to verify accumulation
	amounts := []string{"10000.00", "15000.00", "20000.00"}
	expectedTotal := decimal.RequireFromString("45000.00") // 10k + 15k + 20k

	for i, amount := range amounts {
		txTime := testutil.TestNow().Add(time.Duration(-30+i*10) * time.Second)
		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString(amount),
			Currency:             "BRL",
			TransactionTimestamp: txTime.Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var result testutil.ValidationResponse
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		assert.False(t, detail.Skipped, "Traditional MONTHLY limit should never be skipped")
		assert.False(t, detail.Exceeded, "All amounts are within 50k limit")
	}

	// Final verification: counter should be 45000
	finalTxTime := testutil.TestNow().Add(-5 * time.Second)
	finalReq := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("1.00"), // Small amount to check counter
		Currency:             "BRL",
		TransactionTimestamp: finalTxTime.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	finalResp, finalBody := testutil.CreateValidation(t, finalReq)
	defer finalResp.Body.Close()

	require.Equal(t, http.StatusCreated, finalResp.StatusCode)

	var finalResult testutil.ValidationResponse
	err = json.Unmarshal(finalBody, &finalResult)
	require.NoError(t, err)

	require.Len(t, finalResult.LimitUsageDetails, 1)
	finalDetail := finalResult.LimitUsageDetails[0]

	expectedFinalTotal := expectedTotal.Add(decimal.RequireFromString("1.00"))
	assert.True(t, finalDetail.CurrentUsage.Equal(expectedFinalTotal),
		"Counter should accumulate correctly: expected %s, got %s",
		expectedFinalTotal.String(), finalDetail.CurrentUsage.String())

	t.Logf("✅ Backward Compatibility MONTHLY: Accumulation works correctly (45001)")
}

// TestBackwardCompatibility_WEEKLY_NoTimeWindow verifies that existing WEEKLY
// limits continue to work correctly.
func TestBackwardCompatibility_WEEKLY_NoTimeWindow(t *testing.T) {
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-16T10:00:00Z", // Monday (start of week)
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	accountID := uuid.New().String()

	// Create traditional WEEKLY limit
	limitID := createTraditionalLimit(t, accountID, "WEEKLY", "10000.00")
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

	// Transaction 1: 3000.00
	tx1Time := testutil.TestNow().Add(-30 * time.Second)
	req1 := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("3000.00"),
		Currency:             "BRL",
		TransactionTimestamp: tx1Time.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp1, body1 := testutil.CreateValidation(t, req1)
	defer resp1.Body.Close()

	require.Equal(t, http.StatusCreated, resp1.StatusCode)

	var result1 testutil.ValidationResponse
	err = json.Unmarshal(body1, &result1)
	require.NoError(t, err)

	require.Len(t, result1.LimitUsageDetails, 1)
	detail1 := result1.LimitUsageDetails[0]

	assert.False(t, detail1.Skipped)
	assert.True(t, detail1.CurrentUsage.Equal(decimal.RequireFromString("3000.00")))
	assert.False(t, detail1.Exceeded)

	// Transaction 2: 8000.00 (would exceed: 3000 + 8000 = 11000 > 10000)
	tx2Time := testutil.TestNow().Add(-20 * time.Second)
	req2 := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("8000.00"),
		Currency:             "BRL",
		TransactionTimestamp: tx2Time.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp2, body2 := testutil.CreateValidation(t, req2)
	defer resp2.Body.Close()

	require.Equal(t, http.StatusCreated, resp2.StatusCode)

	var result2 testutil.ValidationResponse
	err = json.Unmarshal(body2, &result2)
	require.NoError(t, err)

	require.Len(t, result2.LimitUsageDetails, 1)
	detail2 := result2.LimitUsageDetails[0]

	// Should be exceeded
	assert.False(t, detail2.Skipped)
	assert.True(t, detail2.Exceeded, "11000 > 10000, should be exceeded")

	t.Logf("✅ Backward Compatibility WEEKLY: Traditional limit works correctly (3000 allowed, 8000 blocked)")
}

// TestBackwardCompatibility_API_Fields verifies that API responses maintain
// backward compatibility (no breaking changes in response structure).
func TestBackwardCompatibility_API_Fields(t *testing.T) {
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-15T10:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	accountID := uuid.New().String()

	// Create traditional limit
	limitID := createTraditionalLimit(t, accountID, "DAILY", "5000.00")
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

	// Send transaction
	txTime := testutil.TestNow().Add(-30 * time.Second)
	req := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("1000.00"),
		Currency:             "BRL",
		TransactionTimestamp: txTime.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Parse response as generic map to check all fields
	var rawResponse map[string]interface{}
	err = json.Unmarshal(body, &rawResponse)
	require.NoError(t, err)

	// CRITICAL: All existing fields must be present
	requiredFields := []string{
		"validationId",
		"requestId",
		"decision",
		"reason",
		"matchedRuleIds",
		"limitUsageDetails",
		"evaluatedAt",
	}

	for _, field := range requiredFields {
		assert.Contains(t, rawResponse, field,
			"Response must contain field '%s' for backward compatibility", field)
	}

	// Verify limitUsageDetails structure
	limitDetails, ok := rawResponse["limitUsageDetails"].([]interface{})
	require.True(t, ok, "limitUsageDetails must be an array")
	require.NotEmpty(t, limitDetails, "limitUsageDetails should not be empty")

	firstDetail, ok := limitDetails[0].(map[string]interface{})
	require.True(t, ok, "limitUsageDetails[0] must be an object")

	// Required fields in limitUsageDetails (actual fields from API)
	limitDetailFields := []string{
		"limitId",
		"limitAmount", // Not maxAmount
		"currentUsage",
		"exceeded",
		"period",
		"scope",
		"attemptedAmount",
	}

	for _, field := range limitDetailFields {
		assert.Contains(t, firstDetail, field,
			"limitUsageDetails must contain field '%s' for backward compatibility", field)
	}

	// NEW fields (skipped/skipReason) - may not be present when false/empty
	// This is acceptable for backward compatibility (omitempty)

	// If skipped=false, skipReason should be empty or omitted
	if skipReason, exists := firstDetail["skipReason"]; exists {
		assert.Empty(t, skipReason, "skipReason should be empty when not skipped")
	}

	t.Logf("✅ Backward Compatibility API: All required fields present, new fields with correct defaults")
}

// TestBackwardCompatibility_MultipleTraditionalLimits verifies that multiple
// traditional limits can coexist and work independently.
func TestBackwardCompatibility_MultipleTraditionalLimits(t *testing.T) {
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-15T10:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	accountID := uuid.New().String()

	// Create 3 traditional limits
	dailyID := createTraditionalLimit(t, accountID, "DAILY", "1000.00")
	testutil.ActivateLimit(t, dailyID)
	defer testutil.CleanupLimit(t, dailyID)

	weeklyID := createTraditionalLimit(t, accountID, "WEEKLY", "5000.00")
	testutil.ActivateLimit(t, weeklyID)
	defer testutil.CleanupLimit(t, weeklyID)

	monthlyID := createTraditionalLimit(t, accountID, "MONTHLY", "20000.00")
	testutil.ActivateLimit(t, monthlyID)
	defer testutil.CleanupLimit(t, monthlyID)

	// Send transaction
	txTime := testutil.TestNow().Add(-30 * time.Second)
	req := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("500.00"),
		Currency:             "BRL",
		TransactionTimestamp: txTime.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var result testutil.ValidationResponse
	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Should return 3 limit details (one per limit)
	require.Len(t, result.LimitUsageDetails, 3, "Should evaluate all 3 traditional limits")

	// All should be evaluated (not skipped)
	for i, detail := range result.LimitUsageDetails {
		assert.False(t, detail.Skipped,
			"Limit %d should NOT be skipped", i+1)
		assert.True(t, detail.CurrentUsage.Equal(decimal.RequireFromString("500.00")),
			"Limit %d counter should be 500", i+1)
		assert.False(t, detail.Exceeded,
			"Limit %d should NOT be exceeded (500 < all limits)", i+1)
	}

	t.Logf("✅ Backward Compatibility Multiple Limits: All 3 traditional limits work independently (all at 500)")
}

// =============================================================================
// Helper Functions
// =============================================================================

// createTraditionalLimit creates a "traditional" limit without time windows
// or custom periods (backward compatible with pre-feature behavior).
func createTraditionalLimit(t *testing.T, accountID, limitType, maxAmount string) string {
	t.Helper()

	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	reqBody := map[string]interface{}{
		"name":      "Test Traditional " + limitType + " " + accountID + " " + testutil.RandomSuffix(),
		"limitType": limitType,
		"maxAmount": maxAmount,
		"currency":  "BRL",
		"scopes": []map[string]interface{}{
			{"accountId": accountID},
		},
		// NOTE: No activeTimeStart, activeTimeEnd, customStartDate, or customEndDate
		// This is the "traditional" limit format
	}

	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits", bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Failed to create traditional limit: %s", string(respBody))

	var limit map[string]interface{}
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	limitID, ok := limit["limitId"].(string)
	require.True(t, ok, "Response should contain limitId")

	return limitID
}
