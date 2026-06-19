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
// Format: "tink:v{version}:{base64_payload}"
type EnvelopeMarker struct {
	Version uint32 // The organization's keyset version that produced the payload
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
	// Format after prefix removal: "{version}:{base64_payload}"
	remainder := strings.TrimPrefix(value, MarkerPrefix)

	// Find the first colon that separates the keyset version from the payload
	versionStr, payloadB64, found := strings.Cut(remainder, ":")
	if !found {
		return EnvelopeMarker{}, false, fmt.Errorf("invalid envelope marker format: missing payload separator")
	}

	// Parse keyset version
	if versionStr == "" {
		return EnvelopeMarker{}, false, fmt.Errorf("invalid keyset version: empty")
	}

	version, err := strconv.ParseUint(versionStr, 10, 32)
	if err != nil {
		return EnvelopeMarker{}, false, fmt.Errorf("invalid keyset version %q: %w", versionStr, err)
	}

	// Decode payload (URL-safe base64)
	payload, err := base64.URLEncoding.DecodeString(payloadB64)
	if err != nil {
		return EnvelopeMarker{}, false, fmt.Errorf("invalid payload encoding: %w", err)
	}

	return EnvelopeMarker{
		Version: uint32(version),
		Payload: payload,
	}, true, nil
}

// FormatEnvelopeMarker creates a marked ciphertext string from the organization's
// keyset version and the ciphertext payload. The version is the monotonic
// per-organization keyset version (NOT the Tink primary key id) and is what
// decrypt routes on.
//
// The payload is encoded with base64 URL-safe (consistent with search tokens),
// while the legacy on-disk path uses base64 Std for lib-commons compatibility.
// These two alphabets are intentionally different and MUST NOT be unified.
func FormatEnvelopeMarker(version uint32, payload []byte) string {
	payloadB64 := base64.URLEncoding.EncodeToString(payload)
	return fmt.Sprintf("%s%d:%s", MarkerPrefix, version, payloadB64)
}

// HasEnvelopeMarker returns true if the value starts with the envelope marker prefix.
func HasEnvelopeMarker(value string) bool {
	return strings.HasPrefix(value, MarkerPrefix)
}
