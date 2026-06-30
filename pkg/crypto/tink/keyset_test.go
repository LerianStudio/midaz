// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"testing"

	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	t.Run("returns true when keyset has legacy HMAC PRF key", func(t *testing.T) {
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

func TestKeysetFactory_GenerateAEADKeyset(t *testing.T) {
	t.Parallel()

	t.Run("generates and wraps AEAD keyset", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		bundle, err := factory.GenerateAEADKeyset(context.Background(), "transit-mt", "tenant-x_org-123")

		require.NoError(t, err)
		assert.NotEmpty(t, bundle.Wrapped.WrappedData)
		assert.NotEmpty(t, bundle.RawKeyset)
		assert.NotZero(t, bundle.Wrapped.Info.PrimaryKeyID)
		assert.Len(t, bundle.Wrapped.Info.Keys, 1)
		assert.Equal(t, KeyTypeAES256GCM, bundle.Wrapped.Info.Keys[0].Type)
		assert.False(t, bundle.Wrapped.LegacyKeyImported)
		assert.Equal(t, "transit-mt", kms.gotEncryptMount)
	})

	t.Run("fails on KMS encryption error", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		kms.encryptErr = fmt.Errorf("KMS unavailable")
		factory := NewKeysetFactory(kms)

		_, err := factory.GenerateAEADKeyset(context.Background(), "transit-mt", "tenant-x_org-123")

		require.Error(t, err)
	})
}

func TestKeysetFactory_GeneratePRFKeyset(t *testing.T) {
	t.Parallel()

	t.Run("generates and wraps PRF keyset", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		bundle, err := factory.GeneratePRFKeyset(context.Background(), "transit-mt", "tenant-x_org-123")

		require.NoError(t, err)
		assert.NotEmpty(t, bundle.Wrapped.WrappedData)
		assert.NotEmpty(t, bundle.RawKeyset)
		assert.NotZero(t, bundle.Wrapped.Info.PrimaryKeyID)
		assert.Len(t, bundle.Wrapped.Info.Keys, 1)
		assert.Equal(t, KeyTypeHMACPRF, bundle.Wrapped.Info.Keys[0].Type)
		assert.False(t, bundle.Wrapped.LegacyKeyImported)
		assert.Equal(t, "transit-mt", kms.gotEncryptMount)
	})

	t.Run("fails on KMS encryption error", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		kms.encryptErr = fmt.Errorf("KMS unavailable")
		factory := NewKeysetFactory(kms)

		_, err := factory.GeneratePRFKeyset(context.Background(), "transit-mt", "tenant-x_org-123")

		require.Error(t, err)
	})
}

