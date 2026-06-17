// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
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

// TestLegacyPRFPrimitiveFromSecret_ByteEqualsLibCommonsAndLegacyHMAC is the HARD GATE
// for T-2.1.4: the PRF-backed legacy primitive MUST produce, for the same secret
// and value, a hex token byte-identical to BOTH (a) lib-commons GenerateHash and
// (b) the prior HMAC-SHA256(secret, value) token construction. That legacy
// construction is asserted against a reference HMAC computed inline with
// crypto/hmac. Covers empty string and unicode.
func TestLegacyPRFPrimitiveFromSecret_ByteEqualsLibCommonsAndLegacyHMAC(t *testing.T) {
	t.Parallel()

	legacy := &libCrypto.Crypto{HashSecretKey: testLegacyHashKey, EncryptSecretKey: testLegacyEncryptHexKey}

	prfPrimitive, err := NewLegacyPRFPrimitiveFromSecret(testLegacyHashKey)
	require.NoError(t, err)

	values := []string{
		"abc123",
		"",
		"José da Silva — 日本語 🔐",
		"a@b.com",
	}

	for _, value := range values {
		value := value

		t.Run("value="+value, func(t *testing.T) {
			t.Parallel()

			wantLibCommons := legacy.GenerateHash(&value)

			// (b) reference: raw HMAC-SHA256(secret, value) hex, matching the
			// removed legacy HMAC construction (RAW HmacKey, SHA256, TagSize 32).
			refHMAC := hmac.New(sha256.New, []byte(testLegacyHashKey))
			_, err := refHMAC.Write([]byte(value))
			require.NoError(t, err)
			wantHMAC := hex.EncodeToString(refHMAC.Sum(nil))

			gotPRF, err := prfPrimitive.ComputeLegacyHexToken([]byte(value))
			require.NoError(t, err)

			// (a) PRF == lib-commons GenerateHash (the abort gate)
			assert.Equal(t, wantLibCommons, gotPRF, "PRF token must byte-match lib-commons GenerateHash")
			// (b) PRF == prior legacy HMAC construction
			assert.Equal(t, wantHMAC, gotPRF, "PRF token must byte-match prior HMAC-SHA256 token output")
			// 64-char lowercase hex (full HMAC-SHA256 tag)
			assert.Len(t, gotPRF, 64, "legacy hex token must be 64 chars (32-byte HMAC-SHA256)")
		})
	}
}

func TestLegacyPRFPrimitiveFromSecret_EmptySecretRejected(t *testing.T) {
	t.Parallel()

	_, err := NewLegacyPRFPrimitiveFromSecret("")
	require.Error(t, err)
}

func TestLegacyPRFPrimitive_NilGuard(t *testing.T) {
	t.Parallel()

	var primitive *LegacyPRFPrimitive

	_, err := primitive.ComputeLegacyHexToken([]byte("x"))
	require.Error(t, err)
}
