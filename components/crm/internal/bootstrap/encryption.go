// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/LerianStudio/lib-observability/metrics"
	mongoEncryption "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services/encryption"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/kms/vault"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
)

// defaultKEKMountPath is the default Vault Transit mount path for KEK operations.
const defaultKEKMountPath = "transit"

// resolveBaseMountPath is the SINGLE base-mount normalizer: base normalization
// lives here, not in resolveMount. It resolves the base Vault Transit mount,
// trimming surrounding slashes/whitespace so the returned base equals the effective
// mount used downstream. Empty/whitespace/slash-only input falls back to the default
// "transit"; never blank. Callers inject the result as the pre-normalized base that
// resolveMount consumes verbatim.
func resolveBaseMountPath(configured string) string {
	// One cutset for guard and returned value so they cannot drift.
	const cut = "/ \t\n"

	trimmed := strings.Trim(configured, cut)
	if trimmed == "" {
		return defaultKEKMountPath
	}

	return trimmed
}

// wireEncryptionServicesInput contains all dependencies for wiring encryption services.
// For testing with mocks, use testWireEncryptionServicesWithMocks in encryption_test.go.
type wireEncryptionServicesInput struct {
	mode                 string
	vaultClient          *vault.Client
	keysetRepo           mongoEncryption.KeysetRepository
	registryRepo         mongoEncryption.RegistryRepository
	auditWriter          encryption.AuditWriter
	legacyCrypto         encryption.LegacyCrypto
	metricsFactory       *metrics.MetricsFactory
	vaultMountPath       string
	multiTenant          bool
	allowGracefulDegrade bool

	// legacyAESHexKey and legacyHMACSecret are the process-level legacy key
	// material (LCRYPTO_ENCRYPT_SECRET_KEY / LCRYPTO_HASH_SECRET_KEY) used by the
	// manual migration path to compose mixed keysets. They are threaded only into
	// the keyset generator adapter and never persisted; the migration generator
	// returns only wrapped keyset bytes. Empty values are tolerated here: mixed
	// generation fails closed at call time if requested without material.
	legacyAESHexKey  string
	legacyHMACSecret string
}

// wireEncryptionServicesOutput contains the wired encryption services.
type wireEncryptionServicesOutput struct {
	protectionStateResolver *encryption.ProtectionStateResolver
	keysetManager           *encryption.KeysetManager
	encryptionService       encryption.EncryptionService
	provisioningService     encryption.ProvisioningService
	degradedToLegacy        bool
	err                     error
}

// wireEncryptionServices wires up the encryption services based on the encryption mode.
//
// For legacy mode (mode == "legacy"):
//   - Returns nil services; the application uses legacy libCommons crypto only.
//
// For envelope mode (mode == "envelope"):
//   - Validates that the vault client, keyset repository, and registry
//     repository are all available.
//   - Wires ProtectionStateResolver with RegistryRepository.
//   - Builds the Tink keyset wrapper and keyset factory backed by the Vault
//     client as KMS.
//   - Wires ProvisioningService (keyset/registry repos, keyset generator
//     adapter, audit writer) BEFORE KeysetManager, which depends on it for
//     lazy provisioning.
//   - Wires KeysetManager with the keyset repository, keyset wrapper, and
//     ProvisioningService.
//   - Wires EncryptionService with all dependencies.
//
// Graceful degradation (allowGracefulDegrade == true):
//   - Applies ONLY when the Vault client is nil: returns nil services with
//     degradedToLegacy=true instead of an error. A missing keyset or registry
//     repository still returns an error regardless of this flag.
//
// For testing with mock dependencies, use testWireEncryptionServicesWithMocks in encryption_test.go.
func wireEncryptionServices(input wireEncryptionServicesInput) wireEncryptionServicesOutput {
	// Build the nil-safe protection metrics seam ONCE from the (possibly nil)
	// metrics factory and thread it into every constructor below. A nil factory
	// (telemetry disabled) yields a no-op emitter.
	pm := encryption.NewProtectionMetrics(input.metricsFactory)

	// Legacy mode: wire EncryptionService with nil dependencies for legacy-only operation.
	// ProtectionStateResolver with nil registryRepo returns legacy readable state.
	// KeysetManager is nil since no envelope encryption is available.
	// Uses lib-commons crypto directly (no Tink).
	if strings.EqualFold(input.mode, "legacy") {
		protectionStateResolver := encryption.NewProtectionStateResolver(nil, pm)
		encryptionService := encryption.NewEncryptionService(
			protectionStateResolver,
			nil,                // No keyset manager in legacy mode
			nil,                // No keyset repo in legacy mode
			input.legacyCrypto, // lib-commons crypto for legacy encryption
			pm,
			crypto.EncryptionModeLegacy,
		)

		return wireEncryptionServicesOutput{
			protectionStateResolver: protectionStateResolver,
			encryptionService:       encryptionService,
		}
	}

	// Envelope mode: validate required dependencies
	if input.vaultClient == nil {
		if input.allowGracefulDegrade {
			// Graceful degradation: Vault unavailable, fall back to legacy-only
			return wireEncryptionServicesOutput{
				degradedToLegacy: true,
			}
		}

		return wireEncryptionServicesOutput{
			err: fmt.Errorf("envelope encryption requires vault client"),
		}
	}

	if input.keysetRepo == nil {
		return wireEncryptionServicesOutput{
			err: fmt.Errorf("envelope encryption requires keyset repository"),
		}
	}

	if input.registryRepo == nil {
		return wireEncryptionServicesOutput{
			err: fmt.Errorf("envelope encryption requires registry repository"),
		}
	}

	// Resolve the base Vault Transit mount once (see resolveBaseMountPath).
	baseMountPath := resolveBaseMountPath(input.vaultMountPath)

	// Wire ProtectionStateResolver with RegistryRepository
	protectionStateResolver := encryption.NewProtectionStateResolver(input.registryRepo, pm)

	// Create Tink keyset wrapper using Vault client as KMS
	keysetWrapper := tink.NewKeysetWrapper(input.vaultClient)

	// Create Tink keyset factory for provisioning
	keysetFactory := tink.NewKeysetFactory(input.vaultClient)

	// Wire ProvisioningService FIRST (required by KeysetManager for lazy provisioning).
	// auditWriter is envelope-only: bootstrap constructs a repository-backed writer
	// in the envelope branch and threads it in here. It is never nil in this path.
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

	// Wire KeysetManager with KeysetRepository, VaultKeysetUnwrapper, and ProvisioningService.
	// Tenant ID for auto-provisioning is obtained from context. The base mount is
	// injected so per-tenant sub-mounts resolve consistently with provisioning.
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

	// Wire EncryptionService with all dependencies
	// Pass EncryptionModeEnvelope as globalMode to enable lazy provisioning
	// via KeysetManager for all organizations, regardless of their registry state
	// Uses Tink-backed LegacyKeyMaterial (passed as LegacyCrypto) for reading legacy data during migration
	encryptionService := encryption.NewEncryptionService(
		protectionStateResolver,
		keysetManager,
		input.keysetRepo,
		input.legacyCrypto,
		pm,
		crypto.EncryptionModeEnvelope,
	)

	return wireEncryptionServicesOutput{
		protectionStateResolver: protectionStateResolver,
		keysetManager:           keysetManager,
		encryptionService:       encryptionService,
		provisioningService:     provisioningService,
	}
}

