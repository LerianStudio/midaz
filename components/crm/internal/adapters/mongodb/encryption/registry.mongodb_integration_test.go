//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	mongotestutil "github.com/LerianStudio/midaz/v3/tests/utils/mongodb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============================================================================
// Test Helpers
// ============================================================================

// createRegistryRepository creates a RegistryMongoDBRepository for integration testing.
// Resets the global index tracker for this database to ensure a fresh state,
// since each test runs with a new MongoDB container.
func createRegistryRepository(t *testing.T, container *mongotestutil.ContainerResult) *RegistryMongoDBRepository {
	t.Helper()

	// Reset index tracker state for this database — each test has a fresh container
	globalIndexTracker.reset(container.DBName + ":" + registryCollection)

	conn := mongotestutil.CreateConnection(t, container.URI, container.DBName)

	repo, err := NewRegistryMongoDBRepository(conn)
	require.NoError(t, err)

	return repo
}

// createValidRegistry creates a valid OrganizationRegistryRecord for testing.
func createValidRegistry(t *testing.T, tenantID, organizationID string) *mmodel.OrganizationRegistryRecord {
	t.Helper()

	record, err := mmodel.NewOrganizationRegistryRecord(tenantID, organizationID, "system", "integration test setup")
	require.NoError(t, err)

	return record
}

// ============================================================================
// Save Tests
// ============================================================================

func TestIntegration_RegistryRepo_Save(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-" + uuid.New().String()[:8]
	organizationID := "org-" + uuid.New().String()[:8]
	registry := createValidRegistry(t, tenantID, organizationID)

	// Act
	err := repo.Save(ctx, registry)

	// Assert
	require.NoError(t, err, "Save should not return error")

	// Verify via direct query
	count := mongotestutil.CountDocuments(t, container.Database, registryCollection, bson.M{"organization_id": organizationID})
	assert.Equal(t, int64(1), count, "should have exactly 1 document")
}

func TestIntegration_RegistryRepo_Save_AlreadyExists(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-dup-" + uuid.New().String()[:8]
	organizationID := "org-dup-" + uuid.New().String()[:8]

	registry1 := createValidRegistry(t, tenantID, organizationID)

	// First save should succeed
	err := repo.Save(ctx, registry1)
	require.NoError(t, err, "first save should succeed")

	// Act - Try to save again with same organization_id
	registry2 := createValidRegistry(t, tenantID, organizationID)
	err = repo.Save(ctx, registry2)

	// Assert
	require.Error(t, err, "second save should fail")
	assert.ErrorIs(t, err, mmodel.ErrRegistryAlreadyExists, "should return ErrRegistryAlreadyExists")
}

func TestIntegration_RegistryRepo_Save_DifferentOrganizations(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-" + uuid.New().String()[:8]
	org1 := "org-1-" + uuid.New().String()[:8]
	org2 := "org-2-" + uuid.New().String()[:8]

	registry1 := createValidRegistry(t, tenantID, org1)
	registry2 := createValidRegistry(t, tenantID, org2)

	// Act
	err1 := repo.Save(ctx, registry1)
	err2 := repo.Save(ctx, registry2)

	// Assert - Both should succeed
	require.NoError(t, err1, "first org save should succeed")
	require.NoError(t, err2, "second org save should succeed")

	count := mongotestutil.CountDocuments(t, container.Database, registryCollection, bson.M{})
	assert.Equal(t, int64(2), count, "should have 2 documents")
}

func TestIntegration_RegistryRepo_Save_InitialStatus(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-status-" + uuid.New().String()[:8]
	organizationID := "org-status-" + uuid.New().String()[:8]
	registry := createValidRegistry(t, tenantID, organizationID)

	// Act
	err := repo.Save(ctx, registry)
	require.NoError(t, err)

	// Assert - Get and verify initial status (NewOrganizationRegistryRecord sets active)
	result, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.Equal(t, mmodel.RegistryStatusActive, result.Status, "initial status should be active")
	assert.Equal(t, mmodel.ProtectionModelEnvelope, result.ProtectionModel, "initial protection model should be envelope")
}

func TestIntegration_RegistryRepo_Save_WithReadableVersions(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-versions-" + uuid.New().String()[:8]
	organizationID := "org-versions-" + uuid.New().String()[:8]
	registry := createValidRegistry(t, tenantID, organizationID)
	registry.ReadableVersions = []int{1, 2, 3}
	registry.CurrentVersion = 3

	// Act
	err := repo.Save(ctx, registry)
	require.NoError(t, err)

	// Assert
	result, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, result.ReadableVersions)
	assert.Equal(t, 3, result.CurrentVersion)
}

