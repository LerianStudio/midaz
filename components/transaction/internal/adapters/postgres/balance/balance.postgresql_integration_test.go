//go:build integration

package balance

import (
	"context"
	"sync"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createRepository creates a BalanceRepository connected to the test database.
func createRepository(t *testing.T, container *pgtestutil.ContainerResult) *BalancePostgreSQLRepository {
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

	return NewBalancePostgreSQLRepository(conn)
}

// createTestAccountForBalance inserts a minimal account directly for balance tests.
// Transaction component doesn't have account table, so we skip FK validation.
func createTestAccountID() uuid.UUID {
	return libCommons.GenerateUUIDv7()
}

// ============================================================================
// Find Tests
// ============================================================================

func TestIntegration_BalanceRepository_Find_ReturnsBalance(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	// Insert test balance directly
	params := pgtestutil.BalanceParams{
		Alias:          "@find-test",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(500),
		OnHold:         decimal.NewFromInt(50),
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}
	balanceID := pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params)

	ctx := context.Background()

	// Act
	balance, err := repo.Find(ctx, orgID, ledgerID, balanceID)

	// Assert
	require.NoError(t, err, "Find should not return error for existing balance")
	require.NotNil(t, balance, "balance should not be nil")

	assert.Equal(t, balanceID.String(), balance.ID, "balance ID should match")
	assert.Equal(t, orgID.String(), balance.OrganizationID, "organization ID should match")
	assert.Equal(t, ledgerID.String(), balance.LedgerID, "ledger ID should match")
	assert.Equal(t, accountID.String(), balance.AccountID, "account ID should match")
	assert.Equal(t, "@find-test", balance.Alias, "alias should match")
	assert.Equal(t, "default", balance.Key, "key should match")
	assert.Equal(t, "USD", balance.AssetCode, "asset code should match")
	assert.True(t, balance.Available.Equal(decimal.NewFromInt(500)), "available should match")
	assert.True(t, balance.OnHold.Equal(decimal.NewFromInt(50)), "on_hold should match")
	assert.Equal(t, "deposit", balance.AccountType, "account type should match")
	assert.True(t, balance.AllowSending, "allow_sending should be true")
	assert.True(t, balance.AllowReceiving, "allow_receiving should be true")
}

func TestIntegration_BalanceRepository_Find_ReturnsEntityNotFoundError(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	balance, err := repo.Find(ctx, orgID, ledgerID, nonExistentID)

	// Assert
	require.Error(t, err, "Find should return error for non-existent balance")
	assert.Nil(t, balance, "balance should be nil")

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
	assert.Equal(t, constant.ErrEntityNotFound.Error(), entityNotFoundErr.Code, "error code should be ErrEntityNotFound")
	assert.Equal(t, "Balance", entityNotFoundErr.EntityType, "entity type should be Balance")
}

func TestIntegration_BalanceRepository_Find_IgnoresDeletedBalance(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	// Insert deleted balance
	deletedAt := time.Now().Add(-1 * time.Hour)
	params := pgtestutil.BalanceParams{
		Alias:          "@deleted",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		DeletedAt:      &deletedAt,
	}
	balanceID := pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params)

	ctx := context.Background()

	// Act
	balance, err := repo.Find(ctx, orgID, ledgerID, balanceID)

	// Assert
	require.Error(t, err, "Find should return error for deleted balance")
	assert.Nil(t, balance, "deleted balance should not be returned")
}

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_BalanceRepository_Create_Success(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()
	balanceID := libCommons.GenerateUUIDv7()

	now := time.Now().Truncate(time.Microsecond)

	balance := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          "@created",
		Key:            "savings",
		AssetCode:      "EUR",
		Available:      decimal.NewFromInt(2000),
		OnHold:         decimal.NewFromInt(100),
		Version:        1,
		AccountType:    "savings",
		AllowSending:   true,
		AllowReceiving: false,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	ctx := context.Background()

	// Act
	err := repo.Create(ctx, balance)

	// Assert
	require.NoError(t, err, "Create should not return error")

	// Verify by finding
	found, err := repo.Find(ctx, orgID, ledgerID, balanceID)
	require.NoError(t, err)
	assert.Equal(t, "@created", found.Alias)
	assert.Equal(t, "savings", found.Key)
	assert.True(t, found.Available.Equal(decimal.NewFromInt(2000)))
	assert.False(t, found.AllowReceiving, "allow_receiving should be false")
}

// ============================================================================
// Schema Default Values Tests
// ============================================================================

