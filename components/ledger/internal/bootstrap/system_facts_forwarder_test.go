// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/LerianStudio/lib-system-facts/reconciler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/LerianStudio/midaz/v4/tests/utils/stubs"
)

// fakeSystemFactsForwarder is the forwarder seam of the Launcher app. Each Run
// consumes the next entry of earlyReturns and returns it at once, nil included;
// once they are spent Run blocks until its context ends or Shutdown is called,
// like the kit's forwarder. beforeReturn runs before each early return, so a
// test can move the clock while a run is "in progress".
type fakeSystemFactsForwarder struct {
	earlyReturns []error
	shutdownErr  error
	beforeReturn func(run int)

	blocked  chan struct{}
	returned chan struct{}
	stop     chan struct{}

	blockOnce  sync.Once
	returnOnce sync.Once
	stopOnce   sync.Once

	mu                  sync.Mutex
	runs                int
	shutdownCalls       int
	shutdownHasDeadline bool
}

func newFakeSystemFactsForwarder(earlyReturns ...error) *fakeSystemFactsForwarder {
	return &fakeSystemFactsForwarder{
		earlyReturns: earlyReturns,
		blocked:      make(chan struct{}),
		returned:     make(chan struct{}),
		stop:         make(chan struct{}),
	}
}

func (f *fakeSystemFactsForwarder) Run(ctx context.Context) error {
	f.mu.Lock()
	run := f.runs
	f.runs++
	f.mu.Unlock()

	if run < len(f.earlyReturns) {
		if f.beforeReturn != nil {
			f.beforeReturn(run)
		}

		return f.earlyReturns[run]
	}

	f.blockOnce.Do(func() { close(f.blocked) })
	defer f.returnOnce.Do(func() { close(f.returned) })

	select {
	case <-ctx.Done():
	case <-f.stop:
	}

	return nil
}

func (f *fakeSystemFactsForwarder) Shutdown(ctx context.Context) error {
	f.stopOnce.Do(func() { close(f.stop) })

	f.mu.Lock()
	defer f.mu.Unlock()

	f.shutdownCalls++
	_, f.shutdownHasDeadline = ctx.Deadline()

	return f.shutdownErr
}

func (f *fakeSystemFactsForwarder) runCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.runs
}

func (f *fakeSystemFactsForwarder) shutdowns() (int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.shutdownCalls, f.shutdownHasDeadline
}

// waitRecorder is the backoff seam: it records every pause the runnable asked
// for and returns at once.
type waitRecorder struct {
	mu    sync.Mutex
	waits []time.Duration
}

func (w *waitRecorder) wait(ctx context.Context, d time.Duration) error {
	w.mu.Lock()
	w.waits = append(w.waits, d)
	w.mu.Unlock()

	return ctx.Err()
}

func (w *waitRecorder) recorded() []time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()

	return append([]time.Duration(nil), w.waits...)
}

// forwarderClock is a mutex-guarded clock the runnable reads and a test moves.
type forwarderClock struct {
	mu  sync.Mutex
	now time.Time
}

func newForwarderClock() *forwarderClock {
	return &forwarderClock{now: time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)}
}

func (c *forwarderClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *forwarderClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
}

// cancelWhen is the signal seam: the context ends once ch is closed, standing
// in for SIGTERM.
func cancelWhen(ch <-chan struct{}) func() (context.Context, context.CancelFunc) {
	return func() (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			<-ch
			cancel()
		}()

		return ctx, cancel
	}
}

func awaitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not happen within 5s", what)
	}
}

// newSystemFactsMetricsFactory builds a MetricsFactory over a manual reader so
// a test reads back the counters the runnable emitted.
func newSystemFactsMetricsFactory(t *testing.T) (*sdkmetric.ManualReader, *metrics.MetricsFactory) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	factory, err := metrics.NewMetricsFactory(mp.Meter("system-facts-forwarder-test"), nil)
	require.NoError(t, err)

	return reader, factory
}

// counterTotal sums the named counter across every label set.
func counterTotal(t *testing.T, reader *sdkmetric.ManualReader, name string) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	var total int64

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "counter data type must be Sum[int64], got %T", m.Data)

			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
		}
	}

	return total
}

// releasedContract is the reconcile seam of a contract already ACTIVE.
func releasedContract(context.Context) bool { return true }

