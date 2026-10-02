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
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// TestWidenAssetColumnsMigration is the behavioral contract for migration
// 000025_widen_asset_columns.
//
// 000025 widens the money-asset column so the tracer stores the asset
// vocabulary the Midaz ledger accepts (1 to 100 uppercase letters):
//   - limits.asset                  VARCHAR(3) -> VARCHAR(100)
//   - transaction_validations.asset CHAR(3)    -> VARCHAR(100)
//
// Post-conditions enforced:
//  1. After migrate-up both columns are character varying(100).
//  2. Before the migration a 6-letter asset does not fit; after it, a limit
//     with asset POINTS persists and reads back through the limit repository.
//  3. Pre-existing 3-letter values read back byte-identical (CHAR padding is
//     not carried into VARCHAR).
//  4. migrate-down on tables holding only short codes restores the original
//     types; with a longer code stored it fails explicitly and leaves the
//     widened column and its data in place.
//  5. A longer code stored only in transaction_validations still refuses the
//     down, and limits is not narrowed either: both tables are checked before
//     either is altered.
//  6. With a conflicting lock held, the up fails with lock_not_available
//     within its lock_timeout instead of queueing.
//  7. An up -> down -> up cycle is idempotent.
//
// Each sub-test provisions its OWN throwaway Postgres container so a step-down
// in one case never leaks schema state into another.
func TestWidenAssetColumnsMigration(t *testing.T) {
	// Sub-tests intentionally do NOT call t.Parallel(): the integration Makefile
	// enforces -p=1 and each case drives its own container to completion.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	tests := []struct {
		name string
		run  func(t *testing.T, mig *migrate.Migrate, db *sql.DB)
	}{
		{
			name: "up_widens_both_asset_columns_to_varchar_100",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB) {
				require.NoError(t, applyWidenMigrationUp(mig), "apply migrations up to 000025")

				dt, ml, ok := columnCharInfo(ctx, t, db, "limits", "asset")
				require.True(t, ok, "limits.asset must exist")
				require.Equal(t, "character varying", dt, "limits.asset must be VARCHAR")
				require.Equal(t, int64(100), ml, "limits.asset must be widened to 100")

				dt, ml, ok = columnCharInfo(ctx, t, db, "transaction_validations", "asset")
				require.True(t, ok, "transaction_validations.asset must exist")
				require.Equal(t, "character varying", dt, "transaction_validations.asset must become VARCHAR")
				require.Equal(t, int64(100), ml, "transaction_validations.asset must be widened to 100")
			},
		},
		{
			name: "long_asset_does_not_fit_before_the_migration",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB) {
				require.NoError(t, mig.Migrate(widenMigrationVersion-1), "migrate up to version 24 (pre-000025)")

				_, err := db.ExecContext(ctx,
					`INSERT INTO limits (name, limit_type, max_amount, asset)
					 VALUES ('pre-widen-points', 'PER_TRANSACTION', 1000, 'POINTS')`)
				require.Error(t, err, "a 6-letter asset must not fit limits.asset VARCHAR(3)")

				var pgErr *pgconn.PgError
				require.ErrorAs(t, err, &pgErr, "the refusal must be a Postgres error")
				require.Equal(t, "22001", pgErr.Code, "the refusal must be string_data_right_truncation")
			},
		},
		{
			name: "points_limit_round_trips_through_the_repository",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB) {
				// The repository reads and writes the HEAD limits schema.
				require.NoError(t, upToHead(mig), "apply migrations up to HEAD")

				accountID := testutil.MustDeterministicUUID(25001)
				createdAt := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

				lmt, err := model.NewLimit(
					"widen-points-limit",
					model.LimitTypePerTransaction,
					decimal.RequireFromString("1000"),
					"POINTS",
					[]model.Scope{{AccountID: &accountID}},
					nil,
					createdAt,
				)
				require.NoError(t, err, "the domain must accept a POINTS limit")

				repo := postgres.NewLimitRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
				require.NoError(t, repo.CreateWithTx(ctx, db, lmt), "persist the POINTS limit")

				got, err := repo.GetByID(ctx, lmt.ID)
				require.NoError(t, err, "read the POINTS limit back")
				require.Equal(t, "POINTS", got.Asset, "asset must round-trip unchanged")

				const txRequest = "25002000-0000-0000-0000-000000000000"

				_, err = db.ExecContext(ctx,
					`INSERT INTO transaction_validations
						(request_id, transaction_type, amount, asset, transaction_timestamp,
						 account, decision, processing_time_ms)
					 VALUES ($1, 'PIX', 500, 'USDT', '2026-01-02T03:04:05Z'::timestamptz,
						 '{}'::jsonb, 'ALLOW', 10)`, txRequest)
				require.NoError(t, err, "a 4-letter asset must fit transaction_validations.asset")
				require.Equal(t, "USDT", readValue(ctx, t, db,
					`SELECT asset FROM transaction_validations WHERE request_id = $1`, txRequest),
					"transaction_validations asset must round-trip unchanged")
			},
		},
		{
			name: "three_letter_values_survive_the_widen_unpadded",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB) {
				const txRequest = "25003000-0000-0000-0000-000000000000"

				require.NoError(t, mig.Migrate(widenMigrationVersion-1), "migrate up to version 24 (pre-000025)")
				seedThreeLetterAssets(ctx, t, db, txRequest)

				require.NoError(t, applyWidenMigrationUp(mig), "apply 000025 over seeded rows")

				require.Equal(t, "BRL", readValue(ctx, t, db,
					`SELECT asset FROM limits WHERE name = 'widen-value-preserve'`),
					"limits value must survive the widen unchanged")
				require.Equal(t, "JPY", readValue(ctx, t, db,
					`SELECT asset FROM transaction_validations WHERE request_id = $1`, txRequest),
					"transaction_validations value must survive CHAR -> VARCHAR without padding")
			},
		},
		{
			name: "down_restores_original_types_when_codes_fit",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB) {
				const txRequest = "25004000-0000-0000-0000-000000000000"

				require.NoError(t, applyWidenMigrationUp(mig), "apply migrations up to 000025")
				seedThreeLetterAssets(ctx, t, db, txRequest)

				require.NoError(t, mig.Steps(-1), "step down 000025")

				dt, ml, ok := columnCharInfo(ctx, t, db, "limits", "asset")
				require.True(t, ok, "limits.asset must still exist after down")
				require.Equal(t, "character varying", dt, "limits.asset must return to VARCHAR")
				require.Equal(t, int64(3), ml, "limits.asset must return to length 3")

				dt, ml, ok = columnCharInfo(ctx, t, db, "transaction_validations", "asset")
				require.True(t, ok, "transaction_validations.asset must still exist after down")
				require.Equal(t, "character", dt, "transaction_validations.asset must return to CHAR")
				require.Equal(t, int64(3), ml, "transaction_validations.asset must return to length 3")

				require.Equal(t, "BRL", readValue(ctx, t, db,
					`SELECT asset FROM limits WHERE name = 'widen-value-preserve'`),
					"limits value must survive the narrow unchanged")
				require.Equal(t, "JPY", readValue(ctx, t, db,
					`SELECT asset FROM transaction_validations WHERE request_id = $1`, txRequest),
					"transaction_validations value must survive the narrow unchanged")
			},
		},
		{
			name: "down_refuses_when_a_longer_code_is_stored",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB) {
				require.NoError(t, applyWidenMigrationUp(mig), "apply migrations up to 000025")

				_, err := db.ExecContext(ctx,
					`INSERT INTO limits (name, limit_type, max_amount, asset)
					 VALUES ('widen-points-blocks-down', 'PER_TRANSACTION', 1000, 'POINTS')`)
				require.NoError(t, err, "seed a POINTS limit after the widen")

				err = mig.Steps(-1)
				require.Error(t, err, "down must refuse to narrow over a 6-letter asset")
				require.Contains(t, err.Error(), "cannot narrow limits.asset", "the refusal must name the blocking table")

				dt, ml, ok := columnCharInfo(ctx, t, db, "limits", "asset")
				require.True(t, ok, "limits.asset must still exist after the refused down")
				require.Equal(t, "character varying", dt, "limits.asset must stay VARCHAR")
				require.Equal(t, int64(100), ml, "limits.asset must stay widened after the refused down")

				require.Equal(t, "POINTS", readValue(ctx, t, db,
					`SELECT asset FROM limits WHERE name = 'widen-points-blocks-down'`),
					"the stored POINTS value must be untouched by the refused down")
			},
		},
		{
			name: "down_refuses_on_a_long_validation_code_and_alters_neither_table",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB) {
				const txRequest = "25005000-0000-0000-0000-000000000000"

				require.NoError(t, applyWidenMigrationUp(mig), "apply migrations up to 000025")

				_, err := db.ExecContext(ctx,
					`INSERT INTO transaction_validations
						(request_id, transaction_type, amount, asset, transaction_timestamp,
						 account, decision, processing_time_ms)
					 VALUES ($1, 'PIX', 500, 'USDT', '2026-01-02T03:04:05Z'::timestamptz,
						 '{}'::jsonb, 'ALLOW', 10)`, txRequest)
				require.NoError(t, err, "seed a USDT validation after the widen; limits holds no long code")

				err = mig.Steps(-1)
				require.Error(t, err, "down must refuse to narrow over a 4-letter validation asset")
				require.Contains(t, err.Error(), "cannot narrow transaction_validations.asset",
					"the refusal must name the blocking table")

				dt, ml, ok := columnCharInfo(ctx, t, db, "limits", "asset")
				require.True(t, ok, "limits.asset must still exist after the refused down")
				require.Equal(t, "character varying", dt, "limits.asset must stay VARCHAR")
				require.Equal(t, int64(100), ml, "limits.asset must not be narrowed by a refused down")

				dt, ml, ok = columnCharInfo(ctx, t, db, "transaction_validations", "asset")
				require.True(t, ok, "transaction_validations.asset must still exist after the refused down")
				require.Equal(t, "character varying", dt, "transaction_validations.asset must stay VARCHAR")
				require.Equal(t, int64(100), ml, "transaction_validations.asset must stay widened")
			},
		},
		{
			name: "up_fails_fast_when_a_conflicting_lock_is_held",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB) {
				require.NoError(t, mig.Migrate(widenMigrationVersion-1), "migrate up to version 24 (pre-000025)")

				holder, err := db.BeginTx(ctx, nil)
				require.NoError(t, err, "open the lock-holding transaction")

				defer func() { _ = holder.Rollback() }()

				_, err = holder.ExecContext(ctx, `LOCK TABLE transaction_validations IN ACCESS SHARE MODE`)
				require.NoError(t, err, "hold a lock that conflicts with ALTER TABLE")

				started := time.Now()
				err = mig.Migrate(widenMigrationVersion)
				elapsed := time.Since(started)

				require.Error(t, err, "the ALTER must give up while the conflicting lock is held")
				require.Contains(t, err.Error(), "55P03", "the failure must be lock_not_available")
				require.Less(t, elapsed, 30*time.Second, "the ALTER must fail within its lock_timeout, not queue")
			},
		},
		{
			name: "up_down_up_cycle_is_idempotent",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB) {
				require.NoError(t, applyWidenMigrationUp(mig), "apply migrations up to 000025")
				require.NoError(t, mig.Steps(-1), "step down 000025 for idempotency check")
				require.NoError(t, applyWidenMigrationUp(mig), "re-apply 000025 (idempotent cycle)")

				_, ml, ok := columnCharInfo(ctx, t, db, "limits", "asset")
				require.True(t, ok, "limits.asset must be present after up/down/up")
				require.Equal(t, int64(100), ml, "limits.asset must be widened after up/down/up")

				_, ml, ok = columnCharInfo(ctx, t, db, "transaction_validations", "asset")
				require.True(t, ok, "transaction_validations.asset must be present after up/down/up")
				require.Equal(t, int64(100), ml, "transaction_validations.asset must be widened after up/down/up")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn := startUpgradePathContainer(ctx, t)
			mig, db := newHeadReservationMigrate(ctx, t, dsn)
			tt.run(t, mig, db)
		})
	}
}

