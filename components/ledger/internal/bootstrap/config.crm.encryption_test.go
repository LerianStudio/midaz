// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	mongoEncryption "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg/crypto"
	"github.com/LerianStudio/midaz/v4/pkg/crypto/kms/vault"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveEncryptionMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		kmsVendor     string
		expectedMode  crypto.EncryptionMode
		expectError   bool
		errorContains string
	}{
		// The empty-vendor (env-fallback) case lives in the non-parallel
		// TestResolveEncryptionMode_FromEnvironment so it can control KMS_VENDOR;
		// here every case sets an explicit vendor to stay environment-independent.
		{
			name:         "none vendor resolves to legacy mode",
			kmsVendor:    "none",
			expectedMode: crypto.EncryptionModeLegacy,
		},
		{
			name:         "hashicorp-vault vendor resolves to envelope mode",
			kmsVendor:    "hashicorp-vault",
			expectedMode: crypto.EncryptionModeEnvelope,
		},
		{
			name:          "invalid vendor returns error",
			kmsVendor:     "invalid-vendor",
			expectError:   true,
			errorContains: "unsupported KMS vendor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{KMSVendor: tt.kmsVendor}

			mode, err := resolveEncryptionMode(cfg)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedMode, mode)
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
		{name: "NONE uppercase resolves to legacy", kmsVendor: "NONE", expectedMode: crypto.EncryptionModeLegacy},
		{name: "HASHICORP-VAULT uppercase resolves to envelope", kmsVendor: "HASHICORP-VAULT", expectedMode: crypto.EncryptionModeEnvelope},
		{name: "Hashicorp-Vault mixed case resolves to envelope", kmsVendor: "Hashicorp-Vault", expectedMode: crypto.EncryptionModeEnvelope},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{KMSVendor: tt.kmsVendor}

			mode, err := resolveEncryptionMode(cfg)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedMode, mode)
		})
	}
}

// TestResolveEncryptionMode_FromEnvironment exercises the os.LookupEnv fallback
// taken when cfg.KMSVendor is empty. These subtests mutate KMS_VENDOR via
// t.Setenv (which restores prior state on cleanup), so they must not run in
// parallel.
func TestResolveEncryptionMode_FromEnvironment(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		expectedMode crypto.EncryptionMode
	}{
		{name: "empty env value resolves to legacy", envValue: "", expectedMode: crypto.EncryptionModeLegacy},
		{name: "env set to none resolves to legacy", envValue: "none", expectedMode: crypto.EncryptionModeLegacy},
		{name: "env set to hashicorp-vault resolves to envelope", envValue: "hashicorp-vault", expectedMode: crypto.EncryptionModeEnvelope},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(crypto.EnvKMSVendor, tt.envValue)

			cfg := &Config{} // empty KMSVendor forces the LookupEnv fallback

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
			name:        "legacy mode skips vault validation",
			mode:        crypto.EncryptionModeLegacy,
			cfg:         &Config{},
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
		},
		{
			name: "envelope mode with token in local passes without approle creds",
			mode: crypto.EncryptionModeEnvelope,
			cfg: &Config{
				VaultAddr:       "https://vault.example.com:8200",
				VaultAuthMethod: "token",
				DeploymentMode:  DeploymentModeLocal,
			},
		},
		{
			name: "envelope mode with missing vault addr fails",
			mode: crypto.EncryptionModeEnvelope,
			cfg: &Config{
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
				VaultAddr:      "https://vault.example.com:8200",
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
				DeploymentMode:  DeploymentModeSaaS,
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

				return
			}

			require.NoError(t, err)
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
	assert.Empty(t, vaultCfg.Token, "approle config must not carry a token")

	var _ vault.Config = vaultCfg
}

