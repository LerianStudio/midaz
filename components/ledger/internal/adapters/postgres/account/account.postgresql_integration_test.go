//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libPointers "github.com/LerianStudio/lib-commons/v4/commons/pointers"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createRepository creates an AccountRepository connected to the test database.
// Uses MigrationsPath to point directly to migrations, avoiding directory changes.
func createRepository(t *testing.T, container *pgtestutil.ContainerResult) *AccountPostgreSQLRepository {
	t.Helper()

	migrationsPath := pgtestutil.FindMigrationsPath(t, "onboarding")

	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)

	conn := pgtestutil.CreatePostgresClient(t, connStr, connStr, container.Config.DBName, migrationsPath)

	return NewAccountPostgreSQLRepository(conn)
}

// ============================================================================
// Find Tests
// ============================================================================

func TestIntegration_AccountRepository_Find_ReturnsAccount(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	// Create repository - lib-commons auto-runs migrations via MigrationsPath
	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Insert test account directly
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	alias := fmt.Sprintf("@test-%s", uuid.Must(libCommons.GenerateUUIDv7()).String()[:8])
	now := time.Now().Truncate(time.Microsecond)

	_, err := container.DB.Exec(`
		INSERT INTO account (id, name, asset_code, organization_id, ledger_id, status, alias, type, blocked, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, accountID, "Test Account", "USD", orgID, ledgerID, "ACTIVE", alias, "deposit", false, now, now)
	require.NoError(t, err, "failed to insert test account")

	ctx := context.Background()

	// Act
	account, err := repo.Find(ctx, orgID, ledgerID, nil, accountID)

	// Assert
	require.NoError(t, err, "Find should not return error for existing account")
	require.NotNil(t, account, "account should not be nil")

	assert.Equal(t, accountID.String(), account.ID, "account ID should match")
	assert.Equal(t, "Test Account", account.Name, "account name should match")
	assert.Equal(t, "USD", account.AssetCode, "asset code should match")
	assert.Equal(t, orgID.String(), account.OrganizationID, "organization ID should match")
	assert.Equal(t, ledgerID.String(), account.LedgerID, "ledger ID should match")
	assert.Equal(t, "ACTIVE", account.Status.Code, "status should match")
	assert.Equal(t, alias, *account.Alias, "alias should match")
	assert.Equal(t, "deposit", account.Type, "type should match")
	assert.NotNil(t, account.Blocked, "blocked should not be nil")
	assert.False(t, *account.Blocked, "blocked should be false")
}

func TestIntegration_AccountRepository_Find_ReturnsEntityNotFoundError(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Setup: create soft-deleted account for one test case
	softDeletedID := uuid.Must(libCommons.GenerateUUIDv7())
	alias := fmt.Sprintf("@deleted-%s", uuid.Must(libCommons.GenerateUUIDv7()).String()[:8])
	now := time.Now().Truncate(time.Microsecond)

	_, err := container.DB.Exec(`
		INSERT INTO account (id, name, asset_code, organization_id, ledger_id, status, alias, type, blocked, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, softDeletedID, "Deleted Account", "USD", orgID, ledgerID, "DELETED", alias, "deposit", false, now, now, now)
	require.NoError(t, err, "failed to insert soft-deleted account")

	tests := []struct {
		name      string
		accountID uuid.UUID
	}{
		{
			name:      "non-existent account",
			accountID: uuid.Must(libCommons.GenerateUUIDv7()),
		},
		{
			name:      "soft-deleted account",
			accountID: softDeletedID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			account, err := repo.Find(context.Background(), orgID, ledgerID, nil, tt.accountID)

			// Assert
			require.Error(t, err, "Find should return error")
			assert.Nil(t, account, "account should be nil")

			// Validate error type: sql.ErrNoRows → pkg.EntityNotFoundError
			var entityNotFoundErr pkg.EntityNotFoundError
			require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")

			// Validate error code matches constant.ErrEntityNotFound ("0007")
			assert.Equal(t, constant.ErrEntityNotFound.Error(), entityNotFoundErr.Code, "error code should be ErrEntityNotFound")

			// Validate entity type is Account
			assert.Equal(t, "Account", entityNotFoundErr.EntityType, "entity type should be Account")
		})
	}
}

