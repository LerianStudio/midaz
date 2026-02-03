package utils

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEnvFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prefixed string
		fallback string
		expected string
	}{
		{
			name:     "returns prefixed when not empty",
			prefixed: "prefixed-value",
			fallback: "fallback-value",
			expected: "prefixed-value",
		},
		{
			name:     "returns fallback when prefixed is empty",
			prefixed: "",
			fallback: "fallback-value",
			expected: "fallback-value",
		},
		{
			name:     "returns empty when both are empty",
			prefixed: "",
			fallback: "",
			expected: "",
		},
		{
			name:     "returns prefixed even when fallback is empty",
			prefixed: "prefixed-value",
			fallback: "",
			expected: "prefixed-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := EnvFallback(tt.prefixed, tt.fallback)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnvFallbackInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prefixed int
		fallback int
		expected int
	}{
		{
			name:     "returns prefixed when not zero",
			prefixed: 100,
			fallback: 50,
			expected: 100,
		},
		{
			name:     "returns fallback when prefixed is zero",
			prefixed: 0,
			fallback: 50,
			expected: 50,
		},
		{
			name:     "returns zero when both are zero",
			prefixed: 0,
			fallback: 0,
			expected: 0,
		},
		{
			name:     "returns negative prefixed when not zero",
			prefixed: -10,
			fallback: 50,
			expected: -10,
		},
		{
			name:     "returns negative fallback when prefixed is zero",
			prefixed: 0,
			fallback: -20,
			expected: -20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := EnvFallbackInt(tt.prefixed, tt.fallback)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUint32WithDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		value        uint32
		defaultValue uint32
		expected     uint32
	}{
		{
			name:         "returns value when not zero",
			value:        15,
			defaultValue: 10,
			expected:     15,
		},
		{
			name:         "returns default when value is zero",
			value:        0,
			defaultValue: 10,
			expected:     10,
		},
		{
			name:         "returns zero when both are zero",
			value:        0,
			defaultValue: 0,
			expected:     0,
		},
		{
			name:         "handles max uint32 value",
			value:        4294967295,
			defaultValue: 10,
			expected:     4294967295,
		},
		{
			name:         "returns value of 1",
			value:        1,
			defaultValue: 100,
			expected:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := GetUint32WithDefault(tt.value, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetFloat64WithDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		value        float64
		defaultValue float64
		expected     float64
	}{
		{
			name:         "returns value when not zero",
			value:        0.5,
			defaultValue: 0.3,
			expected:     0.5,
		},
		{
			name:         "returns default when value is zero",
			value:        0,
			defaultValue: 0.5,
			expected:     0.5,
		},
		{
			name:         "returns zero when both are zero",
			value:        0,
			defaultValue: 0,
			expected:     0,
		},
		{
			name:         "handles negative value",
			value:        -0.5,
			defaultValue: 0.3,
			expected:     -0.5,
		},
		{
			name:         "handles very small value",
			value:        0.0001,
			defaultValue: 0.5,
			expected:     0.0001,
		},
		{
			name:         "handles value of 1.0",
			value:        1.0,
			defaultValue: 0.5,
			expected:     1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := GetFloat64WithDefault(tt.value, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetDurationWithDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		value        time.Duration
		defaultValue time.Duration
		expected     time.Duration
	}{
		{
			name:         "returns value when not zero",
			value:        30 * time.Second,
			defaultValue: 10 * time.Second,
			expected:     30 * time.Second,
		},
		{
			name:         "returns default when value is zero",
			value:        0,
			defaultValue: 30 * time.Second,
			expected:     30 * time.Second,
		},
		{
			name:         "returns zero when both are zero",
			value:        0,
			defaultValue: 0,
			expected:     0,
		},
		{
			name:         "handles minute duration",
			value:        2 * time.Minute,
			defaultValue: 1 * time.Minute,
			expected:     2 * time.Minute,
		},
		{
			name:         "handles millisecond duration",
			value:        500 * time.Millisecond,
			defaultValue: 100 * time.Millisecond,
			expected:     500 * time.Millisecond,
		},
		{
			name:         "handles negative duration",
			value:        -5 * time.Second,
			defaultValue: 10 * time.Second,
			expected:     -5 * time.Second,
		},
		{
			name:         "handles nanosecond precision",
			value:        1,
			defaultValue: 1000,
			expected:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := GetDurationWithDefault(tt.value, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUint32FromIntWithDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		value        int
		defaultValue uint32
		expected     uint32
	}{
		{
			name:         "returns value when positive",
			value:        15,
			defaultValue: 10,
			expected:     15,
		},
		{
			name:         "returns value when zero",
			value:        0,
			defaultValue: 10,
			expected:     0,
		},
		{
			name:         "returns default when negative",
			value:        -5,
			defaultValue: 10,
			expected:     10,
		},
		{
			name:         "handles max uint32 value",
			value:        math.MaxUint32,
			defaultValue: 10,
			expected:     math.MaxUint32,
		},
		{
			name:         "returns default when exceeds uint32 max",
			value:        math.MaxUint32 + 1,
			defaultValue: 10,
			expected:     10,
		},
		{
			name:         "handles value of 1",
			value:        1,
			defaultValue: 100,
			expected:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := GetUint32FromIntWithDefault(tt.value, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetFloat64FromIntPercentWithDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		value        int
		defaultValue float64
		expected     float64
	}{
		{
			name:         "converts 50% to 0.5",
			value:        50,
			defaultValue: 0.3,
			expected:     0.5,
		},
		{
			name:         "converts 100% to 1.0",
			value:        100,
			defaultValue: 0.3,
			expected:     1.0,
		},
		{
			name:         "converts 1% to 0.01",
			value:        1,
			defaultValue: 0.3,
			expected:     0.01,
		},
		{
			name:         "returns default when value is zero",
			value:        0,
			defaultValue: 0.5,
			expected:     0.5,
		},
		{
			name:         "returns default when value is negative",
			value:        -10,
			defaultValue: 0.5,
			expected:     0.5,
		},
		{
			name:         "returns default when value exceeds 100",
			value:        101,
			defaultValue: 0.5,
			expected:     0.5,
		},
		{
			name:         "returns default when value is 200",
			value:        200,
			defaultValue: 0.75,
			expected:     0.75,
		},
		{
			name:         "converts 75% to 0.75",
			value:        75,
			defaultValue: 0.5,
			expected:     0.75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := GetFloat64FromIntPercentWithDefault(tt.value, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetDurationSecondsWithDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		value        int
		defaultValue time.Duration
		expected     time.Duration
	}{
		{
			name:         "converts 30 seconds",
			value:        30,
			defaultValue: 10 * time.Second,
			expected:     30 * time.Second,
		},
		{
			name:         "returns default when value is zero",
			value:        0,
			defaultValue: 30 * time.Second,
			expected:     30 * time.Second,
		},
		{
			name:         "returns default when value is negative",
			value:        -5,
			defaultValue: 30 * time.Second,
			expected:     30 * time.Second,
		},
		{
			name:         "converts 120 seconds to 2 minutes",
			value:        120,
			defaultValue: 1 * time.Minute,
			expected:     2 * time.Minute,
		},
		{
			name:         "converts 1 second",
			value:        1,
			defaultValue: 10 * time.Second,
			expected:     1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := GetDurationSecondsWithDefault(tt.value, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}
