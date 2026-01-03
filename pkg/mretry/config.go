// Package mretry provides shared retry configuration for workers and consumers.
package mretry

import "time"

// Config defines common retry behavior configuration.
// Used by workers and consumers that implement retry logic with exponential backoff.
type Config struct {
	// MaxRetries is the maximum number of retry attempts before moving to DLQ or giving up.
	MaxRetries int

	// InitialBackoff is the initial delay before the first retry attempt.
	InitialBackoff time.Duration

	// MaxBackoff is the maximum delay between retry attempts (backoff cap).
	MaxBackoff time.Duration

	// JitterFactor is the percentage of jitter to add to backoff (0.0-1.0).
	// For example, 0.25 means 25% jitter.
	JitterFactor float64
}

// Default retry configuration values.
const (
	// DefaultMaxRetries is the default maximum retry attempts.
	DefaultMaxRetries = 10

	// DefaultInitialBackoff is the default initial backoff delay.
	DefaultInitialBackoff = 1 * time.Second

	// DefaultMaxBackoff is the default maximum backoff delay.
	DefaultMaxBackoff = 30 * time.Minute

	// DefaultJitterFactor is the default jitter percentage (25%).
	DefaultJitterFactor = 0.25

	// DLQ-specific defaults (longer backoffs for infrastructure recovery).
	DLQInitialBackoff = 1 * time.Minute
)

// DefaultMetadataOutboxConfig returns the default retry config for metadata outbox worker.
// Uses shorter initial backoff (1s) suitable for transient failures.
func DefaultMetadataOutboxConfig() Config {
	return Config{
		MaxRetries:     DefaultMaxRetries,
		InitialBackoff: DefaultInitialBackoff,
		MaxBackoff:     DefaultMaxBackoff,
		JitterFactor:   DefaultJitterFactor,
	}
}

// DefaultDLQConfig returns the default retry config for DLQ processing.
// Uses longer initial backoff (1m) because DLQ processing happens after
// infrastructure recovery and should allow more time between attempts.
func DefaultDLQConfig() Config {
	return Config{
		MaxRetries:     DefaultMaxRetries,
		InitialBackoff: DLQInitialBackoff,
		MaxBackoff:     DefaultMaxBackoff,
		JitterFactor:   DefaultJitterFactor,
	}
}

// WithMaxRetries returns a copy of the config with the specified max retries.
func (c Config) WithMaxRetries(maxRetries int) Config {
	c.MaxRetries = maxRetries
	return c
}

// WithInitialBackoff returns a copy of the config with the specified initial backoff.
func (c Config) WithInitialBackoff(backoff time.Duration) Config {
	c.InitialBackoff = backoff
	return c
}

// WithMaxBackoff returns a copy of the config with the specified max backoff.
func (c Config) WithMaxBackoff(backoff time.Duration) Config {
	c.MaxBackoff = backoff
	return c
}

// WithJitterFactor returns a copy of the config with the specified jitter factor.
func (c Config) WithJitterFactor(jitter float64) Config {
	c.JitterFactor = jitter
	return c
}

// ConfigValidationError represents a validation error for retry configuration.
type ConfigValidationError struct {
	Field   string
	Message string
}

func (e ConfigValidationError) Error() string {
	return "mretry: invalid " + e.Field + ": " + e.Message
}

// Validate checks if the Config has valid values.
// Returns an error if any validation fails:
//   - MaxRetries must be >= 1
//   - InitialBackoff must be > 0
//   - MaxBackoff must be > 0
//   - MaxBackoff must be >= InitialBackoff
//   - JitterFactor must be in range [0.0, 1.0]
func (c Config) Validate() error {
	if c.MaxRetries < 1 {
		return ConfigValidationError{Field: "MaxRetries", Message: "must be >= 1"}
	}

	if c.InitialBackoff <= 0 {
		return ConfigValidationError{Field: "InitialBackoff", Message: "must be > 0"}
	}

	if c.MaxBackoff <= 0 {
		return ConfigValidationError{Field: "MaxBackoff", Message: "must be > 0"}
	}

	if c.MaxBackoff < c.InitialBackoff {
		return ConfigValidationError{Field: "MaxBackoff", Message: "must be >= InitialBackoff"}
	}

	if c.JitterFactor < 0.0 || c.JitterFactor > 1.0 {
		return ConfigValidationError{Field: "JitterFactor", Message: "must be in range [0.0, 1.0]"}
	}

	return nil
}
