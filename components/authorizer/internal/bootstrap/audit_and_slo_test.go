// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libMetrics "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"
)

// captureLogger is a libLog.Logger that records every formatted message so
// tests can assert audit / security log lines carry the right fields without
// relying on stdout capture (which is racy under parallel tests).
type captureLogger struct {
	mu    sync.Mutex
	lines []string
}

func (l *captureLogger) record(_, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *captureLogger) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]string, len(l.lines))
	copy(out, l.lines)

	return out
}

func (l *captureLogger) Info(args ...any)                                  { _ = args }
func (l *captureLogger) Infof(f string, a ...any)                          { l.record("info", f, a...) }
func (l *captureLogger) Infoln(args ...any)                                { _ = args }
func (l *captureLogger) Warn(args ...any)                                  { _ = args }
func (l *captureLogger) Warnf(f string, a ...any)                          { l.record("warn", f, a...) }
func (l *captureLogger) Warnln(args ...any)                                { _ = args }
func (l *captureLogger) Error(args ...any)                                 { _ = args }
func (l *captureLogger) Errorf(f string, a ...any)                         { l.record("error", f, a...) }
func (l *captureLogger) Errorln(args ...any)                               { _ = args }
func (l *captureLogger) Debug(args ...any)                                 { _ = args }
func (l *captureLogger) Debugf(f string, a ...any)                         { _ = f; _ = a }
func (l *captureLogger) Debugln(args ...any)                               { _ = args }
func (l *captureLogger) Fatal(args ...any)                                 { _ = args }
func (l *captureLogger) Fatalf(f string, a ...any)                         { _ = f; _ = a }
func (l *captureLogger) Fatalln(args ...any)                               { _ = args }
func (l *captureLogger) Sync() error                                       { return nil }
func (l *captureLogger) WithFields(_ ...any) libLog.Logger                 { return l }
func (l *captureLogger) WithDefaultMessageTemplate(_ string) libLog.Logger { return l }

// TestAuthorizeAuditEvent_WritesTenantContext verifies the audit channel
// carries organization_id, ledger_id, tx_id, actor, result, rejection_code,
// amount_bucket, and cross_shard. Metrics labels cannot safely include
// organization_id (cardinality), so the audit log is the sole source of
// per-tenant decision history.
func TestAuthorizeAuditEvent_WritesTenantContext(t *testing.T) {
	logger := &captureLogger{}
	metrics := &authorizerMetrics{logger: logger}

	metrics.EmitAuthorizationAuditEvent(
		context.Background(),
		"org-42",
		"ledger-9",
		"tx-abc",
		"user@example.com",
		"APPROVED",
		"", // empty rejection_code — normalizes to "none" (approved)
		"1_10",
		true,
	)

	lines := logger.snapshot()
	require.Len(t, lines, 1, "expected exactly one audit line, got %v", lines)

	line := lines[0]
	require.Contains(t, line, "AUTHORIZER_AUDIT")
	require.Contains(t, line, "tenant=org-42")
	require.Contains(t, line, "ledger=ledger-9")
	require.Contains(t, line, "tx_id=tx-abc")
	require.Contains(t, line, "actor=user@example.com")
	require.Contains(t, line, "result=APPROVED")
	require.Contains(t, line, "rejection_code=none")
	require.Contains(t, line, "amount_bucket=1_10")
	require.Contains(t, line, "cross_shard=true")
}

// TestAuthorizeAuditEvent_SanitizesLogForgery guards against newline
// injection via user-controlled fields. A forged organization_id containing
// \n + "FORGED" must be stripped of the CR/LF before hitting the log line.
func TestAuthorizeAuditEvent_SanitizesLogForgery(t *testing.T) {
	logger := &captureLogger{}
	metrics := &authorizerMetrics{logger: logger}

	metrics.EmitAuthorizationAuditEvent(
		context.Background(),
		"org\nFORGED_TENANT",
		"ledger\rFORGED",
		"tx",
		"actor",
		"APPROVED",
		"",
		"",
		false,
	)

	lines := logger.snapshot()
	require.Len(t, lines, 1)
	require.NotContains(t, lines[0], "\n", "CR/LF MUST be stripped from audit line")
	require.NotContains(t, lines[0], "\r")
}

