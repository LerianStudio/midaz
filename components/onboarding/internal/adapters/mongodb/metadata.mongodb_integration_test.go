//go:build integration

package mongodb

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	mongotestutil "github.com/LerianStudio/midaz/v3/pkg/testutils/mongodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// ============================================================================
// Test Helpers
// ============================================================================

func createRepository(t *testing.T, container *mongotestutil.ContainerResult) *MetadataMongoDBRepository {
	t.Helper()

	conn := mongotestutil.CreateConnection(t, container.URI, container.DBName)

	return &MetadataMongoDBRepository{
		connection: conn,
		Database:   container.DBName,
	}
}

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_MetadataRepository_Create_InsertsMetadata(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	metadata := &Metadata{
		EntityID:   "acc-123",
		EntityName: "Account",
		Data:       map[string]any{"group": "treasury", "priority": "high"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Act
	err := repo.Create(ctx, collection, metadata)

	// Assert
	require.NoError(t, err, "Create should not return error")

	// Verify via direct query
	count := mongotestutil.CountDocuments(t, container.Database, strings.ToLower(collection), bson.M{"entity_id": "acc-123"})
	assert.Equal(t, int64(1), count, "should have exactly 1 document")
}

// ============================================================================
// FindList Tests
// ============================================================================

func TestIntegration_MetadataRepository_FindList_FiltersbyMetadata(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	// Insert test data with different groups
	fixtures := []mongotestutil.MetadataFixture{
		{EntityID: "acc-1", EntityName: "Account", Data: map[string]any{"group": "cash"}},
		{EntityID: "acc-2", EntityName: "Account", Data: map[string]any{"group": "cash"}},
		{EntityID: "acc-3", EntityName: "Account", Data: map[string]any{"group": "ops"}},
		{EntityID: "acc-4", EntityName: "Account", Data: map[string]any{"group": "treasury"}},
	}
	mongotestutil.InsertManyMetadata(t, container.Database, strings.ToLower(collection), fixtures)

	// Filter for group=cash
	metadataFilter := bson.M{"metadata.group": "cash"}
	filter := http.QueryHeader{
		Metadata:    &metadataFilter,
		UseMetadata: true,
		Limit:       10,
		Page:        1,
	}

	// Act
	results, err := repo.FindList(ctx, collection, filter)

	// Assert
	require.NoError(t, err, "FindList should not return error")
	assert.Len(t, results, 2, "should return exactly 2 accounts with group=cash")

	for _, r := range results {
		group, ok := r.Data["group"].(string)
		require.True(t, ok, "group should be a string")
		assert.Equal(t, "cash", group, "all results should have group=cash")
	}
}

func TestIntegration_MetadataRepository_FindList_SupportsPagination(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	// Insert 5 accounts with same group
	fixtures := make([]mongotestutil.MetadataFixture, 5)
	for i := 0; i < 5; i++ {
		fixtures[i] = mongotestutil.MetadataFixture{
			EntityID:   "acc-page-" + string(rune('a'+i)),
			EntityName: "Account",
			Data:       map[string]any{"group": "paginated"},
		}
	}
	mongotestutil.InsertManyMetadata(t, container.Database, strings.ToLower(collection), fixtures)

	metadataFilter := bson.M{"metadata.group": "paginated"}

	// Act - Get first page (limit 2)
	filter := http.QueryHeader{
		Metadata:    &metadataFilter,
		UseMetadata: true,
		Limit:       2,
		Page:        1,
	}
	page1, err := repo.FindList(ctx, collection, filter)
	require.NoError(t, err)

	// Act - Get second page
	filter.Page = 2
	page2, err := repo.FindList(ctx, collection, filter)
	require.NoError(t, err)

	// Act - Get third page
	filter.Page = 3
	page3, err := repo.FindList(ctx, collection, filter)
	require.NoError(t, err)

	// Assert
	assert.Len(t, page1, 2, "page 1 should have 2 items")
	assert.Len(t, page2, 2, "page 2 should have 2 items")
	assert.Len(t, page3, 1, "page 3 should have 1 item")

	// Verify no duplicates across pages
	allIDs := make(map[string]bool)
	for _, r := range append(append(page1, page2...), page3...) {
		assert.False(t, allIDs[r.EntityID], "should not have duplicate entity IDs across pages")
		allIDs[r.EntityID] = true
	}
	assert.Len(t, allIDs, 5, "should have 5 unique entity IDs total")
}

func TestIntegration_MetadataRepository_FindList_ReturnsEmptyForNoMatch(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	// Insert data that won't match
	mongotestutil.InsertMetadata(t, container.Database, strings.ToLower(collection), mongotestutil.MetadataFixture{
		EntityID:   "acc-other",
		EntityName: "Account",
		Data:       map[string]any{"group": "other"},
	})

	metadataFilter := bson.M{"metadata.group": "nonexistent"}
	filter := http.QueryHeader{
		Metadata:    &metadataFilter,
		UseMetadata: true,
		Limit:       10,
		Page:        1,
	}

	// Act
	results, err := repo.FindList(ctx, collection, filter)

	// Assert
	require.NoError(t, err, "FindList should not error on empty result")
	assert.Empty(t, results, "should return empty slice for no matches")
}

// ============================================================================
// FindByEntity Tests
// ============================================================================

func TestIntegration_MetadataRepository_FindByEntity_ReturnsMetadata(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	mongotestutil.InsertMetadata(t, container.Database, strings.ToLower(collection), mongotestutil.MetadataFixture{
		EntityID:   "acc-find-1",
		EntityName: "Account",
		Data:       map[string]any{"key": "value", "number": float64(42)},
	})

	// Act
	result, err := repo.FindByEntity(ctx, collection, "acc-find-1")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "acc-find-1", result.EntityID)
	assert.Equal(t, "value", result.Data["key"])
	assert.Equal(t, float64(42), result.Data["number"])
}

func TestIntegration_MetadataRepository_FindByEntity_ReturnsNilForNonExistent(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)
	ctx := context.Background()

	// Act
	result, err := repo.FindByEntity(ctx, "account", "nonexistent-id")

	// Assert
	require.NoError(t, err, "FindByEntity should not error on missing document")
	assert.Nil(t, result, "should return nil for non-existent entity")
}