func TestKeysetFactory_GenerateMixedAEADKeyset(t *testing.T) {
	t.Parallel()

	t.Run("composes legacy non-primary key with fresh primary key", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		bundle, err := factory.GenerateMixedAEADKeyset(context.Background(), "transit-mt", "tenant-x_org-123", testLegacyEncryptHexKey)

		require.NoError(t, err)
		assert.NotEmpty(t, bundle.Wrapped.WrappedData)
		assert.NotEmpty(t, bundle.RawKeyset)
		assert.True(t, bundle.Wrapped.LegacyKeyImported, "mixed keyset must flag the imported legacy key")
		assert.Equal(t, "transit-mt", kms.gotEncryptMount)

		// Exactly two enabled keys: fresh primary + legacy non-primary.
		require.Len(t, bundle.Wrapped.Info.Keys, 2, "mixed AEAD keyset must hold two keys")

		primary := bundle.Wrapped.Info.GetPrimaryKey()
		require.NotNil(t, primary, "mixed keyset must have a primary key")
		assert.Equal(t, KeyTypeAES256GCM, primary.Type, "primary must be the fresh envelope key")
		assert.Equal(t, KeyStatusEnabled, primary.Status)

		// The non-primary key must be the imported legacy key, enabled.
		var legacy *KeyInfo

		for i := range bundle.Wrapped.Info.Keys {
			if !bundle.Wrapped.Info.Keys[i].IsPrimary {
				legacy = &bundle.Wrapped.Info.Keys[i]
			}
		}

		require.NotNil(t, legacy, "mixed keyset must have a non-primary legacy key")
		assert.Equal(t, KeyTypeLegacyAESGCM, legacy.Type)
		assert.Equal(t, KeyStatusEnabled, legacy.Status)
		assert.NotEqual(t, primary.KeyID, legacy.KeyID, "fresh and legacy key IDs must differ")
		assert.Equal(t, primary.KeyID, bundle.Wrapped.Info.PrimaryKeyID)
	})

	t.Run("unwrapped keyset decrypts legacy lib-commons ciphertext and round-trips fresh primary", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		bundle, err := factory.GenerateMixedAEADKeyset(context.Background(), "transit-mt", "tenant-x_org-123", testLegacyEncryptHexKey)
		require.NoError(t, err)

		primitive, err := factory.UnwrapAEAD(context.Background(), "transit-mt", "tenant-x_org-123", bundle.Wrapped)
		require.NoError(t, err)

		// (a) The composite keyset must decrypt ciphertext produced by lib-commons
		// with the legacy key (legacy key is ENABLED, used on decrypt).
		legacy := &libCrypto.Crypto{HashSecretKey: testLegacyHashKey, EncryptSecretKey: testLegacyEncryptHexKey}
		require.NoError(t, legacy.InitializeCipher())

		plaintext := "legacy-encrypted-value"
		legacyCipher, err := legacy.Encrypt(&plaintext)
		require.NoError(t, err)

		decodedLegacy, err := base64.StdEncoding.DecodeString(*legacyCipher)
		require.NoError(t, err)

		opened, err := primitive.Decrypt(decodedLegacy, nil)
		require.NoError(t, err)
		assert.Equal(t, plaintext, string(opened))

		// (b) Fresh primary round-trips through the same composite keyset.
		freshPlain := []byte("fresh-value")
		freshCipher, err := primitive.Encrypt(freshPlain, nil)
		require.NoError(t, err)

		freshOpened, err := primitive.Decrypt(freshCipher, nil)
		require.NoError(t, err)
		assert.Equal(t, freshPlain, freshOpened)
	})

	t.Run("fails closed on empty legacy key material", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		_, err := factory.GenerateMixedAEADKeyset(context.Background(), "transit-mt", "tenant-x_org-123", "")

		require.Error(t, err)
		assert.Empty(t, kms.gotEncryptMount, "must fail before reaching the KMS wrap step")
	})

	t.Run("fails closed on invalid legacy key material", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		_, err := factory.GenerateMixedAEADKeyset(context.Background(), "transit-mt", "tenant-x_org-123", "not-hex-zz")

		require.Error(t, err)
	})

	t.Run("fails on KMS encryption error", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		kms.encryptErr = fmt.Errorf("KMS unavailable")
		factory := NewKeysetFactory(kms)

		_, err := factory.GenerateMixedAEADKeyset(context.Background(), "transit-mt", "tenant-x_org-123", testLegacyEncryptHexKey)

		require.Error(t, err)
	})
}

