package bootstrap

import (
	"errors"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testStateListener implements libCircuitBreaker.StateChangeListener for testing.
// Uses atomic counter for thread safety since lib-commons calls OnStateChange in a goroutine.
type testStateListener struct {
	calls atomic.Int32
}

func (t *testStateListener) OnStateChange(serviceName string, from, to libCircuitBreaker.State) {
	t.calls.Add(1)
}

func (t *testStateListener) getCalls() int {
	return int(t.calls.Load())
}

// testCircuitBreakerConfig returns a standard config for testing
func testCircuitBreakerConfig() rabbitmq.CircuitBreakerConfig {
	return rabbitmq.CircuitBreakerConfig{
		ConsecutiveFailures: 15,
		FailureRatio:        0.5,
		Interval:            2 * time.Minute,
		MaxRequests:         3,
		MinRequests:         10,
		Timeout:             30 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		HealthCheckTimeout:  10 * time.Second,
	}
}

// testRabbitMQConnection returns a standard connection for testing
func testRabbitMQConnection() *libRabbitmq.RabbitMQConnection {
	return &libRabbitmq.RabbitMQConnection{
		HealthCheckURL: "http://localhost:15672",
		User:           "guest",
		Pass:           "guest",
	}
}

func TestNewCircuitBreakerManager_CreatesManagerSuccessfully(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()

	conn := testRabbitMQConnection()
	cbConfig := testCircuitBreakerConfig()

	cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, nil)

	require.NoError(t, err)
	assert.NotNil(t, cbm)
	assert.NotNil(t, cbm.Manager)
	assert.NotNil(t, cbm.HealthChecker)
}

func TestNewCircuitBreakerManager_ReturnsErrorOnNilLogger(t *testing.T) {
	conn := testRabbitMQConnection()
	cbConfig := testCircuitBreakerConfig()

	cbm, err := NewCircuitBreakerManager(nil, conn, cbConfig, nil)

	assert.Nil(t, cbm)
	assert.ErrorIs(t, err, ErrNilLogger)
}

func TestNewCircuitBreakerManager_ReturnsErrorOnNilRabbitConn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	cbConfig := testCircuitBreakerConfig()

	cbm, err := NewCircuitBreakerManager(logger, nil, cbConfig, nil)

	assert.Nil(t, cbm)
	assert.ErrorIs(t, err, ErrNilRabbitConn)
}

func TestNewCircuitBreakerManager_RegistersStateListener(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()

	conn := testRabbitMQConnection()
	listener := &testStateListener{}
	cbConfig := testCircuitBreakerConfig()

	cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, listener)

	require.NoError(t, err)
	assert.NotNil(t, cbm)
	assert.NotNil(t, cbm.Manager)

	// Verify circuit breaker state is accessible (confirms manager is properly initialized)
	state := cbm.Manager.GetState(rabbitmq.CircuitBreakerServiceName)
	assert.Equal(t, libCircuitBreaker.StateClosed, state, "circuit breaker should start in closed state")
}

