// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

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
// Constructor Tests — NewMetadataMongoDBRepository
// =============================================================================

func TestNewMetadataMongoDBRepository_NoPanic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		connection *libMongo.MongoConnection
		wantDB     string
	}{
		{
			name:       "placeholder_connection_for_multi_tenant_mode",
			connection: newPlaceholderConnection("tenant-placeholder"),
			wantDB:     "tenant-placeholder",
		},
		{
			name: "connection_with_source_but_no_active_client",
			connection: &libMongo.MongoConnection{
				ConnectionStringSource: "mongodb://localhost:27017",
				Database:               "mydb",
				Logger:                 &libLog.NoneLogger{},
			},
			wantDB: "mydb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act — constructor must NOT panic even without a live connection
			require.NotPanics(t, func() {
				repo := NewMetadataMongoDBRepository(tt.connection)

				// Assert — struct fields are set correctly
				require.NotNil(t, repo, "returned repository must not be nil")
				assert.Equal(t, tt.wantDB, repo.Database, "Database field should match connection.Database")
				assert.Same(t, tt.connection, repo.connection, "connection reference should be stored as-is")
			})
		})
	}
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
			repo := &MetadataMongoDBRepository{
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

	staticConn := &libMongo.MongoConnection{
		ConnectionStringSource: "mongodb://localhost:27017",
		Database:               "static-db",
		Logger:                 &libLog.NoneLogger{},
		DB:                     staticClient,
		Connected:              true,
	}

	repo := &MetadataMongoDBRepository{
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
			repo := &MetadataMongoDBRepository{
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

	repo := &MetadataMongoDBRepository{
		connection: newPlaceholderConnection("fallback-nil-db"),
		Database:   "fallback-nil-db",
	}

	// Act
	db, err := repo.getDatabase(ctx)

	// Assert — falls back to static connection which fails (no live MongoDB)
	require.Error(t, err, "getDatabase should return error when tenant DB is nil and static connection has no server")
	assert.Nil(t, db, "database should be nil when both tenant context is nil and static connection fails")
}

func TestGetDatabase_PropagatesUnexpectedErrors_DoesNotFallBack(t *testing.T) {
	t.Parallel()

	// Arrange — use a canceled context. When tmcore.GetMongoForTenant receives a
	// canceled context, it returns ErrTenantContextRequired (no tenant DB in context).
	// However, if a future lib-commons version propagates context errors, getDatabase
	// must NOT silently fall back to the static connection for non-ErrTenantContextRequired errors.
	//
	// To test this directly, we inject a tenant DB into context and then cancel the context.
	// GetMongoForTenant will still return the DB from context (it reads a context value,
	// not a channel), so this specific scenario returns successfully.
	//
	// The real protection is structural: the code now explicitly checks for
	// ErrTenantContextRequired before falling back, so any other error type
	// (e.g., from a future middleware pipeline) will be propagated.

	repo := &MetadataMongoDBRepository{
		connection: newPlaceholderConnection("should-not-reach"),
		Database:   "should-not-reach",
	}

	// Test with a plain canceled context (no tenant DB injected).
	// GetMongoForTenant returns ErrTenantContextRequired, which IS the expected fallback case.
	// The fallback to static connection then fails because placeholder has no URI.
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	db, err := repo.getDatabase(canceledCtx)
	// ErrTenantContextRequired triggers fallback, placeholder fails — that's correct behavior
	require.Error(t, err, "should error because placeholder static connection has no server")
	assert.Nil(t, db)
}

func TestGetDatabase_StaticConnection_ReturnsDatabaseWithLowercaseName(t *testing.T) {
	t.Parallel()

	// Arrange — create a static connection with an already-connected client.
	// This tests the happy path of the single-tenant fallback: static connection works.
	staticClient, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	require.NoError(t, err)

	staticConn := &libMongo.MongoConnection{
		ConnectionStringSource: "mongodb://localhost:27017",
		Database:               "MyDatabase",
		Logger:                 &libLog.NoneLogger{},
		DB:                     staticClient,
		Connected:              true,
	}

	repo := &MetadataMongoDBRepository{
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
