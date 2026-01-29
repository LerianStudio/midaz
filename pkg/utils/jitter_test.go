package utils

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// setupTest resets the configuration singleton and clears environment variables.
// Note: t.Parallel() is intentionally NOT used in these tests because:
// 1. Tests modify global state (config singleton via sync.Once)
// 2. Tests modify shared environment variables
// 3. ResetConfigForTesting() is not safe for concurrent use
func setupTest(t *testing.T) {
	t.Helper()

	originalMaxRetries := os.Getenv("RETRY_MAX_RETRIES")
	originalInitialBackoff := os.Getenv("RETRY_INITIAL_BACKOFF")
	originalMaxBackoff := os.Getenv("RETRY_MAX_BACKOFF")
	originalBackoffFactor := os.Getenv("RETRY_BACKOFF_FACTOR")

	t.Cleanup(func() {
		if originalMaxRetries != "" {
			os.Setenv("RETRY_MAX_RETRIES", originalMaxRetries)
		} else {
			os.Unsetenv("RETRY_MAX_RETRIES")
		}
		if originalInitialBackoff != "" {
			os.Setenv("RETRY_INITIAL_BACKOFF", originalInitialBackoff)
		} else {
			os.Unsetenv("RETRY_INITIAL_BACKOFF")
		}
		if originalMaxBackoff != "" {
			os.Setenv("RETRY_MAX_BACKOFF", originalMaxBackoff)
		} else {
			os.Unsetenv("RETRY_MAX_BACKOFF")
		}
		if originalBackoffFactor != "" {
			os.Setenv("RETRY_BACKOFF_FACTOR", originalBackoffFactor)
		} else {
			os.Unsetenv("RETRY_BACKOFF_FACTOR")
		}
		ResetConfigForTesting()
	})

	ResetConfigForTesting()
	os.Unsetenv("RETRY_MAX_RETRIES")
	os.Unsetenv("RETRY_INITIAL_BACKOFF")
	os.Unsetenv("RETRY_MAX_BACKOFF")
	os.Unsetenv("RETRY_BACKOFF_FACTOR")
}

func TestFullJitter_ReturnsWithinRange(t *testing.T) {
	setupTest(t)

	tests := []struct {
		name      string
		baseDelay time.Duration
	}{
		{"small_delay", 100 * time.Millisecond},
		{"medium_delay", 1 * time.Second},
		{"large_delay", 5 * time.Second},
		{"initial_backoff", InitialBackoff()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < 100; i++ {
				result := FullJitter(tt.baseDelay)

				assert.GreaterOrEqual(t, result, time.Duration(0), "jitter should be non-negative")
				assert.LessOrEqual(t, result, tt.baseDelay, "jitter should not exceed baseDelay")
			}
		})
	}
}

func TestFullJitter_RespectsMaxBackoff(t *testing.T) {
	setupTest(t)

	baseDelay := MaxBackoff() * 3

	for i := 0; i < 100; i++ {
		result := FullJitter(baseDelay)

		assert.GreaterOrEqual(t, result, time.Duration(0), "jitter should be non-negative")
		assert.LessOrEqual(t, result, MaxBackoff(), "jitter should be capped at MaxBackoff")
	}
}

func TestFullJitter_ZeroBaseDelay(t *testing.T) {
	setupTest(t)

	result := FullJitter(0)

	assert.Equal(t, time.Duration(0), result, "jitter with zero baseDelay should be zero")
}

