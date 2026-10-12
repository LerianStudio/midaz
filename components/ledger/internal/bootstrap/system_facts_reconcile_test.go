// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/LerianStudio/lib-system-facts/reconciler"
	"github.com/LerianStudio/lib-system-facts/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/LerianStudio/midaz/v4/tests/utils/stubs"
)

// reconcileAnswer is one scripted answer of the fake reconciler.
type reconcileAnswer struct {
	report reconciler.Report
	err    error
}

// fakeSystemFactsReconciler answers Preflight and Run from scripts, the last
// entry repeating, and records the deadline of every call it received.
type fakeSystemFactsReconciler struct {
	preflights []reconcileAnswer
	runs       []reconcileAnswer

	mu             sync.Mutex
	preflightCalls int
	runCalls       int
	deadlines      []time.Time
	undeadlined    int
}

func (f *fakeSystemFactsReconciler) Preflight(ctx context.Context) (reconciler.Report, error) {
	return f.answer(ctx, f.preflights, &f.preflightCalls)
}

func (f *fakeSystemFactsReconciler) Run(ctx context.Context) (reconciler.Report, error) {
	return f.answer(ctx, f.runs, &f.runCalls)
}

func (f *fakeSystemFactsReconciler) answer(ctx context.Context, script []reconcileAnswer, calls *int) (reconciler.Report, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	index := *calls
	*calls++

	if deadline, ok := ctx.Deadline(); ok {
		f.deadlines = append(f.deadlines, deadline)
	} else {
		f.undeadlined++
	}

	if len(script) == 0 {
		return reconciler.Report{}, errors.New("unscripted reconciler call")
	}

	if index >= len(script) {
		index = len(script) - 1
	}

	return script[index].report, script[index].err
}

func (f *fakeSystemFactsReconciler) counts() (preflights, runs int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.preflightCalls, f.runCalls
}

func (f *fakeSystemFactsReconciler) recordedDeadlines() ([]time.Time, int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]time.Time(nil), f.deadlines...), f.undeadlined
}

// The subjects the scripted steps name: the ledger's own reference type and
// its one contract version.
const (
	reconcileRefTypeSubject  = "lerian.midaz.transaction_id"
	reconcileContractSubject = "midaz/ledger/midaz.transaction_committed/1.0.0"
)

func okAnswer(steps ...reconciler.Step) reconcileAnswer {
	return reconcileAnswer{report: reconciler.Report{Outcome: reconciler.OutcomeOK, Steps: steps}}
}

func unavailableAnswer(step reconciler.Step) reconcileAnswer {
	step.Result = reconciler.ResultUnavailable

	return reconcileAnswer{
		report: reconciler.Report{Outcome: reconciler.OutcomeUnavailable, Steps: []reconciler.Step{step}},
		err:    fmt.Errorf("%w: %s %s: dial tcp 10.0.0.9:443: connection refused", reconciler.ErrUnavailable, step.Kind, step.Subject),
	}
}

func rejectedAnswer(step reconciler.Step, problem *registry.Problem) reconcileAnswer {
	step.Result = reconciler.ResultRejected
	step.Problem = problem

	err := fmt.Errorf("%w: %s %s: %s", reconciler.ErrRejected, step.Kind, step.Subject, step.Detail)
	if problem != nil {
		err = fmt.Errorf("%w: %s %s: %w", reconciler.ErrRejected, step.Kind, step.Subject, problem)
	}

	return reconcileAnswer{report: reconciler.Report{Outcome: reconciler.OutcomeRejected, Steps: []reconciler.Step{step}}, err: err}
}

func invalidAnswer(step reconciler.Step) reconcileAnswer {
	step.Result = reconciler.ResultInvalid

	return reconcileAnswer{
		report: reconciler.Report{Outcome: reconciler.OutcomeInvalid, Steps: []reconciler.Step{step}},
		err:    fmt.Errorf("%w: %s", reconciler.ErrInvalid, step.Detail),
	}
}

