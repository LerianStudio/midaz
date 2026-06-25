// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModeResolver_Resolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		envValue      string
		envSet        bool
		expectedMode  EncryptionMode
		expectError   bool
		errorContains string
	}{
		{
			name:         "missing env var defaults to legacy mode",
			envValue:     "",
			envSet:       false,
			expectedMode: EncryptionModeLegacy,
			expectError:  false,
		},
		{
			name:         "empty string defaults to legacy mode",
			envValue:     "",
			envSet:       true,
			expectedMode: EncryptionModeLegacy,
			expectError:  false,
		},
		{
			name:         "none vendor maps to legacy mode",
			envValue:     "none",
			envSet:       true,
			expectedMode: EncryptionModeLegacy,
			expectError:  false,
		},
		{
			name:         "hashicorp-vault vendor maps to envelope mode",
			envValue:     "hashicorp-vault",
			envSet:       true,
			expectedMode: EncryptionModeEnvelope,
			expectError:  false,
		},
		{
			name:          "invalid vendor returns error",
			envValue:      "invalid-vendor",
			envSet:        true,
			expectedMode:  EncryptionModeLegacy,
			expectError:   true,
			errorContains: "unsupported KMS vendor",
		},
		{
			name:          "unknown vendor returns error",
			envValue:      "aws-kms",
			envSet:        true,
			expectedMode:  EncryptionModeLegacy,
			expectError:   true,
			errorContains: "unsupported KMS vendor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create resolver with test environment getter
			resolver := NewModeResolver(func(key string) (string, bool) {
				if key == EnvKMSVendor && tt.envSet {
					return tt.envValue, true
				}
				return "", false
			})

			mode, err := resolver.Resolve()

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedMode, mode)
			}
		})
	}
}

func TestModeResolver_ResolveWithOSEnv(t *testing.T) {
	t.Parallel()

	t.Run("resolver created with nil getter uses os.LookupEnv", func(t *testing.T) {
		t.Parallel()

		// NewModeResolverFromEnv should create resolver that uses os.LookupEnv
		resolver := NewModeResolverFromEnv()
		require.NotNil(t, resolver)

		// Since we cannot control os env in parallel tests, just verify it returns without panic
		_, _ = resolver.Resolve()
	})
}

func TestNewModeResolver_NilGuard(t *testing.T) {
	t.Parallel()

	t.Run("nil lookupEnv defaults to os.LookupEnv", func(t *testing.T) {
		t.Parallel()

		// NewModeResolver with nil should not panic and should default to os.LookupEnv
		resolver := NewModeResolver(nil)
		require.NotNil(t, resolver)

		// Should not panic when resolving - defaults to os.LookupEnv
		mode, err := resolver.Resolve()

		// Since env var is not set in test environment, should default to legacy mode
		require.NoError(t, err)
		assert.Equal(t, EncryptionModeLegacy, mode)
	})

	t.Run("nil lookupEnv GetVendor does not panic", func(t *testing.T) {
		t.Parallel()

		resolver := NewModeResolver(nil)
		require.NotNil(t, resolver)

		// Should not panic
		vendor := resolver.GetVendor()

		// Since env var is not set in test environment, should return empty
		assert.Empty(t, vendor)
	})
}

func TestModeResolver_CaseInsensitivity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		envValue     string
		expectedMode EncryptionMode
		expectError  bool
	}{
		{
			name:         "NONE uppercase maps to legacy",
			envValue:     "NONE",
			expectedMode: EncryptionModeLegacy,
			expectError:  false,
		},
		{
			name:         "None mixed case maps to legacy",
			envValue:     "None",
			expectedMode: EncryptionModeLegacy,
			expectError:  false,
		},
		{
			name:         "HASHICORP-VAULT uppercase maps to envelope",
			envValue:     "HASHICORP-VAULT",
			expectedMode: EncryptionModeEnvelope,
			expectError:  false,
		},
		{
			name:         "Hashicorp-Vault mixed case maps to envelope",
			envValue:     "Hashicorp-Vault",
			expectedMode: EncryptionModeEnvelope,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := NewModeResolver(func(key string) (string, bool) {
				if key == EnvKMSVendor {
					return tt.envValue, true
				}
				return "", false
			})

			mode, err := resolver.Resolve()

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedMode, mode)
			}
		})
	}
}

func TestModeResolver_WhitespaceHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		envValue     string
		expectedMode EncryptionMode
		expectError  bool
	}{
		{
			name:         "leading whitespace is trimmed",
			envValue:     "  none",
			expectedMode: EncryptionModeLegacy,
			expectError:  false,
		},
		{
			name:         "trailing whitespace is trimmed",
			envValue:     "none  ",
			expectedMode: EncryptionModeLegacy,
			expectError:  false,
		},
		{
			name:         "surrounding whitespace is trimmed",
			envValue:     "  hashicorp-vault  ",
			expectedMode: EncryptionModeEnvelope,
			expectError:  false,
		},
		{
			name:         "only whitespace defaults to legacy",
			envValue:     "   ",
			expectedMode: EncryptionModeLegacy,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := NewModeResolver(func(key string) (string, bool) {
				if key == EnvKMSVendor {
					return tt.envValue, true
				}
				return "", false
			})

			mode, err := resolver.Resolve()

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedMode, mode)
			}
		})
	}
}

func TestModeResolver_ErrorDetails(t *testing.T) {
	t.Parallel()

	t.Run("error includes invalid vendor value", func(t *testing.T) {
		t.Parallel()

		invalidVendor := "my-custom-kms"
		resolver := NewModeResolver(func(key string) (string, bool) {
			if key == EnvKMSVendor {
				return invalidVendor, true
			}
			return "", false
		})

		_, err := resolver.Resolve()

		require.Error(t, err)
		assert.Contains(t, err.Error(), invalidVendor)
	})

	t.Run("error includes supported vendors", func(t *testing.T) {
		t.Parallel()

		resolver := NewModeResolver(func(key string) (string, bool) {
			if key == EnvKMSVendor {
				return "unsupported", true
			}
			return "", false
		})

		_, err := resolver.Resolve()

		require.Error(t, err)
		assert.Contains(t, err.Error(), VendorNone)
		assert.Contains(t, err.Error(), VendorHashicorpVault)
	})
}

func TestModeResolver_GetVendor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		envValue       string
		envSet         bool
		expectedVendor string
	}{
		{
			name:           "returns vendor when set",
			envValue:       "hashicorp-vault",
			envSet:         true,
			expectedVendor: "hashicorp-vault",
		},
		{
			name:           "returns empty when not set",
			envValue:       "",
			envSet:         false,
			expectedVendor: "",
		},
		{
			name:           "returns normalized vendor (lowercase, trimmed)",
			envValue:       "  HASHICORP-VAULT  ",
			envSet:         true,
			expectedVendor: "hashicorp-vault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := NewModeResolver(func(key string) (string, bool) {
				if key == EnvKMSVendor && tt.envSet {
					return tt.envValue, true
				}
				return "", false
			})

			vendor := resolver.GetVendor()
			assert.Equal(t, tt.expectedVendor, vendor)
		})
	}
}
