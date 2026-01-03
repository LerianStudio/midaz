//go:build integration

package portfolio

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v3/pkg/testutils/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createRepository creates a PortfolioPostgreSQLRepository connected to the test database.
func createRepository(t *testing.T, container *pgtestutil.ContainerResult) *PortfolioPostgreSQLRepository {
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

	return NewPortfolioPostgreSQLRepository(conn)
}

// ============================================================================
// Find Tests
// ============================================================================

func TestIntegration_PortfolioRepository_Find_ReturnsPortfolio(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.PortfolioParams{
		Name:     "Investment Portfolio",
		EntityID: "entity-abc-123",
		Status:   "ACTIVE",
	}
	portfolioID := pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	portfolio, err := repo.Find(ctx, orgID, ledgerID, portfolioID)

	// Assert
	require.NoError(t, err, "Find should not return error for existing portfolio")
	require.NotNil(t, portfolio, "portfolio should not be nil")

	assert.Equal(t, portfolioID.String(), portfolio.ID, "ID should match")
	assert.Equal(t, orgID.String(), portfolio.OrganizationID, "organization ID should match")
	assert.Equal(t, ledgerID.String(), portfolio.LedgerID, "ledger ID should match")
	assert.Equal(t, "Investment Portfolio", portfolio.Name, "name should match")
	assert.Equal(t, "entity-abc-123", portfolio.EntityID, "entity ID should match")
	assert.Equal(t, "ACTIVE", portfolio.Status.Code, "status should match")
}

func TestIntegration_PortfolioRepository_Find_ReturnsErrNotFound(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	portfolio, err := repo.Find(ctx, orgID, ledgerID, nonExistentID)

	// Assert
	require.Error(t, err, "Find should return error for non-existent portfolio")
	assert.Nil(t, portfolio, "portfolio should be nil")

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
	assert.Equal(t, constant.ErrEntityNotFound.Error(), entityNotFoundErr.Code, "error code should be ErrEntityNotFound")
}

func TestIntegration_PortfolioRepository_Find_IgnoresDeletedPortfolio(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	deletedAt := time.Now().Add(-1 * time.Hour)
	params := pgtestutil.PortfolioParams{
		Name:      "Deleted Portfolio",
		EntityID:  "entity-deleted",
		Status:    "ACTIVE",
		DeletedAt: &deletedAt,
	}
	portfolioID := pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	portfolio, err := repo.Find(ctx, orgID, ledgerID, portfolioID)

	// Assert
	require.Error(t, err, "Find should return error for deleted portfolio")
	assert.Nil(t, portfolio, "deleted portfolio should not be returned")
}

// ============================================================================
// FindByIDEntity Tests
// ============================================================================

func TestIntegration_PortfolioRepository_FindByIDEntity_ReturnsPortfolio(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	entityID := libCommons.GenerateUUIDv7()
	params := pgtestutil.PortfolioParams{
		Name:     "Entity Portfolio",
		EntityID: entityID.String(),
		Status:   "ACTIVE",
	}
	portfolioID := pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	portfolio, err := repo.FindByIDEntity(ctx, orgID, ledgerID, entityID)

	// Assert
	require.NoError(t, err, "FindByIDEntity should not return error for existing entity")
	require.NotNil(t, portfolio, "portfolio should not be nil")

	assert.Equal(t, portfolioID.String(), portfolio.ID, "ID should match")
	assert.Equal(t, entityID.String(), portfolio.EntityID, "entity ID should match")
	assert.Equal(t, "Entity Portfolio", portfolio.Name, "name should match")
}

