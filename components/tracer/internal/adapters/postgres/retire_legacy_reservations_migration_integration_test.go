//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/migrations"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

const (
	retireLegacyReservationsMigration = "000035_retire_legacy_reservations"
	validateDecisionRequiredMigration = "000036_validate_reservation_decision_required"
	featureNotSupported               = "0A000"
)

// databaseBeforeRetirement returns a fresh tenant database migrated up to, but
// not including, the legacy reservation retirement, plus the directory holding
// the materialized migration files.
func databaseBeforeRetirement(t *testing.T) (*sql.DB, string) {
	t.Helper()

	admin := testutil.SetupIntegrationDB(t)
	digest := sha256.Sum256([]byte(t.Name()))
	name := fmt.Sprintf("retire_legacy_%x", digest[:8])

	_, err := admin.ExecContext(t.Context(), "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
		require.NoError(t, err)
	})

	dsn, err := url.Parse(testutil.GetTestDSN())
	require.NoError(t, err)
	dsn.Path = "/" + name
	db, err := sql.Open("pgx", dsn.String())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	dir := t.TempDir()
	require.NoError(t, migrations.WriteTo(dir))
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	require.NoError(t, err)
	sort.Strings(files)

	for _, file := range files {
		if strings.HasPrefix(filepath.Base(file), retireLegacyReservationsMigration) {
			break
		}

		body, err := os.ReadFile(file)
		require.NoError(t, err)
		_, err = db.ExecContext(t.Context(), string(body))
		require.NoError(t, err, filepath.Base(file))
	}

	return db, dir
}

func execMigrationFile(t *testing.T, db *sql.DB, dir, file string) error {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(dir, file))
	require.NoError(t, err)

	_, err = db.ExecContext(t.Context(), string(body))

	return err
}

// seedCounterBucket writes a usage counter bucket holding current and reserved.
func seedCounterBucket(t *testing.T, db *sql.DB, seed int64, limitID uuid.UUID, scopeKey, periodKey, current, reserved string) {
	t.Helper()

	_, err := db.ExecContext(t.Context(), `INSERT INTO usage_counters
		(id, limit_id, scope_key, period_key, current_usage, reserved_usage, last_updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		testutil.MustDeterministicUUID(seed), limitID, scopeKey, periodKey, current, reserved, testutil.FixedTime())
	require.NoError(t, err)
}

// insertLegacyReservation writes a reservation row without a decision, the shape
// the retired writer produced.
func insertLegacyReservation(t *testing.T, db *sql.DB, res *model.Reservation) {
	t.Helper()

	_, err := db.ExecContext(t.Context(), `INSERT INTO usage_reservations
		(id, limit_id, scope_key, period_key, amount, status, transaction_id, reservation_expires_at, created_at, confirmed_at, released_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		res.ID, res.LimitID, res.ScopeKey, res.PeriodKey, res.Amount, string(res.Status), res.TransactionID,
		res.ReservationExpiresAt, res.CreatedAt, res.ConfirmedAt, res.ReleasedAt)
	require.NoError(t, err)
}

func legacyReservation(limitID uuid.UUID, seed int64, scopeKey, amount string, status model.ReservationStatus) *model.Reservation {
	res := &model.Reservation{
		ID: testutil.MustDeterministicUUID(seed), LimitID: limitID, TransactionID: testutil.MustDeterministicUUID(seed + 1),
		ScopeKey: scopeKey, PeriodKey: "2026-09", Amount: decimal.RequireFromString(amount), Status: status,
		ReservationExpiresAt: testutil.FixedTime().Add(time.Hour), CreatedAt: testutil.FixedTime(),
	}

	settledAt := testutil.FixedTime().Add(time.Minute)

	switch status {
	case model.StatusConfirmed:
		res.ConfirmedAt = &settledAt
	case model.StatusReleased, model.StatusExpired:
		res.ReleasedAt = &settledAt
	}

	return res
}

type retiredReservation struct {
	status     string
	amount     decimal.Decimal
	released   bool
	confirmed  bool
	retiredSet bool
}

func readRetiredReservation(t *testing.T, db *sql.DB, id uuid.UUID) retiredReservation {
	t.Helper()

	var r retiredReservation

	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT status, amount, released_at IS NOT NULL,
		confirmed_at IS NOT NULL, retired_at IS NOT NULL FROM retired_legacy_reservations WHERE id = $1`, id).
		Scan(&r.status, &r.amount, &r.released, &r.confirmed, &r.retiredSet))

	return r
}

// decisionReservationRow is the stored state of one reservation row, compared
// whole so any column the retirement touches shows up as a difference.
type decisionReservationRow struct {
	status     string
	amount     decimal.Decimal
	decisionID uuid.NullUUID
	expiresAt  time.Time
	confirmed  sql.NullTime
	released   sql.NullTime
}

func readDecisionReservationRow(t *testing.T, db *sql.DB, id uuid.UUID) decisionReservationRow {
	t.Helper()

	var r decisionReservationRow

	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT status, amount, decision_id, reservation_expires_at,
		confirmed_at, released_at FROM usage_reservations WHERE id = $1`, id).
		Scan(&r.status, &r.amount, &r.decisionID, &r.expiresAt, &r.confirmed, &r.released))

	return r
}

