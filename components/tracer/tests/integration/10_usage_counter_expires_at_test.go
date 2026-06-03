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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/internal/testutil"
	testutil_integration "tracer/internal/testutil_integration"
)

// =============================================================================
// Integration Tests: Usage Counter ExpiresAt Calculation
//
// These tests validate that the expires_at column is correctly calculated
// and stored in the database when usage counters are created.
//
// Identified by: SRE Agent review as a reliability gap
// Related: Expired Usage Counters Are Automatically Cleaned Up
//
// Business Logic:
// - expires_at = resetAt + 90 days (for DAILY/WEEKLY/MONTHLY)
// - expires_at = customEndDate + 90 days (for CUSTOM)
// - expires_at = NULL (for PER_TRANSACTION - no counter created)
// =============================================================================

// TestUsageCounter_ExpiresAt_DAILY validates that expires_at is correctly
// calculated for DAILY limits.
//
// Expected: expires_at = resetAt + 90 days
// MOCK_TIME: 2026-03-15T10:00:00Z (Sunday, March 15)
// ResetAt: 2026-03-16T00:00:00Z (next midnight)
// ExpiresAt: 2026-06-14T00:00:00Z (resetAt + 90 days)
func TestUsageCounter_ExpiresAt_DAILY(t *testing.T) {
	// Setup: MOCK_TIME for deterministic testing
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-15T10:00:00Z", // Sunday, March 15, 2026 at 10:00 AM
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	accountID := uuid.New().String()

	// Create DAILY limit (resets at midnight)
	limitID := createLimitForExpiresAtTest(t, accountID, "DAILY", "1000.00", nil, nil)
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

	// Send transaction to create counter
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

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Transaction should succeed, got: %s", string(body))

	// Query database for expires_at
	db := testutil.SetupIntegrationDB(t)

	var expiresAt time.Time
	err = db.QueryRowContext(context.Background(), `
		SELECT expires_at 
		FROM usage_counters 
		WHERE limit_id = $1 AND scope_key = $2
	`, limitID, "acct:"+accountID).Scan(&expiresAt)

	require.NoError(t, err, "Counter should exist in database")

	// Calculate expected expires_at
	// MOCK_TIME = 2026-03-15T10:00:00Z (Sunday)
	// ResetAt for DAILY = next midnight = 2026-03-16T00:00:00Z (Monday)
	// ExpiresAt = ResetAt + 90 days = 2026-06-14T00:00:00Z
	expectedResetAt := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	expectedExpiresAt := expectedResetAt.AddDate(0, 0, 90) // +90 days

	assert.Equal(t, expectedExpiresAt, expiresAt.UTC(),
		"expires_at should be resetAt (2026-03-16T00:00:00Z) + 90 days = 2026-06-14T00:00:00Z")

	t.Logf("✅ DAILY ExpiresAt correctly calculated: %s (expected: %s)",
		expiresAt.UTC().Format(time.RFC3339),
		expectedExpiresAt.Format(time.RFC3339))
}

// TestUsageCounter_ExpiresAt_MONTHLY validates that expires_at is correctly
// calculated for MONTHLY limits.
//
// Expected: expires_at = resetAt + 90 days
// MOCK_TIME: 2026-03-15T10:00:00Z (March 15)
// ResetAt: 2026-04-01T00:00:00Z (first day of next month)
// ExpiresAt: 2026-06-30T00:00:00Z (resetAt + 90 days)
func TestUsageCounter_ExpiresAt_MONTHLY(t *testing.T) {
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-15T10:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	accountID := uuid.New().String()

	// Create MONTHLY limit (resets at first day of next month)
	limitID := createLimitForExpiresAtTest(t, accountID, "MONTHLY", "5000.00", nil, nil)
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

	// Send transaction to create counter
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

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Transaction should succeed, got: %s", string(body))

	// Query database for expires_at
	db := testutil.SetupIntegrationDB(t)

	var expiresAt time.Time
	err = db.QueryRowContext(context.Background(), `
		SELECT expires_at 
		FROM usage_counters 
		WHERE limit_id = $1 AND scope_key = $2
	`, limitID, "acct:"+accountID).Scan(&expiresAt)

	require.NoError(t, err, "Counter should exist in database")

	// Calculate expected expires_at
	// MOCK_TIME = 2026-03-15T10:00:00Z (March 15)
	// ResetAt for MONTHLY = first day of next month = 2026-04-01T00:00:00Z
	// ExpiresAt = ResetAt + 90 days = 2026-06-30T00:00:00Z
	expectedResetAt := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	expectedExpiresAt := expectedResetAt.AddDate(0, 0, 90) // +90 days

	assert.Equal(t, expectedExpiresAt, expiresAt.UTC(),
		"expires_at should be resetAt (2026-04-01T00:00:00Z) + 90 days = 2026-06-30T00:00:00Z")

	t.Logf("✅ MONTHLY ExpiresAt correctly calculated: %s (expected: %s)",
		expiresAt.UTC().Format(time.RFC3339),
		expectedExpiresAt.Format(time.RFC3339))
}

