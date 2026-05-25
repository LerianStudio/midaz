// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeysetFromEntity(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	rotatedAt := now.Add(time.Hour)

	entity := &mmodel.OrganizationKeyset{
		TenantID:       "tenant-a",
		OrganizationID: "org-a",
		KEKPath:        "transit/keys/org-a",
		WrappedKeyset:  "vault:v1:encrypted-dek",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: 2,
			Keys: []mmodel.KeyInfo{
				{KeyID: 1, Status: "ENABLED", Type: "LEGACY_AES_GCM", IsPrimary: false},
				{KeyID: 2, Status: "ENABLED", Type: "AES256_GCM", IsPrimary: true},
			},
		},
		LegacyKeyImported:     true,
		WrappedHMACKeyset:     "vault:v1:encrypted-hmac",
		HMACKeysetInfo:        mmodel.KeysetInfo{PrimaryKeyID: 1},
		LegacyHMACKeyImported: true,
		Revision:              5,
		CreatedAt:             now,
		RotatedAt:             &rotatedAt,
	}

	model := KeysetFromEntity(entity)

	require.NotNil(t, model)
	assert.Equal(t, entity.TenantID, model.TenantID)
	assert.Equal(t, entity.OrganizationID, model.OrganizationID)
	assert.Equal(t, entity.KEKPath, model.KEKPath)
	assert.Equal(t, entity.WrappedKeyset, model.WrappedKeyset)
	assert.Equal(t, entity.KeysetInfo.PrimaryKeyID, model.KeysetInfo.PrimaryKeyID)
	assert.Len(t, model.KeysetInfo.Keys, 2)
	assert.Equal(t, entity.LegacyKeyImported, model.LegacyKeyImported)
	assert.Equal(t, entity.WrappedHMACKeyset, model.WrappedHMACKeyset)
	assert.Equal(t, entity.LegacyHMACKeyImported, model.LegacyHMACKeyImported)
	assert.Equal(t, entity.Revision, model.Revision)
	assert.Equal(t, entity.CreatedAt, model.CreatedAt)
	assert.Equal(t, entity.RotatedAt, model.RotatedAt)
}

func TestKeysetFromEntity_NilEntity(t *testing.T) {
	t.Parallel()

	model := KeysetFromEntity(nil)

	assert.Nil(t, model)
}

func TestKeysetMongoDBModel_ToEntity(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	rotatedAt := now.Add(time.Hour)

	model := &KeysetMongoDBModel{
		TenantID:       "tenant-a",
		OrganizationID: "org-a",
		KEKPath:        "transit/keys/org-a",
		WrappedKeyset:  "vault:v1:encrypted-dek",
		KeysetInfo: KeysetInfoModel{
			PrimaryKeyID: 2,
			Keys: []KeyInfoModel{
				{KeyID: 1, Status: "ENABLED", Type: "LEGACY_AES_GCM", IsPrimary: false},
				{KeyID: 2, Status: "ENABLED", Type: "AES256_GCM", IsPrimary: true},
			},
		},
		LegacyKeyImported:     true,
		WrappedHMACKeyset:     "vault:v1:encrypted-hmac",
		HMACKeysetInfo:        KeysetInfoModel{PrimaryKeyID: 1},
		LegacyHMACKeyImported: true,
		Revision:              5,
		CreatedAt:             now,
		RotatedAt:             &rotatedAt,
	}

	entity := model.ToEntity()

	require.NotNil(t, entity)
	assert.Equal(t, model.TenantID, entity.TenantID)
	assert.Equal(t, model.OrganizationID, entity.OrganizationID)
	assert.Equal(t, model.KEKPath, entity.KEKPath)
	assert.Equal(t, model.WrappedKeyset, entity.WrappedKeyset)
	assert.Equal(t, model.KeysetInfo.PrimaryKeyID, entity.KeysetInfo.PrimaryKeyID)
	assert.Len(t, entity.KeysetInfo.Keys, 2)
	assert.Equal(t, model.LegacyKeyImported, entity.LegacyKeyImported)
	assert.Equal(t, model.WrappedHMACKeyset, entity.WrappedHMACKeyset)
	assert.Equal(t, model.LegacyHMACKeyImported, entity.LegacyHMACKeyImported)
	assert.Equal(t, model.Revision, entity.Revision)
	assert.Equal(t, model.CreatedAt, entity.CreatedAt)
	assert.Equal(t, model.RotatedAt, entity.RotatedAt)
}

func TestKeysetMongoDBModel_ToEntity_NilModel(t *testing.T) {
	t.Parallel()

	var model *KeysetMongoDBModel

	entity := model.ToEntity()

	assert.Nil(t, entity)
}

func TestKeysetConversion_RoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	original := &mmodel.OrganizationKeyset{
		TenantID:       "tenant-a",
		OrganizationID: "org-a",
		KEKPath:        "transit/keys/org-a",
		WrappedKeyset:  "vault:v1:encrypted-dek",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: 2,
			Keys: []mmodel.KeyInfo{
				{KeyID: 1, Status: "ENABLED", Type: "LEGACY_AES_GCM", IsPrimary: false},
				{KeyID: 2, Status: "ENABLED", Type: "AES256_GCM", IsPrimary: true},
			},
		},
		LegacyKeyImported:     true,
		WrappedHMACKeyset:     "vault:v1:encrypted-hmac",
		HMACKeysetInfo:        mmodel.KeysetInfo{PrimaryKeyID: 1},
		LegacyHMACKeyImported: true,
		Revision:              5,
		CreatedAt:             now,
	}

	// Convert to model and back
	model := KeysetFromEntity(original)
	recovered := model.ToEntity()

	assert.Equal(t, original.TenantID, recovered.TenantID)
	assert.Equal(t, original.OrganizationID, recovered.OrganizationID)
	assert.Equal(t, original.KEKPath, recovered.KEKPath)
	assert.Equal(t, original.WrappedKeyset, recovered.WrappedKeyset)
	assert.Equal(t, original.KeysetInfo.PrimaryKeyID, recovered.KeysetInfo.PrimaryKeyID)
	assert.Equal(t, len(original.KeysetInfo.Keys), len(recovered.KeysetInfo.Keys))
	assert.Equal(t, original.LegacyKeyImported, recovered.LegacyKeyImported)
	assert.Equal(t, original.WrappedHMACKeyset, recovered.WrappedHMACKeyset)
	assert.Equal(t, original.LegacyHMACKeyImported, recovered.LegacyHMACKeyImported)
	assert.Equal(t, original.Revision, recovered.Revision)
}