func verifiedStep(kind reconciler.StepKind, subject string) reconciler.Step {
	return reconciler.Step{Kind: kind, Subject: subject, Result: reconciler.ResultVerified}
}

// reconcileBoot is the fixed clock of the gate under test.
var reconcileBoot = time.Date(2026, time.October, 12, 9, 0, 0, 0, time.UTC)

// newManualMetricReader is a manual OTel reader a test collects counters from.
func newManualMetricReader(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()

	return sdkmetric.NewManualReader()
}

// newSystemFactsMetricsFactoryOver builds a MetricsFactory whose counters
// reader collects.
func newSystemFactsMetricsFactoryOver(t *testing.T, reader *sdkmetric.ManualReader) *metrics.MetricsFactory {
	t.Helper()

	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	factory, err := metrics.NewMetricsFactory(mp.Meter("system-facts-reconcile-test"), nil)
	require.NoError(t, err)

	return factory
}

// testContractGate wires the gate over fake with every seam injected: the
// delay is attempt seconds, the wait is recorded instead of slept.
func testContractGate(t *testing.T, fake systemFactsReconciler, logger libLog.Logger, reader *sdkmetric.ManualReader, waits *waitRecorder) *systemFactsContractGate {
	t.Helper()

	return &systemFactsContractGate{
		reconciler:     fake,
		attemptTimeout: 2500 * time.Millisecond,
		logger:         logger,
		metricsFactory: newSystemFactsMetricsFactoryOver(t, reader),
		now:            func() time.Time { return reconcileBoot },
		delay:          func(attempt int) time.Duration { return time.Duration(attempt) * time.Second },
		wait:           waits.wait,
	}
}

// counterByLabel reads the named counter's value for every value of label.
func counterByLabel(t *testing.T, reader *sdkmetric.ManualReader, name, label string) map[string]int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	totals := map[string]int64{}

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "counter data type must be Sum[int64], got %T", m.Data)

			for _, dp := range sum.DataPoints {
				value, found := dp.Attributes.Value(attribute.Key(label))
				require.True(t, found, "every sample carries the %s label", label)

				totals[value.AsString()] += dp.Value
			}
		}
	}

	return totals
}

