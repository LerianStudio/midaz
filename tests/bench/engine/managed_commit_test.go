//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/errgroup"
	"github.com/LerianStudio/lib-commons/v7/commons/outbox"
	outboxpg "github.com/LerianStudio/lib-commons/v7/commons/outbox/postgres"
	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const (
	// commitNoticeKey names, in one physical connection's pgx CustomData, the slot of the COMMIT it is running.
	commitNoticeKey = "midaz.bench.commit-notice"
	managedTenantID = "tenant-a"
	rowAndEventSQL  = `SELECT (SELECT count(*) FROM account_control WHERE account_id = $1)
		+ (SELECT count(*) FROM outbox_events WHERE aggregate_id = $1)`
)

// onCommitNotice is the managed pools' only notice handler. pgx runs it on the goroutine reading that connection, which
// database/sql serializes with Conn.Raw under the connection's lock; it never blocks, queries or keeps c.
func onCommitNotice(c *pgconn.PgConn, n *pgconn.Notice) {
	if slot, ok := c.CustomData()[commitNoticeKey].(*atomic.Pointer[pgconn.Notice]); ok {
		slot.CompareAndSwap(nil, n)
	}
}

// pgConnOf returns the pgx connection inside the otelsql wrapper every managed primary pool hands out; any other
// driver connection is refused, so its COMMIT is never sent.
func pgConnOf(dc any) (*pgconn.PgConn, error) {
	if w, ok := dc.(interface{ Raw() driver.Conn }); ok {
		if sc, ok := w.Raw().(*stdlib.Conn); ok {
			return sc.Conn().PgConn(), nil
		}
	}

	return nil, fmt.Errorf("unsupported driver connection %T", dc)
}

// commitManaged commits tx, begun on conn by beginManagedRow, exactly once. It returns nil only when COMMIT returned no
// error and no notice reached this commit's fresh slot; any other result after COMMIT is sent is an unknown outcome.
func commitManaged(conn *sql.Conn, tx *sql.Tx) error {
	slot := new(atomic.Pointer[pgconn.Notice])

	if err := conn.Raw(func(dc any) error {
		pg, err := pgConnOf(dc)
		if err == nil {
			pg.CustomData()[commitNoticeKey] = slot
		}

		return err
	}); err != nil {
		return errors.Join(fmt.Errorf("commit not attempted: %w", err), tx.Rollback())
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit outcome unknown: %w", err)
	}

	if n := slot.Load(); n != nil {
		return fmt.Errorf("commit outcome unknown: %s %s: %s", n.Severity, n.Code, n.Message)
	}

	return nil
}

// Through the library-managed ST and MT primary pools, with PGOPTIONS hiding warnings on every dial, a COMMIT notice
// reaches only its own connection's slot. This proves the library seam, not product code: with standby replay paused,
// a deadline and a cancelled SyncRep wait both commit locally and report unknown; sibling, reused and new stay clean.
func TestManagedPoolCommit(t *testing.T) {
	env := requireDB(t)

	setupCtx, cancelSetup := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancelSetup()

	// The guarded rebuild empties account_control and outbox_events, so every row below is this test's own.
	require.NoError(t, bootstrapSchema(setupCtx, env))

	stack, err := openVerifiedStack(setupCtx, env)
	require.NoError(t, err)

	defer stack.close(setupCtx)

	_, _, repo, _ := newTestOutbox(setupCtx, t, env)

	for _, pool := range []struct {
		name string
		open func(context.Context, *testing.T, *benchEnv) *sql.DB
	}{{"st", openManagedST}, {"mt", openManagedMT}} {
		t.Run(pool.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()

			// The parsed admin configs ignore it; every managed dial re-reads it, as a deployment's environment may.
			t.Setenv("PGOPTIONS", "-c client_min_messages=error")
			proveManagedCommit(ctx, t, env, stack, repo, pool.open(ctx, t, env))
		})
	}
}

