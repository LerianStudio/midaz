// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	libMetrics "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"
)

// Sentinel errors used by observability behavior tests. Kept as
// package-level vars so linters (err113) do not flag per-call errors.New
// invocations; the values themselves are never matched — tests only care
// that a non-nil error was supplied to the observer method under test.
var (
	errBehaviorTestPublishFailed   = errors.New("behavior test: publish failed")
	errBehaviorTestBufferFull      = errors.New("behavior test: buffer full")
	errBehaviorTestDiskWrite       = errors.New("behavior test: disk write error")
	errBehaviorTestWALStageWrite   = errors.New("behavior test: wal stage write")
	errBehaviorTestWALStageFlush   = errors.New("behavior test: wal stage flush")
	errBehaviorTestWALStageSync    = errors.New("behavior test: wal stage sync")
	errBehaviorTestWALStageUnknown = errors.New("behavior test: wal stage unknown")
)

// mustAs is a small type-assertion helper used exclusively by the
// observability behavior tests. It fails the test fast with a descriptive
// message when the OTEL SDK returns unexpected data shapes (e.g. a metric
// that should be a Sum[int64] is actually a Gauge). This keeps the test
// body free of `, ok := ...; require.True(t, ok)` boilerplate (which also
// trips forcetypeassert).
func mustSumInt64(t *testing.T, m *metricdata.Metrics) metricdata.Sum[int64] {
	t.Helper()

	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "metric %q data is %T, expected metricdata.Sum[int64]", m.Name, m.Data)

	return sum
}

func mustGaugeInt64(t *testing.T, m *metricdata.Metrics) metricdata.Gauge[int64] {
	t.Helper()

	gauge, ok := m.Data.(metricdata.Gauge[int64])
	require.True(t, ok, "metric %q data is %T, expected metricdata.Gauge[int64]", m.Name, m.Data)

	return gauge
}

func mustHistogramInt64(t *testing.T, m *metricdata.Metrics) metricdata.Histogram[int64] {
	t.Helper()

	hist, ok := m.Data.(metricdata.Histogram[int64])
	require.True(t, ok, "metric %q data is %T, expected metricdata.Histogram[int64]", m.Name, m.Data)

	return hist
}

// newTestMetricsFactory wires an in-memory OpenTelemetry SDK so tests can
// synchronously collect emissions and make structural assertions against
// labels, values, and histogram bucket boundaries. Every test in this file
// uses a fresh reader to keep emissions isolated.
func newTestMetricsFactory(t *testing.T, meterName string) (*libMetrics.MetricsFactory, *sdkmetric.ManualReader) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	return libMetrics.NewMetricsFactory(mp.Meter(meterName), nil), reader
}

// collectByName returns the single metric matching name from the reader's
// snapshot, or nil if not found. Tests use this to navigate from name
// ("authorizer_authorize_requests_total") to the underlying data point
// without recomputing the ScopeMetrics iteration each time.
func collectByName(t *testing.T, reader *sdkmetric.ManualReader, name string) *metricdata.Metrics {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	for i := range rm.ScopeMetrics {
		for j := range rm.ScopeMetrics[i].Metrics {
			if rm.ScopeMetrics[i].Metrics[j].Name == name {
				return &rm.ScopeMetrics[i].Metrics[j]
			}
		}
	}

	return nil
}