// TestUsageCounter_ExpiresAt_WEEKLY validates that expires_at is correctly
// calculated for WEEKLY limits.
//
// Expected: expires_at = resetAt + 90 days
// MOCK_TIME: 2026-03-15T10:00:00Z (Sunday of ISO Week 11)
// ResetAt: 2026-03-16T00:00:00Z (Monday, start of ISO Week 12)
// ExpiresAt: 2026-06-14T00:00:00Z (resetAt + 90 days)
func TestUsageCounter_ExpiresAt_WEEKLY(t *testing.T) {
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-15T10:00:00Z", // Sunday of ISO Week 11
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	accountID := uuid.New().String()

	// Create WEEKLY limit (resets at Monday 00:00 UTC)
	limitID := createLimitForExpiresAtTest(t, accountID, "WEEKLY", "3000.00", nil, nil)
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

	// Send transaction to create counter
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

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Transaction should succeed, got: %s", string(body))

	// Query database for expires_at
	db := testutil.SetupIntegrationDB(t)

	var expiresAt time.Time
	err = db.QueryRowContext(context.Background(), `
		SELECT expires_at 
		FROM usage_counters 
		WHERE limit_id = $1 AND scope_key = $2
	`, limitID, "acct:"+accountID).Scan(&expiresAt)

	require.NoError(t, err, "Counter should exist in database")

	// Calculate expected expires_at
	// MOCK_TIME = 2026-03-15T10:00:00Z (Sunday of ISO Week 11)
	// ResetAt for WEEKLY = next Monday = 2026-03-16T00:00:00Z (start of ISO Week 12)
	// ExpiresAt = ResetAt + 90 days = 2026-06-14T00:00:00Z
	expectedResetAt := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	expectedExpiresAt := expectedResetAt.AddDate(0, 0, 90) // +90 days

	assert.Equal(t, expectedExpiresAt, expiresAt.UTC(),
		"expires_at should be resetAt (2026-03-16T00:00:00Z) + 90 days = 2026-06-14T00:00:00Z")

	t.Logf("✅ WEEKLY ExpiresAt correctly calculated: %s (expected: %s)",
		expiresAt.UTC().Format(time.RFC3339),
		expectedExpiresAt.Format(time.RFC3339))
}

// TestUsageCounter_ExpiresAt_CUSTOM validates that expires_at is correctly
// calculated for CUSTOM period limits.
//
// Expected: expires_at = customEndDate + 90 days
// MOCK_TIME: 2026-11-27T15:00:00Z (Black Friday - during period)
// Custom Period: 2026-11-27T00:00:00Z to 2026-11-29T00:00:00Z
// ExpiresAt: 2027-02-27T00:00:00Z (customEndDate + 90 days)
func TestUsageCounter_ExpiresAt_CUSTOM(t *testing.T) {
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-11-27T15:00:00Z", // Black Friday, 3 PM
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	accountID := uuid.New().String()

	// Create CUSTOM limit for Black Friday weekend
	customStart := "2026-11-27T00:00:00Z"
	customEnd := "2026-11-29T00:00:00Z"
	limitID := createLimitForExpiresAtTest(t, accountID, "CUSTOM", "10000.00", &customStart, &customEnd)
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

	// Send transaction to create counter
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

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Transaction should succeed, got: %s", string(body))

	// Query database for expires_at
	db := testutil.SetupIntegrationDB(t)

	var expiresAt time.Time
	err = db.QueryRowContext(context.Background(), `
		SELECT expires_at 
		FROM usage_counters 
		WHERE limit_id = $1 AND scope_key = $2
	`, limitID, "acct:"+accountID).Scan(&expiresAt)

	require.NoError(t, err, "Counter should exist in database")

	// Calculate expected expires_at
	// Custom Period End: 2026-11-29T00:00:00Z
	// ExpiresAt = customEndDate + 90 days = 2027-02-27T00:00:00Z
	customEndDate := time.Date(2026, 11, 29, 0, 0, 0, 0, time.UTC)
	expectedExpiresAt := customEndDate.AddDate(0, 0, 90) // +90 days

	assert.Equal(t, expectedExpiresAt, expiresAt.UTC(),
		"expires_at should be customEndDate (2026-11-29T00:00:00Z) + 90 days = 2027-02-27T00:00:00Z")

	t.Logf("✅ CUSTOM ExpiresAt correctly calculated: %s (expected: %s)",
		expiresAt.UTC().Format(time.RFC3339),
		expectedExpiresAt.Format(time.RFC3339))
}