// supervisedRunnable wires the runnable over the fake with every seam
// injected: the contract is released at once, the signal fires once the
// forwarder blocks, pauses are recorded instead of slept, and the clock is
// the test's.
func supervisedRunnable(forwarder *fakeSystemFactsForwarder, logger libLog.Logger, factory *metrics.MetricsFactory, clock *forwarderClock, waits *waitRecorder) *systemFactsForwarderRunnable {
	return &systemFactsForwarderRunnable{
		forwarder:      forwarder,
		reconcile:      releasedContract,
		logger:         logger,
		metricsFactory: factory,
		signalContext:  cancelWhen(forwarder.blocked),
		now:            clock.Now,
		wait:           waits.wait,
	}
}

// TestService_launcherApps_SystemFactsForwarderGuard asserts the forwarder app
// is present IFF the producer built a forwarder: the app itself holds the
// forwarder until the boot reconcile releases it.
func TestService_launcherApps_SystemFactsForwarderGuard(t *testing.T) {
	t.Parallel()

	assert.NotContains(t, launcherAppNames(&Service{}), "System Facts Forwarder",
		"no System Facts wiring must register no forwarder app")
	assert.NotContains(t, launcherAppNames(&Service{SystemFacts: &systemFactsComponents{}}), "System Facts Forwarder",
		"SYSTEM_FACTS_ENABLED=false builds no producer, so no forwarder app")

	cfg := enabledSystemFactsConfig()
	cfg.SystemFactsURL = systemFactsLoopbackURL

	components, err := buildSystemFacts(context.Background(), &cfg, libLog.NewNop(), nil, nil, offlineTransactionClient(t), nil,
		systemFactsDeps{authClient: systemFactsAuthClient()})
	require.NoError(t, err)
	require.NotNil(t, components.gate)

	assert.Contains(t, launcherAppNames(&Service{SystemFacts: components, Logger: libLog.NewNop()}), "System Facts Forwarder",
		"an enabled producer registers the forwarder app, gated on the reconcile")
}

func TestSystemFactsForwarderRunnable_Run_GatesTheForwarderOnTheContract(t *testing.T) {
	t.Parallel()

	t.Run("a released contract starts the forwarder once, after the reconcile", func(t *testing.T) {
		t.Parallel()

		forwarder := newFakeSystemFactsForwarder()
		reconciled := make(chan struct{})
		runnable := supervisedRunnable(forwarder, &stubs.LoggerStub{}, nil, newForwarderClock(), &waitRecorder{})
		runnable.reconcile = func(ctx context.Context) bool {
			assert.Equal(t, 0, forwarder.runCount(), "the forwarder does not run before the reconcile")
			assert.NoError(t, ctx.Err(), "the reconcile runs on a live context")
			close(reconciled)

			return true
		}

		require.NoError(t, runnable.Run(nil))

		awaitClosed(t, reconciled, "the reconcile")
		assert.Equal(t, 1, forwarder.runCount())
	})

	t.Run("a contract that is not released never starts the forwarder and Shutdown still runs", func(t *testing.T) {
		t.Parallel()

		forwarder := newFakeSystemFactsForwarder()
		signalled := make(chan struct{})
		logger := &stubs.LoggerStub{}
		runnable := supervisedRunnable(forwarder, logger, nil, newForwarderClock(), &waitRecorder{})
		runnable.signalContext = cancelWhen(signalled)
		runnable.reconcile = func(context.Context) bool {
			close(signalled)

			return false
		}

		require.NoError(t, runnable.Run(nil))

		assert.Equal(t, 0, forwarder.runCount(), "a refused or pending contract never starts the forwarder")

		calls, hasDeadline := forwarder.shutdowns()
		assert.Equal(t, 1, calls, "Shutdown is safe before Run and still runs")
		assert.True(t, hasDeadline)
		assert.Empty(t, logger.Errors, "the gate logs the refusal; the runnable adds nothing")
	})

	t.Run("the signal ends a reconcile still waiting", func(t *testing.T) {
		t.Parallel()

		forwarder := newFakeSystemFactsForwarder()
		signalled := make(chan struct{})
		runnable := supervisedRunnable(forwarder, &stubs.LoggerStub{}, nil, newForwarderClock(), &waitRecorder{})
		runnable.signalContext = cancelWhen(signalled)
		runnable.reconcile = func(ctx context.Context) bool {
			close(signalled)
			<-ctx.Done()

			return false
		}

		require.NoError(t, runnable.Run(nil))

		assert.Equal(t, 0, forwarder.runCount())

		calls, _ := forwarder.shutdowns()
		assert.Equal(t, 1, calls)
	})

	t.Run("no reconcile seam fails closed with one error", func(t *testing.T) {
		t.Parallel()

		forwarder := newFakeSystemFactsForwarder()
		logger := &stubs.LoggerStub{}
		runnable := supervisedRunnable(forwarder, logger, nil, newForwarderClock(), &waitRecorder{})
		runnable.reconcile = nil
		runnable.signalContext = func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			return ctx, cancel
		}

		require.NoError(t, runnable.Run(nil))

		assert.Equal(t, 0, forwarder.runCount(), "no gate means no forwarder")
		require.Len(t, logger.Errors, 1)
		assert.Contains(t, logger.Errors[0], "the forwarder stays stopped")
	})
}

