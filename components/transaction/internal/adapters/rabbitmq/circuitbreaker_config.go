package rabbitmq

import (
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
)

// CircuitBreakerServiceName is the service identifier for RabbitMQ producer circuit breaker.
const CircuitBreakerServiceName = "rabbitmq-producer"

// CircuitBreakerConfig holds the configuration parameters for the RabbitMQ circuit breaker.
type CircuitBreakerConfig struct {
	ConsecutiveFailures uint32
	FailureRatio        float64
	Interval            time.Duration
	MaxRequests         uint32
	MinRequests         uint32
	Timeout             time.Duration
	HealthCheckInterval time.Duration
	HealthCheckTimeout  time.Duration
}

// RabbitMQCircuitBreakerConfig creates circuit breaker settings from provided configuration.
// These settings control fail-fast behavior for financial transaction processing.
func RabbitMQCircuitBreakerConfig(cfg CircuitBreakerConfig) libCircuitBreaker.Config {
	return libCircuitBreaker.Config{
		MaxRequests:         cfg.MaxRequests,         // Requests allowed in half-open state
		Interval:            cfg.Interval,            // Reset failure count interval
		Timeout:             cfg.Timeout,             // Wait time before trying half-open
		ConsecutiveFailures: cfg.ConsecutiveFailures, // Failures needed to open circuit
		FailureRatio:        cfg.FailureRatio,        // Failure ratio to open circuit
		MinRequests:         cfg.MinRequests,         // Min requests before checking ratio
	}
}
