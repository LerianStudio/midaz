//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package segment

import (
	"context"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createRepository creates a SegmentPostgreSQLRepository connected to the test database.
func createRepository(t *testing.T, container *pgtestutil.ContainerResult) *SegmentPostgreSQLRepository {
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

	return NewSegmentPostgreSQLRepository(conn)
}

// ============================================================================
// Find Tests
// ============================================================================

func TestIntegration_SegmentRepository_Find_ReturnsSegment(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.SegmentParams{
		Name:   "Business Segment",
		Status: "ACTIVE",
	}
	segmentID := pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	segment, err := repo.Find(ctx, orgID, ledgerID, segmentID)

	// Assert
	require.NoError(t, err, "Find should not return error for existing segment")
	require.NotNil(t, segment, "segment should not be nil")

	assert.Equal(t, segmentID.String(), segment.ID, "ID should match")
	assert.Equal(t, orgID.String(), segment.OrganizationID, "organization ID should match")
	assert.Equal(t, ledgerID.String(), segment.LedgerID, "ledger ID should match")
	assert.Equal(t, "Business Segment", segment.Name, "name should match")
	assert.Equal(t, "ACTIVE", segment.Status.Code, "status should match")
}

func TestIntegration_SegmentRepository_Find_ReturnsErrNotFound(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	segment, err := repo.Find(ctx, orgID, ledgerID, nonExistentID)

	// Assert
	require.Error(t, err, "Find should return error for non-existent segment")
	assert.Nil(t, segment, "segment should be nil")

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
	assert.Equal(t, constant.ErrEntityNotFound.Error(), entityNotFoundErr.Code, "error code should be ErrEntityNotFound")
}

func TestIntegration_SegmentRepository_Find_IgnoresDeletedSegment(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	deletedAt := time.Now().Add(-1 * time.Hour)
	params := pgtestutil.SegmentParams{
		Name:      "Deleted Segment",
		Status:    "ACTIVE",
		DeletedAt: &deletedAt,
	}
	segmentID := pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	segment, err := repo.Find(ctx, orgID, ledgerID, segmentID)

	// Assert
	require.Error(t, err, "Find should return error for deleted segment")
	assert.Nil(t, segment, "deleted segment should not be returned")
}

// ============================================================================
// FindByName Tests
// ============================================================================

func TestIntegration_SegmentRepository_FindByName_ReturnsTrueForDuplicate(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.SegmentParams{
		Name:   "Unique Segment Name",
		Status: "ACTIVE",
	}
	pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	isDuplicate, err := repo.FindByName(ctx, orgID, ledgerID, "Unique Segment Name")

	// Assert
	assert.True(t, isDuplicate, "should return true when duplicate is found")
	require.Error(t, err, "should return error for duplicate")

	var conflictErr pkg.EntityConflictError
	require.ErrorAs(t, err, &conflictErr, "error should be EntityConflictError")
	assert.Equal(t, constant.ErrDuplicateSegmentName.Error(), conflictErr.Code, "error code should be ErrDuplicateSegmentName")
}

func TestIntegration_SegmentRepository_FindByName_ReturnsFalseWhenNotFound(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	ctx := context.Background()

	// Act
	isDuplicate, err := repo.FindByName(ctx, orgID, ledgerID, "Non Existent Segment")

	// Assert
	assert.False(t, isDuplicate, "should return false when no duplicate found")
	require.NoError(t, err, "should not return error when no duplicate")
}

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_SegmentRepository_Create_InsertsAndReturnsSegment(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	now := time.Now().Truncate(time.Microsecond)

	segment := &mmodel.Segment{
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Name:           "New Segment",
		Status:         mmodel.Status{Code: "ACTIVE"},
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	ctx := context.Background()

	// Act
	created, err := repo.Create(ctx, segment)

	// Assert
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, created)

	// Verify by finding - use the ID from created segment since FromEntity generates new UUIDs
	createdID, err := uuid.Parse(created.ID)
	require.NoError(t, err, "created ID should be valid UUID")

	found, err := repo.Find(ctx, orgID, ledgerID, createdID)
	require.NoError(t, err)
	assert.Equal(t, "New Segment", found.Name)
	assert.Equal(t, "ACTIVE", found.Status.Code)
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_SegmentRepository_Update_ChangesNameAndStatus(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.SegmentParams{
		Name:   "Original Name",
		Status: "ACTIVE",
	}
	segmentID := pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Get original to compare updated_at
	original, err := repo.Find(ctx, orgID, ledgerID, segmentID)
	require.NoError(t, err)
	originalUpdatedAt := original.UpdatedAt

	// Small delay to ensure updated_at differs
	time.Sleep(10 * time.Millisecond)

	// Act
	statusDesc := "Deactivated for review"
	updateData := &mmodel.Segment{
		Name:   "Updated Name",
		Status: mmodel.Status{Code: "INACTIVE", Description: &statusDesc},
	}
	updated, err := repo.Update(ctx, orgID, ledgerID, segmentID, updateData)

	// Assert
	require.NoError(t, err, "Update should not return error")
	require.NotNil(t, updated)

	found, err := repo.Find(ctx, orgID, ledgerID, segmentID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", found.Name, "name should be updated")
	assert.Equal(t, "INACTIVE", found.Status.Code, "status should be updated")
	assert.Equal(t, "Deactivated for review", *found.Status.Description, "status description should be updated")
	assert.True(t, found.UpdatedAt.After(originalUpdatedAt), "updated_at should be changed after update")
}

func TestIntegration_SegmentRepository_Update_ReturnsErrNotFound(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	updateData := &mmodel.Segment{
		Name: "Updated Name",
	}
	updated, err := repo.Update(ctx, orgID, ledgerID, nonExistentID, updateData)

	// Assert
	require.Error(t, err, "Update should return error for non-existent segment")
	assert.Nil(t, updated)

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
}

// ============================================================================
// FindAll Tests (page/limit pagination)
// ============================================================================

func TestIntegration_SegmentRepository_FindAll_ReturnsSegments(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create multiple segments
	params1 := pgtestutil.SegmentParams{
		Name:   "Segment Alpha",
		Status: "ACTIVE",
	}
	pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, params1)

	params2 := pgtestutil.SegmentParams{
		Name:   "Segment Beta",
		Status: "ACTIVE",
	}
	pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, params2)

	ctx := context.Background()

	// Act
	segments, err := repo.FindAll(ctx, orgID, ledgerID, defaultPagination())

	// Assert
	require.NoError(t, err, "FindAll should not return error")
	assert.Len(t, segments, 2, "should return 2 segments")
}

func TestIntegration_SegmentRepository_FindAll_Pagination(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create 5 segments
	names := []string{"Segment A", "Segment B", "Segment C", "Segment D", "Segment E"}
	for _, name := range names {
		params := pgtestutil.SegmentParams{
			Name:   name,
			Status: "ACTIVE",
		}
		pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, params)
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
	for _, s := range page1 {
		allIDs[s.ID] = true
	}
	for _, s := range page2 {
		allIDs[s.ID] = true
	}
	for _, s := range page3 {
		allIDs[s.ID] = true
	}
	assert.Len(t, allIDs, 5, "should have 5 unique segments across all pages")
}

func TestIntegration_SegmentRepository_FindAll_ExcludesDeleted(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create active segment
	activeParams := pgtestutil.SegmentParams{
		Name:   "Active Segment",
		Status: "ACTIVE",
	}
	pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, activeParams)

	// Create deleted segment
	deletedAt := time.Now().Add(-1 * time.Hour)
	deletedParams := pgtestutil.SegmentParams{
		Name:      "Deleted Segment",
		Status:    "ACTIVE",
		DeletedAt: &deletedAt,
	}
	pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, deletedParams)

	ctx := context.Background()

	// Act
	segments, err := repo.FindAll(ctx, orgID, ledgerID, defaultPagination())

	// Assert
	require.NoError(t, err, "FindAll should not return error")
	assert.Len(t, segments, 1, "should return only active segment")
	assert.Equal(t, "Active Segment", segments[0].Name)
}

