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

	libMongo "github.com/LerianStudio/lib-commons/v7/commons/mongo"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/tests/utils/chaos"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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

func TestIntegration_MetadataRepository_Delete_MarksDeletedAtAndPreservesMetadata(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupReusableContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "operationroute"

	mongotestutil.InsertMetadata(t, container.Database, collection, mongotestutil.MetadataFixture{
		EntityID:   "delete-1",
		EntityName: "OperationRoute",
		Data:       map[string]any{"toDelete": true},
	})

	// Act
	err := repo.Delete(ctx, collection, "delete-1")

	// Assert
	require.NoError(t, err, "Delete should not return error")

	stored := findStoredMetadata(t, container.Database, collection, "delete-1")
	require.NotNil(t, stored.DeletedAt, "Delete should stamp deleted_at")
	assert.True(t, stored.DeletedAt.Equal(stored.UpdatedAt), "Delete should stamp updated_at with the same instant")
	assert.Equal(t, true, stored.Data["toDelete"], "Delete should keep the metadata keys")
	assert.Equal(t, int64(1), mongotestutil.CountDocuments(t, container.Database, collection, bson.M{"entity_id": "delete-1"}),
		"Delete should keep the document")

	after, err := repo.FindByEntity(ctx, collection, "delete-1")
	require.NoError(t, err)
	assert.Nil(t, after, "a soft-deleted document should be invisible to FindByEntity")
}

func TestIntegration_MetadataRepository_Delete_NonExistentCreatesNoDocument(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupReusableContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "operationroute"

	// Act
	err := repo.Delete(ctx, collection, "never-existed")

	// Assert
	require.NoError(t, err, "Delete of an entity without metadata should be a no-op")
	assert.Equal(t, int64(0), mongotestutil.CountDocuments(t, container.Database, collection, bson.M{"entity_id": "never-existed"}),
		"Delete must not upsert a document")
}

func TestIntegration_MetadataRepository_Create_OmitsDeletedAt(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupReusableContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "operationroute"
	now := time.Date(2026, time.October, 7, 12, 0, 0, 0, time.UTC)

	// Act
	err := repo.Create(ctx, collection, &Metadata{
		EntityID:   "create-live",
		EntityName: "OperationRoute",
		Data:       map[string]any{"k": "v"},
		CreatedAt:  now,
		UpdatedAt:  now,
	})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(1), mongotestutil.CountDocuments(t, container.Database, collection,
		bson.M{"entity_id": "create-live", "deleted_at": bson.M{"$exists": false}}),
		"Create should write a document without deleted_at")
}

func TestIntegration_MetadataRepository_Create_AfterDeleteDoesNotRevive(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupReusableContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "operationroute"
	now := time.Date(2026, time.October, 7, 12, 0, 0, 0, time.UTC)

	_, err := container.Database.Collection(collection).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "entity_id", Value: 1}}, Options: options.Index().SetUnique(true),
	})
	require.NoError(t, err)

	original := &Metadata{EntityID: "revive-1", EntityName: "OperationRoute", Data: map[string]any{"g41": "probe"}, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.Create(ctx, collection, original))
	require.NoError(t, repo.Delete(ctx, collection, "revive-1"))

	deletedAt := findStoredMetadata(t, container.Database, collection, "revive-1").DeletedAt
	require.NotNil(t, deletedAt)

	// Act
	err = repo.Create(ctx, collection, &Metadata{EntityID: "revive-1", EntityName: "OperationRoute", Data: map[string]any{"g41": "revived"}, CreatedAt: now, UpdatedAt: now})

	// Assert
	require.NoError(t, err, "Create matches the marked document and leaves it untouched")

	stored := findStoredMetadata(t, container.Database, collection, "revive-1")
	require.NotNil(t, stored.DeletedAt, "Create must not clear deleted_at")
	assert.True(t, deletedAt.Equal(*stored.DeletedAt), "Create must not change deleted_at")
	assert.Equal(t, "probe", stored.Data["g41"], "Create must not overwrite the marked metadata")
	assert.Equal(t, int64(1), mongotestutil.CountDocuments(t, container.Database, collection, bson.M{"entity_id": "revive-1"}),
		"Create must not add a second document for the entity")

	found, err := repo.FindByEntity(ctx, collection, "revive-1")
	require.NoError(t, err)
	assert.Nil(t, found, "the entity should stay invisible")
}

