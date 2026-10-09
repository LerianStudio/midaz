// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v7/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
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
	underlying ProducerRepository
	cbManager  libCircuitBreaker.Manager
	logger     libLog.Logger
}

// NewCircuitBreakerProducer creates a new circuit breaker wrapped producer.
// The cbManager must already have the circuit breaker initialized via NewCircuitBreakerManager.
// State listeners should be registered in NewCircuitBreakerManager, not here.
// Returns an error if any required parameter is nil.
func NewCircuitBreakerProducer(
	underlying ProducerRepository,
	cbManager libCircuitBreaker.Manager,
	logger libLog.Logger,
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

	return &CircuitBreakerProducer{
		underlying: underlying,
		cbManager:  cbManager,
		logger:     logger,
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
			p.logger.Log(ctx, libLog.LevelWarn, "Circuit breaker open for RabbitMQ - returning error immediately", libLog.Err(err))
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
		p.logger.Log(ctx, libLog.LevelError, "Unexpected result type from producer", libLog.String("type", "*string expected"), libLog.Any("result", result))
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

// Close releases resources held by the underlying producer.
// Safe to call multiple times or on nil receivers.
func (p *CircuitBreakerProducer) Close() error {
	if p == nil || p.underlying == nil {
		return nil
	}

	return p.underlying.Close()
}
