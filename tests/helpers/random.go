// Package helpers provides random value generation utilities for Midaz integration tests.
//
// # Purpose
//
// This file provides utilities for generating random strings and hex values
// for use in test data. These are useful for:
//   - Generating unique identifiers
//   - Creating random names for test entities
//   - Producing random tokens or keys
//
// # Functions
//
//   - RandString: Alphanumeric string using math/rand (fast, not cryptographic)
//   - RandHex: Hexadecimal string using crypto/rand (cryptographically secure)
//
// # Thread Safety
//
// Both functions are thread-safe:
//   - RandString uses global rand functions (thread-safe as of Go 1.20)
//   - RandHex uses crypto/rand which is thread-safe
//
// # Security Note
//
// RandString uses math/rand and should NOT be used for security-sensitive
// values like tokens or passwords. Use RandHex for those purposes.
package helpers

import (
	crand "crypto/rand"
	"encoding/hex"
	"math/rand"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

// RandString generates a random alphanumeric string of length n.
//
// This function uses math/rand which is fast but NOT cryptographically secure.
// Use RandHex for security-sensitive values.
//
// # Parameters
//
//   - n: Length of string to generate
//
// # Returns
//
//   - string: Random alphanumeric string of length n
//
// # Character Set
//
// Uses: a-z, A-Z, 0-9 (62 characters)
//
// # Example
//
//	name := "test_" + RandString(8) // e.g., "test_Kj2mX9pL"
func RandString(n int) string {
	b := make([]rune, n)
	for i := range b {
		// Using global rand functions which are thread-safe as of Go 1.20
		b[i] = letters[rand.Intn(len(letters))]
	}

	return string(b)
}

// RandHex generates a cryptographically secure random hex string.
//
// This function uses crypto/rand and is suitable for security-sensitive
// values like tokens, keys, or identifiers that must be unpredictable.
//
// # Parameters
//
//   - n: Number of random bytes (output string will be 2n characters)
//
// # Returns
//
//   - string: Hex-encoded random string (lowercase)
//
// # Panics
//
// Panics if crypto/rand fails to read random bytes. This should only happen
// in exceptional circumstances (system entropy exhaustion).
//
// # Example
//
//	token := RandHex(16) // e.g., "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6" (32 chars)
func RandHex(n int) string {
	b := make([]byte, n)
	if _, err := crand.Read(b); err != nil {
		panic("failed to read random bytes: " + err.Error())
	}

	return hex.EncodeToString(b)
}