// ============================================================================
// FindByEntityIDs Tests
// ============================================================================

func TestIntegration_MetadataRepository_FindByEntityIDs_ReturnsBatch(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	fixtures := []mongotestutil.MetadataFixture{
		{EntityID: "batch-1", EntityName: "Account", Data: map[string]any{"idx": 1}},
		{EntityID: "batch-2", EntityName: "Account", Data: map[string]any{"idx": 2}},
		{EntityID: "batch-3", EntityName: "Account", Data: map[string]any{"idx": 3}},
	}
	mongotestutil.InsertManyMetadata(t, container.Database, strings.ToLower(collection), fixtures)

	// Act - Request only 2 of 3
	results, err := repo.FindByEntityIDs(ctx, collection, []string{"batch-1", "batch-3"})

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 2, "should return exactly 2 metadata entries")

	ids := make(map[string]bool)
	for _, r := range results {
		ids[r.EntityID] = true
	}
	assert.True(t, ids["batch-1"])
	assert.True(t, ids["batch-3"])
	assert.False(t, ids["batch-2"], "batch-2 should not be in results")
}

func TestIntegration_MetadataRepository_FindByEntityIDs_ReturnsEmptyForEmptyInput(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)
	ctx := context.Background()

	// Act
	results, err := repo.FindByEntityIDs(ctx, "account", []string{})

	// Assert
	require.NoError(t, err)
	assert.Empty(t, results, "should return empty slice for empty input")
}

