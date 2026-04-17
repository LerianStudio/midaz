// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v3/tests/utils"
)

// newUnreachableRepo builds a holder.MongoDBRepository wired to a mongo.Client
// that is intentionally unreachable. mongo.Connect is lazy: the client is
// returned without dialing. Subsequent operations fail fast with a
// ServerSelectionError because the server selection timeout is 100ms.
//
// This lets unit tests drive real adapter code paths without a live MongoDB —
// we exercise filter construction, tracing setup, and error-wrapping branches
// deterministically.
func newUnreachableRepo(t *testing.T) *MongoDBRepository {
	t.Helper()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

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

	return &MongoDBRepository{
		connection:   conn,
		Database:     "test_db",
		DataSecurity: testutils.SetupCrypto(t),
	}
}

// newBrokenConnRepo returns a repo whose GetDB always fails. The underlying
// MongoConnection has no pre-populated DB and a bad URI; calling GetDB will
// call Connect, which returns an error immediately.
func newBrokenConnRepo(t *testing.T) *MongoDBRepository {
	t.Helper()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	conn := &libMongo.MongoConnection{
		ConnectionStringSource: "not-a-valid-uri://",
		Database:               "test_db",
		Logger:                 logger,
	}

	return &MongoDBRepository{
		connection:   conn,
		Database:     "test_db",
		DataSecurity: testutils.SetupCrypto(t),
	}
}

// makeHolder returns a minimal valid holder. Kept small so tests remain
// legible; individual tests override fields they need.
func makeHolder() *mmodel.Holder {
	id := uuid.New()
	doc := "98765432100"
	typ := "LEGAL_PERSON"
	extID := "ext-1"
	now := time.Now().UTC()

	return &mmodel.Holder{
		ID:         &id,
		Document:   &doc,
		Type:       &typ,
		ExternalID: &extID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// -- Create ------------.

func TestMongoDBRepository_Create_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnRepo(t)

	result, err := repo.Create(context.Background(), "org-1", makeHolder())

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get db connection")
}

func TestMongoDBRepository_Create_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableRepo(t)

	result, err := repo.Create(context.Background(), "org-1", makeHolder())

	require.Error(t, err)
	assert.Nil(t, result)
	// GetDB returns a client, createIndexes fails on server selection.
	assert.Contains(t, err.Error(), "create indexes")
}

// -- Find ------------.

func TestMongoDBRepository_Find_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnRepo(t)

	result, err := repo.Find(context.Background(), "org-1", uuid.New(), false)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get db connection")
}

func TestMongoDBRepository_Find_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableRepo(t)

	for _, includeDeleted := range []bool{false, true} {
		t.Run(fmtBool("includeDeleted", includeDeleted), func(t *testing.T) {
			t.Parallel()

			result, err := repo.Find(context.Background(), "org-1", uuid.New(), includeDeleted)
			require.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), "find holder")
		})
	}
}

// -- FindAll ------------.

func TestMongoDBRepository_FindAll_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnRepo(t)

	result, err := repo.FindAll(context.Background(), "org-1", pkgHTTP.QueryHeader{Limit: 10, Page: 1}, false)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get db connection")
}

func TestMongoDBRepository_FindAll_InvalidMetadataBuildsFilterError(t *testing.T) {
	t.Parallel()

	repo := newUnreachableRepo(t)

	// Nested map values are rejected by http.ValidateMetadataValue, so this
	// exercises the buildHolderFilter error branch inside FindAll without a
	// Mongo round-trip.
	query := pkgHTTP.QueryHeader{
		Limit: 10,
		Page:  1,
		Metadata: &map[string]any{
			"metadata.nested": map[string]any{"inner": "value"},
		},
	}

	result, err := repo.FindAll(context.Background(), "org-1", query, false)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "build holder filter")
}

func TestMongoDBRepository_FindAll_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableRepo(t)

	result, err := repo.FindAll(context.Background(), "org-1", pkgHTTP.QueryHeader{Limit: 10, Page: 1}, false)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "find holders")
}

// -- Update ------------.

func TestMongoDBRepository_Update_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnRepo(t)

	result, err := repo.Update(context.Background(), "org-1", uuid.New(), makeHolder(), []string{"name"})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get db connection")
}

func TestMongoDBRepository_Update_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableRepo(t)

	result, err := repo.Update(context.Background(), "org-1", uuid.New(), makeHolder(), []string{"name"})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "update holder")
}

// -- Delete ------------.

func TestMongoDBRepository_Delete_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnRepo(t)

	for _, hardDelete := range []bool{false, true} {
		t.Run(fmtBool("hardDelete", hardDelete), func(t *testing.T) {
			t.Parallel()

			err := repo.Delete(context.Background(), "org-1", uuid.New(), hardDelete)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "get db connection")
		})
	}
}

func TestMongoDBRepository_Delete_UnreachableMongo_Soft(t *testing.T) {
	t.Parallel()

	repo := newUnreachableRepo(t)

	err := repo.Delete(context.Background(), "org-1", uuid.New(), false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "soft delete holder")
}

func TestMongoDBRepository_Delete_UnreachableMongo_Hard(t *testing.T) {
	t.Parallel()

	repo := newUnreachableRepo(t)

	err := repo.Delete(context.Background(), "org-1", uuid.New(), true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "hard delete holder")
}

// -- NewMongoDBRepository ------------.

func TestNewMongoDBRepository_FailsOnBadConnection(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	conn := &libMongo.MongoConnection{
		ConnectionStringSource: "not-a-valid-uri://",
		Database:               "test_db",
		Logger:                 logger,
	}

	repo, err := NewMongoDBRepository(conn, testutils.SetupCrypto(t))

	require.Error(t, err)
	assert.Nil(t, repo)
	assert.Contains(t, err.Error(), "failed to connect to MongoDB for holder repository")
}

func TestNewMongoDBRepository_SucceedsWithPrePopulatedClient(t *testing.T) {
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

	repo, err := NewMongoDBRepository(conn, testutils.SetupCrypto(t))

	require.NoError(t, err)
	require.NotNil(t, repo)
	assert.Equal(t, "test_db", repo.Database)
}

// fmtBool produces a stable sub-test name for table runs that toggle a boolean.
func fmtBool(name string, v bool) string {
	if v {
		return name + "=true"
	}

	return name + "=false"
}
