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
