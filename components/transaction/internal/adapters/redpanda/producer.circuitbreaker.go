// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

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
	// ErrServiceUnavailable indicates the broker service is temporarily unavailable.
	ErrServiceUnavailable = errors.New("message queue service temporarily unavailable")
	// ErrInternalProducerError indicates an unexpected internal error in the producer.
	ErrInternalProducerError = errors.New("internal producer error")
)

// CircuitBreakerProducer wraps ProducerRepository with circuit breaker protection.
type CircuitBreakerProducer struct {
	underlying       ProducerRepository
	cbManager        libCircuitBreaker.Manager
	logger           libLog.Logger
	operationTimeout time.Duration
}

// NewCircuitBreakerProducer creates a new circuit breaker wrapped producer.
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
func (p *CircuitBreakerProducer) ProducerDefault(ctx context.Context, topic, key string, message []byte) (*string, error) {
	return p.executeWithCircuit(func() (any, error) {
		return p.underlying.ProducerDefault(ctx, topic, key, message)
	})
}

// ProducerDefaultWithContext publishes a message through circuit breaker with timeout.
func (p *CircuitBreakerProducer) ProducerDefaultWithContext(ctx context.Context, topic, key string, message []byte) (*string, error) {
	scopedCtx, cancel := context.WithTimeout(ctx, p.operationTimeout)
	defer cancel()

	return p.executeWithCircuit(func() (any, error) {
		return p.underlying.ProducerDefaultWithContext(scopedCtx, topic, key, message)
	})
}

func (p *CircuitBreakerProducer) executeWithCircuit(operation func() (any, error)) (*string, error) {
	if p == nil || p.cbManager == nil || p.underlying == nil {
		return nil, ErrInternalProducerError
	}

	result, err := p.cbManager.Execute(CircuitBreakerServiceName, operation)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}

		state := p.cbManager.GetState(CircuitBreakerServiceName)
		if state == libCircuitBreaker.StateOpen {
			p.logger.Warnf("Circuit breaker open for Redpanda - returning error immediately: %v", err)
			return nil, ErrServiceUnavailable
		}

		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	str, ok := result.(*string)
	if !ok {
		p.logger.Errorf("Unexpected result type from producer: %T", result)
		return nil, ErrInternalProducerError
	}

	return str, nil
}

// CheckHealth delegates to the underlying producer's health check.
func (p *CircuitBreakerProducer) CheckHealth() bool {
	if p == nil || p.underlying == nil {
		return false
	}

	return p.underlying.CheckHealth()
}

// GetCircuitState returns the current state of the circuit breaker.
func (p *CircuitBreakerProducer) GetCircuitState() libCircuitBreaker.State {
	if p == nil || p.cbManager == nil {
		return libCircuitBreaker.StateOpen
	}

	return p.cbManager.GetState(CircuitBreakerServiceName)
}

// IsCircuitHealthy returns true if the circuit breaker is in closed state.
func (p *CircuitBreakerProducer) IsCircuitHealthy() bool {
	if p == nil || p.cbManager == nil {
		return false
	}

	return p.cbManager.IsHealthy(CircuitBreakerServiceName)
}

// GetCounts returns the current circuit breaker statistics.
func (p *CircuitBreakerProducer) GetCounts() libCircuitBreaker.Counts {
	if p == nil || p.cbManager == nil {
		return libCircuitBreaker.Counts{}
	}

	return p.cbManager.GetCounts(CircuitBreakerServiceName)
}

// Close releases resources held by the underlying producer.
func (p *CircuitBreakerProducer) Close() error {
	if p == nil || p.underlying == nil {
		return nil
	}

	return p.underlying.Close()
}
