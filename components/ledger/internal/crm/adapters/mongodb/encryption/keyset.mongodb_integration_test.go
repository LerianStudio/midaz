//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============================================================================
// Test Helpers
// ============================================================================

// createKeysetRepository creates a KeysetMongoDBRepository for integration testing.
// Resets the global index tracker for this database to ensure a fresh state,
// since each test runs with a new MongoDB container.
func createKeysetRepository(t *testing.T, container *mongotestutil.ContainerResult) *KeysetMongoDBRepository {
	t.Helper()

	// Reset index tracker state for this database — each test has a fresh container
	globalIndexTracker.reset(container.DBName + ":" + keysetCollection)

	conn := mongotestutil.CreateConnection(t, container.URI, container.DBName)

	repo, err := NewKeysetMongoDBRepository(conn)
	require.NoError(t, err)

	return repo
}

// createValidKeyset creates a valid OrganizationKeyset for testing.
func createValidKeyset(organizationID string) *mmodel.OrganizationKeyset {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	return &mmodel.OrganizationKeyset{
		OrganizationID: organizationID,
		Version:        1,
		KEKPath:        "transit/keys/crm-" + organizationID,
		KEKMountPath:   "transit",
		WrappedKeyset:  "vault:v1:encrypted-dek-" + uuid.New().String()[:8],
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: 1,
			Keys: []mmodel.KeyInfo{
				{KeyID: 1, Status: "ENABLED", Type: "AES256_GCM", IsPrimary: true},
			},
		},
		WrappedHMACKeyset: "vault:v1:encrypted-hmac-" + uuid.New().String()[:8],
		HMACKeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: 1,
			Keys: []mmodel.KeyInfo{
				{KeyID: 1, Status: "ENABLED", Type: "HMAC_SHA256", IsPrimary: true},
			},
		},
		Revision:  0,
		CreatedAt: now,
	}
}

// ============================================================================
// Save Tests
// ============================================================================

func TestIntegration_KeysetRepo_Save(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-" + uuid.New().String()[:8]
	keyset := createValidKeyset(organizationID)

	// Act
	err := repo.Save(ctx, keyset)

	// Assert
	require.NoError(t, err, "Save should not return error")

	// Verify via direct query
	count := mongotestutil.CountDocuments(t, container.Database, keysetCollection, bson.M{"organization_id": organizationID})
	assert.Equal(t, int64(1), count, "should have exactly 1 document")
}

func TestIntegration_KeysetRepo_Save_SetsRevisionToOne(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-rev-" + uuid.New().String()[:8]
	keyset := createValidKeyset(organizationID)
	keyset.Revision = 0

	// Act
	err := repo.Save(ctx, keyset)
	require.NoError(t, err)

	// Assert - Get and verify revision was set to 1
	result, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Revision, "revision should be set to 1 on save")
}

func TestIntegration_KeysetRepo_Save_AlreadyExists(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-dup-" + uuid.New().String()[:8]
	keyset1 := createValidKeyset(organizationID)

	// First save should succeed
	err := repo.Save(ctx, keyset1)
	require.NoError(t, err, "first save should succeed")

	// Act - Try to save again with same organization_id
	keyset2 := createValidKeyset(organizationID)
	err = repo.Save(ctx, keyset2)

	// Assert
	require.Error(t, err, "second save should fail")
	assert.ErrorIs(t, err, mmodel.ErrKeysetAlreadyExists, "should return ErrKeysetAlreadyExists")
}

func TestIntegration_KeysetRepo_Save_DifferentOrganizations(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	org1 := "org-1-" + uuid.New().String()[:8]
	org2 := "org-2-" + uuid.New().String()[:8]

	keyset1 := createValidKeyset(org1)
	keyset2 := createValidKeyset(org2)

	// Act
	err1 := repo.Save(ctx, keyset1)
	err2 := repo.Save(ctx, keyset2)

	// Assert - Both should succeed
	require.NoError(t, err1, "first org save should succeed")
	require.NoError(t, err2, "second org save should succeed")

	count := mongotestutil.CountDocuments(t, container.Database, keysetCollection, bson.M{})
	assert.Equal(t, int64(2), count, "should have 2 documents")
}

