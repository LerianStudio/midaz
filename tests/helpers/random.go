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

func RandString(n int) string {
	b := make([]rune, n)
	for i := range b {
		// Using global rand functions which are thread-safe as of Go 1.20
		b[i] = letters[rand.Intn(len(letters))]
	}

	return string(b)
}

func RandHex(n int) string {
	b := make([]byte, n)
	if _, err := crand.Read(b); err != nil {
		panic("failed to read random bytes: " + err.Error())
	}

	return hex.EncodeToString(b)
}
