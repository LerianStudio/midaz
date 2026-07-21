// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	libLog "github.com/LerianStudio/lib-observability/log"
)

const (
	functionsMigrationsTable = "schema_migrations_functions"
	// Advisory lock ID for function migrations (arbitrary number)
	migrationLockID = 9876543210
)

var (
	ErrNoFunctionsDirectory = errors.New("functions directory does not exist")
	ErrInvalidMigrationFile = errors.New("invalid migration file format")
	ErrDirtyMigration       = errors.New("database is in dirty state - manual intervention required")
)

// FunctionMigrator handles PostgreSQL function migrations.
type FunctionMigrator struct {
	db            *sql.DB
	functionsPath string
	logger        libLog.Logger
}

// NewFunctionMigrator creates a new function migrator.
// If logger is nil, warnings about skipped files will not be logged.
func NewFunctionMigrator(db *sql.DB, functionsPath string, logger libLog.Logger) *FunctionMigrator {
	return &FunctionMigrator{
		db:            db,
		functionsPath: functionsPath,
		logger:        logger,
	}
}

// Up applies all pending function migrations.
func (m *FunctionMigrator) Up(ctx context.Context) error {
	if _, err := os.Stat(m.functionsPath); os.IsNotExist(err) {
		return nil
	}

	if err := m.ensureMigrationsTable(ctx); err != nil {
		return fmt.Errorf("failed to ensure migrations table: %w", err)
	}

	// Acquire migration lock to prevent concurrent execution
	if err := m.acquireMigrationLock(ctx); err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}

	defer func() {
		if err := m.releaseMigrationLock(ctx); err != nil {
			// Log but don't fail on lock release errors
			if m.logger != nil {
				m.logger.With(libLog.Err(err)).Log(ctx, libLog.LevelWarn, "failed to release migration lock")
			}
		}
	}()

	currentVersion, dirty, err := m.getCurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	if dirty {
		return fmt.Errorf("%w: version %d is marked as dirty", ErrDirtyMigration, currentVersion)
	}

	result, err := m.loadMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	for _, migration := range result.Migrations {
		if migration.Version <= currentVersion {
			continue
		}

		if err := m.applyMigration(ctx, migration); err != nil {
			return fmt.Errorf("failed to apply migration %06d: %w", migration.Version, err)
		}
	}

	return nil
}

// Version returns the current migration version.
func (m *FunctionMigrator) Version(ctx context.Context) (int, bool, error) {
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return 0, false, fmt.Errorf("failed to ensure migrations table: %w", err)
	}

	return m.getCurrentVersion(ctx)
}

// Migration represents a single migration file.
type Migration struct {
	Version int
	Name    string
	UpSQL   string
}

// ensureMigrationsTable creates the migrations tracking table if it doesn't exist.
func (m *FunctionMigrator) ensureMigrationsTable(ctx context.Context) error {
	query := `CREATE TABLE IF NOT EXISTS ` + functionsMigrationsTable + ` (
		version BIGINT PRIMARY KEY,
		dirty BOOLEAN NOT NULL DEFAULT FALSE
	)`

	_, err := m.db.ExecContext(ctx, query)

	return err
}

// acquireMigrationLock acquires a PostgreSQL advisory lock to prevent concurrent migrations.
func (m *FunctionMigrator) acquireMigrationLock(ctx context.Context) error {
	query := `SELECT pg_advisory_lock($1)`

	_, err := m.db.ExecContext(ctx, query, migrationLockID)
	if err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}

	return nil
}

// releaseMigrationLock releases the PostgreSQL advisory lock.
func (m *FunctionMigrator) releaseMigrationLock(ctx context.Context) error {
	query := `SELECT pg_advisory_unlock($1)`

	_, err := m.db.ExecContext(ctx, query, migrationLockID)
	if err != nil {
		return fmt.Errorf("failed to release migration lock: %w", err)
	}

	return nil
}

// getCurrentVersion returns the current migration version and dirty state.
func (m *FunctionMigrator) getCurrentVersion(ctx context.Context) (int, bool, error) {
	query := `SELECT version, dirty FROM ` + functionsMigrationsTable + ` ORDER BY version DESC LIMIT 1`

	var (
		version int
		dirty   bool
	)

	err := m.db.QueryRowContext(ctx, query).Scan(&version, &dirty)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}

		return 0, false, err
	}

	return version, dirty, nil
}

// MigrationLoadResult contains the result of loading migrations.
type MigrationLoadResult struct {
	Migrations   []Migration
	SkippedFiles []string
}

