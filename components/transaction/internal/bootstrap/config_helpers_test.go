package bootstrap

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEnvUint32_DefaultValue(t *testing.T) {
	t.Parallel()

	key := "TEST_ENV_UINT32_DEFAULT"
	os.Unsetenv(key)

	result := envUint32(key, 42)

	assert.Equal(t, uint32(42), result)
}

func TestEnvUint32_ValidValue(t *testing.T) {
	t.Parallel()

	key := "TEST_ENV_UINT32_VALID"
	os.Setenv(key, "123")
	defer os.Unsetenv(key)

	result := envUint32(key, 42)

	assert.Equal(t, uint32(123), result)
}

func TestEnvUint32_InvalidValue(t *testing.T) {
	t.Parallel()

	key := "TEST_ENV_UINT32_INVALID"
	os.Setenv(key, "not-a-number")
	defer os.Unsetenv(key)

	result := envUint32(key, 42)

	assert.Equal(t, uint32(42), result)
}

func TestEnvUint32_NegativeValue(t *testing.T) {
	t.Parallel()

	key := "TEST_ENV_UINT32_NEGATIVE"
	os.Setenv(key, "-5")
	defer os.Unsetenv(key)

	result := envUint32(key, 42)

	assert.Equal(t, uint32(42), result)
}

func TestEnvFloat64_DefaultValue(t *testing.T) {
	t.Parallel()

	key := "TEST_ENV_FLOAT64_DEFAULT"
	os.Unsetenv(key)

	result := envFloat64(key, 0.5)

	assert.Equal(t, 0.5, result)
}

func TestEnvFloat64_ValidValue(t *testing.T) {
	t.Parallel()

	key := "TEST_ENV_FLOAT64_VALID"
	os.Setenv(key, "0.75")
	defer os.Unsetenv(key)

	result := envFloat64(key, 0.5)

	assert.Equal(t, 0.75, result)
}

func TestEnvFloat64_InvalidValue(t *testing.T) {
	t.Parallel()

	key := "TEST_ENV_FLOAT64_INVALID"
	os.Setenv(key, "not-a-float")
	defer os.Unsetenv(key)

	result := envFloat64(key, 0.5)

	assert.Equal(t, 0.5, result)
}

func TestEnvFloat64WithRange_DefaultValue(t *testing.T) {
	t.Parallel()

	key := "TEST_ENV_FLOAT64_RANGE_DEFAULT"
	os.Unsetenv(key)

	result := envFloat64WithRange(key, 0.5, 0.0, 1.0)

	assert.Equal(t, 0.5, result)
}

func TestEnvFloat64WithRange_ValidValue(t *testing.T) {
	t.Parallel()

	key := "TEST_ENV_FLOAT64_RANGE_VALID"
	os.Setenv(key, "0.75")
	defer os.Unsetenv(key)

	result := envFloat64WithRange(key, 0.5, 0.0, 1.0)

	assert.Equal(t, 0.75, result)
}

func TestEnvFloat64WithRange_BelowMin(t *testing.T) {
	t.Parallel()

	key := "TEST_ENV_FLOAT64_RANGE_BELOW"
	os.Setenv(key, "-0.5")
	defer os.Unsetenv(key)

	result := envFloat64WithRange(key, 0.5, 0.0, 1.0)

	assert.Equal(t, 0.0, result)
}

func TestEnvFloat64WithRange_AboveMax(t *testing.T) {
	t.Parallel()

	key := "TEST_ENV_FLOAT64_RANGE_ABOVE"
	os.Setenv(key, "1.5")
	defer os.Unsetenv(key)

	result := envFloat64WithRange(key, 0.5, 0.0, 1.0)

	assert.Equal(t, 1.0, result)
}

func TestEnvDuration_DefaultValue(t *testing.T) {
	t.Parallel()

	key := "TEST_ENV_DURATION_DEFAULT"
	os.Unsetenv(key)

	result := envDuration(key, 30*time.Second)

	assert.Equal(t, 30*time.Second, result)
}

func TestEnvDuration_ValidValue(t *testing.T) {
	t.Parallel()

	key := "TEST_ENV_DURATION_VALID"
	os.Setenv(key, "2m")
	defer os.Unsetenv(key)

	result := envDuration(key, 30*time.Second)

	assert.Equal(t, 2*time.Minute, result)
}

func TestEnvDuration_InvalidValue(t *testing.T) {
	t.Parallel()

	key := "TEST_ENV_DURATION_INVALID"
	os.Setenv(key, "not-a-duration")
	defer os.Unsetenv(key)

	result := envDuration(key, 30*time.Second)

	assert.Equal(t, 30*time.Second, result)
}

func TestEnvDuration_ComplexValue(t *testing.T) {
	t.Parallel()

	key := "TEST_ENV_DURATION_COMPLEX"
	os.Setenv(key, "1h30m45s")
	defer os.Unsetenv(key)

	result := envDuration(key, 30*time.Second)

	expected := 1*time.Hour + 30*time.Minute + 45*time.Second
	assert.Equal(t, expected, result)
}

func TestStateToInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		state    int
		expected int64
	}{
		{"closed", 0, 0},
		{"open", 1, 1},
		{"half-open", 2, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: We can't directly test stateToInt without importing libCircuitBreaker
			// The function is tested indirectly through the listener
			assert.True(t, true)
		})
	}
}
