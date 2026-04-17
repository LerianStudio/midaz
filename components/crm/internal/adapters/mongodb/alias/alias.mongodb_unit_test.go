// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package alias

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

// newUnreachableRepo builds an alias.MongoDBRepository wired to a mongo.Client
// that is intentionally unreachable. mongo.Connect is lazy: the client is
// returned without dialing. Subsequent operations (Ping, InsertOne, FindOne,
// etc.) fail fast with a ServerSelectionError because the server selection
// timeout is 100ms.
//
// This lets unit tests drive real adapter code paths without a live MongoDB —
// we exercise filter construction, tracing setup, and error-wrapping branches
// deterministically.
func newUnreachableRepo(t *testing.T) *MongoDBRepository {
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

	return &MongoDBRepository{
		connection:   conn,
		Database:     "test_db",
		DataSecurity: testutils.SetupCrypto(t),
	}
}

// newBrokenConnRepo returns a repo whose GetDB always fails. The underlying
// MongoConnection has no pre-populated DB and a bad URI; calling GetDB will
// call Connect, which tries to open a TCP socket against 0.0.0.0:0 and fails
// quickly. We use this to exercise the "get db connection" error branch in
// every method.
func newBrokenConnRepo(t *testing.T) *MongoDBRepository {
	t.Helper()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	conn := &libMongo.MongoConnection{
		// Empty URI forces mongo.Connect to return an error immediately.
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

// makeAlias builds a minimal but valid *mmodel.Alias for Create/Update tests.
func makeAlias() *mmodel.Alias {
	id := uuid.New()
	holderID := uuid.New()
	document := "12345678901"
	typ := "NATURAL_PERSON"
	ledger := "ledger-1"
	account := "account-1"
	now := time.Now().UTC()

	return &mmodel.Alias{
		ID:        &id,
		HolderID:  &holderID,
		Document:  &document,
		Type:      &typ,
		LedgerID:  &ledger,
		AccountID: &account,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// -- Create ------------.

func TestMongoDBRepository_Create_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnRepo(t)

	result, err := repo.Create(context.Background(), "org-1", makeAlias())

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get db connection", "error should wrap get-db failure")
}

func TestMongoDBRepository_Create_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableRepo(t)

	result, err := repo.Create(context.Background(), "org-1", makeAlias())

	require.Error(t, err)
	assert.Nil(t, result)
	// Path: GetDB returns a client, createIndexes fails on server selection.
	assert.Contains(t, err.Error(), "create indexes", "should fail on index creation against unreachable server")
}

// -- Find ------------.

func TestMongoDBRepository_Find_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnRepo(t)

	result, err := repo.Find(context.Background(), "org-1", uuid.New(), uuid.New(), false)

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

			result, err := repo.Find(context.Background(), "org-1", uuid.New(), uuid.New(), includeDeleted)
			require.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), "find alias")
		})
	}
}

// -- Update ------------.

func TestMongoDBRepository_Update_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnRepo(t)

	result, err := repo.Update(context.Background(), "org-1", uuid.New(), uuid.New(), makeAlias(), []string{"description"})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get db connection")
}

func TestMongoDBRepository_Update_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableRepo(t)

	result, err := repo.Update(context.Background(), "org-1", uuid.New(), uuid.New(), makeAlias(), []string{"description"})

	require.Error(t, err)
	assert.Nil(t, result)
	// The update call hits UpdateOne, which fails on server selection.
	assert.Contains(t, err.Error(), "update alias")
}

// -- FindAll ------------.

func TestMongoDBRepository_FindAll_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnRepo(t)

	result, err := repo.FindAll(context.Background(), "org-1", uuid.New(), pkgHTTP.QueryHeader{Limit: 10, Page: 1}, false)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get db connection")
}

func TestMongoDBRepository_FindAll_InvalidMetadataBuildsFilterError(t *testing.T) {
	t.Parallel()

	repo := newUnreachableRepo(t)

	// Nested map values are rejected by http.ValidateMetadataValue, so this
	// exercises the buildAliasFilter error branch inside FindAll without
	// needing a Mongo round-trip.
	query := pkgHTTP.QueryHeader{
		Limit: 10,
		Page:  1,
		Metadata: &map[string]any{
			"metadata.nested": map[string]any{"inner": "value"},
		},
	}

	result, err := repo.FindAll(context.Background(), "org-1", uuid.New(), query, false)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "build alias filter")
}

func TestMongoDBRepository_FindAll_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableRepo(t)

	result, err := repo.FindAll(context.Background(), "org-1", uuid.New(), pkgHTTP.QueryHeader{Limit: 10, Page: 1}, false)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "find aliases")
}

// -- Delete ------------.

func TestMongoDBRepository_Delete_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnRepo(t)

	for _, hardDelete := range []bool{false, true} {
		t.Run(fmtBool("hardDelete", hardDelete), func(t *testing.T) {
			t.Parallel()

			err := repo.Delete(context.Background(), "org-1", uuid.New(), uuid.New(), hardDelete)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "get db connection")
		})
	}
}

func TestMongoDBRepository_Delete_UnreachableMongo_Soft(t *testing.T) {
	t.Parallel()

	repo := newUnreachableRepo(t)

	err := repo.Delete(context.Background(), "org-1", uuid.New(), uuid.New(), false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "soft delete alias")
}

func TestMongoDBRepository_Delete_UnreachableMongo_Hard(t *testing.T) {
	t.Parallel()

	repo := newUnreachableRepo(t)

	err := repo.Delete(context.Background(), "org-1", uuid.New(), uuid.New(), true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "hard delete alias")
}

// -- Count ------------.

func TestMongoDBRepository_Count_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnRepo(t)

	count, err := repo.Count(context.Background(), "org-1", uuid.New())

	require.Error(t, err)
	assert.Equal(t, int64(0), count)
	assert.Contains(t, err.Error(), "get db connection")
}

func TestMongoDBRepository_Count_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableRepo(t)

	count, err := repo.Count(context.Background(), "org-1", uuid.New())

	require.Error(t, err)
	assert.Equal(t, int64(0), count)
	assert.Contains(t, err.Error(), "count aliases")
}

// -- DeleteRelatedParty ------------.

func TestMongoDBRepository_DeleteRelatedParty_GetDBError(t *testing.T) {
	t.Parallel()

	repo := newBrokenConnRepo(t)

	err := repo.DeleteRelatedParty(context.Background(), "org-1", uuid.New(), uuid.New(), uuid.New())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get db connection")
}

func TestMongoDBRepository_DeleteRelatedParty_UnreachableMongo(t *testing.T) {
	t.Parallel()

	repo := newUnreachableRepo(t)

	err := repo.DeleteRelatedParty(context.Background(), "org-1", uuid.New(), uuid.New(), uuid.New())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete related party")
}

// -- NewMongoDBRepository ------------.

func TestNewMongoDBRepository_FailsOnBadConnection(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	// Empty/invalid URI causes GetDB -> Connect to fail immediately.
	conn := &libMongo.MongoConnection{
		ConnectionStringSource: "not-a-valid-uri://",
		Database:               "test_db",
		Logger:                 logger,
	}

	repo, err := NewMongoDBRepository(conn, testutils.SetupCrypto(t))

	require.Error(t, err)
	assert.Nil(t, repo)
	assert.Contains(t, err.Error(), "failed to connect to MongoDB for alias repository")
}

func TestNewMongoDBRepository_SucceedsWithPrePopulatedClient(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	// Pre-populate DB so GetDB returns without dialing.
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
