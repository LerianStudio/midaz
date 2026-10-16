//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/errgroup"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// 32 workers each run batches on their own session: with batches of one, 32 callers' requests wait on one held money
// row from 32 backends at once. Released, every request approves once, and the rows reconcile after Close.
func TestOwnerlessWriterWorkers(t *testing.T) {
	noLeaks(t)

	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, broker := newTestBatcher(ctx, t, env, clock.now)
	baseline := readTotals(ctx, t, conn)

	const workers, app = 32, "bench-ownerless-worker"

	reqs := make([]request, workers)
	for i := range reqs {
		name := fmt.Sprintf("ownerless/worker/%d", i)
		reqs[i] = batchReq(t, name, name, clock.now(), mv(classHot, 0, constant.DefaultBalanceKey)(-1),
			mv(classOrdinary, i%4, constant.DefaultBalanceKey)(1))
	}

	holder := holdLock(ctx, t, env, lockBalance, balanceID(classHot, 0, constant.DefaultBalanceKey))
	w := newTestOwnerless(ctx, t, env, b, app, workers, 1)
	run := submitEach(ctx, t, w, reqs)

	awaitWaiting(ctx, t, conn, app+"-%", workers)
	require.NoError(t, holder.Rollback(ctx))

	got := summarize(t, reqs, run.wait(t))
	require.NoError(t, w.Close(ctx))

	assert.Equal(t, slices.Repeat([]string{"approved+tx"}, workers), got)

	for i, s := range w.workers {
		assert.Equal(t, workerStats{batches: 1, batchStats: batchStats{passes: 1}}, s, "worker %d", i)
	}

	assert.Zero(t, w.entryLate.Load())

	totals := readTotals(ctx, t, conn)
	assert.Equal(t, baseline.Available, totals.Available, "money is conserved")
	assert.Equal(t, baseline.Transactions+workers, totals.Transactions)
	assertBatchLedger(ctx, t, conn)
	assert.Zero(t, broker.publishes.Load(), "obligations stay in the outbox")
}

// One worker busy on a held row while nine attempts queue behind it: released, it drains them in batches of at most
// maxBatch, eight and then one, answering each caller by position, so a copy of the busy identity replays it.
func TestOwnerlessWriterDrain(t *testing.T) {
	noLeaks(t)

	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, _ := newTestBatcher(ctx, t, env, clock.now)
	hot := mv(classHot, 0, constant.DefaultBalanceKey)
	baseline := readTotals(ctx, t, conn)

	const app, maxBatch = "bench-ownerless-drain", 8

	busy := batchReq(t, "ownerless/drain/busy", "ownerless/drain/busy", clock.now(), hot(-1), mv(classOrdinary, 0, constant.DefaultBalanceKey)(1))
	queued := append(make([]request, 0, maxBatch+1), busy)

	for i := range maxBatch {
		name := fmt.Sprintf("ownerless/drain/%d", i)
		queued = append(queued, batchReq(t, name, name, clock.now(), hot(-1), mv(classOrdinary, i%4, constant.DefaultBalanceKey)(1)))
	}

	holder := holdLock(ctx, t, env, lockBalance, balanceID(classHot, 0, constant.DefaultBalanceKey))
	w := newTestOwnerless(ctx, t, env, b, app, 1, maxBatch)
	first := submitEach(ctx, t, w, []request{busy})

	awaitBackend(ctx, t, conn, app+"-0", "Lock")

	rest := submitEach(ctx, t, w, queued)
	require.Eventually(t, func() bool { return len(w.queue) == len(queued) }, 30*time.Second, 10*time.Millisecond)
	require.NoError(t, holder.Rollback(ctx))

	assert.Equal(t, []string{"approved+tx"}, summarize(t, []request{busy}, first.wait(t)))
	assert.Equal(t, append([]string{"replay/approved+tx"}, slices.Repeat([]string{"approved+tx"}, maxBatch)...),
		summarize(t, queued, rest.wait(t)))
	require.NoError(t, w.Close(ctx))

	assert.Equal(t, 3, w.workers[0].batches, "the busy batch, a full batch of maxBatch, then the one left")

	totals := readTotals(ctx, t, conn)
	assert.Equal(t, baseline.Available, totals.Available, "money is conserved")
	assert.Equal(t, baseline.Transactions+maxBatch+1, totals.Transactions, "each identity approved once")
	assertBatchLedger(ctx, t, conn)
}

