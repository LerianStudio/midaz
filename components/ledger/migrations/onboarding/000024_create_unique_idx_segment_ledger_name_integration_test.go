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
	// versionBeforeSegmentUniqueIndex is the last onboarding migration that
	// predates the segment name uniqueness guarantee.
	versionBeforeSegmentUniqueIndex = 23
	// versionSegmentUniqueIndex installs idx_segment_ledger_name_unique.
	versionSegmentUniqueIndex = 24

	segmentNameIndex = "idx_segment_ledger_name_unique"
)

// TestIntegration_Migration000024_AppliesCleanOnDatabaseWithoutDuplicates covers
// the ordinary deploy: the index is installed, it is UNIQUE, keyed on
// organization_id + ledger_id + LOWER(name), and partial on live rows.
func TestIntegration_Migration000024_AppliesCleanOnDatabaseWithoutDuplicates(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	m := newOnboardingMigrator(t, container.DB)

	require.NoError(t, m.Migrate(versionBeforeSegmentUniqueIndex), "failed to migrate to the version before the unique index")
	require.Empty(t, indexDefinition(t, container.DB, segmentNameIndex), "the index must not exist before migration 000024")

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := seedLedger(t, container.DB, orgID, "L1")
	require.NoError(t, insertSegmentRow(t, container.DB, uuid.New(), orgID, ledgerID, "Retail", nil))

	require.NoError(t, m.Migrate(versionSegmentUniqueIndex), "migration 000024 must apply on a database without duplicates")

	definition := indexDefinition(t, container.DB, segmentNameIndex)
	require.NotEmpty(t, definition, "migration 000024 must create idx_segment_ledger_name_unique")
	assert.Contains(t, definition, "UNIQUE INDEX")
	assert.Contains(t, definition, "organization_id")
	assert.Contains(t, definition, "ledger_id")
	assert.Contains(t, definition, "lower(name)")
	assert.Contains(t, definition, "deleted_at IS NULL")

	// The guarantee itself: a second live row with the same name in the same
	// ledger is rejected, in any case, while a soft-deleted row is not in the
	// way and other ledgers are unaffected.
	err := insertSegmentRow(t, container.DB, uuid.New(), orgID, ledgerID, "RETAIL", nil)
	require.Error(t, err, "a case-insensitive duplicate must be rejected by the index")
	assert.Contains(t, err.Error(), segmentNameIndex)

	deletedAt := time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC)
	require.NoError(t, insertSegmentRow(t, container.DB, uuid.New(), orgID, ledgerID, "Wholesale", &deletedAt))
	require.NoError(t, insertSegmentRow(t, container.DB, uuid.New(), orgID, ledgerID, "Wholesale", nil),
		"a soft-deleted segment must release its name")

	otherLedgerID := seedLedger(t, container.DB, orgID, "L2")
	require.NoError(t, insertSegmentRow(t, container.DB, uuid.New(), orgID, otherLedgerID, "Retail", nil),
		"uniqueness is scoped to one ledger")
}

// TestIntegration_Migration000024_FailsOnPreExistingDuplicates is the recorded
// decision: the migration aborts instead of repairing data. It must leave both
// rows untouched, name the index in the failure, and apply on the next run once
// the duplicate is cleaned up by hand.
func TestIntegration_Migration000024_FailsOnPreExistingDuplicates(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	m := newOnboardingMigrator(t, container.DB)

	require.NoError(t, m.Migrate(versionBeforeSegmentUniqueIndex), "failed to migrate to the version before the unique index")

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := seedLedger(t, container.DB, orgID, "L1")
	keptID := uuid.New()
	duplicateID := uuid.New()

	require.NoError(t, insertSegmentRow(t, container.DB, keptID, orgID, ledgerID, "Dup", nil))
	require.NoError(t, insertSegmentRow(t, container.DB, duplicateID, orgID, ledgerID, "dup", nil),
		"the pre-existing duplicate must be insertable before the index exists")

	err := m.Migrate(versionSegmentUniqueIndex)
	require.Error(t, err, "migration 000024 must fail on pre-existing duplicates")
	assert.Contains(t, err.Error(), segmentNameIndex,
		"the failure must name the index so an operator knows what to clean up")

	assert.Empty(t, indexDefinition(t, container.DB, segmentNameIndex), "a failed build must leave no index behind")

	var rowCount int
	require.NoError(t, container.DB.QueryRow(
		`SELECT COUNT(*) FROM segment WHERE ledger_id = $1 AND LOWER(name) = 'dup'`, ledgerID,
	).Scan(&rowCount))
	assert.Equal(t, 2, rowCount, "the migration must not alter data when it aborts")

	// Runbook: clean the duplicate by hand, clear the dirty marker left by the
	// aborted run, and migrate again.
	_, execErr := container.DB.Exec(`DELETE FROM segment WHERE id = $1`, duplicateID)
	require.NoError(t, execErr, "failed to remove the seeded duplicate")

	require.NoError(t, m.Force(versionBeforeSegmentUniqueIndex), "failed to clear the dirty migration marker")
	require.NoError(t, m.Migrate(versionSegmentUniqueIndex), "migration 000024 must apply once duplicates are cleaned up")
	assert.NotEmpty(t, indexDefinition(t, container.DB, segmentNameIndex))
}

// TestIntegration_Migration000024_DownRemovesIndex verifies the software
// rollback path: the index disappears and the previous behavior returns.
func TestIntegration_Migration000024_DownRemovesIndex(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	m := newOnboardingMigrator(t, container.DB)

	require.NoError(t, m.Migrate(versionSegmentUniqueIndex), "failed to migrate to the unique index version")
	require.NotEmpty(t, indexDefinition(t, container.DB, segmentNameIndex))

	require.NoError(t, m.Migrate(versionBeforeSegmentUniqueIndex), "the down migration must roll the index back")
	assert.Empty(t, indexDefinition(t, container.DB, segmentNameIndex), "the down migration must drop idx_segment_ledger_name_unique")

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := seedLedger(t, container.DB, orgID, "L1")
	require.NoError(t, insertSegmentRow(t, container.DB, uuid.New(), orgID, ledgerID, "Retail", nil))
	require.NoError(t, insertSegmentRow(t, container.DB, uuid.New(), orgID, ledgerID, "retail", nil),
		"without the index the database accepts duplicates again")
}
