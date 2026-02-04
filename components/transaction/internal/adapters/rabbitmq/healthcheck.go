package rabbitmq

import (
	"context"
	"errors"
	"maps"
	"sync"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// ErrRabbitMQUnhealthy indicates the RabbitMQ broker health check failed.
var ErrRabbitMQUnhealthy = errors.New("rabbitmq health check failed")

// ErrRabbitMQChannelUnavailable indicates the RabbitMQ channel could not be established.
var ErrRabbitMQChannelUnavailable = errors.New("rabbitmq channel unavailable")

// ErrNilHealthChecker indicates that the underlying health checker parameter is nil.
var ErrNilHealthChecker = errors.New("underlying health checker cannot be nil")

// ErrNilHealthCheckerLogger indicates that the logger parameter is nil.
var ErrNilHealthCheckerLogger = errors.New("logger cannot be nil")

// RabbitMQHealthChecker defines the interface for RabbitMQ health check operations.
// Satisfied by *libRabbitmq.RabbitMQConnection.
type RabbitMQHealthChecker interface {
	// HealthCheck verifies the RabbitMQ service is responding (HTTP management API).
	HealthCheck() bool
	// EnsureChannel verifies and establishes AMQP connection and channel.
	EnsureChannel() error
}

// NewRabbitMQHealthCheckFunc creates the health check function that tests RabbitMQ connectivity.
//
// The health check performs two validations:
//  1. Service health - Calls HealthCheck() to verify RabbitMQ management API is responding
//  2. Channel state - Calls EnsureChannel() to verify AMQP connection and channel are alive
//
// Both checks must pass for the service to be considered healthy.
//
// Usage:
//
//	healthCheckFn := NewRabbitMQHealthCheckFunc(rabbitConn)
//	healthChecker.Register("rabbitmq-producer", healthCheckFn)
func NewRabbitMQHealthCheckFunc(conn RabbitMQHealthChecker) libCircuitBreaker.HealthCheckFunc {
	return func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if conn == nil {
			return ErrRabbitMQUnhealthy
		}

		// Check 1: Service health (HTTP management API)
		if !conn.HealthCheck() {
			return ErrRabbitMQUnhealthy
		}

		// Check 2: Connection and channel state (AMQP)
		if err := conn.EnsureChannel(); err != nil {
			return errors.Join(ErrRabbitMQChannelUnavailable, err)
		}

		return nil
	}
}

// StateAwareHealthChecker controls when the health checker runs based on circuit state.
//
// Instead of running health checks continuously, it only runs when needed:
//   - Starts when any circuit opens (service is down, need to detect recovery)
//   - Keeps running during half-open (still testing)
//   - Stops when all circuits close (all services recovered)
//
// This reduces unnecessary resource usage when all services are healthy.
type StateAwareHealthChecker struct {
	underlying      libCircuitBreaker.HealthChecker
	logger          libLog.Logger
	mu              sync.RWMutex
	running         bool
	unhealthyStates map[string]libCircuitBreaker.State
}

// NewStateAwareHealthChecker wraps a health checker with state-aware start/stop behavior.
//
// The underlying health checker (from lib-commons) runs the actual health check loop.
// This wrapper controls when that loop starts and stops based on circuit breaker state.
// Returns an error if any required parameter is nil.
func NewStateAwareHealthChecker(
	underlying libCircuitBreaker.HealthChecker,
	logger libLog.Logger,
) (*StateAwareHealthChecker, error) {
	if underlying == nil {
		return nil, ErrNilHealthChecker
	}

	if logger == nil {
		return nil, ErrNilHealthCheckerLogger
	}

	return &StateAwareHealthChecker{
		underlying:      underlying,
		logger:          logger,
		unhealthyStates: make(map[string]libCircuitBreaker.State),
	}, nil
}

// Register adds a health check function for a service.
// Delegates to the underlying health checker.
func (s *StateAwareHealthChecker) Register(serviceName string, healthCheckFn libCircuitBreaker.HealthCheckFunc) {
	s.underlying.Register(serviceName, healthCheckFn)
}

// Start is called during initialization but doesn't actually start the health checker.
// The health checker starts automatically when a circuit opens (see OnStateChange).
func (s *StateAwareHealthChecker) Start() {
	s.logger.Info("StateAwareHealthChecker initialized - will start when circuit opens")
}

// Stop stops the health checker if it's running.
// Called during graceful shutdown.
func (s *StateAwareHealthChecker) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		s.underlying.Stop()
		s.running = false
		s.logger.Info("StateAwareHealthChecker stopped")
	}
}

// GetHealthStatus returns the health status of all registered services.
func (s *StateAwareHealthChecker) GetHealthStatus() map[string]string {
	return s.underlying.GetHealthStatus()
}

// OnStateChange is called when a circuit breaker changes state.
// It starts/stops the health checker based on circuit state transitions.
//
// State transitions:
//   - Any circuit opens (CLOSED → OPEN): Start health checker
//   - Circuit goes half-open (OPEN → HALF-OPEN): Keep running
//   - All circuits close (→ CLOSED): Stop health checker
func (s *StateAwareHealthChecker) OnStateChange(serviceName string, from, to libCircuitBreaker.State) {
	// Log state change outside the critical section to reduce lock contention
	s.logger.Debugf("StateAwareHealthChecker: %s state changed %s -> %s", serviceName, from, to)

	var shouldStart, shouldStop bool
	var logServiceName string

	// Critical section: update state and determine actions
	s.mu.Lock()
	wasAllHealthy := len(s.unhealthyStates) == 0

	if to == libCircuitBreaker.StateClosed {
		delete(s.unhealthyStates, serviceName)
	} else {
		s.unhealthyStates[serviceName] = to
	}

	isAllHealthy := len(s.unhealthyStates) == 0

	// Determine actions to take after releasing lock
	if wasAllHealthy && !isAllHealthy && !s.running {
		shouldStart = true
		logServiceName = serviceName
		s.running = true
	}

	if !wasAllHealthy && isAllHealthy && s.running {
		shouldStop = true
		s.running = false
	}
	s.mu.Unlock()

	// Execute actions outside the critical section
	if shouldStart {
		s.logger.Infof("Circuit opened for %s - starting health checker", logServiceName)
		s.underlying.Start()
	}

	if shouldStop {
		s.logger.Infof("All circuits closed - stopping health checker")
		s.underlying.Stop()
	}

	// Forward to underlying for immediate health check on open
	s.underlying.OnStateChange(serviceName, from, to)
}

// IsRunning returns whether the health checker is currently active.
func (s *StateAwareHealthChecker) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.running
}

// GetUnhealthyServices returns services that are currently in non-closed state.
func (s *StateAwareHealthChecker) GetUnhealthyServices() map[string]libCircuitBreaker.State {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]libCircuitBreaker.State, len(s.unhealthyStates))
	maps.Copy(result, s.unhealthyStates)

	return result
}
