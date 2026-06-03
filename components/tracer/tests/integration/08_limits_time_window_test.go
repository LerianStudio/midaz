// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil_integration"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Integration Tests: Limits with Time Windows and Custom Periods (08)
//
// Tests for PIX Compliance pattern, time windows, custom periods, and WEEKLY limits.
// Validates that transactions are evaluated only within configured time windows
// and custom date ranges.
// =============================================================================

// =============================================================================
// 8.1 Time Window Tests (4 tests)
// =============================================================================

// TestTimeWindow_InsideWindow verifies that transactions inside the time window are evaluated.
func TestTimeWindow_InsideWindow(t *testing.T) {
	accountID := uuid.New().String()

	// Use MOCK_TIME (server time) for deterministic window calculation
	now := testutil.TestNow().UTC()
	currentHour := now.Hour()

	startHour := (currentHour - 6 + 24) % 24
	endHour := (currentHour + 6) % 24

	startTime := fmt.Sprintf("%02d:00", startHour)
	endTime := fmt.Sprintf("%02d:00", endHour)

	limitID := createLimitWithTimeWindow(t, accountID, startTime, endTime, "1000.00")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Transaction 1 hour ago (inside window)
	txTime := now.Add(-1 * time.Hour)
	req := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("300.00"),
		Currency:             "BRL",
		TransactionTimestamp: txTime.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Transaction should be allowed, got: %s", string(body))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body, &result)
	require.NoError(t, err)

	require.Len(t, result.LimitUsageDetails, 1, "Should have 1 limit detail")
	detail := result.LimitUsageDetails[0]

	assert.False(t, detail.Skipped, "Limit should NOT be skipped (inside window)")
	assert.Empty(t, detail.SkipReason, "Should have no skip reason")
	assert.True(t, detail.CurrentUsage.Equal(decimal.RequireFromString("300.00")),
		"Counter should be incremented to 300, got %s", detail.CurrentUsage)
	assert.False(t, detail.Exceeded, "Limit should not be exceeded")
}

// TestTimeWindow_OutsideWindow verifies that transactions outside the time window are skipped.
func TestTimeWindow_OutsideWindow(t *testing.T) {
	accountID := uuid.New().String()

	// Create a limit with a narrow window that excludes current time
	// Use MOCK_TIME (server time) for deterministic window calculation
	now := testutil.TestNow().UTC()
	currentHour := now.Hour()

	// Window starts 6 hours from now, ends 10 hours from now (excludes current time)
	startHour := (currentHour + 6) % 24
	endHour := (currentHour + 10) % 24

	startTime := fmt.Sprintf("%02d:00", startHour)
	endTime := fmt.Sprintf("%02d:00", endHour)

	limitID := createLimitWithTimeWindow(t, accountID, startTime, endTime, "1000.00")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Transaction 1 hour ago (outside window)
	txTime := now.Add(-1 * time.Hour)
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

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Transaction should be allowed, got: %s", string(body))

	var result testutil.ValidationResponse
	err := json.Unmarshal(body, &result)
	require.NoError(t, err)

	require.Len(t, result.LimitUsageDetails, 1, "Should have 1 limit detail")
	detail := result.LimitUsageDetails[0]

	assert.True(t, detail.Skipped, "Limit should be skipped (outside window)")
	assert.Equal(t, "outside_time_window", detail.SkipReason, "Skip reason should be 'outside_time_window'")
	assert.True(t, detail.CurrentUsage.IsZero(),
		"Counter should be zero (not incremented), got %s", detail.CurrentUsage)
	assert.False(t, detail.Exceeded, "Skipped limit should not be exceeded")
}

// TestTimeWindow_OvernightWindow verifies overnight windows spanning midnight work correctly.
// Since time windows are evaluated using serverNow (security), we test with overnight window
// that spans midnight to verify the overnight logic works correctly.
// Tests explicit hours: 23:00 inside, 03:00 inside, 12:00 outside.
func TestTimeWindow_OvernightWindow(t *testing.T) {
	accountID := uuid.New().String()

	// Test Case 1: 23:00 (inside overnight window 20:00-06:00)
	t.Run("23h_inside_overnight_window", func(t *testing.T) {
		// Restart server with MOCK_TIME=23:00
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-03-11T23:00:00Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		// Create overnight window (20:00-06:00)
		limitID := createLimitWithTimeWindow(t, accountID, "20:00", "06:00", "2000.00")
		testutil.ActivateLimit(t, limitID)
		defer testutil.CleanupLimit(t, limitID)

		txTime := testutil.TestNow().Add(-30 * time.Second)
		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("100.00"),
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

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		// CRITICAL: 23:00 is INSIDE overnight window (20:00-06:00)
		assert.False(t, detail.Skipped, "23:00 should be inside overnight window 20:00-06:00")
		assert.True(t, detail.CurrentUsage.Equal(decimal.RequireFromString("100.00")),
			"Counter should be incremented, got %s", detail.CurrentUsage)
	})

	// Test Case 2: 03:00 (inside overnight window 20:00-06:00)
	t.Run("03h_inside_overnight_window", func(t *testing.T) {
		// Restart server with MOCK_TIME=03:00 (next day)
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-03-12T03:00:00Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		// Create overnight window (20:00-06:00)
		limitID := createLimitWithTimeWindow(t, accountID, "20:00", "06:00", "2000.00")
		testutil.ActivateLimit(t, limitID)
		defer testutil.CleanupLimit(t, limitID)

		txTime := testutil.TestNow().Add(-30 * time.Second)
		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("100.00"),
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

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		// CRITICAL: 03:00 is INSIDE overnight window (20:00-06:00)
		assert.False(t, detail.Skipped, "03:00 should be inside overnight window 20:00-06:00")
		assert.True(t, detail.CurrentUsage.Equal(decimal.RequireFromString("100.00")),
			"Counter should be incremented, got %s", detail.CurrentUsage)
	})

	// Test Case 3: 12:00 (outside overnight window 20:00-06:00)
	t.Run("12h_outside_overnight_window", func(t *testing.T) {
		// Restart server with MOCK_TIME=12:00
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-03-12T12:00:00Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		// Create overnight window (20:00-06:00)
		limitID := createLimitWithTimeWindow(t, accountID, "20:00", "06:00", "2000.00")
		testutil.ActivateLimit(t, limitID)
		defer testutil.CleanupLimit(t, limitID)

		txTime := testutil.TestNow().Add(-30 * time.Second)
		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("100.00"),
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

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		// CRITICAL: 12:00 is OUTSIDE overnight window (20:00-06:00)
		assert.True(t, detail.Skipped, "12:00 should be outside overnight window 20:00-06:00")
		assert.Equal(t, "outside_time_window", detail.SkipReason)
		assert.True(t, detail.CurrentUsage.IsZero(),
			"Counter should be zero (not incremented), got %s", detail.CurrentUsage)
	})
}

