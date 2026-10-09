//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package asset

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// createRepository creates an AssetPostgreSQLRepository connected to the test database.
func createRepository(t *testing.T, container *pgtestutil.ContainerResult) *AssetPostgreSQLRepository {
	t.Helper()

	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)

	conn := pgtestutil.ConnectPostgresClient(t.Context(), t, connStr, connStr)

	return NewAssetPostgreSQLRepository(conn)
}

// ============================================================================
// Find Tests
// ============================================================================

func TestIntegration_AssetRepository_Find_ReturnsAsset(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.AssetParams{
		Name:   "US Dollar",
		Type:   "currency",
		Code:   "USD",
		Status: "ACTIVE",
	}
	assetID := pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	asset, err := repo.Find(ctx, orgID, ledgerID, assetID)

	// Assert
	require.NoError(t, err, "Find should not return error for existing asset")
	require.NotNil(t, asset, "asset should not be nil")

	assert.Equal(t, assetID.String(), asset.ID, "ID should match")
	assert.Equal(t, orgID.String(), asset.OrganizationID, "organization ID should match")
	assert.Equal(t, ledgerID.String(), asset.LedgerID, "ledger ID should match")
	assert.Equal(t, "US Dollar", asset.Name, "name should match")
	assert.Equal(t, "currency", asset.Type, "type should match")
	assert.Equal(t, "USD", asset.Code, "code should match")
	assert.Equal(t, "ACTIVE", asset.Status.Code, "status should match")
}

func TestIntegration_AssetRepository_Find_ReturnsErrNotFound(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	nonExistentID := uuid.Must(libCommons.GenerateUUIDv7())

	ctx := context.Background()

	// Act
	asset, err := repo.Find(ctx, orgID, ledgerID, nonExistentID)

	// Assert
	require.Error(t, err, "Find should return error for non-existent asset")
	assert.Nil(t, asset, "asset should be nil")

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
	assert.Equal(t, constant.ErrEntityNotFound.Error(), entityNotFoundErr.Code, "error code should be ErrEntityNotFound")
}

func TestIntegration_AssetRepository_Find_IgnoresDeletedAsset(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	deletedAt := time.Now().Add(-1 * time.Hour)
	params := pgtestutil.AssetParams{
		Name:      "Deleted Asset",
		Type:      "currency",
		Code:      "DEL",
		Status:    "ACTIVE",
		DeletedAt: &deletedAt,
	}
	assetID := pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	asset, err := repo.Find(ctx, orgID, ledgerID, assetID)

	// Assert
	require.Error(t, err, "Find should return error for deleted asset")
	assert.Nil(t, asset, "deleted asset should not be returned")
}

// ============================================================================
// FindByNameOrCode Tests
// ============================================================================

func TestIntegration_AssetRepository_FindByNameOrCode_ReturnsTrueForDuplicateName(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.AssetParams{
		Name:   "US Dollar",
		Type:   "currency",
		Code:   "USD",
		Status: "ACTIVE",
	}
	pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act - search for same name but different code
	found, err := repo.FindByNameOrCode(ctx, orgID, ledgerID, "US Dollar", "EUR")

	// Assert
	assert.True(t, found, "should find duplicate by name")
	require.Error(t, err, "should return error for duplicate")
}

func TestIntegration_AssetRepository_FindByNameOrCode_ReturnsTrueForDuplicateCode(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.AssetParams{
		Name:   "US Dollar",
		Type:   "currency",
		Code:   "USD",
		Status: "ACTIVE",
	}
	pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act - search for different name but same code
	found, err := repo.FindByNameOrCode(ctx, orgID, ledgerID, "Different Name", "USD")

	// Assert
	assert.True(t, found, "should find duplicate by code")
	require.Error(t, err, "should return error for duplicate")
}

