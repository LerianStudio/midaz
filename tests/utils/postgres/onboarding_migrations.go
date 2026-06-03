//go:build integration || chaos

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ApplyOnboardingSchema applies the onboarding component's *.up.sql migration
// files directly to the given database via raw Exec, in ascending file-name
// order.
//
// It exists because the transaction integration harness applies its own schema
// via golang-migrate (which owns the schema_migrations version table). A second
// golang-migrate Up() against the same database with a different source would
// collide on that version table. The onboarding tables (organization, ledger,
// segment, account, ...) are disjoint from the transaction tables and every
// onboarding migration is written with CREATE TABLE/INDEX IF NOT EXISTS, so
// applying the SQL directly is safe and idempotent — exactly what a test fixture
// needs when both schemas must coexist in one Postgres for a full
// fee-resolution path (the fee engine reads accounts/segments via the ledger
// query layer).
func ApplyOnboardingSchema(t *testing.T, db *sql.DB) {
	t.Helper()

	dir := FindMigrationsPath(t, "onboarding")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "failed to read onboarding migrations directory")

	var upFiles []string

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		if strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}

	sort.Strings(upFiles)

	for _, name := range upFiles {
		content, readErr := os.ReadFile(filepath.Join(dir, name))
		require.NoErrorf(t, readErr, "failed to read onboarding migration %s", name)

		_, execErr := db.Exec(string(content))
		require.NoErrorf(t, execErr, "failed to apply onboarding migration %s", name)
	}
}
