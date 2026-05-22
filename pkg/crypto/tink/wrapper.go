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
	// Encrypt encrypts plaintext using the key identified by keyName.
	// The keyName format is provider-specific and determined by the caller.
	// Returns the ciphertext in provider-specific format (e.g., "vault:v1:..." for Vault).
	Encrypt(ctx context.Context, keyName string, plaintext []byte) (string, error)

	// Decrypt decrypts ciphertext using the key identified by keyName.
	// The ciphertext format is provider-specific.
	Decrypt(ctx context.Context, keyName string, ciphertext string) ([]byte, error)

	// MountPath returns the KMS mount path (e.g., "transit" for Vault Transit).
	// Used by callers to construct full key paths if needed.
	MountPath() string
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
// The keyName format is determined by the caller (e.g., "tenant/org-123", "keys/mykey").
// Returns the wrapped keyset in provider-specific ciphertext format.
func (w *KeysetWrapper) WrapKeyset(ctx context.Context, keyName string, keyset []byte) (string, error) {
	if len(keyset) == 0 {
		return "", fmt.Errorf("cannot wrap empty keyset")
	}

	ciphertext, err := w.kms.Encrypt(ctx, keyName, keyset)
	if err != nil {
		return "", fmt.Errorf("failed to wrap keyset with KMS: %w", err)
	}

	return ciphertext, nil
}

// UnwrapKeyset decrypts a wrapped keyset using the KMS.
// The keyName must match the key used during wrapping.
// Returns the serialized keyset bytes that can be parsed into primitives.
func (w *KeysetWrapper) UnwrapKeyset(ctx context.Context, keyName string, wrappedKeyset string) ([]byte, error) {
	if wrappedKeyset == "" {
		return nil, fmt.Errorf("cannot unwrap empty ciphertext")
	}

	plaintext, err := w.kms.Decrypt(ctx, keyName, wrappedKeyset)
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap keyset with KMS: %w", err)
	}

	return plaintext, nil
}

// MountPath returns the KMS mount path from the underlying client.
// Useful for callers that need to construct full key paths.
func (w *KeysetWrapper) MountPath() string {
	return w.kms.MountPath()
}

// KeysetBundle contains a generated keyset with its wrapped form and metadata.
// This is returned by generation methods and can be stored by the caller.
type KeysetBundle struct {
	// Wrapped contains the KMS-encrypted keyset and its metadata.
	Wrapped WrappedKeyset
	// RawKeyset contains the serialized keyset bytes (cleartext).
	// This should only be held in memory temporarily and never persisted.
	RawKeyset []byte
}

// KeysetFactory provides convenient methods for generating and wrapping keysets.
// It combines keyset generation with KMS wrapping in a single operation.
type KeysetFactory struct {
	wrapper       *KeysetWrapper
	aeadGenerator *AEADKeysetGenerator
	macGenerator  *MACKeysetGenerator
}

// NewKeysetFactory creates a factory for generating and wrapping keysets.
// The KMS client is used for all wrapping operations.
func NewKeysetFactory(kms KMSClient) *KeysetFactory {
	return &KeysetFactory{
		wrapper:       NewKeysetWrapper(kms),
		aeadGenerator: NewAEADKeysetGenerator(),
		macGenerator:  NewMACKeysetGenerator(),
	}
}

// GenerateAEADKeyset creates a new AES256-GCM keyset and wraps it with the KMS.
// The keyName is caller-defined and should follow the caller's naming convention.
// Returns a bundle containing both the wrapped keyset and raw bytes for immediate use.
func (f *KeysetFactory) GenerateAEADKeyset(ctx context.Context, keyName string) (KeysetBundle, error) {
	handle, rawKeyset, err := f.aeadGenerator.Generate()
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to generate AEAD keyset: %w", err)
	}

	info, err := f.aeadGenerator.ExtractInfo(handle)
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to extract AEAD keyset info: %w", err)
	}

	wrappedData, err := f.wrapper.WrapKeyset(ctx, keyName, rawKeyset)
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to wrap AEAD keyset: %w", err)
	}

	return KeysetBundle{
		Wrapped:   NewWrappedKeyset(wrappedData, info, false),
		RawKeyset: rawKeyset,
	}, nil
}

// GenerateMACKeyset creates a new HMAC-SHA256 keyset and wraps it with the KMS.
// The keyName is caller-defined and should follow the caller's naming convention.
// Returns a bundle containing both the wrapped keyset and raw bytes for immediate use.
func (f *KeysetFactory) GenerateMACKeyset(ctx context.Context, keyName string) (KeysetBundle, error) {
	handle, rawKeyset, err := f.macGenerator.Generate()
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to generate MAC keyset: %w", err)
	}

	info, err := f.macGenerator.ExtractInfo(handle)
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to extract MAC keyset info: %w", err)
	}

	wrappedData, err := f.wrapper.WrapKeyset(ctx, keyName, rawKeyset)
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to wrap MAC keyset: %w", err)
	}

	return KeysetBundle{
		Wrapped:   NewWrappedKeyset(wrappedData, info, false),
		RawKeyset: rawKeyset,
	}, nil
}

// UnwrapAEAD unwraps a keyset and returns an AEAD primitive ready for use.
// The keyName must match the key used during wrapping.
func (f *KeysetFactory) UnwrapAEAD(ctx context.Context, keyName string, wrapped WrappedKeyset) (*AEADPrimitive, error) {
	keysetBytes, err := f.wrapper.UnwrapKeyset(ctx, keyName, wrapped.WrappedData)
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap AEAD keyset: %w", err)
	}

	primitive, err := ParseAEADKeyset(keysetBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AEAD keyset: %w", err)
	}

	return primitive, nil
}

// UnwrapMAC unwraps a keyset and returns a MAC primitive ready for use.
// The keyName must match the key used during wrapping.
func (f *KeysetFactory) UnwrapMAC(ctx context.Context, keyName string, wrapped WrappedKeyset) (*MACPrimitive, error) {
	keysetBytes, err := f.wrapper.UnwrapKeyset(ctx, keyName, wrapped.WrappedData)
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap MAC keyset: %w", err)
	}

	primitive, err := ParseMACKeyset(keysetBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MAC keyset: %w", err)
	}

	return primitive, nil
}

// Wrapper returns the underlying KeysetWrapper for direct access.
// Use this when you need fine-grained control over wrap/unwrap operations.
func (f *KeysetFactory) Wrapper() *KeysetWrapper {
	return f.wrapper
}
