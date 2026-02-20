//go:build integration

package accounttype

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createRepository creates an AccountTypePostgreSQLRepository connected to the test database.
func createRepository(t *testing.T, container *pgtestutil.ContainerResult) *AccountTypePostgreSQLRepository {
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

	return NewAccountTypePostgreSQLRepository(conn)
}

// ============================================================================
// FindByID Tests
// ============================================================================

func TestIntegration_AccountTypeRepository_FindByID_ReturnsAccountType(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.AccountTypeParams{
		Name:        "Savings Account",
		Description: "Account type for savings",
		KeyValue:    "savings",
	}
	accountTypeID := pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	accountType, err := repo.FindByID(ctx, orgID, ledgerID, accountTypeID)

	// Assert
	require.NoError(t, err, "FindByID should not return error for existing account type")
	require.NotNil(t, accountType, "account type should not be nil")

	assert.Equal(t, accountTypeID, accountType.ID, "ID should match")
	assert.Equal(t, orgID, accountType.OrganizationID, "organization ID should match")
	assert.Equal(t, ledgerID, accountType.LedgerID, "ledger ID should match")
	assert.Equal(t, "Savings Account", accountType.Name, "name should match")
	assert.Equal(t, "Account type for savings", accountType.Description, "description should match")
	assert.Equal(t, "savings", accountType.KeyValue, "key_value should match")
}

func TestIntegration_AccountTypeRepository_FindByID_ReturnsErrNotFound(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	accountType, err := repo.FindByID(ctx, orgID, ledgerID, nonExistentID)

	// Assert
	require.Error(t, err, "FindByID should return error for non-existent account type")
	assert.Nil(t, accountType, "account type should be nil")
	assert.ErrorIs(t, err, services.ErrDatabaseItemNotFound, "error should be ErrDatabaseItemNotFound")
}

func TestIntegration_AccountTypeRepository_FindByID_IgnoresDeletedAccountType(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Insert deleted account type
	deletedAt := time.Now().Add(-1 * time.Hour)
	params := pgtestutil.AccountTypeParams{
		Name:        "Deleted Type",
		Description: "This type was deleted",
		KeyValue:    "deleted-type",
		DeletedAt:   &deletedAt,
	}
	accountTypeID := pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	accountType, err := repo.FindByID(ctx, orgID, ledgerID, accountTypeID)

	// Assert
	require.Error(t, err, "FindByID should return error for deleted account type")
	assert.Nil(t, accountType, "deleted account type should not be returned")
}

// ============================================================================
// FindByKey Tests
// ============================================================================

func TestIntegration_AccountTypeRepository_FindByKey_ReturnsAccountType(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.AccountTypeParams{
		Name:        "Checking Account",
		Description: "Account type for checking",
		KeyValue:    "checking",
	}
	pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	accountType, err := repo.FindByKey(ctx, orgID, ledgerID, "checking")

	// Assert
	require.NoError(t, err, "FindByKey should not return error for existing key")
	require.NotNil(t, accountType)
	assert.Equal(t, "Checking Account", accountType.Name)
	assert.Equal(t, "checking", accountType.KeyValue)
}

func TestIntegration_AccountTypeRepository_FindByKey_ReturnsErrNotFound(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	ctx := context.Background()

	// Act
	accountType, err := repo.FindByKey(ctx, orgID, ledgerID, "non-existent-key")

	// Assert
	require.Error(t, err, "FindByKey should return error for non-existent key")
	assert.Nil(t, accountType)
	assert.ErrorIs(t, err, services.ErrDatabaseItemNotFound, "error should be ErrDatabaseItemNotFound")
}