// TestSLOBreachCounter_IncludesRejectionCodeAndCrossShard verifies the SLO
// breach counter now carries rejection_code and cross_shard — the two
// dimensions an oncall reaches for first when a breach fires. Missing
// either forces blind root-cause investigation.
func TestSLOBreachCounter_IncludesRejectionCodeAndCrossShard(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	factory := libMetrics.NewMetricsFactory(mp.Meter("authorizer-slo-test"), nil)

	metrics := &authorizerMetrics{
		factory:             factory,
		authorizeLatencySLO: 100 * time.Millisecond,
	}

	// Record a breach: 200ms > 100ms SLO with a rejection_code and
	// cross_shard=true.
	metrics.RecordAuthorize(
		context.Background(),
		"Authorize",
		"rejected",
		"INSUFFICIENT_FUNDS",
		false,
		"created",
		2,
		1,
		200*time.Millisecond,
		true,
	)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	var breachMetric *metricdata.Metrics

	for i := range rm.ScopeMetrics {
		for j := range rm.ScopeMetrics[i].Metrics {
			if rm.ScopeMetrics[i].Metrics[j].Name == authorizeLatencySLOBreachesTotal.Name {
				breachMetric = &rm.ScopeMetrics[i].Metrics[j]
				break
			}
		}
	}

	require.NotNil(t, breachMetric, "expected %q to be emitted", authorizeLatencySLOBreachesTotal.Name)

	sum, ok := breachMetric.Data.(metricdata.Sum[int64])
	require.Truef(t, ok, "metric %q is %T, want Sum[int64]", authorizeLatencySLOBreachesTotal.Name, breachMetric.Data)
	require.NotEmpty(t, sum.DataPoints)

	dp := sum.DataPoints[0]
	attrs := dp.Attributes.ToSlice()

	var (
		gotRejectionCode string
		gotCrossShard    string
	)

	for _, kv := range attrs {
		switch string(kv.Key) {
		case "rejection_code":
			gotRejectionCode = kv.Value.AsString()
		case "cross_shard":
			gotCrossShard = kv.Value.AsString()
		}
	}

	require.Equal(t, "insufficient_funds", gotRejectionCode,
		"SLO breach counter MUST carry rejection_code label")
	require.Equal(t, "true", gotCrossShard,
		"SLO breach counter MUST carry cross_shard label")
}

// TestSecurityLogger_RoutesAuthFailuresDistinctly verifies RecordSecurityEvent
// routes events through a dedicated prefix so operators can filter on it
// separately from operational INFO. Category is bounded; severity controls
// log level.
func TestSecurityLogger_RoutesAuthFailuresDistinctly(t *testing.T) {
	logger := &captureLogger{}
	metrics := &authorizerMetrics{logger: logger}

	metrics.RecordSecurityEvent("error", "auth_failure", "peer token invalid")
	metrics.RecordSecurityEvent("warn", "policy_rejection", "denied by policy")
	metrics.RecordSecurityEvent("", "rate_limit", "bucket exhausted")

	lines := logger.snapshot()
	require.Len(t, lines, 3)

	for _, line := range lines {
		require.True(t, strings.HasPrefix(line, "AUTHORIZER_SECURITY"),
			"every security line MUST begin with AUTHORIZER_SECURITY, got %q", line)
	}

	require.Contains(t, lines[0], "category=auth_failure")
	require.Contains(t, lines[0], "detail=peer token invalid")
	require.Contains(t, lines[1], "category=policy_rejection")
	require.Contains(t, lines[2], "category=rate_limit")
}

// TestPreparedExpired_MetricAndLog verifies ObservePreparedExpired emits both
// the counter and a warning-level log line with the normalized reason.
func TestPreparedExpired_MetricAndLog(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	factory := libMetrics.NewMetricsFactory(mp.Meter("authorizer-expired-test"), nil)
	logger := &captureLogger{}
	metrics := &authorizerMetrics{factory: factory, logger: logger}

	metrics.ObservePreparedExpired("timeout")
	metrics.ObservePreparedExpired("UNKNOWN_REASON")

	lines := logger.snapshot()
	require.Len(t, lines, 2)
	require.Contains(t, lines[0], "reason=timeout")
	require.Contains(t, lines[1], "reason=other")

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	found := false

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == authorizerPreparedExpiredTotal.Name {
				found = true
				break
			}
		}
	}

	require.True(t, found, "authorizer_prepared_expired_total MUST be emitted")
}

// TestBucketPreparedDepth_Boundaries ensures the shard_range label collapses
// the depth value into bounded-cardinality ranges.
func TestBucketPreparedDepth_Boundaries(t *testing.T) {
	tests := []struct {
		depth int
		want  string
	}{
		{depth: 0, want: "zero"},
		{depth: -1, want: "zero"},
		{depth: 1, want: "1_9"},
		{depth: 9, want: "1_9"},
		{depth: 10, want: "10_99"},
		{depth: 99, want: "10_99"},
		{depth: 100, want: "100_999"},
		{depth: 999, want: "100_999"},
		{depth: 1000, want: "1000_plus"},
		{depth: 9999, want: "1000_plus"},
	}

	for _, tc := range tests {
		require.Equalf(t, tc.want, bucketPreparedDepth(tc.depth),
			"bucketPreparedDepth(%d)", tc.depth)
	}
}

// TestNormalizePreparedExpirationReason_Bounds verifies only the two stable
// reasons pass through; anything else collapses to "other" / "unknown".
func TestNormalizePreparedExpirationReason_Bounds(t *testing.T) {
	require.Equal(t, "unknown", normalizePreparedExpirationReason(""))
	require.Equal(t, "timeout", normalizePreparedExpirationReason("timeout"))
	require.Equal(t, "timeout", normalizePreparedExpirationReason("  TIMEOUT  "))
	require.Equal(t, "force_abort", normalizePreparedExpirationReason("force_abort"))
	require.Equal(t, "other", normalizePreparedExpirationReason("something_else"))
}

// Ensure the capture logger satisfies the libLog.Logger interface at compile
// time. Failure here forces the mock to be fixed alongside library updates.
var _ libLog.Logger = (*captureLogger)(nil)

// Reference libOpentelemetry to keep the import in sync with the production
// code path — removing it would hide test-to-production drift.
var _ = libOpentelemetry.Telemetry{}
