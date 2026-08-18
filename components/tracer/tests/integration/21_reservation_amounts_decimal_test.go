// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

// TestReservationAmountsDecimalMigration is the behavioral contract for
// migration 000021_reservation_amounts_to_decimal.
//
// Context: usage_counters.reserved_usage (000018) and usage_reservations.amount
// (000019) were introduced as BIGINT AFTER 000005 decimalized current_usage /
// max_amount. They hold a whole currency UNIT (the reservation path stores the
// result of IntPart), not cents, so 000021 converts them BIGINT -> DECIMAL with
// a DIRECT cast and NO divide-by-100.
//
// Post-conditions enforced:
//  1. After migrate-up, both columns report data_type = 'numeric'.
//  2. Re-applying the up migration (down one step, then up again) is idempotent
//     and lands back at 'numeric'.
//  3. migrate-down ABORTS (RAISE EXCEPTION) when any persisted reservation value
//     carries a fractional part — it must fail loud, never ROUND/truncate.
//  4. migrate-down reverts both columns to 'bigint' when every persisted value
//     is integral.
//
// Each sub-test provisions its OWN throwaway Postgres container (via
// startUpgradePathContainer from 10_upgrade_path_test.go): the fractional-abort
// case leaves schema_migrations dirty by design, so it must never share a DB
// with another case.
func TestReservationAmountsDecimalMigration(t *testing.T) {
	// Sub-tests below intentionally do NOT call t.Parallel(): the integration
	// Makefile enforces -p=1 and each case drives its own container to
	// completion; parallelism would only invite Docker contention.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	t.Run("up_converts_reservation_amounts_to_numeric", func(t *testing.T) {
		dsn := startUpgradePathContainer(ctx, t)
		mig, db := newHeadReservationMigrate(ctx, t, dsn)

		require.NoError(t, applyMigrateUp(mig), "apply HEAD migrations up")

		require.Equal(t, "numeric",
			reservationColumnType(ctx, t, db, "usage_counters", "reserved_usage"),
			"000021 up must convert usage_counters.reserved_usage BIGINT -> numeric")
		require.Equal(t, "numeric",
			reservationColumnType(ctx, t, db, "usage_reservations", "amount"),
			"000021 up must convert usage_reservations.amount BIGINT -> numeric")

		// Idempotent up: reverse 000021 then re-apply — must land back at numeric.
		require.NoError(t, mig.Steps(-1), "step down 000021 for idempotency check")
		require.NoError(t, applyMigrateUp(mig), "re-apply HEAD migrations up (idempotent cycle)")
		require.Equal(t, "numeric",
			reservationColumnType(ctx, t, db, "usage_counters", "reserved_usage"),
			"re-applied 000021 up must be idempotent (numeric)")
		require.Equal(t, "numeric",
			reservationColumnType(ctx, t, db, "usage_reservations", "amount"),
			"re-applied 000021 up must be idempotent (numeric)")
	})

	t.Run("down_aborts_when_fractional_values_present", func(t *testing.T) {
		dsn := startUpgradePathContainer(ctx, t)
		mig, db := newHeadReservationMigrate(ctx, t, dsn)

		require.NoError(t, applyMigrateUp(mig), "apply HEAD migrations up")

		limitID := insertReservationTestLimit(ctx, t, db)

		// A fractional reserved_usage is only insertable once 000021 up has made
		// the column numeric; this write is itself proof the column widened.
		_, err := db.ExecContext(ctx, `
			INSERT INTO usage_counters (limit_id, scope_key, period_key, current_usage, reserved_usage)
			VALUES ($1, 'scope-frac', 'period-frac', 0, 10.5)`, limitID)
		require.NoError(t, err, "insert fractional reserved_usage (requires numeric column)")

		require.Error(t, mig.Steps(-1),
			"000021 down MUST abort when fractional reservation values are present "+
				"(RAISE EXCEPTION, never a silent ROUND)")
	})

	t.Run("down_reverts_to_bigint_when_all_integer", func(t *testing.T) {
		dsn := startUpgradePathContainer(ctx, t)
		mig, db := newHeadReservationMigrate(ctx, t, dsn)

		require.NoError(t, applyMigrateUp(mig), "apply HEAD migrations up")

		limitID := insertReservationTestLimit(ctx, t, db)

		_, err := db.ExecContext(ctx, `
			INSERT INTO usage_counters (limit_id, scope_key, period_key, current_usage, reserved_usage)
			VALUES ($1, 'scope-int', 'period-int', 0, 42)`, limitID)
		require.NoError(t, err, "insert integer reserved_usage")

		require.NoError(t, mig.Steps(-1),
			"000021 down must succeed when every reservation value is integral")
		require.Equal(t, "bigint",
			reservationColumnType(ctx, t, db, "usage_counters", "reserved_usage"),
			"000021 down must revert usage_counters.reserved_usage numeric -> bigint")
		require.Equal(t, "bigint",
			reservationColumnType(ctx, t, db, "usage_reservations", "amount"),
			"000021 down must revert usage_reservations.amount numeric -> bigint")
	})
}

