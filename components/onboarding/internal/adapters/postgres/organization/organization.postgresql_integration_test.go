//go:build integration

package organization

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

// createRepository creates an OrganizationRepository connected to the test database.
func createRepository(t *testing.T, container *pgtestutil.ContainerResult) *OrganizationPostgreSQLRepository {
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

	return NewOrganizationPostgreSQLRepository(conn)
}

// ============================================================================
// ListByIDs Tests
// ============================================================================

func TestIntegration_OrganizationRepository_ListByIDs_ReturnsMatchingOrganizations(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)

	// Create 3 organizations
	params1 := pgtestutil.DefaultOrganizationParams()
	params1.LegalName = "Org Alpha"
	params1.LegalDocument = "11111111111111"
	id1 := pgtestutil.CreateTestOrganizationWithParams(t, container.DB, params1)

	params2 := pgtestutil.DefaultOrganizationParams()
	params2.LegalName = "Org Beta"
	params2.LegalDocument = "22222222222222"
	id2 := pgtestutil.CreateTestOrganizationWithParams(t, container.DB, params2)

	params3 := pgtestutil.DefaultOrganizationParams()
	params3.LegalName = "Org Gamma"
	params3.LegalDocument = "33333333333333"
	pgtestutil.CreateTestOrganizationWithParams(t, container.DB, params3)

	ctx := context.Background()

	// Act - Request only 2 of 3
	orgs, err := repo.ListByIDs(ctx, []uuid.UUID{id1, id2})

	// Assert
	require.NoError(t, err, "ListByIDs should not return error")
	assert.Len(t, orgs, 2, "should return exactly 2 organizations")

	ids := make(map[string]bool)
	for _, o := range orgs {
		ids[o.ID] = true
	}
	assert.True(t, ids[id1.String()])
	assert.True(t, ids[id2.String()])
}

func TestIntegration_OrganizationRepository_ListByIDs_ExcludesSoftDeleted(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)

	// Create 1 active + 1 deleted
	activeParams := pgtestutil.DefaultOrganizationParams()
	activeParams.LegalName = "Active Org"
	activeParams.LegalDocument = "11111111111111"
	activeID := pgtestutil.CreateTestOrganizationWithParams(t, container.DB, activeParams)

	deletedAt := time.Now()
	deletedParams := pgtestutil.DefaultOrganizationParams()
	deletedParams.LegalName = "Deleted Org"
	deletedParams.LegalDocument = "22222222222222"
	deletedParams.DeletedAt = &deletedAt
	deletedID := pgtestutil.CreateTestOrganizationWithParams(t, container.DB, deletedParams)

	ctx := context.Background()

	// Act
	orgs, err := repo.ListByIDs(ctx, []uuid.UUID{activeID, deletedID})

	// Assert
	require.NoError(t, err)
	assert.Len(t, orgs, 1, "should only return active organization")
	assert.Equal(t, activeID.String(), orgs[0].ID)
}

func TestIntegration_OrganizationRepository_ListByIDs_EdgeCases(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)

	// Pre-create one organization for partial match test case
	params := pgtestutil.DefaultOrganizationParams()
	params.LegalName = "Existing Org"
	params.LegalDocument = "11111111111111"
	existingID := pgtestutil.CreateTestOrganizationWithParams(t, container.DB, params)

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
			orgs, err := repo.ListByIDs(context.Background(), tc.inputIDs)

			require.NoError(t, err)
			assert.Len(t, orgs, tc.expectedLen)

			if tc.expectedIDs != nil {
				for _, expectedID := range tc.expectedIDs {
					found := false
					for _, org := range orgs {
						if org.ID == expectedID.String() {
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