// A failed batch does not end another worker's accepted one: one pass ends at its 2 s response deadline on a held
// row, the other's caller leaves, and a normal Close drains that batch on the row until released. It approves once,
// and Close reports the failed batch.
func TestOwnerlessWriterErrorIsolation(t *testing.T) {
	noLeaks(t)

	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, _ := newTestBatcher(ctx, t, env, clock.now)
	hot, ord := mv(classHot, 0, constant.DefaultBalanceKey), mv(classOrdinary, 0, constant.DefaultBalanceKey)
	baseline := readTotals(ctx, t, conn)

	short, err := newRequest(uuid.NewSHA1(benchNS, []byte("ownerless/iso/short")), uuid.NewSHA1(benchNS, []byte("ownerless/tx/short")),
		[]posting{hot(-1), ord(1)}, clock.now(), backlogBudget)
	require.NoError(t, err)

	long := batchReq(t, "ownerless/iso/long", "ownerless/iso/long", clock.now(), hot(-1), ord(1))

	const app = "bench-ownerless-iso"

	holder := holdLock(ctx, t, env, lockBalance, balanceID(classHot, 0, constant.DefaultBalanceKey))
	w := newTestOwnerless(ctx, t, env, b, app, 2, 1)

	callerCtx, leave := context.WithCancel(ctx)
	shortCaller, longCaller := submitEach(ctx, t, w, []request{short}), submitEach(callerCtx, t, w, []request{long})

	awaitWaiting(ctx, t, conn, app+"-%", 2)
	assertUnknown(t, shortCaller.wait(t), context.DeadlineExceeded.Error())
	awaitWaiting(ctx, t, conn, app+"-%", 1) // the short pass ended; the long one still waits
	clock.advance(backlogBudget.caller)

	leave()
	assertUnknown(t, longCaller.wait(t), context.Canceled.Error())

	closing := closeAsync(ctx, w)
	require.Eventually(t, w.closed.Load, 30*time.Second, 10*time.Millisecond)
	assertWaiting(ctx, t, conn, app+"-%", 1)
	require.False(t, closing.returned(), "a normal Close returned before the long batch drained")
	require.NoError(t, holder.Rollback(ctx))

	err = closing.wait(t)
	require.ErrorContains(t, err, "lock balances", "Close reports the failed batch")
	assert.NotContains(t, err.Error(), "close aborted", "a normal Close aborted nothing")

	assert.Equal(t, map[string]string{long.id.String(): "approved"},
		readRows(ctx, t, conn, `SELECT request_id::text, outcome FROM engine_request`))

	totals := readTotals(ctx, t, conn)
	assert.Equal(t, baseline.Available, totals.Available, "money is conserved")
	assert.Equal(t, baseline.Transactions+1, totals.Transactions, "the long request approved once")
	assertBatchLedger(ctx, t, conn)
}

// Callers that leave do not end accepted work: one cancelled while its batch waits on a held row, one queued behind
// it already past admission with 1 s of response left, which waits only that second. Both answer unknown; a normal
// Close drains both, cancelling neither, and SQL approves the first and expires the second, never renewing admission.
func TestOwnerlessWriterCallerLeaves(t *testing.T) {
	noLeaks(t)

	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, _ := newTestBatcher(ctx, t, env, clock.now)
	hot, ord := mv(classHot, 0, constant.DefaultBalanceKey), mv(classOrdinary, 0, constant.DefaultBalanceKey)
	baseline := readTotals(ctx, t, conn)

	inSQL := batchReq(t, "ownerless/leaves/sql", "ownerless/leaves/sql", clock.now(), hot(-1), ord(1))
	// Scheduled 29 s ago under usualBudget: 1 s past its admission cutoff, 1 s short of its response deadline.
	queued := batchReq(t, "ownerless/leaves/queued", "ownerless/leaves/queued", clock.now().Add(-29*time.Second), hot(-1), ord(1))

	const app = "bench-ownerless-leaves"

	holder := holdLock(ctx, t, env, lockBalance, balanceID(classHot, 0, constant.DefaultBalanceKey))
	w := newTestOwnerless(ctx, t, env, b, app, 1, 1)

	callerCtx, leave := context.WithCancel(ctx)
	first := submitEach(callerCtx, t, w, []request{inSQL})

	awaitBackend(ctx, t, conn, app+"-0", "Lock")

	// The test's own bound, room for host scheduling past the remaining second: a renewed 30 s budget ends here.
	bounded, stop := context.WithTimeoutCause(ctx, 6*time.Second, errors.New("outlived its remaining budget"))
	defer stop()

	second := submitEach(bounded, t, w, []request{queued})

	leave()
	assertUnknown(t, first.wait(t), context.Canceled.Error())
	assertUnknown(t, second.wait(t), context.DeadlineExceeded.Error())
	clock.advance(time.Second) // the queued caller's remaining second

	closing := closeAsync(ctx, w)
	require.Eventually(t, w.closed.Load, 30*time.Second, 10*time.Millisecond)
	assertWaiting(ctx, t, conn, app+"-0", 1)
	require.False(t, closing.returned(), "a normal Close returned before its accepted batches drained")
	require.NoError(t, holder.Rollback(ctx))
	require.NoError(t, closing.wait(t))

	assert.Equal(t, map[string]string{inSQL.id.String(): "approved", queued.id.String(): "expired"},
		readRows(ctx, t, conn, `SELECT request_id::text, outcome FROM engine_request`))
	assert.EqualValues(t, 1, w.entryLate.Load())

	totals := readTotals(ctx, t, conn)
	assert.Equal(t, baseline.Available, totals.Available, "money is conserved")
	assert.Equal(t, baseline.Transactions+1, totals.Transactions, "only the admissible request approved")
	assertBatchLedger(ctx, t, conn)
}

