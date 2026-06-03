// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
)

func RandString(n int) string {
	if n <= 0 {
		n = 8
	}

	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	out := make([]byte, n)

	upperBound := big.NewInt(int64(len(letters)))
	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, upperBound)
		if err != nil {
			panic("failed to generate random number: " + err.Error())
		}

		out[i] = letters[num.Int64()]
	}

	return string(out)
}

func RandHex(n int) string {
	if n <= 0 {
		n = 8
	}

	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("failed to read random bytes: " + err.Error())
	}

	return hex.EncodeToString(b)
}
