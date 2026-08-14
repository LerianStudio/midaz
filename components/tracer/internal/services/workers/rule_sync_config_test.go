// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestDefaultSyncCircuitBreakerConfig pins the polling-sync circuit breaker
// tuning. The values are operationally meaningful (3 consecutive failures trip
// the breaker, 30s before a single half-open probe), and the
// FailureRatio==0 <-> Interval==0 pairing is a documented invariant: the
// gobreaker consecutive-only mode requires Interval to be zero. A regression
// that enables a cyclic Interval while leaving FailureRatio disabled would
// silently change trip semantics, so it is asserted explicitly.
func TestDefaultSyncCircuitBreakerConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultSyncCircuitBreakerConfig()

	assert.Equal(t, "rule_sync_poller", cfg.Name)
	assert.Equal(t, uint32(1), cfg.MaxRequests, "single half-open probe")
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, uint32(3), cfg.FailureThresh, "trip after 3 consecutive failures")
	assert.Zero(t, cfg.FailureRatio, "failure-ratio mode disabled")
	assert.Zero(t, cfg.MinRequests)

	// Invariant: consecutive-only mode (FailureRatio==0) requires Interval==0.
	if cfg.FailureRatio == 0 {
		assert.Zero(t, cfg.Interval, "Interval must be 0 when FailureRatio is disabled")
	}
}

// TestDefaultRuleSyncWorkerConfig pins the worker polling cadence defaults and
// the ordering relationships that keep them coherent: the staleness threshold
// must exceed the poll interval (so a single missed poll does not flap health),
// and the overlap buffer must be positive and smaller than the poll interval
// (so delta queries overlap poll boundaries without re-scanning whole windows).
func TestDefaultRuleSyncWorkerConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultRuleSyncWorkerConfig()

	assert.Equal(t, 10*time.Second, cfg.PollInterval)
	assert.Equal(t, 50*time.Second, cfg.StalenessThreshold)
	assert.Equal(t, 2*time.Second, cfg.OverlapBuffer)

	assert.Greater(t, cfg.StalenessThreshold, cfg.PollInterval,
		"staleness threshold must exceed poll interval")
	assert.Positive(t, cfg.OverlapBuffer, "overlap buffer must be positive")
	assert.Less(t, cfg.OverlapBuffer, cfg.PollInterval,
		"overlap buffer must be smaller than the poll interval")
}
