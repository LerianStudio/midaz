//go:build integration

package operationroute

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v3/pkg/testutils/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createRepository creates an OperationRoutePostgreSQLRepository connected to the test database.
func createRepository(t *testing.T, container *pgtestutil.ContainerResult) *OperationRoutePostgreSQLRepository {
	t.Helper()

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

	return NewOperationRoutePostgreSQLRepository(conn)
}

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_OperationRouteRepository_Create(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	operationRoute := &mmodel.OperationRoute{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Title:          "Cashin Route",
		Description:    "Route for cash-in operations",
		Code:           "CASHIN-001",
		OperationType:  "source",
		Account: &mmodel.AccountRule{
			RuleType: "alias",
			ValidIf:  "@cash_account",
		},
		CreatedAt: time.Now().Truncate(time.Microsecond),
		UpdatedAt: time.Now().Truncate(time.Microsecond),
	}

	ctx := context.Background()

	// Act
	created, err := repo.Create(ctx, orgID, ledgerID, operationRoute)

	// Assert
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, created, "created operation route should not be nil")

	assert.Equal(t, operationRoute.ID, created.ID, "ID should match")
	assert.Equal(t, orgID, created.OrganizationID, "organization ID should match")
	assert.Equal(t, ledgerID, created.LedgerID, "ledger ID should match")
	assert.Equal(t, "Cashin Route", created.Title, "title should match")
	assert.Equal(t, "Route for cash-in operations", created.Description, "description should match")
	assert.Equal(t, "CASHIN-001", created.Code, "code should match")
	assert.Equal(t, "source", created.OperationType, "operation type should match")
	require.NotNil(t, created.Account, "account should not be nil")
	assert.Equal(t, "alias", created.Account.RuleType, "account rule type should match")
	assert.Equal(t, "@cash_account", created.Account.ValidIf, "account rule valid_if should match")
}

func TestIntegration_OperationRouteRepository_Create_WithAccountTypeRule(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	operationRoute := &mmodel.OperationRoute{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Title:          "Account Type Route",
		Description:    "Route with account type validation",
		OperationType:  "destination",
		Account: &mmodel.AccountRule{
			RuleType: "account_type",
			ValidIf:  []string{"deposit", "savings", "checking"},
		},
		CreatedAt: time.Now().Truncate(time.Microsecond),
		UpdatedAt: time.Now().Truncate(time.Microsecond),
	}

	ctx := context.Background()

	// Act
	created, err := repo.Create(ctx, orgID, ledgerID, operationRoute)

	// Assert
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, created, "created operation route should not be nil")
	require.NotNil(t, created.Account, "account should not be nil")
	assert.Equal(t, "account_type", created.Account.RuleType, "account rule type should match")

	// ValidIf is stored as comma-separated and returned as []string for account_type
	validIf, ok := created.Account.ValidIf.([]string)
	require.True(t, ok, "ValidIf should be []string for account_type rule")
	assert.Contains(t, validIf, "deposit", "ValidIf should contain deposit")
	assert.Contains(t, validIf, "savings", "ValidIf should contain savings")
	assert.Contains(t, validIf, "checking", "ValidIf should contain checking")
}

func TestIntegration_OperationRouteRepository_Create_WithoutOptionalFields(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	operationRoute := &mmodel.OperationRoute{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Title:          "Minimal Route",
		OperationType:  "source",
		CreatedAt:      time.Now().Truncate(time.Microsecond),
		UpdatedAt:      time.Now().Truncate(time.Microsecond),
	}

	ctx := context.Background()

	// Act
	created, err := repo.Create(ctx, orgID, ledgerID, operationRoute)

	// Assert
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, created, "created operation route should not be nil")

	assert.Empty(t, created.Description, "description should be empty")
	assert.Empty(t, created.Code, "code should be empty")
	assert.Nil(t, created.Account, "account should be nil when no rules provided")
}

