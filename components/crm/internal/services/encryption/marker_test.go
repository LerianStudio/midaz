// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"encoding/base64"
	"testing"
)

func TestParseEnvelopeMarker(t *testing.T) {
	t.Parallel()

	// Pre-encode a valid payload for testing
	validPayload := []byte("encrypted-data-bytes")
	validPayloadB64 := base64.URLEncoding.EncodeToString(validPayload)

	tests := []struct {
		name          string
		value         string
		wantMarker    EnvelopeMarker
		wantHasMarker bool
		wantErr       bool
		errContains   string
	}{
		{
			name:  "valid marker with key ID 1",
			value: "tink:v1:" + validPayloadB64,
			wantMarker: EnvelopeMarker{
				KeyID:   1,
				Payload: validPayload,
			},
			wantHasMarker: true,
			wantErr:       false,
		},
		{
			name:  "valid marker with large key ID",
			value: "tink:v4294967295:" + validPayloadB64,
			wantMarker: EnvelopeMarker{
				KeyID:   4294967295, // max uint32
				Payload: validPayload,
			},
			wantHasMarker: true,
			wantErr:       false,
		},
		{
			name:          "no marker - legacy encrypted value",
			value:         "some-legacy-encrypted-base64-data",
			wantMarker:    EnvelopeMarker{},
			wantHasMarker: false,
			wantErr:       false,
		},
		{
			name:          "empty string - legacy candidate",
			value:         "",
			wantMarker:    EnvelopeMarker{},
			wantHasMarker: false,
			wantErr:       false,
		},
		{
			name:          "malformed - missing key ID",
			value:         "tink:v:" + validPayloadB64,
			wantMarker:    EnvelopeMarker{},
			wantHasMarker: false,
			wantErr:       true,
			errContains:   "invalid key ID",
		},
		{
			name:          "malformed - non-numeric key ID",
			value:         "tink:vabc:" + validPayloadB64,
			wantMarker:    EnvelopeMarker{},
			wantHasMarker: false,
			wantErr:       true,
			errContains:   "invalid key ID",
		},
		{
			name:          "malformed - negative key ID",
			value:         "tink:v-1:" + validPayloadB64,
			wantMarker:    EnvelopeMarker{},
			wantHasMarker: false,
			wantErr:       true,
			errContains:   "invalid key ID",
		},
		{
			name:          "malformed - key ID exceeds uint32",
			value:         "tink:v4294967296:" + validPayloadB64,
			wantMarker:    EnvelopeMarker{},
			wantHasMarker: false,
			wantErr:       true,
			errContains:   "invalid key ID",
		},
		{
			name:          "malformed - missing payload separator",
			value:         "tink:v1",
			wantMarker:    EnvelopeMarker{},
			wantHasMarker: false,
			wantErr:       true,
			errContains:   "invalid envelope marker format",
		},
		{
			name:          "malformed - invalid base64 payload",
			value:         "tink:v1:not-valid-base64!!!",
			wantMarker:    EnvelopeMarker{},
			wantHasMarker: false,
			wantErr:       true,
			errContains:   "invalid payload encoding",
		},
		{
			name:  "valid marker with empty payload",
			value: "tink:v1:",
			wantMarker: EnvelopeMarker{
				KeyID:   1,
				Payload: []byte{},
			},
			wantHasMarker: true,
			wantErr:       false,
		},
		{
			name:          "prefix only - incomplete marker",
			value:         "tink:v",
			wantMarker:    EnvelopeMarker{},
			wantHasMarker: false,
			wantErr:       true,
			errContains:   "invalid envelope marker format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			marker, hasMarker, err := ParseEnvelopeMarker(tt.value)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseEnvelopeMarker() expected error containing %q, got nil", tt.errContains)
					return
				}

				if !containsString(err.Error(), tt.errContains) {
					t.Errorf("ParseEnvelopeMarker() error = %q, want error containing %q", err.Error(), tt.errContains)
				}

				return
			}

			if err != nil {
				t.Errorf("ParseEnvelopeMarker() unexpected error = %v", err)
				return
			}

			if hasMarker != tt.wantHasMarker {
				t.Errorf("ParseEnvelopeMarker() hasMarker = %v, want %v", hasMarker, tt.wantHasMarker)
			}

			if hasMarker {
				if marker.KeyID != tt.wantMarker.KeyID {
					t.Errorf("ParseEnvelopeMarker() KeyID = %d, want %d", marker.KeyID, tt.wantMarker.KeyID)
				}

				if string(marker.Payload) != string(tt.wantMarker.Payload) {
					t.Errorf("ParseEnvelopeMarker() Payload = %q, want %q", string(marker.Payload), string(tt.wantMarker.Payload))
				}
			}
		})
	}
}

func TestFormatEnvelopeMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		keyID    uint32
		payload  []byte
		expected string
	}{
		{
			name:     "standard key ID and payload",
			keyID:    1,
			payload:  []byte("encrypted-data"),
			expected: "tink:v1:" + base64.URLEncoding.EncodeToString([]byte("encrypted-data")),
		},
		{
			name:     "large key ID",
			keyID:    4294967295,
			payload:  []byte("data"),
			expected: "tink:v4294967295:" + base64.URLEncoding.EncodeToString([]byte("data")),
		},
		{
			name:     "empty payload",
			keyID:    42,
			payload:  []byte{},
			expected: "tink:v42:",
		},
		{
			name:     "zero key ID",
			keyID:    0,
			payload:  []byte("test"),
			expected: "tink:v0:" + base64.URLEncoding.EncodeToString([]byte("test")),
		},
		{
			name:     "binary payload",
			keyID:    123,
			payload:  []byte{0x00, 0x01, 0x02, 0xFF, 0xFE},
			expected: "tink:v123:" + base64.URLEncoding.EncodeToString([]byte{0x00, 0x01, 0x02, 0xFF, 0xFE}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := FormatEnvelopeMarker(tt.keyID, tt.payload)

			if got != tt.expected {
				t.Errorf("FormatEnvelopeMarker(%d, %q) = %q, want %q", tt.keyID, string(tt.payload), got, tt.expected)
			}
		})
	}
}

func TestFormatEnvelopeMarker_Roundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		keyID   uint32
		payload []byte
	}{
		{
			name:    "standard roundtrip",
			keyID:   12345,
			payload: []byte("some encrypted data here"),
		},
		{
			name:    "max key ID roundtrip",
			keyID:   4294967295,
			payload: []byte("max key test"),
		},
		{
			name:    "binary data roundtrip",
			keyID:   99,
			payload: []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
		},
		{
			name:    "empty payload roundtrip",
			keyID:   1,
			payload: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Format the marker
			formatted := FormatEnvelopeMarker(tt.keyID, tt.payload)

			// Parse it back
			marker, hasMarker, err := ParseEnvelopeMarker(formatted)
			if err != nil {
				t.Errorf("Roundtrip failed: ParseEnvelopeMarker() error = %v", err)
				return
			}

			if !hasMarker {
				t.Errorf("Roundtrip failed: ParseEnvelopeMarker() hasMarker = false, want true")
				return
			}

			if marker.KeyID != tt.keyID {
				t.Errorf("Roundtrip failed: KeyID = %d, want %d", marker.KeyID, tt.keyID)
			}

			if string(marker.Payload) != string(tt.payload) {
				t.Errorf("Roundtrip failed: Payload = %q, want %q", string(marker.Payload), string(tt.payload))
			}
		})
	}
}

func TestHasEnvelopeMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{
			name:     "valid marker",
			value:    "tink:v1:somedata",
			expected: true,
		},
		{
			name:     "prefix only",
			value:    "tink:v",
			expected: true,
		},
		{
			name:     "prefix with key ID",
			value:    "tink:v123",
			expected: true,
		},
		{
			name:     "legacy value - no prefix",
			value:    "some-legacy-data",
			expected: false,
		},
		{
			name:     "empty string",
			value:    "",
			expected: false,
		},
		{
			name:     "partial prefix - tink only",
			value:    "tink:",
			expected: false,
		},
		{
			name:     "partial prefix - tink:x",
			value:    "tink:x",
			expected: false,
		},
		{
			name:     "similar but different prefix - tink:version starts with tink:v",
			value:    "tink:version1:data",
			expected: true, // Note: "tink:version1" starts with "tink:v", so this is detected as a marker
		},
		{
			name:     "case sensitive - uppercase",
			value:    "TINK:V1:data",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := HasEnvelopeMarker(tt.value)

			if got != tt.expected {
				t.Errorf("HasEnvelopeMarker(%q) = %v, want %v", tt.value, got, tt.expected)
			}
		})
	}
}
