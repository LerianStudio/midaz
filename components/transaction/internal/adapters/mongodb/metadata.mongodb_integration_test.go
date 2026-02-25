//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	libMongo "github.com/LerianStudio/lib-commons/v3/commons/mongo"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	mongotestutil "github.com/LerianStudio/midaz/v3/tests/utils/mongodb"
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

	// Use constructor to validate connection via GetDB()
	return NewMetadataMongoDBRepository(conn)
}

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_MetadataRepository_Create_InsertsMetadata(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	metadata := &Metadata{
		EntityID:   "txn-123",
		EntityName: "Transaction",
		Data:       map[string]any{"type": "credit", "status": "completed"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Act
	err := repo.Create(ctx, collection, metadata)

	// Assert
	require.NoError(t, err, "Create should not return error")

	// Verify via direct query
	count := mongotestutil.CountDocuments(t, container.Database, strings.ToLower(collection), bson.M{"entity_id": "txn-123"})
	assert.Equal(t, int64(1), count, "should have exactly 1 document")
}

// ============================================================================
// FindList Tests
// ============================================================================

func TestIntegration_MetadataRepository_FindList_FiltersByMetadata(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	// Insert test data with different types
	fixtures := []mongotestutil.MetadataFixture{
		{EntityID: "txn-1", EntityName: "Transaction", Data: map[string]any{"type": "credit"}},
		{EntityID: "txn-2", EntityName: "Transaction", Data: map[string]any{"type": "credit"}},
		{EntityID: "txn-3", EntityName: "Transaction", Data: map[string]any{"type": "debit"}},
		{EntityID: "txn-4", EntityName: "Transaction", Data: map[string]any{"type": "transfer"}},
	}
	mongotestutil.InsertManyMetadata(t, container.Database, strings.ToLower(collection), fixtures)

	// Filter for type=credit
	metadataFilter := bson.M{"metadata.type": "credit"}
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
	assert.Len(t, results, 2, "should return exactly 2 transactions with type=credit")

	for _, r := range results {
		txnType, ok := r.Data["type"].(string)
		require.True(t, ok, "type should be a string")
		assert.Equal(t, "credit", txnType, "all results should have type=credit")
	}
}

func TestIntegration_MetadataRepository_FindList_ReturnsMultipleResults(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	// Insert 5 transactions with same type
	fixtures := make([]mongotestutil.MetadataFixture, 5)
	for i := 0; i < 5; i++ {
		fixtures[i] = mongotestutil.MetadataFixture{
			EntityID:   fmt.Sprintf("txn-multi-%d", i),
			EntityName: "Transaction",
			Data:       map[string]any{"type": "batch"},
		}
	}
	mongotestutil.InsertManyMetadata(t, container.Database, strings.ToLower(collection), fixtures)

	metadataFilter := bson.M{"metadata.type": "batch"}
	filter := http.QueryHeader{
		Metadata:    &metadataFilter,
		UseMetadata: true,
	}

	// Act
	results, err := repo.FindList(ctx, collection, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 5, "should return all 5 matching items")

	// Verify no duplicates
	seenIDs := make(map[string]bool)
	for _, r := range results {
		assert.False(t, seenIDs[r.EntityID], "should not have duplicate entity IDs")
		seenIDs[r.EntityID] = true
		assert.Equal(t, "batch", r.Data["type"], "all results should have type=batch")
	}
}

func TestIntegration_MetadataRepository_FindList_ReturnsEmptyForNoMatch(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	// Insert data that won't match
	mongotestutil.InsertMetadata(t, container.Database, strings.ToLower(collection), mongotestutil.MetadataFixture{
		EntityID:   "txn-other",
		EntityName: "Transaction",
		Data:       map[string]any{"type": "other"},
	})

	metadataFilter := bson.M{"metadata.type": "nonexistent"}
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

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	mongotestutil.InsertMetadata(t, container.Database, strings.ToLower(collection), mongotestutil.MetadataFixture{
		EntityID:   "txn-find-1",
		EntityName: "Transaction",
		Data:       map[string]any{"key": "value", "amount": float64(1000)},
	})

	// Act
	result, err := repo.FindByEntity(ctx, collection, "txn-find-1")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "txn-find-1", result.EntityID)
	assert.Equal(t, "value", result.Data["key"])
	assert.Equal(t, float64(1000), result.Data["amount"])
}

func TestIntegration_MetadataRepository_FindByEntity_ReturnsNilForNonExistent(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	// Act
	result, err := repo.FindByEntity(ctx, "transaction", "nonexistent-id")

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

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	fixtures := []mongotestutil.MetadataFixture{
		{EntityID: "batch-1", EntityName: "Transaction", Data: map[string]any{"idx": 1}},
		{EntityID: "batch-2", EntityName: "Transaction", Data: map[string]any{"idx": 2}},
		{EntityID: "batch-3", EntityName: "Transaction", Data: map[string]any{"idx": 3}},
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

	repo := createRepository(t, container)
	ctx := context.Background()

	// Act
	results, err := repo.FindByEntityIDs(ctx, "transaction", []string{})

	// Assert
	require.NoError(t, err)
	assert.Empty(t, results, "should return empty slice for empty input")
}

func TestIntegration_MetadataRepository_FindByEntityIDs_HandlesPartialMatch(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	mongotestutil.InsertMetadata(t, container.Database, strings.ToLower(collection), mongotestutil.MetadataFixture{
		EntityID:   "partial-exists",
		EntityName: "Transaction",
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

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	mongotestutil.InsertMetadata(t, container.Database, strings.ToLower(collection), mongotestutil.MetadataFixture{
		EntityID:   "update-1",
		EntityName: "Transaction",
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

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

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

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	mongotestutil.InsertMetadata(t, container.Database, strings.ToLower(collection), mongotestutil.MetadataFixture{
		EntityID:   "delete-1",
		EntityName: "Transaction",
		Data:       map[string]any{"toDelete": true},
	})

	// Verify exists before delete
	before, err := repo.FindByEntity(ctx, collection, "delete-1")
	require.NoError(t, err, "FindByEntity should not error during pre-delete verification")
	require.NotNil(t, before, "document should exist before delete")

	// Act
	err = repo.Delete(ctx, collection, "delete-1")

	// Assert
	require.NoError(t, err, "Delete should not return error")

	after, err := repo.FindByEntity(ctx, collection, "delete-1")
	require.NoError(t, err)
	assert.Nil(t, after, "document should not exist after delete")
}

func TestIntegration_MetadataRepository_Delete_IsIdempotent(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	// Act - Delete non-existent (should not error)
	err := repo.Delete(ctx, "transaction", "never-existed")

	// Assert
	require.NoError(t, err, "Delete should be idempotent - no error for non-existent")
}

// ============================================================================
// Collection Isolation Tests
// ============================================================================

func TestIntegration_MetadataRepository_CollectionIsolation(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	// Insert same entity_id in different collections
	transactionMeta := &Metadata{
		EntityID:   "shared-id",
		EntityName: "Transaction",
		Data:       map[string]any{"type": "transaction"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	operationMeta := &Metadata{
		EntityID:   "shared-id",
		EntityName: "Operation",
		Data:       map[string]any{"type": "operation"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	require.NoError(t, repo.Create(ctx, "Transaction", transactionMeta))
	require.NoError(t, repo.Create(ctx, "Operation", operationMeta))

	// Act & Assert - Each collection should have its own data
	fromTransaction, err := repo.FindByEntity(ctx, "Transaction", "shared-id")
	require.NoError(t, err)
	require.NotNil(t, fromTransaction)
	assert.Equal(t, "transaction", fromTransaction.Data["type"])

	fromOperation, err := repo.FindByEntity(ctx, "Operation", "shared-id")
	require.NoError(t, err)
	require.NotNil(t, fromOperation)
	assert.Equal(t, "operation", fromOperation.Data["type"])
}

// ============================================================================
// CreateIndex Tests
// ============================================================================

func TestIntegration_MetadataRepository_CreateIndex(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	input := &mmodel.CreateMetadataIndexInput{
		MetadataKey: "status",
		Unique:      false,
		Sparse:      nil, // default to true
	}

	// Act
	result, err := repo.CreateIndex(ctx, collection, input)

	// Assert
	require.NoError(t, err, "CreateIndex should not return error")
	require.NotNil(t, result)
	assert.Equal(t, "metadata.status_1", result.IndexName)
	assert.Equal(t, collection, result.EntityName)
	assert.Equal(t, "status", result.MetadataKey)
	assert.False(t, result.Unique)
	assert.True(t, result.Sparse, "sparse should default to true")

	// Verify index exists via FindAllIndexes
	indexes, err := repo.FindAllIndexes(ctx, collection)
	require.NoError(t, err)

	found := false
	for _, idx := range indexes {
		if idx.IndexName == "metadata.status_1" {
			found = true
			assert.Equal(t, "status", idx.MetadataKey)
			break
		}
	}
	assert.True(t, found, "created index should be found in FindAllIndexes")
}

func TestIntegration_MetadataRepository_CreateIndex_Unique(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	sparse := false
	input := &mmodel.CreateMetadataIndexInput{
		MetadataKey: "uniqueKey",
		Unique:      true,
		Sparse:      &sparse,
	}

	// Act
	result, err := repo.CreateIndex(ctx, collection, input)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Unique, "index should be unique")
	assert.False(t, result.Sparse, "sparse should be false as specified")

	// Verify unique constraint works - insert duplicate metadata
	meta1 := &Metadata{
		EntityID:   "txn-1",
		EntityName: "Transaction",
		Data:       map[string]any{"uniqueKey": "duplicate-value"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	meta2 := &Metadata{
		EntityID:   "txn-2",
		EntityName: "Transaction",
		Data:       map[string]any{"uniqueKey": "duplicate-value"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err = repo.Create(ctx, collection, meta1)
	require.NoError(t, err, "first insert should succeed")

	err = repo.Create(ctx, collection, meta2)
	require.Error(t, err, "second insert with duplicate unique key should fail")
	assert.Contains(t, err.Error(), "duplicate key", "error should indicate duplicate key violation")
}

func TestIntegration_MetadataRepository_CreateIndex_DuplicateIndex(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	input := &mmodel.CreateMetadataIndexInput{
		MetadataKey: "duplicateTest",
		Unique:      false,
	}

	// Act - Create first time
	result1, err := repo.CreateIndex(ctx, collection, input)
	require.NoError(t, err)
	require.NotNil(t, result1)

	// Act - Create same index again (MongoDB is idempotent for identical indexes)
	result2, err := repo.CreateIndex(ctx, collection, input)

	// Assert - MongoDB allows creating the same index again (idempotent)
	require.NoError(t, err, "creating identical index should be idempotent")
	require.NotNil(t, result2)
	assert.Equal(t, result1.IndexName, result2.IndexName)
}

// ============================================================================
// FindAllIndexes Tests
// ============================================================================

func TestIntegration_MetadataRepository_FindAllIndexes(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	// Create multiple indexes
	_, err := repo.CreateIndex(ctx, collection, &mmodel.CreateMetadataIndexInput{
		MetadataKey: "type",
		Unique:      false,
	})
	require.NoError(t, err)

	_, err = repo.CreateIndex(ctx, collection, &mmodel.CreateMetadataIndexInput{
		MetadataKey: "priority",
		Unique:      true,
	})
	require.NoError(t, err)

	// Insert some data to generate index usage stats
	for i := 0; i < 3; i++ {
		meta := &Metadata{
			EntityID:   fmt.Sprintf("txn-%d", i),
			EntityName: "Transaction",
			Data:       map[string]any{"type": "test", "priority": fmt.Sprintf("p%d", i)},
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		require.NoError(t, repo.Create(ctx, collection, meta))
	}

	// Act
	indexes, err := repo.FindAllIndexes(ctx, collection)

	// Assert
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(indexes), 2, "should have at least 2 metadata indexes")

	// Verify we get the expected indexes
	indexNames := make(map[string]bool)
	for _, idx := range indexes {
		indexNames[idx.MetadataKey] = true
		// All indexes should have stats
		assert.NotNil(t, idx.Stats, "index %s should have stats", idx.IndexName)
		assert.NotNil(t, idx.Stats.StatsSince, "index %s should have StatsSince", idx.IndexName)
	}

	assert.True(t, indexNames["type"], "should find 'type' index")
	assert.True(t, indexNames["priority"], "should find 'priority' index")
}

func TestIntegration_MetadataRepository_FindAllIndexes_Empty(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "emptyCollection"

	// Act - Query collection with no custom metadata indexes
	indexes, err := repo.FindAllIndexes(ctx, collection)

	// Assert
	require.NoError(t, err, "FindAllIndexes should not error on collection with no metadata indexes")
	assert.Empty(t, indexes, "should return empty slice when no metadata indexes exist")
}

func TestIntegration_MetadataRepository_FindAllIndexes_FiltersMetadataOnly(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	// Create a metadata index
	_, err := repo.CreateIndex(ctx, collection, &mmodel.CreateMetadataIndexInput{
		MetadataKey: "filterTest",
		Unique:      false,
	})
	require.NoError(t, err)

	// Note: MongoDB automatically creates _id index, which should NOT appear in results

	// Act
	indexes, err := repo.FindAllIndexes(ctx, collection)

	// Assert
	require.NoError(t, err)

	for _, idx := range indexes {
		// All returned indexes should have metadata key (not _id or other system indexes)
		assert.NotEmpty(t, idx.MetadataKey, "all indexes should have MetadataKey set")
		assert.True(t, strings.HasPrefix(idx.IndexName, "metadata."),
			"index name %s should start with 'metadata.'", idx.IndexName)
	}
}

// ============================================================================
// DeleteIndex Tests
// ============================================================================

func TestIntegration_MetadataRepository_DeleteIndex(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	// Create an index first
	result, err := repo.CreateIndex(ctx, collection, &mmodel.CreateMetadataIndexInput{
		MetadataKey: "toDelete",
		Unique:      false,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify it exists
	indexes, err := repo.FindAllIndexes(ctx, collection)
	require.NoError(t, err)

	found := false
	for _, idx := range indexes {
		if idx.IndexName == result.IndexName {
			found = true
			break
		}
	}
	require.True(t, found, "index should exist before deletion")

	// Act
	err = repo.DeleteIndex(ctx, collection, result.IndexName)

	// Assert
	require.NoError(t, err, "DeleteIndex should not return error")

	// Verify it no longer exists
	indexes, err = repo.FindAllIndexes(ctx, collection)
	require.NoError(t, err)

	found = false
	for _, idx := range indexes {
		if idx.IndexName == result.IndexName {
			found = true
			break
		}
	}
	assert.False(t, found, "index should not exist after deletion")
}

func TestIntegration_MetadataRepository_DeleteIndex_NotFound(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	// First, ensure collection exists by creating a document (MongoDB returns NamespaceNotFound
	// if collection doesn't exist, but we want to test IndexNotFound specifically)
	meta := &Metadata{
		EntityID:   "setup-doc",
		EntityName: "Transaction",
		Data:       map[string]any{"setup": true},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, repo.Create(ctx, collection, meta))

	// Act - Delete non-existent index on existing collection
	err := repo.DeleteIndex(ctx, collection, "metadata.nonexistent_1")

	// Assert - Should return EntityNotFoundError mapped from IndexNotFound
	require.Error(t, err, "DeleteIndex should error for non-existent index")
	assert.Contains(t, err.Error(), "metadata index does not exist", "error should indicate index not found")
}

// ============================================================================
// CHAOS TEST HELPERS
// ============================================================================

// skipIfNotChaos skips the test if CHAOS=1 environment variable is not set.
// Use this for tests that inject failures (network chaos, container restarts, etc.)
func skipIfNotChaos(t *testing.T) {
	t.Helper()
	if os.Getenv("CHAOS") != "1" {
		t.Skip("skipping chaos test (set CHAOS=1 to run)")
	}
}

// ============================================================================
// CHAOS TEST INFRASTRUCTURE
// ============================================================================

// chaosTestInfra holds the infrastructure for chaos tests (container restart, etc.).
type chaosTestInfra struct {
	container  *mongotestutil.ContainerResult
	repo       *MetadataMongoDBRepository
	chaosOrch  *chaos.Orchestrator
	collection string
}

// networkChaosTestInfra holds infrastructure for network chaos tests with Toxiproxy.
type networkChaosTestInfra struct {
	chaosInfra  *chaos.Infrastructure
	mongoResult *mongotestutil.ContainerResult
	conn        *libMongo.MongoConnection
	repo        *MetadataMongoDBRepository
	proxy       *chaos.Proxy
	collection  string
}

// setupChaosInfra sets up the test infrastructure for chaos testing (container restart).
func setupChaosInfra(t *testing.T) *chaosTestInfra {
	t.Helper()

	// Setup MongoDB container
	container := mongotestutil.SetupContainer(t)

	// Create repository using constructor (validates connection via GetDB())
	conn := mongotestutil.CreateConnection(t, container.URI, container.DBName)
	repo := NewMetadataMongoDBRepository(conn)

	// Create chaos orchestrator
	chaosOrch := chaos.NewOrchestrator(t)

	return &chaosTestInfra{
		container:  container,
		repo:       repo,
		chaosOrch:  chaosOrch,
		collection: "chaos_test",
	}
}

// setupNetworkChaosInfra sets up the infrastructure with Toxiproxy for network chaos testing.
func setupNetworkChaosInfra(t *testing.T) *networkChaosTestInfra {
	t.Helper()

	// Create chaos infrastructure with Toxiproxy
	chaosInfra := chaos.NewInfrastructure(t)

	// Setup MongoDB container
	mongoResult := mongotestutil.SetupContainer(t)

	// Register the container with chaos infrastructure
	_, err := chaosInfra.RegisterContainerWithPort("mongodb", mongoResult.Container, "27017/tcp")
	require.NoError(t, err, "failed to register MongoDB container")

	// Create proxy for MongoDB using an exposed Toxiproxy port (8666)
	proxy, err := chaosInfra.CreateProxyFor("mongodb", "8666/tcp")
	require.NoError(t, err, "failed to create Toxiproxy proxy for MongoDB")

	// Get proxy address for client connections
	containerInfo, ok := chaosInfra.GetContainer("mongodb")
	require.True(t, ok, "MongoDB container should be registered")
	require.NotEmpty(t, containerInfo.ProxyListen, "proxy address should be set")

	// Create lib-commons MongoDB connection through proxy
	logger := libZap.InitializeLogger()
	proxyURI := "mongodb://" + containerInfo.ProxyListen

	conn := &libMongo.MongoConnection{
		ConnectionStringSource: proxyURI,
		Database:               mongoResult.DBName,
		Logger:                 logger,
	}

	// Create repository (uses constructor to validate connection via GetDB())
	repo := NewMetadataMongoDBRepository(conn)

	return &networkChaosTestInfra{
		chaosInfra:  chaosInfra,
		mongoResult: mongoResult,
		conn:        conn,
		repo:        repo,
		proxy:       proxy,
		collection:  "network_chaos_test",
	}
}

// cleanup releases all resources for chaos tests.
func (infra *chaosTestInfra) cleanup() {
	if infra.chaosOrch != nil {
		infra.chaosOrch.Close()
	}
}

// cleanup releases all resources for network chaos infrastructure.
func (infra *networkChaosTestInfra) cleanup() {
	// Cleanup Infrastructure (Toxiproxy, network, orchestrator)
	if infra.chaosInfra != nil {
		infra.chaosInfra.Cleanup()
	}
}

// createTestMetadata creates a metadata document for chaos testing.
func (infra *chaosTestInfra) createTestMetadata(t *testing.T, entityID, description string) *Metadata {
	t.Helper()

	metadata := &Metadata{
		EntityID:   entityID,
		EntityName: "ChaosTest",
		Data:       map[string]any{"description": description, "timestamp": time.Now().Unix()},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err := infra.repo.Create(context.Background(), infra.collection, metadata)
	require.NoError(t, err)
	return metadata
}

// createTestMetadata creates a metadata document for network chaos testing.
func (infra *networkChaosTestInfra) createTestMetadata(t *testing.T, entityID, description string) *Metadata {
	t.Helper()

	metadata := &Metadata{
		EntityID:   entityID,
		EntityName: "NetworkChaosTest",
		Data:       map[string]any{"description": description, "timestamp": time.Now().Unix()},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err := infra.repo.Create(context.Background(), infra.collection, metadata)
	require.NoError(t, err)
	return metadata
}

// ============================================================================
// CHAOS TESTS - DATA INTEGRITY
// ============================================================================

// TestIntegration_Metadata_DataIntegrity is a baseline test that verifies data
// remains consistent (no data loss, no corruption) under normal conditions.
// This serves as a control for chaos tests - it validates the assertions work
// without fault injection.
func TestIntegration_Metadata_DataIntegrity(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping integrity test in short mode")
	}

	infra := setupChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Create multiple metadata documents
	var createdMetadata []*Metadata
	for i := 0; i < 5; i++ {
		metadata := infra.createTestMetadata(t, fmt.Sprintf("integrity-test-%d", i), "Integrity test metadata")
		createdMetadata = append(createdMetadata, metadata)
	}

	t.Logf("Created %d metadata documents", len(createdMetadata))

	// Verify all data is intact (baseline - no chaos injected)
	chaos.AssertDataIntegrity(t, func() error {
		for _, m := range createdMetadata {
			_, err := infra.repo.FindByEntity(ctx, infra.collection, m.EntityID)
			if err != nil {
				return err
			}
		}
		return nil
	}, "all metadata should be retrievable")

	// Verify each metadata document's data
	for _, expected := range createdMetadata {
		actual, err := infra.repo.FindByEntity(ctx, infra.collection, expected.EntityID)
		require.NoError(t, err)
		require.NotNil(t, actual)
		chaos.AssertNoDataLoss(t, expected.EntityID, actual.EntityID, "entity ID mismatch")
		chaos.AssertNoDataLoss(t, expected.Data["description"], actual.Data["description"], "description mismatch")
	}

	t.Log("Baseline integrity test passed: data consistency verified")
}

// ============================================================================
// CHAOS TESTS - NETWORK CHAOS
// ============================================================================

// TestChaos_Metadata_NetworkLatency tests that the repository handles
// network latency gracefully without timing out inappropriately.
func TestIntegration_Chaos_Metadata_NetworkLatency(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()
	t.Logf("Using Toxiproxy proxy: %s -> %s", infra.proxy.Listen(), infra.proxy.Upstream())

	// Create metadata before adding latency
	metadata := infra.createTestMetadata(t, "latency-test-1", "Pre-latency metadata")
	t.Logf("Created metadata %s before adding latency", metadata.EntityID)

	// Add 200ms latency to the connection
	t.Log("Chaos: Adding 200ms network latency")
	err := infra.proxy.AddLatency(200*time.Millisecond, 50*time.Millisecond)
	require.NoError(t, err, "failed to add latency")
	defer infra.proxy.RemoveAllToxics()

	// Operations should still succeed (with higher latency)
	start := time.Now()
	found, err := infra.repo.FindByEntity(ctx, infra.collection, metadata.EntityID)
	elapsed := time.Since(start)

	require.NoError(t, err, "operation should succeed despite latency")
	require.NotNil(t, found)
	assert.Equal(t, metadata.EntityID, found.EntityID)
	t.Logf("Query completed in %v (with 200ms injected latency)", elapsed)

	// Latency should be noticeable
	assert.Greater(t, elapsed, 150*time.Millisecond, "query should take longer due to injected latency")

	// Create new metadata under latency
	start = time.Now()
	newMetadata := infra.createTestMetadata(t, "latency-test-2", "Under-latency metadata")
	elapsed = time.Since(start)

	require.NotNil(t, newMetadata, "should be able to create metadata under latency")
	t.Logf("Create completed in %v (with 200ms injected latency)", elapsed)

	t.Log("Chaos test passed: network latency handled gracefully")
}

// TestChaos_Metadata_NetworkPartition tests that the repository handles
// network partitions (disconnections) gracefully.
func TestIntegration_Chaos_Metadata_NetworkPartition(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Create metadata before partition
	metadata := infra.createTestMetadata(t, "partition-test-1", "Pre-partition metadata")
	t.Logf("Created metadata %s before partition", metadata.EntityID)

	// Disconnect the proxy (simulate network partition)
	t.Log("Chaos: Disconnecting network (simulating partition)")
	err := infra.proxy.Disconnect()
	require.NoError(t, err, "failed to disconnect proxy")

	// Operations should fail gracefully during partition
	partitionCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, partitionErr := infra.repo.FindByEntity(partitionCtx, infra.collection, metadata.EntityID)
	if partitionErr != nil {
		t.Logf("Operation during partition failed as expected: %v", partitionErr)
	} else {
		t.Log("Operation during partition succeeded (connection pool still had active connections)")
	}

	// Reconnect the proxy
	t.Log("Chaos: Reconnecting network")
	err = infra.proxy.Reconnect()
	require.NoError(t, err, "failed to reconnect proxy")

	// Wait for recovery
	chaos.AssertRecoveryWithin(t, func() error {
		_, err := infra.repo.FindByEntity(ctx, infra.collection, metadata.EntityID)
		return err
	}, 30*time.Second, "repository should recover after network partition")

	// Verify data integrity after partition
	found, err := infra.repo.FindByEntity(ctx, infra.collection, metadata.EntityID)
	require.NoError(t, err, "should find metadata after recovery")
	require.NotNil(t, found)
	assert.Equal(t, metadata.EntityID, found.EntityID)
	assert.Equal(t, metadata.Data["description"], found.Data["description"])

	t.Log("Chaos test passed: network partition handled gracefully")
}

// TestChaos_Metadata_PacketLoss tests that the repository handles
// packet loss gracefully with retries.
func TestIntegration_Chaos_Metadata_PacketLoss(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Create metadata before adding packet loss
	metadata := infra.createTestMetadata(t, "packetloss-test-1", "Pre-packet-loss metadata")
	t.Logf("Created metadata %s before packet loss", metadata.EntityID)

	// Add 10% packet loss
	t.Log("Chaos: Adding 10% packet loss")
	err := infra.proxy.AddPacketLoss(10)
	require.NoError(t, err, "failed to add packet loss")
	defer infra.proxy.RemoveAllToxics()

	// Execute multiple operations - some may fail, but overall should be resilient
	successCount := 0
	errorCount := 0
	totalAttempts := 20

	for i := 0; i < totalAttempts; i++ {
		_, err := infra.repo.FindByEntity(ctx, infra.collection, metadata.EntityID)
		if err != nil {
			errorCount++
		} else {
			successCount++
		}
	}

	t.Logf("Packet loss test: %d/%d operations succeeded", successCount, totalAttempts)

	// Most operations should succeed despite packet loss
	assert.Greater(t, successCount, totalAttempts/2, "majority of operations should succeed despite packet loss")

	t.Log("Chaos test passed: packet loss handled with acceptable success rate")
}

// TestChaos_Metadata_IntermittentFailure tests that the repository handles
// intermittent network failures (flapping connection).
func TestIntegration_Chaos_Metadata_IntermittentFailure(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Create metadata
	metadata := infra.createTestMetadata(t, "intermittent-test-1", "Intermittent test metadata")

	// Simulate intermittent failures with multiple disconnect/reconnect cycles
	cycles := 3
	for i := 0; i < cycles; i++ {
		// Disconnect
		t.Logf("Chaos: Cycle %d - disconnecting", i+1)
		disconnectErr := infra.proxy.Disconnect()
		require.NoError(t, disconnectErr, "failed to disconnect proxy on cycle %d", i+1)
		time.Sleep(500 * time.Millisecond)

		// Reconnect
		t.Logf("Chaos: Cycle %d - reconnecting", i+1)
		reconnectErr := infra.proxy.Reconnect()
		require.NoError(t, reconnectErr, "failed to reconnect proxy on cycle %d", i+1)

		// Wait for recovery and verify
		chaos.AssertRecoveryWithin(t, func() error {
			_, err := infra.repo.FindByEntity(ctx, infra.collection, metadata.EntityID)
			return err
		}, 10*time.Second, "should recover after cycle %d", i+1)
	}

	// Final verification
	found, err := infra.repo.FindByEntity(ctx, infra.collection, metadata.EntityID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, metadata.EntityID, found.EntityID)

	t.Log("Chaos test passed: intermittent failures handled correctly")
}
