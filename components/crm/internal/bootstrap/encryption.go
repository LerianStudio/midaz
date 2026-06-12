// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/LerianStudio/lib-commons/v5/commons/opentelemetry/metrics"
	mongoEncryption "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services/encryption"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/kms/vault"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
)

// defaultKEKMountPath is the default Vault Transit mount path for KEK operations.
const defaultKEKMountPath = "transit"

// resolveBaseMountPath resolves the base Vault Transit mount, trimming surrounding
// slashes/whitespace so the returned base equals the effective mount used downstream.
// Empty/whitespace/slash-only input falls back to the default "transit"; never blank.
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
	allowGracefulDegrade bool
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
//   - Validates that all required dependencies are available.
//   - Wires ProtectionStateResolver with RegistryRepository.
//   - Wires KeysetManager with KeysetRepository and VaultKeysetUnwrapper.
//   - Wires EncryptionService with all dependencies.
//   - Wires ProvisioningService with all dependencies.
//
// Graceful degradation (allowGracefulDegrade == true):
//   - When envelope mode is requested but Vault is unavailable,
//     returns nil services with degradedToLegacy=true instead of error.
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
		&keysetGeneratorAdapter{factory: keysetFactory},
		encryption.ProvisioningConfig{KEKMountPath: baseMountPath},
		input.auditWriter,
		pm,
	)

	// Wire KeysetManager with KeysetRepository, VaultKeysetUnwrapper, and ProvisioningService.
	// Tenant ID for auto-provisioning is obtained from context. The base mount is
	// injected so per-tenant sub-mounts resolve consistently with provisioning.
	keysetManagerConfig := encryption.DefaultKeysetManagerConfig()
	keysetManagerConfig.BaseMountPath = baseMountPath

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
type keysetGeneratorAdapter struct {
	factory *tink.KeysetFactory
}

// GenerateAEADKeyset generates a new AEAD keyset and wraps it with the KMS.
// The per-tenant mountPath is forwarded verbatim to the underlying factory.
func (a *keysetGeneratorAdapter) GenerateAEADKeyset(ctx context.Context, mountPath, keyName string) (tink.KeysetBundle, error) {
	return a.factory.GenerateAEADKeyset(ctx, mountPath, keyName)
}

// GenerateMACKeyset generates a new MAC keyset and wraps it with the KMS.
// The per-tenant mountPath is forwarded verbatim to the underlying factory.
func (a *keysetGeneratorAdapter) GenerateMACKeyset(ctx context.Context, mountPath, keyName string) (tink.KeysetBundle, error) {
	return a.factory.GenerateMACKeyset(ctx, mountPath, keyName)
}
