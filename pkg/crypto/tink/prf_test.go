// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"encoding/base64"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tink-crypto/tink-go/v2/keyset"
	"github.com/tink-crypto/tink-go/v2/prf"
)

func TestPRFKeysetGenerator_Generate(t *testing.T) {
	t.Parallel()

	t.Run("generates valid keyset", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()

		handle, serialized, err := generator.Generate()

		require.NoError(t, err)
		assert.NotNil(t, handle)
		assert.NotEmpty(t, serialized)
	})

	t.Run("generates unique keysets each time", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()

		_, serialized1, err1 := generator.Generate()
		require.NoError(t, err1)

		_, serialized2, err2 := generator.Generate()
		require.NoError(t, err2)

		assert.NotEqual(t, serialized1, serialized2)
	})
}

func TestPRFKeysetGenerator_ExtractInfo(t *testing.T) {
	t.Parallel()

	t.Run("labels keyset as HMAC_PRF", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()

		handle, _, err := generator.Generate()
		require.NoError(t, err)

		info, err := generator.ExtractInfo(handle)

		require.NoError(t, err)
		assert.NotZero(t, info.PrimaryKeyID)
		assert.Len(t, info.Keys, 1)
		assert.Equal(t, KeyTypeHMACPRF, info.Keys[0].Type)
		assert.Equal(t, KeyStatusEnabled, info.Keys[0].Status)
		assert.True(t, info.Keys[0].IsPrimary)
	})
}

func TestExtractKeysetInfo_TypeLabelingByPurpose(t *testing.T) {
	t.Parallel()

	t.Run("PRF keyset is labeled HMAC_PRF", func(t *testing.T) {
		t.Parallel()

		handle, _, err := NewPRFKeysetGenerator().Generate()
		require.NoError(t, err)

		info, err := extractKeysetInfo(handle, keyPurposePRF)
		require.NoError(t, err)
		require.Len(t, info.Keys, 1)
		assert.Equal(t, KeyTypeHMACPRF, info.Keys[0].Type)
	})

	t.Run("unknown purpose defaults to HMAC_SHA256 label", func(t *testing.T) {
		t.Parallel()

		handle, _, err := NewPRFKeysetGenerator().Generate()
		require.NoError(t, err)

		info, err := extractKeysetInfo(handle, keyPurpose(99))
		require.NoError(t, err)
		require.Len(t, info.Keys, 1)
		assert.Equal(t, KeyTypeHMACSHA256, info.Keys[0].Type)
	})

	t.Run("AEAD keyset still labeled AES256_GCM", func(t *testing.T) {
		t.Parallel()

		handle, _, err := NewAEADKeysetGenerator().Generate()
		require.NoError(t, err)

		info, err := extractKeysetInfo(handle, keyPurposeAEAD)
		require.NoError(t, err)
		require.Len(t, info.Keys, 1)
		assert.Equal(t, KeyTypeAES256GCM, info.Keys[0].Type)
	})
}

