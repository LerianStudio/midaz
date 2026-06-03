// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/reporter/pkg/constant"
	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreakerManager_New(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	cbm := NewCircuitBreakerManager(logger)

	assert.NotNil(t, cbm)
	assert.NotNil(t, cbm.breakers)
	assert.Equal(t, 0, len(cbm.breakers))
}

func TestCircuitBreakerManager_GetOrCreate(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	tests := []struct {
		name           string
		datasourceName string
	}{
		{
			name:           "Create new circuit breaker",
			datasourceName: "test_db_1",
		},
		{
			name:           "Get existing circuit breaker",
			datasourceName: "test_db_1",
		},
		{
			name:           "Create another circuit breaker",
			datasourceName: "test_db_2",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Note: Cannot use t.Parallel() because subtests share cbm state
			// and the assertion after the loop depends on subtests completing

			breaker := cbm.GetOrCreate(tt.datasourceName)
			assert.NotNil(t, breaker)
		})
	}

	// Verify both breakers exist
	assert.Equal(t, 2, len(cbm.breakers))
}

func TestCircuitBreakerManager_Execute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		fn             func() (any, error)
		expectedResult any
		expectError    bool
		errContains    string
	}{
		{
			name: "Success - returns result without error",
			fn: func() (any, error) {
				return "success", nil
			},
			expectedResult: "success",
			expectError:    false,
		},
		{
			name: "Error - returns nil result with error",
			fn: func() (any, error) {
				return nil, errors.New("test error")
			},
			expectedResult: nil,
			expectError:    true,
			errContains:    "test error",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
			cbm := NewCircuitBreakerManager(logger)

			result, err := cbm.Execute("test_db", tt.fn)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, result)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestCircuitBreakerManager_GetState(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	tests := []struct {
		name           string
		datasourceName string
		setup          func()
		expectedState  string
	}{
		{
			name:           "Not initialized state",
			datasourceName: "unknown_db",
			setup:          func() {},
			expectedState:  "not_initialized",
		},
		{
			name:           "Closed state (healthy)",
			datasourceName: "healthy_db",
			setup: func() {
				cbm.GetOrCreate("healthy_db")
			},
			expectedState: constant.CircuitBreakerStateClosed,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.setup()
			state := cbm.GetState(tt.datasourceName)
			assert.Equal(t, tt.expectedState, state)
		})
	}
}

func TestCircuitBreakerManager_GetCounts(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Test non-existent breaker
	counts := cbm.GetCounts("non_existent")
	assert.Equal(t, uint32(0), counts.Requests)

	// Create breaker and execute some requests
	cbm.GetOrCreate("test_db")
	_, _ = cbm.Execute("test_db", func() (any, error) {
		return "ok", nil
	})

	counts = cbm.GetCounts("test_db")
	assert.Equal(t, uint32(1), counts.Requests)
	assert.Equal(t, uint32(1), counts.TotalSuccesses)
}

func TestCircuitBreakerManager_Reset(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Create and use breaker
	cbm.GetOrCreate("test_db")
	_, _ = cbm.Execute("test_db", func() (any, error) {
		return nil, errors.New("error")
	})

	counts := cbm.GetCounts("test_db")
	assert.Equal(t, uint32(1), counts.TotalFailures)

	// Reset breaker
	cbm.Reset("test_db")

	// After reset, counts should be zero
	counts = cbm.GetCounts("test_db")
	assert.Equal(t, uint32(0), counts.Requests)
}

func TestCircuitBreakerManager_Reset_NonExistent(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Should not panic when resetting non-existent breaker
	assert.NotPanics(t, func() {
		cbm.Reset("non_existent")
	})
}

