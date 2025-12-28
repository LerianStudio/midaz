package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvFallback(t *testing.T) {
	tests := []struct {
		name     string
		prefixed string
		fallback string
		expected string
	}{
		{
			name:     "prefixed value takes precedence",
			prefixed: "prefixed-value",
			fallback: "fallback-value",
			expected: "prefixed-value",
		},
		{
			name:     "fallback used when prefixed is empty",
			prefixed: "",
			fallback: "fallback-value",
			expected: "fallback-value",
		},
		{
			name:     "both empty returns empty",
			prefixed: "",
			fallback: "",
			expected: "",
		},
		{
			name:     "prefixed with whitespace is considered non-empty",
			prefixed: "  ",
			fallback: "fallback",
			expected: "  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnvFallback(tt.prefixed, tt.fallback)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnvFallbackInt(t *testing.T) {
	tests := []struct {
		name     string
		prefixed int
		fallback int
		expected int
	}{
		{
			name:     "prefixed value takes precedence",
			prefixed: 100,
			fallback: 50,
			expected: 100,
		},
		{
			name:     "fallback used when prefixed is zero",
			prefixed: 0,
			fallback: 50,
			expected: 50,
		},
		{
			name:     "both zero returns zero",
			prefixed: 0,
			fallback: 0,
			expected: 0,
		},
		{
			name:     "negative prefixed takes precedence",
			prefixed: -10,
			fallback: 50,
			expected: -10,
		},
		{
			name:     "negative fallback used when prefixed is zero",
			prefixed: 0,
			fallback: -10,
			expected: -10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnvFallbackInt(tt.prefixed, tt.fallback)
			assert.Equal(t, tt.expected, result)
		})
	}
}
