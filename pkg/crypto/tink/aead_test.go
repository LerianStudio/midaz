// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAEADKeysetGenerator_Generate(t *testing.T) {
	t.Parallel()

	t.Run("generates valid keyset", func(t *testing.T) {
		t.Parallel()

		generator := NewAEADKeysetGenerator()

		handle, serialized, err := generator.Generate()

		require.NoError(t, err)
		assert.NotNil(t, handle)
		assert.NotEmpty(t, serialized)
	})

	t.Run("generates unique keysets each time", func(t *testing.T) {
		t.Parallel()

		generator := NewAEADKeysetGenerator()

		_, serialized1, err1 := generator.Generate()
		require.NoError(t, err1)

		_, serialized2, err2 := generator.Generate()
		require.NoError(t, err2)

		// Keys should be different (cryptographically random)
		assert.NotEqual(t, serialized1, serialized2)
	})
}

func TestAEADKeysetGenerator_ExtractInfo(t *testing.T) {
	t.Parallel()

	t.Run("extracts info from generated keyset", func(t *testing.T) {
		t.Parallel()

		generator := NewAEADKeysetGenerator()

		handle, _, err := generator.Generate()
		require.NoError(t, err)

		info, err := generator.ExtractInfo(handle)

		require.NoError(t, err)
		assert.NotZero(t, info.PrimaryKeyID)
		assert.Len(t, info.Keys, 1)
		assert.Equal(t, KeyTypeAES256GCM, info.Keys[0].Type)
		assert.Equal(t, KeyStatusEnabled, info.Keys[0].Status)
		assert.True(t, info.Keys[0].IsPrimary)
	})
}

func TestAEADPrimitive_EncryptDecrypt(t *testing.T) {
	t.Parallel()

	t.Run("encrypt and decrypt round trip", func(t *testing.T) {
		t.Parallel()

		generator := NewAEADKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewAEADPrimitive(handle)
		require.NoError(t, err)

		plaintext := []byte("sensitive data for encryption")
		associatedData := []byte("context information")

		// Encrypt
		ciphertext, err := primitive.Encrypt(plaintext, associatedData)
		require.NoError(t, err)
		assert.NotEmpty(t, ciphertext)
		assert.NotEqual(t, plaintext, ciphertext)

		// Decrypt
		decrypted, err := primitive.Decrypt(ciphertext, associatedData)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})

	t.Run("decrypt fails with wrong associated data", func(t *testing.T) {
		t.Parallel()

		generator := NewAEADKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewAEADPrimitive(handle)
		require.NoError(t, err)

		plaintext := []byte("sensitive data")
		correctAD := []byte("correct context")
		wrongAD := []byte("wrong context")

		ciphertext, err := primitive.Encrypt(plaintext, correctAD)
		require.NoError(t, err)

		_, err = primitive.Decrypt(ciphertext, wrongAD)
		require.Error(t, err)
	})

	t.Run("decrypt fails with tampered ciphertext", func(t *testing.T) {
		t.Parallel()

		generator := NewAEADKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewAEADPrimitive(handle)
		require.NoError(t, err)

		plaintext := []byte("sensitive data")
		ad := []byte("context")

		ciphertext, err := primitive.Encrypt(plaintext, ad)
		require.NoError(t, err)

		// Tamper with ciphertext
		tamperedCiphertext := make([]byte, len(ciphertext))
		copy(tamperedCiphertext, ciphertext)
		tamperedCiphertext[len(tamperedCiphertext)-1] ^= 0xFF

		_, err = primitive.Decrypt(tamperedCiphertext, ad)
		require.Error(t, err)
	})

	t.Run("encrypts empty plaintext", func(t *testing.T) {
		t.Parallel()

		generator := NewAEADKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewAEADPrimitive(handle)
		require.NoError(t, err)

		ciphertext, err := primitive.Encrypt([]byte{}, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, ciphertext)

		decrypted, err := primitive.Decrypt(ciphertext, nil)
		require.NoError(t, err)
		assert.Empty(t, decrypted)
	})

	t.Run("encrypts with nil associated data", func(t *testing.T) {
		t.Parallel()

		generator := NewAEADKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewAEADPrimitive(handle)
		require.NoError(t, err)

		plaintext := []byte("data without context")

		ciphertext, err := primitive.Encrypt(plaintext, nil)
		require.NoError(t, err)

		decrypted, err := primitive.Decrypt(ciphertext, nil)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})
}

func TestParseAEADKeyset(t *testing.T) {
	t.Parallel()

	t.Run("parses valid serialized keyset", func(t *testing.T) {
		t.Parallel()

		generator := NewAEADKeysetGenerator()
		handle, serialized, err := generator.Generate()
		require.NoError(t, err)

		// Use original handle to encrypt
		originalPrimitive, err := NewAEADPrimitive(handle)
		require.NoError(t, err)

		plaintext := []byte("test data")
		ciphertext, err := originalPrimitive.Encrypt(plaintext, nil)
		require.NoError(t, err)

		// Parse serialized keyset
		parsedPrimitive, err := ParseAEADKeyset(serialized)
		require.NoError(t, err)

		// Parsed primitive should be able to decrypt
		decrypted, err := parsedPrimitive.Decrypt(ciphertext, nil)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})

	t.Run("fails on invalid keyset data", func(t *testing.T) {
		t.Parallel()

		_, err := ParseAEADKeyset([]byte("invalid keyset data"))

		require.Error(t, err)
	})

	t.Run("fails on empty keyset data", func(t *testing.T) {
		t.Parallel()

		_, err := ParseAEADKeyset([]byte{})

		require.Error(t, err)
	})
}

func TestSerializeDeserialize_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("keyset survives serialization round trip", func(t *testing.T) {
		t.Parallel()

		generator := NewAEADKeysetGenerator()
		originalHandle, serialized, err := generator.Generate()
		require.NoError(t, err)

		// Deserialize
		deserializedHandle, err := deserializeKeyset(serialized)
		require.NoError(t, err)

		// Both should produce working primitives
		originalPrimitive, err := NewAEADPrimitive(originalHandle)
		require.NoError(t, err)

		deserializedPrimitive, err := NewAEADPrimitive(deserializedHandle)
		require.NoError(t, err)

		plaintext := []byte("round trip test")

		// Encrypt with original, decrypt with deserialized
		ciphertext, err := originalPrimitive.Encrypt(plaintext, nil)
		require.NoError(t, err)

		decrypted, err := deserializedPrimitive.Decrypt(ciphertext, nil)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})
}
