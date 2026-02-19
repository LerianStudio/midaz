//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package ledger

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createRepository creates a LedgerRepository connected to the test database.
func createRepository(t *testing.T, container *pgtestutil.ContainerResult) *LedgerPostgreSQLRepository {
	t.Helper()

	logger := libZap.InitializeLogger()
	migrationsPath := pgtestutil.FindMigrationsPath(t, "onboarding")

	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)

	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           container.Config.DBName,
		ReplicaDBName:           container.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	return NewLedgerPostgreSQLRepository(conn)
}

// ============================================================================
// ListByIDs Tests
// ============================================================================

func TestIntegration_LedgerRepository_ListByIDs_ReturnsMatchingLedgers(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	// Create 3 ledgers
	params1 := pgtestutil.DefaultLedgerParams()
	params1.Name = "Ledger Alpha"
	id1 := pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params1)

	params2 := pgtestutil.DefaultLedgerParams()
	params2.Name = "Ledger Beta"
	id2 := pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params2)

	params3 := pgtestutil.DefaultLedgerParams()
	params3.Name = "Ledger Gamma"
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params3)

	ctx := context.Background()

	// Act - Request only 2 of 3
	ledgers, err := repo.ListByIDs(ctx, orgID, []uuid.UUID{id1, id2})

	// Assert
	require.NoError(t, err, "ListByIDs should not return error")
	assert.Len(t, ledgers, 2, "should return exactly 2 ledgers")

	ids := make(map[string]bool)
	for _, l := range ledgers {
		ids[l.ID] = true
	}
	assert.True(t, ids[id1.String()])
	assert.True(t, ids[id2.String()])
}

func TestIntegration_LedgerRepository_ListByIDs_ExcludesSoftDeleted(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	// Create 1 active + 1 deleted
	activeParams := pgtestutil.DefaultLedgerParams()
	activeParams.Name = "Active Ledger"
	activeID := pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, activeParams)

	deletedAt := time.Now()
	deletedParams := pgtestutil.DefaultLedgerParams()
	deletedParams.Name = "Deleted Ledger"
	deletedParams.DeletedAt = &deletedAt
	deletedID := pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, deletedParams)

	ctx := context.Background()

	// Act
	ledgers, err := repo.ListByIDs(ctx, orgID, []uuid.UUID{activeID, deletedID})

	// Assert
	require.NoError(t, err)
	assert.Len(t, ledgers, 1, "should only return active ledger")
	assert.Equal(t, activeID.String(), ledgers[0].ID)
}

func TestIntegration_LedgerRepository_ListByIDs_IsolatesByOrganization(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	// Create 2 organizations
	org1ID := pgtestutil.CreateTestOrganization(t, container.DB)
	org2ID := pgtestutil.CreateTestOrganization(t, container.DB)

	// Create ledger in org1
	params := pgtestutil.DefaultLedgerParams()
	params.Name = "Org1 Ledger"
	ledgerID := pgtestutil.CreateTestLedgerWithParams(t, container.DB, org1ID, params)

	ctx := context.Background()

	// Act - Try to find ledger from org1 using org2's context
	ledgers, err := repo.ListByIDs(ctx, org2ID, []uuid.UUID{ledgerID})

	// Assert - Should not find it (wrong org)
	require.NoError(t, err)
	assert.Empty(t, ledgers, "should not find ledger from different organization")

	// Act - Find with correct org
	ledgers, err = repo.ListByIDs(ctx, org1ID, []uuid.UUID{ledgerID})

	// Assert - Should find it
	require.NoError(t, err)
	assert.Len(t, ledgers, 1, "should find ledger with correct organization")
}

