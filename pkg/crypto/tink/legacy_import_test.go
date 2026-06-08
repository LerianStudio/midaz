// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"encoding/base64"
	"testing"

	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testLegacyEncryptHexKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	testLegacyHashKey       = "legacy-hash-secret"
)

func TestLegacyAESGCMPrimitiveFromHexKey_DecryptsLibCommonsCiphertext(t *testing.T) {
	t.Parallel()

	legacy := &libCrypto.Crypto{HashSecretKey: testLegacyHashKey, EncryptSecretKey: testLegacyEncryptHexKey}
	require.NoError(t, legacy.InitializeCipher())

	plaintext := "crm-sensitive-value"
	ciphertext, err := legacy.Encrypt(&plaintext)
	require.NoError(t, err)
	require.NotNil(t, ciphertext)

	primitive, err := NewLegacyAESGCMPrimitiveFromHexKey(testLegacyEncryptHexKey)
	require.NoError(t, err)

	decoded, err := base64.StdEncoding.DecodeString(*ciphertext)
	require.NoError(t, err)

	opened, err := primitive.Decrypt(decoded, nil)
	require.NoError(t, err)
	assert.Equal(t, plaintext, string(opened))
}

func TestLegacyAESGCMPrimitiveFromHexKey_EncryptsLibCommonsCompatibleCiphertext(t *testing.T) {
	t.Parallel()

	primitive, err := NewLegacyAESGCMPrimitiveFromHexKey(testLegacyEncryptHexKey)
	require.NoError(t, err)

	plaintext := "crm-sensitive-value"
	cipherBytes, err := primitive.Encrypt([]byte(plaintext), nil)
	require.NoError(t, err)
	ciphertext := base64.StdEncoding.EncodeToString(cipherBytes)

	legacy := &libCrypto.Crypto{HashSecretKey: testLegacyHashKey, EncryptSecretKey: testLegacyEncryptHexKey}
	require.NoError(t, legacy.InitializeCipher())
	opened, err := legacy.Decrypt(&ciphertext)
	require.NoError(t, err)
	require.NotNil(t, opened)
	assert.Equal(t, plaintext, *opened)
}

func TestLegacyMACPrimitiveFromSecret_ProducesLibCommonsTokenExactly(t *testing.T) {
	t.Parallel()

	legacy := &libCrypto.Crypto{HashSecretKey: testLegacyHashKey, EncryptSecretKey: testLegacyEncryptHexKey}
	normalized := "abc123"
	want := legacy.GenerateHash(&normalized)

	primitive, err := NewLegacyMACPrimitiveFromSecret(testLegacyHashKey)
	require.NoError(t, err)

	got, err := primitive.ComputeLegacyHexToken([]byte(normalized))
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
