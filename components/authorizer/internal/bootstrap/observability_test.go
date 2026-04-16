// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	libMetrics "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"
)

func TestNormalizeReplaySkipReason(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: "unknown"},
		{name: "missing balance", input: "missing_balance", want: "missing_balance"},
		{name: "version mismatch", input: "version_mismatch", want: "version_mismatch"},
		{name: "mutation limit", input: "mutation_limit_exceeded", want: "mutation_limit_exceeded"},
		{name: "lock limit", input: "lock_limit_exceeded", want: "lock_limit_exceeded"},
		{name: "trim and uppercase known", input: "  VERSION_MISMATCH  ", want: "version_mismatch"},
		{name: "trim and uppercase unknown", input: "  SOMETHING_NEW  ", want: "other"},
		{name: "unknown", input: "anything_else", want: "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeReplaySkipReason(tt.input))
		})
	}
}

func TestObserveWALReplaySkippedHandlesNilDepsAndNormalizesInputs(t *testing.T) {
	metrics := &authorizerMetrics{}

	require.NotPanics(t, func() {
		metrics.ObserveWALReplaySkipped("  LOCK_LIMIT_EXCEEDED  ", "tx-1", 3)
	})

	metricsWithNilFactory := &authorizerMetrics{logger: nil}

	require.NotPanics(t, func() {
		metricsWithNilFactory.ObserveWALReplaySkipped("  unknown_reason  ", "tx-2", 9)
	})
}

func TestBucketLockCount(t *testing.T) {
	tests := []struct {
		lockCount int
		want      string
	}{
		{lockCount: 0, want: "0"},
		{lockCount: 1, want: "1"},
		{lockCount: 2, want: "2_4"},
		{lockCount: 4, want: "2_4"},
		{lockCount: 5, want: "5_10"},
		{lockCount: 10, want: "5_10"},
		{lockCount: 11, want: "11_plus"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			require.Equal(t, tt.want, bucketLockCount(tt.lockCount))
		})
	}
}

func TestBucketOperationCount(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  string
	}{
		{name: "negative", input: -1, want: "0"},
		{name: "zero", input: 0, want: "0"},
		{name: "one", input: 1, want: "1"},
		{name: "two (lower boundary 2_4)", input: 2, want: "2_4"},
		{name: "three (mid 2_4)", input: 3, want: "2_4"},
		{name: "four (upper boundary 2_4)", input: 4, want: "2_4"},
		{name: "five (lower boundary 5_10)", input: 5, want: "5_10"},
		{name: "ten (upper boundary 5_10)", input: 10, want: "5_10"},
		{name: "eleven (lower boundary 11_plus)", input: 11, want: "11_plus"},
		{name: "fifty", input: 50, want: "11_plus"},
		{name: "hundred", input: 100, want: "11_plus"},
		{name: "five_hundred", input: 500, want: "11_plus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, bucketOperationCount(tt.input))
		})
	}
}

func TestBucketShardCount(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  string
	}{
		{name: "negative", input: -1, want: "0"},
		{name: "zero", input: 0, want: "0"},
		{name: "one", input: 1, want: "1"},
		{name: "two", input: 2, want: "2"},
		{name: "three (lower boundary 3_4)", input: 3, want: "3_4"},
		{name: "four (upper boundary 3_4)", input: 4, want: "3_4"},
		{name: "five (lower boundary 5_plus)", input: 5, want: "5_plus"},
		{name: "eight", input: 8, want: "5_plus"},
		{name: "sixteen", input: 16, want: "5_plus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, bucketShardCount(tt.input))
		})
	}
}

func TestNormalizeStatusLabel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: "empty"},
		{name: "created", input: "created", want: "created"},
		{name: "pending", input: "pending", want: "pending"},
		{name: "approved", input: "approved", want: "approved"},
		{name: "canceled", input: "canceled", want: "canceled"},
		{name: "approved_compensate", input: "approved_compensate", want: "approved_compensate"},
		{name: "uppercase known", input: "CREATED", want: "created"},
		{name: "unknown", input: "something_else", want: "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeStatusLabel(tt.input))
		})
	}
}

