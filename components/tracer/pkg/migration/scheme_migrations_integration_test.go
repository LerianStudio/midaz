// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package migration

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

const (
	// schemeMigrationVersion is 000028_transaction_validations_scheme.
	schemeMigrationVersion = 28
	// schemeIndexMigrationVersion is 000029_transaction_validations_scheme_idx.
	schemeIndexMigrationVersion = 29

	// dashboardSchemeIndexMigrationVersion is
	// 000030_transaction_validations_dashboard_scheme_idx.
	dashboardSchemeIndexMigrationVersion = 30
	// dropDashboardIndexMigrationVersion is
	// 000031_drop_transaction_validations_dashboard_idx.
	dropDashboardIndexMigrationVersion = 31

	schemeIndexName          = "idx_transaction_validations_scheme"
	schemeFunctionName       = "transaction_validation_scheme"
	dashboardIndexName       = "idx_transaction_validations_dashboard"
	dashboardSchemeIndexName = "idx_transaction_validations_dashboard_scheme"
)

// TestTransactionValidationsSchemeMigrations is the behavioral contract for
// migrations 000028 and 000029 on a fresh database.
//
// Post-conditions enforced, in order on one database:
//  1. 000028 up makes transaction_type nullable, adds scheme VARCHAR(50)
//     leaving the enum column's type untouched, and installs
//     transaction_validation_scheme, which prefers scheme over the enum.
//  2. 000029 up builds a valid idx_transaction_validations_scheme; applying it
//     through the single-statement runner is the proof CONCURRENTLY is legal.
//  3. 000029 down drops the index; 000028 down over a table with no NULL
//     transaction_type drops the function and scheme and restores NOT NULL.
//  4. Replaying both ups lands back in state 2 (Migration Renumbering
//     Invariant), clean and at the expected version.
//  5. Once a row stores scheme alone, 000028 down still drops scheme but
//     leaves transaction_type nullable: the table forbids UPDATE and DELETE,
//     so SET NOT NULL could never succeed and must not be attempted.
func TestTransactionValidationsSchemeMigrations(t *testing.T) {
	ctx := context.Background()
	mig, db := newSchemeMigrate(t)

	require.NoError(t, migrateTo(mig, schemeMigrationVersion-1), "migrate to the version before 000028")
	require.Equal(t, "NO", columnNullable(ctx, t, db, "transaction_type"), "transaction_type starts NOT NULL")
	require.False(t, columnExists(ctx, t, db, "scheme"), "scheme must not exist before 000028")

	assertSchemeExpanded := func(t *testing.T) {
		t.Helper()

		require.Equal(t, "YES", columnNullable(ctx, t, db, "transaction_type"), "000028 up must drop NOT NULL on transaction_type")
		require.Equal(t, "transaction_type_enum", columnUDTName(ctx, t, db, "transaction_type"), "000028 must not rewrite the enum column")
		require.True(t, columnExists(ctx, t, db, "scheme"), "000028 up must add scheme")
		require.Equal(t, "YES", columnNullable(ctx, t, db, "scheme"), "scheme must be nullable")
		require.Equal(t, "character varying", columnDataType(ctx, t, db, "scheme"))
		require.Equal(t, int64(50), columnMaxLength(ctx, t, db, "scheme"), "scheme must be VARCHAR(50)")
		require.True(t, functionExists(ctx, t, db, schemeFunctionName), "000028 up must install %s", schemeFunctionName)
		require.Equal(t, "BOLETO", effectiveScheme(ctx, t, db, "BOLETO", "PIX"), "scheme wins over the enum")
		require.Equal(t, "PIX", effectiveScheme(ctx, t, db, "", "PIX"), "a row without scheme falls back to the enum")
		require.Equal(t, "BOLETO", effectiveScheme(ctx, t, db, "BOLETO", ""), "a row without transaction_type reads scheme")
	}

	assertSchemeContracted := func(t *testing.T, wantNullable string) {
		t.Helper()

		require.False(t, columnExists(ctx, t, db, "scheme"), "000028 down must drop scheme")
		require.False(t, functionExists(ctx, t, db, schemeFunctionName), "000028 down must drop %s", schemeFunctionName)
		require.Equal(t, wantNullable, columnNullable(ctx, t, db, "transaction_type"))
		require.False(t, indexExists(ctx, t, db, schemeIndexName), "000029 down must drop the index")
	}

	t.Run("up_000028_opens_transaction_type_without_rewrite", func(t *testing.T) {
		require.NoError(t, mig.Steps(1), "apply 000028 up")
		assertSchemeExpanded(t)
		require.False(t, indexExists(ctx, t, db, schemeIndexName), "the index belongs to 000029")
	})

	t.Run("up_000029_builds_valid_expression_index_concurrently", func(t *testing.T) {
		require.NoError(t, mig.Steps(1), "apply 000029 up")
		require.True(t, indexExists(ctx, t, db, schemeIndexName))
		require.True(t, indexIsValid(ctx, t, db, schemeIndexName), "a CONCURRENTLY build that failed would leave an INVALID index")
		require.Contains(t, indexDefinition(ctx, t, db, schemeIndexName), schemeFunctionName+"(")
		assertVersion(ctx, t, db, schemeIndexMigrationVersion)
	})

	t.Run("down_restores_not_null_when_no_row_lacks_transaction_type", func(t *testing.T) {
		require.NoError(t, mig.Steps(-1), "apply 000029 down")
		require.False(t, indexExists(ctx, t, db, schemeIndexName))
		assertSchemeExpanded(t)

		require.NoError(t, mig.Steps(-1), "apply 000028 down")
		assertSchemeContracted(t, "NO")
		assertVersion(ctx, t, db, schemeMigrationVersion-1)
	})

	t.Run("replay_is_idempotent", func(t *testing.T) {
		require.NoError(t, migrateTo(mig, schemeIndexMigrationVersion), "re-apply 000028 and 000029")
		assertSchemeExpanded(t)
		require.True(t, indexIsValid(ctx, t, db, schemeIndexName))
		assertVersion(ctx, t, db, schemeIndexMigrationVersion)
	})

	t.Run("down_keeps_transaction_type_nullable_once_a_row_stores_scheme_alone", func(t *testing.T) {
		insertSchemeOnlyValidation(ctx, t, db)

		require.NoError(t, mig.Steps(-2), "apply 000029 and 000028 down over a scheme-only row")
		assertSchemeContracted(t, "YES")
		assertVersion(ctx, t, db, schemeMigrationVersion-1)
	})
}