// A Close whose own deadline passes while an accepted batch waits on a held row aborts it, returns that deadline with
// the batch's error well short of the 10 s run cap, and moves no money. A later Submit answers unknown, unqueued.
func TestOwnerlessWriterCloseDeadline(t *testing.T) {
	noLeaks(t)

	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, _ := newTestBatcher(ctx, t, env, clock.now)
	hot, ord := mv(classHot, 0, constant.DefaultBalanceKey), mv(classOrdinary, 0, constant.DefaultBalanceKey)
	baseline := readTotals(ctx, t, conn)

	const app = "bench-ownerless-abort"

	holder := holdLock(ctx, t, env, lockBalance, balanceID(classHot, 0, constant.DefaultBalanceKey))
	w := newTestOwnerless(ctx, t, env, b, app, 1, 256)

	callerCtx, leave := context.WithCancel(ctx)
	caller := submitEach(callerCtx, t, w, []request{batchReq(t, "ownerless/abort", "ownerless/abort", clock.now(), hot(-1), ord(1))})

	awaitBackend(ctx, t, conn, app+"-0", "Lock")
	leave()
	assertUnknown(t, caller.wait(t), context.Canceled.Error())

	closeCtx, stop := context.WithTimeout(ctx, 500*time.Millisecond)
	defer stop()

	closing := closeAsync(closeCtx, w)

	// The test's own bound: room for host scheduling past the 500 ms Close deadline, short of the 10 s run cap.
	waited, stopWaiting := context.WithTimeout(ctx, 6*time.Second)
	defer stopWaiting()

	select {
	case <-closing.done:
	case <-waited.Done():
		require.FailNow(t, "Close outlived its own deadline")
	}

	err := closing.wait(t)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "lock balances", "the aborted batch's own error joins the deadline")

	awaitPrimary(ctx, t, conn, "the aborted batch's backend is gone",
		`SELECT NOT EXISTS (SELECT 1 FROM pg_stat_activity WHERE application_name = $1)`, app+"-0")
	require.NoError(t, holder.Rollback(ctx))
	assert.Equal(t, baseline, readTotals(ctx, t, conn), "the aborted batch left no claim and moved nothing")

	late := batchReq(t, "ownerless/closed", "ownerless/closed", clock.now(), hot(-1), ord(1))
	assertUnknown(t, []result{w.Submit(ctx, late)}, "closed")
}

// Lateness at entry is counted, not answered: a request late at entry still resolves its identity in SQL, so a known
// approval replays and a new identity expires, neither moving money.
func TestOwnerlessWriterExpiredEntry(t *testing.T) {
	noLeaks(t)

	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, _ := newTestBatcher(ctx, t, env, clock.now)
	hot, ord := mv(classHot, 0, constant.DefaultBalanceKey), mv(classOrdinary, 0, constant.DefaultBalanceKey)
	w := newTestOwnerless(ctx, t, env, b, "bench-ownerless-entry", 1, 256)

	paid := batchReq(t, "ownerless/entry/paid", "ownerless/entry/paid", clock.now(), hot(-1), ord(1))
	fresh := batchReq(t, "ownerless/entry/new", "ownerless/entry/new", clock.now(), hot(-1), ord(1))

	assert.Equal(t, []string{"approved+tx"}, summarize(t, []request{paid}, []result{w.Submit(ctx, paid)}))

	want := readTotals(ctx, t, conn)

	clock.advance(usualBudget.admission) // both are late at entry, with 2 s of response budget left

	got := summarize(t, []request{paid, fresh}, []result{w.Submit(ctx, paid), w.Submit(ctx, fresh)})

	const gone = 32 // response deadlines already passed at entry, as dispatch lag leaves them: never queued

	for i := range gone {
		name := fmt.Sprintf("ownerless/entry/gone/%d", i)
		req := batchReq(t, name, name, clock.now().Add(-usualBudget.caller-time.Second), hot(-1), ord(1))
		assertUnknown(t, []result{w.Submit(ctx, req)}, context.DeadlineExceeded.Error())
	}

	require.NoError(t, w.Close(ctx))

	assert.Equal(t, []string{"replay/approved+tx", "expired"}, got)
	assert.EqualValues(t, 2+gone, w.entryLate.Load(), "late at entry, queued or not")

	want.Requests++ // the new identity, finalized expired; none gone at entry is claimed
	want.Touches++  // the replay's WAL touch
	assert.Equal(t, want, readTotals(ctx, t, conn), "no late request moved money")
	assertBatchLedger(ctx, t, conn)
}

