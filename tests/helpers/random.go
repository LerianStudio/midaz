// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package helpers

import (
	crand "crypto/rand"
	"encoding/hex"
	"math/rand"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

// RandString returns a random alphanumeric string of length n for use in test fixtures.
func RandString(n int) string {
	b := make([]rune, n)
	for i := range b {
		// Using global rand functions which are thread-safe as of Go 1.20
		b[i] = letters[rand.Intn(len(letters))] //nolint:gosec // G404: math/rand acceptable for non-cryptographic test data
	}

	return string(b)
}

// RandHex returns a cryptographically random hex string of n bytes (2n hex chars).
// Returns an empty string if the random source fails.
func RandHex(n int) string {
	b := make([]byte, n)
	if _, err := crand.Read(b); err != nil {
		// crand.Read only fails on catastrophic system entropy failure;
		// returning empty string allows callers to detect the failure.
		return ""
	}

	return hex.EncodeToString(b)
}
