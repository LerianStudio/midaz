//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package ledger

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
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

// ============================================================================
// FindAll (Pagination) Tests
// ============================================================================

// defaultPagination returns a Pagination with valid date range for tests.
func defaultPagination(page, limit int) http.Pagination {
	return http.Pagination{
		Page:      page,
		Limit:     limit,
		StartDate: time.Now().AddDate(-1, 0, 0), // 1 year ago
		EndDate:   time.Now().AddDate(0, 0, 1),  // tomorrow
	}
}

func TestIntegration_LedgerRepository_FindAll_Pagination(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	// Create 5 ledgers
	for i := 0; i < 5; i++ {
		params := pgtestutil.DefaultLedgerParams()
		params.Name = "Ledger-" + string(rune('A'+i))
		pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params)
	}

	cases := []struct {
		name        string
		filter      http.Pagination
		expectCount int
	}{
		{
			name:        "page 1 limit 2",
			filter:      defaultPagination(1, 2),
			expectCount: 2,
		},
		{
			name:        "page 2 limit 2",
			filter:      defaultPagination(2, 2),
			expectCount: 2,
		},
		{
			name:        "page 3 limit 2 (last page with 1 item)",
			filter:      defaultPagination(3, 2),
			expectCount: 1,
		},
		{
			name:        "page beyond data returns empty",
			filter:      defaultPagination(10, 2),
			expectCount: 0,
		},
		{
			name:        "limit larger than total returns all",
			filter:      defaultPagination(1, 100),
			expectCount: 5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ledgers, err := repo.FindAll(ctx, orgID, tc.filter, nil)

			require.NoError(t, err)
			assert.Len(t, ledgers, tc.expectCount)
		})
	}
}

func TestIntegration_LedgerRepository_FindAll_ExcludesSoftDeleted(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	// Create 2 active + 1 deleted
	for i := 0; i < 2; i++ {
		params := pgtestutil.DefaultLedgerParams()
		params.Name = "Active-" + string(rune('A'+i))
		pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params)
	}

	deletedAt := time.Now()
	deletedParams := pgtestutil.DefaultLedgerParams()
	deletedParams.Name = "Deleted Ledger"
	deletedParams.DeletedAt = &deletedAt
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, deletedParams)

	ledgers, err := repo.FindAll(ctx, orgID, defaultPagination(1, 10), nil)

	require.NoError(t, err)
	assert.Len(t, ledgers, 2, "should only return active ledgers")

	for _, l := range ledgers {
		assert.NotEqual(t, "Deleted Ledger", l.Name)
	}
}

func TestIntegration_LedgerRepository_FindAll_IsolatesByOrganization(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	org1ID := pgtestutil.CreateTestOrganization(t, container.DB)
	org2ID := pgtestutil.CreateTestOrganization(t, container.DB)

	// Create 2 ledgers in org1, 1 in org2
	for i := 0; i < 2; i++ {
		params := pgtestutil.DefaultLedgerParams()
		params.Name = "Org1-Ledger-" + string(rune('A'+i))
		pgtestutil.CreateTestLedgerWithParams(t, container.DB, org1ID, params)
	}

	params := pgtestutil.DefaultLedgerParams()
	params.Name = "Org2-Ledger"
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, org2ID, params)

	// FindAll for org1 should only return org1's ledgers
	ledgers, err := repo.FindAll(ctx, org1ID, defaultPagination(1, 10), nil)

	require.NoError(t, err)
	assert.Len(t, ledgers, 2, "should only return org1's ledgers")

	// FindAll for org2 should only return org2's ledger
	ledgers, err = repo.FindAll(ctx, org2ID, defaultPagination(1, 10), nil)

	require.NoError(t, err)
	assert.Len(t, ledgers, 1, "should only return org2's ledgers")
}

// ============================================================================
// FindAll Name Filter Tests
// ============================================================================

func TestIntegration_LedgerRepository_FindAll_FilterByName(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	// Create ledgers with distinct names
	params1 := pgtestutil.DefaultLedgerParams()
	params1.Name = "Primary Ledger"
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params1)

	params2 := pgtestutil.DefaultLedgerParams()
	params2.Name = "Primary Backup"
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params2)

	params3 := pgtestutil.DefaultLedgerParams()
	params3.Name = "Secondary Ledger"
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params3)

	// Filter by "Primary" prefix - should return 2
	name := "Primary"
	ledgers, err := repo.FindAll(ctx, orgID, defaultPagination(1, 10), &name)

	require.NoError(t, err)
	assert.Len(t, ledgers, 2, "should return ledgers matching 'Primary' prefix")

	for _, l := range ledgers {
		assert.Contains(t, l.Name, "Primary")
	}
}

