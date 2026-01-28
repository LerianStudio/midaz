package utils

import (
	"math"
	"os"
	"strconv"
	"time"
)

// EnvFallback returns the prefixed value if not empty, otherwise returns the fallback value.
// This is useful for supporting both prefixed env vars (e.g., DB_ONBOARDING_HOST) with
// fallback to non-prefixed (e.g., DB_HOST) for backward compatibility.
func EnvFallback(prefixed, fallback string) string {
	if prefixed != "" {
		return prefixed
	}

	return fallback
}

// EnvFallbackInt returns the prefixed value if not zero, otherwise returns the fallback value.
// This is useful for supporting both prefixed env vars with fallback to non-prefixed
// for backward compatibility.
func EnvFallbackInt(prefixed, fallback int) int {
	if prefixed != 0 {
		return prefixed
	}

	return fallback
}

// GetEnvInt returns the environment variable value as int, or defaultVal if not set or invalid.
func GetEnvInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}

	parsed, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}

	return parsed
}

// GetEnvUint32 returns the environment variable value as uint32, or defaultVal if not set or invalid.
func GetEnvUint32(key string, defaultVal uint32) uint32 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}

	parsed, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return defaultVal
	}

	return uint32(parsed)
}

// GetEnvFloat64 returns the environment variable value as float64, or defaultVal if not set or invalid.
// Non-finite values (NaN, Inf, -Inf) are treated as invalid and return defaultVal.
func GetEnvFloat64(key string, defaultVal float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}

	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultVal
	}

	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return defaultVal
	}

	return parsed
}

// GetEnvFloat64WithRange returns the environment variable value as float64, clamped to [minVal, maxVal] range.
func GetEnvFloat64WithRange(key string, defaultVal, minVal, maxVal float64) float64 {
	if minVal > maxVal {
		minVal, maxVal = maxVal, minVal
	}

	value := GetEnvFloat64(key, defaultVal)

	if value < minVal {
		return minVal
	}

	if value > maxVal {
		return maxVal
	}

	return value
}

// GetEnvDuration returns the environment variable value as time.Duration, or defaultVal if not set or invalid.
func GetEnvDuration(key string, defaultVal time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}

	parsed, err := time.ParseDuration(v)
	if err != nil {
		return defaultVal
	}

	return parsed
}
