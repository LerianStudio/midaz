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
	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/workers/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/resilience"
)

// newTestCircuitBreaker creates a circuit breaker with fast thresholds for testing.
func newTestCircuitBreaker(logger libLog.Logger) *resilience.CircuitBreaker {
	cfg := resilience.CircuitBreakerConfig{
		Name:          "test_sync",
		MaxRequests:   1,
		Interval:      0,
		Timeout:       100 * time.Millisecond, // fast timeout for tests
		FailureThresh: 3,
		FailureRatio:  0,
		MinRequests:   0,
	}

	return resilience.NewCircuitBreaker(cfg, logger)
}

func TestSyncCycle_CircuitBreakerClosed(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	mockCache := mocks.NewMockRuleSyncCache(ctrl)
	repo := mocks.NewMockRuleSyncRepository(ctrl)
	compiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := &testutil.MockClock{FixedTime: testutil.FixedTime()}
	cb := newTestCircuitBreaker(logger)

	worker, err := NewRuleSyncWorker(mockCache, repo, compiler, defaultSyncConfig(), logger, cb, clk, "")
	require.NoError(t, err)
	newRule := newSyncTestActiveRule(1)

	// Circuit closed: repo query executes normally
	mockCache.EXPECT().GetActiveRules(gomock.Any(), nil).Return(nil)
	mockCache.EXPECT().LastSyncTime(gomock.Any()).Return(testutil.FixedTime()).AnyTimes()
	mockCache.EXPECT().Size(gomock.Any()).Return(1).AnyTimes()
	mockCache.EXPECT().MarkReady(gomock.Any()).AnyTimes()
	repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return([]*model.Rule{newRule}, nil)
	compiler.EXPECT().Compile(gomock.Any(), newRule.Expression).Return("compiled", nil)
	mockCache.EXPECT().ApplyChanges(gomock.Any(), gomock.Len(1), gomock.Len(0))

	worker.runSyncCycle(context.Background())

	assert.Equal(t, gobreaker.StateClosed, cb.State())
}

func TestSyncCycle_CircuitBreakerOpen(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	mockCache := mocks.NewMockRuleSyncCache(ctrl)
	repo := mocks.NewMockRuleSyncRepository(ctrl)
	compiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := &testutil.MockClock{FixedTime: testutil.FixedTime()}
	cb := newTestCircuitBreaker(logger)

	worker, err := NewRuleSyncWorker(mockCache, repo, compiler, defaultSyncConfig(), logger, cb, clk, "")
	require.NoError(t, err)

	mockCache.EXPECT().LastSyncTime(gomock.Any()).Return(testutil.FixedTime()).AnyTimes()

	// Trip the circuit: 3 consecutive failures
	for i := 0; i < 3; i++ {
		repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return(nil, errors.New("db error"))
	}

	for i := 0; i < 3; i++ {
		worker.runSyncCycle(context.Background())
	}

	require.True(t, cb.IsOpen(), "circuit should be open after 3 failures")

	// Next cycle: circuit open -> poll skipped, NO repo call
	// (repo has no remaining expectations)
	worker.runSyncCycle(context.Background())

	// Explicit assertions: circuit remains open, no cache changes
	assert.True(t, cb.IsOpen(), "circuit should remain open after skipped cycle")
}

func TestSyncCycle_CircuitBreakerHalfOpen_Success(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	mockCache := mocks.NewMockRuleSyncCache(ctrl)
	repo := mocks.NewMockRuleSyncRepository(ctrl)
	compiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := &testutil.MockClock{FixedTime: testutil.FixedTime()}
	cb := newTestCircuitBreaker(logger)

	worker, err := NewRuleSyncWorker(mockCache, repo, compiler, defaultSyncConfig(), logger, cb, clk, "")
	require.NoError(t, err)

	mockCache.EXPECT().LastSyncTime(gomock.Any()).Return(testutil.FixedTime()).AnyTimes()

	// Trip the circuit
	for i := 0; i < 3; i++ {
		repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return(nil, errors.New("db error"))
	}

	for i := 0; i < 3; i++ {
		worker.runSyncCycle(context.Background())
	}

	require.True(t, cb.IsOpen())

	// Wait for timeout to transition to half-open (poll instead of sleep)
	require.Eventually(t, func() bool {
		return cb.State() == gobreaker.StateHalfOpen
	}, 1*time.Second, 10*time.Millisecond, "circuit should transition to half-open")

	// Half-open: probe succeeds -> transitions to closed
	// Empty result takes the len(fetched)==0 early return path (no GetActiveRules call)
	repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return([]*model.Rule{}, nil)
	mockCache.EXPECT().ApplyChanges(gomock.Any(), nil, nil) // touch for staleness
	mockCache.EXPECT().Size(gomock.Any()).Return(0).AnyTimes()
	mockCache.EXPECT().MarkReady(gomock.Any()).AnyTimes()

	worker.runSyncCycle(context.Background())

	assert.Equal(t, gobreaker.StateClosed, cb.State(), "circuit should be closed after successful probe")
}

