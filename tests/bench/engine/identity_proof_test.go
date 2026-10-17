//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/errgroup"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Candidate identity statements on a model table in the proof's own schema. exp holds literal +/-1 hour expiries or a
// fixture-set instant, not product retention. idemRow renders tx/state/live|expired[/result tx]; "negative control"
// marks a contrast.
const (
	idemFP  = "identity-proof-fingerprint"
	idemRow = `concat_ws('/', tx, state, CASE WHEN exp > statement_timestamp() THEN 'live' ELSE 'expired' END, result->>'tx')`
	idemDDL = `CREATE SCHEMA identity_proof;
		CREATE TABLE identity_proof.idem (
			k text PRIMARY KEY,
			tx uuid NOT NULL,
			fp text NOT NULL CHECK (fp <> ''),
			state text NOT NULL CHECK (state IN ('inflight', 'done')),
			exp timestamptz NOT NULL,
			result jsonb,
			CHECK ((state = 'done') = (result IS NOT NULL)))`
	idemSeed = `INSERT INTO identity_proof.idem (k, tx, fp, state, exp, result) VALUES ($1, $2::uuid, $3, $4,
		statement_timestamp() + $5::interval, CASE WHEN $4 = 'done' THEN jsonb_build_object('tx', $2::uuid) END)`
	idemGate   = `SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0))::text`
	idemRoute  = `SELECT ` + idemRow + ` FROM identity_proof.idem WHERE k = $1`
	idemLock   = idemRoute + ` FOR UPDATE NOWAIT`
	idemTouch  = `UPDATE identity_proof.idem SET exp = exp WHERE k = $1`
	idemReplay = idemTouch + ` AND state = 'done' AND fp = $2 AND exp > statement_timestamp() RETURNING ` + idemRow
	idemExpire = `UPDATE identity_proof.idem SET exp = statement_timestamp() WHERE k = $1` // fixture: expires k now
	idemTake   = `UPDATE identity_proof.idem SET tx = $2, fp = $3, state = 'inflight',
		exp = statement_timestamp() + interval '1 hour', result = NULL WHERE k = $1`
	idemTakeover         = idemTake + ` RETURNING ` + idemRow
	idemTakeoverPreImage = idemTake + ` AND tx = $4 RETURNING ` + idemRow // negative control: fenced on the routed owner
	idemInsert           = `INSERT INTO identity_proof.idem (k, tx, fp, state, exp)
		VALUES ($1, $2, $3, 'inflight', statement_timestamp() + interval '1 hour') RETURNING ` + idemRow
	idemFinalize = `UPDATE identity_proof.idem SET state = 'done', result = jsonb_build_object('tx', tx),
		exp = statement_timestamp() + interval '1 hour' WHERE k = $1 AND tx = $2 AND state = 'inflight' RETURNING ` + idemRow
	idemDelete = `DELETE FROM identity_proof.idem WHERE k = $1 AND tx = $2 AND state = 'inflight' RETURNING ` + idemRow
	idemPick   = `DELETE FROM identity_proof.idem WHERE k = ANY (ARRAY (SELECT k FROM identity_proof.idem
		WHERE state = 'done' AND exp <= statement_timestamp() LIMIT $1 FOR UPDATE`
	idemPurge        = idemPick + ` SKIP LOCKED)) RETURNING k`
	idemPurgeWaiting = idemPick + `)) RETURNING k` // negative control: the purge waits on a held row
	idemXID          = `SELECT (pg_current_xact_id_if_assigned() IS NOT NULL)::text`
)

var (
	idemT1 = uuid.NewSHA1(benchNS, []byte("identity-proof/T1"))
	idemT2 = uuid.NewSHA1(benchNS, []byte("identity-proof/T2"))
)