func TestIntegration_AccountRepository_Find_FiltersCorrectlyByOrgAndLedger(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	// Create two organizations with ledgers
	org1ID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledger1ID := pgtestutil.CreateTestLedger(t, container.DB, org1ID)

	org2ID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledger2ID := pgtestutil.CreateTestLedger(t, container.DB, org2ID)

	// Insert account in org1/ledger1
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	alias := fmt.Sprintf("@org1-%s", uuid.Must(libCommons.GenerateUUIDv7()).String()[:8])
	now := time.Now().Truncate(time.Microsecond)

	_, err := container.DB.Exec(`
		INSERT INTO account (id, name, asset_code, organization_id, ledger_id, status, alias, type, blocked, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, accountID, "Org1 Account", "USD", org1ID, ledger1ID, "ACTIVE", alias, "deposit", false, now, now)
	require.NoError(t, err, "failed to insert account")

	ctx := context.Background()

	// Act & Assert: should find with correct org/ledger
	account, err := repo.Find(ctx, org1ID, ledger1ID, nil, accountID)
	require.NoError(t, err, "should find account with correct org/ledger")
	assert.NotNil(t, account)

	// Act & Assert: should NOT find with wrong organization
	account, err = repo.Find(ctx, org2ID, ledger1ID, nil, accountID)
	require.Error(t, err, "should not find account with wrong organization")
	assert.Nil(t, account)

	// Act & Assert: should NOT find with wrong ledger
	account, err = repo.Find(ctx, org1ID, ledger2ID, nil, accountID)
	require.Error(t, err, "should not find account with wrong ledger")
	assert.Nil(t, account)
}

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_AccountRepository_Create_InsertsAccount(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	ctx := context.Background()

	alias := fmt.Sprintf("@new-%s", uuid.Must(libCommons.GenerateUUIDv7()).String()[:8])
	blocked := false
	now := time.Now().Truncate(time.Microsecond)

	newAccount := &mmodel.Account{
		Name:           "New Account",
		AssetCode:      "BRL",
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Status:         mmodel.Status{Code: "ACTIVE"},
		Alias:          &alias,
		Type:           "deposit",
		Blocked:        &blocked,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Act
	created, err := repo.Create(ctx, newAccount)

	// Assert
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, created, "created account should not be nil")

	assert.NotEmpty(t, created.ID, "created account should have an ID")
	assert.Equal(t, "New Account", created.Name)
	assert.Equal(t, "BRL", created.AssetCode)
	assert.Equal(t, alias, *created.Alias)

	// Verify it can be retrieved
	parsedID, _ := uuid.Parse(created.ID)
	found, err := repo.Find(ctx, orgID, ledgerID, nil, parsedID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
}

// ============================================================================
// Backward Compatibility Tests
// ============================================================================

// TestIntegration_AccountRepository_Find_BackwardCompatible_ExtraColumns validates that
// the repository Find operation doesn't break when the database schema has extra columns
// that the application code doesn't know about.
//
// This is critical for:
// - Rolling deployments: First pod runs migration (adds column), other pods still run old code
// - Rollbacks: Migration adds column, then rollback to old app version that doesn't expect it
//
// The repository must handle unknown columns gracefully (SELECT specific columns, not SELECT *).
func TestIntegration_AccountRepository_Find_BackwardCompatible_ExtraColumns(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Simulate a future migration that adds a new column the current code doesn't know about
	_, err := container.DB.Exec(`ALTER TABLE account ADD COLUMN future_feature TEXT`)
	require.NoError(t, err, "failed to add future column")

	// Also add a NOT NULL column with default to simulate more aggressive schema changes
	_, err = container.DB.Exec(`ALTER TABLE account ADD COLUMN another_future_column BOOLEAN NOT NULL DEFAULT false`)
	require.NoError(t, err, "failed to add another future column")

	// Insert account with the extra columns populated
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	alias := fmt.Sprintf("@compat-%s", uuid.Must(libCommons.GenerateUUIDv7()).String()[:8])
	now := time.Now().Truncate(time.Microsecond)

	_, err = container.DB.Exec(`
		INSERT INTO account (id, name, asset_code, organization_id, ledger_id, status, alias, type, blocked, created_at, updated_at, future_feature, another_future_column)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, accountID, "Compat Account", "USD", orgID, ledgerID, "ACTIVE", alias, "deposit", false, now, now, "some future value", true)
	require.NoError(t, err, "failed to insert account with extra columns")

	ctx := context.Background()

	// Act - The old code (current repository) tries to read the account
	account, err := repo.Find(ctx, orgID, ledgerID, nil, accountID)

	// Assert - Must NOT break, even with unknown columns in the table
	require.NoError(t, err, "Find must not break when table has extra columns (backward compatibility)")
	require.NotNil(t, account, "account should be returned despite extra columns")

	// Verify all known fields are correctly read
	assert.Equal(t, accountID.String(), account.ID)
	assert.Equal(t, "Compat Account", account.Name)
	assert.Equal(t, "USD", account.AssetCode)
	assert.Equal(t, alias, *account.Alias)
	assert.Equal(t, "ACTIVE", account.Status.Code)
}

// TestIntegration_AccountRepository_Create_BackwardCompatible_ExtraColumns validates that
// the repository Create operation doesn't break when the database has extra columns.
//
// This ensures INSERT statements explicitly list columns and don't fail due to missing
// values for columns the application doesn't know about.
func TestIntegration_AccountRepository_Create_BackwardCompatible_ExtraColumns(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Simulate future migration: add column with DEFAULT (common pattern)
	_, err := container.DB.Exec(`ALTER TABLE account ADD COLUMN future_feature TEXT DEFAULT 'default_value'`)
	require.NoError(t, err, "failed to add future column with default")

	// Add a NOT NULL column with default - the hardest case for backward compatibility
	_, err = container.DB.Exec(`ALTER TABLE account ADD COLUMN required_future_field VARCHAR(100) NOT NULL DEFAULT 'required_default'`)
	require.NoError(t, err, "failed to add required future column")

	ctx := context.Background()

	alias := fmt.Sprintf("@new-compat-%s", uuid.Must(libCommons.GenerateUUIDv7()).String()[:8])
	blocked := false
	now := time.Now().Truncate(time.Microsecond)

	newAccount := &mmodel.Account{
		Name:           "New Account During Migration",
		AssetCode:      "EUR",
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Status:         mmodel.Status{Code: "ACTIVE"},
		Alias:          &alias,
		Type:           "deposit",
		Blocked:        &blocked,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Act - Old code creates account while new columns exist
	created, err := repo.Create(ctx, newAccount)

	// Assert - Must NOT break
	require.NoError(t, err, "Create must not break when table has extra columns (backward compatibility)")
	require.NotNil(t, created, "created account should not be nil")

	// Verify the account was actually persisted
	parsedID, _ := uuid.Parse(created.ID)
	found, err := repo.Find(ctx, orgID, ledgerID, nil, parsedID)
	require.NoError(t, err, "should be able to retrieve the created account")
	assert.Equal(t, created.ID, found.ID)

	// Verify the extra columns got their default values (checking via raw SQL)
	var futureFeature, requiredField string
	err = container.DB.QueryRow(`SELECT future_feature, required_future_field FROM account WHERE id = $1`, parsedID).
		Scan(&futureFeature, &requiredField)
	require.NoError(t, err, "should be able to query extra columns")
	assert.Equal(t, "default_value", futureFeature, "future column should have default value")
	assert.Equal(t, "required_default", requiredField, "required column should have default value")
}

// ============================================================================
// FindAll Tests
// ============================================================================

func TestIntegration_AccountRepository_FindAll_ReturnsPaginatedAccounts(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Insert 5 accounts
	for i := 0; i < 5; i++ {
		alias := fmt.Sprintf("@findall-%d-%s", i, uuid.Must(libCommons.GenerateUUIDv7()).String()[:8])
		pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, fmt.Sprintf("Account %d", i), alias, "USD", nil)
	}

	ctx := context.Background()
	filter := http.Pagination{
		Limit:     3,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
	}

	// Act
	accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, filter)

	// Assert
	require.NoError(t, err, "FindAll should not return error")
	assert.Len(t, accounts, 3, "should return exactly 3 accounts (limit)")

	// Verify all returned accounts belong to correct org/ledger
	for _, acc := range accounts {
		assert.Equal(t, orgID.String(), acc.OrganizationID)
		assert.Equal(t, ledgerID.String(), acc.LedgerID)
	}
}

