package rabbitmq

import (
	"context"
	"errors"
	"testing"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestProducerCircuitBreaker_ProducerDefault_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockProducerRepository(ctrl)
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)
	manager := libCircuitBreaker.NewManager(logger)
	cb := manager.GetOrCreate("rabbitmq-test-success", libCircuitBreaker.DefaultConfig())

	wrapper, err := NewProducerCircuitBreaker(mockRepo, cb)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test-key"
	message := []byte("test-message")

	mockRepo.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, nil).
		Times(1)

	result, err := wrapper.ProducerDefault(ctx, exchange, key, message)

	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestProducerCircuitBreaker_ProducerDefault_ReturnsMessageID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockProducerRepository(ctrl)
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)
	manager := libCircuitBreaker.NewManager(logger)
	cb := manager.GetOrCreate("rabbitmq-test-msgid", libCircuitBreaker.DefaultConfig())

	wrapper, err := NewProducerCircuitBreaker(mockRepo, cb)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test-key"
	message := []byte("test-message")
	msgID := "msg-123"

	mockRepo.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(&msgID, nil).
		Times(1)

	result, err := wrapper.ProducerDefault(ctx, exchange, key, message)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "msg-123", *result)
}

func TestProducerCircuitBreaker_ProducerDefault_Error(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockProducerRepository(ctrl)
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)
	manager := libCircuitBreaker.NewManager(logger)
	cb := manager.GetOrCreate("rabbitmq-test-error", libCircuitBreaker.DefaultConfig())

	wrapper, err := NewProducerCircuitBreaker(mockRepo, cb)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test-key"
	message := []byte("test-message")
	expectedErr := errors.New("connection failed")

	mockRepo.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, expectedErr).
		Times(1)

	result, err := wrapper.ProducerDefault(ctx, exchange, key, message)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "connection failed")
}

func TestProducerCircuitBreaker_CircuitOpens_AfterConsecutiveFailures(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockProducerRepository(ctrl)
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)
	manager := libCircuitBreaker.NewManager(logger)

	config := libCircuitBreaker.Config{
		MaxRequests:         1,
		Interval:            0,
		Timeout:             0,
		ConsecutiveFailures: 3,
		FailureRatio:        0.5,
		MinRequests:         1,
	}
	cb := manager.GetOrCreate("rabbitmq-test-circuit", config)

	wrapper, err := NewProducerCircuitBreaker(mockRepo, cb)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test-key"
	message := []byte("test-message")
	expectedErr := errors.New("connection failed")

	// Mock allows calls until circuit opens - circuit may open during the 3rd failure processing
	mockRepo.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, expectedErr).
		MinTimes(1).
		MaxTimes(3)

	// Trigger consecutive failures to open the circuit
	for i := 0; i < 3; i++ {
		_, err := wrapper.ProducerDefault(ctx, exchange, key, message)
		assert.Error(t, err, "attempt %d should return error", i+1)
	}

	state := cb.State()
	assert.Equal(t, libCircuitBreaker.StateOpen, state, "circuit should be open after consecutive failures")

	// When circuit is open, request should fail immediately without calling mock
	start := time.Now()
	_, err = wrapper.ProducerDefault(ctx, exchange, key, message)
	fastFailDuration := time.Since(start)

	assert.Error(t, err, "should return error when circuit is open")
	assert.Less(t, fastFailDuration, 100*time.Millisecond, "fast-fail should be quick")
}

func TestProducerCircuitBreaker_CheckRabbitMQHealth_ReturnsTrue(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockProducerRepository(ctrl)
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)
	manager := libCircuitBreaker.NewManager(logger)
	cb := manager.GetOrCreate("rabbitmq-test-health-true", libCircuitBreaker.DefaultConfig())

	wrapper, err := NewProducerCircuitBreaker(mockRepo, cb)
	require.NoError(t, err)

	mockRepo.EXPECT().
		CheckRabbitMQHealth().
		Return(true).
		Times(1)

	result := wrapper.CheckRabbitMQHealth()

	assert.True(t, result)
}

