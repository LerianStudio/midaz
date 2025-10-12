package utils

import (
	"math/rand"
	"time"
)

const (
	// MaxRetries defines the default maximum number of retry attempts.
	MaxRetries = 5
	// InitialBackoff is the initial delay used when calculating backoff.
	InitialBackoff = 500 * time.Millisecond
	// MaxBackoff caps any computed backoff or jitter delay.
	MaxBackoff = 10 * time.Second
	// BackoffFactor is the multiplier for exponential backoff progression.
	BackoffFactor = 2.0
)

// FullJitter returns a random delay between [0, baseDelay], capped by MaxBackoff.
func FullJitter(baseDelay time.Duration) time.Duration {
	// #nosec G404
	jitter := time.Duration(rand.Float64() * float64(baseDelay))
	if jitter > MaxBackoff {
		return MaxBackoff
	}

	return jitter
}

// NextBackoff calculates the next exponential backoff, respecting the MaxBackoff capped.
func NextBackoff(current time.Duration) time.Duration {
	next := time.Duration(float64(current) * BackoffFactor)
	if next > MaxBackoff {
		return MaxBackoff
	}

	return next
}
