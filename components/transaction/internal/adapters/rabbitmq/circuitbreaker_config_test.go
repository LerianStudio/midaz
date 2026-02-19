// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRabbitMQCircuitBreakerConfig_HasCorrectValues(t *testing.T) {
	cfg := CircuitBreakerConfig{
		ConsecutiveFailures: 15,
		FailureRatio:        0.5,
		Interval:            2 * time.Minute,
		MaxRequests:         3,
		MinRequests:         10,
		Timeout:             30 * time.Second,
	}

	config := RabbitMQCircuitBreakerConfig(cfg)

	// Verify settings match configuration
	assert.Equal(t, uint32(3), config.MaxRequests, "MaxRequests should allow 3 requests in half-open")
	assert.Equal(t, 2*time.Minute, config.Interval, "Interval should be 2 minutes")
	assert.Equal(t, 30*time.Second, config.Timeout, "Timeout should be 30 seconds")
	assert.Equal(t, uint32(15), config.ConsecutiveFailures, "ConsecutiveFailures should be 15")
	assert.Equal(t, 0.5, config.FailureRatio, "FailureRatio should be 50%")
	assert.Equal(t, uint32(10), config.MinRequests, "MinRequests should be 10")
}

func TestServiceNameConstant_IsCorrect(t *testing.T) {
	assert.Equal(t, "rabbitmq-producer", CircuitBreakerServiceName)
}

func TestCircuitBreakerConfig_WithZeroValues(t *testing.T) {
	cfg := CircuitBreakerConfig{
		ConsecutiveFailures: 0,
		FailureRatio:        0,
		Interval:            0,
		MaxRequests:         0,
		MinRequests:         0,
		Timeout:             0,
	}

	config := RabbitMQCircuitBreakerConfig(cfg)

	// Verify zero values are passed through (validation happens elsewhere)
	assert.Equal(t, uint32(0), config.MaxRequests)
	assert.Equal(t, time.Duration(0), config.Interval)
	assert.Equal(t, time.Duration(0), config.Timeout)
	assert.Equal(t, uint32(0), config.ConsecutiveFailures)
	assert.Equal(t, 0.0, config.FailureRatio)
	assert.Equal(t, uint32(0), config.MinRequests)
}

func TestCircuitBreakerConfig_WithHealthCheckFields(t *testing.T) {
	cfg := CircuitBreakerConfig{
		ConsecutiveFailures: 15,
		FailureRatio:        0.5,
		Interval:            2 * time.Minute,
		MaxRequests:         3,
		MinRequests:         10,
		Timeout:             30 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		HealthCheckTimeout:  10 * time.Second,
	}

	// Health check fields are stored in CircuitBreakerConfig but not passed to lib-commons Config
	config := RabbitMQCircuitBreakerConfig(cfg)

	// Verify core config is correct (health check fields are used by NewCircuitBreakerManager)
	assert.Equal(t, uint32(15), config.ConsecutiveFailures)
	assert.Equal(t, cfg.HealthCheckInterval, 30*time.Second)
	assert.Equal(t, uint32(3), config.MaxRequests)
	assert.Equal(t, 2*time.Minute, config.Interval)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, uint32(10), config.MinRequests)
	assert.Equal(t, 0.5, config.FailureRatio)
}
