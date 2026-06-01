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
	legacyCrypto         encryption.LegacyCrypto
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
	// Legacy mode: no envelope encryption services needed
	if strings.EqualFold(input.mode, "legacy") {
		return wireEncryptionServicesOutput{}
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

	// Wire KeysetManager with KeysetRepository and VaultKeysetUnwrapper
	keysetManager := encryption.NewKeysetManager(
		input.keysetRepo,
		keysetWrapper,
		encryption.DefaultKeysetManagerConfig(),
	)

	// Wire EncryptionService with all dependencies
	encryptionService := encryption.NewEncryptionService(
		protectionStateResolver,
		keysetManager,
		input.keysetRepo,
		input.legacyCrypto,
	)

	// Create Tink keyset factory for provisioning
	keysetFactory := tink.NewKeysetFactory(input.vaultClient)

	// Wire ProvisioningService with all dependencies
	provisioningService := encryption.NewProvisioningService(
		input.keysetRepo,
		input.registryRepo,
		&keysetGeneratorAdapter{factory: keysetFactory},
		encryption.ProvisioningConfig{KEKMountPath: vaultMountPath},
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