func TestIntegration_BalanceRepository_SchemaDefaults(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	tests := []struct {
		name       string
		insertSQL  string
		argsFunc   func(id, orgID, ledgerID, accountID uuid.UUID, now time.Time) []any
		assertFunc func(t *testing.T, balance *mmodel.Balance)
	}{
		{
			name: "key defaults to 'default'",
			insertSQL: `
				INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, asset_code, available, on_hold, version, account_type, allow_sending, allow_receiving, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			argsFunc: func(id, orgID, ledgerID, accountID uuid.UUID, now time.Time) []any {
				return []any{id, orgID, ledgerID, accountID, "@test-key", "USD", 1000, 0, 1, "deposit", true, true, now, now}
			},
			assertFunc: func(t *testing.T, balance *mmodel.Balance) {
				assert.Equal(t, "default", balance.Key, "key should default to 'default'")
			},
		},
		{
			name: "version defaults to 0",
			insertSQL: `
				INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, asset_code, available, on_hold, account_type, allow_sending, allow_receiving, created_at, updated_at, key)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			argsFunc: func(id, orgID, ledgerID, accountID uuid.UUID, now time.Time) []any {
				return []any{id, orgID, ledgerID, accountID, "@test-version", "USD", 1000, 0, "deposit", true, true, now, now, "default"}
			},
			assertFunc: func(t *testing.T, balance *mmodel.Balance) {
				assert.Equal(t, int64(0), balance.Version, "version should default to 0")
			},
		},
		{
			name: "updated_at defaults to now()",
			insertSQL: `
				INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, asset_code, available, on_hold, version, account_type, allow_sending, allow_receiving, created_at, key)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			argsFunc: func(id, orgID, ledgerID, accountID uuid.UUID, now time.Time) []any {
				return []any{id, orgID, ledgerID, accountID, "@test-updated", "USD", 1000, 0, 1, "deposit", true, true, now, "default"}
			},
			assertFunc: func(t *testing.T, balance *mmodel.Balance) {
				now := time.Now()
				assert.True(t, balance.UpdatedAt.After(now.Add(-5*time.Second)), "updated_at should be recent")
				assert.True(t, balance.UpdatedAt.Before(now.Add(5*time.Second)), "updated_at should not be in future")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orgID := libCommons.GenerateUUIDv7()
			ledgerID := libCommons.GenerateUUIDv7()
			accountID := libCommons.GenerateUUIDv7()
			balanceID := libCommons.GenerateUUIDv7()
			now := time.Now().Truncate(time.Microsecond)

			args := tt.argsFunc(balanceID, orgID, ledgerID, accountID, now)
			_, err := container.DB.Exec(tt.insertSQL, args...)
			require.NoError(t, err, "raw insert should succeed")

			balance, err := repo.Find(ctx, orgID, ledgerID, balanceID)
			require.NoError(t, err, "Find should succeed")

			tt.assertFunc(t, balance)
		})
	}
}

// ============================================================================
// ListAllByAccountID Tests
// ============================================================================

func TestIntegration_BalanceRepository_ListAllByAccountID_ReturnsBalances(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	// Create multiple balances for same account with different keys
	params1 := pgtestutil.DefaultBalanceParams()
	params1.Alias = "@multi"
	params1.Key = "default"
	pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params1)

	params2 := pgtestutil.DefaultBalanceParams()
	params2.Alias = "@multi"
	params2.Key = "savings"
	params2.Available = decimal.NewFromInt(2000)
	pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params2)

	ctx := context.Background()

	// Act
	balances, cur, err := repo.ListAllByAccountID(ctx, orgID, ledgerID, accountID, defaultPagination())

	// Assert
	require.NoError(t, err, "ListAllByAccountID should not return error")
	assert.Len(t, balances, 2, "should return 2 balances")
	assert.Empty(t, cur.Next, "should not have next cursor")
}

func TestIntegration_BalanceRepository_ListAllByAccountID_EmptyForNonExistentAccount(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	nonExistentAccountID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	balances, _, err := repo.ListAllByAccountID(ctx, orgID, ledgerID, nonExistentAccountID, defaultPagination())

	// Assert
	require.NoError(t, err, "should not error for empty result")
	assert.Empty(t, balances, "should return empty slice")
}

func TestIntegration_BalanceRepository_ListAllByAccountID_FiltersByDateRange(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	// Create a balance (created today)
	params := pgtestutil.DefaultBalanceParams()
	params.Alias = "@date-account-test"
	pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params)

	ctx := context.Background()

	// Act 1: Query with past-only window (should return 0 items)
	pastFilter := http.Pagination{
		Limit:     10,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(0, 0, -10),
		EndDate:   time.Now().AddDate(0, 0, -9),
	}
	balancesPast, _, err := repo.ListAllByAccountID(ctx, orgID, ledgerID, accountID, pastFilter)
	require.NoError(t, err)
	assert.Empty(t, balancesPast, "past-only window should return 0 items")

	// Act 2: Query with today's window (should return 1 item)
	todayFilter := http.Pagination{
		Limit:     10,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(0, 0, -1),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	balancesToday, _, err := repo.ListAllByAccountID(ctx, orgID, ledgerID, accountID, todayFilter)
	require.NoError(t, err)
	assert.Len(t, balancesToday, 1, "today's window should return 1 item")
}

func TestIntegration_BalanceRepository_ListAllByAccountID_Pagination(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	// Create 7 balances with different keys for the same account
	for i := 0; i < 7; i++ {
		params := pgtestutil.DefaultBalanceParams()
		params.Alias = "@pagination-account"
		params.Key = "key-" + string(rune('a'+i))
		params.Available = decimal.NewFromInt(int64(i * 100))
		pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params)
	}

	ctx := context.Background()

	// Page 1: limit=3
	page1Filter := http.Pagination{
		Limit:     3,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	page1, cur1, err := repo.ListAllByAccountID(ctx, orgID, ledgerID, accountID, page1Filter)

	require.NoError(t, err)
	assert.Len(t, page1, 3, "page 1 should have 3 items")
	assert.NotEmpty(t, cur1.Next, "page 1 should have next cursor")

	// Page 2: using next cursor
	page2Filter := http.Pagination{
		Limit:     3,
		SortOrder: "DESC",
		Cursor:    cur1.Next,
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	page2, cur2, err := repo.ListAllByAccountID(ctx, orgID, ledgerID, accountID, page2Filter)

	require.NoError(t, err)
	assert.Len(t, page2, 3, "page 2 should have 3 items")
	assert.NotEmpty(t, cur2.Prev, "page 2 should have prev cursor")

	// Page 3: last page with 1 item
	page3Filter := http.Pagination{
		Limit:     3,
		SortOrder: "DESC",
		Cursor:    cur2.Next,
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	page3, cur3, err := repo.ListAllByAccountID(ctx, orgID, ledgerID, accountID, page3Filter)

	require.NoError(t, err)
	assert.Len(t, page3, 1, "page 3 should have 1 item")
	assert.Empty(t, cur3.Next, "page 3 should not have next cursor")
	assert.NotEmpty(t, cur3.Prev, "page 3 should have prev cursor")
}

func TestIntegration_BalanceRepository_ListAllByAccountID_PreservesLargePrecision(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	// Use shared helper to create balance with large precision values
	testData := createLargePrecisionBalance(t, container, orgID, ledgerID, accountID, "@large-precision-account")

	ctx := context.Background()

	// Act
	balances, _, err := repo.ListAllByAccountID(ctx, orgID, ledgerID, accountID, defaultPagination())

	// Assert
	require.NoError(t, err)
	require.Len(t, balances, 1)
	assert.True(t, balances[0].Available.Equal(testData.LargeAvail), "available should preserve large precision")
	assert.True(t, balances[0].OnHold.Equal(testData.LargeHold), "on_hold should preserve large precision")
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestIntegration_BalanceRepository_Delete_SoftDeletesBalance(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = "@to-delete"
	balanceID := pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params)

	ctx := context.Background()

	// Act
	err := repo.Delete(ctx, orgID, ledgerID, balanceID)

	// Assert
	require.NoError(t, err, "Delete should not return error")

	// Verify deleted_at is actually set in DB
	var deletedAt *time.Time
	err = container.DB.QueryRow(`SELECT deleted_at FROM balance WHERE id = $1`, balanceID).Scan(&deletedAt)
	require.NoError(t, err, "should be able to query balance directly")
	require.NotNil(t, deletedAt, "deleted_at should be set")

	// Balance should not be findable anymore via repository
	found, err := repo.Find(ctx, orgID, ledgerID, balanceID)
	require.Error(t, err, "Find should return error after delete")
	assert.Nil(t, found, "deleted balance should not be returned")
}

func TestIntegration_BalanceRepository_Delete_ReturnsErrorForNonExistent(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	err := repo.Delete(ctx, orgID, ledgerID, nonExistentID)

	// Assert
	require.Error(t, err, "Delete should return error for non-existent balance")

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_BalanceRepository_Update_ReturnsUpdatedBalance(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = "@update-return-test"
	params.AllowSending = true
	params.AllowReceiving = true
	balanceID := pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params)

	ctx := context.Background()

	// Get original balance to compare updated_at
	original, err := repo.Find(ctx, orgID, ledgerID, balanceID)
	require.NoError(t, err)
	originalUpdatedAt := original.UpdatedAt

	// Small delay to ensure updated_at differs
	time.Sleep(10 * time.Millisecond)

	// Act
	newSending := false
	newReceiving := false
	returned, err := repo.Update(ctx, orgID, ledgerID, balanceID, mmodel.UpdateBalance{
		AllowSending:   &newSending,
		AllowReceiving: &newReceiving,
	})

	// Assert
	require.NoError(t, err, "Update should not return error")
	require.NotNil(t, returned, "Update should return the updated balance")

	// Verify returned balance has the updated values
	assert.Equal(t, balanceID.String(), returned.ID, "returned ID should match")
	assert.False(t, returned.AllowSending, "returned allow_sending should be updated to false")
	assert.False(t, returned.AllowReceiving, "returned allow_receiving should be updated to false")
	assert.True(t, returned.UpdatedAt.After(originalUpdatedAt), "returned updated_at should be after original")

	// Verify unchanged fields are preserved
	assert.Equal(t, original.Alias, returned.Alias, "alias should be unchanged")
	assert.Equal(t, original.AssetCode, returned.AssetCode, "asset_code should be unchanged")
	assert.True(t, original.Available.Equal(returned.Available), "available should be unchanged")
	assert.True(t, original.OnHold.Equal(returned.OnHold), "on_hold should be unchanged")
}

func TestIntegration_BalanceRepository_Update_ReturnedBalanceMatchesPersistedData(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = "@update-match-test"
	params.AllowSending = true
	params.AllowReceiving = false
	balanceID := pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params)

	ctx := context.Background()

	// Act
	newSending := false
	newReceiving := true
	returned, err := repo.Update(ctx, orgID, ledgerID, balanceID, mmodel.UpdateBalance{
		AllowSending:   &newSending,
		AllowReceiving: &newReceiving,
	})

	// Assert - returned balance should match what's actually in the database
	require.NoError(t, err)
	require.NotNil(t, returned)

	// Fetch from DB to compare
	persisted, err := repo.Find(ctx, orgID, ledgerID, balanceID)
	require.NoError(t, err)

	// All fields should match between returned and persisted
	assert.Equal(t, persisted.ID, returned.ID, "ID should match")
	assert.Equal(t, persisted.OrganizationID, returned.OrganizationID, "OrganizationID should match")
	assert.Equal(t, persisted.LedgerID, returned.LedgerID, "LedgerID should match")
	assert.Equal(t, persisted.AccountID, returned.AccountID, "AccountID should match")
	assert.Equal(t, persisted.Alias, returned.Alias, "Alias should match")
	assert.Equal(t, persisted.Key, returned.Key, "Key should match")
	assert.Equal(t, persisted.AssetCode, returned.AssetCode, "AssetCode should match")
	assert.True(t, persisted.Available.Equal(returned.Available), "Available should match")
	assert.True(t, persisted.OnHold.Equal(returned.OnHold), "OnHold should match")
	assert.Equal(t, persisted.Version, returned.Version, "Version should match")
	assert.Equal(t, persisted.AccountType, returned.AccountType, "AccountType should match")
	assert.Equal(t, persisted.AllowSending, returned.AllowSending, "AllowSending should match")
	assert.Equal(t, persisted.AllowReceiving, returned.AllowReceiving, "AllowReceiving should match")
	assert.True(t, persisted.CreatedAt.Equal(returned.CreatedAt), "CreatedAt should match")
	assert.True(t, persisted.UpdatedAt.Equal(returned.UpdatedAt), "UpdatedAt should match")
}

func TestIntegration_BalanceRepository_Update_ReturnsEntityNotFoundError(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	newSending := false
	returned, err := repo.Update(ctx, orgID, ledgerID, nonExistentID, mmodel.UpdateBalance{
		AllowSending: &newSending,
	})

	// Assert
	require.Error(t, err, "Update should return error for non-existent balance")
	assert.Nil(t, returned, "returned balance should be nil on error")

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
	assert.Equal(t, constant.ErrEntityNotFound.Error(), entityNotFoundErr.Code, "error code should be ErrEntityNotFound")
	assert.Equal(t, "Balance", entityNotFoundErr.EntityType, "entity type should be Balance")
}

func TestIntegration_BalanceRepository_Update_ChangesAllowFlags(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = "@update-test"
	params.AllowSending = true
	params.AllowReceiving = true
	balanceID := pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params)

	ctx := context.Background()

	// Get original balance to compare updated_at
	original, err := repo.Find(ctx, orgID, ledgerID, balanceID)
	require.NoError(t, err)
	originalUpdatedAt := original.UpdatedAt

	// Small delay to ensure updated_at differs
	time.Sleep(10 * time.Millisecond)

	// Act
	newSending := false
	newReceiving := false
	returned, err := repo.Update(ctx, orgID, ledgerID, balanceID, mmodel.UpdateBalance{
		AllowSending:   &newSending,
		AllowReceiving: &newReceiving,
	})

	// Assert
	require.NoError(t, err, "Update should not return error")
	require.NotNil(t, returned, "Update should return the updated balance")

	// Verify returned balance has expected values
	assert.False(t, returned.AllowSending, "returned allow_sending should be updated to false")
	assert.False(t, returned.AllowReceiving, "returned allow_receiving should be updated to false")
	assert.True(t, returned.UpdatedAt.After(originalUpdatedAt), "returned updated_at should be changed after update")

	// Also verify via Find to ensure persistence
	found, err := repo.Find(ctx, orgID, ledgerID, balanceID)
	require.NoError(t, err)
	assert.False(t, found.AllowSending, "persisted allow_sending should be updated to false")
	assert.False(t, found.AllowReceiving, "persisted allow_receiving should be updated to false")
	assert.True(t, found.UpdatedAt.After(originalUpdatedAt), "persisted updated_at should be changed after update")
}

// ============================================================================
// ListByAliases Tests
// ============================================================================

func TestIntegration_BalanceRepository_ListByAliases_ReturnsMatchingBalances(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID1 := createTestAccountID()
	accountID2 := createTestAccountID()

	// Create balances with different aliases
	params1 := pgtestutil.DefaultBalanceParams()
	params1.Alias = "@alice"
	pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID1, params1)

	params2 := pgtestutil.DefaultBalanceParams()
	params2.Alias = "@bob"
	pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID2, params2)

	// Create @charlie to verify it's excluded from results
	params3 := pgtestutil.DefaultBalanceParams()
	params3.Alias = "@charlie"
	pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID1, params3)

	ctx := context.Background()

	// Act
	balances, err := repo.ListByAliases(ctx, orgID, ledgerID, []string{"@alice", "@bob"})

	// Assert
	require.NoError(t, err, "ListByAliases should not return error")
	assert.Len(t, balances, 2, "should return only 2 balances matching requested aliases")

	aliases := make([]string, len(balances))
	for i, b := range balances {
		aliases[i] = b.Alias
	}
	assert.ElementsMatch(t, []string{"@alice", "@bob"}, aliases)
	assert.NotContains(t, aliases, "@charlie", "should not include unrequested alias")
}

func TestIntegration_BalanceRepository_ListByAliases_PreservesLargePrecision(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	// Use shared helper to create balance with large precision values
	testData := createLargePrecisionBalance(t, container, orgID, ledgerID, accountID, "@large-precision-alias")

	ctx := context.Background()

	// Act
	balances, err := repo.ListByAliases(ctx, orgID, ledgerID, []string{"@large-precision-alias"})

	// Assert
	require.NoError(t, err)
	require.Len(t, balances, 1)
	assert.True(t, balances[0].Available.Equal(testData.LargeAvail), "available should preserve large precision")
	assert.True(t, balances[0].OnHold.Equal(testData.LargeHold), "on_hold should preserve large precision")
}

func TestIntegration_BalanceRepository_ListByAliases_EmptyForNonExistentAlias(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	balances, err := repo.ListByAliases(ctx, orgID, ledgerID, []string{"@non-existent"})

	// Assert
	require.NoError(t, err, "should not error for empty result")
	assert.Empty(t, balances, "should return empty slice")
}

// ============================================================================
// FindByAccountIDAndKey Tests
// ============================================================================

func TestIntegration_BalanceRepository_FindByAccountIDAndKey_ReturnsBalance(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = "@key-test"
	params.Key = "special"
	pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params)

	ctx := context.Background()

	// Act
	balance, err := repo.FindByAccountIDAndKey(ctx, orgID, ledgerID, accountID, "special")

	// Assert
	require.NoError(t, err, "FindByAccountIDAndKey should not return error")
	require.NotNil(t, balance)
	assert.Equal(t, "@key-test", balance.Alias)
	assert.Equal(t, "special", balance.Key)
}

func TestIntegration_BalanceRepository_FindByAccountIDAndKey_ReturnsErrorForWrongKey(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = "@key-test"
	params.Key = "default"
	pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params)

	ctx := context.Background()

	// Act
	balance, err := repo.FindByAccountIDAndKey(ctx, orgID, ledgerID, accountID, "non-existent-key")

	// Assert
	require.Error(t, err, "should return error for non-existent key")
	assert.Nil(t, balance)

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
}

// ============================================================================
// Sync Tests (Redis → Postgres)
// ============================================================================

func TestIntegration_BalanceRepository_Sync_UpdatesBalanceFromRedis(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = "@sync-test"
	params.Available = decimal.NewFromInt(100)
	params.OnHold = decimal.Zero
	balanceID := pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params)

	ctx := context.Background()

	// Simulate Redis balance with higher version
	redisBalance := mmodel.BalanceRedis{
		ID:        balanceID.String(),
		Available: decimal.NewFromInt(500),
		OnHold:    decimal.NewFromInt(25),
		Version:   10,
	}

	// Act
	updated, err := repo.Sync(ctx, orgID, ledgerID, redisBalance)

	// Assert
	require.NoError(t, err, "Sync should not return error")
	assert.True(t, updated, "should indicate balance was updated")

	found, err := repo.Find(ctx, orgID, ledgerID, balanceID)
	require.NoError(t, err)
	assert.True(t, found.Available.Equal(decimal.NewFromInt(500)), "available should be synced")
	assert.True(t, found.OnHold.Equal(decimal.NewFromInt(25)), "on_hold should be synced")
	assert.Equal(t, int64(10), found.Version, "version should be synced")
}

func TestIntegration_BalanceRepository_Sync_IgnoresOlderVersion(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	// Insert balance with version 10
	balanceID := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	_, err := container.DB.Exec(`
		INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, key, asset_code, available, on_hold, version, account_type, allow_sending, allow_receiving, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, balanceID, orgID, ledgerID, accountID, "@sync-old", "default", "USD",
		decimal.NewFromInt(1000), decimal.Zero, 10, "deposit", true, true, now, now)
	require.NoError(t, err)

	ctx := context.Background()

	// Try to sync with older version
	redisBalance := mmodel.BalanceRedis{
		ID:        balanceID.String(),
		Available: decimal.NewFromInt(500),
		OnHold:    decimal.NewFromInt(25),
		Version:   5, // older than 10
	}

	// Act
	updated, err := repo.Sync(ctx, orgID, ledgerID, redisBalance)

	// Assert
	require.NoError(t, err, "Sync should not error for old version")
	assert.False(t, updated, "should indicate balance was NOT updated")

	// Verify original values unchanged
	found, err := repo.Find(ctx, orgID, ledgerID, balanceID)
	require.NoError(t, err)
	assert.True(t, found.Available.Equal(decimal.NewFromInt(1000)), "available should be unchanged")
	assert.Equal(t, int64(10), found.Version, "version should be unchanged")
}

