//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/errgroup"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// One batch decides new requests in arrival order over shared, tentatively decided rows, answers in-batch copies and
// stored identities, and finalizes every claim in the same COMMIT, all-refused and all-expired batches included. A
// batch naming an unseeded balance errors whole: every answer unknown, nothing written.
func TestBatchDecidesResolvesAndFinalizes(t *testing.T) {
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, broker := newTestBatcher(ctx, t, env, clock)

	limitFixtures(ctx, t, conn)

	baseline := readTotals(ctx, t, conn)
	now, late := clock.now(), clock.now().Add(-time.Hour)
	hot, fee := mv(classHot, 0, constant.DefaultBalanceKey), mv(classFee, 0, constant.DefaultBalanceKey)
	mk, mk2 := mv(classMultikey, 0, constant.DefaultBalanceKey), mv(classMultikey, 0, keySecondary)
	ord := func(i int) func(int64) posting { return mv(classOrdinary, i, constant.DefaultBalanceKey) }

	r1 := batchReq(t, "r1", "r1", now, hot(-60), ord(0)(60))
	r2 := batchReq(t, "r2", "r2", now, hot(-50), ord(1)(50))
	r3 := batchReq(t, "r3", "r3", now, hot(-40), ord(1)(40))
	r9 := batchReq(t, "r9", "r9", late, ord(0)(-1), ord(1)(1))
	batchA := []request{
		r1, r2, r3,
		batchReq(t, "r4", "r4", now, ord(1)(-10), ord(2)(10)),
		batchReq(t, "r5", "r5", now, ord(2)(-10), ord(0)(10)), // funded only by r4
		batchReq(t, "r6", "r6", now, ord(2)(-1), ord(0)(1)),
		batchReq(t, "r7", "r7", now, ord(3)(-5), ord(0)(5)),
		batchReq(t, "r8", "r8", now, mk(-30), mk2(-20), hot(49), fee(1)),
		r9,
		r1, r2, batchReq(t, "r3", "r3-rebuilt", now, hot(-40), ord(1)(40)), // copies of r1 and r2, then r3 rebuilt
	}

	res, stats, err := b.run(ctx, batchA)
	require.NoError(t, err)
	assert.Equal(t, batchStats{passes: 1}, stats)
	assert.Equal(t, []string{
		"approved+tx", "refused/limit_exceeded", "approved+tx", "approved+tx", "approved+tx", "refused/insufficient_funds",
		"refused/blocked", "approved+tx", "expired", "replay/approved+tx", "replay/refused/limit_exceeded", "conflict",
	}, summarize(t, batchA, res))

	totals := readTotals(ctx, t, conn)
	assert.Equal(t, baseline.Available, totals.Available, "money is conserved")
	assertMixedBatchRows(ctx, t, conn)

	// Stored identities replay or conflict against the stored payload, never against an earlier copy, and move nothing.
	batchB := []request{batchReq(t, "r1", "r1-rebuilt", now, hot(-60), ord(0)(60)), r1, r2, r9}
	res, _, err = b.run(ctx, batchB)
	require.NoError(t, err)
	assert.Equal(t, []string{"conflict", "replay/approved+tx", "replay/refused/limit_exceeded", "replay/expired"},
		summarize(t, batchB, res))

	totals.Touches += 3
	assert.Equal(t, totals, readTotals(ctx, t, conn), "replays only touch their identity")

	// A batch with nothing to approve still finalizes every claim.
	batchC := []request{
		batchReq(t, "r10", "r10", now, ord(3)(-1), ord(0)(1)),
		batchReq(t, "r11", "r11", late, ord(0)(-1), ord(1)(1)),
	}
	res, _, err = b.run(ctx, batchC)
	require.NoError(t, err)
	assert.Equal(t, []string{"refused/blocked", "expired"}, summarize(t, batchC, res))

	totals.Requests += 2
	assert.Equal(t, totals, readTotals(ctx, t, conn), "refusals and expiries move nothing")

	unseeded := posting{BalanceID: uuid.NewSHA1(benchNS, []byte("batch/unseeded")), AccountID: accountID(classOrdinary, 1), Amount: 1}
	batchD := []request{r1, batchReq(t, "r12", "r12", now, ord(0)(-1), unseeded)}
	res, _, err = b.run(ctx, batchD)
	require.ErrorContains(t, err, "not seeded")
	assert.Equal(t, []string{"unknown", "unknown"}, summarize(t, batchD, res))
	assert.Equal(t, totals, readTotals(ctx, t, conn), "an errored batch leaves no claim, touch or money")

	assertBatchLedger(ctx, t, conn)
	assert.Zero(t, broker.publishes.Load(), "the batch writes the outbox and never publishes")
}

