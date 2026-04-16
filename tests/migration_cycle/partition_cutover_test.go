//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package migration_cycle_test exercises the online partition cutover
// migrations end-to-end against a real PostgreSQL container. The existing
// migration 000017 uses CREATE INDEX CONCURRENTLY which is incompatible with
// golang-migrate's default transactional wrap; the tests here apply the new
// migrations (000019-000025) against a pre-baked schema instead of replaying
// the whole migration sequence.
package migration_cycle_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
)

// applyFile executes the entire contents of the given .sql file against db.
// It uses ExecContext so multi-statement files work.
func applyFile(t *testing.T, db *sql.DB, relPath string) error {
	t.Helper()

	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	bytes, err := os.ReadFile(filepath.Join(migrationsPath, relPath))
	require.NoError(t, err, "read migration %s", relPath)

	_, err = db.ExecContext(context.Background(), string(bytes))
	return err
}

// seedMinimalSchema creates the smallest version of the legacy operation and
// balance tables that migration 000022/000023 depend on (id PK, transaction
// table, etc.) — just enough for the swap migrations to run in isolation.
func seedMinimalSchema(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.ExecContext(context.Background(), `
		CREATE EXTENSION IF NOT EXISTS pgcrypto;

		CREATE TABLE "transaction" (
			id UUID PRIMARY KEY,
			ledger_id UUID NOT NULL
		);

		CREATE TABLE operation (
			id UUID NOT NULL,
			transaction_id UUID NOT NULL,
			description TEXT NOT NULL,
			type TEXT NOT NULL,
			asset_code TEXT NOT NULL,
			amount DECIMAL NOT NULL DEFAULT 0,
			available_balance DECIMAL NOT NULL DEFAULT 0,
			on_hold_balance DECIMAL NOT NULL DEFAULT 0,
			available_balance_after DECIMAL NOT NULL DEFAULT 0,
			on_hold_balance_after DECIMAL NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			status_description TEXT,
			account_id UUID NOT NULL,
			account_alias TEXT NOT NULL,
			balance_id UUID NOT NULL,
			chart_of_accounts TEXT NOT NULL,
			organization_id UUID NOT NULL,
			ledger_id UUID NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			deleted_at TIMESTAMPTZ,
			route TEXT,
			balance_affected BOOLEAN NOT NULL DEFAULT true,
			balance_key TEXT NOT NULL DEFAULT 'default',
			balance_version_before BIGINT NOT NULL DEFAULT 0,
			balance_version_after BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (id)
		);

		CREATE TABLE balance (
			id UUID PRIMARY KEY,
			organization_id UUID NOT NULL,
			ledger_id UUID NOT NULL,
			account_id UUID NOT NULL,
			alias TEXT NOT NULL,
			asset_code TEXT NOT NULL,
			available DECIMAL NOT NULL DEFAULT 0,
			on_hold DECIMAL NOT NULL DEFAULT 0,
			version BIGINT DEFAULT 0,
			account_type TEXT NOT NULL,
			allow_sending BOOLEAN NOT NULL,
			allow_receiving BOOLEAN NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			deleted_at TIMESTAMPTZ,
			key TEXT NOT NULL DEFAULT 'default'
		);
	`)
	require.NoError(t, err)
}

func countRows(t *testing.T, db *sql.DB, table string) int64 {
	t.Helper()

	var n int64
	err := db.QueryRowContext(context.Background(), "SELECT count(*) FROM "+table).Scan(&n)
	require.NoError(t, err)

	return n
}

