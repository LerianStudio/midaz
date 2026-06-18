// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationKeyset_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		keyset     OrganizationKeyset
		wantErrMsg string
	}{
		{
			name: "valid keyset",
			keyset: OrganizationKeyset{
				OrganizationID: "org-a",
				KEKPath:        "transit/keys/test",
				KEKMountPath:   "transit",
				WrappedKeyset:  "vault:v1:encrypted",
				KeysetInfo:     KeysetInfo{PrimaryKeyID: 1},
			},
			wantErrMsg: "",
		},
		{
			name: "empty kek_mount_path",
			keyset: OrganizationKeyset{
				OrganizationID: "org-a",
				KEKPath:        "transit/keys/test",
				KEKMountPath:   "",
				WrappedKeyset:  "vault:v1:encrypted",
				KeysetInfo:     KeysetInfo{PrimaryKeyID: 1},
			},
			wantErrMsg: "kek_mount_path is required",
		},
		{
			name: "empty organization_id",
			keyset: OrganizationKeyset{
				OrganizationID: "",
				KEKPath:        "transit/keys/test",
				WrappedKeyset:  "vault:v1:encrypted",
				KeysetInfo:     KeysetInfo{PrimaryKeyID: 1},
			},
			wantErrMsg: "organization_id is required",
		},
		{
			name: "empty kek_path",
			keyset: OrganizationKeyset{
				OrganizationID: "org-a",
				KEKPath:        "",
				WrappedKeyset:  "vault:v1:encrypted",
				KeysetInfo:     KeysetInfo{PrimaryKeyID: 1},
			},
			wantErrMsg: "kek_path is required",
		},
		{
			name: "empty wrapped_keyset",
			keyset: OrganizationKeyset{
				OrganizationID: "org-a",
				KEKPath:        "transit/keys/test",
				KEKMountPath:   "transit",
				WrappedKeyset:  "",
				KeysetInfo:     KeysetInfo{PrimaryKeyID: 1},
			},
			wantErrMsg: "wrapped_keyset is required",
		},
		{
			name: "zero primary_key_id",
			keyset: OrganizationKeyset{
				OrganizationID: "org-a",
				KEKPath:        "transit/keys/test",
				KEKMountPath:   "transit",
				WrappedKeyset:  "vault:v1:encrypted",
				KeysetInfo:     KeysetInfo{PrimaryKeyID: 0},
			},
			wantErrMsg: "keyset_info.primary_key_id is required",
		},
		{
			name: "hmac_keyset_without_hmac_info",
			keyset: OrganizationKeyset{
				OrganizationID:    "org-a",
				KEKPath:           "transit/keys/test",
				KEKMountPath:      "transit",
				WrappedKeyset:     "vault:v1:encrypted",
				KeysetInfo:        KeysetInfo{PrimaryKeyID: 1},
				WrappedHMACKeyset: "vault:v1:hmac-encrypted",
				HMACKeysetInfo:    KeysetInfo{PrimaryKeyID: 0},
			},
			wantErrMsg: "hmac_keyset_info.primary_key_id is required when wrapped_hmac_keyset is provided",
		},
		{
			name: "valid keyset with hmac",
			keyset: OrganizationKeyset{
				OrganizationID:    "org-a",
				KEKPath:           "transit/keys/test",
				KEKMountPath:      "transit",
				WrappedKeyset:     "vault:v1:encrypted",
				KeysetInfo:        KeysetInfo{PrimaryKeyID: 1},
				WrappedHMACKeyset: "vault:v1:hmac-encrypted",
				HMACKeysetInfo:    KeysetInfo{PrimaryKeyID: 2},
			},
			wantErrMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.keyset.Validate()

			if tt.wantErrMsg == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
			}
		})
	}
}