// Model evidence for the candidate identity statements on this PostgreSQL 17 stack. It adopts no protocol: the product
// path stays Redis, and the table, keys, expiries and negative controls are the proof's own.
func TestIdentityRaceProof(t *testing.T) {
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()

	stack, err := openVerifiedStack(ctx, env)
	require.NoError(t, err)

	defer stack.close(ctx)

	createIdemSchema(ctx, t, env, stack)
	// Commits under the guarded on still return: the standby keeps flushing while replay is paused.
	pauseReplay(ctx, t, env, stack.standby)

	for _, c := range []struct {
		name string
		run  func(p *idemCase)
	}{
		{"Q1-takeover", q1Takeover},
		{"Q1a-finalized", q1Finalized},
		{"Q2-1-finalizing", q2Finalizing},
		{"Q2-2-finalized", q2Finalized},
		{"Q2-3-deleted", q2Deleted},
		{"Q2-4-fenced", q2Fenced},
		{"Q4a-gate", q4Gate},
		{"Q4b-row-lock", q4RowLock},
		{"Q4d-managed", q4Managed},
		{"Q5-purge", q5Purge},
	} {
		if !t.Run(c.name, func(t *testing.T) { runIdemCase(ctx, t, env, stack.primary, c.name, c.run) }) {
			return
		}
	}
}

// createIdemSchema refuses an identity_proof that already exists: it is an earlier run's evidence. Only a create that
// returned success registers the drop, which runs after the replay resume because pauseReplay registers later.
func createIdemSchema(ctx context.Context, t *testing.T, env *benchEnv, stack *verifiedStack) {
	t.Helper()

	var version, checksums, hints string
	require.NoError(t, stack.primary.QueryRow(ctx, `SELECT current_setting('server_version'),
		current_setting('data_checksums'), current_setting('wal_log_hints')`).Scan(&version, &checksums, &hints))
	t.Logf("server_version=%s data_checksums=%s wal_log_hints=%s", version, checksums, hints)

	for _, node := range []*pgx.Conn{stack.primary, stack.standby} {
		var absent bool
		require.NoError(t, node.QueryRow(ctx, `SELECT to_regnamespace('identity_proof') IS NULL`).Scan(&absent))
		require.True(t, absent, "identity_proof exists on %s: retained evidence, inspect it before removing it", node.Config().Host)
	}

	_, err := stack.primary.Exec(ctx, idemDDL)
	require.NoError(t, err, "create outcome unknown: any identity_proof left is kept as evidence")

	t.Cleanup(func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		conn, _, err := connectNode(dropCtx, env.primary, primaryClusterName, false)
		if err == nil {
			defer conn.Close(dropCtx)

			_, err = conn.Exec(dropCtx, `SET LOCAL synchronous_commit = 'remote_apply'; DROP SCHEMA identity_proof CASCADE`)
		}

		assert.NoError(t, err, "drop this run's identity_proof schema")
	})
}

// idemCase is one case: its context, sessions and the asynchronous statements it joins before they close.
type idemCase struct {
	t     *testing.T
	ctx   context.Context
	env   *benchEnv
	admin *pgx.Conn
	name  string
	wg    sync.WaitGroup
}

// runIdemCase gives a case an empty table and its own context. However the case ends, it cancels that context and joins
// every asynchronous statement before cleanup closes the case's sessions.
func runIdemCase(ctx context.Context, t *testing.T, env *benchEnv, admin *pgx.Conn, name string, run func(p *idemCase)) {
	t.Helper()

	caseCtx, cancel := context.WithCancel(ctx)
	p := &idemCase{t: t, ctx: caseCtx, env: env, admin: admin, name: name}

	defer func() {
		cancel()
		p.wg.Wait()
	}()

	_, err := admin.Exec(caseCtx, `TRUNCATE identity_proof.idem`)
	require.NoError(t, err)

	run(p)
}

func (p *idemCase) app(role string) string { return "bench-idem-" + p.name + "-" + role }

