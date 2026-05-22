// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMACKeysetGenerator_Generate(t *testing.T) {
	t.Parallel()

	t.Run("generates valid keyset", func(t *testing.T) {
		t.Parallel()

		generator := NewMACKeysetGenerator()

		handle, serialized, err := generator.Generate()

		require.NoError(t, err)
		assert.NotNil(t, handle)
		assert.NotEmpty(t, serialized)
	})

	t.Run("generates unique keysets each time", func(t *testing.T) {
		t.Parallel()

		generator := NewMACKeysetGenerator()

		_, serialized1, err1 := generator.Generate()
		require.NoError(t, err1)

		_, serialized2, err2 := generator.Generate()
		require.NoError(t, err2)

		// Keys should be different (cryptographically random)
		assert.NotEqual(t, serialized1, serialized2)
	})
}

func TestMACKeysetGenerator_ExtractInfo(t *testing.T) {
	t.Parallel()

	t.Run("extracts info from generated keyset", func(t *testing.T) {
		t.Parallel()

		generator := NewMACKeysetGenerator()

		handle, _, err := generator.Generate()
		require.NoError(t, err)

		info, err := generator.ExtractInfo(handle)

		require.NoError(t, err)
		assert.NotZero(t, info.PrimaryKeyID)
		assert.Len(t, info.Keys, 1)
		assert.Equal(t, KeyTypeHMACSHA256, info.Keys[0].Type)
		assert.Equal(t, KeyStatusEnabled, info.Keys[0].Status)
		assert.True(t, info.Keys[0].IsPrimary)
	})
}

func TestMACPrimitive_ComputeMAC(t *testing.T) {
	t.Parallel()

	t.Run("computes MAC for data", func(t *testing.T) {
		t.Parallel()

		generator := NewMACKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewMACPrimitive(handle)
		require.NoError(t, err)

		data := []byte("data to authenticate")

		tag, err := primitive.ComputeMAC(data)

		require.NoError(t, err)
		assert.NotEmpty(t, tag)
	})

	t.Run("produces deterministic MAC", func(t *testing.T) {
		t.Parallel()

		generator := NewMACKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewMACPrimitive(handle)
		require.NoError(t, err)

		data := []byte("consistent data")

		tag1, err := primitive.ComputeMAC(data)
		require.NoError(t, err)

		tag2, err := primitive.ComputeMAC(data)
		require.NoError(t, err)

		assert.Equal(t, tag1, tag2)
	})

	t.Run("different data produces different MAC", func(t *testing.T) {
		t.Parallel()

		generator := NewMACKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewMACPrimitive(handle)
		require.NoError(t, err)

		tag1, err := primitive.ComputeMAC([]byte("data one"))
		require.NoError(t, err)

		tag2, err := primitive.ComputeMAC([]byte("data two"))
		require.NoError(t, err)

		assert.NotEqual(t, tag1, tag2)
	})

	t.Run("computes MAC for empty data", func(t *testing.T) {
		t.Parallel()

		generator := NewMACKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewMACPrimitive(handle)
		require.NoError(t, err)

		tag, err := primitive.ComputeMAC([]byte{})

		require.NoError(t, err)
		assert.NotEmpty(t, tag)
	})
}

func TestMACPrimitive_VerifyMAC(t *testing.T) {
	t.Parallel()

	t.Run("verifies valid MAC", func(t *testing.T) {
		t.Parallel()

		generator := NewMACKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewMACPrimitive(handle)
		require.NoError(t, err)

		data := []byte("data to verify")
		tag, err := primitive.ComputeMAC(data)
		require.NoError(t, err)

		err = primitive.VerifyMAC(tag, data)

		require.NoError(t, err)
	})

	t.Run("fails on invalid MAC", func(t *testing.T) {
		t.Parallel()

		generator := NewMACKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewMACPrimitive(handle)
		require.NoError(t, err)

		data := []byte("data to verify")
		invalidTag := []byte("not a valid tag")

		err = primitive.VerifyMAC(invalidTag, data)

		require.Error(t, err)
	})

	t.Run("fails when data is tampered", func(t *testing.T) {
		t.Parallel()

		generator := NewMACKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewMACPrimitive(handle)
		require.NoError(t, err)

		originalData := []byte("original data")
		tag, err := primitive.ComputeMAC(originalData)
		require.NoError(t, err)

		tamperedData := []byte("tampered data")

		err = primitive.VerifyMAC(tag, tamperedData)

		require.Error(t, err)
	})
}