func TestNewCircuitBreakerManager_CircuitTripsAfterConsecutiveFailures(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()
	logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Warnf(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	// Connection with logger to prevent panic during health checks
	conn := &libRabbitmq.RabbitMQConnection{
		HealthCheckURL: "http://localhost:15672",
		User:           "guest",
		Pass:           "guest",
		Logger:         logger, // Required for health check logging
	}
	listener := &testStateListener{}

	// Use config with low consecutive failures to easily trip the circuit
	cbConfig := rabbitmq.CircuitBreakerConfig{
		ConsecutiveFailures: 3,
		FailureRatio:        0.5,
		Interval:            2 * time.Minute,
		MaxRequests:         3,
		MinRequests:         10,
		Timeout:             30 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		HealthCheckTimeout:  10 * time.Second,
	}

	cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, listener)
	require.NoError(t, err)
	defer cbm.Stop() // Ensure health checker is stopped after test

	// Initial state should be closed
	initialState := cbm.Manager.GetState(rabbitmq.CircuitBreakerServiceName)
	assert.Equal(t, libCircuitBreaker.StateClosed, initialState, "circuit should start in closed state")

	// Trip the circuit by recording consecutive failures
	for i := 0; i < 3; i++ {
		_, err := cbm.Manager.Execute(rabbitmq.CircuitBreakerServiceName, func() (any, error) {
			return nil, errors.New("simulated failure")
		})
		require.Error(t, err, "iteration %d should fail", i)
	}

	// Verify the circuit is now open
	state := cbm.Manager.GetState(rabbitmq.CircuitBreakerServiceName)
	assert.Equal(t, libCircuitBreaker.StateOpen, state, "circuit should be open after consecutive failures")

	// Verify the circuit is unhealthy
	assert.False(t, cbm.Manager.IsHealthy(rabbitmq.CircuitBreakerServiceName), "circuit should be unhealthy when open")

	// Wait briefly for async state change notification (lib-commons calls listeners in a goroutine)
	time.Sleep(50 * time.Millisecond)

	// Verify the state listener was invoked (use thread-safe getter)
	assert.Greater(t, listener.getCalls(), 0, "state listener should have been called on state change")
}

func TestCircuitBreakerManager_StartStop_DoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()
	// Allow any Info() calls (lib-commons health checker also logs)
	logger.EXPECT().Info(gomock.Any()).AnyTimes()

	conn := testRabbitMQConnection()
	cbConfig := testCircuitBreakerConfig()

	cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, nil)
	require.NoError(t, err)

	// Verify Start and Stop can be called without panicking
	cbm.Start()
	cbm.Stop()

	// Verify circuit breaker is still functional after lifecycle
	state := cbm.Manager.GetState(rabbitmq.CircuitBreakerServiceName)
	assert.Equal(t, libCircuitBreaker.StateClosed, state)
}

func TestCircuitBreakerManager_Stop_WithoutStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()
	// Allow any Info() calls (lib-commons health checker also logs)
	logger.EXPECT().Info(gomock.Any()).AnyTimes()

	conn := testRabbitMQConnection()
	cbConfig := testCircuitBreakerConfig()

	cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, nil)
	require.NoError(t, err)

	// Stop without Start should not panic
	cbm.Stop()

	// Verify circuit breaker is still functional after stop
	state := cbm.Manager.GetState(rabbitmq.CircuitBreakerServiceName)
	assert.Equal(t, libCircuitBreaker.StateClosed, state)
}

func TestCircuitBreakerManager_CircuitBreakerServiceNameIsRegistered(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()

	conn := testRabbitMQConnection()
	cbConfig := testCircuitBreakerConfig()

	cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, nil)
	require.NoError(t, err)

	// Verify the circuit breaker is registered with the correct service name
	isHealthy := cbm.Manager.IsHealthy(rabbitmq.CircuitBreakerServiceName)
	assert.True(t, isHealthy, "newly created circuit breaker should be healthy (closed)")
}

func TestNewCircuitBreakerManager_UsesDefaultHealthCheckInterval(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()

	conn := testRabbitMQConnection()
	cbConfig := rabbitmq.CircuitBreakerConfig{
		ConsecutiveFailures: 15,
		FailureRatio:        0.5,
		Interval:            2 * time.Minute,
		MaxRequests:         3,
		MinRequests:         10,
		Timeout:             30 * time.Second,
		HealthCheckInterval: 0, // Zero to trigger default
		HealthCheckTimeout:  10 * time.Second,
	}

	cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, nil)

	require.NoError(t, err)
	assert.NotNil(t, cbm)
	assert.NotNil(t, cbm.HealthChecker)
}

func TestNewCircuitBreakerManager_UsesDefaultHealthCheckTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()

	conn := testRabbitMQConnection()
	cbConfig := rabbitmq.CircuitBreakerConfig{
		ConsecutiveFailures: 15,
		FailureRatio:        0.5,
		Interval:            2 * time.Minute,
		MaxRequests:         3,
		MinRequests:         10,
		Timeout:             30 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		HealthCheckTimeout:  0, // Zero to trigger default
	}

	cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, nil)

	require.NoError(t, err)
	assert.NotNil(t, cbm)
	assert.NotNil(t, cbm.HealthChecker)
}