func TestIntegration_PortfolioRepository_FindByIDEntity_ReturnsErrNotFound(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	nonExistentEntityID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	portfolio, err := repo.FindByIDEntity(ctx, orgID, ledgerID, nonExistentEntityID)

	// Assert
	require.Error(t, err, "FindByIDEntity should return error for non-existent entity")
	assert.Nil(t, portfolio, "portfolio should be nil")

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
}

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_PortfolioRepository_Create_InsertsAndReturnsPortfolio(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	now := time.Now().Truncate(time.Microsecond)

	portfolio := &mmodel.Portfolio{
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Name:           "New Portfolio",
		EntityID:       "entity-new-123",
		Status:         mmodel.Status{Code: "ACTIVE"},
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	ctx := context.Background()

	// Act
	created, err := repo.Create(ctx, portfolio)

	// Assert
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, created)

	// Verify by finding - use the ID from created portfolio since FromEntity generates new UUIDs
	createdID, err := uuid.Parse(created.ID)
	require.NoError(t, err, "created ID should be valid UUID")

	found, err := repo.Find(ctx, orgID, ledgerID, createdID)
	require.NoError(t, err)
	assert.Equal(t, "New Portfolio", found.Name)
	assert.Equal(t, "entity-new-123", found.EntityID)
	assert.Equal(t, "ACTIVE", found.Status.Code)
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_PortfolioRepository_Update_ChangesNameAndStatus(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.PortfolioParams{
		Name:     "Original Name",
		EntityID: "entity-original",
		Status:   "ACTIVE",
	}
	portfolioID := pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Get original to compare updated_at
	original, err := repo.Find(ctx, orgID, ledgerID, portfolioID)
	require.NoError(t, err)
	originalUpdatedAt := original.UpdatedAt

	// Small delay to ensure updated_at differs
	time.Sleep(10 * time.Millisecond)

	// Act
	statusDesc := "Deactivated for review"
	updateData := &mmodel.Portfolio{
		Name:   "Updated Name",
		Status: mmodel.Status{Code: "INACTIVE", Description: &statusDesc},
	}
	updated, err := repo.Update(ctx, orgID, ledgerID, portfolioID, updateData)

	// Assert
	require.NoError(t, err, "Update should not return error")
	require.NotNil(t, updated)

	found, err := repo.Find(ctx, orgID, ledgerID, portfolioID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", found.Name, "name should be updated")
	assert.Equal(t, "INACTIVE", found.Status.Code, "status should be updated")
	assert.Equal(t, "Deactivated for review", *found.Status.Description, "status description should be updated")
	assert.Equal(t, "entity-original", found.EntityID, "entity ID should remain unchanged")
	assert.True(t, found.UpdatedAt.After(originalUpdatedAt), "updated_at should be changed after update")
}

func TestIntegration_PortfolioRepository_Update_ReturnsErrNotFound(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	updateData := &mmodel.Portfolio{
		Name: "Updated Name",
	}
	updated, err := repo.Update(ctx, orgID, ledgerID, nonExistentID, updateData)

	// Assert
	require.Error(t, err, "Update should return error for non-existent portfolio")
	assert.Nil(t, updated)

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
}

// ============================================================================
// FindAll Tests (page/limit pagination)
// ============================================================================

func TestIntegration_PortfolioRepository_FindAll_ReturnsPortfolios(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create multiple portfolios
	params1 := pgtestutil.PortfolioParams{
		Name:     "Portfolio Alpha",
		EntityID: "entity-alpha",
		Status:   "ACTIVE",
	}
	pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, params1)

	params2 := pgtestutil.PortfolioParams{
		Name:     "Portfolio Beta",
		EntityID: "entity-beta",
		Status:   "ACTIVE",
	}
	pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, params2)

	ctx := context.Background()

	// Act
	portfolios, err := repo.FindAll(ctx, orgID, ledgerID, defaultPagination())

	// Assert
	require.NoError(t, err, "FindAll should not return error")
	assert.Len(t, portfolios, 2, "should return 2 portfolios")
}

