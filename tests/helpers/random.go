package helpers

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"

	"github.com/google/uuid"
)

var (
	letters     = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	lettersOnly = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
)

// RandString generates a random alphanumeric string of length n.
func RandString(n int) string {
	b := make([]rune, n)

	lettersLen := big.NewInt(int64(len(letters)))
	for i := range b {
		// Using crypto/rand for secure random number generation
		idx, err := rand.Int(rand.Reader, lettersLen)
		if err != nil {
			//nolint:panicguardwarn // Test helper: panic is acceptable for fatal setup errors
			panic("failed to generate random index: " + err.Error())
		}

		b[i] = letters[idx.Int64()]
	}

	return string(b)
}

// RandLetters generates a random string of letters only (no digits) of length n.
// Use this for asset codes which require letters-only uppercase strings.
func RandLetters(n int) string {
	b := make([]rune, n)

	lettersLen := big.NewInt(int64(len(lettersOnly)))
	for i := range b {
		idx, err := rand.Int(rand.Reader, lettersLen)
		if err != nil {
			//nolint:panicguardwarn // Test helper: panic is acceptable for fatal setup errors
			panic("failed to generate random index: " + err.Error())
		}

		b[i] = lettersOnly[idx.Int64()]
	}

	return string(b)
}

// RandHex generates a random hex string from n random bytes.
func RandHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		//nolint:panicguardwarn // Test helper: panic is acceptable for fatal setup errors
		panic("failed to read random bytes: " + err.Error())
	}

	return hex.EncodeToString(b)
}

// RandIntN generates a random int in [0, n) using crypto/rand.
func RandIntN(n int) int {
	if n <= 0 {
		return 0
	}

	upperBound := big.NewInt(int64(n))

	val, err := rand.Int(rand.Reader, upperBound)
	if err != nil {
		//nolint:panicguardwarn // Test helper: panic is acceptable for fatal setup errors
		panic("failed to generate random int: " + err.Error())
	}

	return int(val.Int64())
}

// RandUUID generates a random UUID v4 string.
func RandUUID() string {
	return uuid.New().String()
}
