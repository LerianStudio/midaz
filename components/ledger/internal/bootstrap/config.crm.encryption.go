// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"strings"

	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/lib-observability/metrics"
	mongoAudit "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/audit"
	mongoEncryption "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg/crypto"
	"github.com/LerianStudio/midaz/v4/pkg/crypto/kms/vault"
	"github.com/LerianStudio/midaz/v4/pkg/crypto/tink"
)

// Compile-time guarantee that the MongoDB keyset adapter satisfies the encryption
// service's KeysetRepository contract. The interface lives in the consuming service
// package (dependencies flow inward); this assertion sits at the wiring seam, the
// one place that legitimately imports both packages.
var _ encryption.KeysetRepository = (*mongoEncryption.KeysetMongoDBRepository)(nil)

// defaultKEKMountPath is the default Vault Transit mount path for KEK operations.
const defaultKEKMountPath = "transit"

// DefaultVaultDevToken is the hardcoded token used by token auth.
// This matches the Vault dev container's default root token.
const DefaultVaultDevToken = "root"

// kmsResult contains the results of KMS initialization.
type kmsResult struct {
	Mode        crypto.EncryptionMode
	VaultClient *vault.Client
}

// crmEncryption holds the wired CRM field-encryption surface: the FieldEncryptor
// injected into the holder/instrument repositories plus the envelope-only services
// and audit repository consumed by the encryption/audit HTTP handlers and readyz.
// In legacy mode (KMS_VENDOR=none) provisioningService and auditRepo are nil and
// vaultClient is nil; fieldEncryptor is always non-nil so the holder repository's
// non-nil guard is satisfied.
type crmEncryption struct {
	fieldEncryptor      encryption.FieldEncryptor
	provisioningService encryption.ProvisioningService
	auditRepo           mongoAudit.Repository
	vaultClient         *vault.Client
	mode                crypto.EncryptionMode
}

