//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/errgroup"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const (
	// shortLease lets a test wait for expiry on the database clock; longLease outlives every test step.
	shortLease   = time.Second
	longLease    = time.Minute
	renewRefused = "renew fence: 0 rows, want 1"

	fenceExpired = `SELECT lease_until <= clock_timestamp() FROM engine_writer_fence WHERE tenant_id = $1`
	leaseExpired = `SELECT $1::timestamptz <= clock_timestamp()`
	readLease    = `SELECT lease_until FROM engine_writer_fence WHERE tenant_id = $1`

	holdBalances = `LOCK TABLE balance IN ACCESS EXCLUSIVE MODE`
	holdControls = `LOCK TABLE account_control IN ACCESS EXCLUSIVE MODE`
	// waitsOnTable is true while the session named $1 waits for a lock on table $2.
	waitsOnTable = `SELECT EXISTS (SELECT 1 FROM pg_locks l JOIN pg_stat_activity a ON a.pid = l.pid
		WHERE a.application_name = $1 AND NOT l.granted AND l.relation = $2::text::regclass)`
)

// A live lease refuses every takeover. Only its owner and epoch renew it, and only while the database clock has not
// passed it, even inside a transaction that began before it expired.
func TestOwnerFenceLease(t *testing.T) {
	noLeaks(t)
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	conn := newFenceFixture(ctx, t, env)
	a, b := openTestSession(t, env.primary), openTestSession(t, env.primary)

	first, err := acquireFence(ctx, a, "owner-a", longLease)
	require.NoError(t, err)
	assert.Equal(t, ownerToken{owner: "owner-a", epoch: 1}, first, "takeover of the seeded expired fence")

	_, err = acquireFence(ctx, b, "owner-b", longLease)
	require.ErrorIs(t, err, sql.ErrNoRows, "a live lease refuses another takeover")

	held, acquired := readFence(ctx, t, conn)
	assert.Equal(t, first, held)

	require.NoError(t, renewFence(ctx, a, first, longLease))

	_, renewed := readFence(ctx, t, conn)
	assert.True(t, renewed.After(acquired), "renewal moves the lease on the database clock")

	// Shorten the lease, then begin a pass before it expires and renew only after the database clock passed it.
	require.NoError(t, renewFence(ctx, a, first, shortLease))

	tx, err := a.begin(ctx)
	require.NoError(t, err)

	awaitPrimary(ctx, t, conn, "the lease expires on the database clock", fenceExpired, benchTenantID)
	require.ErrorContains(t, renewFenceTx(ctx, tx, first, longLease), renewRefused, "owner and epoch still match")

	var begunLive bool
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT now() < lease_until FROM engine_writer_fence WHERE tenant_id = $1`,
		benchTenantID).Scan(&begunLive))
	require.True(t, begunLive, "the pass began before the lease expired")
	require.NoError(t, tx.Rollback())

	// The same owner name takes over again: only the epoch tells the finished ownership's token apart.
	second, err := acquireFence(ctx, a, "owner-a", longLease)
	require.NoError(t, err)
	assert.Equal(t, ownerToken{owner: "owner-a", epoch: 2}, second)
	require.ErrorContains(t, renewFence(ctx, a, first, longLease), renewRefused, "token of a finished epoch")
	require.NoError(t, renewFence(ctx, a, second, longLease))
}

// A takeover that found the committed lease expired waits on the fence row a renewal holds, then rechecks the
// committed renewal on the database clock read at recheck: a renewal expired by then yields, a live one keeps its owner.
func TestOwnerFenceTakeoverRechecksRenewal(t *testing.T) {
	noLeaks(t)
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	conn := newFenceFixture(ctx, t, env)
	a, b, c := namedSession(t, env, "owner-a", nil), namedSession(t, env, "owner-b", nil), namedSession(t, env, "owner-c", nil)

	tokA, err := acquireFence(ctx, a, "owner-a", shortLease)
	require.NoError(t, err)

	held, until := holdRenewal(ctx, t, a, tokA, 2*shortLease)

	awaitPrimary(ctx, t, conn, "owner-a's committed lease expires", fenceExpired, benchTenantID)

	takeover := startAcquire(ctx, b, "owner-b")

	awaitBackend(ctx, t, conn, "owner-b", "Lock")

	var expired bool
	require.NoError(t, conn.QueryRow(ctx, leaseExpired, until).Scan(&expired))
	require.False(t, expired, "owner-b's statement started before the held renewal expires")

	awaitPrimary(ctx, t, conn, "the renewal owner-a holds expires too", leaseExpired, until)
	require.NoError(t, a.commit(held))

	tokB, err := takeover.wait(t)
	require.NoError(t, err)
	assert.Equal(t, ownerToken{owner: "owner-b", epoch: 2}, tokB, "acquired once the committed renewal expired")

	held, _ = holdRenewal(ctx, t, b, tokB, longLease)

	awaitPrimary(ctx, t, conn, "owner-b's committed lease expires", fenceExpired, benchTenantID)

	refused := startAcquire(ctx, c, "owner-c")

	awaitBackend(ctx, t, conn, "owner-c", "Lock")
	require.NoError(t, b.commit(held))

	_, err = refused.wait(t)
	require.ErrorIs(t, err, sql.ErrNoRows, "the committed renewal is live when the waiter rechecks")

	stored, _ := readFence(ctx, t, conn)
	assert.Equal(t, tokB, stored)
	require.NoError(t, renewFence(ctx, b, tokB, longLease), "owner-b keeps the fence")
}

// A takeover or renewal whose COMMIT waited in SyncRep and was cancelled committed locally, yet returns no token and
// no renewal. The renewal keeps the fence row locked through that wait: a takeover that found the visible lease
// expired waits on that renewal alone, then rechecks it and is refused.
func TestOwnerFenceUnknownCommit(t *testing.T) {
	noLeaks(t)
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	conn := newFenceFixture(ctx, t, env)

	stack, err := openVerifiedStack(ctx, env)
	require.NoError(t, err)

	defer stack.close(ctx)

	const app, otherApp = "bench-fence-syncrep", "bench-fence-other"

	s := namedSession(t, env, app, map[string]string{"synchronous_commit": "remote_apply"})
	flushOnly := namedSession(t, env, otherApp, nil)

	pauseReplay(ctx, t, env, stack.standby)

	takeover := startAcquire(ctx, s, "owner-x")
	cancelSyncRep(ctx, t, conn, app)

	tok, err := takeover.wait(t)
	require.ErrorContains(t, err, "canceling wait for synchronous replication")
	assert.Zero(t, tok, "an unknown COMMIT yields no token")

	stored, _ := readFence(ctx, t, conn)
	assert.Equal(t, ownerToken{owner: "owner-x", epoch: 1}, stored, "the takeover committed locally")

	awaitPrimary(ctx, t, conn, "the unconfirmed lease expires", fenceExpired, benchTenantID)

	// synchronous_commit=on waits for standby flush only, which continues while replay is paused.
	tokY, err := acquireFence(ctx, flushOnly, "owner-y", shortLease)
	require.NoError(t, err)

	renew := &errgroup.Group{}
	renew.Go(func() error { return renewFence(ctx, s, tokY, longLease) })

	renewPID := awaitBackend(ctx, t, conn, app, "SyncRep")

	awaitPrimary(ctx, t, conn, "owner-y's visible lease expires while its renewal waits in SyncRep", fenceExpired, benchTenantID)

	refused := startAcquire(ctx, flushOnly, "owner-z")
	waiterPID := awaitBackend(ctx, t, conn, otherApp, "Lock")

	var blocked bool
	require.NoError(t, conn.QueryRow(ctx, `SELECT pg_blocking_pids($1) = ARRAY[$2::int]
		AND EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid = $2::int AND wait_event = 'SyncRep')`, waiterPID, renewPID).Scan(&blocked))
	require.True(t, blocked, "owner-z waits only on the renewal, whose COMMIT still waits in SyncRep")

	cancelSyncRep(ctx, t, conn, app)
	require.ErrorContains(t, renew.Wait(), "canceling wait for synchronous replication")

	_, err = refused.wait(t)
	require.ErrorIs(t, err, sql.ErrNoRows, "the waiter rechecks the unconfirmed renewal and finds its lease live")

	stored, _ = readFence(ctx, t, conn)
	assert.Equal(t, tokY, stored)
}

// A reload pages every live balance by id and every control by its whole key, renewing between pages. Held tables
// make its pages of 3 run past the lease it acquired with; its final renewal still succeeds.
func TestOwnerReloadPages(t *testing.T) {
	noLeaks(t)
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	conn := newFenceFixture(ctx, t, env)
	addOtherLedger(ctx, t, conn)

	// PostgreSQL renders the expected rows, not the scanners under test. Pages: balances 3, 3, 3, 0; controls 3, 3, 2.
	wantBalances := readRows(ctx, t, conn, `SELECT id::text, concat_ws(' ', account_id, alias, key, asset_code, account_type,
		allow_sending::text, allow_receiving::text, available, on_hold, version, overdraft_used) FROM balance WHERE deleted_at IS NULL`)
	wantControls := readRows(ctx, t, conn, `SELECT concat_ws(' ', organization_id, ledger_id, account_id),
		concat_ws(' ', blocked::text, limit_remaining, version) FROM account_control`)
	require.Len(t, wantBalances, 9)
	require.Len(t, wantControls, 8)

	const app, lease = "bench-reload-pages", 3 * time.Second

	s := namedSession(t, env, app, nil)
	balanceLock, controlLock := holdLock(ctx, t, env, holdBalances), holdLock(ctx, t, env, holdControls)

	tok, err := acquireFence(ctx, s, "owner-a", lease)
	require.NoError(t, err)

	_, acquired := readFence(ctx, t, conn)

	everyPage := make(chan time.Time)
	close(everyPage)

	run := startReload(ctx, t, s, tok, lease, everyPage, 3)

	awaitPrimary(ctx, t, conn, "the reload waits on balance", waitsOnTable, app, "balance")
	awaitPrimary(ctx, t, conn, "half the acquired lease passed", leaseExpired, acquired.Add(-lease/2))
	require.NoError(t, balanceLock.Rollback(ctx))

	awaitPrimary(ctx, t, conn, "the reload waits on account_control", waitsOnTable, app, "account_control")
	awaitPrimary(ctx, t, conn, "the acquired lease expired", leaseExpired, acquired)
	require.NoError(t, controlLock.Rollback(ctx))

	require.NoError(t, run.wait(t))

	gotBalances, gotControls := map[string]string{}, map[string]string{}
	for id, b := range run.balances {
		gotBalances[id.String()] = fmt.Sprintf("%s %s %s %s %s %t %t %s %s %d %s", b.accountID, b.alias, b.key, b.asset,
			b.accountType, b.allowSending, b.allowReceiving, b.Available, b.OnHold, b.Version, b.OverdraftUsed)
	}

	for k, c := range run.controls {
		gotControls[fmt.Sprintf("%s %s %s", k.org, k.ledger, k.account)] = fmt.Sprintf("%t %s %d", c.blocked, c.limit, c.version)
	}

	assert.Equal(t, wantBalances, gotBalances)
	assert.Equal(t, wantControls, gotControls)
}

// The renewal after the last page decides the reload: a successor that takes over while that page waits refuses it,
// and the reload returns neither map.
func TestOwnerReloadExpiredPage(t *testing.T) {
	noLeaks(t)
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	conn := newFenceFixture(ctx, t, env)

	const app = "bench-reload-expired"

	a, b := namedSession(t, env, app, nil), openTestSession(t, env.primary)
	controlLock := holdLock(ctx, t, env, holdControls)

	tokA, err := acquireFence(ctx, a, "owner-a", shortLease)
	require.NoError(t, err)

	run := startReload(ctx, t, a, tokA, shortLease, nil, 50_000)

	awaitBackend(ctx, t, conn, app, "Lock")
	awaitPrimary(ctx, t, conn, "owner-a's lease expires while its last page waits", fenceExpired, benchTenantID)

	tokB, err := acquireFence(ctx, b, "owner-b", longLease)
	require.NoError(t, err)
	require.NoError(t, controlLock.Rollback(ctx))

	require.ErrorContains(t, run.wait(t), renewRefused)
	assert.Nil(t, run.balances)
	assert.Nil(t, run.controls)

	held, _ := readFence(ctx, t, conn)
	assert.Equal(t, tokB, held, "the successor keeps the fence")
}

// Cancelling the control read discards the balances already read and returns neither map.
func TestOwnerReloadCancelled(t *testing.T) {
	noLeaks(t)
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	conn := newFenceFixture(ctx, t, env)

	const app = "bench-reload-cancelled"

	s := namedSession(t, env, app, nil)
	controlLock := holdLock(ctx, t, env, holdControls)

	tok, err := acquireFence(ctx, s, "owner-a", longLease)
	require.NoError(t, err)

	reloadCtx, stop := context.WithCancel(ctx)
	run := startReload(reloadCtx, t, s, tok, longLease, nil, 50_000)

	awaitPrimary(ctx, t, conn, "the reload waits on account_control", waitsOnTable, app, "account_control")
	stop()

	require.ErrorIs(t, run.wait(t), context.Canceled)
	assert.Nil(t, run.balances)
	assert.Nil(t, run.controls)
	require.NoError(t, controlLock.Rollback(ctx))
}

// Idle ticks keep an owner's lease live: a takeover is refused, a payment approves. A pass parked past its renewed
// lease holds the fence row: a successor waits on it, the COMMIT approves, then the successor uses the pass's new state.
// The next renewal refuses: the lost owner claims nothing, answers unknown without SQL, and Close reports that cause.
func TestOwnerWriterLease(t *testing.T) {
	noLeaks(t)
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, broker := newTestBatcher(ctx, t, env, clock.now)
	hot := mv(classHot, 0, constant.DefaultBalanceKey)

	snapshotBaseline(ctx, t, conn)

	const app, rivalApp, successorApp = "bench-owner-lease", "bench-owner-rival", "bench-owner-lease-successor"

	inner, err := startOwner(ctx, t, namedBatcher(t, b, env, app, nil), "owner-a", gateMaxBatch).wait(t)
	require.NoError(t, err)

	w := captureCalls(inner, 4)
	_, started := readFence(ctx, t, conn)

	awaitPrimary(ctx, t, conn, "the lease the owner started with expires", leaseExpired, started)

	rival := namedSession(t, env, rivalApp, nil)
	_, err = acquireFence(ctx, rival, "owner-b", shortLease)
	require.ErrorIs(t, err, sql.ErrNoRows, "idle ticks keep the lease live")

	idle := batchReq(t, "owner/lease/idle", "owner/lease/idle", clock.now(), hot(-1), ordinaryOf(0)(1))
	assert.Equal(t, []string{"approved+tx"}, summarize(t, []request{idle}, []result{w.Submit(ctx, idle)}))

	row := holdLock(ctx, t, env, lockBalance, hot(0).BalanceID)
	parked := batchReq(t, "owner/lease/parked", "owner/lease/parked", clock.now(), hot(-1), ordinaryOf(1)(1))
	paying := submitEach(ctx, t, w, []request{parked})

	ownerPID := awaitBackend(ctx, t, conn, app, "Lock") // its UPDATE waits on the held balance

	var waitAt time.Time
	require.NoError(t, conn.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&waitAt))
	awaitPrimary(ctx, t, conn, "the lease the pass renewed before waiting expires", leaseExpired, waitAt.Add(ownerLease))

	successor := startOwner(ctx, t, namedBatcher(t, b, env, successorApp, nil), "owner-b", gateMaxBatch)
	successorPID := awaitBackend(ctx, t, conn, successorApp, "Lock")

	var blocked bool
	require.NoError(t, conn.QueryRow(ctx, `SELECT pg_blocking_pids($1) = ARRAY[$2::int]`, successorPID, ownerPID).Scan(&blocked))
	require.True(t, blocked, "the takeover waits only on the fence row the pass holds")
	require.NoError(t, row.Rollback(ctx))

	assert.Equal(t, []string{"approved+tx"}, summarize(t, []request{parked}, paying.wait(t)))

	innerB, err := successor.wait(t)
	require.NoError(t, err)

	tokB, _ := readFence(ctx, t, conn)
	assert.Equal(t, ownerToken{owner: "owner-b", epoch: 2}, tokB, "the takeover follows the approval's COMMIT")

	next := batchReq(t, "owner/lease/next", "owner/lease/next", clock.now(), hot(-1), ordinaryOf(2)(1))
	assertUnknown(t, []result{w.Submit(ctx, next)}, renewRefused)

	ran := queryStart(ctx, t, conn, app)
	later := batchReq(t, "owner/lease/later", "owner/lease/later", clock.now(), ordinaryOf(2)(-1), ordinaryOf(3)(1))
	assertUnknown(t, []result{w.Submit(ctx, later)}, renewRefused)
	assert.Equal(t, ran, queryStart(ctx, t, conn, app), "the lost owner ran no SQL")

	require.ErrorContains(t, w.Close(ctx), renewRefused, "Close reports the first cause")
	assert.Equal(t, map[string]string{idle.id.String(): "approved", parked.id.String(): "approved"},
		readRows(ctx, t, conn, `SELECT request_id::text, outcome FROM engine_request`), "the lost owner claimed nothing")

	wB := captureCalls(innerB, 1)
	again := batchReq(t, "owner/lease/again", "owner/lease/again", clock.now(), hot(-1), ordinaryOf(3)(1))
	assert.Equal(t, []string{"approved+tx"}, summarize(t, []request{again}, []result{wB.Submit(ctx, again)}),
		"the successor's reload read the versions the parked pass left")
	require.NoError(t, wB.Close(ctx))
	assert.Equal(t, map[string]int{"approved": 3, "unknown": 2}, verifyLedger(ctx, t, conn, slices.Concat(w.calls, wB.calls)))
	assert.Zero(t, broker.publishes.Load(), "obligations stay in the outbox")
}

// A financial COMMIT cancelled in SyncRep committed locally yet answers unknown and ends the owner, which answers later
// attempts unknown without SQL. Its successor holds nothing while the standby lacks the original, then replays it past
// its cutoff without moving money and approves on the balances the original moved.
func TestOwnerWriterSuccessor(t *testing.T) {
	noLeaks(t)
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, broker := newTestBatcher(ctx, t, env, clock.now)

	stack, err := openVerifiedStack(ctx, env)
	require.NoError(t, err)

	defer stack.close(ctx)

	snapshotBaseline(ctx, t, conn)

	const appA, appB, syncRepCancelled = "bench-owner-first", "bench-owner-successor", "canceling wait for synchronous replication"

	remoteApply := map[string]string{"synchronous_commit": "remote_apply"}

	innerA, err := startOwner(ctx, t, namedBatcher(t, b, env, appA, remoteApply), "owner-a", gateMaxBatch).wait(t)
	require.NoError(t, err)

	first := captureCalls(innerA, 2)
	hot := mv(classHot, 0, constant.DefaultBalanceKey)
	original := batchReq(t, "owner/successor/original", "owner/successor/original", clock.now(), hot(-5), ordinaryOf(0)(5))

	outbox := holdLock(ctx, t, env, `LOCK TABLE outbox_events IN SHARE MODE`)
	paying := submitEach(ctx, t, first, []request{original})

	awaitBackend(ctx, t, conn, appA, "relation") // inside its financial pass, writing the approval's events
	pauseReplay(ctx, t, env, stack.standby)
	require.NoError(t, outbox.Rollback(ctx))
	cancelSyncRep(ctx, t, conn, appA)
	assertUnknown(t, paying.wait(t), syncRepCancelled)

	ran := queryStart(ctx, t, conn, appA)
	untouched := batchReq(t, "owner/successor/untouched", "owner/successor/untouched", clock.now(), ordinaryOf(2)(-1), ordinaryOf(3)(1))
	assertUnknown(t, []result{first.Submit(ctx, untouched)}, syncRepCancelled)
	assert.Equal(t, ran, queryStart(ctx, t, conn, appA), "the lost owner ran no SQL")
	require.ErrorContains(t, first.Close(ctx), syncRepCancelled)

	awaitPrimary(ctx, t, conn, "the lost owner's lease expires", fenceExpired, benchTenantID)

	successor := startOwner(ctx, t, namedBatcher(t, b, env, appB, remoteApply), "owner-b", gateMaxBatch)

	awaitBackend(ctx, t, conn, appB, "SyncRep")
	require.False(t, successor.returned(), "no ownership while its acquisition waits for the standby")
	assert.Zero(t, countTransactions(ctx, t, stack.standby, []request{original}), "the paused standby lacks the original")

	_, err = stack.standby.Exec(ctx, `SELECT pg_wal_replay_resume()`)
	require.NoError(t, err)

	innerB, err := successor.wait(t)
	require.NoError(t, err)

	second := captureCalls(innerB, 2)

	clock.advance(time.Minute) // past the original's cutoff and response deadline

	retry := original
	retry.deadline = clock.now().Add(usualBudget.caller)
	want := readTotals(ctx, t, conn)

	assert.Equal(t, []string{"replay/approved+tx"}, summarize(t, []request{retry}, []result{second.Submit(ctx, retry)}))

	want.Touches++
	assert.Equal(t, want, readTotals(ctx, t, conn), "the replay only touched its identity")

	again := batchReq(t, "owner/successor/again", "owner/successor/again", clock.now(), hot(-5), ordinaryOf(0)(5))
	assert.Equal(t, []string{"approved+tx"}, summarize(t, []request{again}, []result{second.Submit(ctx, again)}),
		"the reload read the versions the original left")
	require.NoError(t, second.Close(ctx))

	assert.Equal(t, map[string]int{"unknown": 2, "replay/approved": 1, "approved": 1, unansweredApproval: 1},
		verifyLedger(ctx, t, conn, slices.Concat(first.calls, second.calls)))
	assert.Zero(t, broker.publishes.Load(), "obligations stay in the outbox")
}

// An idle renewal's deadline ends the owner while replay is paused. No later renewal keeps the fence,
// and Close, with nothing ever submitted, reports that cause.
func TestOwnerWriterIdleLoss(t *testing.T) {
	noLeaks(t)
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, _ := newTestBatcher(ctx, t, env, clock.now)

	stack, err := openVerifiedStack(ctx, env)
	require.NoError(t, err)

	defer stack.close(ctx)

	const app = "bench-owner-idle"

	w, err := startOwner(ctx, t, namedBatcher(t, b, env, app, map[string]string{"synchronous_commit": "remote_apply"}),
		"owner-a", gateMaxBatch).wait(t)
	require.NoError(t, err)

	pauseReplay(ctx, t, env, stack.standby)
	awaitBackend(ctx, t, conn, app, "SyncRep") // a tick renewal, not a financial pass: nothing is submitted
	awaitPrimary(ctx, t, conn, "the renewal leaves SyncRep while replay stays paused",
		`SELECT NOT EXISTS (SELECT 1 FROM pg_stat_activity WHERE application_name = $1 AND wait_event = 'SyncRep')`, app)

	_, err = stack.standby.Exec(ctx, `SELECT pg_wal_replay_resume()`)
	require.NoError(t, err)

	awaitPrimary(ctx, t, conn, "no later renewal: the lease expires while the writer is open", fenceExpired, benchTenantID)
	require.ErrorContains(t, w.Close(ctx), "renew on tick")
}

// ownerStart is one owner constructor on its own recovered goroutine. Registered after its session, its cleanup runs
// before that session closes: it joins the constructor, which a failed test's ended ctx releases, then closes a
// writer the test left open.
type ownerStart struct {
	g    errgroup.Group
	done chan struct{}
	w    *ownerlessWriter
	err  error
}

func startOwner(ctx context.Context, t *testing.T, b *batcher, owner string, maxBatch int) *ownerStart {
	t.Helper()

	s := &ownerStart{done: make(chan struct{})}
	s.g.Go(func() error {
		defer close(s.done)

		s.w, s.err = newOwnerWriter(ctx, b, owner, maxBatch)

		return nil
	})

	t.Cleanup(func() {
		assert.NoError(t, s.g.Wait())

		if s.w != nil && !s.w.closed.Load() {
			closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			t.Logf("close after a failure: %v", s.w.Close(closeCtx))
		}
	})

	return s
}

func (s *ownerStart) returned() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *ownerStart) wait(t *testing.T) (*ownerlessWriter, error) {
	t.Helper()
	require.NoError(t, s.g.Wait())

	return s.w, s.err
}

// queryStart returns when the session named app last started a statement.
func queryStart(ctx context.Context, t *testing.T, conn *pgx.Conn, app string) time.Time {
	t.Helper()

	var at time.Time
	require.NoError(t, conn.QueryRow(ctx, `SELECT query_start FROM pg_stat_activity WHERE application_name = $1`, app).Scan(&at))

	return at
}

// addOtherLedger adds a second organization and ledger with one account: a live balance, a soft-deleted one on the
// same key and a control, each value distinct. The org sorts after the seeded one and the account before every
// seeded account, so a control cursor on account alone, or on each column apart, loses that control.
func addOtherLedger(ctx context.Context, t *testing.T, conn *pgx.Conn) {
	t.Helper()

	org, ledger := uuid.MustParse("ffffffff-ffff-4fff-bfff-ffffffffffff"), uuid.NewSHA1(benchNS, []byte("reload/ledger"))
	account := uuid.MustParse("00000000-0000-4000-8000-000000000001")

	_, err := conn.Exec(ctx, `INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, asset_code, available,
		on_hold, version, account_type, allow_sending, allow_receiving, created_at, updated_at, deleted_at, key, overdraft_used)
		VALUES ($1, $3, $4, $5, '@reload_other', 'USD', 98765432109876543210.123456789, 7.25, 5, 'external', false, true, $6, $6, NULL, 'reload', 3.5),
		($2, $3, $4, $5, '@reload_other', 'USD', 1, 0, 1, 'external', true, false, $6, $6, $6, 'reload', 0)`,
		uuid.NewSHA1(benchNS, []byte("reload/balance/live")), uuid.NewSHA1(benchNS, []byte("reload/balance/deleted")),
		org, ledger, account, benchSeedTime)
	require.NoError(t, err)

	_, err = conn.Exec(ctx, `INSERT INTO account_control VALUES ($1, $2, $3, true, 123456789.987654321, 9)`, org, ledger, account)
	require.NoError(t, err)
}

// reloadRun is one reload on its own recovered goroutine. A test that fails first has cancelled ctx by the time its
// cleanup joins the run.
type reloadRun struct {
	g        errgroup.Group
	balances map[uuid.UUID]lockedBalance
	controls map[controlKey]lockedControl
	err      error
}

func startReload(ctx context.Context, t *testing.T, s *sqlSession, tok ownerToken, lease time.Duration,
	ticks <-chan time.Time, pageSize int,
) *reloadRun {
	t.Helper()

	r := &reloadRun{}
	r.g.Go(func() error {
		r.balances, r.controls, r.err = reload(ctx, s, tok, lease, ticks, pageSize)

		return nil
	})
	t.Cleanup(func() { assert.NoError(t, r.g.Wait()) })

	return r
}

func (r *reloadRun) wait(t *testing.T) error {
	t.Helper()
	require.NoError(t, r.g.Wait())

	return r.err
}

// newFenceFixture rebuilds and seeds the guarded stack, whose fence starts expired at epoch 0, and returns the
// primary connection that observes it.
func newFenceFixture(ctx context.Context, t *testing.T, env *benchEnv) *pgx.Conn {
	t.Helper()

	require.NoError(t, bootstrapSchema(ctx, env))
	require.NoError(t, seedFixtures(ctx, env, 4))

	return connectPrimary(ctx, t, env)
}

func readFence(ctx context.Context, t *testing.T, conn *pgx.Conn) (ownerToken, time.Time) {
	t.Helper()

	var (
		tok   ownerToken
		until time.Time
	)

	require.NoError(t, conn.QueryRow(ctx, `SELECT owner_id, epoch, lease_until FROM engine_writer_fence WHERE tenant_id = $1`,
		benchTenantID).Scan(&tok.owner, &tok.epoch, &until))

	return tok, until
}

// holdRenewal renews tok in a transaction it leaves open, holding the fence row locked, and returns it with the
// lease it wrote, which no other session sees before COMMIT. A test that fails first ends it by cancelling ctx.
func holdRenewal(ctx context.Context, t *testing.T, s *sqlSession, tok ownerToken, lease time.Duration) (*sql.Tx, time.Time) {
	t.Helper()

	tx, err := s.begin(ctx)
	require.NoError(t, err)
	require.NoError(t, renewFenceTx(ctx, tx, tok, lease))

	var until time.Time
	require.NoError(t, tx.QueryRowContext(ctx, readLease, benchTenantID).Scan(&until))

	return tx, until
}

// cancelSyncRep cancels the backend of the session named app once its COMMIT waits in SyncRep.
func cancelSyncRep(ctx context.Context, t *testing.T, conn *pgx.Conn, app string) {
	t.Helper()

	pid := awaitBackend(ctx, t, conn, app, "SyncRep")

	var signalled bool
	require.NoError(t, conn.QueryRow(ctx, `SELECT pg_cancel_backend($1)`, pid).Scan(&signalled))
	require.True(t, signalled)
}

// acquireRun is one acquireFence of a shortLease on its own recovered goroutine; wait returns its result.
type acquireRun struct {
	g   errgroup.Group
	tok ownerToken
	err error
}

func startAcquire(ctx context.Context, s *sqlSession, owner string) *acquireRun {
	r := &acquireRun{}
	r.g.Go(func() error {
		r.tok, r.err = acquireFence(ctx, s, owner, shortLease)

		return nil
	})

	return r
}

func (r *acquireRun) wait(t *testing.T) (ownerToken, error) {
	t.Helper()
	require.NoError(t, r.g.Wait())

	return r.tok, r.err
}
