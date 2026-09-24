// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/migrations"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func capacityMigration(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, migrations.WriteTo(dir))
	data, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	return string(data)
}

func capacityDatabase(t *testing.T) (*sql.DB, *ReserveDecisionRepository, *UsageReservationRepository) {
	t.Helper()
	db, decisions, _ := operationDatabase(t, "capacity")
	_, err := db.ExecContext(t.Context(), `CREATE TABLE limits (LIKE public.limits INCLUDING ALL);
        CREATE TABLE usage_counters (LIKE public.usage_counters INCLUDING ALL)`)
	require.NoError(t, err)
	for _, name := range []string{"000019_create_usage_reservations.up.sql", "000021_reservation_amounts_to_decimal.up.sql", "000030_decision_reservations.up.sql"} {
		_, err = db.ExecContext(t.Context(), capacityMigration(t, name))
		require.NoError(t, err)
	}
	return db, decisions, newReservationRepoIntegration(db)
}

func decisionCapacity(limitID uuid.UUID, seed int64) *model.Reservation {
	return &model.Reservation{
		ID: testutil.MustDeterministicUUID(seed), LimitID: limitID,
		TransactionID: persistedDecision().Key.TransactionID, ScopeKey: "account:capacity", PeriodKey: "2026-09",
		Amount: decimal.RequireFromString("10.125"), Status: model.StatusReserved,
		ReservationExpiresAt: testutil.FixedTime().Add(-time.Hour), CreatedAt: testutil.FixedTime(),
	}
}

func persistDecisionCapacity(t *testing.T, db *sql.DB, repo *UsageReservationRepository, decisions *ReserveDecisionRepository, d model.ReserveDecision, res *model.Reservation) {
	t.Helper()
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		if _, err := NewReserveOperationRepository().LockWithTx(t.Context(), tx, d.Key.Identity()); err != nil {
			return err
		}
		if err := repo.ReserveForDecisionWithTx(t.Context(), tx, d.Result.EvaluationID, res, decimal.NewFromInt(100), testutil.FixedTime().Add(time.Hour)); err != nil {
			return err
		}
		d.Result.ReservationIDs = []uuid.UUID{res.ID}
		return decisions.CreateWithTx(t.Context(), tx, d)
	}))
}

func TestIntegrationDecisionCapacityOwnershipAndLegacyIsolation(t *testing.T) {
	db, decisions, repo := capacityDatabase(t)
	limitID := createTestLimitNamed(t, db, 75001, "ownership")
	a, b := persistedDecision(), persistedDecision()
	b.Key.IntegrationID = "producer-b"
	b.Result.EvaluationID = testutil.MustDeterministicUUID(75002)
	first, second, legacy := decisionCapacity(limitID, 75003), decisionCapacity(limitID, 75004), decisionCapacity(limitID, 75005)
	persistDecisionCapacity(t, db, repo, decisions, a, first)
	persistDecisionCapacity(t, db, repo, decisions, b, second)
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error { return repo.ReserveWithTx(t.Context(), tx, legacy, decimal.NewFromInt(100)) }))
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error { return repo.ReserveWithTx(t.Context(), tx, legacy, decimal.NewFromInt(100)) }))
	cur, held := readCounterDecimal(t, db, limitID, first.ScopeKey, first.PeriodKey)
	require.True(t, cur.IsZero())
	require.Equal(t, "30.375", held.String())
	// Old addressing must neither confirm nor release new decision-owned rows.
	require.ErrorIs(t, inRealTx(t, db, func(tx *sql.Tx) error { return repo.ConfirmWithTx(t.Context(), tx, first.ID) }), constant.ErrReservationNotFound)
	require.ErrorIs(t, inRealTx(t, db, func(tx *sql.Tx) error { return repo.ReleaseWithTx(t.Context(), tx, first.ID, model.StatusExpired) }), constant.ErrReservationNotFound)
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		rows, err := repo.ConfirmByTransactionWithTx(t.Context(), tx, a.Key.TransactionID)
		require.Len(t, rows, 1)
		require.Equal(t, legacy.ID, rows[0].ID)
		return err
	}))
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		rows, err := repo.ReleaseByTransactionWithTx(t.Context(), tx, a.Key.TransactionID, model.StatusReleased)
		require.Empty(t, rows)
		return err
	}))
	cur, held = readCounterDecimal(t, db, limitID, first.ScopeKey, first.PeriodKey)
	require.Equal(t, "10.125", cur.String())
	require.Equal(t, "20.25", held.String())
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		rows, err := repo.SettleDecisionWithTx(t.Context(), tx, a.Result.EvaluationID, model.StatusConfirmed)
		require.Len(t, rows, 1)
		require.Equal(t, first.ID, rows[0].ID)
		return err
	}))
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		rows, err := repo.SettleDecisionWithTx(t.Context(), tx, b.Result.EvaluationID, model.StatusReleased)
		require.Len(t, rows, 1)
		require.Equal(t, second.ID, rows[0].ID)
		return err
	}))
	cur, held = readCounterDecimal(t, db, limitID, first.ScopeKey, first.PeriodKey)
	require.Equal(t, "20.25", cur.String())
	require.True(t, held.IsZero())
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		rows, err := repo.SettleDecisionWithTx(t.Context(), tx, a.Result.EvaluationID, model.StatusConfirmed)
		require.Empty(t, rows)
		return err
	}))
}