func TestIntegration_AccountRepository_FindAll_PaginatesWithoutDuplicates(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Insert 5 accounts
	for i := 0; i < 5; i++ {
		alias := fmt.Sprintf("@paginate-%d-%s", i, uuid.Must(libCommons.GenerateUUIDv7()).String()[:8])
		pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, fmt.Sprintf("Paginate Account %d", i), alias, "USD", nil)
	}

	ctx := context.Background()
	baseFilter := http.Pagination{
		Limit:     2,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
	}

	// Act - Get all 3 pages
	baseFilter.Page = 1
	page1, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, baseFilter)
	require.NoError(t, err)

	baseFilter.Page = 2
	page2, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, baseFilter)
	require.NoError(t, err)

	baseFilter.Page = 3
	page3, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, baseFilter)
	require.NoError(t, err)

	// Assert - Correct counts per page
	assert.Len(t, page1, 2, "page 1 should have 2 accounts")
	assert.Len(t, page2, 2, "page 2 should have 2 accounts")
	assert.Len(t, page3, 1, "page 3 should have 1 account")

	// Assert - No duplicates across pages
	seen := make(map[string]int)
	allAccounts := append(append(page1, page2...), page3...)
	for _, acc := range allAccounts {
		seen[acc.ID]++
	}

	for id, count := range seen {
		assert.Equal(t, 1, count, "account %s should appear exactly once across all pages", id)
	}
	assert.Len(t, seen, 5, "should have 5 unique accounts total")
}

func TestIntegration_AccountRepository_FindAll_ExcludesSoftDeleted(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Insert 2 active + 1 deleted
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Active 1", "@active1", "USD", nil)
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Active 2", "@active2", "USD", nil)
	deletedAt := time.Now()
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Deleted", "@deleted", "USD", &deletedAt)

	ctx := context.Background()
	filter := http.Pagination{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
	}

	// Act
	accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 2, "should only return active accounts")

	for _, acc := range accounts {
		assert.NotEqual(t, "Deleted", acc.Name)
	}
}

func TestIntegration_AccountRepository_FindAll_FiltersByPortfolio(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	portfolioID := pgtestutil.CreateTestPortfolio(t, container.DB, orgID, ledgerID)

	// Insert 2 in portfolio, 1 without
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, &portfolioID, "In Portfolio 1", "@inportfolio1", "USD", nil)
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, &portfolioID, "In Portfolio 2", "@inportfolio2", "USD", nil)
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "No Portfolio", "@noportfolio", "USD", nil)

	ctx := context.Background()
	filter := http.Pagination{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
	}

	// Act
	accounts, err := repo.FindAll(ctx, orgID, ledgerID, &portfolioID, nil, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 2, "should only return accounts in the portfolio")

	for _, acc := range accounts {
		assert.NotNil(t, acc.PortfolioID)
		assert.Equal(t, portfolioID.String(), *acc.PortfolioID)
	}
}

func TestIntegration_AccountRepository_FindAll_FiltersBySegment(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	segmentID := pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, pgtestutil.DefaultSegmentParams())

	// Insert 2 accounts with segment, 1 without.
	acc1 := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "In Segment 1", "@insegment1", "USD", nil)
	acc2 := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "In Segment 2", "@insegment2", "USD", nil)
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "No Segment", "@nosegment", "USD", nil)

	// Assign segment_id directly via SQL since CreateTestAccount doesn't support it.
	_, err := container.DB.Exec("UPDATE account SET segment_id = $1 WHERE id IN ($2, $3)", segmentID, acc1, acc2)
	require.NoError(t, err, "failed to assign segment_id to test accounts")

	ctx := context.Background()
	filter := http.Pagination{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
	}

	// Act
	accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, &segmentID, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 2, "should only return accounts in the segment")

	for _, acc := range accounts {
		assert.NotNil(t, acc.SegmentID)
		assert.Equal(t, segmentID.String(), *acc.SegmentID)
	}
}

// ============================================================================
// FindWithDeleted Tests
// ============================================================================

func TestIntegration_AccountRepository_FindWithDeleted_ReturnsDeletedAccount(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Insert soft-deleted account
	deletedAt := time.Now()
	accountID := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Deleted Account", "@deleted", "USD", &deletedAt)

	ctx := context.Background()

	// Act - Find should fail
	_, errFind := repo.Find(ctx, orgID, ledgerID, nil, accountID)
	require.Error(t, errFind, "Find should not return soft-deleted account")

	// Act - FindWithDeleted should succeed
	account, err := repo.FindWithDeleted(ctx, orgID, ledgerID, nil, accountID)

	// Assert
	require.NoError(t, err, "FindWithDeleted should return soft-deleted account")
	require.NotNil(t, account)
	assert.Equal(t, accountID.String(), account.ID)
	assert.Equal(t, "Deleted Account", account.Name)
	assert.NotNil(t, account.DeletedAt, "deleted_at should be set")
}

func TestIntegration_AccountRepository_FindWithDeleted_ReturnsActiveAccount(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	accountID := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Active Account", "@active", "USD", nil)

	ctx := context.Background()

	// Act
	account, err := repo.FindWithDeleted(ctx, orgID, ledgerID, nil, accountID)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, account)
	assert.Equal(t, accountID.String(), account.ID)
	assert.Nil(t, account.DeletedAt)
}

