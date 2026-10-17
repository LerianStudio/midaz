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

// Stage bounds every cell shares; each phase adds its own abort of window + caller budget + two answer graces.
const cellSetup, cellClose, cellChecker = time.Minute, 30 * time.Second, 5 * time.Minute

// TestBenchHotAccount runs the cell BENCH_MATRIX selects and skips without one. An invalid measurement or an unexpected
// outcome fails it; a rate below the target does not. Final PASS, including cleanup, validates a printed rate, and only
// then does BENCH_REPORT receive the cell's JSON.
func TestBenchHotAccount(t *testing.T) {
	c, err := selectCell(os.Getenv)
	if errors.Is(err, errNoCell) {
		t.Skip(err)
	}

	require.NoError(t, err, "selection, before any database work")

	rep := &cellReport{Cell: c, Unmeasured: unmeasured}
	if path := os.Getenv("BENCH_REPORT"); path != "" {
		reportOnPass(t, path, rep) // the first cleanup registered runs last
	}

	noLeaks(t)

	env, ctx := requireDB(t), t.Context()
	warm := newTestWorkload(t, c.Label+"/warmup", c.mix, false, c.Ordinary)
	meas := newTestWorkload(t, c.Label+"/measured", c.mix, false, c.Ordinary)

	last := warm.now()
	mark := func(name string) {
		at := warm.now()
		rep.Stages, last = append(rep.Stages, stage{name, at.Sub(last)}), at
	}

	// Deferred, so a stage that fails the test, the checker included, keeps its attribution.
	defer func() {
		t.Logf("stages%s; bounds: setup %s where calls take a context (migrations after Up only the process), run abort window+%s+2*%s, close %s, boundary read %s, probe %s/%s, toxiproxy SDK 30s per call, standby wait %s, checker %s",
			rep.stagesLine(), cellSetup, usualBudget.caller, answerGrace, cellClose, snapshotTimeout, probeOuterTimeout, probeInnerTimeout,
			replicationWait, cellChecker)
	}()

	conn, broker, inner := startCell(ctx, t, env, c, warm.now, mark, rep)
	w := captureCalls(inner, int(c.WarmupCap+c.MeasuredCap))
	warmRec, measRec := newRecorder(c.WarmupCap), newRecorder(c.MeasuredCap)

	mark("allocate")

	warmErr := runPhase(ctx, w, warm, c.Producers, c.WarmupCap, c.Warmup, warmRec)
	rep.WarmupJoin.Unknown, rep.WarmupJoin.Queued = warmRec.outcomes[outcomeUnknown], len(inner.queue)

	mark("warmup")

	measErr := errors.New("skipped: the warm-up is invalid")
	if warmErr == nil {
		rep.S1ToS2, measErr = measure(ctx, t, conn, w, c, meas, measRec, mark)
	}

	closing, endClose := context.WithTimeout(ctx, cellClose)
	closeErr := w.Close(closing)

	endClose()
	mark("close")
	rep.observe(inner, broker, warmRec, measRec)

	t.Logf("warm-up, excluded from measurement: %s", rep.Warmup.Summary)
	t.Logf("warm-up join: unknown=%d bounds the attempts carried over, queued=%d, in flight unmeasured", rep.WarmupJoin.Unknown, rep.WarmupJoin.Queued)
	t.Logf("measured: %s", rep.Measured.Summary)
	mark("summaries")
	t.Logf("writer lifetime, warm-up included: passes=%d deadlocks=%d entryLate=%d publishes=%d close=%v; measured window took %+v; capture and in-process generator on, effect on rate/CPU/GC unmeasured",
		rep.Writer.Passes, rep.Writer.Deadlocks, rep.Writer.EntryLate, rep.Writer.Publishes, closeErr, rep.Occupancy)
	assert.Zero(t, rep.Writer.Publishes, "obligations stay in the outbox")
	assert.NoError(t, warmErr, "warm-up")
	assert.NoError(t, measErr, "measured run and its boundaries")
	require.NoError(t, closeErr, "only a drained writer is verified: an aborted backend may still commit")

	rep.Answers, rep.ProbeAfter = verifyCell(ctx, t, env, conn, c, w.calls, mark)
	t.Logf("unmeasured: %s", strings.Join(unmeasured, ", "))

	if !t.Failed() {
		t.Logf("window rate %s approvals/s with dbAck before stop over the %s measured window; below 8k or p99 above 15 ms is a result, not a failure",
			perSecond(int(rep.Measured.InWindow), rep.Measured.Window), rep.Measured.Window)
	}
}