// proveManagedCommit runs the SQL-session COMMIT cases through one managed primary pool. A backend PID is only the
// test's fault handle and identity check; commitManaged never sees one.
func proveManagedCommit(ctx context.Context, t *testing.T, env *benchEnv, stack *verifiedStack, repo *outboxpg.Repository, db *sql.DB) {
	t.Helper()

	a, b, c := pin(ctx, t, db), pin(ctx, t, db), pin(ctx, t, db)
	for _, conn := range []*sql.Conn{a, b, c} {
		var visibility string
		require.NoError(t, conn.QueryRowContext(ctx, `SHOW client_min_messages`).Scan(&visibility))
		require.Equal(t, "error", visibility, "the hostile PGOPTIONS must reach every managed connection")
	}

	var statementTimeout, transactionTimeout string
	require.NoError(t, a.QueryRowContext(ctx, `SELECT current_setting('statement_timeout'), current_setting('transaction_timeout')`).
		Scan(&statementTimeout, &transactionTimeout))
	t.Logf("managed session statement_timeout=%q transaction_timeout=%q", statementTimeout, transactionTimeout)

	pauseReplay(ctx, t, env, stack.standby)

	timedOut, timedOutPID := managedDeadlineCommit(ctx, t, c, repo, stack.primary)
	require.NoError(t, c.Close())

	tx, cancelled, pid := beginManagedRow(ctx, t, a, repo)
	commit := startManagedCommit(a, tx)

	awaitPrimary(ctx, t, stack.primary, "tested COMMIT waits in SyncRep", inSyncRep, pid)

	tx, sibling, siblingPID := beginManagedRow(ctx, t, b, repo)
	siblingCommit := startManagedCommit(b, tx)

	awaitPrimary(ctx, t, stack.primary, "sibling COMMIT waits in SyncRep", inSyncRep, siblingPID)

	var signalled bool
	require.NoError(t, stack.primary.QueryRow(ctx, `SELECT pg_cancel_backend($1)`, pid).Scan(&signalled))
	require.True(t, signalled)

	// Only the slot path formats a notice this way, so the driver's COMMIT itself returned nil.
	err := commit.Wait()
	require.ErrorContains(t, err, "commit outcome unknown: WARNING 01000: canceling wait for synchronous replication")
	t.Logf("pid %d after pg_cancel_backend: %v", pid, err)
	assert.Equal(t, 2, rowAndEvent(ctx, t, stack.primary, cancelled), "the cancelled COMMIT committed row and event locally")
	assert.Zero(t, rowAndEvent(ctx, t, stack.standby, cancelled), "the paused standby has replayed neither")

	_, err = stack.standby.Exec(ctx, `SELECT pg_wal_replay_resume()`)
	require.NoError(t, err)
	require.NoError(t, siblingCommit.Wait(), "the other connection's WARNING arrived while this COMMIT was in flight")

	tx, reused, reusedPID := beginManagedRow(ctx, t, a, repo)
	require.Equal(t, pid, reusedPID, "A keeps its physical connection")
	require.NoError(t, commitManaged(a, tx), "A's earlier WARNING stays in its old slot")

	d := pin(ctx, t, db)

	tx, replaced, replacedPID := beginManagedRow(ctx, t, d, repo)
	require.NotEqual(t, timedOutPID, replacedPID, "the timed-out connection is replaced")
	require.NoError(t, commitManaged(d, tx), "clean COMMIT on the replacement connection")

	// A clean remote_apply COMMIT returns once the standby applied WAL through it, earlier commits included.
	for _, row := range []uuid.UUID{timedOut, sibling, cancelled, reused, replaced} {
		assert.Equal(t, 2, rowAndEvent(ctx, t, stack.standby, row), "row %s and its event on the standby", row)
	}
}

// managedDeadlineCommit lets a deadline expire while COMMIT waits in SyncRep. pgx sends a CancelRequest and closes the
// connection; the backend commits locally. It returns the row and the PID of the backend that is gone.
func managedDeadlineCommit(ctx context.Context, t *testing.T, conn *sql.Conn, repo *outboxpg.Repository, primary *pgx.Conn) (uuid.UUID, int32) {
	t.Helper()

	deadlineCtx, cancel := context.WithTimeout(ctx, commitDeadline)
	defer cancel()

	tx, row, pid := beginManagedRow(deadlineCtx, t, conn, repo)
	commit := startManagedCommit(conn, tx)

	awaitPrimary(ctx, t, primary, "COMMIT under a deadline waits in SyncRep", inSyncRep, pid)
	require.NoError(t, deadlineCtx.Err(), "SyncRep must be observed before the %s deadline", commitDeadline)

	err := commit.Wait()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "commit outcome unknown")
	t.Logf("pid %d after the %s deadline: %v", pid, commitDeadline, err)

	awaitPrimary(ctx, t, primary, "timed-out COMMIT is visible and its backend gone", committedAndGone, row, pid)

	return row, pid
}