// While another transaction holds a money row: a new request late at entry expires without waiting on it; one still
// admissible waits only until its pass's response deadline, well short of the 10 s run cap, and leaves no claim; one
// whose wait outlasts its cutoff expires on the rows it locked once they are free.
func TestBatchBoundsWaitsOnHeldMoneyRow(t *testing.T) {
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, _ := newTestBatcher(ctx, t, env, clock)
	hot, ord := mv(classHot, 0, constant.DefaultBalanceKey), mv(classOrdinary, 0, constant.DefaultBalanceKey)
	want := readTotals(ctx, t, conn)
	holder := holdLock(ctx, t, env, lockBalance, balanceID(classHot, 0, constant.DefaultBalanceKey))

	late := []request{batchReq(t, "lock/late", "lock/late", clock.now().Add(-time.Hour), hot(-1), ord(1))}
	res, stats, err := b.run(ctx, late)
	require.NoError(t, err, "a request late at entry must not wait on the held row")
	assert.Equal(t, batchStats{passes: 1}, stats)
	assert.Equal(t, []string{"expired"}, summarize(t, late, res))

	want.Requests++

	short, err := newRequest(uuid.NewSHA1(benchNS, []byte("batch/lock/short")), uuid.NewSHA1(benchNS, []byte("batch/tx/lock/short")),
		[]posting{hot(-1), ord(1)}, clock.now(), backlogBudget) // its pass may last 2 s
	require.NoError(t, err)

	const shortApp = "bench-batch-deadline"

	reqs := []request{short, batchReq(t, "lock/long", "lock/long", clock.now(), hot(-1), ord(1))}
	run := startBatch(ctx, namedBatcher(t, b, env, shortApp, nil), reqs)

	// The test's own bound: room for host scheduling past the 2 s pass bound, short of the 10 s run cap.
	waited, stop := context.WithTimeout(ctx, 6*time.Second)
	defer stop()

	select {
	case <-run.done:
	case <-waited.Done():
		require.FailNow(t, "the pass outlived its response deadline")
	}

	run.wait(t)
	require.ErrorIs(t, run.err, context.DeadlineExceeded)
	require.ErrorContains(t, run.err, "lock balances")
	assert.Equal(t, []string{"unknown", "unknown"}, summarize(t, reqs, run.res))
	awaitPrimary(ctx, t, conn, "the cut pass's backend is gone",
		`SELECT NOT EXISTS (SELECT 1 FROM pg_stat_activity WHERE application_name = $1)`, shortApp)
	assert.Equal(t, want, readTotals(ctx, t, conn), "the cut pass left no claim")

	const app = "bench-batch-lockwait"

	fresh := []request{batchReq(t, "lock/fresh", "lock/fresh", clock.now(), hot(-1), ord(1))}
	run = startBatch(ctx, namedBatcher(t, b, env, app, nil), fresh)

	awaitBackend(ctx, t, conn, app, "Lock")
	clock.advance(usualBudget.admission) // at the cutoff instant the request is already late
	require.NoError(t, holder.Rollback(ctx))

	run.wait(t)
	require.NoError(t, run.err)
	assert.Equal(t, batchStats{passes: 1}, run.stats)
	assert.Equal(t, []string{"expired"}, summarize(t, fresh, run.res))

	want.Requests++
	assert.Equal(t, want, readTotals(ctx, t, conn), "the late requests moved nothing")
	assertBatchLedger(ctx, t, conn)
}