func TestIntegration_KeysetRepo_Save_WithHMACKeyset(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-hmac-" + uuid.New().String()[:8]
	keyset := createValidKeyset(organizationID)

	// Act
	err := repo.Save(ctx, keyset)
	require.NoError(t, err)

	// Assert - Get and verify HMAC fields
	result, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.NotEmpty(t, result.WrappedHMACKeyset, "HMAC keyset should be persisted")
	assert.Equal(t, uint32(1), result.HMACKeysetInfo.PrimaryKeyID, "HMAC keyset info should be persisted")
}

func TestIntegration_KeysetRepo_Save_WithoutHMACKeyset(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-no-hmac-" + uuid.New().String()[:8]
	keyset := createValidKeyset(organizationID)
	keyset.WrappedHMACKeyset = ""
	keyset.HMACKeysetInfo = mmodel.KeysetInfo{}

	// Act
	err := repo.Save(ctx, keyset)
	require.NoError(t, err)

	// Assert
	result, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.Empty(t, result.WrappedHMACKeyset, "HMAC keyset should be empty")
}

// ============================================================================
// Get Tests
// ============================================================================

func TestIntegration_KeysetRepo_Get(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-get-" + uuid.New().String()[:8]
	keyset := createValidKeyset(organizationID)

	err := repo.Save(ctx, keyset)
	require.NoError(t, err)

	// Act
	result, err := repo.Get(ctx, organizationID)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, organizationID, result.OrganizationID)
	assert.Equal(t, keyset.KEKPath, result.KEKPath)
	assert.Equal(t, keyset.WrappedKeyset, result.WrappedKeyset)
	assert.Equal(t, keyset.KeysetInfo.PrimaryKeyID, result.KeysetInfo.PrimaryKeyID)
}

func TestIntegration_KeysetRepo_Get_NotFound(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	nonExistentOrg := "org-notfound-" + uuid.New().String()[:8]

	// Act
	result, err := repo.Get(ctx, nonExistentOrg)

	// Assert
	require.Error(t, err, "should return error for non-existent organization")
	assert.Nil(t, result)
	assert.ErrorIs(t, err, mmodel.ErrKeysetNotFound, "should return ErrKeysetNotFound")
}

func TestIntegration_KeysetRepo_Get_ReturnsAllFields(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-fields-" + uuid.New().String()[:8]
	keyset := createValidKeyset(organizationID)

	err := repo.Save(ctx, keyset)
	require.NoError(t, err)

	// Act
	result, err := repo.Get(ctx, organizationID)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, keyset.OrganizationID, result.OrganizationID)
	assert.Equal(t, keyset.KEKPath, result.KEKPath)
	assert.Equal(t, keyset.WrappedKeyset, result.WrappedKeyset)
	assert.Equal(t, keyset.WrappedHMACKeyset, result.WrappedHMACKeyset)
	assert.Equal(t, int64(1), result.Revision, "revision should be 1")
	assert.False(t, result.CreatedAt.IsZero(), "created_at should be set")
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_KeysetRepo_Update(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-update-" + uuid.New().String()[:8]
	keyset := createValidKeyset(organizationID)

	err := repo.Save(ctx, keyset)
	require.NoError(t, err)

	// Get the saved keyset to confirm revision
	saved, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	require.Equal(t, int64(1), saved.Revision)

	// Modify keyset
	saved.WrappedKeyset = "vault:v2:new-encrypted-dek"
	saved.KEKPath = "transit/keys/crm-" + organizationID + "-rotated"

	// Act
	err = repo.Update(ctx, saved, 1)

	// Assert
	require.NoError(t, err)

	// Verify update
	result, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.Equal(t, "vault:v2:new-encrypted-dek", result.WrappedKeyset)
	assert.Equal(t, "transit/keys/crm-"+organizationID+"-rotated", result.KEKPath)
}

func TestIntegration_KeysetRepo_Update_IncrementRevision(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-increv-" + uuid.New().String()[:8]
	keyset := createValidKeyset(organizationID)

	err := repo.Save(ctx, keyset)
	require.NoError(t, err)

	saved, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	require.Equal(t, int64(1), saved.Revision)

	// Act - Update with correct revision
	saved.WrappedKeyset = "vault:v2:updated-dek"
	err = repo.Update(ctx, saved, 1)
	require.NoError(t, err)

	// Assert - Revision should increment
	result, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Revision, "revision should increment to 2")
}

