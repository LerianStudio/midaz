// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/migrations"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func operationMigration(t *testing.T, direction string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, migrations.WriteTo(dir))
	data, err := os.ReadFile(filepath.Join(dir, "000029_reserve_operations."+direction+".sql"))
	require.NoError(t, err)
	return string(data)
}

func operationDatabase(t *testing.T, suffix string) (*sql.DB, *ReserveDecisionRepository, *ReserveOperationRepository) {
	t.Helper()
	db, decisions := decisionDatabase(t, suffix)
	_, err := db.ExecContext(t.Context(), operationMigration(t, "up"))
	require.NoError(t, err)
	return db, decisions, NewReserveOperationRepository()
}

func completeOperation(t *testing.T, db *sql.DB, repo *ReserveOperationRepository, identity model.ReserveOperationIdentity, status model.ReserveOperationStatus, at time.Time) (*model.ReserveOperationState, bool, error) {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	state, changed, err := repo.CompleteWithTx(t.Context(), tx, identity, status, at)
	if err != nil {
		require.NoError(t, tx.Rollback())
		return nil, false, err
	}
	require.NoError(t, tx.Commit())
	return state, changed, nil
}

func TestIntegrationReserveOperationCompletionBeforeDecision(t *testing.T) {
	for _, status := range []model.ReserveOperationStatus{model.OperationConfirmed, model.OperationReleased} {
		t.Run(string(status), func(t *testing.T) {
			db, decisions, operations := operationDatabase(t, "early")
			d := persistedDecision()
			at := testutil.FixedTime()
			identity := d.Key.Identity()
			state, changed, err := completeOperation(t, db, operations, identity, status, at)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, status, state.Status)
			require.Equal(t, at, *state.CompletedAt)
			repeated, changed, err := completeOperation(t, db, operations, identity, status, at.Add(time.Hour))
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, state, repeated)
			contradiction := model.OperationConfirmed
			if status == contradiction {
				contradiction = model.OperationReleased
			}
			_, _, err = completeOperation(t, db, operations, identity, contradiction, at)
			require.ErrorIs(t, err, constant.ErrReserveOperationConflict)
			tx, err := db.BeginTx(t.Context(), nil)
			require.NoError(t, err)
			require.ErrorIs(t, decisions.CreateWithTx(t.Context(), tx, d), constant.ErrReserveOperationConflict)
			require.NoError(t, tx.Rollback())
			got, err := decisions.Get(t.Context(), d.Key)
			require.NoError(t, err)
			require.Nil(t, got, "completion must not invent an evaluation")
			other := d.Clone()
			other.Key.IntegrationID = "another-producer"
			other.Result.EvaluationID[0]++
			saveDecision(t, db, decisions, other)
		})
	}
}

func TestIntegrationReserveOperationDecisionReplayAfterCompletion(t *testing.T) {
	db, decisions, operations := operationDatabase(t, "replay")
	d := persistedDecision()
	saveDecision(t, db, decisions, d)
	_, changed, err := completeOperation(t, db, operations, d.Key.Identity(), model.OperationConfirmed, testutil.FixedTime())
	require.NoError(t, err)
	require.True(t, changed)
	got, err := decisions.Get(t.Context(), d.Key)
	require.NoError(t, err)
	require.Equal(t, d, *got)
	for _, statement := range []string{"UPDATE reserve_operations SET status='OPEN',completed_at=NULL", "DELETE FROM reserve_operations", "TRUNCATE reserve_operations CASCADE"} {
		_, err := db.ExecContext(t.Context(), statement)
		require.Error(t, err)
	}
	_, err = db.ExecContext(t.Context(), operationMigration(t, "down"))
	require.Error(t, err, "rollback cannot remove completion protection")
	got, err = decisions.Get(t.Context(), d.Key)
	require.NoError(t, err)
	require.Equal(t, d, *got)
}