func TestSyncCycle_CircuitBreakerHalfOpen_Failure(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	mockCache := mocks.NewMockRuleSyncCache(ctrl)
	repo := mocks.NewMockRuleSyncRepository(ctrl)
	compiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := &testutil.MockClock{FixedTime: testutil.FixedTime()}
	cb := newTestCircuitBreaker(logger)

	worker, err := NewRuleSyncWorker(mockCache, repo, compiler, defaultSyncConfig(), logger, cb, clk, "")
	require.NoError(t, err)

	mockCache.EXPECT().LastSyncTime(gomock.Any()).Return(testutil.FixedTime()).AnyTimes()

	// Trip the circuit
	for i := 0; i < 3; i++ {
		repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return(nil, errors.New("db error"))
	}

	for i := 0; i < 3; i++ {
		worker.runSyncCycle(context.Background())
	}

	require.True(t, cb.IsOpen())

	// Wait for half-open transition (poll instead of sleep)
	require.Eventually(t, func() bool {
		return cb.State() == gobreaker.StateHalfOpen
	}, 1*time.Second, 10*time.Millisecond, "circuit should transition to half-open")

	// Half-open: probe fails -> back to open
	repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return(nil, errors.New("still failing"))

	worker.runSyncCycle(context.Background())

	assert.True(t, cb.IsOpen(), "circuit should return to open after failed probe")
}

func TestSyncCycle_TripsAfterThreeFailures(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	mockCache := mocks.NewMockRuleSyncCache(ctrl)
	repo := mocks.NewMockRuleSyncRepository(ctrl)
	compiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := &testutil.MockClock{FixedTime: testutil.FixedTime()}
	cb := newTestCircuitBreaker(logger)

	worker, err := NewRuleSyncWorker(mockCache, repo, compiler, defaultSyncConfig(), logger, cb, clk, "")
	require.NoError(t, err)

	mockCache.EXPECT().LastSyncTime(gomock.Any()).Return(testutil.FixedTime()).AnyTimes()

	dbErr := errors.New("connection refused")

	// Failure 1: still closed
	repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return(nil, dbErr)
	worker.runSyncCycle(context.Background())
	assert.Equal(t, gobreaker.StateClosed, cb.State())

	// Failure 2: still closed
	repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return(nil, dbErr)
	worker.runSyncCycle(context.Background())
	assert.Equal(t, gobreaker.StateClosed, cb.State())

	// Failure 3: circuit opens
	repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return(nil, dbErr)
	worker.runSyncCycle(context.Background())
	assert.True(t, cb.IsOpen(), "circuit should open after 3rd failure")
}

