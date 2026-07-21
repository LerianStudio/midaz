// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryMongoDBRepositoryImplementsRepository(t *testing.T) {
	t.Parallel()

	repo, err := NewRegistryMongoDBRepository(nil)

	require.NoError(t, err)
	require.Implements(t, (*RegistryRepository)(nil), repo)
}

func TestNewRegistryMongoDBRepository_NilConnection(t *testing.T) {
	t.Parallel()

	repo, err := NewRegistryMongoDBRepository(nil)

	require.NoError(t, err)
	require.NotNil(t, repo)
	assert.Nil(t, repo.connection)
}

func TestRegistryMongoDBRepository_Save_NilRecord(t *testing.T) {
	t.Parallel()

	repo, err := NewRegistryMongoDBRepository(nil)
	require.NoError(t, err)

	err = repo.Save(context.Background(), nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry record is required")
}

func TestRegistryMongoDBRepository_Update_NilRecord(t *testing.T) {
	t.Parallel()

	repo, err := NewRegistryMongoDBRepository(nil)
	require.NoError(t, err)

	err = repo.Update(context.Background(), nil, 1)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry record is required")
}

func TestRegistryMongoDBRepository_getDatabase_NoConnection(t *testing.T) {
	t.Parallel()

	repo, err := NewRegistryMongoDBRepository(nil)
	require.NoError(t, err)

	// Without tenant context and without connection, should error
	db, err := repo.getDatabase(context.Background())

	require.Error(t, err)
	assert.Nil(t, db)
	assert.Contains(t, err.Error(), "no database connection available")
}

func TestRegistryMongoDBRepository_collection_NoConnection(t *testing.T) {
	t.Parallel()

	repo, err := NewRegistryMongoDBRepository(nil)
	require.NoError(t, err)

	// Without tenant context and without connection, should error
	coll, err := repo.collection(context.Background())

	require.Error(t, err)
	assert.Nil(t, coll)
	assert.Contains(t, err.Error(), "no database connection available")
}

func TestRegistryMongoDBRepository_Save_ValidRecord(t *testing.T) {
	t.Parallel()

	repo, err := NewRegistryMongoDBRepository(nil)
	require.NoError(t, err)

	record, err := mmodel.NewOrganizationRegistryRecord("tenant-a", "org-a", "system", "initial setup")
	require.NoError(t, err)

	// Without connection, Save will fail at collection() but validation passes
	err = repo.Save(context.Background(), record)

	require.Error(t, err)
	// Error should be about connection, not validation
	assert.Contains(t, err.Error(), "no database connection available")
}

func TestRegistryMongoDBRepository_Get_NoConnection(t *testing.T) {
	t.Parallel()

	repo, err := NewRegistryMongoDBRepository(nil)
	require.NoError(t, err)

	record, err := repo.Get(context.Background(), "org-a")

	require.Error(t, err)
	assert.Nil(t, record)
	assert.Contains(t, err.Error(), "no database connection available")
}

func TestRegistryMongoDBRepository_Update_ValidRecord(t *testing.T) {
	t.Parallel()

	repo, err := NewRegistryMongoDBRepository(nil)
	require.NoError(t, err)

	record, err := mmodel.NewOrganizationRegistryRecord("tenant-a", "org-a", "system", "initial setup")
	require.NoError(t, err)

	// Without connection, Update will fail at collection() but validation passes
	err = repo.Update(context.Background(), record, 1)

	require.Error(t, err)
	// Error should be about connection, not validation
	assert.Contains(t, err.Error(), "no database connection available")
}

func TestRegistryMongoDBRepository_Update_DoesNotMutateInputOnFailure(t *testing.T) {
	t.Parallel()

	repo, err := NewRegistryMongoDBRepository(nil)
	require.NoError(t, err)

	record, err := mmodel.NewOrganizationRegistryRecord("tenant-a", "org-a", "system", "initial setup")
	require.NoError(t, err)

	initialRevision := record.Revision
	expectedRevision := int64(5)

	// Update should NOT mutate the input record when the operation fails.
	// The revision increment is applied only to the internal model, not the caller's object.
	err = repo.Update(context.Background(), record, expectedRevision)

	require.Error(t, err)
	// The input record should remain unchanged because the operation failed.
	assert.Equal(t, initialRevision, record.Revision, "input record should not be mutated on failure")
}

func TestRegistryMongoDBRepository_getDatabase_TenantContextTakesPrecedence(t *testing.T) {
	t.Parallel()

	tenantDB := newDisconnectedDatabase(t, "tenant-registry-priority")
	repo := &RegistryMongoDBRepository{connection: newPlaceholderConnection("static-db")}
	ctx := tmcore.ContextWithMB(context.Background(), tenantDB)

	db, err := repo.getDatabase(ctx)

	require.NoError(t, err, "getDatabase should not return error when tenant DB is in context")
	require.NotNil(t, db, "returned database must not be nil")
	assert.Same(t, tenantDB, db, "tenant DB must take precedence over static connection")
	assert.Equal(t, "tenant-registry-priority", db.Name(), "returned DB name should be the tenant DB name")
}

func TestRegistryMongoDBRepository_getDatabase_FallbackToStaticConnection(t *testing.T) {
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

			repo := &RegistryMongoDBRepository{connection: newPlaceholderConnection("fallback-db")}

			db, err := repo.getDatabase(tt.ctx)

			if tt.wantErr {
				require.Error(t, err, "getDatabase should return error when static connection has no live MongoDB")
				assert.Nil(t, db, "database should be nil when static connection fails")
			}
		})
	}
}

