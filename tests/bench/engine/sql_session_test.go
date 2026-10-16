//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"database/sql"
	"maps"
	"testing"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/backoff"
	"github.com/LerianStudio/lib-commons/v7/commons/errgroup"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	inSyncRep        = `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid = $1 AND wait_event = 'SyncRep')`
	rowExists        = `SELECT EXISTS (SELECT 1 FROM account_control WHERE account_id = $1)`
	committedAndGone = rowExists + ` AND NOT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid = $2)`
	// commitDeadline only has to outlast observing the backend in SyncRep, which the test checks before it expires.
	commitDeadline = 5 * time.Second
)

// While standby replay is paused, a COMMIT under remote_apply waits in SyncRep until it is cancelled. A cancelled
// wait and a pgx deadline both commit locally, so the session must report them unknown, whatever its config does
// to warnings, and later commits clean.
func TestSQLSessionCommit(t *testing.T) {
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	// The guarded rebuild empties account_control, so every row below is this test's own.
	require.NoError(t, bootstrapSchema(ctx, env))

	stack, err := openVerifiedStack(ctx, env)
	require.NoError(t, err)

	defer stack.close(ctx)

	// Session A's config filters warnings out, as a DSN or PGOPTIONS legitimately may; B keeps the default.
	hostile := env.primary.Copy()
	hostile.RuntimeParams["options"] = "-c client_min_messages=error"
	hostile.RuntimeParams["Client_Min_Messages"] = "error"
	a, b := openTestSession(t, hostile), openTestSession(t, env.primary)

	pauseReplay(ctx, t, env, stack.standby)

	// synchronous_commit=on waits for standby flush only, which continues while replay is paused.
	tx, flushed, _ := beginRow(ctx, t, b, false)
	require.NoError(t, b.commit(tx), "ordinary commit while replay is paused")

	timedOut := deadlineCommit(ctx, t, a, stack.primary)

	tx, sibling, siblingPID := beginRow(ctx, t, b, true)
	siblingCommit := startCommit(b, tx)

	awaitPrimary(ctx, t, stack.primary, "sibling COMMIT waits in SyncRep", inSyncRep, siblingPID)

	cancelled := cancelledCommit(ctx, t, a, stack)

	_, err = stack.standby.Exec(ctx, "SELECT pg_wal_replay_resume()")
	require.NoError(t, err)
	require.NoError(t, siblingCommit.Wait(), "the other session's WARNING arrived while this COMMIT was in flight")

	tx, final, _ := beginRow(ctx, t, a, true)
	require.NoError(t, a.commit(tx), "clean remote_apply commit after the session's earlier WARNING")

	// A clean remote_apply COMMIT returns once the standby applied WAL through it, earlier commits included.
	var applied int
	require.NoError(t, stack.standby.QueryRow(ctx, `SELECT count(*) FROM account_control WHERE account_id IN ($1, $2, $3, $4, $5)`,
		flushed, timedOut, sibling, cancelled, final).Scan(&applied))
	assert.Equal(t, 5, applied)
}

// deadlineCommit lets the default pgx deadline expire while COMMIT waits in SyncRep. pgx sends a CancelRequest and
// closes the connection; the backend commits locally, which only that CancelRequest can cause while replay is paused.
func deadlineCommit(ctx context.Context, t *testing.T, s *sqlSession, primary *pgx.Conn) uuid.UUID {
	t.Helper()

	deadlineCtx, cancel := context.WithTimeout(ctx, commitDeadline)
	defer cancel()

	tx, row, pid := beginRow(deadlineCtx, t, s, true)
	commit := startCommit(s, tx)

	awaitPrimary(ctx, t, primary, "COMMIT under a deadline waits in SyncRep", inSyncRep, pid)
	require.NoError(t, deadlineCtx.Err(), "SyncRep must be observed before the %s deadline", commitDeadline)

	err := commit.Wait()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	t.Logf("pid %d after the %s deadline: %v", pid, commitDeadline, err)

	awaitPrimary(ctx, t, primary, "timed-out COMMIT is visible and its backend gone", committedAndGone, row, pid)

	return row
}