func TestIntegration_AssetRepository_FindByNameOrCode_ReturnsFalseWhenNotFound(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	ctx := context.Background()

	// Act
	found, err := repo.FindByNameOrCode(ctx, orgID, ledgerID, "Non Existent", "XXX")

	// Assert
	assert.False(t, found, "should not find non-existent asset")
	require.NoError(t, err, "should not return error when not found")
}

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_AssetRepository_Create(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	now := time.Now().Truncate(time.Microsecond)
	assetID := uuid.Must(libCommons.GenerateUUIDv7())

	asset := &mmodel.Asset{
		ID:             assetID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Name:           "Euro",
		Type:           "currency",
		Code:           "EUR",
		Status:         mmodel.Status{Code: "ACTIVE"},
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	ctx := context.Background()

	// Act
	created, err := repo.Create(ctx, asset)

	// Assert
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, created)

	// Verify by finding - use the ID from created asset since FromEntity generates new UUIDs
	createdID, err := uuid.Parse(created.ID)
	require.NoError(t, err, "created ID should be valid UUID")

	found, err := repo.Find(ctx, orgID, ledgerID, createdID)
	require.NoError(t, err)
	assert.Equal(t, "Euro", found.Name)
	assert.Equal(t, "EUR", found.Code)
	assert.Equal(t, "currency", found.Type)
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_AssetRepository_Update_ChangesNameAndStatus(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.AssetParams{
		Name:   "Original Name",
		Type:   "currency",
		Code:   "ORG",
		Status: "ACTIVE",
	}
	assetID := pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Get original to compare updated_at
	original, err := repo.Find(ctx, orgID, ledgerID, assetID)
	require.NoError(t, err)
	originalUpdatedAt := original.UpdatedAt

	// Small delay to ensure updated_at differs
	time.Sleep(10 * time.Millisecond)

	// Act
	statusDesc := "Deactivated for maintenance"
	updateData := &mmodel.Asset{
		Name:   "Updated Name",
		Status: mmodel.Status{Code: "INACTIVE", Description: &statusDesc},
	}
	updated, err := repo.Update(ctx, orgID, ledgerID, assetID, updateData)

	// Assert
	require.NoError(t, err, "Update should not return error")
	require.NotNil(t, updated)

	found, err := repo.Find(ctx, orgID, ledgerID, assetID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", found.Name, "name should be updated")
	assert.Equal(t, "INACTIVE", found.Status.Code, "status should be updated")
	assert.Equal(t, "Deactivated for maintenance", *found.Status.Description, "status description should be updated")
	assert.Equal(t, "ORG", found.Code, "code should remain unchanged")
	assert.True(t, found.UpdatedAt.After(originalUpdatedAt), "updated_at should be changed after update")
}

func TestIntegration_AssetRepository_Update_ReturnsErrNotFound(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	nonExistentID := uuid.Must(libCommons.GenerateUUIDv7())

	ctx := context.Background()

	// Act
	updateData := &mmodel.Asset{
		Name: "Updated Name",
	}
	updated, err := repo.Update(ctx, orgID, ledgerID, nonExistentID, updateData)

	// Assert
	require.Error(t, err, "Update should return error for non-existent asset")
	assert.Nil(t, updated)

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
}

// ============================================================================
// FindAll Tests (page/limit pagination)
// ============================================================================

func TestIntegration_AssetRepository_FindAll_ReturnsAssets(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create multiple assets
	params1 := pgtestutil.AssetParams{
		Name:   "US Dollar",
		Type:   "currency",
		Code:   "USD",
		Status: "ACTIVE",
	}
	pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, params1)

	params2 := pgtestutil.AssetParams{
		Name:   "Euro",
		Type:   "currency",
		Code:   "EUR",
		Status: "ACTIVE",
	}
	pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, params2)

	ctx := context.Background()

	// Act
	assets, err := repo.FindAll(ctx, orgID, ledgerID, defaultPagination())

	// Assert
	require.NoError(t, err, "FindAll should not return error")
	assert.Len(t, assets, 2, "should return 2 assets")
}

