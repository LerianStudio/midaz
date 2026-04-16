// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"strconv"
	"strings"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libMetrics "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
)

// Constants for metric bucket boundaries and normalization limits.
const (
	defaultAuthorizeLatencySLOMs = 150
	bucketBoundary2              = 2
	bucketBoundary4              = 4
	bucketBoundary10             = 10
	maxStageLength               = 64
	maxLogTokenLength            = 128
	labelOther                   = "other"
	labelUnknown                 = "unknown"
)

// Compile-time interface compliance checks.
var (
	_ engine.Observer = (*authorizerMetrics)(nil)
	_ wal.Observer    = (*authorizerMetrics)(nil)
)

// Explicit histogram bucket boundaries (ms) aligned with SLO targets.
// The OTEL SDK defaults ({0, 5, 10, 25, 50, 75, 100, 250, 500, ...} ms)
// collapse sub-millisecond latencies and do not cross the 150 ms
// authorize-latency SLO boundary — forcing dashboards to interpolate.
// These boundaries are applied at instrument creation time via
// libMetrics.Metric.Buckets, which lib-commons passes to the OTEL SDK
// as metric.WithExplicitBucketBoundaries when the instrument is created.
// All histograms record int64 milliseconds via durationMillis(); the
// boundary values are expressed as float64 because the OTEL metric API
// requires float64 bounds regardless of the instrument value type.
var (
	// authorizeLatencyBucketsMs covers sub-ms through 1s with an explicit
	// edge at 150 ms matching defaultAuthorizeLatencySLOMs.
	authorizeLatencyBucketsMs = []float64{0.5, 1, 2, 5, 10, 25, 50, 100, 150, 250, 500, 1000}

	// engineLockBucketsMs reuses the authorize-latency shape so lock wait
	// and hold P99s can be compared against the authorize SLO directly.
	engineLockBucketsMs = []float64{0.5, 1, 2, 5, 10, 25, 50, 100, 150, 250, 500, 1000}

	// redpandaPublishBucketsMs aligns with downstream publish SLO targets
	// (same shape as authorize latency; 150 ms edge shared with SLO).
	redpandaPublishBucketsMs = []float64{0.5, 1, 2, 5, 10, 25, 50, 100, 150, 250, 500, 1000}

	// walFsyncBucketsMs is tighter and tops out at 25 ms because WAL
	// fsync beyond that is a hard failure indicator, not a percentile.
	walFsyncBucketsMs = []float64{0.1, 0.5, 1, 2, 5, 10, 25}
)

var (
	authorizeRequestsTotal = libMetrics.Metric{
		Name:        "authorizer_authorize_requests_total",
		Unit:        "1",
		Description: "Total number of authorizer authorization requests.",
	}
	authorizeLatencyMs = libMetrics.Metric{
		Name:        "authorizer_authorize_latency_ms",
		Unit:        "ms",
		Description: "Authorization request latency in milliseconds.",
		Buckets:     authorizeLatencyBucketsMs,
	}
	authorizeOperationsPerRequest = libMetrics.Metric{
		Name:        "authorizer_operations_per_request",
		Unit:        "1",
		Description: "Number of operations per authorization request.",
	}
	authorizeShardsTouchedPerRequest = libMetrics.Metric{
		Name:        "authorizer_shards_touched_per_request",
		Unit:        "1",
		Description: "Number of shards touched per authorization request.",
	}
	engineLockWaitMs = libMetrics.Metric{
		Name:        "authorizer_engine_lock_wait_ms",
		Unit:        "ms",
		Description: "Total lock wait time while acquiring balance locks.",
		Buckets:     engineLockBucketsMs,
	}
	engineLockHoldMs = libMetrics.Metric{
		Name:        "authorizer_engine_lock_hold_ms",
		Unit:        "ms",
		Description: "Lock hold duration for balance-locked authorization critical section.",
		Buckets:     engineLockBucketsMs,
	}
	walQueueDepth = libMetrics.Metric{
		Name:        "authorizer_wal_queue_depth",
		Unit:        "1",
		Description: "Current pending entries in the authorizer WAL queue.",
	}
	walAppendDroppedTotal = libMetrics.Metric{
		Name:        "authorizer_wal_append_drop_total",
		Unit:        "1",
		Description: "Total number of dropped WAL appends.",
	}
	walWriteErrorsTotal = libMetrics.Metric{
		Name:        "authorizer_wal_write_errors_total",
		Unit:        "1",
		Description: "Total WAL write/flush/sync errors.",
	}
	walReplaySkippedTotal = libMetrics.Metric{
		Name:        "authorizer_wal_replay_skipped_total",
		Unit:        "1",
		Description: "Total WAL replay entries skipped due to safety checks.",
	}
	walFsyncLatencyMs = libMetrics.Metric{
		Name:        "authorizer_wal_fsync_latency_ms",
		Unit:        "ms",
		Description: "WAL fsync latency in milliseconds.",
		Buckets:     walFsyncBucketsMs,
	}
	redpandaPublishLatencyMs = libMetrics.Metric{
		Name:        "authorizer_redpanda_publish_latency_ms",
		Unit:        "ms",
		Description: "Redpanda publish latency in milliseconds.",
		Buckets:     redpandaPublishBucketsMs,
	}
	redpandaPublishErrorsTotal = libMetrics.Metric{
		Name:        "authorizer_redpanda_publish_errors_total",
		Unit:        "1",
		Description: "Total failed publish attempts to Redpanda.",
	}
	authorizeLatencySLOBreachesTotal = libMetrics.Metric{
		Name:        "authorizer_authorize_latency_slo_breaches_total",
		Unit:        "1",
		Description: "Total authorization requests above configured latency SLO (telemetry-only, no behavior trigger).",
	}
)

