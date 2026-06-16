// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockKMSClient implements KMSClient for testing.
// It records the mountPath received on the most recent Encrypt/Decrypt call
// so tests can assert that the mountPath is forwarded verbatim.
type mockKMSClient struct {
	encryptErr error
	decryptErr error

	gotEncryptMount string
	gotDecryptMount string
}

func newMockKMSClient() *mockKMSClient {
	return &mockKMSClient{}
}

func (m *mockKMSClient) Encrypt(_ context.Context, mountPath, _ string, plaintext []byte) (string, error) {
	m.gotEncryptMount = mountPath

	if m.encryptErr != nil {
		return "", m.encryptErr
	}

	// Simulate Vault Transit encryption format
	encoded := base64.StdEncoding.EncodeToString(plaintext)
	ciphertext := fmt.Sprintf("vault:v1:%s", encoded)

	return ciphertext, nil
}

func (m *mockKMSClient) Decrypt(_ context.Context, mountPath, _ string, ciphertext string) ([]byte, error) {
	m.gotDecryptMount = mountPath

	if m.decryptErr != nil {
		return nil, m.decryptErr
	}

	// Extract base64 from "vault:v1:base64data"
	if len(ciphertext) < 10 {
		return nil, fmt.Errorf("invalid ciphertext format")
	}

	encoded := ciphertext[9:] // Skip "vault:v1:"
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	return decoded, nil
}

func TestKeysetWrapper_WrapKeyset(t *testing.T) {
	t.Parallel()

	t.Run("wraps keyset successfully", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		wrapper := NewKeysetWrapper(kms)

		keyset := []byte("serialized keyset data")

		wrapped, err := wrapper.WrapKeyset(context.Background(), "transit/tenant-x", "my-key", keyset)

		require.NoError(t, err)
		assert.Contains(t, wrapped, "vault:v1:")
	})

	t.Run("forwards mountPath verbatim to KMS", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		wrapper := NewKeysetWrapper(kms)

		_, err := wrapper.WrapKeyset(context.Background(), "transit/tenant-x", "my-key", []byte("keyset"))

		require.NoError(t, err)
		assert.Equal(t, "transit/tenant-x", kms.gotEncryptMount)
	})

	t.Run("fails on empty keyset", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		wrapper := NewKeysetWrapper(kms)

		_, err := wrapper.WrapKeyset(context.Background(), "transit/tenant-x", "my-key", []byte{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty keyset")
	})

	t.Run("fails on KMS error", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		kms.encryptErr = fmt.Errorf("KMS unavailable")
		wrapper := NewKeysetWrapper(kms)

		_, err := wrapper.WrapKeyset(context.Background(), "transit/tenant-x", "my-key", []byte("keyset"))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "KMS")
	})
}

func TestKeysetWrapper_UnwrapKeyset(t *testing.T) {
	t.Parallel()

	t.Run("unwraps keyset successfully", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		wrapper := NewKeysetWrapper(kms)

		originalKeyset := []byte("original keyset data")

		// Wrap first
		wrapped, err := wrapper.WrapKeyset(context.Background(), "transit/tenant-x", "my-key", originalKeyset)
		require.NoError(t, err)

		// Unwrap
		unwrapped, err := wrapper.UnwrapKeyset(context.Background(), "transit/tenant-x", "my-key", wrapped)

		require.NoError(t, err)
		assert.Equal(t, originalKeyset, unwrapped)
	})

	t.Run("forwards mountPath verbatim to KMS", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		wrapper := NewKeysetWrapper(kms)

		_, err := wrapper.UnwrapKeyset(context.Background(), "transit/tenant-x", "my-key", "vault:v1:c29tZWRhdGE=")

		require.NoError(t, err)
		assert.Equal(t, "transit/tenant-x", kms.gotDecryptMount)
	})

	t.Run("fails on empty ciphertext", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		wrapper := NewKeysetWrapper(kms)

		_, err := wrapper.UnwrapKeyset(context.Background(), "transit/tenant-x", "my-key", "")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty ciphertext")
	})

	t.Run("fails on KMS error", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		kms.decryptErr = fmt.Errorf("permission denied")
		wrapper := NewKeysetWrapper(kms)

		_, err := wrapper.UnwrapKeyset(context.Background(), "transit/tenant-x", "my-key", "vault:v1:somedata")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "KMS")
	})
}