func TestIntegrationDecisionCapacityDeferredOwnershipAndRollback(t *testing.T) {
	for _, scenario := range []string{"missing decision", "wrong transaction", "denied capacity", "caller rollback"} {
		t.Run(scenario, func(t *testing.T) {
			db, decisions, repo := capacityDatabase(t)
			limitID := createTestLimitNamed(t, db, 75101, "rollback")
			d := persistedDecision()
			res := decisionCapacity(limitID, 75102)
			tx, err := db.BeginTx(t.Context(), nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = tx.Rollback() })
			maxAmount := decimal.NewFromInt(100)
			if scenario == "denied capacity" {
				maxAmount = decimal.NewFromInt(1)
			}
			err = repo.ReserveForDecisionWithTx(t.Context(), tx, d.Result.EvaluationID, res, maxAmount, testutil.FixedTime().Add(time.Hour))
			if scenario == "denied capacity" {
				require.ErrorIs(t, err, constant.ErrUsageCounterExceedsLimit)
				require.NoError(t, tx.Rollback())
			} else {
				require.NoError(t, err, "FK permits capacity before final decision in the same transaction")
				if scenario != "missing decision" {
					if scenario == "wrong transaction" {
						d.Key.TransactionID = testutil.MustDeterministicUUID(75103)
						d.Result.TransactionID = d.Key.TransactionID
					}
					d.Result.ReservationIDs = []uuid.UUID{res.ID}
					require.NoError(t, decisions.CreateWithTx(t.Context(), tx, d))
				}
				if scenario == "caller rollback" {
					require.NoError(t, tx.Rollback())
				} else {
					require.Error(t, tx.Commit())
				}
			}
			var count int
			for _, table := range []string{"usage_reservations", "usage_counters", "reserve_decisions"} {
				require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count))
				require.Zero(t, count)
			}
		})
	}
}

func TestIntegrationDecisionCapacityIgnoresTTLAndProtectsCounter(t *testing.T) {
	db, decisions, repo := capacityDatabase(t)
	limitID := createTestLimitNamed(t, db, 75201, "ttl")
	d, res := persistedDecision(), decisionCapacity(limitID, 75202)
	persistDecisionCapacity(t, db, repo, decisions, d, res)
	adapter := &testutil.IntegrationDBAdapter{DB: db}
	reaper := NewReservationReaperRepository(adapter, nil, repo)
	ids, err := reaper.FindExpiredReservations(t.Context(), testutil.FixedTime())
	require.NoError(t, err)
	require.Empty(t, ids)
	for _, query := range []string{
		"UPDATE usage_reservations SET decision_id=NULL",
		"UPDATE usage_reservations SET status='EXPIRED'",
		"UPDATE usage_reservations SET amount=0",
		"DELETE FROM usage_reservations",
		"TRUNCATE usage_reservations",
	} {
		_, err := db.ExecContext(t.Context(), query)
		require.Error(t, err)
	}
	_, err = db.ExecContext(t.Context(), "UPDATE usage_counters SET expires_at=$1", testutil.FixedTime().Add(-time.Hour))
	require.NoError(t, err)
	counters := NewUsageCounterRepositoryWithConnection(adapter)
	deleted, err := counters.DeleteExpiredCounters(t.Context(), testutil.FixedTime())
	require.NoError(t, err)
	require.Zero(t, deleted)
	_, held := readCounterDecimal(t, db, limitID, res.ScopeKey, res.PeriodKey)
	require.Equal(t, "10.125", held.String())
	_, err = db.ExecContext(t.Context(), capacityMigration(t, "000030_decision_reservations.down.sql"))
	require.Error(t, err, "rollback cannot erase decision ownership")
	require.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, res.ID))
}

