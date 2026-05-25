// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/kms/vault"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_KMSFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fieldName string
		fieldType string
		envTag    string
	}{
		{
			name:      "has KMSVendor string field",
			fieldName: "KMSVendor",
			fieldType: "string",
			envTag:    "KMS_VENDOR",
		},
		{
			name:      "has VaultAddr string field",
			fieldName: "VaultAddr",
			fieldType: "string",
			envTag:    "KMS_VAULT_ADDR",
		},
		{
			name:      "has VaultRoleID string field",
			fieldName: "VaultRoleID",
			fieldType: "string",
			envTag:    "KMS_VAULT_ROLE_ID",
		},
		{
			name:      "has VaultSecretID string field",
			fieldName: "VaultSecretID",
			fieldType: "string",
			envTag:    "KMS_VAULT_SECRET_ID",
		},
		{
			name:      "has VaultMountPath string field",
			fieldName: "VaultMountPath",
			fieldType: "string",
			envTag:    "KMS_VAULT_MOUNT_PATH",
		},
	}

	configType := reflect.TypeOf(Config{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			field, found := configType.FieldByName(tt.fieldName)
			require.True(t, found, "Config struct must have field %s", tt.fieldName)

			assert.Equal(t, tt.fieldType, field.Type.String(),
				"field %s must be of type %s", tt.fieldName, tt.fieldType)

			envValue := field.Tag.Get("env")
			assert.Equal(t, tt.envTag, envValue,
				"field %s must have env tag %q", tt.fieldName, tt.envTag)
		})
	}
}

func TestConfig_KMSDefaults(t *testing.T) {
	t.Parallel()

	cfg := &Config{}

	assert.Empty(t, cfg.KMSVendor,
		"KMSVendor must default to empty string (zero value)")
	assert.Empty(t, cfg.VaultAddr,
		"VaultAddr must default to empty string (zero value)")
	assert.Empty(t, cfg.VaultRoleID,
		"VaultRoleID must default to empty string (zero value)")
	assert.Empty(t, cfg.VaultSecretID,
		"VaultSecretID must default to empty string (zero value)")
	assert.Empty(t, cfg.VaultMountPath,
		"VaultMountPath must default to empty string (zero value)")
}

func TestService_HasEncryptionModeField(t *testing.T) {
	t.Parallel()

	serviceType := reflect.TypeOf(Service{})
	field, found := serviceType.FieldByName("EncryptionMode")

	require.True(t, found, "Service struct must have EncryptionMode field")
	assert.Equal(t, "crypto.EncryptionMode", field.Type.String(),
		"EncryptionMode field must be of type crypto.EncryptionMode")
}

func TestService_HasVaultClientField(t *testing.T) {
	t.Parallel()

	serviceType := reflect.TypeOf(Service{})
	field, found := serviceType.FieldByName("VaultClient")

	require.True(t, found, "Service struct must have VaultClient field")
	assert.Equal(t, "*vault.Client", field.Type.String(),
		"VaultClient field must be of type *vault.Client")
}

func TestKMSResult_Struct(t *testing.T) {
	t.Parallel()

	t.Run("has Mode field", func(t *testing.T) {
		t.Parallel()

		resultType := reflect.TypeOf(KMSResult{})
		field, found := resultType.FieldByName("Mode")

		require.True(t, found, "KMSResult struct must have Mode field")
		assert.Equal(t, "crypto.EncryptionMode", field.Type.String())
	})

	t.Run("has VaultClient field", func(t *testing.T) {
		t.Parallel()

		resultType := reflect.TypeOf(KMSResult{})
		field, found := resultType.FieldByName("VaultClient")

		require.True(t, found, "KMSResult struct must have VaultClient field")
		assert.Equal(t, "*vault.Client", field.Type.String())
	})
}

func TestResolveEncryptionMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		kmsVendor     string
		expectedMode  crypto.EncryptionMode
		expectError   bool
		errorContains string
	}{
		{
			name:         "empty vendor defaults to legacy mode",
			kmsVendor:    "",
			expectedMode: crypto.EncryptionModeLegacy,
			expectError:  false,
		},
		{
			name:         "none vendor resolves to legacy mode",
			kmsVendor:    "none",
			expectedMode: crypto.EncryptionModeLegacy,
			expectError:  false,
		},
		{
			name:         "hashicorp-vault vendor resolves to envelope mode",
			kmsVendor:    "hashicorp-vault",
			expectedMode: crypto.EncryptionModeEnvelope,
			expectError:  false,
		},
		{
			name:          "invalid vendor returns error",
			kmsVendor:     "invalid-vendor",
			expectedMode:  crypto.EncryptionModeLegacy,
			expectError:   true,
			errorContains: "unsupported KMS vendor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				KMSVendor: tt.kmsVendor,
			}

			mode, err := resolveEncryptionMode(cfg)

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

func TestResolveEncryptionMode_CaseInsensitive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		kmsVendor    string
		expectedMode crypto.EncryptionMode
	}{
		{
			name:         "NONE uppercase resolves to legacy",
			kmsVendor:    "NONE",
			expectedMode: crypto.EncryptionModeLegacy,
		},
		{
			name:         "HASHICORP-VAULT uppercase resolves to envelope",
			kmsVendor:    "HASHICORP-VAULT",
			expectedMode: crypto.EncryptionModeEnvelope,
		},
		{
			name:         "Hashicorp-Vault mixed case resolves to envelope",
			kmsVendor:    "Hashicorp-Vault",
			expectedMode: crypto.EncryptionModeEnvelope,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				KMSVendor: tt.kmsVendor,
			}

			mode, err := resolveEncryptionMode(cfg)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedMode, mode)
		})
	}
}

func TestValidateVaultConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		mode          crypto.EncryptionMode
		cfg           *Config
		expectError   bool
		errorContains string
	}{
		{
			name: "legacy mode skips vault validation",
			mode: crypto.EncryptionModeLegacy,
			cfg:  &Config{
				// No vault config needed for legacy mode
			},
			expectError: false,
		},
		{
			name: "envelope mode with valid vault config passes",
			mode: crypto.EncryptionModeEnvelope,
			cfg: &Config{
				VaultAddr:      "https://vault.example.com:8200",
				VaultRoleID:    "role-123",
				VaultSecretID:  "secret-456",
				VaultMountPath: "transit",
			},
			expectError: false,
		},
		{
			name: "envelope mode with missing vault addr fails",
			mode: crypto.EncryptionModeEnvelope,
			cfg: &Config{
				VaultAddr:      "",
				VaultRoleID:    "role-123",
				VaultSecretID:  "secret-456",
				VaultMountPath: "transit",
			},
			expectError:   true,
			errorContains: "addr",
		},
		{
			name:          "envelope mode with all vault fields empty fails",
			mode:          crypto.EncryptionModeEnvelope,
			cfg:           &Config{},
			expectError:   true,
			errorContains: "vault config missing required fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateVaultConfig(tt.mode, tt.cfg)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBuildVaultConfig(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		VaultAddr:      "https://vault.example.com:8200",
		VaultRoleID:    "role-123",
		VaultSecretID:  "secret-456",
		VaultMountPath: "transit",
	}

	vaultCfg := buildVaultConfig(cfg)

	assert.Equal(t, cfg.VaultAddr, vaultCfg.Addr)
	assert.Equal(t, cfg.VaultRoleID, vaultCfg.RoleID)
	assert.Equal(t, cfg.VaultSecretID, vaultCfg.SecretID)
	assert.Equal(t, cfg.VaultMountPath, vaultCfg.MountPath)
}

func TestResolveEncryptionMode_FromEnvironment(t *testing.T) {
	// This test verifies integration with os.Getenv
	// We can't run in parallel because we modify environment variables

	originalValue := os.Getenv("KMS_VENDOR")
	defer func() {
		if originalValue != "" {
			os.Setenv("KMS_VENDOR", originalValue)
		} else {
			os.Unsetenv("KMS_VENDOR")
		}
	}()

	tests := []struct {
		name         string
		envValue     string
		expectedMode crypto.EncryptionMode
	}{
		{
			name:         "env not set defaults to legacy",
			envValue:     "",
			expectedMode: crypto.EncryptionModeLegacy,
		},
		{
			name:         "env set to none resolves to legacy",
			envValue:     "none",
			expectedMode: crypto.EncryptionModeLegacy,
		},
		{
			name:         "env set to hashicorp-vault resolves to envelope",
			envValue:     "hashicorp-vault",
			expectedMode: crypto.EncryptionModeEnvelope,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue == "" {
				os.Unsetenv("KMS_VENDOR")
			} else {
				os.Setenv("KMS_VENDOR", tt.envValue)
			}

			cfg := &Config{
				KMSVendor: tt.envValue,
			}

			mode, err := resolveEncryptionMode(cfg)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedMode, mode)
		})
	}
}

func TestBuildVaultConfig_ReturnsVaultConfigType(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		VaultAddr:      "https://vault.example.com:8200",
		VaultRoleID:    "role-123",
		VaultSecretID:  "secret-456",
		VaultMountPath: "transit",
	}

	vaultCfg := buildVaultConfig(cfg)

	// Type assertion to verify the return type
	var _ vault.Config = vaultCfg
}

func TestBuildTransitKeyName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		orgID          uuid.UUID
		expectedResult string
	}{
		{
			name:           "generates correct key name for organization",
			orgID:          uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			expectedResult: "org/550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:           "works with different organization ID",
			orgID:          uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
			expectedResult: "org/123e4567-e89b-12d3-a456-426614174000",
		},
		{
			name:           "handles nil UUID (zero value)",
			orgID:          uuid.Nil,
			expectedResult: "org/00000000-0000-0000-0000-000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := buildTransitKeyName(tt.orgID)

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestBuildTransitKeyName_KeyNameConvention(t *testing.T) {
	t.Parallel()

	t.Run("key name follows convention: org/{org_id}", func(t *testing.T) {
		t.Parallel()

		orgID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

		keyName := buildTransitKeyName(orgID)

		assert.Equal(t, "org/550e8400-e29b-41d4-a716-446655440000", keyName)
		assert.Contains(t, keyName, "org/")
		assert.Contains(t, keyName, orgID.String())
	})
}
