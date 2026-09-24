//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrations

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

const (
	// versionAssetUniqueIndex installs idx_asset_ledger_name_unique and
	// idx_asset_ledger_code_unique; the version before it is
	// versionSegmentUniqueIndex.
	versionAssetUniqueIndex = 25

	assetNameIndex = "idx_asset_ledger_name_unique"
	assetCodeIndex = "idx_asset_ledger_code_unique"
)

// TestIntegration_Migration000025_AppliesCleanOnDatabaseWithoutDuplicates covers
// the ordinary deploy: both indexes are installed, UNIQUE, keyed on
// organization_id + ledger_id + LOWER(name) and + code, and partial on live rows.
func TestIntegration_Migration000025_AppliesCleanOnDatabaseWithoutDuplicates(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	m := newOnboardingMigrator(t, container.DB)

	require.NoError(t, m.Migrate(versionSegmentUniqueIndex), "failed to migrate to the version before the unique indexes")
	require.Empty(t, indexDefinition(t, container.DB, assetNameIndex), "the name index must not exist before migration 000025")
	require.Empty(t, indexDefinition(t, container.DB, assetCodeIndex), "the code index must not exist before migration 000025")

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := seedLedger(t, container.DB, orgID, "L1")
	require.NoError(t, insertAssetRow(t, container.DB, uuid.New(), orgID, ledgerID, "US Dollar", "USD", nil))

	require.NoError(t, m.Migrate(versionAssetUniqueIndex), "migration 000025 must apply on a database without duplicates")

	nameDefinition := indexDefinition(t, container.DB, assetNameIndex)
	require.NotEmpty(t, nameDefinition, "migration 000025 must create idx_asset_ledger_name_unique")
	assert.Contains(t, nameDefinition, "UNIQUE INDEX")
	assert.Contains(t, nameDefinition, "organization_id")
	assert.Contains(t, nameDefinition, "ledger_id")
	assert.Contains(t, nameDefinition, "lower(name)")
	assert.Contains(t, nameDefinition, "deleted_at IS NULL")

	codeDefinition := indexDefinition(t, container.DB, assetCodeIndex)
	require.NotEmpty(t, codeDefinition, "migration 000025 must create idx_asset_ledger_code_unique")
	assert.Contains(t, codeDefinition, "UNIQUE INDEX")
	assert.Contains(t, codeDefinition, "(organization_id, ledger_id, code)")
	assert.Contains(t, codeDefinition, "deleted_at IS NULL")

	// The guarantee itself: a second live row with the same name (in any case)
	// or the same code in the same ledger is rejected, while a soft-deleted row
	// is not in the way and other ledgers are unaffected.
	err := insertAssetRow(t, container.DB, uuid.New(), orgID, ledgerID, "us dollar", "USX", nil)
	require.Error(t, err, "a case-insensitive name duplicate must be rejected by the index")
	assert.Contains(t, err.Error(), assetNameIndex)

	err = insertAssetRow(t, container.DB, uuid.New(), orgID, ledgerID, "Dólar Americano", "USD", nil)
	require.Error(t, err, "a code duplicate must be rejected by the index")
	assert.Contains(t, err.Error(), assetCodeIndex)

	deletedAt := time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC)
	require.NoError(t, insertAssetRow(t, container.DB, uuid.New(), orgID, ledgerID, "Euro", "EUR", &deletedAt))
	require.NoError(t, insertAssetRow(t, container.DB, uuid.New(), orgID, ledgerID, "Euro", "EUR", nil),
		"a soft-deleted asset must release its name and code")

	otherLedgerID := seedLedger(t, container.DB, orgID, "L2")
	require.NoError(t, insertAssetRow(t, container.DB, uuid.New(), orgID, otherLedgerID, "US Dollar", "USD", nil),
		"uniqueness is scoped to one ledger")
}

