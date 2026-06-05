// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// createTestLimit creates a test limit in the database and returns its ID.
// This is required because usage_counters has a foreign key constraint to limits.
func createTestLimit(t *testing.T, db *sql.DB, base int64) uuid.UUID {
	t.Helper()

	limitID := testutil.MustDeterministicUUID(base)

	_, err := db.Exec(`
		INSERT INTO limits (id, name, limit_type, max_amount, currency, scopes, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, limitID, "Test Limit "+limitID.String()[:8], "DAILY", decimal.NewFromInt(10000), "USD", "[]", "ACTIVE")
	require.NoError(t, err, "Failed to create test limit")

	return limitID
}

// cleanupTestLimit removes a test limit from the database.
// Due to ON DELETE CASCADE, this also removes associated usage_counters.
func cleanupTestLimit(t *testing.T, db *sql.DB, limitID uuid.UUID) {
	t.Helper()

	_, err := db.Exec("DELETE FROM limits WHERE id = $1", limitID)
	if err != nil {
		t.Logf("Cleanup: failed to delete limit %s: %v", limitID, err)
	}
}

// TestUsageCounterRepository_IncrementAtomic_Concurrent_Integration tests that
// concurrent IncrementAtomic calls do not lose updates when using SELECT FOR UPDATE.
//
// This is a real integration test that:
// 1. Uses a real PostgreSQL database (from docker-compose)
// 2. Creates a usage counter with currentUsage=0
// 3. Runs 10 goroutines in parallel, each calling IncrementAtomic with amount=10
// 4. Verifies final currentUsage equals exactly 100 (10 * 10)
//
// If SELECT FOR UPDATE is working correctly, no updates should be lost due to race conditions.
func TestUsageCounterRepository_IncrementAtomic_Concurrent_Integration(t *testing.T) {
	// Setup test tracing (required for lib-commons context extraction)
	testutil.SetupTestTracing(t)

	// Connect to real PostgreSQL (cleanup registered via t.Cleanup)
	db := testutil.SetupIntegrationDB(t)

	// Create repository with real database connection
	adapter := &testutil.IntegrationDBAdapter{DB: db}
	repo := NewUsageCounterRepositoryWithConnection(adapter)

	// Test parameters
	const numGoroutines = 10
	incrementAmount := decimal.RequireFromString("10")
	expectedFinalUsage := decimal.RequireFromString("100") // numGoroutines(10) * incrementAmount(10)

	// Create a test limit first (required for FK constraint)
	limitID := createTestLimit(t, db, 9001)
	scopeKey := "test:concurrent-" + testutil.MustDeterministicUUID(9101).String()[:8]
	periodKey := "2025-01"

	// Cleanup: remove test limit (cascades to counters)
	t.Cleanup(func() {
		cleanupTestLimit(t, db, limitID)
	})

	// Create initial counter with currentUsage=0
	ctx := context.Background()
	counter, err := repo.GetOrCreateForUpdate(ctx, limitID, scopeKey, periodKey)
	require.NoError(t, err, "Failed to create initial counter")
	require.NotNil(t, counter)
	require.True(t, decimal.Zero.Equal(counter.CurrentUsage), "Initial counter should have 0 usage")

	counterID := counter.ID
	t.Logf("Created counter %s with initial usage 0", counterID)

	// Run concurrent increments
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := range numGoroutines {
		wg.Add(1)

		go func(goroutineID int) {
			defer wg.Done()

			// Each goroutine gets its own context
			goroutineCtx := context.Background()

			err := repo.IncrementAtomic(goroutineCtx, counterID, incrementAmount)
			if err != nil {
				errors <- fmt.Errorf("goroutine %d: %w", goroutineID, err)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errors)

	// Check for errors from goroutines
	var goroutineErrors []error
	for err := range errors {
		goroutineErrors = append(goroutineErrors, err)
	}

	require.Empty(t, goroutineErrors, "Goroutines should not have errors: %v", goroutineErrors)

	// Verify final usage equals expected value (no lost updates)
	var finalUsage decimal.Decimal
	err = db.QueryRowContext(ctx, "SELECT current_usage FROM usage_counters WHERE id = $1", counterID).Scan(&finalUsage)
	require.NoError(t, err, "Failed to query final usage")

	assert.True(t, expectedFinalUsage.Equal(finalUsage),
		"Final usage should be exactly %s (no lost updates), but got %s",
		expectedFinalUsage.String(), finalUsage.String())

	t.Logf("SUCCESS: %d goroutines each incremented by %s, final usage = %s (expected %s)",
		numGoroutines, incrementAmount.String(), finalUsage.String(), expectedFinalUsage.String())
}

// TestUsageCounterRepository_GetOrCreateForUpdate_Concurrent_Integration tests that
// concurrent GetOrCreateForUpdate calls correctly handle the race condition where
// multiple goroutines try to create the same counter simultaneously.
//
// This verifies the retry logic in GetOrCreateForUpdate handles concurrent inserts.
func TestUsageCounterRepository_GetOrCreateForUpdate_Concurrent_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)

	adapter := &testutil.IntegrationDBAdapter{DB: db}
	repo := NewUsageCounterRepositoryWithConnection(adapter)

	const numGoroutines = 10

	// Create a test limit first (required for FK constraint)
	limitID := createTestLimit(t, db, 9002)
	scopeKey := "test:concurrent-create-" + testutil.MustDeterministicUUID(9102).String()[:8]
	periodKey := "2025-01"

	// Cleanup: remove test limit (cascades to counters)
	t.Cleanup(func() {
		cleanupTestLimit(t, db, limitID)
	})

	// Run concurrent GetOrCreateForUpdate calls
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)
	counters := make(chan *model.UsageCounter, numGoroutines)

	for i := range numGoroutines {
		wg.Add(1)

		go func(goroutineID int) {
			defer wg.Done()

			ctx := context.Background()

			counter, err := repo.GetOrCreateForUpdate(ctx, limitID, scopeKey, periodKey)
			if err != nil {
				errors <- fmt.Errorf("goroutine %d: %w", goroutineID, err)
				return
			}

			counters <- counter
		}(i)
	}

	wg.Wait()
	close(errors)
	close(counters)

	// Check for errors
	var goroutineErrors []error
	for err := range errors {
		goroutineErrors = append(goroutineErrors, err)
	}

	require.Empty(t, goroutineErrors, "Goroutines should not have errors: %v", goroutineErrors)

	// All goroutines should return the same counter (same ID)
	var collectedCounters []*model.UsageCounter
	for counter := range counters {
		collectedCounters = append(collectedCounters, counter)
	}

	require.Len(t, collectedCounters, numGoroutines, "Should have %d counters", numGoroutines)

	// Verify all counters have the same ID (only one was created)
	firstID := collectedCounters[0].ID
	for i, counter := range collectedCounters {
		assert.Equal(t, firstID, counter.ID,
			"All goroutines should return the same counter ID, but goroutine %d got different ID", i)
		assert.Equal(t, limitID, counter.LimitID)
		assert.Equal(t, scopeKey, counter.ScopeKey)
		assert.Equal(t, periodKey, counter.PeriodKey)
	}

	// Verify only one counter exists in the database
	var count int
	err := db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM usage_counters WHERE limit_id = $1 AND scope_key = $2 AND period_key = $3",
		limitID, scopeKey, periodKey).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Only one counter should exist in the database")

	t.Logf("SUCCESS: %d goroutines all returned the same counter ID %s", numGoroutines, firstID)
}

// TestUsageCounterRepository_UpsertAndIncrementAtomic_Concurrent_Integration tests that
// concurrent UpsertAndIncrementAtomic calls correctly enforce maxAmount limits under race conditions.
//
// Scenario 1: 20 goroutines concurrently increment a counter with maxAmount enforcement.
// - Each goroutine calls UpsertAndIncrementAtomic with amount=100 and maxAmount=1000
// - With barrier synchronization to maximize contention
// - Expected: exactly 10 succeed (10 * 100 = 1000), exactly 10 fail with ErrUsageCounterExceedsLimit
// - Final current_usage must be exactly 1000
//
// This validates that the atomic INSERT ... ON CONFLICT DO UPDATE ... WHERE guard correctly
// prevents race conditions when enforcing limit maximums.
func TestUsageCounterRepository_UpsertAndIncrementAtomic_Concurrent_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)

	adapter := &testutil.IntegrationDBAdapter{DB: db}
	repo := NewUsageCounterRepositoryWithConnection(adapter)

	// Test parameters
	const numGoroutines = 20
	amount := decimal.RequireFromString("100")
	maxAmount := decimal.RequireFromString("1000")
	expectedSuccesses := 10 // maxAmount(1000) / amount(100) = 10
	expectedFailures := numGoroutines - expectedSuccesses
	expectedFinalUsage := decimal.RequireFromString("1000")

	// Create a test limit (required for FK constraint) using seed 90100
	limitID := createTestLimit(t, db, 90100)
	scopeKey := testutil.MustDeterministicUUID(90101).String()
	periodKey := "2025-01"

	// Cleanup: remove test limit (cascades to counters)
	t.Cleanup(func() {
		cleanupTestLimit(t, db, limitID)
	})

	// Barrier synchronization: all goroutines wait until ready, then start simultaneously
	var ready sync.WaitGroup
	var start sync.WaitGroup

	ready.Add(numGoroutines)
	start.Add(1)

	// Channel to collect results (nil = success, non-nil = error)
	results := make(chan error, numGoroutines)

	// Launch concurrent goroutines
	for i := range numGoroutines {
		go func(goroutineID int) {
			// Signal ready and wait for start barrier
			ready.Done()
			start.Wait()

			ctx := context.Background()

			_, err := repo.UpsertAndIncrementAtomic(ctx, db, limitID, scopeKey, periodKey, amount, maxAmount, nil)
			if err != nil {
				results <- fmt.Errorf("goroutine %d: %w", goroutineID, err)
			} else {
				results <- nil
			}
		}(i)
	}

	// Wait for all goroutines to be ready
	ready.Wait()

	// Release all goroutines simultaneously to maximize contention
	start.Done()

	// Collect all results
	var successCount, failCount int
	var unexpectedErrors []error

	for i := 0; i < numGoroutines; i++ {
		err := <-results

		if err == nil {
			successCount++
		} else if errors.Is(err, constant.ErrUsageCounterExceedsLimit) {
			failCount++
		} else {
			unexpectedErrors = append(unexpectedErrors, fmt.Errorf("goroutine: unexpected error: %w", err))
		}
	}

	close(results)

	// Assert no unexpected errors
	require.Empty(t, unexpectedErrors, "Should have no unexpected errors: %v", unexpectedErrors)

	// Assert exactly 10 succeed
	assert.Equal(t, expectedSuccesses, successCount,
		"Expected exactly %d successes, got %d", expectedSuccesses, successCount)

	// Assert exactly 10 fail with ErrUsageCounterExceedsLimit
	assert.Equal(t, expectedFailures, failCount,
		"Expected exactly %d failures with ErrUsageCounterExceedsLimit, got %d", expectedFailures, failCount)

	// Verify final usage in database equals exactly 1000
	ctx := context.Background()

	var finalUsage decimal.Decimal

	err := db.QueryRowContext(ctx,
		"SELECT current_usage FROM usage_counters WHERE limit_id = $1 AND scope_key = $2 AND period_key = $3",
		limitID, scopeKey, periodKey).Scan(&finalUsage)
	require.NoError(t, err, "Failed to query final usage")

	assert.True(t, expectedFinalUsage.Equal(finalUsage),
		"Final usage should be exactly %s, but got %s",
		expectedFinalUsage.String(), finalUsage.String())

	t.Logf("SUCCESS: %d goroutines with amount=%s and maxAmount=%s: %d succeeded, %d failed (ErrUsageCounterExceedsLimit), final usage = %s",
		numGoroutines, amount.String(), maxAmount.String(), successCount, failCount, finalUsage.String())
}

// TestUsageCounterRepository_UpsertAndIncrementAtomic_InsertRace_Integration tests that
// concurrent UpsertAndIncrementAtomic calls correctly handle the INSERT-INSERT race condition
// where multiple goroutines try to create a counter that does not exist yet.
//
// Scenario 2: 10 goroutines each call UpsertAndIncrementAtomic with amount=100 and maxAmount=500
// where NO counter exists yet (INSERT-INSERT race).
//
// Expected behavior:
// - Exactly 5 succeed (5 * 100 = 500)
// - Exactly 5 fail with ErrUsageCounterExceedsLimit
// - Final current_usage = 500
// - Only 1 counter row exists (no duplicates from INSERT race)
//
// This validates that the atomic INSERT ... ON CONFLICT DO UPDATE ... WHERE guard correctly
// handles the race condition when multiple transactions try to INSERT the same counter.
func TestUsageCounterRepository_UpsertAndIncrementAtomic_InsertRace_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)

	adapter := &testutil.IntegrationDBAdapter{DB: db}
	repo := NewUsageCounterRepositoryWithConnection(adapter)

	// Test parameters
	const numGoroutines = 10
	amount := decimal.RequireFromString("100")
	maxAmount := decimal.RequireFromString("500")
	expectedSuccesses := 5 // maxAmount(500) / amount(100) = 5
	expectedFailures := numGoroutines - expectedSuccesses
	expectedFinalUsage := decimal.RequireFromString("500")

	// Create a test limit (required for FK constraint) using seed 90110
	limitID := createTestLimit(t, db, 90110)
	scopeKey := testutil.MustDeterministicUUID(90111).String()
	periodKey := "2025-02"

	// Cleanup: remove test limit (cascades to counters)
	t.Cleanup(func() {
		cleanupTestLimit(t, db, limitID)
	})

	// Verify no existing counter exists before the test
	ctx := context.Background()

	var existingCount int

	err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM usage_counters WHERE limit_id = $1 AND scope_key = $2 AND period_key = $3",
		limitID, scopeKey, periodKey).Scan(&existingCount)
	require.NoError(t, err, "Failed to check for existing counter")
	require.Equal(t, 0, existingCount, "No counter should exist before the test (INSERT-INSERT race scenario)")

	// Barrier synchronization: all goroutines wait until ready, then start simultaneously
	var ready sync.WaitGroup
	var start sync.WaitGroup

	ready.Add(numGoroutines)
	start.Add(1)

	// Channel to collect results (nil = success, non-nil = error)
	results := make(chan error, numGoroutines)

	// Launch concurrent goroutines
	for i := range numGoroutines {
		go func(goroutineID int) {
			// Signal ready and wait for start barrier
			ready.Done()
			start.Wait()

			goroutineCtx := context.Background()

			_, err := repo.UpsertAndIncrementAtomic(goroutineCtx, db, limitID, scopeKey, periodKey, amount, maxAmount, nil)
			if err != nil {
				results <- fmt.Errorf("goroutine %d: %w", goroutineID, err)
			} else {
				results <- nil
			}
		}(i)
	}

	// Wait for all goroutines to be ready
	ready.Wait()

	// Release all goroutines simultaneously to maximize contention
	start.Done()

	// Collect all results
	var successCount, failCount int
	var unexpectedErrors []error

	for i := 0; i < numGoroutines; i++ {
		err := <-results

		if err == nil {
			successCount++
		} else if errors.Is(err, constant.ErrUsageCounterExceedsLimit) {
			failCount++
		} else {
			unexpectedErrors = append(unexpectedErrors, fmt.Errorf("goroutine: unexpected error: %w", err))
		}
	}

	close(results)

	// Assert no unexpected errors
	require.Empty(t, unexpectedErrors, "Should have no unexpected errors: %v", unexpectedErrors)

	// Assert exactly 5 succeed
	assert.Equal(t, expectedSuccesses, successCount,
		"Expected exactly %d successes, got %d", expectedSuccesses, successCount)

	// Assert exactly 5 fail with ErrUsageCounterExceedsLimit
	assert.Equal(t, expectedFailures, failCount,
		"Expected exactly %d failures with ErrUsageCounterExceedsLimit, got %d", expectedFailures, failCount)

	// Verify final usage in database equals exactly 500
	var finalUsage decimal.Decimal

	err = db.QueryRowContext(ctx,
		"SELECT current_usage FROM usage_counters WHERE limit_id = $1 AND scope_key = $2 AND period_key = $3",
		limitID, scopeKey, periodKey).Scan(&finalUsage)
	require.NoError(t, err, "Failed to query final usage")

	assert.True(t, expectedFinalUsage.Equal(finalUsage),
		"Final usage should be exactly %s, but got %s",
		expectedFinalUsage.String(), finalUsage.String())

	// Verify only 1 counter row exists (no duplicates from INSERT race)
	var rowCount int

	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM usage_counters WHERE limit_id = $1 AND scope_key = $2 AND period_key = $3",
		limitID, scopeKey, periodKey).Scan(&rowCount)
	require.NoError(t, err, "Failed to count counter rows")

	assert.Equal(t, 1, rowCount,
		"Expected exactly 1 counter row (no duplicates from INSERT race), got %d", rowCount)

	t.Logf("SUCCESS: INSERT-INSERT race test with %d goroutines, amount=%s, maxAmount=%s: %d succeeded, %d failed (ErrUsageCounterExceedsLimit), final usage = %s, row count = %d",
		numGoroutines, amount.String(), maxAmount.String(), successCount, failCount, finalUsage.String(), rowCount)
}

// TestUsageCounterRepository_UpsertAndIncrementAtomic_Boundary_Integration tests that
// UpsertAndIncrementAtomic correctly enforces maxAmount limits when a counter already
// has pre-seeded usage that puts it near the boundary.
//
// Scenario 3: maxAmount=500, pre-seed current_usage=200, then 10 goroutines each call
// UpsertAndIncrementAtomic with amount=100.
//
// Expected behavior:
// - Exactly 3 succeed (200 + 3*100 = 500)
// - Exactly 7 fail with ErrUsageCounterExceedsLimit
// - Final current_usage = 500
//
// This validates boundary condition handling where the counter starts with existing usage.
func TestUsageCounterRepository_UpsertAndIncrementAtomic_Boundary_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)

	adapter := &testutil.IntegrationDBAdapter{DB: db}
	repo := NewUsageCounterRepositoryWithConnection(adapter)

	// Test parameters
	const numGoroutines = 10
	amount := decimal.RequireFromString("100")
	maxAmount := decimal.RequireFromString("500")
	preSeededUsage := decimal.RequireFromString("200")
	expectedSuccesses := 3 // (maxAmount(500) - preSeededUsage(200)) / amount(100) = 3
	expectedFailures := numGoroutines - expectedSuccesses
	expectedFinalUsage := decimal.RequireFromString("500")

	// Create a test limit (required for FK constraint) using seed 90120
	limitID := createTestLimit(t, db, 90120)
	scopeKey := testutil.MustDeterministicUUID(90121).String()
	periodKey := "2025-03"
	counterID := testutil.MustDeterministicUUID(90122)

	// Cleanup: remove test limit (cascades to counters)
	t.Cleanup(func() {
		cleanupTestLimit(t, db, limitID)
	})

	ctx := context.Background()

	// Pre-seed the counter with current_usage=200 via direct SQL INSERT
	_, err := db.ExecContext(ctx, `
		INSERT INTO usage_counters (id, limit_id, scope_key, period_key, current_usage, last_updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`, counterID, limitID, scopeKey, periodKey, preSeededUsage)
	require.NoError(t, err, "Failed to pre-seed counter")

	// Verify pre-seeded value via SELECT
	var verifyUsage decimal.Decimal

	err = db.QueryRowContext(ctx,
		"SELECT current_usage FROM usage_counters WHERE id = $1",
		counterID).Scan(&verifyUsage)
	require.NoError(t, err, "Failed to verify pre-seeded counter")
	require.True(t, preSeededUsage.Equal(verifyUsage),
		"Pre-seeded usage should be %s, got %s", preSeededUsage.String(), verifyUsage.String())

	t.Logf("Pre-seeded counter %s with current_usage=%s", counterID, preSeededUsage.String())

	// Barrier synchronization: all goroutines wait until ready, then start simultaneously
	var ready sync.WaitGroup
	var start sync.WaitGroup

	ready.Add(numGoroutines)
	start.Add(1)

	// Channel to collect results (nil = success, non-nil = error)
	results := make(chan error, numGoroutines)

	// Launch concurrent goroutines
	for i := range numGoroutines {
		go func(goroutineID int) {
			// Signal ready and wait for start barrier
			ready.Done()
			start.Wait()

			goroutineCtx := context.Background()

			_, err := repo.UpsertAndIncrementAtomic(goroutineCtx, db, limitID, scopeKey, periodKey, amount, maxAmount, nil)
			if err != nil {
				results <- fmt.Errorf("goroutine %d: %w", goroutineID, err)
			} else {
				results <- nil
			}
		}(i)
	}

	// Wait for all goroutines to be ready
	ready.Wait()

	// Release all goroutines simultaneously to maximize contention
	start.Done()

	// Collect all results
	var successCount, failCount int
	var unexpectedErrors []error

	for i := 0; i < numGoroutines; i++ {
		err := <-results

		if err == nil {
			successCount++
		} else if errors.Is(err, constant.ErrUsageCounterExceedsLimit) {
			failCount++
		} else {
			unexpectedErrors = append(unexpectedErrors, fmt.Errorf("goroutine: unexpected error: %w", err))
		}
	}

	close(results)

	// Assert no unexpected errors
	require.Empty(t, unexpectedErrors, "Should have no unexpected errors: %v", unexpectedErrors)

	// Assert exactly 3 succeed (200 + 3*100 = 500)
	assert.Equal(t, expectedSuccesses, successCount,
		"Expected exactly %d successes (pre-seeded %s + %d*%s = %s), got %d",
		expectedSuccesses, preSeededUsage.String(), expectedSuccesses, amount.String(), expectedFinalUsage.String(), successCount)

	// Assert exactly 7 fail with ErrUsageCounterExceedsLimit
	assert.Equal(t, expectedFailures, failCount,
		"Expected exactly %d failures with ErrUsageCounterExceedsLimit, got %d", expectedFailures, failCount)

	// Verify final usage in database equals exactly 500 via direct SQL
	var finalUsage decimal.Decimal

	err = db.QueryRowContext(ctx,
		"SELECT current_usage FROM usage_counters WHERE id = $1",
		counterID).Scan(&finalUsage)
	require.NoError(t, err, "Failed to query final usage")

	assert.True(t, expectedFinalUsage.Equal(finalUsage),
		"Final usage should be exactly %s, but got %s",
		expectedFinalUsage.String(), finalUsage.String())

	t.Logf("SUCCESS: Boundary test with pre-seeded %s, %d goroutines with amount=%s and maxAmount=%s: %d succeeded, %d failed (ErrUsageCounterExceedsLimit), final usage = %s",
		preSeededUsage.String(), numGoroutines, amount.String(), maxAmount.String(), successCount, failCount, finalUsage.String())
}