// ============================================================================
// ListAll Tests (covers date filtering and pagination)
// ============================================================================

func TestIntegration_BalanceRepository_ListAll_ReturnsBalances(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID1 := createTestAccountID()
	accountID2 := createTestAccountID()

	// Create balances for different accounts in same ledger
	params1 := pgtestutil.DefaultBalanceParams()
	params1.Alias = "@alice"
	pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID1, params1)

	params2 := pgtestutil.DefaultBalanceParams()
	params2.Alias = "@bob"
	params2.Available = decimal.NewFromInt(2000)
	pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID2, params2)

	ctx := context.Background()

	// Act
	balances, cur, err := repo.ListAll(ctx, orgID, ledgerID, defaultPagination())

	// Assert
	require.NoError(t, err, "ListAll should not return error")
	assert.Len(t, balances, 2, "should return 2 balances")
	assert.Empty(t, cur.Next, "should not have next cursor with only 2 items")
}

func TestIntegration_BalanceRepository_ListAll_FiltersByDateRange(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	// Create a balance (created today)
	params := pgtestutil.DefaultBalanceParams()
	params.Alias = "@date-test"
	pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params)

	ctx := context.Background()

	// Act 1: Query with past-only window (should return 0 items)
	pastFilter := http.Pagination{
		Limit:     10,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(0, 0, -10),
		EndDate:   time.Now().AddDate(0, 0, -9),
	}
	balancesPast, _, err := repo.ListAll(ctx, orgID, ledgerID, pastFilter)
	require.NoError(t, err)
	assert.Empty(t, balancesPast, "past-only window should return 0 items")

	// Act 2: Query with today's window (should return 1 item)
	// Use day-based range since NormalizeDateTime normalizes to day boundaries
	todayFilter := http.Pagination{
		Limit:     10,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(0, 0, -1), // yesterday (covers timezone edge cases)
		EndDate:   time.Now().AddDate(0, 0, 1),  // tomorrow
	}
	balancesToday, _, err := repo.ListAll(ctx, orgID, ledgerID, todayFilter)
	require.NoError(t, err)
	assert.Len(t, balancesToday, 1, "today's window should return 1 item")
}

