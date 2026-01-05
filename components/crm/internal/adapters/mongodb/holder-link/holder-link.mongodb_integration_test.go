//go:build integration

package holderlink

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/tests/utils"
	mongotestutil "github.com/LerianStudio/midaz/v3/tests/utils/mongodb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// ============================================================================
// Test Helpers
// ============================================================================

// createRepository creates a MongoDBRepository for integration testing.
func createRepository(t *testing.T, container *mongotestutil.ContainerResult) *MongoDBRepository {
	t.Helper()

	conn := mongotestutil.CreateConnection(t, container.URI, container.DBName)

	return &MongoDBRepository{
		connection: conn,
		Database:   container.DBName,
	}
}

// createTestHolderLink builds a test holder link with default values.
func createTestHolderLink(holderID, aliasID uuid.UUID, linkType string) *mmodel.HolderLink {
	id := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	return &mmodel.HolderLink{
		ID:        &id,
		HolderID:  &holderID,
		AliasID:   &aliasID,
		LinkType:  testutils.Ptr(linkType),
		Metadata:  map[string]any{"test": true},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_HolderLinkRepo_Create(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-" + uuid.New().String()[:8]
	holderID := uuid.New()
	aliasID := uuid.New()

	holderLink := createTestHolderLink(holderID, aliasID, string(mmodel.LinkTypeLegalRepresentative))

	// Act
	result, err := repo.Create(ctx, organizationID, holderLink)

	// Assert
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, result)
	assert.Equal(t, holderLink.ID, result.ID)
	assert.Equal(t, holderLink.HolderID, result.HolderID)
	assert.Equal(t, holderLink.AliasID, result.AliasID)
	assert.Equal(t, *holderLink.LinkType, *result.LinkType)

	// Verify via direct query
	collName := strings.ToLower("holder_links_" + organizationID)
	count := mongotestutil.CountDocuments(t, container.Database, collName, bson.M{"_id": holderLink.ID})
	assert.Equal(t, int64(1), count, "should have exactly 1 document")
}

func TestIntegration_HolderLinkRepo_Create_DuplicateHolderLink(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-dup-" + uuid.New().String()[:8]
	holderID := uuid.New()
	aliasID := uuid.New()
	linkType := string(mmodel.LinkTypeLegalRepresentative)

	// Create first holder link
	holderLink1 := createTestHolderLink(holderID, aliasID, linkType)
	_, err := repo.Create(ctx, organizationID, holderLink1)
	require.NoError(t, err, "first create should succeed")

	// Act - Try to create second holder link with same alias_id + link_type
	holderLink2 := createTestHolderLink(uuid.New(), aliasID, linkType) // Different holder, same alias + link_type
	_, err = repo.Create(ctx, organizationID, holderLink2)

	// Assert
	require.Error(t, err, "duplicate alias_id + link_type should fail")
	assert.Contains(t, err.Error(), "already exists", "should return ErrDuplicateHolderLink")
}

func TestIntegration_HolderLinkRepo_Create_DuplicatePrimaryHolder(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-primary-" + uuid.New().String()[:8]
	aliasID := uuid.New()

	// Create first PRIMARY_HOLDER for the alias
	holderLink1 := createTestHolderLink(uuid.New(), aliasID, string(mmodel.LinkTypePrimaryHolder))
	_, err := repo.Create(ctx, organizationID, holderLink1)
	require.NoError(t, err, "first PRIMARY_HOLDER create should succeed")

	// Act - Try to create second PRIMARY_HOLDER for the same alias (different holder)
	holderLink2 := createTestHolderLink(uuid.New(), aliasID, string(mmodel.LinkTypePrimaryHolder))
	_, err = repo.Create(ctx, organizationID, holderLink2)

	// Assert
	// Note: Both alias_id_link_type_unique and alias_id_primary_holder_unique indexes are violated.
	// MongoDB reports the first index that fails. Since alias_id_link_type_unique is checked first
	// in getDuplicateKeyErrorType, we get ErrDuplicateHolderLink (CRM-0020) instead of
	// ErrPrimaryHolderAlreadyExists (CRM-0019). Both are valid for this scenario.
	require.Error(t, err, "duplicate PRIMARY_HOLDER for same alias should fail")
	assert.Contains(t, err.Error(), "holder link with the same alias ID and link type already exists", "should return ErrDuplicateHolderLink")
}

func TestIntegration_HolderLinkRepo_Create_MultipleLinkTypesForSameAlias(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-multi-" + uuid.New().String()[:8]
	aliasID := uuid.New()

	// Create PRIMARY_HOLDER
	holderLink1 := createTestHolderLink(uuid.New(), aliasID, string(mmodel.LinkTypePrimaryHolder))
	_, err := repo.Create(ctx, organizationID, holderLink1)
	require.NoError(t, err, "PRIMARY_HOLDER create should succeed")

	// Create LEGAL_REPRESENTATIVE for same alias
	holderLink2 := createTestHolderLink(uuid.New(), aliasID, string(mmodel.LinkTypeLegalRepresentative))
	_, err = repo.Create(ctx, organizationID, holderLink2)
	require.NoError(t, err, "LEGAL_REPRESENTATIVE create should succeed")

	// Create RESPONSIBLE_PARTY for same alias
	holderLink3 := createTestHolderLink(uuid.New(), aliasID, string(mmodel.LinkTypeResponsibleParty))
	_, err = repo.Create(ctx, organizationID, holderLink3)
	require.NoError(t, err, "RESPONSIBLE_PARTY create should succeed")

	// Assert - All three should exist
	collName := strings.ToLower("holder_links_" + organizationID)
	count := mongotestutil.CountDocuments(t, container.Database, collName, bson.M{"alias_id": aliasID})
	assert.Equal(t, int64(3), count, "should have 3 holder links for same alias")
}

// ============================================================================
// Find Tests
// ============================================================================

func TestIntegration_HolderLinkRepo_Find(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-find-" + uuid.New().String()[:8]
	holderID := uuid.New()
	aliasID := uuid.New()

	holderLink := createTestHolderLink(holderID, aliasID, string(mmodel.LinkTypeLegalRepresentative))
	_, err := repo.Create(ctx, organizationID, holderLink)
	require.NoError(t, err)

	// Act
	result, err := repo.Find(ctx, organizationID, *holderLink.ID, false)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, holderLink.ID, result.ID)
	assert.Equal(t, holderLink.HolderID, result.HolderID)
	assert.Equal(t, holderLink.AliasID, result.AliasID)
	assert.Equal(t, *holderLink.LinkType, *result.LinkType)
}

