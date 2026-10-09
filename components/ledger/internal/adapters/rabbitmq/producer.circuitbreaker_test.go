// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"testing"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v7/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func setupMockLogger(ctrl *gomock.Controller) *libLog.MockLogger {
	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Log(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	return logger
}

// setupCircuitBreakerManager creates a cbManager with the circuit breaker pre-registered.
// This simulates what NewCircuitBreakerManager does in bootstrap.
func setupCircuitBreakerManager(logger libLog.Logger) libCircuitBreaker.Manager {
	cbManager, err := libCircuitBreaker.NewManager(logger)
	if err != nil {
		return nil
	}

	// Pre-register the circuit breaker (normally done by bootstrap.NewCircuitBreakerManager)
	cbConfig := CircuitBreakerConfig{
		ConsecutiveFailures: 3,
		FailureRatio:        0.5,
		Interval:            30 * time.Second,
		MaxRequests:         3,
		MinRequests:         5,
		Timeout:             30 * time.Second,
	}
	cbManager.GetOrCreate(CircuitBreakerServiceName, RabbitMQCircuitBreakerConfig(cbConfig))
	return cbManager
}

func TestCircuitBreakerProducer_ImplementsProducerRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	// Verify it implements the interface
	var _ ProducerRepository = cbProducer
}

func TestCircuitBreakerProducer_ProducerDefault_SuccessPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Setup expectation
	mockProducer.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, nil)

	// Execute
	result, err := cbProducer.ProducerDefault(ctx, exchange, key, message)

	// Verify
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestCircuitBreakerProducer_ProducerDefault_FailureTripsCircuit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Setup: Mock returns error on every call
	mockProducer.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, errors.New("connection refused")).
		Times(3)

	// Execute: Call 3 times to trip the circuit
	for i := 0; i < 3; i++ {
		_, err := cbProducer.ProducerDefault(ctx, exchange, key, message)
		assert.Error(t, err)
	}

	// Verify circuit is now open
	state := cbManager.GetState(CircuitBreakerServiceName)
	assert.Equal(t, libCircuitBreaker.StateOpen, state)
}

func TestCircuitBreakerProducer_ProducerDefault_CircuitOpenReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Trip the circuit first
	mockProducer.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, errors.New("connection refused")).
		Times(3)

	for i := 0; i < 3; i++ {
		_, err := cbProducer.ProducerDefault(ctx, exchange, key, message)
		require.Error(t, err, "call %d should fail", i)
	}

	// Now when circuit is open, no calls to underlying producer
	// ProducerDefault should not be called again (circuit is open)
	_, producerErr := cbProducer.ProducerDefault(ctx, exchange, key, message)

	// Verify error indicates service is unavailable (generic error, no internal details exposed)
	require.Error(t, producerErr)
	assert.ErrorIs(t, producerErr, ErrServiceUnavailable)
}

func TestCircuitBreakerProducer_CheckRabbitMQHealth_DelegatesToUnderlying(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	// Setup expectation
	mockProducer.EXPECT().
		CheckRabbitMQHealth().
		Return(true)

	// Execute
	result := cbProducer.CheckRabbitMQHealth()

	// Verify
	assert.True(t, result)
}

func TestCircuitBreakerProducer_GetCircuitState_ReturnsCurrentState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	// Initial state should be closed
	state := cbProducer.GetCircuitState()
	assert.Equal(t, libCircuitBreaker.StateClosed, state)
}

func TestCircuitBreakerProducer_IsCircuitHealthy_ReturnsTrueWhenClosed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	// Initial state should be healthy (closed)
	assert.True(t, cbProducer.IsCircuitHealthy())
}

func TestCircuitBreakerProducer_IsCircuitHealthy_ReturnsFalseWhenOpen(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Trip the circuit first
	mockProducer.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, errors.New("connection refused")).
		Times(3)

	for i := 0; i < 3; i++ {
		_, err := cbProducer.ProducerDefault(ctx, exchange, key, message)
		require.Error(t, err, "Call %d should fail", i)
	}

	// Circuit should now be unhealthy (open)
	assert.False(t, cbProducer.IsCircuitHealthy())
	assert.Equal(t, libCircuitBreaker.StateOpen, cbProducer.GetCircuitState())
}

