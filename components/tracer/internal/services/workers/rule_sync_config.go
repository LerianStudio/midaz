// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"time"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/resilience"
)

// RuleSyncWorkerConfig holds configuration for the rule sync worker.
type RuleSyncWorkerConfig struct {
	// PollInterval is how often the worker polls for rule changes (default: 10s).
	PollInterval time.Duration
	// StalenessThreshold is the duration after which the cache is considered stale (default: 50s).
	// Used by health checker to report DEGRADED state when cache has not been refreshed within this duration.
	StalenessThreshold time.Duration
	// OverlapBuffer is subtracted from lastSync when querying deltas (default: 2s).
	// Ensures no changes are missed at poll boundaries due to clock skew or transaction lag.
	OverlapBuffer time.Duration
}

// DefaultRuleSyncWorkerConfig returns default configuration values.
func DefaultRuleSyncWorkerConfig() RuleSyncWorkerConfig {
	return RuleSyncWorkerConfig{
		PollInterval:       10 * time.Second,
		StalenessThreshold: 50 * time.Second,
		OverlapBuffer:      2 * time.Second,
	}
}

// DefaultSyncCircuitBreakerConfig returns circuit breaker configuration
// tuned for the polling sync worker.
// - 3 consecutive failures -> circuit opens (fast detection for polling)
// - 30s timeout before half-open probe
// - 1 request allowed in half-open state (single probe)
// - Failure ratio disabled (0) -- only consecutive failures count
// IMPORTANT: Interval must be 0 when FailureRatio=0 (consecutive-only mode).
// If FailureRatio is ever enabled, set Interval to a bounded window.
func DefaultSyncCircuitBreakerConfig() resilience.CircuitBreakerConfig {
	return resilience.CircuitBreakerConfig{
		Name:          "rule_sync_poller",
		MaxRequests:   1,                // 1 probe in half-open
		Interval:      0,                // no cyclic reset (rely on consecutive only)
		Timeout:       30 * time.Second, // wait 30s before half-open probe
		FailureThresh: 3,                // trip after 3 consecutive failures
		FailureRatio:  0,                // disabled
		MinRequests:   0,                // disabled
	}
}
