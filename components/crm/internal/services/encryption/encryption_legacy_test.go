// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.
package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"

	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	legacyEncryptHexKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	legacyHashKey       = "legacy-hash-secret"
)

func TestLegacyCrypto_Characterization_LibCommonsCiphertextFormat(t *testing.T) {
	t.Parallel()

	legacy := &libCrypto.Crypto{HashSecretKey: legacyHashKey, EncryptSecretKey: legacyEncryptHexKey}
	require.NoError(t, legacy.InitializeCipher())

	plaintext := "crm-sensitive-value"
	ciphertext, err := legacy.Encrypt(&plaintext)
	require.NoError(t, err)
	require.NotNil(t, ciphertext)

	decoded, err := base64.StdEncoding.DecodeString(*ciphertext)
	require.NoError(t, err)
	require.Greater(t, len(decoded), 12)

	key, err := hex.DecodeString(legacyEncryptHexKey)
	require.NoError(t, err)
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	aead, err := cipher.NewGCM(block)
	require.NoError(t, err)

	nonceSize := aead.NonceSize()
	assert.Equal(t, 12, nonceSize)
	nonce := decoded[:nonceSize]
	body := decoded[nonceSize:]

	opened, err := aead.Open(nil, nonce, body, nil)
	require.NoError(t, err)
	assert.Equal(t, plaintext, string(opened))
}

func TestLegacyCrypto_Characterization_LibCommonsSearchTokenFormat(t *testing.T) {
	t.Parallel()

	legacy := &libCrypto.Crypto{HashSecretKey: legacyHashKey, EncryptSecretKey: legacyEncryptHexKey}
	normalized := "abc123"

	wantMAC := hmac.New(sha256.New, []byte(legacyHashKey))
	_, err := wantMAC.Write([]byte(normalized))
	require.NoError(t, err)
	want := hex.EncodeToString(wantMAC.Sum(nil))

	assert.Equal(t, want, legacy.GenerateHash(&normalized))
}

// Task 4: Tests for LegacyKeyMaterial - Tink-backed legacy key import

func TestNewLegacyKeyMaterial_PreservesLibCommonsCompatibility(t *testing.T) {
	t.Parallel()

	legacy := &libCrypto.Crypto{HashSecretKey: legacyHashKey, EncryptSecretKey: legacyEncryptHexKey}
	require.NoError(t, legacy.InitializeCipher())

	material, err := NewLegacyKeyMaterial(legacyEncryptHexKey, legacyHashKey)
	require.NoError(t, err)

	plaintext := "crm-sensitive-value"
	legacyCiphertext, err := legacy.Encrypt(&plaintext)
	require.NoError(t, err)

	// Tink-backed legacy material can decrypt lib-commons ciphertext
	cipherBytes, err := base64.StdEncoding.DecodeString(*legacyCiphertext)
	require.NoError(t, err)
	decryptedBytes, err := material.aead.Decrypt(cipherBytes, nil)
	require.NoError(t, err)
	assert.Equal(t, plaintext, string(decryptedBytes))

	// Tink-backed legacy material produces lib-commons compatible ciphertext
	newCipherBytes, err := material.aead.Encrypt([]byte(plaintext), nil)
	require.NoError(t, err)
	newCiphertext := base64.StdEncoding.EncodeToString(newCipherBytes)
	opened, err := legacy.Decrypt(&newCiphertext)
	require.NoError(t, err)
	require.NotNil(t, opened)
	assert.Equal(t, plaintext, *opened)

	// Tink-backed legacy material produces exact same search token
	assert.Equal(t, legacy.GenerateHash(&plaintext), material.legacySearchToken(plaintext))
}

func TestNewLegacyKeyMaterial_RejectsInvalidKeyMaterial(t *testing.T) {
	t.Parallel()

	// Invalid hex key
	_, err := NewLegacyKeyMaterial("not-hex", legacyHashKey)
	require.Error(t, err)

	// Empty hash key
	_, err = NewLegacyKeyMaterial(legacyEncryptHexKey, "")
	require.Error(t, err)
}