// ============================================================================
// Get Tests
// ============================================================================

func TestIntegration_RegistryRepo_Get(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-get-" + uuid.New().String()[:8]
	organizationID := "org-get-" + uuid.New().String()[:8]
	registry := createValidRegistry(t, tenantID, organizationID)

	err := repo.Save(ctx, registry)
	require.NoError(t, err)

	// Act
	result, err := repo.Get(ctx, organizationID)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, organizationID, result.OrganizationID)
	assert.Equal(t, tenantID, result.TenantID)
	assert.Equal(t, mmodel.RegistryStatusActive, result.Status)
}

func TestIntegration_RegistryRepo_Get_NotFound(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	nonExistentOrg := "org-notfound-" + uuid.New().String()[:8]

	// Act
	result, err := repo.Get(ctx, nonExistentOrg)

	// Assert
	require.Error(t, err, "should return error for non-existent organization")
	assert.Nil(t, result)
	assert.ErrorIs(t, err, mmodel.ErrRegistryNotFound, "should return ErrRegistryNotFound")
}

func TestIntegration_RegistryRepo_Get_ReturnsAllFields(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-fields-" + uuid.New().String()[:8]
	organizationID := "org-fields-" + uuid.New().String()[:8]
	registry := createValidRegistry(t, tenantID, organizationID)
	registry.LegacyReadable = true
	registry.CurrentVersion = 2
	registry.ReadableVersions = []int{1, 2}

	err := repo.Save(ctx, registry)
	require.NoError(t, err)

	// Act
	result, err := repo.Get(ctx, organizationID)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, tenantID, result.TenantID)
	assert.Equal(t, organizationID, result.OrganizationID)
	assert.Equal(t, mmodel.RegistryStatusActive, result.Status)
	assert.Equal(t, mmodel.ProtectionModelEnvelope, result.ProtectionModel)
	assert.Equal(t, 2, result.CurrentVersion)
	assert.Equal(t, []int{1, 2}, result.ReadableVersions)
	assert.True(t, result.LegacyReadable)
	assert.Equal(t, "system", result.CreatedBy)
	assert.Equal(t, "system", result.UpdatedBy)
	assert.Equal(t, "integration test setup", result.LastTransitionReason)
	assert.False(t, result.CreatedAt.IsZero())
	assert.False(t, result.UpdatedAt.IsZero())
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_RegistryRepo_Update(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-update-" + uuid.New().String()[:8]
	organizationID := "org-update-" + uuid.New().String()[:8]
	registry := createValidRegistry(t, tenantID, organizationID)

	err := repo.Save(ctx, registry)
	require.NoError(t, err)

	// Get the saved registry
	saved, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)

	// Modify registry
	saved.Status = mmodel.RegistryStatusActive
	saved.ProtectionModel = mmodel.ProtectionModelEnvelope
	saved.CurrentVersion = 1
	saved.ReadableVersions = []int{1}
	saved.UpdatedBy = "migration-service"
	saved.LastTransitionReason = "migration completed"

	// Act
	err = repo.Update(ctx, saved, saved.Revision)

	// Assert
	require.NoError(t, err)

	// Verify update
	result, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.Equal(t, mmodel.RegistryStatusActive, result.Status)
	assert.Equal(t, mmodel.ProtectionModelEnvelope, result.ProtectionModel)
	assert.Equal(t, 1, result.CurrentVersion)
	assert.Equal(t, []int{1}, result.ReadableVersions)
	assert.Equal(t, "migration-service", result.UpdatedBy)
	assert.Equal(t, "migration completed", result.LastTransitionReason)
}

func TestIntegration_RegistryRepo_Update_IncrementRevision(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-increv-" + uuid.New().String()[:8]
	organizationID := "org-increv-" + uuid.New().String()[:8]
	registry := createValidRegistry(t, tenantID, organizationID)

	err := repo.Save(ctx, registry)
	require.NoError(t, err)

	saved, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	initialRevision := saved.Revision

	// Act - Update a field to trigger revision increment
	saved.LegacyReadable = !saved.LegacyReadable
	err = repo.Update(ctx, saved, initialRevision)
	require.NoError(t, err)

	// Assert
	result, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.Equal(t, initialRevision+1, result.Revision, "revision should increment")
}