func TestIntegration_MetadataRepository_FindList_ExcludesDeletedAndIncludesLegacy(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupReusableContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "operationroute"

	mongotestutil.InsertManyMetadata(t, container.Database, collection, []mongotestutil.MetadataFixture{
		{EntityID: "legacy-1", EntityName: "OperationRoute", Data: map[string]any{"group": "cash"}},
		{EntityID: "deleted-1", EntityName: "OperationRoute", Data: map[string]any{"group": "cash"}},
		{EntityID: "other-1", EntityName: "OperationRoute", Data: map[string]any{"group": "ops"}},
	})

	_, err := container.Database.Collection(collection).InsertOne(ctx, bson.M{
		"entity_id": "explicit-null-1", "entity_name": "OperationRoute", "metadata": bson.M{"group": "cash"}, "deleted_at": nil,
	})
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, collection, "deleted-1"))

	metadataFilter := bson.M{"metadata.group": "cash"}
	filter := http.QueryHeader{
		Metadata: &metadataFilter,
	}

	// Act
	results, err := repo.FindList(ctx, collection, filter)

	// Assert
	require.NoError(t, err)

	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.EntityID)
	}

	assert.ElementsMatch(t, []string{"legacy-1", "explicit-null-1"}, ids,
		"FindList should skip the marked document and keep documents without deleted_at or with a null one")
	assert.Equal(t, bson.M{"metadata.group": "cash"}, metadataFilter, "FindList must not mutate the caller's filter")
}

func TestIntegration_MetadataRepository_FindByEntity_ReturnsNilForDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupReusableContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "operationroute"

	mongotestutil.InsertMetadata(t, container.Database, collection, mongotestutil.MetadataFixture{
		EntityID:   "find-deleted-1",
		EntityName: "OperationRoute",
		Data:       map[string]any{"k": "v"},
	})

	before, err := repo.FindByEntity(ctx, collection, "find-deleted-1")
	require.NoError(t, err)
	require.NotNil(t, before, "a legacy document without deleted_at should be visible")

	require.NoError(t, repo.Delete(ctx, collection, "find-deleted-1"))

	// Act
	after, err := repo.FindByEntity(ctx, collection, "find-deleted-1")

	// Assert
	require.NoError(t, err)
	assert.Nil(t, after, "FindByEntity should return nil for a soft-deleted document")
}

func TestIntegration_MetadataRepository_FindByEntityIDs_OmitsDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupReusableContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "operationroute"

	mongotestutil.InsertManyMetadata(t, container.Database, collection, []mongotestutil.MetadataFixture{
		{EntityID: "batch-1", EntityName: "OperationRoute", Data: map[string]any{"i": 1}},
		{EntityID: "batch-2", EntityName: "OperationRoute", Data: map[string]any{"i": 2}},
		{EntityID: "batch-3", EntityName: "OperationRoute", Data: map[string]any{"i": 3}},
	})

	require.NoError(t, repo.Delete(ctx, collection, "batch-2"))

	// Act
	results, err := repo.FindByEntityIDs(ctx, collection, []string{"batch-1", "batch-2", "batch-3"})

	// Assert
	require.NoError(t, err)

	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.EntityID)
	}

	assert.ElementsMatch(t, []string{"batch-1", "batch-3"}, ids, "FindByEntityIDs should omit the soft-deleted document")
}

