//go:build integration

package ledger

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	pgtestutil "github.com/LerianStudio/midaz/v3/pkg/testutils/postgres"
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
	defer container.Cleanup()

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
	defer container.Cleanup()

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
	defer container.Cleanup()

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
	defer container.Cleanup()

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
