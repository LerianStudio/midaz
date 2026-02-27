// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"context"
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v3/commons/mongo"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// newDisconnectedDatabase creates a *mongo.Database handle without establishing
// a real MongoDB connection. Uses mongo.Connect with a disconnected client —
// the driver allows creating database handles that are purely in-memory metadata;
// no network call happens until an actual query is performed.
func newDisconnectedDatabase(t *testing.T, dbName string) *mongo.Database {
	t.Helper()

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	require.NoError(t, err, "mongo.Connect should succeed for a disconnected handle")

	t.Cleanup(func() {
		require.NoError(t, client.Disconnect(context.Background()), "client disconnect should not error")
	})

	return client.Database(dbName)
}

// newPlaceholderConnection creates a *libMongo.MongoConnection with only a Logger
// set (no ConnectionStringSource). This simulates the multi-tenant bootstrap mode
// where the static connection is a placeholder and the real database comes from
// the per-request tenant context.
func newPlaceholderConnection(dbName string) *libMongo.MongoConnection {
	return &libMongo.MongoConnection{
		Database: dbName,
		Logger:   &libLog.NoneLogger{},
	}
}

// =============================================================================
// Constructor Tests — NewMongoDBRepository
// =============================================================================

func TestNewMongoDBRepository_NilConnection(t *testing.T) {
	t.Parallel()

	// In multi-tenant mode, connection may be nil. Constructor must handle this.
	repo, err := NewMongoDBRepository(nil, nil)

	require.NoError(t, err, "constructor should not error when connection is nil")
	require.NotNil(t, repo, "returned repository must not be nil")
}

func TestNewMongoDBRepository_WithPlaceholderConnection(t *testing.T) {
	t.Parallel()

	conn := newPlaceholderConnection("tenant-placeholder")

	// Placeholder connection has no live server, but constructor should succeed
	// because it only does a health check if connection is non-nil and has a source.
	repo, err := NewMongoDBRepository(conn, nil)

	require.NoError(t, err, "constructor should not error with placeholder connection")
	require.NotNil(t, repo, "returned repository must not be nil")
	assert.Equal(t, "tenant-placeholder", repo.Database, "Database field should match connection.Database")
}

// =============================================================================
// getDatabase Tests — Multi-Tenant Path
// =============================================================================

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

			// Arrange
			repo := &MongoDBRepository{
				connection: newPlaceholderConnection("static-db"),
				Database:   "static-db",
			}
			ctx := tmcore.ContextWithTenantMongo(context.Background(), tt.tenantDB)

			// Act
			db, err := repo.getDatabase(ctx)

			// Assert
			require.NoError(t, err, "getDatabase should not return error when tenant DB is in context")
			require.NotNil(t, db, "returned database must not be nil")
			assert.Same(t, tt.tenantDB, db, "must return the exact tenant DB from context")
			assert.Equal(t, tt.wantName, db.Name(), "database name should match the tenant DB")
		})
	}
}

func TestGetDatabase_TenantDB_TakesPrecedence_OverStaticConnection(t *testing.T) {
	t.Parallel()

	// Arrange — create a static connection that has a real client set
	// (simulating a working single-tenant connection).
	staticClient, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := staticClient.Disconnect(context.Background()); err != nil {
			t.Logf("warning: failed to disconnect static client: %v", err)
		}
	})

	staticConn := &libMongo.MongoConnection{
		ConnectionStringSource: "mongodb://localhost:27017",
		Database:               "static-db",
		Logger:                 &libLog.NoneLogger{},
		DB:                     staticClient,
		Connected:              true,
	}

	repo := &MongoDBRepository{
		connection: staticConn,
		Database:   "static-db",
	}

	tenantDB := newDisconnectedDatabase(t, "tenant-priority")
	ctx := tmcore.ContextWithTenantMongo(context.Background(), tenantDB)

	// Act
	db, dbErr := repo.getDatabase(ctx)

	// Assert — tenant DB should be returned, not the static connection's database
	require.NoError(t, dbErr, "getDatabase should not return error")
	require.NotNil(t, db, "returned database must not be nil")
	assert.Same(t, tenantDB, db, "tenant DB must take precedence over static connection")
	assert.Equal(t, "tenant-priority", db.Name(), "returned DB name should be the tenant DB name, not static-db")
}

// =============================================================================
// getDatabase Tests — Single-Tenant Fallback Path
// =============================================================================

func TestGetDatabase_FallsBackToStaticConnection_WhenNoTenantContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     context.Context
		wantErr bool
	}{
		{
			name:    "plain_background_context_no_tenant",
			ctx:     context.Background(),
			wantErr: true,
		},
		{
			name:    "context_with_unrelated_values",
			ctx:     context.WithValue(context.Background(), struct{}{}, "unrelated"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange — placeholder connection without a live MongoDB client.
			// GetDB will attempt to connect and fail since there is no real server.
			repo := &MongoDBRepository{
				connection: newPlaceholderConnection("fallback-db"),
				Database:   "fallback-db",
			}

			// Act
			db, err := repo.getDatabase(tt.ctx)

			// Assert — the static connection path runs but fails because there is
			// no live MongoDB server. This proves the fallback path was taken.
			if tt.wantErr {
				require.Error(t, err, "getDatabase should return error when static connection has no live MongoDB")
				assert.Nil(t, db, "database should be nil when static connection fails")
			}
		})
	}
}