// TestTimeWindow_Boundaries verifies half-open interval semantics [start, end).
// Tests exact boundaries: start (inclusive), before start (excluded), before end (included), end (excluded).
func TestTimeWindow_Boundaries(t *testing.T) {
	accountID := uuid.New().String()

	// Test Case 1: Exactly at start (06:00:00) - INCLUSIVE
	t.Run("at_start_06h00m00s_inclusive", func(t *testing.T) {
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-03-11T06:00:00Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		// Window: 06:00-20:00
		limitID := createLimitWithTimeWindow(t, accountID, "06:00", "20:00", "1000.00")
		testutil.ActivateLimit(t, limitID)
		defer testutil.CleanupLimit(t, limitID)

		txTime := testutil.TestNow().Add(-30 * time.Second)
		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("200.00"),
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

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		// CRITICAL: 06:00:00 is INCLUSIVE (start of [start, end))
		assert.False(t, detail.Skipped, "06:00:00 should be inside window [06:00, 20:00)")
		assert.True(t, detail.CurrentUsage.Equal(decimal.RequireFromString("200.00")),
			"Counter should be incremented, got %s", detail.CurrentUsage)
	})

	// Test Case 2: One second before start (05:59:59) - EXCLUSIVE
	t.Run("before_start_05h59m59s_exclusive", func(t *testing.T) {
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-03-11T05:59:59Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		// Window: 06:00-20:00
		limitID := createLimitWithTimeWindow(t, accountID, "06:00", "20:00", "1000.00")
		testutil.ActivateLimit(t, limitID)
		defer testutil.CleanupLimit(t, limitID)

		txTime := testutil.TestNow().Add(-30 * time.Second)
		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("200.00"),
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

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		// CRITICAL: 05:59:59 is OUTSIDE window [06:00, 20:00)
		assert.True(t, detail.Skipped, "05:59:59 should be outside window [06:00, 20:00)")
		assert.Equal(t, "outside_time_window", detail.SkipReason)
		assert.True(t, detail.CurrentUsage.IsZero(),
			"Counter should be zero, got %s", detail.CurrentUsage)
	})

	// Test Case 3: One second before end (19:59:59) - INCLUSIVE (still inside)
	t.Run("before_end_19h59m59s_inclusive", func(t *testing.T) {
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-03-11T19:59:59Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		// Window: 06:00-20:00
		limitID := createLimitWithTimeWindow(t, accountID, "06:00", "20:00", "1000.00")
		testutil.ActivateLimit(t, limitID)
		defer testutil.CleanupLimit(t, limitID)

		txTime := testutil.TestNow().Add(-30 * time.Second)
		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("200.00"),
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

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		// CRITICAL: 19:59:59 is INSIDE window [06:00, 20:00)
		assert.False(t, detail.Skipped, "19:59:59 should be inside window [06:00, 20:00)")
		assert.True(t, detail.CurrentUsage.Equal(decimal.RequireFromString("200.00")),
			"Counter should be incremented, got %s", detail.CurrentUsage)
	})

	// Test Case 4: Exactly at end (20:00:00) - EXCLUSIVE
	t.Run("at_end_20h00m00s_exclusive", func(t *testing.T) {
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-03-11T20:00:00Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		// Window: 06:00-20:00
		limitID := createLimitWithTimeWindow(t, accountID, "06:00", "20:00", "1000.00")
		testutil.ActivateLimit(t, limitID)
		defer testutil.CleanupLimit(t, limitID)

		txTime := testutil.TestNow().Add(-30 * time.Second)
		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("200.00"),
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

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		// CRITICAL: 20:00:00 is EXCLUSIVE (end of [start, end))
		assert.True(t, detail.Skipped, "20:00:00 should be outside window [06:00, 20:00)")
		assert.Equal(t, "outside_time_window", detail.SkipReason)
		assert.True(t, detail.CurrentUsage.IsZero(),
			"Counter should be zero, got %s", detail.CurrentUsage)
	})
}

// Helper Functions
// =============================================================================

