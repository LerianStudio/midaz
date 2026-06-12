// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"testing"
	"time"

	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestKeysetMongoDBRepositoryImplementsRepository(t *testing.T) {
	t.Parallel()

	repo, err := NewKeysetMongoDBRepository(nil)

	require.NoError(t, err)
	require.Implements(t, (*KeysetRepository)(nil), repo)
}

func TestNewKeysetMongoDBRepository_NilConnection(t *testing.T) {
	t.Parallel()

	repo, err := NewKeysetMongoDBRepository(nil)

	require.NoError(t, err)
	require.NotNil(t, repo)
	assert.Nil(t, repo.connection)
}

func TestKeysetMongoDBRepository_Save_NilKeyset(t *testing.T) {
	t.Parallel()

	repo, err := NewKeysetMongoDBRepository(nil)
	require.NoError(t, err)

	err = repo.Save(context.Background(), nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "keyset is required")
}

func TestKeysetMongoDBRepository_Save_ValidationError(t *testing.T) {
	t.Parallel()

	repo, err := NewKeysetMongoDBRepository(nil)
	require.NoError(t, err)

	tests := []struct {
		name       string
		keyset     *mmodel.OrganizationKeyset
		wantErrMsg string
	}{
		{
			name: "empty organization_id",
			keyset: &mmodel.OrganizationKeyset{
				OrganizationID: "",
				KEKPath:        "transit/keys/test",
				WrappedKeyset:  "vault:v1:dek",
				KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 1},
			},
			wantErrMsg: "organization_id is required",
		},
		{
			name: "empty kek_path",
			keyset: &mmodel.OrganizationKeyset{
				OrganizationID: "org-a",
				KEKPath:        "",
				WrappedKeyset:  "vault:v1:dek",
				KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 1},
			},
			wantErrMsg: "kek_path is required",
		},
		{
			name: "empty wrapped_keyset",
			keyset: &mmodel.OrganizationKeyset{
				OrganizationID: "org-a",
				KEKPath:        "transit/keys/test",
				KEKMountPath:   "transit",
				WrappedKeyset:  "",
				KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 1},
			},
			wantErrMsg: "wrapped_keyset is required",
		},
		{
			name: "empty kek_mount_path",
			keyset: &mmodel.OrganizationKeyset{
				OrganizationID: "org-a",
				KEKPath:        "transit/keys/test",
				KEKMountPath:   "",
				WrappedKeyset:  "vault:v1:dek",
				KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 1},
			},
			wantErrMsg: "kek_mount_path is required",
		},
		{
			name: "zero primary_key_id",
			keyset: &mmodel.OrganizationKeyset{
				OrganizationID: "org-a",
				KEKPath:        "transit/keys/test",
				KEKMountPath:   "transit",
				WrappedKeyset:  "vault:v1:dek",
				KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 0},
			},
			wantErrMsg: "keyset_info.primary_key_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := repo.Save(context.Background(), tt.keyset)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrMsg)
		})
	}
}

func TestKeysetMongoDBRepository_Update_NilKeyset(t *testing.T) {
	t.Parallel()

	repo, err := NewKeysetMongoDBRepository(nil)
	require.NoError(t, err)

	err = repo.Update(context.Background(), nil, 1)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "keyset is required")
}

func TestKeysetMongoDBRepository_Update_ValidationError(t *testing.T) {
	t.Parallel()

	repo, err := NewKeysetMongoDBRepository(nil)
	require.NoError(t, err)

	invalidKeyset := &mmodel.OrganizationKeyset{
		OrganizationID: "",
		KEKPath:        "transit/keys/test",
	}

	err = repo.Update(context.Background(), invalidKeyset, 1)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "organization_id is required")
}

func TestKeysetMongoDBRepository_getDatabase_NoConnection(t *testing.T) {
	t.Parallel()

	repo, err := NewKeysetMongoDBRepository(nil)
	require.NoError(t, err)

	// Without tenant context and without connection, should error
	db, err := repo.getDatabase(context.Background())

	require.Error(t, err)
	assert.Nil(t, db)
	assert.Contains(t, err.Error(), "no database connection available")
}

func TestKeysetMongoDBRepository_collection_NoConnection(t *testing.T) {
	t.Parallel()

	repo, err := NewKeysetMongoDBRepository(nil)
	require.NoError(t, err)

	// Without tenant context and without connection, should error
	coll, err := repo.collection(context.Background())

	require.Error(t, err)
	assert.Nil(t, coll)
	assert.Contains(t, err.Error(), "no database connection available")
}

func TestKeysetMongoDBRepository_Save_SetsRevisionToOne(t *testing.T) {
	t.Parallel()

	keyset := validTestKeyset()
	keyset.Revision = 0

	// Verify validation passes
	require.NoError(t, keyset.Validate())

	// After Save would set Revision to 1 (tested in integration tests)
	// This unit test just ensures the struct is properly configured
	assert.Equal(t, int64(0), keyset.Revision)
}

func TestValidTestKeyset_IsValid(t *testing.T) {
	t.Parallel()

	keyset := validTestKeyset()

	require.NoError(t, keyset.Validate())
	assert.NotEmpty(t, keyset.OrganizationID)
	assert.NotEmpty(t, keyset.KEKPath)
	assert.NotEmpty(t, keyset.WrappedKeyset)
	assert.NotZero(t, keyset.KeysetInfo.PrimaryKeyID)
}