func TestGetDatabase_FallsBack_WhenTenantDBIsNilInContext(t *testing.T) {
	t.Parallel()

	// Arrange — inject a nil *mongo.Database into context via ContextWithTenantMongo.
	// GetMongoForTenant checks db != nil, so it should return ErrTenantContextRequired,
	// which causes getDatabase to fall through to the static connection path.
	ctx := tmcore.ContextWithTenantMongo(context.Background(), nil)

	repo := &MongoDBRepository{
		connection: newPlaceholderConnection("fallback-nil-db"),
		Database:   "fallback-nil-db",
	}

	// Act
	db, err := repo.getDatabase(ctx)

	// Assert — falls back to static connection which fails (no live MongoDB)
	require.Error(t, err, "getDatabase should return error when tenant DB is nil and static connection has no server")
	assert.Nil(t, db, "database should be nil when both tenant context is nil and static connection fails")
}

// =============================================================================
// getDatabase Tests — Tenant Isolation
// =============================================================================

func TestGetDatabase_TwoTenants_ResolveToDifferentDatabases(t *testing.T) {
	t.Parallel()

	// Arrange — create two distinct tenant databases simulating different tenants
	tenantAcmeDB := newDisconnectedDatabase(t, "tenant-acme")
	tenantGlobexDB := newDisconnectedDatabase(t, "tenant-globex")

	repo := &MongoDBRepository{
		connection: newPlaceholderConnection("static-db"),
		Database:   "static-db",
	}

	ctxTenantA := tmcore.ContextWithTenantMongo(context.Background(), tenantAcmeDB)
	ctxTenantB := tmcore.ContextWithTenantMongo(context.Background(), tenantGlobexDB)

	// Act — resolve database for each tenant
	dbA, errA := repo.getDatabase(ctxTenantA)
	dbB, errB := repo.getDatabase(ctxTenantB)

	// Assert — both succeed and return different database instances
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

	// Arrange — same tenant DB injected into two separate contexts
	tenantDB := newDisconnectedDatabase(t, "tenant-acme")

	repo := &MongoDBRepository{
		connection: newPlaceholderConnection("static-db"),
		Database:   "static-db",
	}

	ctx1 := tmcore.ContextWithTenantMongo(context.Background(), tenantDB)
	ctx2 := tmcore.ContextWithTenantMongo(context.Background(), tenantDB)

	// Act
	db1, err1 := repo.getDatabase(ctx1)
	db2, err2 := repo.getDatabase(ctx2)

	// Assert — same tenant DB must return the same pointer
	require.NoError(t, err1, "getDatabase should not error for first call")
	require.NoError(t, err2, "getDatabase should not error for second call")
	assert.Same(t, db1, db2, "same tenant DB must return the same *mongo.Database pointer")
}

// =============================================================================
// getDatabase Tests — Error Cases
// =============================================================================

func TestGetDatabase_NilConnection_NoTenantContext_ReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange — repository with nil connection (multi-tenant mode) but no tenant in context.
	// getDatabase must return a descriptive error instead of panicking on nil receiver.
	repo := &MongoDBRepository{
		connection: nil,
		Database:   "",
	}

	// Act
	db, err := repo.getDatabase(context.Background())

	// Assert — must return error, not panic
	require.Error(t, err, "getDatabase must return error when connection is nil and no tenant context exists")
	assert.Nil(t, db, "database must be nil when no connection and no tenant context")
	assert.Contains(t, err.Error(), "no database connection available",
		"error message should indicate no connection is available")
}

func TestGetDatabase_NilConnection_WithTenantContext_Succeeds(t *testing.T) {
	t.Parallel()

	// Arrange — nil connection but tenant context is present.
	// This is the normal multi-tenant mode: connection is nil, tenant provides the DB.
	tenantDB := newDisconnectedDatabase(t, "tenant-nil-conn")

	repo := &MongoDBRepository{
		connection: nil,
		Database:   "",
	}

	ctx := tmcore.ContextWithTenantMongo(context.Background(), tenantDB)

	// Act
	db, err := repo.getDatabase(ctx)

	// Assert — should succeed using the tenant context path
	require.NoError(t, err, "getDatabase should not error when tenant DB is in context")
	require.NotNil(t, db, "returned database must not be nil")
	assert.Same(t, tenantDB, db, "must return the exact tenant DB from context")
	assert.Equal(t, "tenant-nil-conn", db.Name(), "database name should match the tenant DB")
}

func TestGetDatabase_StaticConnection_ReturnsDatabaseWithLowercaseName(t *testing.T) {
	t.Parallel()

	// Arrange — create a static connection with an already-connected client.
	// This tests the happy path of the single-tenant fallback: static connection works.
	staticClient, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := staticClient.Disconnect(context.Background()); err != nil {
			t.Logf("warning: failed to disconnect static client: %v", err)
		}
	})

	staticConn := &libMongo.MongoConnection{
		ConnectionStringSource: "mongodb://localhost:27017",
		Database:               "MyDatabase",
		Logger:                 &libLog.NoneLogger{},
		DB:                     staticClient,
		Connected:              true,
	}

	repo := &MongoDBRepository{
		connection: staticConn,
		Database:   "MyDatabase",
	}

	// Act — no tenant context, so falls back to static connection
	db, dbErr := repo.getDatabase(context.Background())

	// Assert — static path should succeed and lowercase the database name
	require.NoError(t, dbErr, "getDatabase should not return error when static connection has a client")
	require.NotNil(t, db, "returned database must not be nil")
	assert.Equal(t, "mydatabase", db.Name(), "database name should be lowercased by strings.ToLower")
}
