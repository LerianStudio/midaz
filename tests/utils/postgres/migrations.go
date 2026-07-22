//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file" // file:// migration source driver
	"github.com/stretchr/testify/require"
)

// FindMigrationsPath locates a migrations directory by traversing up from the current directory.
// It looks for the pattern: components/ledger/migrations/{component}
//
// Example:
//
//	path := FindMigrationsPath(t, "onboarding")  // finds components/ledger/migrations/onboarding
//	path := FindMigrationsPath(t, "transaction") // finds components/ledger/migrations/transaction
func FindMigrationsPath(t *testing.T, component string) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err, "failed to get current working directory")

	for {
		candidate := filepath.Join(dir, "components", "ledger", "migrations", component)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find migrations directory for component %q", component)
		}

		dir = parent
	}
}

// ApplyMigrations applies all up migrations for the given module ("onboarding"
// or "transaction") to db via the golang-migrate library. It resolves the
// migration source directory with FindMigrationsPath and runs it against the
// already-open *sql.DB using the postgres database driver.
//
// This is the sanctioned way for integration suites to migrate a testcontainer
// schema now that the application no longer migrates at startup: the migration
// runner image owns production migrations, so tests must apply their own schema.
//
// migrate.WithInstance takes ownership of the driver wrapper only, not the
// *sql.DB; the caller (SetupContainer) retains responsibility for closing db.
func ApplyMigrations(t *testing.T, db *sql.DB, module string) {
	t.Helper()

	migrationsPath := FindMigrationsPath(t, module)

	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	require.NoErrorf(t, err, "failed to build migrate driver for module %q", module)

	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsPath, "postgres", driver)
	require.NoErrorf(t, err, "failed to build migrator for module %q", module)

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		require.NoErrorf(t, err, "failed to apply migrations for module %q", module)
	}
}