func TestOrganizationKeyset_SafeView(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	keyset := OrganizationKeyset{
		TenantID:          "tenant-a",
		OrganizationID:    "org-a",
		KEKPath:           "transit/keys/test",
		KEKMountPath:      "transit",
		WrappedKeyset:     "vault:v1:secret-dek-material",
		WrappedHMACKeyset: "vault:v1:secret-hmac-material",
		KeysetInfo:        KeysetInfo{PrimaryKeyID: 1},
		HMACKeysetInfo:    KeysetInfo{PrimaryKeyID: 2},
		Revision:          5,
		CreatedAt:         now,
	}

	safe := keyset.SafeView()

	// Verify wrapped keysets are redacted
	assert.Equal(t, "[REDACTED]", safe.WrappedKeyset)
	assert.Equal(t, "[REDACTED]", safe.WrappedHMACKeyset)

	// Verify other fields are preserved
	assert.Equal(t, keyset.TenantID, safe.TenantID)
	assert.Equal(t, keyset.OrganizationID, safe.OrganizationID)
	assert.Equal(t, keyset.KEKPath, safe.KEKPath)
	assert.Equal(t, keyset.KEKMountPath, safe.KEKMountPath, "non-secret mount path must remain visible in SafeView")
	assert.Equal(t, keyset.KeysetInfo, safe.KeysetInfo)
	assert.Equal(t, keyset.HMACKeysetInfo, safe.HMACKeysetInfo)
	assert.Equal(t, keyset.Revision, safe.Revision)
	assert.Equal(t, keyset.CreatedAt, safe.CreatedAt)
}

// Recognizable sentinel values standing in for raw legacy key material that
// manual provisioning imports into a keyset (LCRYPTO_ENCRYPT_SECRET_KEY hex AES
// key, LCRYPTO_HASH_SECRET_KEY HMAC secret). These are NOT real secrets; the
// obvious sentinel pattern lets the safety assertions detect any leak of raw
// legacy material into a redacted/serialized surface.
const (
	sentinelLegacyAESHex = "5345435245544145534b4559" + // "SECRETAESKEY" in hex
		"5345435245544145534b4559" +
		"5345435245544145534b4559" // 32-byte AES key, hex-encoded
	sentinelLegacyHMACSecret = "SENTINEL-LEGACY-HMAC-SECRET-DO-NOT-LEAK"
)

// TestOrganizationKeyset_SafeView_NeverExposesRawLegacySecret is a non-disclosure
// regression: even when an OrganizationKeyset's wrapped fields are constructed
// from recognizable raw legacy secret material, SafeView() MUST redact the
// wrapped fields so the raw secrets never reach a logging/API surface. No genuine
// RED is possible here: SafeView replaces both wrapped fields with the static
// "[REDACTED]" literal, so the raw material is structurally absent. This test
// asserts that ABSENCE over the serialized safe view and guards against a future
// regression that would echo wrapped material verbatim.
func TestOrganizationKeyset_SafeView_NeverExposesRawLegacySecret(t *testing.T) {
	t.Parallel()

	// Construct wrapped fields embedding the raw legacy sentinels, mimicking a
	// buggy persistence path that stored raw material instead of KMS ciphertext.
	keyset := OrganizationKeyset{
		TenantID:          "tenant-a",
		OrganizationID:    "org-a",
		KEKPath:           "transit/keys/test",
		KEKMountPath:      "transit",
		WrappedKeyset:     "vault:v1:" + sentinelLegacyAESHex,
		WrappedHMACKeyset: "vault:v1:" + sentinelLegacyHMACSecret,
		KeysetInfo:        KeysetInfo{PrimaryKeyID: 1},
		HMACKeysetInfo:    KeysetInfo{PrimaryKeyID: 2},
		Revision:          5,
		CreatedAt:         time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC),
	}

	safe := keyset.SafeView()

	// Wrapped fields are redacted, not echoed.
	assert.Equal(t, "[REDACTED]", safe.WrappedKeyset)
	assert.Equal(t, "[REDACTED]", safe.WrappedHMACKeyset)

	// The serialized safe view MUST NOT contain the raw legacy secret material.
	serialized, err := json.Marshal(safe)
	require.NoError(t, err)

	rendered := string(serialized) + fmt.Sprintf("%+v", safe)
	assert.NotContains(t, rendered, sentinelLegacyAESHex, "raw legacy AES key material must never appear in the safe view")
	assert.NotContains(t, rendered, sentinelLegacyHMACSecret, "raw legacy HMAC secret must never appear in the safe view")
}