func TestIntegration_HolderLinkRepo_Find_NotFound(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-notfound-" + uuid.New().String()[:8]
	nonExistentID := uuid.New()

	// Act
	result, err := repo.Find(ctx, organizationID, nonExistentID, false)

	// Assert
	require.Error(t, err, "should return error for non-existent holder link")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "holder link", "should return ErrHolderLinkNotFound")
}

func TestIntegration_HolderLinkRepo_Find_ExcludesDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-deleted-" + uuid.New().String()[:8]
	holderID := uuid.New()
	aliasID := uuid.New()

	holderLink := createTestHolderLink(holderID, aliasID, string(mmodel.LinkTypeLegalRepresentative))
	_, err := repo.Create(ctx, organizationID, holderLink)
	require.NoError(t, err)

	// Soft delete
	err = repo.Delete(ctx, organizationID, *holderLink.ID, false)
	require.NoError(t, err)

	// Act - Find without includeDeleted
	result, err := repo.Find(ctx, organizationID, *holderLink.ID, false)

	// Assert
	require.Error(t, err, "should not find soft-deleted holder link")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "holder link")
}

func TestIntegration_HolderLinkRepo_Find_IncludesDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-incldel-" + uuid.New().String()[:8]
	holderID := uuid.New()
	aliasID := uuid.New()

	holderLink := createTestHolderLink(holderID, aliasID, string(mmodel.LinkTypeLegalRepresentative))
	_, err := repo.Create(ctx, organizationID, holderLink)
	require.NoError(t, err)

	// Soft delete
	err = repo.Delete(ctx, organizationID, *holderLink.ID, false)
	require.NoError(t, err)

	// Act - Find with includeDeleted=true
	result, err := repo.Find(ctx, organizationID, *holderLink.ID, true)

	// Assert
	require.NoError(t, err, "should find soft-deleted holder link with includeDeleted=true")
	require.NotNil(t, result)
	assert.NotNil(t, result.DeletedAt, "deleted_at should be set")
}

