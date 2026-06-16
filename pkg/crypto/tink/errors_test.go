// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorCategory_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		category ErrorCategory
		expected string
	}{
		{
			name:     "unknown",
			category: ErrorCategoryUnknown,
			expected: "unknown",
		},
		{
			name:     "configuration",
			category: ErrorCategoryConfiguration,
			expected: "configuration",
		},
		{
			name:     "kms",
			category: ErrorCategoryKMS,
			expected: "kms",
		},
		{
			name:     "crypto",
			category: ErrorCategoryCrypto,
			expected: "crypto",
		},
		{
			name:     "input",
			category: ErrorCategoryInput,
			expected: "input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.category.String())
		})
	}
}

func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected ErrorCategory
	}{
		{
			name:     "nil error returns unknown",
			err:      nil,
			expected: ErrorCategoryUnknown,
		},
		{
			name:     "KMS unavailable sentinel",
			err:      ErrKMSUnavailable,
			expected: ErrorCategoryKMS,
		},
		{
			name:     "vault error string",
			err:      fmt.Errorf("vault connection failed"),
			expected: ErrorCategoryKMS,
		},
		{
			name:     "transit error string",
			err:      fmt.Errorf("transit encrypt failed"),
			expected: ErrorCategoryKMS,
		},
		{
			name:     "kms error string",
			err:      fmt.Errorf("kms operation error"),
			expected: ErrorCategoryKMS,
		},
		{
			name:     "KMS uppercase error string",
			err:      fmt.Errorf("KMS unavailable"),
			expected: ErrorCategoryKMS,
		},
		{
			name:     "Vault uppercase error string",
			err:      fmt.Errorf("VAULT connection failed"),
			expected: ErrorCategoryKMS,
		},
		{
			name:     "decryption failed sentinel",
			err:      ErrDecryptionFailed,
			expected: ErrorCategoryCrypto,
		},
		{
			name:     "encryption failed sentinel",
			err:      ErrEncryptionFailed,
			expected: ErrorCategoryCrypto,
		},
		{
			name:     "keyset corrupted sentinel",
			err:      ErrKeysetCorrupted,
			expected: ErrorCategoryCrypto,
		},
		{
			name:     "decrypt error string",
			err:      fmt.Errorf("failed to decrypt data"),
			expected: ErrorCategoryCrypto,
		},
		{
			name:     "encrypt error string",
			err:      fmt.Errorf("failed to encrypt data"),
			expected: ErrorCategoryCrypto,
		},
		{
			name:     "keyset error string",
			err:      fmt.Errorf("keyset parsing failed"),
			expected: ErrorCategoryCrypto,
		},
		{
			name:     "DECRYPT uppercase error string",
			err:      fmt.Errorf("DECRYPT operation failed"),
			expected: ErrorCategoryCrypto,
		},
		{
			name:     "invalid key name sentinel",
			err:      ErrInvalidKeyName,
			expected: ErrorCategoryInput,
		},
		{
			name:     "invalid input string",
			err:      fmt.Errorf("invalid organization id"),
			expected: ErrorCategoryInput,
		},
		{
			name:     "empty input string",
			err:      fmt.Errorf("empty data"),
			expected: ErrorCategoryInput,
		},
		{
			name:     "config error string",
			err:      fmt.Errorf("config validation failed"),
			expected: ErrorCategoryConfiguration,
		},
		{
			name:     "missing config string",
			err:      fmt.Errorf("missing required field"),
			expected: ErrorCategoryConfiguration,
		},
		{
			name:     "generic error returns unknown",
			err:      fmt.Errorf("something went wrong"),
			expected: ErrorCategoryUnknown,
		},
		{
			name:     "wrapped KMS error",
			err:      fmt.Errorf("operation failed: %w", ErrKMSUnavailable),
			expected: ErrorCategoryKMS,
		},
		{
			name:     "wrapped crypto error",
			err:      fmt.Errorf("operation failed: %w", ErrDecryptionFailed),
			expected: ErrorCategoryCrypto,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := ClassifyError(tt.err)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSafeToLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error is safe",
			err:      nil,
			expected: true,
		},
		{
			name:     "generic error is safe",
			err:      fmt.Errorf("operation failed"),
			expected: true,
		},
		{
			name:     "keyset not found is safe",
			err:      ErrKeysetNotFound,
			expected: true,
		},
		{
			name:     "error mentioning key material is not safe",
			err:      fmt.Errorf("key material leaked"),
			expected: false,
		},
		{
			name:     "error mentioning secret is not safe",
			err:      fmt.Errorf("secret value: xyz"),
			expected: false,
		},
		{
			name:     "error mentioning token is not safe",
			err:      fmt.Errorf("token: abc123"),
			expected: false,
		},
		{
			name:     "error mentioning password is not safe",
			err:      fmt.Errorf("password: pass123"),
			expected: false,
		},
		{
			name:     "error mentioning credential is not safe",
			err:      fmt.Errorf("credential error: abc"),
			expected: false,
		},
		{
			name:     "error mentioning plaintext is not safe",
			err:      fmt.Errorf("plaintext: hello"),
			expected: false,
		},
		{
			name:     "case insensitive check for SECRET",
			err:      fmt.Errorf("SECRET value exposed"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := IsSafeToLog(tt.err)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error returns empty string",
			err:      nil,
			expected: "",
		},
		{
			name:     "KMS error sanitized",
			err:      fmt.Errorf("vault: permission denied for path transit/encrypt/org/123"),
			expected: "key management service error",
		},
		{
			name:     "crypto error sanitized",
			err:      fmt.Errorf("decrypt: authentication tag mismatch"),
			expected: "cryptographic operation failed",
		},
		{
			name:     "input error sanitized",
			err:      ErrInvalidKeyName,
			expected: "invalid input",
		},
		{
			name:     "config error sanitized",
			err:      fmt.Errorf("config missing required field: addr"),
			expected: "configuration error",
		},
		{
			name:     "unknown error sanitized",
			err:      fmt.Errorf("unexpected error occurred"),
			expected: "internal error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := SanitizeError(tt.err)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSentinelErrors(t *testing.T) {
	t.Parallel()

	t.Run("errors are distinct", func(t *testing.T) {
		t.Parallel()

		sentinels := []error{
			ErrKeysetNotFound,
			ErrKeysetCorrupted,
			ErrDecryptionFailed,
			ErrEncryptionFailed,
			ErrKMSUnavailable,
			ErrInvalidKeyName,
		}

		for i, err1 := range sentinels {
			for j, err2 := range sentinels {
				if i != j {
					assert.False(t, errors.Is(err1, err2), "%v should not be %v", err1, err2)
				}
			}
		}
	})

	t.Run("errors can be wrapped and unwrapped", func(t *testing.T) {
		t.Parallel()

		wrapped := fmt.Errorf("operation failed: %w", ErrKeysetNotFound)

		assert.True(t, errors.Is(wrapped, ErrKeysetNotFound))
		assert.False(t, errors.Is(wrapped, ErrKMSUnavailable))
	})
}