// mixedKeyset builds an OrganizationKeyset carrying MIXED metadata: two AEAD key
// entries and two PRF key entries. In each keyset a freshly-generated envelope key
// is PRIMARY while an imported legacy key is ENABLED non-primary and listed first,
// mirroring the persisted shape produced by manual provisioning of imported keys
// (legacy non-primary + envelope primary).
func mixedKeyset() OrganizationKeyset {
	return OrganizationKeyset{
		TenantID:       "tenant-a",
		OrganizationID: "org-a",
		KEKPath:        "transit/keys/test",
		KEKMountPath:   "transit",
		WrappedKeyset:  "vault:v1:secret-dek-material",
		KeysetInfo: KeysetInfo{
			// Primary is the envelope key (ID 200), not the first entry.
			PrimaryKeyID: 200,
			Keys: []KeyInfo{
				{KeyID: 100, Status: "ENABLED", Type: "AES256_GCM", IsPrimary: false},
				{KeyID: 200, Status: "ENABLED", Type: "AES256_GCM", IsPrimary: true},
			},
		},
		WrappedHMACKeyset: "vault:v1:secret-hmac-material",
		HMACKeysetInfo: KeysetInfo{
			PrimaryKeyID: 400,
			Keys: []KeyInfo{
				{KeyID: 300, Status: "ENABLED", Type: "HMAC_PRF", IsPrimary: false},
				{KeyID: 400, Status: "ENABLED", Type: "HMAC_PRF", IsPrimary: true},
			},
		},
		Revision:  3,
		CreatedAt: time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC),
	}
}

// TestOrganizationKeyset_Validate_MixedKeys asserts that validation accepts a keyset
// with multiple AEAD and PRF key entries (legacy non-primary listed before the
// envelope primary). Validation keys off PrimaryKeyID, not key count or position,
// so a mixed keyset must validate cleanly.
func TestOrganizationKeyset_Validate_MixedKeys(t *testing.T) {
	t.Parallel()

	keyset := mixedKeyset()

	require.NoError(t, keyset.Validate())

	// Sanity: the metadata genuinely carries two entries per keyset, with the
	// primary not in position 0, proving Validate does not assume a single key.
	require.Len(t, keyset.KeysetInfo.Keys, 2)
	require.Len(t, keyset.HMACKeysetInfo.Keys, 2)
	assert.False(t, keyset.KeysetInfo.Keys[0].IsPrimary, "legacy AEAD entry is non-primary and listed first")
	assert.True(t, keyset.KeysetInfo.Keys[1].IsPrimary, "envelope AEAD entry is primary")
	assert.Equal(t, uint32(200), keyset.KeysetInfo.PrimaryKeyID)
	assert.False(t, keyset.HMACKeysetInfo.Keys[0].IsPrimary, "legacy PRF entry is non-primary and listed first")
	assert.True(t, keyset.HMACKeysetInfo.Keys[1].IsPrimary, "envelope PRF entry is primary")
	assert.Equal(t, uint32(400), keyset.HMACKeysetInfo.PrimaryKeyID)
}

// TestOrganizationKeyset_SafeView_MixedKeys asserts the redacted view preserves ALL
// key metadata entries (both AEAD and both PRF) while exposing no raw wrapped keyset
// material. This guards against a single-key SafeView regression and secret leakage.
func TestOrganizationKeyset_SafeView_MixedKeys(t *testing.T) {
	t.Parallel()

	keyset := mixedKeyset()

	safe := keyset.SafeView()

	// No raw wrapped keyset material is exposed in the safe view.
	assert.Equal(t, "[REDACTED]", safe.WrappedKeyset)
	assert.Equal(t, "[REDACTED]", safe.WrappedHMACKeyset)
	assert.NotContains(t, safe.WrappedKeyset, "secret-dek-material")
	assert.NotContains(t, safe.WrappedHMACKeyset, "secret-hmac-material")

	// All non-secret key metadata is preserved unchanged (both entries per keyset).
	assert.Equal(t, keyset.KeysetInfo, safe.KeysetInfo)
	assert.Equal(t, keyset.HMACKeysetInfo, safe.HMACKeysetInfo)
	require.Len(t, safe.KeysetInfo.Keys, 2)
	require.Len(t, safe.HMACKeysetInfo.Keys, 2)
	assert.Equal(t, keyset.KeysetInfo.Keys[0], safe.KeysetInfo.Keys[0])
	assert.Equal(t, keyset.KeysetInfo.Keys[1], safe.KeysetInfo.Keys[1])
	assert.Equal(t, keyset.HMACKeysetInfo.Keys[0], safe.HMACKeysetInfo.Keys[0])
	assert.Equal(t, keyset.HMACKeysetInfo.Keys[1], safe.HMACKeysetInfo.Keys[1])
	assert.Equal(t, keyset.KeysetInfo.PrimaryKeyID, safe.KeysetInfo.PrimaryKeyID)
	assert.Equal(t, keyset.HMACKeysetInfo.PrimaryKeyID, safe.HMACKeysetInfo.PrimaryKeyID)
}