// ============================================================================
// FindByAliasID Tests
// ============================================================================

func TestIntegration_HolderLinkRepo_FindByAliasID(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-byalias-" + uuid.New().String()[:8]
	aliasID := uuid.New()

	// Create multiple holder links for the same alias
	holderLink1 := createTestHolderLink(uuid.New(), aliasID, string(mmodel.LinkTypePrimaryHolder))
	_, err := repo.Create(ctx, organizationID, holderLink1)
	require.NoError(t, err)

	holderLink2 := createTestHolderLink(uuid.New(), aliasID, string(mmodel.LinkTypeLegalRepresentative))
	_, err = repo.Create(ctx, organizationID, holderLink2)
	require.NoError(t, err)

	// Create holder link for different alias (should not be returned)
	holderLink3 := createTestHolderLink(uuid.New(), uuid.New(), string(mmodel.LinkTypeLegalRepresentative))
	_, err = repo.Create(ctx, organizationID, holderLink3)
	require.NoError(t, err)

	// Act
	results, err := repo.FindByAliasID(ctx, organizationID, aliasID, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 2, "should return only holder links for the specified alias")

	// Verify all returned links belong to the correct alias
	for _, hl := range results {
		assert.Equal(t, aliasID, *hl.AliasID)
	}
}

func TestIntegration_HolderLinkRepo_FindByAliasID_ExcludesDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-byaliasdel-" + uuid.New().String()[:8]
	aliasID := uuid.New()

	// Create two holder links
	holderLink1 := createTestHolderLink(uuid.New(), aliasID, string(mmodel.LinkTypePrimaryHolder))
	_, err := repo.Create(ctx, organizationID, holderLink1)
	require.NoError(t, err)

	holderLink2 := createTestHolderLink(uuid.New(), aliasID, string(mmodel.LinkTypeLegalRepresentative))
	_, err = repo.Create(ctx, organizationID, holderLink2)
	require.NoError(t, err)

	// Soft delete one
	err = repo.Delete(ctx, organizationID, *holderLink1.ID, false)
	require.NoError(t, err)

	// Act
	results, err := repo.FindByAliasID(ctx, organizationID, aliasID, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 1, "should exclude soft-deleted holder link")
	assert.Equal(t, holderLink2.ID, results[0].ID)
}

func TestIntegration_HolderLinkRepo_FindByAliasID_ReturnsEmpty(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-byaliasempty-" + uuid.New().String()[:8]
	nonExistentAliasID := uuid.New()

	// Act
	results, err := repo.FindByAliasID(ctx, organizationID, nonExistentAliasID, false)

	// Assert
	require.NoError(t, err, "should not error on empty result")
	assert.Empty(t, results)
}

// ============================================================================
// FindByHolderID Tests
// ============================================================================

func TestIntegration_HolderLinkRepo_FindByHolderID(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-byholder-" + uuid.New().String()[:8]
	holderID := uuid.New()

	// Create multiple holder links for the same holder
	holderLink1 := createTestHolderLink(holderID, uuid.New(), string(mmodel.LinkTypePrimaryHolder))
	_, err := repo.Create(ctx, organizationID, holderLink1)
	require.NoError(t, err)

	holderLink2 := createTestHolderLink(holderID, uuid.New(), string(mmodel.LinkTypeLegalRepresentative))
	_, err = repo.Create(ctx, organizationID, holderLink2)
	require.NoError(t, err)

	// Create holder link for different holder (should not be returned)
	holderLink3 := createTestHolderLink(uuid.New(), uuid.New(), string(mmodel.LinkTypeLegalRepresentative))
	_, err = repo.Create(ctx, organizationID, holderLink3)
	require.NoError(t, err)

	// Act
	results, err := repo.FindByHolderID(ctx, organizationID, holderID, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 2, "should return only holder links for the specified holder")

	// Verify all returned links belong to the correct holder
	for _, hl := range results {
		assert.Equal(t, holderID, *hl.HolderID)
	}
}

