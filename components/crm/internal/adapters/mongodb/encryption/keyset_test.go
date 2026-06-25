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
	"go.mongodb.org/mongo-driver/bson"
)

func TestKeysetFromEntity(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	rotatedAt := now.Add(time.Hour)

	entity := &mmodel.OrganizationKeyset{
		TenantID:       "tenant-a",
		OrganizationID: "org-a",
		KEKPath:        "transit/keys/org-123",
		KEKMountPath:   "transit/tenant-a",
		WrappedKeyset:  "vault:v1:encrypted-dek",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: 2,
			Keys: []mmodel.KeyInfo{
				{KeyID: 1, Status: "ENABLED", Type: "LEGACY_AES_GCM", IsPrimary: false},
				{KeyID: 2, Status: "ENABLED", Type: "AES256_GCM", IsPrimary: true},
			},
		},
		WrappedHMACKeyset: "vault:v1:encrypted-hmac",
		HMACKeysetInfo:    mmodel.KeysetInfo{PrimaryKeyID: 1},
		Revision:          5,
		CreatedAt:         now,
		RotatedAt:         &rotatedAt,
	}

	model := KeysetFromEntity(entity)

	require.NotNil(t, model)
	assert.Equal(t, entity.TenantID, model.TenantID)
	assert.Equal(t, entity.OrganizationID, model.OrganizationID)
	assert.Equal(t, entity.KEKPath, model.KEKPath)
	assert.Equal(t, entity.KEKMountPath, model.KEKMountPath)
	assert.Equal(t, entity.WrappedKeyset, model.WrappedKeyset)
	assert.Equal(t, entity.KeysetInfo.PrimaryKeyID, model.KeysetInfo.PrimaryKeyID)
	assert.Len(t, model.KeysetInfo.Keys, 2)
	assert.Equal(t, entity.WrappedHMACKeyset, model.WrappedHMACKeyset)
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
		KEKPath:        "transit/keys/org-123",
		KEKMountPath:   "transit/tenant-a",
		WrappedKeyset:  "vault:v1:encrypted-dek",
		KeysetInfo: KeysetInfoModel{
			PrimaryKeyID: 2,
			Keys: []KeyInfoModel{
				{KeyID: 1, Status: "ENABLED", Type: "LEGACY_AES_GCM", IsPrimary: false},
				{KeyID: 2, Status: "ENABLED", Type: "AES256_GCM", IsPrimary: true},
			},
		},
		WrappedHMACKeyset: "vault:v1:encrypted-hmac",
		HMACKeysetInfo:    KeysetInfoModel{PrimaryKeyID: 1},
		Revision:          5,
		CreatedAt:         now,
		RotatedAt:         &rotatedAt,
	}

	entity := model.ToEntity()

	require.NotNil(t, entity)
	assert.Equal(t, model.TenantID, entity.TenantID)
	assert.Equal(t, model.OrganizationID, entity.OrganizationID)
	assert.Equal(t, model.KEKPath, entity.KEKPath)
	assert.Equal(t, model.KEKMountPath, entity.KEKMountPath)
	assert.Equal(t, model.WrappedKeyset, entity.WrappedKeyset)
	assert.Equal(t, model.KeysetInfo.PrimaryKeyID, entity.KeysetInfo.PrimaryKeyID)
	assert.Len(t, entity.KeysetInfo.Keys, 2)
	assert.Equal(t, model.WrappedHMACKeyset, entity.WrappedHMACKeyset)
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
		KEKPath:        "transit/keys/org-123",
		KEKMountPath:   "transit/tenant-a",
		WrappedKeyset:  "vault:v1:encrypted-dek",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: 2,
			Keys: []mmodel.KeyInfo{
				{KeyID: 1, Status: "ENABLED", Type: "LEGACY_AES_GCM", IsPrimary: false},
				{KeyID: 2, Status: "ENABLED", Type: "AES256_GCM", IsPrimary: true},
			},
		},
		WrappedHMACKeyset: "vault:v1:encrypted-hmac",
		HMACKeysetInfo:    mmodel.KeysetInfo{PrimaryKeyID: 1},
		Revision:          5,
		CreatedAt:         now,
	}

	// Convert to model and back
	model := KeysetFromEntity(original)
	recovered := model.ToEntity()

	assert.Equal(t, original.TenantID, recovered.TenantID)
	assert.Equal(t, original.OrganizationID, recovered.OrganizationID)
	assert.Equal(t, original.KEKPath, recovered.KEKPath)
	assert.Equal(t, original.KEKMountPath, recovered.KEKMountPath)
	assert.Equal(t, original.WrappedKeyset, recovered.WrappedKeyset)
	assert.Equal(t, original.KeysetInfo.PrimaryKeyID, recovered.KeysetInfo.PrimaryKeyID)
	assert.Equal(t, len(original.KeysetInfo.Keys), len(recovered.KeysetInfo.Keys))
	assert.Equal(t, original.WrappedHMACKeyset, recovered.WrappedHMACKeyset)
	assert.Equal(t, original.Revision, recovered.Revision)
}