func TestIntegration_LedgerRepository_ListByIDs_EdgeCases(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	// Pre-create one ledger for partial match test case
	params := pgtestutil.DefaultLedgerParams()
	params.Name = "Existing Ledger"
	existingID := pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params)

	cases := []struct {
		name        string
		inputIDs    []uuid.UUID
		expectedLen int
		expectedIDs []uuid.UUID
	}{
		{
			name:        "empty input returns empty",
			inputIDs:    []uuid.UUID{},
			expectedLen: 0,
			expectedIDs: nil,
		},
		{
			name:        "non-matching ID returns empty",
			inputIDs:    []uuid.UUID{libCommons.GenerateUUIDv7()},
			expectedLen: 0,
			expectedIDs: nil,
		},
		{
			name:        "partial match returns only existing",
			inputIDs:    []uuid.UUID{existingID, libCommons.GenerateUUIDv7()},
			expectedLen: 1,
			expectedIDs: []uuid.UUID{existingID},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ledgers, err := repo.ListByIDs(context.Background(), orgID, tc.inputIDs)

			require.NoError(t, err)
			assert.Len(t, ledgers, tc.expectedLen)

			if tc.expectedIDs != nil {
				for _, expectedID := range tc.expectedIDs {
					found := false
					for _, ledger := range ledgers {
						if ledger.ID == expectedID.String() {
							found = true
							break
						}
					}
					assert.True(t, found, "expected ID %s not found", expectedID)
				}
			}
		})
	}
}

// ============================================================================
// GetSettings Tests
// ============================================================================

func TestIntegration_LedgerRepository_GetSettings_ReturnsExistingSettings(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	expectedSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}
	pgtestutil.SetLedgerSettings(t, container.DB, ledgerID, expectedSettings)

	ctx := context.Background()

	// Act
	settings, err := repo.GetSettings(ctx, orgID, ledgerID)

	// Assert
	require.NoError(t, err, "GetSettings should not return error")
	assert.NotNil(t, settings)
	assert.NotEmpty(t, settings)

	accounting, ok := settings["accounting"].(map[string]any)
	require.True(t, ok, "accounting should be a map")
	assert.Equal(t, true, accounting["validateAccountType"])
}

func TestIntegration_LedgerRepository_GetSettings_ReturnsEmptyMapWhenNoSettings(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	ctx := context.Background()

	// Act
	settings, err := repo.GetSettings(ctx, orgID, ledgerID)

	// Assert
	require.NoError(t, err, "GetSettings should not return error")
	assert.NotNil(t, settings, "settings should not be nil")
	assert.Empty(t, settings, "settings should be empty map")
}

func TestIntegration_LedgerRepository_GetSettings_ReturnsErrorWhenLedgerNotFound(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	nonExistentLedgerID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	settings, err := repo.GetSettings(ctx, orgID, nonExistentLedgerID)

	// Assert
	require.Error(t, err, "GetSettings should return error when ledger not found")
	assert.Nil(t, settings)
}

func TestIntegration_LedgerRepository_GetSettings_IsolatesByOrganization(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	org1ID := pgtestutil.CreateTestOrganization(t, container.DB)
	org2ID := pgtestutil.CreateTestOrganization(t, container.DB)

	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, org1ID)

	expectedSettings := map[string]any{"key": "value"}
	pgtestutil.SetLedgerSettings(t, container.DB, ledgerID, expectedSettings)

	ctx := context.Background()

	// Act - Try to get settings with wrong org
	settings, err := repo.GetSettings(ctx, org2ID, ledgerID)

	// Assert - Should not find it
	require.Error(t, err, "GetSettings should fail when using wrong organization")
	assert.Nil(t, settings)

	// Act - Get with correct org
	settings, err = repo.GetSettings(ctx, org1ID, ledgerID)

	// Assert - Should find it
	require.NoError(t, err)
	assert.Equal(t, "value", settings["key"])
}