// ============================================================================
// FindAlias Tests
// ============================================================================

func TestIntegration_AccountRepository_FindAlias_ReturnsAccountByAlias(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	alias := "@treasury-main"
	accountID := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Treasury Account", alias, "USD", nil)

	ctx := context.Background()

	// Act
	account, err := repo.FindAlias(ctx, orgID, ledgerID, nil, alias)

	// Assert
	require.NoError(t, err, "FindAlias should find account by alias")
	require.NotNil(t, account)
	assert.Equal(t, accountID.String(), account.ID)
	assert.Equal(t, alias, *account.Alias)
	assert.Equal(t, "Treasury Account", account.Name)
}

func TestIntegration_AccountRepository_FindAlias_ReturnsErrorForNonExistent(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	ctx := context.Background()

	// Act
	account, err := repo.FindAlias(ctx, orgID, ledgerID, nil, "@nonexistent")

	// Assert
	require.Error(t, err, "FindAlias should return error for non-existent alias")
	assert.Nil(t, account)
}

func TestIntegration_AccountRepository_FindAlias_ExcludesSoftDeleted(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	alias := "@deleted-alias"
	deletedAt := time.Now()
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Deleted Account", alias, "USD", &deletedAt)

	ctx := context.Background()

	// Act
	account, err := repo.FindAlias(ctx, orgID, ledgerID, nil, alias)

	// Assert
	require.Error(t, err, "FindAlias should not find soft-deleted account")
	assert.Nil(t, account)
}

// ============================================================================
// FindByAlias Tests
// ============================================================================

func TestIntegration_AccountRepository_FindByAlias_ReturnsTrueIfExists(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	alias := "@existing-alias"
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Existing Account", alias, "USD", nil)

	ctx := context.Background()

	// Act
	exists, err := repo.FindByAlias(ctx, orgID, ledgerID, alias)

	// Assert - exists=true means alias is taken, returns ErrAliasUnavailability
	assert.True(t, exists, "should return true for existing alias")
	assert.Error(t, err, "should return error indicating alias is unavailable")
}

func TestIntegration_AccountRepository_FindByAlias_ReturnsFalseIfNotExists(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	ctx := context.Background()

	// Act
	exists, err := repo.FindByAlias(ctx, orgID, ledgerID, "@nonexistent-alias")

	// Assert
	assert.False(t, exists, "should return false for non-existent alias")
	assert.NoError(t, err, "should not return error when alias is available")
}