func TestPRFPrimitive_ComputeSearchToken(t *testing.T) {
	t.Parallel()

	t.Run("generates base64 encoded token", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewPRFPrimitive(handle)
		require.NoError(t, err)

		token, err := primitive.ComputeSearchToken([]byte("searchable value"))

		require.NoError(t, err)
		assert.NotEmpty(t, token)

		decoded, err := base64.URLEncoding.DecodeString(token)
		require.NoError(t, err)
		assert.NotEmpty(t, decoded)
	})

	t.Run("produces deterministic tokens", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewPRFPrimitive(handle)
		require.NoError(t, err)

		data := []byte("searchable@example.com")

		token1, err := primitive.ComputeSearchToken(data)
		require.NoError(t, err)

		token2, err := primitive.ComputeSearchToken(data)
		require.NoError(t, err)

		assert.Equal(t, token1, token2)
	})

	t.Run("different data produces different tokens", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewPRFPrimitive(handle)
		require.NoError(t, err)

		token1, err := primitive.ComputeSearchToken([]byte("email1@example.com"))
		require.NoError(t, err)

		token2, err := primitive.ComputeSearchToken([]byte("email2@example.com"))
		require.NoError(t, err)

		assert.NotEqual(t, token1, token2)
	})

	t.Run("token decodes to exactly 32 bytes (raw, no key-id prefix)", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewPRFPrimitive(handle)
		require.NoError(t, err)

		token, err := primitive.ComputeSearchToken([]byte("raw output check"))
		require.NoError(t, err)

		decoded, err := base64.URLEncoding.DecodeString(token)
		require.NoError(t, err)
		// RAW output: exactly the frozen PRF length, no Tink 5-byte key-id prefix.
		assert.Len(t, decoded, 32)
	})

	t.Run("different keysets produce different tokens for same input", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()

		handle1, _, err := generator.Generate()
		require.NoError(t, err)

		handle2, _, err := generator.Generate()
		require.NoError(t, err)

		primitive1, err := NewPRFPrimitive(handle1)
		require.NoError(t, err)

		primitive2, err := NewPRFPrimitive(handle2)
		require.NoError(t, err)

		data := []byte("same data")

		token1, err := primitive1.ComputeSearchToken(data)
		require.NoError(t, err)

		token2, err := primitive2.ComputeSearchToken(data)
		require.NoError(t, err)

		assert.NotEqual(t, token1, token2)
	})
}

func TestParsePRFKeyset(t *testing.T) {
	t.Parallel()

	t.Run("parses valid serialized keyset", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, serialized, err := generator.Generate()
		require.NoError(t, err)

		originalPrimitive, err := NewPRFPrimitive(handle)
		require.NoError(t, err)

		data := []byte("test data")
		token, err := originalPrimitive.ComputeSearchToken(data)
		require.NoError(t, err)

		parsedPrimitive, err := ParsePRFKeyset(serialized)
		require.NoError(t, err)

		parsedToken, err := parsedPrimitive.ComputeSearchToken(data)
		require.NoError(t, err)
		assert.Equal(t, token, parsedToken)
	})

	t.Run("fails on invalid keyset data", func(t *testing.T) {
		t.Parallel()

		_, err := ParsePRFKeyset([]byte("invalid keyset data"))

		require.Error(t, err)
	})

	t.Run("fails on empty keyset data", func(t *testing.T) {
		t.Parallel()

		_, err := ParsePRFKeyset([]byte{})

		require.Error(t, err)
	})
}

