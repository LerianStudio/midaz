// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"testing"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v3/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func setupMockLogger(ctrl *gomock.Controller) *libLog.MockLogger {
	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Warnf(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()

	return logger
}

// setupCircuitBreakerManager creates a cbManager with the circuit breaker pre-registered.
// This simulates what NewCircuitBreakerManager does in bootstrap.
func setupCircuitBreakerManager(logger libLog.Logger) libCircuitBreaker.Manager {
	cbManager := libCircuitBreaker.NewManager(logger)
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

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

	cbProducer, err := NewCircuitBreakerProducer(nil, cbManager, logger, 0)

	assert.Nil(t, cbProducer)
	assert.ErrorIs(t, err, ErrNilUnderlying)
}

func TestNewCircuitBreakerProducer_ReturnsErrorOnNilCBManager(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, nil, logger, 0)

	assert.Nil(t, cbProducer)
	assert.ErrorIs(t, err, ErrNilCBManager)
}

func TestNewCircuitBreakerProducer_ReturnsErrorOnNilLogger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, nil, 0)

	assert.Nil(t, cbProducer)
	assert.ErrorIs(t, err, ErrNilCBLogger)
}

func TestCircuitBreakerProducer_ProducerDefault_HalfOpenState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)

	// Create circuit breaker with very short timeout to trigger half-open quickly
	cbManager := libCircuitBreaker.NewManager(logger)
	cbConfig := CircuitBreakerConfig{
		ConsecutiveFailures: 3,
		FailureRatio:        0.5,
		Interval:            30 * time.Second,
		MaxRequests:         3, // Requests allowed in half-open before deciding
		MinRequests:         5,
		Timeout:             50 * time.Millisecond, // Very short timeout for test
	}
	cbManager.GetOrCreate(CircuitBreakerServiceName, RabbitMQCircuitBreakerConfig(cbConfig))

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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
	cbManager := libCircuitBreaker.NewManager(logger)
	cbConfig := CircuitBreakerConfig{
		ConsecutiveFailures: 3,
		FailureRatio:        0.5,
		Interval:            30 * time.Second,
		MaxRequests:         3,
		MinRequests:         5,
		Timeout:             50 * time.Millisecond, // Very short timeout for test
	}
	cbManager.GetOrCreate(CircuitBreakerServiceName, RabbitMQCircuitBreakerConfig(cbConfig))

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

func TestCircuitBreakerProducer_ProducerDefaultWithContext_SuccessPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Setup expectation - the producer receives a context with timeout (scoped context)
	mockProducer.EXPECT().
		ProducerDefaultWithContext(gomock.Any(), exchange, key, message).
		Return(nil, nil)

	// Execute
	result, err := cbProducer.ProducerDefaultWithContext(ctx, exchange, key, message)

	// Verify
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestCircuitBreakerProducer_ProducerDefaultWithContext_WithNonNilReturn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Setup: Mock returns a non-nil string pointer
	expectedID := "msg-456"
	mockProducer.EXPECT().
		ProducerDefaultWithContext(gomock.Any(), exchange, key, message).
		Return(&expectedID, nil)

	// Execute
	result, err := cbProducer.ProducerDefaultWithContext(ctx, exchange, key, message)

	// Verify
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, expectedID, *result)
}

func TestCircuitBreakerProducer_ProducerDefaultWithContext_FailureTripsCircuit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Setup: Mock returns error on every call
	mockProducer.EXPECT().
		ProducerDefaultWithContext(gomock.Any(), exchange, key, message).
		Return(nil, errors.New("connection refused")).
		Times(3)

	// Execute: Call 3 times to trip the circuit
	for i := 0; i < 3; i++ {
		_, err := cbProducer.ProducerDefaultWithContext(ctx, exchange, key, message)
		assert.Error(t, err)
	}

	// Verify circuit is now open
	state := cbManager.GetState(CircuitBreakerServiceName)
	assert.Equal(t, libCircuitBreaker.StateOpen, state)
}

func TestCircuitBreakerProducer_ProducerDefaultWithContext_CircuitOpenReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Trip the circuit first
	mockProducer.EXPECT().
		ProducerDefaultWithContext(gomock.Any(), exchange, key, message).
		Return(nil, errors.New("connection refused")).
		Times(3)

	for i := 0; i < 3; i++ {
		_, err := cbProducer.ProducerDefaultWithContext(ctx, exchange, key, message)
		require.Error(t, err, "call %d should fail", i)
	}

	// Now when circuit is open, no calls to underlying producer
	_, producerErr := cbProducer.ProducerDefaultWithContext(ctx, exchange, key, message)

	// Verify error indicates service is unavailable
	require.Error(t, producerErr)
	assert.ErrorIs(t, producerErr, ErrServiceUnavailable)
}

