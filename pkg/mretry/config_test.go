package mretry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultMetadataOutboxConfig(t *testing.T) {
	cfg := DefaultMetadataOutboxConfig()

	assert.Equal(t, DefaultMaxRetries, cfg.MaxRetries)
	assert.Equal(t, DefaultInitialBackoff, cfg.InitialBackoff)
	assert.Equal(t, DefaultMaxBackoff, cfg.MaxBackoff)
	assert.Equal(t, DefaultJitterFactor, cfg.JitterFactor)
}

func TestDefaultDLQConfig(t *testing.T) {
	cfg := DefaultDLQConfig()

	assert.Equal(t, DefaultMaxRetries, cfg.MaxRetries)
	assert.Equal(t, DLQInitialBackoff, cfg.InitialBackoff)
	assert.Equal(t, DefaultMaxBackoff, cfg.MaxBackoff)
	assert.Equal(t, DefaultJitterFactor, cfg.JitterFactor)
}

func TestConfig_WithMaxRetries(t *testing.T) {
	cfg := DefaultMetadataOutboxConfig().WithMaxRetries(5)

	assert.Equal(t, 5, cfg.MaxRetries)
	// Other fields should remain unchanged
	assert.Equal(t, DefaultInitialBackoff, cfg.InitialBackoff)
}

func TestConfig_WithInitialBackoff(t *testing.T) {
	cfg := DefaultMetadataOutboxConfig().WithInitialBackoff(2 * time.Second)

	assert.Equal(t, 2*time.Second, cfg.InitialBackoff)
	// Other fields should remain unchanged
	assert.Equal(t, DefaultMaxRetries, cfg.MaxRetries)
}

func TestConfig_WithMaxBackoff(t *testing.T) {
	cfg := DefaultMetadataOutboxConfig().WithMaxBackoff(1 * time.Hour)

	assert.Equal(t, 1*time.Hour, cfg.MaxBackoff)
	// Other fields should remain unchanged
	assert.Equal(t, DefaultMaxRetries, cfg.MaxRetries)
}

func TestConfig_WithJitterFactor(t *testing.T) {
	cfg := DefaultMetadataOutboxConfig().WithJitterFactor(0.5)

	assert.Equal(t, 0.5, cfg.JitterFactor)
	// Other fields should remain unchanged
	assert.Equal(t, DefaultMaxRetries, cfg.MaxRetries)
}

func TestConfig_Chaining(t *testing.T) {
	cfg := DefaultMetadataOutboxConfig().
		WithMaxRetries(5).
		WithInitialBackoff(2 * time.Second).
		WithMaxBackoff(1 * time.Hour).
		WithJitterFactor(0.5)

	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 2*time.Second, cfg.InitialBackoff)
	assert.Equal(t, 1*time.Hour, cfg.MaxBackoff)
	assert.Equal(t, 0.5, cfg.JitterFactor)
}

func TestDefaultValues(t *testing.T) {
	// Verify default constants match expected values
	assert.Equal(t, 10, DefaultMaxRetries)
	assert.Equal(t, 1*time.Second, DefaultInitialBackoff)
	assert.Equal(t, 30*time.Minute, DefaultMaxBackoff)
	assert.Equal(t, 0.25, DefaultJitterFactor)
	assert.Equal(t, 1*time.Minute, DLQInitialBackoff)
}

func TestConfig_Validate_ValidConfig(t *testing.T) {
	// Default configs should be valid
	assert.NoError(t, DefaultMetadataOutboxConfig().Validate())
	assert.NoError(t, DefaultDLQConfig().Validate())

	// Custom valid config
	cfg := Config{
		MaxRetries:     1,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     1 * time.Millisecond,
		JitterFactor:   0.0,
	}
	assert.NoError(t, cfg.Validate())

	// Edge case: JitterFactor = 1.0
	cfg.JitterFactor = 1.0
	assert.NoError(t, cfg.Validate())
}

func TestConfig_Validate_InvalidMaxRetries(t *testing.T) {
	cfg := DefaultMetadataOutboxConfig().WithMaxRetries(0)
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MaxRetries")
	assert.Contains(t, err.Error(), "must be >= 1")

	// Negative value
	cfg = DefaultMetadataOutboxConfig().WithMaxRetries(-1)
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MaxRetries")
}

func TestConfig_Validate_InvalidInitialBackoff(t *testing.T) {
	cfg := DefaultMetadataOutboxConfig().WithInitialBackoff(0)
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "InitialBackoff")
	assert.Contains(t, err.Error(), "must be > 0")

	// Negative value
	cfg = DefaultMetadataOutboxConfig().WithInitialBackoff(-1 * time.Second)
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "InitialBackoff")
}

func TestConfig_Validate_InvalidMaxBackoff(t *testing.T) {
	cfg := DefaultMetadataOutboxConfig().WithMaxBackoff(0)
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MaxBackoff")
	assert.Contains(t, err.Error(), "must be > 0")

	// Negative value
	cfg = DefaultMetadataOutboxConfig().WithMaxBackoff(-1 * time.Second)
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MaxBackoff")
}

func TestConfig_Validate_MaxBackoffLessThanInitial(t *testing.T) {
	cfg := Config{
		MaxRetries:     10,
		InitialBackoff: 10 * time.Second,
		MaxBackoff:     5 * time.Second, // Less than InitialBackoff
		JitterFactor:   0.25,
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MaxBackoff")
	assert.Contains(t, err.Error(), "must be >= InitialBackoff")
}

func TestConfig_Validate_InvalidJitterFactor(t *testing.T) {
	// Negative jitter
	cfg := DefaultMetadataOutboxConfig().WithJitterFactor(-0.1)
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "JitterFactor")
	assert.Contains(t, err.Error(), "must be in range [0.0, 1.0]")

	// Jitter > 1.0
	cfg = DefaultMetadataOutboxConfig().WithJitterFactor(1.1)
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "JitterFactor")
}

func TestConfigValidationError_Error(t *testing.T) {
	err := ConfigValidationError{Field: "TestField", Message: "test message"}
	assert.Equal(t, "mretry: invalid TestField: test message", err.Error())
}
