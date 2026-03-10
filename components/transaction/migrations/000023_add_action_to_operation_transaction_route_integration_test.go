//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrations

import (
	"database/sql"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMigrator creates a golang-migrate instance configured the same way
// lib-commons does: MultiStatementEnabled = true, file:// source driver.
func newMigrator(t *testing.T, db *sql.DB, migrationsDir string) *migrate.Migrate {
	t.Helper()

	driver, err := postgres.WithInstance(db, &postgres.Config{
		MultiStatementEnabled: true,
		DatabaseName:          pgtestutil.DefaultDBName,
		SchemaName:            "public",
	})
	require.NoError(t, err, "failed to create postgres driver for migrate")

	absDir, err := filepath.Abs(migrationsDir)
	require.NoError(t, err, "failed to resolve migrations path")

	sourceURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absDir)}).String()

	m, err := migrate.NewWithDatabaseInstance(sourceURL, pgtestutil.DefaultDBName, driver)
	require.NoError(t, err, "failed to create migrate instance")

	return m
}

// migrateUpTo runs all up migrations through the given version (inclusive)
// using the same golang-migrate engine that lib-commons uses in production.
func migrateUpTo(t *testing.T, db *sql.DB, migrationsDir string, version int) {
	t.Helper()

	m := newMigrator(t, db, migrationsDir)

	err := m.Migrate(uint(version))
	if err != nil && err != migrate.ErrNoChange {
		t.Fatalf("failed to migrate up to version %d: %v", version, err)
	}
}

// migrateDown steps one migration down using the same golang-migrate engine
// that lib-commons uses in production.
func migrateDown(t *testing.T, db *sql.DB, migrationsDir string) {
	t.Helper()

	m := newMigrator(t, db, migrationsDir)

	err := m.Steps(-1)
	require.NoError(t, err, "failed to step down one migration")
}

// columnExists checks whether a column exists in a given table.
func columnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()

	var exists bool

	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = $1 AND column_name = $2
		)
	`, table, column).Scan(&exists)
	require.NoError(t, err, "failed to check column existence")

	return exists
}

// constraintExists checks whether a constraint exists on a given table.
func constraintExists(t *testing.T, db *sql.DB, table, constraint string) bool {
	t.Helper()

	var exists bool

	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.table_constraints
			WHERE table_name = $1 AND constraint_name = $2
		)
	`, table, constraint).Scan(&exists)
	require.NoError(t, err, "failed to check constraint existence")

	return exists
}

