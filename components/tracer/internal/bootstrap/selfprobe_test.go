// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build testhooks

package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/observability"
)

// newProbeRecorderForTest builds a Recorder backed by a fresh per-test
// Prometheus registry. The registry is returned so individual tests can
// scrape selfprobe_result for assertions. Per-test registries keep parallel
// test runs isolated and avoid the duplicate-collector panic that would
// otherwise hit the package-default registry.
func newProbeRecorderForTest(t *testing.T) (*observability.Recorder, *prometheus.Registry) {
	t.Helper()

	reg := prometheus.NewRegistry()

	factory, shutdown, err := observability.NewPrometheusBackedFactory(reg, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = shutdown()
	})

	return observability.NewRecorder(factory, nil), reg
}

// gaugeValueFromRegistry scrapes the bridged registry and returns the
// current value of selfprobe_result{dep=<dep>}. Returns 0 when the series
// has not been emitted — callers asserting "never emitted" should check
// gather output directly via reg.Gather().
func gaugeValueFromRegistry(t *testing.T, reg *prometheus.Registry, dep string) float64 {
	t.Helper()

	families, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range families {
		if mf.GetName() != "selfprobe_result" {
			continue
		}

		for _, m := range mf.GetMetric() {
			if !depLabelMatches(m.GetLabel(), dep) {
				continue
			}

			if g := m.GetGauge(); g != nil {
				return g.GetValue()
			}
		}
	}

	return 0
}

// depLabelMatches returns true when the label set contains dep=<dep>.
// Used by gaugeValueFromRegistry to scope the gauge read to a single
// dependency without copying the full label-iteration boilerplate at
// every assertion site.
func depLabelMatches(labels []*dto.LabelPair, dep string) bool {
	for _, l := range labels {
		if l.GetName() == "dep" && l.GetValue() == dep {
			return true
		}
	}

	return false
}

// stubChecker is an inline SelfProbeChecker. The closure captures whatever
// behaviour each test scenario needs (return nil, return err, observe ctx).
type stubChecker struct {
	fn func(ctx context.Context) error
}

func (s stubChecker) Check(ctx context.Context) error {
	if s.fn == nil {
		return nil
	}

	return s.fn(ctx)
}

// TestRunSelfProbe_AllUp_FlipsSelfProbeOKAndReturnsNil exercises the happy
// path: every dep reports nil → selfProbeOK flips to true and gauges read 1.
func TestRunSelfProbe_AllUp_FlipsSelfProbeOKAndReturnsNil(t *testing.T) {
	resetSelfProbeForTest()

	rec, reg := newProbeRecorderForTest(t)

	// Use canonical dep names per the bounded-cardinality contract.
	checks := SelfProbeChecks{
		"postgres":   stubChecker{},
		"rule_cache": stubChecker{},
	}

	err := RunSelfProbe(context.Background(), checks, rec, libLog.NewNop())

	require.NoError(t, err)
	assert.True(t, IsSelfProbeOK(), "self-probe must flip to OK after all deps up")

	assert.EqualValues(t, 1.0, gaugeValueFromRegistry(t, reg, "postgres"))
	assert.EqualValues(t, 1.0, gaugeValueFromRegistry(t, reg, "rule_cache"))
}

// TestRunSelfProbe_OneDown_LeavesSelfProbeOKFalseAndReturnsError verifies the
// failure path: a single down dep is enough to fail the probe, and the gauge
// for that dep reads 0 while the up deps read 1.
func TestRunSelfProbe_OneDown_LeavesSelfProbeOKFalseAndReturnsError(t *testing.T) {
	resetSelfProbeForTest()

	rec, reg := newProbeRecorderForTest(t)

	depErr := errors.New("postgres unreachable")
	checks := SelfProbeChecks{
		"postgres":   stubChecker{fn: func(context.Context) error { return depErr }},
		"rule_cache": stubChecker{},
	}

	err := RunSelfProbe(context.Background(), checks, rec, libLog.NewNop())

	require.Error(t, err)
	assert.False(t, IsSelfProbeOK(), "self-probe must stay false when any dep down")
	assert.Contains(t, err.Error(), "postgres",
		"error must name the failing dep so operators can find it in logs")

	assert.EqualValues(t, 0.0, gaugeValueFromRegistry(t, reg, "postgres"))
	assert.EqualValues(t, 1.0, gaugeValueFromRegistry(t, reg, "rule_cache"))
}

