// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"

	mongoEncryption "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services/encryption"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/kms/vault"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
)

// defaultKEKMountPath is the default Vault Transit mount path for KEK operations.
const defaultKEKMountPath = "transit"

// wireEncryptionServicesInput contains all dependencies for wiring encryption services.
// For testing with mocks, use testWireEncryptionServicesWithMocks in encryption_test.go.
type wireEncryptionServicesInput struct {
	mode                 string
	vaultClient          *vault.Client
	keysetRepo           mongoEncryption.KeysetRepository
	registryRepo         mongoEncryption.RegistryRepository
	legacyKeys           *encryption.LegacyKeyMaterial
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
	// Legacy mode: wire EncryptionService with nil dependencies for legacy-only operation.
	// ProtectionStateResolver with nil registryRepo returns legacy readable state.
	// KeysetManager is nil since no envelope encryption is available.
	if strings.EqualFold(input.mode, "legacy") {
		protectionStateResolver := encryption.NewProtectionStateResolver(nil)
		encryptionService := encryption.NewEncryptionService(
			protectionStateResolver,
			nil,              // No keyset manager in legacy mode
			nil,              // No keyset repo in legacy mode
			input.legacyKeys, // Tink-backed legacy keys for legacy encryption
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

	// Resolve Vault mount path with default
	vaultMountPath := input.vaultMountPath
	if vaultMountPath == "" {
		vaultMountPath = defaultKEKMountPath
	}

	// Wire ProtectionStateResolver with RegistryRepository
	protectionStateResolver := encryption.NewProtectionStateResolver(input.registryRepo)

	// Create Tink keyset wrapper using Vault client as KMS
	keysetWrapper := tink.NewKeysetWrapper(input.vaultClient)

	// Create Tink keyset factory for provisioning
	keysetFactory := tink.NewKeysetFactory(input.vaultClient)

	// Wire ProvisioningService FIRST (required by KeysetManager for lazy provisioning)
	provisioningService := encryption.NewProvisioningService(
		input.keysetRepo,
		input.registryRepo,
		&keysetGeneratorAdapter{factory: keysetFactory},
		encryption.ProvisioningConfig{KEKMountPath: vaultMountPath},
	)

	// Wire KeysetManager with KeysetRepository, VaultKeysetUnwrapper, and ProvisioningService
	// Tenant ID for auto-provisioning is obtained from context
	keysetManager := encryption.NewKeysetManager(
		input.keysetRepo,
		keysetWrapper,
		provisioningService,
		encryption.DefaultKeysetManagerConfig(),
	)

	// Wire EncryptionService with all dependencies
	// Pass EncryptionModeEnvelope as globalMode to enable lazy provisioning
	// via KeysetManager for all organizations, regardless of their registry state
	encryptionService := encryption.NewEncryptionService(
		protectionStateResolver,
		keysetManager,
		input.keysetRepo,
		input.legacyKeys,
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
func (a *keysetGeneratorAdapter) GenerateAEADKeyset(ctx context.Context, keyName string) (tink.KeysetBundle, error) {
	return a.factory.GenerateAEADKeyset(ctx, keyName)
}

// GenerateMACKeyset generates a new MAC keyset and wraps it with the KMS.
func (a *keysetGeneratorAdapter) GenerateMACKeyset(ctx context.Context, keyName string) (tink.KeysetBundle, error) {
	return a.factory.GenerateMACKeyset(ctx, keyName)
}
