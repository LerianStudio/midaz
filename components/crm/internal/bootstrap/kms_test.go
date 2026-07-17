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
			name:      "has VaultAuthMethod string field",
			fieldName: "VaultAuthMethod",
			fieldType: "string",
			envTag:    "KMS_VAULT_AUTH_METHOD",
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
	assert.Empty(t, cfg.VaultAuthMethod,
		"VaultAuthMethod must default to empty string (zero value)")
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
			name: "envelope mode with approle and valid config passes",
			mode: crypto.EncryptionModeEnvelope,
			cfg: &Config{
				VaultAddr:       "https://vault.example.com:8200",
				VaultRoleID:     "role-123",
				VaultSecretID:   "secret-456",
				VaultAuthMethod: "approle",
			},
			expectError: false,
		},
		{
			name: "envelope mode with token in local passes without approle creds",
			mode: crypto.EncryptionModeEnvelope,
			cfg: &Config{
				VaultAddr:       "https://vault.example.com:8200",
				VaultAuthMethod: "token",
				DeploymentMode:  "local",
			},
			expectError: false,
		},
		{
			name: "envelope mode with missing vault addr fails",
			mode: crypto.EncryptionModeEnvelope,
			cfg: &Config{
				VaultAddr:       "",
				VaultRoleID:     "role-123",
				VaultSecretID:   "secret-456",
				VaultAuthMethod: "approle",
			},
			expectError:   true,
			errorContains: "addr",
		},
		{
			name: "envelope mode with unset auth method fails closed",
			mode: crypto.EncryptionModeEnvelope,
			cfg: &Config{
				VaultAddr: "https://vault.example.com:8200",
			},
			expectError:   true,
			errorContains: "KMS_VAULT_AUTH_METHOD",
		},
		{
			name: "envelope mode with approle but missing credentials fails closed",
			mode: crypto.EncryptionModeEnvelope,
			cfg: &Config{
				VaultAddr:       "https://vault.example.com:8200",
				VaultAuthMethod: "approle",
			},
			expectError:   true,
			errorContains: "approle auth requires",
		},
		{
			name: "envelope mode with token in saas fails closed (safety floor)",
			mode: crypto.EncryptionModeEnvelope,
			cfg: &Config{
				VaultAddr:       "https://vault.example.com:8200",
				VaultAuthMethod: "token",
				DeploymentMode:  "saas",
			},
			expectError:   true,
			errorContains: "approle",
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
		VaultAddr:       "https://vault.example.com:8200",
		VaultRoleID:     "role-123",
		VaultSecretID:   "secret-456",
		VaultAuthMethod: "approle",
	}

	vaultCfg, err := buildVaultConfig(cfg)

	require.NoError(t, err)
	assert.Equal(t, cfg.VaultAddr, vaultCfg.Addr)
	assert.Equal(t, cfg.VaultRoleID, vaultCfg.RoleID)
	assert.Equal(t, cfg.VaultSecretID, vaultCfg.SecretID)
	assert.Equal(t, vault.AuthMethodAppRole, vaultCfg.AuthMethod)
	// vault.Config no longer carries a MountPath: the base mount is the mode-derived
	// shared engine resolved downstream, not at vault client construction time.
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
		VaultAddr:       "https://vault.example.com:8200",
		VaultRoleID:     "role-123",
		VaultSecretID:   "secret-456",
		VaultAuthMethod: "approle",
	}

	vaultCfg, err := buildVaultConfig(cfg)

	// Type assertion to verify the return type
	require.NoError(t, err)
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

