package utils

import (
	"math/rand"
	"time"
)

const (
	MaxRetries     = 5
	InitialBackoff = 500 * time.Millisecond
	MaxBackoff     = 10 * time.Second
	BackoffFactor  = 2.0
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