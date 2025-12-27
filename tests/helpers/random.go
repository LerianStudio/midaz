package helpers

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

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

// RandHex generates a random hex string from n random bytes.
func RandHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		//nolint:panicguardwarn // Test helper: panic is acceptable for fatal setup errors
		panic("failed to read random bytes: " + err.Error())
	}

	return hex.EncodeToString(b)
}
