//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

var scheduleStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newTestWorkload(t *testing.T, run string, m mix, feeLeg bool, ordinary int) *workload {
	t.Helper()

	wl, err := newWorkload(run, m, feeLeg, ordinary)
	require.NoError(t, err)

	return wl
}

func testRequest(t *testing.T, wl *workload, i int64, scheduledAt time.Time, b budget) request {
	t.Helper()

	req, err := wl.request(i, scheduledAt, b)
	require.NoError(t, err)

	return req
}

// Fixed instants: the absolute schedule, the indices due after a late wake and the admission cutoff boundary.
func TestOpenLoopSchedule(t *testing.T) {
	for _, tc := range []struct {
		i, rate int64
		want    time.Duration
	}{
		{0, 8000, 0},
		{1, 8000, 125 * time.Microsecond},
		{8000, 8000, time.Second},
		{14_400_000, 8000, 1800 * time.Second},
		{1, 3, 333_333_333},
		{3, 3, time.Second},
	} {
		assert.Equal(t, scheduleStart.Add(tc.want), scheduleOf(scheduleStart, tc.i, tc.rate), "index %d at %d/s", tc.i, tc.rate)
	}

	for _, tc := range []struct {
		next, n, rate int64
		at            time.Duration
		want          int64
	}{
		{0, 100, 8000, -1, 0},
		{0, 100, 8000, 0, 1},
		{0, 100, 8000, 124_999, 1},
		{0, 100, 8000, 125 * time.Microsecond, 2},
		{0, 100, 8000, time.Millisecond, 9},
		{9, 100, 8000, time.Millisecond, 9},
		{0, 5, 8000, time.Millisecond, 5},
		{0, 100, 3, 333_333_333, 2},
	} {
		assert.Equal(t, tc.want, due(scheduleStart, tc.rate, tc.next, tc.n, scheduleStart.Add(tc.at)),
			"from %d of %d at %d/s, +%s", tc.next, tc.n, tc.rate, tc.at)
	}

	req := testRequest(t, newTestWorkload(t, "schedule", mixOutflow, false, 10), 0, scheduleStart, backlogBudget)
	assert.Equal(t, [2]time.Time{scheduleStart.Add(1500 * time.Millisecond), scheduleStart.Add(2 * time.Second)},
		[2]time.Time{req.admissionCutoff, req.deadline})
	assert.True(t, req.admissible(req.admissionCutoff.Add(-time.Nanosecond)))
	assert.False(t, req.admissible(req.admissionCutoff), "at the cutoff instant the request is already late")
}