// createLimitWithTimeWindow creates a DAILY limit with a time window for the specified account.
// start and end are in "HH:MM" format (e.g., "06:00", "20:00").
// maxAmount is a decimal string (e.g., "5000.00").
// Returns the limit ID.
func createLimitWithTimeWindow(t *testing.T, accountID, start, end, maxAmount string) string {
	t.Helper()

	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Use sanitized name without colons (validation rule TRC-0129)
	sanitizedStart := strings.ReplaceAll(start, ":", "")
	sanitizedEnd := strings.ReplaceAll(end, ":", "")

	reqBody := map[string]interface{}{
		"name":            fmt.Sprintf("Test Limit TW %s-%s %s", sanitizedStart, sanitizedEnd, testutil.RandomSuffix()),
		"limitType":       "DAILY",
		"maxAmount":       maxAmount,
		"currency":        "BRL",
		"activeTimeStart": start,
		"activeTimeEnd":   end,
		"scopes": []map[string]interface{}{
			{"accountId": accountID},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits", bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)

	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Failed to create limit with time window: %s", string(respBody))

	var limit map[string]interface{}
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	limitID, ok := limit["limitId"].(string)
	require.True(t, ok, "Response should contain limitId")

	return limitID
}

// =============================================================================
// 8.2 Custom Period Tests (5 tests)
// =============================================================================

// TestCustomPeriod_BeforePeriod verifies transactions before custom period are skipped.
func TestCustomPeriod_BeforePeriod(t *testing.T) {
	accountID := uuid.New().String()

	// Black Friday period: 27/nov - 28/nov
	// Test with MOCK_TIME = 26/nov (before period)
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-11-26T10:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	// Create CUSTOM limit for Black Friday (27-28 nov)
	limitID := createLimitWithCustomPeriod(t, accountID, "2026-11-27T00:00:00Z", "2026-11-29T00:00:00Z", "10000.00")
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

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

	require.Len(t, result.LimitUsageDetails, 1)
	detail := result.LimitUsageDetails[0]

	// CRITICAL: 26/nov is BEFORE custom period [27/nov, 29/nov)
	assert.True(t, detail.Skipped, "Transaction before custom period should be skipped")
	assert.Equal(t, "outside_custom_period", detail.SkipReason)
	assert.True(t, detail.CurrentUsage.IsZero(),
		"Counter should be zero (not incremented), got %s", detail.CurrentUsage)
}

// TestCustomPeriod_DuringPeriod verifies transactions during custom period are evaluated.
func TestCustomPeriod_DuringPeriod(t *testing.T) {
	accountID := uuid.New().String()

	// Black Friday period: 27/nov - 28/nov
	// Test with MOCK_TIME = 27/nov 15:00 (during period)
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-11-27T15:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	// Create CUSTOM limit for Black Friday (27-28 nov)
	limitID := createLimitWithCustomPeriod(t, accountID, "2026-11-27T00:00:00Z", "2026-11-29T00:00:00Z", "10000.00")
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

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

	require.Len(t, result.LimitUsageDetails, 1)
	detail := result.LimitUsageDetails[0]

	// CRITICAL: 27/nov 15:00 is INSIDE custom period [27/nov, 29/nov)
	assert.False(t, detail.Skipped, "Transaction during custom period should be evaluated")
	assert.Empty(t, detail.SkipReason)
	assert.True(t, detail.CurrentUsage.Equal(decimal.RequireFromString("500.00")),
		"Counter should be incremented, got %s", detail.CurrentUsage)
}

// TestCustomPeriod_AfterPeriod verifies transactions after custom period are skipped.
func TestCustomPeriod_AfterPeriod(t *testing.T) {
	accountID := uuid.New().String()

	// Black Friday period: 27/nov - 28/nov
	// Test with MOCK_TIME = 29/nov (after period - exclusive end)
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-11-29T10:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	// Create CUSTOM limit for Black Friday (27-28 nov)
	limitID := createLimitWithCustomPeriod(t, accountID, "2026-11-27T00:00:00Z", "2026-11-29T00:00:00Z", "10000.00")
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

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

	require.Len(t, result.LimitUsageDetails, 1)
	detail := result.LimitUsageDetails[0]

	// CRITICAL: 29/nov is AFTER custom period [27/nov, 29/nov) - end is exclusive
	assert.True(t, detail.Skipped, "Transaction after custom period should be skipped")
	assert.Equal(t, "outside_custom_period", detail.SkipReason)
	assert.True(t, detail.CurrentUsage.IsZero(),
		"Counter should be zero (not incremented), got %s", detail.CurrentUsage)
}

// TestCustomPeriod_Boundaries verifies half-open interval semantics [start, end).
func TestCustomPeriod_Boundaries(t *testing.T) {
	accountID := uuid.New().String()

	// Test Case 1: Exactly at start (27/nov 00:00:00) - INCLUSIVE
	t.Run("at_start_inclusive", func(t *testing.T) {
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-11-27T00:00:00Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		limitID := createLimitWithCustomPeriod(t, accountID, "2026-11-27T00:00:00Z", "2026-11-29T00:00:00Z", "10000.00")
		testutil.ActivateLimit(t, limitID)
		defer testutil.CleanupLimit(t, limitID)

		txTime := testutil.TestNow().Add(-30 * time.Second)
		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("100.00"),
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

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		// Start is INCLUSIVE
		assert.False(t, detail.Skipped, "Start boundary should be inclusive")
		assert.True(t, detail.CurrentUsage.Equal(decimal.RequireFromString("100.00")))
	})

	// Test Case 2: One second before end (28/nov 23:59:59) - INSIDE
	t.Run("before_end_inclusive", func(t *testing.T) {
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-11-28T23:59:59Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		limitID := createLimitWithCustomPeriod(t, accountID, "2026-11-27T00:00:00Z", "2026-11-29T00:00:00Z", "10000.00")
		testutil.ActivateLimit(t, limitID)
		defer testutil.CleanupLimit(t, limitID)

		txTime := testutil.TestNow().Add(-30 * time.Second)
		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("100.00"),
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

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		// Before end is INSIDE
		assert.False(t, detail.Skipped, "Before end should be inside period")
		assert.True(t, detail.CurrentUsage.Equal(decimal.RequireFromString("100.00")))
	})

	// Test Case 3: Exactly at end (29/nov 00:00:00) - EXCLUSIVE
	t.Run("at_end_exclusive", func(t *testing.T) {
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-11-29T00:00:00Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		limitID := createLimitWithCustomPeriod(t, accountID, "2026-11-27T00:00:00Z", "2026-11-29T00:00:00Z", "10000.00")
		testutil.ActivateLimit(t, limitID)
		defer testutil.CleanupLimit(t, limitID)

		txTime := testutil.TestNow().Add(-30 * time.Second)
		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("100.00"),
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

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		// End is EXCLUSIVE
		assert.True(t, detail.Skipped, "End boundary should be exclusive")
		assert.Equal(t, "outside_custom_period", detail.SkipReason)
		assert.True(t, detail.CurrentUsage.IsZero())
	})
}

// TestCustomPeriod_AccumulationAcrossDays verifies single counter without daily reset.
// CUSTOM periods use a single counter for the entire period (no daily reset like DAILY limits).
func TestCustomPeriod_AccumulationAcrossDays(t *testing.T) {
	accountID := uuid.New().String()

	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-11-27T10:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	limitID := createLimitWithCustomPeriod(t, accountID, "2026-11-27T00:00:00Z", "2026-11-29T00:00:00Z", "10000.00")
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

	// Transaction 1: 100.00
	txTime1 := testutil.TestNow().Add(-30 * time.Second)
	req1 := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("100.00"),
		Currency:             "BRL",
		TransactionTimestamp: txTime1.Format(time.RFC3339),
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
	assert.True(t, detail1.CurrentUsage.Equal(decimal.RequireFromString("100.00")),
		"First transaction: Counter should be 100, got %s", detail1.CurrentUsage)

	// Transaction 2: 200.00 - should ACCUMULATE to 300.00
	txTime2 := testutil.TestNow().Add(-20 * time.Second)
	req2 := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("200.00"),
		Currency:             "BRL",
		TransactionTimestamp: txTime2.Format(time.RFC3339),
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

	// CRITICAL: Counter should ACCUMULATE (100 from tx1 + 200 from tx2 = 300)
	// CUSTOM periods use a single counter for the entire period (no daily reset)
	assert.False(t, detail2.Skipped)
	assert.True(t, detail2.CurrentUsage.Equal(decimal.RequireFromString("300.00")),
		"Second transaction: Counter should accumulate to 300 (100+200), got %s", detail2.CurrentUsage)
}

// createLimitWithCustomPeriod creates a CUSTOM limit with a custom period for the specified account.
// startDate and endDate are in RFC3339 format (e.g., "2026-11-27T00:00:00Z").
// maxAmount is a decimal string (e.g., "10000.00").
// Returns the limit ID.
func createLimitWithCustomPeriod(t *testing.T, accountID, startDate, endDate, maxAmount string) string {
	t.Helper()

	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	reqBody := map[string]interface{}{
		"name":            "Test Limit Custom Period " + testutil.RandomSuffix(),
		"limitType":       "CUSTOM",
		"maxAmount":       maxAmount,
		"currency":        "BRL",
		"customStartDate": startDate,
		"customEndDate":   endDate,
		"scopes": []map[string]interface{}{
			{"accountId": accountID},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits", bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)

	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Failed to create limit with custom period: %s", string(respBody))

	var limit map[string]interface{}
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	limitID, ok := limit["limitId"].(string)
	require.True(t, ok, "Response should contain limitId")

	return limitID
}

// =============================================================================
// 8.3 Skip Logic Tests (3 tests)
// =============================================================================

// TestSkipLogic_NoCounterCreated verifies that skipped limits do not create
// usage counters in the database.
//
// This is critical for resource efficiency: if a transaction is outside the
// time window, we should NOT create a counter row at all.
func TestSkipLogic_NoCounterCreated(t *testing.T) {
	accountID := uuid.New().String()

	// Use MOCK_TIME at 10:00 (outside nighttime window 20:00-06:00)
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-11T10:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	// Create nighttime limit (20:00-06:00)
	limitID := createLimitWithTimeWindow(t, accountID, "20:00", "06:00", "1000.00")
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

	// Transaction at 10:00 (outside window)
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

	require.Len(t, result.LimitUsageDetails, 1)
	detail := result.LimitUsageDetails[0]

	// Verify skipped
	assert.True(t, detail.Skipped, "Limit should be skipped (outside window)")
	assert.Equal(t, "outside_time_window", detail.SkipReason)
	assert.True(t, detail.CurrentUsage.IsZero(), "CurrentUsage should be zero")

	// CRITICAL: Verify no counter was created in database
	db := testutil.SetupIntegrationDB(t)

	var count int
	err = db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) 
		FROM usage_counters 
		WHERE limit_id = $1 AND scope_key = $2
	`, limitID, "acct:"+accountID).Scan(&count)

	require.NoError(t, err)
	assert.Equal(t, 0, count, "No counter should be created when limit is skipped")

	t.Logf("✅ Skip Logic: No counter created (verified in database)")
}

// TestSkipLogic_MixedInsideOutside verifies that when multiple limits are
// evaluated, only the limits inside their windows increment counters.
func TestSkipLogic_MixedInsideOutside(t *testing.T) {
	accountID := uuid.New().String()

	// Use MOCK_TIME at 10:00
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-11T10:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	// Create 3 limits:
	// 1. Daytime (06:00-20:00) - INSIDE at 10:00
	daytimeID := createLimitWithTimeWindow(t, accountID, "06:00", "20:00", "5000.00")
	testutil.ActivateLimit(t, daytimeID)
	defer testutil.CleanupLimit(t, daytimeID)

	// 2. Nighttime (20:00-06:00) - OUTSIDE at 10:00
	nighttimeID := createLimitWithTimeWindow(t, accountID, "20:00", "06:00", "1000.00")
	testutil.ActivateLimit(t, nighttimeID)
	defer testutil.CleanupLimit(t, nighttimeID)

	// 3. Morning only (06:00-12:00) - INSIDE at 10:00
	morningID := createLimitWithTimeWindow(t, accountID, "06:00", "12:00", "2000.00")
	testutil.ActivateLimit(t, morningID)
	defer testutil.CleanupLimit(t, morningID)

	// Transaction at 10:00
	txTime := testutil.TestNow().Add(-30 * time.Second)
	req := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("300.00"),
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

	require.Len(t, result.LimitUsageDetails, 3, "Should have 3 limit details")

	// Find each limit in response
	var daytime, nighttime, morning *testutil.LimitUsageDetail
	for i := range result.LimitUsageDetails {
		switch result.LimitUsageDetails[i].LimitID {
		case daytimeID:
			daytime = &result.LimitUsageDetails[i]
		case nighttimeID:
			nighttime = &result.LimitUsageDetails[i]
		case morningID:
			morning = &result.LimitUsageDetails[i]
		}
	}

	require.NotNil(t, daytime, "Daytime limit should be in response")
	require.NotNil(t, nighttime, "Nighttime limit should be in response")
	require.NotNil(t, morning, "Morning limit should be in response")

	// Daytime: INSIDE window (06:00-20:00 includes 10:00)
	assert.False(t, daytime.Skipped, "Daytime should NOT be skipped")
	assert.True(t, daytime.CurrentUsage.Equal(decimal.RequireFromString("300.00")),
		"Daytime counter should be incremented to 300")

	// Nighttime: OUTSIDE window (20:00-06:00 excludes 10:00)
	assert.True(t, nighttime.Skipped, "Nighttime should be skipped")
	assert.Equal(t, "outside_time_window", nighttime.SkipReason)
	assert.True(t, nighttime.CurrentUsage.IsZero(),
		"Nighttime counter should be zero (not incremented)")

	// Morning: INSIDE window (06:00-12:00 includes 10:00)
	assert.False(t, morning.Skipped, "Morning should NOT be skipped")
	assert.True(t, morning.CurrentUsage.Equal(decimal.RequireFromString("300.00")),
		"Morning counter should be incremented to 300")

	t.Logf("✅ Mixed Inside/Outside: Only inside windows incremented (daytime=300, nighttime=0, morning=300)")
}

// TestSkipLogic_SkippedNeverBlocks verifies that a skipped limit never blocks
// a transaction, even if the amount would exceed the limit's maxAmount.
//
// This is critical: if a transaction is outside the time window, it should
// be allowed regardless of the limit amount.
func TestSkipLogic_SkippedNeverBlocks(t *testing.T) {
	accountID := uuid.New().String()

	// Use MOCK_TIME at 10:00 (outside nighttime window)
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-11T10:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	// Create nighttime limit with LOW maxAmount (20:00-06:00, max=100)
	limitID := createLimitWithTimeWindow(t, accountID, "20:00", "06:00", "100.00")
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

	// Transaction at 10:00 with amount MUCH HIGHER than limit (5000 > 100)
	txTime := testutil.TestNow().Add(-30 * time.Second)
	req := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("5000.00"), // 50x the limit!
		Currency:             "BRL",
		TransactionTimestamp: txTime.Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	// CRITICAL: Should return 201 Created (allowed), NOT 400/422 (blocked)
	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Transaction should be allowed even with amount > maxAmount because limit is skipped")

	var result testutil.ValidationResponse
	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	require.Len(t, result.LimitUsageDetails, 1)
	detail := result.LimitUsageDetails[0]

	// Verify skipped
	assert.True(t, detail.Skipped, "Limit should be skipped")
	assert.Equal(t, "outside_time_window", detail.SkipReason)
	assert.False(t, detail.Exceeded, "Skipped limit should NOT be marked as exceeded")
	assert.True(t, detail.CurrentUsage.IsZero(), "Counter should be zero")

	// Verify transaction was allowed (Decision should be ALLOW)
	assert.Equal(t, "ALLOW", result.Decision, "Transaction should be ALLOWED (skipped limits don't block)")

	t.Logf("✅ Skipped Never Blocks: Transaction allowed despite amount (5000) > maxAmount (100)")
}

// =============================================================================
// 8.4 EvaluatedAt Tests (2 tests)
// =============================================================================

// TestEvaluatedAt_PresentAndValid verifies that the evaluatedAt field is
// always present in the response and in valid ISO 8601 format.
func TestEvaluatedAt_PresentAndValid(t *testing.T) {
	accountID := uuid.New().String()

	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-11T15:30:45Z", // Specific time with seconds
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	// Create a simple DAILY limit
	limitID := createLimitWithTimeWindow(t, accountID, "06:00", "20:00", "5000.00")
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

	// Send transaction
	txTime := testutil.TestNow().Add(-30 * time.Second)
	req := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("100.00"),
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

	// CRITICAL: evaluatedAt must be present
	require.NotEmpty(t, result.EvaluatedAt, "evaluatedAt field must be present in response")

	// CRITICAL: evaluatedAt must be valid ISO 8601 (RFC3339)
	evaluatedAt, errParse := time.Parse(time.RFC3339, result.EvaluatedAt)
	require.NoError(t, errParse, "evaluatedAt must be valid ISO 8601 format (RFC3339)")

	// With MOCK_TIME, evaluatedAt should equal the mocked time
	expectedTime, _ := time.Parse(time.RFC3339, "2026-03-11T15:30:45Z")
	assert.Equal(t, expectedTime, evaluatedAt,
		"evaluatedAt should equal MOCK_TIME (2026-03-11T15:30:45Z)")

	t.Logf("✅ EvaluatedAt Present and Valid: %s (ISO 8601 format)", result.EvaluatedAt)
}

// TestEvaluatedAt_UsesServerNow verifies that evaluatedAt uses server time
// (not transaction timestamp) for security.
//
// This prevents timestamp injection attacks where a malicious client could
// provide a fake transaction timestamp to bypass time window checks.
func TestEvaluatedAt_UsesServerNow(t *testing.T) {
	accountID := uuid.New().String()

	// MOCK_TIME at 10:00 (server time)
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-11T10:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	limitID := createLimitWithTimeWindow(t, accountID, "06:00", "20:00", "5000.00")
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

	// Transaction with DIFFERENT timestamp (yesterday 22:00 - nighttime)
	// This simulates a client trying to manipulate time.
	// The timestamp is within the 24h validation window (MOCK_TIME 10:00 - 24h = yesterday 10:00)
	// but at a nighttime hour to test that evaluatedAt uses serverNow (10:00), not this timestamp.
	fakeTransactionTime := time.Date(2026, 3, 10, 22, 0, 0, 0, time.UTC)
	req := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("100.00"),
		Currency:             "BRL",
		TransactionTimestamp: fakeTransactionTime.Format(time.RFC3339), // yesterday 22:00 (fake)
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

	// Parse evaluatedAt
	evaluatedAt, errParse := time.Parse(time.RFC3339, result.EvaluatedAt)
	require.NoError(t, errParse)

	// CRITICAL: evaluatedAt should be serverNow (10:00), NOT transactionTimestamp (22:00)
	expectedServerTime := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, expectedServerTime, evaluatedAt,
		"evaluatedAt must use serverNow (10:00), not transactionTimestamp (22:00)")

	// Verify that limit was evaluated based on serverNow (10:00 inside 06:00-20:00)
	require.Len(t, result.LimitUsageDetails, 1)
	detail := result.LimitUsageDetails[0]
	assert.False(t, detail.Skipped, "Limit should NOT be skipped (10:00 is inside 06:00-20:00)")

	t.Logf("✅ EvaluatedAt Uses ServerNow: %s (ignored fake transactionTimestamp 22:00)",
		evaluatedAt.Format(time.RFC3339))
}

// =============================================================================
// 8.5 WEEKLY Year-Boundary Test (1 test)
// =============================================================================

// TestWeekly_YearBoundary verifies that WEEKLY limits correctly handle the
// ISO week year boundary (Dec 31 → Jan 1).
//
// ISO week calculation can cause Dec 31 to belong to Week 1 of the next year,
// while Dec 28 belongs to Week 53 of the current year.
//
// This test ensures that the periodKey is calculated correctly across this boundary.
func TestWeekly_YearBoundary(t *testing.T) {
	accountID := uuid.New().String()

	// Test Case 1: Dec 28, 2026 (Monday, Week 53 of 2026)
	t.Run("dec28_week53_2026", func(t *testing.T) {
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-12-28T10:00:00Z", // Monday, Dec 28
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		// Create WEEKLY limit
		limitID := createLimitForWeeklyTest(t, accountID, "5000.00")
		testutil.ActivateLimit(t, limitID)
		defer testutil.CleanupLimit(t, limitID)

		// Transaction on Dec 28
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

		var result testutil.ValidationResponse
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		assert.False(t, detail.Skipped)
		assert.True(t, detail.CurrentUsage.Equal(decimal.RequireFromString("1000.00")))

		// Verify periodKey in database (should be 2026-W53)
		db := testutil.SetupIntegrationDB(t)

		var periodKey string
		err = db.QueryRowContext(context.Background(), `
			SELECT period_key 
			FROM usage_counters 
			WHERE limit_id = $1 AND scope_key = $2
		`, limitID, "acct:"+accountID).Scan(&periodKey)

		require.NoError(t, err)
		assert.Equal(t, "2026-W53", periodKey, "Dec 28, 2026 should be in Week 53 of 2026")

		t.Logf("✅ Dec 28, 2026: periodKey = %s (Week 53 of 2026)", periodKey)
	})

	// Test Case 2: Jan 1, 2027 (Friday, Week 53 of 2026 according to ISO)
	t.Run("jan01_week53_2026", func(t *testing.T) {
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2027-01-01T10:00:00Z", // Friday, Jan 1, 2027
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		// Create WEEKLY limit (new server, new limit)
		limitID := createLimitForWeeklyTest(t, accountID, "5000.00")
		testutil.ActivateLimit(t, limitID)
		defer testutil.CleanupLimit(t, limitID)

		// Transaction on Jan 1, 2027
		txTime := testutil.TestNow().Add(-30 * time.Second)
		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("2000.00"),
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

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		assert.False(t, detail.Skipped)
		assert.True(t, detail.CurrentUsage.Equal(decimal.RequireFromString("2000.00")))

		// Verify periodKey in database
		db := testutil.SetupIntegrationDB(t)

		var periodKey string
		err = db.QueryRowContext(context.Background(), `
			SELECT period_key 
			FROM usage_counters 
			WHERE limit_id = $1 AND scope_key = $2
		`, limitID, "acct:"+accountID).Scan(&periodKey)

		require.NoError(t, err)

		// Jan 1, 2027 is a Friday. According to ISO 8601:
		// - If Jan 1 is Mon-Thu: Week 1 of new year
		// - If Jan 1 is Fri-Sun: Last week of previous year
		// Jan 1, 2027 is Friday, so it belongs to Week 53 of 2026
		assert.Equal(t, "2026-W53", periodKey,
			"Jan 1, 2027 (Friday) should be in Week 53 of 2026 (ISO 8601)")

		t.Logf("✅ Jan 1, 2027: periodKey = %s (Week 53 of 2026, not Week 1 of 2027)", periodKey)
	})
}

// =============================================================================
// 8.6 CUSTOM Period + Time Window Combined
// =============================================================================

// TestCustomPeriodWithTimeWindow_AC09 validates CUSTOM period combined with time window.
// Both filters are applied as AND logic -- transaction must be inside BOTH the custom date range
// AND the time window to be evaluated.
//
// Scenario: Black Friday (Nov 27-28) with business hours (09:00-17:00)
// - Inside both → evaluated, counter incremented
// - Inside period but outside window → skipped (outside_time_window)
// - Outside period → skipped (outside_custom_period)
func TestCustomPeriodWithTimeWindow_AC09(t *testing.T) {
	accountID := uuid.New().String()

	// Subtest 1: Inside both period AND window → evaluated
	t.Run("inside_period_and_window_evaluated", func(t *testing.T) {
		// MOCK_TIME: Nov 27 at 10:00 (inside period Nov27-28, inside window 09:00-17:00)
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-11-27T10:00:00Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		limitID := createLimitWithCustomPeriodAndTimeWindow(t, accountID,
			"2026-11-27T00:00:00Z", "2026-11-29T00:00:00Z", "09:00", "17:00", "10000.00")
		testutil.ActivateLimit(t, limitID)
		defer testutil.CleanupLimit(t, limitID)

		txTime := testutil.TestNow().Add(-30 * time.Second)
		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("500.00"),
			Currency:             "BRL",
			TransactionTimestamp: txTime.Format(time.RFC3339),
			Account:              &testutil.AccountContext{ID: accountID},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Transaction should be allowed: %s", string(body))

		var result testutil.ValidationResponse
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		assert.False(t, detail.Skipped, "Inside both period and window → should be evaluated")
		assert.Empty(t, detail.SkipReason)
		assert.True(t, detail.CurrentUsage.Equal(decimal.RequireFromString("500.00")),
			"Counter should be incremented to 500, got %s", detail.CurrentUsage)
	})

	// Subtest 2: Inside period but outside time window → skipped (outside_time_window)
	t.Run("inside_period_outside_window_skipped", func(t *testing.T) {
		// MOCK_TIME: Nov 27 at 22:00 (inside period Nov27-28, outside window 09:00-17:00)
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-11-27T22:00:00Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		limitID := createLimitWithCustomPeriodAndTimeWindow(t, accountID,
			"2026-11-27T00:00:00Z", "2026-11-29T00:00:00Z", "09:00", "17:00", "10000.00")
		testutil.ActivateLimit(t, limitID)
		defer testutil.CleanupLimit(t, limitID)

		txTime := testutil.TestNow().Add(-30 * time.Second)
		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("500.00"),
			Currency:             "BRL",
			TransactionTimestamp: txTime.Format(time.RFC3339),
			Account:              &testutil.AccountContext{ID: accountID},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Transaction should be allowed (skipped): %s", string(body))

		var result testutil.ValidationResponse
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		assert.True(t, detail.Skipped, "Inside period but outside window → should be skipped")
		assert.Equal(t, "outside_time_window", detail.SkipReason)
		assert.True(t, detail.CurrentUsage.IsZero(),
			"Counter should be zero (skipped), got %s", detail.CurrentUsage)
	})

	// Subtest 3: Outside custom period → skipped (outside_custom_period)
	t.Run("outside_period_skipped", func(t *testing.T) {
		// MOCK_TIME: Nov 25 at 10:00 (outside period Nov27-28, inside window 09:00-17:00)
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-11-25T10:00:00Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		limitID := createLimitWithCustomPeriodAndTimeWindow(t, accountID,
			"2026-11-27T00:00:00Z", "2026-11-29T00:00:00Z", "09:00", "17:00", "10000.00")
		testutil.ActivateLimit(t, limitID)
		defer testutil.CleanupLimit(t, limitID)

		txTime := testutil.TestNow().Add(-30 * time.Second)
		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("500.00"),
			Currency:             "BRL",
			TransactionTimestamp: txTime.Format(time.RFC3339),
			Account:              &testutil.AccountContext{ID: accountID},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Transaction should be allowed (skipped): %s", string(body))

		var result testutil.ValidationResponse
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		require.Len(t, result.LimitUsageDetails, 1)
		detail := result.LimitUsageDetails[0]

		assert.True(t, detail.Skipped, "Outside period → should be skipped")
		assert.Equal(t, "outside_custom_period", detail.SkipReason)
		assert.True(t, detail.CurrentUsage.IsZero(),
			"Counter should be zero (skipped), got %s", detail.CurrentUsage)
	})
}

// createLimitWithCustomPeriodAndTimeWindow creates a CUSTOM limit with both date range and time window.
func createLimitWithCustomPeriodAndTimeWindow(t *testing.T, accountID, startDate, endDate, startTime, endTime, maxAmount string) string {
	t.Helper()

	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	sanitizedStart := strings.ReplaceAll(startTime, ":", "")
	sanitizedEnd := strings.ReplaceAll(endTime, ":", "")

	reqBody := map[string]interface{}{
		"name":            fmt.Sprintf("Test Limit CustomTW %s-%s %s", sanitizedStart, sanitizedEnd, testutil.RandomSuffix()),
		"limitType":       "CUSTOM",
		"maxAmount":       maxAmount,
		"currency":        "BRL",
		"customStartDate": startDate,
		"customEndDate":   endDate,
		"activeTimeStart": startTime,
		"activeTimeEnd":   endTime,
		"scopes": []map[string]interface{}{
			{"accountId": accountID},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits", bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)

	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Failed to create limit with custom period and time window: %s", string(respBody))

	var limit map[string]interface{}
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	limitID, ok := limit["limitId"].(string)
	require.True(t, ok, "Response should contain limitId")

	return limitID
}

// Helper for WEEKLY tests
func createLimitForWeeklyTest(t *testing.T, accountID, maxAmount string) string {
	t.Helper()

	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	reqBody := map[string]interface{}{
		"name":      "Test Limit WEEKLY " + testutil.RandomSuffix(),
		"limitType": "WEEKLY",
		"maxAmount": maxAmount,
		"currency":  "BRL",
		"scopes": []map[string]interface{}{
			{"accountId": accountID},
		},
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
		"Failed to create WEEKLY limit: %s", string(respBody))

	var limit map[string]interface{}
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	limitID, ok := limit["limitId"].(string)
	require.True(t, ok, "Response should contain limitId")

	return limitID
}

// =============================================================================
// 8.7 PIX Compliance Pattern
// =============================================================================

// TestPIXCompliancePattern_AC11 validates the PIX Noturno compliance pattern:
// two complementary DAILY limits with time windows covering day and night.
//
// PIX Regulation (Resolution BCB 1/2020):
// - Daytime limits (06:00-20:00): Higher amount (e.g., R$5,000)
// - Nighttime limits (20:00-06:00): Lower amount for fraud protection (e.g., R$1,000)
//
// This test verifies:
// 1. Transaction at 10:00 (morning) → daytime limit evaluated, nighttime skipped
// 2. Transaction at 22:00 (night) → nighttime limit evaluated, daytime skipped
// 3. Counters are independent (different time windows = different evaluations)
// 4. Skip behavior is correct (skipped limits don't increment counters)
// 5. evaluatedAt timestamp is present and valid
func TestPIXCompliancePattern_AC11(t *testing.T) {
	accountID := uuid.New().String()

	// Test Case 1: Morning transaction at EXACTLY 10:00 (daytime evaluated, nighttime skipped)
	t.Run("morning_transaction_10h_daytime_evaluated_nighttime_skipped", func(t *testing.T) {
		// Restart server with MOCK_TIME=10:00 (morning - PIX daytime)
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-03-11T10:00:00Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		// Create PIX daytime limit (06:00-20:00)
		daytimeLimitID := createLimitWithTimeWindow(t, accountID, "06:00", "20:00", "5000.00")
		testutil.ActivateLimit(t, daytimeLimitID)
		defer testutil.CleanupLimit(t, daytimeLimitID)

		// Create PIX nighttime limit (20:00-06:00)
		nighttimeLimitID := createLimitWithTimeWindow(t, accountID, "20:00", "06:00", "1000.00")
		testutil.ActivateLimit(t, nighttimeLimitID)
		defer testutil.CleanupLimit(t, nighttimeLimitID)

		// Transaction timestamp slightly in the past (to pass validation window)
		txTime := testutil.TestNow().Add(-30 * time.Second)

		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("300.00"),
			Currency:             "BRL",
			TransactionTimestamp: txTime.Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Morning transaction should be allowed, got: %s", string(body))

		var result testutil.ValidationResponse
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		// Assert evaluatedAt is present and valid (with MOCK_TIME, it will be the mocked time)
		require.NotEmpty(t, result.EvaluatedAt, "evaluatedAt should be present")
		evaluatedAt, errParse := time.Parse(time.RFC3339, result.EvaluatedAt)
		require.NoError(t, errParse, "evaluatedAt should be valid ISO 8601")
		// With MOCK_TIME, evaluatedAt should be the mocked time (2026-03-11T10:00:00Z)
		assert.Equal(t, "2026-03-11T10:00:00Z", evaluatedAt.Format(time.RFC3339),
			"evaluatedAt should be MOCK_TIME (2026-03-11T10:00:00Z)")

		// Find daytime and nighttime limits in response
		var daytimeDetail, nighttimeDetail *testutil.LimitUsageDetail
		for i := range result.LimitUsageDetails {
			if result.LimitUsageDetails[i].LimitID == daytimeLimitID {
				daytimeDetail = &result.LimitUsageDetails[i]
			}
			if result.LimitUsageDetails[i].LimitID == nighttimeLimitID {
				nighttimeDetail = &result.LimitUsageDetails[i]
			}
		}

		require.NotNil(t, daytimeDetail, "Daytime limit should be in response")
		require.NotNil(t, nighttimeDetail, "Nighttime limit should be in response")

		// CRITICAL: 10:00 is INSIDE daytime window (06:00-20:00)
		assert.False(t, daytimeDetail.Skipped, "10:00 should be inside daytime window 06:00-20:00")
		assert.Empty(t, daytimeDetail.SkipReason)
		assert.True(t, daytimeDetail.CurrentUsage.Equal(decimal.RequireFromString("300.00")),
			"Daytime counter should be incremented to 300, got %s", daytimeDetail.CurrentUsage)
		assert.False(t, daytimeDetail.Exceeded)

		// CRITICAL: 10:00 is OUTSIDE nighttime window (20:00-06:00)
		assert.True(t, nighttimeDetail.Skipped, "10:00 should be outside nighttime window 20:00-06:00")
		assert.Equal(t, "outside_time_window", nighttimeDetail.SkipReason)
		assert.True(t, nighttimeDetail.CurrentUsage.IsZero(),
			"Nighttime counter should remain zero (skipped), got %s", nighttimeDetail.CurrentUsage)
		assert.False(t, nighttimeDetail.Exceeded)
	})

	// Test Case 2: Night transaction at EXACTLY 22:00 (nighttime evaluated, daytime skipped)
	t.Run("night_transaction_22h_nighttime_evaluated_daytime_skipped", func(t *testing.T) {
		// Restart server with MOCK_TIME=22:00 (night - PIX nighttime)
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-03-11T22:00:00Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		// Create PIX daytime limit (06:00-20:00)
		daytimeLimitID := createLimitWithTimeWindow(t, accountID, "06:00", "20:00", "5000.00")
		testutil.ActivateLimit(t, daytimeLimitID)
		defer testutil.CleanupLimit(t, daytimeLimitID)

		// Create PIX nighttime limit (20:00-06:00)
		nighttimeLimitID := createLimitWithTimeWindow(t, accountID, "20:00", "06:00", "1000.00")
		testutil.ActivateLimit(t, nighttimeLimitID)
		defer testutil.CleanupLimit(t, nighttimeLimitID)

		// Transaction timestamp slightly in the past
		txTime := testutil.TestNow().Add(-30 * time.Second)

		req := &testutil.ValidationRequest{
			RequestID:            uuid.New().String(),
			TransactionType:      "PIX",
			Amount:               decimal.RequireFromString("200.00"),
			Currency:             "BRL",
			TransactionTimestamp: txTime.Format(time.RFC3339),
			Account: &testutil.AccountContext{
				ID: accountID,
			},
		}

		resp, body := testutil.CreateValidation(t, req)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "Night transaction should be allowed, got: %s", string(body))

		var result testutil.ValidationResponse
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		// Assert evaluatedAt is present and valid (with MOCK_TIME, it will be the mocked time)
		require.NotEmpty(t, result.EvaluatedAt, "evaluatedAt should be present")
		evaluatedAt, errParse2 := time.Parse(time.RFC3339, result.EvaluatedAt)
		require.NoError(t, errParse2, "evaluatedAt should be valid ISO 8601")
		// With MOCK_TIME, evaluatedAt should be the mocked time (2026-03-11T22:00:00Z)
		assert.Equal(t, "2026-03-11T22:00:00Z", evaluatedAt.Format(time.RFC3339),
			"evaluatedAt should be MOCK_TIME (2026-03-11T22:00:00Z)")

		// Find daytime and nighttime limits in response
		var daytimeDetail, nighttimeDetail *testutil.LimitUsageDetail
		for i := range result.LimitUsageDetails {
			if result.LimitUsageDetails[i].LimitID == daytimeLimitID {
				daytimeDetail = &result.LimitUsageDetails[i]
			}
			if result.LimitUsageDetails[i].LimitID == nighttimeLimitID {
				nighttimeDetail = &result.LimitUsageDetails[i]
			}
		}

		require.NotNil(t, daytimeDetail, "Daytime limit should be in response")
		require.NotNil(t, nighttimeDetail, "Nighttime limit should be in response")

		// CRITICAL: 22:00 is OUTSIDE daytime window (06:00-20:00)
		assert.True(t, daytimeDetail.Skipped, "22:00 should be outside daytime window 06:00-20:00")
		assert.Equal(t, "outside_time_window", daytimeDetail.SkipReason)
		assert.True(t, daytimeDetail.CurrentUsage.IsZero(),
			"Daytime counter should remain zero (skipped), got %s", daytimeDetail.CurrentUsage)

		// CRITICAL: 22:00 is INSIDE nighttime window (20:00-06:00)
		assert.False(t, nighttimeDetail.Skipped, "22:00 should be inside nighttime window 20:00-06:00")
		assert.True(t, nighttimeDetail.CurrentUsage.Equal(decimal.RequireFromString("200.00")),
			"Nighttime counter should be incremented to 200, got %s", nighttimeDetail.CurrentUsage)
	})

	// Test Case 3: Verify counters are independent (multiple transactions accumulate correctly)
	t.Run("counters_are_independent", func(t *testing.T) {
		// Restart server with MOCK_TIME=10:00 (daytime - same as test 1)
		cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
			"MOCK_TIME": "2026-03-11T10:00:00Z",
		})
		require.NoError(t, err, "Failed to restart server with MOCK_TIME")
		defer func() {
			err := cleanup()
			require.NoError(t, err, "Failed to cleanup server restart")
		}()

		// Create PIX daytime limit (06:00-20:00)
		daytimeLimitID := createLimitWithTimeWindow(t, accountID, "06:00", "20:00", "5000.00")
		testutil.ActivateLimit(t, daytimeLimitID)
		defer testutil.CleanupLimit(t, daytimeLimitID)

		// Create PIX nighttime limit (20:00-06:00)
		nighttimeLimitID := createLimitWithTimeWindow(t, accountID, "20:00", "06:00", "1000.00")
		testutil.ActivateLimit(t, nighttimeLimitID)
		defer testutil.CleanupLimit(t, nighttimeLimitID)

		// Send 3 transactions to verify accumulation
		amounts := []string{"300.00", "200.00", "150.00"}
		var lastResult testutil.ValidationResponse

		for _, amount := range amounts {
			txTime := testutil.TestNow().Add(-30 * time.Second)

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

			require.Equal(t, http.StatusCreated, resp.StatusCode, "Transaction should be allowed")

			err := json.Unmarshal(body, &lastResult)
			require.NoError(t, err)
		}

		// Check last result
		result := lastResult

		// Find limits
		var daytimeDetail, nighttimeDetail *testutil.LimitUsageDetail
		for i := range result.LimitUsageDetails {
			if result.LimitUsageDetails[i].LimitID == daytimeLimitID {
				daytimeDetail = &result.LimitUsageDetails[i]
			}
			if result.LimitUsageDetails[i].LimitID == nighttimeLimitID {
				nighttimeDetail = &result.LimitUsageDetails[i]
			}
		}

		require.NotNil(t, daytimeDetail, "Daytime limit should be in response")
		require.NotNil(t, nighttimeDetail, "Nighttime limit should be in response")

		// Daytime counter should accumulate: 300 + 200 + 150 = 650
		assert.False(t, daytimeDetail.Skipped, "Daytime limit should be evaluated at 10:00")
		assert.True(t, daytimeDetail.CurrentUsage.Equal(decimal.RequireFromString("650.00")),
			"Daytime counter should accumulate to 650 (300+200+150), got %s", daytimeDetail.CurrentUsage)

		// Nighttime counter should remain zero (skipped all daytime transactions)
		assert.True(t, nighttimeDetail.Skipped, "Nighttime limit should be skipped at 10:00")
		assert.True(t, nighttimeDetail.CurrentUsage.IsZero(),
			"Nighttime counter should remain zero (always skipped during daytime), got %s", nighttimeDetail.CurrentUsage)
	})
}