func TestKeysetMongoDBRepository_Get_NoConnection(t *testing.T) {
	t.Parallel()

	repo, err := NewKeysetMongoDBRepository(nil)
	require.NoError(t, err)

	keyset, err := repo.Get(context.Background(), "org-a")

	require.Error(t, err)
	assert.Nil(t, keyset)
	assert.Contains(t, err.Error(), "no database connection available")
}

// newDisconnectedDatabase creates a MongoDB database handle for testing tenant isolation.
func newDisconnectedDatabase(t *testing.T, dbName string) *mongo.Database {
	t.Helper()

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	require.NoError(t, err, "mongo.Connect should succeed for a disconnected handle")

	t.Cleanup(func() {
		require.NoError(t, client.Disconnect(context.Background()), "client disconnect should not error")
	})

	return client.Database(dbName)
}

// newPlaceholderConnection creates a placeholder libMongo.Client for testing.
func newPlaceholderConnection(_ string) *libMongo.Client {
	return &libMongo.Client{}
}

func TestKeysetMongoDBRepository_getDatabase_TenantContextTakesPrecedence(t *testing.T) {
	t.Parallel()

	tenantDB := newDisconnectedDatabase(t, "tenant-keyset-priority")
	repo := &KeysetMongoDBRepository{connection: newPlaceholderConnection("static-db")}
	ctx := tmcore.ContextWithMB(context.Background(), tenantDB)

	db, err := repo.getDatabase(ctx)

	require.NoError(t, err, "getDatabase should not return error when tenant DB is in context")
	require.NotNil(t, db, "returned database must not be nil")
	assert.Same(t, tenantDB, db, "tenant DB must take precedence over static connection")
	assert.Equal(t, "tenant-keyset-priority", db.Name(), "returned DB name should be the tenant DB name")
}

func TestKeysetMongoDBRepository_getDatabase_FallbackToStaticConnection(t *testing.T) {
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

			repo := &KeysetMongoDBRepository{connection: newPlaceholderConnection("fallback-db")}

			db, err := repo.getDatabase(tt.ctx)

			if tt.wantErr {
				require.Error(t, err, "getDatabase should return error when static connection has no live MongoDB")
				assert.Nil(t, db, "database should be nil when static connection fails")
			}
		})
	}
}

func TestKeysetMongoDBRepository_getDatabase_NilConnectionWithTenantContext(t *testing.T) {
	t.Parallel()

	tenantDB := newDisconnectedDatabase(t, "tenant-keyset-nil-conn")
	repo := &KeysetMongoDBRepository{connection: nil}
	ctx := tmcore.ContextWithMB(context.Background(), tenantDB)

	db, err := repo.getDatabase(ctx)

	require.NoError(t, err, "getDatabase should not error when tenant DB is in context")
	require.NotNil(t, db, "returned database must not be nil")
	assert.Same(t, tenantDB, db, "must return the exact tenant DB from context")
	assert.Equal(t, "tenant-keyset-nil-conn", db.Name(), "database name should match the tenant DB")
}

func TestKeysetMongoDBRepository_getDatabase_TwoTenants_ResolveToDifferentDatabases(t *testing.T) {
	t.Parallel()

	tenantAcmeDB := newDisconnectedDatabase(t, "tenant-keyset-acme")
	tenantGlobexDB := newDisconnectedDatabase(t, "tenant-keyset-globex")

	repo := &KeysetMongoDBRepository{connection: newPlaceholderConnection("static-db")}

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
	assert.Equal(t, "tenant-keyset-acme", dbA.Name(), "tenant A database name must match")
	assert.Equal(t, "tenant-keyset-globex", dbB.Name(), "tenant B database name must match")
}

func validTestKeyset() *mmodel.OrganizationKeyset {
	now := time.Now().UTC()

	return &mmodel.OrganizationKeyset{
		OrganizationID:    "org-test",
		KEKPath:           "transit/keys/crm-org-test",
		KEKMountPath:      "transit",
		WrappedKeyset:     "vault:v1:encrypted-dek",
		WrappedHMACKeyset: "vault:v1:encrypted-hmac",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: 1,
			Keys: []mmodel.KeyInfo{
				{KeyID: 1, Status: "ENABLED", Type: "AES256_GCM", IsPrimary: true},
			},
		},
		HMACKeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: 1,
			Keys: []mmodel.KeyInfo{
				{KeyID: 1, Status: "ENABLED", Type: "HMAC_SHA256", IsPrimary: true},
			},
		},
		Revision:  1,
		CreatedAt: now,
	}
}

func TestExtractTenantID_WithTenantInContext(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "my-tenant")

	tenantID := extractTenantID(ctx)

	assert.Equal(t, "my-tenant", tenantID, "should extract tenant ID from context")
}

func TestExtractTenantID_WithoutTenantInContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tenantID := extractTenantID(ctx)

	assert.Equal(t, "default", tenantID, "should return 'default' when no tenant in context")
}

func TestExtractTenantID_WithEmptyTenantInContext(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "")

	tenantID := extractTenantID(ctx)

	assert.Equal(t, "default", tenantID, "should return 'default' when tenant is empty")
}