// begin opens role's session and a READ COMMITTED transaction on it; remoteApply makes its COMMIT wait for replay.
func (p *idemCase) begin(role string, remoteApply bool) (*sqlSession, *sql.Tx) {
	s := namedSession(p.t, p.env, p.app(role), nil)

	tx, err := s.begin(p.ctx)
	require.NoError(p.t, err)

	if remoteApply {
		_, err = tx.ExecContext(p.ctx, `SET LOCAL synchronous_commit = 'remote_apply'`)
		require.NoError(p.t, err)
	}

	return s, tx
}

// expect runs query in tx and requires exactly want, nil meaning no row.
func (p *idemCase) expect(tx *sql.Tx, want []string, query string, args ...any) {
	got, err := idemQuery(p.ctx, tx, query, args...)
	require.NoError(p.t, err, query)
	require.Equal(p.t, want, got, query)
}

// claim requires k's advisory try-lock result and the routing read in tx.
func (p *idemCase) claim(tx *sql.Tx, k, gate string, routed []string) {
	p.expect(tx, []string{gate}, idemGate, k)
	p.expect(tx, routed, idemRoute, k)
}

// requireNoWait requires the NOWAIT refusal itself: a lock_timeout cancel shares SQLSTATE 55P03.
func (p *idemCase) requireNoWait(tx *sql.Tx, k string) {
	_, err := idemQuery(p.ctx, tx, idemLock, k)

	var pgErr *pgconn.PgError
	require.ErrorAs(p.t, err, &pgErr)
	require.Equal(p.t, "55P03", pgErr.Code)
	require.Contains(p.t, pgErr.Message, "could not obtain lock on row")
}

// async runs fn on a lib-commons recovered goroutine that the case joins.
func (p *idemCase) async(fn func() error) *errgroup.Group {
	g := &errgroup.Group{}

	p.wg.Add(1)
	g.Go(func() error {
		defer p.wg.Done()

		return fn()
	})

	return g
}

// startRows runs query in tx asynchronously; the returned func waits for its rows.
func (p *idemCase) startRows(tx *sql.Tx, query string, args ...any) func() []string {
	var got []string

	g := p.async(func() (err error) {
		got, err = idemQuery(p.ctx, tx, query, args...)

		return err
	})

	return func() []string {
		require.NoError(p.t, g.Wait(), query)

		return got
	}
}

// immediate ends a transaction asynchronously and requires role's backend idle within 30 s: no SyncRep wait held it.
func (p *idemCase) immediate(role string, end func() error) {
	g := p.async(end)
	awaitPrimary(p.ctx, p.t, p.admin, p.app(role)+" ends without a replication wait", backendIdle, p.app(role))
	require.NoError(p.t, g.Wait())
}

// cancelHeld cancels pid's COMMIT, observed in SyncRep; its session must report the cancelled wait as unknown.
func (p *idemCase) cancelHeld(pid int32, commit *errgroup.Group) {
	var signalled bool
	require.NoError(p.t, p.admin.QueryRow(p.ctx, `SELECT pg_cancel_backend($1)`, pid).Scan(&signalled))
	require.True(p.t, signalled)
	require.ErrorContains(p.t, commit.Wait(), "commit outcome unknown: WARNING 01000: canceling wait for synchronous replication")
}

// seed stores name's row for id through the admin connection; life is '1 hour' (live) or '-1 hour' (expired).
func (p *idemCase) seed(name string, id uuid.UUID, state, life string) string {
	k := idemKey(name)

	_, err := p.admin.Exec(p.ctx, idemSeed, k, id, idemFP, state, life)
	require.NoError(p.t, err)

	return k
}

// stored reads k's row as the admin connection sees it.
func (p *idemCase) stored(k string) []string {
	var row string
	require.NoError(p.t, p.admin.QueryRow(p.ctx, idemRoute, k).Scan(&row))

	return []string{row}
}

// idemKey is the model's own key; the product key shape is not under test.
func idemKey(name string) string { return "identity-proof/" + name }

func idemDone(id uuid.UUID, life string) []string {
	return []string{id.String() + "/done/" + life + "/" + id.String()}
}