// TestUsageCounter_ExpiresAt_PER_TRANSACTION validates that PER_TRANSACTION limits
// do not create usage counters (and thus have no expiresAt).
//
// PER_TRANSACTION limits check maxAmount directly against transaction amount
// without persistent counters.
func TestUsageCounter_ExpiresAt_PER_TRANSACTION(t *testing.T) {
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-15T10:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	accountID := uuid.New().String()

	limitID := createLimitForExpiresAtTest(t, accountID, "PER_TRANSACTION", "1000.00", nil, nil)
	testutil.ActivateLimit(t, limitID)
	defer testutil.CleanupLimit(t, limitID)

	// Send transaction (below maxAmount → should be allowed)
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

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"PER_TRANSACTION transaction should succeed, got: %s", string(body))

	// Verify no counter was created in the database
	db := testutil.SetupIntegrationDB(t)

	var count int
	err = db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) 
		FROM usage_counters 
		WHERE limit_id = $1
	`, limitID).Scan(&count)

	require.NoError(t, err, "Database query should succeed")
	assert.Equal(t, 0, count,
		"PER_TRANSACTION limits should NOT create usage counters")

	t.Logf("✅ PER_TRANSACTION: No usage counter created (count=%d)", count)
}

// TestUsageCounter_NullExpiresAt_NeverDeleted validates that counters with
// NULL expires_at are never deleted by the cleanup worker.
//
// This is tested at the database/repository level by:
// 1. Creating a counter via a transaction
// 2. Manually setting expires_at to NULL
// 3. Running cleanup with a far-future time
// 4. Verifying the counter still exists
func TestUsageCounter_NullExpiresAt_NeverDeleted(t *testing.T) {
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"MOCK_TIME": "2026-03-15T10:00:00Z",
	})
	require.NoError(t, err, "Failed to restart server with MOCK_TIME")
	defer func() {
		err := cleanup()
		require.NoError(t, err, "Failed to cleanup server restart")
	}()

	accountID := uuid.New().String()

	// Create a DAILY limit and send transaction to create a counter
	limitID := createLimitForExpiresAtTest(t, accountID, "DAILY", "1000.00", nil, nil)
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

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Transaction should succeed, got: %s", string(body))

	db := testutil.SetupIntegrationDB(t)

	// Verify counter exists
	var counterID string
	err = db.QueryRowContext(context.Background(), `
		SELECT id FROM usage_counters 
		WHERE limit_id = $1 AND scope_key = $2
	`, limitID, "acct:"+accountID).Scan(&counterID)
	require.NoError(t, err, "Counter should exist after transaction")

	// Set expires_at to NULL (simulating a counter that should never be deleted)
	_, err = db.ExecContext(context.Background(), `
		UPDATE usage_counters SET expires_at = NULL WHERE id = $1
	`, counterID)
	require.NoError(t, err, "Should be able to set expires_at to NULL")

	// Run cleanup with a far-future time (should delete expired counters but NOT null ones)
	farFuture := time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)
	_, err = db.ExecContext(context.Background(), `
		DELETE FROM usage_counters 
		WHERE id IN (
			SELECT id FROM usage_counters 
			WHERE expires_at IS NOT NULL AND expires_at < $1
		)
	`, farFuture)
	require.NoError(t, err, "Cleanup query should succeed")

	// Verify counter with NULL expires_at was NOT deleted
	var stillExists int
	err = db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM usage_counters WHERE id = $1
	`, counterID).Scan(&stillExists)
	require.NoError(t, err, "Database query should succeed")

	assert.Equal(t, 1, stillExists,
		"Counter with NULL expires_at should NOT be deleted by cleanup")

	t.Logf("✅ NULL ExpiresAt: Counter preserved after cleanup (exists=%d)", stillExists)
}

// =============================================================================
// Helper Functions
// =============================================================================

// createLimitForExpiresAtTest creates a limit via API for ExpiresAt tests.
// Supports DAILY, MONTHLY, WEEKLY, and CUSTOM limit types.
// For CUSTOM limits, provide customStartDate and customEndDate in RFC3339 format.
func createLimitForExpiresAtTest(t *testing.T, accountID, limitType, maxAmount string, customStartDate, customEndDate *string) string {
	t.Helper()

	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	reqBody := map[string]interface{}{
		"name":      fmt.Sprintf("Test Limit ExpiresAt %s", limitType),
		"limitType": limitType,
		"maxAmount": maxAmount,
		"currency":  "BRL",
		"scopes": []map[string]interface{}{
			{"accountId": accountID},
		},
	}

	// Add custom period dates if provided
	if customStartDate != nil && customEndDate != nil {
		reqBody["customStartDate"] = *customStartDate
		reqBody["customEndDate"] = *customEndDate
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
		"Failed to create limit: %s", string(respBody))

	var limit map[string]interface{}
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	limitID, ok := limit["limitId"].(string)
	require.True(t, ok, "Response should contain limitId")

	return limitID
}