// noLeaks fails the test when a goroutine started after this call outlives the cleanups registered after it.
func noLeaks(t *testing.T) {
	t.Helper()

	ignore := goleak.IgnoreCurrent()

	t.Cleanup(func() { goleak.VerifyNone(t, ignore) })
}

// newTestOwnerless starts a writer over workers batchers sharing b's outbox, each on a session named app-<i>.
func newTestOwnerless(ctx context.Context, t *testing.T, env *benchEnv, b *batcher, app string, workers, maxBatch int) *ownerlessWriter {
	t.Helper()

	batchers := make([]*batcher, workers)
	for i := range batchers {
		batchers[i] = namedBatcher(t, b, env, fmt.Sprintf("%s-%d", app, i), nil)
	}

	w := newOwnerlessWriter(ctx, batchers, maxBatch)

	// Runs before the sessions close, and only acts for a test that failed before its own Close.
	t.Cleanup(func() {
		if !w.closed.Load() {
			closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			t.Logf("close after a failure: %v", w.Close(closeCtx))
		}
	})

	return w
}

// submits is one caller goroutine per request; wait returns their answers by position.
type submits struct {
	g   errgroup.Group
	res []result
}

func submitEach(ctx context.Context, t *testing.T, w writer, reqs []request) *submits {
	t.Helper()

	s := &submits{res: make([]result, len(reqs))}

	for i, req := range reqs {
		s.g.Go(func() error {
			s.res[i] = w.Submit(ctx, req)
			return nil
		})
	}

	// A failed test has already ended ctx, so its callers return: this joins them before the writer's cleanup Close.
	t.Cleanup(func() { assert.NoError(t, s.g.Wait()) })

	return s
}

func (s *submits) wait(t *testing.T) []result {
	t.Helper()
	require.NoError(t, s.g.Wait())

	return s.res
}

// closing is one Close on its own goroutine; done closes once it returned.
type closing struct {
	g    errgroup.Group
	done chan struct{}
	err  error
}

func closeAsync(ctx context.Context, w writer) *closing {
	c := &closing{done: make(chan struct{})}
	c.g.Go(func() error {
		defer close(c.done)

		c.err = w.Close(ctx)

		return nil
	})

	return c
}

func (c *closing) returned() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *closing) wait(t *testing.T) error {
	t.Helper()
	require.NoError(t, c.g.Wait())

	return c.err
}

// assertUnknown checks answers that resolved nothing: unknown, unmeasured, carrying cause in their reason.
func assertUnknown(t *testing.T, res []result, cause string) {
	t.Helper()

	for _, r := range res {
		assert.Equal(t, outcomeUnknown, r.outcome)
		assert.Contains(t, r.reason, cause)
		assert.True(t, r.decisionStartedAt.IsZero() && r.dbAckAt.IsZero(), "an unknown answer measures nothing")
	}
}

// lockWaiters counts the sessions named like $1 that wait on a lock.
const lockWaiters = `SELECT count(*) FROM pg_stat_activity WHERE application_name LIKE $1 AND wait_event_type = 'Lock'`

// awaitWaiting polls until n sessions named like app wait on a lock.
func awaitWaiting(ctx context.Context, t *testing.T, conn *pgx.Conn, app string, n int) {
	t.Helper()
	awaitPrimary(ctx, t, conn, fmt.Sprintf("%d sessions named like %s wait on a lock", n, app), "SELECT ("+lockWaiters+") = $2", app, n)
}

// assertWaiting checks, once, that n sessions named like app still wait on a lock.
func assertWaiting(ctx context.Context, t *testing.T, conn *pgx.Conn, app string, n int) {
	t.Helper()

	var waiting int
	require.NoError(t, conn.QueryRow(ctx, lockWaiters, app).Scan(&waiting))
	require.Equal(t, n, waiting, "sessions named like %s still waiting on the held row", app)
}
