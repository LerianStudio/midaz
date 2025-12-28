package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFullJitter(t *testing.T) {
	tests := []struct {
		name      string
		baseDelay time.Duration
	}{
		{name: "small delay", baseDelay: 100 * time.Millisecond},
		{name: "medium delay", baseDelay: 1 * time.Second},
		{name: "large delay", baseDelay: 5 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FullJitter(tt.baseDelay)

			// Result should be between 0 and baseDelay (or MaxBackoff if smaller)
			assert.GreaterOrEqual(t, result, time.Duration(0))

			expectedMax := tt.baseDelay
			if expectedMax > MaxBackoff {
				expectedMax = MaxBackoff
			}
			assert.LessOrEqual(t, result, expectedMax)
		})
	}
}

func TestFullJitter_CappedByMaxBackoff(t *testing.T) {
	// When baseDelay exceeds MaxBackoff, result should be capped
	veryLargeDelay := 100 * time.Second // Much larger than MaxBackoff (10s)

	// Run multiple times to test randomness doesn't exceed cap
	for i := 0; i < 10; i++ {
		result := FullJitter(veryLargeDelay)
		assert.LessOrEqual(t, result, MaxBackoff)
	}
}

func TestNextBackoff(t *testing.T) {
	tests := []struct {
		name     string
		current  time.Duration
		expected time.Duration
	}{
		{
			name:     "initial backoff doubles",
			current:  InitialBackoff,
			expected: time.Duration(float64(InitialBackoff) * BackoffFactor),
		},
		{
			name:     "1 second doubles to 2 seconds",
			current:  1 * time.Second,
			expected: 2 * time.Second,
		},
		{
			name:     "5 seconds doubles but capped at MaxBackoff",
			current:  5 * time.Second,
			expected: MaxBackoff,
		},
		{
			name:     "at max stays at max",
			current:  MaxBackoff,
			expected: MaxBackoff,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NextBackoff(tt.current)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNextBackoff_ExceedingMaxBackoff(t *testing.T) {
	// When calculated backoff exceeds MaxBackoff, it should be capped
	largeBackoff := 8 * time.Second // 8 * 2 = 16s > MaxBackoff (10s)

	result := NextBackoff(largeBackoff)

	assert.Equal(t, MaxBackoff, result)
}

func TestJitterConstants(t *testing.T) {
	// Verify constants are set to expected values
	assert.Equal(t, 5, MaxRetries)
	assert.Equal(t, 500*time.Millisecond, InitialBackoff)
	assert.Equal(t, 10*time.Second, MaxBackoff)
	assert.Equal(t, 2.0, BackoffFactor)
}