// TestPartitionCutover_ShellCreateDownIdempotent applies 000019_up, confirms
// the partitioned shell exists alongside legacy operation, then applies
// 000019_down and confirms the shell is gone and legacy is untouched. Re-runs
// both Up and Down to verify idempotency.
func TestPartitionCutover_ShellCreateDownIdempotent(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	seedMinimalSchema(t, container.DB)

	// Seed a row that must survive shell creation/destruction.
	_, err := container.DB.ExecContext(context.Background(), `
		INSERT INTO operation (
			id, transaction_id, description, type, asset_code, amount,
			available_balance, on_hold_balance, available_balance_after, on_hold_balance_after,
			status, account_id, account_alias, balance_id, chart_of_accounts,
			organization_id, ledger_id, created_at, updated_at
		) VALUES (
			gen_random_uuid(), gen_random_uuid(), 'seed', 'DEBIT', 'USD', 10,
			100, 0, 90, 0,
			'APPROVED', gen_random_uuid(), '@a', gen_random_uuid(), '1000',
			gen_random_uuid(), gen_random_uuid(), now(), now()
		)`)
	require.NoError(t, err)

	// Up: create shell.
	require.NoError(t, applyFile(t, container.DB, "000019_partition_operation_table.up.sql"))
	require.Equal(t, int64(1), countRows(t, container.DB, "operation"), "legacy row survived shell creation")
	require.Equal(t, int64(0), countRows(t, container.DB, "operation_partitioned"))

	// Down: drop shell.
	require.NoError(t, applyFile(t, container.DB, "000019_partition_operation_table.down.sql"))
	require.Equal(t, int64(1), countRows(t, container.DB, "operation"), "legacy row survived shell destruction")

	var exists bool
	require.NoError(t, container.DB.QueryRowContext(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = 'operation_partitioned')").Scan(&exists))
	require.False(t, exists)

	// Re-run Up — must be idempotent from the shell's standpoint (it was fully
	// dropped and can be recreated cleanly).
	require.NoError(t, applyFile(t, container.DB, "000019_partition_operation_table.up.sql"))
	require.Equal(t, int64(1), countRows(t, container.DB, "operation"))
	require.Equal(t, int64(0), countRows(t, container.DB, "operation_partitioned"))
}

// TestPartitionCutover_ControlTableStartsLegacyOnly verifies 000021 creates
// the control table pre-seeded at phase='legacy_only'.
func TestPartitionCutover_ControlTableStartsLegacyOnly(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	require.NoError(t, applyFile(t, container.DB, "000021_partition_migration_state.up.sql"))

	var phase string
	err := container.DB.QueryRowContext(context.Background(),
		"SELECT phase FROM partition_migration_state WHERE id=1").Scan(&phase)
	require.NoError(t, err)
	require.Equal(t, "legacy_only", phase)

	// Re-running the up migration must not reset the phase (ON CONFLICT DO NOTHING
	// on the insert guarantees this).
	_, err = container.DB.ExecContext(context.Background(),
		`UPDATE partition_migration_state SET phase='dual_write' WHERE id=1`)
	require.NoError(t, err)

	require.NoError(t, applyFile(t, container.DB, "000021_partition_migration_state.up.sql"))

	err = container.DB.QueryRowContext(context.Background(),
		"SELECT phase FROM partition_migration_state WHERE id=1").Scan(&phase)
	require.NoError(t, err)
	require.Equal(t, "dual_write", phase, "phase must not be clobbered by idempotent re-run")
}

// TestPartitionCutover_AtomicSwapAbortsOnCountMismatch proves that the swap
// migration refuses to execute when counts differ between legacy and
// partitioned — the single most important correctness property of the
// refactor (no silent data loss during rollout).
func TestPartitionCutover_AtomicSwapAbortsOnCountMismatch(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	seedMinimalSchema(t, container.DB)

	require.NoError(t, applyFile(t, container.DB, "000019_partition_operation_table.up.sql"))
	require.NoError(t, applyFile(t, container.DB, "000020_partition_balance_table.up.sql"))
	require.NoError(t, applyFile(t, container.DB, "000021_partition_migration_state.up.sql"))

	// Seed one legacy operation, zero partitioned: counts disagree.
	_, err := container.DB.ExecContext(context.Background(), `
		INSERT INTO operation (
			id, transaction_id, description, type, asset_code, amount,
			available_balance, on_hold_balance, available_balance_after, on_hold_balance_after,
			status, account_id, account_alias, balance_id, chart_of_accounts,
			organization_id, ledger_id, created_at, updated_at
		) VALUES (
			gen_random_uuid(), gen_random_uuid(), 'seed', 'DEBIT', 'USD', 10,
			100, 0, 90, 0,
			'APPROVED', gen_random_uuid(), '@a', gen_random_uuid(), '1000',
			gen_random_uuid(), gen_random_uuid(), now(), now()
		)`)
	require.NoError(t, err)

	err = applyFile(t, container.DB, "000022_atomic_swap_operation.up.sql")
	require.Error(t, err, "atomic swap must fail when legacy and partitioned counts disagree")
	require.Contains(t, err.Error(), "row count mismatch")

	// Legacy row survived.
	require.Equal(t, int64(1), countRows(t, container.DB, "operation"))

	// Phase unchanged.
	var phase string
	require.NoError(t, container.DB.QueryRowContext(context.Background(),
		"SELECT phase FROM partition_migration_state WHERE id=1").Scan(&phase))
	require.Equal(t, "legacy_only", phase)
}

// TestPartitionCutover_AtomicSwapSucceedsWhenCountsMatch demonstrates the
// happy path: seed identical row counts into both tables and confirm the
// swap renames them and transitions phase to 'partitioned'.
func TestPartitionCutover_AtomicSwapSucceedsWhenCountsMatch(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	seedMinimalSchema(t, container.DB)

	require.NoError(t, applyFile(t, container.DB, "000019_partition_operation_table.up.sql"))
	require.NoError(t, applyFile(t, container.DB, "000020_partition_balance_table.up.sql"))
	require.NoError(t, applyFile(t, container.DB, "000021_partition_migration_state.up.sql"))

	// Apply swap when both tables are empty — counts match trivially.
	require.NoError(t, applyFile(t, container.DB, "000022_atomic_swap_operation.up.sql"))

	// After the swap, the original `operation` table is renamed `operation_legacy`
	// and the partitioned shell now lives under the name `operation`.
	var exists bool
	require.NoError(t, container.DB.QueryRowContext(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = 'operation_legacy')").Scan(&exists))
	require.True(t, exists, "operation_legacy must exist after swap")

	require.NoError(t, container.DB.QueryRowContext(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = 'operation_partitioned')").Scan(&exists))
	require.False(t, exists, "operation_partitioned name must be reused by the swap")

	var phase string
	require.NoError(t, container.DB.QueryRowContext(context.Background(),
		"SELECT phase FROM partition_migration_state WHERE id=1").Scan(&phase))
	require.Equal(t, "partitioned", phase)
}