// loadMigrations reads all migration files from the functions directory.
// Returns migrations and a list of skipped files (invalid format).
func (m *FunctionMigrator) loadMigrations(ctx context.Context) (MigrationLoadResult, error) {
	if _, err := os.Stat(m.functionsPath); os.IsNotExist(err) {
		return MigrationLoadResult{}, ErrNoFunctionsDirectory
	}

	files, err := os.ReadDir(m.functionsPath)
	if err != nil {
		return MigrationLoadResult{}, fmt.Errorf("failed to read functions directory: %w", err)
	}

	// Create a scoped root to prevent directory traversal (Go 1.24+)
	// This ensures all file access is constrained under functionsPath
	root, err := os.OpenRoot(m.functionsPath)
	if err != nil {
		return MigrationLoadResult{}, fmt.Errorf("failed to create root scope for %s: %w", m.functionsPath, err)
	}
	defer root.Close()

	migrations := make([]Migration, 0, len(files))
	skippedFiles := make([]string, 0)
	seenVersions := make(map[int]string)

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".up.sql") {
			continue
		}

		version, migrationName, err := parseMigrationFileName(file.Name())
		if err != nil {
			// Use pluggable logger if available
			if m.logger != nil {
				m.logger.With(
					libLog.String("file", file.Name()),
					libLog.Err(err),
				).Log(ctx, libLog.LevelWarn, "skipping migration file")
			}

			skippedFiles = append(skippedFiles, file.Name())

			continue
		}

		// Check for duplicate version numbers
		if existingFile, exists := seenVersions[version]; exists {
			return MigrationLoadResult{}, fmt.Errorf("duplicate migration version %06d: both %s and %s", version, existingFile, file.Name())
		}

		seenVersions[version] = file.Name()

		// Use root.Open instead of os.ReadFile to prevent directory traversal (gosec G304)
		// file.Name() contains only the filename without path, making this safe
		f, err := root.Open(file.Name())
		if err != nil {
			return MigrationLoadResult{}, fmt.Errorf("failed to open migration file %s: %w", file.Name(), err)
		}

		content, err := io.ReadAll(f)
		_ = f.Close() // Close immediately after reading, error intentionally ignored

		if err != nil {
			return MigrationLoadResult{}, fmt.Errorf("failed to read migration file %s: %w", file.Name(), err)
		}

		migrations = append(migrations, Migration{
			Version: version,
			Name:    migrationName,
			UpSQL:   string(content),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return MigrationLoadResult{
		Migrations:   migrations,
		SkippedFiles: skippedFiles,
	}, nil
}

// runInTransaction executes a function within a database transaction with automatic rollback on error.
func (m *FunctionMigrator) runInTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// parseMigrationFileName parses migration file name: 000001_name.up.sql.
func parseMigrationFileName(filename string) (version int, name string, err error) {
	if !strings.HasSuffix(filename, ".up.sql") {
		return 0, "", fmt.Errorf("%w: must end with .up.sql", ErrInvalidMigrationFile)
	}

	filename = strings.TrimSuffix(filename, ".up.sql")

	parts := strings.Split(filename, "_")
	if len(parts) < 2 {
		return 0, "", fmt.Errorf("%w: must be in format 000001_name.up.sql", ErrInvalidMigrationFile)
	}

	version, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf("%w: invalid version number", ErrInvalidMigrationFile)
	}

	if version <= 0 {
		return 0, "", fmt.Errorf("%w: version must be > 0", ErrInvalidMigrationFile)
	}

	name = strings.Join(parts[1:], "_")

	return version, name, nil
}

// applyMigration applies a single migration.
func (m *FunctionMigrator) applyMigration(ctx context.Context, migration Migration) error {
	return m.runInTransaction(ctx, func(tx *sql.Tx) error {
		if err := m.updateVersion(ctx, tx, migration.Version, true); err != nil {
			return fmt.Errorf("failed to set dirty flag: %w", err)
		}

		if _, err := tx.ExecContext(ctx, migration.UpSQL); err != nil {
			return fmt.Errorf("failed to execute migration SQL: %w", err)
		}

		if err := m.updateVersion(ctx, tx, migration.Version, false); err != nil {
			return fmt.Errorf("failed to update version: %w", err)
		}

		return nil
	})
}

// updateVersion sets the current migration version (keeps only latest).
func (m *FunctionMigrator) updateVersion(ctx context.Context, tx *sql.Tx, version int, dirty bool) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM `+functionsMigrationsTable); err != nil {
		return err
	}

	_, err := tx.ExecContext(ctx, `INSERT INTO `+functionsMigrationsTable+` (version, dirty) VALUES ($1, $2)`, version, dirty)

	return err
}
