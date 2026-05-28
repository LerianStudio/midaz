// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"strings"

	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/kms/vault"
	"github.com/google/uuid"
)

// KMSResult contains the results of KMS initialization.
type KMSResult struct {
	Mode        crypto.EncryptionMode
	VaultClient *vault.Client
}

// initKMS resolves the encryption mode, validates configuration, and initializes the KMS client.
// This is the main entry point for KMS initialization during bootstrap.
func initKMS(ctx context.Context, cfg *Config, logger libLog.Logger) (*KMSResult, error) {
	mode, err := resolveEncryptionMode(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve encryption mode: %w", err)
	}

	if err := validateVaultConfig(mode, cfg); err != nil {
		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, "Encryption mode resolved",
		libLog.String("mode", mode.String()))

	result := &KMSResult{Mode: mode}

	// Initialize Vault client for envelope encryption mode
	if mode.IsEnvelope() {
		client, err := initVaultClient(ctx, cfg, logger)
		if err != nil {
			return nil, err
		}

		result.VaultClient = client
	}

	return result, nil
}

// initVaultClient creates and authenticates a Vault client.
func initVaultClient(ctx context.Context, cfg *Config, logger libLog.Logger) (*vault.Client, error) {
	vaultCfg := buildVaultConfig(cfg)

	client, err := vault.NewClient(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	// Verify credentials by performing initial login
	if err := client.Login(ctx); err != nil {
		return nil, fmt.Errorf("failed to authenticate with vault: %w", err)
	}

	logger.Log(ctx, libLog.LevelInfo, "Vault client initialized",
		libLog.String("mount_path", cfg.VaultMountPath))

	return client, nil
}

// resolveEncryptionMode determines the encryption mode from the configuration.
// Returns EncryptionModeLegacy when KMSVendor is empty or "none".
// Returns EncryptionModeEnvelope when KMSVendor is "hashicorp-vault".
// Returns an error for unsupported vendor values.
func resolveEncryptionMode(cfg *Config) (crypto.EncryptionMode, error) {
	resolver := crypto.NewModeResolver(func(key string) (string, bool) {
		// First check the config struct (already loaded from env)
		if key == crypto.EnvKMSVendor {
			if cfg.KMSVendor != "" {
				return cfg.KMSVendor, true
			}

			// Fall back to direct env lookup for unset config
			value, ok := os.LookupEnv(key)

			return value, ok
		}

		return "", false
	})

	return resolver.Resolve()
}

// validateVaultConfig validates the Vault configuration when envelope mode is enabled.
// For legacy mode, validation is skipped.
// For envelope mode, all Vault configuration fields must be present.
func validateVaultConfig(mode crypto.EncryptionMode, cfg *Config) error {
	if mode.IsLegacy() {
		return nil
	}

	vaultCfg := buildVaultConfig(cfg)
	if err := vaultCfg.Validate(); err != nil {
		return fmt.Errorf("envelope encryption mode requires valid vault configuration: %w", err)
	}

	return nil
}

// DefaultVaultDevToken is the hardcoded token for local/development environments.
// This matches the Vault dev container's default root token.
const DefaultVaultDevToken = "root"

// buildVaultConfig creates a vault.Config from the bootstrap Config.
// Auth method selection based on deployment mode:
//
// For local/development environments (DEPLOYMENT_MODE=local or empty):
//   - Uses Token auth with hardcoded root token (DefaultVaultDevToken)
//   - AppRole credentials are ignored in local mode for simplicity
//
// For production environments (DEPLOYMENT_MODE=saas, byoc):
//   - Uses AppRole auth exclusively (more secure)
//   - Token auth is not available in production mode
//
// Whitespace-only values are treated as empty.
func buildVaultConfig(cfg *Config) vault.Config {
	roleID := strings.TrimSpace(cfg.VaultRoleID)
	secretID := strings.TrimSpace(cfg.VaultSecretID)
	hasAppRole := roleID != "" && secretID != ""

	// Determine auth method and token based on deployment mode
	authMethod, token := resolveVaultAuth(cfg.DeploymentMode, hasAppRole)

	return vault.Config{
		Addr:       cfg.VaultAddr,
		RoleID:     cfg.VaultRoleID,
		SecretID:   cfg.VaultSecretID,
		MountPath:  cfg.VaultMountPath,
		Token:      token,
		AuthMethod: authMethod,
	}
}

// resolveVaultAuth determines the Vault auth method and token based on deployment mode.
// Local/development: uses hardcoded root token for simplicity.
// Production (saas/byoc): requires AppRole credentials.
func resolveVaultAuth(deploymentMode string, hasAppRole bool) (vault.AuthMethod, string) {
	mode := strings.ToLower(strings.TrimSpace(deploymentMode))

	// Check if production environment (saas or byoc)
	isProduction := mode == DeploymentModeSaaS || mode == DeploymentModeBYOC

	if isProduction {
		// Production: AppRole only
		if hasAppRole {
			return vault.AuthMethodAppRole, ""
		}

		// No valid auth in production without AppRole
		return "", ""
	}

	// Local/development: always use hardcoded root token
	return vault.AuthMethodToken, DefaultVaultDevToken
}

// buildTransitKeyName constructs the transit key name for an organization.
// Key name convention: org/{organization_id}
// Example: org/550e8400-e29b-41d4-a716-446655440000
//
// Keys are auto-created on first use per organization in Vault Transit.
//
//nolint:unused // Prepared for Phase 1 envelope encryption; will be used when Vault operations are implemented.
func buildTransitKeyName(organizationID uuid.UUID) string {
	return fmt.Sprintf("org/%s", organizationID.String())
}