func TestIntegration_AccountTypeRepository_FindByKey_CaseInsensitive(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.AccountTypeParams{
		Name:        "Investment Account",
		Description: "Account type for investments",
		KeyValue:    "investment",
	}
	pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act - search with uppercase (repository converts to lowercase)
	accountType, err := repo.FindByKey(ctx, orgID, ledgerID, "INVESTMENT")

	// Assert
	require.NoError(t, err, "FindByKey should find key case-insensitively")
	require.NotNil(t, accountType)
	assert.Equal(t, "investment", accountType.KeyValue)
}

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_AccountTypeRepository_Create(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	now := time.Now().Truncate(time.Microsecond)

	accountType := &mmodel.AccountType{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Name:           "Created Type",
		Description:    "Created via repository",
		KeyValue:       "created-type",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	ctx := context.Background()

	// Act
	created, err := repo.Create(ctx, orgID, ledgerID, accountType)

	// Assert
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, created)

	// Verify by finding
	found, err := repo.FindByID(ctx, orgID, ledgerID, accountType.ID)
	require.NoError(t, err)
	assert.Equal(t, "Created Type", found.Name)
	assert.Equal(t, "created-type", found.KeyValue)
}

func TestIntegration_AccountTypeRepository_Create_DuplicateKeyValueFails(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create first account type
	params := pgtestutil.AccountTypeParams{
		Name:        "Original Type",
		Description: "Original",
		KeyValue:    "duplicate-key",
	}
	pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, params)

	now := time.Now().Truncate(time.Microsecond)

	// Try to create second with same key_value
	accountType := &mmodel.AccountType{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Name:           "Duplicate Type",
		Description:    "Should fail",
		KeyValue:       "duplicate-key",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	ctx := context.Background()

	// Act
	created, err := repo.Create(ctx, orgID, ledgerID, accountType)

	// Assert
	require.Error(t, err, "Create should fail for duplicate key_value")
	assert.Nil(t, created)
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_AccountTypeRepository_Update_ChangesNameAndDescription(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.AccountTypeParams{
		Name:        "Original Name",
		Description: "Original Description",
		KeyValue:    "update-test",
	}
	accountTypeID := pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Get original to compare updated_at
	original, err := repo.FindByID(ctx, orgID, ledgerID, accountTypeID)
	require.NoError(t, err)
	originalUpdatedAt := original.UpdatedAt

	// Small delay to ensure updated_at differs
	time.Sleep(10 * time.Millisecond)

	// Act
	updateData := &mmodel.AccountType{
		Name:        "Updated Name",
		Description: "Updated Description",
	}
	updated, err := repo.Update(ctx, orgID, ledgerID, accountTypeID, updateData)

	// Assert
	require.NoError(t, err, "Update should not return error")
	require.NotNil(t, updated)

	found, err := repo.FindByID(ctx, orgID, ledgerID, accountTypeID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", found.Name, "name should be updated")
	assert.Equal(t, "Updated Description", found.Description, "description should be updated")
	assert.Equal(t, "update-test", found.KeyValue, "key_value should remain unchanged")
	assert.True(t, found.UpdatedAt.After(originalUpdatedAt), "updated_at should be changed after update")
}

func TestIntegration_AccountTypeRepository_Update_ReturnsErrNotFound(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	updateData := &mmodel.AccountType{
		Name: "Updated Name",
	}
	updated, err := repo.Update(ctx, orgID, ledgerID, nonExistentID, updateData)

	// Assert
	require.Error(t, err, "Update should return error for non-existent account type")
	assert.Nil(t, updated)
	assert.ErrorIs(t, err, services.ErrDatabaseItemNotFound, "error should be ErrDatabaseItemNotFound")
}

// ============================================================================
// FindAll Tests
// ============================================================================

func TestIntegration_AccountTypeRepository_FindAll_ReturnsAccountTypes(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create multiple account types
	params1 := pgtestutil.AccountTypeParams{
		Name:        "Type A",
		Description: "First type",
		KeyValue:    "type-a",
	}
	pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, params1)

	params2 := pgtestutil.AccountTypeParams{
		Name:        "Type B",
		Description: "Second type",
		KeyValue:    "type-b",
	}
	pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, params2)

	ctx := context.Background()

	// Act
	accountTypes, _, err := repo.FindAll(ctx, orgID, ledgerID, defaultPagination())

	// Assert
	require.NoError(t, err, "FindAll should not return error")
	assert.Len(t, accountTypes, 2, "should return 2 account types")
}

