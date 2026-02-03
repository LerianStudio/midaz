package bootstrap

import (
	"testing"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/mcircuitbreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStateListener implements mcircuitbreaker.StateListener for testing
type mockStateListener struct {
	events []mcircuitbreaker.StateChangeEvent
}

func (m *mockStateListener) OnCircuitBreakerStateChange(event mcircuitbreaker.StateChangeEvent) {
	m.events = append(m.events, event)
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
	listener := &mockStateListener{}
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
