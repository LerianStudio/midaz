// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package tink provides cryptographic primitives using Google Tink library.
// It offers AEAD encryption (AES256-GCM) and PRF-based search tokens (HMAC-SHA256)
// with support for keyset wrapping through external KMS providers.
//
// This package is domain-agnostic and does not contain any business logic.
// Callers are responsible for key naming conventions, storage, and context binding.
package tink

import (
	"context"
	"fmt"
)

// KeyStatus represents the status of a key within a keyset.
type KeyStatus string

const (
	// KeyStatusEnabled indicates the key can be used for cryptographic operations.
	KeyStatusEnabled KeyStatus = "ENABLED"
	// KeyStatusDisabled indicates the key cannot be used but is retained for decryption.
	KeyStatusDisabled KeyStatus = "DISABLED"
	// KeyStatusDestroyed indicates the key material has been destroyed.
	KeyStatusDestroyed KeyStatus = "DESTROYED"
)

// IsValid returns true if the key status is a recognized value.
func (s KeyStatus) IsValid() bool {
	switch s {
	case KeyStatusEnabled, KeyStatusDisabled, KeyStatusDestroyed:
		return true
	default:
		return false
	}
}

// KeyType represents the type of cryptographic key.
type KeyType string

const (
	// KeyTypeAES256GCM represents a standard Tink AES256-GCM key for AEAD operations.
	KeyTypeAES256GCM KeyType = "AES256_GCM"
	// KeyTypeLegacyAESGCM represents a legacy AES-GCM key imported from a previous system.
	KeyTypeLegacyAESGCM KeyType = "LEGACY_AES_GCM"
	// KeyTypeHMACSHA256 preserves the legacy HMAC-SHA256 compatibility label.
	KeyTypeHMACSHA256 KeyType = "HMAC_SHA256"
	// KeyTypeLegacyHMACSHA256 represents a legacy HMAC-SHA256 key imported from a previous system.
	KeyTypeLegacyHMACSHA256 KeyType = "LEGACY_HMAC_SHA256"
	// KeyTypeHMACPRF represents a standard Tink HMAC-SHA256 PRF key for search-token operations.
	KeyTypeHMACPRF KeyType = "HMAC_PRF"
)

// IsAEADType returns true if the key type is an AEAD key.
func (t KeyType) IsAEADType() bool {
	return t == KeyTypeAES256GCM || t == KeyTypeLegacyAESGCM
}

// IsLegacy returns true if the key type is a legacy key.
func (t KeyType) IsLegacy() bool {
	return t == KeyTypeLegacyAESGCM || t == KeyTypeLegacyHMACSHA256
}

// KeyInfo describes a single key within a keyset.
type KeyInfo struct {
	// KeyID is the unique identifier for this key within the keyset.
	KeyID uint32 `json:"key_id"`
	// Status indicates whether the key is enabled for operations.
	Status KeyStatus `json:"status"`
	// Type identifies the cryptographic algorithm of the key.
	Type KeyType `json:"type"`
	// IsPrimary indicates whether this is the primary key used for new operations.
	IsPrimary bool `json:"is_primary"`
}

// KeysetInfo contains metadata about a Tink keyset without exposing key material.
// This structure can be safely stored and transmitted as it contains no secrets.
type KeysetInfo struct {
	// PrimaryKeyID is the ID of the primary key used for new operations.
	PrimaryKeyID uint32 `json:"primary_key_id"`
	// Keys contains metadata about each key in the keyset.
	Keys []KeyInfo `json:"keys"`
}

// NewKeysetInfo creates a KeysetInfo from key entries.
func NewKeysetInfo(primaryKeyID uint32, keys []KeyInfo) KeysetInfo {
	return KeysetInfo{
		PrimaryKeyID: primaryKeyID,
		Keys:         keys,
	}
}

// HasKey returns true if the keyset contains a key with the given ID.
func (k KeysetInfo) HasKey(keyID uint32) bool {
	for _, key := range k.Keys {
		if key.KeyID == keyID {
			return true
		}
	}

	return false
}

// GetPrimaryKey returns the primary key info, or nil if not found.
func (k KeysetInfo) GetPrimaryKey() *KeyInfo {
	for i := range k.Keys {
		if k.Keys[i].IsPrimary {
			return &k.Keys[i]
		}
	}

	return nil
}

// HasLegacyKey returns true if any key in the keyset is a legacy key.
func (k KeysetInfo) HasLegacyKey() bool {
	for _, key := range k.Keys {
		if key.Type.IsLegacy() {
			return true
		}
	}

	return false
}