// keysetGeneratorAdapter adapts tink.KeysetFactory to encryption.KeysetGenerator.
// Since encryption.KeysetGenerator now uses tink types directly, this adapter
// simply delegates to the underlying factory.
//
// legacyAESHexKey and legacyHMACSecret hold the process-level legacy key material
// supplied verbatim to the migration (mixed) generation methods. They mirror the
// material already held by LegacyKeyMaterial; storing them here introduces no new
// long-lived secret exposure and keeps tink.KeysetFactory itself stateless.
type keysetGeneratorAdapter struct {
	factory          *tink.KeysetFactory
	legacyAESHexKey  string
	legacyHMACSecret string
}

// GenerateAEADKeyset generates a new AEAD keyset and wraps it with the KMS.
// The per-tenant mountPath is forwarded verbatim to the underlying factory.
func (a *keysetGeneratorAdapter) GenerateAEADKeyset(ctx context.Context, mountPath, keyName string) (tink.KeysetBundle, error) {
	return a.factory.GenerateAEADKeyset(ctx, mountPath, keyName)
}

// GeneratePRFKeyset generates a new PRF keyset (search tokens) and wraps it with the KMS.
// The per-tenant mountPath is forwarded verbatim to the underlying factory.
func (a *keysetGeneratorAdapter) GeneratePRFKeyset(ctx context.Context, mountPath, keyName string) (tink.KeysetBundle, error) {
	return a.factory.GeneratePRFKeyset(ctx, mountPath, keyName)
}

// GenerateMixedAEADKeyset composes the process-level legacy AES-GCM key with a
// fresh primary key and wraps the composite via the KMS. The legacy material is
// supplied from the adapter (not stored on the factory). Fails closed inside the
// factory when the material is missing or invalid.
func (a *keysetGeneratorAdapter) GenerateMixedAEADKeyset(ctx context.Context, mountPath, keyName, legacyHexKey string) (tink.KeysetBundle, error) {
	if legacyHexKey == "" {
		legacyHexKey = a.legacyAESHexKey
	}

	return a.factory.GenerateMixedAEADKeyset(ctx, mountPath, keyName, legacyHexKey)
}

// GenerateMixedPRFKeyset composes the process-level legacy HMAC key with a fresh
// primary PRF key and wraps the composite via the KMS. The legacy material is
// supplied from the adapter (not stored on the factory). Fails closed inside the
// factory when the material is missing.
func (a *keysetGeneratorAdapter) GenerateMixedPRFKeyset(ctx context.Context, mountPath, keyName, legacySecret string) (tink.KeysetBundle, error) {
	if legacySecret == "" {
		legacySecret = a.legacyHMACSecret
	}

	return a.factory.GenerateMixedPRFKeyset(ctx, mountPath, keyName, legacySecret)
}
