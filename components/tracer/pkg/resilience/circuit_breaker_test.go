// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("test-cb")

	assert.Equal(t, "test-cb", cfg.Name)
	assert.Equal(t, uint32(3), cfg.MaxRequests)
	assert.Equal(t, 60*time.Second, cfg.Interval)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, uint32(5), cfg.FailureThresh)
	assert.Equal(t, 0.5, cfg.FailureRatio)
	assert.Equal(t, uint32(10), cfg.MinRequests)
}

func TestNewCircuitBreaker(t *testing.T) {
	tests := []struct {
		name string
		cfg  CircuitBreakerConfig
	}{
		{
			name: "Success - creates circuit breaker with default config",
			cfg:  DefaultConfig("test"),
		},
		{
			name: "Success - creates circuit breaker with custom config",
			cfg: CircuitBreakerConfig{
				Name:          "custom",
				MaxRequests:   5,
				Interval:      30 * time.Second,
				Timeout:       15 * time.Second,
				FailureThresh: 3,
				FailureRatio:  0.3,
				MinRequests:   5,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := testutil.NewMockLogger()
			cb := NewCircuitBreaker(tc.cfg, logger)

			require.NotNil(t, cb)
			assert.NotNil(t, cb.cb)
			assert.Equal(t, logger, cb.logger)
		})
	}
}

func TestCircuitBreaker_Execute(t *testing.T) {
	tests := []struct {
		name        string
		fn          func() (any, error)
		expectedVal any
		expectedErr error
	}{
		{
			name: "Success - function returns value",
			fn: func() (any, error) {
				return "success", nil
			},
			expectedVal: "success",
			expectedErr: nil,
		},
		{
			name: "Error - function returns error",
			fn: func() (any, error) {
				return nil, errors.New("operation failed")
			},
			expectedVal: nil,
			expectedErr: errors.New("operation failed"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := testutil.NewMockLogger()
			cb := NewCircuitBreaker(DefaultConfig("test"), logger)
			ctx := context.Background()

			val, err := cb.Execute(ctx, tc.fn)

			assert.Equal(t, tc.expectedVal, val)

			if tc.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCircuitBreaker_Execute_ContextCanceled(t *testing.T) {
	logger := testutil.NewMockLogger()
	cb := NewCircuitBreaker(DefaultConfig("test"), logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	val, err := cb.Execute(ctx, func() (any, error) {
		return "success", nil
	})

	assert.Nil(t, val)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestCircuitBreaker_State(t *testing.T) {
	logger := testutil.NewMockLogger()
	cb := NewCircuitBreaker(DefaultConfig("test"), logger)

	// Initial state should be closed
	assert.Equal(t, gobreaker.StateClosed, cb.State())
	assert.False(t, cb.IsOpen())
}

func TestCircuitBreaker_TripsOnConsecutiveFailures(t *testing.T) {
	logger := testutil.NewMockLogger()
	cfg := CircuitBreakerConfig{
		Name:          "test",
		MaxRequests:   1,
		Interval:      60 * time.Second,
		Timeout:       1 * time.Second,
		FailureThresh: 3, // Trip after 3 consecutive failures
		FailureRatio:  0,
		MinRequests:   0,
	}
	cb := NewCircuitBreaker(cfg, logger)
	ctx := context.Background()

	failingFn := func() (any, error) {
		return nil, errors.New("failure")
	}

	// Execute 3 failing requests to trip the circuit
	for i := 0; i < 3; i++ {
		_, err := cb.Execute(ctx, failingFn)
		assert.Error(t, err)
		assert.NotEqual(t, gobreaker.ErrOpenState, err, "Circuit should not be open yet")
	}

	// Circuit should be open now
	assert.True(t, cb.IsOpen())
	assert.Equal(t, gobreaker.StateOpen, cb.State())

	// Next request should fail with circuit breaker error
	_, err := cb.Execute(ctx, func() (any, error) {
		return "success", nil
	})
	assert.Error(t, err)
	assert.Equal(t, gobreaker.ErrOpenState, err)
}

func TestCircuitBreaker_TripsOnFailureRatio(t *testing.T) {
	logger := testutil.NewMockLogger()
	cfg := CircuitBreakerConfig{
		Name:          "test",
		MaxRequests:   1,
		Interval:      60 * time.Second,
		Timeout:       1 * time.Second,
		FailureThresh: 100, // High threshold so it won't trip on consecutive
		FailureRatio:  0.5, // Trip at 50% failure rate
		MinRequests:   4,   // Need at least 4 requests
	}
	cb := NewCircuitBreaker(cfg, logger)
	ctx := context.Background()

	// 2 success + 2 failures = 50% failure rate
	_, _ = cb.Execute(ctx, func() (any, error) { return "ok", nil })
	_, _ = cb.Execute(ctx, func() (any, error) { return "ok", nil })
	_, _ = cb.Execute(ctx, func() (any, error) { return nil, errors.New("fail") })
	_, _ = cb.Execute(ctx, func() (any, error) { return nil, errors.New("fail") })

	// Circuit should be open now (50% failure rate >= 0.5 threshold)
	assert.True(t, cb.IsOpen())
}

func TestCircuitBreaker_DoesNotTripBelowMinRequests(t *testing.T) {
	logger := testutil.NewMockLogger()
	cfg := CircuitBreakerConfig{
		Name:          "test",
		MaxRequests:   1,
		Interval:      60 * time.Second,
		Timeout:       1 * time.Second,
		FailureThresh: 100,
		FailureRatio:  0.5, // 50% failure rate threshold
		MinRequests:   10,  // Need at least 10 requests
	}
	cb := NewCircuitBreaker(cfg, logger)
	ctx := context.Background()

	// Execute 3 failures out of 3 requests (100% failure rate, but below MinRequests)
	for i := 0; i < 3; i++ {
		_, _ = cb.Execute(ctx, func() (any, error) { return nil, errors.New("fail") })
	}

	// Circuit should NOT be open (only 3 requests, MinRequests is 10)
	assert.False(t, cb.IsOpen(), "Circuit should not trip below MinRequests threshold")
	assert.Equal(t, gobreaker.StateClosed, cb.State())
}

func TestCircuitBreaker_Execute_ContextTimeout(t *testing.T) {
	logger := testutil.NewMockLogger()
	cb := NewCircuitBreaker(DefaultConfig("test"), logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Function that takes longer than context timeout
	val, err := cb.Execute(ctx, func() (any, error) {
		time.Sleep(100 * time.Millisecond)
		return "success", nil
	})

	assert.Nil(t, val)
	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
}
