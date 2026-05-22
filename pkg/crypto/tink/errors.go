// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"errors"
	"strings"
)

// Sentinel errors for keyset operations.
var (
	// ErrKeysetNotFound indicates the requested keyset does not exist.
	ErrKeysetNotFound = errors.New("keyset not found")
	// ErrKeysetCorrupted indicates the keyset data is invalid or corrupted.
	ErrKeysetCorrupted = errors.New("keyset corrupted")
	// ErrDecryptionFailed indicates the decryption operation failed.
	ErrDecryptionFailed = errors.New("decryption failed")
	// ErrEncryptionFailed indicates the encryption operation failed.
	ErrEncryptionFailed = errors.New("encryption failed")
	// ErrMACComputationFailed indicates the MAC computation failed.
	ErrMACComputationFailed = errors.New("mac computation failed")
	// ErrMACVerificationFailed indicates the MAC verification failed.
	ErrMACVerificationFailed = errors.New("mac verification failed")
	// ErrKMSUnavailable indicates the KMS service is not available.
	ErrKMSUnavailable = errors.New("kms unavailable")
	// ErrInvalidKeyName indicates the key name format is invalid.
	ErrInvalidKeyName = errors.New("invalid key name")
)

// ErrorCategory classifies errors for safe handling and logging.
type ErrorCategory int

const (
	// ErrorCategoryUnknown indicates an unclassified error.
	ErrorCategoryUnknown ErrorCategory = iota
	// ErrorCategoryConfiguration indicates a configuration error.
	ErrorCategoryConfiguration
	// ErrorCategoryKMS indicates a KMS-related error.
	ErrorCategoryKMS
	// ErrorCategoryCrypto indicates a cryptographic operation error.
	ErrorCategoryCrypto
	// ErrorCategoryInput indicates invalid input data.
	ErrorCategoryInput
)

// String returns the string representation of the error category.
func (c ErrorCategory) String() string {
	switch c {
	case ErrorCategoryConfiguration:
		return "configuration"
	case ErrorCategoryKMS:
		return "kms"
	case ErrorCategoryCrypto:
		return "crypto"
	case ErrorCategoryInput:
		return "input"
	default:
		return "unknown"
	}
}

// ClassifyError categorizes an error for safe handling.
// This helps determine appropriate logging and user messaging.
func ClassifyError(err error) ErrorCategory {
	if err == nil {
		return ErrorCategoryUnknown
	}

	// Check sentinel errors first
	if category := classifyBySentinel(err); category != ErrorCategoryUnknown {
		return category
	}

	// Fall back to string matching
	return classifyByString(err.Error())
}

// classifyBySentinel checks if the error matches known sentinel errors.
func classifyBySentinel(err error) ErrorCategory {
	kmsErrors := []error{ErrKMSUnavailable}
	cryptoErrors := []error{ErrDecryptionFailed, ErrEncryptionFailed, ErrMACComputationFailed, ErrMACVerificationFailed, ErrKeysetCorrupted}
	inputErrors := []error{ErrInvalidKeyName}

	for _, sentinel := range kmsErrors {
		if errors.Is(err, sentinel) {
			return ErrorCategoryKMS
		}
	}

	for _, sentinel := range cryptoErrors {
		if errors.Is(err, sentinel) {
			return ErrorCategoryCrypto
		}
	}

	for _, sentinel := range inputErrors {
		if errors.Is(err, sentinel) {
			return ErrorCategoryInput
		}
	}

	return ErrorCategoryUnknown
}

// classifyByString categorizes errors based on string patterns.
func classifyByString(errStr string) ErrorCategory {
	kmsPatterns := []string{"kms", "vault", "transit"}
	cryptoPatterns := []string{"decrypt", "encrypt", "mac", "keyset"}
	inputPatterns := []string{"invalid", "empty"}
	configPatterns := []string{"config", "missing"}

	if containsAny(errStr, kmsPatterns) {
		return ErrorCategoryKMS
	}

	if containsAny(errStr, cryptoPatterns) {
		return ErrorCategoryCrypto
	}

	if containsAny(errStr, inputPatterns) {
		return ErrorCategoryInput
	}

	if containsAny(errStr, configPatterns) {
		return ErrorCategoryConfiguration
	}

	return ErrorCategoryUnknown
}

// containsAny returns true if s contains any of the patterns.
func containsAny(s string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(s, pattern) {
			return true
		}
	}

	return false
}

// IsSafeToLog returns true if the error message is safe to log.
// Some errors may contain sensitive information that should not be logged.
func IsSafeToLog(err error) bool {
	if err == nil {
		return true
	}

	errStr := strings.ToLower(err.Error())

	// Patterns that may indicate sensitive data in error messages
	sensitivePatterns := []string{
		"key material",
		"secret",
		"token",
		"password",
		"credential",
		"plaintext",
	}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(errStr, pattern) {
			return false
		}
	}

	return true
}

// SanitizeError returns a safe error message for external consumption.
// Internal details are replaced with generic messages.
func SanitizeError(err error) string {
	if err == nil {
		return ""
	}

	category := ClassifyError(err)

	switch category {
	case ErrorCategoryKMS:
		return "key management service error"
	case ErrorCategoryCrypto:
		return "cryptographic operation failed"
	case ErrorCategoryInput:
		return "invalid input"
	case ErrorCategoryConfiguration:
		return "configuration error"
	default:
		return "internal error"
	}
}
