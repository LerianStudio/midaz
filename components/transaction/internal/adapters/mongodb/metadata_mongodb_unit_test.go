// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

// newUnreachableMetaRepo builds a MetadataMongoDBRepository wired to a mongo.Client
// that is intentionally unreachable. mongo.Connect is lazy: the client is returned
// without dialing. Subsequent operations fail fast with a ServerSelectionError
// because the server selection timeout is 100ms.
//
// This lets unit tests drive real adapter code paths (tracing setup, filter
// construction, BSON marshalling, logging) without a live MongoDB and exercise
// error-wrapping branches deterministically.
func newUnreachableMetaRepo(t *testing.T) *MetadataMongoDBRepository {
	t.Helper()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	// 127.0.0.1:1 is a port that will never accept connections in a test env.
	// SetServerSelectionTimeout keeps failures bounded to ~100ms.
	clientOpts := options.Client().
		ApplyURI("mongodb://127.0.0.1:1/").
		SetServerSelectionTimeout(100 * time.Millisecond).
		SetConnectTimeout(100 * time.Millisecond)

	client, err := mongo.Connect(context.Background(), clientOpts)
	require.NoError(t, err, "mongo.Connect should return lazily even with unreachable host")

	t.Cleanup(func() {
		_ = client.Disconnect(context.Background())
	})

	conn := &libMongo.MongoConnection{
		ConnectionStringSource: "mongodb://127.0.0.1:1/",
		Database:               "test_db",
		Logger:                 logger,
		DB:                     client,
		Connected:              true,
	}

	return &MetadataMongoDBRepository{
		connection: conn,
		Database:   "test_db",
	}
}

// newBrokenConnMetaRepo returns a repo whose GetDB always fails. The underlying
// MongoConnection has no pre-populated DB and a bad URI; calling GetDB invokes
// Connect which returns an error immediately. We use this to exercise the
// "get database connection" error branch in every method.
func newBrokenConnMetaRepo(t *testing.T) *MetadataMongoDBRepository {
	t.Helper()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	conn := &libMongo.MongoConnection{
		ConnectionStringSource: "not-a-valid-uri://",
		Database:               "test_db",
		Logger:                 logger,
	}

	return &MetadataMongoDBRepository{
		connection: conn,
		Database:   "test_db",
	}
}

// makeMetadata builds a minimal valid *Metadata for Create tests.
func makeMetadata() *Metadata {
	return &Metadata{
		EntityID:   "entity-1",
		EntityName: "transaction",
		Data:       JSON{"key": "value"},
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
}

// -- NewMetadataMongoDBRepository ------------.

func TestNewMetadataMongoDBRepository_FailsOnBadConnection(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	conn := &libMongo.MongoConnection{
		ConnectionStringSource: "not-a-valid-uri://",
		Database:               "test_db",
		Logger:                 logger,
	}

	repo, err := NewMetadataMongoDBRepository(conn)

	require.Error(t, err)
	assert.Nil(t, repo)
	assert.Contains(t, err.Error(), "failed to connect to MongoDB")
}

func TestNewMetadataMongoDBRepository_SucceedsWithPrePopulatedClient(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	clientOpts := options.Client().
		ApplyURI("mongodb://127.0.0.1:1/").
		SetServerSelectionTimeout(100 * time.Millisecond).
		SetConnectTimeout(100 * time.Millisecond)

	client, err := mongo.Connect(context.Background(), clientOpts)
	require.NoError(t, err)

	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	conn := &libMongo.MongoConnection{
		ConnectionStringSource: "mongodb://127.0.0.1:1/",
		Database:               "test_db",
		Logger:                 logger,
		DB:                     client,
		Connected:              true,
	}

	repo, err := NewMetadataMongoDBRepository(conn)

	require.NoError(t, err)
	require.NotNil(t, repo)
	assert.Equal(t, "test_db", repo.Database)
}

// -- Create ------------.

func TestMetadataMongoDBRepository_Create_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnMetaRepo(t)

	err := repo.Create(context.Background(), "transaction", makeMetadata())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get database connection")
}

func TestMetadataMongoDBRepository_Create_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableMetaRepo(t)

	err := repo.Create(context.Background(), "transaction", makeMetadata())

	require.Error(t, err)
	// FromEntity succeeds for metadata, so we expect the insert to fail
	// on server selection against the unreachable host.
	assert.Contains(t, err.Error(), "insert metadata")
}

// -- FindList ------------.

func TestMetadataMongoDBRepository_FindList_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnMetaRepo(t)

	result, err := repo.FindList(context.Background(), "transaction", http.QueryHeader{})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get database connection")
}