// initCRMEncryption resolves the encryption mode, initializes the KMS client (for
// envelope mode), wires the encryption services, and returns the FieldEncryptor and
// the envelope-only services. mongoConnection is nil in multi-tenant mode; the
// keyset/registry/audit repositories tolerate a nil connection and resolve the
// per-request tenant database from context, mirroring the holder/instrument repos.
//
// metricsFactory may be nil when telemetry is not yet wired at this stage of
// bootstrap; the protection metrics seam is nil-safe and degrades to a no-op emitter.
//
// multiTenant is the EFFECTIVE tenant mode resolved by initCRM from Options (not
// cfg.MultiTenantEnabled), so the keyset-manager and envelope-provisioning
// namespace mode always matches the CRM repo mode the dispatcher selected.
func initCRMEncryption(ctx context.Context, cfg *Config, mongoConnection *libMongo.Client, multiTenant bool, metricsFactory *metrics.MetricsFactory, logger libLog.Logger) (*crmEncryption, error) {
	kms, err := initKMS(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	legacyCrypto, err := initLegacyCrypto(cfg, kms, logger)
	if err != nil {
		return nil, err
	}

	keysetRepo, registryRepo, auditRepo, auditWriter, err := initEncryptionRepos(kms, mongoConnection, logger)
	if err != nil {
		return nil, err
	}

	wired := wireEncryptionServices(wireEncryptionServicesInput{
		mode:             kms.Mode.String(),
		vaultClient:      kms.VaultClient,
		keysetRepo:       keysetRepo,
		registryRepo:     registryRepo,
		auditWriter:      auditWriter,
		legacyCrypto:     legacyCrypto,
		metricsFactory:   metricsFactory,
		vaultMountPath:   cfg.VaultMountPath,
		multiTenant:      multiTenant,
		legacyAESHexKey:  cfg.CrmEncryptSecretKey,
		legacyHMACSecret: cfg.CrmHashSecretKey,
	})
	if wired.err != nil {
		return nil, fmt.Errorf("failed to wire encryption services: %w", wired.err)
	}

	return &crmEncryption{
		fieldEncryptor:      encryption.NewFieldEncryptorAdapter(wired.encryptionService),
		provisioningService: wired.provisioningService,
		auditRepo:           auditRepo,
		vaultClient:         kms.VaultClient,
		mode:                kms.Mode,
	}, nil
}

// initKMS resolves the encryption mode, validates configuration, and initializes
// the Vault client for envelope mode.
func initKMS(ctx context.Context, cfg *Config, logger libLog.Logger) (*kmsResult, error) {
	mode, err := resolveEncryptionMode(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve encryption mode: %w", err)
	}

	if err := validateVaultConfig(mode, cfg); err != nil {
		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, "Encryption mode resolved",
		libLog.String("mode", mode.String()))

	result := &kmsResult{Mode: mode}

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

	if err := client.Login(ctx); err != nil {
		return nil, fmt.Errorf("failed to authenticate with vault: %w", err)
	}

	logger.Log(ctx, libLog.LevelInfo, "Vault client initialized",
		libLog.String("base_mount_path", resolveBaseMountPath(cfg.VaultMountPath)))

	return client, nil
}

// resolveEncryptionMode determines the encryption mode from the configuration.
// Returns EncryptionModeLegacy when KMSVendor is empty or "none", and
// EncryptionModeEnvelope when KMSVendor is "hashicorp-vault".
func resolveEncryptionMode(cfg *Config) (crypto.EncryptionMode, error) {
	resolver := crypto.NewModeResolver(func(key string) (string, bool) {
		if key == crypto.EnvKMSVendor {
			if cfg.KMSVendor != "" {
				return cfg.KMSVendor, true
			}

			value, ok := os.LookupEnv(key)

			return value, ok
		}

		return "", false
	})

	return resolver.Resolve()
}

// validateVaultConfig validates the Vault configuration for envelope mode.
// Legacy mode skips validation.
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

// buildVaultConfig creates a vault.Config from the bootstrap Config. Authentication
// is driven solely by KMS_VAULT_AUTH_METHOD: "token" uses the dev root token (local
// only, rejected for saas/byoc), "approle" uses role_id/secret_id, and any other
// value fails closed.
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

// resolveVaultAuth determines the Vault auth method and token from
// KMS_VAULT_AUTH_METHOD, failing closed for unset/invalid methods. Token auth
// returns the dev root token ONLY when DEPLOYMENT_MODE resolves to local; every
// other deployment mode (saas, byoc, or any unrecognized value) is rejected so an
// unset or typo'd DEPLOYMENT_MODE can never fall through to the dev root token.
func resolveVaultAuth(cfg *Config) (vault.AuthMethod, string, error) {
	method, err := vault.ParseAuthMethod(cfg.VaultAuthMethod)
	if err != nil {
		return "", "", fmt.Errorf("KMS_VAULT_AUTH_METHOD must be set to a valid value: %w", err)
	}

	switch method {
	case vault.AuthMethodToken:
		if !isLocalDeployment(cfg.DeploymentMode) {
			return "", "", fmt.Errorf(
				"KMS_VAULT_AUTH_METHOD=token is only allowed when DEPLOYMENT_MODE=local (got %q): use approle",
				ResolveDeploymentMode(cfg.DeploymentMode))
		}

		return vault.AuthMethodToken, DefaultVaultDevToken, nil
	case vault.AuthMethodAppRole:
		return vault.AuthMethodAppRole, "", nil
	default:
		return "", "", fmt.Errorf("unsupported vault auth method %q", method)
	}
}

// isLocalDeployment reports whether the deployment mode resolves to local, the
// only mode where token auth with the dev root token is permitted. Empty or
// whitespace input resolves to the default (local) via ResolveDeploymentMode;
// any unrecognized value resolves to its lowercased form and is rejected.
func isLocalDeployment(deploymentMode string) bool {
	return ResolveDeploymentMode(deploymentMode) == DeploymentModeLocal
}

// resolveBaseMountPath resolves the base Vault Transit mount, trimming surrounding
// slashes/whitespace. Empty/whitespace/slash-only input falls back to "transit".
func resolveBaseMountPath(configured string) string {
	const cut = "/ \t\n"

	trimmed := strings.Trim(configured, cut)
	if trimmed == "" {
		return defaultKEKMountPath
	}

	return trimmed
}

// initLegacyCrypto builds the LegacyCrypto for the active KMS mode: envelope mode
// uses Tink-backed LegacyKeyMaterial (for reading legacy data during migration),
// legacy mode uses lib-commons crypto directly.
func initLegacyCrypto(cfg *Config, kms *kmsResult, logger libLog.Logger) (encryption.LegacyCrypto, error) {
	if kms.Mode.IsEnvelope() {
		legacyKeys, err := encryption.NewLegacyKeyMaterial(cfg.CrmEncryptSecretKey, cfg.CrmHashSecretKey)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize legacy key material: %w", err)
		}

		return legacyKeys, nil
	}

	cipher := &libCrypto.Crypto{
		HashSecretKey:    cfg.CrmHashSecretKey,
		EncryptSecretKey: cfg.CrmEncryptSecretKey,
		Logger:           logger,
	}
	if err := cipher.InitializeCipher(); err != nil {
		return nil, fmt.Errorf("failed to initialize legacy crypto cipher: %w", err)
	}

	return cipher, nil
}

