// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// MarkerPrefix is the prefix used to identify envelope-encrypted values.
const MarkerPrefix = "tink:v"

// EnvelopeMarker identifies ciphertext encrypted with envelope encryption.
// Format: "tink:v{keyID}:{base64_payload}"
type EnvelopeMarker struct {
	KeyID   uint32 // Primary key ID from the keyset
	Payload []byte // The actual ciphertext (decoded from base64)
}

// ParseEnvelopeMarker parses a marked ciphertext string.
// Returns (marker, true, nil) if parsing succeeds.
// Returns (EnvelopeMarker{}, false, nil) if the value has no marker (legacy candidate).
// Returns (EnvelopeMarker{}, false, error) if the value has a marker but is malformed.
func ParseEnvelopeMarker(value string) (EnvelopeMarker, bool, error) {
	// Check if value has the envelope marker prefix
	if !HasEnvelopeMarker(value) {
		return EnvelopeMarker{}, false, nil
	}

	// Remove prefix and split by colon
	// Format after prefix removal: "{keyID}:{base64_payload}"
	remainder := strings.TrimPrefix(value, MarkerPrefix)

	// Find the first colon that separates key ID from payload
	keyIDStr, payloadB64, found := strings.Cut(remainder, ":")
	if !found {
		return EnvelopeMarker{}, false, fmt.Errorf("invalid envelope marker format: missing payload separator")
	}

	// Parse key ID
	if keyIDStr == "" {
		return EnvelopeMarker{}, false, fmt.Errorf("invalid key ID: empty")
	}

	keyID, err := strconv.ParseUint(keyIDStr, 10, 32)
	if err != nil {
		return EnvelopeMarker{}, false, fmt.Errorf("invalid key ID %q: %w", keyIDStr, err)
	}

	// Decode payload (URL-safe base64)
	payload, err := base64.URLEncoding.DecodeString(payloadB64)
	if err != nil {
		return EnvelopeMarker{}, false, fmt.Errorf("invalid payload encoding: %w", err)
	}

	return EnvelopeMarker{
		KeyID:   uint32(keyID),
		Payload: payload,
	}, true, nil
}

// FormatEnvelopeMarker creates a marked ciphertext string from key ID and payload.
func FormatEnvelopeMarker(keyID uint32, payload []byte) string {
	payloadB64 := base64.URLEncoding.EncodeToString(payload)
	return fmt.Sprintf("%s%d:%s", MarkerPrefix, keyID, payloadB64)
}

// HasEnvelopeMarker returns true if the value starts with the envelope marker prefix.
func HasEnvelopeMarker(value string) bool {
	return strings.HasPrefix(value, MarkerPrefix)
}
