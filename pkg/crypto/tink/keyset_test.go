// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeyStatus_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   KeyStatus
		expected bool
	}{
		{
			name:     "enabled is valid",
			status:   KeyStatusEnabled,
			expected: true,
		},
		{
			name:     "disabled is valid",
			status:   KeyStatusDisabled,
			expected: true,
		},
		{
			name:     "destroyed is valid",
			status:   KeyStatusDestroyed,
			expected: true,
		},
		{
			name:     "empty is invalid",
			status:   KeyStatus(""),
			expected: false,
		},
		{
			name:     "unknown is invalid",
			status:   KeyStatus("UNKNOWN"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.status.IsValid())
		})
	}
}

func TestKeyType_IsAEADType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		keyType  KeyType
		expected bool
	}{
		{
			name:     "AES256_GCM is AEAD",
			keyType:  KeyTypeAES256GCM,
			expected: true,
		},
		{
			name:     "LEGACY_AES_GCM is AEAD",
			keyType:  KeyTypeLegacyAESGCM,
			expected: true,
		},
		{
			name:     "HMAC_SHA256 is not AEAD",
			keyType:  KeyTypeHMACSHA256,
			expected: false,
		},
		{
			name:     "LEGACY_HMAC_SHA256 is not AEAD",
			keyType:  KeyTypeLegacyHMACSHA256,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.keyType.IsAEADType())
		})
	}
}

func TestKeyType_IsMACType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		keyType  KeyType
		expected bool
	}{
		{
			name:     "HMAC_SHA256 is MAC",
			keyType:  KeyTypeHMACSHA256,
			expected: true,
		},
		{
			name:     "LEGACY_HMAC_SHA256 is MAC",
			keyType:  KeyTypeLegacyHMACSHA256,
			expected: true,
		},
		{
			name:     "AES256_GCM is not MAC",
			keyType:  KeyTypeAES256GCM,
			expected: false,
		},
		{
			name:     "LEGACY_AES_GCM is not MAC",
			keyType:  KeyTypeLegacyAESGCM,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.keyType.IsMACType())
		})
	}
}

func TestKeyType_IsLegacy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		keyType  KeyType
		expected bool
	}{
		{
			name:     "LEGACY_AES_GCM is legacy",
			keyType:  KeyTypeLegacyAESGCM,
			expected: true,
		},
		{
			name:     "LEGACY_HMAC_SHA256 is legacy",
			keyType:  KeyTypeLegacyHMACSHA256,
			expected: true,
		},
		{
			name:     "AES256_GCM is not legacy",
			keyType:  KeyTypeAES256GCM,
			expected: false,
		},
		{
			name:     "HMAC_SHA256 is not legacy",
			keyType:  KeyTypeHMACSHA256,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.keyType.IsLegacy())
		})
	}
}

func TestKeysetInfo_HasKey(t *testing.T) {
	t.Parallel()

	info := KeysetInfo{
		PrimaryKeyID: 2,
		Keys: []KeyInfo{
			{KeyID: 1, Status: KeyStatusEnabled, Type: KeyTypeLegacyAESGCM, IsPrimary: false},
			{KeyID: 2, Status: KeyStatusEnabled, Type: KeyTypeAES256GCM, IsPrimary: true},
		},
	}

	t.Run("returns true for existing key", func(t *testing.T) {
		t.Parallel()

		assert.True(t, info.HasKey(1))
		assert.True(t, info.HasKey(2))
	})

	t.Run("returns false for non-existing key", func(t *testing.T) {
		t.Parallel()

		assert.False(t, info.HasKey(3))
		assert.False(t, info.HasKey(999))
	})
}

func TestKeysetInfo_GetPrimaryKey(t *testing.T) {
	t.Parallel()

	t.Run("returns primary key when present", func(t *testing.T) {
		t.Parallel()

		info := KeysetInfo{
			PrimaryKeyID: 2,
			Keys: []KeyInfo{
				{KeyID: 1, Status: KeyStatusEnabled, Type: KeyTypeLegacyAESGCM, IsPrimary: false},
				{KeyID: 2, Status: KeyStatusEnabled, Type: KeyTypeAES256GCM, IsPrimary: true},
			},
		}

		primary := info.GetPrimaryKey()

		assert.NotNil(t, primary)
		assert.Equal(t, uint32(2), primary.KeyID)
		assert.True(t, primary.IsPrimary)
	})

	t.Run("returns nil when no primary key", func(t *testing.T) {
		t.Parallel()

		info := KeysetInfo{
			PrimaryKeyID: 1,
			Keys: []KeyInfo{
				{KeyID: 1, Status: KeyStatusEnabled, Type: KeyTypeAES256GCM, IsPrimary: false},
			},
		}

		primary := info.GetPrimaryKey()

		assert.Nil(t, primary)
	})

	t.Run("returns nil for empty keyset", func(t *testing.T) {
		t.Parallel()

		info := KeysetInfo{}

		primary := info.GetPrimaryKey()

		assert.Nil(t, primary)
	})
}