func TestIntegration_PortfolioRepository_FindAll_Pagination(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create 5 portfolios
	entityIDs := []string{"entity-1", "entity-2", "entity-3", "entity-4", "entity-5"}
	for i, entityID := range entityIDs {
		params := pgtestutil.PortfolioParams{
			Name:     "Portfolio " + string(rune('A'+i)),
			EntityID: entityID,
			Status:   "ACTIVE",
		}
		pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, params)
	}

	ctx := context.Background()

	// Page 1: limit=2, page=1
	page1Filter := http.Pagination{
		Limit:     2,
		Page:      1,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	page1, err := repo.FindAll(ctx, orgID, ledgerID, page1Filter)

	require.NoError(t, err)
	assert.Len(t, page1, 2, "page 1 should have 2 items")

	// Page 2: limit=2, page=2
	page2Filter := http.Pagination{
		Limit:     2,
		Page:      2,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	page2, err := repo.FindAll(ctx, orgID, ledgerID, page2Filter)

	require.NoError(t, err)
	assert.Len(t, page2, 2, "page 2 should have 2 items")

	// Page 3: limit=2, page=3 (should have 1 item)
	page3Filter := http.Pagination{
		Limit:     2,
		Page:      3,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	page3, err := repo.FindAll(ctx, orgID, ledgerID, page3Filter)

	require.NoError(t, err)
	assert.Len(t, page3, 1, "page 3 should have 1 item")

	// Verify no duplicates across pages
	allIDs := make(map[string]bool)
	for _, p := range page1 {
		allIDs[p.ID] = true
	}
	for _, p := range page2 {
		allIDs[p.ID] = true
	}
	for _, p := range page3 {
		allIDs[p.ID] = true
	}
	assert.Len(t, allIDs, 5, "should have 5 unique portfolios across all pages")
}

func TestIntegration_PortfolioRepository_FindAll_ExcludesDeleted(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create active portfolio
	activeParams := pgtestutil.PortfolioParams{
		Name:     "Active Portfolio",
		EntityID: "entity-active",
		Status:   "ACTIVE",
	}
	pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, activeParams)

	// Create deleted portfolio
	deletedAt := time.Now().Add(-1 * time.Hour)
	deletedParams := pgtestutil.PortfolioParams{
		Name:      "Deleted Portfolio",
		EntityID:  "entity-deleted",
		Status:    "ACTIVE",
		DeletedAt: &deletedAt,
	}
	pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, deletedParams)

	ctx := context.Background()

	// Act
	portfolios, err := repo.FindAll(ctx, orgID, ledgerID, defaultPagination())

	// Assert
	require.NoError(t, err, "FindAll should not return error")
	assert.Len(t, portfolios, 1, "should return only active portfolio")
	assert.Equal(t, "Active Portfolio", portfolios[0].Name)
}

// ============================================================================
// ListByIDs Tests
// ============================================================================

func TestIntegration_PortfolioRepository_ListByIDs_ReturnsMatchingPortfolios(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create 3 portfolios
	id1 := pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, pgtestutil.PortfolioParams{
		Name: "Portfolio 1", EntityID: "entity-1", Status: "ACTIVE",
	})
	id2 := pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, pgtestutil.PortfolioParams{
		Name: "Portfolio 2", EntityID: "entity-2", Status: "ACTIVE",
	})
	pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, pgtestutil.PortfolioParams{
		Name: "Portfolio 3", EntityID: "entity-3", Status: "ACTIVE",
	})

	ctx := context.Background()

	// Act - request only first 2
	portfolios, err := repo.ListByIDs(ctx, orgID, ledgerID, []uuid.UUID{id1, id2})

	// Assert
	require.NoError(t, err, "ListByIDs should not return error")
	assert.Len(t, portfolios, 2, "should return only 2 portfolios")

	names := make([]string, len(portfolios))
	for i, p := range portfolios {
		names[i] = p.Name
	}
	assert.ElementsMatch(t, []string{"Portfolio 1", "Portfolio 2"}, names)
}