func TestRegistryMongoDBRepository_getDatabase_NilConnectionWithTenantContext(t *testing.T) {
	t.Parallel()

	tenantDB := newDisconnectedDatabase(t, "tenant-registry-nil-conn")
	repo := &RegistryMongoDBRepository{connection: nil}
	ctx := tmcore.ContextWithMB(context.Background(), tenantDB)

	db, err := repo.getDatabase(ctx)

	require.NoError(t, err, "getDatabase should not error when tenant DB is in context")
	require.NotNil(t, db, "returned database must not be nil")
	assert.Same(t, tenantDB, db, "must return the exact tenant DB from context")
	assert.Equal(t, "tenant-registry-nil-conn", db.Name(), "database name should match the tenant DB")
}

func TestRegistryMongoDBRepository_getDatabase_NilConnectionWithoutTenantContext(t *testing.T) {
	t.Parallel()

	repo := &RegistryMongoDBRepository{connection: nil}

	db, err := repo.getDatabase(context.Background())

	require.Error(t, err, "getDatabase must return error when connection is nil and no tenant context exists")
	assert.Nil(t, db, "database must be nil when no connection and no tenant context")
	assert.Contains(t, err.Error(), "no database connection available", "error message should indicate no connection is available")
}

func TestRegistryMongoDBRepository_getDatabase_TwoTenants_ResolveToDifferentDatabases(t *testing.T) {
	t.Parallel()

	tenantAcmeDB := newDisconnectedDatabase(t, "tenant-registry-acme")
	tenantGlobexDB := newDisconnectedDatabase(t, "tenant-registry-globex")

	repo := &RegistryMongoDBRepository{connection: newPlaceholderConnection("static-db")}

	ctxTenantA := tmcore.ContextWithMB(context.Background(), tenantAcmeDB)
	ctxTenantB := tmcore.ContextWithMB(context.Background(), tenantGlobexDB)

	dbA, errA := repo.getDatabase(ctxTenantA)
	dbB, errB := repo.getDatabase(ctxTenantB)

	require.NoError(t, errA, "getDatabase should not error for tenant A")
	require.NoError(t, errB, "getDatabase should not error for tenant B")
	require.NotNil(t, dbA, "tenant A database must not be nil")
	require.NotNil(t, dbB, "tenant B database must not be nil")

	assert.NotSame(t, dbA, dbB, "two different tenants must resolve to different *mongo.Database instances")
	assert.NotEqual(t, dbA.Name(), dbB.Name(), "two different tenants must have different database names")
	assert.Equal(t, "tenant-registry-acme", dbA.Name(), "tenant A database name must match")
	assert.Equal(t, "tenant-registry-globex", dbB.Name(), "tenant B database name must match")
}