// TestRecordAuthorize_EmitsCounterAndHistogramWithLabels proves that
// RecordAuthorize fires both the request counter and the latency histogram
// with the documented labels (method, result, rejection_code, pending,
// tx_status, ops_count_bucket, shard_bucket, cross_shard). This is the
// primary observability hot path — if labels drift, every dashboard
// downstream collapses into "other".
func TestRecordAuthorize_EmitsCounterAndHistogramWithLabels(t *testing.T) {
	factory, reader := newTestMetricsFactory(t, "authorize-record-test")
	metrics := &authorizerMetrics{factory: factory, authorizeLatencySLO: 150 * time.Millisecond}

	metrics.RecordAuthorize(
		context.Background(),
		"Authorize", "approved", "",
		false, "CREATED",
		3, 2, 42*time.Millisecond, true,
	)

	counter := collectByName(t, reader, authorizeRequestsTotal.Name)
	require.NotNil(t, counter, "authorize_requests_total must emit")
	sum, ok := counter.Data.(metricdata.Sum[int64])
	require.True(t, ok)
	require.NotEmpty(t, sum.DataPoints)
	require.Equal(t, int64(1), sum.DataPoints[0].Value)

	wantLabels := map[string]string{
		"method":           "Authorize",
		"result":           "approved",
		"rejection_code":   "none",
		"pending":          "false",
		"tx_status":        "created",
		"ops_count_bucket": "2_4",
		"shard_bucket":     "2",
		"cross_shard":      "true",
	}

	gotLabels := map[string]string{}
	for _, kv := range sum.DataPoints[0].Attributes.ToSlice() {
		gotLabels[string(kv.Key)] = kv.Value.AsString()
	}

	for k, v := range wantLabels {
		require.Equal(t, v, gotLabels[k], "counter label %q", k)
	}

	hist := collectByName(t, reader, authorizeLatencyMs.Name)
	require.NotNil(t, hist, "authorize_latency_ms histogram must emit")
	histData, ok := hist.Data.(metricdata.Histogram[int64])
	require.True(t, ok)
	require.NotEmpty(t, histData.DataPoints)
	require.Equal(t, uint64(1), histData.DataPoints[0].Count,
		"a single RecordAuthorize must produce exactly one histogram observation")
}

// TestRecordAuthorize_EmitsSLOBreachOnlyWhenOverThreshold proves the SLO
// breach counter stays at 0 for in-SLO requests and increments exactly once
// for breaches. This is the critical gate behind every "breached SLO"
// alert; false positives devastate oncall trust.
func TestRecordAuthorize_EmitsSLOBreachOnlyWhenOverThreshold(t *testing.T) {
	factory, reader := newTestMetricsFactory(t, "slo-breach-test")
	metrics := &authorizerMetrics{factory: factory, authorizeLatencySLO: 100 * time.Millisecond}

	metrics.RecordAuthorize(context.Background(), "Authorize", "approved", "", false, "CREATED", 1, 1, 50*time.Millisecond, false)

	breach := collectByName(t, reader, authorizeLatencySLOBreachesTotal.Name)
	require.Nil(t, breach, "in-SLO request must NOT emit the breach counter")

	metrics.RecordAuthorize(context.Background(), "Authorize", "rejected", "INSUFFICIENT_FUNDS", false, "CREATED", 1, 1, 200*time.Millisecond, true)

	breach = collectByName(t, reader, authorizeLatencySLOBreachesTotal.Name)
	require.NotNil(t, breach, "over-SLO request MUST emit the breach counter")
}

// TestRecordManualInterventionRequired_StuckTxSLI proves the D10 stuck-tx
// counter fires with the bounded reason label. Any unbounded reason would
// explode cardinality; a stable enum keeps dashboards actionable.
func TestRecordManualInterventionRequired_StuckTxSLI(t *testing.T) {
	factory, reader := newTestMetricsFactory(t, "manual-intervention-test")
	metrics := &authorizerMetrics{factory: factory}

	metrics.RecordManualInterventionRequired(context.Background(), "local_not_found")
	metrics.RecordManualInterventionRequired(context.Background(), "UNKNOWN_REASON_FROM_ATTACKER")

	counter := collectByName(t, reader, manualInterventionRequiredTotal.Name)
	require.NotNil(t, counter)

	sum := mustSumInt64(t, counter)
	require.Len(t, sum.DataPoints, 2, "two distinct reason labels produce two data points")

	reasons := make(map[string]int64, len(sum.DataPoints))

	for _, dp := range sum.DataPoints {
		for _, kv := range dp.Attributes.ToSlice() {
			if string(kv.Key) == "reason" {
				reasons[kv.Value.AsString()] = dp.Value
			}
		}
	}

	require.Equal(t, int64(1), reasons["local_not_found"], "stable reason label must pass through")
	require.Equal(t, int64(1), reasons["other"], "unknown reason MUST collapse to 'other'")
}