func TestNormalizeRejectionCode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: "none"},
		{name: "whitespace only", input: "   ", want: "none"},
		{name: "insufficient_funds lowercase", input: "insufficient_funds", want: "insufficient_funds"},
		{name: "insufficient_funds uppercase", input: "INSUFFICIENT_FUNDS", want: "insufficient_funds"},
		{name: "amount_exceeds_hold", input: "AMOUNT_EXCEEDS_HOLD", want: "amount_exceeds_hold"},
		{name: "balance_not_found", input: "BALANCE_NOT_FOUND", want: "balance_not_found"},
		{name: "account_ineligible", input: "ACCOUNT_INELIGIBLE", want: "account_ineligible"},
		{name: "request_too_large", input: "REQUEST_TOO_LARGE", want: "request_too_large"},
		{name: "request_too_large with whitespace", input: "  request_too_large  ", want: "request_too_large"},
		{name: "internal_error", input: "INTERNAL_ERROR", want: "internal_error"},
		{name: "unknown code", input: "SOMETHING_NEW", want: "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeRejectionCode(tt.input))
		})
	}
}

func TestNormalizeTopic(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: "unknown"},
		{name: "whitespace only", input: "   ", want: "unknown"},
		{name: "balance operations", input: "ledger.balance.operations", want: "ledger.balance.operations"},
		{name: "balance operations uppercase", input: "LEDGER.BALANCE.OPERATIONS", want: "ledger.balance.operations"},
		{name: "balance create", input: "ledger.balance.create", want: "ledger.balance.create"},
		{name: "cross-shard commits", input: "authorizer.cross-shard.commits", want: "authorizer.cross-shard.commits"},
		{name: "cross-shard commits with whitespace", input: "  authorizer.cross-shard.commits  ", want: "authorizer.cross-shard.commits"},
		{name: "unknown topic", input: "some.random.topic", want: "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeTopic(tt.input))
		})
	}
}

func TestBoolLabel(t *testing.T) {
	require.Equal(t, "true", boolLabel(true))
	require.Equal(t, "false", boolLabel(false))
}

func TestDurationMillis(t *testing.T) {
	tests := []struct {
		name  string
		input time.Duration
		want  int64
	}{
		{name: "zero", input: 0, want: 0},
		{name: "sub-millisecond (500us)", input: 500 * time.Microsecond, want: 0},
		{name: "1ms", input: 1 * time.Millisecond, want: 1},
		{name: "100ms", input: 100 * time.Millisecond, want: 100},
		{name: "1s", input: 1 * time.Second, want: 1000},
		{name: "2.5s", input: 2500 * time.Millisecond, want: 2500},
		{name: "negative", input: -100 * time.Millisecond, want: 0},
		{name: "negative 1ns", input: -1 * time.Nanosecond, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, durationMillis(tt.input))
		})
	}
}

func TestNormalizeStageCapsLengthAndFallback(t *testing.T) {
	require.Equal(t, "unknown", normalizeStage("   "))
	require.Equal(t, "wal_append", normalizeStage("  WAL_APPEND  "))
	require.Len(t, normalizeStage("abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"), 64)
}

