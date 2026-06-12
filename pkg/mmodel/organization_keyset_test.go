// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
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