// TestDashboardSchemeCoveringIndexMigrations is the behavioral contract for
// migrations 000030 and 000031.
//
// Post-conditions enforced, in order on one database:
//  1. 000030 up builds a valid idx_transaction_validations_dashboard_scheme
//     beside the 000024 index, keyed on created_at and including scheme.
//  2. 000031 up drops idx_transaction_validations_dashboard, leaving the
//     scheme-aware index as the only dashboard covering index.
//  3. 000031 down recreates the 000024 index valid and with its original
//     INCLUDE list, and 000030 down then drops the scheme-aware index.
//  4. Replaying both ups lands back in state 2 (Migration Renumbering
//     Invariant), clean and at the expected version.
func TestDashboardSchemeCoveringIndexMigrations(t *testing.T) {
	ctx := context.Background()
	mig, db := newSchemeMigrate(t)

	require.NoError(t, migrateTo(mig, dashboardSchemeIndexMigrationVersion-1), "migrate to the version before 000030")
	require.True(t, indexIsValid(ctx, t, db, dashboardIndexName), "000024 must have built the dashboard index")
	require.False(t, indexExists(ctx, t, db, dashboardSchemeIndexName), "the scheme-aware index belongs to 000030")

	assertOnlySchemeAwareIndex := func(t *testing.T) {
		t.Helper()

		require.False(t, indexExists(ctx, t, db, dashboardIndexName), "000031 up must drop the superseded index")
		require.True(t, indexIsValid(ctx, t, db, dashboardSchemeIndexName), "a CONCURRENTLY build that failed would leave an INVALID index")
		require.Contains(t, indexDefinition(ctx, t, db, dashboardSchemeIndexName),
			"(created_at) INCLUDE (decision, transaction_type, asset, amount, processing_time_ms, scheme)")
	}

	t.Run("up_000030_builds_scheme_aware_covering_index_concurrently", func(t *testing.T) {
		require.NoError(t, mig.Steps(1), "apply 000030 up")
		require.True(t, indexIsValid(ctx, t, db, dashboardSchemeIndexName), "a CONCURRENTLY build that failed would leave an INVALID index")
		require.True(t, indexExists(ctx, t, db, dashboardIndexName), "the old index is dropped only by 000031")
		assertVersion(ctx, t, db, dashboardSchemeIndexMigrationVersion)
	})

	t.Run("up_000031_drops_superseded_covering_index_concurrently", func(t *testing.T) {
		require.NoError(t, mig.Steps(1), "apply 000031 up")
		assertOnlySchemeAwareIndex(t)
		assertVersion(ctx, t, db, dropDashboardIndexMigrationVersion)
	})

	t.Run("down_restores_the_000024_index_before_dropping_its_replacement", func(t *testing.T) {
		require.NoError(t, mig.Steps(-1), "apply 000031 down")
		require.True(t, indexIsValid(ctx, t, db, dashboardIndexName), "000031 down must rebuild the 000024 index valid")
		require.Contains(t, indexDefinition(ctx, t, db, dashboardIndexName),
			"(created_at) INCLUDE (decision, transaction_type, asset, amount, processing_time_ms)")
		require.NotContains(t, indexDefinition(ctx, t, db, dashboardIndexName), "scheme",
			"000031 down must restore the 000024 definition, not the scheme-aware one")
		require.True(t, indexExists(ctx, t, db, dashboardSchemeIndexName))

		require.NoError(t, mig.Steps(-1), "apply 000030 down")
		require.False(t, indexExists(ctx, t, db, dashboardSchemeIndexName), "000030 down must drop the scheme-aware index")
		require.True(t, indexIsValid(ctx, t, db, dashboardIndexName))
		assertVersion(ctx, t, db, dashboardSchemeIndexMigrationVersion-1)
	})

	t.Run("replay_is_idempotent", func(t *testing.T) {
		require.NoError(t, migrateTo(mig, dropDashboardIndexMigrationVersion), "re-apply 000030 and 000031")
		assertOnlySchemeAwareIndex(t)
		assertVersion(ctx, t, db, dropDashboardIndexMigrationVersion)
	})
}