func TestIntegration_AccountRepository_FindByAlias_IgnoresSoftDeleted(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	alias := "@reusable-alias"
	deletedAt := time.Now()
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Deleted Account", alias, "USD", &deletedAt)

	ctx := context.Background()

	// Act - alias should be available since original is soft-deleted
	exists, err := repo.FindByAlias(ctx, orgID, ledgerID, alias)

	// Assert
	assert.False(t, exists, "soft-deleted alias should be available for reuse")
	assert.NoError(t, err)
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_AccountRepository_Update_UpdatesName(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	accountID := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Original Name", "@update-name", "USD", nil)

	ctx := context.Background()

	updateData := &mmodel.Account{
		Name: "Updated Name",
	}

	// Act
	updated, err := repo.Update(ctx, orgID, ledgerID, nil, accountID, updateData)

	// Assert
	require.NoError(t, err, "Update should not return error")
	require.NotNil(t, updated)

	// Verify via Find
	found, err := repo.Find(ctx, orgID, ledgerID, nil, accountID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", found.Name)
}

func TestIntegration_AccountRepository_Update_UpdatesStatus(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	accountID := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Status Account", "@update-status", "USD", nil)

	ctx := context.Background()

	updateData := &mmodel.Account{
		Status: mmodel.Status{
			Code:        "BLOCKED",
			Description: libPointers.String("Account blocked for review"),
		},
	}

	// Act
	_, err := repo.Update(ctx, orgID, ledgerID, nil, accountID, updateData)

	// Assert
	require.NoError(t, err)

	found, err := repo.Find(ctx, orgID, ledgerID, nil, accountID)
	require.NoError(t, err)
	assert.Equal(t, "BLOCKED", found.Status.Code)
	assert.NotNil(t, found.Status.Description)
	assert.Equal(t, "Account blocked for review", *found.Status.Description)
}

func TestIntegration_AccountRepository_Update_UpdatesBlocked(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	accountID := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Blocked Account", "@update-blocked", "USD", nil)

	ctx := context.Background()

	blocked := true
	updateData := &mmodel.Account{
		Blocked: &blocked,
	}

	// Act
	_, err := repo.Update(ctx, orgID, ledgerID, nil, accountID, updateData)

	// Assert
	require.NoError(t, err)

	found, err := repo.Find(ctx, orgID, ledgerID, nil, accountID)
	require.NoError(t, err)
	require.NotNil(t, found.Blocked)
	assert.True(t, *found.Blocked)
}

func TestIntegration_AccountRepository_Update_ReturnsErrorForNonExistent(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	ctx := context.Background()
	nonExistentID := uuid.Must(libCommons.GenerateUUIDv7())

	updateData := &mmodel.Account{Name: "Updated"}

	// Act
	_, err := repo.Update(ctx, orgID, ledgerID, nil, nonExistentID, updateData)

	// Assert
	require.Error(t, err, "Update should return error for non-existent account")
}

func TestIntegration_AccountRepository_Update_CannotUpdateSoftDeleted(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	deletedAt := time.Now()
	accountID := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Deleted", "@deleted-update", "USD", &deletedAt)

	ctx := context.Background()

	updateData := &mmodel.Account{Name: "Should Fail"}

	// Act
	_, err := repo.Update(ctx, orgID, ledgerID, nil, accountID, updateData)

	// Assert
	require.Error(t, err, "Update should return error for soft-deleted account")
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestIntegration_AccountRepository_Delete_SoftDeletesAccount(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	accountID := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "To Delete", "@todelete", "USD", nil)

	ctx := context.Background()

	// Verify account exists before delete
	_, err := repo.Find(ctx, orgID, ledgerID, nil, accountID)
	require.NoError(t, err, "account should exist before delete")

	// Act
	err = repo.Delete(ctx, orgID, ledgerID, nil, accountID)

	// Assert
	require.NoError(t, err, "Delete should not return error")

	// Find should fail now
	_, err = repo.Find(ctx, orgID, ledgerID, nil, accountID)
	require.Error(t, err, "Find should not return soft-deleted account")

	// FindWithDeleted should still work
	found, err := repo.FindWithDeleted(ctx, orgID, ledgerID, nil, accountID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.NotNil(t, found.DeletedAt, "deleted_at should be set")

	// Verify via raw SQL: record still exists in database, only deleted_at is set
	var (
		dbName      string
		dbDeletedAt *time.Time
	)
	err = container.DB.QueryRow(`
		SELECT name, deleted_at 
		FROM account 
		WHERE id = $1
	`, accountID).Scan(&dbName, &dbDeletedAt)

	require.NoError(t, err, "record should still exist in database after soft delete")
	assert.Equal(t, "To Delete", dbName, "record data should be preserved")
	assert.NotNil(t, dbDeletedAt, "deleted_at should be set in database")
}

func TestIntegration_AccountRepository_Delete_IsIdempotent(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	accountID := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Double Delete", "@doubledelete", "USD", nil)

	ctx := context.Background()

	// Act - Delete twice
	err1 := repo.Delete(ctx, orgID, ledgerID, nil, accountID)
	err2 := repo.Delete(ctx, orgID, ledgerID, nil, accountID)

	// Assert - First delete succeeds
	require.NoError(t, err1, "first delete should succeed")

	// Second delete should also succeed (idempotent) since the WHERE clause
	// includes deleted_at IS NULL, it simply matches 0 rows on second call
	require.NoError(t, err2, "second delete should be idempotent (no error)")

	// Verify account is still soft-deleted (only once)
	found, err := repo.FindWithDeleted(ctx, orgID, ledgerID, nil, accountID)
	require.NoError(t, err)
	assert.NotNil(t, found.DeletedAt, "account should remain soft-deleted")
}

func TestIntegration_AccountRepository_Delete_RespectsOrgLedgerIsolation(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	org1ID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledger1ID := pgtestutil.CreateTestLedger(t, container.DB, org1ID)

	org2ID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledger2ID := pgtestutil.CreateTestLedger(t, container.DB, org2ID)

	accountID := pgtestutil.CreateTestAccount(t, container.DB, org1ID, ledger1ID, nil, "Org1 Account", "@org1account", "USD", nil)

	ctx := context.Background()

	// Act - Try to delete with wrong org/ledger
	err := repo.Delete(ctx, org2ID, ledger2ID, nil, accountID)

	// Assert - Delete with wrong org/ledger should succeed (no-op, 0 rows affected)
	// The repository doesn't error on 0 rows for delete operations
	require.NoError(t, err, "delete with wrong org/ledger should not error (no-op)")

	// Critical assertion: Account must still exist in org1 (isolation preserved)
	found, err := repo.Find(ctx, org1ID, ledger1ID, nil, accountID)
	require.NoError(t, err, "account should still exist in original org/ledger")
	require.NotNil(t, found, "account should not be nil")
	assert.Equal(t, accountID.String(), found.ID, "account ID should match")
	assert.Nil(t, found.DeletedAt, "account should NOT be soft-deleted")
}

// ============================================================================
// ListByIDs Tests
// ============================================================================

func TestIntegration_AccountRepository_ListByIDs_ReturnsMatchingAccounts(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	id1 := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Account 1", "@listbyids1", "USD", nil)
	id2 := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Account 2", "@listbyids2", "USD", nil)
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Account 3", "@listbyids3", "USD", nil) // Not in query

	ctx := context.Background()

	// Act
	accounts, err := repo.ListByIDs(ctx, orgID, ledgerID, nil, nil, []uuid.UUID{id1, id2})

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 2, "should return exactly 2 accounts")

	ids := make(map[string]bool)
	for _, acc := range accounts {
		ids[acc.ID] = true
	}
	assert.True(t, ids[id1.String()])
	assert.True(t, ids[id2.String()])
}

func TestIntegration_AccountRepository_ListByIDs_FiltersBySegment(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	segmentID := pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, pgtestutil.DefaultSegmentParams())

	// Create 3 accounts, assign segment to 2 of them.
	id1 := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "In Segment 1", "@listsg1", "USD", nil)
	id2 := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "In Segment 2", "@listsg2", "USD", nil)
	id3 := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "No Segment", "@listsg3", "USD", nil)

	_, err := container.DB.Exec("UPDATE account SET segment_id = $1 WHERE id IN ($2, $3)", segmentID, id1, id2)
	require.NoError(t, err, "failed to assign segment_id to test accounts")

	ctx := context.Background()

	// Act: ListByIDs with all 3 IDs but segment filter should exclude id3.
	accounts, err := repo.ListByIDs(ctx, orgID, ledgerID, nil, &segmentID, []uuid.UUID{id1, id2, id3})

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 2, "should only return accounts matching the segment")

	for _, acc := range accounts {
		assert.NotNil(t, acc.SegmentID)
		assert.Equal(t, segmentID.String(), *acc.SegmentID)
	}
}

func TestIntegration_AccountRepository_ListByIDs_ExcludesSoftDeleted(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	id1 := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Active", "@listactive", "USD", nil)
	deletedAt := time.Now()
	id2 := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Deleted", "@listdeleted", "USD", &deletedAt)

	ctx := context.Background()

	// Act
	accounts, err := repo.ListByIDs(ctx, orgID, ledgerID, nil, nil, []uuid.UUID{id1, id2})

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 1, "should only return active account")
	assert.Equal(t, id1.String(), accounts[0].ID)
}

