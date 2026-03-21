// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"context"
	"testing"

	libMongo "github.com/LerianStudio/lib-commons/v4/commons/mongo"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func newDisconnectedDatabase(t *testing.T, dbName string) *mongo.Database {
	t.Helper()

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	require.NoError(t, err, "mongo.Connect should succeed for a disconnected handle")

	t.Cleanup(func() {
		require.NoError(t, client.Disconnect(context.Background()), "client disconnect should not error")
	})

	return client.Database(dbName)
}

func newPlaceholderConnection(_ string) *libMongo.Client {
	return &libMongo.Client{}
}

func TestNewMongoDBRepository_NilConnection(t *testing.T) {
	t.Parallel()

	repo, err := NewMongoDBRepository(nil, nil)

	require.NoError(t, err, "constructor should not error when connection is nil")
	require.NotNil(t, repo, "returned repository must not be nil")
}

func TestNewMongoDBRepository_WithPlaceholderConnection(t *testing.T) {
	t.Parallel()

	conn := newPlaceholderConnection("tenant-placeholder")

	repo, err := NewMongoDBRepository(conn, nil)

	require.Error(t, err, "constructor should return error with placeholder connection without active database")
	assert.Nil(t, repo, "repository must be nil when constructor fails")
}

func TestGetDatabase_ReturnsTenantDB_WhenContextHasTenantMongo(t *testing.T) {
	t.Parallel()

	tenantDB := newDisconnectedDatabase(t, "tenant-acme")

	tests := []struct {
		name     string
		tenantDB *mongo.Database
		wantName string
	}{
		{
			name:     "tenant_db_present_in_context",
			tenantDB: tenantDB,
			wantName: "tenant-acme",
		},
		{
			name:     "different_tenant_db_name",
			tenantDB: newDisconnectedDatabase(t, "tenant-globex"),
			wantName: "tenant-globex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &MongoDBRepository{connection: newPlaceholderConnection("static-db")}
			ctx := tmcore.ContextWithTenantMongo(context.Background(), tt.tenantDB)

			db, err := repo.getDatabase(ctx)

			require.NoError(t, err, "getDatabase should not return error when tenant DB is in context")
			require.NotNil(t, db, "returned database must not be nil")
			assert.Same(t, tt.tenantDB, db, "must return the exact tenant DB from context")
			assert.Equal(t, tt.wantName, db.Name(), "database name should match the tenant DB")
		})
	}
}

func TestGetDatabase_TenantDB_TakesPrecedence_OverStaticConnection(t *testing.T) {
	t.Parallel()

	repo := &MongoDBRepository{connection: newPlaceholderConnection("static-db")}

	tenantDB := newDisconnectedDatabase(t, "tenant-priority")
	ctx := tmcore.ContextWithTenantMongo(context.Background(), tenantDB)

	db, dbErr := repo.getDatabase(ctx)

	require.NoError(t, dbErr, "getDatabase should not return error")
	require.NotNil(t, db, "returned database must not be nil")
	assert.Same(t, tenantDB, db, "tenant DB must take precedence over static connection")
	assert.Equal(t, "tenant-priority", db.Name(), "returned DB name should be the tenant DB name, not static-db")
}

func TestGetDatabase_FallsBackToStaticConnection_WhenNoTenantContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     context.Context
		wantErr bool
	}{
		{name: "plain_background_context_no_tenant", ctx: context.Background(), wantErr: true},
		{name: "context_with_unrelated_values", ctx: context.WithValue(context.Background(), struct{}{}, "unrelated"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &MongoDBRepository{connection: newPlaceholderConnection("fallback-db")}

			db, err := repo.getDatabase(tt.ctx)

			if tt.wantErr {
				require.Error(t, err, "getDatabase should return error when static connection has no live MongoDB")
				assert.Nil(t, db, "database should be nil when static connection fails")
			}
		})
	}
}

