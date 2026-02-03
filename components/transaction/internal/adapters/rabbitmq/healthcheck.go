package rabbitmq

import (
	"context"
	"errors"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
)

// ErrRabbitMQUnhealthy indicates the RabbitMQ broker health check failed.
var ErrRabbitMQUnhealthy = errors.New("rabbitmq health check failed")

// RabbitMQHealthChecker defines the interface for RabbitMQ health check operations.
// This interface is satisfied by *libRabbitmq.RabbitMQConnection.
type RabbitMQHealthChecker interface {
	HealthCheck() bool
}

// NewRabbitMQHealthCheckFunc creates a health check function for circuit breaker recovery.
// This function is called by the health checker when the circuit is open to test if
// the broker has recovered. If it returns nil, the circuit breaker will be reset.
func NewRabbitMQHealthCheckFunc(conn RabbitMQHealthChecker) libCircuitBreaker.HealthCheckFunc {
	return func(ctx context.Context) error {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Guard against nil connection
		if conn == nil {
			return ErrRabbitMQUnhealthy
		}

		// Use the existing HealthCheck method from lib-commons
		if conn.HealthCheck() {
			return nil
		}

		return ErrRabbitMQUnhealthy
	}
}
