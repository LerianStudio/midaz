// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.
package rabbitmq

import (
	"context"
	"errors"
	"maps"
	"sync"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v3/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
)

// ErrRabbitMQUnhealthy indicates the RabbitMQ broker health check failed.
var ErrRabbitMQUnhealthy = errors.New("rabbitmq health check failed")

// ErrRabbitMQChannelUnavailable indicates the RabbitMQ channel could not be established.
var ErrRabbitMQChannelUnavailable = errors.New("rabbitmq channel unavailable")

// ErrNilHealthChecker indicates that the underlying health checker parameter is nil.
var ErrNilHealthChecker = errors.New("underlying health checker cannot be nil")

// ErrNilHealthCheckerLogger indicates that the logger parameter is nil.
var ErrNilHealthCheckerLogger = errors.New("logger cannot be nil")

// ErrNilCircuitBreakerManager indicates that the circuit breaker manager parameter is nil.
var ErrNilCircuitBreakerManager = errors.New("circuit breaker manager cannot be nil")

// CircuitStateChecker is the minimal interface needed to check circuit breaker state.
// This interface allows for easier testing by not requiring the full Manager interface.
type CircuitStateChecker interface {
	// IsHealthy returns true if the circuit breaker for the service is in closed state.
	IsHealthy(serviceName string) bool
}

// RabbitMQHealthChecker defines the interface for RabbitMQ health check operations.
// Satisfied by *libRabbitmq.RabbitMQConnection.
type RabbitMQHealthChecker interface {
	// HealthCheck verifies the RabbitMQ service is responding (HTTP management API).
	HealthCheck() bool
	// EnsureChannel verifies and establishes AMQP connection and channel.
	EnsureChannel() error
	// EnsureChannelWithContext verifies and establishes AMQP connection and channel,
	// respecting context cancellation and deadline for timeout control.
	EnsureChannelWithContext(ctx context.Context) error
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
		// Use context-aware version to respect cancellation and timeouts
		if err := conn.EnsureChannelWithContext(ctx); err != nil {
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
//
// Note: lib-commons Reset() doesn't trigger state change listeners, so this wrapper
// includes a recovery monitor that detects when circuits are reset and manually
// triggers the closed notification.
type StateAwareHealthChecker struct {
	underlying      libCircuitBreaker.HealthChecker
	stateChecker    CircuitStateChecker
	logger          libLog.Logger
	mu              sync.RWMutex
	startStopMu     sync.Mutex // serializes Start/Stop to prevent interleaving
	running         bool
	unhealthyStates map[string]libCircuitBreaker.State
	stopMonitor     chan struct{}
}

// NewStateAwareHealthChecker wraps a health checker with state-aware start/stop behavior.
//
// The underlying health checker (from lib-commons) runs the actual health check loop.
// This wrapper controls when that loop starts and stops based on circuit breaker state.
// The cbManager is used to detect when circuits are reset (workaround for lib-commons
// Reset() not triggering state change listeners).
// Returns an error if any required parameter is nil.
func NewStateAwareHealthChecker(
	underlying libCircuitBreaker.HealthChecker,
	stateChecker CircuitStateChecker,
	logger libLog.Logger,
) (*StateAwareHealthChecker, error) {
	if underlying == nil {
		return nil, ErrNilHealthChecker
	}

	if stateChecker == nil {
		return nil, ErrNilCircuitBreakerManager
	}

	if logger == nil {
		return nil, ErrNilHealthCheckerLogger
	}

	return &StateAwareHealthChecker{
		underlying:      underlying,
		stateChecker:    stateChecker,
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
	// Acquire startStopMu first to serialize with OnStateChange and prevent
	// interleaving that could restart the checker after we set running=false
	s.startStopMu.Lock()
	defer s.startStopMu.Unlock()

	// Check and update running state under mu
	s.mu.Lock()
	wasRunning := s.running

	if wasRunning {
		s.running = false
	}

	s.mu.Unlock()

	if wasRunning {
		s.stopRecoveryMonitorLocked()
		s.underlying.Stop()
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

	// Update unhealthy states map
	s.mu.Lock()

	if to == libCircuitBreaker.StateClosed {
		delete(s.unhealthyStates, serviceName)
	} else {
		s.unhealthyStates[serviceName] = to
	}

	s.mu.Unlock()

	// Serialize start/stop decisions and underlying execution to prevent race conditions
	// where Start() could run after Stop() due to interleaving
	s.startStopMu.Lock()

	// Make decision under s.mu - re-read state to get current values
	s.mu.Lock()
	hasUnhealthy := len(s.unhealthyStates) > 0
	shouldStart := hasUnhealthy && !s.running
	shouldStop := !hasUnhealthy && s.running

	if shouldStart {
		s.running = true
	}

	if shouldStop {
		s.running = false
	}

	s.mu.Unlock()

	// Execute underlying start/stop and monitor operations under startStopMu
	if shouldStart {
		s.logger.Infof("Circuit opened for %s - starting health checker", serviceName)
		s.underlying.Start()
		s.startRecoveryMonitorLocked()
	}

	if shouldStop {
		s.logger.Infof("All circuits closed - stopping health checker")
		s.stopRecoveryMonitorLocked()
		s.underlying.Stop()
	}

	s.startStopMu.Unlock()

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

// startRecoveryMonitorLocked starts a goroutine that periodically checks if circuits
// have been reset (closed) via lib-commons Reset() which doesn't trigger listeners.
// When a reset is detected, it manually triggers the OnStateChange notification.
// Caller must hold startStopMu.
func (s *StateAwareHealthChecker) startRecoveryMonitorLocked() {
	// Defensive double-start check: if monitor already exists, don't start another
	if s.stopMonitor != nil {
		return
	}

	// Create local channel and assign to struct field while holding lock
	ch := make(chan struct{})
	s.stopMonitor = ch

	// Start goroutine closing over the local channel to avoid race with stopRecoveryMonitorLocked
	go func() {
		// Check every 5 seconds for recovered services
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ch:
				return
			case <-ticker.C:
				s.checkForRecoveredServices()
			}
		}
	}()
}

// stopRecoveryMonitorLocked stops the recovery monitor goroutine.
// Caller must hold startStopMu.
func (s *StateAwareHealthChecker) stopRecoveryMonitorLocked() {
	if s.stopMonitor != nil {
		close(s.stopMonitor)
		s.stopMonitor = nil
	}
}

// checkForRecoveredServices queries the circuit breaker manager to detect if any
// services in unhealthyStates have been reset to closed state.
func (s *StateAwareHealthChecker) checkForRecoveredServices() {
	s.mu.RLock()
	// Copy services to check to avoid holding lock while querying manager
	toCheck := make([]string, 0, len(s.unhealthyStates))

	previousStates := make(map[string]libCircuitBreaker.State, len(s.unhealthyStates))
	for serviceName, state := range s.unhealthyStates {
		toCheck = append(toCheck, serviceName)
		previousStates[serviceName] = state
	}

	s.mu.RUnlock()

	// Check each service against the circuit breaker state checker
	for _, serviceName := range toCheck {
		if s.stateChecker.IsHealthy(serviceName) {
			// Circuit was reset to closed - manually trigger notification
			previousState := previousStates[serviceName]
			s.logger.Infof("Recovery monitor detected %s circuit reset from %s to closed", serviceName, previousState)

			// Trigger the state change notification (this will update unhealthyStates and potentially stop the checker)
			s.OnStateChange(serviceName, previousState, libCircuitBreaker.StateClosed)
		}
	}
}