func TestIntegration_PortfolioRepository_ListByIDs_ReturnsEmptyForNoMatches(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	ctx := context.Background()

	// Act
	nonExistentIDs := []uuid.UUID{libCommons.GenerateUUIDv7(), libCommons.GenerateUUIDv7()}
	portfolios, err := repo.ListByIDs(ctx, orgID, ledgerID, nonExistentIDs)

	// Assert
	require.NoError(t, err, "should not error for empty result")
	assert.Empty(t, portfolios, "should return empty slice")
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestIntegration_PortfolioRepository_Delete_SoftDeletesPortfolio(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.PortfolioParams{
		Name:     "To Delete",
		EntityID: "entity-to-delete",
		Status:   "ACTIVE",
	}
	portfolioID := pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	err := repo.Delete(ctx, orgID, ledgerID, portfolioID)

	// Assert
	require.NoError(t, err, "Delete should not return error")

	// Verify deleted_at is actually set in DB
	var deletedAt *time.Time
	err = container.DB.QueryRow(`SELECT deleted_at FROM portfolio WHERE id = $1`, portfolioID).Scan(&deletedAt)
	require.NoError(t, err, "should be able to query portfolio directly")
	require.NotNil(t, deletedAt, "deleted_at should be set")

	// Portfolio should not be findable anymore via repository
	found, err := repo.Find(ctx, orgID, ledgerID, portfolioID)
	require.Error(t, err, "Find should return error after delete")
	assert.Nil(t, found, "deleted portfolio should not be returned")
}

func TestIntegration_PortfolioRepository_Delete_ReturnsErrNotFound(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	err := repo.Delete(ctx, orgID, ledgerID, nonExistentID)

	// Assert
	require.Error(t, err, "Delete should return error for non-existent portfolio")

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
}

// ============================================================================
// Count Tests
// ============================================================================

func TestIntegration_PortfolioRepository_Count_ReturnsCorrectCount(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create 3 portfolios
	entityIDs := []string{"entity-1", "entity-2", "entity-3"}
	for i, entityID := range entityIDs {
		params := pgtestutil.PortfolioParams{
			Name:     "Portfolio " + string(rune('A'+i)),
			EntityID: entityID,
			Status:   "ACTIVE",
		}
		pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, params)
	}

	ctx := context.Background()

	// Act
	count, err := repo.Count(ctx, orgID, ledgerID)

	// Assert
	require.NoError(t, err, "Count should not return error")
	assert.Equal(t, int64(3), count, "count should be 3")
}

func TestIntegration_PortfolioRepository_Count_ExcludesDeletedPortfolios(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create 2 active portfolios
	pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, pgtestutil.PortfolioParams{
		Name: "Active 1", EntityID: "entity-active-1", Status: "ACTIVE",
	})
	pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, pgtestutil.PortfolioParams{
		Name: "Active 2", EntityID: "entity-active-2", Status: "ACTIVE",
	})

	// Create 1 deleted portfolio
	deletedAt := time.Now().Add(-1 * time.Hour)
	pgtestutil.CreateTestPortfolioWithParams(t, container.DB, orgID, ledgerID, pgtestutil.PortfolioParams{
		Name: "Deleted", EntityID: "entity-deleted", Status: "ACTIVE", DeletedAt: &deletedAt,
	})

	ctx := context.Background()

	// Act
	count, err := repo.Count(ctx, orgID, ledgerID)

	// Assert
	require.NoError(t, err, "Count should not return error")
	assert.Equal(t, int64(2), count, "count should exclude deleted portfolio")
}

func TestIntegration_PortfolioRepository_Count_ReturnsZeroForEmptyLedger(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	ctx := context.Background()

	// Act
	count, err := repo.Count(ctx, orgID, ledgerID)

	// Assert
	require.NoError(t, err, "Count should not return error")
	assert.Equal(t, int64(0), count, "count should be 0 for empty ledger")
}

// ============================================================================
// Helpers
// ============================================================================

func defaultPagination() http.Pagination {
	return http.Pagination{
		Limit:     10,
		Page:      1,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(-1, 0, 0), // 1 year ago
		EndDate:   time.Now().AddDate(0, 0, 1),  // 1 day ahead
	}
}