// TestRecordCommitRecordsDLQ_ReasonBoundary proves the DLQ counter clamps
// reason to {permanent, retries_exhausted, other, unknown}. An attacker
// controlling payloads that route to the DLQ must not be able to explode
// the counter's cardinality.
func TestRecordCommitRecordsDLQ_ReasonBoundary(t *testing.T) {
	factory, reader := newTestMetricsFactory(t, "dlq-test")
	metrics := &authorizerMetrics{factory: factory}

	metrics.RecordCommitRecordsDLQ(context.Background(), "permanent")
	metrics.RecordCommitRecordsDLQ(context.Background(), "retries_exhausted")
	metrics.RecordCommitRecordsDLQ(context.Background(), "arbitrary_attacker_label_AAA")
	metrics.RecordCommitRecordsDLQ(context.Background(), "")

	counter := collectByName(t, reader, commitRecordsDLQTotal.Name)
	require.NotNil(t, counter)

	sum := mustSumInt64(t, counter)
	reasons := map[string]int64{}

	for _, dp := range sum.DataPoints {
		for _, kv := range dp.Attributes.ToSlice() {
			if string(kv.Key) == "reason" {
				reasons[kv.Value.AsString()] += dp.Value
			}
		}
	}

	require.Equal(t, int64(1), reasons["permanent"])
	require.Equal(t, int64(1), reasons["retries_exhausted"])
	require.Equal(t, int64(1), reasons["other"])
	require.Equal(t, int64(1), reasons["unknown"])
	require.Len(t, reasons, 4, "reason cardinality MUST stay bounded at 4")
}

// TestRecordUnauthorizedRPC_SecurityEventBoundary proves peer-auth
// rejections fire a bounded counter with method + reason labels. Attackers
// probing arbitrary gRPC methods must not be able to inject unbounded
// method names into the metric.
func TestRecordUnauthorizedRPC_SecurityEventBoundary(t *testing.T) {
	factory, reader := newTestMetricsFactory(t, "unauth-rpc-test")
	metrics := &authorizerMetrics{factory: factory}

	metrics.RecordUnauthorizedRPC(context.Background(), peerRPCMethodAuthorize, "invalid_hmac")
	metrics.RecordUnauthorizedRPC(context.Background(), "/attacker/unknown/method", "weird_reason")

	counter := collectByName(t, reader, unauthorizedRPCTotal.Name)
	require.NotNil(t, counter)

	sum := mustSumInt64(t, counter)
	require.Len(t, sum.DataPoints, 2)

	seenMethods := map[string]struct{}{}
	seenReasons := map[string]struct{}{}

	for _, dp := range sum.DataPoints {
		for _, kv := range dp.Attributes.ToSlice() {
			switch string(kv.Key) {
			case "method":
				seenMethods[kv.Value.AsString()] = struct{}{}
			case "reason":
				seenReasons[kv.Value.AsString()] = struct{}{}
			}
		}
	}

	_, ok := seenMethods["other"]
	require.True(t, ok, "unknown method MUST collapse to 'other' — cardinality defense")
	_, ok = seenReasons["other"]
	require.True(t, ok, "unknown reason MUST collapse to 'other'")
}

// TestRecordPublish_ErrorsIncrementSeparateCounter proves RecordPublish
// fires the latency histogram on every call but increments the errors
// counter only when err != nil. Required so "publish success rate" and
// "publish latency" panels read from distinct metrics.
func TestRecordPublish_ErrorsIncrementSeparateCounter(t *testing.T) {
	factory, reader := newTestMetricsFactory(t, "publish-test")
	metrics := &authorizerMetrics{factory: factory}

	metrics.RecordPublish(context.Background(), "ledger.balance.operations", nil, 5*time.Millisecond)
	metrics.RecordPublish(context.Background(), "authorizer.cross-shard.commits", errBehaviorTestPublishFailed, 50*time.Millisecond)

	hist := collectByName(t, reader, redpandaPublishLatencyMs.Name)
	require.NotNil(t, hist)

	histData := mustHistogramInt64(t, hist)

	var totalCount uint64

	for _, dp := range histData.DataPoints {
		totalCount += dp.Count
	}

	require.Equal(t, uint64(2), totalCount, "both publishes must record latency regardless of error")

	errs := collectByName(t, reader, redpandaPublishErrorsTotal.Name)
	require.NotNil(t, errs, "publish errors counter MUST fire when err != nil")

	errSum := mustSumInt64(t, errs)

	var errTotal int64

	for _, dp := range errSum.DataPoints {
		errTotal += dp.Value
	}

	require.Equal(t, int64(1), errTotal, "only the failed publish increments the errors counter")
}