func TestIntegration_HolderLinkRepo_FindByHolderID_ExcludesDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-byholderdel-" + uuid.New().String()[:8]
	holderID := uuid.New()

	// Create two holder links
	holderLink1 := createTestHolderLink(holderID, uuid.New(), string(mmodel.LinkTypePrimaryHolder))
	_, err := repo.Create(ctx, organizationID, holderLink1)
	require.NoError(t, err)

	holderLink2 := createTestHolderLink(holderID, uuid.New(), string(mmodel.LinkTypeLegalRepresentative))
	_, err = repo.Create(ctx, organizationID, holderLink2)
	require.NoError(t, err)

	// Soft delete one
	err = repo.Delete(ctx, organizationID, *holderLink1.ID, false)
	require.NoError(t, err)

	// Act
	results, err := repo.FindByHolderID(ctx, organizationID, holderID, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 1, "should exclude soft-deleted holder link")
	assert.Equal(t, holderLink2.ID, results[0].ID)
}

func TestIntegration_HolderLinkRepo_FindByHolderID_ReturnsEmpty(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-byholderempty-" + uuid.New().String()[:8]
	nonExistentHolderID := uuid.New()

	// Act
	results, err := repo.FindByHolderID(ctx, organizationID, nonExistentHolderID, false)

	// Assert
	require.NoError(t, err, "should not error on empty result")
	assert.Empty(t, results)
}

// ============================================================================
// FindByAliasIDAndLinkType Tests
// ============================================================================

func TestIntegration_HolderLinkRepo_FindByAliasIDAndLinkType(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-byaliastype-" + uuid.New().String()[:8]
	aliasID := uuid.New()

	// Create holder links with different link types
	holderLink1 := createTestHolderLink(uuid.New(), aliasID, string(mmodel.LinkTypePrimaryHolder))
	_, err := repo.Create(ctx, organizationID, holderLink1)
	require.NoError(t, err)

	holderLink2 := createTestHolderLink(uuid.New(), aliasID, string(mmodel.LinkTypeLegalRepresentative))
	_, err = repo.Create(ctx, organizationID, holderLink2)
	require.NoError(t, err)

	// Act - Find specific link type
	result, err := repo.FindByAliasIDAndLinkType(ctx, organizationID, aliasID, string(mmodel.LinkTypeLegalRepresentative), false)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, holderLink2.ID, result.ID)
	assert.Equal(t, string(mmodel.LinkTypeLegalRepresentative), *result.LinkType)
}

func TestIntegration_HolderLinkRepo_FindByAliasIDAndLinkType_NotFound(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-byaliastypenotfound-" + uuid.New().String()[:8]
	aliasID := uuid.New()

	// Create holder link with different link type
	holderLink := createTestHolderLink(uuid.New(), aliasID, string(mmodel.LinkTypePrimaryHolder))
	_, err := repo.Create(ctx, organizationID, holderLink)
	require.NoError(t, err)

	// Act - Find link type that doesn't exist
	result, err := repo.FindByAliasIDAndLinkType(ctx, organizationID, aliasID, string(mmodel.LinkTypeLegalRepresentative), false)

	// Assert
	require.NoError(t, err, "should not error when not found")
	assert.Nil(t, result, "should return nil when not found")
}

func TestIntegration_HolderLinkRepo_FindByAliasIDAndLinkType_ExcludesDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-byaliastypedel-" + uuid.New().String()[:8]
	aliasID := uuid.New()

	holderLink := createTestHolderLink(uuid.New(), aliasID, string(mmodel.LinkTypeLegalRepresentative))
	_, err := repo.Create(ctx, organizationID, holderLink)
	require.NoError(t, err)

	// Soft delete
	err = repo.Delete(ctx, organizationID, *holderLink.ID, false)
	require.NoError(t, err)

	// Act
	result, err := repo.FindByAliasIDAndLinkType(ctx, organizationID, aliasID, string(mmodel.LinkTypeLegalRepresentative), false)

	// Assert
	require.NoError(t, err)
	assert.Nil(t, result, "should not find soft-deleted holder link")
}