func TestIntegrationDecisionCapacityRequiresAllowAndResponseOwnership(t *testing.T) {
	for _, scenario := range []string{"deny", "missing handle"} {
		t.Run(scenario, func(t *testing.T) {
			db, decisions, repo := capacityDatabase(t)
			limitID := createTestLimitNamed(t, db, 75301, "owner-result")
			d, res := persistedDecision(), decisionCapacity(limitID, 75302)
			tx, err := db.BeginTx(t.Context(), nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = tx.Rollback() })
			require.NoError(t, repo.ReserveForDecisionWithTx(t.Context(), tx, d.Result.EvaluationID, res, decimal.NewFromInt(100), testutil.FixedTime().Add(time.Hour)))
			if scenario == "deny" {
				d.Result.Decision = tracercontract.DecisionDeny
				d.Result.Reasons = []tracercontract.ReserveReason{tracercontract.ReasonLimitExceeded}
			}
			require.NoError(t, decisions.CreateWithTx(t.Context(), tx, d))
			require.Error(t, tx.Commit(), "provisional capacity needs an ALLOW decision naming its handle")
			var count int
			require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM usage_counters").Scan(&count))
			require.Zero(t, count)
		})
	}
}

func TestIntegrationDecisionCapacitySavepointPersistsDenyWithoutHolds(t *testing.T) {
	db, decisions, repo := capacityDatabase(t)
	limitID := createTestLimitNamed(t, db, 75401, "savepoint")
	d, res := persistedDecision(), decisionCapacity(limitID, 75402)
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := NewReserveOperationRepository().LockWithTx(t.Context(), tx, d.Key.Identity())
		require.NoError(t, err)
		_, err = tx.ExecContext(t.Context(), "SAVEPOINT provisional_capacity")
		require.NoError(t, err)
		require.NoError(t, repo.ReserveForDecisionWithTx(t.Context(), tx, d.Result.EvaluationID, res, decimal.NewFromInt(100), testutil.FixedTime().Add(time.Hour)))
		_, err = tx.ExecContext(t.Context(), "ROLLBACK TO SAVEPOINT provisional_capacity")
		require.NoError(t, err)
		d.Result.Decision = tracercontract.DecisionDeny
		d.Result.Reasons = []tracercontract.ReserveReason{tracercontract.ReasonLimitExceeded}
		return decisions.CreateWithTx(t.Context(), tx, d)
	}))
	got, err := decisions.Get(t.Context(), d.Key)
	require.NoError(t, err)
	require.Equal(t, tracercontract.DecisionDeny, got.Result.Decision)
	var count int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM usage_reservations").Scan(&count))
	require.Zero(t, count)
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM usage_counters").Scan(&count))
	require.Zero(t, count)
}

