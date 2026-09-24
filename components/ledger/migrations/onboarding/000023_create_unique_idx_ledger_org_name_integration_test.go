//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrations

import (
	"database/sql"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file" // file:// migration source driver
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

const (
	// versionBeforeUniqueIndex is the last onboarding migration that predates the
	// ledger name uniqueness guarantee.
	versionBeforeUniqueIndex = 22
	// versionUniqueIndex installs idx_ledger_org_name_unique.
	versionUniqueIndex = 23
)

// newOnboardingMigrator binds golang-migrate to an already-open database. The
// Up-only lib-commons Migrator cannot express the version stepping these tests
// need, which is why the raw handle is used here.
func newOnboardingMigrator(t *testing.T, db *sql.DB) *migrate.Migrate {
	t.Helper()

	migrationsPath := pgtestutil.FindMigrationsPath(t, "onboarding")

	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	require.NoError(t, err, "failed to build migrate driver for onboarding")

	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsPath, "postgres", driver)
	require.NoError(t, err, "failed to build migrator for onboarding")

	return m
}

// ledgerNameIndexDefinition returns the index definition of
// idx_ledger_org_name_unique, or an empty string when the index is absent.
func ledgerNameIndexDefinition(t *testing.T, db *sql.DB) string {
	t.Helper()

	return indexDefinition(t, db, "idx_ledger_org_name_unique")
}

// indexDefinition returns the definition of the named public index, or an
// empty string when the index is absent.
func indexDefinition(t *testing.T, db *sql.DB, indexName string) string {
	t.Helper()

	var definition sql.NullString

	err := db.QueryRow(
		`SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1`, indexName,
	).Scan(&definition)
	if err != nil {
		require.ErrorIs(t, err, sql.ErrNoRows, "failed to read pg_indexes")

		return ""
	}

	return definition.String
}

// insertLedgerRow inserts a ledger row directly, bypassing the repository, so a
// test can seed states the application would refuse to create. The row helpers
// here do not reuse the pgtestutil fixtures because those stamp time.Now();
// these tests need fixed timestamps.
func insertLedgerRow(t *testing.T, db *sql.DB, id, orgID uuid.UUID, name string, deletedAt *time.Time) error {
	t.Helper()

	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

	_, err := db.Exec(`
		INSERT INTO ledger (id, name, organization_id, status, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, 'ACTIVE', $4, $4, $5)
	`, id, name, orgID, createdAt, deletedAt)

	return err
}

// insertSegmentRow inserts a segment row directly, bypassing the repository, so
// a test can seed states the application would refuse to create.
func insertSegmentRow(t *testing.T, db *sql.DB, id, orgID, ledgerID uuid.UUID, name string, deletedAt *time.Time) error {
	t.Helper()

	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

	_, err := db.Exec(`
		INSERT INTO segment (id, name, ledger_id, organization_id, status, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, 'ACTIVE', $5, $5, $6)
	`, id, name, ledgerID, orgID, createdAt, deletedAt)

	return err
}

// insertAssetRow inserts an asset row directly, bypassing the repository, so a
// test can seed states the application would refuse to create.
func insertAssetRow(t *testing.T, db *sql.DB, id, orgID, ledgerID uuid.UUID, name, code string, deletedAt *time.Time) error {
	t.Helper()

	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

	_, err := db.Exec(`
		INSERT INTO asset (id, name, type, code, status, ledger_id, organization_id, created_at, updated_at, deleted_at)
		VALUES ($1, $2, 'currency', $3, 'ACTIVE', $4, $5, $6, $6, $7)
	`, id, name, code, ledgerID, orgID, createdAt, deletedAt)

	return err
}

// seedLedger inserts a live ledger with the given name and returns its ID.
func seedLedger(t *testing.T, db *sql.DB, orgID uuid.UUID, name string) uuid.UUID {
	t.Helper()

	id := uuid.New()
	require.NoError(t, insertLedgerRow(t, db, id, orgID, name, nil), "failed to seed ledger")

	return id
}