func TestKeysetFactory_GenerateAEADKeyset(t *testing.T) {
	t.Parallel()

	t.Run("generates and wraps AEAD keyset", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		bundle, err := factory.GenerateAEADKeyset(context.Background(), "transit/tenant-x", "test-key")

		require.NoError(t, err)
		assert.NotEmpty(t, bundle.Wrapped.WrappedData)
		assert.NotEmpty(t, bundle.RawKeyset)
		assert.NotZero(t, bundle.Wrapped.Info.PrimaryKeyID)
		assert.Len(t, bundle.Wrapped.Info.Keys, 1)
		assert.Equal(t, KeyTypeAES256GCM, bundle.Wrapped.Info.Keys[0].Type)
		assert.False(t, bundle.Wrapped.LegacyKeyImported)
		assert.Equal(t, "transit/tenant-x", kms.gotEncryptMount)
	})

	t.Run("fails on KMS encryption error", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		kms.encryptErr = fmt.Errorf("KMS unavailable")
		factory := NewKeysetFactory(kms)

		_, err := factory.GenerateAEADKeyset(context.Background(), "transit/tenant-x", "test-key")

		require.Error(t, err)
	})
}

func TestKeysetFactory_GeneratePRFKeyset(t *testing.T) {
	t.Parallel()

	t.Run("generates and wraps PRF keyset", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		factory := NewKeysetFactory(kms)

		bundle, err := factory.GeneratePRFKeyset(context.Background(), "transit/tenant-x", "test-key")

		require.NoError(t, err)
		assert.NotEmpty(t, bundle.Wrapped.WrappedData)
		assert.NotEmpty(t, bundle.RawKeyset)
		assert.NotZero(t, bundle.Wrapped.Info.PrimaryKeyID)
		assert.Len(t, bundle.Wrapped.Info.Keys, 1)
		assert.Equal(t, KeyTypeHMACPRF, bundle.Wrapped.Info.Keys[0].Type)
		assert.False(t, bundle.Wrapped.LegacyKeyImported)
		assert.Equal(t, "transit/tenant-x", kms.gotEncryptMount)
	})

	t.Run("fails on KMS encryption error", func(t *testing.T) {
		t.Parallel()

		kms := newMockKMSClient()
		kms.encryptErr = fmt.Errorf("KMS unavailable")
		factory := NewKeysetFactory(kms)

		_, err := factory.GeneratePRFKeyset(context.Background(), "transit/tenant-x", "test-key")

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
		bundle, err := factory.GenerateAEADKeyset(context.Background(), "transit/tenant-x", "test-key")
		require.NoError(t, err)

		// Unwrap AEAD keyset
		primitive, err := factory.UnwrapAEAD(context.Background(), "transit/tenant-x", "test-key", bundle.Wrapped)
		require.NoError(t, err)
		assert.Equal(t, "transit/tenant-x", kms.gotDecryptMount)

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

		_, err := factory.UnwrapAEAD(context.Background(), "transit/tenant-x", "test-key", wrapped)

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
		bundle, err := factory.GeneratePRFKeyset(context.Background(), "transit/tenant-x", "test-key")
		require.NoError(t, err)

		// Unwrap PRF keyset
		primitive, err := factory.UnwrapPRF(context.Background(), "transit/tenant-x", "test-key", bundle.Wrapped)
		require.NoError(t, err)
		assert.Equal(t, "transit/tenant-x", kms.gotDecryptMount)

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

		_, err := factory.UnwrapPRF(context.Background(), "transit/tenant-x", "test-key", wrapped)

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
		mountPath := "transit/tenant-abc"
		keyName := "tenant/abc/entity/123"
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
		mountPath := "transit/tenant-abc"
		keyName := "tenant/abc/entity/123"
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