func TestIntegration_MetadataRepository_Delete_SecondCallKeepsFirstDeletedAt(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupReusableContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "operationroute"

	mongotestutil.InsertMetadata(t, container.Database, collection, mongotestutil.MetadataFixture{
		EntityID:   "delete-twice-1",
		EntityName: "OperationRoute",
		Data:       map[string]any{"k": "v"},
	})

	require.NoError(t, repo.Delete(ctx, collection, "delete-twice-1"))

	first := findStoredMetadata(t, container.Database, collection, "delete-twice-1")
	require.NotNil(t, first.DeletedAt)

	// Act
	err := repo.Delete(ctx, collection, "delete-twice-1")

	// Assert
	require.NoError(t, err, "a second Delete should be a no-op")

	second := findStoredMetadata(t, container.Database, collection, "delete-twice-1")
	require.NotNil(t, second.DeletedAt)
	assert.True(t, first.DeletedAt.Equal(*second.DeletedAt), "a second Delete must keep the first deleted_at")
	assert.True(t, first.UpdatedAt.Equal(second.UpdatedAt), "a second Delete must keep the first updated_at")
}

func TestIntegration_MetadataRepository_Delete_MarksEveryLiveDuplicate(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupReusableContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "operationroute_duplicates"

	mongotestutil.InsertManyMetadata(t, container.Database, collection, []mongotestutil.MetadataFixture{
		{EntityID: "dup-1", EntityName: "OperationRoute", Data: map[string]any{"copy": "older"}},
		{EntityID: "dup-1", EntityName: "OperationRoute", Data: map[string]any{"copy": "newer"}},
	})

	// Act
	err := repo.Delete(ctx, collection, "dup-1")

	// Assert
	require.NoError(t, err)

	assert.Equal(t, int64(2), mongotestutil.CountDocuments(t, container.Database, collection,
		bson.M{"entity_id": "dup-1", "deleted_at": bson.M{"$type": "date"}}),
		"Delete should mark every live duplicate of the entity")

	found, err := repo.FindByEntity(ctx, collection, "dup-1")
	require.NoError(t, err)
	assert.Nil(t, found, "no duplicate of a deleted entity should stay visible")
}

// findStoredMetadata reads the raw document of an entity, bypassing the repository's deleted_at filter.
func findStoredMetadata(t *testing.T, db *mongo.Database, collection, entityID string) MetadataMongoDBModel {
	t.Helper()

	var stored MetadataMongoDBModel

	err := db.Collection(collection).FindOne(context.Background(), bson.M{"entity_id": entityID}).Decode(&stored)
	require.NoError(t, err, "stored metadata document should exist")

	return stored
}

// ============================================================================
// Collection Isolation Tests
// ============================================================================

