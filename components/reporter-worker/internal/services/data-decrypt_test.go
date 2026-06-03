// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// encryptForTest encrypts plaintext using AES-GCM and returns base64-encoded
// ciphertext (nonce || ciphertext+tag), matching the format decryptFetcherData expects.
func encryptForTest(t *testing.T, plaintext, key []byte) []byte {
	t.Helper()

	block, err := aes.NewCipher(key)
	require.NoError(t, err)

	aesGCM, err := cipher.NewGCM(block)
	require.NoError(t, err)

	nonce := make([]byte, aesGCM.NonceSize())
	_, err = rand.Read(nonce)
	require.NoError(t, err)

	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)

	return []byte(encoded)
}

func TestDecryptFetcherData(t *testing.T) {
	t.Parallel()

	// Valid AES-256 key (32 bytes)
	validKey := make([]byte, 32)
	for i := range validKey {
		validKey[i] = byte(i)
	}

	tests := []struct {
		name          string
		encryptedData []byte
		storageKey    []byte
		expectedData  string
		expectError   bool
		errContains   string
	}{
		{
			name:          "Success - decrypts valid AES-GCM payload",
			encryptedData: encryptForTest(t, []byte(`{"accounts": [{"id": 1}]}`), validKey),
			storageKey:    validKey,
			expectedData:  `{"accounts": [{"id": 1}]}`,
			expectError:   false,
		},
		{
			name:          "Success - decrypts empty JSON object",
			encryptedData: encryptForTest(t, []byte(`{}`), validKey),
			storageKey:    validKey,
			expectedData:  `{}`,
			expectError:   false,
		},
		{
			name:          "Success - decrypts large payload",
			encryptedData: encryptForTest(t, []byte(`{"data": "some large payload with multiple fields and values for testing purposes"}`), validKey),
			storageKey:    validKey,
			expectedData:  `{"data": "some large payload with multiple fields and values for testing purposes"}`,
			expectError:   false,
		},
		{
			name:          "Error - empty encrypted data",
			encryptedData: []byte{},
			storageKey:    validKey,
			expectError:   true,
			errContains:   "encrypted data is empty",
		},
		{
			name:          "Error - nil encrypted data",
			encryptedData: nil,
			storageKey:    validKey,
			expectError:   true,
			errContains:   "encrypted data is empty",
		},
		{
			name:          "Error - empty storage key",
			encryptedData: encryptForTest(t, []byte(`{"test": true}`), validKey),
			storageKey:    []byte{},
			expectError:   true,
			errContains:   "storage decryption key not configured",
		},
		{
			name:          "Error - nil storage key",
			encryptedData: encryptForTest(t, []byte(`{"test": true}`), validKey),
			storageKey:    nil,
			expectError:   true,
			errContains:   "storage decryption key not configured",
		},
		{
			name:          "Error - invalid base64 encoding",
			encryptedData: []byte("not-valid-base64!!!"),
			storageKey:    validKey,
			expectError:   true,
			errContains:   "base64 decode encrypted data",
		},
		{
			name:          "Error - invalid key size (15 bytes)",
			encryptedData: encryptForTest(t, []byte(`{"test": true}`), validKey),
			storageKey:    []byte("fifteen-byte-k!"),
			expectError:   true,
			errContains:   "create AES cipher",
		},
		{
			name: "Error - ciphertext too short for nonce",
			// Base64 encode a very short ciphertext (less than 12 bytes for GCM nonce)
			encryptedData: []byte(base64.StdEncoding.EncodeToString([]byte("short"))),
			storageKey:    validKey,
			expectError:   true,
			errContains:   "ciphertext too short",
		},
		{
			name:          "Error - wrong key causes authentication failure",
			encryptedData: encryptForTest(t, []byte(`{"secret": "data"}`), validKey),
			storageKey: func() []byte {
				wrongKey := make([]byte, 32)
				for i := range wrongKey {
					wrongKey[i] = byte(i + 100)
				}
				return wrongKey
			}(),
			expectError: true,
			errContains: "AES-GCM decrypt",
		},
		{
			name: "Error - tampered ciphertext",
			encryptedData: func() []byte {
				encrypted := encryptForTest(t, []byte(`{"test": true}`), validKey)
				decoded, _ := base64.StdEncoding.DecodeString(string(encrypted))
				// Flip a byte in the ciphertext (after the nonce)
				if len(decoded) > 13 {
					decoded[13] ^= 0xFF
				}
				return []byte(base64.StdEncoding.EncodeToString(decoded))
			}(),
			storageKey:  validKey,
			expectError: true,
			errContains: "AES-GCM decrypt",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := decryptFetcherData(tt.encryptedData, tt.storageKey)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedData, string(result))
			}
		})
	}
}

func TestDecryptFetcherData_RoundTrip(t *testing.T) {
	t.Parallel()

	// Test with all valid AES key sizes: 128, 192, 256 bits
	keySizes := []struct {
		name string
		size int
	}{
		{name: "AES-128", size: 16},
		{name: "AES-192", size: 24},
		{name: "AES-256", size: 32},
	}

	for _, ks := range keySizes {
		ks := ks
		t.Run(ks.name, func(t *testing.T) {
			t.Parallel()

			key := make([]byte, ks.size)
			_, err := rand.Read(key)
			require.NoError(t, err)

			original := []byte(`{"org": "lerian", "accounts": [1, 2, 3]}`)
			encrypted := encryptForTest(t, original, key)

			decrypted, err := decryptFetcherData(encrypted, key)
			require.NoError(t, err)
			assert.Equal(t, original, decrypted)
		})
	}
}
