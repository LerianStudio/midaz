// Package helpers provides test utilities for the Midaz test suite.
package helpers

import "time"

// Standardized poll intervals for test helpers.
// These values are tuned for the balance between test speed and reliability.
//
// Design rationale:
// - Fast polling (100ms): Use for quick checks where latency matters
// - Standard polling (150ms): Use for balance convergence checks
// - Slow polling (300ms+): Use for infrastructure checks (HTTP health, TCP)
//
// If you need to add a new poll interval, consider:
// 1. Can an existing interval be reused?
// 2. Is the new interval justified by specific timing requirements?
// 3. Document why a custom interval is needed
const (
	// PollIntervalFast is for quick convergence checks (100ms)
	// Use for: Redis balance polling, balance tracking changes
	PollIntervalFast = 100 * time.Millisecond

	// PollIntervalStandard is for standard balance checks (150ms)
	// Use for: Balance availability polling, asset setup polling
	PollIntervalStandard = 150 * time.Millisecond

	// PollIntervalSlow is for infrastructure checks (300ms)
	// Use for: HTTP health checks, TCP connectivity, environment setup
	PollIntervalSlow = 300 * time.Millisecond

	// PollIntervalDLQ is for DLQ message count checks (5s)
	// Use for: Waiting for DLQ to empty after chaos tests
	// Higher interval because DLQ replay has exponential backoff
	PollIntervalDLQ = 5 * time.Second
)
