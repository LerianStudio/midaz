package helpers

import (
	crand "crypto/rand"
	"encoding/hex"
	"math/rand"
)

func RandString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	b := make([]rune, n)
	for i := range b {
		// Using global rand functions which are thread-safe as of Go 1.20
		b[i] = letters[rand.Intn(len(letters))]
	}

	return string(b)
}

func RandHex(n int) string {
	b := make([]byte, n)
	_, _ = crand.Read(b)

	return hex.EncodeToString(b)
}