func TestKeysetInfo_HasLegacyKey(t *testing.T) {
	t.Parallel()

	t.Run("returns true when keyset has legacy AEAD key", func(t *testing.T) {
		t.Parallel()

		info := KeysetInfo{
			PrimaryKeyID: 2,
			Keys: []KeyInfo{
				{KeyID: 1, Status: KeyStatusEnabled, Type: KeyTypeLegacyAESGCM, IsPrimary: false},
				{KeyID: 2, Status: KeyStatusEnabled, Type: KeyTypeAES256GCM, IsPrimary: true},
			},
		}

		assert.True(t, info.HasLegacyKey())
	})

	t.Run("returns true when keyset has legacy MAC key", func(t *testing.T) {
		t.Parallel()

		info := KeysetInfo{
			PrimaryKeyID: 2,
			Keys: []KeyInfo{
				{KeyID: 1, Status: KeyStatusEnabled, Type: KeyTypeLegacyHMACSHA256, IsPrimary: false},
				{KeyID: 2, Status: KeyStatusEnabled, Type: KeyTypeHMACSHA256, IsPrimary: true},
			},
		}

		assert.True(t, info.HasLegacyKey())
	})

	t.Run("returns false when keyset has no legacy keys", func(t *testing.T) {
		t.Parallel()

		info := KeysetInfo{
			PrimaryKeyID: 1,
			Keys: []KeyInfo{
				{KeyID: 1, Status: KeyStatusEnabled, Type: KeyTypeAES256GCM, IsPrimary: true},
			},
		}

		assert.False(t, info.HasLegacyKey())
	})

	t.Run("returns false for empty keyset", func(t *testing.T) {
		t.Parallel()

		info := KeysetInfo{}

		assert.False(t, info.HasLegacyKey())
	})
}

func TestNewKeysetInfo(t *testing.T) {
	t.Parallel()

	keys := []KeyInfo{
		{KeyID: 1, Status: KeyStatusEnabled, Type: KeyTypeAES256GCM, IsPrimary: true},
	}

	info := NewKeysetInfo(1, keys)

	assert.Equal(t, uint32(1), info.PrimaryKeyID)
	assert.Equal(t, keys, info.Keys)
}

func TestNewWrappedKeyset(t *testing.T) {
	t.Parallel()

	info := KeysetInfo{PrimaryKeyID: 1, Keys: []KeyInfo{
		{KeyID: 1, Status: KeyStatusEnabled, Type: KeyTypeAES256GCM, IsPrimary: true},
	}}

	wrapped := NewWrappedKeyset("vault:v1:encrypted", info, true)

	assert.Equal(t, "vault:v1:encrypted", wrapped.WrappedData)
	assert.Equal(t, info, wrapped.Info)
	assert.True(t, wrapped.LegacyKeyImported)
}

func TestWrappedKeyset_Fields(t *testing.T) {
	t.Parallel()

	t.Run("stores wrapped data correctly", func(t *testing.T) {
		t.Parallel()

		wrapped := WrappedKeyset{
			WrappedData:       "vault:v1:somedata",
			Info:              KeysetInfo{PrimaryKeyID: 1},
			LegacyKeyImported: false,
		}

		assert.Equal(t, "vault:v1:somedata", wrapped.WrappedData)
		assert.Equal(t, uint32(1), wrapped.Info.PrimaryKeyID)
		assert.False(t, wrapped.LegacyKeyImported)
	})

	t.Run("legacy flag can be set", func(t *testing.T) {
		t.Parallel()

		wrapped := WrappedKeyset{
			LegacyKeyImported: true,
		}

		assert.True(t, wrapped.LegacyKeyImported)
	})
}