func TestNextBackoff_ExponentialGrowth(t *testing.T) {
	setupTest(t)

	tests := []struct {
		name     string
		current  time.Duration
		expected time.Duration
	}{
		{"from_initial", 500 * time.Millisecond, 1 * time.Second},
		{"from_1_second", 1 * time.Second, 2 * time.Second},
		{"from_2_seconds", 2 * time.Second, 4 * time.Second},
		{"from_500ms", 500 * time.Millisecond, 1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NextBackoff(tt.current)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNextBackoff_RespectsMaxBackoff(t *testing.T) {
	setupTest(t)

	tests := []struct {
		name    string
		current time.Duration
	}{
		{"at_max", MaxBackoff()},
		{"near_max", MaxBackoff() - 1*time.Second},
		{"exceeds_max_after_factor", 6 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NextBackoff(tt.current)

			assert.LessOrEqual(t, result, MaxBackoff(), "backoff should not exceed MaxBackoff")
		})
	}
}

func TestNextBackoff_ZeroCurrent(t *testing.T) {
	setupTest(t)

	result := NextBackoff(0)

	assert.Equal(t, time.Duration(0), result, "backoff from zero should be zero")
}

func TestFullJitter_AlwaysNonNegative(t *testing.T) {
	setupTest(t)

	testDurations := []time.Duration{
		0,
		1 * time.Nanosecond,
		1 * time.Microsecond,
		1 * time.Millisecond,
		100 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
		MaxBackoff(),
		MaxBackoff() * 2,
		100 * time.Second,
	}

	for _, duration := range testDurations {
		for i := 0; i < 50; i++ {
			result := FullJitter(duration)
			assert.GreaterOrEqual(t, result, time.Duration(0),
				"PROPERTY VIOLATED: FullJitter(%v) returned negative value %v", duration, result)
		}
	}
}

func TestFullJitter_NeverExceedsBound(t *testing.T) {
	setupTest(t)

	testDurations := []time.Duration{
		1 * time.Millisecond,
		100 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
		MaxBackoff(),
		MaxBackoff() * 2,
		100 * time.Second,
	}

	for _, duration := range testDurations {
		expectedMax := duration
		if expectedMax > MaxBackoff() {
			expectedMax = MaxBackoff()
		}

		for i := 0; i < 50; i++ {
			result := FullJitter(duration)
			assert.LessOrEqual(t, result, expectedMax,
				"PROPERTY VIOLATED: FullJitter(%v) returned %v, expected <= %v", duration, result, expectedMax)
		}
	}
}

func TestNextBackoff_NeverExceedsMax(t *testing.T) {
	setupTest(t)

	testDurations := []time.Duration{
		0,
		1 * time.Millisecond,
		100 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
		MaxBackoff(),
		MaxBackoff() * 2,
	}

	for _, duration := range testDurations {
		result := NextBackoff(duration)
		assert.LessOrEqual(t, result, MaxBackoff(),
			"PROPERTY VIOLATED: NextBackoff(%v) returned %v, expected <= MaxBackoff(%v)", duration, result, MaxBackoff())
	}
}

func TestNextBackoff_Monotonic(t *testing.T) {
	setupTest(t)

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

func TestDefaultConstants(t *testing.T) {
	setupTest(t)

	assert.Equal(t, 5, DefaultMaxRetries, "DefaultMaxRetries should be 5")
	assert.Equal(t, 500*time.Millisecond, DefaultInitialBackoff, "DefaultInitialBackoff should be 500ms")
	assert.Equal(t, 10*time.Second, DefaultMaxBackoff, "DefaultMaxBackoff should be 10s")
	assert.Equal(t, 2.0, DefaultBackoffFactor, "DefaultBackoffFactor should be 2.0")
}

func TestConfigFunctions_ReturnDefaults(t *testing.T) {
	setupTest(t)

	assert.Equal(t, DefaultMaxRetries, MaxRetries(), "MaxRetries() should return default")
	assert.Equal(t, DefaultInitialBackoff, InitialBackoff(), "InitialBackoff() should return default")
	assert.Equal(t, DefaultMaxBackoff, MaxBackoff(), "MaxBackoff() should return default")
	assert.Equal(t, DefaultBackoffFactor, BackoffFactor(), "BackoffFactor() should return default")
}

func TestConfigFunctions_ReadFromEnvironment(t *testing.T) {
	setupTest(t)

	os.Setenv("RETRY_MAX_RETRIES", "3")
	os.Setenv("RETRY_INITIAL_BACKOFF", "100ms")
	os.Setenv("RETRY_MAX_BACKOFF", "5s")
	os.Setenv("RETRY_BACKOFF_FACTOR", "1.5")

	assert.Equal(t, 3, MaxRetries(), "MaxRetries() should read from env")
	assert.Equal(t, 100*time.Millisecond, InitialBackoff(), "InitialBackoff() should read from env")
	assert.Equal(t, 5*time.Second, MaxBackoff(), "MaxBackoff() should read from env")
	assert.Equal(t, 1.5, BackoffFactor(), "BackoffFactor() should read from env")
}

func TestConfigFunctions_InvalidEnvValues_UseDefaults(t *testing.T) {
	setupTest(t)

	os.Setenv("RETRY_MAX_RETRIES", "not-a-number")
	os.Setenv("RETRY_INITIAL_BACKOFF", "invalid-duration")
	os.Setenv("RETRY_MAX_BACKOFF", "bad")
	os.Setenv("RETRY_BACKOFF_FACTOR", "not-float")

	assert.Equal(t, DefaultMaxRetries, MaxRetries(), "MaxRetries() should use default for invalid env")
	assert.Equal(t, DefaultInitialBackoff, InitialBackoff(), "InitialBackoff() should use default for invalid env")
	assert.Equal(t, DefaultMaxBackoff, MaxBackoff(), "MaxBackoff() should use default for invalid env")
	assert.Equal(t, DefaultBackoffFactor, BackoffFactor(), "BackoffFactor() should use default for invalid env")
}

func TestConfigValidation_NegativeMaxRetries(t *testing.T) {
	setupTest(t)

	os.Setenv("RETRY_MAX_RETRIES", "-1")

	assert.Equal(t, DefaultMaxRetries, MaxRetries(), "MaxRetries() should use default for negative value")
}

func TestConfigValidation_ZeroBackoff(t *testing.T) {
	setupTest(t)

	os.Setenv("RETRY_INITIAL_BACKOFF", "0s")
	os.Setenv("RETRY_MAX_BACKOFF", "0s")

	assert.Equal(t, DefaultInitialBackoff, InitialBackoff(), "InitialBackoff() should use default for zero")
	assert.Equal(t, DefaultMaxBackoff, MaxBackoff(), "MaxBackoff() should use default for zero")
}

func TestConfigValidation_BackoffFactorLessThanOne(t *testing.T) {
	setupTest(t)

	os.Setenv("RETRY_BACKOFF_FACTOR", "0.5")

	assert.Equal(t, DefaultBackoffFactor, BackoffFactor(), "BackoffFactor() should use default when < 1.0")
}

func TestConfigValidation_InitialExceedsMax(t *testing.T) {
	setupTest(t)

	os.Setenv("RETRY_INITIAL_BACKOFF", "20s")
	os.Setenv("RETRY_MAX_BACKOFF", "5s")

	assert.Equal(t, 5*time.Second, InitialBackoff(), "InitialBackoff() should be capped to MaxBackoff")
	assert.Equal(t, 5*time.Second, MaxBackoff(), "MaxBackoff() should be 5s")
}

func TestConfigSingleton_LoadsOnce(t *testing.T) {
	setupTest(t)

	os.Setenv("RETRY_MAX_RETRIES", "7")

	first := MaxRetries()
	assert.Equal(t, 7, first, "First call should load from env")

	os.Setenv("RETRY_MAX_RETRIES", "99")

	second := MaxRetries()
	assert.Equal(t, 7, second, "Second call should return cached value, not re-read env")
}

func TestFullJitter_WithCustomMaxBackoff(t *testing.T) {
	setupTest(t)

	os.Setenv("RETRY_MAX_BACKOFF", "2s")

	baseDelay := 10 * time.Second

	for i := 0; i < 100; i++ {
		result := FullJitter(baseDelay)
		assert.LessOrEqual(t, result, 2*time.Second, "jitter should be capped at custom MaxBackoff (2s)")
	}
}

func TestNextBackoff_WithCustomFactor(t *testing.T) {
	setupTest(t)

	os.Setenv("RETRY_BACKOFF_FACTOR", "3.0")
	os.Setenv("RETRY_MAX_BACKOFF", "30s")

	result := NextBackoff(1 * time.Second)
	assert.Equal(t, 3*time.Second, result, "NextBackoff should use custom factor (3.0)")
}

func TestConfigAtomicLoading(t *testing.T) {
	setupTest(t)

	os.Setenv("RETRY_MAX_RETRIES", "10")
	os.Setenv("RETRY_INITIAL_BACKOFF", "200ms")
	os.Setenv("RETRY_MAX_BACKOFF", "5s")
	os.Setenv("RETRY_BACKOFF_FACTOR", "1.5")

	done := make(chan struct{})
	results := make(chan int, 100)

	for i := 0; i < 100; i++ {
		go func() {
			<-done
			results <- MaxRetries()
		}()
	}

	close(done)

	first := <-results
	for i := 1; i < 100; i++ {
		val := <-results
		assert.Equal(t, first, val, "All goroutines should see the same config value")
	}

	assert.Equal(t, 10, first, "Config should be loaded from env")
}

func TestMaxRetriesZero_BehaviorDocumentation(t *testing.T) {
	setupTest(t)

	os.Setenv("RETRY_MAX_RETRIES", "0")

	result := MaxRetries()
	assert.Equal(t, 0, result, "MaxRetries=0 means no retries (only initial attempt)")
}
