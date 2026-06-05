// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil_integration"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMockTime_TimeWindow_Explicit_22h validates that MOCK_TIME allows testing specific hours.
// This test demonstrates that we can now test scenarios that were impossible before:
// - Transaction at exactly 22:00 (nighttime)
// - Transaction at exactly 10:00 (daytime)
// Without MOCK_TIME, we could only test relative to current time.
func TestMockTime_TimeWindow_Explicit_22h(t *testing.T) {
	// Restart server with MOCK_TIME=22:00
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-11T22:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	accountID := uuid.New().String()

	// Create nighttime limit: 20:00-06:00 (should include 22:00)
	limitID := createLimitWithTimeWindow(t, accountID, "20:00", "06:00", "1000.00")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Send transaction (serverNow = 22:00)
	req := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("500.00"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.TestNow().Add(-30 * time.Second).Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Response: %s", string(body))

	var result testutil.ValidationResponse
	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	require.Len(t, result.LimitUsageDetails, 1)
	detail := result.LimitUsageDetails[0]

	// CRITICAL ASSERTION: 22:00 is INSIDE 20:00-06:00 window
	assert.False(t, detail.Skipped, "22:00 should be inside nighttime window 20:00-06:00")
	assert.Empty(t, detail.SkipReason)
	assert.True(t, detail.CurrentUsage.Equal(decimal.RequireFromString("500.00")),
		"Counter should be incremented, got %s", detail.CurrentUsage)
}

// TestMockTime_TimeWindow_Explicit_10h validates daytime window evaluation.
func TestMockTime_TimeWindow_Explicit_10h(t *testing.T) {
	// Restart server with MOCK_TIME=10:00
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-11T10:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	accountID := uuid.New().String()

	// Create nighttime limit: 20:00-06:00 (should EXCLUDE 10:00)
	limitID := createLimitWithTimeWindow(t, accountID, "20:00", "06:00", "1000.00")
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Send transaction (serverNow = 10:00)
	req := &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("500.00"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.TestNow().Add(-30 * time.Second).Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	resp, body := testutil.CreateValidation(t, req)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Response: %s", string(body))

	var result testutil.ValidationResponse
	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	require.Len(t, result.LimitUsageDetails, 1)
	detail := result.LimitUsageDetails[0]

	// CRITICAL ASSERTION: 10:00 is OUTSIDE 20:00-06:00 window
	assert.True(t, detail.Skipped, "10:00 should be outside nighttime window 20:00-06:00")
	assert.Equal(t, "outside_time_window", detail.SkipReason)
	assert.True(t, detail.CurrentUsage.IsZero(),
		"Counter should NOT be incremented, got %s", detail.CurrentUsage)
}