func TestNewCircuitBreakerManager_ReturnsErrorOnInvalidFailureRatio(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	conn := testRabbitMQConnection()

	tests := []struct {
		name         string
		failureRatio float64
	}{
		{"negative ratio", -0.1},
		{"ratio greater than 1", 1.1},
		{"ratio equals 2", 2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cbConfig := rabbitmq.CircuitBreakerConfig{
				ConsecutiveFailures: 15,
				FailureRatio:        tt.failureRatio,
				Interval:            2 * time.Minute,
				MaxRequests:         3,
				MinRequests:         10,
				Timeout:             30 * time.Second,
			}

			cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, nil)

			assert.Nil(t, cbm)
			assert.ErrorIs(t, err, ErrInvalidFailureRatio)
		})
	}
}

func TestNewCircuitBreakerManager_ReturnsErrorOnZeroConsecutiveFailures(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	conn := testRabbitMQConnection()
	cbConfig := rabbitmq.CircuitBreakerConfig{
		ConsecutiveFailures: 0, // Invalid: must be > 0
		FailureRatio:        0.5,
		Interval:            2 * time.Minute,
		MaxRequests:         3,
		MinRequests:         10,
		Timeout:             30 * time.Second,
	}

	cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, nil)

	assert.Nil(t, cbm)
	assert.ErrorIs(t, err, ErrInvalidConsecutiveFailures)
}

func TestNewCircuitBreakerManager_ReturnsErrorOnInvalidTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	conn := testRabbitMQConnection()

	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{"zero timeout", 0},
		{"negative timeout", -1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cbConfig := rabbitmq.CircuitBreakerConfig{
				ConsecutiveFailures: 15,
				FailureRatio:        0.5,
				Interval:            2 * time.Minute,
				MaxRequests:         3,
				MinRequests:         10,
				Timeout:             tt.timeout,
			}

			cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, nil)

			assert.Nil(t, cbm)
			assert.ErrorIs(t, err, ErrInvalidTimeout)
		})
	}
}

func TestNewCircuitBreakerManager_ReturnsErrorOnInvalidInterval(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	conn := testRabbitMQConnection()

	tests := []struct {
		name     string
		interval time.Duration
	}{
		{"zero interval", 0},
		{"negative interval", -1 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cbConfig := rabbitmq.CircuitBreakerConfig{
				ConsecutiveFailures: 15,
				FailureRatio:        0.5,
				Interval:            tt.interval,
				MaxRequests:         3,
				MinRequests:         10,
				Timeout:             30 * time.Second,
			}

			cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, nil)

			assert.Nil(t, cbm)
			assert.ErrorIs(t, err, ErrInvalidInterval)
		})
	}
}

func TestNewCircuitBreakerManager_AcceptsValidBoundaryValues(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()

	conn := testRabbitMQConnection()

	tests := []struct {
		name   string
		config rabbitmq.CircuitBreakerConfig
	}{
		{
			name: "failure ratio at 0",
			config: rabbitmq.CircuitBreakerConfig{
				ConsecutiveFailures: 1,
				FailureRatio:        0.0,
				Interval:            1 * time.Second,
				MaxRequests:         1,
				MinRequests:         1,
				Timeout:             1 * time.Second,
			},
		},
		{
			name: "failure ratio at 1.0",
			config: rabbitmq.CircuitBreakerConfig{
				ConsecutiveFailures: 1,
				FailureRatio:        1.0,
				Interval:            1 * time.Second,
				MaxRequests:         1,
				MinRequests:         1,
				Timeout:             1 * time.Second,
			},
		},
		{
			name: "minimum consecutive failures",
			config: rabbitmq.CircuitBreakerConfig{
				ConsecutiveFailures: 1,
				FailureRatio:        0.5,
				Interval:            1 * time.Second,
				MaxRequests:         1,
				MinRequests:         1,
				Timeout:             1 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cbm, err := NewCircuitBreakerManager(logger, conn, tt.config, nil)

			require.NoError(t, err)
			assert.NotNil(t, cbm)
		})
	}
}