func TestGetDatabase_FallsBack_WhenTenantDBIsNilInContext(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantMongo(context.Background(), nil)

	repo := &MongoDBRepository{connection: newPlaceholderConnection("fallback-nil-db")}

	db, err := repo.getDatabase(ctx)

	require.Error(t, err, "getDatabase should return error when tenant DB is nil and static connection has no server")
	assert.Nil(t, db, "database should be nil when both tenant context is nil and static connection fails")
}

func TestGetDatabase_TwoTenants_ResolveToDifferentDatabases(t *testing.T) {
	t.Parallel()

	tenantAcmeDB := newDisconnectedDatabase(t, "tenant-acme")
	tenantGlobexDB := newDisconnectedDatabase(t, "tenant-globex")

	repo := &MongoDBRepository{connection: newPlaceholderConnection("static-db")}

	ctxTenantA := tmcore.ContextWithTenantMongo(context.Background(), tenantAcmeDB)
	ctxTenantB := tmcore.ContextWithTenantMongo(context.Background(), tenantGlobexDB)

	dbA, errA := repo.getDatabase(ctxTenantA)
	dbB, errB := repo.getDatabase(ctxTenantB)

	require.NoError(t, errA, "getDatabase should not error for tenant A")
	require.NoError(t, errB, "getDatabase should not error for tenant B")
	require.NotNil(t, dbA, "tenant A database must not be nil")
	require.NotNil(t, dbB, "tenant B database must not be nil")

	assert.NotSame(t, dbA, dbB, "two different tenants must resolve to different *mongo.Database instances")
	assert.NotEqual(t, dbA.Name(), dbB.Name(), "two different tenants must have different database names")
	assert.Equal(t, "tenant-acme", dbA.Name(), "tenant A database name must match")
	assert.Equal(t, "tenant-globex", dbB.Name(), "tenant B database name must match")
}

func TestGetDatabase_SameTenant_ReturnsSameDatabase(t *testing.T) {
	t.Parallel()

	tenantDB := newDisconnectedDatabase(t, "tenant-acme")

	repo := &MongoDBRepository{connection: newPlaceholderConnection("static-db")}

	ctx1 := tmcore.ContextWithTenantMongo(context.Background(), tenantDB)
	ctx2 := tmcore.ContextWithTenantMongo(context.Background(), tenantDB)

	db1, err1 := repo.getDatabase(ctx1)
	db2, err2 := repo.getDatabase(ctx2)

	require.NoError(t, err1, "getDatabase should not error for first call")
	require.NoError(t, err2, "getDatabase should not error for second call")
	assert.Same(t, db1, db2, "same tenant DB must return the same *mongo.Database pointer")
}

func TestGetDatabase_NilConnection_NoTenantContext_ReturnsError(t *testing.T) {
	t.Parallel()

	repo := &MongoDBRepository{connection: nil}

	db, err := repo.getDatabase(context.Background())

	require.Error(t, err, "getDatabase must return error when connection is nil and no tenant context exists")
	assert.Nil(t, db, "database must be nil when no connection and no tenant context")
	assert.Contains(t, err.Error(), "no database connection available", "error message should indicate no connection is available")
}

func TestGetDatabase_NilConnection_WithTenantContext_Succeeds(t *testing.T) {
	t.Parallel()

	tenantDB := newDisconnectedDatabase(t, "tenant-nil-conn")

	repo := &MongoDBRepository{connection: nil}

	ctx := tmcore.ContextWithTenantMongo(context.Background(), tenantDB)

	db, err := repo.getDatabase(ctx)

	require.NoError(t, err, "getDatabase should not error when tenant DB is in context")
	require.NotNil(t, db, "returned database must not be nil")
	assert.Same(t, tenantDB, db, "must return the exact tenant DB from context")
	assert.Equal(t, "tenant-nil-conn", db.Name(), "database name should match the tenant DB")
}

func TestGetDatabase_StaticConnection_ReturnsErrorWithPlaceholder(t *testing.T) {
	t.Parallel()

	repo := &MongoDBRepository{connection: newPlaceholderConnection("MyDatabase")}

	db, dbErr := repo.getDatabase(context.Background())

	require.Error(t, dbErr, "getDatabase should return error when static connection has no active client")
	assert.Nil(t, db, "database should be nil when static connection is not connected")
}