func TestIntegration_BalanceRepository_ListAll_Pagination(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create 7 balances
	for i := 0; i < 7; i++ {
		accountID := createTestAccountID()
		params := pgtestutil.DefaultBalanceParams()
		params.Alias = "@pg" + string(rune('a'+i))
		params.Available = decimal.NewFromInt(int64(i * 100))
		pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params)
	}

	ctx := context.Background()

	// Page 1: limit=3
	page1Filter := http.Pagination{
		Limit:     3,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	page1, cur1, err := repo.ListAll(ctx, orgID, ledgerID, page1Filter)

	require.NoError(t, err)
	assert.Len(t, page1, 3, "page 1 should have 3 items")
	assert.NotEmpty(t, cur1.Next, "page 1 should have next cursor")

	// Page 2: using next cursor
	page2Filter := http.Pagination{
		Limit:     3,
		SortOrder: "DESC",
		Cursor:    cur1.Next,
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	page2, cur2, err := repo.ListAll(ctx, orgID, ledgerID, page2Filter)

	require.NoError(t, err)
	assert.Len(t, page2, 3, "page 2 should have 3 items")
	assert.NotEmpty(t, cur2.Prev, "page 2 should have prev cursor")

	// Page 3: last page with 1 item
	page3Filter := http.Pagination{
		Limit:     3,
		SortOrder: "DESC",
		Cursor:    cur2.Next,
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	page3, cur3, err := repo.ListAll(ctx, orgID, ledgerID, page3Filter)

	require.NoError(t, err)
	assert.Len(t, page3, 1, "page 3 should have 1 item")
	assert.Empty(t, cur3.Next, "page 3 should not have next cursor")
	assert.NotEmpty(t, cur3.Prev, "page 3 should have prev cursor")
}

func TestIntegration_BalanceRepository_ListAll_EmptyForNonExistentLedger(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	nonExistentLedgerID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	balances, _, err := repo.ListAll(ctx, orgID, nonExistentLedgerID, defaultPagination())

	// Assert
	require.NoError(t, err, "should not error for empty result")
	assert.Empty(t, balances, "should return empty slice")
}

func TestIntegration_BalanceRepository_ListAll_PreservesLargePrecision(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	// Use shared helper to create balance with large precision values
	testData := createLargePrecisionBalance(t, container, orgID, ledgerID, accountID, "@large-precision")

	ctx := context.Background()

	// Act
	balances, _, err := repo.ListAll(ctx, orgID, ledgerID, defaultPagination())

	// Assert
	require.NoError(t, err)
	require.Len(t, balances, 1)
	assert.True(t, balances[0].Available.Equal(testData.LargeAvail), "available should preserve large precision")
	assert.True(t, balances[0].OnHold.Equal(testData.LargeHold), "on_hold should preserve large precision")
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

// largePrecisionTestData holds the expected values for large precision tests.
type largePrecisionTestData struct {
	BalanceID  uuid.UUID
	LargeAvail decimal.Decimal
	LargeHold  decimal.Decimal
}

// createLargePrecisionBalance inserts a balance with very large precision values
// for testing decimal precision preservation across different query methods.
func createLargePrecisionBalance(t *testing.T, container *pgtestutil.ContainerResult, orgID, ledgerID, accountID uuid.UUID, alias string) largePrecisionTestData {
	t.Helper()

	balanceID := libCommons.GenerateUUIDv7()
	largeAvail, _ := decimal.NewFromString("123456789012345678901234567890.123456789012345678901234567890")
	largeHold, _ := decimal.NewFromString("987654321098765432109876543210.987654321098765432109876543210")
	now := time.Now().Truncate(time.Microsecond)

	_, err := container.DB.Exec(`
		INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, key, asset_code, available, on_hold, version, account_type, allow_sending, allow_receiving, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, balanceID, orgID, ledgerID, accountID, alias, "default", "USD",
		largeAvail, largeHold, 1, "deposit", true, true, now, now)
	require.NoError(t, err, "failed to insert large precision balance")

	return largePrecisionTestData{
		BalanceID:  balanceID,
		LargeAvail: largeAvail,
		LargeHold:  largeHold,
	}
}

// ============================================================================
// Optimistic Locking Tests - Core Concurrency Mechanism
// ============================================================================

func TestIntegration_BalancesUpdate_OptimisticLock_HighestVersionWins(t *testing.T) {
	// This test verifies that when multiple goroutines try to update the same
	// balance with different versions, the highest version wins.
	// Each goroutine sets Available = version * 100, so we can verify
	// which version actually got persisted by checking the final Available value.

	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	// Create balance with version 5 directly
	balanceID := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	_, err := container.DB.Exec(`
		INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, key, asset_code, available, on_hold, version, account_type, allow_sending, allow_receiving, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, balanceID, orgID, ledgerID, accountID, "@highest-version-test", "default", "USD",
		decimal.NewFromInt(1000), decimal.Zero, 5, "deposit", true, true, now, now)
	require.NoError(t, err)

	ctx := context.Background()

	// Launch 10 goroutines with different versions (6-15)
	// Each sets Available = version * 100 to identify the winner
	const goroutines = 10
	const baseVersion = 6
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			version := int64(baseVersion + idx) // versions 6, 7, 8, ..., 15

			balance := &mmodel.Balance{
				ID:        balanceID.String(),
				Available: decimal.NewFromInt(version * 100), // 600, 700, 800, ..., 1500
				OnHold:    decimal.Zero,
				Version:   version,
			}

			_ = repo.BalancesUpdate(ctx, orgID, ledgerID, []*mmodel.Balance{balance})
		}(i)
	}

	wg.Wait()

	// Verify final state - highest version (15) should have won
	found, err := repo.Find(ctx, orgID, ledgerID, balanceID)
	require.NoError(t, err)

	const highestVersion = baseVersion + goroutines - 1           // 15
	expectedAvailable := decimal.NewFromInt(highestVersion * 100) // 1500

	assert.Equal(t, int64(highestVersion), found.Version,
		"highest version (%d) should win", highestVersion)
	assert.True(t, found.Available.Equal(expectedAvailable),
		"available should be %s (from version %d), got %s",
		expectedAvailable.String(), highestVersion, found.Available.String())

	t.Logf("Highest version wins: %d goroutines competed with versions %d-%d, winner: version=%d, Available=%s",
		goroutines, baseVersion, highestVersion, found.Version, found.Available.String())
}

func TestIntegration_BalancesUpdate_ParallelUpdates_DifferentBalances(t *testing.T) {
	// This test verifies that parallel updates to DIFFERENT balances
	// do not interfere with each other.

	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Create 10 different balances
	const numBalances = 10
	balanceIDs := make([]uuid.UUID, numBalances)
	accountIDs := make([]uuid.UUID, numBalances)

	for i := 0; i < numBalances; i++ {
		accountIDs[i] = createTestAccountID()
		params := pgtestutil.BalanceParams{
			Alias:          "@parallel-" + string(rune('a'+i)),
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.Zero,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}
		balanceIDs[i] = pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountIDs[i], params)
	}

	// Update all balances in parallel
	var wg sync.WaitGroup
	errors := make([]error, numBalances)

	for i := 0; i < numBalances; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			balance := &mmodel.Balance{
				ID:        balanceIDs[idx].String(),
				Available: decimal.NewFromInt(int64(2000 + idx*100)),
				OnHold:    decimal.NewFromInt(int64(idx * 10)),
				Version:   1,
			}

			errors[idx] = repo.BalancesUpdate(ctx, orgID, ledgerID, []*mmodel.Balance{balance})
		}(i)
	}

	wg.Wait()

	// All updates should succeed
	for i, err := range errors {
		assert.NoError(t, err, "update %d should succeed", i)
	}

	// Verify all balances were updated correctly
	for i := 0; i < numBalances; i++ {
		found, err := repo.Find(ctx, orgID, ledgerID, balanceIDs[i])
		require.NoError(t, err)
		expectedAvailable := decimal.NewFromInt(int64(2000 + i*100))
		expectedOnHold := decimal.NewFromInt(int64(i * 10))
		assert.True(t, found.Available.Equal(expectedAvailable), "balance %d available should match", i)
		assert.True(t, found.OnHold.Equal(expectedOnHold), "balance %d on_hold should match", i)
		assert.Equal(t, int64(1), found.Version, "balance %d version should be 1", i)
	}
}