func TestKeysetFactory_GenerateMixedPRFKeyset(t *testing.T) {
	t.Parallel()

	t.Run("composes legacy non-primary PRF key with fresh primary key", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		bundle, err := factory.GenerateMixedPRFKeyset(context.Background(), "transit-mt", "tenant-x_org-123", testLegacyHashKey)

		require.NoError(t, err)
		assert.NotEmpty(t, bundle.Wrapped.WrappedData)
		assert.NotEmpty(t, bundle.RawKeyset)
		assert.True(t, bundle.Wrapped.LegacyKeyImported)
		assert.Equal(t, "transit-mt", kms.gotEncryptMount)

		require.Len(t, bundle.Wrapped.Info.Keys, 2, "mixed PRF keyset must hold two keys")

		primary := bundle.Wrapped.Info.GetPrimaryKey()
		require.NotNil(t, primary)
		assert.Equal(t, KeyTypeHMACPRF, primary.Type, "primary must be the fresh PRF key")
		assert.Equal(t, KeyStatusEnabled, primary.Status)

		var legacy *KeyInfo

		for i := range bundle.Wrapped.Info.Keys {
			if !bundle.Wrapped.Info.Keys[i].IsPrimary {
				legacy = &bundle.Wrapped.Info.Keys[i]
			}
		}

		require.NotNil(t, legacy)
		assert.Equal(t, KeyTypeLegacyHMACSHA256, legacy.Type)
		assert.Equal(t, KeyStatusEnabled, legacy.Status)
		assert.NotEqual(t, primary.KeyID, legacy.KeyID)
	})

	t.Run("legacy key in composite produces lib-commons-compatible search candidate", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		bundle, err := factory.GenerateMixedPRFKeyset(context.Background(), "transit-mt", "tenant-x_org-123", testLegacyHashKey)
		require.NoError(t, err)

		keysetBytes, err := factory.Wrapper().UnwrapKeyset(context.Background(), "transit-mt", "tenant-x_org-123", bundle.Wrapped.WrappedData)
		require.NoError(t, err)

		handle, err := DeserializePRFKeyset(keysetBytes)
		require.NoError(t, err)

		multi, err := NewPRFMultiPrimitive(handle)
		require.NoError(t, err)

		// The multi-primitive must yield two candidates (one per enabled key).
		input := []byte("a@b.com")
		candidates, err := multi.ComputeSearchTokenCandidates(input)
		require.NoError(t, err)
		assert.Len(t, candidates, 2, "composite PRF keyset must yield one candidate per enabled key")

		// Value assertion (not just count): the composite must carry a candidate
		// produced by the IMPORTED legacy HMAC key, so legacy-indexed rows stay
		// findable. The legacy key is RAW HMAC-SHA256 over testLegacyHashKey; the
		// multi-primitive computes ComputePRF(input, 32) and base64.URLEncodes it
		// (see ComputeSearchTokenCandidates). Compute that reference token by value
		// and assert membership. A wrong HMAC key would still yield 2 candidates but
		// would NOT contain this token.
		refHMAC := hmac.New(sha256.New, []byte(testLegacyHashKey))
		_, err = refHMAC.Write(input)
		require.NoError(t, err)
		wantLegacyCandidate := base64.URLEncoding.EncodeToString(refHMAC.Sum(nil))

		assert.Contains(t, candidates, wantLegacyCandidate,
			"composite PRF keyset must yield the legacy HMAC search candidate by value, not merely by count")
	})

	t.Run("fails closed on empty legacy secret", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		_, err := factory.GenerateMixedPRFKeyset(context.Background(), "transit-mt", "tenant-x_org-123", "")

		require.Error(t, err)
	})

	t.Run("fails on KMS encryption error", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		kms.encryptErr = fmt.Errorf("KMS unavailable")
		factory := NewKeysetFactory(kms)

		_, err := factory.GenerateMixedPRFKeyset(context.Background(), "transit-mt", "tenant-x_org-123", testLegacyHashKey)

		require.Error(t, err)
	})
}

func TestKeysetFactory_UnwrapAEAD(t *testing.T) {
	t.Parallel()

	t.Run("unwraps and returns working AEAD primitive", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		// Generate keyset first
		bundle, err := factory.GenerateAEADKeyset(context.Background(), "transit-mt", "tenant-x_org-123")
		require.NoError(t, err)

		// Unwrap AEAD keyset
		primitive, err := factory.UnwrapAEAD(context.Background(), "transit-mt", "tenant-x_org-123", bundle.Wrapped)
		require.NoError(t, err)
		assert.Equal(t, "transit-mt", kms.gotDecryptMount)

		// Test that primitive works
		plaintext := []byte("test encryption")
		ciphertext, err := primitive.Encrypt(plaintext, nil)
		require.NoError(t, err)

		decrypted, err := primitive.Decrypt(ciphertext, nil)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})

	t.Run("fails on KMS decryption error", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		wrapped := WrappedKeyset{
			WrappedData: "vault:v1:invaliddata",
		}

		kms.decryptErr = fmt.Errorf("permission denied")

		_, err := factory.UnwrapAEAD(context.Background(), "transit-mt", "tenant-x_org-123", wrapped)

		require.Error(t, err)
	})
}

