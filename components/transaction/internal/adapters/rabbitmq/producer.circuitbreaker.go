// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

var (
	// ErrNilUnderlying indicates that the underlying producer parameter is nil.
	ErrNilUnderlying = errors.New("underlying producer cannot be nil")
	// ErrNilCBManager indicates that the circuit breaker manager parameter is nil.
	ErrNilCBManager = errors.New("circuit breaker manager cannot be nil")
	// ErrNilCBLogger indicates that the logger parameter is nil.
	ErrNilCBLogger = errors.New("logger cannot be nil")
	// ErrServiceUnavailable indicates the message queue service is temporarily unavailable.
	ErrServiceUnavailable = errors.New("message queue service temporarily unavailable")
	// ErrInternalProducerError indicates an unexpected internal error in the producer.
	ErrInternalProducerError = errors.New("internal producer error")
)

// CircuitBreakerProducer wraps ProducerRepository with circuit breaker protection.
//
// State flow: CLOSED → OPEN (on failures) → HALF-OPEN (after Timeout, lazy) → CLOSED/OPEN
//
// Half-open transition is lazy: happens on first request after Timeout expires, not automatically.
// HealthChecker can bypass half-open by resetting directly to closed when service recovers.
type CircuitBreakerProducer struct {
	underlying       ProducerRepository
	cbManager        libCircuitBreaker.Manager
	logger           libLog.Logger
	operationTimeout time.Duration // timeout for RabbitMQ operations (connection, publish)
}

// NewCircuitBreakerProducer creates a new circuit breaker wrapped producer.
// The cbManager must already have the circuit breaker initialized via NewCircuitBreakerManager.
// State listeners should be registered in NewCircuitBreakerManager, not here.
// The operationTimeout controls how long ProducerDefaultWithContext waits before timing out.
// If operationTimeout is 0, DefaultOperationTimeout is used.
// Returns an error if any required parameter is nil.
func NewCircuitBreakerProducer(
	underlying ProducerRepository,
	cbManager libCircuitBreaker.Manager,
	logger libLog.Logger,
	operationTimeout time.Duration,
) (*CircuitBreakerProducer, error) {
	if underlying == nil {
		return nil, ErrNilUnderlying
	}

	if cbManager == nil {
		return nil, ErrNilCBManager
	}

	if logger == nil {
		return nil, ErrNilCBLogger
	}

	if operationTimeout <= 0 {
		operationTimeout = DefaultOperationTimeout
	} else if operationTimeout > MaxOperationTimeout {
		operationTimeout = MaxOperationTimeout
	}

	return &CircuitBreakerProducer{
		underlying:       underlying,
		cbManager:        cbManager,
		logger:           logger,
		operationTimeout: operationTimeout,
	}, nil
}

// ProducerDefault publishes a message through the circuit breaker.
// CLOSED/HALF-OPEN: attempts publish. OPEN: returns error immediately.
// In HALF-OPEN, success closes circuit, failure reopens it.
func (p *CircuitBreakerProducer) ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	result, err := p.cbManager.Execute(CircuitBreakerServiceName, func() (any, error) {
		return p.underlying.ProducerDefault(ctx, exchange, key, message)
	})
	if err != nil {
		state := p.cbManager.GetState(CircuitBreakerServiceName)
		if state == libCircuitBreaker.StateOpen {
			// Log detailed info internally, return generic error to caller
			p.logger.Warnf("Circuit breaker open for RabbitMQ - returning error immediately: %v", err)
			return nil, ErrServiceUnavailable
		}

		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	str, ok := result.(*string)
	if !ok {
		// Log detailed type info internally, return generic error to caller
		p.logger.Errorf("Unexpected result type from producer: %T", result)
		return nil, ErrInternalProducerError
	}

	return str, nil
}

// ProducerDefaultWithContext publishes a message through the circuit breaker with context-aware timeout.
// Creates a scoped timeout context internally using operationTimeout, ensuring that RabbitMQ
// connection attempts don't block indefinitely. The original context is used for cancellation
// propagation but the scoped timeout ensures fast failure when RabbitMQ is unavailable.
// CLOSED/HALF-OPEN: attempts publish with scoped timeout. OPEN: returns error immediately.
// In HALF-OPEN, success closes circuit, failure reopens it.
func (p *CircuitBreakerProducer) ProducerDefaultWithContext(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	// Create scoped timeout context for RabbitMQ operations.
	// This ensures that even if RabbitMQ connection hangs, we timeout quickly
	// and allow fallback to execute within the HTTP request timeout.
	scopedCtx, cancel := context.WithTimeout(ctx, p.operationTimeout)
	defer cancel()

	result, err := p.cbManager.Execute(CircuitBreakerServiceName, func() (any, error) {
		return p.underlying.ProducerDefaultWithContext(scopedCtx, exchange, key, message)
	})
	if err != nil {
		state := p.cbManager.GetState(CircuitBreakerServiceName)
		if state == libCircuitBreaker.StateOpen {
			// Log detailed info internally, return generic error to caller
			p.logger.Warnf("Circuit breaker open for RabbitMQ - returning error immediately: %v", err)
			return nil, ErrServiceUnavailable
		}

		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	str, ok := result.(*string)
	if !ok {
		// Log detailed type info internally, return generic error to caller
		p.logger.Errorf("Unexpected result type from producer: %T", result)
		return nil, ErrInternalProducerError
	}

	return str, nil
}

// CheckRabbitMQHealth delegates to the underlying producer's health check.
func (p *CircuitBreakerProducer) CheckRabbitMQHealth() bool {
	return p.underlying.CheckRabbitMQHealth()
}

// GetCircuitState returns the current state of the circuit breaker.
func (p *CircuitBreakerProducer) GetCircuitState() libCircuitBreaker.State {
	return p.cbManager.GetState(CircuitBreakerServiceName)
}

// IsCircuitHealthy returns true if the circuit breaker is in closed state.
func (p *CircuitBreakerProducer) IsCircuitHealthy() bool {
	return p.cbManager.IsHealthy(CircuitBreakerServiceName)
}

// GetCounts returns the current circuit breaker statistics.
func (p *CircuitBreakerProducer) GetCounts() libCircuitBreaker.Counts {
	return p.cbManager.GetCounts(CircuitBreakerServiceName)
}