func TestIntegration_KeysetRepo_Update_RevisionConflict(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-conflict-" + uuid.New().String()[:8]
	keyset := createValidKeyset(organizationID)

	err := repo.Save(ctx, keyset)
	require.NoError(t, err)

	saved, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)

	// Act - Try to update with wrong revision
	saved.WrappedKeyset = "vault:v2:should-fail"
	wrongRevision := int64(999)
	err = repo.Update(ctx, saved, wrongRevision)

	// Assert
	require.Error(t, err, "should return error for revision conflict")
	assert.ErrorIs(t, err, mmodel.ErrKeysetRevisionConflict, "should return ErrKeysetRevisionConflict")

	// Verify original data unchanged
	result, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.NotEqual(t, "vault:v2:should-fail", result.WrappedKeyset, "data should not be updated")
	assert.Equal(t, int64(1), result.Revision, "revision should still be 1")
}

func TestIntegration_KeysetRepo_Update_DoesNotMutateInputOnFailure(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-nomutate-" + uuid.New().String()[:8]
	keyset := createValidKeyset(organizationID)

	err := repo.Save(ctx, keyset)
	require.NoError(t, err)

	saved, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)

	initialRevision := saved.Revision
	wrongRevision := int64(999)

	// Act - Update should fail
	err = repo.Update(ctx, saved, wrongRevision)

	// Assert - Input object should not be mutated
	require.Error(t, err)
	assert.Equal(t, initialRevision, saved.Revision, "input revision should not be mutated on failure")
}

func TestIntegration_KeysetRepo_Update_NotFound(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	nonExistentOrg := "org-updatenotfound-" + uuid.New().String()[:8]
	keyset := createValidKeyset(nonExistentOrg)
	keyset.Revision = 1

	// Act - Try to update non-existent keyset
	err := repo.Update(ctx, keyset, 1)

	// Assert
	require.Error(t, err, "should return error for non-existent organization")
	assert.ErrorIs(t, err, mmodel.ErrKeysetRevisionConflict, "should return revision conflict (no document matched)")
}

func TestIntegration_KeysetRepo_Update_MultipleUpdates(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-multi-" + uuid.New().String()[:8]
	keyset := createValidKeyset(organizationID)

	err := repo.Save(ctx, keyset)
	require.NoError(t, err)

	// Act - Perform multiple sequential updates
	for i := 1; i <= 5; i++ {
		current, err := repo.Get(ctx, organizationID)
		require.NoError(t, err)

		current.WrappedKeyset = "vault:v" + string(rune('0'+i)) + ":dek-update-" + string(rune('0'+i))
		err = repo.Update(ctx, current, current.Revision)
		require.NoError(t, err, "update %d should succeed", i)
	}

	// Assert
	final, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.Equal(t, int64(6), final.Revision, "revision should be 6 after 5 updates")
}

func TestIntegration_KeysetRepo_Update_RotatedAt(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-rotated-" + uuid.New().String()[:8]
	keyset := createValidKeyset(organizationID)

	err := repo.Save(ctx, keyset)
	require.NoError(t, err)

	saved, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)

	// Set rotated_at timestamp
	rotatedAt := time.Now().UTC().Truncate(time.Second)
	saved.RotatedAt = &rotatedAt
	saved.WrappedKeyset = "vault:v2:rotated-dek"

	// Act
	err = repo.Update(ctx, saved, saved.Revision)
	require.NoError(t, err)

	// Assert
	result, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	require.NotNil(t, result.RotatedAt, "rotated_at should be set")
	assert.Equal(t, rotatedAt.Unix(), result.RotatedAt.Unix(), "rotated_at should match")
}

// ============================================================================
// Index Constraint Tests
// ============================================================================