func TestIntegration_AccountRepository_ListByIDs_ReturnsEmptyForNoMatch(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	ctx := context.Background()

	// Act
	accounts, err := repo.ListByIDs(ctx, orgID, ledgerID, nil, nil, []uuid.UUID{uuid.Must(libCommons.GenerateUUIDv7())})

	// Assert
	require.NoError(t, err)
	assert.Empty(t, accounts, "should return empty slice for non-matching IDs")
}

// ============================================================================
// ListAccountsByAlias Tests
// ============================================================================

func TestIntegration_AccountRepository_ListAccountsByAlias_ReturnsMatchingAccounts(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	alias1, alias2, alias3 := "@byalias1", "@byalias2", "@byalias3"
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Account 1", alias1, "USD", nil)
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Account 2", alias2, "USD", nil)
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Account 3", alias3, "USD", nil)

	ctx := context.Background()

	// Act
	accounts, err := repo.ListAccountsByAlias(ctx, orgID, ledgerID, []string{alias1, alias2})

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 2, "should return exactly 2 accounts")

	aliases := make(map[string]bool)
	for _, acc := range accounts {
		if acc.Alias != nil {
			aliases[*acc.Alias] = true
		}
	}
	assert.True(t, aliases[alias1])
	assert.True(t, aliases[alias2])
}

func TestIntegration_AccountRepository_ListAccountsByAlias_ExcludesSoftDeleted(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	alias1, alias2 := "@aliasactive", "@aliasdeleted"
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Active", alias1, "USD", nil)
	deletedAt := time.Now()
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Deleted", alias2, "USD", &deletedAt)

	ctx := context.Background()

	// Act
	accounts, err := repo.ListAccountsByAlias(ctx, orgID, ledgerID, []string{alias1, alias2})

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 1, "should only return active account")
	assert.Equal(t, alias1, *accounts[0].Alias)
}

// ============================================================================
// Count Tests
// ============================================================================

func TestIntegration_AccountRepository_Count_Scenarios(t *testing.T) {
	tests := []struct {
		name          string
		activeCount   int
		deletedCount  int
		expectedCount int64
	}{
		{
			name:          "returns zero for empty ledger",
			activeCount:   0,
			deletedCount:  0,
			expectedCount: 0,
		},
		{
			name:          "returns correct count for active accounts",
			activeCount:   4,
			deletedCount:  0,
			expectedCount: 4,
		},
		{
			name:          "excludes soft-deleted accounts",
			activeCount:   3,
			deletedCount:  2,
			expectedCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			container := pgtestutil.SetupContainer(t)

			repo := createRepository(t, container)

			orgID := pgtestutil.CreateTestOrganization(t, container.DB)
			ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

			// Insert active accounts
			for i := 0; i < tt.activeCount; i++ {
				alias := fmt.Sprintf("@active%d", i)
				pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, fmt.Sprintf("Active %d", i), alias, "USD", nil)
			}

			// Insert soft-deleted accounts
			if tt.deletedCount > 0 {
				deletedAt := time.Now()
				for i := 0; i < tt.deletedCount; i++ {
					alias := fmt.Sprintf("@deleted%d", i)
					pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, fmt.Sprintf("Deleted %d", i), alias, "USD", &deletedAt)
				}
			}

			// Act
			count, err := repo.Count(context.Background(), orgID, ledgerID)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCount, count)
		})
	}
}

func TestIntegration_AccountRepository_Count_IsolatesByOrgLedger(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	org1ID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledger1ID := pgtestutil.CreateTestLedger(t, container.DB, org1ID)

	org2ID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledger2ID := pgtestutil.CreateTestLedger(t, container.DB, org2ID)

	// Insert 3 in org1, 1 in org2
	for i := 0; i < 3; i++ {
		alias := fmt.Sprintf("@org1count%d", i)
		pgtestutil.CreateTestAccount(t, container.DB, org1ID, ledger1ID, nil, fmt.Sprintf("Org1 Acc %d", i), alias, "USD", nil)
	}
	pgtestutil.CreateTestAccount(t, container.DB, org2ID, ledger2ID, nil, "Org2 Acc", "@org2count", "USD", nil)

	ctx := context.Background()

	// Act
	count1, err1 := repo.Count(ctx, org1ID, ledger1ID)
	count2, err2 := repo.Count(ctx, org2ID, ledger2ID)

	// Assert
	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.Equal(t, int64(3), count1, "org1 should have 3 accounts")
	assert.Equal(t, int64(1), count2, "org2 should have 1 account")
}

// ============================================================================
// FindAll Filter Tests (Phase 1 - Account Filters)
// ============================================================================

func TestIntegration_AccountRepository_FindAll_FiltersByStatus(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create accounts with different statuses
	activeParams := pgtestutil.DefaultAccountParams()
	activeParams.Name = "Active Account 1"
	activeParams.Alias = "@active1"
	activeParams.Status = "ACTIVE"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, activeParams)

	activeParams2 := pgtestutil.DefaultAccountParams()
	activeParams2.Name = "Active Account 2"
	activeParams2.Alias = "@active2"
	activeParams2.Status = "ACTIVE"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, activeParams2)

	inactiveParams := pgtestutil.DefaultAccountParams()
	inactiveParams.Name = "Inactive Account"
	inactiveParams.Alias = "@inactive1"
	inactiveParams.Status = "INACTIVE"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, inactiveParams)

	blockedParams := pgtestutil.DefaultAccountParams()
	blockedParams.Name = "Blocked Account"
	blockedParams.Alias = "@blocked1"
	blockedParams.Status = "BLOCKED"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, blockedParams)

	ctx := context.Background()
	statusFilter := "ACTIVE"
	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
		Status:    &statusFilter,
	}

	// Act
	accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 2, "should return only ACTIVE accounts")

	for _, acc := range accounts {
		assert.Equal(t, "ACTIVE", acc.Status.Code, "all returned accounts should have ACTIVE status")
	}
}