// newSchemeMigrate builds a golang-migrate instance over the production
// migrations tree and a dedicated *sql.DB on the suite container. golang-migrate
// is driven directly because the lib-commons Migrator exposes only Up, and
// MultiStatementEnabled stays false, the production runner's mode.
func newSchemeMigrate(t *testing.T) (*migrate.Migrate, *sql.DB) {
	t.Helper()

	db, err := sql.Open("pgx", testutil.GetTestDSN())
	require.NoError(t, err, "open db for scheme migration test")

	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("close scheme migration test db: %v", closeErr)
		}
	})

	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{
		DatabaseName:          "tracer_test",
		SchemaName:            "public",
		MultiStatementEnabled: false,
	})
	require.NoError(t, err, "build migrate postgres driver")

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve this file's path")

	migrationsDir := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")

	mig, err := migrate.NewWithDatabaseInstance("file://"+migrationsDir, "tracer_test", driver)
	require.NoError(t, err, "build migrate instance")

	t.Cleanup(func() {
		if srcErr, _ := mig.Close(); srcErr != nil {
			t.Logf("close scheme migration source: %v", srcErr)
		}
	})

	return mig, db
}

// migrateTo moves to version, treating migrate.ErrNoChange as success.
func migrateTo(mig *migrate.Migrate, version uint) error {
	if err := mig.Migrate(version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}

// insertSchemeOnlyValidation writes one validation that carries a scheme and
// no transaction_type. The table forbids UPDATE and DELETE, so the row stays.
func insertSchemeOnlyValidation(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.ExecContext(ctx, `
		INSERT INTO transaction_validations (
			request_id, transaction_type, scheme, amount, asset, transaction_timestamp,
			account, decision, processing_time_ms
		) VALUES (
			$1, NULL, 'BOLETO', 10, 'BRL', NOW(),
			'{"accountId": "`+uuid.NewString()+`", "type": "deposit", "status": "ACTIVE"}',
			'ALLOW', 1
		)`, uuid.New())
	require.NoError(t, err, "insert a validation that stores scheme alone")
}

// effectiveScheme evaluates transaction_validation_scheme; an empty argument
// is passed as NULL.
func effectiveScheme(ctx context.Context, t *testing.T, db *sql.DB, scheme, transactionType string) string {
	t.Helper()

	var got string

	err := db.QueryRowContext(
		ctx,
		`SELECT transaction_validation_scheme(NULLIF($1, '')::varchar, NULLIF($2, '')::transaction_type_enum)`,
		scheme, transactionType,
	).Scan(&got)
	require.NoError(t, err, "evaluate transaction_validation_scheme(%q, %q)", scheme, transactionType)

	return got
}

func functionExists(ctx context.Context, t *testing.T, db *sql.DB, function string) bool {
	t.Helper()

	var exists bool

	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_proc p
			JOIN pg_namespace n ON n.oid = p.pronamespace
			WHERE n.nspname = 'public' AND p.proname = $1
		)`, function).Scan(&exists)
	require.NoError(t, err, "probe function %s", function)

	return exists
}

func assertVersion(ctx context.Context, t *testing.T, db *sql.DB, want uint) {
	t.Helper()

	var (
		version uint
		dirty   bool
	)

	err := db.QueryRowContext(
		ctx,
		`SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1`,
	).Scan(&version, &dirty)
	require.NoError(t, err, "read schema_migrations")
	require.Equal(t, want, version)
	require.False(t, dirty, "schema_migrations must not be dirty")
}

func columnExists(ctx context.Context, t *testing.T, db *sql.DB, column string) bool {
	t.Helper()

	var exists bool

	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'transaction_validations' AND column_name = $1
		)`, column).Scan(&exists)
	require.NoError(t, err, "probe column %s", column)

	return exists
}

