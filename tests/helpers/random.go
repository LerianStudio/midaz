package helpers

import (
    crand "crypto/rand"
    "encoding/hex"
    "math/big"
)

func RandString(n int) string {
    letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
    b := make([]rune, n)
    max := big.NewInt(int64(len(letters)))
    for i := range b {
        idx, _ := crand.Int(crand.Reader, max)
        b[i] = letters[idx.Int64()]
    }
    return string(b)
}

func RandHex(n int) string {
    b := make([]byte, n)
    _, _ = crand.Read(b)
    return hex.EncodeToString(b)
}

