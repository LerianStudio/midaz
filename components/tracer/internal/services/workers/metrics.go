// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	libMetrics "github.com/LerianStudio/lib-observability/metrics"
)

// Metric is an alias for libMetrics.Metric to allow local usage without importing.
type Metric = libMetrics.Metric

// MetricCacheSyncPollsTotal tracks the total number of cache sync poll attempts.
// Labels: status (success, error, skipped)
// - success: poll completed and changes applied (or no changes found)
// - error: poll failed (DB error, compilation error)
// - skipped: poll skipped because circuit breaker is open
var MetricCacheSyncPollsTotal = Metric{
	Name:        "tracer_cache_sync_polls_total",
	Unit:        "1",
	Description: "Total cache sync poll attempts by status",
}

// MetricCacheSyncErrorsTotal tracks cache sync errors by reason.
// Labels: reason (db_error, compile_error, circuit_open)
var MetricCacheSyncErrorsTotal = Metric{
	Name:        "tracer_cache_sync_errors_total",
	Unit:        "1",
	Description: "Total cache sync errors by reason",
}

// MetricCacheSyncDuration tracks the duration of each sync cycle.
// Measured from start to end of runSyncCycle, excluding skipped polls.
var MetricCacheSyncDuration = Metric{
	Name:        "tracer_cache_sync_duration_milliseconds",
	Unit:        "ms",
	Description: "Duration of cache sync poll cycles in milliseconds",
}

// MetricCacheSyncRulesChanged tracks the number of rules changed per sync cycle.
// Only incremented when changes are actually applied (upserts + removals).
var MetricCacheSyncRulesChanged = Metric{
	Name:        "tracer_cache_sync_rules_changed_total",
	Unit:        "1",
	Description: "Total rules changed per sync cycle (upserts + removals)",
}

// MetricCacheSyncRuleCacheSize tracks the current number of rules in the cache.
// Updated after each successful sync cycle. Gauge (can go up or down).
var MetricCacheSyncRuleCacheSize = Metric{
	Name:        "tracer_cache_sync_rule_cache_size",
	Unit:        "1",
	Description: "Current number of rules in the in-memory cache",
}

// MetricCacheSyncStalenessSeconds tracks time since last successful sync.
// Updated on each poll cycle (even when circuit is open). Gauge.
var MetricCacheSyncStalenessSeconds = Metric{
	Name:        "tracer_cache_sync_staleness_seconds",
	Unit:        "s",
	Description: "Cache staleness: seconds since last successful sync",
}