func TestKeysetFactory_UnwrapPRF(t *testing.T) {
	t.Parallel()

	t.Run("unwraps and returns working PRF primitive", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		// Generate keyset first
		bundle, err := factory.GeneratePRFKeyset(context.Background(), "transit-mt", "tenant-x_org-123")
		require.NoError(t, err)

		// Unwrap PRF keyset
		primitive, err := factory.UnwrapPRF(context.Background(), "transit-mt", "tenant-x_org-123", bundle.Wrapped)
		require.NoError(t, err)
		assert.Equal(t, "transit-mt", kms.gotDecryptMount)

		// Test that primitive works: deterministic search token over RAW PRF output.
		data := []byte("data to mac")
		token1, err := primitive.ComputeSearchToken(data)
		require.NoError(t, err)

		token2, err := primitive.ComputeSearchToken(data)
		require.NoError(t, err)

		assert.Equal(t, token1, token2)
	})

	t.Run("fails on KMS decryption error", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		wrapped := WrappedKeyset{
			WrappedData: "vault:v1:invaliddata",
		}

		kms.decryptErr = fmt.Errorf("permission denied")

		_, err := factory.UnwrapPRF(context.Background(), "transit-mt", "tenant-x_org-123", wrapped)

		require.Error(t, err)
	})
}

func TestKeysetFactory_Wrapper(t *testing.T) {
	t.Parallel()

	kms := newMockKMSClient()
	factory := NewKeysetFactory(kms)

	wrapper := factory.Wrapper()

	assert.NotNil(t, wrapper)
}

func TestKeysetFactory_EndToEnd(t *testing.T) {
	t.Parallel()

	t.Run("complete encryption workflow", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		// Generate AEAD keyset with caller-defined mount and key name
		mountPath := "transit-mt"
		keyName := "tenant-abc/org-123"
		bundle, err := factory.GenerateAEADKeyset(context.Background(), mountPath, keyName)
		require.NoError(t, err)

		// Unwrap and use for encryption
		aeadPrimitive, err := factory.UnwrapAEAD(context.Background(), mountPath, keyName, bundle.Wrapped)
		require.NoError(t, err)

		// Encrypt sensitive data
		sensitiveData := []byte("PII: John Doe, john@example.com")
		associatedData := []byte("context:field:email")

		ciphertext, err := aeadPrimitive.Encrypt(sensitiveData, associatedData)
		require.NoError(t, err)

		// Simulate storage and retrieval...

		// Unwrap AEAD again (simulating different request)
		aeadPrimitive2, err := factory.UnwrapAEAD(context.Background(), mountPath, keyName, bundle.Wrapped)
		require.NoError(t, err)

		// Decrypt
		decrypted, err := aeadPrimitive2.Decrypt(ciphertext, associatedData)
		require.NoError(t, err)
		assert.Equal(t, sensitiveData, decrypted)
	})

	t.Run("complete search token workflow", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		// Generate PRF keyset
		mountPath := "transit-mt"
		keyName := "tenant-abc/org-123"
		bundle, err := factory.GeneratePRFKeyset(context.Background(), mountPath, keyName)
		require.NoError(t, err)

		// Unwrap PRF for search tokens
		prfPrimitive, err := factory.UnwrapPRF(context.Background(), mountPath, keyName, bundle.Wrapped)
		require.NoError(t, err)

		// Generate search token for email
		email := []byte("john@example.com")
		token, err := prfPrimitive.ComputeSearchToken(email)
		require.NoError(t, err)

		// Simulate storage...

		// Later, search by generating same token
		searchToken, err := prfPrimitive.ComputeSearchToken(email)
		require.NoError(t, err)

		assert.Equal(t, token, searchToken)
	})
}
