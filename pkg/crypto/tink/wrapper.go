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
// The mountPath and keyName are determined by the caller (e.g., mount
// "transit/tenant-x" with key "tenant/org-123"). They are forwarded verbatim
// to the KMS; this wrapper performs no mount resolution.
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

// KeysetBundle contains a generated keyset with its wrapped form and metadata.
// This is returned by generation methods and can be stored by the caller.
//
// SECURITY: This struct contains cleartext key material in RawKeyset.
// Callers MUST NOT persist RawKeyset to disk, logs, or any storage.
// Use RawKeyset only for immediate in-memory cryptographic operations,
// then discard it. For storage, use only the Wrapped field.
type KeysetBundle struct {
	// Wrapped contains the KMS-encrypted keyset and its metadata.
	// This is the ONLY field safe to persist.
	Wrapped WrappedKeyset
	// RawKeyset contains the serialized keyset bytes (CLEARTEXT KEY MATERIAL).
	// WARNING: Never persist, log, or transmit this field.
	// Intended only for immediate use after generation, then discard.
	RawKeyset []byte
}

// KeysetFactory provides convenient methods for generating and wrapping keysets.
// It combines keyset generation with KMS wrapping in a single operation.
type KeysetFactory struct {
	wrapper       *KeysetWrapper
	aeadGenerator *AEADKeysetGenerator
	prfGenerator  *PRFKeysetGenerator
}

// NewKeysetFactory creates a factory for generating and wrapping keysets.
// The KMS client is used for all wrapping operations.
func NewKeysetFactory(kms KMSClient) *KeysetFactory {
	return &KeysetFactory{
		wrapper:       NewKeysetWrapper(kms),
		aeadGenerator: NewAEADKeysetGenerator(),
		prfGenerator:  NewPRFKeysetGenerator(),
	}
}

// GenerateAEADKeyset creates a new AES256-GCM keyset and wraps it with the KMS.
// The mountPath and keyName are caller-defined and should follow the caller's
// naming convention; both are forwarded verbatim to the wrapper.
// Returns a bundle containing both the wrapped keyset and raw bytes for immediate use.
func (f *KeysetFactory) GenerateAEADKeyset(ctx context.Context, mountPath, keyName string) (KeysetBundle, error) {
	handle, rawKeyset, err := f.aeadGenerator.Generate()
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to generate AEAD keyset: %w", err)
	}

	info, err := f.aeadGenerator.ExtractInfo(handle)
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to extract AEAD keyset info: %w", err)
	}

	wrappedData, err := f.wrapper.WrapKeyset(ctx, mountPath, keyName, rawKeyset)
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to wrap AEAD keyset: %w", err)
	}

	return KeysetBundle{
		Wrapped:   NewWrappedKeyset(wrappedData, info, false),
		RawKeyset: rawKeyset,
	}, nil
}

// GeneratePRFKeyset creates a new HMAC-SHA256 PRF keyset and wraps it with the KMS.
// The mountPath and keyName are caller-defined and should follow the caller's
// naming convention; both are forwarded verbatim to the wrapper.
// The keyset metadata is labeled with KeyTypeHMACPRF via ExtractInfo.
// Returns a bundle containing both the wrapped keyset and raw bytes for immediate use.
func (f *KeysetFactory) GeneratePRFKeyset(ctx context.Context, mountPath, keyName string) (KeysetBundle, error) {
	handle, rawKeyset, err := f.prfGenerator.Generate()
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to generate PRF keyset: %w", err)
	}

	info, err := f.prfGenerator.ExtractInfo(handle)
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to extract PRF keyset info: %w", err)
	}

	wrappedData, err := f.wrapper.WrapKeyset(ctx, mountPath, keyName, rawKeyset)
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to wrap PRF keyset: %w", err)
	}

	return KeysetBundle{
		Wrapped:   NewWrappedKeyset(wrappedData, info, false),
		RawKeyset: rawKeyset,
	}, nil
}

// UnwrapAEAD unwraps a keyset and returns an AEAD primitive ready for use.
// The mountPath and keyName must match those used during wrapping.
func (f *KeysetFactory) UnwrapAEAD(ctx context.Context, mountPath, keyName string, wrapped WrappedKeyset) (*AEADPrimitive, error) {
	keysetBytes, err := f.wrapper.UnwrapKeyset(ctx, mountPath, keyName, wrapped.WrappedData)
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap AEAD keyset: %w", err)
	}

	primitive, err := ParseAEADKeyset(keysetBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AEAD keyset: %w", err)
	}

	return primitive, nil
}

// UnwrapPRF unwraps a keyset and returns a PRF primitive ready for use.
// The mountPath and keyName must match those used during wrapping.
func (f *KeysetFactory) UnwrapPRF(ctx context.Context, mountPath, keyName string, wrapped WrappedKeyset) (*PRFPrimitive, error) {
	keysetBytes, err := f.wrapper.UnwrapKeyset(ctx, mountPath, keyName, wrapped.WrappedData)
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap PRF keyset: %w", err)
	}

	primitive, err := ParsePRFKeyset(keysetBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PRF keyset: %w", err)
	}

	return primitive, nil
}

// Wrapper returns the underlying KeysetWrapper for direct access.
// Use this when you need fine-grained control over wrap/unwrap operations.
func (f *KeysetFactory) Wrapper() *KeysetWrapper {
	return f.wrapper
}
