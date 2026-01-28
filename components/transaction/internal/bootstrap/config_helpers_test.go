package bootstrap

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestEnvUint32_DefaultValue(t *testing.T) {
	result := utils.GetEnvUint32("TEST_ENV_UINT32_DEFAULT_UNSET", 42)

	assert.Equal(t, uint32(42), result)
}

func TestEnvUint32_ValidValue(t *testing.T) {
	t.Setenv("TEST_ENV_UINT32_VALID", "123")

	result := utils.GetEnvUint32("TEST_ENV_UINT32_VALID", 42)

	assert.Equal(t, uint32(123), result)
}

func TestEnvUint32_InvalidValue(t *testing.T) {
	t.Setenv("TEST_ENV_UINT32_INVALID", "not-a-number")

	result := utils.GetEnvUint32("TEST_ENV_UINT32_INVALID", 42)

	assert.Equal(t, uint32(42), result)
}

func TestEnvUint32_NegativeValue(t *testing.T) {
	t.Setenv("TEST_ENV_UINT32_NEGATIVE", "-5")

	result := utils.GetEnvUint32("TEST_ENV_UINT32_NEGATIVE", 42)

	assert.Equal(t, uint32(42), result)
}

func TestEnvFloat64_DefaultValue(t *testing.T) {
	result := utils.GetEnvFloat64("TEST_ENV_FLOAT64_DEFAULT_UNSET", 0.5)

	assert.Equal(t, 0.5, result)
}

func TestEnvFloat64_ValidValue(t *testing.T) {
	t.Setenv("TEST_ENV_FLOAT64_VALID", "0.75")

	result := utils.GetEnvFloat64("TEST_ENV_FLOAT64_VALID", 0.5)

	assert.Equal(t, 0.75, result)
}

func TestEnvFloat64_InvalidValue(t *testing.T) {
	t.Setenv("TEST_ENV_FLOAT64_INVALID", "not-a-float")

	result := utils.GetEnvFloat64("TEST_ENV_FLOAT64_INVALID", 0.5)

	assert.Equal(t, 0.5, result)
}

func TestEnvFloat64WithRange_DefaultValue(t *testing.T) {
	result := utils.GetEnvFloat64WithRange("TEST_ENV_FLOAT64_RANGE_DEFAULT_UNSET", 0.5, 0.0, 1.0)

	assert.Equal(t, 0.5, result)
}

func TestEnvFloat64WithRange_ValidValue(t *testing.T) {
	t.Setenv("TEST_ENV_FLOAT64_RANGE_VALID", "0.75")

	result := utils.GetEnvFloat64WithRange("TEST_ENV_FLOAT64_RANGE_VALID", 0.5, 0.0, 1.0)

	assert.Equal(t, 0.75, result)
}

func TestEnvFloat64WithRange_BelowMin(t *testing.T) {
	t.Setenv("TEST_ENV_FLOAT64_RANGE_BELOW", "-0.5")

	result := utils.GetEnvFloat64WithRange("TEST_ENV_FLOAT64_RANGE_BELOW", 0.5, 0.0, 1.0)

	assert.Equal(t, 0.0, result)
}

func TestEnvFloat64WithRange_AboveMax(t *testing.T) {
	t.Setenv("TEST_ENV_FLOAT64_RANGE_ABOVE", "1.5")

	result := utils.GetEnvFloat64WithRange("TEST_ENV_FLOAT64_RANGE_ABOVE", 0.5, 0.0, 1.0)

	assert.Equal(t, 1.0, result)
}

func TestEnvDuration_DefaultValue(t *testing.T) {
	result := utils.GetEnvDuration("TEST_ENV_DURATION_DEFAULT_UNSET", 30*time.Second)

	assert.Equal(t, 30*time.Second, result)
}

func TestEnvDuration_ValidValue(t *testing.T) {
	t.Setenv("TEST_ENV_DURATION_VALID", "2m")

	result := utils.GetEnvDuration("TEST_ENV_DURATION_VALID", 30*time.Second)

	assert.Equal(t, 2*time.Minute, result)
}

func TestEnvDuration_InvalidValue(t *testing.T) {
	t.Setenv("TEST_ENV_DURATION_INVALID", "not-a-duration")

	result := utils.GetEnvDuration("TEST_ENV_DURATION_INVALID", 30*time.Second)

	assert.Equal(t, 30*time.Second, result)
}

func TestEnvDuration_ComplexValue(t *testing.T) {
	t.Setenv("TEST_ENV_DURATION_COMPLEX", "1h30m45s")

	result := utils.GetEnvDuration("TEST_ENV_DURATION_COMPLEX", 30*time.Second)

	expected := 1*time.Hour + 30*time.Minute + 45*time.Second
	assert.Equal(t, expected, result)
}