func TestIntegration_AccountTypeRepository_FindAll_EmptyForNonExistentLedger(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	nonExistentLedgerID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	accountTypes, _, err := repo.FindAll(ctx, orgID, nonExistentLedgerID, defaultPagination())

	// Assert
	require.NoError(t, err, "should not error for empty result")
	assert.Empty(t, accountTypes, "should return empty slice")
}

func TestIntegration_AccountTypeRepository_FindAll_Pagination(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create 5 account types
	const totalItems = 5
	for i := 0; i < totalItems; i++ {
		params := pgtestutil.AccountTypeParams{
			Name:        "Type " + string(rune('A'+i)),
			Description: "Description " + string(rune('A'+i)),
			KeyValue:    "type-" + string(rune('a'+i)),
		}
		pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, params)
	}

	ctx := context.Background()

	// Test 1: Limit is respected
	limitFilter := http.Pagination{
		Limit:     2,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	limited, cur, err := repo.FindAll(ctx, orgID, ledgerID, limitFilter)

	require.NoError(t, err)
	assert.Len(t, limited, 2, "should respect limit parameter")
	assert.NotEmpty(t, cur.Next, "should have next cursor when more items exist")

	// Test 2: Can retrieve all items with higher limit
	allFilter := http.Pagination{
		Limit:     10,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	all, _, err := repo.FindAll(ctx, orgID, ledgerID, allFilter)

	require.NoError(t, err)
	assert.Len(t, all, totalItems, "should return all items when limit exceeds total")
}

func TestIntegration_AccountTypeRepository_FindAll_FiltersByDateRange(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create an account type (created today)
	params := pgtestutil.AccountTypeParams{
		Name:        "Date Test Type",
		Description: "For date filtering",
		KeyValue:    "date-test",
	}
	pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act 1: Query with past-only window (should return 0 items)
	pastFilter := http.Pagination{
		Limit:     10,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(0, 0, -10),
		EndDate:   time.Now().AddDate(0, 0, -9),
	}
	accountTypesPast, _, err := repo.FindAll(ctx, orgID, ledgerID, pastFilter)
	require.NoError(t, err)
	assert.Empty(t, accountTypesPast, "past-only window should return 0 items")

	// Act 2: Query with today's window (should return 1 item)
	todayFilter := http.Pagination{
		Limit:     10,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(0, 0, -1),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	accountTypesToday, _, err := repo.FindAll(ctx, orgID, ledgerID, todayFilter)
	require.NoError(t, err)
	assert.Len(t, accountTypesToday, 1, "today's window should return 1 item")
}

// ============================================================================
// ListByIDs Tests
// ============================================================================

func TestIntegration_AccountTypeRepository_ListByIDs_ReturnsMatchingAccountTypes(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create 3 account types
	params1 := pgtestutil.AccountTypeParams{
		Name:        "Type 1",
		Description: "First",
		KeyValue:    "list-type-1",
	}
	id1 := pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, params1)

	params2 := pgtestutil.AccountTypeParams{
		Name:        "Type 2",
		Description: "Second",
		KeyValue:    "list-type-2",
	}
	id2 := pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, params2)

	params3 := pgtestutil.AccountTypeParams{
		Name:        "Type 3",
		Description: "Third",
		KeyValue:    "list-type-3",
	}
	pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, params3)

	ctx := context.Background()

	// Act - request only first 2
	accountTypes, err := repo.ListByIDs(ctx, orgID, ledgerID, []uuid.UUID{id1, id2})

	// Assert
	require.NoError(t, err, "ListByIDs should not return error")
	assert.Len(t, accountTypes, 2, "should return only 2 account types")

	names := make([]string, len(accountTypes))
	for i, at := range accountTypes {
		names[i] = at.Name
	}
	assert.ElementsMatch(t, []string{"Type 1", "Type 2"}, names)
}

