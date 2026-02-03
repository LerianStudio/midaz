package bootstrap

import (
	"errors"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/mcircuitbreaker"
)

var (
	// ErrNilLogger indicates that the logger parameter is nil.
	ErrNilLogger = errors.New("logger cannot be nil")
	// ErrNilRabbitConn indicates that the RabbitMQ connection parameter is nil.
	ErrNilRabbitConn = errors.New("rabbitConn cannot be nil")
)

const (
	// DefaultHealthCheckInterval is the default interval for health checker runs.
	DefaultHealthCheckInterval = 30 * time.Second
	// DefaultHealthCheckTimeout is the default timeout for each health check operation.
	DefaultHealthCheckTimeout = 10 * time.Second
)

// CircuitBreakerManager manages the circuit breaker infrastructure for RabbitMQ.
// It coordinates the circuit breaker manager and health checker lifecycle.
type CircuitBreakerManager struct {
	Manager       libCircuitBreaker.Manager
	HealthChecker libCircuitBreaker.HealthChecker
	logger        libLog.Logger
}

// NewCircuitBreakerManager creates a new circuit breaker manager with health checking.
// The stateListener parameter is optional - pass nil if you don't need state change notifications.
func NewCircuitBreakerManager(
	logger libLog.Logger,
	rabbitConn *libRabbitmq.RabbitMQConnection,
	cbConfig rabbitmq.CircuitBreakerConfig,
	stateListener mcircuitbreaker.StateListener,
) (*CircuitBreakerManager, error) {
	// Validate required parameters
	if logger == nil {
		return nil, ErrNilLogger
	}

	if rabbitConn == nil {
		return nil, ErrNilRabbitConn
	}

	// Create circuit breaker manager
	cbManager := libCircuitBreaker.NewManager(logger)

	// Initialize circuit breaker for RabbitMQ with provided config
	cbManager.GetOrCreate(rabbitmq.CircuitBreakerServiceName, rabbitmq.RabbitMQCircuitBreakerConfig(cbConfig))

	// Register state change listener if provided
	if stateListener != nil {
		adapter := mcircuitbreaker.NewLibCommonsAdapter(stateListener)
		cbManager.RegisterStateChangeListener(adapter)
	}

	// Determine health check interval and timeout (use config values or defaults)
	healthCheckInterval := cbConfig.HealthCheckInterval
	if healthCheckInterval == 0 {
		healthCheckInterval = DefaultHealthCheckInterval
	}

	healthCheckTimeout := cbConfig.HealthCheckTimeout
	if healthCheckTimeout == 0 {
		healthCheckTimeout = DefaultHealthCheckTimeout
	}

	// Create health checker
	healthChecker, err := libCircuitBreaker.NewHealthCheckerWithValidation(
		cbManager,
		healthCheckInterval,
		healthCheckTimeout,
		logger,
	)
	if err != nil {
		return nil, err
	}

	// Register RabbitMQ health check function
	healthCheckFn := rabbitmq.NewRabbitMQHealthCheckFunc(rabbitConn)
	healthChecker.Register(rabbitmq.CircuitBreakerServiceName, healthCheckFn)

	// Register health checker as state change listener for immediate recovery attempts
	cbManager.RegisterStateChangeListener(healthChecker)

	return &CircuitBreakerManager{
		Manager:       cbManager,
		HealthChecker: healthChecker,
		logger:        logger,
	}, nil
}

// Start begins the health checker background process.
func (cbm *CircuitBreakerManager) Start() {
	cbm.logger.Info("Starting circuit breaker health checker")
	cbm.HealthChecker.Start()
}

// Stop gracefully stops the health checker.
func (cbm *CircuitBreakerManager) Stop() {
	cbm.logger.Info("Stopping circuit breaker health checker")
	cbm.HealthChecker.Stop()
}