// TestRunSelfProbe_AllDown_LogsAllAndReturnsErr verifies that when every dep
// fails, the returned error captures the failure set so the operator can
// triage from a single line.
func TestRunSelfProbe_AllDown_LogsAllAndReturnsErr(t *testing.T) {
	resetSelfProbeForTest()

	rec, reg := newProbeRecorderForTest(t)

	checks := SelfProbeChecks{
		"postgres":   stubChecker{fn: func(context.Context) error { return errors.New("x down") }},
		"rule_cache": stubChecker{fn: func(context.Context) error { return errors.New("y down") }},
	}

	err := RunSelfProbe(context.Background(), checks, rec, libLog.NewNop())

	require.Error(t, err)
	assert.False(t, IsSelfProbeOK())

	// All names must surface in the error string for triage.
	for _, name := range []string{"postgres", "rule_cache"} {
		assert.Contains(t, err.Error(), name)
		assert.EqualValues(t, 0.0, gaugeValueFromRegistry(t, reg, name),
			"gauge for failed dep must be 0")
	}
}

// TestRunSelfProbe_EmptyChecks_FlipsOKAndReturnsNil documents the degenerate
// case: a service with no required deps trivially passes self-probe.
func TestRunSelfProbe_EmptyChecks_FlipsOKAndReturnsNil(t *testing.T) {
	resetSelfProbeForTest()

	rec, _ := newProbeRecorderForTest(t)

	err := RunSelfProbe(context.Background(), SelfProbeChecks{}, rec, libLog.NewNop())

	require.NoError(t, err)
	assert.True(t, IsSelfProbeOK(),
		"empty checks ⇒ nothing to fail ⇒ self-probe is trivially OK")
}

// TestIsSelfProbeOK_DefaultsFalse pins the initial atomic value: BEFORE any
// RunSelfProbe call, /health must report 503 (selfProbeOK is false).
func TestIsSelfProbeOK_DefaultsFalse(t *testing.T) {
	resetSelfProbeForTest()

	assert.False(t, IsSelfProbeOK(),
		"selfProbeOK must default to false so /health returns 503 until probe runs")
}

// TestRunSelfProbe_EmitsGaugePerDep verifies the metrics contract: a gauge
// reading is emitted for every dep, regardless of outcome.
func TestRunSelfProbe_EmitsGaugePerDep(t *testing.T) {
	resetSelfProbeForTest()

	rec, reg := newProbeRecorderForTest(t)

	// Use canonical dep names — the metrics layer drops out-of-set values to
	// preserve the bounded-cardinality contract.
	checks := SelfProbeChecks{
		"postgres":   stubChecker{},
		"rule_cache": stubChecker{fn: func(context.Context) error { return errors.New("down") }},
	}

	_ = RunSelfProbe(context.Background(), checks, rec, libLog.NewNop())

	assert.EqualValues(t, 1.0, gaugeValueFromRegistry(t, reg, "postgres"))
	assert.EqualValues(t, 0.0, gaugeValueFromRegistry(t, reg, "rule_cache"))
}

// TestRunSelfProbe_RespectsContextCancellation verifies the probe surfaces
// ctx.Err() when the parent context is already cancelled.
func TestRunSelfProbe_RespectsContextCancellation(t *testing.T) {
	resetSelfProbeForTest()

	rec, _ := newProbeRecorderForTest(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	checks := SelfProbeChecks{
		"dep_ctx_test": stubChecker{fn: func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				return err
			}

			return nil
		}},
	}

	err := RunSelfProbe(ctx, checks, rec, libLog.NewNop())

	require.Error(t, err)
	assert.False(t, IsSelfProbeOK())
}

