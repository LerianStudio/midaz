// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"strings"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/kms/vault"
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
	vaultCfg, err := buildVaultConfig(cfg)
	if err != nil {
		return nil, err
	}

	client, err := vault.NewClient(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	// Verify credentials by performing initial login
	if err := client.Login(ctx); err != nil {
		return nil, fmt.Errorf("failed to authenticate with vault: %w", err)
	}

	// Log the RESOLVED base mount (the same base bootstrap injects into
	// provisioning/keyset_manager), not the raw config value. KMS_VAULT_MOUNT_PATH
	// is optional, so the raw value is frequently empty/misleading; resolve it to
	// the safe default so the logged base always matches what is actually wired
	// downstream and is never blank.
	logger.Log(ctx, libLog.LevelInfo, "Vault client initialized",
		libLog.String("base_mount_path", resolveBaseMountPath(cfg.VaultMountPath)))

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

	vaultCfg, err := buildVaultConfig(cfg)
	if err != nil {
		return fmt.Errorf("envelope encryption mode requires valid vault configuration: %w", err)
	}

	if err := vaultCfg.Validate(); err != nil {
		return fmt.Errorf("envelope encryption mode requires valid vault configuration: %w", err)
	}

	return nil
}

// DefaultVaultDevToken is the hardcoded token used by token auth.
// This matches the Vault dev container's default root token.
const DefaultVaultDevToken = "root"

// buildVaultConfig creates a vault.Config from the bootstrap Config.
//
// Authentication is driven solely by KMS_VAULT_AUTH_METHOD and is resolved only in
// envelope mode (KMS_VENDOR=hashicorp-vault); legacy mode never reaches here.
//
//   - "token":   token auth with the hardcoded dev root token (local development).
//   - "approle": AppRole auth with role_id/secret_id (development and production).
//     Missing credentials fail closed via vault.Config.Validate.
//   - unset/empty/invalid: fail closed with an error. There is no default.
//
// Safety floor: token auth is rejected when DEPLOYMENT_MODE is saas or byoc, so a
// production deployment can never authenticate with the dev root token.
func buildVaultConfig(cfg *Config) (vault.Config, error) {
	authMethod, token, err := resolveVaultAuth(cfg)
	if err != nil {
		return vault.Config{}, err
	}

	return vault.Config{
		Addr:       cfg.VaultAddr,
		RoleID:     cfg.VaultRoleID,
		SecretID:   cfg.VaultSecretID,
		Token:      token,
		AuthMethod: authMethod,
	}, nil
}

// resolveVaultAuth determines the Vault auth method and token from KMS_VAULT_AUTH_METHOD.
// It fails closed for an unset, empty, or invalid auth method, and rejects token auth
// in production (saas/byoc) so the dev root token can never be used there.
func resolveVaultAuth(cfg *Config) (vault.AuthMethod, string, error) {
	method, err := vault.ParseAuthMethod(cfg.VaultAuthMethod)
	if err != nil {
		return "", "", fmt.Errorf("KMS_VAULT_AUTH_METHOD must be set to a valid value: %w", err)
	}

	switch method {
	case vault.AuthMethodToken:
		if isProductionDeployment(cfg.DeploymentMode) {
			return "", "", fmt.Errorf(
				"KMS_VAULT_AUTH_METHOD=token is not allowed when DEPLOYMENT_MODE is %q: production requires approle",
				strings.ToLower(strings.TrimSpace(cfg.DeploymentMode)))
		}

		return vault.AuthMethodToken, DefaultVaultDevToken, nil
	case vault.AuthMethodAppRole:
		return vault.AuthMethodAppRole, "", nil
	default:
		return "", "", fmt.Errorf("unsupported vault auth method %q", method)
	}
}

// isProductionDeployment reports whether the deployment mode is a production mode
// (saas or byoc), where token auth with the dev root token is forbidden.
func isProductionDeployment(deploymentMode string) bool {
	mode := strings.ToLower(strings.TrimSpace(deploymentMode))
	return mode == DeploymentModeSaaS || mode == DeploymentModeBYOC
}
