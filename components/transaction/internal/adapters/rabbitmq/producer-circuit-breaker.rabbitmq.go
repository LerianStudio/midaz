package rabbitmq

import (
	"context"
	"fmt"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
)

// Compile-time interface compliance check
var _ ProducerRepository = (*ProducerCircuitBreaker)(nil)

// ProducerCircuitBreaker wraps a ProducerRepository with circuit breaker protection.
// When the circuit is open, requests return immediately with an error (<1ms),
// triggering the existing fallback mechanism in the UseCase.
type ProducerCircuitBreaker struct {
	repo ProducerRepository
	cb   libCircuitBreaker.CircuitBreaker
}

// NewProducerCircuitBreaker creates a new ProducerCircuitBreaker that wraps the given
// repository with circuit breaker protection.
func NewProducerCircuitBreaker(repo ProducerRepository, cb libCircuitBreaker.CircuitBreaker) *ProducerCircuitBreaker {
	if repo == nil {
		panic("repo cannot be nil")
	}

	if cb == nil {
		panic("cb cannot be nil")
	}

	return &ProducerCircuitBreaker{
		repo: repo,
		cb:   cb,
	}
}

// ProducerDefault sends a message through the circuit breaker.
// If the circuit is open, returns an error immediately without attempting to publish.
// If the circuit is closed or half-open, delegates to the underlying repository.
func (p *ProducerCircuitBreaker) ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	result, err := p.cb.Execute(func() (any, error) {
		return p.repo.ProducerDefault(ctx, exchange, key, message)
	})
	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	str, ok := result.(*string)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	return str, nil
}

// CheckRabbitMQHealth delegates directly to the underlying repository.
// Health checks bypass the circuit breaker to provide accurate status.
func (p *ProducerCircuitBreaker) CheckRabbitMQHealth() bool {
	return p.repo.CheckRabbitMQHealth()
}
