// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libCircuitBreaker "github.com/LerianStudio/lib-commons/v4/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libRabbitmq "github.com/LerianStudio/lib-commons/v4/commons/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/rabbitmq"
)

var (
	// ErrNilLogger indicates that the logger parameter is nil.
	ErrNilLogger = errors.New("logger cannot be nil")
	// ErrNilRabbitConn indicates that the RabbitMQ connection parameter is nil.
	ErrNilRabbitConn = errors.New("rabbitConn cannot be nil")
	// ErrInvalidFailureRatio indicates that FailureRatio is outside valid bounds (0.0-1.0).
	ErrInvalidFailureRatio = errors.New("failure_ratio must be between 0.0 and 1.0")
	// ErrInvalidConsecutiveFailures indicates that ConsecutiveFailures must be greater than 0.
	ErrInvalidConsecutiveFailures = errors.New("consecutive_failures must be greater than 0")
	// ErrInvalidMaxRequests indicates that MaxRequests must be greater than 0.
	ErrInvalidMaxRequests = errors.New("max_requests must be greater than 0")
	// ErrInvalidTimeout indicates that Timeout must be positive.
	ErrInvalidTimeout = errors.New("timeout must be positive")
	// ErrInvalidInterval indicates that Interval must be positive.
	ErrInvalidInterval = errors.New("interval must be positive")
	// ErrInvalidMinRequests indicates that MinRequests must be greater than 0 when FailureRatio is used.
	ErrInvalidMinRequests = errors.New("min_requests must be greater than 0 when failure_ratio is configured")
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
	stateListener libCircuitBreaker.StateChangeListener,
) (*CircuitBreakerManager, error) {
	if err := validateCircuitBreakerManagerInputs(logger, rabbitConn, cbConfig); err != nil {
		return nil, err
	}

	cbConfig = normalizeCircuitBreakerConfig(cbConfig)

	// Create circuit breaker manager
	cbManager, err := libCircuitBreaker.NewManager(logger)
	if err != nil {
		return nil, err
	}

	// Initialize circuit breaker for RabbitMQ with provided config
	if _, err = cbManager.GetOrCreate(rabbitmq.CircuitBreakerServiceName, rabbitmq.RabbitMQCircuitBreakerConfig(cbConfig)); err != nil {
		return nil, err
	}

	// Register state change listener if provided
	if stateListener != nil {
		cbManager.RegisterStateChangeListener(stateListener)
	}

	healthCheckInterval, healthCheckTimeout := resolveHealthCheckSettings(cbConfig)

	// Create underlying health checker
	underlyingHealthChecker, err := libCircuitBreaker.NewHealthCheckerWithValidation(
		cbManager,
		healthCheckInterval,
		healthCheckTimeout,
		logger,
	)
	if err != nil {
		return nil, err
	}

	// Wrap with state-aware health checker that starts/stops based on circuit state
	// Pass cbManager so it can detect when circuits are reset (lib-commons Reset() doesn't trigger listeners)
	stateAwareHealthChecker, err := rabbitmq.NewStateAwareHealthChecker(underlyingHealthChecker, cbManager, logger)
	if err != nil {
		return nil, err
	}

	// Register RabbitMQ health check function
	healthCheckFn := rabbitmq.NewRabbitMQHealthCheckFunc(rabbitConn)
	stateAwareHealthChecker.Register(rabbitmq.CircuitBreakerServiceName, healthCheckFn)

	// Register state-aware health checker as state change listener
	// This enables dynamic start/stop based on circuit state
	cbManager.RegisterStateChangeListener(stateAwareHealthChecker)

	return &CircuitBreakerManager{
		Manager:       cbManager,
		HealthChecker: stateAwareHealthChecker,
		logger:        logger,
	}, nil
}

func validateCircuitBreakerManagerInputs(
	logger libLog.Logger,
	rabbitConn *libRabbitmq.RabbitMQConnection,
	cbConfig rabbitmq.CircuitBreakerConfig,
) error {
	if logger == nil {
		return ErrNilLogger
	}

	if rabbitConn == nil {
		return ErrNilRabbitConn
	}

	if cbConfig.FailureRatio < 0 || cbConfig.FailureRatio > 1.0 {
		return ErrInvalidFailureRatio
	}

	if cbConfig.ConsecutiveFailures == 0 {
		return ErrInvalidConsecutiveFailures
	}

	if cbConfig.MaxRequests == 0 {
		return ErrInvalidMaxRequests
	}

	if cbConfig.Timeout <= 0 {
		return ErrInvalidTimeout
	}

	if cbConfig.Interval <= 0 {
		return ErrInvalidInterval
	}

	if cbConfig.FailureRatio > 0 && cbConfig.MinRequests == 0 {
		return ErrInvalidMinRequests
	}

	return nil
}

func normalizeCircuitBreakerConfig(cbConfig rabbitmq.CircuitBreakerConfig) rabbitmq.CircuitBreakerConfig {
	if cbConfig.FailureRatio == 0 {
		cbConfig.MinRequests = 0
	}

	return cbConfig
}

func resolveHealthCheckSettings(cbConfig rabbitmq.CircuitBreakerConfig) (time.Duration, time.Duration) {
	interval := cbConfig.HealthCheckInterval
	if interval == 0 {
		interval = DefaultHealthCheckInterval
	}

	timeout := cbConfig.HealthCheckTimeout
	if timeout == 0 {
		timeout = DefaultHealthCheckTimeout
	}

	return interval, timeout
}

// Start begins the health checker background process.
func (cbm *CircuitBreakerManager) Start() {
	cbm.logger.Log(context.Background(), libLog.LevelInfo, "Starting circuit breaker manager")
	cbm.HealthChecker.Start()
}

// Stop gracefully stops the health checker.
func (cbm *CircuitBreakerManager) Stop() {
	cbm.logger.Log(context.Background(), libLog.LevelInfo, "Stopping circuit breaker manager")
	cbm.HealthChecker.Stop()
}

// CircuitBreakerRunnable wraps CircuitBreakerManager to implement the Runnable interface
// for integration with the launcher in unified ledger mode.
type CircuitBreakerRunnable struct {
	manager *CircuitBreakerManager
}

// NewCircuitBreakerRunnable creates a new runnable wrapper for the circuit breaker manager.
func NewCircuitBreakerRunnable(manager *CircuitBreakerManager) *CircuitBreakerRunnable {
	return &CircuitBreakerRunnable{manager: manager}
}

// Run implements the Runnable interface for integration with libCommons.Launcher.
// It starts the health checker and blocks until the process receives a shutdown signal.
func (r *CircuitBreakerRunnable) Run(_ *libCommons.Launcher) error {
	if r.manager == nil {
		// Silently return - manager may be nil in test scenarios or when circuit breaker is disabled
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	r.manager.Start()

	// Wait for shutdown signal
	<-ctx.Done()

	r.manager.Stop()

	return nil
}