func idemInflight(id uuid.UUID, life string) []string {
	return []string{id.String() + "/inflight/" + life}
}

// idemQuery returns the one text column of query's rows in tx.
func idemQuery(ctx context.Context, tx *sql.Tx, query string, args ...any) (got []string, err error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer func() { err = errors.Join(err, rows.Close()) }()

	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}

		got = append(got, v)
	}

	return got, rows.Err()
}

// replayAfterExpiry routes k as T1's live done row in A, then the fixture expires it: a controlled transition, not a
// measured TTL. B claims, NOWAIT-locks and takes over the expired row, runs then and commits before A's replay touch.
func (p *idemCase) replayAfterExpiry(k string, remoteApply bool, then func(b *sql.Tx)) (*sqlSession, *sql.Tx, []string) {
	as, a := p.begin("a", remoteApply)
	p.expect(a, idemDone(idemT1, "live"), idemRoute, k)

	_, err := p.admin.Exec(p.ctx, idemExpire, k)
	require.NoError(p.t, err)

	bs, b := p.begin("b", false)
	p.claim(b, k, "true", idemDone(idemT1, "expired"))
	p.expect(b, idemDone(idemT1, "expired"), idemLock, k)
	p.expect(b, idemInflight(idemT2, "live"), idemTakeover, k, idemT2, idemFP)
	then(b)
	require.NoError(p.t, bs.commit(b))

	got, err := idemQuery(p.ctx, a, idemReplay, k, idemFP)
	require.NoError(p.t, err)

	return as, a, got
}

// After the takeover, the replay touch returns no row and assigns no xid, so its remote_apply COMMIT waits for no
// replication: a success built on the routed read would be unbarriered.
func q1Takeover(p *idemCase) {
	k := p.seed("k", idemT1, "done", "1 hour")
	as, a, got := p.replayAfterExpiry(k, true, func(*sql.Tx) {})
	require.Empty(p.t, got)
	p.expect(a, []string{"false"}, idemXID)
	p.immediate("a", func() error { return as.commit(a) })
}

// After the new owner takes over and finalizes, the replay touch returns T2's result, not the routed T1.
func q1Finalized(p *idemCase) {
	k := p.seed("k", idemT1, "done", "1 hour")
	as, a, got := p.replayAfterExpiry(k, false, func(b *sql.Tx) {
		p.expect(b, idemDone(idemT2, "live"), idemFinalize, k, idemT2)
	})
	require.Equal(p.t, idemDone(idemT2, "live"), got)
	require.NoError(p.t, as.commit(a))
}

// While the owner's finalize waits in SyncRep, a claimer still routes the expired in-flight version, and its NOWAIT
// lock refuses instead of deciding from that version.
func q2Finalizing(p *idemCase) {
	k := p.seed("k", idemT1, "inflight", "-1 hour")
	s1s, s1 := p.begin("s1", true)
	p.expect(s1, idemDone(idemT1, "live"), idemFinalize, k, idemT1)
	commit := p.async(func() error { return s1s.commit(s1) })
	pid := awaitBackend(p.ctx, p.t, p.admin, p.app("s1"), "SyncRep")

	_, s2 := p.begin("s2", false)
	p.claim(s2, k, "true", idemInflight(idemT1, "expired"))
	p.requireNoWait(s2, k)
	require.NoError(p.t, s2.Rollback())

	p.cancelHeld(pid, commit)
	assert.Equal(p.t, idemDone(idemT1, "live"), p.stored(k))
}