func TestMACPrimitive_ComputeSearchToken(t *testing.T) {
	t.Parallel()

	t.Run("generates base64 encoded token", func(t *testing.T) {
		t.Parallel()

		generator := NewMACKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewMACPrimitive(handle)
		require.NoError(t, err)

		data := []byte("searchable value")

		token, err := primitive.ComputeSearchToken(data)

		require.NoError(t, err)
		assert.NotEmpty(t, token)

		// Should be valid base64
		decoded, err := base64.URLEncoding.DecodeString(token)
		require.NoError(t, err)
		assert.NotEmpty(t, decoded)
	})

	t.Run("produces deterministic tokens", func(t *testing.T) {
		t.Parallel()

		generator := NewMACKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewMACPrimitive(handle)
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

		generator := NewMACKeysetGenerator()
		handle, _, err := generator.Generate()
		require.NoError(t, err)

		primitive, err := NewMACPrimitive(handle)
		require.NoError(t, err)

		token1, err := primitive.ComputeSearchToken([]byte("email1@example.com"))
		require.NoError(t, err)

		token2, err := primitive.ComputeSearchToken([]byte("email2@example.com"))
		require.NoError(t, err)

		assert.NotEqual(t, token1, token2)
	})
}

func TestParseMACKeyset(t *testing.T) {
	t.Parallel()

	t.Run("parses valid serialized keyset", func(t *testing.T) {
		t.Parallel()

		generator := NewMACKeysetGenerator()
		handle, serialized, err := generator.Generate()
		require.NoError(t, err)

		// Use original handle to compute MAC
		originalPrimitive, err := NewMACPrimitive(handle)
		require.NoError(t, err)

		data := []byte("test data")
		tag, err := originalPrimitive.ComputeMAC(data)
		require.NoError(t, err)

		// Parse serialized keyset
		parsedPrimitive, err := ParseMACKeyset(serialized)
		require.NoError(t, err)

		// Parsed primitive should verify the same MAC
		err = parsedPrimitive.VerifyMAC(tag, data)
		require.NoError(t, err)

		// And compute identical MACs
		parsedTag, err := parsedPrimitive.ComputeMAC(data)
		require.NoError(t, err)
		assert.Equal(t, tag, parsedTag)
	})

	t.Run("fails on invalid keyset data", func(t *testing.T) {
		t.Parallel()

		_, err := ParseMACKeyset([]byte("invalid keyset data"))

		require.Error(t, err)
	})

	t.Run("fails on empty keyset data", func(t *testing.T) {
		t.Parallel()

		_, err := ParseMACKeyset([]byte{})

		require.Error(t, err)
	})
}

func TestMACKeyset_CrossKeyVerification(t *testing.T) {
	t.Parallel()

	t.Run("different keys produce different MACs", func(t *testing.T) {
		t.Parallel()

		generator := NewMACKeysetGenerator()

		handle1, _, err := generator.Generate()
		require.NoError(t, err)

		handle2, _, err := generator.Generate()
		require.NoError(t, err)

		primitive1, err := NewMACPrimitive(handle1)
		require.NoError(t, err)

		primitive2, err := NewMACPrimitive(handle2)
		require.NoError(t, err)

		data := []byte("same data")

		tag1, err := primitive1.ComputeMAC(data)
		require.NoError(t, err)

		tag2, err := primitive2.ComputeMAC(data)
		require.NoError(t, err)

		// Different keys should produce different tags
		assert.NotEqual(t, tag1, tag2)
	})

	t.Run("MAC from one key cannot be verified by another", func(t *testing.T) {
		t.Parallel()

		generator := NewMACKeysetGenerator()

		handle1, _, err := generator.Generate()
		require.NoError(t, err)

		handle2, _, err := generator.Generate()
		require.NoError(t, err)

		primitive1, err := NewMACPrimitive(handle1)
		require.NoError(t, err)

		primitive2, err := NewMACPrimitive(handle2)
		require.NoError(t, err)

		data := []byte("test data")

		// Compute with key 1
		tag, err := primitive1.ComputeMAC(data)
		require.NoError(t, err)

		// Try to verify with key 2
		err = primitive2.VerifyMAC(tag, data)
		require.Error(t, err)
	})
}