func TestBuildVaultConfig_TokenAuth(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		VaultAddr:       "https://vault.example.com:8200",
		VaultAuthMethod: "token",
		DeploymentMode:  DeploymentModeLocal,
	}

	vaultCfg, err := buildVaultConfig(cfg)

	require.NoError(t, err)
	assert.Equal(t, vault.AuthMethodToken, vaultCfg.AuthMethod)
	assert.Equal(t, DefaultVaultDevToken, vaultCfg.Token, "token auth must use DefaultVaultDevToken")
}

func TestDefaultVaultDevToken(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "root", DefaultVaultDevToken,
		"DefaultVaultDevToken must be 'root' to match the Vault dev container")
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
			deploymentMode: DeploymentModeLocal,
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
			deploymentMode: DeploymentModeSaaS,
			expectedMethod: vault.AuthMethodAppRole,
			expectedToken:  "",
		},
		{
			name:           "uppercase APPROLE is normalized",
			authMethod:     "APPROLE",
			deploymentMode: DeploymentModeBYOC,
			expectedMethod: vault.AuthMethodAppRole,
			expectedToken:  "",
		},
		{
			name:           "whitespace-padded token is normalized",
			authMethod:     "  token  ",
			deploymentMode: DeploymentModeLocal,
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
			deploymentMode: DeploymentModeSaaS,
			expectError:    true,
			errorContains:  "approle",
		},
		{
			name:           "token auth rejected in byoc (safety floor)",
			authMethod:     "token",
			deploymentMode: DeploymentModeBYOC,
			expectError:    true,
			errorContains:  "approle",
		},
		{
			name:           "token auth rejected for unrecognized deployment mode (fail closed)",
			authMethod:     "token",
			deploymentMode: "staging",
			expectError:    true,
			errorContains:  "local",
		},
		{
			name:           "token auth rejected for typo'd deployment mode (fail closed)",
			authMethod:     "token",
			deploymentMode: "prod",
			expectError:    true,
			errorContains:  "local",
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
			assert.Equal(t, tt.expectedMethod, method)
			assert.Equal(t, tt.expectedToken, token)
		})
	}
}

func TestIsLocalDeployment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		deploymentMode string
		want           bool
	}{
		{name: "local is local", deploymentMode: "local", want: true},
		{name: "empty resolves to local (default)", deploymentMode: "", want: true},
		{name: "whitespace-only resolves to local (default)", deploymentMode: "  ", want: true},
		{name: "uppercase LOCAL is local", deploymentMode: "LOCAL", want: true},
		{name: "whitespace-padded local is local", deploymentMode: "  local  ", want: true},
		{name: "saas is not local", deploymentMode: "saas", want: false},
		{name: "byoc is not local", deploymentMode: "byoc", want: false},
		{name: "unrecognized value is not local", deploymentMode: "staging", want: false},
		{name: "typo is not local", deploymentMode: "prod", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, isLocalDeployment(tt.deploymentMode))
		})
	}
}

func TestDefaultMountPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		multiTenant bool
		want        string
	}{
		{name: "multi-tenant resolves to the shared MT engine", multiTenant: true, want: "transit-mt"},
		{name: "single-tenant resolves to the shared ST engine", multiTenant: false, want: "transit-st"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, defaultMountPath(tt.multiTenant),
				"defaultMountPath(%v) must resolve to %q", tt.multiTenant, tt.want)
		})
	}
}

func TestWireEncryptionServices_LegacyMode(t *testing.T) {
	t.Parallel()

	// Legacy mode wires an EncryptionService from legacyCrypto only; it never
	// invokes legacyCrypto during wiring, so a nil value is sufficient here.
	// ProvisioningService stays nil.
	out := wireEncryptionServices(wireEncryptionServicesInput{
		mode:           crypto.EncryptionModeLegacy.String(),
		legacyCrypto:   nil,
	})

	require.NoError(t, out.err)
	assert.NotNil(t, out.encryptionService, "EncryptionService must be wired in legacy mode")
	assert.Nil(t, out.provisioningService, "ProvisioningService must be nil in legacy mode")
}