func columnNullable(ctx context.Context, t *testing.T, db *sql.DB, column string) string {
	t.Helper()

	return columnAttribute(ctx, t, db, column, "is_nullable")
}

func columnDataType(ctx context.Context, t *testing.T, db *sql.DB, column string) string {
	t.Helper()

	return columnAttribute(ctx, t, db, column, "data_type")
}

func columnUDTName(ctx context.Context, t *testing.T, db *sql.DB, column string) string {
	t.Helper()

	return columnAttribute(ctx, t, db, column, "udt_name")
}

func columnMaxLength(ctx context.Context, t *testing.T, db *sql.DB, column string) int64 {
	t.Helper()

	var length int64

	err := db.QueryRowContext(ctx, `
		SELECT character_maximum_length FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'transaction_validations' AND column_name = $1`,
		column).Scan(&length)
	require.NoError(t, err, "read character_maximum_length of %s", column)

	return length
}

// columnAttribute reads one text attribute of a transaction_validations column
// from information_schema.columns. attr is one of a fixed set of identifiers
// chosen by the callers above, never caller input.
func columnAttribute(ctx context.Context, t *testing.T, db *sql.DB, column, attr string) string {
	t.Helper()

	var value string

	err := db.QueryRowContext(ctx, `
		SELECT `+attr+` FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'transaction_validations' AND column_name = $1`,
		column).Scan(&value)
	require.NoError(t, err, "read %s of %s", attr, column)

	return value
}

func indexExists(ctx context.Context, t *testing.T, db *sql.DB, index string) bool {
	t.Helper()

	var exists bool

	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE schemaname = 'public' AND tablename = 'transaction_validations' AND indexname = $1
		)`, index).Scan(&exists)
	require.NoError(t, err, "probe index %s", index)

	return exists
}

func indexDefinition(ctx context.Context, t *testing.T, db *sql.DB, index string) string {
	t.Helper()

	var def string

	err := db.QueryRowContext(ctx, `
		SELECT indexdef FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = 'transaction_validations' AND indexname = $1`,
		index).Scan(&def)
	require.NoError(t, err, "read definition of %s", index)

	return def
}

func indexIsValid(ctx context.Context, t *testing.T, db *sql.DB, index string) bool {
	t.Helper()

	var valid bool

	err := db.QueryRowContext(
		ctx,
		`SELECT indisvalid FROM pg_index WHERE indexrelid = to_regclass($1)`, index,
	).Scan(&valid)
	require.NoError(t, err, "read validity of %s", index)

	return valid
}