type authorizerMetrics struct {
	factory             *libMetrics.MetricsFactory
	logger              libLog.Logger
	authorizeLatencySLO time.Duration
}

// Enabled reports whether metrics recording is active. This is a
// convenience for callers that want to skip expensive metric label
// computation. Note that individual Record/Observe methods already
// guard against nil receiver, so calling Enabled() is optional.
func (m *authorizerMetrics) Enabled() bool {
	return m != nil && m.factory != nil
}

func newAuthorizerMetrics(telemetry *libOpentelemetry.Telemetry, logger libLog.Logger, authorizeLatencySLO time.Duration) *authorizerMetrics {
	if authorizeLatencySLO <= 0 {
		authorizeLatencySLO = defaultAuthorizeLatencySLOMs * time.Millisecond
	}

	if telemetry == nil {
		return &authorizerMetrics{logger: logger, authorizeLatencySLO: authorizeLatencySLO}
	}

	return &authorizerMetrics{factory: telemetry.MetricsFactory, logger: logger, authorizeLatencySLO: authorizeLatencySLO}
}

// RecordAuthorize records metrics for an authorization request including latency, operation count, and SLO breaches.
func (m *authorizerMetrics) RecordAuthorize(
	ctx context.Context,
	method string,
	result string,
	rejectionCode string,
	pending bool,
	transactionStatus string,
	operationCount int,
	shardCount int,
	latency time.Duration,
	crossShard bool,
) {
	if m == nil || m.factory == nil {
		return
	}

	labels := map[string]string{
		"method":           method,
		"result":           result,
		"rejection_code":   normalizeRejectionCode(rejectionCode),
		"pending":          boolLabel(pending),
		"tx_status":        normalizeStatusLabel(transactionStatus),
		"ops_count_bucket": bucketOperationCount(operationCount),
		"shard_bucket":     bucketShardCount(shardCount),
		// cross_shard distinguishes 2PC cross-instance paths from single-shard fast paths.
		// Query example: sum(rate(authorizer_authorize_requests_total{cross_shard="true"}[5m]))
		// to verify that cross-shard traffic is actually being exercised during benchmarks.
		"cross_shard": boolLabel(crossShard),
	}

	m.factory.Counter(authorizeRequestsTotal).WithLabels(labels).AddOne(ctx)
	m.factory.Histogram(authorizeLatencyMs).WithLabels(labels).Record(ctx, durationMillis(latency))
	m.factory.Histogram(authorizeOperationsPerRequest).WithLabels(map[string]string{"method": method}).Record(ctx, int64(operationCount))
	m.factory.Histogram(authorizeShardsTouchedPerRequest).WithLabels(map[string]string{"method": method}).Record(ctx, int64(shardCount))

	if m.authorizeLatencySLO > 0 && latency > m.authorizeLatencySLO {
		// NOTE: slo_target_ms is a deployment-scoped constant (set at init),
		// not a per-request value. Cardinality = 1 per deployment, not per request.
		sloLabels := map[string]string{
			"method":        method,
			"result":        result,
			"pending":       boolLabel(pending),
			"slo_target_ms": strconv.FormatInt(durationMillis(m.authorizeLatencySLO), 10),
		}
		m.factory.Counter(authorizeLatencySLOBreachesTotal).WithLabels(sloLabels).AddOne(ctx)
	}
}