func TestSyncCycle_SuccessBetweenFailuresResetsCounter(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	mockCache := mocks.NewMockRuleSyncCache(ctrl)
	repo := mocks.NewMockRuleSyncRepository(ctrl)
	compiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := &testutil.MockClock{FixedTime: testutil.FixedTime()}
	cb := newTestCircuitBreaker(logger)

	worker, err := NewRuleSyncWorker(mockCache, repo, compiler, defaultSyncConfig(), logger, cb, clk, "")
	require.NoError(t, err)

	mockCache.EXPECT().LastSyncTime(gomock.Any()).Return(testutil.FixedTime()).AnyTimes()

	dbErr := errors.New("connection refused")

	// Fail, Fail, Succeed, Fail, Fail -> circuit stays closed
	// (success resets the consecutive failure counter)

	// Failure 1
	repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return(nil, dbErr)
	worker.runSyncCycle(context.Background())

	// Failure 2
	repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return(nil, dbErr)
	worker.runSyncCycle(context.Background())

	// Success (resets counter)
	// Empty result takes the len(fetched)==0 early return path (no GetActiveRules call)
	repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return([]*model.Rule{}, nil)
	mockCache.EXPECT().ApplyChanges(gomock.Any(), nil, nil)
	mockCache.EXPECT().Size(gomock.Any()).Return(0).AnyTimes()
	mockCache.EXPECT().MarkReady(gomock.Any()).AnyTimes()
	worker.runSyncCycle(context.Background())
	assert.Equal(t, gobreaker.StateClosed, cb.State(), "success should keep circuit closed")

	// Failure 1 (after reset)
	repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return(nil, dbErr)
	worker.runSyncCycle(context.Background())

	// Failure 2 (after reset) -- still closed, need 3 consecutive
	repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return(nil, dbErr)
	worker.runSyncCycle(context.Background())
	assert.Equal(t, gobreaker.StateClosed, cb.State(),
		"circuit should remain closed -- counter was reset by success")
}

func TestSyncCycle_ContextCancellationNotCounted(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	mockCache := mocks.NewMockRuleSyncCache(ctrl)
	repo := mocks.NewMockRuleSyncRepository(ctrl)
	compiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := &testutil.MockClock{FixedTime: testutil.FixedTime()}
	cb := newTestCircuitBreaker(logger)

	worker, err := NewRuleSyncWorker(mockCache, repo, compiler, defaultSyncConfig(), logger, cb, clk, "")
	require.NoError(t, err)

	mockCache.EXPECT().LastSyncTime(gomock.Any()).Return(testutil.FixedTime()).AnyTimes()

	// Simulate 5 context cancellations + deadline exceeded (more than threshold of 3)
	for i := 0; i < 3; i++ {
		repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return(nil, context.Canceled)
	}
	for i := 0; i < 3; i++ {
		repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return(nil, context.DeadlineExceeded)
	}

	for i := 0; i < 6; i++ {
		worker.runSyncCycle(context.Background())
	}

	// Circuit should still be closed -- context cancellations and deadline exceeded are not failures
	assert.Equal(t, gobreaker.StateClosed, cb.State(),
		"context cancellations and deadline exceeded should not trip the circuit breaker")
}

// Circuit breaker state change logging tests

func TestCircuitBreaker_StateChangeLogged_ClosedToOpen(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	mockCache := mocks.NewMockRuleSyncCache(ctrl)
	repo := mocks.NewMockRuleSyncRepository(ctrl)
	compiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := &testutil.MockClock{FixedTime: testutil.FixedTime()}
	cb := newTestCircuitBreaker(logger)

	worker, err := NewRuleSyncWorker(mockCache, repo, compiler, defaultSyncConfig(), logger, cb, clk, "")
	require.NoError(t, err)

	mockCache.EXPECT().LastSyncTime(gomock.Any()).Return(testutil.FixedTime()).AnyTimes()

	// Trip circuit with 3 failures
	for i := 0; i < 3; i++ {
		repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return(nil, errors.New("db error"))
	}

	for i := 0; i < 3; i++ {
		worker.runSyncCycle(context.Background())
	}

	assert.True(t, cb.IsOpen())

	// Verify logger was called with state change info
	// The OnStateChange callback in resilience.NewCircuitBreaker logs at Info level
	// with fields: circuit_breaker.name, circuit_breaker.state_from, circuit_breaker.state_to
	found := false

	for _, call := range logger.Calls {
		if call.Level == "info" && call.Message == "Circuit breaker state changed" {
			fields := testutil.FieldsToMap(call.Fields)
			assert.Equal(t, "closed", fields["circuit_breaker.state_from"])
			assert.Equal(t, "open", fields["circuit_breaker.state_to"])

			found = true

			break
		}
	}

	assert.True(t, found, "state change from closed->open should be logged at info level with correct fields")
}