// ============================================================================
// FindAll Tests
// ============================================================================

func TestIntegration_HolderLinkRepo_FindAll(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-findall-" + uuid.New().String()[:8]

	// Create multiple holder links
	for i := 0; i < 5; i++ {
		holderLink := createTestHolderLink(uuid.New(), uuid.New(), string(mmodel.LinkTypeLegalRepresentative))
		_, err := repo.Create(ctx, organizationID, holderLink)
		require.NoError(t, err)
	}

	// Act
	filter := http.QueryHeader{Limit: 10, Page: 1}
	results, err := repo.FindAll(ctx, organizationID, filter, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 5, "should return all 5 holder links")
}

func TestIntegration_HolderLinkRepo_FindAll_Pagination(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-page-" + uuid.New().String()[:8]

	// Create 5 holder links
	for i := 0; i < 5; i++ {
		holderLink := createTestHolderLink(uuid.New(), uuid.New(), string(mmodel.LinkTypeLegalRepresentative))
		_, err := repo.Create(ctx, organizationID, holderLink)
		require.NoError(t, err)
	}

	// Act - Get page 1 (limit 2)
	page1, err := repo.FindAll(ctx, organizationID, http.QueryHeader{Limit: 2, Page: 1}, false)
	require.NoError(t, err)

	// Act - Get page 2
	page2, err := repo.FindAll(ctx, organizationID, http.QueryHeader{Limit: 2, Page: 2}, false)
	require.NoError(t, err)

	// Act - Get page 3
	page3, err := repo.FindAll(ctx, organizationID, http.QueryHeader{Limit: 2, Page: 3}, false)
	require.NoError(t, err)

	// Assert
	assert.Len(t, page1, 2, "page 1 should have 2 items")
	assert.Len(t, page2, 2, "page 2 should have 2 items")
	assert.Len(t, page3, 1, "page 3 should have 1 item")

	// Verify no duplicates
	allIDs := make(map[uuid.UUID]bool)
	for _, r := range append(append(page1, page2...), page3...) {
		assert.False(t, allIDs[*r.ID], "should not have duplicates")
		allIDs[*r.ID] = true
	}
	assert.Len(t, allIDs, 5)
}

func TestIntegration_HolderLinkRepo_FindAll_FilterByHolderID(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-filterholder-" + uuid.New().String()[:8]
	targetHolderID := uuid.New()
	targetHolderIDStr := targetHolderID.String()

	// Create holder links with different holders
	holderLink1 := createTestHolderLink(targetHolderID, uuid.New(), string(mmodel.LinkTypeLegalRepresentative))
	_, err := repo.Create(ctx, organizationID, holderLink1)
	require.NoError(t, err)

	holderLink2 := createTestHolderLink(targetHolderID, uuid.New(), string(mmodel.LinkTypePrimaryHolder))
	_, err = repo.Create(ctx, organizationID, holderLink2)
	require.NoError(t, err)

	holderLink3 := createTestHolderLink(uuid.New(), uuid.New(), string(mmodel.LinkTypeLegalRepresentative))
	_, err = repo.Create(ctx, organizationID, holderLink3)
	require.NoError(t, err)

	// Act
	filter := http.QueryHeader{
		Limit:    10,
		Page:     1,
		HolderID: &targetHolderIDStr,
	}
	results, err := repo.FindAll(ctx, organizationID, filter, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 2, "should return only holder links for target holder")
	for _, hl := range results {
		assert.Equal(t, targetHolderID, *hl.HolderID)
	}
}