func TestIntegration_LedgerRepository_FindAll_FilterByName_CaseInsensitive(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	params := pgtestutil.DefaultLedgerParams()
	params.Name = "Primary Ledger"
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params)

	// Search with lowercase
	name := "primary"
	ledgers, err := repo.FindAll(ctx, orgID, defaultPagination(1, 10), &name)

	require.NoError(t, err)
	assert.Len(t, ledgers, 1, "ILIKE should match case-insensitively")
	assert.Equal(t, "Primary Ledger", ledgers[0].Name)
}

func TestIntegration_LedgerRepository_FindAll_NilNameReturnsAll(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	// Create 3 ledgers
	for i := 0; i < 3; i++ {
		params := pgtestutil.DefaultLedgerParams()
		params.Name = "NilFilter-" + string(rune('A'+i))
		pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params)
	}

	// Nil name should return all
	ledgers, err := repo.FindAll(ctx, orgID, defaultPagination(1, 10), nil)

	require.NoError(t, err)
	assert.Len(t, ledgers, 3, "nil name filter should return all ledgers")
}

func TestIntegration_LedgerRepository_FindAll_PrefixMatchOnly(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	params := pgtestutil.DefaultLedgerParams()
	params.Name = "MyPrimaryLedger"
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params)

	// "Primary" should NOT match "MyPrimaryLedger" because we use prefix match (term%)
	name := "Primary"
	ledgers, err := repo.FindAll(ctx, orgID, defaultPagination(1, 10), &name)

	require.NoError(t, err)
	assert.Len(t, ledgers, 0, "prefix match should not find 'Primary' inside 'MyPrimaryLedger'")
}

// ============================================================================
// FindAll Wildcard Injection Tests
// ============================================================================

