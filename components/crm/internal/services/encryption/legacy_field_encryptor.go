// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"

	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
)

// legacyFieldEncryptor implements FieldEncryptor using the legacy libCrypto.Crypto.
// It provides backward compatibility for legacy encryption mode where envelope
// encryption is not available. The legacy encryptor ignores the FieldContext/
// SearchTokenContext parameters since legacy encryption does not use AAD binding.
//
// Deprecated: This type will be removed in T-008. Use NewFieldEncryptorAdapter
// with an EncryptionService that has Tink-backed legacy keys instead.
type legacyFieldEncryptor struct {
	crypto *libCrypto.Crypto
}

// NewLegacyFieldEncryptor creates a new FieldEncryptor that wraps legacy libCrypto.Crypto.
// Use this for backward compatibility when envelope encryption is not configured.
//
// Deprecated: This function will be removed in T-008. Use NewFieldEncryptorAdapter
// with an EncryptionService that has Tink-backed legacy keys instead.
func NewLegacyFieldEncryptor(crypto *libCrypto.Crypto) FieldEncryptor {
	return &legacyFieldEncryptor{
		crypto: crypto,
	}
}

// EncryptField encrypts a plaintext value using legacy encryption.
// The FieldContext is accepted but not used for AAD binding in legacy mode.
func (l *legacyFieldEncryptor) EncryptField(_ context.Context, _ FieldContext, plaintext string) (string, error) {
	result, err := l.crypto.Encrypt(&plaintext)
	if err != nil {
		return "", err
	}

	if result == nil {
		return "", nil
	}

	return *result, nil
}

// DecryptField decrypts a ciphertext value using legacy encryption.
// The FieldContext is accepted but not used for AAD binding in legacy mode.
func (l *legacyFieldEncryptor) DecryptField(_ context.Context, _ FieldContext, ciphertext string) (string, error) {
	result, err := l.crypto.Decrypt(&ciphertext)
	if err != nil {
		return "", err
	}

	if result == nil {
		return "", nil
	}

	return *result, nil
}

// GenerateSearchToken generates a deterministic search token using legacy hash.
// The SearchTokenContext is accepted but legacy mode uses simple hash without context binding.
func (l *legacyFieldEncryptor) GenerateSearchToken(_ context.Context, _ SearchTokenContext, normalizedValue string) (string, error) {
	return l.crypto.GenerateHash(&normalizedValue), nil
}
