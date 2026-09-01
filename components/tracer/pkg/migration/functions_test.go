// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migration

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

func TestParseMigrationFileName(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		wantVersion int
		wantName    string
		wantErr     bool
	}{
		{
			name:        "valid up migration",
			filename:    "000001_create_function.up.sql",
			wantVersion: 1,
			wantName:    "create_function",
			wantErr:     false,
		},
		{
			name:        "multi-word name",
			filename:    "000003_calculate_audit_event_hash.up.sql",
			wantVersion: 3,
			wantName:    "calculate_audit_event_hash",
			wantErr:     false,
		},
		{
			name:     "missing direction (plain sql)",
			filename: "000001_create_function.sql",
			wantErr:  true,
		},
		{
			name:     "down migration not supported",
			filename: "000002_verify_hash_chain.down.sql",
			wantErr:  true,
		},
		{
			name:     "invalid version",
			filename: "abc_create_function.up.sql",
			wantErr:  true,
		},
		{
			name:     "no underscore",
			filename: "000001.up.sql",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, name, err := parseMigrationFileName(tt.filename)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if version != tt.wantVersion {
				t.Errorf("version = %d, want %d", version, tt.wantVersion)
			}

			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestLoadMigrations(t *testing.T) {
	tempDir := t.TempDir()

	testMigrations := map[string]string{
		"000001_first_migration.up.sql":  "CREATE FUNCTION test1();",
		"000002_second_migration.up.sql": "CREATE FUNCTION test2();",
	}

	for filename, content := range testMigrations {
		filePath := filepath.Join(tempDir, filename)
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	migrator := NewFunctionMigrator(nil, tempDir, nil)
	result, err := migrator.loadMigrations(context.Background())
	if err != nil {
		t.Fatalf("loadMigrations() error = %v", err)
	}

	if len(result.Migrations) != 2 {
		t.Errorf("got %d migrations, want 2", len(result.Migrations))
	}

	if result.Migrations[0].Version != 1 {
		t.Errorf("first migration version = %d, want 1", result.Migrations[0].Version)
	}

	if result.Migrations[1].Version != 2 {
		t.Errorf("second migration version = %d, want 2", result.Migrations[1].Version)
	}

	if result.Migrations[0].UpSQL != "CREATE FUNCTION test1();" {
		t.Errorf("first migration up SQL incorrect")
	}
}

func TestLoadMigrations_IgnoresNonUpFiles(t *testing.T) {
	tempDir := t.TempDir()

	files := map[string]string{
		"000001_test.up.sql":   "CREATE FUNCTION test();",
		"000001_test.down.sql": "DROP FUNCTION test();",
		"README.md":            "Documentation",
	}

	for filename, content := range files {
		filePath := filepath.Join(tempDir, filename)
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	migrator := NewFunctionMigrator(nil, tempDir, nil)
	result, err := migrator.loadMigrations(context.Background())
	if err != nil {
		t.Fatalf("loadMigrations() error = %v", err)
	}

	if len(result.Migrations) != 1 {
		t.Errorf("got %d migrations, want 1 (should ignore .down.sql and README.md)", len(result.Migrations))
	}
}

func TestVersion_ReturnsCurrentVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupMock   func(mock sqlmock.Sqlmock)
		wantVersion int
		wantDirty   bool
		wantErr     bool
		errContains string
	}{
		{
			name: "returns version and dirty state",
			setupMock: func(mock sqlmock.Sqlmock) {
				// ensureMigrationsTable
				mock.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
				// getCurrentVersion
				mock.ExpectQuery("SELECT version, dirty FROM").
					WillReturnRows(sqlmock.NewRows([]string{"version", "dirty"}).AddRow(5, false))
			},
			wantVersion: 5,
			wantDirty:   false,
			wantErr:     false,
		},
		{
			name: "returns zero version when table is empty",
			setupMock: func(mock sqlmock.Sqlmock) {
				// ensureMigrationsTable
				mock.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
				// getCurrentVersion - no rows
				mock.ExpectQuery("SELECT version, dirty FROM").
					WillReturnError(sql.ErrNoRows)
			},
			wantVersion: 0,
			wantDirty:   false,
			wantErr:     false,
		},
		{
			name: "returns dirty state",
			setupMock: func(mock sqlmock.Sqlmock) {
				// ensureMigrationsTable
				mock.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
				// getCurrentVersion
				mock.ExpectQuery("SELECT version, dirty FROM").
					WillReturnRows(sqlmock.NewRows([]string{"version", "dirty"}).AddRow(3, true))
			},
			wantVersion: 3,
			wantDirty:   true,
			wantErr:     false,
		},
		{
			name: "error on table creation",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("CREATE TABLE IF NOT EXISTS").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr:     true,
			errContains: "failed to ensure migrations table",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			tc.setupMock(mock)

			migrator := NewFunctionMigrator(db, "/test/path", nil)
			version, dirty, err := migrator.Version(context.Background())

			if tc.wantErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantVersion, version)
				assert.Equal(t, tc.wantDirty, dirty)
			}

			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUp_NonExistentDirectory(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	migrator := NewFunctionMigrator(db, "/non/existent/path", nil)

	err = migrator.Up(context.Background())
	require.NoError(t, err, "Up should return nil when directory doesn't exist")
	require.NoError(t, mock.ExpectationsWereMet(), "no database operations should occur")
}

func TestUp_Success(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Create temp directory with migration files
	tempDir := t.TempDir()

	err = os.WriteFile(
		filepath.Join(tempDir, "000001_test_func.up.sql"),
		[]byte("CREATE FUNCTION test_func() RETURNS void AS $$ BEGIN END; $$ LANGUAGE plpgsql;"),
		0o644,
	)
	require.NoError(t, err)

	// Setup mock expectations
	// 1. ensureMigrationsTable
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	// 2. acquireMigrationLock
	mock.ExpectExec("SELECT pg_advisory_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	// 3. getCurrentVersion - no existing migrations
	mock.ExpectQuery("SELECT version, dirty FROM").WillReturnError(sql.ErrNoRows)
	// 4. Begin transaction for applyMigration
	mock.ExpectBegin()
	// 5. updateVersion (set dirty = true)
	mock.ExpectExec("DELETE FROM").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO").WithArgs(1, true).WillReturnResult(sqlmock.NewResult(1, 1))
	// 6. Execute migration SQL
	mock.ExpectExec("CREATE FUNCTION test_func").WillReturnResult(sqlmock.NewResult(0, 0))
	// 7. updateVersion (set dirty = false)
	mock.ExpectExec("DELETE FROM").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO").WithArgs(1, false).WillReturnResult(sqlmock.NewResult(1, 1))
	// 8. Commit transaction
	mock.ExpectCommit()
	// 9. releaseMigrationLock
	mock.ExpectExec("SELECT pg_advisory_unlock").WillReturnResult(sqlmock.NewResult(0, 0))

	migrator := NewFunctionMigrator(db, tempDir, nil)

	err = migrator.Up(context.Background())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUp_SkipsAlreadyAppliedMigrations(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Create temp directory with migration files
	tempDir := t.TempDir()

	err = os.WriteFile(
		filepath.Join(tempDir, "000001_first.up.sql"),
		[]byte("SELECT 1;"),
		0o644,
	)
	require.NoError(t, err)

	err = os.WriteFile(
		filepath.Join(tempDir, "000002_second.up.sql"),
		[]byte("SELECT 2;"),
		0o644,
	)
	require.NoError(t, err)

	// Setup mock - version already at 1, should only apply migration 2
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("SELECT pg_advisory_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	// Return version 1 (migration 1 already applied)
	mock.ExpectQuery("SELECT version, dirty FROM").
		WillReturnRows(sqlmock.NewRows([]string{"version", "dirty"}).AddRow(1, false))
	// Should only apply migration 2
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO").WithArgs(2, true).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("SELECT 2").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO").WithArgs(2, false).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectExec("SELECT pg_advisory_unlock").WillReturnResult(sqlmock.NewResult(0, 0))

	migrator := NewFunctionMigrator(db, tempDir, nil)

	err = migrator.Up(context.Background())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUp_DirtyStateError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	tempDir := t.TempDir()

	err = os.WriteFile(
		filepath.Join(tempDir, "000001_test.up.sql"),
		[]byte("SELECT 1;"),
		0o644,
	)
	require.NoError(t, err)

	mock.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("SELECT pg_advisory_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	// Return dirty state
	mock.ExpectQuery("SELECT version, dirty FROM").
		WillReturnRows(sqlmock.NewRows([]string{"version", "dirty"}).AddRow(1, true))
	// Should release lock even on error
	mock.ExpectExec("SELECT pg_advisory_unlock").WillReturnResult(sqlmock.NewResult(0, 0))

	migrator := NewFunctionMigrator(db, tempDir, nil)

	err = migrator.Up(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, ErrDirtyMigration)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUp_AcquireLockError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	tempDir := t.TempDir()

	err = os.WriteFile(
		filepath.Join(tempDir, "000001_test.up.sql"),
		[]byte("SELECT 1;"),
		0o644,
	)
	require.NoError(t, err)

	mock.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("SELECT pg_advisory_lock").WillReturnError(sql.ErrConnDone)

	migrator := NewFunctionMigrator(db, tempDir, nil)

	err = migrator.Up(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to acquire migration lock")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUp_MigrationSQLError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	tempDir := t.TempDir()

	err = os.WriteFile(
		filepath.Join(tempDir, "000001_test.up.sql"),
		[]byte("INVALID SQL SYNTAX"),
		0o644,
	)
	require.NoError(t, err)

	mock.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("SELECT pg_advisory_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT version, dirty FROM").WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO").WithArgs(1, true).WillReturnResult(sqlmock.NewResult(1, 1))
	// SQL execution fails
	mock.ExpectExec("INVALID SQL SYNTAX").WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()
	mock.ExpectExec("SELECT pg_advisory_unlock").WillReturnResult(sqlmock.NewResult(0, 0))

	migrator := NewFunctionMigrator(db, tempDir, nil)

	err = migrator.Up(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to apply migration")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadMigrations_DuplicateVersion(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// Create two files with same version
	err := os.WriteFile(
		filepath.Join(tempDir, "000001_first.up.sql"),
		[]byte("SELECT 1;"),
		0o644,
	)
	require.NoError(t, err)

	err = os.WriteFile(
		filepath.Join(tempDir, "000001_second.up.sql"),
		[]byte("SELECT 2;"),
		0o644,
	)
	require.NoError(t, err)

	migrator := NewFunctionMigrator(nil, tempDir, nil)

	_, err = migrator.loadMigrations(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate migration version")
}

func TestLoadMigrations_LogsSkippedFiles(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// Create valid migration
	err := os.WriteFile(
		filepath.Join(tempDir, "000001_valid.up.sql"),
		[]byte("SELECT 1;"),
		0o644,
	)
	require.NoError(t, err)

	// Create invalid migration (no underscore)
	err = os.WriteFile(
		filepath.Join(tempDir, "000002.up.sql"),
		[]byte("SELECT 2;"),
		0o644,
	)
	require.NoError(t, err)

	logger := testutil.NewMockLogger()
	migrator := NewFunctionMigrator(nil, tempDir, logger)

	result, err := migrator.loadMigrations(context.Background())
	require.NoError(t, err)
	assert.Len(t, result.Migrations, 1)
	assert.Len(t, result.SkippedFiles, 1)
	assert.Contains(t, result.SkippedFiles, "000002.up.sql")
}

func TestParseMigrationFileName_ZeroVersion(t *testing.T) {
	t.Parallel()

	_, _, err := parseMigrationFileName("000000_zero_version.up.sql")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidMigrationFile)
	assert.Contains(t, err.Error(), "version must be > 0")
}

func TestRunInTransaction_RollbackOnError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectRollback()

	migrator := NewFunctionMigrator(db, "/test", nil)

	expectedErr := sql.ErrNoRows
	err = migrator.runInTransaction(context.Background(), func(tx *sql.Tx) error {
		return expectedErr
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunInTransaction_CommitSuccess(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()

	migrator := NewFunctionMigrator(db, "/test", nil)

	err = migrator.runInTransaction(context.Background(), func(tx *sql.Tx) error {
		return nil
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
