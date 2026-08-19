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
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

// TestRenameCurrencyToAssetMigration is the behavioral contract for migration
// 000022_rename_currency_to_asset.
//
// 000022 renames the money-asset column currency -> asset IN PLACE on two
// tables, with NO type change:
//   - limits.currency                 VARCHAR(3) -> asset character varying(3)
//   - transaction_validations.currency CHAR(3)   -> asset character(3)
//
// RENAME COLUMN is metadata-only in PostgreSQL, so type and length are
// preserved; the migration must neither widen nor rewrite. No index covers
// either column, so no ALTER INDEX accompanies the rename.
//
// Post-conditions enforced:
//  1. After migrate-up, both tables expose column `asset` at the ORIGINAL
//     type/length (limits: character varying(3); transaction_validations:
//     character(3)) and no longer expose `currency`.
//  2. migrate-down reverts both tables to `currency` at the original
//     type/length and drops `asset` — losslessly.
//  3. An up -> down -> up cycle is idempotent and lands back at `asset`.
//
// Each sub-test provisions its OWN throwaway Postgres container (via
// startUpgradePathContainer from 10_upgrade_path_test.go) so a step-down in one
// case never leaks schema state into another. The migrate helpers
// (newHeadReservationMigrate / applyMigrateUp) are shared with the Phase 1
// reservation-decimal migration test.
func TestRenameCurrencyToAssetMigration(t *testing.T) {
	// Sub-tests intentionally do NOT call t.Parallel(): the integration Makefile
	// enforces -p=1 and each case drives its own container to completion.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	// Each case gets its OWN throwaway Postgres container + migrate instance so a
	// step-down in one never leaks schema state into another. The run closure
	// receives that fresh pair and owns the migrate sequence for its case.
	tests := []struct {
		name string
		run  func(t *testing.T, mig *migrate.Migrate, db *sql.DB)
	}{
		{
			name: "up_renames_currency_to_asset_preserving_type_and_length",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB) {
				require.NoError(t, applyMigrateUp(mig), "apply HEAD migrations up")

				// limits: VARCHAR(3) is reported as character varying, length 3.
				dt, ml, ok := columnCharInfo(ctx, t, db, "limits", "asset")
				require.True(t, ok, "000022 up must expose limits.asset")
				require.Equal(t, "character varying", dt, "limits.asset must keep the VARCHAR type")
				require.Equal(t, int64(3), ml, "limits.asset must keep length 3 (no widen)")

				_, _, currencyOK := columnCharInfo(ctx, t, db, "limits", "currency")
				require.False(t, currencyOK, "000022 up must drop limits.currency")

				// transaction_validations: CHAR(3) is reported as character, length 3.
				dt, ml, ok = columnCharInfo(ctx, t, db, "transaction_validations", "asset")
				require.True(t, ok, "000022 up must expose transaction_validations.asset")
				require.Equal(t, "character", dt, "transaction_validations.asset must keep the CHAR type")
				require.Equal(t, int64(3), ml, "transaction_validations.asset must keep length 3 (no widen)")

				_, _, currencyOK = columnCharInfo(ctx, t, db, "transaction_validations", "currency")
				require.False(t, currencyOK, "000022 up must drop transaction_validations.currency")
			},
		},
		{
			name: "down_reverts_asset_to_currency_losslessly",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB) {
				require.NoError(t, applyMigrateUp(mig), "apply HEAD migrations up")
				require.NoError(t, mig.Steps(-1), "step down 000022")

				dt, ml, ok := columnCharInfo(ctx, t, db, "limits", "currency")
				require.True(t, ok, "000022 down must restore limits.currency")
				require.Equal(t, "character varying", dt, "restored limits.currency must keep the VARCHAR type")
				require.Equal(t, int64(3), ml, "restored limits.currency must keep length 3")

				_, _, assetOK := columnCharInfo(ctx, t, db, "limits", "asset")
				require.False(t, assetOK, "000022 down must drop limits.asset")

				dt, ml, ok = columnCharInfo(ctx, t, db, "transaction_validations", "currency")
				require.True(t, ok, "000022 down must restore transaction_validations.currency")
				require.Equal(t, "character", dt, "restored transaction_validations.currency must keep the CHAR type")
				require.Equal(t, int64(3), ml, "restored transaction_validations.currency must keep length 3")

				_, _, assetOK = columnCharInfo(ctx, t, db, "transaction_validations", "asset")
				require.False(t, assetOK, "000022 down must drop transaction_validations.asset")
			},
		},
		{
			name: "up_down_up_cycle_is_idempotent",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB) {
				require.NoError(t, applyMigrateUp(mig), "apply HEAD migrations up")
				require.NoError(t, mig.Steps(-1), "step down 000022 for idempotency check")
				require.NoError(t, applyMigrateUp(mig), "re-apply HEAD migrations up (idempotent cycle)")

				_, _, ok := columnCharInfo(ctx, t, db, "limits", "asset")
				require.True(t, ok, "limits.asset must be present after up/down/up")
				_, _, ok = columnCharInfo(ctx, t, db, "transaction_validations", "asset")
				require.True(t, ok, "transaction_validations.asset must be present after up/down/up")
			},
		},
		{
			// RENAME COLUMN is metadata-only, so a stored value cannot change — this
			// case PROVES it: a distinct known value seeded under `currency` (at
			// version 000021, below the rename) must read back byte-identical under
			// `asset` after up, under `currency` again after down, and under `asset`
			// once more after an idempotent up/down/up cycle.
			name: "value_survives_rename_revert_and_recycle_losslessly",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB) {
				const (
					// Distinct values per table so a cross-table mix-up cannot pass.
					limitAsset = "BRL"
					txAsset    = "JPY"
					txRequest  = "11111111-1111-1111-1111-111111111111"
				)

				// Pin at version 000021 (one below the rename) so both columns are
				// still named `currency` and can hold the seeded values.
				require.NoError(t, mig.Migrate(21), "migrate up to version 21 (pre-000022)")

				_, _, ok := columnCharInfo(ctx, t, db, "limits", "currency")
				require.True(t, ok, "pre-000022 limits.currency must exist for the value seed")
				_, _, ok = columnCharInfo(ctx, t, db, "transaction_validations", "currency")
				require.True(t, ok, "pre-000022 transaction_validations.currency must exist for the value seed")

				_, err := db.ExecContext(ctx,
					`INSERT INTO limits (name, limit_type, max_amount, currency)
					 VALUES ('rename-value-preserve', 'PER_TRANSACTION', 1000, $1)`, limitAsset)
				require.NoError(t, err, "seed limits row under currency")

				// transaction_timestamp is a fixed literal (never time.Now) so the
				// seed stays deterministic.
				_, err = db.ExecContext(ctx,
					`INSERT INTO transaction_validations
						(request_id, transaction_type, amount, currency, transaction_timestamp,
						 account, decision, processing_time_ms)
					 VALUES ($1, 'PIX', 500, $2, '2026-01-02T03:04:05Z'::timestamptz,
						 '{}'::jsonb, 'ALLOW', 10)`, txRequest, txAsset)
				require.NoError(t, err, "seed transaction_validations row under currency")

				// UP applies 000022: currency -> asset. Values must be intact.
				require.NoError(t, applyMigrateUp(mig), "apply 000022 up over seeded rows")
				require.Equal(t, limitAsset, readValue(ctx, t, db,
					`SELECT asset FROM limits WHERE name = 'rename-value-preserve'`),
					"limits value must survive the rename to asset unchanged")
				require.Equal(t, txAsset, readValue(ctx, t, db,
					`SELECT asset FROM transaction_validations WHERE request_id = $1`, txRequest),
					"transaction_validations value must survive the rename to asset unchanged")

				// DOWN reverts 000022: asset -> currency. Values must still be intact.
				require.NoError(t, mig.Steps(-1), "step down 000022 over seeded rows")
				require.Equal(t, limitAsset, readValue(ctx, t, db,
					`SELECT currency FROM limits WHERE name = 'rename-value-preserve'`),
					"limits value must survive the revert to currency unchanged")
				require.Equal(t, txAsset, readValue(ctx, t, db,
					`SELECT currency FROM transaction_validations WHERE request_id = $1`, txRequest),
					"transaction_validations value must survive the revert to currency unchanged")

				// UP again (idempotent cycle): value must land back under asset intact.
				require.NoError(t, applyMigrateUp(mig), "re-apply 000022 up (idempotent cycle)")
				require.Equal(t, limitAsset, readValue(ctx, t, db,
					`SELECT asset FROM limits WHERE name = 'rename-value-preserve'`),
					"limits value must survive the up/down/up cycle unchanged")
				require.Equal(t, txAsset, readValue(ctx, t, db,
					`SELECT asset FROM transaction_validations WHERE request_id = $1`, txRequest),
					"transaction_validations value must survive the up/down/up cycle unchanged")
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

// readValue runs a single-column, single-row query and returns the scanned
// string. Used by the value-preservation case to read a seeded asset/currency
// value back under whichever column name the current migration exposes.
func readValue(ctx context.Context, t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()

	var value string

	require.NoError(t, db.QueryRowContext(ctx, query, args...).Scan(&value), "read back seeded value")

	return value
}

// columnCharInfo returns the information_schema data_type and
// character_maximum_length of table.column, plus whether the column exists.
// A missing column yields ("", 0, false); a present non-character column yields
// its data_type with maxLen 0 (character_maximum_length NULL).
func columnCharInfo(ctx context.Context, t *testing.T, db *sql.DB, table, column string) (dataType string, maxLen int64, exists bool) {
	t.Helper()

	var (
		dt sql.NullString
		ml sql.NullInt64
	)

	err := db.QueryRowContext(
		ctx,
		`SELECT data_type, character_maximum_length
		 FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2`,
		table, column,
	).Scan(&dt, &ml)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false
	}
	require.NoError(t, err, "lookup %s.%s column info", table, column)

	return dt.String, ml.Int64, true
}