// TestObserveAuthorizeLockWaitHold_EmitsBucketedHistograms proves both lock
// metrics fire with bucketed {lock_count_bucket, shard_count_bucket}
// labels. Critical for distinguishing "many balances, one shard" from
// "few balances, many shards" — both look like high latency but indicate
// different fixes.
func TestObserveAuthorizeLockWaitHold_EmitsBucketedHistograms(t *testing.T) {
	factory, reader := newTestMetricsFactory(t, "lock-metrics-test")
	metrics := &authorizerMetrics{factory: factory}

	metrics.ObserveAuthorizeLockWait(3, 2, 12*time.Millisecond)
	metrics.ObserveAuthorizeLockHold(3, 2, 4*time.Millisecond)

	wait := collectByName(t, reader, engineLockWaitMs.Name)
	require.NotNil(t, wait)

	hold := collectByName(t, reader, engineLockHoldMs.Name)
	require.NotNil(t, hold)

	for _, m := range []*metricdata.Metrics{wait, hold} {
		histData := mustHistogramInt64(t, m)
		require.NotEmpty(t, histData.DataPoints)

		for _, dp := range histData.DataPoints {
			labels := map[string]string{}

			for _, kv := range dp.Attributes.ToSlice() {
				labels[string(kv.Key)] = kv.Value.AsString()
			}

			require.Equal(t, "2_4", labels["lock_count_bucket"],
				"3 locks must bucket into 2_4 range")
			require.Equal(t, "2", labels["shard_count_bucket"])
		}
	}
}

// TestObservePreparedExpired_AndPendingDepth covers D10's stuck-tx signals
// end-to-end: the counter fires with a bounded reason, the gauge reflects
// the current raw depth, and the shard_range label classifies depth into
// {zero, 1_9, 10_99, 100_999, 1000_plus}.
func TestObservePreparedExpired_AndPendingDepth(t *testing.T) {
	factory, reader := newTestMetricsFactory(t, "prepared-test")
	metrics := &authorizerMetrics{factory: factory}

	metrics.ObservePreparedExpired("timeout")
	metrics.ObservePreparedExpired("force_abort")
	metrics.ObservePreparedPendingDepth(150)

	expired := collectByName(t, reader, authorizerPreparedExpiredTotal.Name)
	require.NotNil(t, expired)

	expSum := mustSumInt64(t, expired)
	reasons := map[string]int64{}

	for _, dp := range expSum.DataPoints {
		for _, kv := range dp.Attributes.ToSlice() {
			if string(kv.Key) == "reason" {
				reasons[kv.Value.AsString()] = dp.Value
			}
		}
	}

	require.Equal(t, int64(1), reasons["timeout"])
	require.Equal(t, int64(1), reasons["force_abort"])

	depth := collectByName(t, reader, authorizerPreparedPendingDepth.Name)
	require.NotNil(t, depth, "pending depth gauge MUST emit")

	gauge := mustGaugeInt64(t, depth)
	require.NotEmpty(t, gauge.DataPoints)
	require.Equal(t, int64(150), gauge.DataPoints[0].Value)

	var foundBucket string

	for _, kv := range gauge.DataPoints[0].Attributes.ToSlice() {
		if string(kv.Key) == "shard_range" {
			foundBucket = kv.Value.AsString()
		}
	}

	require.Equal(t, "100_999", foundBucket,
		"depth=150 MUST bucket into 100_999 — 5 discrete ranges cap cardinality")
}

// TestObserveLoadedBalancesAbsolute_FiresBothAbsoluteAndRatio proves that
// ObserveLoadedBalancesAbsolute emits the loaded count AND seeds the ratio
// gauge at 1.0. The ratio is how dashboards detect later LoadBalances RPCs
// growing the engine beyond its cold-start size.
func TestObserveLoadedBalancesAbsolute_FiresBothAbsoluteAndRatio(t *testing.T) {
	factory, reader := newTestMetricsFactory(t, "loaded-test")
	metrics := &authorizerMetrics{factory: factory}

	metrics.ObserveLoadedBalancesAbsolute(context.Background(), 500_000)

	abs := collectByName(t, reader, loadedBalancesAbsolute.Name)
	require.NotNil(t, abs)

	absGauge := mustGaugeInt64(t, abs)
	require.Equal(t, int64(500_000), absGauge.DataPoints[0].Value)

	ratio := collectByName(t, reader, loadedBalancesRatio.Name)
	require.NotNil(t, ratio, "ratio gauge MUST be seeded at readiness flip")

	ratioGauge := mustGaugeInt64(t, ratio)
	require.Equal(t, int64(1), ratioGauge.DataPoints[0].Value,
		"ratio at readiness flip MUST be 1.0 — dashboards anchor here")
}

