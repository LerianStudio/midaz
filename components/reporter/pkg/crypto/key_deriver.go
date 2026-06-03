// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package crypto provides key derivation utilities for the reporter worker.
// The HKDFKeyDeriver derives cryptographically independent keys from a shared
// master key (APP_ENC_KEY) using HKDF-SHA256 (RFC 5869). The derived keys are
// compatible with the fetcher's key derivation (same contexts produce same keys).
//
// TODO: Move to lib-commons as shared library (see lib-commons TASK-002).
package crypto

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// Key derivation contexts — must match the fetcher's contexts exactly
// to produce the same derived keys from the same master key.
const (
	// ContextExternalHMAC derives the key for verifying document HMACs from fetcher.
	ContextExternalHMAC = "fetcher-external-hmac-v1"

	// ContextStorageEncryption derives the key for decrypting extracted data from SeaweedFS.
	ContextStorageEncryption = "fetcher-storage-encryption-v1"
)

// DefaultKeyLength is the standard key length for AES-256 and HMAC-SHA256.
const DefaultKeyLength = 32

// HKDFKeyDeriver implements key derivation using HKDF (RFC 5869) with SHA-256.
// Keys are derived once at construction time and cached for performance.
type HKDFKeyDeriver struct {
	masterKey         []byte
	externalHMACKey   []byte
	storageEncryptKey []byte
}

// NewHKDFKeyDeriver creates a new key deriver from a master key.
// The master key should be at least 32 bytes for security.
func NewHKDFKeyDeriver(masterKey []byte) (*HKDFKeyDeriver, error) {
	if len(masterKey) < DefaultKeyLength {
		return nil, fmt.Errorf("master key too short: got %d bytes, minimum %d required", len(masterKey), DefaultKeyLength)
	}

	deriver := &HKDFKeyDeriver{
		masterKey: masterKey,
	}

	var err error

	deriver.externalHMACKey, err = deriver.DeriveKey(ContextExternalHMAC, DefaultKeyLength)
	if err != nil {
		return nil, fmt.Errorf("failed to derive external HMAC key: %w", err)
	}

	deriver.storageEncryptKey, err = deriver.DeriveKey(ContextStorageEncryption, DefaultKeyLength)
	if err != nil {
		return nil, fmt.Errorf("failed to derive storage encryption key: %w", err)
	}

	return deriver, nil
}

// DeriveKey derives a key of the specified length for the given context using HKDF.
func (d *HKDFKeyDeriver) DeriveKey(context string, length int) ([]byte, error) {
	if length <= 0 {
		return nil, fmt.Errorf("key length must be positive")
	}

	if context == "" {
		return nil, fmt.Errorf("context cannot be empty")
	}

	reader := hkdf.New(sha256.New, d.masterKey, nil, []byte(context))

	key := make([]byte, length)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	return key, nil
}

// GetExternalHMACKey returns a copy of the pre-derived key for HMAC verification.
func (d *HKDFKeyDeriver) GetExternalHMACKey() []byte {
	keyCopy := make([]byte, len(d.externalHMACKey))
	copy(keyCopy, d.externalHMACKey)

	return keyCopy
}

// GetStorageEncryptKey returns a copy of the pre-derived key for storage decryption.
func (d *HKDFKeyDeriver) GetStorageEncryptKey() []byte {
	keyCopy := make([]byte, len(d.storageEncryptKey))
	copy(keyCopy, d.storageEncryptKey)

	return keyCopy
}

// DecodeMasterKey decodes a Base64-encoded master key (APP_ENC_KEY).
func DecodeMasterKey(keyBase64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid master key (base64): %w", err)
	}

	if len(key) < DefaultKeyLength {
		return nil, fmt.Errorf("master key too short: got %d bytes, minimum %d required", len(key), DefaultKeyLength)
	}

	return key, nil
}
