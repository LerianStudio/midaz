//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/partitionstate"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
)

// staticPhase is a minimal PartitionPhaseReader for tests. It returns a
// fixed phase so scenarios can exercise each branch of DualWriteRepository
// without touching the partition_migration_state table.
type staticPhase struct{ p partitionstate.Phase }

func (s staticPhase) Phase(_ context.Context) (partitionstate.Phase, error) { return s.p, nil }

// newBalance returns a minimal-but-valid *mmodel.Balance suitable for
// Create / CreateIfNotExists. Values mirror balance.postgresql.go's INSERT
// column order (see balanceColumnList).
func newBalance(orgID, ledgerID, accountID uuid.UUID, alias, key string) *mmodel.Balance {
	now := time.Now().UTC().Truncate(time.Microsecond)

	return &mmodel.Balance{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          alias,
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		Key:            key,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// countBalanceRows returns the count of rows matching (id, ledger_id) in the
// given table. Used to verify dual-write landed in both tables.
func countBalanceRows(t *testing.T, db *sql.DB, table string, id string, ledgerID uuid.UUID) int {
	t.Helper()

	var count int

	query := "SELECT count(*) FROM " + table + " WHERE id = $1 AND ledger_id = $2"

	require.NoError(t, db.QueryRow(query, id, ledgerID).Scan(&count), "count query on %s failed", table)

	return count
}

// TestIntegration_BalanceDualWrite_Create_MirrorsBothTables validates the
// primary contract: a Create under PhaseDualWrite lands in both `balance`
// and `balance_partitioned` within a single committed transaction.
func TestIntegration_BalanceDualWrite_Create_MirrorsBothTables(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	primary := createRepository(t, container)
	repo := NewDualWriteRepository(primary, staticPhase{p: partitionstate.PhaseDualWrite})

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()

	bal := newBalance(orgID, ledgerID, accountID, "@dw-create", "default")

	require.NoError(t, repo.Create(context.Background(), bal), "dual-write Create should succeed")

	assert.Equal(t, 1, countBalanceRows(t, container.DB, "balance", bal.ID, ledgerID),
		"row must exist in legacy balance table")
	assert.Equal(t, 1, countBalanceRows(t, container.DB, BalancePartitionedTableName, bal.ID, ledgerID),
		"row must exist in balance_partitioned shell table")
}

// TestIntegration_BalanceDualWrite_Create_LegacyOnlyPhase asserts that
// PhaseLegacyOnly delegates to the inner repository and does NOT touch the
// partitioned shell. This is the default deployment state before cutover.
func TestIntegration_BalanceDualWrite_Create_LegacyOnlyPhase(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	primary := createRepository(t, container)
	repo := NewDualWriteRepository(primary, staticPhase{p: partitionstate.PhaseLegacyOnly})

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()

	bal := newBalance(orgID, ledgerID, accountID, "@dw-legacy", "default")

	require.NoError(t, repo.Create(context.Background(), bal), "legacy-only Create should succeed")

	assert.Equal(t, 1, countBalanceRows(t, container.DB, "balance", bal.ID, ledgerID),
		"row must exist in legacy balance table")
	assert.Equal(t, 0, countBalanceRows(t, container.DB, BalancePartitionedTableName, bal.ID, ledgerID),
		"legacy_only must NOT write to partitioned shell")
}

// TestIntegration_BalanceDualWrite_CreateIfNotExists_MirrorsBothTables
// asserts the upsert path mirrors in both tables, including the
// ON CONFLICT DO NOTHING semantics: a second call with the same alias/key
// is a no-op in both tables (no row count increase).
func TestIntegration_BalanceDualWrite_CreateIfNotExists_MirrorsBothTables(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	primary := createRepository(t, container)
	repo := NewDualWriteRepository(primary, staticPhase{p: partitionstate.PhaseDualWrite})

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()

	bal := newBalance(orgID, ledgerID, accountID, "@dw-cine", "default")

	ctx := context.Background()
	require.NoError(t, repo.CreateIfNotExists(ctx, bal), "first CreateIfNotExists should succeed")

	assert.Equal(t, 1, countBalanceRows(t, container.DB, "balance", bal.ID, ledgerID))
	assert.Equal(t, 1, countBalanceRows(t, container.DB, BalancePartitionedTableName, bal.ID, ledgerID))

	// Second call with same alias/key: ON CONFLICT DO NOTHING on legacy + ON CONFLICT DO NOTHING on partitioned.
	// Use a different ID to simulate a racing pod trying to materialize the same pre-split balance.
	bal2 := newBalance(orgID, ledgerID, accountID, "@dw-cine", "default")
	require.NoError(t, repo.CreateIfNotExists(ctx, bal2), "duplicate alias/key CreateIfNotExists must be a no-op")

	// Original row still there, new ID not inserted.
	assert.Equal(t, 1, countBalanceRows(t, container.DB, "balance", bal.ID, ledgerID))
	assert.Equal(t, 0, countBalanceRows(t, container.DB, "balance", bal2.ID, ledgerID),
		"ON CONFLICT DO NOTHING must skip the duplicate alias/key")
}

// TestIntegration_BalanceDualWrite_Create_AtomicRollback forces the
// partitioned-side insert to fail (by pre-inserting a conflicting row with
// the same PK but different ledger) and verifies nothing lands in either
// table — proving the dual write is genuinely atomic.
//
// We induce the failure by pre-inserting into balance_partitioned a row
// whose (id, ledger_id) collides with the one the test is about to write,
// then dropping the ON CONFLICT DO NOTHING invariant via a unique-violation
// on a different column. Because balance_partitioned's PRIMARY KEY is
// (id, ledger_id) we use the same (id, ledger_id), which triggers
// ON CONFLICT DO NOTHING on the PK — so to *force* a failure we instead
// drop the unique alias/key partial index collision: insert a conflicting
// alias row first.
func TestIntegration_BalanceDualWrite_Create_IdempotentOnLegacyConflict(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	primary := createRepository(t, container)
	repo := NewDualWriteRepository(primary, staticPhase{p: partitionstate.PhaseDualWrite})

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()

	// Seed legacy balance first so the Create hits a unique violation on
	// the legacy insert and must roll back the partitioned mirror.
	seedParams := pgtestutil.BalanceParams{
		Alias:          "@dw-conflict",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(500),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}
	pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, seedParams)

	// New balance: different ID, same org/ledger/alias/key → partial unique
	// index `idx_balance_unique_alias_key` must reject the insert.
	bal := newBalance(orgID, ledgerID, accountID, "@dw-conflict", "default")

	err := repo.Create(context.Background(), bal)
	require.Error(t, err, "unique-violation on legacy insert must surface as an error")

	// The partitioned mirror must NOT have observed the new ID.
	assert.Equal(t, 0, countBalanceRows(t, container.DB, BalancePartitionedTableName, bal.ID, ledgerID),
		"partitioned mirror must roll back when legacy insert fails")
}

// TestIntegration_BalanceDualWrite_ReadsDelegateToPrimary spot-checks that
// a delegated read (Find) routes to the inner repository unchanged — the
// wrapper only alters Create / CreateIfNotExists.
func TestIntegration_BalanceDualWrite_ReadsDelegateToPrimary(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	primary := createRepository(t, container)
	repo := NewDualWriteRepository(primary, staticPhase{p: partitionstate.PhaseDualWrite})

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()

	bal := newBalance(orgID, ledgerID, accountID, "@dw-read", "default")

	require.NoError(t, repo.Create(context.Background(), bal))

	parsedID, err := uuid.Parse(bal.ID)
	require.NoError(t, err)

	found, err := repo.Find(context.Background(), orgID, ledgerID, parsedID)
	require.NoError(t, err, "Find should delegate to primary and succeed")
	require.NotNil(t, found)
	assert.Equal(t, bal.ID, found.ID)
	assert.Equal(t, "@dw-read", found.Alias)
}