func TestIntegration_HolderLinkRepo_FindAll_FilterByMetadata(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-filtermeta-" + uuid.New().String()[:8]

	// Create holder link with specific metadata
	holderLink1 := createTestHolderLink(uuid.New(), uuid.New(), string(mmodel.LinkTypeLegalRepresentative))
	holderLink1.Metadata = map[string]any{"region": "us-east", "priority": "high"}
	_, err := repo.Create(ctx, organizationID, holderLink1)
	require.NoError(t, err)

	holderLink2 := createTestHolderLink(uuid.New(), uuid.New(), string(mmodel.LinkTypeLegalRepresentative))
	holderLink2.Metadata = map[string]any{"region": "eu-west", "priority": "low"}
	_, err = repo.Create(ctx, organizationID, holderLink2)
	require.NoError(t, err)

	// Act - Filter by metadata.region
	filter := http.QueryHeader{
		Limit: 10,
		Page:  1,
		Metadata: &bson.M{
			"metadata.region": "us-east",
		},
	}
	results, err := repo.FindAll(ctx, organizationID, filter, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 1, "should return only holder link with matching metadata")
	assert.Equal(t, holderLink1.ID, results[0].ID)
}

func TestIntegration_HolderLinkRepo_FindAll_ReturnsEmpty(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-empty-" + uuid.New().String()[:8]

	// Act - Query empty collection
	filter := http.QueryHeader{Limit: 10, Page: 1}
	results, err := repo.FindAll(ctx, organizationID, filter, false)

	// Assert
	require.NoError(t, err, "should not error on empty result")
	assert.Empty(t, results)
}

func TestIntegration_HolderLinkRepo_FindAll_ExcludesDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-findalldel-" + uuid.New().String()[:8]

	// Create holder links
	holderLink1 := createTestHolderLink(uuid.New(), uuid.New(), string(mmodel.LinkTypeLegalRepresentative))
	_, err := repo.Create(ctx, organizationID, holderLink1)
	require.NoError(t, err)

	holderLink2 := createTestHolderLink(uuid.New(), uuid.New(), string(mmodel.LinkTypePrimaryHolder))
	_, err = repo.Create(ctx, organizationID, holderLink2)
	require.NoError(t, err)

	// Soft delete one
	err = repo.Delete(ctx, organizationID, *holderLink1.ID, false)
	require.NoError(t, err)

	// Act
	filter := http.QueryHeader{Limit: 10, Page: 1}
	results, err := repo.FindAll(ctx, organizationID, filter, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 1, "should exclude soft-deleted holder link")
	assert.Equal(t, holderLink2.ID, results[0].ID)
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_HolderLinkRepo_Update(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-update-" + uuid.New().String()[:8]
	holderID := uuid.New()
	aliasID := uuid.New()

	holderLink := createTestHolderLink(holderID, aliasID, string(mmodel.LinkTypeLegalRepresentative))
	_, err := repo.Create(ctx, organizationID, holderLink)
	require.NoError(t, err)

	// Act - Update metadata
	updatedHolderLink := &mmodel.HolderLink{
		Metadata: map[string]any{"updated": true, "version": 2},
	}
	result, err := repo.Update(ctx, organizationID, *holderLink.ID, updatedHolderLink, nil)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, true, result.Metadata["updated"])
	assert.EqualValues(t, 2, result.Metadata["version"]) // BSON preserves int types, use EqualValues for type-agnostic comparison
}

func TestIntegration_HolderLinkRepo_Update_NotFound(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-upnotfound-" + uuid.New().String()[:8]
	nonExistentID := uuid.New()

	// Act
	updatedHolderLink := &mmodel.HolderLink{
		Metadata: map[string]any{"key": "value"},
	}
	result, err := repo.Update(ctx, organizationID, nonExistentID, updatedHolderLink, nil)

	// Assert
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "holder link", "should return ErrHolderLinkNotFound")
}

