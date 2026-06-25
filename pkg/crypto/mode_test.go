// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptionMode_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mode     EncryptionMode
		expected string
	}{
		{
			name:     "legacy mode returns legacy string",
			mode:     EncryptionModeLegacy,
			expected: "legacy",
		},
		{
			name:     "envelope mode returns envelope string",
			mode:     EncryptionModeEnvelope,
			expected: "envelope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.mode.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEncryptionMode_IsLegacy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mode     EncryptionMode
		expected bool
	}{
		{
			name:     "legacy mode returns true",
			mode:     EncryptionModeLegacy,
			expected: true,
		},
		{
			name:     "envelope mode returns false",
			mode:     EncryptionModeEnvelope,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.mode.IsLegacy()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEncryptionMode_IsEnvelope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mode     EncryptionMode
		expected bool
	}{
		{
			name:     "legacy mode returns false for IsEnvelope",
			mode:     EncryptionModeLegacy,
			expected: false,
		},
		{
			name:     "envelope mode returns true for IsEnvelope",
			mode:     EncryptionModeEnvelope,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.mode.IsEnvelope()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnvironmentVariableConstants(t *testing.T) {
	t.Parallel()

	t.Run("EnvKMSVendor constant value", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "KMS_VENDOR", EnvKMSVendor, "EnvKMSVendor must equal KMS_VENDOR")
	})
}

func TestVendorConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "VendorNone is none",
			constant: VendorNone,
			expected: "none",
		},
		{
			name:     "VendorHashicorpVault is hashicorp-vault",
			constant: VendorHashicorpVault,
			expected: "hashicorp-vault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.constant)
		})
	}
}

func TestEncryptionModeValues(t *testing.T) {
	t.Parallel()

	t.Run("EncryptionModeLegacy has value 0", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, EncryptionMode(0), EncryptionModeLegacy)
	})

	t.Run("EncryptionModeEnvelope has value 1", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, EncryptionMode(1), EncryptionModeEnvelope)
	})
}

func TestEncryptionMode_MutualExclusivity(t *testing.T) {
	t.Parallel()

	t.Run("legacy mode is not envelope", func(t *testing.T) {
		t.Parallel()

		mode := EncryptionModeLegacy
		assert.True(t, mode.IsLegacy(), "legacy mode must return true for IsLegacy")
		assert.False(t, mode.IsEnvelope(), "legacy mode must return false for IsEnvelope")
	})

	t.Run("envelope mode is not legacy", func(t *testing.T) {
		t.Parallel()

		mode := EncryptionModeEnvelope
		assert.False(t, mode.IsLegacy(), "envelope mode must return false for IsLegacy")
		assert.True(t, mode.IsEnvelope(), "envelope mode must return true for IsEnvelope")
	})
}

func TestEncryptionMode_UnknownValue(t *testing.T) {
	t.Parallel()

	t.Run("unknown mode value returns empty string", func(t *testing.T) {
		t.Parallel()

		unknownMode := EncryptionMode(99)
		result := unknownMode.String()
		assert.Empty(t, result, "unknown mode must return empty string")
	})

	t.Run("unknown mode is neither legacy nor envelope", func(t *testing.T) {
		t.Parallel()

		unknownMode := EncryptionMode(99)
		assert.False(t, unknownMode.IsLegacy(), "unknown mode must return false for IsLegacy")
		assert.False(t, unknownMode.IsEnvelope(), "unknown mode must return false for IsEnvelope")
	})
}