func TestIntegrationDecisionCapacityMigrationPreservesLegacy(t *testing.T) {
	db, _, repo := capacityDatabase(t)
	limitID := createTestLimitNamed(t, db, 75501, "legacy-migration")
	res := decisionCapacity(limitID, 75502)
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error { return repo.ReserveWithTx(t.Context(), tx, res, decimal.NewFromInt(100)) }))
	const snapshot = `SELECT jsonb_build_array((SELECT to_jsonb(r)-'decision_id' FROM usage_reservations r), (SELECT to_jsonb(c) FROM usage_counters c))::text`
	var before, after string
	require.NoError(t, db.QueryRowContext(t.Context(), snapshot).Scan(&before))
	_, err := db.ExecContext(t.Context(), capacityMigration(t, "000030_decision_reservations.down.sql"))
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(t.Context(), snapshot).Scan(&after))
	require.Equal(t, before, after)
	// The full legacy index is restored for an old binary's ON CONFLICT target.
	_, err = db.ExecContext(t.Context(), `INSERT INTO usage_reservations SELECT * FROM usage_reservations
        ON CONFLICT (transaction_id, limit_id, scope_key, period_key) DO NOTHING`)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), capacityMigration(t, "000030_decision_reservations.up.sql"))
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(t.Context(), snapshot).Scan(&after))
	require.Equal(t, before, after)
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error { return repo.ReserveWithTx(t.Context(), tx, res, decimal.NewFromInt(100)) }))
	_, held := readCounterDecimal(t, db, limitID, res.ScopeKey, res.PeriodKey)
	require.Equal(t, "10.125", held.String())
}

