//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/errgroup"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
