//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// closedAtMigration is the migration that introduces account.closed_at. The
// tests below drive it directly so the "existing accounts stay open" claim is
// proven against the migration's own SQL rather than a hand-written ALTER.
const closedAtMigration = "000022_add_account_closed_at_column"

// applyOnboardingMigrationFile executes one onboarding migration file (up or
// down) against the given database.
func applyOnboardingMigrationFile(t *testing.T, db *sql.DB, name string) {
	t.Helper()

	path := filepath.Join(pgtestutil.FindMigrationsPath(t, "onboarding"), name)

	content, err := os.ReadFile(path)
	require.NoErrorf(t, err, "failed to read onboarding migration %s", name)

	_, err = db.Exec(string(content))
	require.NoErrorf(t, err, "failed to apply onboarding migration %s", name)
}

// insertAccountRow writes an account with the minimal column set, i.e. without
// naming closed_at. It stands for an account that predates the closing feature.
func insertAccountRow(t *testing.T, db *sql.DB, orgID, ledgerID, accountID uuid.UUID, createdAt time.Time) {
	t.Helper()

	alias := fmt.Sprintf("@closing-%s", accountID.String()[:8])

	_, err := db.Exec(`
		INSERT INTO account (id, name, asset_code, organization_id, ledger_id, status, alias, type, blocked, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
	`, accountID, "Closing Test Account", "USD", orgID, ledgerID, "ACTIVE", alias, "deposit", false, createdAt)
	require.NoError(t, err, "failed to insert test account")
}

// readClosedAt reads the raw closed_at column of one account.
func readClosedAt(t *testing.T, db *sql.DB, accountID uuid.UUID) sql.NullTime {
	t.Helper()

	var closedAt sql.NullTime

	err := db.QueryRow(`SELECT closed_at FROM account WHERE id = $1`, accountID).Scan(&closedAt)
	require.NoError(t, err, "failed to read closed_at")

	return closedAt
}

// TestIntegration_AccountClosingMigrationLeavesExistingAccountsOpen rewinds the
// account table to the shape it had before the closing migration, writes an
// account there, then re-applies the migration. The pre-existing row must come
// back with closed_at NULL: the column carries no default, so no historical
// account is retroactively reported as closed.
func TestIntegration_AccountClosingMigrationLeavesExistingAccountsOpen(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	applyOnboardingMigrationFile(t, container.DB, closedAtMigration+".down.sql")

	var columnCount int

	err := container.DB.QueryRow(`
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'account' AND column_name = 'closed_at'
	`).Scan(&columnCount)
	require.NoError(t, err, "failed to inspect account columns")
	require.Zero(t, columnCount, "the down migration must remove closed_at")

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	legacyAccountID := uuid.Must(libCommons.GenerateUUIDv7())
	insertAccountRow(t, container.DB, orgID, ledgerID, legacyAccountID, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))

	applyOnboardingMigrationFile(t, container.DB, closedAtMigration+".up.sql")

	var dataType, isNullable, columnDefault sql.NullString

	err = container.DB.QueryRow(`
		SELECT data_type, is_nullable, column_default FROM information_schema.columns
		WHERE table_name = 'account' AND column_name = 'closed_at'
	`).Scan(&dataType, &isNullable, &columnDefault)
	require.NoError(t, err, "the up migration must add closed_at")

	assert.Equal(t, "timestamp with time zone", dataType.String, "closed_at must be TIMESTAMPTZ")
	assert.Equal(t, "YES", isNullable.String, "closed_at must be nullable")
	assert.False(t, columnDefault.Valid, "closed_at must carry no default")

	assert.False(t, readClosedAt(t, container.DB, legacyAccountID).Valid, "an account that predates the migration must stay open")

	// A fresh account written after the migration is open as well.
	newAccountID := uuid.Must(libCommons.GenerateUUIDv7())
	insertAccountRow(t, container.DB, orgID, ledgerID, newAccountID, time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC))

	assert.False(t, readClosedAt(t, container.DB, newAccountID).Valid, "a newly created account must be open")
}

// TestIntegration_AccountClosingMigrationIsIdempotent re-applies the up
// migration over an already-migrated schema and over a row that carries a
// closing instant. IF NOT EXISTS must make the re-run a no-op that preserves
// the recorded instant.
func TestIntegration_AccountClosingMigrationIsIdempotent(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	insertAccountRow(t, container.DB, orgID, ledgerID, accountID, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))

	closedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	_, err := container.DB.Exec(`UPDATE account SET closed_at = $1 WHERE id = $2`, closedAt, accountID)
	require.NoError(t, err, "failed to record a closing instant")

	applyOnboardingMigrationFile(t, container.DB, closedAtMigration+".up.sql")

	got := readClosedAt(t, container.DB, accountID)
	require.True(t, got.Valid, "re-running the migration must preserve the closing instant")
	assert.True(t, closedAt.Equal(got.Time), "the closing instant must be unchanged")
}