func TestSystemFactsContractGate_Reconcile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		preflights         []reconcileAnswer
		runs               []reconcileAnswer
		wantReleased       bool
		wantState          systemFactsContractState
		wantPreflightCalls int
		wantRunCalls       int
		wantWaits          []time.Duration
		wantOutcomes       map[string]int64
		wantInfo           []string
		wantWarnings       [][]string
		wantErrors         []string
	}{
		{
			name: "registered contract: preflight OK releases without Run",
			preflights: []reconcileAnswer{okAnswer(
				verifiedStep(reconciler.StepReferenceType, reconcileRefTypeSubject),
				verifiedStep(reconciler.StepReadBack, reconcileContractSubject),
			)},
			wantReleased:       true,
			wantState:          systemFactsContractActive,
			wantPreflightCalls: 1,
			wantOutcomes:       map[string]int64{"ok": 1},
			wantInfo:           []string{"System Facts contract active", "source=preflight", "created=0", "replayed=0", "verified=2"},
		},
		{
			name: "gap: preflight rejected, Run OK releases",
			preflights: []reconcileAnswer{rejectedAnswer(
				reconciler.Step{Kind: reconciler.StepReadBack, Subject: reconcileContractSubject, Detail: "contract version not registered"}, nil)},
			runs: []reconcileAnswer{okAnswer(
				reconciler.Step{Kind: reconciler.StepReferenceType, Subject: reconcileRefTypeSubject, Result: reconciler.ResultReplayed},
				reconciler.Step{Kind: reconciler.StepContract, Subject: reconcileContractSubject, Result: reconciler.ResultCreated},
				verifiedStep(reconciler.StepReadBack, reconcileContractSubject),
			)},
			wantReleased:       true,
			wantState:          systemFactsContractActive,
			wantPreflightCalls: 1,
			wantRunCalls:       1,
			wantOutcomes:       map[string]int64{"ok": 1},
			wantInfo:           []string{"source=reconcile", "created=1", "replayed=1", "verified=1"},
		},
		{
			name: "registry unavailable at preflight: deferred with a growing delay, released on the first OK",
			preflights: []reconcileAnswer{
				unavailableAnswer(reconciler.Step{Kind: reconciler.StepReferenceType, Subject: reconcileRefTypeSubject}),
				unavailableAnswer(reconciler.Step{Kind: reconciler.StepReferenceType, Subject: reconcileRefTypeSubject}),
				okAnswer(verifiedStep(reconciler.StepReadBack, reconcileContractSubject)),
			},
			wantReleased:       true,
			wantState:          systemFactsContractActive,
			wantPreflightCalls: 3,
			wantWaits:          []time.Duration{time.Second, 2 * time.Second},
			wantOutcomes:       map[string]int64{"unavailable": 2, "ok": 1},
			wantInfo:           []string{"source=preflight"},
			wantWarnings: [][]string{
				{"System Facts contract reconcile deferred", "attempt=1", "outcome=unavailable", "next_retry_in=1s", "step_kind=reference-type", "step_subject=" + reconcileRefTypeSubject},
				{"attempt=2", "next_retry_in=2s"},
			},
		},
		{
			name: "registry unavailable at Run: deferred, released on the next attempt",
			preflights: []reconcileAnswer{
				rejectedAnswer(reconciler.Step{Kind: reconciler.StepReadBack, Subject: reconcileContractSubject, Detail: "contract version not registered"}, nil),
				okAnswer(verifiedStep(reconciler.StepReadBack, reconcileContractSubject)),
			},
			runs:               []reconcileAnswer{unavailableAnswer(reconciler.Step{Kind: reconciler.StepContract, Subject: reconcileContractSubject})},
			wantReleased:       true,
			wantState:          systemFactsContractActive,
			wantPreflightCalls: 2,
			wantRunCalls:       1,
			wantWaits:          []time.Duration{time.Second},
			wantOutcomes:       map[string]int64{"unavailable": 1, "ok": 1},
			wantInfo:           []string{"source=preflight"},
			wantWarnings:       [][]string{{"attempt=1", "step_kind=contract"}},
		},
		{
			name: "Run refused with a 404 is an ingress mid-rollout: deferred and retried, not rejected",
			preflights: []reconcileAnswer{
				rejectedAnswer(reconciler.Step{Kind: reconciler.StepReadBack, Subject: reconcileContractSubject, Detail: "contract version not registered"}, nil),
				okAnswer(verifiedStep(reconciler.StepReadBack, reconcileContractSubject)),
			},
			runs: []reconcileAnswer{rejectedAnswer(
				reconciler.Step{Kind: reconciler.StepContract, Subject: reconcileContractSubject},
				&registry.Problem{Status: http.StatusNotFound, Title: "Not Found"})},
			wantReleased:       true,
			wantState:          systemFactsContractActive,
			wantPreflightCalls: 2,
			wantRunCalls:       1,
			wantWaits:          []time.Duration{time.Second},
			wantOutcomes:       map[string]int64{"unavailable": 1, "ok": 1},
			wantInfo:           []string{"source=preflight"},
			wantWarnings:       [][]string{{"attempt=1", "outcome=unavailable", "step_kind=contract"}},
		},
		{
			name: "Run rejected: one error, no further attempt, gate rejected",
			preflights: []reconcileAnswer{rejectedAnswer(
				reconciler.Step{Kind: reconciler.StepReadBack, Subject: reconcileContractSubject, Detail: "contract version not registered"}, nil)},
			runs: []reconcileAnswer{rejectedAnswer(
				reconciler.Step{Kind: reconciler.StepContract, Subject: reconcileContractSubject},
				&registry.Problem{Status: http.StatusConflict, Code: "SF-0002", Detail: "content hash differs from the registered version"})},
			wantState:          systemFactsContractRejected,
			wantPreflightCalls: 1,
			wantRunCalls:       1,
			wantOutcomes:       map[string]int64{"rejected": 1},
			wantErrors: []string{"System Facts contract rejected; the forwarder stays stopped", "outcome=rejected",
				"step_kind=contract", "step_subject=" + reconcileContractSubject, "problem_code=SF-0002",
				"problem_detail=content hash differs from the registered version"},
		},
		{
			name: "Run invalid: one error, gate rejected",
			preflights: []reconcileAnswer{rejectedAnswer(
				reconciler.Step{Kind: reconciler.StepReadBack, Subject: reconcileContractSubject, Detail: "contract version not registered"}, nil)},
			runs:               []reconcileAnswer{invalidAnswer(reconciler.Step{Kind: reconciler.StepManifest, Subject: "manifest", Detail: "the manifest is nil"})},
			wantState:          systemFactsContractRejected,
			wantPreflightCalls: 1,
			wantRunCalls:       1,
			wantOutcomes:       map[string]int64{"invalid": 1},
			wantErrors:         []string{"outcome=invalid", "step_kind=manifest", "step_subject=manifest"},
		},
		{
			name:               "preflight invalid: one error, no Run, gate rejected",
			preflights:         []reconcileAnswer{invalidAnswer(reconciler.Step{Kind: reconciler.StepManifest, Subject: "manifest", Detail: "the manifest is nil"})},
			wantState:          systemFactsContractRejected,
			wantPreflightCalls: 1,
			wantOutcomes:       map[string]int64{"invalid": 1},
			wantErrors:         []string{"outcome=invalid", "step_kind=manifest"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeSystemFactsReconciler{preflights: tt.preflights, runs: tt.runs}
			logger := &stubs.LoggerStub{}
			reader := newManualMetricReader(t)
			waits := &waitRecorder{}
			gate := testContractGate(t, fake, logger, reader, waits)

			released := gate.reconcile(context.Background())

			assert.Equal(t, tt.wantReleased, released)
			assert.Equal(t, tt.wantState, gate.contractState())

			preflights, runs := fake.counts()
			assert.Equal(t, tt.wantPreflightCalls, preflights, "preflight calls")
			assert.Equal(t, tt.wantRunCalls, runs, "run calls")
			assert.Equal(t, tt.wantWaits, waits.recorded())
			assert.Equal(t, tt.wantOutcomes, counterByLabel(t, reader, "system_facts_contract_reconcile_total", "outcome"))

			if len(tt.wantInfo) == 0 {
				assert.Empty(t, logger.Infos, "nothing is logged at Info before a release")
			} else {
				require.Len(t, logger.Infos, 1, "a release is logged exactly once")

				for _, want := range tt.wantInfo {
					assert.Contains(t, logger.Infos[0], want)
				}
			}

			require.Len(t, logger.Warnings, len(tt.wantWarnings), "one Warn per deferred attempt")

			for i, wants := range tt.wantWarnings {
				for _, want := range wants {
					assert.Contains(t, logger.Warnings[i], want)
				}
			}

			if len(tt.wantErrors) == 0 {
				assert.Empty(t, logger.Errors)

				return
			}

			require.Len(t, logger.Errors, 1, "a permanent refusal is logged exactly once")

			for _, want := range tt.wantErrors {
				assert.Contains(t, logger.Errors[0], want)
			}

			assert.NotContains(t, logger.Errors[0], "10.0.0.9", "a host never reaches the log")
		})
	}
}