func TestCircuitBreaker_RecoveryLogged_OpenToClosed(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	mockCache := mocks.NewMockRuleSyncCache(ctrl)
	repo := mocks.NewMockRuleSyncRepository(ctrl)
	compiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := &testutil.MockClock{FixedTime: testutil.FixedTime()}
	cb := newTestCircuitBreaker(logger)

	worker, err := NewRuleSyncWorker(mockCache, repo, compiler, defaultSyncConfig(), logger, cb, clk, "")
	require.NoError(t, err)

	mockCache.EXPECT().LastSyncTime(gomock.Any()).Return(testutil.FixedTime()).AnyTimes()

	// Trip the circuit
	for i := 0; i < 3; i++ {
		repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return(nil, errors.New("db error"))
	}

	for i := 0; i < 3; i++ {
		worker.runSyncCycle(context.Background())
	}

	require.True(t, cb.IsOpen())

	// Wait for half-open transition (poll instead of sleep)
	require.Eventually(t, func() bool {
		return cb.State() == gobreaker.StateHalfOpen
	}, 1*time.Second, 10*time.Millisecond, "circuit should transition to half-open")

	// Successful probe -> recovery
	repo.EXPECT().GetRulesUpdatedSince(gomock.Any(), gomock.Any()).Return([]*model.Rule{}, nil)
	mockCache.EXPECT().ApplyChanges(gomock.Any(), nil, nil)
	mockCache.EXPECT().Size(gomock.Any()).Return(0).AnyTimes()
	mockCache.EXPECT().MarkReady(gomock.Any()).AnyTimes()

	worker.runSyncCycle(context.Background())

	assert.Equal(t, gobreaker.StateClosed, cb.State())

	// Verify recovery was logged (at least 2 state changes: open->half-open, half-open->closed)
	stateChangeCount := 0

	var transitions []string

	for _, call := range logger.Calls {
		if call.Level == "info" && call.Message == "Circuit breaker state changed" {
			fields := testutil.FieldsToMap(call.Fields)
			from, _ := fields["circuit_breaker.state_from"].(string)
			to, _ := fields["circuit_breaker.state_to"].(string)
			transitions = append(transitions, from+"->"+to)
			stateChangeCount++
		}
	}

	assert.GreaterOrEqual(t, stateChangeCount, 2,
		"recovery should log at least 2 state changes (open->half-open, half-open->closed)")
	assert.Contains(t, transitions, "open->half-open", "should log open->half-open transition")
	assert.Contains(t, transitions, "half-open->closed", "should log half-open->closed transition")
}

// Concurrent stress test — reads during circuit recovery

func TestConcurrentReads_DuringCircuitRecovery(t *testing.T) {
	t.Parallel()

	clk := &testutil.MockClock{FixedTime: testutil.FixedTime()}
	ruleCache := cache.NewRuleCache(clk)

	// Populate cache with some rules
	rules := make([]*cache.CachedRule, 50)
	for i := range rules {
		rules[i] = newSyncTestCachedRule(newSyncTestActiveRule(int64(i + 1)))
	}

	ruleCache.SetRules(context.Background(), rules)
	ruleCache.MarkReady(context.Background())

	// Simulate circuit recovery: readers active while a writer goroutine
	// periodically calls ApplyChanges (simulating sync cycles resuming)
	const numReaders = 20

	var wg sync.WaitGroup

	wg.Add(numReaders + 1) // +1 for writer goroutine

	// Writer goroutine: simulates sync cycles applying changes during recovery
	go func() {
		defer wg.Done()

		for j := 0; j < 10; j++ {
			newRule := newSyncTestCachedRule(newSyncTestActiveRule(int64(100 + j)))
			ruleCache.ApplyChanges(context.Background(), []*cache.CachedRule{newRule}, nil)
		}
	}()

	// Reader goroutines: concurrent reads during writes
	for i := 0; i < numReaders; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < 100; j++ {
				result := ruleCache.GetActiveRules(context.Background(), nil)
				assert.GreaterOrEqual(t, len(result), 50,
					"cache should have at least the original 50 rules")
			}
		}()
	}

	wg.Wait()
}
