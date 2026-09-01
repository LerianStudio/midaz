//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package integration

import (
	"database/sql"
	"testing"

	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file" // file:// migration source driver
	_ "github.com/jackc/pgx/v5/stdlib"                   // register the "pgx" database/sql driver used by SetupContainer
	"github.com/stretchr/testify/require"
)

// expectedTables is the representative table set each module's migrations MUST
// create. These are named explicitly (not "some tables") so a dropped or
// renamed table in a future migration surfaces as a failing assertion.
var expectedTables = map[string][]string{
	"onboarding": {
		"organization",
		"ledger",
		"account",
		"account_type",
		"asset",
		"portfolio",
		"segment",
	},
	"transaction": {
		"transaction",
		"operation",
		"balance",
		"asset_rate",
		"operation_route",
		"transaction_route",
		"operation_transaction_route",
		"transaction_backup_quarantine",
	},
}

// TestIntegration_LedgerMigrations_UpDownIdempotency verifies, for each ledger
// database module (onboarding and transaction), the migration behavior the
// decoupled runner actually exercises. The runner only ever performs `up`, so
// the test asserts:
//
//  1. A fresh full Up creates the representative table set.
//  2. A second Up at the latest version is a no-op (migrate.ErrNoChange).
//  3. A bounded single-step down/up round-trip: Steps(-1) rolls back only the
//     latest migration, and a following Up re-applies it back to latest.
//
// A full down-to-zero is intentionally NOT exercised: it verifies far more than
// the forward-only runner does, and the schema state after a bounded round-trip
// is what the runner and production rely on.
//
// Each module runs against its OWN Postgres container so the two migration sets
// cannot cross-contaminate. The suite respects `-p 1` (containers are created
// and torn down sequentially within the single test process).
func TestIntegration_LedgerMigrations_UpDownIdempotency(t *testing.T) {
	for _, module := range []string{"onboarding", "transaction"} {
		t.Run(module, func(t *testing.T) {
			container := pgtestutil.SetupContainer(t)

			m := newMigrator(t, container.DB, module)

			// 1. Fresh full Up: expect a real migration (not ErrNoChange).
			err := m.Up()
			require.NoErrorf(t, err, "initial Up for module %q must apply migrations", module)

			assertTablesExist(t, container.DB, module)

			// 2. Second Up at latest version must be a no-op.
			err = m.Up()
			require.ErrorIsf(t, err, migrate.ErrNoChange,
				"second Up for module %q must return ErrNoChange", module)

			// 3. Bounded single-step down, then Up again (round-trip). Steps(-1)
			// rolls back only the latest migration; the following Up re-applies
			// it, so it returns no error rather than ErrNoChange.
			err = m.Steps(-1)
			require.NoErrorf(t, err, "single-step down (Steps(-1)) for module %q must succeed", module)

			err = m.Up()
			require.NoErrorf(t, err, "Up after single-step down for module %q must re-apply the migration", module)

			assertTablesExist(t, container.DB, module)
		})
	}
}

// newMigrator builds a *migrate.Migrate bound to an already-open *sql.DB for the
// given module. migrate takes ownership of the driver wrapper only; the caller
// (SetupContainer's t.Cleanup) retains responsibility for closing db.
//
// This test uses raw golang-migrate rather than lib-commons' Migrator on
// purpose: lib-commons exposes only Up (forward-only), but this test needs the
// bounded Steps(-1) single-step rollback plus repeated Up to exercise the
// down/up round-trip. That control flow cannot be expressed through the
// Up-only Migrator, so the raw *migrate.Migrate handle is required here.
func newMigrator(t *testing.T, db *sql.DB, module string) *migrate.Migrate {
	t.Helper()

	migrationsPath := pgtestutil.FindMigrationsPath(t, module)

	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	require.NoErrorf(t, err, "failed to build migrate driver for module %q", module)

	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsPath, "postgres", driver)
	require.NoErrorf(t, err, "failed to build migrator for module %q", module)

	return m
}

// assertTablesExist fails the test unless every table in expectedTables[module]
// is present in the public schema.
func assertTablesExist(t *testing.T, db *sql.DB, module string) {
	t.Helper()

	for _, table := range expectedTables[module] {
		var exists bool

		err := db.QueryRow(
			`SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)`,
			table,
		).Scan(&exists)
		require.NoErrorf(t, err, "failed to query information_schema for table %q (module %q)", table, module)
		require.Truef(t, exists, "expected table %q to exist after Up for module %q", table, module)
	}
}
