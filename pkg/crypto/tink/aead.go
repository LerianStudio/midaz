// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"bytes"
	"fmt"

	"github.com/tink-crypto/tink-go/v2/aead"
	"github.com/tink-crypto/tink-go/v2/insecurecleartextkeyset"
	"github.com/tink-crypto/tink-go/v2/keyset"
	"github.com/tink-crypto/tink-go/v2/tink"
)

// AEADKeysetGenerator creates new AEAD keysets for encryption operations.
type AEADKeysetGenerator struct{}

// NewAEADKeysetGenerator creates a new AEAD keyset generator.
func NewAEADKeysetGenerator() *AEADKeysetGenerator {
	return &AEADKeysetGenerator{}
}

// Generate creates a new AEAD keyset with AES256-GCM as the primary key.
// Returns the keyset handle and serialized keyset bytes.
func (g *AEADKeysetGenerator) Generate() (*keyset.Handle, []byte, error) {
	handle, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create AEAD keyset: %w", err)
	}

	serialized, err := serializeKeyset(handle)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to serialize AEAD keyset: %w", err)
	}

	return handle, serialized, nil
}

// ExtractInfo extracts keyset metadata from a keyset handle.
func (g *AEADKeysetGenerator) ExtractInfo(handle *keyset.Handle) (KeysetInfo, error) {
	return extractKeysetInfo(handle, true)
}

// AEADPrimitive wraps a Tink AEAD primitive for encryption/decryption operations.
type AEADPrimitive struct {
	primitive tink.AEAD
}

// NewAEADPrimitive creates an AEAD primitive from a keyset handle.
func NewAEADPrimitive(handle *keyset.Handle) (*AEADPrimitive, error) {
	primitive, err := aead.New(handle)
	if err != nil {
		return nil, fmt.Errorf("failed to create AEAD primitive: %w", err)
	}

	return &AEADPrimitive{primitive: primitive}, nil
}

// Encrypt encrypts plaintext with optional associated data.
// The associated data is authenticated but not encrypted.
func (a *AEADPrimitive) Encrypt(plaintext, associatedData []byte) ([]byte, error) {
	ciphertext, err := a.primitive.Encrypt(plaintext, associatedData)
	if err != nil {
		return nil, fmt.Errorf("AEAD encryption failed: %w", err)
	}

	return ciphertext, nil
}

// Decrypt decrypts ciphertext with the same associated data used for encryption.
func (a *AEADPrimitive) Decrypt(ciphertext, associatedData []byte) ([]byte, error) {
	plaintext, err := a.primitive.Decrypt(ciphertext, associatedData)
	if err != nil {
		return nil, fmt.Errorf("AEAD decryption failed: %w", err)
	}

	return plaintext, nil
}

// ParseAEADKeyset parses a serialized keyset and returns an AEAD primitive.
func ParseAEADKeyset(serialized []byte) (*AEADPrimitive, error) {
	handle, err := deserializeKeyset(serialized)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AEAD keyset: %w", err)
	}

	return NewAEADPrimitive(handle)
}

// serializeKeyset serializes a keyset handle to binary format.
// Uses insecure cleartext for wrapping by external KMS.
func serializeKeyset(handle *keyset.Handle) ([]byte, error) {
	var buf bytes.Buffer

	writer := keyset.NewBinaryWriter(&buf)

	if err := insecurecleartextkeyset.Write(handle, writer); err != nil {
		return nil, fmt.Errorf("failed to write keyset: %w", err)
	}

	return buf.Bytes(), nil
}

// deserializeKeyset deserializes a binary keyset into a handle.
func deserializeKeyset(data []byte) (*keyset.Handle, error) {
	reader := keyset.NewBinaryReader(bytes.NewReader(data))

	handle, err := insecurecleartextkeyset.Read(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read keyset: %w", err)
	}

	return handle, nil
}

// extractKeysetInfo extracts metadata from a keyset handle.
func extractKeysetInfo(handle *keyset.Handle, isAEAD bool) (KeysetInfo, error) {
	info := handle.KeysetInfo()
	if info == nil {
		return KeysetInfo{}, fmt.Errorf("keyset info is nil")
	}

	keys := make([]KeyInfo, 0, len(info.KeyInfo))

	for _, keyInfo := range info.KeyInfo {
		keyType := determineKeyType(keyInfo.TypeUrl, isAEAD)
		status := mapKeyStatus(int32(keyInfo.Status))

		keys = append(keys, KeyInfo{
			KeyID:     keyInfo.KeyId,
			Status:    status,
			Type:      keyType,
			IsPrimary: keyInfo.KeyId == info.PrimaryKeyId,
		})
	}

	return KeysetInfo{
		PrimaryKeyID: info.PrimaryKeyId,
		Keys:         keys,
	}, nil
}

// determineKeyType maps Tink type URLs to our KeyType constants.
func determineKeyType(_ string, isAEAD bool) KeyType {
	// Tink type URLs follow the pattern: type.googleapis.com/google.crypto.tink.*
	// For AES-GCM: type.googleapis.com/google.crypto.tink.AesGcmKey
	// For HMAC: type.googleapis.com/google.crypto.tink.HmacKey
	// Currently we infer the type from the keyset purpose (AEAD vs MAC).
	// Future enhancement: parse typeURL to detect specific algorithm variants.
	if isAEAD {
		return KeyTypeAES256GCM
	}

	return KeyTypeHMACSHA256
}

// mapKeyStatus maps Tink KeyStatusType to our KeyStatus.
// The Tink proto uses int32 values: UNKNOWN=0, ENABLED=1, DISABLED=2, DESTROYED=3.
func mapKeyStatus(status int32) KeyStatus {
	const (
		tinkStatusEnabled   = 1
		tinkStatusDisabled  = 2
		tinkStatusDestroyed = 3
	)

	switch status {
	case tinkStatusEnabled:
		return KeyStatusEnabled
	case tinkStatusDisabled:
		return KeyStatusDisabled
	case tinkStatusDestroyed:
		return KeyStatusDestroyed
	default:
		return KeyStatusDisabled
	}
}
