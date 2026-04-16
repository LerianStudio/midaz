//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/partitionstate"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
)

// TestIntegration_Bootstrap_DualWritePhase_CreatesInBothTables validates
// the full wiring chain:
//
//  1. Flip partition_migration_state.phase to 'dual_write'
//  2. Construct the partition Reader via newPartitionReader (as config.go does)
//  3. Resolve the phase via resolveInitialPartitionPhase
//  4. Wrap the primary balance repo via wrapBalanceForPhase
//  5. Call Create on the wrapped repo
//  6. Assert the row landed in both legacy and partitioned tables
//
// This is the canonical bootstrap-level acceptance test for the cutover.
func TestIntegration_Bootstrap_DualWritePhase_CreatesInBothTables(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	logger := libZap.InitializeLogger()
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")

	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           container.Config.DBName,
		ReplicaDBName:           container.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	// Force the connection to initialise so partitionReaderDB.GetDB() hits
	// a warm cache on first Phase() call.
	_, err := conn.GetDB()
	require.NoError(t, err, "postgres connection must initialise before wiring")

	t.Cleanup(func() {
		if conn.ConnectionDB != nil {
			_ = (*conn.ConnectionDB).Close()
		}
	})

	// Flip the control table to dual_write before the Reader first looks.
	setPartitionPhase(t, container.DB, partitionstate.PhaseDualWrite)

	// Build exactly the dependencies config.go builds.
	partitionReader := newPartitionReader(conn, logger)
	phase := resolveInitialPartitionPhase(context.Background(), partitionReader, logger)
	require.Equal(t, partitionstate.PhaseDualWrite, phase, "phase must reflect control table state")

	primary, err := balance.NewBalancePostgreSQLRepository(conn)
	require.NoError(t, err)

	wrapped := wrapBalanceForPhase(primary, partitionReader, phase, logger)

	// Sanity check: the wrapper must be a *DualWriteRepository under dual_write.
	_, isDualWrite := wrapped.(*balance.DualWriteRepository)
	require.True(t, isDualWrite, "PhaseDualWrite must install DualWriteRepository; got %T", wrapped)

	// Drive a Create through the wrapped repo and assert both tables see it.
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()

	bal := &mmodel.Balance{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          "@bootstrap-dw",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		Key:            "default",
	}

	require.NoError(t, wrapped.Create(context.Background(), bal), "wrapped Create under dual_write must succeed")

	legacyCount := rowCount(t, container.DB, "balance", bal.ID, ledgerID)
	partitionedCount := rowCount(t, container.DB, balance.BalancePartitionedTableName, bal.ID, ledgerID)

	assert.Equal(t, 1, legacyCount, "row must exist in legacy balance")
	assert.Equal(t, 1, partitionedCount, "row must exist in balance_partitioned")
}

// TestIntegration_Bootstrap_LegacyOnlyPhase_SkipsPartitioned validates the
// default (pre-cutover) path: PhaseLegacyOnly installs the plain
// repository and no row lands in the partitioned shell.
func TestIntegration_Bootstrap_LegacyOnlyPhase_SkipsPartitioned(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	logger := libZap.InitializeLogger()
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")

	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           container.Config.DBName,
		ReplicaDBName:           container.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	_, err := conn.GetDB()
	require.NoError(t, err)

	t.Cleanup(func() {
		if conn.ConnectionDB != nil {
			_ = (*conn.ConnectionDB).Close()
		}
	})

	// The control table default is legacy_only; don't touch it.
	partitionReader := newPartitionReader(conn, logger)
	phase := resolveInitialPartitionPhase(context.Background(), partitionReader, logger)
	require.Equal(t, partitionstate.PhaseLegacyOnly, phase)

	primary, err := balance.NewBalancePostgreSQLRepository(conn)
	require.NoError(t, err)

	wrapped := wrapBalanceForPhase(primary, partitionReader, phase, logger)

	// In legacy_only the returned repository is the primary, not the wrapper.
	_, isDualWrite := wrapped.(*balance.DualWriteRepository)
	assert.False(t, isDualWrite, "PhaseLegacyOnly must install the plain repository")

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()

	bal := &mmodel.Balance{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          "@bootstrap-legacy",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		Key:            "default",
	}

	require.NoError(t, wrapped.Create(context.Background(), bal))

	assert.Equal(t, 1, rowCount(t, container.DB, "balance", bal.ID, ledgerID),
		"row must exist in legacy balance")
	assert.Equal(t, 0, rowCount(t, container.DB, balance.BalancePartitionedTableName, bal.ID, ledgerID),
		"legacy_only must NOT write to partitioned shell")
}

// setPartitionPhase flips partition_migration_state to the desired phase.
// The control table ships with phase=legacy_only (see migration 000021);
// we UPDATE rather than INSERT so the single-row invariant holds.
func setPartitionPhase(t *testing.T, db *sql.DB, phase partitionstate.Phase) {
	t.Helper()

	res, err := db.Exec(
		`UPDATE partition_migration_state SET phase = $1, updated_at = NOW() WHERE id = 1`,
		string(phase),
	)
	require.NoError(t, err, "UPDATE partition_migration_state")

	rows, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rows, "partition_migration_state must have exactly one row")
}

// rowCount returns the number of rows matching (id, ledger_id) in the
// given table.
func rowCount(t *testing.T, db *sql.DB, table, id string, ledgerID uuid.UUID) int {
	t.Helper()

	var count int

	query := "SELECT count(*) FROM " + table + " WHERE id = $1 AND ledger_id = $2"

	require.NoError(t, db.QueryRow(query, id, ledgerID).Scan(&count), "count(%s)", table)

	return count
}
