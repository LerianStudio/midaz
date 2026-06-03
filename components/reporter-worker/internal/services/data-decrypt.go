// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg"
)

// decryptFetcherData decrypts AES-GCM encrypted data received from the Fetcher service.
// The storageKey is the HKDF-derived key from APP_ENC_KEY with context "fetcher-storage-encryption-v1".
//
// The encryptedData parameter is expected to be base64-encoded ciphertext where the
// first 12 bytes are the GCM nonce and the remainder is the ciphertext with authentication tag.
//
// Returns the decrypted JSON payload or an error if decryption fails.
func decryptFetcherData(encryptedData []byte, storageKey []byte) ([]byte, error) {
	if len(encryptedData) == 0 {
		return nil, pkg.FailedPreconditionError{Code: "REP-0073", Title: "Empty Encrypted Data", Message: "encrypted data is empty"}
	}

	if len(storageKey) == 0 {
		return nil, pkg.FailedPreconditionError{Code: "REP-0074", Title: "Decryption Key Not Configured", Message: "storage decryption key not configured"}
	}

	// Decode the base64-encoded ciphertext
	ciphertext, err := base64.StdEncoding.DecodeString(string(encryptedData))
	if err != nil {
		return nil, pkg.FailedPreconditionError{Code: "REP-0075", Title: "Invalid Encrypted Data", Message: fmt.Sprintf("base64 decode encrypted data: %s", err.Error()), Err: err}
	}

	block, err := aes.NewCipher(storageKey)
	if err != nil {
		return nil, pkg.FailedPreconditionError{Code: "REP-0076", Title: "Cipher Creation Failed", Message: fmt.Sprintf("create AES cipher: %s", err.Error()), Err: err}
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, pkg.FailedPreconditionError{Code: "REP-0077", Title: "Cipher Creation Failed", Message: fmt.Sprintf("create GCM: %s", err.Error()), Err: err}
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, pkg.FailedPreconditionError{Code: "REP-0078", Title: "Corrupt Encrypted Data", Message: fmt.Sprintf("ciphertext too short: expected at least %d bytes for nonce, got %d", nonceSize, len(ciphertext))}
	}

	// Split nonce and ciphertext
	nonce := ciphertext[:nonceSize]
	encryptedPayload := ciphertext[nonceSize:]

	plaintext, err := aesGCM.Open(nil, nonce, encryptedPayload, nil)
	if err != nil {
		return nil, pkg.FailedPreconditionError{Code: "REP-0079", Title: "Decryption Failed", Message: fmt.Sprintf("AES-GCM decrypt: %s", err.Error()), Err: err}
	}

	return plaintext, nil
}