// A finalize that commits between the claimer's route and its lock is seen by the lock, so the claimer replays.
// Negative control: a takeover fenced only on the routed owner would overwrite the done result.
func q2Finalized(p *idemCase) {
	k := p.seed("k", idemT1, "inflight", "-1 hour")
	_, s2 := p.begin("s2", false)
	p.claim(s2, k, "true", idemInflight(idemT1, "expired"))

	s1s, s1 := p.begin("s1", false)
	p.expect(s1, idemDone(idemT1, "live"), idemFinalize, k, idemT1)
	require.NoError(p.t, s1s.commit(s1))

	p.expect(s2, idemDone(idemT1, "live"), idemLock, k)
	p.expect(s2, idemDone(idemT1, "live"), idemReplay, k, idemFP)
	p.expect(s2, idemInflight(idemT2, "live"), idemTakeoverPreImage, k, idemT2, idemFP, idemT1)
	require.NoError(p.t, s2.Rollback())
	assert.Equal(p.t, idemDone(idemT1, "live"), p.stored(k))
}

// An owner's delete that commits between the claimer's route and its lock leaves no row to lock; the claimer inserts.
func q2Deleted(p *idemCase) {
	k := p.seed("k", idemT1, "inflight", "-1 hour")
	s2s, s2 := p.begin("s2", false)
	p.claim(s2, k, "true", idemInflight(idemT1, "expired"))

	s1s, s1 := p.begin("s1", false)
	p.expect(s1, idemInflight(idemT1, "expired"), idemDelete, k, idemT1)
	require.NoError(p.t, s1s.commit(s1))

	p.expect(s2, nil, idemLock, k)
	p.expect(s2, idemInflight(idemT2, "live"), idemInsert, k, idemT2, idemFP)
	require.NoError(p.t, s2s.commit(s2))
	assert.Equal(p.t, idemInflight(idemT2, "live"), p.stored(k))
}

// A takeover decided on the locked expired version fences the slow owner: its finalize waits, matches nothing, yet locks
// T2's version. Its ROLLBACK returns without a replication wait, and only then does T2's owner finalize.
func q2Fenced(p *idemCase) {
	k := p.seed("k", idemT1, "inflight", "-1 hour")
	s2s, s2 := p.begin("s2", false)
	p.claim(s2, k, "true", idemInflight(idemT1, "expired"))
	p.expect(s2, idemInflight(idemT1, "expired"), idemLock, k)
	p.expect(s2, idemInflight(idemT2, "live"), idemTakeover, k, idemT2, idemFP)

	_, o := p.begin("o", true)
	finalize := p.startRows(o, idemFinalize, k, idemT1)
	awaitBackend(p.ctx, p.t, p.admin, p.app("o"), "Lock")
	require.NoError(p.t, s2s.commit(s2))
	require.Empty(p.t, finalize())
	p.expect(o, []string{"true"}, idemXID)

	o2s, o2 := p.begin("o2", false)
	next := p.startRows(o2, idemFinalize, k, idemT2)
	awaitBackend(p.ctx, p.t, p.admin, p.app("o2"), "Lock")
	p.immediate("o", o.Rollback)
	require.Equal(p.t, idemDone(idemT2, "live"), next())
	require.NoError(p.t, o2s.commit(o2))
	assert.Equal(p.t, idemDone(idemT2, "live"), p.stored(k))
}

// The gate stays held while the winner's COMMIT waits in SyncRep; the loser's try-lock, read and remote_apply COMMIT
// assign no xid and wait for nothing. Once the winner is released, a new claim takes the gate and routes its row.
func q4Gate(p *idemCase) {
	k := idemKey("k")
	s1s, s1 := p.begin("s1", true)
	p.claim(s1, k, "true", nil)
	p.expect(s1, []string{"false"}, idemXID)
	p.expect(s1, idemInflight(idemT1, "live"), idemInsert, k, idemT1, idemFP)
	commit := p.async(func() error { return s1s.commit(s1) })
	pid := awaitBackend(p.ctx, p.t, p.admin, p.app("s1"), "SyncRep")

	s2s, s2 := p.begin("s2", true)
	p.claim(s2, k, "false", nil)
	p.expect(s2, []string{"false"}, idemXID)
	p.immediate("s2", func() error { return s2s.commit(s2) })

	var waiting bool
	require.NoError(p.t, p.admin.QueryRow(p.ctx, inSyncRep, pid).Scan(&waiting))
	require.True(p.t, waiting, "the winner still waits in SyncRep")
	p.cancelHeld(pid, commit)

	again, err := s2s.begin(p.ctx)
	require.NoError(p.t, err)
	p.claim(again, k, "true", idemInflight(idemT1, "live"))
	require.NoError(p.t, again.Rollback())
}