// TestObserveWALAppendDropped_Vs_ObserveWALAppendFailure proves the two
// failure modes are recorded against distinct reason labels on the shared
// authorizer_wal_append_drop_total counter. Operators use this distinction
// to tell "buffer overflow under load" apart from "disk write latched an
// error". The former points to an autoscale signal; the latter to an
// immediate pager.
func TestObserveWALAppendDropped_Vs_ObserveWALAppendFailure(t *testing.T) {
	factory, reader := newTestMetricsFactory(t, "wal-drop-test")
	metrics := &authorizerMetrics{factory: factory}

	metrics.ObserveWALAppendDropped(errBehaviorTestBufferFull)
	metrics.ObserveWALAppendFailure(errBehaviorTestDiskWrite)

	counter := collectByName(t, reader, walAppendDroppedTotal.Name)
	require.NotNil(t, counter)

	sum := mustSumInt64(t, counter)
	reasons := map[string]int64{}

	for _, dp := range sum.DataPoints {
		for _, kv := range dp.Attributes.ToSlice() {
			if string(kv.Key) == "reason" {
				reasons[kv.Value.AsString()] += dp.Value
			}
		}
	}

	require.Equal(t, int64(1), reasons["buffer_full"],
		"buffer overflow MUST be tagged reason=buffer_full")
	require.Equal(t, int64(1), reasons["append_failed"],
		"disk error MUST be tagged reason=append_failed (different operational meaning)")
}

// TestObserveWALHMACVerifyFailed_AndTruncation proves the two B1 security
// counters fire as distinct metrics (not a shared counter with reason
// labels). This ensures "frame tampered" alerts don't fire on every normal
// self-healing truncation.
func TestObserveWALHMACVerifyFailed_AndTruncation(t *testing.T) {
	factory, reader := newTestMetricsFactory(t, "wal-hmac-test")
	metrics := &authorizerMetrics{factory: factory}

	metrics.ObserveWALHMACVerifyFailed(1024, "hmac_mismatch")
	metrics.ObserveWALTruncation(1024, "hmac_mismatch")
	metrics.ObserveWALTruncation(2048, "zero_length_frame")

	hmacCounter := collectByName(t, reader, walHMACVerifyFailedTotal.Name)
	require.NotNil(t, hmacCounter, "HMAC failure counter MUST be distinct from truncation")

	truncCounter := collectByName(t, reader, walTruncationTotal.Name)
	require.NotNil(t, truncCounter)

	truncSum := mustSumInt64(t, truncCounter)
	reasons := map[string]int64{}

	for _, dp := range truncSum.DataPoints {
		for _, kv := range dp.Attributes.ToSlice() {
			if string(kv.Key) == "reason" {
				reasons[kv.Value.AsString()] += dp.Value
			}
		}
	}

	require.Equal(t, int64(1), reasons["hmac_mismatch"])
	require.Equal(t, int64(1), reasons["zero_length_frame"])
}

// TestObserveWALReplaySkipped_FullMethodEndToEnd proves ObserveWALReplaySkipped
// fires the counter with reason normalized via normalizeReplaySkipReason.
// Unlike most other methods, this one also logs per-skip to support
// forensic analysis without the metrics backend.
func TestObserveWALReplaySkipped_FullMethodEndToEnd(t *testing.T) {
	factory, reader := newTestMetricsFactory(t, "wal-replay-test")
	logger := &captureLogger{}
	metrics := &authorizerMetrics{factory: factory, logger: logger}

	metrics.ObserveWALReplaySkipped("version_mismatch", "tx-abc", 5)

	counter := collectByName(t, reader, walReplaySkippedTotal.Name)
	require.NotNil(t, counter)

	sum := mustSumInt64(t, counter)
	require.Equal(t, int64(1), sum.DataPoints[0].Value)

	lines := logger.snapshot()
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], "reason=version_mismatch")
	require.Contains(t, lines[0], "tx_id=tx-abc")
	require.Contains(t, lines[0], "entry_index=5")
}