// widenMigrationVersion is the version this file is the contract for
// (000025_widen_asset_columns). The test migrates to exactly this version
// rather than to HEAD so migrations layered on top of 000025 do not change
// which migration a single down step reverses.
const widenMigrationVersion = 25

// applyWidenMigrationUp migrates up to widenMigrationVersion, treating
// migrate.ErrNoChange as success so a re-apply is a clean no-op.
func applyWidenMigrationUp(mig *migrate.Migrate) error {
	if err := mig.Migrate(widenMigrationVersion); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}

// seedThreeLetterAssets inserts one limit (BRL) and one transaction validation
// (JPY, keyed by txRequest) with asset codes that fit both the original and the
// widened column types. Values are fixed literals so the seed is deterministic.
func seedThreeLetterAssets(ctx context.Context, t *testing.T, db *sql.DB, txRequest string) {
	t.Helper()

	_, err := db.ExecContext(ctx,
		`INSERT INTO limits (name, limit_type, max_amount, asset)
		 VALUES ('widen-value-preserve', 'PER_TRANSACTION', 1000, 'BRL')`)
	require.NoError(t, err, "seed limits row")

	_, err = db.ExecContext(ctx,
		`INSERT INTO transaction_validations
			(request_id, transaction_type, amount, asset, transaction_timestamp,
			 account, decision, processing_time_ms)
		 VALUES ($1, 'PIX', 500, 'JPY', '2026-01-02T03:04:05Z'::timestamptz,
			 '{}'::jsonb, 'ALLOW', 10)`, txRequest)
	require.NoError(t, err, "seed transaction_validations row")
}