// newHeadReservationMigrate builds a golang-migrate instance bound to the HEAD
// migrations tree and a dedicated *sql.DB over dsn. Both Up() and Steps(-1) are
// exercised here (libPostgres.Migrator exposes only Up), so we drive
// golang-migrate directly — the same pattern as applyLegacySchemaMigrationsUpTo
// in 10_upgrade_path_test.go.
func newHeadReservationMigrate(ctx context.Context, t *testing.T, dsn string) (*migrate.Migrate, *sql.DB) {
	t.Helper()

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err, "open db for reservation-decimal migration test")

	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("close reservation-decimal test db: %v", closeErr)
		}
	})

	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{
		DatabaseName:          "tracer_test",
		SchemaName:            "public",
		MultiStatementEnabled: false,
	})
	require.NoError(t, err, "build migrate postgres driver")

	mig, err := migrate.NewWithDatabaseInstance(
		"file://"+resolveHeadMigrationsDir(ctx, t), "tracer_test", driver,
	)
	require.NoError(t, err, "build migrate instance")

	t.Cleanup(func() {
		// mig.Close() intentionally does NOT close the caller-owned *sql.DB when
		// built via WithInstance; the db.Close cleanup above owns that.
		if srcErr, _ := mig.Close(); srcErr != nil {
			t.Logf("close reservation-decimal migrate source: %v", srcErr)
		}
	})

	return mig, db
}

// applyMigrateUp runs mig.Up(), treating migrate.ErrNoChange as success so a
// re-apply on an already-migrated DB is a clean no-op.
func applyMigrateUp(mig *migrate.Migrate) error {
	if err := mig.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}

// reservationColumnType returns the information_schema data_type of table.column.
func reservationColumnType(ctx context.Context, t *testing.T, db *sql.DB, table, column string) string {
	t.Helper()

	var dataType string

	err := db.QueryRowContext(
		ctx,
		`SELECT data_type FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2`,
		table, column,
	).Scan(&dataType)
	require.NoError(t, err, "lookup %s.%s data_type", table, column)

	return dataType
}

// insertReservationTestLimit inserts a minimal PER_TRANSACTION limit (which
// satisfies every limits CHECK constraint without custom dates or a time window)
// and returns its id, so usage_counters / usage_reservations rows can satisfy
// their limit_id foreign key.
func insertReservationTestLimit(ctx context.Context, t *testing.T, db *sql.DB) string {
	t.Helper()

	var id string

	err := db.QueryRowContext(
		ctx,
		`INSERT INTO limits (name, limit_type, max_amount, currency)
		 VALUES ('reservation-decimal-test', 'PER_TRANSACTION', 1000, 'USD')
		 RETURNING id`,
	).Scan(&id)
	require.NoError(t, err, "insert test limit")

	return id
}