func TestWireEncryptionServices_EnvelopeGuards(t *testing.T) {
	t.Parallel()

	t.Run("nil vault client fails closed", func(t *testing.T) {
		t.Parallel()

		out := wireEncryptionServices(wireEncryptionServicesInput{
			mode:           crypto.EncryptionModeEnvelope.String(),
			vaultClient:    nil,
			keysetRepo:     &mockKeysetRepo{},
			registryRepo:   &mockRegistryRepo{},
		})

		require.Error(t, out.err)
		assert.Contains(t, out.err.Error(), "vault client")
	})

	t.Run("nil keyset repo fails closed", func(t *testing.T) {
		t.Parallel()

		out := wireEncryptionServices(wireEncryptionServicesInput{
			mode:           crypto.EncryptionModeEnvelope.String(),
			vaultClient:    newWiringVaultClient(t),
			keysetRepo:     nil,
			registryRepo:   &mockRegistryRepo{},
		})

		require.Error(t, out.err)
		assert.Contains(t, out.err.Error(), "keyset repository")
	})

	t.Run("nil registry repo fails closed", func(t *testing.T) {
		t.Parallel()

		out := wireEncryptionServices(wireEncryptionServicesInput{
			mode:           crypto.EncryptionModeEnvelope.String(),
			vaultClient:    newWiringVaultClient(t),
			keysetRepo:     &mockKeysetRepo{},
			registryRepo:   nil,
		})

		require.Error(t, out.err)
		assert.Contains(t, out.err.Error(), "registry repository")
	})
}

// newWiringVaultClient builds a real, unauthenticated *vault.Client. NewClient
// validates config and constructs the API client but performs no network I/O
// until Login/Encrypt, so it is safe for wiring tests that only exercise the
// envelope nil-dependency guards (which fire before any client call).
func newWiringVaultClient(t *testing.T) *vault.Client {
	t.Helper()

	client, err := vault.NewClient(vault.Config{
		Addr:       "https://vault.example.com:8200",
		AuthMethod: vault.AuthMethodToken,
		Token:      "test-token",
	})
	require.NoError(t, err)

	return client
}

// mockKeysetRepo implements encryption.KeysetRepository for wiring tests.
type mockKeysetRepo struct{}

func (m *mockKeysetRepo) Save(_ context.Context, _ *mmodel.OrganizationKeyset) error { return nil }

func (m *mockKeysetRepo) Get(_ context.Context, _ string) (*mmodel.OrganizationKeyset, error) {
	return &mmodel.OrganizationKeyset{}, nil
}

func (m *mockKeysetRepo) GetByVersion(_ context.Context, _ string, _ int) (*mmodel.OrganizationKeyset, error) {
	return &mmodel.OrganizationKeyset{}, nil
}

func (m *mockKeysetRepo) GetActive(_ context.Context, _ string) (*mmodel.OrganizationKeyset, error) {
	return &mmodel.OrganizationKeyset{}, nil
}

func (m *mockKeysetRepo) Update(_ context.Context, _ *mmodel.OrganizationKeyset, _ int64) error {
	return nil
}

// mockRegistryRepo implements mongoEncryption.RegistryRepository for wiring tests.
type mockRegistryRepo struct{}

func (m *mockRegistryRepo) Save(_ context.Context, _ *mmodel.OrganizationRegistryRecord) error {
	return nil
}

func (m *mockRegistryRepo) Get(_ context.Context, _ string) (*mmodel.OrganizationRegistryRecord, error) {
	return &mmodel.OrganizationRegistryRecord{}, nil
}

func (m *mockRegistryRepo) Update(_ context.Context, _ *mmodel.OrganizationRegistryRecord, _ int64) error {
	return nil
}

// Compile-time assertions that the mocks satisfy the repository interfaces the
// wiring input fields require.
var (
	_ encryption.KeysetRepository        = (*mockKeysetRepo)(nil)
	_ mongoEncryption.RegistryRepository = (*mockRegistryRepo)(nil)
)