// RecordPublish records metrics for a Redpanda publish attempt including latency and error counts.
func (m *authorizerMetrics) RecordPublish(ctx context.Context, topic string, err error, latency time.Duration) {
	if m == nil || m.factory == nil {
		return
	}

	labels := map[string]string{"topic": normalizeTopic(topic)}
	m.factory.Histogram(redpandaPublishLatencyMs).WithLabels(labels).Record(ctx, durationMillis(latency))

	if err != nil {
		m.factory.Counter(redpandaPublishErrorsTotal).WithLabels(labels).AddOne(ctx)
	}
}

// ObserveAuthorizeLockWait records the time spent waiting to acquire balance locks.
func (m *authorizerMetrics) ObserveAuthorizeLockWait(lockCount, shardCount int, wait time.Duration) {
	if m == nil || m.factory == nil {
		return
	}

	lockBucket := bucketLockCount(lockCount)
	shardBucket := bucketShardCount(shardCount)

	m.factory.Histogram(engineLockWaitMs).WithLabels(map[string]string{
		"lock_count_bucket":  lockBucket,
		"shard_count_bucket": shardBucket,
	}).
		Record(context.Background(), durationMillis(wait))
}

// ObserveAuthorizeLockHold records the duration that balance locks are held during authorization.
func (m *authorizerMetrics) ObserveAuthorizeLockHold(lockCount, shardCount int, hold time.Duration) {
	if m == nil || m.factory == nil {
		return
	}

	lockBucket := bucketLockCount(lockCount)
	shardBucket := bucketShardCount(shardCount)

	m.factory.Histogram(engineLockHoldMs).WithLabels(map[string]string{
		"lock_count_bucket":  lockBucket,
		"shard_count_bucket": shardBucket,
	}).
		Record(context.Background(), durationMillis(hold))
}

func bucketLockCount(lockCount int) string {
	switch {
	case lockCount <= 0:
		return "0"
	case lockCount == 1:
		return "1"
	case lockCount <= bucketBoundary4:
		return "2_4"
	case lockCount <= bucketBoundary10:
		return "5_10"
	default:
		return "11_plus"
	}
}

// ObserveWALAppendFailure records a WAL append failure metric and logs the error.
func (m *authorizerMetrics) ObserveWALAppendFailure(err error) {
	if m == nil {
		return
	}

	if m.factory != nil {
		m.factory.Counter(walAppendDroppedTotal).WithLabels(map[string]string{"reason": "append_failed"}).AddOne(context.Background())
	}

	if m.logger != nil && err != nil {
		m.logger.Errorf("Authorizer WAL append failed (fail-closed): %v", err)
	}
}

// ObserveWALReplaySkipped records a skipped WAL replay entry with the given reason and logs a warning.
func (m *authorizerMetrics) ObserveWALReplaySkipped(reason, transactionID string, entryIndex int) {
	if m == nil {
		return
	}

	normalizedReason := normalizeReplaySkipReason(reason)

	if m.factory != nil {
		m.factory.Counter(walReplaySkippedTotal).
			WithLabels(map[string]string{"reason": normalizedReason}).
			AddOne(context.Background())
	}

	if m.logger != nil {
		m.logger.Warnf("Authorizer WAL replay skipped entry: reason=%s tx_id=%s entry_index=%d", normalizedReason, normalizeLogToken(transactionID), entryIndex)
	}
}