// mixedKeysetEntity builds a MIXED OrganizationKeyset: both the AEAD (KeysetInfo)
// and PRF (HMACKeysetInfo) sides carry TWO keys — a fresh envelope PRIMARY and an
// imported legacy ENABLED non-primary entry. This mirrors the document E-1.2
// manual provisioning persists for a migrated organization.
func mixedKeysetEntity(now time.Time) *mmodel.OrganizationKeyset {
	return &mmodel.OrganizationKeyset{
		TenantID:       "tenant-mixed",
		OrganizationID: "org-mixed",
		KEKPath:        "transit/keys/crm-org-mixed",
		KEKMountPath:   "transit/tenant-mixed",
		WrappedKeyset:  "vault:v1:encrypted-mixed-dek",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: 100,
			Keys: []mmodel.KeyInfo{
				{KeyID: 100, Status: "ENABLED", Type: "AES256_GCM", IsPrimary: true},
				{KeyID: 1, Status: "ENABLED", Type: "LEGACY_AES_GCM", IsPrimary: false},
			},
		},
		WrappedHMACKeyset: "vault:v1:encrypted-mixed-hmac",
		HMACKeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: 200,
			Keys: []mmodel.KeyInfo{
				{KeyID: 200, Status: "ENABLED", Type: "HMAC_PRF", IsPrimary: true},
				{KeyID: 1, Status: "ENABLED", Type: "LEGACY_HMAC_SHA256", IsPrimary: false},
			},
		},
		Revision:  3,
		CreatedAt: now,
	}
}

// assertKeysetInfoEqual asserts every KeysetInfo field round-trips, including the
// full Keys array (ID, Status, Type, IsPrimary) and the primary key ID.
func assertKeysetInfoEqual(t *testing.T, want, got mmodel.KeysetInfo, label string) {
	t.Helper()

	assert.Equal(t, want.PrimaryKeyID, got.PrimaryKeyID, "%s primary_key_id", label)
	require.Len(t, got.Keys, len(want.Keys), "%s key count", label)

	for i := range want.Keys {
		assert.Equal(t, want.Keys[i].KeyID, got.Keys[i].KeyID, "%s keys[%d].key_id", label, i)
		assert.Equal(t, want.Keys[i].Status, got.Keys[i].Status, "%s keys[%d].status", label, i)
		assert.Equal(t, want.Keys[i].Type, got.Keys[i].Type, "%s keys[%d].type", label, i)
		assert.Equal(t, want.Keys[i].IsPrimary, got.Keys[i].IsPrimary, "%s keys[%d].is_primary", label, i)
	}
}

// TestKeysetConversion_MixedKeyset_RoundTrip proves a MIXED keyset (two keys on
// BOTH the AEAD and PRF sides) survives the domain<->BSON mapper with every
// KeyInfo field and primary flag intact. The existing round-trip test only counts
// AEAD keys and ignores the HMAC keys array entirely.
func TestKeysetConversion_MixedKeyset_RoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	original := mixedKeysetEntity(now)

	recovered := KeysetFromEntity(original).ToEntity()

	require.NotNil(t, recovered)
	assert.Equal(t, original.TenantID, recovered.TenantID, "tenant_id must not be dropped")
	assert.Equal(t, original.OrganizationID, recovered.OrganizationID)
	assert.Equal(t, original.WrappedKeyset, recovered.WrappedKeyset)
	assert.Equal(t, original.WrappedHMACKeyset, recovered.WrappedHMACKeyset)

	assertKeysetInfoEqual(t, original.KeysetInfo, recovered.KeysetInfo, "AEAD keyset_info")
	assertKeysetInfoEqual(t, original.HMACKeysetInfo, recovered.HMACKeysetInfo, "PRF hmac_keyset_info")

	// Primary identity preserved on both sides: the fresh envelope key is the
	// single primary, and the legacy entry remains enabled + non-primary.
	assertSinglePrimary(t, recovered.KeysetInfo, 100, 1, "AEAD")
	assertSinglePrimary(t, recovered.HMACKeysetInfo, 200, 1, "PRF")
}