func TestIntegration_KeysetRepo_UniqueIndex_OrganizationID(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-unique-" + uuid.New().String()[:8]

	// Save first keyset. Save stamps tenant_id from context (single-tenant: "default").
	keyset1 := createValidKeyset(organizationID)
	err := repo.Save(ctx, keyset1)
	require.NoError(t, err)

	// Act - Try to insert directly via MongoDB (bypassing Save logic). The unique index
	// is compound over (tenant_id, organization_id, version), so the direct insert must
	// carry the same tenant_id Save stamped to collide.
	keyset2 := createValidKeyset(organizationID)
	keyset2.TenantID = keyset1.TenantID
	keyset2Model := KeysetFromEntity(keyset2)
	_, err = container.Database.Collection(keysetCollection).InsertOne(ctx, keyset2Model)

	// Assert - Should fail due to unique index
	require.Error(t, err, "direct insert with duplicate (tenant_id, organization_id, version) should fail")
	assert.Contains(t, err.Error(), "duplicate key", "should be a duplicate key error")
}

// ============================================================================
// Round-Trip Tests
// ============================================================================

func TestIntegration_KeysetRepo_RoundTrip(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-roundtrip-" + uuid.New().String()[:8]
	original := createValidKeyset(organizationID)

	// Act - Save and retrieve
	err := repo.Save(ctx, original)
	require.NoError(t, err)

	result, err := repo.Get(ctx, organizationID)

	// Assert - All fields should match
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, original.OrganizationID, result.OrganizationID)
	assert.Equal(t, original.KEKPath, result.KEKPath)
	assert.Equal(t, original.KEKMountPath, result.KEKMountPath)
	assert.Equal(t, original.WrappedKeyset, result.WrappedKeyset)
	assert.Equal(t, original.WrappedHMACKeyset, result.WrappedHMACKeyset)
	assert.Equal(t, original.KeysetInfo.PrimaryKeyID, result.KeysetInfo.PrimaryKeyID)
	assert.Equal(t, original.HMACKeysetInfo.PrimaryKeyID, result.HMACKeysetInfo.PrimaryKeyID)
	assert.Equal(t, int64(1), result.Revision)

	// Keys array
	require.Len(t, result.KeysetInfo.Keys, len(original.KeysetInfo.Keys))
	assert.Equal(t, original.KeysetInfo.Keys[0].KeyID, result.KeysetInfo.Keys[0].KeyID)
	assert.Equal(t, original.KeysetInfo.Keys[0].Status, result.KeysetInfo.Keys[0].Status)
	assert.Equal(t, original.KeysetInfo.Keys[0].Type, result.KeysetInfo.Keys[0].Type)
	assert.Equal(t, original.KeysetInfo.Keys[0].IsPrimary, result.KeysetInfo.Keys[0].IsPrimary)
}

// createMixedKeyset builds a MIXED keyset whose AEAD (KeysetInfo) and PRF
// (HMACKeysetInfo) sides BOTH hold two keys: a fresh envelope PRIMARY and an
// imported legacy ENABLED non-primary entry. This mirrors the document E-1.2
// manual provisioning persists for a migrated organization.
func createMixedKeyset(organizationID string) *mmodel.OrganizationKeyset {
	keyset := createValidKeyset(organizationID)

	keyset.KeysetInfo = mmodel.KeysetInfo{
		PrimaryKeyID: 100,
		Keys: []mmodel.KeyInfo{
			{KeyID: 100, Status: "ENABLED", Type: "AES256_GCM", IsPrimary: true},
			{KeyID: 1, Status: "ENABLED", Type: "LEGACY_AES_GCM", IsPrimary: false},
		},
	}
	keyset.HMACKeysetInfo = mmodel.KeysetInfo{
		PrimaryKeyID: 200,
		Keys: []mmodel.KeyInfo{
			{KeyID: 200, Status: "ENABLED", Type: "HMAC_PRF", IsPrimary: true},
			{KeyID: 1, Status: "ENABLED", Type: "LEGACY_HMAC_SHA256", IsPrimary: false},
		},
	}

	return keyset
}