func TestProducerCircuitBreaker_CheckRabbitMQHealth_ReturnsFalse(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockProducerRepository(ctrl)
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)
	manager := libCircuitBreaker.NewManager(logger)
	cb := manager.GetOrCreate("rabbitmq-test-health-false", libCircuitBreaker.DefaultConfig())

	wrapper, err := NewProducerCircuitBreaker(mockRepo, cb)
	require.NoError(t, err)

	mockRepo.EXPECT().
		CheckRabbitMQHealth().
		Return(false).
		Times(1)

	result := wrapper.CheckRabbitMQHealth()

	assert.False(t, result)
}

func TestNewProducerCircuitBreaker_ValidParameters(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockProducerRepository(ctrl)
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)
	manager := libCircuitBreaker.NewManager(logger)
	cb := manager.GetOrCreate("rabbitmq-test-new", libCircuitBreaker.DefaultConfig())

	wrapper, err := NewProducerCircuitBreaker(mockRepo, cb)
	require.NoError(t, err)

	require.NotNil(t, wrapper)
	assert.NotNil(t, wrapper.repo)
	assert.NotNil(t, wrapper.cb)
}

func TestNewProducerCircuitBreaker_NilRepo_ReturnsError(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)
	manager := libCircuitBreaker.NewManager(logger)
	cb := manager.GetOrCreate("rabbitmq-test-nil-repo", libCircuitBreaker.DefaultConfig())

	wrapper, err := NewProducerCircuitBreaker(nil, cb)

	assert.Error(t, err)
	assert.Nil(t, wrapper)
	assert.Contains(t, err.Error(), "repo cannot be nil")
}

func TestNewProducerCircuitBreaker_NilCircuitBreaker_ReturnsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockProducerRepository(ctrl)

	wrapper, err := NewProducerCircuitBreaker(mockRepo, nil)

	assert.Error(t, err)
	assert.Nil(t, wrapper)
	assert.Contains(t, err.Error(), "cb cannot be nil")
}

func TestProducerCircuitBreaker_ProducerDefault_WithCancelledContext(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockProducerRepository(ctrl)
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)
	manager := libCircuitBreaker.NewManager(logger)
	cb := manager.GetOrCreate("rabbitmq-test-cancelled", libCircuitBreaker.DefaultConfig())

	wrapper, err := NewProducerCircuitBreaker(mockRepo, cb)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exchange := "test-exchange"
	key := "test-key"
	message := []byte("test-message")
	expectedErr := context.Canceled

	mockRepo.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, expectedErr).
		Times(1)

	result, err := wrapper.ProducerDefault(ctx, exchange, key, message)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestProducerCircuitBreaker_ProducerDefault_EmptyInputs(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockProducerRepository(ctrl)
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)
	manager := libCircuitBreaker.NewManager(logger)
	cb := manager.GetOrCreate("rabbitmq-test-empty", libCircuitBreaker.DefaultConfig())

	wrapper, err := NewProducerCircuitBreaker(mockRepo, cb)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := ""
	key := ""
	message := []byte{}

	mockRepo.EXPECT().
		ProducerDefault(ctx, exchange, key, message).
		Return(nil, nil).
		Times(1)

	result, err := wrapper.ProducerDefault(ctx, exchange, key, message)

	assert.NoError(t, err)
	assert.Nil(t, result)
}

// stubCircuitBreakerWithWrongType is a test stub that returns an unexpected type
// to test the type assertion error path.
type stubCircuitBreakerWithWrongType struct{}

func (s *stubCircuitBreakerWithWrongType) Execute(_ func() (any, error)) (any, error) {
	return 12345, nil
}

func (s *stubCircuitBreakerWithWrongType) State() libCircuitBreaker.State {
	return libCircuitBreaker.StateClosed
}

func (s *stubCircuitBreakerWithWrongType) Counts() libCircuitBreaker.Counts {
	return libCircuitBreaker.Counts{}
}

func TestProducerCircuitBreaker_ProducerDefault_TypeAssertionError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockProducerRepository(ctrl)
	stubCB := &stubCircuitBreakerWithWrongType{}

	wrapper, err := NewProducerCircuitBreaker(mockRepo, stubCB)
	require.NoError(t, err)

	ctx := context.Background()
	exchange := "test-exchange"
	key := "test-key"
	message := []byte("test-message")

	result, err := wrapper.ProducerDefault(ctx, exchange, key, message)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unexpected result type")
	assert.Contains(t, err.Error(), "int")
}
