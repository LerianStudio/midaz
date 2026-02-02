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