// TestHistogramBucketBoundariesMatchSLOTargets verifies every latency
// histogram is registered with the explicit bucket boundaries declared
// in observability.go — guarding against a regression where the OTEL
// SDK default buckets ({0, 5, 10, 25, 50, 75, 100, 250, 500, ...} ms)
// collapse sub-ms latencies and miss the authorize SLO edge (150 ms),
// forcing dashboards to interpolate instead of observe (see FINAL_REVIEW.md#B7).
//
// The test builds a MeterProvider wired to an in-memory ManualReader,
// constructs a MetricsFactory against it, records a single sample per
// histogram, collects the ResourceMetrics snapshot, and asserts the
// Bounds of each HistogramDataPoint match the configured buckets.
func TestHistogramBucketBoundariesMatchSLOTargets(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	factory := libMetrics.NewMetricsFactory(mp.Meter("authorizer-test"), nil)

	// Record one sample per histogram so the SDK emits a data point.
	factory.Histogram(authorizeLatencyMs).Record(context.Background(), 100)
	factory.Histogram(engineLockWaitMs).Record(context.Background(), 1)
	factory.Histogram(engineLockHoldMs).Record(context.Background(), 1)
	factory.Histogram(redpandaPublishLatencyMs).Record(context.Background(), 5)
	factory.Histogram(walFsyncLatencyMs).Record(context.Background(), 1)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	require.NotEmpty(t, rm.ScopeMetrics, "expected at least one scope metric")

	// Flatten metrics by name for lookup.
	byName := make(map[string]metricdata.Metrics)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			byName[m.Name] = m
		}
	}

	cases := []struct {
		name string
		want []float64
	}{
		{name: authorizeLatencyMs.Name, want: authorizeLatencyBucketsMs},
		{name: engineLockWaitMs.Name, want: engineLockBucketsMs},
		{name: engineLockHoldMs.Name, want: engineLockBucketsMs},
		{name: redpandaPublishLatencyMs.Name, want: redpandaPublishBucketsMs},
		{name: walFsyncLatencyMs.Name, want: walFsyncBucketsMs},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, ok := byName[tc.name]
			require.True(t, ok, "metric %q not emitted", tc.name)

			hist, ok := m.Data.(metricdata.Histogram[int64])
			require.Truef(t, ok, "metric %q is %T, want metricdata.Histogram[int64]", tc.name, m.Data)
			require.NotEmpty(t, hist.DataPoints, "metric %q has no data points", tc.name)

			require.Equal(t, tc.want, hist.DataPoints[0].Bounds,
				"metric %q bucket boundaries drifted from SLO-aligned configuration", tc.name)
		})
	}
}

// TestAuthorizeLatencyBucketsContainSLOEdge guards the SLO-to-bucket
// contract: if defaultAuthorizeLatencySLOMs ever changes, the bucket
// boundaries MUST contain that exact edge so dashboards can report
// "% of requests above SLO" without interpolation.
func TestAuthorizeLatencyBucketsContainSLOEdge(t *testing.T) {
	want := float64(defaultAuthorizeLatencySLOMs)
	found := false

	for _, b := range authorizeLatencyBucketsMs {
		if b == want {
			found = true
			break
		}
	}

	require.Truef(t, found,
		"authorizeLatencyBucketsMs %v must contain SLO edge %.0f ms (defaultAuthorizeLatencySLOMs)",
		authorizeLatencyBucketsMs, want)
}

func TestNormalizeLogTokenSanitizesControlCharsAndLength(t *testing.T) {
	require.Equal(t, "unknown", normalizeLogToken("\n\r\t"))
	require.Equal(t, "tx-1", normalizeLogToken("  tx-1  "))

	long := strings.Repeat("ab", 80)
	result := normalizeLogToken(long)
	require.Len(t, result, 128)
	require.Equal(t, long[:128], result)

	// C1 control characters (U+0080 through U+009F) must be stripped.
	require.Equal(t, "abc", normalizeLogToken("a\u0080b\u009Fc"))

	// Zero-width characters must be stripped.
	require.Equal(t, "abc", normalizeLogToken("a\u200Bb\u200Cc"))
	require.Equal(t, "abc", normalizeLogToken("a\u200Db\uFEFFc"))

	// Input consisting entirely of zero-width characters returns "unknown".
	require.Equal(t, "unknown", normalizeLogToken("\u200B\u200C\u200D\uFEFF"))
}