// WrappedKeyset represents a keyset that has been wrapped (encrypted) using a KMS.
// The actual key material is encrypted and only the metadata is visible.
type WrappedKeyset struct {
	// WrappedData contains the encrypted keyset data from the KMS provider.
	// Format depends on provider (e.g., "vault:v<version>:<base64_ciphertext>" for Vault).
	WrappedData string `json:"wrapped_data"`
	// Info contains metadata about the keyset without key material.
	Info KeysetInfo `json:"keyset_info"`
	// LegacyKeyImported indicates whether a legacy key was imported into this keyset.
	LegacyKeyImported bool `json:"legacy_key_imported,omitempty"`
}

// NewWrappedKeyset creates a WrappedKeyset with the given data and info.
func NewWrappedKeyset(wrappedData string, info KeysetInfo, legacyImported bool) WrappedKeyset {
	return WrappedKeyset{
		WrappedData:       wrappedData,
		Info:              info,
		LegacyKeyImported: legacyImported,
	}
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

// GenerateMixedAEADKeyset creates a COMPOSITE AEAD keyset for legacy-key import
// migration: a freshly generated AES-256-GCM key set as PRIMARY plus the imported
// legacy AES-GCM key (from legacyHexKey) as an ENABLED, NON-primary entry. The
// complete composite keyset is KMS-wrapped via the same path as GenerateAEADKeyset.
//
// The returned bundle's Wrapped.LegacyKeyImported is true and Wrapped.Info.Keys
// holds BOTH entries with correct IsPrimary flags and KeyTypes (AES256_GCM for the
// fresh primary, LEGACY_AES_GCM for the imported key).
//
// It FAILS CLOSED if legacyHexKey is missing or invalid (no keyset is generated or
// wrapped in that case). It never returns or persists the raw legacy secret.
func (f *KeysetFactory) GenerateMixedAEADKeyset(ctx context.Context, mountPath, keyName, legacyHexKey string) (KeysetBundle, error) {
	legacyKey, err := legacyAESGCMKeysetKey(legacyHexKey, legacyComposedKeyID)
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to import legacy AES-GCM key: %w", err)
	}

	freshHandle, _, err := f.aeadGenerator.Generate()
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to generate fresh AEAD keyset: %w", err)
	}

	rawKeyset, info, err := composeMixedKeyset(freshHandle, legacyKey, keyPurposeAEAD, KeyTypeLegacyAESGCM)
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to compose mixed AEAD keyset: %w", err)
	}

	wrappedData, err := f.wrapper.WrapKeyset(ctx, mountPath, keyName, rawKeyset)
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to wrap mixed AEAD keyset: %w", err)
	}

	return KeysetBundle{
		Wrapped:   NewWrappedKeyset(wrappedData, info, true),
		RawKeyset: rawKeyset,
	}, nil
}

// GenerateMixedPRFKeyset creates a COMPOSITE PRF keyset for legacy-key import
// migration: a freshly generated HMAC-SHA256-PRF key set as PRIMARY plus the
// imported legacy HMAC-SHA256 key (from legacySecret) as an ENABLED, NON-primary
// entry. The complete composite keyset is KMS-wrapped via the same path as
// GeneratePRFKeyset.
//
// The returned bundle's Wrapped.LegacyKeyImported is true and Wrapped.Info.Keys
// holds BOTH entries with correct IsPrimary flags and KeyTypes (HMAC_PRF for the
// fresh primary, LEGACY_HMAC_SHA256 for the imported key).
//
// It FAILS CLOSED if legacySecret is empty (no keyset is generated or wrapped in
// that case). It never returns or persists the raw legacy secret.
func (f *KeysetFactory) GenerateMixedPRFKeyset(ctx context.Context, mountPath, keyName, legacySecret string) (KeysetBundle, error) {
	legacyKey, err := legacyHMACPRFKeysetKey(legacySecret, legacyComposedKeyID)
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to import legacy HMAC key: %w", err)
	}

	freshHandle, _, err := f.prfGenerator.Generate()
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to generate fresh PRF keyset: %w", err)
	}

	rawKeyset, info, err := composeMixedKeyset(freshHandle, legacyKey, keyPurposePRF, KeyTypeLegacyHMACSHA256)
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to compose mixed PRF keyset: %w", err)
	}

	wrappedData, err := f.wrapper.WrapKeyset(ctx, mountPath, keyName, rawKeyset)
	if err != nil {
		return KeysetBundle{}, fmt.Errorf("failed to wrap mixed PRF keyset: %w", err)
	}

	return KeysetBundle{
		Wrapped:   NewWrappedKeyset(wrappedData, info, true),
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
