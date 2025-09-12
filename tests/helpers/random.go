package helpers

import (
    crand "crypto/rand"
    "encoding/hex"
    "math/rand"
    "time"
)

// Seed math/rand once per test run.
func init() { rand.Seed(time.Now().UnixNano()) }

func RandString(n int) string {
    letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
    b := make([]rune, n)
    for i := range b {
        b[i] = letters[rand.Intn(len(letters))]
    }
    return string(b)
}

func RandHex(n int) string {
    b := make([]byte, n)
    _, _ = crand.Read(b)
    return hex.EncodeToString(b)
}