// startCell rebuilds and seeds the counterparts, records settings and the baseline, sets the delay and probes it idle,
// then starts the writer on ctx, its lifetime. One setup bound covers the calls that take a context and the owner wait.
func startCell(ctx context.Context, t *testing.T, env *benchEnv, c cell, now func() time.Time, mark func(string), rep *cellReport) (*pgx.Conn, *refusingBroker, *ownerlessWriter) {
	t.Helper()

	setup, endSetup := context.WithTimeout(ctx, cellSetup)
	defer endSetup()

	require.NoError(t, bootstrapSchema(setup, env))
	require.NoError(t, seedFixtures(setup, env, c.Ordinary))

	app := "bench-" + c.Name
	_, emitter, repo, broker := newTestOutbox(setup, t, env) // Build validates locally and keeps no context
	b, err := newBatcher(namedSession(t, env, app+"-0", nil), emitter, repo, now)
	require.NoError(t, err)

	conn := connectPrimary(setup, t, env)
	rep.PGSettings, err = pgSettings(setup, conn)
	require.NoError(t, err)
	t.Logf("settings: %s", rep.PGSettings)
	snapshotBaseline(setup, t, conn)
	mark("setup")

	// Registered before the first change and before the writer, so a failed test closes the writer first.
	t.Cleanup(func() {
		assert.NoError(t, setReplLatency(env, 0), "pg-repl cleanup; after an error its toxics are unknown")
		http.DefaultClient.CloseIdleConnections() // the SDK's keep-alive connections would outlive noLeaks
	})
	require.NoError(t, setReplLatency(env, c.DelayMS))
	rep.ProbeIdle, err = logProbe(ctx, t, "idle before warm-up", c.DelayMS)
	require.NoError(t, err)
	mark("delay+probe")
	require.NoError(t, setup.Err(), "setup bound after delay and probe")

	if c.Writer == "ownerless" {
		inner := newTestOwnerless(ctx, t, env, b, app, c.Workers, c.MaxBatch)

		mark("ownerless")

		return conn, broker, inner
	}

	start := startOwner(ctx, t, b, app, c.MaxBatch)
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

// runPhase runs one closed-loop phase of producers under its own abort: the window plus one caller budget and two
// answer graces. The window's stop cancels nothing. A run error, an abort or admission ended by the count rather than
// the window makes the phase invalid.
func runPhase(ctx context.Context, w writer, wl *workload, producers int, n int64, window time.Duration, rec *recorder) error {
	guard, cancel := context.WithTimeout(ctx, window+usualBudget.caller+2*answerGrace)
	defer cancel()

	err := runClosed(guard, w, wl, producers, n, window, usualBudget, rec)
	if int64(len(rec.lag)) == n {
		err = errors.Join(err, fmt.Errorf("all %d requests admitted: the count, not the window, ended admission", n))
	}

	return err
}

// measure reads S1, runs the measured phase and reads S2 on the verification connection; a failed S1 skips the run.
// It returns the S1-to-S2 line, empty without both boundaries.
func measure(ctx context.Context, t *testing.T, conn *pgx.Conn, w writer, c cell, wl *workload, rec *recorder, mark func(string)) (string, error) {
	t.Helper()

	s1, err := readBoundary(ctx, conn)
	if err != nil {
		return "", errors.Join(errors.New("skipped: no boundary before the measured run"), err)
	}

	runErr := runPhase(ctx, w, wl, c.Producers, c.MeasuredCap, c.Measured, rec)
	s2, err := readBoundary(ctx, conn)

	mark("S1+measured+S2")

	if err != nil {
		return "", errors.Join(runErr, err)
	}

	line, err := s1.until(s2)
	t.Logf("S1-to-S2: drain/background WAL included; PG stats lazily flushed; flushLag is one post-drain sample: %s", line)

	return line, errors.Join(runErr, err)
}

// verifyCell probes the delay after Close, removes it and awaits standby1 streaming sync, then checks the ledger over
// every captured call on the baseline connection. Expired and unknown answers are reported; any other non-approval
// fails. It returns the checker's tally and the probe.
func verifyCell(ctx context.Context, t *testing.T, env *benchEnv, conn *pgx.Conn, c cell, calls []call, mark func(string)) (map[string]int, probe) {
	t.Helper()

	after, err := logProbe(ctx, t, "after Close, delay still set", c.DelayMS)
	assert.NoError(t, err)
	assert.NoError(t, setReplLatency(env, 0), "remove the delay before the checker")

	if stack, err := openVerifiedStack(ctx, env); assert.NoError(t, err, "standby1 streaming sync after removal") {
		stack.close(ctx)
	}

	mark("probe+removal")

	check, endCheck := context.WithTimeout(ctx, cellChecker)
	defer endCheck()
	defer mark("checker") // also when a check fails the test

	got := verifyLedger(check, t, conn, calls)
	t.Logf("answers over both phases, %s unwindowed: %v", unansweredApproval, got)

	for key, n := range got {
		if !slices.Contains([]string{string(outcomeApproved), string(outcomeExpired), string(outcomeUnknown), unansweredApproval}, key) {
			t.Errorf("%d answers %s: new identities of funded, unblocked postings never replay, conflict or refuse", n, key)
		}
	}

	return got, after
}

// logProbe logs sql_roundtrip_pg_repl with no workload running: SQL round trips across the delay, not network RTT.
func logProbe(ctx context.Context, t *testing.T, when string, delayMS int) (probe, error) {
	t.Helper()

	samples, err := probeReplRoundtrip(ctx)
	if err != nil {
		return probe{}, err
	}

	slices.Sort(samples)
	p := probe{N: len(samples), Min: samples[0], MedianLower: samples[(len(samples)-1)/2], Max: samples[len(samples)-1]}
	t.Logf("sql_roundtrip_pg_repl %s, %d ms downstream configured: n=%d min=%s median(lower)=%s max=%s",
		when, delayMS, p.N, p.Min, p.MedianLower, p.Max)

	return p, nil
}

// Each answer steps the clock 1 ms, so 256 producers send at least 1000 and fewer than 1256 in a 1 s
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

			err := runPhase(ctx, w, wl, 256, tc.n, time.Second, rec)
			if tc.invalid == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.invalid)
			}
		})
	}
}