// A new decision crossing its cutoff before COMMIT voids the whole pass, its events included: the next pass rereads
// the rows and recomputes what depended on it. A declined identity later replays declined, even once funded.
func TestBatchRecomputesPassCrossingCutoffBeforeCommit(t *testing.T) {
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, _ := newTestBatcher(ctx, t, env, clock)
	ord := func(i int) func(int64) posting { return mv(classOrdinary, i, constant.DefaultBalanceKey) }
	recipient := balanceID(classOrdinary, 2, constant.DefaultBalanceKey)

	_, err := conn.Exec(ctx, `UPDATE balance SET available = 50 WHERE id = $1`, recipient)
	require.NoError(t, err)

	credit, err := newRequest(uuid.NewSHA1(benchNS, []byte("batch/recompute/a")), uuid.NewSHA1(benchNS, []byte("batch/tx/recompute/a")),
		[]posting{ord(0)(-40), ord(2)(40)}, clock.now(), budget{caller: usualBudget.caller, admission: time.Second})
	require.NoError(t, err)

	reqs := []request{credit, batchReq(t, "recompute/b", "recompute/b", clock.now(), ord(2)(-80), ord(1)(80))} // b needs a's 40
	want := readTotals(ctx, t, conn)
	holder := holdLock(ctx, t, env, `LOCK TABLE outbox_events IN SHARE MODE`)

	const app = "bench-batch-recompute"

	run := startBatch(ctx, namedBatcher(t, b, env, app, nil), reqs)

	awaitBackend(ctx, t, conn, app, "relation") // both approved, now writing their events
	clock.advance(time.Second)
	require.NoError(t, holder.Rollback(ctx))

	run.wait(t)
	require.NoError(t, run.err)
	assert.Equal(t, batchStats{passes: 2}, run.stats)
	assert.Equal(t, []string{"expired", "refused/insufficient_funds"}, summarize(t, reqs, run.res))

	want.Requests += 2
	assert.Equal(t, want, readTotals(ctx, t, conn), "the voided pass left no money, operation, version or outbox row")

	funded := []request{batchReq(t, "recompute/c", "recompute/c", clock.now(), ord(0)(-100), ord(2)(100)), reqs[1]}
	res, _, err := b.run(ctx, funded)
	require.NoError(t, err)
	assert.Equal(t, []string{"approved+tx", "replay/refused/insufficient_funds"}, summarize(t, funded, res))

	var after string
	require.NoError(t, conn.QueryRow(ctx, `SELECT available::text || ' v' || version FROM balance WHERE id = $1`, recipient).Scan(&after))
	assert.Equal(t, "150 v1", after, "the credit alone moved the recipient; the declined replay debited nothing")
	assertBatchLedger(ctx, t, conn)
}

// A COMMIT whose synchronous wait is cancelled answers every position unknown and is never retried, though it committed
// locally. Retries under new response deadlines replay the original decisions past their cutoffs and move nothing; their
// touch makes their own COMMIT wait for the paused standby, so they answer only once it has applied the original.
func TestBatchUnknownCommitReplaysWithoutMovingMoney(t *testing.T) {
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, _ := newTestBatcher(ctx, t, env, clock)

	stack, err := openVerifiedStack(ctx, env)
	require.NoError(t, err)

	defer stack.close(ctx)

	const app = "bench-batch-syncrep"

	nb := namedBatcher(t, b, env, app, map[string]string{"synchronous_commit": "remote_apply"})
	hot, ord := mv(classHot, 0, constant.DefaultBalanceKey), mv(classOrdinary, 0, constant.DefaultBalanceKey)
	reqs := []request{
		batchReq(t, "unknown/a", "unknown/a", clock.now(), hot(-5), ord(5)),
		batchReq(t, "unknown/b", "unknown/b", clock.now(), ord(-3), hot(3)),
	}
	want := readTotals(ctx, t, conn)

	pauseReplay(ctx, t, env, stack.standby)

	run := startBatch(ctx, nb, reqs)
	pid := awaitBackend(ctx, t, conn, app, "SyncRep")

	var signalled bool
	require.NoError(t, conn.QueryRow(ctx, `SELECT pg_cancel_backend($1)`, pid).Scan(&signalled))
	require.True(t, signalled)

	run.wait(t)
	require.ErrorContains(t, run.err, "canceling wait for synchronous replication")
	assert.Equal(t, batchStats{passes: 1}, run.stats, "nothing retries once COMMIT was sent")
	assert.Equal(t, []string{"unknown", "unknown"}, summarize(t, reqs, run.res))

	local := readTotals(ctx, t, conn)
	assert.Equal(t, want.Transactions+2, local.Transactions, "the cancelled COMMIT committed locally")

	clock.advance(time.Minute) // past every cutoff and original response deadline

	retries := slices.Clone(reqs)

	for i := range retries {
		retries[i].deadline = clock.now().Add(usualBudget.caller)
	}

	replay := startBatch(ctx, nb, retries)

	awaitBackend(ctx, t, conn, app, "SyncRep")
	assert.Zero(t, countTransactions(ctx, t, stack.standby, reqs), "the replay waits while the standby has applied nothing")

	_, err = stack.standby.Exec(ctx, `SELECT pg_wal_replay_resume()`)
	require.NoError(t, err)

	replay.wait(t)
	require.NoError(t, replay.err)
	assert.Equal(t, batchStats{passes: 1}, replay.stats)
	assert.Equal(t, []string{"replay/approved+tx", "replay/approved+tx"}, summarize(t, retries, replay.res))

	local.Touches += 2
	assert.Equal(t, local, readTotals(ctx, t, conn), "the retries only touched their identities")
	assert.Equal(t, 2, countTransactions(ctx, t, stack.standby, reqs), "the replay answered after the standby applied the original")
	assertBatchLedger(ctx, t, conn)
}