func TestCircuitBreakerProducer_GetCounts_ReturnsStatistics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Initial counts should be zero
	initialCounts := cbProducer.GetCounts()
	assert.Equal(t, uint32(0), initialCounts.Requests)
	assert.Equal(t, uint32(0), initialCounts.TotalSuccesses)
	assert.Equal(t, uint32(0), initialCounts.TotalFailures)

	// Make a successful call
	mockProducer.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, nil)

	_, producerErr := cbProducer.ProducerDefault(ctx, exchange, key, message)
	require.NoError(t, producerErr)

	// Verify counts updated
	countsAfterSuccess := cbProducer.GetCounts()
	assert.Equal(t, uint32(1), countsAfterSuccess.Requests)
	assert.Equal(t, uint32(1), countsAfterSuccess.TotalSuccesses)
	assert.Equal(t, uint32(0), countsAfterSuccess.TotalFailures)

	// Make a failing call
	mockProducer.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, errors.New("connection refused"))

	_, producerErr = cbProducer.ProducerDefault(ctx, exchange, key, message)
	require.Error(t, producerErr)

	// Verify counts updated
	countsAfterFailure := cbProducer.GetCounts()
	assert.Equal(t, uint32(2), countsAfterFailure.Requests)
	assert.Equal(t, uint32(1), countsAfterFailure.TotalSuccesses)
	assert.Equal(t, uint32(1), countsAfterFailure.TotalFailures)
}

func TestCircuitBreakerProducer_ProducerDefault_WithNonNilReturn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Setup: Mock returns a non-nil string pointer
	expectedID := "msg-123"
	mockProducer.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(&expectedID, nil)

	// Execute
	result, err := cbProducer.ProducerDefault(ctx, exchange, key, message)

	// Verify
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, expectedID, *result)
}

func TestCircuitBreakerProducer_ProducerDefault_WithEmptyMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	emptyMessage := []byte{}

	// Setup: Mock accepts empty message
	mockProducer.EXPECT().
		ProducerDefault(ctx, exchange, key, emptyMessage).
		Return(nil, nil)

	// Execute
	result, err := cbProducer.ProducerDefault(ctx, exchange, key, emptyMessage)

	// Verify - circuit breaker passes through empty messages
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestCircuitBreakerProducer_ProducerDefault_WithEmptyExchangeAndKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	ctx := context.Background()
	emptyExchange := ""
	emptyKey := ""
	message := []byte("test message")

	// Setup: Mock accepts empty exchange and key
	mockProducer.EXPECT().
		ProducerDefault(ctx, emptyExchange, emptyKey, message).
		Return(nil, nil)

	// Execute
	result, err := cbProducer.ProducerDefault(ctx, emptyExchange, emptyKey, message)

	// Verify - circuit breaker passes through empty strings
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestCircuitBreakerProducer_ProducerDefault_WithNilMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	var nilMessage []byte = nil

	// Setup: Mock accepts nil message
	mockProducer.EXPECT().
		ProducerDefault(ctx, exchange, key, nilMessage).
		Return(nil, nil)

	// Execute
	result, producerErr := cbProducer.ProducerDefault(ctx, exchange, key, nilMessage)

	// Verify - circuit breaker passes through nil messages
	assert.NoError(t, producerErr)
	assert.Nil(t, result)
}

func TestNewCircuitBreakerProducer_ReturnsErrorOnNilUnderlying(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(nil, cbManager, logger)

	assert.Nil(t, cbProducer)
	assert.ErrorIs(t, err, ErrNilUnderlying)
}

func TestNewCircuitBreakerProducer_ReturnsErrorOnNilCBManager(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, nil, logger)

	assert.Nil(t, cbProducer)
	assert.ErrorIs(t, err, ErrNilCBManager)
}

func TestNewCircuitBreakerProducer_ReturnsErrorOnNilLogger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, nil)

	assert.Nil(t, cbProducer)
	assert.ErrorIs(t, err, ErrNilCBLogger)
}