// TestSystemFactsForwarderRunnable_Run_ShutdownWhileTheGateWaitsOnTheRegistry
// drives the real gate through the runnable: the registry stays unavailable,
// the signal fires during the backoff wait, and the process leaves with the
// gate pending, no Run, and a clean Shutdown.
func TestSystemFactsForwarderRunnable_Run_ShutdownWhileTheGateWaitsOnTheRegistry(t *testing.T) {
	t.Parallel()

	forwarder := newFakeSystemFactsForwarder()
	signalled := make(chan struct{})
	logger := &stubs.LoggerStub{}
	fake := &fakeSystemFactsReconciler{preflights: []reconcileAnswer{
		unavailableAnswer(reconciler.Step{Kind: reconciler.StepReferenceType, Subject: reconcileRefTypeSubject}),
	}}
	waits := &waitRecorder{}
	gate := testContractGate(t, fake, logger, newManualMetricReader(t), waits)
	gate.wait = func(ctx context.Context, d time.Duration) error {
		close(signalled)
		<-ctx.Done()

		return waits.wait(ctx, d)
	}

	runnable := supervisedRunnable(forwarder, logger, nil, newForwarderClock(), &waitRecorder{})
	runnable.signalContext = cancelWhen(signalled)
	runnable.reconcile = gate.reconcile

	require.NoError(t, runnable.Run(nil))

	assert.Equal(t, 0, forwarder.runCount(), "no Run while the contract is pending")
	assert.Equal(t, systemFactsContractPending, gate.contractState())
	assert.Equal(t, []time.Duration{time.Second}, waits.recorded(), "the wait was cut short by the signal")
	assert.Empty(t, logger.Errors, "shutdown is not a refusal")

	preflights, runs := fake.counts()
	assert.Equal(t, 1, preflights)
	assert.Equal(t, 0, runs)

	calls, _ := forwarder.shutdowns()
	assert.Equal(t, 1, calls, "the kit Shutdown is safe before Run and runs once")
}

func TestSystemFactsForwarderRunnable_Run_ShutsDownAfterTheSignal(t *testing.T) {
	t.Parallel()

	forwarder := newFakeSystemFactsForwarder()
	logger := &stubs.LoggerStub{}
	runnable := supervisedRunnable(forwarder, logger, nil, newForwarderClock(), &waitRecorder{})

	require.NoError(t, runnable.Run(nil))

	calls, hasDeadline := forwarder.shutdowns()
	assert.Equal(t, 1, calls, "Shutdown runs exactly once after the signal")
	assert.True(t, hasDeadline, "Shutdown is bounded by a deadline")
	awaitClosed(t, forwarder.returned, "the forwarder goroutine returning")

	assert.Equal(t, 1, forwarder.runCount(), "a Run ended by Shutdown is never restarted")
	assert.Empty(t, logger.Errors, "a Run ended by Shutdown is not an unexpected stop")
}

func TestSystemFactsForwarderRunnable_Run_RestartsAForwarderThatStoppedEarly(t *testing.T) {
	t.Parallel()

	forwarder := newFakeSystemFactsForwarder(
		errors.New("outbox: forwarder run: dispatcher refused"),
		nil,
		errors.New("outbox: forwarder run: pool closed"),
	)
	logger := &stubs.LoggerStub{}
	reader, factory := newSystemFactsMetricsFactory(t)
	waits := &waitRecorder{}
	runnable := supervisedRunnable(forwarder, logger, factory, newForwarderClock(), waits)

	require.NoError(t, runnable.Run(nil), "an early return is restarted, never propagated")

	assert.Equal(t, 4, forwarder.runCount(), "three early returns, then the run that blocked until the signal")
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}, waits.recorded(),
		"the restart backoff starts at 1s and doubles")

	require.Len(t, logger.Errors, 3, "every early return is logged at Error")
	assert.Contains(t, logger.Errors[0], "error=outbox: forwarder run: dispatcher refused")
	assert.Contains(t, logger.Errors[1], "stopped unexpectedly")
	assert.NotContains(t, logger.Errors[1], "error=", "a nil return carries no error field")
	assert.Contains(t, logger.Errors[2], "error=outbox: forwarder run: pool closed")

	assert.Equal(t, int64(3), counterTotal(t, reader, "system_facts_forwarder_restarts_total"))

	calls, _ := forwarder.shutdowns()
	assert.Equal(t, 1, calls)
}