// countTransactions counts the transactions of reqs that conn sees.
func countTransactions(ctx context.Context, t *testing.T, conn *pgx.Conn, reqs []request) int {
	t.Helper()

	ids := column(reqs, func(r request) string { return r.transactionID.String() })

	var n int
	require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM "transaction" WHERE id = ANY($1::uuid[])`, ids).Scan(&n))

	return n
}

// Concurrent batches on separate sessions share request values and one hot account limit. Every identity gets exactly
// one decision, approvals stop where the limit runs out, every other copy replays and touches it, and the rows chain.
// Each batch finishing in one pass is this one measured interleaving, not a proof that batches never deadlock.
func TestBatchConcurrentSessionsShareLimitAndIdentities(t *testing.T) {
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, _ := newTestBatcher(ctx, t, env, clock)
	hotAccount := accountID(classHot, 0)

	_, err := conn.Exec(ctx, `UPDATE account_control SET limit_remaining = 100 WHERE account_id = $1`, hotAccount)
	require.NoError(t, err)

	const workers, distinct = 8, 24

	shared := make([]request, distinct)
	for i := range shared {
		name := fmt.Sprintf("shared/%d", i)
		shared[i] = batchReq(t, name, name, clock.now(), mv(classHot, 0, constant.DefaultBalanceKey)(-10),
			mv(classOrdinary, i%4, constant.DefaultBalanceKey)(10))
	}

	baseline := readTotals(ctx, t, conn)
	batches, runs := make([][]request, workers), make([]*batchRun, workers)
	holder := holdLock(ctx, t, env, lockBalance, balanceID(classHot, 0, constant.DefaultBalanceKey))

	for w := range workers {
		for k := range distinct / 2 {
			batches[w] = append(batches[w], shared[(3*w+k)%distinct])
		}

		batches[w] = append(batches[w], batches[w][0]) // and an in-batch copy
		runs[w] = startBatch(ctx, namedBatcher(t, b, env, fmt.Sprintf("bench-batch-worker-%d", w), nil), batches[w])
	}

	// Each worker now waits on the held balance or on an identity another worker has in flight: all overlap.
	awaitPrimary(ctx, t, conn, "every worker waits on a lock", `SELECT count(*) = $1 FROM pg_stat_activity
		WHERE application_name LIKE 'bench-batch-worker-%' AND wait_event_type = 'Lock'`, workers)
	require.NoError(t, holder.Rollback(ctx))

	decided, replays := decisionsOf(t, batches, runs)
	outcomes := map[string]int{}

	for _, r := range decided {
		outcomes[string(r.outcome)+"/"+r.reason]++
	}

	assert.Equal(t, map[string]int{"approved/": 10, "refused/limit_exceeded": 14}, outcomes)
	assert.Len(t, replays, workers*(distinct/2+1)-distinct, "every other copy replays")

	for _, r := range replays {
		assert.Equal(t, decided[r.requestID].outcome, r.original, "replay of %s", r.requestID)
		assert.Equal(t, decided[r.requestID].transactionID, r.transactionID, "replay of %s", r.requestID)
	}

	var control string
	require.NoError(t, conn.QueryRow(ctx, `SELECT limit_remaining::text || ' v' || version FROM account_control WHERE account_id = $1`,
		hotAccount).Scan(&control))
	assert.Equal(t, "0 v10", control, "ten approved debits of 10 exhaust the limit of 100, never below it")
	totals := readTotals(ctx, t, conn)
	assert.Equal(t, baseline.Available, totals.Available, "money is conserved")
	assert.Equal(t, baseline.Touches+3*distinct, totals.Touches, "each of an identity's 3 other batches touched it once")
	assertBatchLedger(ctx, t, conn)
}

// decisionsOf waits for every run and splits its answers: each identity's one decision, and every replay.
func decisionsOf(t *testing.T, batches [][]request, runs []*batchRun) (map[uuid.UUID]result, []result) {
	t.Helper()

	decided, replays := map[uuid.UUID]result{}, []result{}

	for w, run := range runs {
		run.wait(t)
		require.NoError(t, run.err, "worker %d", w)
		assert.Equal(t, batchStats{passes: 1}, run.stats, "worker %d", w)

		for i, r := range run.res {
			require.Equal(t, batches[w][i].id, r.requestID)

			if r.outcome == outcomeReplay {
				replays = append(replays, r)
				continue
			}

			assert.NotContains(t, decided, r.requestID, "identity decided twice")
			decided[r.requestID] = r
		}
	}

	return decided, replays
}

// A batch that closes a lock cycle with another transaction, and whose own deadlock check fires first, is the victim
// PostgreSQL aborts with 40P01 before COMMIT: it retries once on fresh rows and approves.
func TestBatchRetriesDeadlockOnce(t *testing.T) {
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, _ := newTestBatcher(ctx, t, env, clock)
	from, to := mv(classOrdinary, 0, constant.DefaultBalanceKey), mv(classOrdinary, 1, constant.DefaultBalanceKey)
	reqs := []request{batchReq(t, "deadlock", "deadlock", clock.now(), from(-5), to(5))}
	ids := sortedDistinct([]uuid.UUID{from(0).BalanceID, to(0).BalanceID})
	want := readTotals(ctx, t, conn)

	// The holder checks for deadlock only after a minute, so the batch's own check is the one that ends the cycle.
	holder := holdLock(ctx, t, env, `SET LOCAL deadlock_timeout = '1min'`)
	_, err := holder.Exec(ctx, lockBalance, ids[1])
	require.NoError(t, err)

	const app = "bench-batch-deadlock"

	run := startBatch(ctx, namedBatcher(t, b, env, app, map[string]string{"deadlock_timeout": "3s"}), reqs)

	awaitBackend(ctx, t, conn, app, "Lock") // the batch holds ids[0], in key order, and waits on ids[1]

	_, err = holder.Exec(ctx, lockBalance, ids[0])
	require.NoError(t, err, "the holder got the batch's lock once the batch was aborted")
	require.NoError(t, holder.Rollback(ctx))

	run.wait(t)
	require.NoError(t, run.err)
	assert.Equal(t, batchStats{passes: 2, deadlocks: 1}, run.stats)
	assert.Equal(t, []string{"approved+tx"}, summarize(t, reqs, run.res))

	totals := readTotals(ctx, t, conn)
	assert.Equal(t, want.Available, totals.Available, "money is conserved")
	assert.Equal(t, want.Transactions+1, totals.Transactions, "the retry approved once")
	assertBatchLedger(ctx, t, conn)
}

// limitFixtures sets the rows this test owns: hot limit 100, ordinary 2 empty, ordinary 3 blocked, multikey limit 50.
func limitFixtures(ctx context.Context, t *testing.T, conn *pgx.Conn) {
	t.Helper()

	_, err := conn.Exec(ctx, `UPDATE account_control SET limit_remaining = CASE WHEN account_id = $1 THEN 100 ELSE 50 END
		WHERE account_id IN ($1, $2)`, accountID(classHot, 0), accountID(classMultikey, 0))
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `UPDATE balance SET available = 0 WHERE id = $1`, balanceID(classOrdinary, 2, constant.DefaultBalanceKey))
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `UPDATE account_control SET blocked = true WHERE account_id = $1`, accountID(classOrdinary, 3))
	require.NoError(t, err)
}

// assertMixedBatchRows checks the mixed batch's balances, debited controls, finalized claims and outbox rows.
func assertMixedBatchRows(ctx context.Context, t *testing.T, conn *pgx.Conn) {
	t.Helper()

	assert.Equal(t, map[string]string{
		"hot0": "9999999949 v3", "ordinary0": "10000000070 v2", "ordinary1": "10000000030 v2", "ordinary2": "0 v2",
		"ordinary3": "10000000000 v0", "multikey0": "9999999970 v1", "multikey0/secondary": "9999999980 v1", "fee0": "10000000001 v1",
	}, readRows(ctx, t, conn, `SELECT regexp_replace(alias, '^@bench_|_0{6}', '', 'g') || CASE WHEN key = 'default' THEN '' ELSE '/' || key END,
		available::text || ' v' || version FROM balance`))
	assert.Equal(t, map[string]string{
		"hot0": "0 v2", "ordinary0": "10000000000 v0", "ordinary1": "9999999990 v1", "ordinary2": "9999999990 v1",
		"ordinary3": "10000000000 v0", "multikey0": "0 v2", "fee0": "10000000000 v0",
	}, readRows(ctx, t, conn, `SELECT regexp_replace(b.alias, '^@bench_|_0{6}', '', 'g'), c.limit_remaining::text || ' v' || c.version
		FROM account_control c JOIN balance b ON b.account_id = c.account_id AND b.key = 'default'`))
	assert.Equal(t, map[string]string{"approved": "5 5", "refused": "3 0", "expired": "1 0"},
		readRows(ctx, t, conn, `SELECT outcome, count(*) || ' ' || count(transaction_id) FROM engine_request GROUP BY outcome`))
	assert.Equal(t, map[string]int{
		keyPosted: 5, keyChanged: 12, keyApplied: 1, kindMetadataUpsert: 2, kindTracerConfirm: 5,
	}, assertBatchLedger(ctx, t, conn))
}

// testClock is the batch's injected clock: fixed until a test moves it.
type testClock struct{ ns atomic.Int64 }

func newTestClock(at time.Time) *testClock {
	c := &testClock{}
	c.ns.Store(at.UnixNano())

	return c
}

func (c *testClock) now() time.Time { return time.Unix(0, c.ns.Load()).UTC() }

func (c *testClock) advance(d time.Duration) { c.ns.Add(int64(d)) }

const (
	lockBalance = `SELECT FROM balance WHERE id = $1 FOR UPDATE`
	// backendWaits is true while the session named $1 waits on $2, a wait event or its type.
	backendWaits = `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE application_name = $1 AND $2 IN (wait_event_type, wait_event))`
)

// namedBatcher returns a batcher sharing b's outbox on a session of its own, named app so the test can find its
// backend; settings are extra startup parameters.
func namedBatcher(t *testing.T, b *batcher, env *benchEnv, app string, settings map[string]string) *batcher {
	t.Helper()

	cfg := env.primary.Copy()
	maps.Copy(cfg.RuntimeParams, settings)
	cfg.RuntimeParams["application_name"] = app

	nb, err := newBatcher(openTestSession(t, cfg), b.events, b.rows, b.now)
	require.NoError(t, err)

	return nb
}

// batchRun is one batch on its own recovered goroutine; done closes and wait returns once its answers are set.
type batchRun struct {
	g     errgroup.Group
	done  chan struct{}
	res   []result
	stats batchStats
	err   error
}

func startBatch(ctx context.Context, b *batcher, reqs []request) *batchRun {
	r := &batchRun{done: make(chan struct{})}
	r.g.Go(func() error {
		defer close(r.done)

		r.res, r.stats, r.err = b.run(ctx, reqs)

		return nil
	})

	return r
}

func (r *batchRun) wait(t *testing.T) {
	t.Helper()
	require.NoError(t, r.g.Wait())
}

// awaitBackend returns the PID of the session named app once it waits on event.
func awaitBackend(ctx context.Context, t *testing.T, conn *pgx.Conn, app, event string) int32 {
	t.Helper()
	awaitPrimary(ctx, t, conn, app+" waits on "+event, backendWaits, app, event)

	var pid int32
	require.NoError(t, conn.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE application_name = $1`, app).Scan(&pid))

	return pid
}