func TestPercentiles(t *testing.T) {
	_, measured := quantiles(nil)
	assert.False(t, measured, "an empty series is unmeasured, never zero")

	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	upTo := func(n int) []time.Duration {
		series := make([]time.Duration, n)
		for i := range series {
			series[i] = ms(i + 1)
		}

		return series
	}
	shuffled := upTo(100)
	rand.New(rand.NewPCG(1, 2)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	before := slices.Clone(shuffled)

	for _, tc := range []struct {
		name   string
		series []time.Duration
		want   [4]time.Duration // p50, p99, p99.9, max
	}{
		{"one", []time.Duration{ms(7)}, [4]time.Duration{ms(7), ms(7), ms(7), ms(7)}},
		{"1..100 shuffled", shuffled, [4]time.Duration{ms(50), ms(99), ms(100), ms(100)}},
		{"1..1000", upTo(1000), [4]time.Duration{ms(500), ms(990), ms(999), ms(1000)}},
		{"ties", []time.Duration{ms(5), ms(5), ms(5), ms(9)}, [4]time.Duration{ms(5), ms(9), ms(9), ms(9)}},
	} {
		got, ok := quantiles(tc.series)
		require.True(t, ok, tc.name)
		assert.Equal(t, tc.want, got, tc.name)
	}

	assert.Equal(t, before, shuffled, "quantiles sorts its own copy")
}

// postingShape checks one generated request and reports whether it debits the hot balance.
func postingShape(req request, feeLeg bool, accounts map[uuid.UUID]uuid.UUID) (hotDebit bool, err error) {
	hot, fee := fixturePosting(classHot, 0).BalanceID, fixturePosting(classFee, 0).BalanceID
	legs, hots := 2, 0

	if feeLeg {
		legs = 3
	}

	for _, p := range req.postings {
		switch account, seeded := accounts[p.BalanceID]; {
		case !seeded || account != p.AccountID:
			return false, fmt.Errorf("posting %s/%s is not a seeded balance and its account", p.BalanceID, p.AccountID)
		case p.Amount < -100 || p.Amount > 100:
			return false, fmt.Errorf("amount %d outside ±100", p.Amount)
		case p.BalanceID == hot:
			hots, hotDebit = hots+1, p.Amount < 0
		case p.BalanceID == fee && p.Amount != 1:
			return false, fmt.Errorf("fee leg %d, want a credit of 1", p.Amount)
		}
	}

	if len(req.postings) != legs || hots != 1 {
		return false, fmt.Errorf("%d legs (want %d), %d hot legs", len(req.postings), legs, hots)
	}

	return hotDebit, nil
}

// Identities never repeat across runs or kinds, regeneration repeats a run's draws, every request conserves money
// in its direction, and the fingerprint covers exactly the transaction and its postings.
func TestWorkloadKeysUnique(t *testing.T) {
	const ordinary = 1000

	accounts := make(map[uuid.UUID]uuid.UUID, ordinary+2)
	for _, p := range []posting{fixturePosting(classHot, 0), fixturePosting(classFee, 0)} {
		accounts[p.BalanceID] = p.AccountID
	}

	for j := range ordinary {
		p := fixturePosting(classOrdinary, j)
		accounts[p.BalanceID] = p.AccountID
	}

	seen := make(map[uuid.UUID]string, 230_000)

	for _, tc := range []struct {
		run    string
		mix    mix
		feeLeg bool
		n      int64
	}{
		{"measure", mixOutflow, false, 200_000},
		{"warmup", mixOutflow, false, 10_000},
		{"inflow", mixInflow, false, 10_000},
		{"mixed-fee", mixMixed, true, 10_000},
	} {
		wl, again := newTestWorkload(t, tc.run, tc.mix, tc.feeLeg, ordinary), newTestWorkload(t, tc.run, tc.mix, tc.feeLeg, ordinary)

		var hotDebits int64

		for i := range tc.n {
			req := testRequest(t, wl, i, scheduleStart, usualBudget)
			if where, dup := seen[req.id]; dup {
				t.Fatalf("%s/%d repeats the request ID of %s", tc.run, i, where)
			}

			seen[req.id] = fmt.Sprintf("%s/%d", tc.run, i)

			hotDebit, err := postingShape(req, tc.feeLeg, accounts)
			if err != nil {
				t.Fatalf("%s/%d: %v", tc.run, i, err)
			}

			if hotDebit {
				hotDebits++
			}

			if i < 1000 {
				other := testRequest(t, again, i, scheduleStart, usualBudget)
				require.Equal(t, [2]any{req.id, req.postings}, [2]any{other.id, other.postings}, "%s/%d regenerates alike", tc.run, i)
				require.NotEqual(t, req.transactionID, other.transactionID, "building again conflicts under one request ID, never a retry")
			}
		}

		switch tc.mix {
		case mixOutflow:
			assert.Equal(t, tc.n, hotDebits, "%s debits hot every time", tc.run)
		case mixInflow:
			assert.Zero(t, hotDebits, "%s credits hot every time", tc.run)
		case mixMixed:
			assert.True(t, hotDebits > 0 && hotDebits < tc.n, "%s holds both directions: %d of %d debit hot", tc.run, hotDebits, tc.n)
		}
	}

	t.Run("fingerprint and invariants", testRequestBoundary)
}

// testRequestBoundary covers newRequest: frozen postings, the fields the fingerprint covers and each rejection.
func testRequestBoundary(t *testing.T) {
	id, tx, other := uuid.NewSHA1(benchNS, []byte("fp/id")), uuid.NewSHA1(benchNS, []byte("fp/tx")), uuid.NewSHA1(benchNS, []byte("fp/other"))
	hot, counter := fixturePosting(classHot, 0), fixturePosting(classOrdinary, 0)
	legs := func(debit, credit int64, credited posting) []posting {
		hot.Amount, credited.Amount = debit, credit

		return []posting{hot, credited}
	}
	build := func(rid, txID uuid.UUID, postings []posting, at time.Time, b budget) request {
		req, err := newRequest(rid, txID, postings, at, b)
		require.NoError(t, err)

		return req
	}

	input := legs(-5, 5, counter)
	base := build(id, tx, input, scheduleStart, usualBudget)
	input[0].Amount = -7
	assert.Equal(t, int64(-5), base.postings[0].Amount, "postings are frozen at the boundary")

	same := build(other, tx, base.postings, scheduleStart.Add(time.Hour), backlogBudget)
	assert.Equal(t, base.fingerprint, same.fingerprint, "request ID, schedule and budget stay outside the fingerprint")

	for name, changed := range map[string]request{
		"transaction": build(id, other, base.postings, scheduleStart, usualBudget),
		"amount":      build(id, tx, legs(-6, 6, counter), scheduleStart, usualBudget),
		"balance":     build(id, tx, legs(-5, 5, fixturePosting(classOrdinary, 1)), scheduleStart, usualBudget),
	} {
		assert.NotEqual(t, base.fingerprint, changed.fingerprint, "the fingerprint covers the %s", name)
	}

	for name, tc := range map[string]struct {
		id, tx   uuid.UUID
		postings []posting
	}{
		"nil request": {uuid.Nil, tx, base.postings}, "nil transaction": {id, uuid.Nil, base.postings},
		"shared ID": {id, id, base.postings}, "one leg": {id, tx, base.postings[:1]}, "zero amount": {id, tx, legs(0, 0, counter)},
		"unkeyed": {id, tx, legs(-5, 5, posting{})}, "unbalanced": {id, tx, legs(-5, 4, counter)},
	} {
		_, err := newRequest(tc.id, tc.tx, tc.postings, scheduleStart, usualBudget)
		assert.Error(t, err, name)
	}
}

// nullWriter approves at once without a database: it measures the generator, never a writer. It keeps each
// identity's instants and, when cancelAt is set, cancels the run on its cancelAt-th distinct identity.
type nullWriter struct {
	mu       sync.Mutex
	seen     map[uuid.UUID][3]time.Time // scheduledAt, admission cutoff, deadline
	cancelAt int
	cancel   context.CancelFunc
}

// newNullWriter sizes its map for n so the capture never grows it mid-run, and closes on every test path.
func newNullWriter(ctx context.Context, t *testing.T, n int) *nullWriter {
	t.Helper()

	w := &nullWriter{seen: make(map[uuid.UUID][3]time.Time, n)}

	t.Cleanup(func() { assert.NoError(t, w.Close(ctx)) })

	return w
}

func (w *nullWriter) Submit(_ context.Context, req request) result {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.seen[req.id] = [3]time.Time{req.scheduledAt, req.admissionCutoff, req.deadline}
	if len(w.seen) == w.cancelAt {
		w.cancel()
	}

	return result{requestID: req.id, outcome: outcomeApproved, transactionID: req.transactionID}
}

func (*nullWriter) Close(context.Context) error { return nil }

// assertNullRun: n answers from n distinct identities at the writer means each was submitted exactly once.
func assertNullRun(t *testing.T, w *nullWriter, rec *recorder, n int) {
	t.Helper()

	assert.Equal(t, [4]int{n, n, n, n}, [4]int{int(rec.scheduled), len(rec.lag), len(rec.latency), len(w.seen)},
		"scheduled, dispatched, answered, distinct identities at the writer")
	assert.Equal(t, map[outcome]int64{outcomeApproved: int64(n)}, rec.outcomes)
	assert.Zero(t, len(rec.dbAck)+len(rec.brokerAck), "simulated approvals carry no ack: unmeasured, not zero")
}

// assertSchedule checks at the writer that index i of the dispatched prefix kept start + i*step and the usual
// budget's cutoff and deadline from that instant, however late it was sent.
func assertSchedule(t *testing.T, w *nullWriter, wl *workload, start time.Time, step time.Duration) {
	t.Helper()

	for i := range len(w.seen) {
		at := start.Add(time.Duration(i) * step)
		got, sent := w.seen[testRequest(t, wl, int64(i), at, usualBudget).id]
		require.True(t, sent, "index %d of a %d-request dispatched prefix never reached the writer", i, len(w.seen))
		require.Equal(t, [3]time.Time{at, at.Add(28 * time.Second), at.Add(30 * time.Second)}, got, "index %d", i)
	}
}

// Real-clock runs against a writer that does no I/O: throughput here is generator capacity only.
func TestWorkloadNullWriter(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	t.Run("open 20k/s", func(t *testing.T) {
		const n = 40_000

		w, rec, wl := newNullWriter(ctx, t, n), newRecorder(), newTestWorkload(t, "null-open", mixMixed, true, 1000)
		start := wl.now()
		err := runOpen(ctx, w, wl, start, 20_000, n, usualBudget, rec)

		t.Logf("generator-only, simulated approvals: %s", rec.summary())
		require.NoError(t, err)
		assertNullRun(t, w, rec, n)
		assertSchedule(t, w, wl, start, 50*time.Microsecond)
	})

	t.Run("closed 256 producers", func(t *testing.T) {
		const n = 20_000

		w, rec := newNullWriter(ctx, t, n), newRecorder()
		err := runClosed(ctx, w, newTestWorkload(t, "null-closed", mixOutflow, false, 1000), 256, n, usualBudget, rec)

		t.Logf("generator-only, simulated approvals: %s", rec.summary())
		require.NoError(t, err)
		assertNullRun(t, w, rec, n)
	})

	t.Run("late start cancelled at the writer", func(t *testing.T) {
		const n = 100_000

		run, stop := context.WithCancel(ctx)
		defer stop()

		w, rec, wl := newNullWriter(ctx, t, n), newRecorder(), newTestWorkload(t, "null-late", mixOutflow, false, 1000)
		start := wl.now().Add(-time.Second)
		w.cancelAt, w.cancel = 1000, stop
		err := runOpen(run, w, wl, start, 1_000_000, n, usualBudget, rec)

		t.Logf("generator-only, simulated approvals: %s", rec.summary())
		require.ErrorIs(t, err, context.Canceled, "a cancelled run is not a result")
		assert.Less(t, len(rec.lag), n, "1 s late at 1M/s, all due at the first wake: the writer's cancel at 1000 stops dispatch")
		assert.Equal(t, [2]int{len(rec.lag), len(rec.lag)}, [2]int{len(rec.latency), len(w.seen)}, "every attempt sent answers once")
		assertSchedule(t, w, wl, start, time.Microsecond)
	})

	t.Run("classification", func(t *testing.T) {
		rec, wl := newRecorder(), newTestWorkload(t, "classify", mixOutflow, false, 10)
		answers := []result{
			{outcome: outcomeApproved, dbAckAt: scheduleStart.Add(3 * time.Millisecond), brokerAckAt: scheduleStart.Add(4 * time.Millisecond)},
			{outcome: outcomeApproved},
			{outcome: outcomeRefused, reason: "insufficient funds", decisionStartedAt: scheduleStart.Add(time.Millisecond)},
			{outcome: outcomeExpired},
			{outcome: outcomeReplay, original: outcomeApproved},
			{outcome: outcomeReplay, original: outcomeRefused, reason: "insufficient funds"},
			{outcome: outcomeConflict},
			{outcome: outcomeUnknown, reason: "commit unconfirmed"},
		}

		for i, res := range answers {
			req := testRequest(t, wl, int64(i), scheduleStart, usualBudget)
			rec.observe(req, res, scheduleStart.Add(5*time.Millisecond))
		}

		require.NoError(t, rec.lateErr())

		late := testRequest(t, wl, 99, scheduleStart, backlogBudget)
		rec.observe(late, result{requestID: late.id, outcome: outcomeUnknown}, late.deadline.Add(answerGrace+time.Nanosecond))

		assert.Equal(t, map[outcome]int64{
			outcomeApproved: 2, outcomeRefused: 1, outcomeExpired: 1, outcomeReplay: 2, outcomeConflict: 1, outcomeUnknown: 2,
		}, rec.outcomes, "only deciding answers count as approvals")
		assert.Equal(t, map[outcome]int64{outcomeApproved: 1, outcomeRefused: 1}, rec.replays, "a replay repeats its original")
		assert.Equal(t, map[string]int64{"insufficient funds": 1}, rec.refusals, "a replayed refusal adds no reason")
		assert.Equal(t, [3][]time.Duration{{time.Millisecond}, {3 * time.Millisecond}, {4 * time.Millisecond}},
			[3][]time.Duration{rec.decisionStart, rec.dbAck, rec.brokerAck}, "an instant the path did not measure stays out")
		assert.Len(t, rec.latency, len(answers)+1, "every answer, fast refusals included")
		assert.Error(t, rec.lateErr(), "an answer past deadline plus grace fails the run")
	})
}