func TestIntegration_AccountRepository_FindAll_FiltersByType(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create accounts with different types
	depositParams := pgtestutil.DefaultAccountParams()
	depositParams.Name = "Deposit Account 1"
	depositParams.Alias = "@deposit1"
	depositParams.Type = "deposit"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, depositParams)

	depositParams2 := pgtestutil.DefaultAccountParams()
	depositParams2.Name = "Deposit Account 2"
	depositParams2.Alias = "@deposit2"
	depositParams2.Type = "deposit"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, depositParams2)

	savingsParams := pgtestutil.DefaultAccountParams()
	savingsParams.Name = "Savings Account"
	savingsParams.Alias = "@savings1"
	savingsParams.Type = "savings"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, savingsParams)

	externalParams := pgtestutil.DefaultAccountParams()
	externalParams.Name = "External Account"
	externalParams.Alias = "@external1"
	externalParams.Type = "external"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, externalParams)

	ctx := context.Background()
	typeFilter := "deposit"
	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
		Type:      &typeFilter,
	}

	// Act
	accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 2, "should return only deposit accounts")

	for _, acc := range accounts {
		assert.Equal(t, "deposit", acc.Type, "all returned accounts should have deposit type")
	}
}

func TestIntegration_AccountRepository_FindAll_FiltersByAssetCode(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create accounts with different asset codes
	usdParams := pgtestutil.DefaultAccountParams()
	usdParams.Name = "USD Account 1"
	usdParams.Alias = "@usd1"
	usdParams.AssetCode = "USD"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, usdParams)

	usdParams2 := pgtestutil.DefaultAccountParams()
	usdParams2.Name = "USD Account 2"
	usdParams2.Alias = "@usd2"
	usdParams2.AssetCode = "USD"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, usdParams2)

	brlParams := pgtestutil.DefaultAccountParams()
	brlParams.Name = "BRL Account"
	brlParams.Alias = "@brl1"
	brlParams.AssetCode = "BRL"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, brlParams)

	eurParams := pgtestutil.DefaultAccountParams()
	eurParams.Name = "EUR Account"
	eurParams.Alias = "@eur1"
	eurParams.AssetCode = "EUR"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, eurParams)

	ctx := context.Background()
	assetCodeFilter := "USD"
	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
		AssetCode: &assetCodeFilter,
	}

	// Act
	accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 2, "should return only USD accounts")

	for _, acc := range accounts {
		assert.Equal(t, "USD", acc.AssetCode, "all returned accounts should have USD asset code")
	}
}

func TestIntegration_AccountRepository_FindAll_CombinesMultipleFiltersWithAND(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create accounts with various combinations
	// Account 1: ACTIVE, deposit, USD - should match
	acc1Params := pgtestutil.DefaultAccountParams()
	acc1Params.Name = "Match Account"
	acc1Params.Alias = "@match1"
	acc1Params.Status = "ACTIVE"
	acc1Params.Type = "deposit"
	acc1Params.AssetCode = "USD"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acc1Params)

	// Account 2: ACTIVE, deposit, BRL - should NOT match (wrong asset)
	acc2Params := pgtestutil.DefaultAccountParams()
	acc2Params.Name = "Wrong Asset"
	acc2Params.Alias = "@wrongasset"
	acc2Params.Status = "ACTIVE"
	acc2Params.Type = "deposit"
	acc2Params.AssetCode = "BRL"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acc2Params)

	// Account 3: ACTIVE, savings, USD - should NOT match (wrong type)
	acc3Params := pgtestutil.DefaultAccountParams()
	acc3Params.Name = "Wrong Type"
	acc3Params.Alias = "@wrongtype"
	acc3Params.Status = "ACTIVE"
	acc3Params.Type = "savings"
	acc3Params.AssetCode = "USD"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acc3Params)

	// Account 4: INACTIVE, deposit, USD - should NOT match (wrong status)
	acc4Params := pgtestutil.DefaultAccountParams()
	acc4Params.Name = "Wrong Status"
	acc4Params.Alias = "@wrongstatus"
	acc4Params.Status = "INACTIVE"
	acc4Params.Type = "deposit"
	acc4Params.AssetCode = "USD"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acc4Params)

	ctx := context.Background()
	statusFilter := "ACTIVE"
	typeFilter := "deposit"
	assetCodeFilter := "USD"
	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
		Status:    &statusFilter,
		Type:      &typeFilter,
		AssetCode: &assetCodeFilter,
	}

	// Act
	accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 1, "should return only the one account matching all filters")

	assert.Equal(t, "Match Account", accounts[0].Name)
	assert.Equal(t, "ACTIVE", accounts[0].Status.Code)
	assert.Equal(t, "deposit", accounts[0].Type)
	assert.Equal(t, "USD", accounts[0].AssetCode)
}

func TestIntegration_AccountRepository_FindAll_EmptyFilterReturnsAll(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create 3 accounts with different statuses/types
	acc1Params := pgtestutil.DefaultAccountParams()
	acc1Params.Name = "Account 1"
	acc1Params.Alias = "@acc1"
	acc1Params.Status = "ACTIVE"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acc1Params)

	acc2Params := pgtestutil.DefaultAccountParams()
	acc2Params.Name = "Account 2"
	acc2Params.Alias = "@acc2"
	acc2Params.Status = "INACTIVE"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acc2Params)

	acc3Params := pgtestutil.DefaultAccountParams()
	acc3Params.Name = "Account 3"
	acc3Params.Alias = "@acc3"
	acc3Params.Status = "BLOCKED"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acc3Params)

	ctx := context.Background()
	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
	}

	// Act - empty filter (no filters applied)
	accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 3, "empty filter should return all accounts")
}

// ============================================================================
// FindAll Filter Tests - Name and Alias Prefix Search (B-tree Expression Index)
// ============================================================================