// holdLock runs query in a transaction on a fresh connection, which cleanup closes; rolling back releases its locks.
func holdLock(ctx context.Context, t *testing.T, env *benchEnv, query string, args ...any) pgx.Tx {
	t.Helper()

	tx, err := connectPrimary(ctx, t, env).Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, query, args...)
	require.NoError(t, err)

	return tx
}

// newTestBatcher rebuilds and seeds the guarded stack with 4 ordinary balances, then returns a batcher on a session
// of its own, the primary connection that owns test setup, and the broker the producer must never reach.
func newTestBatcher(ctx context.Context, t *testing.T, env *benchEnv, clock *testClock) (*batcher, *pgx.Conn, *refusingBroker) {
	t.Helper()

	require.NoError(t, bootstrapSchema(ctx, env))
	require.NoError(t, seedFixtures(ctx, env, 4))

	_, emitter, repo, broker := newTestOutbox(ctx, t, env)

	b, err := newBatcher(openTestSession(t, env.primary), emitter, repo, clock.now)
	require.NoError(t, err)

	return b, connectPrimary(ctx, t, env), broker
}

// mv returns a posting builder on one seeded balance.
func mv(class string, i int, key string) func(int64) posting {
	return func(amount int64) posting {
		return posting{BalanceID: balanceID(class, i, key), AccountID: accountID(class, i), Amount: amount}
	}
}