func TestIntegration_RegistryRepo_Update_RevisionConflict(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-conflict-" + uuid.New().String()[:8]
	organizationID := "org-conflict-" + uuid.New().String()[:8]
	registry := createValidRegistry(t, tenantID, organizationID)

	err := repo.Save(ctx, registry)
	require.NoError(t, err)

	saved, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)

	// Act - Try to update with wrong revision
	saved.LegacyReadable = true
	wrongRevision := int64(999)
	err = repo.Update(ctx, saved, wrongRevision)

	// Assert
	require.Error(t, err, "should return error for revision conflict")
	assert.ErrorIs(t, err, mmodel.ErrRegistryRevisionConflict, "should return ErrRegistryRevisionConflict")

	// Verify original data unchanged
	result, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.False(t, result.LegacyReadable, "LegacyReadable should not be updated")
}

func TestIntegration_RegistryRepo_Update_DoesNotMutateInputOnFailure(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-nomutate-" + uuid.New().String()[:8]
	organizationID := "org-nomutate-" + uuid.New().String()[:8]
	registry := createValidRegistry(t, tenantID, organizationID)

	err := repo.Save(ctx, registry)
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

func TestIntegration_RegistryRepo_Update_NotFound(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-notfound-" + uuid.New().String()[:8]
	nonExistentOrg := "org-updatenotfound-" + uuid.New().String()[:8]
	registry := createValidRegistry(t, tenantID, nonExistentOrg)

	// Act - Try to update non-existent registry
	err := repo.Update(ctx, registry, 1)

	// Assert
	require.Error(t, err, "should return error for non-existent organization")
	assert.ErrorIs(t, err, mmodel.ErrRegistryRevisionConflict, "should return revision conflict (no document matched)")
}

func TestIntegration_RegistryRepo_Update_MultipleUpdates(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-multi-" + uuid.New().String()[:8]
	organizationID := "org-multi-" + uuid.New().String()[:8]
	registry := createValidRegistry(t, tenantID, organizationID)

	err := repo.Save(ctx, registry)
	require.NoError(t, err)

	// Act - Perform multiple sequential updates
	for i := 1; i <= 4; i++ {
		current, err := repo.Get(ctx, organizationID)
		require.NoError(t, err)

		current.CurrentVersion = i
		current.LastTransitionReason = "update " + string(rune('0'+i))
		err = repo.Update(ctx, current, current.Revision)
		require.NoError(t, err, "update %d should succeed", i)
	}

	// Assert
	final, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.Equal(t, mmodel.RegistryStatusActive, final.Status)
	assert.Equal(t, 4, final.CurrentVersion)
	assert.Equal(t, int64(5), final.Revision, "revision should be 5 after 4 updates") // 1 initial + 4 updates
}

// ============================================================================
// Status Tests
// ============================================================================

func TestIntegration_RegistryRepo_Update_Status(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-" + uuid.New().String()[:8]
	organizationID := "org-" + uuid.New().String()[:8]
	registry := createValidRegistry(t, tenantID, organizationID)

	err := repo.Save(ctx, registry)
	require.NoError(t, err)

	saved, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.Equal(t, mmodel.RegistryStatusActive, saved.Status)

	// Act - Update the reason field
	saved.LastTransitionReason = "updated reason"
	err = repo.Update(ctx, saved, saved.Revision)

	// Assert
	require.NoError(t, err)
	result, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.Equal(t, mmodel.RegistryStatusActive, result.Status)
	assert.Equal(t, "updated reason", result.LastTransitionReason)
}

// ============================================================================
// Protection Model Tests
// ============================================================================

func TestIntegration_RegistryRepo_Update_ProtectionModel(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-protection-" + uuid.New().String()[:8]
	organizationID := "org-protection-" + uuid.New().String()[:8]
	registry := createValidRegistry(t, tenantID, organizationID)

	err := repo.Save(ctx, registry)
	require.NoError(t, err)

	saved, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.Equal(t, mmodel.ProtectionModelEnvelope, saved.ProtectionModel)

	// Act - Switch to legacy encryption (to test protection model updates work both directions)
	saved.ProtectionModel = mmodel.ProtectionModelLegacy
	saved.Status = mmodel.RegistryStatusActive
	err = repo.Update(ctx, saved, saved.Revision)

	// Assert
	require.NoError(t, err)
	result, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.Equal(t, mmodel.ProtectionModelLegacy, result.ProtectionModel)
}

// ============================================================================
// Index Constraint Tests
// ============================================================================

func TestIntegration_RegistryRepo_UniqueIndex_OrganizationID(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-unique-" + uuid.New().String()[:8]
	organizationID := "org-unique-" + uuid.New().String()[:8]

	// Save first registry
	registry1 := createValidRegistry(t, tenantID, organizationID)
	err := repo.Save(ctx, registry1)
	require.NoError(t, err)

	// Act - Try to insert directly via MongoDB (bypassing Save logic)
	registry2Model := RegistryFromEntity(createValidRegistry(t, tenantID, organizationID))
	_, err = container.Database.Collection(registryCollection).InsertOne(ctx, registry2Model)

	// Assert - Should fail due to unique index
	require.Error(t, err, "direct insert with duplicate organization_id should fail")
	assert.Contains(t, err.Error(), "duplicate key", "should be a duplicate key error")
}

// ============================================================================
// Round-Trip Tests
// ============================================================================

func TestIntegration_RegistryRepo_RoundTrip(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-roundtrip-" + uuid.New().String()[:8]
	organizationID := "org-roundtrip-" + uuid.New().String()[:8]
	original := createValidRegistry(t, tenantID, organizationID)
	original.LegacyReadable = true
	original.CurrentVersion = 3
	original.ReadableVersions = []int{1, 2, 3}

	// Act - Save and retrieve
	err := repo.Save(ctx, original)
	require.NoError(t, err)

	result, err := repo.Get(ctx, organizationID)

	// Assert - All fields should match
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, original.TenantID, result.TenantID)
	assert.Equal(t, original.OrganizationID, result.OrganizationID)
	assert.Equal(t, mmodel.RegistryStatusActive, result.Status, "status should be active from constructor")
	assert.Equal(t, original.ProtectionModel, result.ProtectionModel)
	assert.Equal(t, original.CurrentVersion, result.CurrentVersion)
	assert.Equal(t, original.ReadableVersions, result.ReadableVersions)
	assert.Equal(t, original.LegacyReadable, result.LegacyReadable)
	assert.Equal(t, original.CreatedBy, result.CreatedBy)
	assert.Equal(t, original.UpdatedBy, result.UpdatedBy)
	assert.Equal(t, original.LastTransitionReason, result.LastTransitionReason)
}

