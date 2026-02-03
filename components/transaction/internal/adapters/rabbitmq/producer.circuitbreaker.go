package rabbitmq

import (
	"context"
	"errors"
	"fmt"

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
)

// CircuitBreakerProducer wraps ProducerRepository with circuit breaker protection.
// When RabbitMQ becomes unavailable, the circuit opens and returns errors immediately,
// allowing the caller to trigger fallback behavior without waiting for connection timeouts.
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
// If the circuit is open, returns an error immediately without attempting to publish.
// If the circuit is closed or half-open, attempts to publish and records success/failure.
func (p *CircuitBreakerProducer) ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	result, err := p.cbManager.Execute(CircuitBreakerServiceName, func() (any, error) {
		return p.underlying.ProducerDefault(ctx, exchange, key, message)
	})
	if err != nil {
		// Check if error is due to circuit being open
		state := p.cbManager.GetState(CircuitBreakerServiceName)
		if state == libCircuitBreaker.StateOpen {
			p.logger.Warnf("Circuit breaker open for RabbitMQ - returning error immediately")
			return nil, fmt.Errorf("circuit breaker open for RabbitMQ: %w", err)
		}

		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	str, ok := result.(*string)
	if !ok {
		return nil, fmt.Errorf("unexpected result type from circuit breaker: %T", result)
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
