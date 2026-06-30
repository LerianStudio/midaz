// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"context"
	"fmt"
)

// KMSClient defines the interface for KMS operations used by the keyset wrapper.
// This abstraction allows the wrapper to work with different KMS providers
// (e.g., HashiCorp Vault, AWS KMS, Google Cloud KMS).
//
// Implementations must handle authentication and connection management internally.
// Callers should not need to manage KMS credentials after client construction.
type KMSClient interface {
	// Encrypt encrypts plaintext using the key identified by keyName under the
	// given mountPath. Both mountPath and keyName are provider-specific and
	// determined by the caller.
	// Returns the ciphertext in provider-specific format (e.g., "vault:v1:..." for Vault).
	Encrypt(ctx context.Context, mountPath, keyName string, plaintext []byte) (string, error)

	// Decrypt decrypts ciphertext using the key identified by keyName under the
	// given mountPath. The ciphertext format is provider-specific.
	Decrypt(ctx context.Context, mountPath, keyName string, ciphertext string) ([]byte, error)
}

// KeysetWrapper handles wrapping and unwrapping of Tink keysets using a KMS.
// It provides envelope encryption where DEKs (data encryption keys) are
// wrapped by KEKs (key encryption keys) managed by the KMS.
//
// The wrapper is stateless and thread-safe. Key naming conventions are
// determined by the caller, not by this wrapper.
type KeysetWrapper struct {
	kms KMSClient
}

// NewKeysetWrapper creates a new keyset wrapper with the given KMS client.
// The KMS client must be fully configured and authenticated.
func NewKeysetWrapper(kms KMSClient) *KeysetWrapper {
	return &KeysetWrapper{kms: kms}
}

// WrapKeyset encrypts a serialized keyset using the KMS.
// The mountPath and keyName are determined by the caller (e.g., shared engine
// "transit-mt" with key "tenant-x_org-123", op path
// "transit-mt/encrypt/tenant-x_org-123"). They are forwarded verbatim to the KMS;
// this wrapper performs no mount resolution.
// Returns the wrapped keyset in provider-specific ciphertext format.
func (w *KeysetWrapper) WrapKeyset(ctx context.Context, mountPath, keyName string, keyset []byte) (string, error) {
	if len(keyset) == 0 {
		return "", fmt.Errorf("cannot wrap empty keyset")
	}

	ciphertext, err := w.kms.Encrypt(ctx, mountPath, keyName, keyset)
	if err != nil {
		return "", fmt.Errorf("failed to wrap keyset with KMS: %w", err)
	}

	return ciphertext, nil
}

// UnwrapKeyset decrypts a wrapped keyset using the KMS.
// The mountPath and keyName must match those used during wrapping; both are
// forwarded verbatim to the KMS.
// Returns the serialized keyset bytes that can be parsed into primitives.
func (w *KeysetWrapper) UnwrapKeyset(ctx context.Context, mountPath, keyName string, wrappedKeyset string) ([]byte, error) {
	if wrappedKeyset == "" {
		return nil, fmt.Errorf("cannot unwrap empty ciphertext")
	}

	plaintext, err := w.kms.Decrypt(ctx, mountPath, keyName, wrappedKeyset)
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap keyset with KMS: %w", err)
	}

	return plaintext, nil
}