func TestIntegrationReserveOperationConcurrentCompletions(t *testing.T) {
	for _, contradictory := range []bool{false, true} {
		t.Run(map[bool]string{false: "same", true: "conflicting"}[contradictory], func(t *testing.T) {
			db, _, repo := operationDatabase(t, "concurrent")
			identity := persistedDecision().Key.Identity()
			type outcome struct {
				changed bool
				err     error
			}
			start := make(chan struct{})
			results := make(chan outcome, 2)
			var workers sync.WaitGroup
			for i := range 2 {
				workers.Go(func() {
					<-start
					status := model.OperationConfirmed
					if contradictory && i == 1 {
						status = model.OperationReleased
					}
					tx, err := db.BeginTx(t.Context(), nil)
					if err != nil {
						results <- outcome{err: err}
						return
					}
					defer tx.Rollback()
					_, changed, err := repo.CompleteWithTx(t.Context(), tx, identity, status, testutil.FixedTime())
					if err == nil {
						err = tx.Commit()
					}
					results <- outcome{changed, err}
				})
			}
			close(start)
			workers.Wait()
			close(results)
			changed, conflicts := 0, 0
			for result := range results {
				if result.changed {
					changed++
				}
				if errors.Is(result.err, constant.ErrReserveOperationConflict) {
					conflicts++
				} else {
					require.NoError(t, result.err)
				}
			}
			require.Equal(t, 1, changed)
			if contradictory {
				require.Equal(t, 1, conflicts)
			} else {
				require.Zero(t, conflicts)
			}
		})
	}
}

func TestIntegrationReserveOperationDelayedDecisionAndRollback(t *testing.T) {
	for _, commitCompletion := range []bool{true, false} {
		t.Run(map[bool]string{true: "committed completion", false: "rolled back completion"}[commitCompletion], func(t *testing.T) {
			db, decisions, operations := operationDatabase(t, "delayed")
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			d := persistedDecision()
			completion, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = completion.Rollback() })
			_, _, err = operations.CompleteWithTx(ctx, completion, d.Key.Identity(), model.OperationReleased, testutil.FixedTime())
			require.NoError(t, err)
			conn, err := db.Conn(ctx)
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })
			var backendID int
			require.NoError(t, conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&backendID))
			writer, err := conn.BeginTx(ctx, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = writer.Rollback() })
			result := make(chan error, 1)
			go func() {
				err := decisions.CreateWithTx(ctx, writer, d)
				if err == nil {
					err = writer.Commit()
				} else {
					_ = writer.Rollback()
				}
				result <- err
			}()
			require.Eventually(t, func() bool {
				var blocked bool
				err := db.QueryRowContext(ctx, "SELECT cardinality(pg_blocking_pids($1)) > 0", backendID).Scan(&blocked)
				return err == nil && blocked
			}, 2*time.Second, 10*time.Millisecond, "decision insert must wait for operation completion")
			if commitCompletion {
				require.NoError(t, completion.Commit())
				require.ErrorIs(t, <-result, constant.ErrReserveOperationConflict)
			} else {
				require.NoError(t, completion.Rollback())
				require.NoError(t, <-result)
			}
			got, err := decisions.Get(ctx, d.Key)
			require.NoError(t, err)
			if commitCompletion {
				require.Nil(t, got)
			} else {
				require.Equal(t, d, *got)
			}
		})
	}
}