// initEncryptionRepos constructs the envelope-only encryption repositories, the
// read-side audit Repository, and a repository-backed AuditWriter. In legacy mode
// it returns nil for all of them. A single auditRepo instance backs both the read
// path (returned directly) and the write path (wrapped by NewAuditWriter).
func initEncryptionRepos(
	kms *kmsResult,
	mongoConnection *libMongo.Client,
	logger libLog.Logger,
) (encryption.KeysetRepository, mongoEncryption.RegistryRepository, mongoAudit.Repository, encryption.AuditWriter, error) {
	if !kms.Mode.IsEnvelope() {
		return nil, nil, nil, nil, nil
	}

	keysetRepo, err := mongoEncryption.NewKeysetMongoDBRepository(mongoConnection)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to initialize keyset repository: %w", err)
	}

	registryRepo, err := mongoEncryption.NewRegistryMongoDBRepository(mongoConnection)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to initialize registry repository: %w", err)
	}

	auditRepo, err := mongoAudit.NewMongoDBRepository(mongoConnection)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to initialize audit repository: %w", err)
	}

	logger.Log(context.Background(), libLog.LevelInfo, "Encryption repositories initialized for envelope mode")

	return keysetRepo, registryRepo, auditRepo, encryption.NewAuditWriter(auditRepo, logger), nil
}

// wireEncryptionServicesInput contains all dependencies for wiring encryption services.
type wireEncryptionServicesInput struct {
	mode             string
	vaultClient      *vault.Client
	keysetRepo       encryption.KeysetRepository
	registryRepo     mongoEncryption.RegistryRepository
	auditWriter      encryption.AuditWriter
	legacyCrypto     encryption.LegacyCrypto
	metricsFactory   *metrics.MetricsFactory
	vaultMountPath   string
	multiTenant      bool
	legacyAESHexKey  string
	legacyHMACSecret string
}

// wireEncryptionServicesOutput contains the wired encryption services.
type wireEncryptionServicesOutput struct {
	encryptionService   encryption.EncryptionService
	provisioningService encryption.ProvisioningService
	err                 error
}