func TestIntegration_BalancesUpdate_SequentialVersioning(t *testing.T) {
	// This test verifies that sequential updates with incrementing versions work correctly.

	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := createTestAccountID()

	params := pgtestutil.BalanceParams{
		Alias:          "@sequential-test",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}
	balanceID := pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params)

	ctx := context.Background()

	// Perform 5 sequential updates with incrementing versions
	for v := int64(1); v <= 5; v++ {
		balance := &mmodel.Balance{
			ID:        balanceID.String(),
			Available: decimal.NewFromInt(1000 - v*100),
			OnHold:    decimal.NewFromInt(v * 10),
			Version:   v,
		}

		err := repo.BalancesUpdate(ctx, orgID, ledgerID, []*mmodel.Balance{balance})
		require.NoError(t, err, "update with version %d should succeed", v)

		// Verify after each update
		found, err := repo.Find(ctx, orgID, ledgerID, balanceID)
		require.NoError(t, err)
		assert.Equal(t, v, found.Version, "version should be %d", v)
	}

	// Final state
	found, err := repo.Find(ctx, orgID, ledgerID, balanceID)
	require.NoError(t, err)
	assert.True(t, found.Available.Equal(decimal.NewFromInt(500)), "final available should be 500")
	assert.True(t, found.OnHold.Equal(decimal.NewFromInt(50)), "final on_hold should be 50")
	assert.Equal(t, int64(5), found.Version, "final version should be 5")
}