func TestIntegration_MetadataRepository_CollectionIsolation(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

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
	container := mongotestutil.SetupReusableContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	// Act - a missing collection lists no indexes, like one without the index
	err := repo.DeleteIndex(ctx, collection, "metadata.nonexistent_1")

	// Assert - the index list has no such name, so the delete answers not found
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
	conn        *libMongo.Client
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
	proxyURI := "mongodb://" + containerInfo.ProxyListen

	conn := mongotestutil.CreateConnection(t, proxyURI, mongoResult.DBName)

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

	// Each operation MUST carry its own deadline. The toxiproxy "timeout" toxic
	// used by AddPacketLoss stalls a fraction of connections indefinitely (it
	// holds the stream open without delivering data until the toxic is removed),
	// and the lib-commons mongo client sets no client-level operation timeout
	// (only ServerSelectionTimeout). With an unbounded context a stalled stream
	// would block forever and time out the whole package. A per-call deadline
	// bounds each attempt: a stalled stream becomes a counted error, a healthy
	// stream succeeds well within the budget. This keeps the resilience invariant
	// (majority succeed) honest while making the test self-bounding regardless of
	// the client's missing operation timeout. See the production finding in the
	// task report: the unbounded mongo client timeout is a real exposure.
	const perOpTimeout = 8 * time.Second

	// Execute multiple operations - some may fail, but overall should be resilient
	successCount := 0
	errorCount := 0
	totalAttempts := 20

	for i := 0; i < totalAttempts; i++ {
		opCtx, opCancel := context.WithTimeout(ctx, perOpTimeout)
		_, err := infra.repo.FindByEntity(opCtx, infra.collection, metadata.EntityID)
		opCancel()

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

// ============================================================================
// Tenant Isolation Tests (Multi-Tenant with Real MongoDB)
// ============================================================================

// TestIntegration_MetadataRepository_TenantIsolation_CreateAndFind verifies that
// two tenants sharing the same MongoDB cluster but using different databases have
// complete data isolation. A document created under tenant A must not be visible
// under tenant B, and vice versa.
func TestIntegration_MetadataRepository_TenantIsolation_CreateAndFind(t *testing.T) {
	// Arrange — single container, two databases, one shared repo with placeholder connection
	container := mongotestutil.SetupReusableContainer(t)

	tenantADB := mongotestutil.CreateOwnedDatabase(t, container)
	tenantBDB := mongotestutil.CreateOwnedDatabase(t, container)

	repo := NewMetadataMongoDBRepository(&libMongo.Client{})

	collection := "operation"

	ctxA := tmcore.ContextWithMB(context.Background(), tenantADB)
	ctxB := tmcore.ContextWithMB(context.Background(), tenantBDB)

	metaA := &Metadata{
		EntityID:   "iso-entity-1",
		EntityName: "Operation",
		Data:       map[string]any{"tenant": "A", "amount": float64(100)},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	metaB := &Metadata{
		EntityID:   "iso-entity-2",
		EntityName: "Operation",
		Data:       map[string]any{"tenant": "B", "amount": float64(200)},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Act — create one entity per tenant
	err := repo.Create(ctxA, collection, metaA)
	require.NoError(t, err, "Create under tenant A should succeed")

	err = repo.Create(ctxB, collection, metaB)
	require.NoError(t, err, "Create under tenant B should succeed")

	// Assert — tenant A can find its own entity but not tenant B's
	foundA, err := repo.FindByEntity(ctxA, collection, "iso-entity-1")
	require.NoError(t, err)
	require.NotNil(t, foundA, "tenant A should find its own entity")
	assert.Equal(t, "A", foundA.Data["tenant"])

	notFoundA, err := repo.FindByEntity(ctxA, collection, "iso-entity-2")
	require.NoError(t, err)
	assert.Nil(t, notFoundA, "tenant A must NOT see tenant B's entity")

	// Assert — tenant B can find its own entity but not tenant A's
	foundB, err := repo.FindByEntity(ctxB, collection, "iso-entity-2")
	require.NoError(t, err)
	require.NotNil(t, foundB, "tenant B should find its own entity")
	assert.Equal(t, "B", foundB.Data["tenant"])

	notFoundB, err := repo.FindByEntity(ctxB, collection, "iso-entity-1")
	require.NoError(t, err)
	assert.Nil(t, notFoundB, "tenant B must NOT see tenant A's entity")

	// Assert — direct database verification confirms isolation
	countA := mongotestutil.CountDocuments(t, tenantADB, strings.ToLower(collection), bson.M{})
	assert.Equal(t, int64(1), countA, "tenant A database should have exactly 1 document")

	countB := mongotestutil.CountDocuments(t, tenantBDB, strings.ToLower(collection), bson.M{})
	assert.Equal(t, int64(1), countB, "tenant B database should have exactly 1 document")
}

// TestIntegration_MetadataRepository_TenantIsolation_UpdateDoesNotCrossTenants verifies
// that updating an entity in tenant A does not affect an entity with the same ID in
// tenant B. This proves that the getDatabase(ctx) method correctly routes updates to
// the tenant-specific database.
func TestIntegration_MetadataRepository_TenantIsolation_UpdateDoesNotCrossTenants(t *testing.T) {
	// Arrange — single container, two databases, shared repo
	container := mongotestutil.SetupReusableContainer(t)

	tenantADB := mongotestutil.CreateOwnedDatabase(t, container)
	tenantBDB := mongotestutil.CreateOwnedDatabase(t, container)

	repo := NewMetadataMongoDBRepository(&libMongo.Client{})

	collection := "operation"

	ctxA := tmcore.ContextWithMB(context.Background(), tenantADB)
	ctxB := tmcore.ContextWithMB(context.Background(), tenantBDB)

	// Insert same entity_id into both tenants with different data
	metaA := &Metadata{
		EntityID:   "shared-id",
		EntityName: "Operation",
		Data:       map[string]any{"version": "A-original"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	metaB := &Metadata{
		EntityID:   "shared-id",
		EntityName: "Operation",
		Data:       map[string]any{"version": "B-original"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	require.NoError(t, repo.Create(ctxA, collection, metaA))
	require.NoError(t, repo.Create(ctxB, collection, metaB))

	// Act — update ONLY tenant A's entity
	err := repo.Update(ctxA, collection, "shared-id", map[string]any{"version": "A-updated"})
	require.NoError(t, err, "Update under tenant A should succeed")

	// Assert — tenant A sees the update
	foundA, err := repo.FindByEntity(ctxA, collection, "shared-id")
	require.NoError(t, err)
	require.NotNil(t, foundA)
	assert.Equal(t, "A-updated", foundA.Data["version"], "tenant A should see updated data")

	// Assert — tenant B is NOT affected by tenant A's update
	foundB, err := repo.FindByEntity(ctxB, collection, "shared-id")
	require.NoError(t, err)
	require.NotNil(t, foundB)
	assert.Equal(t, "B-original", foundB.Data["version"], "tenant B must NOT be affected by tenant A's update")
}

// TestIntegration_MetadataRepository_TenantIsolation_DeleteDoesNotCrossTenants verifies
// that deleting an entity in tenant A does not affect an entity with the same ID in
// tenant B. This proves per-tenant database isolation for delete operations.
func TestIntegration_MetadataRepository_TenantIsolation_DeleteDoesNotCrossTenants(t *testing.T) {
	// Arrange — single container, two databases, shared repo
	container := mongotestutil.SetupReusableContainer(t)

	tenantADB := mongotestutil.CreateOwnedDatabase(t, container)
	tenantBDB := mongotestutil.CreateOwnedDatabase(t, container)

	repo := NewMetadataMongoDBRepository(&libMongo.Client{})

	collection := "operation"

	ctxA := tmcore.ContextWithMB(context.Background(), tenantADB)
	ctxB := tmcore.ContextWithMB(context.Background(), tenantBDB)

	// Insert same entity_id into both tenants
	metaA := &Metadata{
		EntityID:   "delete-shared-id",
		EntityName: "Operation",
		Data:       map[string]any{"owner": "tenantA"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	metaB := &Metadata{
		EntityID:   "delete-shared-id",
		EntityName: "Operation",
		Data:       map[string]any{"owner": "tenantB"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	require.NoError(t, repo.Create(ctxA, collection, metaA))
	require.NoError(t, repo.Create(ctxB, collection, metaB))

	// Verify both exist before delete
	beforeA, err := repo.FindByEntity(ctxA, collection, "delete-shared-id")
	require.NoError(t, err)
	require.NotNil(t, beforeA, "tenant A entity should exist before delete")

	beforeB, err := repo.FindByEntity(ctxB, collection, "delete-shared-id")
	require.NoError(t, err)
	require.NotNil(t, beforeB, "tenant B entity should exist before delete")

	// Act — delete ONLY from tenant A
	err = repo.Delete(ctxA, collection, "delete-shared-id")
	require.NoError(t, err, "Delete under tenant A should succeed")

	// Assert — tenant A's entity is invisible
	afterA, err := repo.FindByEntity(ctxA, collection, "delete-shared-id")
	require.NoError(t, err)
	assert.Nil(t, afterA, "tenant A entity should be invisible after delete")

	// Assert — tenant B's entity is NOT affected
	afterB, err := repo.FindByEntity(ctxB, collection, "delete-shared-id")
	require.NoError(t, err)
	require.NotNil(t, afterB, "tenant B entity must NOT be deleted by tenant A's delete")
	assert.Equal(t, "tenantB", afterB.Data["owner"], "tenant B data should remain unchanged")
}

// TestIntegration_MetadataRepository_FallbackToStaticConnection_WhenNoTenantContext
// verifies that when no tenant context is present in the request context, the
// repository falls back to the static connection (single-tenant mode) and operations
// succeed against the default database.
func TestIntegration_MetadataRepository_FallbackToStaticConnection_WhenNoTenantContext(t *testing.T) {
	// Arrange — standard setup with real static connection (no tenant context)
	container := mongotestutil.SetupReusableContainer(t)
	repo := createRepository(t, container)

	ctx := context.Background()
	collection := "operation"

	metadata := &Metadata{
		EntityID:   "fallback-entity-1",
		EntityName: "Operation",
		Data:       map[string]any{"mode": "single-tenant"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Act — plain context without tenant, should use static connection
	err := repo.Create(ctx, collection, metadata)
	require.NoError(t, err, "Create with no tenant context should succeed via static connection")

	// Assert — entity is findable via the same static connection
	found, err := repo.FindByEntity(ctx, collection, "fallback-entity-1")
	require.NoError(t, err)
	require.NotNil(t, found, "should find entity via static connection fallback")
	assert.Equal(t, "single-tenant", found.Data["mode"])

	// Assert — verify directly in the default database
	count := mongotestutil.CountDocuments(t, container.Database, strings.ToLower(collection), bson.M{"entity_id": "fallback-entity-1"})
	assert.Equal(t, int64(1), count, "document should exist in the default database")
}

// TestIntegration_MetadataRepository_TenantContext_TakesPrecedence_OverStaticConnection
// verifies that when both a static connection AND a tenant context are present, the
// tenant context wins. Data written via tenant context must land in the tenant database,
// NOT in the static connection's default database.
func TestIntegration_MetadataRepository_TenantContext_TakesPrecedence_OverStaticConnection(t *testing.T) {
	// Arrange — static connection points to the container's default_db
	container := mongotestutil.SetupReusableContainer(t)
	repo := createRepository(t, container)

	// Create a tenant database that is different from the default
	tenantDB := mongotestutil.CreateOwnedDatabase(t, container)

	collection := "operation"

	ctxWithTenant := tmcore.ContextWithMB(context.Background(), tenantDB)

	metadata := &Metadata{
		EntityID:   "precedence-entity-1",
		EntityName: "Operation",
		Data:       map[string]any{"target": "tenant_db"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Act — create with tenant context (static connection exists but should be overridden)
	err := repo.Create(ctxWithTenant, collection, metadata)
	require.NoError(t, err, "Create with tenant context should succeed")

	// Assert — entity is findable via tenant context
	found, err := repo.FindByEntity(ctxWithTenant, collection, "precedence-entity-1")
	require.NoError(t, err)
	require.NotNil(t, found, "should find entity in tenant database")
	assert.Equal(t, "tenant_db", found.Data["target"])

	// Assert — entity does NOT exist in the default (static) database
	countInDefault := mongotestutil.CountDocuments(t, container.Database, strings.ToLower(collection), bson.M{"entity_id": "precedence-entity-1"})
	assert.Equal(t, int64(0), countInDefault, "document must NOT exist in the default database — tenant context should take precedence")

	// Assert — entity DOES exist in the tenant database (direct verification)
	countInTenant := mongotestutil.CountDocuments(t, tenantDB, strings.ToLower(collection), bson.M{"entity_id": "precedence-entity-1"})
	assert.Equal(t, int64(1), countInTenant, "document should exist in the tenant database")
}

// ============================================================================
// mongo-driver v2 migration regression tests (Gate-9 behavioral bar)
// ============================================================================

// TestIntegration_MetadataRepository_Create_ServerAssignsObjectID asserts that a zero
// bson.ObjectID carrying `bson:"_id,omitempty"` is omitted on marshal (ObjectID
// implements Zeroer), so MongoDB assigns a non-zero _id rather than persisting the
// zero ObjectID. This guarantees server-side _id generation for new documents.
func TestIntegration_MetadataRepository_Create_ServerAssignsObjectID(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupReusableContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	metadata := &Metadata{
		EntityID:   "txn-oid-assign",
		EntityName: "Transaction",
		Data:       map[string]any{"type": "credit"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Act — Create leaves metadata.ID as the zero ObjectID; omitempty must omit it.
	err := repo.Create(ctx, collection, metadata)
	require.NoError(t, err, "Create should not return error")

	// Assert — read the raw document and confirm _id is a non-zero ObjectID.
	var raw bson.M
	err = container.Database.Collection(strings.ToLower(collection)).
		FindOne(ctx, bson.M{"entity_id": "txn-oid-assign"}).Decode(&raw)
	require.NoError(t, err, "should find the inserted document")

	rawID, ok := raw["_id"].(bson.ObjectID)
	require.True(t, ok, "_id should decode as a bson.ObjectID, got %T", raw["_id"])
	assert.False(t, rawID.IsZero(), "server must assign a non-zero _id (zero ObjectID must not be persisted)")
}

// TestIntegration_MetadataRepository_FindByEntity_DecodeRoundTrips asserts that a full
// document survives a write/read round-trip with every field type intact: string,
// 64-bit integer, and a nested document. Numeric and nested values are the types most
// sensitive to the BSON decoder's representation choices, so they are asserted by
// concrete type and value.
func TestIntegration_MetadataRepository_FindByEntity_DecodeRoundTrips(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupReusableContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "transaction"

	created := time.Now().UTC().Truncate(time.Millisecond)
	metadata := &Metadata{
		EntityID:   "txn-roundtrip",
		EntityName: "Transaction",
		Data: map[string]any{
			"type":   "credit",
			"amount": int64(4200),
			"nested": map[string]any{"k": "v"},
		},
		CreatedAt: created,
		UpdatedAt: created,
	}
	require.NoError(t, repo.Create(ctx, collection, metadata), "Create should not return error")

	// Act — read it back through the v2 decoder.
	found, err := repo.FindByEntity(ctx, collection, "txn-roundtrip")

	// Assert — every field decoded back intact.
	require.NoError(t, err, "FindByEntity should not return error")
	require.NotNil(t, found, "document should be found")
	assert.Equal(t, "txn-roundtrip", found.EntityID)
	assert.Equal(t, "Transaction", found.EntityName)
	assert.Equal(t, "credit", found.Data["type"], "string field should round-trip")

	// A BSON 64-bit integer decodes into an int64 when the target is `any`.
	amount, ok := found.Data["amount"].(int64)
	require.True(t, ok, "amount should decode as int64, got %T", found.Data["amount"])
	assert.Equal(t, int64(4200), amount, "int64 field should round-trip")

	// Data is a typed map[string]any at the top level, but a nested document has no
	// concrete Go target, so the decoder represents it as an ordered bson.D.
	nested, ok := found.Data["nested"].(bson.D)
	require.True(t, ok, "nested document should decode as bson.D, got %T", found.Data["nested"])

	var nestedK any
	for _, e := range nested {
		if e.Key == "k" {
			nestedK = e.Value
		}
	}

	assert.Equal(t, "v", nestedK, "nested document value should round-trip")
}
