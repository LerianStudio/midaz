// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v3/commons/circuitbreaker"
)

// CircuitBreakerServiceName is the service identifier for RabbitMQ producer circuit breaker.
const CircuitBreakerServiceName = "rabbitmq-producer"

// DefaultOperationTimeout is the default timeout for RabbitMQ connection and publish operations.
// This ensures fallback can execute within HTTP request timeout.
const DefaultOperationTimeout = 5 * time.Second

// MaxOperationTimeout is the maximum allowed timeout for RabbitMQ operations.
// This prevents misconfiguration that could cause fallback to never have time to execute.
const MaxOperationTimeout = 30 * time.Second

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
	OperationTimeout    time.Duration // Timeout for connection and publish operations
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