// A row lock assigns an xid and writes WAL, so its COMMIT waits in SyncRep; Q2-4 observes that a ROLLBACK after such a
// lock does not.
func q4RowLock(p *idemCase) {
	k := p.seed("k", idemT1, "inflight", "-1 hour")
	ss, s := p.begin("s", true)
	p.claim(s, k, "true", idemInflight(idemT1, "expired"))
	p.expect(s, idemInflight(idemT1, "expired"), idemLock, k)
	p.expect(s, []string{"true"}, idemXID)
	commit := p.async(func() error { return ss.commit(s) })
	p.cancelHeld(awaitBackend(p.ctx, p.t, p.admin, p.app("s"), "SyncRep"), commit)
}

// On the library-managed ST pool, the replay touch's SET exp = exp writes the WAL a replay barrier needs: its COMMIT
// waits in SyncRep, and the managed commit reports the cancelled wait unknown.
func q4Managed(p *idemCase) {
	k := p.seed("k", idemT1, "done", "1 hour")
	conn := pin(p.ctx, p.t, openManagedST(p.ctx, p.t, p.env))

	tx, err := conn.BeginTx(p.ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	require.NoError(p.t, err)

	for _, set := range []string{`SET LOCAL client_min_messages = warning`, `SET LOCAL synchronous_commit = 'remote_apply'`} {
		_, err = tx.ExecContext(p.ctx, set)
		require.NoError(p.t, err)
	}

	var pid int32
	require.NoError(p.t, tx.QueryRowContext(p.ctx, `SELECT pg_backend_pid()`).Scan(&pid))
	p.expect(tx, idemDone(idemT1, "live"), idemReplay, k, idemFP)
	p.expect(tx, []string{"true"}, idemXID)

	commit := p.async(func() error { return commitManaged(conn, tx) })
	awaitPrimary(p.ctx, p.t, p.admin, "managed replay touch waits in SyncRep", inSyncRep, pid)
	p.cancelHeld(pid, commit)
}

// The purge skips a locked expired row and leaves in-flight and live rows; until it commits, its deleted key refuses a
// NOWAIT lock. Negative control: without SKIP LOCKED the purge waits on the held row.
func q5Purge(p *idemCase) {
	k1, k2 := p.seed("p1", idemT1, "done", "-1 hour"), p.seed("p2", idemT1, "done", "-1 hour")
	k3, k4 := p.seed("p3", idemT1, "inflight", "-1 hour"), p.seed("p4", idemT1, "done", "1 hour")

	_, h := p.begin("h", false)
	p.expect(h, idemDone(idemT1, "expired"), idemLock, k1)

	purges, purge := p.begin("p", false)
	p.expect(purge, []string{k2}, idemPurge, 10)

	_, c := p.begin("c", false)
	p.claim(c, k2, "true", idemDone(idemT1, "expired"))
	p.requireNoWait(c, k2)
	require.NoError(p.t, c.Rollback())
	require.NoError(p.t, purges.commit(purge))

	_, n := p.begin("n", false)
	waiting := p.startRows(n, idemPurgeWaiting, 10)
	awaitBackend(p.ctx, p.t, p.admin, p.app("n"), "Lock")
	require.NoError(p.t, h.Rollback())
	require.Equal(p.t, []string{k1}, waiting())
	require.NoError(p.t, n.Rollback())

	var keys []string
	require.NoError(p.t, p.admin.QueryRow(p.ctx, `SELECT array_agg(k ORDER BY k) FROM identity_proof.idem`).Scan(&keys))
	assert.Equal(p.t, []string{k1, k3, k4}, keys)
}