// TestIntegration_KeysetRepo_RoundTrip_MixedKeyset proves a MIXED keyset document
// round-trips through real MongoDB persistence with BOTH the AEAD and PRF keys
// arrays preserving their two entries (correct primary flags + legacy metadata).
// Existing integration round-trips only exercise the AEAD keys array; the PRF
// (hmac_keyset_info.keys) array is the migrated-org gap this asserts.
func TestIntegration_KeysetRepo_RoundTrip_MixedKeyset(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-mixed-" + uuid.New().String()[:8]
	original := createMixedKeyset(organizationID)

	// Act
	err := repo.Save(ctx, original)
	require.NoError(t, err)

	result, err := repo.Get(ctx, organizationID)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	// AEAD side: two keys, fresh primary + legacy non-primary.
	assert.Equal(t, uint32(100), result.KeysetInfo.PrimaryKeyID, "AEAD primary key ID preserved")
	require.Len(t, result.KeysetInfo.Keys, 2, "AEAD keyset must keep both keys")
	assert.Equal(t, uint32(100), result.KeysetInfo.Keys[0].KeyID)
	assert.True(t, result.KeysetInfo.Keys[0].IsPrimary, "fresh AEAD key is primary")
	assert.Equal(t, "AES256_GCM", result.KeysetInfo.Keys[0].Type)
	assert.Equal(t, uint32(1), result.KeysetInfo.Keys[1].KeyID)
	assert.False(t, result.KeysetInfo.Keys[1].IsPrimary, "legacy AEAD key is non-primary")
	assert.Equal(t, "LEGACY_AES_GCM", result.KeysetInfo.Keys[1].Type)
	assert.Equal(t, "ENABLED", result.KeysetInfo.Keys[1].Status, "legacy AEAD key enabled")

	// PRF side: two keys, fresh primary + legacy non-primary.
	assert.Equal(t, uint32(200), result.HMACKeysetInfo.PrimaryKeyID, "PRF primary key ID preserved")
	require.Len(t, result.HMACKeysetInfo.Keys, 2, "PRF keyset must keep both keys")
	assert.Equal(t, uint32(200), result.HMACKeysetInfo.Keys[0].KeyID)
	assert.True(t, result.HMACKeysetInfo.Keys[0].IsPrimary, "fresh PRF key is primary")
	assert.Equal(t, "HMAC_PRF", result.HMACKeysetInfo.Keys[0].Type)
	assert.Equal(t, uint32(1), result.HMACKeysetInfo.Keys[1].KeyID)
	assert.False(t, result.HMACKeysetInfo.Keys[1].IsPrimary, "legacy PRF key is non-primary")
	assert.Equal(t, "LEGACY_HMAC_SHA256", result.HMACKeysetInfo.Keys[1].Type)
	assert.Equal(t, "ENABLED", result.HMACKeysetInfo.Keys[1].Status, "legacy PRF key enabled")
}

func TestIntegration_KeysetRepo_RoundTrip_WithMultipleKeys(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createKeysetRepository(t, container)
	ctx := context.Background()

	organizationID := "org-multikey-" + uuid.New().String()[:8]
	keyset := createValidKeyset(organizationID)

	// Add multiple keys (simulating key rotation history)
	keyset.KeysetInfo.Keys = []mmodel.KeyInfo{
		{KeyID: 1, Status: "DISABLED", Type: "AES256_GCM", IsPrimary: false},
		{KeyID: 2, Status: "DISABLED", Type: "AES256_GCM", IsPrimary: false},
		{KeyID: 3, Status: "ENABLED", Type: "AES256_GCM", IsPrimary: true},
	}
	keyset.KeysetInfo.PrimaryKeyID = 3

	// Act
	err := repo.Save(ctx, keyset)
	require.NoError(t, err)

	result, err := repo.Get(ctx, organizationID)

	// Assert
	require.NoError(t, err)
	require.Len(t, result.KeysetInfo.Keys, 3, "should have 3 keys")
	assert.Equal(t, uint32(3), result.KeysetInfo.PrimaryKeyID)

	// Verify key order and values
	assert.Equal(t, uint32(1), result.KeysetInfo.Keys[0].KeyID)
	assert.Equal(t, "DISABLED", result.KeysetInfo.Keys[0].Status)
	assert.Equal(t, uint32(3), result.KeysetInfo.Keys[2].KeyID)
	assert.True(t, result.KeysetInfo.Keys[2].IsPrimary)
}
