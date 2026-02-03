package rabbitmq

import (
	"context"
	"errors"
	"testing"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
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

func TestCircuitBreakerProducer_ImplementsProducerRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := libCircuitBreaker.NewManager(logger)

	cbProducer := NewCircuitBreakerProducer(mockProducer, cbManager, logger, nil)

	// Verify it implements the interface
	var _ ProducerRepository = cbProducer
}

func TestCircuitBreakerProducer_ProducerDefault_SuccessPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := libCircuitBreaker.NewManager(logger)

	cbProducer := NewCircuitBreakerProducer(mockProducer, cbManager, logger, nil)

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
	cbManager := libCircuitBreaker.NewManager(logger)

	cbProducer := NewCircuitBreakerProducer(mockProducer, cbManager, logger, nil)

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
	cbManager := libCircuitBreaker.NewManager(logger)

	cbProducer := NewCircuitBreakerProducer(mockProducer, cbManager, logger, nil)

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
		cbProducer.ProducerDefault(ctx, exchange, key, message)
	}

	// Now when circuit is open, no calls to underlying producer
	// ProducerDefault should not be called again (circuit is open)
	_, err := cbProducer.ProducerDefault(ctx, exchange, key, message)

	// Verify error indicates circuit is open
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker open")
}

func TestCircuitBreakerProducer_CheckRabbitMQHealth_DelegatesToUnderlying(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := libCircuitBreaker.NewManager(logger)

	cbProducer := NewCircuitBreakerProducer(mockProducer, cbManager, logger, nil)

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
	cbManager := libCircuitBreaker.NewManager(logger)

	cbProducer := NewCircuitBreakerProducer(mockProducer, cbManager, logger, nil)

	// Initial state should be closed
	state := cbProducer.GetCircuitState()
	assert.Equal(t, libCircuitBreaker.StateClosed, state)
}

func TestCircuitBreakerProducer_IsCircuitHealthy_ReturnsTrueWhenClosed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := libCircuitBreaker.NewManager(logger)

	cbProducer := NewCircuitBreakerProducer(mockProducer, cbManager, logger, nil)

	// Initial state should be healthy (closed)
	assert.True(t, cbProducer.IsCircuitHealthy())
}

func TestCircuitBreakerProducer_IsCircuitHealthy_ReturnsFalseWhenOpen(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProducer := NewMockProducerRepository(ctrl)
	logger := setupMockLogger(ctrl)
	cbManager := libCircuitBreaker.NewManager(logger)

	cbProducer := NewCircuitBreakerProducer(mockProducer, cbManager, logger, nil)

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
	cbManager := libCircuitBreaker.NewManager(logger)

	cbProducer := NewCircuitBreakerProducer(mockProducer, cbManager, logger, nil)

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

	_, err := cbProducer.ProducerDefault(ctx, exchange, key, message)
	require.NoError(t, err)

	// Verify counts updated
	countsAfterSuccess := cbProducer.GetCounts()
	assert.Equal(t, uint32(1), countsAfterSuccess.Requests)
	assert.Equal(t, uint32(1), countsAfterSuccess.TotalSuccesses)
	assert.Equal(t, uint32(0), countsAfterSuccess.TotalFailures)

	// Make a failing call
	mockProducer.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, errors.New("connection refused"))

	_, err = cbProducer.ProducerDefault(ctx, exchange, key, message)
	require.Error(t, err)

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
	cbManager := libCircuitBreaker.NewManager(logger)

	cbProducer := NewCircuitBreakerProducer(mockProducer, cbManager, logger, nil)

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
	cbManager := libCircuitBreaker.NewManager(logger)

	cbProducer := NewCircuitBreakerProducer(mockProducer, cbManager, logger, nil)

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
	cbManager := libCircuitBreaker.NewManager(logger)

	cbProducer := NewCircuitBreakerProducer(mockProducer, cbManager, logger, nil)

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
	cbManager := libCircuitBreaker.NewManager(logger)

	cbProducer := NewCircuitBreakerProducer(mockProducer, cbManager, logger, nil)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test.key"
	var nilMessage []byte = nil

	// Setup: Mock accepts nil message
	mockProducer.EXPECT().
		ProducerDefault(ctx, exchange, key, nilMessage).
		Return(nil, nil)

	// Execute
	result, err := cbProducer.ProducerDefault(ctx, exchange, key, nilMessage)

	// Verify - circuit breaker passes through nil messages
	assert.NoError(t, err)
	assert.Nil(t, result)
}