func TestIntegration_HolderLinkRepo_Update_FieldsToRemove(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-remove-" + uuid.New().String()[:8]
	holderID := uuid.New()
	aliasID := uuid.New()

	holderLink := createTestHolderLink(holderID, aliasID, string(mmodel.LinkTypeLegalRepresentative))
	holderLink.Metadata = map[string]any{"key1": "value1", "key2": "value2", "key3": "value3"}
	_, err := repo.Create(ctx, organizationID, holderLink)
	require.NoError(t, err)

	// Act - Remove metadata.key1
	result, err := repo.Update(ctx, organizationID, *holderLink.ID, &mmodel.HolderLink{}, []string{"metadata.key1"})

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	_, hasKey1 := result.Metadata["key1"]
	assert.False(t, hasKey1, "key1 should be removed")
	assert.Equal(t, "value2", result.Metadata["key2"], "key2 should still exist")
	assert.Equal(t, "value3", result.Metadata["key3"], "key3 should still exist")
}

func TestIntegration_HolderLinkRepo_Update_CannotUpdateDeletedRecord(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-updel-" + uuid.New().String()[:8]
	holderID := uuid.New()
	aliasID := uuid.New()

	holderLink := createTestHolderLink(holderID, aliasID, string(mmodel.LinkTypeLegalRepresentative))
	_, err := repo.Create(ctx, organizationID, holderLink)
	require.NoError(t, err)

	// Soft delete
	err = repo.Delete(ctx, organizationID, *holderLink.ID, false)
	require.NoError(t, err)

	// Act - Try to update deleted record
	updatedHolderLink := &mmodel.HolderLink{
		Metadata: map[string]any{"key": "value"},
	}
	result, err := repo.Update(ctx, organizationID, *holderLink.ID, updatedHolderLink, nil)

	// Assert
	require.Error(t, err, "should not be able to update soft-deleted record")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "holder link")
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestIntegration_HolderLinkRepo_Delete_Soft(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-softdel-" + uuid.New().String()[:8]
	holderID := uuid.New()
	aliasID := uuid.New()

	holderLink := createTestHolderLink(holderID, aliasID, string(mmodel.LinkTypeLegalRepresentative))
	_, err := repo.Create(ctx, organizationID, holderLink)
	require.NoError(t, err)

	// Act - Soft delete
	err = repo.Delete(ctx, organizationID, *holderLink.ID, false)

	// Assert
	require.NoError(t, err)

	// Verify document still exists with deleted_at set
	result, err := repo.Find(ctx, organizationID, *holderLink.ID, true)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.DeletedAt, "deleted_at should be set")
}

func TestIntegration_HolderLinkRepo_Delete_Hard(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-harddel-" + uuid.New().String()[:8]
	holderID := uuid.New()
	aliasID := uuid.New()

	holderLink := createTestHolderLink(holderID, aliasID, string(mmodel.LinkTypeLegalRepresentative))
	_, err := repo.Create(ctx, organizationID, holderLink)
	require.NoError(t, err)

	// Act - Hard delete
	err = repo.Delete(ctx, organizationID, *holderLink.ID, true)

	// Assert
	require.NoError(t, err)

	// Verify document is completely removed
	collName := strings.ToLower("holder_links_" + organizationID)
	count := mongotestutil.CountDocuments(t, container.Database, collName, bson.M{"_id": holderLink.ID})
	assert.Equal(t, int64(0), count, "document should be removed from collection")
}

func TestIntegration_HolderLinkRepo_Delete_NotFound(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-delnotfound-" + uuid.New().String()[:8]
	nonExistentID := uuid.New()

	// Act
	err := repo.Delete(ctx, organizationID, nonExistentID, false)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "holder link", "should return ErrHolderLinkNotFound")
}

func TestIntegration_HolderLinkRepo_Delete_AlreadyDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-delalready-" + uuid.New().String()[:8]
	holderID := uuid.New()
	aliasID := uuid.New()

	holderLink := createTestHolderLink(holderID, aliasID, string(mmodel.LinkTypeLegalRepresentative))
	_, err := repo.Create(ctx, organizationID, holderLink)
	require.NoError(t, err)

	// Soft delete first time
	err = repo.Delete(ctx, organizationID, *holderLink.ID, false)
	require.NoError(t, err)

	// Act - Try to soft delete again
	err = repo.Delete(ctx, organizationID, *holderLink.ID, false)

	// Assert
	require.Error(t, err, "should not be able to delete already deleted record")
	assert.Contains(t, err.Error(), "holder link")
}