func TestMetadataMongoDBRepository_FindList_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableMetaRepo(t)

	start := time.Now().Add(-24 * time.Hour).UTC()
	end := time.Now().UTC()

	// Exercise all filter branches: metadata map, start-only, end-only, both.
	cases := []struct {
		name  string
		query http.QueryHeader
	}{
		{"no_filter", http.QueryHeader{}},
		{"with_metadata_filter", http.QueryHeader{Metadata: &map[string]any{"foo": "bar"}}},
		{"with_start_date_only", http.QueryHeader{StartDate: start}},
		{"with_end_date_only", http.QueryHeader{EndDate: end}},
		{"with_both_dates", http.QueryHeader{StartDate: start, EndDate: end}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := repo.FindList(context.Background(), "transaction", tc.query)
			require.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), "find metadata")
		})
	}
}

// -- FindByEntity ------------.

func TestMetadataMongoDBRepository_FindByEntity_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnMetaRepo(t)

	result, err := repo.FindByEntity(context.Background(), "transaction", "some-id")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get database")
}

func TestMetadataMongoDBRepository_FindByEntity_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableMetaRepo(t)

	result, err := repo.FindByEntity(context.Background(), "transaction", "some-id")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "find metadata by entity")
}

// -- FindByEntityIDs ------------.

func TestMetadataMongoDBRepository_FindByEntityIDs_EmptyReturnsEmpty(t *testing.T) {
	t.Parallel()

	repo := newUnreachableMetaRepo(t)

	// Empty list short-circuits before touching the database.
	result, err := repo.FindByEntityIDs(context.Background(), "transaction", nil)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestMetadataMongoDBRepository_FindByEntityIDs_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnMetaRepo(t)

	result, err := repo.FindByEntityIDs(context.Background(), "transaction", []string{"id-1"})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get database connection")
}

func TestMetadataMongoDBRepository_FindByEntityIDs_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableMetaRepo(t)

	result, err := repo.FindByEntityIDs(context.Background(), "transaction", []string{"id-1", "id-2"})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "find metadata by entity IDs")
}

// -- Update ------------.

func TestMetadataMongoDBRepository_Update_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnMetaRepo(t)

	err := repo.Update(context.Background(), "transaction", "id-1", map[string]any{"k": "v"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get database")
}

func TestMetadataMongoDBRepository_Update_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableMetaRepo(t)

	err := repo.Update(context.Background(), "transaction", "id-1", map[string]any{"k": "v"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "update metadata")
}

// -- Delete ------------.

func TestMetadataMongoDBRepository_Delete_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnMetaRepo(t)

	err := repo.Delete(context.Background(), "transaction", "id-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get database")
}

func TestMetadataMongoDBRepository_Delete_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableMetaRepo(t)

	err := repo.Delete(context.Background(), "transaction", "id-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete metadata")
}

// -- CreateIndex ------------.

func TestMetadataMongoDBRepository_CreateIndex_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnMetaRepo(t)

	input := &mmodel.CreateMetadataIndexInput{
		MetadataKey: "customer_id",
		Unique:      false,
	}

	result, err := repo.CreateIndex(context.Background(), "transaction", input)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get database")
}

func TestMetadataMongoDBRepository_CreateIndex_UnreachableMongo_DefaultSparse(t *testing.T) {
	t.Parallel()

	repo := newUnreachableMetaRepo(t)

	// No Sparse pointer → defaults to true.
	input := &mmodel.CreateMetadataIndexInput{
		MetadataKey: "customer_id",
		Unique:      true,
	}

	result, err := repo.CreateIndex(context.Background(), "transaction", input)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "create index")
}

func TestMetadataMongoDBRepository_CreateIndex_UnreachableMongo_ExplicitSparse(t *testing.T) {
	t.Parallel()

	repo := newUnreachableMetaRepo(t)

	sparse := false
	input := &mmodel.CreateMetadataIndexInput{
		MetadataKey: "region",
		Unique:      false,
		Sparse:      &sparse,
	}

	result, err := repo.CreateIndex(context.Background(), "transaction", input)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "create index")
}

// -- FindAllIndexes ------------.

func TestMetadataMongoDBRepository_FindAllIndexes_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnMetaRepo(t)

	result, err := repo.FindAllIndexes(context.Background(), "transaction")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get database")
}

func TestMetadataMongoDBRepository_FindAllIndexes_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableMetaRepo(t)

	result, err := repo.FindAllIndexes(context.Background(), "transaction")

	require.Error(t, err)
	assert.Nil(t, result)
	// Aggregate is called first, so error bubbles up from index stats.
	assert.Contains(t, err.Error(), "aggregate index stats")
}

// -- DeleteIndex ------------.

func TestMetadataMongoDBRepository_DeleteIndex_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnMetaRepo(t)

	err := repo.DeleteIndex(context.Background(), "transaction", "metadata.customer_id_1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get database")
}

func TestMetadataMongoDBRepository_DeleteIndex_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableMetaRepo(t)

	err := repo.DeleteIndex(context.Background(), "transaction", "metadata.customer_id_1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop index")
}