// cancelledCommit cancels only this session's backend while its COMMIT waits in SyncRep. PostgreSQL ends the wait
// with a WARNING and a successful COMMIT: the row is visible on the primary, not yet on the paused standby.
func cancelledCommit(ctx context.Context, t *testing.T, s *sqlSession, stack *verifiedStack) uuid.UUID {
	t.Helper()

	tx, row, pid := beginRow(ctx, t, s, true)
	commit := startCommit(s, tx)

	awaitPrimary(ctx, t, stack.primary, "tested COMMIT waits in SyncRep", inSyncRep, pid)

	var signalled bool
	require.NoError(t, stack.primary.QueryRow(ctx, `SELECT pg_cancel_backend($1)`, pid).Scan(&signalled))
	require.True(t, signalled)

	err := commit.Wait()
	require.ErrorContains(t, err, "WARNING 01000: canceling wait for synchronous replication")
	t.Logf("pid %d after pg_cancel_backend: %v", pid, err)

	var onPrimary, onStandby bool
	require.NoError(t, stack.primary.QueryRow(ctx, rowExists, row).Scan(&onPrimary))
	require.NoError(t, stack.standby.QueryRow(ctx, rowExists, row).Scan(&onStandby))
	assert.True(t, onPrimary, "the cancelled COMMIT committed locally")
	assert.False(t, onStandby, "the paused standby has not replayed it")

	return row
}

func openTestSession(t *testing.T, cfg *pgx.ConnConfig) *sqlSession {
	t.Helper()

	s := openSQLSession(cfg)

	t.Cleanup(func() { assert.NoError(t, s.db.Close()) })

	return s
}

// namedSession opens a session named app, so the test can find its backend; settings are extra startup parameters.
func namedSession(t *testing.T, env *benchEnv, app string, settings map[string]string) *sqlSession {
	t.Helper()

	cfg := env.primary.Copy()
	maps.Copy(cfg.RuntimeParams, settings)
	cfg.RuntimeParams["application_name"] = app

	return openTestSession(t, cfg)
}

// beginRow opens a transaction on s that inserts one new control row and returns that row and the backend PID
// running it. remoteApply makes its COMMIT wait for standby replay, a stronger wait than the guarded default. A test
// that fails first ends it by cancelling ctx.
func beginRow(ctx context.Context, t *testing.T, s *sqlSession, remoteApply bool) (*sql.Tx, uuid.UUID, int32) {
	t.Helper()

	tx, err := s.begin(ctx)
	require.NoError(t, err)

	if remoteApply {
		_, err = tx.ExecContext(ctx, `SET LOCAL synchronous_commit = 'remote_apply'`)
		require.NoError(t, err)
	}

	var pid int32
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid))

	row := uuid.New()
	_, err = tx.ExecContext(ctx, `INSERT INTO account_control (organization_id, ledger_id, account_id, limit_remaining, version)
		VALUES ($1, $2, $3, 0, 1)`, benchOrgID, benchLedgerID, row)
	require.NoError(t, err)

	return tx, row, pid
}

// startCommit runs s.commit on a recovered goroutine; Wait returns its result.
func startCommit(s *sqlSession, tx *sql.Tx) *errgroup.Group {
	g := &errgroup.Group{}
	g.Go(func() error { return s.commit(tx) })

	return g
}

// pauseReplay registers the resume before pausing. A resume that fails leaves the standby paused: the assertion
// says so, and nothing else touches the stack.
func pauseReplay(ctx context.Context, t *testing.T, env *benchEnv, standby *pgx.Conn) {
	t.Helper()

	t.Cleanup(func() {
		resumeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		conn, _, err := connectNode(resumeCtx, env.standby, standbyClusterName, true)
		if err == nil {
			defer conn.Close(resumeCtx)

			_, err = conn.Exec(resumeCtx, `SELECT pg_wal_replay_resume()`)
		}

		assert.NoError(t, err, "standby replay may still be paused; run SELECT pg_wal_replay_resume() on the bench standby")
	})

	_, err := standby.Exec(ctx, `SELECT pg_wal_replay_pause()`)
	require.NoError(t, err)

	var state string
	require.NoError(t, standby.QueryRow(ctx, `SELECT pg_get_wal_replay_pause_state()`).Scan(&state))
	require.Contains(t, []string{"pause requested", "paused"}, state)
	t.Logf("standby replay %s", state)
}

// awaitPrimary polls query every 20 ms on conn until it returns true; 30 s bounds the wait.
func awaitPrimary(ctx context.Context, t *testing.T, conn *pgx.Conn, what, query string, args ...any) {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for {
		var done bool
		require.NoError(t, conn.QueryRow(ctx, query, args...).Scan(&done), what)

		if done {
			return
		}

		require.NoError(t, backoff.WaitContext(ctx, 20*time.Millisecond), what)
	}
}
