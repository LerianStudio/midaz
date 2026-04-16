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

	// preparedDepthBucket* boundaries bound cardinality of the
	// authorizer_prepared_pending_depth gauge's shard_range label. The raw
	// pending count can fluctuate between 0 and DefaultMaxPreparedTx
	// (10_000), so the gauge is reported under discrete ranges rather than
	// as a free-form int label.
	preparedDepthBucket10   = 10
	preparedDepthBucket100  = 100
	preparedDepthBucket1000 = 1000
)

// Compile-time interface compliance checks.
var (
	_ engine.Observer         = (*authorizerMetrics)(nil)
	_ engine.PreparedObserver = (*authorizerMetrics)(nil)
	_ wal.Observer            = (*authorizerMetrics)(nil)
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
	walHMACVerifyFailedTotal = libMetrics.Metric{
		Name:        "authorizer_wal_hmac_verify_failed_total",
		Unit:        "1",
		Description: "Total number of WAL frames that failed HMAC-SHA256 verification on replay (tamper or key-rotation signal).",
	}
	walTruncationTotal = libMetrics.Metric{
		Name:        "authorizer_wal_truncation_total",
		Unit:        "1",
		Description: "Total number of WAL self-healing truncations (corruption, partial writes, HMAC mismatches).",
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
	unauthorizedRPCTotal = libMetrics.Metric{
		Name:        "authorizer_unauthorized_rpc_total",
		Unit:        "1",
		Description: "Total internal RPCs rejected due to peer-auth failures (missing token, invalid HMAC, replay, skew, etc.).",
	}
	// loadedBalancesAbsolute is the total count of balances upserted into the
	// in-memory engine at boot time. Emitted once the readiness gate flips to
	// SERVING so operators can correlate boot progress against expected scale.
	// Intentionally a gauge (not counter) so cold restarts reset to zero —
	// dashboards can detect restarts by watching for the drop.
	loadedBalancesAbsolute = libMetrics.Metric{
		Name:        "authorizer_loaded_balances_absolute",
		Unit:        "1",
		Description: "Number of balances currently loaded into the authorizer engine (set at bootstrap post-load).",
	}
	// authorizerPreparedExpiredTotal counts prepared transactions removed
	// from the in-memory prepStore without a committed outcome. Stable
	// reason labels are "timeout" (reaper auto-abort after
	// DefaultPrepareTimeout) and "force_abort" (commit retry limit
	// exceeded). A sustained non-zero rate is an SLI for stuck transactions
	// and coordinator crashes — alert on it.
	authorizerPreparedExpiredTotal = libMetrics.Metric{
		Name:        "authorizer_prepared_expired_total",
		Unit:        "1",
		Description: "Total prepared transactions auto-aborted by the reaper without commit/abort (reason=timeout|force_abort).",
	}
	// authorizerPreparedPendingDepth reports the current count of
	// prepared-but-not-committed 2PC transactions. Gauge (not counter)
	// because the value rises and falls with load. The shard_range label
	// bounds cardinality to 5 discrete buckets derived from the pending
	// count so dashboards can distinguish "idle" from "approaching
	// capacity" without exploding label permutations.
	authorizerPreparedPendingDepth = libMetrics.Metric{
		Name:        "authorizer_prepared_pending_depth",
		Unit:        "1",
		Description: "Current pending prepared-transaction depth (post-mutation snapshot, bucketed via shard_range label).",
	}
	// loadedBalancesRatio is the ratio of observed loaded balances to the
	// expected count captured at the initial LoadBalances call. The expected
	// value is the same count used as the readiness-gate threshold: the
	// gauge reads 1.0 immediately after the flip, then drifts if background
	// LoadBalances RPCs add more shards. Useful for alerting on bootstrap
	// regressions ("gauge < 0.99 for 5m" indicates partial load).
	loadedBalancesRatio = libMetrics.Metric{
		Name:        "authorizer_loaded_balances_ratio",
		Unit:        "1",
		Description: "Ratio of loaded balances to expected at readiness flip; drifts if later LoadBalances RPCs add balances.",
	}
	// manualInterventionRequiredTotal counts every transition into
	// MANUAL_INTERVENTION_REQUIRED, bucketed by reason. A sustained non-zero
	// rate is the SLI operators alert on: at 100K TPS a 0.001% stuck-tx rate
	// is 86,400 stuck transactions/day that require human action. Stable
	// reason labels (see manualInterventionReason* constants in commit_intent.go):
	// local_not_found, remote_not_found, invalid_transition, participant_missing_txid.
	manualInterventionRequiredTotal = libMetrics.Metric{
		Name:        "authorizer_manual_intervention_required_total",
		Unit:        "1",
		Description: "Total commit intents escalated to MANUAL_INTERVENTION_REQUIRED (reason=local_not_found|remote_not_found|invalid_transition|participant_missing_txid).",
	}
	// commitRecordsDLQTotal counts records routed to the commits DLQ topic
	// after exponential-backoff retries were exhausted or a permanent
	// classification was detected. Operators MUST alert on any non-zero
	// rate — DLQ records indicate poison payloads that the recovery loop
	// could not process.
	commitRecordsDLQTotal = libMetrics.Metric{
		Name:        "authorizer_commit_records_dlq_total",
		Unit:        "1",
		Description: "Total records routed to the commits DLQ topic (reason=permanent|retries_exhausted).",
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
		// slo_target_ms is a deployment-scoped constant (set at init), not a
		// per-request value: cardinality = 1 per deployment.
		//
		// rejection_code and cross_shard are the two most diagnostic
		// dimensions when a breach fires — "is this only happening on
		// cross-shard 2PC?" and "are these breaches concentrated on a
		// single rejection path?" are the first two questions an oncall
		// will ask. Both labels are enum-bounded (rejection_code goes
		// through normalizeRejectionCode; cross_shard is a bool), so
		// cardinality remains finite (~8 rejection codes × 2 × methods ×
		// results ≈ a few dozen combinations).
		sloLabels := map[string]string{
			"method":         method,
			"result":         result,
			"rejection_code": normalizeRejectionCode(rejectionCode),
			"cross_shard":    boolLabel(crossShard),
			"pending":        boolLabel(pending),
			"slo_target_ms":  strconv.FormatInt(durationMillis(m.authorizeLatencySLO), 10),
		}
		m.factory.Counter(authorizeLatencySLOBreachesTotal).WithLabels(sloLabels).AddOne(ctx)
	}
}

// RecordManualInterventionRequired increments the
// authorizer_manual_intervention_required_total counter with a bounded
// reason label. Callers MUST pass one of the stable manualInterventionReason*
// values; any unknown value collapses to "other" to keep cardinality bounded.
//
// This is the single entry point for emitting the stuck-tx SLI — call it at
// every site where a commit intent transitions to
// MANUAL_INTERVENTION_REQUIRED (both local and remote NotFound paths, and
// any future escalation point).
func (m *authorizerMetrics) RecordManualInterventionRequired(ctx context.Context, reason string) {
	if m == nil || m.factory == nil {
		return
	}

	m.factory.Counter(manualInterventionRequiredTotal).
		WithLabels(map[string]string{"reason": normalizeManualInterventionReason(reason)}).
		AddOne(ctx)
}

// RecordCommitRecordsDLQ increments authorizer_commit_records_dlq_total
// with a bounded reason label. Called when a record is routed to the
// commits DLQ topic (either permanent classification or retries exhausted).
func (m *authorizerMetrics) RecordCommitRecordsDLQ(ctx context.Context, reason string) {
	if m == nil || m.factory == nil {
		return
	}

	m.factory.Counter(commitRecordsDLQTotal).
		WithLabels(map[string]string{"reason": normalizeDLQReason(reason)}).
		AddOne(ctx)
}

// RecordUnauthorizedRPC increments authorizer_unauthorized_rpc_total with
// stable labels {method, reason}. method is the gRPC full method name of the
// RPC the caller attempted to invoke; reason is one of the stable values
// defined in grpc.go (missing_token, missing_headers, bad_timestamp,
// timestamp_skew, wrong_algo, body_mismatch, invalid_hmac, nonce_replay,
// nonce_internal, hash_internal). Operators MUST alert on any sustained
// non-zero rate — this counter represents a security event (peer-auth denied).
func (m *authorizerMetrics) RecordUnauthorizedRPC(ctx context.Context, method, reason string) {
	if m == nil || m.factory == nil {
		return
	}

	labels := map[string]string{
		"method": normalizeUnauthorizedMethod(method),
		"reason": normalizeUnauthorizedReason(reason),
	}

	m.factory.Counter(unauthorizedRPCTotal).WithLabels(labels).AddOne(ctx)
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

// ObservePreparedExpired increments authorizer_prepared_expired_total with a
// bounded reason label. Called by the prepared-transaction reaper whenever a
// prepared tx is removed without a committed outcome. A sustained non-zero
// rate is an SLI for stuck 2PC transactions — alert on any persistent
// increase.
func (m *authorizerMetrics) ObservePreparedExpired(reason string) {
	if m == nil {
		return
	}

	normalizedReason := normalizePreparedExpirationReason(reason)

	if m.factory != nil {
		m.factory.Counter(authorizerPreparedExpiredTotal).
			WithLabels(map[string]string{"reason": normalizedReason}).
			AddOne(context.Background())
	}

	if m.logger != nil {
		m.logger.Warnf(
			"Authorizer prepared transaction auto-aborted without commit/abort: reason=%s",
			normalizedReason,
		)
	}
}

// ObservePreparedPendingDepth sets authorizer_prepared_pending_depth to the
// current pending count. The raw depth is the gauge value; the
// bucketPreparedDepth-derived "shard_range" label stays bounded to five
// discrete ranges so operators get both the exact depth and a cardinality-
// safe label slice for alerting (e.g. alert when shard_range=hundreds for >
// 1m).
func (m *authorizerMetrics) ObservePreparedPendingDepth(depth int) {
	if m == nil || m.factory == nil {
		return
	}

	m.factory.Gauge(authorizerPreparedPendingDepth).
		WithLabels(map[string]string{"shard_range": bucketPreparedDepth(depth)}).
		Set(context.Background(), int64(depth))
}

// bucketPreparedDepth classifies a raw pending count into a bounded enum so
// it can be used as a metric label without unbounded cardinality. The
// boundaries align with DefaultMaxPreparedTx=10_000: "zero", "1_9",
// "10_99", "100_999", "1000_plus" give dashboards four orders of magnitude
// of resolution.
func bucketPreparedDepth(depth int) string {
	switch {
	case depth <= 0:
		return "zero"
	case depth < preparedDepthBucket10:
		return "1_9"
	case depth < preparedDepthBucket100:
		return "10_99"
	case depth < preparedDepthBucket1000:
		return "100_999"
	default:
		return "1000_plus"
	}
}

// EmitAuthorizationAuditEvent writes a structured audit log entry for every
// authorize decision. Unlike RecordAuthorize (which emits bounded-cardinality
// metrics), this path is intentionally log-based: organization_id and
// ledger_id are high-cardinality and cannot be safely attached as metric
// labels, but financial audit requires full tenant context for every
// decision. Operators can grep the audit stream; metrics remain
// cardinality-safe.
//
// Called by the authorize RPC handler once per decision. Safe to call with a
// nil receiver (no-op). All user-controlled identifiers pass through
// normalizeLogToken before formatting.
func (m *authorizerMetrics) EmitAuthorizationAuditEvent(
	ctx context.Context,
	organizationID string,
	ledgerID string,
	transactionID string,
	actor string,
	result string,
	rejectionCode string,
	amountBucket string,
	crossShard bool,
) {
	_ = ctx // ctx is accepted so call sites can pass the request-scoped
	// context for future correlation (e.g. adding request-id); the current
	// audit-channel implementation does not consume it.

	if m == nil || m.logger == nil {
		return
	}

	m.logger.Warnf(
		"AUTHORIZER_AUDIT event=authorize tenant=%s ledger=%s tx_id=%s actor=%s result=%s rejection_code=%s amount_bucket=%s cross_shard=%s",
		normalizeLogToken(organizationID),
		normalizeLogToken(ledgerID),
		normalizeLogToken(transactionID),
		normalizeLogToken(actor),
		normalizeLogToken(result),
		normalizeRejectionCode(rejectionCode),
		normalizeLogToken(amountBucket),
		boolLabel(crossShard),
	)
}

// RecordSecurityEvent routes a security-significant event through the logger
// at WARN or ERROR level, depending on severity. This is the single entry
// point for auth failures, policy rejections, rate-limit triggers,
// peer-token failures, and WAL HMAC failures — separating them from routine
// operational INFO noise so operators can build a dedicated security SIEM
// sink by filtering on the "AUTHORIZER_SECURITY" prefix.
//
// category is a stable, caller-provided label ("auth_failure",
// "policy_rejection", "rate_limit", "peer_token", "wal_hmac"). detail is a
// free-form human-readable message — all user-controlled tokens MUST be
// pre-sanitized by the caller via normalizeLogToken.
func (m *authorizerMetrics) RecordSecurityEvent(severity, category, detail string) {
	if m == nil || m.logger == nil {
		return
	}

	line := "AUTHORIZER_SECURITY category=" + normalizeStage(category) + " detail=" + normalizeLogToken(detail)

	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "error":
		m.logger.Errorf("%s", line)
	default:
		m.logger.Warnf("%s", line)
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

// ObserveLoadedBalancesAbsolute records the total balance count loaded into
// the engine at readiness-flip time. This is the single source of truth
// operators should correlate against expected counts when diagnosing cold
// start regressions. Called once per process from bootstrap.Run() at the
// moment health flips to SERVING.
//
// ctx is accepted so the metric emission participates in the trace that
// Run() is currently executing under — otherwise the Gauge would look like
// it originated from a detached background worker.
func (m *authorizerMetrics) ObserveLoadedBalancesAbsolute(ctx context.Context, loaded int64) {
	if m == nil || m.factory == nil {
		return
	}

	m.factory.Gauge(loadedBalancesAbsolute).Set(ctx, loaded)

	// Ratio against itself at boot is trivially 1.0 — recording it here
	// establishes the baseline so any later LoadBalances RPC that grows the
	// engine's balance count pushes the ratio above 1.0. An alert of the
	// form "ratio deviates from 1.0 for > N min" catches partial loads
	// (count < expected) and late shard expansions (count > expected).
	m.factory.Gauge(loadedBalancesRatio).Set(ctx, 1)
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

// ObserveWALHMACVerifyFailed emits a high-severity security metric and audit
// log line whenever a WAL frame fails HMAC verification on replay. Operators
// MUST alert on any non-zero rate; sustained failures indicate either disk
// corruption, key rotation misconfiguration, or deliberate tampering.
func (m *authorizerMetrics) ObserveWALHMACVerifyFailed(offset int64, reason string) {
	if m == nil {
		return
	}

	normalizedReason := normalizeStage(reason)

	if m.factory != nil {
		m.factory.Counter(walHMACVerifyFailedTotal).
			WithLabels(map[string]string{"reason": normalizedReason}).
			AddOne(context.Background())
	}

	if m.logger != nil {
		m.logger.Errorf(
			"Authorizer WAL HMAC verify failed (SECURITY): offset=%d reason=%s — investigate immediately",
			offset, normalizedReason,
		)
	}
}

// ObserveWALTruncation records every self-healing WAL truncation. Expected
// during crash recovery; unexpected when co-occurring with HMAC failures.
func (m *authorizerMetrics) ObserveWALTruncation(offset int64, reason string) {
	if m == nil {
		return
	}

	normalizedReason := normalizeStage(reason)

	if m.factory != nil {
		m.factory.Counter(walTruncationTotal).
			WithLabels(map[string]string{"reason": normalizedReason}).
			AddOne(context.Background())
	}

	if m.logger != nil {
		m.logger.Warnf(
			"Authorizer WAL self-healing truncation: offset=%d reason=%s",
			offset, normalizedReason,
		)
	}
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

// normalizePreparedExpirationReason clamps the reason label on
// authorizer_prepared_expired_total to the two stable values emitted by the
// engine's prepared-tx reaper; any other input collapses to "other" to keep
// cardinality bounded.
func normalizePreparedExpirationReason(value string) string {
	reason := strings.TrimSpace(strings.ToLower(value))
	switch reason {
	case "":
		return labelUnknown
	case engine.PreparedExpirationTimeout, engine.PreparedExpirationForceAbort:
		return reason
	default:
		return labelOther
	}
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

// normalizeUnauthorizedMethod clamps the method label to the known authorizer
// RPC surface so an attacker cannot explode metric cardinality by probing
// arbitrary method names before peer-auth rejects them.
func normalizeUnauthorizedMethod(value string) string {
	method := strings.TrimSpace(value)
	switch method {
	case "":
		return labelUnknown
	case peerRPCMethodAuthorize,
		peerRPCMethodAuthorizeStream,
		peerRPCMethodPrepareAuthorize,
		peerRPCMethodCommitPrepared,
		peerRPCMethodAbortPrepared,
		peerRPCMethodLoadBalances,
		peerRPCMethodGetBalance,
		peerRPCMethodPublishBalanceOp,
		adminRPCMethodResolveManualIntervention:
		return method
	default:
		return labelOther
	}
}

// normalizeUnauthorizedReason validates that the caller passed one of the
// stable reason values — any unknown value collapses to "other" to keep the
// counter cardinality bounded.
func normalizeUnauthorizedReason(value string) string {
	reason := strings.TrimSpace(strings.ToLower(value))
	switch reason {
	case "":
		return labelUnknown
	case "missing_token",
		"missing_headers",
		"bad_timestamp",
		"timestamp_skew",
		"wrong_algo",
		"body_mismatch",
		"invalid_hmac",
		"nonce_replay",
		"nonce_internal",
		"hash_internal",
		"missing_admin_token",
		"invalid_admin_token":
		return reason
	default:
		return labelOther
	}
}

func normalizeTopic(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return labelUnknown
	case "ledger.balance.operations",
		"ledger.balance.create",
		"authorizer.cross-shard.commits",
		"authorizer.cross-shard.manual-intervention",
		"authorizer.cross-shard.commits.dlq":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return labelOther
	}
}

// normalizeManualInterventionReason clamps the reason label on
// authorizer_manual_intervention_required_total to the stable values emitted
// by the escalation sites. Unknown values collapse to "other" to bound
// cardinality.
func normalizeManualInterventionReason(value string) string {
	reason := strings.TrimSpace(strings.ToLower(value))
	switch reason {
	case "":
		return labelUnknown
	case "local_not_found", "remote_not_found", "invalid_transition", "participant_missing_txid":
		return reason
	default:
		return labelOther
	}
}

// normalizeDLQReason clamps the reason label on
// authorizer_commit_records_dlq_total to the stable values emitted by the
// recovery loop. Unknown values collapse to "other".
func normalizeDLQReason(value string) string {
	reason := strings.TrimSpace(strings.ToLower(value))
	switch reason {
	case "":
		return labelUnknown
	case "permanent", "retries_exhausted":
		return reason
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