func TestNewPRFMultiPrimitive(t *testing.T) {
	t.Parallel()

	t.Run("creates working primitive from single-key keyset", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		multi, err := NewPRFMultiPrimitive(handle)

		require.NoError(t, err)
		require.NotNil(t, multi)
		assert.Len(t, multi.keyIDs, 1)
		assert.Len(t, multi.prfs, 1)
	})

	t.Run("creates primitives for all enabled keys", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		manager := keyset.NewManagerFromHandle(handle)
		_, err = manager.Add(prf.HMACSHA256PRFKeyTemplate())
		require.NoError(t, err)

		multiKeyHandle, err := manager.Handle()
		require.NoError(t, err)

		multi, err := NewPRFMultiPrimitive(multiKeyHandle)

		require.NoError(t, err)
		require.NotNil(t, multi)
		assert.Len(t, multi.keyIDs, 2)
		assert.Len(t, multi.prfs, 2)
	})

	t.Run("skips disabled keys", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		manager := keyset.NewManagerFromHandle(handle)
		secondKeyID, err := manager.Add(prf.HMACSHA256PRFKeyTemplate())
		require.NoError(t, err)

		err = manager.SetPrimary(secondKeyID)
		require.NoError(t, err)

		firstEntry, err := handle.Entry(0)
		require.NoError(t, err)
		firstKeyID := firstEntry.KeyID()

		err = manager.Disable(firstKeyID)
		require.NoError(t, err)

		multiKeyHandle, err := manager.Handle()
		require.NoError(t, err)

		multi, err := NewPRFMultiPrimitive(multiKeyHandle)

		require.NoError(t, err)
		require.NotNil(t, multi)
		assert.Len(t, multi.keyIDs, 1)
		assert.Len(t, multi.prfs, 1)
		assert.Equal(t, secondKeyID, multi.keyIDs[0])
	})

	t.Run("returns error on nil handle", func(t *testing.T) {
		t.Parallel()

		multi, err := NewPRFMultiPrimitive(nil)

		require.Error(t, err)
		assert.Nil(t, multi)
		assert.Contains(t, err.Error(), "keyset handle is nil")
	})

	t.Run("preserves enabled key order across keyset entries", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		manager := keyset.NewManagerFromHandle(handle)
		_, err = manager.Add(prf.HMACSHA256PRFKeyTemplate())
		require.NoError(t, err)
		_, err = manager.Add(prf.HMACSHA256PRFKeyTemplate())
		require.NoError(t, err)

		multiKeyHandle, err := manager.Handle()
		require.NoError(t, err)

		multi, err := NewPRFMultiPrimitive(multiKeyHandle)
		require.NoError(t, err)
		assert.Len(t, multi.keyIDs, 3)

		for i := 0; i < multiKeyHandle.Len(); i++ {
			entry, err := multiKeyHandle.Entry(i)
			require.NoError(t, err)

			if entry.KeyStatus() == keyset.Enabled {
				assert.True(t, slices.Contains(multi.keyIDs, entry.KeyID()),
					"keyID %d should be present", entry.KeyID())
			}
		}
	})

	t.Run("each enabled key produces a deterministic 32-byte PRF", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		manager := keyset.NewManagerFromHandle(handle)
		_, err = manager.Add(prf.HMACSHA256PRFKeyTemplate())
		require.NoError(t, err)

		multiKeyHandle, err := manager.Handle()
		require.NoError(t, err)

		multi, err := NewPRFMultiPrimitive(multiKeyHandle)
		require.NoError(t, err)

		data := []byte("per-key check")

		for keyID, primitive := range multi.prfs {
			out1, err := primitive.ComputePRF(data, searchTokenPRFOutputBytes)
			require.NoError(t, err, "keyID %d should compute PRF", keyID)
			assert.Len(t, out1, 32, "keyID %d output must be 32 bytes", keyID)

			out2, err := primitive.ComputePRF(data, searchTokenPRFOutputBytes)
			require.NoError(t, err)
			assert.Equal(t, out1, out2, "keyID %d PRF must be deterministic", keyID)
		}
	})
}