func TestIntegration_AssetRepository_FindAll_EmptyForNonExistentLedger(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	nonExistentLedgerID := uuid.Must(libCommons.GenerateUUIDv7())

	ctx := context.Background()

	// Act
	assets, err := repo.FindAll(ctx, orgID, nonExistentLedgerID, defaultPagination())

	// Assert
	require.NoError(t, err, "should not error for empty result")
	assert.Empty(t, assets, "should return empty slice")
}

func TestIntegration_AssetRepository_FindAll_Pagination(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create 5 assets
	codes := []string{"USD", "EUR", "GBP", "JPY", "CHF"}
	for _, code := range codes {
		params := pgtestutil.AssetParams{
			Name:   code + " Currency",
			Type:   "currency",
			Code:   code,
			Status: "ACTIVE",
		}
		pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, params)
	}

	ctx := context.Background()

	// Page 1: limit=2, page=1
	page1Filter := http.QueryHeader{
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
	page2Filter := http.QueryHeader{
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
	page3Filter := http.QueryHeader{
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
	for _, a := range page1 {
		allIDs[a.ID] = true
	}
	for _, a := range page2 {
		allIDs[a.ID] = true
	}
	for _, a := range page3 {
		allIDs[a.ID] = true
	}
	assert.Len(t, allIDs, 5, "should have 5 unique assets across all pages")
}

func TestIntegration_AssetRepository_FindAll_ByEntityIDs_Paginates(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	otherLedgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	requested := make([]uuid.UUID, 0, 3)
	for _, code := range []string{"USD", "EUR", "GBP"} {
		requested = append(requested, pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, pgtestutil.AssetParams{
			Name: code + " Currency", Type: "currency", Code: code, Status: "ACTIVE",
		}))
	}

	unrequestedID := pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, pgtestutil.AssetParams{
		Name: "JPY Currency", Type: "currency", Code: "JPY", Status: "ACTIVE",
	})
	otherLedgerAssetID := pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, otherLedgerID, pgtestutil.AssetParams{
		Name: "CHF Currency", Type: "currency", Code: "CHF", Status: "ACTIVE",
	})

	ctx := context.Background()

	filter := defaultPagination()
	filter.Limit = 2
	filter.EntityIDs = append(append([]uuid.UUID{}, requested...), otherLedgerAssetID)

	filter.Page = 1
	page1, err := repo.FindAll(ctx, orgID, ledgerID, filter)
	require.NoError(t, err)
	assert.Len(t, page1, 2, "page 1 should have 2 items")

	filter.Page = 2
	page2, err := repo.FindAll(ctx, orgID, ledgerID, filter)
	require.NoError(t, err)
	assert.Len(t, page2, 1, "page 2 should have 1 item")

	want := make(map[string]bool, len(requested))
	for _, id := range requested {
		want[id.String()] = true
	}

	seen := make(map[string]bool)
	for _, a := range append(page1, page2...) {
		assert.True(t, want[a.ID], "asset %s should be one of the requested ids", a.ID)
		assert.NotEqual(t, unrequestedID.String(), a.ID, "unrequested asset must not be returned")
		assert.NotEqual(t, otherLedgerAssetID.String(), a.ID, "asset from another ledger must not be returned")
		assert.Equal(t, orgID.String(), a.OrganizationID)
		assert.Equal(t, ledgerID.String(), a.LedgerID)
		seen[a.ID] = true
	}

	assert.Len(t, seen, 3, "should return the 3 requested assets across both pages")
}

