// Package helpers provides reusable utilities and setup functions to streamline
// integration and end-to-end tests.
// This file contains random data generation utilities for test data.
package helpers

import (
	crand "crypto/rand"
	"encoding/hex"
	"math/rand"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

// RandString generates a random alphanumeric string of a given length.
func RandString(n int) string {
	b := make([]rune, n)
	for i := range b {
		// Using global rand functions which are thread-safe as of Go 1.20
		b[i] = letters[rand.Intn(len(letters))]
	}

	return string(b)
}

// RandHex generates a random hexadecimal string of a given length.
func RandHex(n int) string {
	b := make([]byte, n)
	if _, err := crand.Read(b); err != nil {
		panic("failed to read random bytes: " + err.Error())
	}

	return hex.EncodeToString(b)
}