func TestCircuitBreakerProducer_ProducerDefault_HalfOpenState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)

	// Create circuit breaker with very short timeout to trigger half-open quickly
	cbManager, err := libCircuitBreaker.NewManager(logger)
	require.NoError(t, err)
	cbConfig := CircuitBreakerConfig{
		ConsecutiveFailures: 3,
		FailureRatio:        0.5,
		Interval:            30 * time.Second,
		MaxRequests:         3, // Requests allowed in half-open before deciding
		MinRequests:         5,
		Timeout:             50 * time.Millisecond, // Very short timeout for test
	}
	cbManager.GetOrCreate(CircuitBreakerServiceName, RabbitMQCircuitBreakerConfig(cbConfig))

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Trip the circuit first with 3 consecutive failures
	mockProducer.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, errors.New("connection refused")).
		Times(3)

	for i := 0; i < 3; i++ {
		_, err := cbProducer.ProducerDefault(ctx, exchange, key, message)
		require.Error(t, err, "Call %d should fail", i)
	}

	// Verify circuit is now open
	assert.Equal(t, libCircuitBreaker.StateOpen, cbProducer.GetCircuitState())

	// Wait for timeout to transition to half-open
	time.Sleep(100 * time.Millisecond)

	// In half-open state, the circuit allows MaxRequests (3) to test if service recovered
	// After MaxRequests consecutive successes, the circuit should close
	mockProducer.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, nil).
		Times(3)

	for i := 0; i < 3; i++ {
		_, err = cbProducer.ProducerDefault(ctx, exchange, key, message)
		assert.NoError(t, err, "request %d in half-open state should succeed", i)
	}

	// After MaxRequests successes in half-open, circuit should be closed
	assert.Equal(t, libCircuitBreaker.StateClosed, cbProducer.GetCircuitState())
}

func TestCircuitBreakerProducer_ProducerDefault_HalfOpenToOpen(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)

	// Create circuit breaker with very short timeout
	cbManager, err := libCircuitBreaker.NewManager(logger)
	require.NoError(t, err)
	cbConfig := CircuitBreakerConfig{
		ConsecutiveFailures: 3,
		FailureRatio:        0.5,
		Interval:            30 * time.Second,
		MaxRequests:         3,
		MinRequests:         5,
		Timeout:             50 * time.Millisecond, // Very short timeout for test
	}
	cbManager.GetOrCreate(CircuitBreakerServiceName, RabbitMQCircuitBreakerConfig(cbConfig))

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Trip the circuit first with 3 consecutive failures
	mockProducer.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, errors.New("connection refused")).
		Times(3)

	for i := 0; i < 3; i++ {
		_, err := cbProducer.ProducerDefault(ctx, exchange, key, message)
		require.Error(t, err, "Call %d should fail", i)
	}

	// Verify circuit is now open
	assert.Equal(t, libCircuitBreaker.StateOpen, cbProducer.GetCircuitState())

	// Wait for timeout to transition to half-open
	time.Sleep(100 * time.Millisecond)

	// In half-open state, a failure should re-open the circuit
	mockProducer.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, errors.New("still failing")).
		Times(1)

	_, err = cbProducer.ProducerDefault(ctx, exchange, key, message)
	assert.Error(t, err, "request in half-open state should fail")

	// After failure in half-open, circuit should be open again
	assert.Equal(t, libCircuitBreaker.StateOpen, cbProducer.GetCircuitState())
}

func TestCircuitBreakerProducer_CheckRabbitMQHealth_ReturnsFalseWhenUnhealthy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	// Setup expectation - underlying returns false
	mockProducer.EXPECT().
		CheckRabbitMQHealth().
		Return(false)

	// Execute
	result := cbProducer.CheckRabbitMQHealth()

	// Verify
	assert.False(t, result)
}

func TestCircuitBreakerProducer_ConcurrentCalls(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Allow any number of concurrent calls
	mockProducer.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	numGoroutines := 100
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, _ = cbProducer.ProducerDefault(ctx, exchange, key, message)
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Test passes if no race conditions detected (run with -race flag)
	// Verify circuit breaker is still in healthy state
	assert.True(t, cbProducer.IsCircuitHealthy())
}