// wireEncryptionServices wires up the encryption services based on the encryption
// mode. Legacy mode wires an EncryptionService backed by lib-commons crypto (no
// Tink, no keyset manager, no provisioning). Envelope mode validates the vault
// client and keyset/registry repositories, then wires the Tink-backed keyset
// wrapper/factory, ProvisioningService, KeysetManager, and EncryptionService.
func wireEncryptionServices(input wireEncryptionServicesInput) wireEncryptionServicesOutput {
	pm := encryption.NewProtectionMetrics(input.metricsFactory)

	if strings.EqualFold(input.mode, crypto.EncryptionModeLegacy.String()) {
		protectionStateResolver := encryption.NewProtectionStateResolver(nil, pm)
		encryptionService := encryption.NewEncryptionService(
			protectionStateResolver,
			nil,
			nil,
			input.legacyCrypto,
			pm,
			crypto.EncryptionModeLegacy,
		)

		return wireEncryptionServicesOutput{encryptionService: encryptionService}
	}

	if input.vaultClient == nil {
		return wireEncryptionServicesOutput{err: fmt.Errorf("envelope encryption requires vault client")}
	}

	if input.keysetRepo == nil {
		return wireEncryptionServicesOutput{err: fmt.Errorf("envelope encryption requires keyset repository")}
	}

	if input.registryRepo == nil {
		return wireEncryptionServicesOutput{err: fmt.Errorf("envelope encryption requires registry repository")}
	}

	baseMountPath := resolveBaseMountPath(input.vaultMountPath)

	protectionStateResolver := encryption.NewProtectionStateResolver(input.registryRepo, pm)
	keysetWrapper := tink.NewKeysetWrapper(input.vaultClient)
	keysetFactory := tink.NewKeysetFactory(input.vaultClient)

	provisioningService := encryption.NewProvisioningService(
		input.keysetRepo,
		input.registryRepo,
		&keysetGeneratorAdapter{
			factory:          keysetFactory,
			legacyAESHexKey:  input.legacyAESHexKey,
			legacyHMACSecret: input.legacyHMACSecret,
		},
		encryption.ProvisioningConfig{KEKMountPath: baseMountPath, MultiTenant: input.multiTenant},
		input.auditWriter,
		pm,
		protectionStateResolver,
	)

	keysetManagerConfig := encryption.DefaultKeysetManagerConfig()
	keysetManagerConfig.BaseMountPath = baseMountPath
	keysetManagerConfig.MultiTenant = input.multiTenant

	keysetManager := encryption.NewKeysetManager(
		input.keysetRepo,
		keysetWrapper,
		provisioningService,
		keysetManagerConfig,
		pm,
	)

	encryptionService := encryption.NewEncryptionService(
		protectionStateResolver,
		keysetManager,
		input.keysetRepo,
		input.legacyCrypto,
		pm,
		crypto.EncryptionModeEnvelope,
	)

	return wireEncryptionServicesOutput{
		encryptionService:   encryptionService,
		provisioningService: provisioningService,
	}
}

// keysetGeneratorAdapter adapts tink.KeysetFactory to encryption.KeysetGenerator,
// delegating to the underlying factory. legacyAESHexKey and legacyHMACSecret hold
// the process-level legacy key material supplied verbatim to the migration (mixed)
// generation methods.
type keysetGeneratorAdapter struct {
	factory          *tink.KeysetFactory
	legacyAESHexKey  string
	legacyHMACSecret string
}

// GenerateAEADKeyset generates a new AEAD keyset and wraps it with the KMS.
func (a *keysetGeneratorAdapter) GenerateAEADKeyset(ctx context.Context, mountPath, keyName string) (tink.KeysetBundle, error) {
	return a.factory.GenerateAEADKeyset(ctx, mountPath, keyName)
}

// GeneratePRFKeyset generates a new PRF keyset (search tokens) and wraps it with the KMS.
func (a *keysetGeneratorAdapter) GeneratePRFKeyset(ctx context.Context, mountPath, keyName string) (tink.KeysetBundle, error) {
	return a.factory.GeneratePRFKeyset(ctx, mountPath, keyName)
}

// GenerateMixedAEADKeyset composes the process-level legacy AES-GCM key with a fresh
// primary key and wraps the composite via the KMS.
func (a *keysetGeneratorAdapter) GenerateMixedAEADKeyset(ctx context.Context, mountPath, keyName, legacyHexKey string) (tink.KeysetBundle, error) {
	if legacyHexKey == "" {
		legacyHexKey = a.legacyAESHexKey
	}

	return a.factory.GenerateMixedAEADKeyset(ctx, mountPath, keyName, legacyHexKey)
}

// GenerateMixedPRFKeyset composes the process-level legacy HMAC key with a fresh
// primary PRF key and wraps the composite via the KMS.
func (a *keysetGeneratorAdapter) GenerateMixedPRFKeyset(ctx context.Context, mountPath, keyName, legacySecret string) (tink.KeysetBundle, error) {
	if legacySecret == "" {
		legacySecret = a.legacyHMACSecret
	}

	return a.factory.GenerateMixedPRFKeyset(ctx, mountPath, keyName, legacySecret)
}
