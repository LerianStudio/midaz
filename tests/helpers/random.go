package helpers

import (
    crand "crypto/rand"
    "encoding/hex"
    "math/rand"
)

// globalRand provides a properly seeded random source for test data generation.
// As of Go 1.20, the global rand functions are automatically seeded.
var globalRand = rand.New(rand.NewSource(rand.Int63()))

func RandString(n int) string {
    letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
    b := make([]rune, n)
    for i := range b {
        b[i] = letters[globalRand.Intn(len(letters))]
    }
    return string(b)
}

func RandHex(n int) string {
    b := make([]byte, n)
    _, _ = crand.Read(b)
    return hex.EncodeToString(b)
}