// batchReq builds request name under usualBudget; tx names its transaction ID, so a new tx is a new payload.
func batchReq(t *testing.T, name, tx string, scheduledAt time.Time, postings ...posting) request {
	t.Helper()

	req, err := newRequest(uuid.NewSHA1(benchNS, []byte("batch/"+name)), uuid.NewSHA1(benchNS, []byte("batch/tx/"+tx)),
		postings, scheduledAt, usualBudget)
	require.NoError(t, err)

	return req
}

// summarize renders each answer as outcome[/original][/reason][+tx], checking its request, its transaction and that
// only answered positions carry decision and acknowledgement instants.
func summarize(t *testing.T, reqs []request, res []result) []string {
	t.Helper()
	require.Len(t, res, len(reqs))

	out := make([]string, len(res))

	for i, r := range res {
		assert.Equal(t, reqs[i].id, r.requestID)
		assert.Equal(t, r.outcome != outcomeUnknown, !r.decisionStartedAt.IsZero() && !r.dbAckAt.IsZero(), "position %d instants", i)

		parts := slices.DeleteFunc([]string{string(r.outcome), string(r.original), r.reason}, func(s string) bool { return s == "" })
		out[i] = strings.Join(parts, "/")

		if r.transactionID != uuid.Nil {
			assert.Equal(t, reqs[i].transactionID, r.transactionID, "position %d transaction", i)
			out[i] += "+tx"
		}
	}

	return out
}