// ObserveWALQueueDepth records the current pending entry count in the WAL queue.
func (m *authorizerMetrics) ObserveWALQueueDepth(depth int) {
	if m == nil || m.factory == nil {
		return
	}

	m.factory.Gauge(walQueueDepth).Set(context.Background(), int64(depth))
}

// ObserveWALAppendDropped records a dropped WAL append due to a full buffer and logs a warning.
func (m *authorizerMetrics) ObserveWALAppendDropped(err error) {
	if m == nil {
		return
	}

	if m.factory != nil {
		m.factory.Counter(walAppendDroppedTotal).WithLabels(map[string]string{"reason": "buffer_full"}).AddOne(context.Background())
	}

	if m.logger != nil && err != nil {
		m.logger.Warnf("Authorizer WAL append dropped: %v", err)
	}
}

// ObserveWALWriteError records a WAL write/flush/sync error and logs the error with stage context.
func (m *authorizerMetrics) ObserveWALWriteError(stage string, err error) {
	if m == nil {
		return
	}

	if m.factory != nil {
		m.factory.Counter(walWriteErrorsTotal).WithLabels(map[string]string{"stage": normalizeStage(stage)}).AddOne(context.Background())
	}

	if m.logger != nil && err != nil {
		m.logger.Errorf("Authorizer WAL %s error: %v", normalizeStage(stage), err)
	}
}

// ObserveWALFsyncLatency records the fsync latency for WAL writes.
func (m *authorizerMetrics) ObserveWALFsyncLatency(latency time.Duration) {
	if m == nil || m.factory == nil {
		return
	}

	m.factory.Histogram(walFsyncLatencyMs).Record(context.Background(), durationMillis(latency))
}

func bucketOperationCount(operationCount int) string {
	switch {
	case operationCount <= 0:
		return "0"
	case operationCount == 1:
		return "1"
	case operationCount <= bucketBoundary4:
		return "2_4"
	case operationCount <= bucketBoundary10:
		return "5_10"
	default:
		return "11_plus"
	}
}

func bucketShardCount(shardCount int) string {
	switch {
	case shardCount <= 0:
		return "0"
	case shardCount == 1:
		return "1"
	case shardCount == bucketBoundary2:
		return "2"
	case shardCount <= bucketBoundary4:
		return "3_4"
	default:
		return "5_plus"
	}
}

func normalizeStatusLabel(value string) string {
	status := strings.ToLower(strings.TrimSpace(value))

	switch status {
	case "", "created", "pending", "approved", "canceled", "approved_compensate":
		if status == "" {
			return "empty"
		}

		return status
	default:
		return labelOther
	}
}

func normalizeRejectionCode(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "":
		return "none"
	case "insufficient_funds", "amount_exceeds_hold", "balance_not_found",
		"account_ineligible", "request_too_large", "internal_error":
		return strings.ToLower(strings.TrimSpace(code))
	default:
		return labelOther
	}
}

func normalizeStage(value string) string {
	stage := strings.TrimSpace(strings.ToLower(value))
	if stage == "" {
		return labelUnknown
	}

	if len(stage) > maxStageLength {
		return stage[:maxStageLength]
	}

	return stage
}

func normalizeLogToken(value string) string {
	token := strings.TrimSpace(value)
	if token == "" {
		return labelUnknown
	}

	token = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || (r >= 128 && r <= 159) || r == 0x200B || r == 0x200C || r == 0x200D || r == 0xFEFF {
			return -1
		}

		return r
	}, token)

	if token == "" {
		return labelUnknown
	}

	if len(token) > maxLogTokenLength {
		return token[:maxLogTokenLength]
	}

	return token
}

func normalizeReplaySkipReason(value string) string {
	reason := strings.TrimSpace(strings.ToLower(value))
	if reason == "" {
		return labelUnknown
	}

	switch reason {
	case "missing_balance", "version_mismatch", "mutation_limit_exceeded", "lock_limit_exceeded":
		return reason
	default:
		return labelOther
	}
}

func normalizeTopic(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return labelUnknown
	case "ledger.balance.operations", "ledger.balance.create", "authorizer.cross-shard.commits":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return labelOther
	}
}

func boolLabel(value bool) string {
	if value {
		return "true"
	}

	return "false"
}

func durationMillis(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}

	return d.Milliseconds()
}