func TestIntegration_AccountRepository_FindAll_FiltersByNamePrefix(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create accounts with different names
	acmeParams := pgtestutil.DefaultAccountParams()
	acmeParams.Name = "Acme Corporation"
	acmeParams.Alias = "@acme1"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acmeParams)

	acmeParams2 := pgtestutil.DefaultAccountParams()
	acmeParams2.Name = "Acme Industries"
	acmeParams2.Alias = "@acme2"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acmeParams2)

	betaParams := pgtestutil.DefaultAccountParams()
	betaParams.Name = "Beta Holdings"
	betaParams.Alias = "@beta1"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, betaParams)

	ctx := context.Background()
	nameFilter := "Acme"
	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
		Name:      &nameFilter,
	}

	// Act
	accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 2, "should return accounts starting with 'Acme'")

	for _, acc := range accounts {
		assert.True(t, strings.HasPrefix(acc.Name, "Acme"), "account name should start with 'Acme'")
	}
}

func TestIntegration_AccountRepository_FindAll_FiltersByNamePrefix_CaseInsensitive(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create account with mixed case name
	acmeParams := pgtestutil.DefaultAccountParams()
	acmeParams.Name = "Acme Corporation"
	acmeParams.Alias = "@acme1"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acmeParams)

	ctx := context.Background()
	// Search with lowercase - should match case-insensitively
	nameFilter := "acme"
	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
		Name:      &nameFilter,
	}

	// Act
	accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 1, "lowercase 'acme' should match 'Acme Corporation' case-insensitively")
	assert.Equal(t, "Acme Corporation", accounts[0].Name)
}

func TestIntegration_AccountRepository_FindAll_FiltersByAliasPrefix(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create accounts with different aliases
	acc1Params := pgtestutil.DefaultAccountParams()
	acc1Params.Name = "Account 1"
	acc1Params.Alias = "@tech-account-001"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acc1Params)

	acc2Params := pgtestutil.DefaultAccountParams()
	acc2Params.Name = "Account 2"
	acc2Params.Alias = "@tech-account-002"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acc2Params)

	acc3Params := pgtestutil.DefaultAccountParams()
	acc3Params.Name = "Account 3"
	acc3Params.Alias = "@finance-account-001"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acc3Params)

	ctx := context.Background()
	aliasFilter := "@tech"
	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
		Alias:     &aliasFilter,
	}

	// Act
	accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 2, "should return accounts with alias starting with '@tech'")

	for _, acc := range accounts {
		assert.True(t, strings.HasPrefix(acc.Alias, "@tech"), "account alias should start with '@tech'")
	}
}

func TestIntegration_AccountRepository_FindAll_FiltersByNamePrefix_NoMiddleWordMatch(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create account with "Acme" in the middle of the name
	params := pgtestutil.DefaultAccountParams()
	params.Name = "MyAcmeCorp"
	params.Alias = "@myacme"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()
	// Search for "Acme" should NOT match "MyAcmeCorp" (prefix match only)
	nameFilter := "Acme"
	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
		Name:      &nameFilter,
	}

	// Act
	accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 0, "'Acme' should NOT match 'MyAcmeCorp' because we use prefix match only")
}

func TestIntegration_AccountRepository_FindAll_FiltersByName_WildcardInjection(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create accounts
	acmeParams := pgtestutil.DefaultAccountParams()
	acmeParams.Name = "Acme Corporation"
	acmeParams.Alias = "@acme1"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acmeParams)

	ctx := context.Background()

	// Test wildcard injection attempts
	injectionTests := []struct {
		name     string
		filter   string
		expected int
	}{
		{"percent wildcard", "%", 0},
		{"underscore wildcard", "Acm_", 0},
		{"backslash escape", "Acme\\", 0},
		{"percent in middle", "A%e", 0},
	}

	for _, tt := range injectionTests {
		t.Run(tt.name, func(t *testing.T) {
			nameFilter := tt.filter
			filter := http.QueryHeader{
				Limit:     10,
				Page:      1,
				SortOrder: "asc",
				StartDate: time.Now().Add(-24 * time.Hour),
				EndDate:   time.Now().Add(24 * time.Hour),
				Name:      &nameFilter,
			}

			accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, filter)

			require.NoError(t, err)
			assert.Len(t, accounts, tt.expected, "wildcard injection '%s' should return %d results", tt.filter, tt.expected)
		})
	}
}

func TestIntegration_AccountRepository_FindAll_FiltersByName_LiteralSpecialChars(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create account with literal % in name
	params := pgtestutil.DefaultAccountParams()
	params.Name = "100% Organic Corp"
	params.Alias = "@organic"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()
	// Search for "100%" should find the account (escaped properly)
	nameFilter := "100%"
	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
		Name:      &nameFilter,
	}

	// Act
	accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 1, "search for '100%%' should find '100%% Organic Corp'")
	assert.Equal(t, "100% Organic Corp", accounts[0].Name)
}

func TestIntegration_AccountRepository_FindAll_CombinesNameAndAliasFilters(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create accounts with various combinations
	// Account 1: Matches both name AND alias filters
	acc1Params := pgtestutil.DefaultAccountParams()
	acc1Params.Name = "Acme Corporation"
	acc1Params.Alias = "@acme-corp"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acc1Params)

	// Account 2: Matches only name filter
	acc2Params := pgtestutil.DefaultAccountParams()
	acc2Params.Name = "Acme Industries"
	acc2Params.Alias = "@industries"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acc2Params)

	// Account 3: Matches only alias filter
	acc3Params := pgtestutil.DefaultAccountParams()
	acc3Params.Name = "Beta Holdings"
	acc3Params.Alias = "@acme-beta"
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, acc3Params)

	ctx := context.Background()
	nameFilter := "Acme"
	aliasFilter := "@acme"
	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
		Name:      &nameFilter,
		Alias:     &aliasFilter,
	}

	// Act
	accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, accounts, 1, "should return only account matching BOTH name AND alias filters")
	assert.Equal(t, "Acme Corporation", accounts[0].Name)
	assert.Equal(t, "@acme-corp", accounts[0].Alias)
}
