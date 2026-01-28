package utils

import (
	"math/rand"
	"sync"
	"time"
)

// Default values for retry configuration (preserved for backward compatibility)
const (
	DefaultMaxRetries     = 5
	DefaultInitialBackoff = 500 * time.Millisecond
	DefaultMaxBackoff     = 10 * time.Second
	DefaultBackoffFactor  = 2.0
)

// retryConfig holds the cached retry configuration loaded from environment variables.
// Uses sync.Once to ensure configuration is loaded only once (thread-safe singleton).
type retryConfig struct {
	maxRetries     int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	backoffFactor  float64
}

var (
	config     retryConfig
	configOnce sync.Once
	configMu   sync.Mutex
)

// loadConfig loads retry configuration from environment variables with defaults.
// Called once via sync.Once to ensure thread-safe initialization.
// Note: Invalid env values are silently corrected here; use LogRetryConfigWarnings
// at bootstrap time (where a logger is available) to emit warnings.
func loadConfig() {
	config = retryConfig{
		maxRetries:     GetEnvInt("RETRY_MAX_RETRIES", DefaultMaxRetries),
		initialBackoff: GetEnvDuration("RETRY_INITIAL_BACKOFF", DefaultInitialBackoff),
		maxBackoff:     GetEnvDuration("RETRY_MAX_BACKOFF", DefaultMaxBackoff),
		backoffFactor:  GetEnvFloat64("RETRY_BACKOFF_FACTOR", DefaultBackoffFactor),
	}

	if config.maxRetries < 0 {
		config.maxRetries = DefaultMaxRetries
	}

	if config.initialBackoff <= 0 {
		config.initialBackoff = DefaultInitialBackoff
	}

	if config.maxBackoff <= 0 {
		config.maxBackoff = DefaultMaxBackoff
	}

	if config.backoffFactor < 1.0 {
		config.backoffFactor = DefaultBackoffFactor
	}

	if config.initialBackoff > config.maxBackoff {
		config.initialBackoff = config.maxBackoff
	}
}

func getConfig() *retryConfig {
	configMu.Lock()
	defer configMu.Unlock()

	configOnce.Do(loadConfig)

	return &config
}

// MaxRetries returns the configured maximum number of retries.
// Reads from RETRY_MAX_RETRIES environment variable, defaults to 5.
func MaxRetries() int {
	return getConfig().maxRetries
}

// InitialBackoff returns the configured initial backoff duration.
// Reads from RETRY_INITIAL_BACKOFF environment variable, defaults to 500ms.
func InitialBackoff() time.Duration {
	return getConfig().initialBackoff
}

// MaxBackoff returns the configured maximum backoff duration.
// Reads from RETRY_MAX_BACKOFF environment variable, defaults to 10s.
func MaxBackoff() time.Duration {
	return getConfig().maxBackoff
}

// BackoffFactor returns the configured backoff multiplier.
// Reads from RETRY_BACKOFF_FACTOR environment variable, defaults to 2.0.
func BackoffFactor() float64 {
	return getConfig().backoffFactor
}

// FullJitter returns a random delay between [0, baseDelay], capped by MaxBackoff.
func FullJitter(baseDelay time.Duration) time.Duration {
	maxBackoff := MaxBackoff()

	// #nosec G404
	jitter := time.Duration(rand.Float64() * float64(baseDelay))
	if jitter > maxBackoff {
		return maxBackoff
	}

	return jitter
}

// NextBackoff calculates the next exponential backoff, respecting the MaxBackoff cap.
func NextBackoff(current time.Duration) time.Duration {
	backoffFactor := BackoffFactor()
	maxBackoff := MaxBackoff()

	next := time.Duration(float64(current) * backoffFactor)
	if next > maxBackoff {
		return maxBackoff
	}

	return next
}