// indexExists checks whether an index exists in the database.
func indexExists(t *testing.T, db *sql.DB, indexName string) bool {
	t.Helper()

	var exists bool

	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes WHERE indexname = $1
		)
	`, indexName).Scan(&exists)
	require.NoError(t, err, "failed to check index existence")

	return exists
}

// setupMigrationTest creates a PostgreSQL container and applies migrations up to
// migration 22 (the state before migration 23), returning the database and
// migrations directory path.
func setupMigrationTest(t *testing.T) (*sql.DB, string) {
	t.Helper()

	container := pgtestutil.SetupContainer(t)
	db := container.DB
	dir := migrationsDir(t)

	// Apply all migrations up to 22 (pre-migration-23 state)
	migrateUpTo(t, db, dir, 22)

	return db, dir
}

// =============================================================================
// IS-1: Migration up applies cleanly on fresh database
// =============================================================================

func TestIntegration_Migration000023_UpAppliesCleanly(t *testing.T) {
	db, dir := setupMigrationTest(t)

	// Verify column does NOT exist before migration 23
	require.False(t, columnExists(t, db, "operation_transaction_route", "action"),
		"action column must not exist before migration 23")

	// Apply migration 23
	migrateUpTo(t, db, dir, 23)

	// Verify column exists after migration
	assert.True(t, columnExists(t, db, "operation_transaction_route", "action"),
		"action column must exist after migration 23")

	// Verify column type and default
	var colDefault sql.NullString
	var isNullable string
	var dataType string

	err := db.QueryRow(`
		SELECT data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_name = 'operation_transaction_route' AND column_name = 'action'
	`).Scan(&dataType, &isNullable, &colDefault)
	require.NoError(t, err, "failed to query column metadata")

	assert.Equal(t, "character varying", dataType, "action column must be VARCHAR")
	assert.Equal(t, "NO", isNullable, "action column must be NOT NULL")
	assert.True(t, colDefault.Valid, "action column must have a default value")
	assert.Contains(t, colDefault.String, "direct", "action column default must be 'direct'")

	// Verify CHECK constraint exists
	assert.True(t, constraintExists(t, db, "operation_transaction_route", "chk_otr_action"),
		"CHECK constraint chk_otr_action must exist")

	// Verify new unique index exists
	assert.True(t, indexExists(t, db, "idx_operation_transaction_route_unique"),
		"unique index must exist")

	// Verify action lookup index exists
	assert.True(t, indexExists(t, db, "idx_operation_transaction_route_action"),
		"action lookup index must exist")
}

// =============================================================================
// IS-2: CHECK constraint rejects invalid action values
// =============================================================================

func TestIntegration_Migration000023_CheckConstraintRejectsInvalid(t *testing.T) {
	db, dir := setupMigrationTest(t)
	migrateUpTo(t, db, dir, 23)

	// Create prerequisite data
	opRouteID := createTestOperationRouteRaw(t, db)
	txRouteID := createTestTransactionRouteRaw(t, db)

	invalidActions := []string{
		"invalid",
		"INVALID",
		"",
		"unknown",
		"debit",
		"credit",
		"transfer",
	}

	for _, action := range invalidActions {
		t.Run("rejects_"+action, func(t *testing.T) {
			_, err := db.Exec(`
				INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, action, created_at)
				VALUES (gen_random_uuid(), $1, $2, $3, NOW())
			`, opRouteID, txRouteID, action)
			require.Error(t, err, "CHECK constraint must reject action %q", action)
			assert.Contains(t, err.Error(), "chk_otr_action",
				"error must reference the CHECK constraint for action %q", action)
		})
	}

	// Verify valid actions are accepted
	validActions := []string{"direct", "hold", "commit", "cancel", "revert"}

	for _, action := range validActions {
		t.Run("accepts_"+action, func(t *testing.T) {
			_, err := db.Exec(`
				INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, action, created_at)
				VALUES (gen_random_uuid(), $1, $2, $3, NOW())
			`, opRouteID, txRouteID, action)
			assert.NoError(t, err, "CHECK constraint must accept action %q", action)
		})
	}
}

// =============================================================================
// IS-3: Unique index prevents duplicates
// =============================================================================

func TestIntegration_Migration000023_UniqueIndexPreventsDuplicates(t *testing.T) {
	db, dir := setupMigrationTest(t)
	migrateUpTo(t, db, dir, 23)

	opRouteID := createTestOperationRouteRaw(t, db)
	txRouteID := createTestTransactionRouteRaw(t, db)

	// Insert first row
	_, err := db.Exec(`
		INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, action, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'direct', NOW())
	`, opRouteID, txRouteID)
	require.NoError(t, err, "first insert must succeed")

	// Insert duplicate (same operation_route_id, transaction_route_id, action)
	_, err = db.Exec(`
		INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, action, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'direct', NOW())
	`, opRouteID, txRouteID)
	require.Error(t, err, "unique index must prevent duplicate (op_route, tx_route, action)")
	assert.Contains(t, err.Error(), "idx_operation_transaction_route_unique",
		"error must reference the unique index")

	// Soft-deleted rows must not conflict with active rows
	_, err = db.Exec(`
		UPDATE operation_transaction_route
		SET deleted_at = NOW()
		WHERE operation_route_id = $1 AND transaction_route_id = $2 AND action = 'direct'
	`, opRouteID, txRouteID)
	require.NoError(t, err, "soft-delete must succeed")

	// Insert same combination again (should succeed because old row is soft-deleted)
	_, err = db.Exec(`
		INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, action, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'direct', NOW())
	`, opRouteID, txRouteID)
	assert.NoError(t, err, "insert must succeed when previous row is soft-deleted")
}

// =============================================================================
// IS-4: Existing rows get backfilled with action = 'direct'
// =============================================================================

func TestIntegration_Migration000023_BackfillExistingRows(t *testing.T) {
	db, dir := setupMigrationTest(t)

	// Insert rows BEFORE migration 23 (no action column yet)
	opRouteID := createTestOperationRouteRaw(t, db)
	txRouteID1 := createTestTransactionRouteRaw(t, db)
	txRouteID2 := createTestTransactionRouteRaw(t, db)

	// Insert pre-existing links
	_, err := db.Exec(`
		INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, created_at)
		VALUES (gen_random_uuid(), $1, $2, NOW())
	`, opRouteID, txRouteID1)
	require.NoError(t, err, "pre-migration insert 1 must succeed")

	_, err = db.Exec(`
		INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, created_at)
		VALUES (gen_random_uuid(), $1, $2, NOW())
	`, opRouteID, txRouteID2)
	require.NoError(t, err, "pre-migration insert 2 must succeed")

	// Apply migration 23
	migrateUpTo(t, db, dir, 23)

	// Verify all existing rows have action = 'direct'
	var count int

	err = db.QueryRow(`
		SELECT COUNT(*) FROM operation_transaction_route
		WHERE operation_route_id = $1 AND action = 'direct'
	`, opRouteID).Scan(&count)
	require.NoError(t, err, "query must succeed")
	assert.Equal(t, 2, count, "all existing rows must be backfilled with action='direct'")

	// Verify no rows have a different action
	var nonDirectCount int

	err = db.QueryRow(`
		SELECT COUNT(*) FROM operation_transaction_route
		WHERE operation_route_id = $1 AND action != 'direct'
	`, opRouteID).Scan(&nonDirectCount)
	require.NoError(t, err, "query must succeed")
	assert.Equal(t, 0, nonDirectCount, "no rows should have action other than 'direct' after backfill")
}

// =============================================================================
// IS-5: Migration down reverses all changes cleanly
// =============================================================================

func TestIntegration_Migration000023_DownReversesChanges(t *testing.T) {
	db, dir := setupMigrationTest(t)
	migrateUpTo(t, db, dir, 23)

	// Verify migration 23 artifacts exist before down
	require.True(t, columnExists(t, db, "operation_transaction_route", "action"),
		"action column must exist before down migration")
	require.True(t, constraintExists(t, db, "operation_transaction_route", "chk_otr_action"),
		"CHECK constraint must exist before down migration")
	require.True(t, indexExists(t, db, "idx_operation_transaction_route_action"),
		"action lookup index must exist before down migration")

	// Apply down migration for 23
	migrateDown(t, db, dir)

	// Verify action column is gone
	assert.False(t, columnExists(t, db, "operation_transaction_route", "action"),
		"action column must not exist after down migration")

	// Verify CHECK constraint is gone
	assert.False(t, constraintExists(t, db, "operation_transaction_route", "chk_otr_action"),
		"CHECK constraint must not exist after down migration")

	// Verify action lookup index is gone
	assert.False(t, indexExists(t, db, "idx_operation_transaction_route_action"),
		"action lookup index must not exist after down migration")

	// Verify original unique index is restored (without action column)
	assert.True(t, indexExists(t, db, "idx_operation_transaction_route_unique"),
		"original unique index must be restored after down migration")

	// Verify the restored unique index works (no action column in uniqueness)
	opRouteID := createTestOperationRouteRaw(t, db)
	txRouteID := createTestTransactionRouteRaw(t, db)

	_, err := db.Exec(`
		INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, created_at)
		VALUES (gen_random_uuid(), $1, $2, NOW())
	`, opRouteID, txRouteID)
	require.NoError(t, err, "first insert must succeed after down migration")

	_, err = db.Exec(`
		INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, created_at)
		VALUES (gen_random_uuid(), $1, $2, NOW())
	`, opRouteID, txRouteID)
	assert.Error(t, err, "restored unique index must prevent duplicates")
}

// =============================================================================
// IS-6: Same operation route can have different actions for same transaction route
// =============================================================================

func TestIntegration_Migration000023_DifferentActionsAllowed(t *testing.T) {
	db, dir := setupMigrationTest(t)
	migrateUpTo(t, db, dir, 23)

	opRouteID := createTestOperationRouteRaw(t, db)
	txRouteID := createTestTransactionRouteRaw(t, db)

	// Insert same (operation_route_id, transaction_route_id) with different actions
	actions := []string{"direct", "hold", "commit", "cancel", "revert"}

	for _, action := range actions {
		_, err := db.Exec(`
			INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, action, created_at)
			VALUES (gen_random_uuid(), $1, $2, $3, NOW())
		`, opRouteID, txRouteID, action)
		assert.NoError(t, err, "insert with action %q must succeed for same route pair", action)
	}

	// Verify all 5 rows exist
	var count int

	err := db.QueryRow(`
		SELECT COUNT(*) FROM operation_transaction_route
		WHERE operation_route_id = $1 AND transaction_route_id = $2 AND deleted_at IS NULL
	`, opRouteID, txRouteID).Scan(&count)
	require.NoError(t, err, "count query must succeed")
	assert.Equal(t, len(actions), count,
		"all %d different actions must coexist for the same route pair", len(actions))
}

// =============================================================================
// Test data helpers (raw SQL, no external fixture dependency)
// =============================================================================

// createTestOperationRouteRaw inserts a minimal operation_route row directly
// into the database and returns its UUID. Uses gen_random_uuid() for the ID
// to avoid importing additional dependencies.
func createTestOperationRouteRaw(t *testing.T, db *sql.DB) string {
	t.Helper()

	var id string

	err := db.QueryRow(`
		INSERT INTO operation_route (id, organization_id, ledger_id, title, description, operation_type, created_at, updated_at)
		VALUES (gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), 'Test Op Route', 'test', 'source', NOW(), NOW())
		RETURNING id
	`).Scan(&id)
	require.NoError(t, err, "failed to create test operation route")

	return id
}

// createTestTransactionRouteRaw inserts a minimal transaction_route row directly
// into the database and returns its UUID.
func createTestTransactionRouteRaw(t *testing.T, db *sql.DB) string {
	t.Helper()

	var id string

	err := db.QueryRow(`
		INSERT INTO transaction_route (id, organization_id, ledger_id, title, description, created_at, updated_at)
		VALUES (gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), 'Test Tx Route', 'test', NOW(), NOW())
		RETURNING id
	`).Scan(&id)
	require.NoError(t, err, "failed to create test transaction route")

	return id
}