// TestIntegration_Migration000025_FailsOnPreExistingDuplicates is the recorded
// decision: the migration aborts instead of repairing data. For each index it
// must leave the rows untouched, name the index in the failure, and apply on
// the next run once the duplicate is cleaned up by hand.
func TestIntegration_Migration000025_FailsOnPreExistingDuplicates(t *testing.T) {
	tests := []struct {
		name          string
		index         string
		keptName      string
		keptCode      string
		duplicateName string
		duplicateCode string
	}{
		{
			name:          "case-insensitive name duplicate",
			index:         assetNameIndex,
			keptName:      "Dup",
			keptCode:      "DPA",
			duplicateName: "dup",
			duplicateCode: "DPB",
		},
		{
			name:          "code duplicate",
			index:         assetCodeIndex,
			keptName:      "Dup A",
			keptCode:      "DUP",
			duplicateName: "Dup B",
			duplicateCode: "DUP",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			container := pgtestutil.SetupContainer(t)

			m := newOnboardingMigrator(t, container.DB)

			require.NoError(t, m.Migrate(versionSegmentUniqueIndex), "failed to migrate to the version before the unique indexes")

			orgID := pgtestutil.CreateTestOrganization(t, container.DB)
			ledgerID := seedLedger(t, container.DB, orgID, "L1")
			duplicateID := uuid.New()

			require.NoError(t, insertAssetRow(t, container.DB, uuid.New(), orgID, ledgerID, tc.keptName, tc.keptCode, nil))
			require.NoError(t, insertAssetRow(t, container.DB, duplicateID, orgID, ledgerID, tc.duplicateName, tc.duplicateCode, nil),
				"the pre-existing duplicate must be insertable before the index exists")

			err := m.Migrate(versionAssetUniqueIndex)
			require.Error(t, err, "migration 000025 must fail on pre-existing duplicates")
			assert.Contains(t, err.Error(), tc.index,
				"the failure must name the index so an operator knows what to clean up")

			assert.Empty(t, indexDefinition(t, container.DB, assetNameIndex), "a failed migration must leave no name index behind")
			assert.Empty(t, indexDefinition(t, container.DB, assetCodeIndex), "a failed migration must leave no code index behind")

			var rowCount int
			require.NoError(t, container.DB.QueryRow(
				`SELECT COUNT(*) FROM asset WHERE ledger_id = $1`, ledgerID,
			).Scan(&rowCount))
			assert.Equal(t, 2, rowCount, "the migration must not alter data when it aborts")

			// Runbook: clean the duplicate by hand, clear the dirty marker left
			// by the aborted run, and migrate again.
			_, execErr := container.DB.Exec(`DELETE FROM asset WHERE id = $1`, duplicateID)
			require.NoError(t, execErr, "failed to remove the seeded duplicate")

			require.NoError(t, m.Force(versionSegmentUniqueIndex), "failed to clear the dirty migration marker")
			require.NoError(t, m.Migrate(versionAssetUniqueIndex), "migration 000025 must apply once duplicates are cleaned up")
			assert.NotEmpty(t, indexDefinition(t, container.DB, assetNameIndex))
			assert.NotEmpty(t, indexDefinition(t, container.DB, assetCodeIndex))
		})
	}
}

// TestIntegration_Migration000025_DownRemovesIndexes verifies the software
// rollback path: both indexes disappear and the previous behavior returns.
func TestIntegration_Migration000025_DownRemovesIndexes(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	m := newOnboardingMigrator(t, container.DB)

	require.NoError(t, m.Migrate(versionAssetUniqueIndex), "failed to migrate to the unique index version")
	require.NotEmpty(t, indexDefinition(t, container.DB, assetNameIndex))
	require.NotEmpty(t, indexDefinition(t, container.DB, assetCodeIndex))

	require.NoError(t, m.Migrate(versionSegmentUniqueIndex), "the down migration must roll the indexes back")
	assert.Empty(t, indexDefinition(t, container.DB, assetNameIndex), "the down migration must drop idx_asset_ledger_name_unique")
	assert.Empty(t, indexDefinition(t, container.DB, assetCodeIndex), "the down migration must drop idx_asset_ledger_code_unique")

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := seedLedger(t, container.DB, orgID, "L1")
	require.NoError(t, insertAssetRow(t, container.DB, uuid.New(), orgID, ledgerID, "US Dollar", "USD", nil))
	require.NoError(t, insertAssetRow(t, container.DB, uuid.New(), orgID, ledgerID, "us dollar", "USD", nil),
		"without the indexes the database accepts duplicates again")
}
