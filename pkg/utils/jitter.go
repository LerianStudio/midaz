// Package utils provides utility functions and helpers used across the Midaz ledger system.
// This file contains retry and backoff utilities for handling transient failures in distributed systems.
package utils

import (
	"math/rand"
	"time"
)

// Retry and Backoff Configuration Constants
//
// These constants define the default behavior for exponential backoff with jitter,
// a common pattern for handling transient failures in distributed systems (e.g.,
// database connection failures, message broker unavailability, network timeouts).
const (
	// MaxRetries defines the maximum number of retry attempts before giving up.
	// After 5 retries with exponential backoff, the operation is considered failed.
	MaxRetries = 5

	// InitialBackoff is the starting delay duration for the first retry attempt.
	// Subsequent retries use exponential backoff based on this initial value.
	// 500ms provides a reasonable starting point that balances responsiveness
	// with giving the system time to recover.
	InitialBackoff = 500 * time.Millisecond

	// MaxBackoff is the maximum delay duration between retry attempts.
	// This cap prevents exponential backoff from growing indefinitely and ensures
	// retries happen within a reasonable timeframe. 10 seconds is chosen to balance
	// system recovery time with user experience.
	MaxBackoff = 10 * time.Second

	// BackoffFactor is the multiplier used for exponential backoff calculation.
	// Each retry delay is calculated as: previous_delay * BackoffFactor.
	// A factor of 2.0 provides standard exponential growth:
	// 500ms -> 1s -> 2s -> 4s -> 8s -> 10s (capped)
	BackoffFactor = 2.0
)

// FullJitter returns a random delay between [0, baseDelay], capped by MaxBackoff.
//
// This function implements the "Full Jitter" strategy from AWS Architecture Blog's
// "Exponential Backoff And Jitter" article. Full jitter helps prevent the "thundering herd"
// problem where many clients retry simultaneously, potentially overwhelming a recovering service.
//
// The jitter is uniformly distributed across [0, baseDelay], which provides:
// - Better distribution of retry attempts over time
// - Reduced collision probability between concurrent clients
// - Improved system stability during recovery periods
//
// Parameters:
//   - baseDelay: The maximum delay duration for this retry attempt
//
// Returns:
//   - A random duration between 0 and baseDelay, capped at MaxBackoff
//
// Example:
//
//	delay := utils.FullJitter(2 * time.Second)
//	// Returns a random value between 0 and 2 seconds
//	time.Sleep(delay)
//
// Security Note: Uses math/rand instead of crypto/rand for performance.
// This is acceptable for jitter as cryptographic randomness is not required.
// The #nosec G404 directive suppresses the gosec security warning.
func FullJitter(baseDelay time.Duration) time.Duration {
	// #nosec G404 - Non-cryptographic randomness is sufficient for jitter
	jitter := time.Duration(rand.Float64() * float64(baseDelay))
	if jitter > MaxBackoff {
		return MaxBackoff
	}

	return jitter
}

// NextBackoff calculates the next exponential backoff delay, respecting the MaxBackoff cap.
//
// This function implements exponential backoff by multiplying the current delay by BackoffFactor.
// Exponential backoff is a standard technique for handling transient failures in distributed
// systems, gradually increasing the delay between retries to:
// - Give failing services more time to recover
// - Reduce load on struggling systems
// - Prevent resource exhaustion from aggressive retries
//
// The calculation follows the pattern: next_delay = current_delay * BackoffFactor
// With BackoffFactor = 2.0, the sequence is: 500ms, 1s, 2s, 4s, 8s, 10s (capped)
//
// Parameters:
//   - current: The current delay duration
//
// Returns:
//   - The next delay duration (current * BackoffFactor), capped at MaxBackoff
//
// Example:
//
//	backoff := utils.InitialBackoff
//	for attempt := 0; attempt < utils.MaxRetries; attempt++ {
//	    if err := doOperation(); err != nil {
//	        time.Sleep(utils.FullJitter(backoff))
//	        backoff = utils.NextBackoff(backoff)
//	        continue
//	    }
//	    break
//	}
//
// Usage Pattern:
//
//	This function is typically used in conjunction with FullJitter:
//	1. Start with InitialBackoff
//	2. On failure, sleep for FullJitter(backoff)
//	3. Calculate next backoff using NextBackoff(backoff)
//	4. Repeat until success or MaxRetries reached
func NextBackoff(current time.Duration) time.Duration {
	next := time.Duration(float64(current) * BackoffFactor)
	if next > MaxBackoff {
		return MaxBackoff
	}

	return next
}