func TestCircuitBreakerProducer_ProducerDefaultWithContext_UsesScopedTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	// Use a custom timeout
	customTimeout := 100 * time.Millisecond
	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, customTimeout)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Verify that the context passed to underlying has a deadline
	mockProducer.EXPECT().
		ProducerDefaultWithContext(gomock.Any(), exchange, key, message).
		DoAndReturn(func(ctx context.Context, exchange, key string, message []byte) (*string, error) {
			deadline, ok := ctx.Deadline()
			assert.True(t, ok, "context should have a deadline")
			assert.WithinDuration(t, time.Now().Add(customTimeout), deadline, 50*time.Millisecond)
			return nil, nil
		})

	// Execute
	_, err = cbProducer.ProducerDefaultWithContext(ctx, exchange, key, message)
	assert.NoError(t, err)
}

func TestCircuitBreakerProducer_ProducerDefaultWithContext_UsesDefaultTimeoutWhenZero(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	// Use 0 to get default timeout
	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Verify that the context passed to underlying has a deadline with DefaultOperationTimeout
	mockProducer.EXPECT().
		ProducerDefaultWithContext(gomock.Any(), exchange, key, message).
		DoAndReturn(func(ctx context.Context, exchange, key string, message []byte) (*string, error) {
			deadline, ok := ctx.Deadline()
			assert.True(t, ok, "context should have a deadline")
			assert.WithinDuration(t, time.Now().Add(DefaultOperationTimeout), deadline, 50*time.Millisecond)
			return nil, nil
		})

	// Execute
	_, err = cbProducer.ProducerDefaultWithContext(ctx, exchange, key, message)
	assert.NoError(t, err)
}

func TestCircuitBreakerProducer_ProducerDefaultWithContext_PreCanceledContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
	require.NoError(t, err)

	// Create pre-canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Mock should receive the canceled context and return context.Canceled
	mockProducer.EXPECT().
		ProducerDefaultWithContext(gomock.Any(), exchange, key, message).
		Return(nil, context.Canceled)

	_, err = cbProducer.ProducerDefaultWithContext(ctx, exchange, key, message)

	// Should return context.Canceled error
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCircuitBreakerProducer_ProducerDefaultWithContext_ContextDeadlineExceeded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
	require.NoError(t, err)

	// Create context that's already at deadline
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(1 * time.Millisecond) // Ensure deadline passes

	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Mock should receive the expired context
	mockProducer.EXPECT().
		ProducerDefaultWithContext(gomock.Any(), exchange, key, message).
		Return(nil, context.DeadlineExceeded)

	_, err = cbProducer.ProducerDefaultWithContext(ctx, exchange, key, message)

	// Should return deadline exceeded error
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestCircuitBreakerProducer_ConcurrentCalls(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
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

func TestCircuitBreakerProducer_ConcurrentCallsWithContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 0)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	// Allow any number of concurrent calls
	mockProducer.EXPECT().
		ProducerDefaultWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	numGoroutines := 100
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, _ = cbProducer.ProducerDefaultWithContext(ctx, exchange, key, message)
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Test passes if no race conditions detected (run with -race flag)
	assert.True(t, cbProducer.IsCircuitHealthy())
}

func TestNewCircuitBreakerProducer_NegativeTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	// Negative timeout should use default timeout
	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, -1*time.Second)

	// Should succeed and use default timeout
	require.NoError(t, err)
	require.NotNil(t, cbProducer)

	// Verify it uses default timeout by checking context deadline
	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	mockProducer.EXPECT().
		ProducerDefaultWithContext(gomock.Any(), exchange, key, message).
		DoAndReturn(func(ctx context.Context, exchange, key string, message []byte) (*string, error) {
			deadline, ok := ctx.Deadline()
			assert.True(t, ok, "context should have a deadline")
			// Should use default timeout, not negative
			assert.WithinDuration(t, time.Now().Add(DefaultOperationTimeout), deadline, 50*time.Millisecond)
			return nil, nil
		})

	_, err = cbProducer.ProducerDefaultWithContext(ctx, exchange, key, message)
	assert.NoError(t, err)
}

func TestNewCircuitBreakerProducer_ExceedsMaxTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := setupCircuitBreakerManager(logger)

	// Timeout exceeding max should be capped to MaxOperationTimeout
	cbProducer, err := NewCircuitBreakerProducer(mockProducer, cbManager, logger, 100*time.Second)

	// Should succeed and cap to max timeout
	require.NoError(t, err)
	require.NotNil(t, cbProducer)

	// Verify it uses max timeout by checking context deadline
	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	message := []byte("test message")

	mockProducer.EXPECT().
		ProducerDefaultWithContext(gomock.Any(), exchange, key, message).
		DoAndReturn(func(ctx context.Context, exchange, key string, message []byte) (*string, error) {
			deadline, ok := ctx.Deadline()
			assert.True(t, ok, "context should have a deadline")
			// Should use max timeout, not the excessive 100s
			assert.WithinDuration(t, time.Now().Add(MaxOperationTimeout), deadline, 50*time.Millisecond)
			return nil, nil
		})

	_, err = cbProducer.ProducerDefaultWithContext(ctx, exchange, key, message)
	assert.NoError(t, err)
}
