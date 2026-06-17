// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"encoding/base64"
	"fmt"

	"github.com/tink-crypto/tink-go/v2/keyset"
	"github.com/tink-crypto/tink-go/v2/prf"
)

// searchTokenPRFOutputBytes is the fixed output length, in bytes, of every
// PRF-based search token. Frozen so tokens stay comparable across keys and
// keysets; do not thread this through any public signature.
const searchTokenPRFOutputBytes uint32 = 32

// PRFKeysetGenerator creates new PRF keysets for search-token generation.
type PRFKeysetGenerator struct{}

// NewPRFKeysetGenerator creates a new PRF keyset generator.
func NewPRFKeysetGenerator() *PRFKeysetGenerator {
	return &PRFKeysetGenerator{}
}

// Generate creates a new PRF keyset with HMAC-SHA256 PRF as the primary key.
// Returns the keyset handle and serialized keyset bytes.
func (g *PRFKeysetGenerator) Generate() (*keyset.Handle, []byte, error) {
	handle, err := keyset.NewHandle(prf.HMACSHA256PRFKeyTemplate())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create PRF keyset: %w", err)
	}

	serialized, err := serializeKeyset(handle)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to serialize PRF keyset: %w", err)
	}

	return handle, serialized, nil
}

// ExtractInfo extracts keyset metadata from a keyset handle.
func (g *PRFKeysetGenerator) ExtractInfo(handle *keyset.Handle) (KeysetInfo, error) {
	return extractKeysetInfo(handle, keyPurposePRF)
}

// PRFPrimitive wraps a Tink PRF set for computing deterministic search tokens.
// PRF output is RAW (no Tink key-id prefix).
type PRFPrimitive struct {
	set *prf.Set
}

// NewPRFPrimitive creates a PRF primitive from a keyset handle.
func NewPRFPrimitive(handle *keyset.Handle) (*PRFPrimitive, error) {
	set, err := prf.NewPRFSet(handle)
	if err != nil {
		return nil, fmt.Errorf("failed to create PRF primitive: %w", err)
	}

	return &PRFPrimitive{set: set}, nil
}

// ComputeSearchToken generates a base64-encoded search token from data using
// the primary key. Tokens are deterministic: same data always produces the
// same token. Output is the RAW PRF value of fixed length.
func (p *PRFPrimitive) ComputeSearchToken(data []byte) (string, error) {
	out, err := p.set.ComputePrimaryPRF(data, searchTokenPRFOutputBytes)
	if err != nil {
		return "", fmt.Errorf("PRF computation failed: %w", err)
	}

	return base64.URLEncoding.EncodeToString(out), nil
}

// ParsePRFKeyset parses a serialized keyset and returns a PRF primitive.
func ParsePRFKeyset(serialized []byte) (*PRFPrimitive, error) {
	handle, err := deserializeKeyset(serialized)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PRF keyset: %w", err)
	}

	return NewPRFPrimitive(handle)
}

// DeserializePRFKeyset deserializes a PRF keyset and returns the keyset handle.
// This allows callers to create both a PRFPrimitive and a PRFMultiPrimitive from
// the same keyset without deserializing twice.
func DeserializePRFKeyset(serialized []byte) (*keyset.Handle, error) {
	handle, err := deserializeKeyset(serialized)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize PRF keyset: %w", err)
	}

	return handle, nil
}

// PRFMultiPrimitive computes PRF tokens using all enabled keys in a keyset.
// Used for search token generation where queries must match records
// indexed with any enabled key version.
type PRFMultiPrimitive struct {
	prfs   map[uint32]prf.PRF // keyID -> PRF
	keyIDs []uint32           // ordered enabled key IDs
}

// NewPRFMultiPrimitive creates a PRF multi-primitive from a keyset handle.
// It iterates over all keys in the keyset and creates individual PRFs for each
// enabled key. Returns an error if the handle is nil or has no enabled keys.
func NewPRFMultiPrimitive(handle *keyset.Handle) (*PRFMultiPrimitive, error) {
	if handle == nil {
		return nil, fmt.Errorf("keyset handle is nil")
	}

	// A valid Tink handle always has at least one (primary) key, so there is no
	// empty-keyset guard here; the enabled-key count is checked after the loop.
	keyCount := handle.Len()

	prfs := make(map[uint32]prf.PRF)
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

		set, err := prf.NewPRFSet(singleKeyHandle)
		if err != nil {
			return nil, fmt.Errorf("failed to create PRF primitive for key %d: %w", keyID, err)
		}

		primitive, ok := set.PRFs[set.PrimaryID]
		if !ok {
			return nil, fmt.Errorf("PRF not found for key %d", keyID)
		}

		prfs[keyID] = primitive
		keyIDs = append(keyIDs, keyID)
	}

	if len(keyIDs) == 0 {
		return nil, fmt.Errorf("keyset has no enabled keys")
	}

	return &PRFMultiPrimitive{
		prfs:   prfs,
		keyIDs: keyIDs,
	}, nil
}

// ComputeSearchTokenCandidates computes base64-encoded PRF tokens for the given
// data using all enabled keys in the keyset. Returns tokens in the keyset's
// enabled-key order for deterministic output. If any computation fails, returns
// error immediately.
func (p *PRFMultiPrimitive) ComputeSearchTokenCandidates(data []byte) ([]string, error) {
	tokens := make([]string, 0, len(p.keyIDs))

	for _, keyID := range p.keyIDs {
		primitive, ok := p.prfs[keyID]
		if !ok {
			return nil, fmt.Errorf("PRF not found for key %d", keyID)
		}

		out, err := primitive.ComputePRF(data, searchTokenPRFOutputBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to compute PRF for key %d: %w", keyID, err)
		}

		tokens = append(tokens, base64.URLEncoding.EncodeToString(out))
	}

	return tokens, nil
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
