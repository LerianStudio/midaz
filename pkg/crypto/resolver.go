// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package crypto

import (
	"fmt"
	"os"
	"strings"
)

// EnvLookupFunc is a function type for looking up environment variables.
// It returns the value and a boolean indicating if the variable was set.
type EnvLookupFunc func(key string) (string, bool)

// ModeResolver resolves the encryption mode from environment configuration.
type ModeResolver struct {
	lookupEnv EnvLookupFunc
}

// NewModeResolver creates a new ModeResolver with the given environment lookup function.
// If lookupEnv is nil, it defaults to os.LookupEnv.
func NewModeResolver(lookupEnv EnvLookupFunc) *ModeResolver {
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}

	return &ModeResolver{
		lookupEnv: lookupEnv,
	}
}

// NewModeResolverFromEnv creates a new ModeResolver that uses os.LookupEnv.
func NewModeResolverFromEnv() *ModeResolver {
	return NewModeResolver(os.LookupEnv)
}

// Resolve determines the encryption mode based on the KMS_VENDOR environment variable.
// Returns:
//   - EncryptionModeLegacy when env var is missing, empty, or set to "none"
//   - EncryptionModeEnvelope when env var is set to "hashicorp-vault"
//   - Error when env var contains an unsupported vendor value
func (r *ModeResolver) Resolve() (EncryptionMode, error) {
	vendor := r.GetVendor()

	switch vendor {
	case "", VendorNone:
		return EncryptionModeLegacy, nil
	case VendorHashicorpVault:
		return EncryptionModeEnvelope, nil
	default:
		return EncryptionModeLegacy, fmt.Errorf(
			"unsupported KMS vendor %q: supported vendors are %q, %q",
			vendor,
			VendorNone,
			VendorHashicorpVault,
		)
	}
}

// GetVendor returns the normalized vendor string from the environment.
// Returns empty string if the environment variable is not set.
// The vendor is normalized to lowercase with whitespace trimmed.
func (r *ModeResolver) GetVendor() string {
	value, ok := r.lookupEnv(EnvKMSVendor)
	if !ok {
		return ""
	}

	return strings.ToLower(strings.TrimSpace(value))
}