func TestIntegration_LedgerRepository_FindAll_WildcardInjection(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	// Create ledgers to verify wildcards don't match
	params1 := pgtestutil.DefaultLedgerParams()
	params1.Name = "Primary Ledger"
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params1)

	params2 := pgtestutil.DefaultLedgerParams()
	params2.Name = "Secondary Ledger"
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params2)

	cases := []struct {
		name      string
		filter    string
		expectLen int
		reason    string
	}{
		{
			name:      "percent wildcard should not match all",
			filter:    "%",
			expectLen: 0,
			reason:    "'%' should be escaped and treated as literal, not SQL wildcard",
		},
		{
			name:      "underscore wildcard should not match single char",
			filter:    "Primar_",
			expectLen: 0,
			reason:    "'_' should be escaped and treated as literal, not SQL single-char wildcard",
		},
		{
			name:      "backslash should not cause escape issues",
			filter:    `Primary\`,
			expectLen: 0,
			reason:    "backslash should be escaped and treated as literal",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filter := tc.filter
			ledgers, err := repo.FindAll(ctx, orgID, defaultPagination(1, 10), &filter)

			require.NoError(t, err)
			assert.Len(t, ledgers, tc.expectLen, tc.reason)
		})
	}
}

func TestIntegration_LedgerRepository_FindAll_LiteralSpecialCharsInName(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	// Create ledger with literal % in name
	params := pgtestutil.DefaultLedgerParams()
	params.Name = "100% Returns Ledger"
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params)

	// Searching for "100%" should find it (literal match)
	name := "100%"
	ledgers, err := repo.FindAll(ctx, orgID, defaultPagination(1, 10), &name)

	require.NoError(t, err)
	assert.Len(t, ledgers, 1, "should find ledger with literal '%' in name")
	assert.Equal(t, "100% Returns Ledger", ledgers[0].Name)
}

// ============================================================================
// ListByIDs Tests (continued)
// ============================================================================

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

// ============================================================================
// UpdateSettings Edge Cases - Nested JSON Behavior Tests
// These tests verify actual PostgreSQL || operator behavior with nested JSON
// ============================================================================

func TestIntegration_LedgerRepository_UpdateSettings_NestedObjectReplacement(t *testing.T) {
	// This test verifies that PostgreSQL || performs SHALLOW merge.
	// Nested objects are REPLACED entirely, not deep-merged.

	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Set initial settings with nested structure
	initialSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
			"validateRoutes":      true,
			"maxRetries":          3,
		},
		"reporting": map[string]any{
			"enabled": true,
		},
	}
	pgtestutil.SetLedgerSettings(t, container.DB, ledgerID, initialSettings)

	// Update with partial nested object - only validateAccountType
	newSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": false,
		},
	}

	ctx := context.Background()

	// Act
	result, err := repo.UpdateSettings(ctx, orgID, ledgerID, newSettings)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	// CRITICAL: Verify shallow merge behavior
	// The "accounting" object should be REPLACED entirely, losing validateRoutes and maxRetries
	accounting, ok := result["accounting"].(map[string]any)
	require.True(t, ok, "accounting should be a map")

	assert.Equal(t, false, accounting["validateAccountType"], "validateAccountType should be updated")
	assert.NotContains(t, accounting, "validateRoutes", "validateRoutes should be LOST (shallow merge)")
	assert.NotContains(t, accounting, "maxRetries", "maxRetries should be LOST (shallow merge)")

	// Top-level keys not in update should be preserved
	reporting, ok := result["reporting"].(map[string]any)
	require.True(t, ok, "reporting should be preserved")
	assert.Equal(t, true, reporting["enabled"], "reporting.enabled should be preserved")
}

func TestIntegration_LedgerRepository_UpdateSettings_DeeplyNestedThreeLevels(t *testing.T) {
	// Test behavior with 3+ levels of nesting

	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Set initial settings with 3 levels of nesting
	initialSettings := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": map[string]any{
					"deep":  "original",
					"keep":  "this",
					"other": "value",
				},
			},
			"siblingKey": "preserve",
		},
	}
	pgtestutil.SetLedgerSettings(t, container.DB, ledgerID, initialSettings)

	// Update deeply nested value
	newSettings := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": map[string]any{
					"deep": "updated",
				},
			},
		},
	}

	ctx := context.Background()

	// Act
	result, err := repo.UpdateSettings(ctx, orgID, ledgerID, newSettings)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	// Navigate to verify
	level1, ok := result["level1"].(map[string]any)
	require.True(t, ok, "level1 should be a map")

	// siblingKey should be LOST (shallow merge replaces level1)
	assert.NotContains(t, level1, "siblingKey", "siblingKey should be LOST (shallow merge at top level)")

	level2, ok := level1["level2"].(map[string]any)
	require.True(t, ok, "level2 should be a map")

	level3, ok := level2["level3"].(map[string]any)
	require.True(t, ok, "level3 should be a map")

	assert.Equal(t, "updated", level3["deep"], "deep should be updated")
	assert.NotContains(t, level3, "keep", "keep should be LOST (nested replacement)")
	assert.NotContains(t, level3, "other", "other should be LOST (nested replacement)")
}

func TestIntegration_LedgerRepository_UpdateSettings_TypeMismatchObjectToScalar(t *testing.T) {
	// Test what happens when a nested object is replaced with a scalar

	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Set initial settings with nested object
	initialSettings := map[string]any{
		"config": map[string]any{
			"nested": true,
			"value":  123,
		},
	}
	pgtestutil.SetLedgerSettings(t, container.DB, ledgerID, initialSettings)

	// Replace object with scalar
	newSettings := map[string]any{
		"config": "now_a_string",
	}

	ctx := context.Background()

	// Act
	result, err := repo.UpdateSettings(ctx, orgID, ledgerID, newSettings)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	// config should now be a string, not a map
	configValue, ok := result["config"].(string)
	require.True(t, ok, "config should now be a string")
	assert.Equal(t, "now_a_string", configValue)
}

func TestIntegration_LedgerRepository_UpdateSettings_TypeMismatchScalarToObject(t *testing.T) {
	// Test what happens when a scalar is replaced with a nested object

	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Set initial settings with scalar
	initialSettings := map[string]any{
		"config": "simple_string",
	}
	pgtestutil.SetLedgerSettings(t, container.DB, ledgerID, initialSettings)

	// Replace scalar with object
	newSettings := map[string]any{
		"config": map[string]any{
			"now":    "nested",
			"object": true,
		},
	}

	ctx := context.Background()

	// Act
	result, err := repo.UpdateSettings(ctx, orgID, ledgerID, newSettings)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	// config should now be an object
	configMap, ok := result["config"].(map[string]any)
	require.True(t, ok, "config should now be a map")
	assert.Equal(t, "nested", configMap["now"])
	assert.Equal(t, true, configMap["object"])
}

func TestIntegration_LedgerRepository_UpdateSettings_SpecialCharactersInKeys(t *testing.T) {
	// Test that special characters in JSON keys are handled correctly

	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Settings with special characters in keys
	newSettings := map[string]any{
		"key.with.dots":     "value1",
		"key-with-dashes":   "value2",
		"key_with_under":    "value3",
		"key with spaces":   "value4",
		"key:with:colons":   "value5",
		"unicode_キー":        "value6",
		"emoji_🔑":           "value7",
		"123_numeric_start": "value8",
	}

	ctx := context.Background()

	// Act
	result, err := repo.UpdateSettings(ctx, orgID, ledgerID, newSettings)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify all special keys are stored and retrieved correctly
	assert.Equal(t, "value1", result["key.with.dots"])
	assert.Equal(t, "value2", result["key-with-dashes"])
	assert.Equal(t, "value3", result["key_with_under"])
	assert.Equal(t, "value4", result["key with spaces"])
	assert.Equal(t, "value5", result["key:with:colons"])
	assert.Equal(t, "value6", result["unicode_キー"])
	assert.Equal(t, "value7", result["emoji_🔑"])
	assert.Equal(t, "value8", result["123_numeric_start"])
}

func TestIntegration_LedgerRepository_UpdateSettings_EmptyNestedObject(t *testing.T) {
	// Test that empty nested objects are handled correctly

	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Set initial settings
	initialSettings := map[string]any{
		"config": map[string]any{
			"hasValues": true,
		},
	}
	pgtestutil.SetLedgerSettings(t, container.DB, ledgerID, initialSettings)

	// Update with empty nested object
	newSettings := map[string]any{
		"config":    map[string]any{}, // Empty object
		"newConfig": map[string]any{}, // New empty object
	}

	ctx := context.Background()

	// Act
	result, err := repo.UpdateSettings(ctx, orgID, ledgerID, newSettings)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	// config should be replaced with empty object
	configMap, ok := result["config"].(map[string]any)
	require.True(t, ok, "config should be a map")
	assert.Empty(t, configMap, "config should be empty (replaced with empty object)")

	// newConfig should exist as empty object
	newConfigMap, ok := result["newConfig"].(map[string]any)
	require.True(t, ok, "newConfig should be a map")
	assert.Empty(t, newConfigMap, "newConfig should be empty")
}

func TestIntegration_LedgerRepository_UpdateSettings_NullValueSetsNotRemoves(t *testing.T) {
	// Test that setting a key to null sets the value to JSON null, not removes the key

	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Set initial settings
	initialSettings := map[string]any{
		"keyToNull": "originalValue",
		"keyToKeep": "keepThis",
		"nestedNull": map[string]any{
			"inner": "value",
		},
	}
	pgtestutil.SetLedgerSettings(t, container.DB, ledgerID, initialSettings)

	// Set keys to null
	newSettings := map[string]any{
		"keyToNull":  nil,
		"nestedNull": nil,
	}

	ctx := context.Background()

	// Act
	result, err := repo.UpdateSettings(ctx, orgID, ledgerID, newSettings)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	// Keys should EXIST with null value, not be removed
	assert.Contains(t, result, "keyToNull", "keyToNull should exist in result")
	assert.Nil(t, result["keyToNull"], "keyToNull should be nil/null")

	assert.Contains(t, result, "nestedNull", "nestedNull should exist in result")
	assert.Nil(t, result["nestedNull"], "nestedNull should be nil/null")

	// keyToKeep should still exist
	assert.Equal(t, "keepThis", result["keyToKeep"], "keyToKeep should be preserved")
}

func TestIntegration_LedgerRepository_UpdateSettings_ArrayValues(t *testing.T) {
	// Test that array values are handled correctly

	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Settings with array values
	newSettings := map[string]any{
		"simpleArray":  []any{"a", "b", "c"},
		"numberArray":  []any{1, 2, 3},
		"mixedArray":   []any{"string", 123, true, nil},
		"nestedArrays": []any{[]any{1, 2}, []any{3, 4}},
		"arrayOfObjects": []any{
			map[string]any{"name": "first"},
			map[string]any{"name": "second"},
		},
	}

	ctx := context.Background()

	// Act
	result, err := repo.UpdateSettings(ctx, orgID, ledgerID, newSettings)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify arrays are stored and retrieved correctly
	simpleArray, ok := result["simpleArray"].([]any)
	require.True(t, ok, "simpleArray should be an array")
	assert.Len(t, simpleArray, 3)
	assert.Equal(t, "a", simpleArray[0])

	arrayOfObjects, ok := result["arrayOfObjects"].([]any)
	require.True(t, ok, "arrayOfObjects should be an array")
	assert.Len(t, arrayOfObjects, 2)

	firstObj, ok := arrayOfObjects[0].(map[string]any)
	require.True(t, ok, "first element should be a map")
	assert.Equal(t, "first", firstObj["name"])
}

func TestIntegration_LedgerRepository_UpdateSettings_LargeNestedStructure(t *testing.T) {
	// Test with a moderately large nested structure to verify performance/correctness

	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Build a structure with many keys at multiple levels
	newSettings := make(map[string]any)
	for i := 0; i < 50; i++ {
		key := "key_" + string(rune('a'+i%26)) + "_" + string(rune('0'+i/26))
		newSettings[key] = map[string]any{
			"value":  i,
			"nested": map[string]any{"deep": true},
		}
	}

	ctx := context.Background()

	// Act
	result, err := repo.UpdateSettings(ctx, orgID, ledgerID, newSettings)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result, 50, "should have 50 top-level keys")

	// Verify one of the nested structures
	key0, ok := result["key_a_0"].(map[string]any)
	require.True(t, ok, "key_a_0 should be a map")
	assert.Equal(t, float64(0), key0["value"]) // JSON numbers come back as float64
}
