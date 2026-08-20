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
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// TestReservationAmountsDecimalMigration is the behavioral contract for
// migration 000021_reservation_amounts_to_decimal.
//
// Context: usage_counters.reserved_usage (000018) and usage_reservations.amount
// (000019) were introduced as BIGINT AFTER 000005 decimalized current_usage /
// max_amount. They hold a whole asset UNIT (the reservation path stores the
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
//  5. An existing integer BIGINT VALUE survives the ::decimal cast bit-exact —
//     the direct cast neither divides by 100 nor otherwise mutates the number.
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

		require.NoError(t, applyDecimalMigrationUp(mig), "apply migrations up to 000021")

		require.Equal(t, "numeric",
			reservationColumnType(ctx, t, db, "usage_counters", "reserved_usage"),
			"000021 up must convert usage_counters.reserved_usage BIGINT -> numeric")
		require.Equal(t, "numeric",
			reservationColumnType(ctx, t, db, "usage_reservations", "amount"),
			"000021 up must convert usage_reservations.amount BIGINT -> numeric")

		// Idempotent up: reverse 000021 then re-apply — must land back at numeric.
		require.NoError(t, mig.Steps(-1), "step down 000021 for idempotency check")
		require.NoError(t, applyDecimalMigrationUp(mig), "re-apply migrations up to 000021 (idempotent cycle)")
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

		require.NoError(t, applyDecimalMigrationUp(mig), "apply migrations up to 000021")

		limitID := insertReservationTestLimit(ctx, t, db)

		// A fractional reserved_usage is only insertable once 000021 up has made
		// the column numeric; this write is itself proof the column widened.
		_, err := db.ExecContext(ctx, `
			INSERT INTO usage_counters (limit_id, scope_key, period_key, current_usage, reserved_usage)
			VALUES ($1, 'scope-frac', 'period-frac', 0, 10.5)`, limitID)
		require.NoError(t, err, "insert fractional reserved_usage (requires numeric column)")

		// Assert the abort reason, not merely that some error occurred: the guard must
		// fail through its RAISE EXCEPTION, not through an unrelated fault that also
		// happens to error. The message is the down migration's own exception text.
		require.ErrorContains(t, mig.Steps(-1),
			"cannot downgrade: fractional reservation values present",
			"000021 down MUST abort via its fractional-guard RAISE EXCEPTION, never a silent ROUND")
	})

	t.Run("down_reverts_to_bigint_when_all_integer", func(t *testing.T) {
		dsn := startUpgradePathContainer(ctx, t)
		mig, db := newHeadReservationMigrate(ctx, t, dsn)

		require.NoError(t, applyDecimalMigrationUp(mig), "apply migrations up to 000021")

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

	t.Run("up_preserves_existing_bigint_values", func(t *testing.T) {
		dsn := startUpgradePathContainer(ctx, t)
		mig, db := newHeadReservationMigrate(ctx, t, dsn)

		// Stop one short of 000021 so the reservation-seam columns are still BIGINT
		// and can hold a pre-existing integer value the cast must preserve.
		require.NoError(t, mig.Migrate(20), "migrate up to version 20 (pre-000021)")
		require.Equal(t, "bigint",
			reservationColumnType(ctx, t, db, "usage_counters", "reserved_usage"),
			"pre-000021 usage_counters.reserved_usage must still be bigint")
		require.Equal(t, "bigint",
			reservationColumnType(ctx, t, db, "usage_reservations", "amount"),
			"pre-000021 usage_reservations.amount must still be bigint")

		// Seed a distinctive integer value into BOTH reservation-seam columns. A
		// value-corrupting cast (e.g. an unwanted /100) would turn 12345 into 123.
		const wantInt = 12345
		want := decimal.NewFromInt(wantInt)

		limitID := insertReservationTestLimit(ctx, t, db)

		_, err := db.ExecContext(ctx, `
			INSERT INTO usage_counters (limit_id, scope_key, period_key, current_usage, reserved_usage)
			VALUES ($1, 'scope-preserve', 'period-preserve', 0, $2)`, limitID, wantInt)
		require.NoError(t, err, "insert integer reserved_usage before the cast")

		_, err = db.ExecContext(ctx, `
			INSERT INTO usage_reservations
				(limit_id, scope_key, period_key, amount, transaction_id, reservation_expires_at)
			VALUES ($1, 'scope-preserve', 'period-preserve', $2, gen_random_uuid(), NOW() + INTERVAL '5 minutes')`,
			limitID, wantInt)
		require.NoError(t, err, "insert integer amount before the cast")

		// Advance to version 21, applying 000021's BIGINT -> DECIMAL cast.
		require.NoError(t, applyDecimalMigrationUp(mig), "apply 000021 up")

		require.Equal(t, "numeric",
			reservationColumnType(ctx, t, db, "usage_counters", "reserved_usage"),
			"000021 up must convert usage_counters.reserved_usage to numeric")
		require.Equal(t, "numeric",
			reservationColumnType(ctx, t, db, "usage_reservations", "amount"),
			"000021 up must convert usage_reservations.amount to numeric")

		// Read the post-cast values back as exact decimals: the cast must preserve
		// 12345 bit-exact, not divide, round, or truncate it.
		var gotReserved, gotAmount decimal.Decimal

		require.NoError(t, db.QueryRowContext(
			ctx,
			`SELECT reserved_usage FROM usage_counters
			 WHERE scope_key = 'scope-preserve' AND period_key = 'period-preserve'`,
		).Scan(&gotReserved), "read back reserved_usage after cast")
		require.NoError(t, db.QueryRowContext(
			ctx,
			`SELECT amount FROM usage_reservations
			 WHERE scope_key = 'scope-preserve' AND period_key = 'period-preserve'`,
		).Scan(&gotAmount), "read back amount after cast")

		require.True(t, want.Equal(gotReserved),
			"000021 cast must preserve usage_counters.reserved_usage exactly; want %s got %s", want, gotReserved)
		require.True(t, want.Equal(gotAmount),
			"000021 cast must preserve usage_reservations.amount exactly; want %s got %s", want, gotAmount)
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

// decimalMigrationVersion is the version this file is the contract for
// (000021_reservation_amounts_to_decimal). The test migrates to exactly this
// version rather than to HEAD so migrations layered on top of 000021 do not
// change which migration a single down step reverses, nor which columns exist
// while the reservation-decimal behaviour is exercised.
const decimalMigrationVersion = 21

// applyDecimalMigrationUp migrates up to decimalMigrationVersion, treating
// migrate.ErrNoChange as success so a re-apply on an already-migrated DB is a
// clean no-op.
func applyDecimalMigrationUp(mig *migrate.Migrate) error {
	if err := mig.Migrate(decimalMigrationVersion); err != nil && !errors.Is(err, migrate.ErrNoChange) {
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
// their limit_id foreign key. The asset column is written by its pre-000022 name
// (currency) because this test operates at version 000021, below the rename.
func insertReservationTestLimit(ctx context.Context, t *testing.T, db *sql.DB) uuid.UUID {
	t.Helper()

	var id uuid.UUID

	err := db.QueryRowContext(
		ctx,
		`INSERT INTO limits (name, limit_type, max_amount, currency)
		 VALUES ('reservation-decimal-test', 'PER_TRANSACTION', 1000, 'USD')
		 RETURNING id`,
	).Scan(&id)
	require.NoError(t, err, "insert test limit")

	return id
}
