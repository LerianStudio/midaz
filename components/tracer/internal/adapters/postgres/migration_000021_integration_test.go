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

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

func migration000021SQL(t *testing.T, direction string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "migrations", "000021_add_reservation_outcome_delivery."+direction+".sql")
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(contents)
}

func TestIntegration_Migration000021_UpSchemaAndLegacyDefault(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	_, err := db.Exec(migration000021SQL(t, "up"))
	require.NoError(t, err, "up migration must be idempotent")

	var (
		columnDefault string
		nullable      string
	)
	err = db.QueryRow(`
		SELECT column_default, is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'usage_reservations'
		  AND column_name = 'delivery_mode'
	`).Scan(&columnDefault, &nullable)
	require.NoError(t, err)
	require.Contains(t, columnDefault, "LEGACY")
	require.Equal(t, "NO", nullable)

	var receiptPK bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint
			WHERE conrelid = 'reservation_outcome_receipts'::regclass
			  AND contype = 'p'
			  AND pg_get_constraintdef(oid) = 'PRIMARY KEY (transaction_id)'
		)
	`).Scan(&receiptPK)
	require.NoError(t, err)
	require.True(t, receiptPK)

	var supportingIndexes int
	err = db.QueryRow(`
		SELECT COUNT(*)
		FROM pg_indexes
		WHERE schemaname = 'public'
		  AND indexname IN (
			'idx_usage_reservations_reserved_counter',
			'idx_usage_reservations_v2_outstanding'
		  )
	`).Scan(&supportingIndexes)
	require.NoError(t, err)
	require.Equal(t, 2, supportingIndexes)

	var reservationExpiryNullable string
	err = db.QueryRow(`
		SELECT is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'usage_reservations'
		  AND column_name = 'reservation_expires_at'
	`).Scan(&reservationExpiryNullable)
	require.NoError(t, err)
	require.Equal(t, "YES", reservationExpiryNullable)
}

func TestIntegration_Migration000021_OldWorkersCannotMutateV2Holds(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	limitID := createTestLimitNamed(t, db, 9971, "migration-old-worker-v2")
	transactionID := testutil.MustDeterministicUUID(9972)
	now := time.Date(2026, 8, 18, 18, 0, 0, 0, time.UTC)
	reservation := newV2Reservation(t, limitID, transactionID, "v2:9972", "2026-08", 25, now)
	repo := newReservationRepoIntegration(db)
	counterExpiresAt := now.Add(-time.Hour)
	t.Cleanup(func() {
		_, _ = db.Exec(`
			INSERT INTO reservation_outcome_receipts
				(transaction_id, outcome_id, outcome, reservation_count, applied_at)
			VALUES ($1, $2, 'ABORTED', 1, $3)
			ON CONFLICT (transaction_id) DO NOTHING
		`, transactionID, testutil.MustDeterministicUUID(9973), now.Add(time.Minute))
		_, _ = db.Exec(`
			UPDATE usage_reservations
			SET status = 'RELEASED', released_at = $2
			WHERE transaction_id = $1 AND status = 'RESERVED'
		`, transactionID, now.Add(time.Minute))
		cleanupOutcomeTransaction(t, db, transactionID)
		cleanupTestLimit(t, db, limitID)
	})

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, _, err := repo.ReserveWithTx(t.Context(), tx, reservation, 100, &counterExpiresAt)
		return err
	}))

	var expiry sql.NullTime
	require.NoError(t, db.QueryRow(
		"SELECT reservation_expires_at FROM usage_reservations WHERE id = $1", reservation.ID,
	).Scan(&expiry))
	require.False(t, expiry.Valid, "V2 must be invisible to an old reaper that only compares reservation_expires_at")

	rows, err := db.QueryContext(t.Context(), `
		SELECT id FROM usage_reservations
		WHERE status = 'RESERVED' AND reservation_expires_at < $1
	`, now)
	require.NoError(t, err)
	require.False(t, rows.Next(), "legacy reaper query must not discover V2 rows")
	require.NoError(t, rows.Close())

	oldReleaseTx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	_, err = oldReleaseTx.Exec(`
		UPDATE usage_counters
		SET reserved_usage = reserved_usage - $1
		WHERE limit_id = $2 AND scope_key = $3 AND period_key = $4
	`, reservation.Amount, limitID, reservation.ScopeKey, reservation.PeriodKey)
	require.NoError(t, err)
	_, err = oldReleaseTx.Exec(`
		UPDATE usage_reservations
		SET status = 'EXPIRED', released_at = $1
		WHERE id = $2 AND status = 'RESERVED'
	`, now, reservation.ID)
	require.Error(t, err, "database must reject a legacy transition of a live V2 hold")
	require.NoError(t, oldReleaseTx.Rollback())

	result, err := db.Exec(`
		DELETE FROM usage_counters
		WHERE expires_at IS NOT NULL AND expires_at < $1
	`, now)
	require.NoError(t, err)
	deleted, err := result.RowsAffected()
	require.NoError(t, err)
	require.Zero(t, deleted, "legacy cleanup must not delete a counter backing a live V2 hold")

	_, err = db.Exec("DELETE FROM usage_reservations WHERE id = $1", reservation.ID)
	require.Error(t, err, "database must reject deletion of a live V2 hold, including ON DELETE CASCADE")

	current, reserved := readCounter(t, db, limitID, reservation.ScopeKey, reservation.PeriodKey)
	require.Zero(t, current)
	require.Equal(t, reservation.Amount, reserved)
	require.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, reservation.ID))
}

func TestIntegration_Migration000021_DownRefusesLiveReceipt(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	transactionID := testutil.MustDeterministicUUID(9951)
	_, err = tx.Exec(`
		INSERT INTO reservation_outcome_receipts
			(transaction_id, outcome_id, outcome, reservation_count, applied_at)
		VALUES ($1, $2, 'ABORTED', 0, $3)
	`, transactionID, testutil.MustDeterministicUUID(9952), time.Date(2026, 8, 18, 15, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	_, err = tx.Exec(migration000021SQL(t, "down"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "outcome receipts exist")
}

func TestIntegration_Migration000021_DownWaitsForConcurrentReceipt(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	observerDB := testutil.SetupIntegrationDB(t)
	transactionID := testutil.MustDeterministicUUID(9961)

	writerTx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writerTx.Rollback() })

	_, err = writerTx.Exec(`
		INSERT INTO reservation_outcome_receipts
			(transaction_id, outcome_id, outcome, reservation_count, applied_at)
		VALUES ($1, $2, 'ABORTED', 0, $3)
	`, transactionID, testutil.MustDeterministicUUID(9962), time.Date(2026, 8, 18, 16, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	downResult := make(chan error, 1)
	go func() {
		_, downErr := db.Exec(migration000021SQL(t, "down"))
		downResult <- downErr
	}()

	require.Eventually(t, func() bool {
		var waiting bool
		queryErr := observerDB.QueryRow(`
			SELECT EXISTS (
				SELECT 1
				FROM pg_stat_activity
				WHERE pid <> pg_backend_pid()
				  AND query LIKE '%LOCK TABLE reservation_outcome_receipts%'
				  AND wait_event_type = 'Lock'
			)
		`).Scan(&waiting)

		return queryErr == nil && waiting
	}, 5*time.Second, 10*time.Millisecond, "down migration must wait for the outcome writer")

	require.NoError(t, writerTx.Commit())
	err = <-downResult
	require.Error(t, err)
	require.Contains(t, err.Error(), "outcome receipts exist")

	_, err = db.Exec("DELETE FROM reservation_outcome_receipts WHERE transaction_id = $1", transactionID)
	require.NoError(t, err)
}

func TestIntegration_Migration000021_DownRefusesLiveV2Reservation(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	limitID := createTestLimitNamed(t, db, 9953, "migration-down-v2")
	t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })

	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	now := time.Date(2026, 8, 18, 15, 30, 0, 0, time.UTC)
	reservation := newV2Reservation(t, limitID, testutil.MustDeterministicUUID(9954), "v2:9954", "2026-08", 1, now)
	repo := newReservationRepoIntegration(db)
	_, _, err = repo.ReserveWithTx(t.Context(), tx, reservation, 100, nil)
	require.NoError(t, err)

	_, err = tx.Exec(migration000021SQL(t, "down"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "V2 reservations exist")
}

func TestIntegration_Migration000021_DownSucceedsWhenBacklogEmpty(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	tx, err := db.BeginTx(t.Context(), &sql.TxOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	// The suite database is shared within this package. Remove rows only inside
	// this transaction so the down migration sees an empty backlog; rollback
	// restores all schema and data after the assertion.
	_, err = tx.Exec(`
		INSERT INTO reservation_outcome_receipts
			(transaction_id, outcome_id, outcome, reservation_count, applied_at)
		SELECT DISTINCT transaction_id, transaction_id, 'ABORTED', 0, NOW()
		FROM usage_reservations
		WHERE delivery_mode = 'LEDGER_OUTCOME_V2' AND status = 'RESERVED'
		ON CONFLICT (transaction_id) DO NOTHING
	`)
	require.NoError(t, err)
	_, err = tx.Exec(`
		UPDATE usage_reservations AS reservation
		SET status = CASE receipt.outcome
			WHEN 'COMMITTED' THEN 'CONFIRMED'
			ELSE 'RELEASED'
		END,
		confirmed_at = CASE WHEN receipt.outcome = 'COMMITTED' THEN NOW() ELSE confirmed_at END,
		released_at = CASE WHEN receipt.outcome = 'ABORTED' THEN NOW() ELSE released_at END
		FROM reservation_outcome_receipts AS receipt
		WHERE reservation.transaction_id = receipt.transaction_id
		  AND reservation.delivery_mode = 'LEDGER_OUTCOME_V2'
		  AND reservation.status = 'RESERVED'
	`)
	require.NoError(t, err)
	_, err = tx.Exec("DELETE FROM reservation_outcome_receipts")
	require.NoError(t, err)
	_, err = tx.Exec("DELETE FROM usage_reservations")
	require.NoError(t, err)

	_, err = tx.Exec(migration000021SQL(t, "down"))
	require.NoError(t, err)
	_, err = tx.Exec(migration000021SQL(t, "down"))
	require.NoError(t, err, "down migration must be idempotent once V2 state is absent")

	var deliveryModeColumnExists bool
	err = tx.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = 'usage_reservations'
			  AND column_name = 'delivery_mode'
		)
	`).Scan(&deliveryModeColumnExists)
	require.NoError(t, err)
	require.False(t, deliveryModeColumnExists)

	var receiptsTableExists bool
	err = tx.QueryRow("SELECT to_regclass('public.reservation_outcome_receipts') IS NOT NULL").Scan(&receiptsTableExists)
	require.NoError(t, err)
	require.False(t, receiptsTableExists)

	// Pin the domain spelling referenced by both CHECK constraints.
	require.Equal(t, model.DeliveryModeLedgerOutcomeV2, model.ReservationDeliveryMode("LEDGER_OUTCOME_V2"))
}