// ============================================================================
// BalancesUpdate Edge Cases
// ============================================================================

func TestIntegration_BalancesUpdate_EmptySlice_NoError(t *testing.T) {
	// Verify that updating with an empty slice doesn't cause errors

	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	err := repo.BalancesUpdate(ctx, orgID, ledgerID, []*mmodel.Balance{})
	assert.NoError(t, err, "empty slice should not cause error")
}

func TestIntegration_BalancesUpdate_BatchUpdate_AllSucceed(t *testing.T) {
	// Test batch update of multiple balances in a single call

	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Create 5 balances
	const numBalances = 5
	balanceIDs := make([]uuid.UUID, numBalances)

	for i := 0; i < numBalances; i++ {
		accountID := createTestAccountID()
		params := pgtestutil.BalanceParams{
			Alias:          "@batch-" + string(rune('a'+i)),
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.Zero,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}
		balanceIDs[i] = pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, params)
	}

	// Update all in a single batch
	balancesToUpdate := make([]*mmodel.Balance, numBalances)
	for i := 0; i < numBalances; i++ {
		balancesToUpdate[i] = &mmodel.Balance{
			ID:        balanceIDs[i].String(),
			Available: decimal.NewFromInt(int64(500 + i*100)),
			OnHold:    decimal.NewFromInt(int64(i * 10)),
			Version:   1,
		}
	}

	err := repo.BalancesUpdate(ctx, orgID, ledgerID, balancesToUpdate)
	require.NoError(t, err, "batch update should succeed")

	// Verify all were updated
	for i := 0; i < numBalances; i++ {
		found, err := repo.Find(ctx, orgID, ledgerID, balanceIDs[i])
		require.NoError(t, err)
		expectedAvailable := decimal.NewFromInt(int64(500 + i*100))
		assert.True(t, found.Available.Equal(expectedAvailable), "balance %d should be updated", i)
		assert.Equal(t, int64(1), found.Version, "balance %d version should be 1", i)
	}
}