// ============================================================================
// FindByIDs Tests
// ============================================================================

func TestIntegration_SegmentRepository_FindByIDs_ReturnsMatchingSegments(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create 3 segments
	id1 := pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, pgtestutil.SegmentParams{
		Name: "Segment 1", Status: "ACTIVE",
	})
	id2 := pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, pgtestutil.SegmentParams{
		Name: "Segment 2", Status: "ACTIVE",
	})
	pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, pgtestutil.SegmentParams{
		Name: "Segment 3", Status: "ACTIVE",
	})

	ctx := context.Background()

	// Act - request only first 2
	segments, err := repo.FindByIDs(ctx, orgID, ledgerID, []uuid.UUID{id1, id2})

	// Assert
	require.NoError(t, err, "FindByIDs should not return error")
	assert.Len(t, segments, 2, "should return only 2 segments")

	names := make([]string, len(segments))
	for i, s := range segments {
		names[i] = s.Name
	}
	assert.ElementsMatch(t, []string{"Segment 1", "Segment 2"}, names)
}

func TestIntegration_SegmentRepository_FindByIDs_ReturnsEmptyForNoMatches(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	ctx := context.Background()

	// Act
	nonExistentIDs := []uuid.UUID{libCommons.GenerateUUIDv7(), libCommons.GenerateUUIDv7()}
	segments, err := repo.FindByIDs(ctx, orgID, ledgerID, nonExistentIDs)

	// Assert
	require.NoError(t, err, "should not error for empty result")
	assert.Empty(t, segments, "should return empty slice")
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestIntegration_SegmentRepository_Delete_SoftDeletesSegment(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	params := pgtestutil.SegmentParams{
		Name:   "To Delete",
		Status: "ACTIVE",
	}
	segmentID := pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, params)

	ctx := context.Background()

	// Act
	err := repo.Delete(ctx, orgID, ledgerID, segmentID)

	// Assert
	require.NoError(t, err, "Delete should not return error")

	// Verify deleted_at is actually set in DB
	var deletedAt *time.Time
	err = container.DB.QueryRow(`SELECT deleted_at FROM segment WHERE id = $1`, segmentID).Scan(&deletedAt)
	require.NoError(t, err, "should be able to query segment directly")
	require.NotNil(t, deletedAt, "deleted_at should be set")

	// Segment should not be findable anymore via repository
	found, err := repo.Find(ctx, orgID, ledgerID, segmentID)
	require.Error(t, err, "Find should return error after delete")
	assert.Nil(t, found, "deleted segment should not be returned")
}

