package rabbitmq

import (
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
)

// CircuitBreakerServiceName is the service identifier for RabbitMQ producer circuit breaker.
const CircuitBreakerServiceName = "rabbitmq-producer"

// CircuitBreakerConfig holds the configuration parameters for the RabbitMQ circuit breaker.
type CircuitBreakerConfig struct {
	ConsecutiveFailures uint32        // Opens circuit after N consecutive failures
	FailureRatio        float64       // Opens circuit when failure rate >= ratio
	Interval            time.Duration // Resets counters in CLOSED state
	MaxRequests         uint32        // Test requests allowed in HALF-OPEN
	MinRequests         uint32        // Min requests before checking FailureRatio
	Timeout             time.Duration // Time in OPEN before HALF-OPEN (lazy, on next request)
	HealthCheckInterval time.Duration // Active health check polling interval
	HealthCheckTimeout  time.Duration // Timeout per health check probe
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
