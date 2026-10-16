//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// The quick cell: one owner, 10k ordinary counterparts, outflow without fee, 256 closed-loop producers, batch cap 256
// and 1 ms downstream on pg-repl. 60 s of warm-up, then 20 s measured, share one clock, lease, capture and Close.
const (
	quickOrdinary, quickProducers, quickMaxBatch, quickLatencyMS = 10_000, 256, 256, 1
	quickWarmup, quickMeasured                                   = 60 * time.Second, 20 * time.Second
	quickSetup, quickClose, quickChecker                         = time.Minute, 30 * time.Second, 5 * time.Minute
	// Memory ceilings at 16k attempts/s, twice the 8k target, never a capacity: reaching one invalidates the phase.
	quickWarmupCap, quickMeasuredCap = 960_000, 320_000
)

// TestBenchHotAccount runs the cell BENCH_MATRIX names and skips without one. An invalid measurement or an unexpected
// outcome fails it; a rate below the target does not. Final PASS, including cleanup, validates a printed rate.
func TestBenchHotAccount(t *testing.T) {
	switch matrix := os.Getenv("BENCH_MATRIX"); matrix {
	case "":
		t.Skip("set BENCH_MATRIX=quick to run the first owner cell")
	case "quick":
	default:
		t.Fatalf("BENCH_MATRIX=%q is not implemented: only quick exists", matrix)
	}

	noLeaks(t)

	env, ctx := requireDB(t), t.Context()
	warm := newTestWorkload(t, "quick/warmup", mixOutflow, false, quickOrdinary)
	meas := newTestWorkload(t, "quick/measured", mixOutflow, false, quickOrdinary)

	var stages []string

	last := warm.now()
	mark := func(stage string) {
		at := warm.now()
		stages, last = append(stages, stage+"="+at.Sub(last).String()), at
	}

	conn, broker, inner := startQuick(ctx, t, env, warm.now, mark)
	w := captureCalls(inner, quickWarmupCap+quickMeasuredCap)
	warmRec, measRec := newRecorder(quickWarmupCap), newRecorder(quickMeasuredCap)

	mark("allocate")

	warmErr := runPhase(ctx, w, warm, quickWarmupCap, quickWarmup, warmRec)
	carried, queued := warmRec.outcomes[outcomeUnknown], len(inner.queue)

	mark("warmup")

	measErr := errors.New("skipped: the warm-up is invalid")
	if warmErr == nil {
		measErr = measure(ctx, t, conn, w, meas, measRec, mark)
	}

	closing, endClose := context.WithTimeout(ctx, quickClose)
	closeErr := w.Close(closing)

	endClose()
	mark("close")

	var passes, deadlocks int
	for _, s := range inner.workers {
		passes, deadlocks = passes+s.passes, deadlocks+s.deadlocks
	}

	t.Logf("warm-up, excluded from measurement: %s", warmRec.summary())
	t.Logf("warm-up join: unknown=%d bounds the attempts carried over, queued=%d, in flight unmeasured", carried, queued)
	t.Logf("measured: %s", measRec.summary())
	mark("summaries")
	t.Logf("writer lifetime, warm-up included: passes=%d deadlocks=%d entryLate=%d publishes=%d close=%v; measured window took %+v; capture and in-process generator on, effect on rate/CPU/GC unmeasured",
		passes, deadlocks, inner.entryLate.Load(), broker.publishes.Load(), closeErr, inner.occupancy(measRec.started, measRec.stop))
	assert.Zero(t, broker.publishes.Load(), "obligations stay in the outbox")
	assert.NoError(t, warmErr, "warm-up")
	assert.NoError(t, measErr, "measured run and its boundaries")
	require.NoError(t, closeErr, "only a drained writer is verified: an aborted backend may still commit")

	verifyQuick(ctx, t, env, conn, w.calls, mark)
	t.Logf("stages %s; bounds: setup %s where calls take a context (migrations after Up only the process), run abort window+%s+2*%s, close %s, boundary read %s, probe %s/%s, toxiproxy SDK 30s per call, standby wait %s, checker %s",
		strings.Join(stages, " "), quickSetup, usualBudget.caller, answerGrace, quickClose, snapshotTimeout, probeOuterTimeout, probeInnerTimeout,
		replicationWait, quickChecker)
	t.Log("unmeasured: Postgres CPU and memory, cumulative lock wait, standby fsync, network RTT, in-run sampling, capture cost")

	if !t.Failed() {
		window := measRec.stop.Sub(measRec.started)
		t.Logf("window rate %s approvals/s with dbAck before stop over the %s measured window; below 8k or p99 above 15 ms is a result, not a failure",
			perSecond(int(measRec.inWindow), window), window)
	}
}