func TestIntegration_AccountTypeRepository_ListByIDs_EmptyForNonExistentIDs(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	ctx := context.Background()

	// Act
	nonExistentIDs := []uuid.UUID{libCommons.GenerateUUIDv7(), libCommons.GenerateUUIDv7()}
	accountTypes, err := repo.ListByIDs(ctx, orgID, ledgerID, nonExistentIDs)

	// Assert
	require.NoError(t, err, "should not error for empty result")
	assert.Empty(t, accountTypes, "should return empty slice")
}

func TestIntegration_AccountTypeRepository_ListByIDs_IgnoresDeletedAccountTypes(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create active account type
	activeParams := pgtestutil.AccountTypeParams{
		Name:        "Active Type",
		Description: "Active",
		KeyValue:    "active-type",
	}
	activeID := pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, activeParams)

	// Create deleted account type
	deletedAt := time.Now().Add(-1 * time.Hour)
	deletedParams := pgtestutil.AccountTypeParams{
		Name:        "Deleted Type",
		Description: "Deleted",
		KeyValue:    "deleted-type-list",
		DeletedAt:   &deletedAt,
	}
	deletedID := pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, deletedParams)

	ctx := context.Background()

	// Act
	accountTypes, err := repo.ListByIDs(ctx, orgID, ledgerID, []uuid.UUID{activeID, deletedID})

	// Assert
	require.NoError(t, err)
	assert.Len(t, accountTypes, 1, "should return only active account type")
	assert.Equal(t, "Active Type", accountTypes[0].Name)
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestIntegration_AccountTypeRepository_Delete_SoftDeletesAccountType(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.AccountTypeParams{
		Name:        "To Delete",
		Description: "Will be deleted",
		KeyValue:    "to-delete",
	}
	accountTypeID := pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	err := repo.Delete(ctx, orgID, ledgerID, accountTypeID)

	// Assert
	require.NoError(t, err, "Delete should not return error")

	// Verify deleted_at is actually set in DB
	var deletedAt *time.Time
	err = container.DB.QueryRow(`SELECT deleted_at FROM account_type WHERE id = $1`, accountTypeID).Scan(&deletedAt)
	require.NoError(t, err, "should be able to query account type directly")
	require.NotNil(t, deletedAt, "deleted_at should be set")

	// Account type should not be findable anymore via repository
	found, err := repo.FindByID(ctx, orgID, ledgerID, accountTypeID)
	require.Error(t, err, "FindByID should return error after delete")
	assert.Nil(t, found, "deleted account type should not be returned")
}

func TestIntegration_AccountTypeRepository_Delete_ReturnsErrNotFound(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	err := repo.Delete(ctx, orgID, ledgerID, nonExistentID)

	// Assert
	require.Error(t, err, "Delete should return error for non-existent account type")
	assert.ErrorIs(t, err, services.ErrDatabaseItemNotFound, "error should be ErrDatabaseItemNotFound")
}

func TestIntegration_AccountTypeRepository_Delete_AllowsReusingSameKeyValue(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create and delete first account type
	params := pgtestutil.AccountTypeParams{
		Name:        "Original Type",
		Description: "Will be deleted",
		KeyValue:    "reusable-key",
	}
	firstID := pgtestutil.CreateTestAccountType(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	err := repo.Delete(ctx, orgID, ledgerID, firstID)
	require.NoError(t, err)

	// Create new account type with same key_value
	now := time.Now().Truncate(time.Microsecond)
	newAccountType := &mmodel.AccountType{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Name:           "New Type",
		Description:    "Using same key",
		KeyValue:       "reusable-key",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Act
	created, err := repo.Create(ctx, orgID, ledgerID, newAccountType)

	// Assert
	require.NoError(t, err, "should be able to create new account type with same key_value after delete")
	require.NotNil(t, created)
	assert.Equal(t, "reusable-key", created.KeyValue)
}

// ============================================================================
// Helpers
// ============================================================================

func defaultPagination() http.Pagination {
	return http.Pagination{
		Limit:     10,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(-1, 0, 0), // 1 year ago
		EndDate:   time.Now().AddDate(0, 0, 1),  // 1 day ahead
	}
}