// holdDecisionCapacity writes a decision-owned RESERVED row through the
// production reserve path, adding its amount to the bucket's reserved_usage.
func holdDecisionCapacity(t *testing.T, db *sql.DB, decision model.ReserveDecision, res *model.Reservation) {
	t.Helper()

	decisions, err := NewReserveDecisionRepository(&testutil.IntegrationDBAdapter{DB: db}, 10, 100)
	require.NoError(t, err)

	repo := newReservationRepoIntegration(db)
	decision.Result.ReservationIDs = []uuid.UUID{res.ID}

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		if _, err := NewReserveOperationRepository().LockWithTx(t.Context(), tx, decision.Key.Identity()); err != nil {
			return err
		}

		if err := repo.ReserveForDecisionWithTx(t.Context(), tx, decision.Result.EvaluationID, res,
			decimal.NewFromInt(10000), testutil.FixedTime().Add(time.Hour)); err != nil {
			return err
		}

		return decisions.CreateWithTx(t.Context(), tx, decision)
	}))
}

// TestIntegrationRetireLegacyReservationsExpiresHeldCapacity pins the
// retirement: outstanding legacy holds expire and leave reserved_usage summed
// per bucket, counted spending is untouched, every legacy row moves to the
// retired copy with its history, a decision-owned hold on the same bucket is
// left exactly as it was, and afterwards no row without a decision can be
// written.
func TestIntegrationRetireLegacyReservationsExpiresHeldCapacity(t *testing.T) {
	db, dir := databaseBeforeRetirement(t)
	limitID := createTestLimitNamed(t, db, 76001, "retire")

	// Two holds share one bucket, so the bucket must lose their sum.
	shared, other, drifted := "acct:shared", "acct:other", "acct:drifted"
	seedCounterBucket(t, db, 76002, limitID, shared, "2026-09", "50", "400.5")
	seedCounterBucket(t, db, 76003, limitID, other, "2026-09", "7", "30")
	// A bucket already below its outstanding holds floors at zero.
	seedCounterBucket(t, db, 76004, limitID, drifted, "2026-09", "0", "10")

	heldA := legacyReservation(limitID, 76010, shared, "100.5", model.StatusReserved)
	heldB := legacyReservation(limitID, 76012, shared, "200", model.StatusReserved)
	heldOther := legacyReservation(limitID, 76014, other, "30", model.StatusReserved)
	heldDrifted := legacyReservation(limitID, 76016, drifted, "25", model.StatusReserved)
	// A hold whose bucket the cleanup worker already removed has nothing to return.
	heldOrphan := legacyReservation(limitID, 76018, "acct:orphan", "5", model.StatusReserved)
	confirmed := legacyReservation(limitID, 76020, shared, "50", model.StatusConfirmed)
	released := legacyReservation(limitID, 76022, other, "9", model.StatusReleased)

	for _, res := range []*model.Reservation{heldA, heldB, heldOther, heldDrifted, heldOrphan, confirmed, released} {
		insertLegacyReservation(t, db, res)
	}

	// A decision-owned hold on the shared bucket belongs to the live contract.
	owned := decisionCapacity(limitID, 76024)
	owned.ScopeKey = shared
	holdDecisionCapacity(t, db, persistedDecision(), owned)

	_, reserved := readCounterDecimal(t, db, limitID, shared, "2026-09")
	require.Equal(t, "410.625", reserved.String(), "the decision hold joins the bucket's reserved_usage")

	ownedBefore := readDecisionReservationRow(t, db, owned.ID)

	require.NoError(t, execMigrationFile(t, db, dir, retireLegacyReservationsMigration+".up.sql"))
	require.NoError(t, execMigrationFile(t, db, dir, validateDecisionRequiredMigration+".up.sql"))

	current, reserved := readCounterDecimal(t, db, limitID, shared, "2026-09")
	require.Equal(t, "50", current.String(), "counted spending is untouched")
	require.Equal(t, "110.125", reserved.String(),
		"the bucket loses the sum of its outstanding legacy holds and keeps the decision hold")

	current, reserved = readCounterDecimal(t, db, limitID, other, "2026-09")
	require.Equal(t, "7", current.String())
	require.True(t, reserved.IsZero())

	_, reserved = readCounterDecimal(t, db, limitID, drifted, "2026-09")
	require.True(t, reserved.IsZero(), "a drifted bucket floors at zero instead of failing the migration")

	var remaining int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM usage_reservations WHERE decision_id IS NULL").Scan(&remaining))
	require.Zero(t, remaining, "every row without a decision leaves usage_reservations")

	require.Equal(t, ownedBefore, readDecisionReservationRow(t, db, owned.ID), "the decision-owned hold survives unchanged")
	require.Equal(t, string(model.StatusReserved), ownedBefore.status)
	require.True(t, ownedBefore.amount.Equal(owned.Amount))

	var retiredOwned int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM retired_legacy_reservations WHERE id = $1", owned.ID).Scan(&retiredOwned))
	require.Zero(t, retiredOwned, "a decision-owned hold is not retired")

	for _, res := range []*model.Reservation{heldA, heldB, heldOther, heldDrifted, heldOrphan} {
		retired := readRetiredReservation(t, db, res.ID)
		require.Equal(t, string(model.StatusExpired), retired.status, "an outstanding hold expires")
		require.True(t, retired.released, "the expiry records when the hold was returned")
		require.True(t, retired.amount.Equal(res.Amount))
		require.True(t, retired.retiredSet)
	}

	retired := readRetiredReservation(t, db, confirmed.ID)
	require.Equal(t, string(model.StatusConfirmed), retired.status, "settled history keeps its outcome")
	require.True(t, retired.confirmed)

	retired = readRetiredReservation(t, db, released.ID)
	require.Equal(t, string(model.StatusReleased), retired.status)
	require.True(t, retired.released)

	var validated bool
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT convalidated FROM pg_constraint
		WHERE conname = 'usage_reservations_decision_required'`).Scan(&validated))
	require.True(t, validated)

	var legacyIndex sql.NullString
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT to_regclass('idx_usage_reservations_request')::text").Scan(&legacyIndex))
	require.False(t, legacyIndex.Valid, "the legacy idempotency index is dropped")

	late := legacyReservation(limitID, 76030, shared, "1", model.StatusReserved)
	_, err := db.ExecContext(t.Context(), `INSERT INTO usage_reservations
		(id, limit_id, scope_key, period_key, amount, status, transaction_id, reservation_expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, 'RESERVED', $6, $7, $8)`,
		late.ID, late.LimitID, late.ScopeKey, late.PeriodKey, late.Amount, late.TransactionID, late.ReservationExpiresAt, late.CreatedAt)
	requireCheckViolation(t, err, "a reservation without a decision is rejected")
}

// TestIntegrationRetireLegacyReservationsRefusesRollback pins the down
// migration: restoring retired rows is refused and leaves the retirement intact.
func TestIntegrationRetireLegacyReservationsRefusesRollback(t *testing.T) {
	db, dir := databaseBeforeRetirement(t)
	limitID := createTestLimitNamed(t, db, 76101, "retire-down")
	seedCounterBucket(t, db, 76102, limitID, "acct:down", "2026-09", "0", "12")
	held := legacyReservation(limitID, 76110, "acct:down", "12", model.StatusReserved)
	insertLegacyReservation(t, db, held)

	require.NoError(t, execMigrationFile(t, db, dir, retireLegacyReservationsMigration+".up.sql"))
	require.NoError(t, execMigrationFile(t, db, dir, validateDecisionRequiredMigration+".up.sql"))

	err := execMigrationFile(t, db, dir, retireLegacyReservationsMigration+".down.sql")
	requirePgCode(t, featureNotSupported, err, "the retirement cannot be rolled back")

	require.Equal(t, string(model.StatusExpired), readRetiredReservation(t, db, held.ID).status)

	_, reserved := readCounterDecimal(t, db, limitID, "acct:down", "2026-09")
	require.True(t, reserved.IsZero())

	var validated bool
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT convalidated FROM pg_constraint
		WHERE conname = 'usage_reservations_decision_required'`).Scan(&validated))
	require.True(t, validated, "the refused rollback leaves the decision CHECK in place")
}