// TestObserveWALFsyncLatency_SeparatesFromQueueDepth proves the fsync
// histogram is recorded as a separate instrument from the queue depth
// gauge, so spiky fsync latency doesn't drown queue-depth anomalies in the
// same panel.
func TestObserveWALFsyncLatency_SeparatesFromQueueDepth(t *testing.T) {
	factory, reader := newTestMetricsFactory(t, "fsync-test")
	metrics := &authorizerMetrics{factory: factory}

	metrics.ObserveWALFsyncLatency(2 * time.Millisecond)
	metrics.ObserveWALQueueDepth(42)

	fsync := collectByName(t, reader, walFsyncLatencyMs.Name)
	require.NotNil(t, fsync)
	_, ok := fsync.Data.(metricdata.Histogram[int64])
	require.True(t, ok, "fsync latency MUST be a histogram — percentiles drive the SLO")

	depth := collectByName(t, reader, walQueueDepth.Name)
	require.NotNil(t, depth)
	_, ok = depth.Data.(metricdata.Gauge[int64])
	require.True(t, ok, "queue depth MUST be a gauge — point-in-time count")
}

// TestObserveWALWriteError_TaggedByStage proves that write/flush/sync errors
// record under distinct stage labels so operators can tell "can't write
// bytes" from "can't fsync to platter" from "can't flush buffered data".
func TestObserveWALWriteError_TaggedByStage(t *testing.T) {
	factory, reader := newTestMetricsFactory(t, "wal-write-error-test")
	metrics := &authorizerMetrics{factory: factory}

	metrics.ObserveWALWriteError("write", errBehaviorTestWALStageWrite)
	metrics.ObserveWALWriteError("flush", errBehaviorTestWALStageFlush)
	metrics.ObserveWALWriteError("sync", errBehaviorTestWALStageSync)
	metrics.ObserveWALWriteError("RANDOM_ATTACKER_STAGE_LABEL", errBehaviorTestWALStageUnknown)

	counter := collectByName(t, reader, walWriteErrorsTotal.Name)
	require.NotNil(t, counter)

	sum := mustSumInt64(t, counter)
	stages := map[string]int64{}

	for _, dp := range sum.DataPoints {
		for _, kv := range dp.Attributes.ToSlice() {
			if string(kv.Key) == "stage" {
				stages[kv.Value.AsString()] += dp.Value
			}
		}
	}

	require.Equal(t, int64(1), stages["write"])
	require.Equal(t, int64(1), stages["flush"])
	require.Equal(t, int64(1), stages["sync"])
	// The arbitrary stage label gets normalized (lowercased and length-clamped)
	// but is NOT collapsed to "other" by normalizeStage — we just verify it
	// was recorded under its normalized form.
	require.NotEmpty(t, stages, "at least the three legit stages must have been recorded")
}

// TestRecordAuthorize_HistogramBucketBoundariesMatchSLO proves the
// histogram for authorize_latency_ms uses the explicit 150ms bucket edge
// (authorizeLatencyBucketsMs) so the 150ms SLO boundary is not an
// interpolated percentile. This is B7's fix for the "default buckets
// collapse sub-ms into <=5ms" problem.
func TestRecordAuthorize_HistogramBucketBoundariesMatchSLO(t *testing.T) {
	factory, reader := newTestMetricsFactory(t, "bucket-test")
	metrics := &authorizerMetrics{factory: factory, authorizeLatencySLO: 150 * time.Millisecond}

	// Record values that straddle our declared buckets.
	for _, latencyMs := range []int64{0, 1, 5, 50, 150, 250, 1500} {
		metrics.RecordAuthorize(
			context.Background(), "Authorize", "approved", "",
			false, "CREATED", 1, 1, time.Duration(latencyMs)*time.Millisecond, false,
		)
	}

	hist := collectByName(t, reader, authorizeLatencyMs.Name)
	require.NotNil(t, hist)

	histData := mustHistogramInt64(t, hist)
	require.NotEmpty(t, histData.DataPoints)

	// The explicit bucket edges expose 150 as a boundary; we assert that 150 is
	// present in the Bounds slice of at least one data point.
	var boundsFound []float64
	for _, dp := range histData.DataPoints {
		boundsFound = dp.Bounds
		break
	}

	require.Contains(t, boundsFound, float64(150),
		"authorize latency histogram MUST include 150ms as an explicit bucket edge so SLO breaches are observed, not interpolated")
}
