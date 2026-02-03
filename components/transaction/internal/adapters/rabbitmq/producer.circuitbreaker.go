package rabbitmq

import (
	"context"
	"fmt"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/pkg/mcircuitbreaker"
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
// The stateListener parameter is optional - pass nil if you don't need state change notifications.
func NewCircuitBreakerProducer(
	underlying ProducerRepository,
	cbManager libCircuitBreaker.Manager,
	logger libLog.Logger,
	stateListener mcircuitbreaker.StateListener,
) *CircuitBreakerProducer {
	cbConfig := CircuitBreakerConfig{
		ConsecutiveFailures: getEnvAsUint32("RABBITMQ_CB_CONSECUTIVE_FAILURES", 3),
		FailureRatio:        getEnvAsFloat64("RABBITMQ_CB_FAILURE_RATIO", 0.4),
		Interval:            getEnvAsDuration("RABBITMQ_CB_INTERVAL", 30*time.Second),
		MaxRequests:         getEnvAsUint32("RABBITMQ_CB_MAX_REQUESTS", 3),
		MinRequests:         getEnvAsUint32("RABBITMQ_CB_MIN_REQUESTS", 5),
		Timeout:             getEnvAsDuration("RABBITMQ_CB_TIMEOUT", 30*time.Second),
	}

	// Initialize circuit breaker with RabbitMQ-optimized config
	cbManager.GetOrCreate(CircuitBreakerServiceName, RabbitMQCircuitBreakerConfig(cbConfig))

	// Register state change listener if provided
	if stateListener != nil {
		adapter := mcircuitbreaker.NewLibCommonsAdapter(stateListener)
		cbManager.RegisterStateChangeListener(adapter)
	}

	return &CircuitBreakerProducer{
		underlying: underlying,
		cbManager:  cbManager,
		logger:     logger,
	}
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