func TestIntegration_AssetRepository_FindAll_FiltersByDateRange(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create an asset (created today)
	params := pgtestutil.AssetParams{
		Name:   "Date Test Asset",
		Type:   "currency",
		Code:   "DTA",
		Status: "ACTIVE",
	}
	pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act 1: Query with past-only window (should return 0 items)
	pastFilter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(0, 0, -10),
		EndDate:   time.Now().AddDate(0, 0, -9),
	}
	assetsPast, err := repo.FindAll(ctx, orgID, ledgerID, pastFilter)
	require.NoError(t, err)
	assert.Empty(t, assetsPast, "past-only window should return 0 items")

	// Act 2: Query with today's window (should return 1 item)
	todayFilter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(0, 0, -1),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	assetsToday, err := repo.FindAll(ctx, orgID, ledgerID, todayFilter)
	require.NoError(t, err)
	assert.Len(t, assetsToday, 1, "today's window should return 1 item")
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestIntegration_AssetRepository_Delete_SoftDeletesAsset(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.AssetParams{
		Name:   "To Delete",
		Type:   "currency",
		Code:   "TDL",
		Status: "ACTIVE",
	}
	assetID := pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	err := repo.Delete(ctx, orgID, ledgerID, assetID)

	// Assert
	require.NoError(t, err, "Delete should not return error")

	// Verify deleted_at is actually set in DB
	var deletedAt *time.Time
	err = container.DB.QueryRow(`SELECT deleted_at FROM asset WHERE id = $1`, assetID).Scan(&deletedAt)
	require.NoError(t, err, "should be able to query asset directly")
	require.NotNil(t, deletedAt, "deleted_at should be set")

	// Asset should not be findable anymore via repository
	found, err := repo.Find(ctx, orgID, ledgerID, assetID)
	require.Error(t, err, "Find should return error after delete")
	assert.Nil(t, found, "deleted asset should not be returned")
}

func TestIntegration_AssetRepository_Delete_ReturnsErrNotFound(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	nonExistentID := uuid.Must(libCommons.GenerateUUIDv7())

	ctx := context.Background()

	// Act
	err := repo.Delete(ctx, orgID, ledgerID, nonExistentID)

	// Assert
	require.Error(t, err, "Delete should return error for non-existent asset")

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
}

// ============================================================================
// Count Tests
// ============================================================================

func TestIntegration_AssetRepository_Count_ReturnsCorrectCount(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create 3 assets
	codes := []string{"USD", "EUR", "GBP"}
	for _, code := range codes {
		params := pgtestutil.AssetParams{
			Name:   code + " Currency",
			Type:   "currency",
			Code:   code,
			Status: "ACTIVE",
		}
		pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, params)
	}

	ctx := context.Background()

	// Act
	count, err := repo.Count(ctx, orgID, ledgerID)

	// Assert
	require.NoError(t, err, "Count should not return error")
	assert.Equal(t, int64(3), count, "count should be 3")
}

func TestIntegration_AssetRepository_Count_ExcludesDeletedAssets(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create 2 active assets
	pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, pgtestutil.AssetParams{
		Name: "Active 1", Type: "currency", Code: "AC1", Status: "ACTIVE",
	})
	pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, pgtestutil.AssetParams{
		Name: "Active 2", Type: "currency", Code: "AC2", Status: "ACTIVE",
	})

	// Create 1 deleted asset
	deletedAt := time.Now().Add(-1 * time.Hour)
	pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, pgtestutil.AssetParams{
		Name: "Deleted", Type: "currency", Code: "DEL", Status: "ACTIVE", DeletedAt: &deletedAt,
	})

	ctx := context.Background()

	// Act
	count, err := repo.Count(ctx, orgID, ledgerID)

	// Assert
	require.NoError(t, err, "Count should not return error")
	assert.Equal(t, int64(2), count, "count should exclude deleted asset")
}

func TestIntegration_AssetRepository_Count_ReturnsZeroForEmptyLedger(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

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

func defaultPagination() http.QueryHeader {
	return http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(-1, 0, 0), // 1 year ago
		EndDate:   time.Now().AddDate(0, 0, 1),  // 1 day ahead
	}
}
