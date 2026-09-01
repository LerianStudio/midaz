// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package crypto selects the CRM field-encryption strategy from configuration.
// KMS_VENDOR drives the EncryptionMode: unset or "none" yields legacy (local
// symmetric keys, no external KMS), "hashicorp-vault" yields envelope (per-org
// Tink data keys wrapped by a KMS-managed key). The subpackages supply the
// primitives — tink for AEAD/PRF keysets and KMS wrapping, kms/vault for the
// Transit key-encryption client.
package crypto

// Environment variable constants for KMS configuration.
const (
	// EnvKMSVendor is the environment variable that specifies which KMS vendor to use.
	EnvKMSVendor = "KMS_VENDOR"
)

// Vendor constants define supported KMS vendors.
const (
	// VendorNone indicates no external KMS vendor (legacy mode).
	VendorNone = "none"

	// VendorHashicorpVault indicates HashiCorp Vault as the KMS vendor.
	VendorHashicorpVault = "hashicorp-vault"
)

// EncryptionMode represents the encryption key protection strategy.
type EncryptionMode int

const (
	// EncryptionModeLegacy indicates keys are stored locally without external KMS.
	EncryptionModeLegacy EncryptionMode = iota

	// EncryptionModeEnvelope indicates envelope encryption with KMS-managed key wrapping.
	// DEKs (Data Encryption Keys) are wrapped by KEKs (Key Encryption Keys) in the KMS.
	EncryptionModeEnvelope
)

// String returns the string representation of the EncryptionMode.
func (m EncryptionMode) String() string {
	switch m {
	case EncryptionModeLegacy:
		return "legacy"
	case EncryptionModeEnvelope:
		return "envelope"
	default:
		return ""
	}
}

// IsLegacy returns true if the mode is EncryptionModeLegacy.
func (m EncryptionMode) IsLegacy() bool {
	return m == EncryptionModeLegacy
}

// IsEnvelope returns true if the mode is EncryptionModeEnvelope.
func (m EncryptionMode) IsEnvelope() bool {
	return m == EncryptionModeEnvelope
}
