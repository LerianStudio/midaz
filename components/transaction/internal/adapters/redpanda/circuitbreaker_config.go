// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
)

// CircuitBreakerServiceName is the service identifier for Redpanda producer circuit breaker.
const CircuitBreakerServiceName = "redpanda-producer"

// DefaultOperationTimeout is the default timeout for Redpanda produce operations.
const DefaultOperationTimeout = 5 * time.Second

// MaxOperationTimeout is the maximum allowed timeout for Redpanda produce operations.
const MaxOperationTimeout = 30 * time.Second

// CircuitBreakerConfig holds the configuration parameters for the producer circuit breaker.
type CircuitBreakerConfig struct {
	ConsecutiveFailures uint32
	FailureRatio        float64
	Interval            time.Duration
	MaxRequests         uint32
	MinRequests         uint32
	Timeout             time.Duration
	HealthCheckInterval time.Duration
	OperationTimeout    time.Duration
}

// ProducerCircuitBreakerConfig creates circuit breaker settings from provided configuration.
func ProducerCircuitBreakerConfig(cfg CircuitBreakerConfig) libCircuitBreaker.Config {
	return libCircuitBreaker.Config{
		MaxRequests:         cfg.MaxRequests,
		Interval:            cfg.Interval,
		Timeout:             cfg.Timeout,
		ConsecutiveFailures: cfg.ConsecutiveFailures,
		FailureRatio:        cfg.FailureRatio,
		MinRequests:         cfg.MinRequests,
	}
}
