// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/workers/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

// mockClock is a test clock that returns a fixed time.
type mockClock struct {
	fixedTime  time.Time
	tickerChan chan time.Time // Optional: set to control ticker behavior in tests
}

func (m mockClock) Now() time.Time {
	return m.fixedTime
}

// NewTicker returns a controllable ticker for testing.
func (m mockClock) NewTicker(_ time.Duration) (<-chan time.Time, func()) {
	if m.tickerChan != nil {
		return m.tickerChan, func() {}
	}
	ch := make(chan time.Time)
	return ch, func() { close(ch) }
}

// setupTestTracer configures a test tracer provider for lib-commons to use.
// Returns cleanup function that must be called after test.
func setupTestTracer(t *testing.T) (*tracetest.InMemoryExporter, func()) {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)

	cleanup := func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			t.Logf("tp.Shutdown error: %v", err)
		}
	}

	return exporter, cleanup
}

func TestNewUsageCleanupWorker(t *testing.T) {
	tests := []struct {
		name        string
		config      UsageCleanupWorkerConfig
		nilRepo     bool
		nilLogger   bool
		expectError error
	}{
		{
			name: "creates worker with valid config",
			config: UsageCleanupWorkerConfig{
				CleanupInterval: 24 * time.Hour,
			},
			expectError: nil,
		},
		{
			name: "creates worker with minimum interval",
			config: UsageCleanupWorkerConfig{
				CleanupInterval: 1 * time.Minute,
			},
			expectError: nil,
		},
		{
			name: "returns error when repository is nil",
			config: UsageCleanupWorkerConfig{
				CleanupInterval: 24 * time.Hour,
			},
			nilRepo:     true,
			expectError: ErrNilRepository,
		},
		{
			name: "returns error when logger is nil",
			config: UsageCleanupWorkerConfig{
				CleanupInterval: 24 * time.Hour,
			},
			nilLogger:   true,
			expectError: ErrNilLogger,
		},
		{
			name: "returns error when cleanup interval is zero",
			config: UsageCleanupWorkerConfig{
				CleanupInterval: 0,
			},
			expectError: ErrInvalidCleanupInterval,
		},
		{
			name: "returns error when cleanup interval is negative",
			config: UsageCleanupWorkerConfig{
				CleanupInterval: -1 * time.Hour,
			},
			expectError: ErrInvalidCleanupInterval,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			_, cleanup := setupTestTracer(t)
			defer cleanup()

			var repo UsageCounterCleanupRepository
			if !tt.nilRepo {
				repo = mocks.NewMockUsageCounterCleanupRepository(ctrl)
			}

			var logger libLog.Logger = testutil.NewMockLogger()
			if tt.nilLogger {
				logger = nil
			}

			worker, err := NewUsageCleanupWorker(repo, tt.config, logger, nil, "")

			if tt.expectError != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectError)
				assert.Nil(t, worker)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, worker)
			}
		})
	}
}

func TestUsageCleanupWorker_RunWithContext_Stop(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockUsageCounterCleanupRepository(ctrl)
	logger := testutil.NewMockLogger()

	// Channel to signal when cleanup has been called
	cleanupCalled := make(chan struct{}, 1)

	// Expect cleanup to be called at least once when worker runs ( uses expires_at)
	mockRepo.EXPECT().
		DeleteExpiredCounters(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ time.Time) (int64, error) {
			select {
			case cleanupCalled <- struct{}{}:
			default:
			}
			return int64(0), nil
		}).
		MinTimes(1)

	config := UsageCleanupWorkerConfig{
		CleanupInterval: 10 * time.Millisecond, // Very short interval for fast testing
	}

	worker, err := NewUsageCleanupWorker(mockRepo, config, logger, nil, "")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	// Start worker in goroutine using RunWithContext for testability
	var wg sync.WaitGroup
	var workerErr error

	wg.Add(1)

	go func() {
		defer wg.Done()
		workerErr = worker.RunWithContext(ctx)
	}()

	// Wait for at least one cleanup call (with timeout)
	select {
	case <-cleanupCalled:
		// Cleanup was called, proceed to stop
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for cleanup to be called")
	}

	// Cancel context to stop the worker
	cancel()
	wg.Wait()

	// Worker should have stopped gracefully without error
	assert.NoError(t, workerErr)
}