// TestRunSelfProbe_HangingChecker_TimesOut verifies the per-check timeout
// guard: a Check() implementation that never returns on its own MUST be
// bounded by selfProbeCheckTimeout so the boot path never hangs. The probe
// surfaces a context-deadline-exceeded style error and selfProbeOK stays
// false (caller aborts startup).
func TestRunSelfProbe_HangingChecker_TimesOut(t *testing.T) {
	resetSelfProbeForTest()

	// Shrink the timeout so the test runs fast. Restore on cleanup.
	original := selfProbeCheckTimeout
	selfProbeCheckTimeout = 100 * time.Millisecond

	t.Cleanup(func() { selfProbeCheckTimeout = original })

	rec, _ := newProbeRecorderForTest(t)

	checks := SelfProbeChecks{
		"hanging_dep": stubChecker{fn: func(ctx context.Context) error {
			// Block until the per-check ctx is cancelled; surface its err.
			<-ctx.Done()

			return ctx.Err()
		}},
	}

	start := time.Now()
	err := RunSelfProbe(context.Background(), checks, rec, libLog.NewNop())
	elapsed := time.Since(start)

	require.Error(t, err, "hanging checker must surface a self-probe failure")
	assert.False(t, IsSelfProbeOK(),
		"selfProbeOK must stay false when a checker times out")
	assert.Contains(t, err.Error(), "hanging_dep",
		"error must name the timing-out dep so operators can find it in logs")

	// Bound the elapsed time generously. With timeout=100ms, the probe must
	// return within ~1s even on slow CI; a hanging probe without the guard
	// would never return at all.
	assert.Less(t, elapsed, time.Second,
		"per-check timeout must bound the probe; got %s", elapsed)
}

// TestDrainGracePeriod_Default returns 12s when cfg is nil or the field is
// non-positive — matches the K8s readinessProbe default math.
func TestDrainGracePeriod_Default(t *testing.T) {
	got := drainGracePeriod(nil)
	assert.Equal(t, "12s", got.String())

	got = drainGracePeriod(&Config{})
	assert.Equal(t, "12s", got.String())

	got = drainGracePeriod(&Config{ReadyzDrainGraceSeconds: 0})
	assert.Equal(t, "12s", got.String())

	got = drainGracePeriod(&Config{ReadyzDrainGraceSeconds: -5})
	assert.Equal(t, "12s", got.String())
}

// TestDrainGracePeriod_Override honors a positive operator-supplied value.
func TestDrainGracePeriod_Override(t *testing.T) {
	got := drainGracePeriod(&Config{ReadyzDrainGraceSeconds: 7})
	assert.Equal(t, "7s", got.String())

	got = drainGracePeriod(&Config{ReadyzDrainGraceSeconds: 30})
	assert.Equal(t, "30s", got.String())
}

// TestRunSelfProbe_NilChecker_ReturnsErrorAndNamesDep pins the defensive
// nil-checker guard. A SelfProbeChecks map with a nil value would otherwise
// panic on checker.Check(ctx); the guard turns that into a structured error,
// records the dep as failed, and emits selfprobe_result=0 so /health stays
// 503 and operators can grep the dep name from logs.
func TestRunSelfProbe_NilChecker_ReturnsErrorAndNamesDep(t *testing.T) {
	resetSelfProbeForTest()

	rec, reg := newProbeRecorderForTest(t)

	// Use canonical dep names — the metrics layer drops out-of-set values
	// silently to preserve the bounded-cardinality contract, so the gauge
	// assertion below would always read 0 for a non-canonical name.
	checks := SelfProbeChecks{
		"postgres":   nil,
		"rule_cache": stubChecker{},
	}

	err := RunSelfProbe(context.Background(), checks, rec, libLog.NewNop())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "postgres",
		"error must name the nil-checker dep so operators can find it in logs")
	assert.False(t, IsSelfProbeOK(),
		"selfProbeOK must remain false when any dep entry is nil")

	// Healthy dep still emits its gauge=1; nil-checker dep emits gauge=0.
	assert.EqualValues(t, 1.0, gaugeValueFromRegistry(t, reg, "rule_cache"))
	assert.EqualValues(t, 0.0, gaugeValueFromRegistry(t, reg, "postgres"),
		"nil-checker dep must record selfprobe_result=0")
}