func TestIntegration_RegistryRepo_RoundTrip_EmptyReadableVersions(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-empty-" + uuid.New().String()[:8]
	organizationID := "org-empty-" + uuid.New().String()[:8]
	registry := createValidRegistry(t, tenantID, organizationID)
	registry.ReadableVersions = []int{}

	// Act
	err := repo.Save(ctx, registry)
	require.NoError(t, err)

	result, err := repo.Get(ctx, organizationID)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, result.ReadableVersions, "empty readable_versions should be preserved")
}

// ============================================================================
// Concurrent Update Tests
// ============================================================================

func TestIntegration_RegistryRepo_ConcurrentUpdate_OptimisticLocking(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRegistryRepository(t, container)
	ctx := context.Background()

	tenantID := "tenant-concurrent-" + uuid.New().String()[:8]
	organizationID := "org-concurrent-" + uuid.New().String()[:8]
	registry := createValidRegistry(t, tenantID, organizationID)

	err := repo.Save(ctx, registry)
	require.NoError(t, err)

	// Simulate two concurrent reads
	snapshot1, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)

	snapshot2, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)

	// First update succeeds
	snapshot1.CurrentVersion = 2
	err = repo.Update(ctx, snapshot1, snapshot1.Revision)
	require.NoError(t, err, "first update should succeed")

	// Act - Second update should fail (stale revision)
	snapshot2.CurrentVersion = 3
	err = repo.Update(ctx, snapshot2, snapshot2.Revision)

	// Assert
	require.Error(t, err, "second update should fail due to stale revision")
	assert.ErrorIs(t, err, mmodel.ErrRegistryRevisionConflict)

	// Verify first update was applied
	result, err := repo.Get(ctx, organizationID)
	require.NoError(t, err)
	assert.Equal(t, 2, result.CurrentVersion)
	assert.Equal(t, int64(2), result.Revision)
}
