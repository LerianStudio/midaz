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

func TestGetEnv_ReturnsValueWhenSet(t *testing.T) {
	key := "TEST_GETENV_VALUE"
	expected := "test-value"
	t.Setenv(key, expected)

	result := getEnv(key, "default")

	assert.Equal(t, expected, result)
}

func TestGetEnv_ReturnsDefaultWhenNotSet(t *testing.T) {
	key := "TEST_GETENV_NOTSET"
	defaultValue := "default-value"

	result := getEnv(key, defaultValue)

	assert.Equal(t, defaultValue, result)
}

func TestGetEnv_ReturnsEmptyStringWhenSetToEmpty(t *testing.T) {
	key := "TEST_GETENV_EMPTY"
	t.Setenv(key, "")

	result := getEnv(key, "default")

	assert.Equal(t, "", result)
}

func TestGetEnvAsUint32_ReturnsValueWhenValid(t *testing.T) {
	key := "TEST_UINT32_VALID"
	t.Setenv(key, "42")

	result := getEnvAsUint32(key, 0)

	assert.Equal(t, uint32(42), result)
}

func TestGetEnvAsUint32_ReturnsDefaultWhenNotSet(t *testing.T) {
	key := "TEST_UINT32_NOTSET"
	defaultValue := uint32(99)

	result := getEnvAsUint32(key, defaultValue)

	assert.Equal(t, defaultValue, result)
}

func TestGetEnvAsUint32_ReturnsDefaultWhenEmpty(t *testing.T) {
	key := "TEST_UINT32_EMPTY"
	t.Setenv(key, "")
	defaultValue := uint32(100)

	result := getEnvAsUint32(key, defaultValue)

	assert.Equal(t, defaultValue, result)
}

func TestGetEnvAsUint32_ReturnsDefaultWhenInvalidString(t *testing.T) {
	key := "TEST_UINT32_INVALID"
	t.Setenv(key, "not-a-number")
	defaultValue := uint32(50)

	result := getEnvAsUint32(key, defaultValue)

	assert.Equal(t, defaultValue, result)
}

func TestGetEnvAsUint32_ReturnsDefaultWhenNegative(t *testing.T) {
	key := "TEST_UINT32_NEGATIVE"
	t.Setenv(key, "-5")
	defaultValue := uint32(25)

	result := getEnvAsUint32(key, defaultValue)

	assert.Equal(t, defaultValue, result)
}

func TestGetEnvAsUint32_ReturnsDefaultWhenOverflow(t *testing.T) {
	key := "TEST_UINT32_OVERFLOW"
	t.Setenv(key, "9999999999999999999")
	defaultValue := uint32(10)

	result := getEnvAsUint32(key, defaultValue)

	assert.Equal(t, defaultValue, result)
}

func TestGetEnvAsFloat64_ReturnsValueWhenValid(t *testing.T) {
	key := "TEST_FLOAT64_VALID"
	t.Setenv(key, "0.75")

	result := getEnvAsFloat64(key, 0.0)

	assert.Equal(t, 0.75, result)
}

func TestGetEnvAsFloat64_ReturnsDefaultWhenNotSet(t *testing.T) {
	key := "TEST_FLOAT64_NOTSET"
	defaultValue := 0.5

	result := getEnvAsFloat64(key, defaultValue)

	assert.Equal(t, defaultValue, result)
}

func TestGetEnvAsFloat64_ReturnsDefaultWhenEmpty(t *testing.T) {
	key := "TEST_FLOAT64_EMPTY"
	t.Setenv(key, "")
	defaultValue := 0.8

	result := getEnvAsFloat64(key, defaultValue)

	assert.Equal(t, defaultValue, result)
}

func TestGetEnvAsFloat64_ReturnsDefaultWhenInvalidString(t *testing.T) {
	key := "TEST_FLOAT64_INVALID"
	t.Setenv(key, "not-a-float")
	defaultValue := 0.3

	result := getEnvAsFloat64(key, defaultValue)

	assert.Equal(t, defaultValue, result)
}

func TestGetEnvAsFloat64_HandlesNegativeValues(t *testing.T) {
	key := "TEST_FLOAT64_NEGATIVE"
	t.Setenv(key, "-0.5")

	result := getEnvAsFloat64(key, 0.0)

	assert.Equal(t, -0.5, result)
}

func TestGetEnvAsDuration_ReturnsValueWhenValid(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{"seconds", "30s", 30 * time.Second},
		{"minutes", "5m", 5 * time.Minute},
		{"hours", "2h", 2 * time.Hour},
		{"complex", "1h30m", 1*time.Hour + 30*time.Minute},
		{"milliseconds", "500ms", 500 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_DURATION_" + tt.name
			t.Setenv(key, tt.input)

			result := getEnvAsDuration(key, 0)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetEnvAsDuration_ReturnsDefaultWhenNotSet(t *testing.T) {
	key := "TEST_DURATION_NOTSET"
	defaultValue := 45 * time.Second

	result := getEnvAsDuration(key, defaultValue)

	assert.Equal(t, defaultValue, result)
}

func TestGetEnvAsDuration_ReturnsDefaultWhenEmpty(t *testing.T) {
	key := "TEST_DURATION_EMPTY"
	t.Setenv(key, "")
	defaultValue := 1 * time.Minute

	result := getEnvAsDuration(key, defaultValue)

	assert.Equal(t, defaultValue, result)
}

func TestGetEnvAsDuration_ReturnsDefaultWhenInvalidFormat(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"plain number", "30"},
		{"invalid unit", "30x"},
		{"text", "invalid"},
		{"missing number", "s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_DURATION_INVALID_" + tt.name
			t.Setenv(key, tt.input)
			defaultValue := 2 * time.Minute

			result := getEnvAsDuration(key, defaultValue)

			assert.Equal(t, defaultValue, result)
		})
	}
}