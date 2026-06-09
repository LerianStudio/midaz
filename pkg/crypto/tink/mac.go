// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"encoding/base64"
	"fmt"

	"github.com/tink-crypto/tink-go/v2/keyset"
	"github.com/tink-crypto/tink-go/v2/mac"
	"github.com/tink-crypto/tink-go/v2/tink"
)

// MACKeysetGenerator creates new MAC keysets for token generation.
type MACKeysetGenerator struct{}

// NewMACKeysetGenerator creates a new MAC keyset generator.
func NewMACKeysetGenerator() *MACKeysetGenerator {
	return &MACKeysetGenerator{}
}

// Generate creates a new MAC keyset with HMAC-SHA256 as the primary key.
// Returns the keyset handle and serialized keyset bytes.
func (g *MACKeysetGenerator) Generate() (*keyset.Handle, []byte, error) {
	handle, err := keyset.NewHandle(mac.HMACSHA256Tag256KeyTemplate())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create MAC keyset: %w", err)
	}

	serialized, err := serializeKeyset(handle)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to serialize MAC keyset: %w", err)
	}

	return handle, serialized, nil
}

// ExtractInfo extracts keyset metadata from a keyset handle.
func (g *MACKeysetGenerator) ExtractInfo(handle *keyset.Handle) (KeysetInfo, error) {
	return extractKeysetInfo(handle, false)
}

// MACPrimitive wraps a Tink MAC primitive for computing message authentication codes.
// Used primarily for generating deterministic, searchable tokens from sensitive data.
type MACPrimitive struct {
	primitive tink.MAC
}

// NewMACPrimitive creates a MAC primitive from a keyset handle.
func NewMACPrimitive(handle *keyset.Handle) (*MACPrimitive, error) {
	primitive, err := mac.New(handle)
	if err != nil {
		return nil, fmt.Errorf("failed to create MAC primitive: %w", err)
	}

	return &MACPrimitive{primitive: primitive}, nil
}

// ComputeMAC computes a MAC tag for the given data.
// The output is deterministic: same data always produces the same tag.
func (m *MACPrimitive) ComputeMAC(data []byte) ([]byte, error) {
	tag, err := m.primitive.ComputeMAC(data)
	if err != nil {
		return nil, fmt.Errorf("MAC computation failed: %w", err)
	}

	return tag, nil
}

// VerifyMAC verifies that the tag is a valid MAC for the given data.
func (m *MACPrimitive) VerifyMAC(tag, data []byte) error {
	if err := m.primitive.VerifyMAC(tag, data); err != nil {
		return fmt.Errorf("MAC verification failed: %w", err)
	}

	return nil
}

// ComputeSearchToken generates a base64-encoded search token from data.
// Search tokens are deterministic MACs used for encrypted field searching.
func (m *MACPrimitive) ComputeSearchToken(data []byte) (string, error) {
	tag, err := m.ComputeMAC(data)
	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(tag), nil
}

// ParseMACKeyset parses a serialized keyset and returns a MAC primitive.
func ParseMACKeyset(serialized []byte) (*MACPrimitive, error) {
	handle, err := deserializeKeyset(serialized)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MAC keyset: %w", err)
	}

	return NewMACPrimitive(handle)
}

// DeserializeMACKeyset deserializes a MAC keyset and returns the keyset handle.
// This allows callers to create both MACPrimitive and MACMultiPrimitive from
// the same keyset without deserializing twice.
func DeserializeMACKeyset(serialized []byte) (*keyset.Handle, error) {
	handle, err := deserializeKeyset(serialized)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize MAC keyset: %w", err)
	}

	return handle, nil
}

// MACMultiPrimitive computes MACs using all enabled keys in a keyset.
// Used for search token generation where queries must match records
// indexed with any enabled key version.
type MACMultiPrimitive struct {
	primitives map[uint32]tink.MAC // keyID -> primitive
	keyIDs     []uint32            // ordered enabled key IDs
}

// NewMACMultiPrimitive creates a MAC multi-primitive from a keyset handle.
// It iterates over all keys in the keyset and creates individual MAC primitives
// for each enabled key. Returns an error if the handle is nil or has no enabled keys.
func NewMACMultiPrimitive(handle *keyset.Handle) (*MACMultiPrimitive, error) {
	if handle == nil {
		return nil, fmt.Errorf("keyset handle is nil")
	}

	keyCount := handle.Len()
	if keyCount == 0 {
		return nil, fmt.Errorf("keyset has no keys")
	}

	primitives := make(map[uint32]tink.MAC)
	keyIDs := make([]uint32, 0, keyCount)

	for i := range keyCount {
		entry, err := handle.Entry(i)
		if err != nil {
			return nil, fmt.Errorf("failed to get keyset entry %d: %w", i, err)
		}

		// Skip non-enabled keys (disabled or destroyed)
		if entry.KeyStatus() != keyset.Enabled {
			continue
		}

		keyID := entry.KeyID()

		// Create a single-key handle for this entry
		singleKeyHandle, err := createSingleKeyHandle(entry)
		if err != nil {
			return nil, fmt.Errorf("failed to create single-key handle for key %d: %w", keyID, err)
		}

		// Create MAC primitive for this key
		primitive, err := mac.New(singleKeyHandle)
		if err != nil {
			return nil, fmt.Errorf("failed to create MAC primitive for key %d: %w", keyID, err)
		}

		primitives[keyID] = primitive
		keyIDs = append(keyIDs, keyID)
	}

	if len(keyIDs) == 0 {
		return nil, fmt.Errorf("keyset has no enabled keys")
	}

	return &MACMultiPrimitive{
		primitives: primitives,
		keyIDs:     keyIDs,
	}, nil
}

// ComputeSearchTokenCandidates computes base64-encoded MAC tokens for the given data
// using all enabled keys in the keyset. Returns tokens ordered by key ID ascending
// for deterministic output. If any MAC computation fails, returns error immediately.
func (m *MACMultiPrimitive) ComputeSearchTokenCandidates(data []byte) ([]string, error) {
	tokens := make([]string, 0, len(m.keyIDs))

	// Iterate in keyIDs order (already sorted by insertion order from keyset)
	for _, keyID := range m.keyIDs {
		primitive, ok := m.primitives[keyID]
		if !ok {
			return nil, fmt.Errorf("primitive not found for key %d", keyID)
		}

		tag, err := primitive.ComputeMAC(data)
		if err != nil {
			return nil, fmt.Errorf("failed to compute MAC for key %d: %w", keyID, err)
		}

		token := base64.URLEncoding.EncodeToString(tag)
		tokens = append(tokens, token)
	}

	return tokens, nil
}

// GetEnabledKeyIDs returns the key IDs of all enabled keys in the primitive.
// The returned slice is a copy to prevent external mutation of internal state.
// Key IDs are ordered by keyset entry order (same order used by ComputeSearchTokenCandidates).
func (m *MACMultiPrimitive) GetEnabledKeyIDs() []uint32 {
	result := make([]uint32, len(m.keyIDs))
	copy(result, m.keyIDs)

	return result
}

// createSingleKeyHandle creates a keyset handle containing only the specified key entry.
func createSingleKeyHandle(entry *keyset.Entry) (*keyset.Handle, error) {
	manager := keyset.NewManager()

	keyID, err := manager.AddKey(entry.Key())
	if err != nil {
		return nil, fmt.Errorf("failed to add key to manager: %w", err)
	}

	if err := manager.SetPrimary(keyID); err != nil {
		return nil, fmt.Errorf("failed to set primary key: %w", err)
	}

	return manager.Handle()
}