// TestRunSelfProbe_NilLogger_ReturnsError pins the defensive nil-logger guard.
// Without this guard, RunSelfProbe panics on logger.With(...) at the very top
// of the function — the panic would crash the bootstrap goroutine without
// emitting a structured error, leaving operators with only a stack trace.
//
// The guard returns a sentinel-shaped error and leaves selfProbeOK unchanged
// (still false) so /health continues to return 503 and K8s restarts the pod.
func TestRunSelfProbe_NilLogger_ReturnsError(t *testing.T) {
	resetSelfProbeForTest()

	checks := SelfProbeChecks{
		"dep_x_nil_logger": stubChecker{},
	}

	rec, _ := newProbeRecorderForTest(t)

	err := RunSelfProbe(context.Background(), checks, rec, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil logger",
		"error must mention the nil-logger condition so operators can grep for it")
	assert.False(t, IsSelfProbeOK(),
		"selfProbeOK must remain false when probe rejects its inputs")
}

// TestSanitizeProbeError pins the credential-redaction contract for self-probe
// log lines. pgx and other drivers commonly include connection-string
// fragments (host=..., user=..., password=..., dbname=...) in error messages;
// surfacing them verbatim leaks operational data into log aggregators that
// may have weaker access controls than the database itself.
func TestSanitizeProbeError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    string
		wantNot []string
	}{
		{
			name: "nil_returns_empty_string",
			err:  nil,
			want: "",
		},
		{
			name:    "no_secrets_returned_unchanged",
			err:     errors.New("connection refused"),
			want:    "connection refused",
			wantNot: []string{"[redacted]"},
		},
		{
			name:    "host_is_redacted",
			err:     errors.New("dial tcp host=db.internal.example.com failed"),
			wantNot: []string{"db.internal.example.com"},
		},
		{
			name:    "user_is_redacted",
			err:     errors.New("auth: user=admin password=hunter2 invalid"),
			wantNot: []string{"admin", "hunter2"},
		},
		{
			name:    "password_is_redacted_at_eol",
			err:     errors.New("connect: password=secretvalue"),
			wantNot: []string{"secretvalue"},
		},
		{
			name:    "dbname_is_redacted",
			err:     errors.New("dbname=production_db: not found"),
			wantNot: []string{"production_db"},
		},
		{
			name:    "multiple_keys_all_redacted",
			err:     errors.New("FATAL: host=db.example.com user=admin password=hunter2 dbname=prod"),
			wantNot: []string{"db.example.com", "admin", "hunter2", "prod"},
		},
		{
			name:    "single_quoted_password_with_spaces_redacted",
			err:     errors.New("FATAL: password='secret with spaces' more"),
			wantNot: []string{"secret with spaces"},
		},
		{
			name:    "double_quoted_password_redacted",
			err:     errors.New(`FATAL: password="quoted" trailing`),
			wantNot: []string{"quoted"},
		},
		{
			name:    "url_form_credentials_redacted",
			err:     errors.New("dial postgres://u:p@h/d connection refused"),
			wantNot: []string{"u:p@h", "//u:p"},
		},
		{
			name:    "url_form_with_query_redacted",
			err:     errors.New("postgresql://user:pass@host:5432/db?sslmode=require failed"),
			wantNot: []string{"user:pass@host", "//user:pass"},
		},
		{
			name: "mixed_quoted_and_unquoted_keys_all_redacted",
			err: errors.New(
				`FATAL: host="db.example.com" user='admin user' password=hunter2 dbname='production db'`,
			),
			wantNot: []string{"db.example.com", "admin user", "hunter2", "production db"},
		},
		{
			name:    "sslmode_value_redacted",
			err:     errors.New("FATAL: sslmode=require host=db.example.com"),
			wantNot: []string{"db.example.com"},
		},
		{
			name:    "single_quoted_with_escaped_quote_consumed",
			err:     errors.New(`password='a\'b' next`),
			wantNot: []string{`a\'b`, "a'b"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeProbeError(tc.err)

			if tc.want != "" {
				assert.Equal(t, tc.want, got)
			}

			for _, forbidden := range tc.wantNot {
				assert.NotContains(t, got, forbidden,
					"sanitized output must not contain credential fragment %q", forbidden)
			}

			if tc.err == nil {
				assert.Empty(t, got, "nil error must produce empty string")
			}
		})
	}
}
