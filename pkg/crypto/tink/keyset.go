// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package tink provides cryptographic primitives using Google Tink library.
// It offers AEAD encryption (AES256-GCM) and MAC operations (HMAC-SHA256)
// with support for keyset wrapping through external KMS providers.
//
// This package is domain-agnostic and does not contain any business logic.
// Callers are responsible for key naming conventions, storage, and context binding.
package tink

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
	// KeyTypeHMACSHA256 represents a standard Tink HMAC-SHA256 key for MAC operations.
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

// IsMACType returns true if the key type is a MAC key.
func (t KeyType) IsMACType() bool {
	return t == KeyTypeHMACSHA256 || t == KeyTypeLegacyHMACSHA256
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
