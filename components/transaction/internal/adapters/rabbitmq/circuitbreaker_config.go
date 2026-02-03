package rabbitmq

import (
	"os"
	"strconv"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
)

// CircuitBreakerServiceName is the service identifier for RabbitMQ producer circuit breaker.
const CircuitBreakerServiceName = "rabbitmq-producer"

// CircuitBreakerConfig holds the configuration parameters for the RabbitMQ circuit breaker.
type CircuitBreakerConfig struct {
	ConsecutiveFailures  uint32
	FailureRatio         float64
	Interval             time.Duration
	MaxRequests          uint32
	MinRequests          uint32
	Timeout              time.Duration
	HealthCheckInterval  time.Duration
	HealthCheckTimeout   time.Duration
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

// getEnv retrieves an environment variable or returns the default value if not set.
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	return defaultValue
}

// getEnvAsUint32 retrieves an environment variable as uint32 or returns the default value.
func getEnvAsUint32(key string, defaultValue uint32) uint32 {
	strValue := getEnv(key, "")
	if strValue == "" {
		return defaultValue
	}

	value, err := strconv.ParseUint(strValue, 10, 32)
	if err != nil {
		return defaultValue
	}

	return uint32(value)
}

// getEnvAsFloat64 retrieves an environment variable as float64 or returns the default value.
func getEnvAsFloat64(key string, defaultValue float64) float64 {
	strValue := getEnv(key, "")
	if strValue == "" {
		return defaultValue
	}

	value, err := strconv.ParseFloat(strValue, 64)
	if err != nil {
		return defaultValue
	}

	return value
}

// getEnvAsDuration retrieves an environment variable as time.Duration or returns the default value.
// The environment variable should be in a format parseable by time.ParseDuration (e.g., "30s", "5m").
func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	strValue := getEnv(key, "")
	if strValue == "" {
		return defaultValue
	}

	value, err := time.ParseDuration(strValue)
	if err != nil {
		return defaultValue
	}

	return value
}