func TestIntegrationDecisionCapacityCleanupRechecksConcurrentHold(t *testing.T) {
	for _, commitHold := range []bool{true, false} {
		t.Run(map[bool]string{true: "committed hold", false: "rolled back hold"}[commitHold], func(t *testing.T) {
			db, decisions, repo := capacityDatabase(t)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			limitID := createTestLimitNamed(t, db, 75601, "cleanup-race")
			res, d := decisionCapacity(limitID, 75602), persistedDecision()
			at := testutil.FixedTime().Add(-time.Hour)
			_, err := db.ExecContext(ctx, `INSERT INTO usage_counters (id,limit_id,scope_key,period_key,current_usage,reserved_usage,last_updated_at,expires_at)
                VALUES ($1,$2,$3,$4,0,0,$5,$5)`, testutil.MustDeterministicUUID(75603), limitID, res.ScopeKey, res.PeriodKey, at)
			require.NoError(t, err)
			writer, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = writer.Rollback() })
			var writerPID int
			require.NoError(t, writer.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&writerPID))
			// Keep the expiry in the past: only the held-capacity guard can save
			// this candidate after the cleanup's old snapshot has seen zero held.
			require.NoError(t, repo.ReserveForDecisionWithTx(ctx, writer, d.Result.EvaluationID, res, decimal.NewFromInt(100), at))
			d.Result.ReservationIDs = []uuid.UUID{res.ID}
			require.NoError(t, decisions.CreateWithTx(ctx, writer, d))
			counters := NewUsageCounterRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
			type outcome struct {
				deleted int64
				err     error
			}
			result := make(chan outcome, 1)
			go func() {
				n, err := counters.DeleteExpiredCounters(ctx, testutil.FixedTime())
				result <- outcome{n, err}
			}()
			require.Eventually(t, func() bool {
				var blocked bool
				err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
                    WHERE query LIKE 'DELETE FROM usage_counters WHERE reserved_usage%'
                      AND $1 = ANY(pg_blocking_pids(pid)))`, writerPID).Scan(&blocked)
				return err == nil && blocked
			}, 2*time.Second, 10*time.Millisecond, "cleanup must reach the locked counter")
			if commitHold {
				require.NoError(t, writer.Commit())
			} else {
				require.NoError(t, writer.Rollback())
			}
			got := <-result
			require.NoError(t, got.err)
			if commitHold {
				require.Zero(t, got.deleted)
				_, held := readCounterDecimal(t, db, limitID, res.ScopeKey, res.PeriodKey)
				require.Equal(t, "10.125", held.String())
			} else {
				require.Equal(t, int64(1), got.deleted)
			}
		})
	}
}

func TestIntegrationDecisionCapacityConcurrentSettlementAndRollback(t *testing.T) {
	db, decisions, repo := capacityDatabase(t)
	limitA := createTestLimitNamed(t, db, 75701, "ordered-a")
	limitB := createTestLimitNamed(t, db, 75702, "ordered-b")
	a, b := persistedDecision(), persistedDecision()
	b.Key.IntegrationID = "producer-b"
	b.Result.EvaluationID = testutil.MustDeterministicUUID(75703)
	rows := []*model.Reservation{decisionCapacity(limitA, 75704), decisionCapacity(limitB, 75705), decisionCapacity(limitB, 75706), decisionCapacity(limitA, 75707)}
	for i, d := range []model.ReserveDecision{a, b} {
		require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
			for _, res := range rows[i*2 : i*2+2] {
				require.NoError(t, repo.ReserveForDecisionWithTx(t.Context(), tx, d.Result.EvaluationID, res, decimal.NewFromInt(100), testutil.FixedTime().Add(time.Hour)))
				d.Result.ReservationIDs = append(d.Result.ReservationIDs, res.ID)
			}
			return decisions.CreateWithTx(t.Context(), tx, d)
		}))
	}
	// Any failure after settlement must be able to roll back operation state,
	// both counter moves and both row transitions together.
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	_, _, err = NewReserveOperationRepository().CompleteWithTx(t.Context(), tx, a.Key.Identity(), model.OperationConfirmed, testutil.FixedTime())
	require.NoError(t, err)
	changed, err := repo.SettleDecisionWithTx(t.Context(), tx, a.Result.EvaluationID, model.StatusConfirmed)
	require.NoError(t, err)
	require.Len(t, changed, 2)
	require.NoError(t, tx.Rollback())
	for _, res := range rows {
		require.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, res.ID))
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	result := make(chan error, 2)
	for _, d := range []model.ReserveDecision{a, b} {
		go func() {
			<-start
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				result <- err
				return
			}
			defer tx.Rollback()
			_, _, err = NewReserveOperationRepository().CompleteWithTx(ctx, tx, d.Key.Identity(), model.OperationConfirmed, testutil.FixedTime())
			if err == nil {
				_, err = repo.SettleDecisionWithTx(ctx, tx, d.Result.EvaluationID, model.StatusConfirmed)
			}
			if err == nil {
				err = tx.Commit()
			}
			result <- err
		}()
	}
	close(start)
	require.NoError(t, <-result)
	require.NoError(t, <-result)
	for _, id := range []uuid.UUID{limitA, limitB} {
		cur, held := readCounterDecimal(t, db, id, rows[0].ScopeKey, rows[0].PeriodKey)
		require.Equal(t, "20.25", cur.String())
		require.True(t, held.IsZero())
	}
	err = inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.SettleDecisionWithTx(t.Context(), tx, a.Result.EvaluationID, model.StatusReleased)
		return err
	})
	require.ErrorIs(t, err, constant.ErrReserveOperationConflict)
}

func TestIntegrationDecisionCapacityGuardAndDuplicateDoNotLeakHolds(t *testing.T) {
	for _, scenario := range []string{"counter guard", "duplicate ownership"} {
		t.Run(scenario, func(t *testing.T) {
			db, decisions, repo := capacityDatabase(t)
			limitID := createTestLimitNamed(t, db, 75801, "guard")
			d, res := persistedDecision(), decisionCapacity(limitID, 75802)
			persistDecisionCapacity(t, db, repo, decisions, d, res)
			next := decisionCapacity(limitID, 75803)
			decisionID := testutil.MustDeterministicUUID(75804)
			want := constant.ErrUsageCounterExceedsLimit
			if scenario == "duplicate ownership" {
				decisionID = d.Result.EvaluationID
				want = constant.ErrReserveDecisionConflict
			}
			err := inRealTx(t, db, func(tx *sql.Tx) error {
				return repo.ReserveForDecisionWithTx(t.Context(), tx, decisionID, next, decimal.NewFromInt(15), testutil.FixedTime().Add(time.Hour))
			})
			require.ErrorIs(t, err, want)
			_, held := readCounterDecimal(t, db, limitID, res.ScopeKey, res.PeriodKey)
			require.Equal(t, "10.125", held.String())
			var count int
			require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM usage_reservations").Scan(&count))
			require.Equal(t, 1, count)
		})
	}
}
