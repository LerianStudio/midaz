// Package helpers provides test utilities for the Midaz test suite.
package helpers

import (
	"testing"
	"time"
)

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

	// PollIntervalReplica is for replica lag tolerance (50ms)
	// Use for: Verifying DELETE propagation in primary/replica setups
	PollIntervalReplica = 50 * time.Millisecond

	// MaxReplicaRetries is the maximum retries for replica lag verification
	MaxReplicaRetries = 10
)

// WaitForDeletedWithRetry polls until the getter function returns an error (indicating deletion).
// This handles replica lag in primary/replica database setups where DELETE on primary
// may not be immediately visible on replica.
// Use for: Verifying soft-delete propagation after DELETE operations.
func WaitForDeletedWithRetry(t *testing.T, resourceName string, getter func() error) {
	t.Helper()

	for i := 0; i < MaxReplicaRetries; i++ {
		err := getter()
		if err != nil {
			// Resource not found - deletion verified
			t.Logf("Verified %s deletion after %d attempts", resourceName, i+1)
			return
		}
		time.Sleep(PollIntervalReplica)
	}
	t.Errorf("GET deleted %s should fail, but succeeded after %d retries (possible replica lag)", resourceName, MaxReplicaRetries)
}

// WaitForCreatedWithRetry polls until the getter function returns success (indicating creation visible).
// This handles replica lag in primary/replica database setups where CREATE on primary
// may not be immediately visible on replica.
// Use for: Verifying resource visibility after CREATE operations.
func WaitForCreatedWithRetry(t *testing.T, resourceName string, getter func() error) {
	t.Helper()

	for i := 0; i < MaxReplicaRetries; i++ {
		err := getter()
		if err == nil {
			// Resource found - creation visible
			t.Logf("Verified %s creation visible after %d attempts", resourceName, i+1)
			return
		}
		time.Sleep(PollIntervalReplica)
	}
	t.Fatalf("GET created %s should succeed, but failed after %d retries (possible replica lag)", resourceName, MaxReplicaRetries)
}