func TestSystemFactsForwarderRunnable_Run_BackoffIsCappedAt30s(t *testing.T) {
	t.Parallel()

	early := make([]error, 7)
	for i := range early {
		early[i] = errors.New("outbox: forwarder run: refused")
	}

	forwarder := newFakeSystemFactsForwarder(early...)
	waits := &waitRecorder{}
	runnable := supervisedRunnable(forwarder, &stubs.LoggerStub{}, nil, newForwarderClock(), waits)

	require.NoError(t, runnable.Run(nil))

	assert.Equal(t, []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second,
	}, waits.recorded())
}

func TestSystemFactsForwarderRunnable_Run_BackoffResetsAfterAStableRun(t *testing.T) {
	t.Parallel()

	clock := newForwarderClock()
	forwarder := newFakeSystemFactsForwarder(
		errors.New("outbox: forwarder run: refused"),
		errors.New("outbox: forwarder run: refused"),
		errors.New("outbox: forwarder run: refused"),
		errors.New("outbox: forwarder run: refused"),
	)
	// The third run lasts longer than a minute before it stops.
	forwarder.beforeReturn = func(run int) {
		if run == 2 {
			clock.Advance(time.Minute + time.Second)
		}
	}
	waits := &waitRecorder{}
	runnable := supervisedRunnable(forwarder, &stubs.LoggerStub{}, nil, clock, waits)

	require.NoError(t, runnable.Run(nil))

	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second, time.Second, 2 * time.Second}, waits.recorded(),
		"a run that lasted longer than a minute resets the backoff")
}

func TestSystemFactsForwarderRunnable_Run_StopsRestartingOnceTheSignalFires(t *testing.T) {
	t.Parallel()

	// Every Run returns at once; the signal fires on the first return instead
	// of on a blocking run, so the supervisor must exit without another Run.
	forwarder := newFakeSystemFactsForwarder(errors.New("outbox: forwarder run: refused"))
	signalled := make(chan struct{})
	waits := &waitRecorder{}
	logger := &stubs.LoggerStub{}
	runnable := &systemFactsForwarderRunnable{
		forwarder:     forwarder,
		reconcile:     releasedContract,
		logger:        logger,
		signalContext: cancelWhen(signalled),
		now:           newForwarderClock().Now,
		wait: func(ctx context.Context, d time.Duration) error {
			close(signalled)
			<-ctx.Done()

			return waits.wait(ctx, d)
		},
	}

	require.NoError(t, runnable.Run(nil))

	assert.Equal(t, []time.Duration{time.Second}, waits.recorded(), "the pause is cut short by the signal")
	assert.Equal(t, 1, forwarder.runCount(), "no restart is attempted after the signal")
}

func TestSystemFactsForwarderRunnable_Run_LogsAShutdownErrorAtWarn(t *testing.T) {
	t.Parallel()

	forwarder := newFakeSystemFactsForwarder()
	forwarder.shutdownErr = errors.New("outbox: forwarder shutdown: wait for run: context deadline exceeded")
	logger := &stubs.LoggerStub{}
	runnable := supervisedRunnable(forwarder, logger, nil, newForwarderClock(), &waitRecorder{})

	require.NoError(t, runnable.Run(nil))

	require.Len(t, logger.Warnings, 1, "a Shutdown error is logged at Warn")
	assert.Contains(t, logger.Warnings[0], "error=outbox: forwarder shutdown")
}

func TestNewSystemFactsForwarderRunnable_DefaultsTheSeams(t *testing.T) {
	t.Parallel()

	runnable := newSystemFactsForwarderRunnable(nil, releasedContract, nil, nil)

	require.NotNil(t, runnable.logger, "a nil logger becomes a no-op logger")
	require.NotNil(t, runnable.reconcile)
	require.NotNil(t, runnable.now)
	require.NotNil(t, runnable.wait)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.ErrorIs(t, runnable.wait(ctx, time.Hour), context.Canceled, "the default pause ends with its context")
	assert.NoError(t, runnable.wait(context.Background(), time.Millisecond), "the default pause ends after its duration")
}