func TestIntegration_MetadataRepository_FindByEntityIDs_HandlesPartialMatch(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	mongotestutil.InsertMetadata(t, container.Database, strings.ToLower(collection), mongotestutil.MetadataFixture{
		EntityID:   "partial-exists",
		EntityName: "Account",
		Data:       map[string]any{"found": true},
	})

	// Act - Request one existing and one non-existing
	results, err := repo.FindByEntityIDs(ctx, collection, []string{"partial-exists", "partial-missing"})

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 1, "should return only existing entries")
	assert.Equal(t, "partial-exists", results[0].EntityID)
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_MetadataRepository_Update_UpdatesExisting(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	mongotestutil.InsertMetadata(t, container.Database, strings.ToLower(collection), mongotestutil.MetadataFixture{
		EntityID:   "update-1",
		EntityName: "Account",
		Data:       map[string]any{"original": true},
	})

	// Act
	err := repo.Update(ctx, collection, "update-1", map[string]any{"updated": true, "newKey": "newValue"})

	// Assert
	require.NoError(t, err, "Update should not return error")

	// Verify
	found, err := repo.FindByEntity(ctx, collection, "update-1")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, true, found.Data["updated"])
	assert.Equal(t, "newValue", found.Data["newKey"])
}

func TestIntegration_MetadataRepository_Update_UpsertsIfNotExists(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	// Act - Update non-existent (should upsert)
	err := repo.Update(ctx, collection, "upsert-new", map[string]any{"created": "via upsert"})

	// Assert
	require.NoError(t, err, "Update should upsert if not exists")

	// Verify
	found, err := repo.FindByEntity(ctx, collection, "upsert-new")
	require.NoError(t, err)
	require.NotNil(t, found, "upserted document should exist")
	assert.Equal(t, "via upsert", found.Data["created"])
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestIntegration_MetadataRepository_Delete_RemovesMetadata(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	mongotestutil.InsertMetadata(t, container.Database, strings.ToLower(collection), mongotestutil.MetadataFixture{
		EntityID:   "delete-1",
		EntityName: "Account",
		Data:       map[string]any{"toDelete": true},
	})

	// Verify exists before delete
	before, _ := repo.FindByEntity(ctx, collection, "delete-1")
	require.NotNil(t, before, "document should exist before delete")

	// Act
	err := repo.Delete(ctx, collection, "delete-1")

	// Assert
	require.NoError(t, err, "Delete should not return error")

	after, err := repo.FindByEntity(ctx, collection, "delete-1")
	require.NoError(t, err)
	assert.Nil(t, after, "document should not exist after delete")
}

func TestIntegration_MetadataRepository_Delete_IsIdempotent(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)
	ctx := context.Background()

	// Act - Delete non-existent (should not error)
	err := repo.Delete(ctx, "account", "never-existed")

	// Assert
	require.NoError(t, err, "Delete should be idempotent - no error for non-existent")
}

// ============================================================================
// Collection Isolation Tests
// ============================================================================

func TestIntegration_MetadataRepository_CollectionIsolation(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	defer container.Cleanup()

	repo := createRepository(t, container)
	ctx := context.Background()

	// Insert same entity_id in different collections
	accountMeta := &Metadata{
		EntityID:   "shared-id",
		EntityName: "Account",
		Data:       map[string]any{"type": "account"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	ledgerMeta := &Metadata{
		EntityID:   "shared-id",
		EntityName: "Ledger",
		Data:       map[string]any{"type": "ledger"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	require.NoError(t, repo.Create(ctx, "Account", accountMeta))
	require.NoError(t, repo.Create(ctx, "Ledger", ledgerMeta))

	// Act & Assert - Each collection should have its own data
	fromAccount, err := repo.FindByEntity(ctx, "Account", "shared-id")
	require.NoError(t, err)
	require.NotNil(t, fromAccount)
	assert.Equal(t, "account", fromAccount.Data["type"])

	fromLedger, err := repo.FindByEntity(ctx, "Ledger", "shared-id")
	require.NoError(t, err)
	require.NotNil(t, fromLedger)
	assert.Equal(t, "ledger", fromLedger.Data["type"])
}