func TestSystemFactsContractGate_Reconcile_ContextEndsWhileWaiting(t *testing.T) {
	t.Parallel()

	fake := &fakeSystemFactsReconciler{preflights: []reconcileAnswer{
		unavailableAnswer(reconciler.Step{Kind: reconciler.StepReferenceType, Subject: reconcileRefTypeSubject}),
	}}
	logger := &stubs.LoggerStub{}
	reader := newManualMetricReader(t)
	waits := &waitRecorder{}
	gate := testContractGate(t, fake, logger, reader, waits)

	ctx, cancel := context.WithCancel(context.Background())
	gate.wait = func(ctx context.Context, d time.Duration) error {
		cancel()

		return waits.wait(ctx, d)
	}

	released := gate.reconcile(ctx)

	assert.False(t, released)
	assert.Equal(t, systemFactsContractPending, gate.contractState(), "an ended context leaves the gate pending")

	preflights, runs := fake.counts()
	assert.Equal(t, 1, preflights)
	assert.Equal(t, 0, runs, "no Run after the context ended")
	assert.Equal(t, []time.Duration{time.Second}, waits.recorded())
	assert.Empty(t, logger.Errors, "shutdown is not a refusal")
}

func TestSystemFactsContractGate_Reconcile_ContextAlreadyEndedCountsNothing(t *testing.T) {
	t.Parallel()

	fake := &fakeSystemFactsReconciler{preflights: []reconcileAnswer{okAnswer()}}
	logger := &stubs.LoggerStub{}
	reader := newManualMetricReader(t)
	gate := testContractGate(t, fake, logger, reader, &waitRecorder{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.False(t, gate.reconcile(ctx))
	assert.Equal(t, systemFactsContractPending, gate.contractState())
	assert.Empty(t, counterByLabel(t, reader, "system_facts_contract_reconcile_total", "outcome"))
	assert.Empty(t, logger.Infos)
	assert.Empty(t, logger.Errors)
}

func TestSystemFactsContractGate_Reconcile_NilReconcilerFailsClosed(t *testing.T) {
	t.Parallel()

	logger := &stubs.LoggerStub{}
	gate := &systemFactsContractGate{logger: logger}

	assert.False(t, gate.reconcile(context.Background()))
	assert.Equal(t, systemFactsContractPending, gate.contractState())
	require.Len(t, logger.Errors, 1)
	assert.Contains(t, logger.Errors[0], "the forwarder stays stopped")

	var nilGate *systemFactsContractGate

	assert.False(t, nilGate.reconcile(context.Background()), "a nil gate releases nothing")
	assert.Equal(t, systemFactsContractPending, nilGate.contractState())
}

func TestSystemFactsContractGate_Reconcile_BoundsEveryCallByTheAttemptTimeout(t *testing.T) {
	t.Parallel()

	fake := &fakeSystemFactsReconciler{
		preflights: []reconcileAnswer{rejectedAnswer(
			reconciler.Step{Kind: reconciler.StepReadBack, Subject: reconcileContractSubject, Detail: "contract version not registered"}, nil)},
		runs: []reconcileAnswer{okAnswer(verifiedStep(reconciler.StepReadBack, reconcileContractSubject))},
	}
	gate := testContractGate(t, fake, libLog.NewNop(), newManualMetricReader(t), &waitRecorder{})

	require.True(t, gate.reconcile(context.Background()))

	deadlines, undeadlined := fake.recordedDeadlines()
	assert.Zero(t, undeadlined, "every registry call carries a deadline")
	require.Len(t, deadlines, 2, "the preflight and the run")

	for _, deadline := range deadlines {
		assert.True(t, deadline.Equal(reconcileBoot.Add(2500*time.Millisecond)),
			"each call gets a fresh attempt timeout past the clock, got %s", deadline)
	}
}

func TestSystemFactsAttemptOutcome(t *testing.T) {
	t.Parallel()

	notFound := &registry.Problem{Status: http.StatusNotFound}
	conflict := &registry.Problem{Status: http.StatusConflict, Code: "SF-0002"}

	tests := []struct {
		name   string
		report reconciler.Report
		source string
		err    error
		want   reconciler.Outcome
	}{
		{name: "ok with no error releases", report: reconciler.Report{Outcome: reconciler.OutcomeOK}, source: systemFactsSourcePreflight, want: reconciler.OutcomeOK},
		{name: "ok report with an error is never a release", report: reconciler.Report{Outcome: reconciler.OutcomeOK}, source: systemFactsSourcePreflight, err: errors.New("boom"), want: reconciler.OutcomeRejected},
		{name: "unavailable report", report: reconciler.Report{Outcome: reconciler.OutcomeUnavailable}, source: systemFactsSourcePreflight, err: reconciler.ErrUnavailable, want: reconciler.OutcomeUnavailable},
		{name: "unavailable error without a report", source: systemFactsSourceReconcile, err: fmt.Errorf("wrapped: %w", reconciler.ErrUnavailable), want: reconciler.OutcomeUnavailable},
		{name: "invalid report", report: reconciler.Report{Outcome: reconciler.OutcomeInvalid}, source: systemFactsSourcePreflight, err: reconciler.ErrInvalid, want: reconciler.OutcomeInvalid},
		{name: "rejected at preflight is the gap that leads to Run", report: rejectedAnswer(reconciler.Step{Kind: reconciler.StepReadBack, Subject: reconcileContractSubject}, notFound).report, source: systemFactsSourcePreflight, err: reconciler.ErrRejected, want: reconciler.OutcomeRejected},
		{name: "rejected at Run with a 404 problem is unavailable", report: rejectedAnswer(reconciler.Step{Kind: reconciler.StepContract, Subject: reconcileContractSubject}, notFound).report, source: systemFactsSourceReconcile, err: reconciler.ErrRejected, want: reconciler.OutcomeUnavailable},
		{name: "rejected at Run with an error matching not found is unavailable", report: reconciler.Report{Outcome: reconciler.OutcomeRejected}, source: systemFactsSourceReconcile, err: fmt.Errorf("%w: contract: %w", reconciler.ErrRejected, notFound), want: reconciler.OutcomeUnavailable},
		{name: "rejected at Run with a conflict stays rejected", report: rejectedAnswer(reconciler.Step{Kind: reconciler.StepContract, Subject: reconcileContractSubject}, conflict).report, source: systemFactsSourceReconcile, err: reconciler.ErrRejected, want: reconciler.OutcomeRejected},
		{name: "an unclassified error is a refusal", source: systemFactsSourceReconcile, err: errors.New("registry exploded"), want: reconciler.OutcomeRejected},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, systemFactsAttemptOutcome(tt.report, tt.source, tt.err))
		})
	}
}

func TestSystemFactsReconcileDelay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		attempt  int
		wantBase time.Duration
	}{
		{attempt: 1, wantBase: 5 * time.Second},
		{attempt: 2, wantBase: 10 * time.Second},
		{attempt: 3, wantBase: 20 * time.Second},
		{attempt: 6, wantBase: 160 * time.Second},
		{attempt: 7, wantBase: 5 * time.Minute},
		{attempt: 12, wantBase: 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt %d", tt.attempt), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.wantBase, systemFactsReconcileBaseDelay(tt.attempt), "the exponential is capped before the jitter")

			jittered := systemFactsReconcileDelay(tt.attempt)
			assert.GreaterOrEqual(t, jittered, time.Duration(0))
			assert.Less(t, jittered, tt.wantBase, "full jitter stays below the capped base")
		})
	}
}

func TestSystemFactsContractGate_DefaultPacing(t *testing.T) {
	t.Parallel()

	gate := &systemFactsContractGate{}
	delay, wait := gate.pacing()

	assert.Less(t, delay(1), 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.Error(t, wait(ctx, time.Hour), "the default wait ends with its context")
	assert.NoError(t, wait(context.Background(), time.Millisecond))
}