func TestIntegrationReserveOperationCompletionWaitsForDecision(t *testing.T) {
	for _, commitDecision := range []bool{true, false} {
		t.Run(map[bool]string{true: "committed decision", false: "rolled back decision"}[commitDecision], func(t *testing.T) {
			db, decisions, operations := operationDatabase(t, "decision-first")
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			d := persistedDecision()
			writer, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = writer.Rollback() })
			require.NoError(t, decisions.CreateWithTx(ctx, writer, d))
			conn, err := db.Conn(ctx)
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })
			var backendID int
			require.NoError(t, conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&backendID))
			completion, err := conn.BeginTx(ctx, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = completion.Rollback() })
			type outcome struct {
				decision *model.ReserveDecision
				err      error
			}
			result := make(chan outcome, 1)
			go func() {
				_, _, err := operations.CompleteWithTx(ctx, completion, d.Key.Identity(), model.OperationConfirmed, testutil.FixedTime())
				var got *model.ReserveDecision
				if err == nil {
					// Completion must see the committed decision in its own
					// transaction before settling capacity and mandatory audit.
					got, err = decisions.GetWithTx(ctx, completion, d.Key)
				}
				if err == nil {
					err = completion.Commit()
				} else {
					_ = completion.Rollback()
				}
				result <- outcome{got, err}
			}()
			require.Eventually(t, func() bool {
				var blocked bool
				err := db.QueryRowContext(ctx, "SELECT cardinality(pg_blocking_pids($1)) > 0", backendID).Scan(&blocked)
				return err == nil && blocked
			}, 2*time.Second, 10*time.Millisecond, "completion must wait for decision transaction")
			if commitDecision {
				require.NoError(t, writer.Commit())
			} else {
				require.NoError(t, writer.Rollback())
			}
			got := <-result
			require.NoError(t, got.err)
			if commitDecision {
				require.Equal(t, &d, got.decision)
			} else {
				require.Nil(t, got.decision)
			}
			state, changed, err := completeOperation(t, db, operations, d.Key.Identity(), model.OperationConfirmed, testutil.FixedTime())
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, model.OperationConfirmed, state.Status)
		})
	}
}

func TestIntegrationReserveOperationMigrationBackfillAndEmptyRollback(t *testing.T) {
	db, decisions := decisionDatabase(t, "upgrade")
	d := persistedDecision()
	saveDecision(t, db, decisions, d)
	_, err := db.ExecContext(t.Context(), operationMigration(t, "up"))
	require.NoError(t, err)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	state, err := NewReserveOperationRepository().LockWithTx(t.Context(), tx, d.Key.Identity())
	require.NoError(t, err)
	require.Equal(t, model.OperationOpen, state.Status)
	require.Nil(t, state.CompletedAt)
	require.NoError(t, tx.Rollback())
	got, err := decisions.Get(t.Context(), d.Key)
	require.NoError(t, err)
	require.Equal(t, d, *got)
	empty, _, _ := operationDatabase(t, "empty")
	_, err = empty.ExecContext(t.Context(), operationMigration(t, "down"))
	require.NoError(t, err)
	_, err = empty.ExecContext(t.Context(), operationMigration(t, "up"))
	require.NoError(t, err)
}

func TestIntegrationReserveOperationTenantIsolationAndCanceledWait(t *testing.T) {
	dbA, _, operations := operationDatabase(t, "tenant-a")
	dbB, decisionsB, _ := operationDatabase(t, "tenant-b")
	d := persistedDecision()
	_, _, err := completeOperation(t, dbA, operations, d.Key.Identity(), model.OperationReleased, testutil.FixedTime())
	require.NoError(t, err)
	// Identical integration/transaction IDs in a different resolved tenant pool
	// are independent; no global lifecycle state may reject this evaluation.
	saveDecision(t, dbB, decisionsB, d)
	holder, err := dbB.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Rollback() })
	_, err = operations.LockWithTx(t.Context(), holder, d.Key.Identity())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	waiter, err := dbB.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = waiter.Rollback() })
	state, changed, err := operations.CompleteWithTx(ctx, waiter, d.Key.Identity(), model.OperationConfirmed, testutil.FixedTime())
	require.Error(t, err)
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	require.Nil(t, state)
	require.False(t, changed)
	_ = waiter.Rollback()
	require.NoError(t, holder.Rollback())
	check, err := dbB.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	state, err = operations.LockWithTx(t.Context(), check, d.Key.Identity())
	require.NoError(t, err)
	require.Equal(t, model.OperationOpen, state.Status)
	require.NoError(t, check.Rollback())
}
