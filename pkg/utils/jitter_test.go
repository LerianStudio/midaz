package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFullJitter_ReturnsWithinRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		baseDelay time.Duration
	}{
		{"small_delay", 100 * time.Millisecond},
		{"medium_delay", 1 * time.Second},
		{"large_delay", 5 * time.Second},
		{"initial_backoff", InitialBackoff},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Run multiple times to account for randomness
			for i := 0; i < 100; i++ {
				result := FullJitter(tt.baseDelay)

				assert.GreaterOrEqual(t, result, time.Duration(0), "jitter should be non-negative")
				assert.LessOrEqual(t, result, tt.baseDelay, "jitter should not exceed baseDelay")
			}
		})
	}
}

func TestFullJitter_RespectsMaxBackoff(t *testing.T) {
	t.Parallel()

	// Use a baseDelay much larger than MaxBackoff
	baseDelay := MaxBackoff * 3

	// Run multiple times to account for randomness
	for i := 0; i < 100; i++ {
		result := FullJitter(baseDelay)

		assert.GreaterOrEqual(t, result, time.Duration(0), "jitter should be non-negative")
		assert.LessOrEqual(t, result, MaxBackoff, "jitter should be capped at MaxBackoff")
	}
}

func TestFullJitter_ZeroBaseDelay(t *testing.T) {
	t.Parallel()

	result := FullJitter(0)

	assert.Equal(t, time.Duration(0), result, "jitter with zero baseDelay should be zero")
}

func TestNextBackoff_ExponentialGrowth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		current  time.Duration
		expected time.Duration
	}{
		{"from_initial", InitialBackoff, time.Duration(float64(InitialBackoff) * BackoffFactor)},
		{"from_1_second", 1 * time.Second, 2 * time.Second},
		{"from_2_seconds", 2 * time.Second, 4 * time.Second},
		{"from_500ms", 500 * time.Millisecond, 1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := NextBackoff(tt.current)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNextBackoff_RespectsMaxBackoff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current time.Duration
	}{
		{"at_max", MaxBackoff},
		{"near_max", MaxBackoff - 1*time.Second},
		{"exceeds_max_after_factor", 6 * time.Second}, // 6s * 2 = 12s > 10s MaxBackoff
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := NextBackoff(tt.current)

			assert.LessOrEqual(t, result, MaxBackoff, "backoff should not exceed MaxBackoff")
		})
	}
}

func TestNextBackoff_ZeroCurrent(t *testing.T) {
	t.Parallel()

	result := NextBackoff(0)

	assert.Equal(t, time.Duration(0), result, "backoff from zero should be zero")
}

// Property-based tests verify invariants hold across many inputs

func TestFullJitter_AlwaysNonNegative(t *testing.T) {
	t.Parallel()

	// Test with various durations including edge cases
	testDurations := []time.Duration{
		0,
		1 * time.Nanosecond,
		1 * time.Microsecond,
		1 * time.Millisecond,
		100 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
		MaxBackoff,
		MaxBackoff * 2,
		100 * time.Second,
	}

	for _, duration := range testDurations {
		// Run multiple times per duration to account for randomness
		for i := 0; i < 50; i++ {
			result := FullJitter(duration)
			assert.GreaterOrEqual(t, result, time.Duration(0),
				"PROPERTY VIOLATED: FullJitter(%v) returned negative value %v", duration, result)
		}
	}
}

func TestFullJitter_NeverExceedsBound(t *testing.T) {
	t.Parallel()

	testDurations := []time.Duration{
		1 * time.Millisecond,
		100 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
		MaxBackoff,
		MaxBackoff * 2,
		100 * time.Second,
	}

	for _, duration := range testDurations {
		expectedMax := duration
		if expectedMax > MaxBackoff {
			expectedMax = MaxBackoff
		}

		// Run multiple times per duration to account for randomness
		for i := 0; i < 50; i++ {
			result := FullJitter(duration)
			assert.LessOrEqual(t, result, expectedMax,
				"PROPERTY VIOLATED: FullJitter(%v) returned %v, expected <= %v", duration, result, expectedMax)
		}
	}
}

func TestNextBackoff_NeverExceedsMax(t *testing.T) {
	t.Parallel()

	testDurations := []time.Duration{
		0,
		1 * time.Millisecond,
		100 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
		MaxBackoff,
		MaxBackoff * 2,
	}

	for _, duration := range testDurations {
		result := NextBackoff(duration)
		assert.LessOrEqual(t, result, MaxBackoff,
			"PROPERTY VIOLATED: NextBackoff(%v) returned %v, expected <= MaxBackoff(%v)", duration, result, MaxBackoff)
	}
}

func TestNextBackoff_Monotonic(t *testing.T) {
	t.Parallel()

	// Property: NextBackoff(a) >= a for all a < MaxBackoff (backoff never decreases until cap)
	testDurations := []time.Duration{
		1 * time.Millisecond,
		100 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
	}

	for _, duration := range testDurations {
		result := NextBackoff(duration)
		assert.GreaterOrEqual(t, result, duration,
			"PROPERTY VIOLATED: NextBackoff(%v) returned %v, expected >= input (monotonic increase)", duration, result)
	}
}

// Test constants are correctly defined
func TestConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 5, MaxRetries, "MaxRetries should be 5")
	assert.Equal(t, 500*time.Millisecond, InitialBackoff, "InitialBackoff should be 500ms")
	assert.Equal(t, 10*time.Second, MaxBackoff, "MaxBackoff should be 10s")
	assert.Equal(t, 2.0, BackoffFactor, "BackoffFactor should be 2.0")
}
