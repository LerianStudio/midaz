// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// verifyHMAC computes HMAC-SHA256 over data using the provided key and compares
// it to receivedHMAC in constant time. Returns true if the signatures match,
// false otherwise. Callers enforce the result (see verifyHMACOrReject, D7).
func verifyHMAC(data []byte, receivedHMAC string, hmacKey []byte) bool {
	if len(data) == 0 || receivedHMAC == "" || len(hmacKey) == 0 {
		return false
	}

	mac := hmac.New(sha256.New, hmacKey)
	mac.Write(data)
	expectedMAC := mac.Sum(nil)

	expectedHex := hex.EncodeToString(expectedMAC)

	return hmac.Equal([]byte(expectedHex), []byte(receivedHMAC))
}