// assertSinglePrimary verifies exactly one key is primary with the expected ID and
// that the legacy key (legacyKeyID) is present, enabled, and non-primary.
func assertSinglePrimary(t *testing.T, info mmodel.KeysetInfo, wantPrimaryKeyID, legacyKeyID uint32, label string) {
	t.Helper()

	assert.Equal(t, wantPrimaryKeyID, info.PrimaryKeyID, "%s primary_key_id", label)

	primaries := 0
	legacySeen := false

	for _, k := range info.Keys {
		if k.IsPrimary {
			primaries++

			assert.Equal(t, wantPrimaryKeyID, k.KeyID, "%s primary key ID", label)
		}

		if k.KeyID == legacyKeyID {
			legacySeen = true

			assert.False(t, k.IsPrimary, "%s legacy key must be non-primary", label)
			assert.Equal(t, "ENABLED", k.Status, "%s legacy key must be enabled", label)
		}
	}

	assert.Equal(t, 1, primaries, "%s must have exactly one primary key", label)
	assert.True(t, legacySeen, "%s legacy key must be present", label)
}

// TestKeysetConversion_MixedKeyset_BSONRoundTrip proves the mapper output survives
// an actual BSON marshal/unmarshal cycle (the wire format MongoDB stores), so both
// keys arrays and primary flags are preserved through the serialization the driver
// performs — not just the in-memory struct copy.
func TestKeysetConversion_MixedKeyset_BSONRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	original := mixedKeysetEntity(now)

	raw, err := bson.Marshal(KeysetFromEntity(original))
	require.NoError(t, err, "BSON marshal of mixed keyset model should succeed")

	var decoded KeysetMongoDBModel
	require.NoError(t, bson.Unmarshal(raw, &decoded), "BSON unmarshal should succeed")

	recovered := decoded.ToEntity()

	require.NotNil(t, recovered)
	assert.Equal(t, original.TenantID, recovered.TenantID, "tenant_id must survive BSON round-trip")
	assertKeysetInfoEqual(t, original.KeysetInfo, recovered.KeysetInfo, "AEAD keyset_info (BSON)")
	assertKeysetInfoEqual(t, original.HMACKeysetInfo, recovered.HMACKeysetInfo, "PRF hmac_keyset_info (BSON)")
}

// TestKeysetConversion_EnvelopeOnly_RoundTrip proves a single-key envelope-only
// document (no legacy import, one key per side) still round-trips intact.
func TestKeysetConversion_EnvelopeOnly_RoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	original := &mmodel.OrganizationKeyset{
		TenantID:       "tenant-env",
		OrganizationID: "org-env",
		KEKPath:        "transit/keys/crm-org-env",
		KEKMountPath:   "transit",
		WrappedKeyset:  "vault:v1:encrypted-dek",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: 7,
			Keys: []mmodel.KeyInfo{
				{KeyID: 7, Status: "ENABLED", Type: "AES256_GCM", IsPrimary: true},
			},
		},
		WrappedHMACKeyset: "vault:v1:encrypted-hmac",
		HMACKeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: 9,
			Keys: []mmodel.KeyInfo{
				{KeyID: 9, Status: "ENABLED", Type: "HMAC_PRF", IsPrimary: true},
			},
		},
		Revision:  1,
		CreatedAt: now,
	}

	recovered := KeysetFromEntity(original).ToEntity()

	require.NotNil(t, recovered)
	assertKeysetInfoEqual(t, original.KeysetInfo, recovered.KeysetInfo, "AEAD keyset_info")
	assertKeysetInfoEqual(t, original.HMACKeysetInfo, recovered.HMACKeysetInfo, "PRF hmac_keyset_info")
}
