// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedRegistryTime is a deterministic UTC instant shared by the registry
// conversion tests so fixtures never depend on the wall clock.
var fixedRegistryTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestRegistryFromEntity(t *testing.T) {
	t.Parallel()

	now := fixedRegistryTime

	entity := &mmodel.OrganizationRegistryRecord{
		TenantID:             "tenant-a",
		OrganizationID:       "org-a",
		Status:               mmodel.RegistryStatusActive,
		ProtectionModel:      mmodel.ProtectionModelEnvelope,
		CurrentVersion:       1,
		ReadableVersions:     []int{1},
		Revision:             2,
		LegacyReadable:       true,
		CreatedAt:            now,
		UpdatedAt:            now,
		CreatedBy:            "system",
		UpdatedBy:            "admin",
		LastTransitionReason: "activated",
	}

	model := RegistryFromEntity(entity)

	require.NotNil(t, model)
	assert.Equal(t, entity.TenantID, model.TenantID)
	assert.Equal(t, entity.OrganizationID, model.OrganizationID)
	assert.Equal(t, entity.Status, model.Status)
	assert.Equal(t, entity.ProtectionModel, model.ProtectionModel)
	assert.Equal(t, entity.CurrentVersion, model.CurrentVersion)
	assert.Equal(t, entity.ReadableVersions, model.ReadableVersions)
	assert.Equal(t, entity.Revision, model.Revision)
	assert.Equal(t, entity.LegacyReadable, model.LegacyReadable)
	assert.Equal(t, entity.CreatedAt, model.CreatedAt)
	assert.Equal(t, entity.UpdatedAt, model.UpdatedAt)
	assert.Equal(t, entity.CreatedBy, model.CreatedBy)
	assert.Equal(t, entity.UpdatedBy, model.UpdatedBy)
	assert.Equal(t, entity.LastTransitionReason, model.LastTransitionReason)
}

func TestRegistryFromEntity_NilEntity(t *testing.T) {
	t.Parallel()

	model := RegistryFromEntity(nil)

	assert.Nil(t, model)
}

func TestRegistryFromEntity_CopiesSlice(t *testing.T) {
	t.Parallel()

	entity := &mmodel.OrganizationRegistryRecord{
		ReadableVersions: []int{1, 2, 3},
	}

	model := RegistryFromEntity(entity)

	// Modify original slice
	entity.ReadableVersions[0] = 999

	// Model should not be affected
	assert.Equal(t, 1, model.ReadableVersions[0])
}

func TestRegistryMongoDBModel_ToEntity(t *testing.T) {
	t.Parallel()

	now := fixedRegistryTime

	model := &RegistryMongoDBModel{
		TenantID:             "tenant-a",
		OrganizationID:       "org-a",
		Status:               mmodel.RegistryStatusActive,
		ProtectionModel:      mmodel.ProtectionModelEnvelope,
		CurrentVersion:       1,
		ReadableVersions:     []int{1},
		Revision:             2,
		LegacyReadable:       true,
		CreatedAt:            now,
		UpdatedAt:            now,
		CreatedBy:            "system",
		UpdatedBy:            "admin",
		LastTransitionReason: "activated",
	}

	entity := model.ToEntity()

	require.NotNil(t, entity)
	assert.Equal(t, model.TenantID, entity.TenantID)
	assert.Equal(t, model.OrganizationID, entity.OrganizationID)
	assert.Equal(t, model.Status, entity.Status)
	assert.Equal(t, model.ProtectionModel, entity.ProtectionModel)
	assert.Equal(t, model.CurrentVersion, entity.CurrentVersion)
	assert.Equal(t, model.ReadableVersions, entity.ReadableVersions)
	assert.Equal(t, model.Revision, entity.Revision)
	assert.Equal(t, model.LegacyReadable, entity.LegacyReadable)
	assert.Equal(t, model.CreatedAt, entity.CreatedAt)
	assert.Equal(t, model.UpdatedAt, entity.UpdatedAt)
	assert.Equal(t, model.CreatedBy, entity.CreatedBy)
	assert.Equal(t, model.UpdatedBy, entity.UpdatedBy)
	assert.Equal(t, model.LastTransitionReason, entity.LastTransitionReason)
}

func TestRegistryMongoDBModel_ToEntity_NilModel(t *testing.T) {
	t.Parallel()

	var model *RegistryMongoDBModel

	entity := model.ToEntity()

	assert.Nil(t, entity)
}

func TestRegistryMongoDBModel_ToEntity_CopiesSlice(t *testing.T) {
	t.Parallel()

	model := &RegistryMongoDBModel{
		ReadableVersions: []int{1, 2, 3},
	}

	entity := model.ToEntity()

	// Modify model slice
	model.ReadableVersions[0] = 999

	// Entity should not be affected
	assert.Equal(t, 1, entity.ReadableVersions[0])
}

func TestRegistryConversion_RoundTrip(t *testing.T) {
	t.Parallel()

	now := fixedRegistryTime

	original := &mmodel.OrganizationRegistryRecord{
		TenantID:             "tenant-a",
		OrganizationID:       "org-a",
		Status:               mmodel.RegistryStatusActive,
		ProtectionModel:      mmodel.ProtectionModelEnvelope,
		CurrentVersion:       1,
		ReadableVersions:     []int{1, 2},
		Revision:             5,
		LegacyReadable:       true,
		CreatedAt:            now,
		UpdatedAt:            now,
		CreatedBy:            "system",
		UpdatedBy:            "admin",
		LastTransitionReason: "key rotated",
	}

	// Convert to model and back
	model := RegistryFromEntity(original)
	recovered := model.ToEntity()

	assert.Equal(t, original.TenantID, recovered.TenantID)
	assert.Equal(t, original.OrganizationID, recovered.OrganizationID)
	assert.Equal(t, original.Status, recovered.Status)
	assert.Equal(t, original.ProtectionModel, recovered.ProtectionModel)
	assert.Equal(t, original.CurrentVersion, recovered.CurrentVersion)
	assert.Equal(t, original.ReadableVersions, recovered.ReadableVersions)
	assert.Equal(t, original.Revision, recovered.Revision)
	assert.Equal(t, original.LegacyReadable, recovered.LegacyReadable)
	assert.Equal(t, original.CreatedBy, recovered.CreatedBy)
	assert.Equal(t, original.UpdatedBy, recovered.UpdatedBy)
	assert.Equal(t, original.LastTransitionReason, recovered.LastTransitionReason)
}