func TestCircuitBreakerManager_IsHealthy(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	tests := []struct {
		name           string
		datasourceName string
		setup          func()
		expected       bool
	}{
		{
			name:           "Not initialized - returns true",
			datasourceName: "unknown_db",
			setup:          func() {},
			expected:       true,
		},
		{
			name:           "Closed state - returns true",
			datasourceName: "healthy_db",
			setup: func() {
				cbm.GetOrCreate("healthy_db")
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.setup()
			result := cbm.IsHealthy(tt.datasourceName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCircuitBreakerManager_ShouldAllowRetry(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	tests := []struct {
		name           string
		datasourceName string
		setup          func()
		expected       bool
	}{
		{
			name:           "Not initialized - allows retry",
			datasourceName: "unknown_db",
			setup:          func() {},
			expected:       true,
		},
		{
			name:           "Closed state - allows retry",
			datasourceName: "healthy_db",
			setup: func() {
				cbm.GetOrCreate("healthy_db")
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.setup()
			result := cbm.ShouldAllowRetry(tt.datasourceName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCircuitBreakerManager_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Test concurrent access doesn't cause race conditions
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			cbm.GetOrCreate("concurrent_db")
			_, _ = cbm.Execute("concurrent_db", func() (any, error) {
				return id, nil
			})
			cbm.GetState("concurrent_db")
			cbm.GetCounts("concurrent_db")
			cbm.IsHealthy("concurrent_db")
			cbm.ShouldAllowRetry("concurrent_db")
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify breaker was created
	assert.Equal(t, 1, len(cbm.breakers))
}

// tripCircuitBreaker sends enough consecutive failures to open the circuit breaker.
func tripCircuitBreaker(cbm *CircuitBreakerManager, datasourceName string) {
	cb := cbm.GetOrCreate(datasourceName)
	for i := 0; i < int(constant.CircuitBreakerThreshold)+5; i++ {
		_, _ = cb.Execute(func() (any, error) {
			return nil, errors.New("deliberate failure")
		})
	}
}

func TestCircuitBreakerManager_Execute_OpenState(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Trip the circuit breaker to open state
	tripCircuitBreaker(cbm, "open_db")

	// Verify state is open
	state := cbm.GetState("open_db")
	assert.Equal(t, constant.CircuitBreakerStateOpen, state)

	// Execute through the manager should wrap the error
	result, err := cbm.Execute("open_db", func() (any, error) {
		return "should not run", nil
	})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "currently unavailable")
	assert.Contains(t, err.Error(), "circuit breaker open")
}

func TestCircuitBreakerManager_GetState_OpenState(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	tripCircuitBreaker(cbm, "state_open_db")

	state := cbm.GetState("state_open_db")
	assert.Equal(t, constant.CircuitBreakerStateOpen, state)
}

func TestCircuitBreakerManager_IsHealthy_OpenState(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	tripCircuitBreaker(cbm, "unhealthy_db")

	// Open state should not be healthy
	assert.False(t, cbm.IsHealthy("unhealthy_db"))
}

func TestCircuitBreakerManager_ShouldAllowRetry_OpenState(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	tripCircuitBreaker(cbm, "retry_open_db")

	// Open state should block retries
	assert.False(t, cbm.ShouldAllowRetry("retry_open_db"))
}

func TestCircuitBreakerManager_Reset_AfterOpen(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Trip the circuit breaker
	tripCircuitBreaker(cbm, "reset_open_db")
	assert.Equal(t, constant.CircuitBreakerStateOpen, cbm.GetState("reset_open_db"))

	// Reset it
	cbm.Reset("reset_open_db")

	// After reset, state should be closed and counts should be zero
	assert.Equal(t, constant.CircuitBreakerStateClosed, cbm.GetState("reset_open_db"))

	counts := cbm.GetCounts("reset_open_db")
	assert.Equal(t, uint32(0), counts.Requests)
	assert.Equal(t, uint32(0), counts.TotalFailures)

	// Should be able to execute again
	result, err := cbm.Execute("reset_open_db", func() (any, error) {
		return "recovered", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "recovered", result)
}

func TestCircuitBreakerManager_Execute_TooManyRequests(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Trip the circuit breaker first
	tripCircuitBreaker(cbm, "toomany_db")
	assert.Equal(t, constant.CircuitBreakerStateOpen, cbm.GetState("toomany_db"))

	// Wait for circuit breaker timeout to transition to half-open
	// CircuitBreakerTimeout is typically short in test scenarios
	// Note: gobreaker transitions to half-open when requests come in after timeout
	// The Execute wraps ErrTooManyRequests but this requires half-open state
	// which needs waiting for the timeout. For unit test purposes, we verify the open state path.

	// Verify the open state error wrapping
	_, err := cbm.Execute("toomany_db", func() (any, error) {
		return nil, nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "currently unavailable")
}

func TestCircuitBreakerManager_Reset_PreservesOtherBreakers(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Create two breakers
	cbm.GetOrCreate("db_a")
	cbm.GetOrCreate("db_b")

	// Trip one
	tripCircuitBreaker(cbm, "db_a")
	assert.Equal(t, constant.CircuitBreakerStateOpen, cbm.GetState("db_a"))

	// Execute on the other should still work
	result, err := cbm.Execute("db_b", func() (any, error) {
		return "ok", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result)

	// Reset only db_a
	cbm.Reset("db_a")
	assert.Equal(t, constant.CircuitBreakerStateClosed, cbm.GetState("db_a"))

	// db_b should still be closed (it was never tripped)
	assert.Equal(t, constant.CircuitBreakerStateClosed, cbm.GetState("db_b"))
}

func TestCircuitBreakerManager_Reset_MultipleResets(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	cbm.GetOrCreate("multi_reset_db")

	// Trip and reset multiple times
	for i := 0; i < 3; i++ {
		tripCircuitBreaker(cbm, "multi_reset_db")
		assert.Equal(t, constant.CircuitBreakerStateOpen, cbm.GetState("multi_reset_db"))

		cbm.Reset("multi_reset_db")
		assert.Equal(t, constant.CircuitBreakerStateClosed, cbm.GetState("multi_reset_db"))

		// Verify it works after reset
		result, err := cbm.Execute("multi_reset_db", func() (any, error) {
			return "attempt", nil
		})
		require.NoError(t, err)
		assert.Equal(t, "attempt", result)
	}
}

func TestCircuitBreakerManager_ReadyToTrip_FailureRatio(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Intersperse successes and failures to avoid hitting the consecutive failure
	// threshold (15), while reaching MinRequests (10) with >= 50% failure ratio.
	// Pattern: fail, success, fail, success, fail, success, fail, success, fail, fail
	// = 10 requests, 6 failures, 4 successes → ratio 0.6 >= 0.5

	cb := cbm.GetOrCreate("ratio_db")
	pattern := []bool{false, true, false, true, false, true, false, true, false, false}

	for _, success := range pattern {
		if success {
			_, _ = cb.Execute(func() (any, error) {
				return "ok", nil
			})
		} else {
			_, _ = cb.Execute(func() (any, error) {
				return nil, errors.New("failure")
			})
		}
	}

	// Circuit breaker should have tripped due to failure ratio
	state := cbm.GetState("ratio_db")
	assert.Equal(t, constant.CircuitBreakerStateOpen, state,
		"circuit breaker should open via failure ratio path (6/10 = 0.6 >= 0.5)")
}

func TestCircuitBreakerManager_ReadyToTrip_BelowMinRequests(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Send fewer than MinRequests (10) with high failure ratio
	// but not enough consecutive failures to hit threshold (15)
	cb := cbm.GetOrCreate("below_min_db")
	pattern := []bool{false, true, false, true, false} // 5 requests, 3 failures

	for _, success := range pattern {
		if success {
			_, _ = cb.Execute(func() (any, error) {
				return "ok", nil
			})
		} else {
			_, _ = cb.Execute(func() (any, error) {
				return nil, errors.New("failure")
			})
		}
	}

	// Should stay closed: below MinRequests and below consecutive threshold
	state := cbm.GetState("below_min_db")
	assert.Equal(t, constant.CircuitBreakerStateClosed, state,
		"circuit breaker should stay closed when below MinRequests")
}

func TestCircuitBreakerManager_ReadyToTrip_ZeroRequests(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Create breaker but don't send any requests - should not panic
	cbm.GetOrCreate("zero_db")

	state := cbm.GetState("zero_db")
	assert.Equal(t, constant.CircuitBreakerStateClosed, state,
		"circuit breaker should stay closed with zero requests")
}

func TestCircuitBreakerManager_GetOrCreate_Idempotent(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	breaker1 := cbm.GetOrCreate("idempotent_db")
	breaker2 := cbm.GetOrCreate("idempotent_db")

	// Should return the same instance
	assert.Equal(t, breaker1, breaker2)
	assert.Equal(t, 1, len(cbm.breakers))
}

func TestCircuitBreakerManager_Execute_SuccessAfterFailures(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Execute a few failures (not enough to trip the breaker)
	for i := 0; i < 3; i++ {
		_, _ = cbm.Execute("recover_db", func() (any, error) {
			return nil, errors.New("transient error")
		})
	}

	// Then a success should still work
	result, err := cbm.Execute("recover_db", func() (any, error) {
		return "recovered", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "recovered", result)

	// Verify counts
	counts := cbm.GetCounts("recover_db")
	assert.Equal(t, uint32(3), counts.TotalFailures)
	assert.Equal(t, uint32(1), counts.TotalSuccesses)
}

func TestCircuitBreakerManager_Execute_PassesThroughFunctionError(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	expectedErr := errors.New("business logic error")

	result, err := cbm.Execute("passthrough_db", func() (any, error) {
		return nil, expectedErr
	})

	assert.Nil(t, result)
	require.Error(t, err)
	// The error from the function should be passed through directly
	// (not wrapped, since it's not a circuit breaker error)
	assert.Equal(t, expectedErr, err)
}

func TestCircuitBreakerManager_GetState_AfterResetIsClosedAgain(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Trip circuit breaker
	tripCircuitBreaker(cbm, "state_reset_db")
	assert.Equal(t, constant.CircuitBreakerStateOpen, cbm.GetState("state_reset_db"))

	// Reset
	cbm.Reset("state_reset_db")
	assert.Equal(t, constant.CircuitBreakerStateClosed, cbm.GetState("state_reset_db"))

	// Should be healthy after reset
	assert.True(t, cbm.IsHealthy("state_reset_db"))
	assert.True(t, cbm.ShouldAllowRetry("state_reset_db"))
}

func TestCircuitBreakerManager_Execute_NilResult(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// A function returning nil result and nil error should work
	result, err := cbm.Execute("nil_db", func() (any, error) {
		return nil, nil
	})

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestCircuitBreakerManager_HalfOpenState(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Create a custom breaker with very short timeout (50ms) to test half-open state
	shortSettings := gobreaker.Settings{
		Name:        "datasource-halfopen_test_db",
		MaxRequests: constant.CircuitBreakerMaxRequests,
		Interval:    constant.CircuitBreakerInterval,
		Timeout:     50 * time.Millisecond, // Very short timeout for testing
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			cbm.logger.Log(context.Background(), log.LevelWarn, fmt.Sprintf("Circuit Breaker [%s] state changed: %s -> %s", name, from.String(), to.String()))

			switch to {
			case gobreaker.StateOpen:
				cbm.logger.Log(context.Background(), log.LevelError, fmt.Sprintf("Circuit Breaker [%s] OPENED - datasource is unhealthy, requests will fast-fail", name))
			case gobreaker.StateHalfOpen:
				cbm.logger.Log(context.Background(), log.LevelInfo, fmt.Sprintf("Circuit Breaker [%s] HALF-OPEN - testing datasource recovery", name))
			case gobreaker.StateClosed:
				cbm.logger.Log(context.Background(), log.LevelInfo, fmt.Sprintf("Circuit Breaker [%s] CLOSED - datasource is healthy", name))
			}
		},
	}

	breaker := gobreaker.NewCircuitBreaker(shortSettings)

	// Inject the custom breaker into the manager
	cbm.mu.Lock()
	cbm.breakers["halfopen_test_db"] = breaker
	cbm.mu.Unlock()

	// Trip the circuit breaker by sending consecutive failures
	for i := 0; i < 5; i++ {
		_, _ = breaker.Execute(func() (any, error) {
			return nil, errors.New("deliberate failure")
		})
	}

	// Verify it's open
	assert.Equal(t, constant.CircuitBreakerStateOpen, cbm.GetState("halfopen_test_db"))

	// Wait for the short timeout to expire so breaker transitions to half-open
	time.Sleep(100 * time.Millisecond)

	// Now GetState should return half-open (covers lines 143-144)
	state := cbm.GetState("halfopen_test_db")
	assert.Equal(t, constant.CircuitBreakerStateHalfOpen, state)

	// IsHealthy in half-open should return true (not open)
	assert.True(t, cbm.IsHealthy("halfopen_test_db"))

	// ShouldAllowRetry in half-open with no requests should allow
	assert.True(t, cbm.ShouldAllowRetry("halfopen_test_db"))
}

func TestCircuitBreakerManager_Execute_ErrTooManyRequests(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Create a custom breaker with very short timeout and MaxRequests=1
	shortSettings := gobreaker.Settings{
		Name:        "datasource-toomany_test_db",
		MaxRequests: 1, // Only allow 1 request in half-open
		Interval:    constant.CircuitBreakerInterval,
		Timeout:     50 * time.Millisecond,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			cbm.logger.Log(context.Background(), log.LevelWarn, fmt.Sprintf("Circuit Breaker [%s] state changed: %s -> %s", name, from.String(), to.String()))
			switch to {
			case gobreaker.StateOpen:
				cbm.logger.Log(context.Background(), log.LevelError, fmt.Sprintf("Circuit Breaker [%s] OPENED", name))
			case gobreaker.StateHalfOpen:
				cbm.logger.Log(context.Background(), log.LevelInfo, fmt.Sprintf("Circuit Breaker [%s] HALF-OPEN", name))
			case gobreaker.StateClosed:
				cbm.logger.Log(context.Background(), log.LevelInfo, fmt.Sprintf("Circuit Breaker [%s] CLOSED", name))
			}
		},
	}

	breaker := gobreaker.NewCircuitBreaker(shortSettings)

	cbm.mu.Lock()
	cbm.breakers["toomany_test_db"] = breaker
	cbm.mu.Unlock()

	// Trip the breaker
	for i := 0; i < 5; i++ {
		_, _ = breaker.Execute(func() (any, error) {
			return nil, errors.New("deliberate failure")
		})
	}

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)

	// First request in half-open should go through (MaxRequests=1)
	// Use a blocking channel to keep the first request "in flight"
	started := make(chan struct{})
	proceed := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		_, err := cbm.Execute("toomany_test_db", func() (any, error) {
			close(started) // signal that we're inside the function
			<-proceed      // wait for signal to complete
			return "ok", nil
		})
		errCh <- err
	}()

	// Wait for the first request to start executing
	<-started

	// Second request should get ErrTooManyRequests (covers lines 118-121)
	result, err := cbm.Execute("toomany_test_db", func() (any, error) {
		return "should not run", nil
	})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recovering")
	assert.Contains(t, err.Error(), "too many requests")

	// Let the first request complete
	close(proceed)
	<-errCh
}

func TestCircuitBreakerManager_HalfOpenRecovery(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Create a custom breaker with very short timeout
	shortSettings := gobreaker.Settings{
		Name:        "datasource-recovery_test_db",
		MaxRequests: 1,
		Interval:    constant.CircuitBreakerInterval,
		Timeout:     50 * time.Millisecond,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			cbm.logger.Log(context.Background(), log.LevelWarn, fmt.Sprintf("Circuit Breaker [%s] state changed: %s -> %s", name, from.String(), to.String()))
			switch to {
			case gobreaker.StateOpen:
				cbm.logger.Log(context.Background(), log.LevelError, fmt.Sprintf("Circuit Breaker [%s] OPENED", name))
			case gobreaker.StateHalfOpen:
				cbm.logger.Log(context.Background(), log.LevelInfo, fmt.Sprintf("Circuit Breaker [%s] HALF-OPEN - testing datasource recovery", name))
			case gobreaker.StateClosed:
				cbm.logger.Log(context.Background(), log.LevelInfo, fmt.Sprintf("Circuit Breaker [%s] CLOSED - datasource is healthy", name))
			}
		},
	}

	breaker := gobreaker.NewCircuitBreaker(shortSettings)

	cbm.mu.Lock()
	cbm.breakers["recovery_test_db"] = breaker
	cbm.mu.Unlock()

	// Trip the breaker
	for i := 0; i < 5; i++ {
		_, _ = breaker.Execute(func() (any, error) {
			return nil, errors.New("deliberate failure")
		})
	}

	assert.Equal(t, constant.CircuitBreakerStateOpen, cbm.GetState("recovery_test_db"))

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, constant.CircuitBreakerStateHalfOpen, cbm.GetState("recovery_test_db"))

	// Execute a successful request to trigger recovery (half-open -> closed)
	// This covers the OnStateChange StateHalfOpen and StateClosed callbacks
	result, err := cbm.Execute("recovery_test_db", func() (any, error) {
		return "recovered", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "recovered", result)

	// After successful request in half-open, should transition to closed
	assert.Equal(t, constant.CircuitBreakerStateClosed, cbm.GetState("recovery_test_db"))
	assert.True(t, cbm.IsHealthy("recovery_test_db"))
}

func TestCircuitBreakerManager_ShouldAllowRetry_HalfOpenAtMaxCapacity(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbm := NewCircuitBreakerManager(logger)

	// Create a custom breaker with MaxRequests higher than CircuitBreakerMaxRequests (3)
	// so we can complete 3 successful requests without closing the breaker,
	// which makes counts.Requests >= constant.CircuitBreakerMaxRequests while still half-open.
	shortSettings := gobreaker.Settings{
		Name:        "datasource-retry_halfopen_db",
		MaxRequests: constant.CircuitBreakerMaxRequests + 2, // Higher than the constant so breaker stays half-open
		Interval:    constant.CircuitBreakerInterval,
		Timeout:     50 * time.Millisecond,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
	}

	breaker := gobreaker.NewCircuitBreaker(shortSettings)

	cbm.mu.Lock()
	cbm.breakers["retry_halfopen_db"] = breaker
	cbm.mu.Unlock()

	// Trip the breaker
	for i := 0; i < 5; i++ {
		_, _ = breaker.Execute(func() (any, error) {
			return nil, errors.New("deliberate failure")
		})
	}

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)

	// Send exactly CircuitBreakerMaxRequests (3) successful requests in half-open.
	// Since MaxRequests=5, the breaker stays in half-open state after 3 successes.
	for i := uint32(0); i < constant.CircuitBreakerMaxRequests; i++ {
		_, _ = breaker.Execute(func() (any, error) {
			return "ok", nil
		})
	}

	// Verify breaker is still half-open
	assert.Equal(t, constant.CircuitBreakerStateHalfOpen, cbm.GetState("retry_halfopen_db"))

	// Now ShouldAllowRetry should return false (half-open + counts.Requests >= CircuitBreakerMaxRequests)
	// covers lines 201-203
	result := cbm.ShouldAllowRetry("retry_halfopen_db")
	assert.False(t, result, "ShouldAllowRetry should be false when half-open at max capacity")
}