// ============================================================================
// FindByID Tests
// ============================================================================

func TestIntegration_OperationRouteRepository_FindByID(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert test operation route
	code := "FIND-001"
	accountRuleType := "alias"
	accountRuleValidIf := "@test_account"
	params := pgtestutil.OperationRouteParams{
		Title:              "Findable Route",
		Description:        "Route to be found",
		Code:               &code,
		OperationType:      "destination",
		AccountRuleType:    &accountRuleType,
		AccountRuleValidIf: &accountRuleValidIf,
	}
	operationRouteID := pgtestutil.CreateTestOperationRoute(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	found, err := repo.FindByID(ctx, orgID, ledgerID, operationRouteID)

	// Assert
	require.NoError(t, err, "FindByID should not return error")
	require.NotNil(t, found, "found operation route should not be nil")

	assert.Equal(t, operationRouteID, found.ID, "ID should match")
	assert.Equal(t, orgID, found.OrganizationID, "organization ID should match")
	assert.Equal(t, ledgerID, found.LedgerID, "ledger ID should match")
	assert.Equal(t, "Findable Route", found.Title, "title should match")
	assert.Equal(t, "Route to be found", found.Description, "description should match")
	assert.Equal(t, "FIND-001", found.Code, "code should match")
	assert.Equal(t, "destination", found.OperationType, "operation type should match")
	require.NotNil(t, found.Account, "account should not be nil")
	assert.Equal(t, "alias", found.Account.RuleType, "account rule type should match")
}

func TestIntegration_OperationRouteRepository_FindByID_NotFound(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	found, err := repo.FindByID(ctx, orgID, ledgerID, nonExistentID)

	// Assert
	require.Error(t, err, "FindByID should return error for non-existent ID")
	assert.Nil(t, found, "found operation route should be nil")

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
	assert.Equal(t, constant.ErrOperationRouteNotFound.Error(), entityNotFoundErr.Code, "error code should be ErrOperationRouteNotFound")
}

func TestIntegration_OperationRouteRepository_FindByID_WrongOrganization(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	otherOrgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert test operation route in orgID
	operationRouteID := pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Org Route", "source")

	ctx := context.Background()

	// Act - try to find with different org
	found, err := repo.FindByID(ctx, otherOrgID, ledgerID, operationRouteID)

	// Assert
	require.Error(t, err, "FindByID should return error for wrong organization")
	assert.Nil(t, found, "found operation route should be nil")
}

func TestIntegration_OperationRouteRepository_FindByID_SoftDeleted(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert soft-deleted operation route
	deletedAt := time.Now().Add(-1 * time.Hour)
	params := pgtestutil.DefaultOperationRouteParams()
	params.Title = "Deleted Route"
	params.DeletedAt = &deletedAt
	operationRouteID := pgtestutil.CreateTestOperationRoute(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	found, err := repo.FindByID(ctx, orgID, ledgerID, operationRouteID)

	// Assert
	require.Error(t, err, "FindByID should return error for soft-deleted record")
	assert.Nil(t, found, "found operation route should be nil")
}

// ============================================================================
// FindByIDs Tests
// ============================================================================

func TestIntegration_OperationRouteRepository_FindByIDs(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert multiple operation routes
	id1 := pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Route 1", "source")
	id2 := pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Route 2", "destination")
	id3 := pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Route 3", "source")

	ctx := context.Background()

	// Act
	routes, err := repo.FindByIDs(ctx, orgID, ledgerID, []uuid.UUID{id1, id2, id3})

	// Assert
	require.NoError(t, err, "FindByIDs should not return error")
	assert.Len(t, routes, 3, "should return all 3 routes")

	// Verify all IDs are present
	foundIDs := make(map[uuid.UUID]bool)
	for _, route := range routes {
		foundIDs[route.ID] = true
	}
	assert.True(t, foundIDs[id1], "should contain id1")
	assert.True(t, foundIDs[id2], "should contain id2")
	assert.True(t, foundIDs[id3], "should contain id3")
}

func TestIntegration_OperationRouteRepository_FindByIDs_EmptyInput(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	routes, err := repo.FindByIDs(ctx, orgID, ledgerID, []uuid.UUID{})

	// Assert
	require.NoError(t, err, "FindByIDs should not return error for empty input")
	assert.Empty(t, routes, "should return empty slice")
}

func TestIntegration_OperationRouteRepository_FindByIDs_SomeMissing(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert only one route
	existingID := pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Existing Route", "source")
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	routes, err := repo.FindByIDs(ctx, orgID, ledgerID, []uuid.UUID{existingID, nonExistentID})

	// Assert
	require.Error(t, err, "FindByIDs should return error when some IDs are missing")
	assert.Nil(t, routes, "routes should be nil on error")

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
}

func TestIntegration_OperationRouteRepository_FindByIDs_ExcludesSoftDeleted(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert active route
	activeID := pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Active Route", "source")

	// Insert soft-deleted route
	deletedAt := time.Now().Add(-1 * time.Hour)
	deletedParams := pgtestutil.DefaultOperationRouteParams()
	deletedParams.Title = "Deleted Route"
	deletedParams.DeletedAt = &deletedAt
	deletedID := pgtestutil.CreateTestOperationRoute(t, container.DB, orgID, ledgerID, deletedParams)

	ctx := context.Background()

	// Act - request both IDs
	routes, err := repo.FindByIDs(ctx, orgID, ledgerID, []uuid.UUID{activeID, deletedID})

	// Assert - should fail because deleted route is not found
	require.Error(t, err, "FindByIDs should return error when soft-deleted ID is requested")
	assert.Nil(t, routes, "routes should be nil on error")
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_OperationRouteRepository_Update(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert initial operation route
	operationRouteID := pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Original Title", "source")

	ctx := context.Background()

	// Prepare update
	updateData := &mmodel.OperationRoute{
		Title:       "Updated Title",
		Description: "Updated description",
		Code:        "UPDATED-001",
		Account: &mmodel.AccountRule{
			RuleType: "alias",
			ValidIf:  "@updated_account",
		},
	}

	// Act
	updated, err := repo.Update(ctx, orgID, ledgerID, operationRouteID, updateData)

	// Assert
	require.NoError(t, err, "Update should not return error")
	require.NotNil(t, updated, "updated operation route should not be nil")

	assert.Equal(t, "Updated Title", updated.Title, "title should be updated")
	assert.Equal(t, "Updated description", updated.Description, "description should be updated")
	assert.Equal(t, "UPDATED-001", updated.Code, "code should be updated")
	require.NotNil(t, updated.Account, "account should not be nil")
	assert.Equal(t, "alias", updated.Account.RuleType, "account rule type should be updated")
}

func TestIntegration_OperationRouteRepository_Update_PartialFields(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert initial operation route with all fields
	code := "ORIGINAL-001"
	params := pgtestutil.OperationRouteParams{
		Title:         "Original Title",
		Description:   "Original description",
		Code:          &code,
		OperationType: "source",
	}
	operationRouteID := pgtestutil.CreateTestOperationRoute(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Update only title (other fields empty - should not update them)
	updateData := &mmodel.OperationRoute{
		Title: "Only Title Updated",
	}

	// Act
	updated, err := repo.Update(ctx, orgID, ledgerID, operationRouteID, updateData)

	// Assert
	require.NoError(t, err, "Update should not return error")
	require.NotNil(t, updated, "updated operation route should not be nil")

	assert.Equal(t, "Only Title Updated", updated.Title, "title should be updated")
	// Note: The update method only sets fields that are non-empty in the input
}

func TestIntegration_OperationRouteRepository_Update_NotFound(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	updateData := &mmodel.OperationRoute{
		Title: "New Title",
	}

	// Act
	updated, err := repo.Update(ctx, orgID, ledgerID, nonExistentID, updateData)

	// Assert
	require.Error(t, err, "Update should return error for non-existent ID")
	assert.Nil(t, updated, "updated operation route should be nil")
	assert.ErrorIs(t, err, services.ErrDatabaseItemNotFound, "error should be ErrDatabaseItemNotFound")
}

func TestIntegration_OperationRouteRepository_Update_SoftDeleted(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert soft-deleted operation route
	deletedAt := time.Now().Add(-1 * time.Hour)
	params := pgtestutil.DefaultOperationRouteParams()
	params.Title = "Deleted Route"
	params.DeletedAt = &deletedAt
	operationRouteID := pgtestutil.CreateTestOperationRoute(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	updateData := &mmodel.OperationRoute{
		Title: "Should Not Update",
	}

	// Act
	updated, err := repo.Update(ctx, orgID, ledgerID, operationRouteID, updateData)

	// Assert
	require.Error(t, err, "Update should return error for soft-deleted record")
	assert.Nil(t, updated, "updated operation route should be nil")
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestIntegration_OperationRouteRepository_Delete(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert operation route
	operationRouteID := pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "To Delete", "source")

	ctx := context.Background()

	// Act
	err := repo.Delete(ctx, orgID, ledgerID, operationRouteID)

	// Assert
	require.NoError(t, err, "Delete should not return error")

	// Verify it's soft-deleted (FindByID should fail)
	found, findErr := repo.FindByID(ctx, orgID, ledgerID, operationRouteID)
	require.Error(t, findErr, "FindByID should return error after delete")
	assert.Nil(t, found, "found should be nil after delete")
}

func TestIntegration_OperationRouteRepository_Delete_AlreadyDeleted(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert already soft-deleted operation route
	deletedAt := time.Now().Add(-1 * time.Hour)
	params := pgtestutil.DefaultOperationRouteParams()
	params.Title = "Already Deleted"
	params.DeletedAt = &deletedAt
	operationRouteID := pgtestutil.CreateTestOperationRoute(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act - delete already deleted record
	// Note: The current implementation doesn't check rows affected for delete,
	// so this will succeed silently (no error returned)
	err := repo.Delete(ctx, orgID, ledgerID, operationRouteID)

	// Assert - current behavior: no error (DELETE WHERE deleted_at IS NULL affects 0 rows)
	require.NoError(t, err, "Delete does not return error for already-deleted record (known behavior)")
}

// ============================================================================
// FindAll Tests
// ============================================================================

func TestIntegration_OperationRouteRepository_FindAll(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert multiple operation routes
	pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Route 1", "source")
	time.Sleep(5 * time.Millisecond) // Ensure different timestamps
	pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Route 2", "destination")
	time.Sleep(5 * time.Millisecond)
	pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Route 3", "source")

	ctx := context.Background()

	// Date range to include test data
	startDate := time.Now().Add(-24 * time.Hour)
	endDate := time.Now().Add(24 * time.Hour)

	filter := http.Pagination{
		Limit:     10,
		StartDate: startDate,
		EndDate:   endDate,
	}

	// Act
	routes, pagination, err := repo.FindAll(ctx, orgID, ledgerID, filter)

	// Assert
	require.NoError(t, err, "FindAll should not return error")
	assert.Len(t, routes, 3, "should return all 3 routes")
	assert.NotNil(t, pagination, "pagination should not be nil")
}

func TestIntegration_OperationRouteRepository_FindAll_Empty(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	startDate := time.Now().Add(-24 * time.Hour)
	endDate := time.Now().Add(24 * time.Hour)

	filter := http.Pagination{
		Limit:     10,
		StartDate: startDate,
		EndDate:   endDate,
	}

	// Act
	routes, _, err := repo.FindAll(ctx, orgID, ledgerID, filter)

	// Assert
	require.NoError(t, err, "FindAll should not return error for empty result")
	assert.Empty(t, routes, "should return empty slice")
}

func TestIntegration_OperationRouteRepository_FindAll_Pagination(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert more routes than page size
	for i := 0; i < 5; i++ {
		pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Route "+string(rune('A'+i)), "source")
		time.Sleep(5 * time.Millisecond) // Ensure different timestamps
	}

	ctx := context.Background()

	startDate := time.Now().Add(-24 * time.Hour)
	endDate := time.Now().Add(24 * time.Hour)

	// First page with limit 2
	filter := http.Pagination{
		Limit:     2,
		StartDate: startDate,
		EndDate:   endDate,
	}

	// Act
	routes, pagination, err := repo.FindAll(ctx, orgID, ledgerID, filter)

	// Assert
	require.NoError(t, err, "FindAll should not return error")
	assert.Len(t, routes, 2, "should return limit number of routes")
	assert.NotEmpty(t, pagination.Next, "next cursor should be set for more pages")
}

func TestIntegration_OperationRouteRepository_FindAll_ExcludesSoftDeleted(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert active route
	pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Active Route", "source")

	// Insert soft-deleted route
	deletedAt := time.Now().Add(-1 * time.Hour)
	deletedParams := pgtestutil.DefaultOperationRouteParams()
	deletedParams.Title = "Deleted Route"
	deletedParams.DeletedAt = &deletedAt
	pgtestutil.CreateTestOperationRoute(t, container.DB, orgID, ledgerID, deletedParams)

	ctx := context.Background()

	startDate := time.Now().Add(-24 * time.Hour)
	endDate := time.Now().Add(24 * time.Hour)

	filter := http.Pagination{
		Limit:     10,
		StartDate: startDate,
		EndDate:   endDate,
	}

	// Act
	routes, _, err := repo.FindAll(ctx, orgID, ledgerID, filter)

	// Assert
	require.NoError(t, err, "FindAll should not return error")
	assert.Len(t, routes, 1, "should only return active route")
	assert.Equal(t, "Active Route", routes[0].Title, "should be the active route")
}

// ============================================================================
// HasTransactionRouteLinks Tests
// ============================================================================

func TestIntegration_OperationRouteRepository_HasTransactionRouteLinks_NoLinks(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert operation route without links
	operationRouteID := pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Unlinked Route", "source")

	ctx := context.Background()

	// Act
	hasLinks, err := repo.HasTransactionRouteLinks(ctx, operationRouteID)

	// Assert
	require.NoError(t, err, "HasTransactionRouteLinks should not return error")
	assert.False(t, hasLinks, "should return false for unlinked route")
}

func TestIntegration_OperationRouteRepository_HasTransactionRouteLinks_WithLinks(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert operation route
	operationRouteID := pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Linked Route", "source")

	// Insert transaction route
	transactionRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, container.DB, orgID, ledgerID, "Transaction Route")

	// Create link
	pgtestutil.CreateTestOperationTransactionRouteLink(t, container.DB, operationRouteID, transactionRouteID)

	ctx := context.Background()

	// Act
	hasLinks, err := repo.HasTransactionRouteLinks(ctx, operationRouteID)

	// Assert
	require.NoError(t, err, "HasTransactionRouteLinks should not return error")
	assert.True(t, hasLinks, "should return true for linked route")
}

func TestIntegration_OperationRouteRepository_HasTransactionRouteLinks_SoftDeletedLink(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert operation route
	operationRouteID := pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Route With Deleted Link", "source")

	// Insert transaction route
	transactionRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, container.DB, orgID, ledgerID, "Transaction Route")

	// Create and soft-delete link
	linkID := pgtestutil.CreateTestOperationTransactionRouteLink(t, container.DB, operationRouteID, transactionRouteID)
	pgtestutil.SoftDeleteOperationTransactionRouteLink(t, container.DB, linkID)

	ctx := context.Background()

	// Act
	hasLinks, err := repo.HasTransactionRouteLinks(ctx, operationRouteID)

	// Assert
	require.NoError(t, err, "HasTransactionRouteLinks should not return error")
	assert.False(t, hasLinks, "should return false when links are soft-deleted")
}

// ============================================================================
// FindTransactionRouteIDs Tests
// ============================================================================

func TestIntegration_OperationRouteRepository_FindTransactionRouteIDs_NoLinks(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert operation route without links
	operationRouteID := pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "No Links Route", "source")

	ctx := context.Background()

	// Act
	ids, err := repo.FindTransactionRouteIDs(ctx, operationRouteID)

	// Assert
	require.NoError(t, err, "FindTransactionRouteIDs should not return error")
	assert.Empty(t, ids, "should return empty slice for unlinked route")
}

func TestIntegration_OperationRouteRepository_FindTransactionRouteIDs_WithLinks(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert operation route
	operationRouteID := pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Multi-Link Route", "source")

	// Insert multiple transaction routes
	txRouteID1 := pgtestutil.CreateTestTransactionRouteSimple(t, container.DB, orgID, ledgerID, "TX Route 1")
	time.Sleep(5 * time.Millisecond) // Ensure different created_at for ordering
	txRouteID2 := pgtestutil.CreateTestTransactionRouteSimple(t, container.DB, orgID, ledgerID, "TX Route 2")
	time.Sleep(5 * time.Millisecond)
	txRouteID3 := pgtestutil.CreateTestTransactionRouteSimple(t, container.DB, orgID, ledgerID, "TX Route 3")

	// Create links
	pgtestutil.CreateTestOperationTransactionRouteLink(t, container.DB, operationRouteID, txRouteID1)
	time.Sleep(5 * time.Millisecond)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, container.DB, operationRouteID, txRouteID2)
	time.Sleep(5 * time.Millisecond)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, container.DB, operationRouteID, txRouteID3)

	ctx := context.Background()

	// Act
	ids, err := repo.FindTransactionRouteIDs(ctx, operationRouteID)

	// Assert
	require.NoError(t, err, "FindTransactionRouteIDs should not return error")
	assert.Len(t, ids, 3, "should return all 3 linked transaction route IDs")

	// Verify all IDs are present
	foundIDs := make(map[uuid.UUID]bool)
	for _, id := range ids {
		foundIDs[id] = true
	}
	assert.True(t, foundIDs[txRouteID1], "should contain txRouteID1")
	assert.True(t, foundIDs[txRouteID2], "should contain txRouteID2")
	assert.True(t, foundIDs[txRouteID3], "should contain txRouteID3")
}

func TestIntegration_OperationRouteRepository_FindTransactionRouteIDs_ExcludesSoftDeletedLinks(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert operation route
	operationRouteID := pgtestutil.CreateTestOperationRouteSimple(t, container.DB, orgID, ledgerID, "Mixed Links Route", "source")

	// Insert transaction routes
	activeTxRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, container.DB, orgID, ledgerID, "Active TX Route")
	deletedTxRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, container.DB, orgID, ledgerID, "Deleted TX Route")

	// Create links
	pgtestutil.CreateTestOperationTransactionRouteLink(t, container.DB, operationRouteID, activeTxRouteID)
	deletedLinkID := pgtestutil.CreateTestOperationTransactionRouteLink(t, container.DB, operationRouteID, deletedTxRouteID)

	// Soft-delete one link
	pgtestutil.SoftDeleteOperationTransactionRouteLink(t, container.DB, deletedLinkID)

	ctx := context.Background()

	// Act
	ids, err := repo.FindTransactionRouteIDs(ctx, operationRouteID)

	// Assert
	require.NoError(t, err, "FindTransactionRouteIDs should not return error")
	assert.Len(t, ids, 1, "should only return active link")
	assert.Equal(t, activeTxRouteID, ids[0], "should be the active transaction route ID")
}