func TestBuildVaultConfig_AuthMethod(t *testing.T) {
	t.Parallel()

	t.Run("token auth uses hardcoded dev token", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:       "https://vault.example.com:8200",
			VaultAuthMethod: "token",
			DeploymentMode:  "local",
		}

		vaultCfg, err := buildVaultConfig(cfg)

		require.NoError(t, err)
		assert.Equal(t, vault.AuthMethodToken, vaultCfg.AuthMethod,
			"token auth method must select token auth")
		assert.Equal(t, DefaultVaultDevToken, vaultCfg.Token,
			"token auth must use DefaultVaultDevToken")
	})

	t.Run("token auth works when DeploymentMode is empty", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:       "https://vault.example.com:8200",
			VaultAuthMethod: "token",
			DeploymentMode:  "",
		}

		vaultCfg, err := buildVaultConfig(cfg)

		require.NoError(t, err)
		assert.Equal(t, vault.AuthMethodToken, vaultCfg.AuthMethod)
		assert.Equal(t, DefaultVaultDevToken, vaultCfg.Token)
	})

	t.Run("approle auth does not set a token", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:       "https://vault.example.com:8200",
			VaultRoleID:     "role-123",
			VaultSecretID:   "secret-456",
			VaultAuthMethod: "approle",
			DeploymentMode:  "saas",
		}

		vaultCfg, err := buildVaultConfig(cfg)

		require.NoError(t, err)
		assert.Equal(t, vault.AuthMethodAppRole, vaultCfg.AuthMethod)
		assert.Empty(t, vaultCfg.Token,
			"approle auth must not set a token")
		assert.Equal(t, cfg.VaultRoleID, vaultCfg.RoleID)
		assert.Equal(t, cfg.VaultSecretID, vaultCfg.SecretID)
	})

	t.Run("approle is accepted for any non-production deployment mode", func(t *testing.T) {
		t.Parallel()

		for _, mode := range []string{"", "local", "byoc", "saas"} {
			cfg := &Config{
				VaultAddr:       "https://vault.example.com:8200",
				VaultRoleID:     "role-123",
				VaultSecretID:   "secret-456",
				VaultAuthMethod: "approle",
				DeploymentMode:  mode,
			}

			vaultCfg, err := buildVaultConfig(cfg)

			require.NoErrorf(t, err, "approle must be accepted for deployment mode %q", mode)
			assert.Equal(t, vault.AuthMethodAppRole, vaultCfg.AuthMethod)
		}
	})

	t.Run("unset auth method fails closed", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr: "https://vault.example.com:8200",
			// VaultAuthMethod intentionally unset
		}

		_, err := buildVaultConfig(cfg)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "KMS_VAULT_AUTH_METHOD")
	})

	t.Run("invalid auth method fails closed", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:       "https://vault.example.com:8200",
			VaultAuthMethod: "not-a-method",
		}

		_, err := buildVaultConfig(cfg)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "KMS_VAULT_AUTH_METHOD")
	})

	t.Run("token auth rejected in saas mode (safety floor)", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:       "https://vault.example.com:8200",
			VaultAuthMethod: "token",
			DeploymentMode:  "saas",
		}

		_, err := buildVaultConfig(cfg)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "approle",
			"production must require approle, not the dev token")
	})

	t.Run("token auth rejected in byoc mode (safety floor)", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:       "https://vault.example.com:8200",
			VaultAuthMethod: "token",
			DeploymentMode:  "byoc",
		}

		_, err := buildVaultConfig(cfg)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "approle")
	})
}

func TestResolveVaultAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		authMethod     string
		deploymentMode string
		expectedMethod vault.AuthMethod
		expectedToken  string
		expectError    bool
		errorContains  string
	}{
		{
			name:           "token auth in local uses dev token",
			authMethod:     "token",
			deploymentMode: "local",
			expectedMethod: vault.AuthMethodToken,
			expectedToken:  DefaultVaultDevToken,
		},
		{
			name:           "token auth with empty deployment mode uses dev token",
			authMethod:     "token",
			deploymentMode: "",
			expectedMethod: vault.AuthMethodToken,
			expectedToken:  DefaultVaultDevToken,
		},
		{
			name:           "approle auth returns approle without token",
			authMethod:     "approle",
			deploymentMode: "saas",
			expectedMethod: vault.AuthMethodAppRole,
			expectedToken:  "",
		},
		{
			name:           "uppercase APPROLE is normalized",
			authMethod:     "APPROLE",
			deploymentMode: "byoc",
			expectedMethod: vault.AuthMethodAppRole,
			expectedToken:  "",
		},
		{
			name:           "whitespace-padded token is normalized",
			authMethod:     "  token  ",
			deploymentMode: "local",
			expectedMethod: vault.AuthMethodToken,
			expectedToken:  DefaultVaultDevToken,
		},
		{
			name:          "unset auth method fails closed",
			authMethod:    "",
			expectError:   true,
			errorContains: "KMS_VAULT_AUTH_METHOD",
		},
		{
			name:          "invalid auth method fails closed",
			authMethod:    "bogus",
			expectError:   true,
			errorContains: "KMS_VAULT_AUTH_METHOD",
		},
		{
			name:           "token auth rejected in saas (safety floor)",
			authMethod:     "token",
			deploymentMode: "saas",
			expectError:    true,
			errorContains:  "approle",
		},
		{
			name:           "token auth rejected in byoc (safety floor)",
			authMethod:     "token",
			deploymentMode: "byoc",
			expectError:    true,
			errorContains:  "approle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				VaultAuthMethod: tt.authMethod,
				DeploymentMode:  tt.deploymentMode,
			}

			method, token, err := resolveVaultAuth(cfg)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)

				return
			}

			require.NoError(t, err)
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
			VaultAddr:       "https://vault.example.com:8200",
			VaultRoleID:     "role-123",
			VaultSecretID:   "secret-456",
			VaultAuthMethod: "approle",
		}

		err := validateVaultConfig(crypto.EncryptionModeEnvelope, cfg)

		require.NoError(t, err,
			"validateVaultConfig must accept AppRole auth credentials for envelope mode")
	})

	t.Run("accepts token auth without AppRole credentials in local", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			VaultAddr:       "https://vault.example.com:8200",
			VaultAuthMethod: "token",
			DeploymentMode:  "local",
			// No AppRole credentials - token auth uses the hardcoded dev token
		}

		err := validateVaultConfig(crypto.EncryptionModeEnvelope, cfg)

		require.NoError(t, err,
			"validateVaultConfig must accept token auth without AppRole credentials")
	})
}
