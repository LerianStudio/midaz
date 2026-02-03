package bootstrap

import (
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

// testStateListener implements libCircuitBreaker.StateChangeListener for testing
type testStateListener struct {
	calls int
}

func (t *testStateListener) OnStateChange(serviceName string, from, to libCircuitBreaker.State, counts libCircuitBreaker.Counts) {
	t.calls++
}

// testCircuitBreakerConfig returns a standard config for testing
func testCircuitBreakerConfig() rabbitmq.CircuitBreakerConfig {
	return rabbitmq.CircuitBreakerConfig{
		ConsecutiveFailures:  15,
		FailureRatio:         0.5,
		Interval:             2 * time.Minute,
		MaxRequests:          3,
		MinRequests:          10,
		Timeout:              30 * time.Second,
		HealthCheckInterval:  30 * time.Second,
		HealthCheckTimeout:   10 * time.Second,
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

func TestCircuitBreakerRunnable_Run_WithNilManager(t *testing.T) {
	runnable := NewCircuitBreakerRunnable(nil)

	// Should return nil without blocking when manager is nil
	err := runnable.Run(nil)

	assert.NoError(t, err)
}
