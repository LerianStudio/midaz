// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

// computeHMACForTest computes the HMAC-SHA256 hex digest for test data, matching
// the algorithm used by verifyHMAC.
func computeHMACForTest(t *testing.T, data, key []byte) string {
	t.Helper()

	mac := hmac.New(sha256.New, key)
	mac.Write(data)

	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyHMAC(t *testing.T) {
	t.Parallel()

	validKey := []byte("test-hmac-secret-key")
	validData := []byte(`{"accounts": [{"id": 1}]}`)
	validHMAC := computeHMACForTest(t, validData, validKey)

	tests := []struct {
		name         string
		data         []byte
		receivedHMAC string
		hmacKey      []byte
		expected     bool
	}{
		{
			name:         "Success - valid HMAC matches",
			data:         validData,
			receivedHMAC: validHMAC,
			hmacKey:      validKey,
			expected:     true,
		},
		{
			name:         "Success - different data produces valid HMAC",
			data:         []byte(`{"org": "lerian"}`),
			receivedHMAC: computeHMACForTest(t, []byte(`{"org": "lerian"}`), validKey),
			hmacKey:      validKey,
			expected:     true,
		},
		{
			name:         "Mismatch - wrong HMAC value",
			data:         validData,
			receivedHMAC: "deadbeef0123456789abcdef0123456789abcdef0123456789abcdef01234567",
			hmacKey:      validKey,
			expected:     false,
		},
		{
			name:         "Mismatch - wrong key",
			data:         validData,
			receivedHMAC: validHMAC,
			hmacKey:      []byte("wrong-key"),
			expected:     false,
		},
		{
			name:         "Mismatch - tampered data",
			data:         []byte(`{"accounts": [{"id": 2}]}`),
			receivedHMAC: validHMAC,
			hmacKey:      validKey,
			expected:     false,
		},
		{
			name:         "False - empty data",
			data:         []byte{},
			receivedHMAC: validHMAC,
			hmacKey:      validKey,
			expected:     false,
		},
		{
			name:         "False - nil data",
			data:         nil,
			receivedHMAC: validHMAC,
			hmacKey:      validKey,
			expected:     false,
		},
		{
			name:         "False - empty HMAC string",
			data:         validData,
			receivedHMAC: "",
			hmacKey:      validKey,
			expected:     false,
		},
		{
			name:         "False - empty key",
			data:         validData,
			receivedHMAC: validHMAC,
			hmacKey:      []byte{},
			expected:     false,
		},
		{
			name:         "False - nil key",
			data:         validData,
			receivedHMAC: validHMAC,
			hmacKey:      nil,
			expected:     false,
		},
		{
			name:         "False - all inputs empty",
			data:         nil,
			receivedHMAC: "",
			hmacKey:      nil,
			expected:     false,
		},
		{
			name:         "Mismatch - HMAC with wrong case (uppercase)",
			data:         validData,
			receivedHMAC: "AAAA" + validHMAC[4:],
			hmacKey:      validKey,
			expected:     false,
		},
		{
			name:         "Mismatch - truncated HMAC",
			data:         validData,
			receivedHMAC: validHMAC[:32],
			hmacKey:      validKey,
			expected:     false,
		},
		{
			name:         "Mismatch - HMAC with extra bytes appended",
			data:         validData,
			receivedHMAC: validHMAC + "extra",
			hmacKey:      validKey,
			expected:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := verifyHMAC(tt.data, tt.receivedHMAC, tt.hmacKey)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVerifyHMAC_DifferentKeys_ProduceDifferentResults(t *testing.T) {
	t.Parallel()

	data := []byte(`{"report_id": "abc-123"}`)
	key1 := []byte("key-one")
	key2 := []byte("key-two")

	hmac1 := computeHMACForTest(t, data, key1)
	hmac2 := computeHMACForTest(t, data, key2)

	// Different keys produce different HMACs
	assert.NotEqual(t, hmac1, hmac2)

	// Each HMAC only validates with its own key
	assert.True(t, verifyHMAC(data, hmac1, key1))
	assert.False(t, verifyHMAC(data, hmac1, key2))
	assert.True(t, verifyHMAC(data, hmac2, key2))
	assert.False(t, verifyHMAC(data, hmac2, key1))
}