// TestIntegration_Migration000023_AppliesCleanOnDatabaseWithoutDuplicates covers
// the ordinary deploy: the index is installed, it is UNIQUE, keyed on
// organization_id + LOWER(name), and partial on live rows.
func TestIntegration_Migration000023_AppliesCleanOnDatabaseWithoutDuplicates(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	m := newOnboardingMigrator(t, container.DB)

	require.NoError(t, m.Migrate(versionBeforeUniqueIndex), "failed to migrate to the version before the unique index")
	require.Empty(t, ledgerNameIndexDefinition(t, container.DB), "the index must not exist before migration 000023")

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	require.NoError(t, insertLedgerRow(t, container.DB, uuid.New(), orgID, "Alpha", nil))

	require.NoError(t, m.Migrate(versionUniqueIndex), "migration 000023 must apply on a database without duplicates")

	definition := ledgerNameIndexDefinition(t, container.DB)
	require.NotEmpty(t, definition, "migration 000023 must create idx_ledger_org_name_unique")
	assert.Contains(t, definition, "UNIQUE INDEX")
	assert.Contains(t, definition, "organization_id")
	assert.Contains(t, definition, "lower(name)")
	assert.Contains(t, definition, "deleted_at IS NULL")

	// The guarantee itself: a second live row with the same name in the same
	// organization is rejected, in any case, while a soft-deleted row is not in
	// the way and other organizations are unaffected.
	err := insertLedgerRow(t, container.DB, uuid.New(), orgID, "alpha", nil)
	require.Error(t, err, "a case-insensitive duplicate must be rejected by the index")
	assert.Contains(t, err.Error(), "idx_ledger_org_name_unique")

	deletedAt := time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC)
	require.NoError(t, insertLedgerRow(t, container.DB, uuid.New(), orgID, "Beta", &deletedAt))
	require.NoError(t, insertLedgerRow(t, container.DB, uuid.New(), orgID, "Beta", nil),
		"a soft-deleted ledger must release its name")

	otherOrgID := pgtestutil.CreateTestOrganization(t, container.DB)
	require.NoError(t, insertLedgerRow(t, container.DB, uuid.New(), otherOrgID, "Alpha", nil),
		"uniqueness is scoped to one organization")
}

// TestIntegration_Migration000023_FailsOnPreExistingDuplicates is the recorded
// decision: the migration aborts instead of repairing data. It must leave both
// rows untouched, name the index in the failure, and apply on the next run once
// the duplicate is cleaned up by hand.
func TestIntegration_Migration000023_FailsOnPreExistingDuplicates(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	m := newOnboardingMigrator(t, container.DB)

	require.NoError(t, m.Migrate(versionBeforeUniqueIndex), "failed to migrate to the version before the unique index")

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	keptID := uuid.New()
	duplicateID := uuid.New()

	require.NoError(t, insertLedgerRow(t, container.DB, keptID, orgID, "Dup", nil))
	require.NoError(t, insertLedgerRow(t, container.DB, duplicateID, orgID, "dup", nil),
		"the pre-existing duplicate must be insertable before the index exists")

	err := m.Migrate(versionUniqueIndex)
	require.Error(t, err, "migration 000023 must fail on pre-existing duplicates")
	assert.Contains(t, err.Error(), "idx_ledger_org_name_unique",
		"the failure must name the index so an operator knows what to clean up")

	assert.Empty(t, ledgerNameIndexDefinition(t, container.DB), "a failed build must leave no index behind")

	var rowCount int
	require.NoError(t, container.DB.QueryRow(
		`SELECT COUNT(*) FROM ledger WHERE organization_id = $1 AND LOWER(name) = 'dup'`, orgID,
	).Scan(&rowCount))
	assert.Equal(t, 2, rowCount, "the migration must not alter data when it aborts")

	// Runbook: clean the duplicate by hand, clear the dirty marker left by the
	// aborted run, and migrate again.
	_, execErr := container.DB.Exec(`DELETE FROM ledger WHERE id = $1`, duplicateID)
	require.NoError(t, execErr, "failed to remove the seeded duplicate")

	require.NoError(t, m.Force(versionBeforeUniqueIndex), "failed to clear the dirty migration marker")
	require.NoError(t, m.Migrate(versionUniqueIndex), "migration 000023 must apply once duplicates are cleaned up")
	assert.NotEmpty(t, ledgerNameIndexDefinition(t, container.DB))
}

// TestIntegration_Migration000023_DownRemovesIndex verifies the software
// rollback path: the index disappears and the previous behavior returns.
func TestIntegration_Migration000023_DownRemovesIndex(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	m := newOnboardingMigrator(t, container.DB)

	require.NoError(t, m.Migrate(versionUniqueIndex), "failed to migrate to the unique index version")
	require.NotEmpty(t, ledgerNameIndexDefinition(t, container.DB))

	require.NoError(t, m.Migrate(versionBeforeUniqueIndex), "the down migration must roll the index back")
	assert.Empty(t, ledgerNameIndexDefinition(t, container.DB), "the down migration must drop idx_ledger_org_name_unique")

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	require.NoError(t, insertLedgerRow(t, container.DB, uuid.New(), orgID, "Alpha", nil))
	require.NoError(t, insertLedgerRow(t, container.DB, uuid.New(), orgID, "alpha", nil),
		"without the index the database accepts duplicates again")
}
