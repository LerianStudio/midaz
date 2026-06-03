// Package metricskit provides metrics collection and assertions for chaos engineering tests.
// This addon is completely decoupled from itestkit core and can be used independently.
package metricskit

import "time"

// MetricsProvider defines the interface for collecting chaos test metrics.
// Implementations must be thread-safe for concurrent access.
type MetricsProvider interface {
	// Lifecycle
	StartTest()
	EndTest()
	StartChaos()
	EndChaos()

	// Recording
	RecordRequest(success bool, timeout bool, latency time.Duration)
	RecordError(errMsg string)

	// Queries
	SuccessRate() float64
	AverageLatency() time.Duration
	Percentile(p float64) time.Duration
	ThroughputRPS() float64

	// Snapshot for assertions
	Snapshot() MetricsSnapshot
}

// MetricsSnapshot provides a read-only view of metrics at a point in time.
// Used by ChaosAssertions to validate SLOs without race conditions.
type MetricsSnapshot interface {
	// Core metrics
	SuccessRate() float64
	AverageLatency() time.Duration

	// Percentiles
	P50() time.Duration
	P90() time.Duration
	P95() time.Duration
	P99() time.Duration
	P999() time.Duration
	Percentile(p float64) time.Duration

	// Throughput
	ThroughputRPS() float64
	SuccessfulThroughputRPS() float64
	ChaosThroughputRPS() float64

	// Counts
	GetTotalRequests() int
	GetFailedRequests() int
	GetTimeoutRequests() int

	// Durations
	ChaosDuration() time.Duration
	TestDuration() time.Duration

	// Latency bounds
	GetMinLatency() time.Duration

	// Error analysis
	GetErrorCounts() map[ErrorCategory]int
}
