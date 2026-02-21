// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"math"
	"os"
	"strings"
	"time"
)

// IsTruthyString normalizes and evaluates common truthy string values.
func IsTruthyString(value string) bool {
	raw := strings.ToLower(strings.TrimSpace(value))

	return raw == "1" || raw == "true" || raw == "yes"
}

// IsEnvTruthy reads an env var and evaluates it using IsTruthyString.
func IsEnvTruthy(envName string) bool {
	return IsTruthyString(os.Getenv(envName))
}

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

// GetUint32WithDefault returns the value if not zero, otherwise returns the default value.
func GetUint32WithDefault(value, defaultValue uint32) uint32 {
	if value != 0 {
		return value
	}

	return defaultValue
}

// GetFloat64WithDefault returns the value if not zero, otherwise returns the default value.
func GetFloat64WithDefault(value, defaultValue float64) float64 {
	if value != 0 {
		return value
	}

	return defaultValue
}

// GetDurationWithDefault returns the value if not zero, otherwise returns the default value.
func GetDurationWithDefault(value, defaultValue time.Duration) time.Duration {
	if value != 0 {
		return value
	}

	return defaultValue
}

// GetUint32FromIntWithDefault converts an int to uint32, returning defaultValue when the
// conversion is not meaningful. Specifically:
//   - values <= 0 (including zero) return defaultValue — zero is intentionally treated as
//     "not configured" for configuration knobs where a zero setting has no practical meaning
//     (e.g., worker counts, pool sizes, shard counts).
//   - values that exceed math.MaxUint32 return defaultValue to prevent truncation.
//
// This is useful when reading config from env vars that are parsed as int but need to
// be stored as uint32.
func GetUint32FromIntWithDefault(value int, defaultValue uint32) uint32 {
	if value > 0 && value <= math.MaxUint32 {
		return uint32(value)
	}

	return defaultValue
}

// GetFloat64FromIntPercentWithDefault converts an int percentage (0-100) to float64 ratio (0.0-1.0),
// returning the default if value is out of range (<=0 or >100).
// Example: 50 -> 0.5, 75 -> 0.75
func GetFloat64FromIntPercentWithDefault(value int, defaultValue float64) float64 {
	if value > 0 && value <= 100 {
		return float64(value) / 100.0
	}

	return defaultValue
}

// GetDurationSecondsWithDefault converts an int (seconds) to time.Duration,
// returning the default if value is <= 0.
func GetDurationSecondsWithDefault(value int, defaultValue time.Duration) time.Duration {
	if value > 0 {
		return time.Duration(value) * time.Second
	}

	return defaultValue
}
