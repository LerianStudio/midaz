// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"testing"

	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
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

// newPlaceholderConnection creates a minimal *libMongo.Client placeholder.
// This simulates the multi-tenant bootstrap mode
// where the static connection is a placeholder and the real database comes from
// the per-request tenant context.
func newPlaceholderConnection() *libMongo.Client {
	return &libMongo.Client{}
}

// =============================================================================
// Constructor Tests — NewMetadataMongoDBRepository
// =============================================================================

func TestNewMetadataMongoDBRepository_NoPanic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		connection *libMongo.Client
	}{
		{
			name:       "placeholder_connection_for_multi_tenant_mode",
			connection: newPlaceholderConnection(),
		},
		{
			name:       "connection_without_active_client",
			connection: &libMongo.Client{},
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
				connection: newPlaceholderConnection(),
			}
			ctx := tmcore.ContextWithMB(context.Background(), tt.tenantDB)

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

	repo := &MetadataMongoDBRepository{
		connection: newPlaceholderConnection(),
	}

	tenantDB := newDisconnectedDatabase(t, "tenant-priority")
	ctx := tmcore.ContextWithMB(context.Background(), tenantDB)

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
				connection: newPlaceholderConnection(),
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

	// Arrange — inject a nil *mongo.Database into context via ContextWithMB.
	// GetMBContext checks db != nil, so it should return nil,
	// which causes getDatabase to fall through to the static connection path.
	ctx := tmcore.ContextWithMB(context.Background(), nil)

	repo := &MetadataMongoDBRepository{
		connection: newPlaceholderConnection(),
	}

	// Act
	db, err := repo.getDatabase(ctx)

	// Assert — falls back to static connection which fails (no live MongoDB)
	require.Error(t, err, "getDatabase should return error when tenant DB is nil and static connection has no server")
	assert.Nil(t, db, "database should be nil when both tenant context is nil and static connection fails")
}

func TestGetDatabase_StaticConnection_ReturnsErrorWithPlaceholder(t *testing.T) {
	t.Parallel()

	repo := &MetadataMongoDBRepository{
		connection: newPlaceholderConnection(),
	}

	// Act — no tenant context, so falls back to static connection
	db, dbErr := repo.getDatabase(context.Background())

	// Assert — placeholder static connection is not connected and should fail
	require.Error(t, dbErr, "getDatabase should return error when static connection has no active client")
	assert.Nil(t, db, "database should be nil when static connection is not connected")
}

func TestGetDatabase_RequireTenantWithoutContext_ReturnsError(t *testing.T) {
	t.Parallel()

	repo := NewMetadataMongoDBRepository(newPlaceholderConnection(), true)

	db, err := repo.getDatabase(context.Background())

	require.Error(t, err)
	require.ErrorContains(t, err, "tenant mongo database missing from context")
	assert.Nil(t, db, "database should be nil when fail-closed tenant mode has no tenant context")
}