// startQuick rebuilds and seeds 10k counterparts, records settings and the baseline, sets the delay and probes it idle,
// then starts the owner on ctx, its lifetime. One setup bound covers the calls that take a context and the owner wait.
func startQuick(ctx context.Context, t *testing.T, env *benchEnv, now func() time.Time, mark func(string)) (*pgx.Conn, *refusingBroker, *ownerlessWriter) {
	t.Helper()

	setup, endSetup := context.WithTimeout(ctx, quickSetup)
	defer endSetup()

	require.NoError(t, bootstrapSchema(setup, env))
	require.NoError(t, seedFixtures(setup, env, quickOrdinary))

	_, emitter, repo, broker := newTestOutbox(setup, t, env) // Build validates locally and keeps no context
	b, err := newBatcher(namedSession(t, env, "bench-quick-0", nil), emitter, repo, now)
	require.NoError(t, err)

	conn := connectPrimary(setup, t, env)
	settings, err := pgSettings(setup, conn)
	require.NoError(t, err)
	t.Logf("settings: %s", settings)
	snapshotBaseline(setup, t, conn)
	mark("setup")

	// Registered before the first change and before the owner, so a failed test closes the owner first.
	t.Cleanup(func() {
		assert.NoError(t, setReplLatency(env, 0), "pg-repl cleanup; after an error its toxics are unknown")
		http.DefaultClient.CloseIdleConnections() // the SDK's keep-alive connections would outlive noLeaks
	})
	require.NoError(t, setReplLatency(env, quickLatencyMS))
	require.NoError(t, logProbe(ctx, t, "idle before warm-up"))
	mark("delay+probe")

	start := startOwner(ctx, t, b, "bench-quick", quickMaxBatch)
	select {
	case <-start.done:
	case <-setup.Done():
		t.Fatalf("setup bound elapsed while awaiting owner; delay and probe used the same clock: %v", context.Cause(setup))
	}

	inner, err := start.wait(t)
	require.NoError(t, err)
	mark("owner")

	return conn, broker, inner
}

// runPhase runs one closed-loop phase of the cell's producers under its own abort: the window plus one caller budget
// and two answer graces. The window's stop cancels nothing. A run error, an abort or admission ended by the count
// rather than the window makes the phase invalid.
func runPhase(ctx context.Context, w writer, wl *workload, n int64, window time.Duration, rec *recorder) error {
	guard, cancel := context.WithTimeout(ctx, window+usualBudget.caller+2*answerGrace)
	defer cancel()

	err := runClosed(guard, w, wl, quickProducers, n, window, usualBudget, rec)
	if int64(len(rec.lag)) == n {
		err = errors.Join(err, fmt.Errorf("all %d requests admitted: the count, not the window, ended admission", n))
	}

	return err
}

// measure reads S1, runs the measured phase and reads S2 on the verification connection; a failed S1 skips the run.
func measure(ctx context.Context, t *testing.T, conn *pgx.Conn, w writer, wl *workload, rec *recorder, mark func(string)) error {
	t.Helper()

	s1, err := readBoundary(ctx, conn)
	if err != nil {
		return errors.Join(errors.New("skipped: no boundary before the measured run"), err)
	}

	runErr := runPhase(ctx, w, wl, quickMeasuredCap, quickMeasured, rec)
	s2, err := readBoundary(ctx, conn)

	mark("S1+measured+S2")

	if err != nil {
		return errors.Join(runErr, err)
	}

	line, err := s1.until(s2)
	t.Logf("S1-to-S2: drain/background WAL included; PG stats lazily flushed; flushLag is one post-drain sample: %s", line)

	return errors.Join(runErr, err)
}

// verifyQuick probes the delay after Close, removes it and awaits standby1 streaming sync, then checks the ledger over
// every captured call on the baseline connection. Expired and unknown answers are reported; any other non-approval fails.
func verifyQuick(ctx context.Context, t *testing.T, env *benchEnv, conn *pgx.Conn, calls []call, mark func(string)) {
	t.Helper()

	assert.NoError(t, logProbe(ctx, t, "after Close, delay still set"))
	assert.NoError(t, setReplLatency(env, 0), "remove the delay before the checker")

	if stack, err := openVerifiedStack(ctx, env); assert.NoError(t, err, "standby1 streaming sync after removal") {
		stack.close(ctx)
	}

	mark("probe+removal")

	check, endCheck := context.WithTimeout(ctx, quickChecker)
	defer endCheck()

	got := verifyLedger(check, t, conn, calls)

	mark("checker")
	t.Logf("answers over both phases, %s unwindowed: %v", unansweredApproval, got)

	for key, n := range got {
		if !slices.Contains([]string{string(outcomeApproved), string(outcomeExpired), string(outcomeUnknown), unansweredApproval}, key) {
			t.Errorf("%d answers %s: new identities of funded, unblocked outflow never replay, conflict or refuse", n, key)
		}
	}
}

// logProbe logs sql_roundtrip_pg_repl with no workload running: SQL round trips across the delay, not network RTT.
func logProbe(ctx context.Context, t *testing.T, when string) error {
	t.Helper()

	samples, err := probeReplRoundtrip(ctx)
	if err != nil {
		return err
	}

	slices.Sort(samples)
	t.Logf("sql_roundtrip_pg_repl %s, %d ms downstream configured: n=%d min=%s median(lower)=%s max=%s",
		when, quickLatencyMS, len(samples), samples[0], samples[(len(samples)-1)/2], samples[len(samples)-1])

	return nil
}

// Each answer steps the clock 1 ms, so the cell's 256 producers send at least 1000 and fewer than 1256 in a 1 s
// window. Only a phase the window ended, on a live context, is valid.
func TestRunPhase(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	for name, tc := range map[string]struct {
		n       int64
		abort   bool
		invalid string
	}{
		"window ends": {n: 2000},
		"count ends":  {n: 500, invalid: "the count, not the window"},
		"aborted":     {n: 2000, abort: true, invalid: context.Canceled.Error()},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			if tc.abort {
				cancel()
			}

			clock := newTestClock(scheduleStart)
			w, rec, wl := newNullWriter(ctx, t, int(tc.n)), newRecorder(tc.n), newTestWorkload(t, "phase", mixOutflow, false, 10)
			w.clock, w.step, wl.now = clock, time.Millisecond, clock.now

			err := runPhase(ctx, w, wl, tc.n, time.Second, rec)
			if tc.invalid == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.invalid)
			}
		})
	}
}
