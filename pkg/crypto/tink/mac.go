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