func TestIntegration_SegmentRepository_Delete_ReturnsErrNotFound(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	err := repo.Delete(ctx, orgID, ledgerID, nonExistentID)

	// Assert
	require.Error(t, err, "Delete should return error for non-existent segment")

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
}

// ============================================================================
// Count Tests
// ============================================================================

func TestIntegration_SegmentRepository_Count_ReturnsCorrectCount(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create 3 segments
	names := []string{"Segment A", "Segment B", "Segment C"}
	for _, name := range names {
		params := pgtestutil.SegmentParams{
			Name:   name,
			Status: "ACTIVE",
		}
		pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, params)
	}

	ctx := context.Background()

	// Act
	count, err := repo.Count(ctx, orgID, ledgerID)

	// Assert
	require.NoError(t, err, "Count should not return error")
	assert.Equal(t, int64(3), count, "count should be 3")
}

func TestIntegration_SegmentRepository_Count_ExcludesDeletedSegments(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create 2 active segments
	pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, pgtestutil.SegmentParams{
		Name: "Active 1", Status: "ACTIVE",
	})
	pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, pgtestutil.SegmentParams{
		Name: "Active 2", Status: "ACTIVE",
	})

	// Create 1 deleted segment
	deletedAt := time.Now().Add(-1 * time.Hour)
	pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, pgtestutil.SegmentParams{
		Name: "Deleted", Status: "ACTIVE", DeletedAt: &deletedAt,
	})

	ctx := context.Background()

	// Act
	count, err := repo.Count(ctx, orgID, ledgerID)

	// Assert
	require.NoError(t, err, "Count should not return error")
	assert.Equal(t, int64(2), count, "count should exclude deleted segment")
}

func TestIntegration_SegmentRepository_Count_ReturnsZeroForEmptyLedger(t *testing.T) {
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