func TestPRFMultiPrimitive_ComputeSearchTokenCandidates(t *testing.T) {
	t.Parallel()

	t.Run("returns one token for single-key keyset", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		multi, err := NewPRFMultiPrimitive(handle)
		require.NoError(t, err)

		tokens, err := multi.ComputeSearchTokenCandidates([]byte("searchable@example.com"))

		require.NoError(t, err)
		assert.Len(t, tokens, 1)
	})

	t.Run("returns N tokens for N-key keyset", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		manager := keyset.NewManagerFromHandle(handle)
		_, err = manager.Add(prf.HMACSHA256PRFKeyTemplate())
		require.NoError(t, err)
		_, err = manager.Add(prf.HMACSHA256PRFKeyTemplate())
		require.NoError(t, err)

		multiKeyHandle, err := manager.Handle()
		require.NoError(t, err)

		multi, err := NewPRFMultiPrimitive(multiKeyHandle)
		require.NoError(t, err)

		tokens, err := multi.ComputeSearchTokenCandidates([]byte("searchable@example.com"))

		require.NoError(t, err)
		assert.Len(t, tokens, 3)
	})

	t.Run("tokens are base64 URL encoded and 32 bytes raw", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		manager := keyset.NewManagerFromHandle(handle)
		_, err = manager.Add(prf.HMACSHA256PRFKeyTemplate())
		require.NoError(t, err)

		multiKeyHandle, err := manager.Handle()
		require.NoError(t, err)

		multi, err := NewPRFMultiPrimitive(multiKeyHandle)
		require.NoError(t, err)

		tokens, err := multi.ComputeSearchTokenCandidates([]byte("searchable value"))

		require.NoError(t, err)
		for i, token := range tokens {
			decoded, err := base64.URLEncoding.DecodeString(token)
			require.NoError(t, err, "token %d should be valid base64 URL", i)
			assert.Len(t, decoded, 32, "token %d should decode to 32 raw bytes", i)
		}
	})

	t.Run("tokens are deterministic", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		multi, err := NewPRFMultiPrimitive(handle)
		require.NoError(t, err)

		data := []byte("consistent@example.com")

		tokens1, err := multi.ComputeSearchTokenCandidates(data)
		require.NoError(t, err)

		tokens2, err := multi.ComputeSearchTokenCandidates(data)
		require.NoError(t, err)

		assert.Equal(t, tokens1, tokens2)
	})

	t.Run("each key produces unique token", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		manager := keyset.NewManagerFromHandle(handle)
		_, err = manager.Add(prf.HMACSHA256PRFKeyTemplate())
		require.NoError(t, err)

		multiKeyHandle, err := manager.Handle()
		require.NoError(t, err)

		multi, err := NewPRFMultiPrimitive(multiKeyHandle)
		require.NoError(t, err)

		tokens, err := multi.ComputeSearchTokenCandidates([]byte("test data"))

		require.NoError(t, err)
		assert.Len(t, tokens, 2)
		assert.NotEqual(t, tokens[0], tokens[1])
	})

	t.Run("primary candidate matches single ComputeSearchToken", func(t *testing.T) {
		t.Parallel()

		generator := NewPRFKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		// Single-key keyset: primary is the only key, so the lone candidate
		// must equal the single-primitive ComputeSearchToken output.
		single, err := NewPRFPrimitive(handle)
		require.NoError(t, err)

		multi, err := NewPRFMultiPrimitive(handle)
		require.NoError(t, err)

		data := []byte("primary-match@example.com")

		singleToken, err := single.ComputeSearchToken(data)
		require.NoError(t, err)

		candidates, err := multi.ComputeSearchTokenCandidates(data)
		require.NoError(t, err)
		require.Len(t, candidates, 1)

		assert.Equal(t, singleToken, candidates[0])
	})
}

func TestDeserializePRFKeyset(t *testing.T) {
	t.Parallel()

	t.Run("deserializes valid keyset and builds primitives", func(t *testing.T) {
		t.Parallel()

		_, serialized, err := NewPRFKeysetGenerator().Generate()
		require.NoError(t, err)

		handle, err := DeserializePRFKeyset(serialized)
		require.NoError(t, err)
		require.NotNil(t, handle)

		// The handle must build both a single and multi PRF primitive.
		single, err := NewPRFPrimitive(handle)
		require.NoError(t, err)

		multi, err := NewPRFMultiPrimitive(handle)
		require.NoError(t, err)

		data := []byte("deserialize@example.com")

		singleToken, err := single.ComputeSearchToken(data)
		require.NoError(t, err)

		candidates, err := multi.ComputeSearchTokenCandidates(data)
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		assert.Equal(t, singleToken, candidates[0])
	})

	t.Run("fails on invalid keyset data", func(t *testing.T) {
		t.Parallel()

		_, err := DeserializePRFKeyset([]byte("not a valid keyset"))
		require.Error(t, err)
	})

	t.Run("fails on empty keyset data", func(t *testing.T) {
		t.Parallel()

		_, err := DeserializePRFKeyset([]byte{})
		require.Error(t, err)
	})
}