// beginManagedRow opens, on conn, a READ COMMITTED transaction with the warning prologue, one new control row and its
// outbox event through the lib-commons repository in the same *sql.Tx. A test that fails first ends it by cancelling ctx.
func beginManagedRow(ctx context.Context, t *testing.T, conn *sql.Conn, repo *outboxpg.Repository) (*sql.Tx, uuid.UUID, int32) {
	t.Helper()

	tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	require.NoError(t, err)

	// The prologue outranks DSN, PGOPTIONS, role and server settings for this transaction, its COMMIT included.
	_, err = tx.ExecContext(ctx, `SET LOCAL client_min_messages = warning`)
	require.NoError(t, err)

	// Fault shaping only: COMMIT waits for standby replay, which the test pauses.
	_, err = tx.ExecContext(ctx, `SET LOCAL synchronous_commit = 'remote_apply'`)
	require.NoError(t, err)

	var (
		visibility string
		pid        int32
	)

	require.NoError(t, tx.QueryRowContext(ctx, `SELECT current_setting('client_min_messages'), pg_backend_pid()`).Scan(&visibility, &pid))
	require.Equal(t, "warning", visibility)

	row := uuid.New()
	_, err = tx.ExecContext(ctx, `INSERT INTO account_control (organization_id, ledger_id, account_id, limit_remaining, version)
		VALUES ($1, $2, $3, 0, 1)`, benchOrgID, benchLedgerID, row)
	require.NoError(t, err)

	event, err := outbox.NewOutboxEvent(ctx, "bench.managed_commit", row, []byte(`{}`))
	require.NoError(t, err)

	_, err = repo.CreateWithTx(ctx, tx, event)
	require.NoError(t, err)

	return tx, row, pid
}

// startManagedCommit runs commitManaged on a recovered goroutine; Wait returns its result.
func startManagedCommit(conn *sql.Conn, tx *sql.Tx) *errgroup.Group {
	g := &errgroup.Group{}
	g.Go(func() error { return commitManaged(conn, tx) })

	return g
}

// pin checks out one physical connection until the test ends; a connection the test already closed stays closed.
func pin(ctx context.Context, t *testing.T, db *sql.DB) *sql.Conn {
	t.Helper()

	conn, err := db.Conn(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		if err := conn.Close(); !errors.Is(err, sql.ErrConnDone) {
			assert.NoError(t, err)
		}
	})

	return conn
}

// rowAndEvent is 2 when row's control row and outbox event are both visible on node, 0 when neither is.
func rowAndEvent(ctx context.Context, t *testing.T, node *pgx.Conn, row uuid.UUID) int {
	t.Helper()

	var n int
	require.NoError(t, node.QueryRow(ctx, rowAndEventSQL, row).Scan(&n))

	return n
}

// openManagedST returns a library-managed single-tenant primary pool with the notice handler.
func openManagedST(ctx context.Context, t *testing.T, _ *benchEnv) *sql.DB {
	t.Helper()

	pg, err := libPostgres.NewWithNotice(libPostgres.Config{
		PrimaryDSN: os.Getenv("BENCH_PG_PRIMARY_DSN"), MaxOpenConnections: 4, MaxIdleConnections: 4,
	}, onCommitNotice)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, pg.Close()) })

	resolver, err := pg.Resolver(ctx)
	require.NoError(t, err)
	require.Len(t, resolver.PrimaryDBs(), 1)

	return resolver.PrimaryDBs()[0]
}

// openManagedMT returns a library-managed tenant primary pool with the notice handler.
// A loopback tenant service supplies the bench primary without a schema, so PGOPTIONS applies.
func openManagedMT(ctx context.Context, t *testing.T, env *benchEnv) *sql.DB {
	t.Helper()

	body, err := json.Marshal(map[string]any{"id": managedTenantID, "databases": map[string]any{
		constant.ModuleTransaction: map[string]any{"postgresql": map[string]any{
			"host": env.primary.Host, "port": env.primary.Port, "database": env.primary.Database,
			"username": env.primary.User, "password": env.primary.Password,
		}},
	}})
	require.NoError(t, err)

	tenants := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		_, err := w.Write(body)
		assert.NoError(t, err)
	}))
	t.Cleanup(tenants.Close)

	tenantClient, err := tmclient.NewClient(tenants.URL, nil, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("bench"))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, tenantClient.Close()) })

	manager := tmpostgres.NewManager(tenantClient, "ledger", tmpostgres.WithModule(constant.ModuleTransaction),
		tmpostgres.WithConnectionLimits(4, 4), tmpostgres.WithOnNotice(onCommitNotice))

	t.Cleanup(func() { assert.NoError(t, manager.Close(context.Background())) })

	conn, err := manager.GetConnection(ctx, managedTenantID)
	require.NoError(t, err)

	resolver, err := conn.GetDB()
	require.NoError(t, err)
	require.Len(t, resolver.PrimaryDBs(), 1)

	return resolver.PrimaryDBs()[0]
}