func TestNewCircuitBreakerManager_ReturnsErrorOnZeroMaxRequests(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	conn := testRabbitMQConnection()
	cbConfig := rabbitmq.CircuitBreakerConfig{
		ConsecutiveFailures: 15,
		FailureRatio:        0.5,
		Interval:            2 * time.Minute,
		MaxRequests:         0, // Invalid: must be > 0
		MinRequests:         10,
		Timeout:             30 * time.Second,
	}

	cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, nil)

	assert.Nil(t, cbm)
	assert.ErrorIs(t, err, ErrInvalidMaxRequests)
}

func TestNewCircuitBreakerManager_ReturnsErrorOnZeroMinRequestsWithFailureRatio(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	conn := testRabbitMQConnection()
	cbConfig := rabbitmq.CircuitBreakerConfig{
		ConsecutiveFailures: 15,
		FailureRatio:        0.5, // FailureRatio > 0 requires MinRequests > 0
		Interval:            2 * time.Minute,
		MaxRequests:         3,
		MinRequests:         0, // Invalid when FailureRatio > 0
		Timeout:             30 * time.Second,
	}

	cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, nil)

	assert.Nil(t, cbm)
	assert.ErrorIs(t, err, ErrInvalidMinRequests)
}

func TestNewCircuitBreakerManager_AcceptsZeroMinRequestsWithZeroFailureRatio(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()

	conn := testRabbitMQConnection()
	cbConfig := rabbitmq.CircuitBreakerConfig{
		ConsecutiveFailures: 15,
		FailureRatio:        0.0, // When FailureRatio is 0, MinRequests can be 0
		Interval:            2 * time.Minute,
		MaxRequests:         3,
		MinRequests:         0, // Valid when FailureRatio is 0
		Timeout:             30 * time.Second,
	}

	cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, nil)

	require.NoError(t, err)
	assert.NotNil(t, cbm)
}

func TestNewCircuitBreakerRunnable_CreatesWrapper(t *testing.T) {
	runnable := NewCircuitBreakerRunnable(nil)

	assert.NotNil(t, runnable)
	assert.Nil(t, runnable.manager)
}

func TestNewCircuitBreakerRunnable_WithManager(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()

	conn := testRabbitMQConnection()
	cbConfig := testCircuitBreakerConfig()

	cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, nil)
	require.NoError(t, err)

	runnable := NewCircuitBreakerRunnable(cbm)

	assert.NotNil(t, runnable)
	assert.NotNil(t, runnable.manager)
	assert.Equal(t, cbm, runnable.manager)
}

func TestCircuitBreakerRunnable_Run_WithValidManager(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()

	conn := testRabbitMQConnection()
	cbConfig := testCircuitBreakerConfig()

	cbm, err := NewCircuitBreakerManager(logger, conn, cbConfig, nil)
	require.NoError(t, err)

	runnable := NewCircuitBreakerRunnable(cbm)

	// Channel to track when Run completes
	done := make(chan error, 1)

	// Run in a goroutine
	go func() {
		done <- runnable.Run(nil)
	}()

	// Wait briefly for it to start
	time.Sleep(50 * time.Millisecond)

	// Send interrupt signal to trigger shutdown
	err = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	require.NoError(t, err)

	// Assert Run returns without error within a timeout
	select {
	case runErr := <-done:
		assert.NoError(t, runErr)
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within timeout")
	}
}

func TestCircuitBreakerRunnable_Run_WithNilManager(t *testing.T) {
	runnable := NewCircuitBreakerRunnable(nil)

	// Should return nil without blocking when manager is nil
	err := runnable.Run(nil)

	assert.NoError(t, err)
}