func TestIntegration_LedgerRepository_GetSettings_ExcludesSoftDeleted(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	deletedAt := time.Now()
	params := pgtestutil.DefaultLedgerParams()
	params.DeletedAt = &deletedAt
	ledgerID := pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params)

	pgtestutil.SetLedgerSettings(t, container.DB, ledgerID, map[string]any{"key": "value"})

	ctx := context.Background()

	// Act
	settings, err := repo.GetSettings(ctx, orgID, ledgerID)

	// Assert - Should not find deleted ledger
	require.Error(t, err, "GetSettings should fail for soft-deleted ledger")
	assert.Nil(t, settings)
}

// ============================================================================
// UpdateSettings Tests
// ============================================================================

func TestIntegration_LedgerRepository_UpdateSettings_CreatesNewSettings(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	newSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}

	ctx := context.Background()

	// Act
	result, err := repo.UpdateSettings(ctx, orgID, ledgerID, newSettings)

	// Assert
	require.NoError(t, err, "UpdateSettings should not return error")
	assert.NotNil(t, result)

	accounting, ok := result["accounting"].(map[string]any)
	require.True(t, ok, "accounting should be a map")
	assert.Equal(t, true, accounting["validateAccountType"])

	// Verify directly in DB
	dbSettings := pgtestutil.GetLedgerSettings(t, container.DB, ledgerID)
	assert.NotEmpty(t, dbSettings)
}

func TestIntegration_LedgerRepository_UpdateSettings_MergesWithExisting(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Set initial settings
	initialSettings := map[string]any{
		"existingKey": "existingValue",
		"toOverwrite": "oldValue",
	}
	pgtestutil.SetLedgerSettings(t, container.DB, ledgerID, initialSettings)

	// New settings to merge
	newSettings := map[string]any{
		"newKey":      "newValue",
		"toOverwrite": "newValue",
	}

	ctx := context.Background()

	// Act
	result, err := repo.UpdateSettings(ctx, orgID, ledgerID, newSettings)

	// Assert
	require.NoError(t, err, "UpdateSettings should not return error")
	assert.NotNil(t, result)
	assert.Equal(t, "existingValue", result["existingKey"], "existing key should be preserved")
	assert.Equal(t, "newValue", result["newKey"], "new key should be added")
	assert.Equal(t, "newValue", result["toOverwrite"], "existing key should be overwritten")
}

func TestIntegration_LedgerRepository_UpdateSettings_ReturnsErrorWhenLedgerNotFound(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	nonExistentLedgerID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	result, err := repo.UpdateSettings(ctx, orgID, nonExistentLedgerID, map[string]any{"key": "value"})

	// Assert
	require.Error(t, err, "UpdateSettings should return error when ledger not found")
	assert.Nil(t, result)
}

func TestIntegration_LedgerRepository_UpdateSettings_IsolatesByOrganization(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	org1ID := pgtestutil.CreateTestOrganization(t, container.DB)
	org2ID := pgtestutil.CreateTestOrganization(t, container.DB)

	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, org1ID)

	ctx := context.Background()

	// Act - Try to update with wrong org
	result, err := repo.UpdateSettings(ctx, org2ID, ledgerID, map[string]any{"key": "value"})

	// Assert - Should fail
	require.Error(t, err, "UpdateSettings should fail when using wrong organization")
	assert.Nil(t, result)

	// Act - Update with correct org
	result, err = repo.UpdateSettings(ctx, org1ID, ledgerID, map[string]any{"key": "value"})

	// Assert - Should succeed
	require.NoError(t, err)
	assert.Equal(t, "value", result["key"])
}

func TestIntegration_LedgerRepository_UpdateSettings_ExcludesSoftDeleted(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	deletedAt := time.Now()
	params := pgtestutil.DefaultLedgerParams()
	params.DeletedAt = &deletedAt
	ledgerID := pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params)

	ctx := context.Background()

	// Act
	result, err := repo.UpdateSettings(ctx, orgID, ledgerID, map[string]any{"key": "value"})

	// Assert - Should fail for soft-deleted ledger
	require.Error(t, err, "UpdateSettings should fail for soft-deleted ledger")
	assert.Nil(t, result)
}