// ledgerTotals is everything a batch may change, summed.
type ledgerTotals struct {
	Requests, Touches, Transactions, Operations, Outbox, Versions int64
	Available, Limits                                             string
}

func readTotals(ctx context.Context, t *testing.T, conn *pgx.Conn) ledgerTotals {
	t.Helper()

	var got ledgerTotals
	require.NoError(t, conn.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM engine_request), (SELECT coalesce(sum(durability_touch), 0) FROM engine_request),
		(SELECT count(*) FROM "transaction"), (SELECT count(*) FROM operation), (SELECT count(*) FROM outbox_events),
		(SELECT sum(version) FROM balance) + (SELECT sum(version) FROM account_control),
		(SELECT sum(available)::text FROM balance), (SELECT sum(limit_remaining)::text FROM account_control)`).
		Scan(&got.Requests, &got.Touches, &got.Transactions, &got.Operations, &got.Outbox, &got.Versions, &got.Available, &got.Limits))

	return got
}

// readRows maps the first text column of query to the second.
func readRows(ctx context.Context, t *testing.T, conn *pgx.Conn, query string) map[string]string {
	t.Helper()

	rows, err := conn.Query(ctx, query)
	require.NoError(t, err)

	got := map[string]string{}

	for rows.Next() {
		var k, v string
		require.NoError(t, rows.Scan(&k, &v))

		got[k] = v
	}

	require.NoError(t, rows.Err())

	return got
}

// assertBatchLedger checks what every committed batch leaves: no unfinalized claim, one transaction per approved
// request, every balance's operations chaining to its row, and an outbox that carries exactly the operation rows and
// the approved requests' private rows. It returns the outbox row count by kind.
func assertBatchLedger(ctx context.Context, t *testing.T, conn *pgx.Conn) map[string]int {
	t.Helper()

	var claims [4]int64
	require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FILTER (WHERE r.outcome IS NULL),
		count(*) FILTER (WHERE (r.outcome = 'approved') IS DISTINCT FROM (t.id IS NOT NULL)),
		count(*) FILTER (WHERE r.transaction_id IS NOT NULL AND t.id IS NULL),
		(SELECT count(*) FROM "transaction") - count(t.id)
		FROM engine_request r LEFT JOIN "transaction" t ON t.id = r.transaction_id`).Scan(&claims[0], &claims[1], &claims[2], &claims[3]))
	assert.Equal(t, [4]int64{}, claims, "unfinalized claims, approvals without a transaction, dangling or orphan transactions")

	var breaks int64
	require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT o.*, lag(o.balance_version_after) OVER w AS prev_version, lag(o.available_balance_after) OVER w AS prev_after,
			lead(o.id) OVER w IS NULL AS last
		FROM operation o WINDOW w AS (PARTITION BY o.balance_id ORDER BY o.balance_version_before)) c
		JOIN balance b ON b.id = c.balance_id
		WHERE c.balance_version_after <> c.balance_version_before + 1
		OR c.available_balance_after <> c.available_balance + CASE WHEN c.type = $1 THEN -c.amount ELSE c.amount END
		OR c.prev_version <> c.balance_version_before OR c.prev_after <> c.available_balance
		OR (c.last AND (b.version <> c.balance_version_after OR b.available <> c.available_balance_after))`, constant.DEBIT).Scan(&breaks))
	assert.Zero(t, breaks, "operations that do not chain each balance from version to version up to its row")

	return assertOutboxCarriesRows(ctx, t, conn)
}

// assertOutboxCarriesRows decodes every outbox row: transaction.posted carries each operation row once, as stored,
// balance.changed reports its operation's after state, and private rows belong to approved requests.
func assertOutboxCarriesRows(ctx context.Context, t *testing.T, conn *pgx.Conn) map[string]int {
	t.Helper()

	ops := readRows(ctx, t, conn, `SELECT id::text, concat_ws('|', transaction_id, balance_id, account_id, account_alias, balance_key, type,
		direction, amount, available_balance, on_hold_balance, balance_version_before, available_balance_after, on_hold_balance_after,
		balance_version_after, snapshot->>'overdraftUsedBefore', snapshot->>'overdraftUsedAfter') FROM operation`)
	approved := readRows(ctx, t, conn, `SELECT request_id::text, transaction_id::text FROM engine_request WHERE outcome = 'approved'`)

	rows, err := conn.Query(ctx, `SELECT event_type, aggregate_id::text, payload FROM outbox_events`)
	require.NoError(t, err)

	counts, carried := map[string]int{}, 0

	for rows.Next() {
		var (
			kind, aggregate string
			payload         []byte
		)

		require.NoError(t, rows.Scan(&kind, &aggregate, &payload))

		if kind != libStreaming.StreamingOutboxEventType {
			counts[kind]++

			assert.Contains(t, approved, aggregate, "private %s row of a request that was not approved", kind)

			if kind == kindTracerConfirm {
				var confirm tracerConfirm
				require.NoError(t, json.Unmarshal(payload, &confirm))
				assert.Equal(t, approved[aggregate], confirm.TransactionID)
			}

			continue
		}

		var envelope libStreaming.OutboxEnvelope
		require.NoError(t, json.Unmarshal(payload, &envelope))
		counts[envelope.DefinitionKey]++

		carried += assertEventCarriesRows(t, envelope, ops)
	}

	require.NoError(t, rows.Err())
	assert.Equal(t, len(ops), carried, "transaction.posted carries every operation row exactly once")

	return counts
}

// assertEventCarriesRows checks one streaming event against the operation rows and returns how many it carried.
func assertEventCarriesRows(t *testing.T, envelope libStreaming.OutboxEnvelope, ops map[string]string) int {
	t.Helper()

	switch envelope.DefinitionKey {
	case keyPosted:
		var posted events.TransactionPayload
		require.NoError(t, json.Unmarshal(envelope.Event.Payload, &posted))

		for _, raw := range posted.Operations {
			var op benchOperation
			require.NoError(t, json.Unmarshal(raw, &op))
			assert.Equal(t, ops[op.ID], strings.Join([]string{
				op.TransactionID, op.BalanceID, op.AccountID, op.AccountAlias, op.BalanceKey, op.Type, op.Direction,
				op.Amount.Value.String(), op.Balance.Available.String(), op.Balance.OnHold.String(), itoa(op.Balance.Version),
				op.BalanceAfter.Available.String(), op.BalanceAfter.OnHold.String(), itoa(op.BalanceAfter.Version),
				op.Balance.OverdraftUsed.String(), op.BalanceAfter.OverdraftUsed.String(),
			}, "|"), "operation %s as posted", op.ID)
		}

		return len(posted.Operations)
	case keyChanged:
		var changed events.BalanceChangedPayload
		require.NoError(t, json.Unmarshal(envelope.Event.Payload, &changed))

		fields := strings.Split(ops[changed.OperationID], "|")
		require.Len(t, fields, 16, "balance.changed of operation %s", changed.OperationID)
		assert.Equal(t, fields[11:14], []string{changed.Available.String(), changed.OnHold.String(), itoa(changed.Version)})
	}

	return 0
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