func TestUsageCleanupWorker_ExecutesCleanup(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockUsageCounterCleanupRepository(ctrl)
	logger := testutil.NewMockLogger()

	deletedCount := int64(42)

	// Use a fixed time for deterministic testing
	fixedTime := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)
	testClock := mockClock{fixedTime: fixedTime}

	//  Cleanup now uses expires_at column directly, passing current time
	mockRepo.EXPECT().
		DeleteExpiredCounters(gomock.Any(), gomock.Eq(fixedTime.UTC())).
		Return(deletedCount, nil).
		MinTimes(1)

	config := UsageCleanupWorkerConfig{
		CleanupInterval: 50 * time.Millisecond,
	}

	worker, err := NewUsageCleanupWorker(mockRepo, config, logger, testClock, "")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	var workerErr error

	wg.Add(1)

	go func() {
		defer wg.Done()
		workerErr = worker.RunWithContext(ctx)
	}()

	// Let it run long enough for at least one cleanup
	time.Sleep(150 * time.Millisecond)

	cancel()
	wg.Wait()

	// Worker should have stopped gracefully without error
	assert.NoError(t, workerErr)
}

func TestUsageCleanupWorker_HandlesRepositoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockUsageCounterCleanupRepository(ctrl)
	logger := testutil.NewMockLogger()

	dbError := errors.New("database connection failed")

	// Expect cleanup calls to fail but worker should continue ( uses expires_at)
	mockRepo.EXPECT().
		DeleteExpiredCounters(gomock.Any(), gomock.Any()).
		Return(int64(0), dbError).
		MinTimes(1)

	config := UsageCleanupWorkerConfig{
		CleanupInterval: 50 * time.Millisecond,
	}

	worker, err := NewUsageCleanupWorker(mockRepo, config, logger, nil, "")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	var workerErr error

	wg.Add(1)

	go func() {
		defer wg.Done()
		workerErr = worker.RunWithContext(ctx)
	}()

	// Let it run and encounter errors - worker should NOT crash
	time.Sleep(150 * time.Millisecond)

	cancel()
	wg.Wait()

	// Worker should stop gracefully without error (cleanup errors are logged, not returned)
	assert.NoError(t, workerErr)
}

func TestUsageCleanupWorker_RunOnce(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockUsageCounterCleanupRepository(ctrl)
	logger := testutil.NewMockLogger()

	deletedCount := int64(15)

	// Use a fixed time for deterministic testing
	fixedTime := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)
	testClock := mockClock{fixedTime: fixedTime}

	//  Cleanup now passes current time to DeleteExpiredCounters
	mockRepo.EXPECT().
		DeleteExpiredCounters(gomock.Any(), fixedTime.UTC()).
		Return(deletedCount, nil).
		Times(1)

	config := UsageCleanupWorkerConfig{
		CleanupInterval: 24 * time.Hour,
	}

	worker, err := NewUsageCleanupWorker(mockRepo, config, logger, testClock, "")
	require.NoError(t, err)

	ctx := context.Background()
	count, err := worker.RunOnce(ctx)

	require.NoError(t, err)
	assert.Equal(t, deletedCount, count)
}

func TestUsageCleanupWorker_RunOnce_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockUsageCounterCleanupRepository(ctrl)
	logger := testutil.NewMockLogger()

	dbError := errors.New("database unavailable")

	//  Uses DeleteExpiredCounters
	mockRepo.EXPECT().
		DeleteExpiredCounters(gomock.Any(), gomock.Any()).
		Return(int64(0), dbError).
		Times(1)

	config := UsageCleanupWorkerConfig{
		CleanupInterval: 24 * time.Hour,
	}

	worker, err := NewUsageCleanupWorker(mockRepo, config, logger, nil, "")
	require.NoError(t, err)

	ctx := context.Background()
	count, err := worker.RunOnce(ctx)

	require.Error(t, err)
	assert.Equal(t, int64(0), count)
	assert.Contains(t, err.Error(), "failed to delete expired counters")
}

func TestUsageCleanupWorker_DefaultConfig(t *testing.T) {
	config := DefaultUsageCleanupWorkerConfig()

	assert.Equal(t, 24*time.Hour, config.CleanupInterval)

}

func TestUsageCleanupWorker_NilClockUsesRealClock(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockUsageCounterCleanupRepository(ctrl)
	logger := testutil.NewMockLogger()

	//  When nil clock is passed, worker should use RealClock
	// Cleanup now passes current time to DeleteExpiredCounters
	mockRepo.EXPECT().
		DeleteExpiredCounters(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, now time.Time) (int64, error) {
			// Verify the time is close to current time (within 1 second)
			assert.WithinDuration(t, time.Now().UTC(), now, 1*time.Second)
			return int64(5), nil
		}).
		Times(1)

	config := UsageCleanupWorkerConfig{
		CleanupInterval: 24 * time.Hour,
	}

	// Pass nil clock - should use RealClock
	worker, err := NewUsageCleanupWorker(mockRepo, config, logger, nil, "")
	require.NoError(t, err)

	ctx := context.Background()
	count, err := worker.RunOnce(ctx)

	require.NoError(t, err)
	assert.Equal(t, int64(5), count)
}
