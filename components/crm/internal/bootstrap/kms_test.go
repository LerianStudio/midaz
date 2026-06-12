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
	// vault.Config no longer carries a MountPath: the base mount lives on the
	// bootstrap Config (VaultMountPath) and is resolved per-tenant downstream,
	// not at vault client construction time.
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

func TestDefaultVaultDevToken(t *testing.T) {
	t.Parallel()

	t.Run("DefaultVaultDevToken constant is defined", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "root", DefaultVaultDevToken,
			"DefaultVaultDevToken must be 'root' to match Vault dev container")
	})
}

func TestBuildVaultConfig_DeploymentMode(t *testing.T) {
	t.Parallel()

	t.Run("local mode uses hardcoded token auth", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:      "https://vault.example.com:8200",
			VaultMountPath: "transit",
			DeploymentMode: "local",
		}

		vaultCfg := buildVaultConfig(cfg)

		assert.Equal(t, vault.AuthMethodToken, vaultCfg.AuthMethod,
			"Local mode must use token auth")
		assert.Equal(t, DefaultVaultDevToken, vaultCfg.Token,
			"Local mode must use DefaultVaultDevToken")
	})

	t.Run("empty DeploymentMode defaults to local behavior with hardcoded token", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:      "https://vault.example.com:8200",
			VaultMountPath: "transit",
			DeploymentMode: "", // Empty defaults to local
		}

		vaultCfg := buildVaultConfig(cfg)

		assert.Equal(t, vault.AuthMethodToken, vaultCfg.AuthMethod,
			"Empty DeploymentMode must default to local behavior (token auth)")
		assert.Equal(t, DefaultVaultDevToken, vaultCfg.Token,
			"Empty DeploymentMode must use DefaultVaultDevToken")
	})

	t.Run("local mode ignores AppRole credentials", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:      "https://vault.example.com:8200",
			VaultRoleID:    "role-123",
			VaultSecretID:  "secret-456",
			VaultMountPath: "transit",
			DeploymentMode: "local",
		}

		vaultCfg := buildVaultConfig(cfg)

		assert.Equal(t, vault.AuthMethodToken, vaultCfg.AuthMethod,
			"Local mode must use token auth even when AppRole credentials are provided")
		assert.Equal(t, DefaultVaultDevToken, vaultCfg.Token,
			"Local mode must use DefaultVaultDevToken even when AppRole credentials are provided")
	})

	t.Run("saas mode uses AppRole when credentials provided", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:      "https://vault.example.com:8200",
			VaultRoleID:    "role-123",
			VaultSecretID:  "secret-456",
			VaultMountPath: "transit",
			DeploymentMode: "saas",
		}

		vaultCfg := buildVaultConfig(cfg)

		assert.Equal(t, vault.AuthMethodAppRole, vaultCfg.AuthMethod,
			"SaaS mode must use AppRole auth when credentials are provided")
		assert.Empty(t, vaultCfg.Token,
			"SaaS mode must not set token when using AppRole")
	})

	t.Run("byoc mode uses AppRole when credentials provided", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:      "https://vault.example.com:8200",
			VaultRoleID:    "role-123",
			VaultSecretID:  "secret-456",
			VaultMountPath: "transit",
			DeploymentMode: "byoc",
		}

		vaultCfg := buildVaultConfig(cfg)

		assert.Equal(t, vault.AuthMethodAppRole, vaultCfg.AuthMethod,
			"BYOC mode must use AppRole auth when credentials are provided")
		assert.Empty(t, vaultCfg.Token,
			"BYOC mode must not set token when using AppRole")
	})

	t.Run("saas mode returns empty auth when AppRole credentials missing", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:      "https://vault.example.com:8200",
			VaultMountPath: "transit",
			DeploymentMode: "saas",
			// No AppRole credentials
		}

		vaultCfg := buildVaultConfig(cfg)

		assert.Empty(t, vaultCfg.AuthMethod,
			"SaaS mode must return empty auth method when AppRole credentials are missing")
		assert.Empty(t, vaultCfg.Token,
			"SaaS mode must not set token when AppRole credentials are missing")
	})

	t.Run("byoc mode returns empty auth when AppRole credentials missing", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:      "https://vault.example.com:8200",
			VaultMountPath: "transit",
			DeploymentMode: "byoc",
			// No AppRole credentials
		}

		vaultCfg := buildVaultConfig(cfg)

		assert.Empty(t, vaultCfg.AuthMethod,
			"BYOC mode must return empty auth method when AppRole credentials are missing")
	})

	t.Run("production mode requires both RoleID and SecretID for AppRole", func(t *testing.T) {
		t.Parallel()

		// Only RoleID provided
		cfg := &Config{
			VaultAddr:      "https://vault.example.com:8200",
			VaultRoleID:    "role-123",
			VaultMountPath: "transit",
			DeploymentMode: "saas",
		}

		vaultCfg := buildVaultConfig(cfg)

		assert.Empty(t, vaultCfg.AuthMethod,
			"Production mode must return empty auth when only RoleID is provided")
	})

	t.Run("whitespace-only RoleID is treated as empty", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:      "https://vault.example.com:8200",
			VaultRoleID:    "   ",
			VaultSecretID:  "secret-456",
			VaultMountPath: "transit",
			DeploymentMode: "saas",
		}

		vaultCfg := buildVaultConfig(cfg)

		assert.Empty(t, vaultCfg.AuthMethod,
			"Whitespace-only RoleID must be treated as empty")
	})

	t.Run("whitespace-only SecretID is treated as empty", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:      "https://vault.example.com:8200",
			VaultRoleID:    "role-123",
			VaultSecretID:  "   ",
			VaultMountPath: "transit",
			DeploymentMode: "saas",
		}

		vaultCfg := buildVaultConfig(cfg)

		assert.Empty(t, vaultCfg.AuthMethod,
			"Whitespace-only SecretID must be treated as empty")
	})
}

func TestResolveVaultAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		deploymentMode string
		hasAppRole     bool
		expectedMethod vault.AuthMethod
		expectedToken  string
	}{
		// Local/development mode tests - always uses hardcoded token
		{
			name:           "local mode always uses hardcoded token",
			deploymentMode: "local",
			hasAppRole:     false,
			expectedMethod: vault.AuthMethodToken,
			expectedToken:  DefaultVaultDevToken,
		},
		{
			name:           "local mode ignores AppRole credentials",
			deploymentMode: "local",
			hasAppRole:     true,
			expectedMethod: vault.AuthMethodToken,
			expectedToken:  DefaultVaultDevToken,
		},
		{
			name:           "empty mode defaults to local behavior",
			deploymentMode: "",
			hasAppRole:     true,
			expectedMethod: vault.AuthMethodToken,
			expectedToken:  DefaultVaultDevToken,
		},
		// Production mode tests (saas) - AppRole only
		{
			name:           "saas mode uses AppRole when credentials provided",
			deploymentMode: "saas",
			hasAppRole:     true,
			expectedMethod: vault.AuthMethodAppRole,
			expectedToken:  "",
		},
		{
			name:           "saas mode returns empty when no AppRole credentials",
			deploymentMode: "saas",
			hasAppRole:     false,
			expectedMethod: "",
			expectedToken:  "",
		},
		// Production mode tests (byoc) - AppRole only
		{
			name:           "byoc mode uses AppRole when credentials provided",
			deploymentMode: "byoc",
			hasAppRole:     true,
			expectedMethod: vault.AuthMethodAppRole,
			expectedToken:  "",
		},
		{
			name:           "byoc mode returns empty when no AppRole credentials",
			deploymentMode: "byoc",
			hasAppRole:     false,
			expectedMethod: "",
			expectedToken:  "",
		},
		// Case insensitivity tests
		{
			name:           "SAAS uppercase uses AppRole",
			deploymentMode: "SAAS",
			hasAppRole:     true,
			expectedMethod: vault.AuthMethodAppRole,
			expectedToken:  "",
		},
		{
			name:           "BYOC uppercase uses AppRole",
			deploymentMode: "BYOC",
			hasAppRole:     true,
			expectedMethod: vault.AuthMethodAppRole,
			expectedToken:  "",
		},
		{
			name:           "Local mixed case uses token",
			deploymentMode: "Local",
			hasAppRole:     true,
			expectedMethod: vault.AuthMethodToken,
			expectedToken:  DefaultVaultDevToken,
		},
		// Whitespace handling
		{
			name:           "whitespace-padded local mode uses token",
			deploymentMode: "  local  ",
			hasAppRole:     false,
			expectedMethod: vault.AuthMethodToken,
			expectedToken:  DefaultVaultDevToken,
		},
		{
			name:           "whitespace-padded saas mode uses AppRole",
			deploymentMode: "  saas  ",
			hasAppRole:     true,
			expectedMethod: vault.AuthMethodAppRole,
			expectedToken:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			method, token := resolveVaultAuth(tt.deploymentMode, tt.hasAppRole)

			assert.Equal(t, tt.expectedMethod, method,
				"AuthMethod must match expected value")
			assert.Equal(t, tt.expectedToken, token,
				"Token must match expected value")
		})
	}
}

func TestValidateVaultConfig_AuthMethods(t *testing.T) {
	t.Parallel()

	t.Run("accepts AppRole auth credentials for envelope mode", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:      "https://vault.example.com:8200",
			VaultRoleID:    "role-123",
			VaultSecretID:  "secret-456",
			VaultMountPath: "transit",
		}

		err := validateVaultConfig(crypto.EncryptionModeEnvelope, cfg)

		require.NoError(t, err,
			"validateVaultConfig must accept AppRole auth credentials for envelope mode")
	})

	t.Run("accepts local mode config without AppRole (uses hardcoded token)", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:      "https://vault.example.com:8200",
			VaultMountPath: "transit",
			DeploymentMode: "local",
			// No AppRole credentials - local mode uses hardcoded token
		}

		err := validateVaultConfig(crypto.EncryptionModeEnvelope, cfg)

		require.NoError(t, err,
			"validateVaultConfig must accept local mode without AppRole credentials")
	})
}
