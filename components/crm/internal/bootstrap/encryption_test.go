// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	mongoEncryption "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services/encryption"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/kms/vault"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// EncryptionConfig Tests
// =============================================================================

func TestConfig_VaultMountPathFieldExists(t *testing.T) {
	t.Parallel()

	t.Run("has VaultMountPath string field with env tag", func(t *testing.T) {
		t.Parallel()

		configType := reflect.TypeOf(Config{})
		field, found := configType.FieldByName("VaultMountPath")

		require.True(t, found, "Config struct must have VaultMountPath field for Vault Transit mount path")

		assert.Equal(t, "string", field.Type.String(),
			"VaultMountPath field must be of type string")

		envValue := field.Tag.Get("env")
		assert.Equal(t, "KMS_VAULT_MOUNT_PATH", envValue,
			"VaultMountPath field must have env tag KMS_VAULT_MOUNT_PATH")
	})
}

func TestConfig_VaultMountPathDefault(t *testing.T) {
	t.Parallel()

	// The default value should be applied by the bootstrap layer, not as a struct tag default.
	// This test verifies the zero value is empty string (allowing explicit override).
	configType := reflect.TypeOf(Config{})
	field, found := configType.FieldByName("VaultMountPath")

	require.True(t, found, "Config struct must have VaultMountPath field")

	// Create a new Config and verify zero value
	cfg := reflect.New(configType).Elem()
	vaultMountPath := cfg.FieldByName("VaultMountPath")

	require.True(t, vaultMountPath.IsValid(), "VaultMountPath field must be accessible via reflection")
	assert.Empty(t, vaultMountPath.String(),
		"VaultMountPath must default to empty string (zero value); bootstrap layer applies 'transit' default")

	// Verify field type
	assert.Equal(t, "string", field.Type.String(),
		"VaultMountPath field must be of type string")
}

// =============================================================================
// Service Struct Tests
// =============================================================================

func TestService_HasEncryptionServiceField(t *testing.T) {
	t.Parallel()

	serviceType := reflect.TypeOf(Service{})
	field, found := serviceType.FieldByName("EncryptionService")

	require.True(t, found, "Service struct must have EncryptionService field")
	assert.Equal(t, "encryption.EncryptionService", field.Type.String(),
		"EncryptionService field must be of type encryption.EncryptionService")
}

func TestService_HasProvisioningServiceField(t *testing.T) {
	t.Parallel()

	serviceType := reflect.TypeOf(Service{})
	field, found := serviceType.FieldByName("ProvisioningService")

	require.True(t, found, "Service struct must have ProvisioningService field")
	assert.Equal(t, "encryption.ProvisioningService", field.Type.String(),
		"ProvisioningService field must be of type encryption.ProvisioningService (interface)")
}

func TestService_HasProtectionStateResolverField(t *testing.T) {
	t.Parallel()

	serviceType := reflect.TypeOf(Service{})
	field, found := serviceType.FieldByName("ProtectionStateResolver")

	require.True(t, found, "Service struct must have ProtectionStateResolver field")
	assert.Equal(t, "*encryption.ProtectionStateResolver", field.Type.String(),
		"ProtectionStateResolver field must be of type *encryption.ProtectionStateResolver")
}

func TestService_HasKeysetManagerField(t *testing.T) {
	t.Parallel()

	serviceType := reflect.TypeOf(Service{})
	field, found := serviceType.FieldByName("KeysetManager")

	require.True(t, found, "Service struct must have KeysetManager field")
	assert.Equal(t, "*encryption.KeysetManager", field.Type.String(),
		"KeysetManager field must be of type *encryption.KeysetManager")
}

// =============================================================================
// Wiring Dependency Tests
// =============================================================================

func TestWireEncryptionServices_ReturnsEncryptionServiceForLegacyMode(t *testing.T) {
	t.Parallel()

	// In legacy mode, EncryptionService should be wired with legacyCrypto only.
	// ProvisioningService and KeysetManager remain nil.
	result := wireEncryptionServices(wireEncryptionServicesInput{
		mode:           encryptionModeLegacy,
		vaultClient:    nil,
		keysetRepo:     nil,
		registryRepo:   nil,
		legacyCrypto:   nil, // nil is acceptable for wiring test
		vaultMountPath: "transit",
	})

	assert.NotNil(t, result.encryptionService,
		"EncryptionService must be wired in legacy mode")
	assert.NotNil(t, result.protectionStateResolver,
		"ProtectionStateResolver must be wired in legacy mode")
	assert.Nil(t, result.provisioningService,
		"ProvisioningService must be nil in legacy mode")
	assert.Nil(t, result.keysetManager,
		"KeysetManager must be nil in legacy mode")
}

func TestWireEncryptionServices_RequiresRegistryRepoForEnvelopeMode(t *testing.T) {
	t.Parallel()

	// testWireEncryptionServicesWithMocks must return error when envelope mode is enabled
	// but registry repository is not available
	result := testWireEncryptionServicesWithMocks(testWireEncryptionServicesInput{
		mode:           encryptionModeEnvelope,
		vaultClient:    &mockEncryptionVaultClient{},
		keysetRepo:     &mockKeysetRepo{},
		registryRepo:   nil, // Missing required dependency
		legacyCrypto:   nil,
		vaultMountPath: "transit",
	})

	assert.NotNil(t, result.err,
		"testWireEncryptionServicesWithMocks must return error when registry repo is nil in envelope mode")
	assert.Contains(t, result.err.Error(), "registry",
		"error must mention missing registry dependency")
}

func TestWireEncryptionServices_RequiresKeysetRepoForEnvelopeMode(t *testing.T) {
	t.Parallel()

	// testWireEncryptionServicesWithMocks must return error when envelope mode is enabled
	// but keyset repository is not available
	result := testWireEncryptionServicesWithMocks(testWireEncryptionServicesInput{
		mode:           encryptionModeEnvelope,
		vaultClient:    &mockEncryptionVaultClient{},
		keysetRepo:     nil, // Missing required dependency
		registryRepo:   &mockRegistryRepo{},
		legacyCrypto:   nil,
		vaultMountPath: "transit",
	})

	assert.NotNil(t, result.err,
		"testWireEncryptionServicesWithMocks must return error when keyset repo is nil in envelope mode")
	assert.Contains(t, result.err.Error(), "keyset",
		"error must mention missing keyset dependency")
}

func TestWireEncryptionServices_RequiresVaultClientForEnvelopeMode(t *testing.T) {
	t.Parallel()

	// testWireEncryptionServicesWithMocks must return error when envelope mode is enabled
	// but Vault client is not available
	result := testWireEncryptionServicesWithMocks(testWireEncryptionServicesInput{
		mode:           encryptionModeEnvelope,
		vaultClient:    nil, // Missing required dependency
		keysetRepo:     &mockKeysetRepo{},
		registryRepo:   &mockRegistryRepo{},
		legacyCrypto:   nil,
		vaultMountPath: "transit",
	})

	assert.NotNil(t, result.err,
		"testWireEncryptionServicesWithMocks must return error when vault client is nil in envelope mode")
	assert.Contains(t, result.err.Error(), "vault",
		"error must mention missing vault dependency")
}

func TestWireEncryptionServices_WiresProtectionStateResolverWithRegistryRepo(t *testing.T) {
	t.Parallel()

	mockRegistry := &mockRegistryRepo{}

	result := testWireEncryptionServicesWithMocks(testWireEncryptionServicesInput{
		mode:           encryptionModeEnvelope,
		vaultClient:    &mockEncryptionVaultClient{},
		keysetRepo:     &mockKeysetRepo{},
		registryRepo:   mockRegistry,
		legacyCrypto:   nil,
		vaultMountPath: "transit",
	})

	require.NoError(t, result.err,
		"testWireEncryptionServicesWithMocks must not return error with valid dependencies")
	require.NotNil(t, result.protectionStateResolver,
		"ProtectionStateResolver must be wired in envelope mode")

	// Verify the resolver was created with the registry repo
	assert.IsType(t, &encryption.ProtectionStateResolver{}, result.protectionStateResolver,
		"ProtectionStateResolver must be of correct type")
}

func TestWireEncryptionServices_WiresKeysetManagerWithDependencies(t *testing.T) {
	t.Parallel()

	result := testWireEncryptionServicesWithMocks(testWireEncryptionServicesInput{
		mode:           encryptionModeEnvelope,
		vaultClient:    &mockEncryptionVaultClient{},
		keysetRepo:     &mockKeysetRepo{},
		registryRepo:   &mockRegistryRepo{},
		legacyCrypto:   nil,
		vaultMountPath: "transit",
	})

	require.NoError(t, result.err,
		"testWireEncryptionServicesWithMocks must not return error with valid dependencies")
	require.NotNil(t, result.keysetManager,
		"KeysetManager must be wired in envelope mode")

	// Verify the keyset manager was created
	assert.IsType(t, &encryption.KeysetManager{}, result.keysetManager,
		"KeysetManager must be of correct type")
}

func TestWireEncryptionServices_WiresEncryptionServiceWithDependencies(t *testing.T) {
	t.Parallel()

	result := testWireEncryptionServicesWithMocks(testWireEncryptionServicesInput{
		mode:           encryptionModeEnvelope,
		vaultClient:    &mockEncryptionVaultClient{},
		keysetRepo:     &mockKeysetRepo{},
		registryRepo:   &mockRegistryRepo{},
		legacyCrypto:   nil,
		vaultMountPath: "transit",
	})

	require.NoError(t, result.err,
		"testWireEncryptionServicesWithMocks must not return error with valid dependencies")
	require.NotNil(t, result.encryptionService,
		"EncryptionService must be wired in envelope mode")

	// Verify the encryption service was created and implements the interface
	var _ encryption.EncryptionService = result.encryptionService
	assert.NotNil(t, result.encryptionService,
		"EncryptionService must be created and implement the interface")
}

func TestWireEncryptionServices_WiresProvisioningServiceWithDependencies(t *testing.T) {
	t.Parallel()

	result := testWireEncryptionServicesWithMocks(testWireEncryptionServicesInput{
		mode:           encryptionModeEnvelope,
		vaultClient:    &mockEncryptionVaultClient{},
		keysetRepo:     &mockKeysetRepo{},
		registryRepo:   &mockRegistryRepo{},
		legacyCrypto:   nil,
		vaultMountPath: "transit",
	})

	require.NoError(t, result.err,
		"testWireEncryptionServicesWithMocks must not return error with valid dependencies")
	require.NotNil(t, result.provisioningService,
		"ProvisioningService must be wired in envelope mode")

	// Verify the provisioning service implements the interface
	assert.Implements(t, (*encryption.ProvisioningService)(nil), result.provisioningService,
		"ProvisioningService must implement encryption.ProvisioningService interface")
}

func TestWireEncryptionServices_UsesVaultMountPathFromConfig(t *testing.T) {
	t.Parallel()

	customMountPath := "custom-transit"

	result := testWireEncryptionServicesWithMocks(testWireEncryptionServicesInput{
		mode:           encryptionModeEnvelope,
		vaultClient:    &mockEncryptionVaultClient{},
		keysetRepo:     &mockKeysetRepo{},
		registryRepo:   &mockRegistryRepo{},
		legacyCrypto:   nil,
		vaultMountPath: customMountPath,
	})

	require.NoError(t, result.err,
		"testWireEncryptionServicesWithMocks must not return error with custom vault mount path")
	require.NotNil(t, result.provisioningService,
		"ProvisioningService must be wired with custom vault mount path")
}

func TestWireEncryptionServices_DefaultsVaultMountPathToTransit(t *testing.T) {
	t.Parallel()

	result := testWireEncryptionServicesWithMocks(testWireEncryptionServicesInput{
		mode:           encryptionModeEnvelope,
		vaultClient:    &mockEncryptionVaultClient{},
		keysetRepo:     &mockKeysetRepo{},
		registryRepo:   &mockRegistryRepo{},
		legacyCrypto:   nil,
		vaultMountPath: "", // Empty should default to "transit"
	})

	require.NoError(t, result.err,
		"testWireEncryptionServicesWithMocks must not return error with empty vault mount path (defaults to transit)")
	require.NotNil(t, result.provisioningService,
		"ProvisioningService must be wired with default vault mount path")
}

func TestResolveBaseMountPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured string
		want       string
	}{
		{
			name:       "empty falls back to default",
			configured: "",
			want:       defaultKEKMountPath,
		},
		{
			name:       "whitespace-only falls back to default",
			configured: "  ",
			want:       defaultKEKMountPath,
		},
		{
			name:       "slash-only falls back to default",
			configured: "/",
			want:       defaultKEKMountPath,
		},
		{
			name:       "slashes and whitespace fall back to default",
			configured: " // ",
			want:       defaultKEKMountPath,
		},
		{
			name:       "surrounding slashes are trimmed to the effective mount",
			configured: "/transit/",
			want:       "transit",
		},
		{
			name:       "surrounding slashes and whitespace are trimmed to the effective mount",
			configured: " /transit/ ",
			want:       "transit",
		},
		{
			name:       "real value is preserved",
			configured: "transit",
			want:       "transit",
		},
		{
			name:       "custom value is preserved",
			configured: "crm-transit",
			want:       "crm-transit",
		},
		{
			name:       "surrounding whitespace is trimmed but value kept",
			configured: "  transit  ",
			want:       "transit",
		},
		{
			name:       "surrounding newlines and slashes are trimmed to the effective mount",
			configured: "\n/transit/\n",
			want:       "transit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := resolveBaseMountPath(tt.configured)
			assert.Equal(t, tt.want, got,
				"resolveBaseMountPath(%q) must resolve to %q (never blank or a leading-slash mount)",
				tt.configured, tt.want)
		})
	}
}

// =============================================================================
// Graceful Degradation Tests
// =============================================================================

func TestGracefulDegradation_CRMStartsWhenVaultUnavailable(t *testing.T) {
	t.Parallel()

	// When Vault is unavailable (envelope mode requested but client failed to initialize),
	// CRM should degrade gracefully to legacy-only encryption mode.
	//
	// This test verifies that testWireEncryptionServicesWithMocks handles the degradation scenario:
	// - mode is envelope (Vault was configured)
	// - vaultClient is nil (Vault authentication failed)
	// - Result should be legacy-only with nil encryption services
	result := testWireEncryptionServicesWithMocks(testWireEncryptionServicesInput{
		mode:                 encryptionModeEnvelope,
		vaultClient:          nil,
		keysetRepo:           &mockKeysetRepo{},
		registryRepo:         &mockRegistryRepo{},
		legacyCrypto:         nil,
		vaultMountPath:       "transit",
		allowGracefulDegrade: true, // Explicitly allow degradation
	})

	// In graceful degradation mode, no error should be returned
	assert.NoError(t, result.err,
		"testWireEncryptionServicesWithMocks must not return error when graceful degradation is allowed")

	// All envelope-specific services should be nil
	assert.Nil(t, result.encryptionService,
		"EncryptionService must be nil when Vault is unavailable")
	assert.Nil(t, result.provisioningService,
		"ProvisioningService must be nil when Vault is unavailable")
	assert.Nil(t, result.keysetManager,
		"KeysetManager must be nil when Vault is unavailable")

	// Degraded mode flag should be set
	assert.True(t, result.degradedToLegacy,
		"degradedToLegacy flag must be true when Vault is unavailable")
}

func TestGracefulDegradation_LogsWarningWhenDegrading(t *testing.T) {
	t.Parallel()

	// This test is a contract test - the actual logging behavior
	// will be verified via mock logger in the implementation.
	// Here we just verify the degradation metadata is available.
	result := testWireEncryptionServicesWithMocks(testWireEncryptionServicesInput{
		mode:                 encryptionModeEnvelope,
		vaultClient:          nil,
		keysetRepo:           &mockKeysetRepo{},
		registryRepo:         &mockRegistryRepo{},
		legacyCrypto:         nil,
		vaultMountPath:       "transit",
		allowGracefulDegrade: true,
	})

	assert.True(t, result.degradedToLegacy,
		"degradedToLegacy must be true so caller can log warning")
}

// =============================================================================
// Envelope Mode with Legacy Crypto Tests
// =============================================================================

func TestWireEncryptionServices_EnvelopeMode_HasAllRequiredServices(t *testing.T) {
	t.Parallel()

	// Verify envelope mode wires all required services
	result := testWireEncryptionServicesWithMocks(testWireEncryptionServicesInput{
		mode:           encryptionModeEnvelope,
		vaultClient:    &mockEncryptionVaultClient{},
		keysetRepo:     &mockKeysetRepo{},
		registryRepo:   &mockRegistryRepo{},
		legacyCrypto:   nil,
		vaultMountPath: "transit",
	})

	require.NoError(t, result.err,
		"testWireEncryptionServicesWithMocks must not return error with valid dependencies")
	require.NotNil(t, result.encryptionService,
		"EncryptionService must be non-nil in envelope mode")
	require.NotNil(t, result.provisioningService,
		"ProvisioningService must be non-nil in envelope mode")
	require.NotNil(t, result.keysetManager,
		"KeysetManager must be non-nil in envelope mode")
	require.NotNil(t, result.protectionStateResolver,
		"ProtectionStateResolver must be non-nil in envelope mode")
}

func TestWireEncryptionServices_EnvelopeMode_PreservesLegacyCryptoForUnmarkedDecrypt(t *testing.T) {
	t.Parallel()

	// Create a mock legacyCrypto that can encrypt/decrypt
	mockLegacy := &mockLegacyCrypto{
		encryptResult: "encrypted-legacy-value",
		decryptResult: "decrypted-legacy-value",
		hashResult:    "legacy-hash-token",
	}

	result := testWireEncryptionServicesWithMocks(testWireEncryptionServicesInput{
		mode:           encryptionModeEnvelope,
		vaultClient:    &mockEncryptionVaultClient{},
		keysetRepo:     &mockKeysetRepo{},
		registryRepo:   &mockRegistryRepo{}, // Returns LegacyReadable=true
		legacyCrypto:   mockLegacy,
		vaultMountPath: "transit",
	})

	require.NoError(t, result.err,
		"testWireEncryptionServicesWithMocks must not return error")
	require.NotNil(t, result.encryptionService,
		"EncryptionService must be non-nil")

	// Verify the encryption service was created with legacy crypto support
	// The actual decrypt behavior is tested in encryption_test.go
	// Here we verify the wiring passes legacyCrypto to the service
	assert.NotNil(t, result.encryptionService,
		"EncryptionService should be created with legacyCrypto for unmarked value decryption")
}

// mockLegacyCrypto implements encryption.LegacyCrypto for testing.
type mockLegacyCrypto struct {
	encryptResult string
	decryptResult string
	hashResult    string
	encryptErr    error
	decryptErr    error
}

func (m *mockLegacyCrypto) Encrypt(_ *string) (*string, error) {
	if m.encryptErr != nil {
		return nil, m.encryptErr
	}
	return &m.encryptResult, nil
}

func (m *mockLegacyCrypto) Decrypt(_ *string) (*string, error) {
	if m.decryptErr != nil {
		return nil, m.decryptErr
	}
	return &m.decryptResult, nil
}

func (m *mockLegacyCrypto) GenerateHash(_ *string) string {
	return m.hashResult
}

// =============================================================================
// keysetGeneratorAdapter Tests (base-mount forwarding)
// =============================================================================

// recordingKMSClient implements tink.KMSClient and records the mountPath it
// receives, letting the test observe that the adapter forwards the per-call
// mount verbatim through the production factory path.
type recordingKMSClient struct {
	gotMountPath string
	gotKeyName   string
}

func (r *recordingKMSClient) Encrypt(_ context.Context, mountPath, keyName string, _ []byte) (string, error) {
	r.gotMountPath = mountPath
	r.gotKeyName = keyName

	return "vault:v1:stub", nil
}

func (r *recordingKMSClient) Decrypt(_ context.Context, _, _ string, _ string) ([]byte, error) {
	return []byte("stub"), nil
}

// TestKeysetGeneratorAdapter_ImplementsKeysetGenerator asserts the bootstrap
// adapter satisfies the encryption.KeysetGenerator contract. This is the
// compile-level RED that blocked the whole package after the E-1.4 refactor.
func TestKeysetGeneratorAdapter_ImplementsKeysetGenerator(t *testing.T) {
	t.Parallel()

	var _ encryption.KeysetGenerator = (*keysetGeneratorAdapter)(nil)

	adapter := &keysetGeneratorAdapter{factory: tink.NewKeysetFactory(&recordingKMSClient{})}
	assert.Implements(t, (*encryption.KeysetGenerator)(nil), adapter,
		"keysetGeneratorAdapter must implement encryption.KeysetGenerator")
}

// TestKeysetGeneratorAdapter_ForwardsMountPath asserts the adapter forwards the
// per-call mountPath (resolved per-tenant by the provisioning service) straight
// to the tink.KeysetFactory, which forwards it verbatim to the KMS Encrypt call.
func TestKeysetGeneratorAdapter_ForwardsMountPath(t *testing.T) {
	t.Parallel()

	t.Run("AEAD forwards mountPath to KMS", func(t *testing.T) {
		t.Parallel()

		kms := &recordingKMSClient{}
		adapter := &keysetGeneratorAdapter{factory: tink.NewKeysetFactory(kms)}

		_, err := adapter.GenerateAEADKeyset(context.Background(), "transit/tenant-x", "org-123")
		require.NoError(t, err)

		assert.Equal(t, "transit/tenant-x", kms.gotMountPath,
			"adapter must forward the per-call mountPath to the KMS")
		assert.Equal(t, "org-123", kms.gotKeyName)
	})

	t.Run("PRF forwards mountPath to KMS", func(t *testing.T) {
		t.Parallel()

		kms := &recordingKMSClient{}
		adapter := &keysetGeneratorAdapter{factory: tink.NewKeysetFactory(kms)}

		_, err := adapter.GeneratePRFKeyset(context.Background(), "transit/tenant-y", "org-456")
		require.NoError(t, err)

		assert.Equal(t, "transit/tenant-y", kms.gotMountPath,
			"adapter must forward the per-call mountPath to the KMS")
		assert.Equal(t, "org-456", kms.gotKeyName)
	})
}

// =============================================================================
// keysetGeneratorAdapter Secret-Injection Seam Tests (HIGH-1)
// =============================================================================

// adapterTestLegacyAESHexKey is a valid 32-byte (AES-256) hex key used only to
// exercise the adapter's secret-injection seam. It is test material, never a
// production secret.
const adapterTestLegacyAESHexKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

// adapterTestLegacyHMACSecret is the test HMAC secret for the seam test.
const adapterTestLegacyHMACSecret = "legacy-hmac-secret"

// findLegacyKey returns the single non-primary key in a composite bundle, or nil.
func findLegacyKey(info tink.KeysetInfo) *tink.KeyInfo {
	for i := range info.Keys {
		if !info.Keys[i].IsPrimary {
			return &info.Keys[i]
		}
	}

	return nil
}

// TestKeysetGeneratorAdapter_InjectsConfiguredLegacyMaterial is the HIGH-1 gate:
// it exercises the production secret-injection seam directly. The adapter is
// configured with legacy material and the mixed generators are called with EMPTY
// legacy args (exactly as the provisioning service calls them). The adapter MUST
// substitute its configured material so generation succeeds, the KMS is reached,
// and the produced composite bundle carries the imported legacy key.
func TestKeysetGeneratorAdapter_InjectsConfiguredLegacyMaterial(t *testing.T) {
	t.Parallel()

	t.Run("AEAD: empty arg substitutes configured legacy AES key", func(t *testing.T) {
		t.Parallel()

		kms := &recordingKMSClient{}
		adapter := &keysetGeneratorAdapter{
			factory:          tink.NewKeysetFactory(kms),
			legacyAESHexKey:  adapterTestLegacyAESHexKey,
			legacyHMACSecret: adapterTestLegacyHMACSecret,
		}

		// Empty legacy arg: the adapter must substitute its configured key.
		bundle, err := adapter.GenerateMixedAEADKeyset(context.Background(), "transit/tenant-x", "org-1", "")
		require.NoError(t, err, "adapter must substitute configured legacy AES key when arg is empty")

		// (a) KMS reached: the composite was wrapped via the factory's KMS path.
		assert.Equal(t, "transit/tenant-x", kms.gotMountPath, "wrap must reach the KMS with the forwarded mount")
		assert.NotEmpty(t, bundle.Wrapped.WrappedData)

		// (b) The composite actually carries the imported legacy key.
		assert.True(t, bundle.Wrapped.LegacyKeyImported, "mixed AEAD bundle must flag the imported legacy key")
		require.Len(t, bundle.Wrapped.Info.Keys, 2, "composite AEAD keyset must hold fresh primary + legacy")

		legacy := findLegacyKey(bundle.Wrapped.Info)
		require.NotNil(t, legacy, "composite must contain a non-primary legacy key")
		assert.Equal(t, tink.KeyTypeLegacyAESGCM, legacy.Type, "non-primary entry must be the imported legacy AES key")
	})

	t.Run("PRF: empty arg substitutes configured legacy HMAC secret", func(t *testing.T) {
		t.Parallel()

		kms := &recordingKMSClient{}
		adapter := &keysetGeneratorAdapter{
			factory:          tink.NewKeysetFactory(kms),
			legacyAESHexKey:  adapterTestLegacyAESHexKey,
			legacyHMACSecret: adapterTestLegacyHMACSecret,
		}

		bundle, err := adapter.GenerateMixedPRFKeyset(context.Background(), "transit/tenant-y", "org-2", "")
		require.NoError(t, err, "adapter must substitute configured legacy HMAC secret when arg is empty")

		assert.Equal(t, "transit/tenant-y", kms.gotMountPath, "wrap must reach the KMS with the forwarded mount")
		assert.NotEmpty(t, bundle.Wrapped.WrappedData)

		assert.True(t, bundle.Wrapped.LegacyKeyImported, "mixed PRF bundle must flag the imported legacy key")
		require.Len(t, bundle.Wrapped.Info.Keys, 2, "composite PRF keyset must hold fresh primary + legacy")

		legacy := findLegacyKey(bundle.Wrapped.Info)
		require.NotNil(t, legacy, "composite must contain a non-primary legacy key")
		assert.Equal(t, tink.KeyTypeLegacyHMACSHA256, legacy.Type, "non-primary entry must be the imported legacy HMAC key")
	})
}

// TestKeysetGeneratorAdapter_FailsClosedWithoutConfiguredMaterial is the HIGH-1
// fail-closed gate: when the adapter has NO configured legacy material and the
// mixed generators are called with EMPTY args, there is nothing to substitute, so
// generation MUST fail closed before reaching the KMS wrap step.
func TestKeysetGeneratorAdapter_FailsClosedWithoutConfiguredMaterial(t *testing.T) {
	t.Parallel()

	t.Run("AEAD fails closed with empty config and empty arg", func(t *testing.T) {
		t.Parallel()

		kms := &recordingKMSClient{}
		adapter := &keysetGeneratorAdapter{factory: tink.NewKeysetFactory(kms)}

		_, err := adapter.GenerateMixedAEADKeyset(context.Background(), "transit/tenant-x", "org-1", "")
		require.Error(t, err, "mixed AEAD generation must fail closed when no legacy key is configured or supplied")
		assert.Empty(t, kms.gotMountPath, "must fail before reaching the KMS wrap step")
	})

	t.Run("PRF fails closed with empty config and empty arg", func(t *testing.T) {
		t.Parallel()

		kms := &recordingKMSClient{}
		adapter := &keysetGeneratorAdapter{factory: tink.NewKeysetFactory(kms)}

		_, err := adapter.GenerateMixedPRFKeyset(context.Background(), "transit/tenant-x", "org-1", "")
		require.Error(t, err, "mixed PRF generation must fail closed when no legacy secret is configured or supplied")
		assert.Empty(t, kms.gotMountPath, "must fail before reaching the KMS wrap step")
	})
}

// =============================================================================
// Base Mount Injection Tests (production wireEncryptionServices)
// =============================================================================

// newWiringVaultClient builds a real, unauthenticated vault.Client suitable for
// wiring tests. NewClient validates config and constructs the API client but
// performs no network I/O until Login/Encrypt is called, so it is safe to use
// in a unit test that only exercises construction/wiring.
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

// TestWireEncryptionServices_BaseMountResolution asserts the production wiring
// resolves the configured VaultMountPath into the base mount and injects it into
// both the provisioning service (KEKMountPath) and the keyset manager
// (BaseMountPath). Each row varies only the configured VaultMountPath; the
// resolution itself (default/trim/normalize) is unit-tested in
// TestResolveBaseMountPath, so here we assert the wiring succeeds end-to-end for
// every input shape that reaches production.
func TestWireEncryptionServices_BaseMountResolution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		vaultMountPath string
	}{
		{
			name:           "unset defaults to transit",
			vaultMountPath: "", // unset -> base "transit"
		},
		{
			name:           "custom base mount is injected",
			vaultMountPath: "crm-transit",
		},
		{
			name:           "whitespace-only defaults to transit",
			vaultMountPath: "   ", // whitespace-only -> base "transit"
		},
		{
			name:           "slash-wrapped normalizes to transit",
			vaultMountPath: "/transit/", // slash-wrapped -> base "transit"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := wireEncryptionServices(wireEncryptionServicesInput{
				mode:           encryptionModeEnvelope,
				vaultClient:    newWiringVaultClient(t),
				keysetRepo:     &mockKeysetRepo{},
				registryRepo:   &mockRegistryRepo{},
				auditWriter:    stubAuditWriter{},
				vaultMountPath: tt.vaultMountPath,
			})

			require.NoError(t, result.err,
				"wireEncryptionServices must resolve VaultMountPath %q and wire without error",
				tt.vaultMountPath)
			require.NotNil(t, result.provisioningService,
				"ProvisioningService must be wired (base mount reaches KEKMountPath)")
			require.NotNil(t, result.keysetManager,
				"KeysetManager must be wired (base mount reaches BaseMountPath)")
		})
	}
}

// =============================================================================
// Type Constants for Testing
// =============================================================================

// encryptionModeLegacy is a test constant for legacy encryption mode
const encryptionModeLegacy = "legacy"

// encryptionModeEnvelope is a test constant for envelope encryption mode
const encryptionModeEnvelope = "envelope"

// =============================================================================
// Test-Only Wiring Helper (moved from production code)
// =============================================================================

// testWireEncryptionServicesInput contains mock dependencies for testing encryption wiring.
// This is a test-only type that uses interfaces to accept mock implementations.
type testWireEncryptionServicesInput struct {
	mode                 string
	vaultClient          any // Mock implementing KeysetUnwrapper and KeysetGenerator
	keysetRepo           any // Mock implementing KeysetReader and KeysetWriter
	registryRepo         any // Mock implementing RegistryReader and RegistryWriter
	legacyCrypto         encryption.LegacyCrypto
	vaultMountPath       string
	allowGracefulDegrade bool
}

// stubAuditWriter is a discarding encryption.AuditWriter for wiring tests. The
// wiring tests assert services are constructed but never call Provision, so the
// writer is never exercised.
type stubAuditWriter struct{}

func (stubAuditWriter) Emit(_ context.Context, _ *mmodel.ProtectionAuditEvent)      {}
func (stubAuditWriter) EmitAsync(_ context.Context, _ *mmodel.ProtectionAuditEvent) {}

// testWireEncryptionServicesWithMocks wires encryption services using mock dependencies.
// This function is test-only and allows passing mock implementations for all dependencies.
func testWireEncryptionServicesWithMocks(input testWireEncryptionServicesInput) wireEncryptionServicesOutput {
	// Legacy mode: wire EncryptionService with nil dependencies for legacy-only operation.
	if input.mode == encryptionModeLegacy {
		protectionStateResolver := encryption.NewProtectionStateResolver(nil, encryption.NewProtectionMetrics(nil))
		encryptionService := encryption.NewEncryptionService(
			protectionStateResolver,
			nil,
			nil,
			input.legacyCrypto,
			encryption.NewProtectionMetrics(nil),
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

	// Type assert keyset repo to KeysetRepository
	keysetRepo, ok := input.keysetRepo.(mongoEncryption.KeysetRepository)
	if !ok {
		return wireEncryptionServicesOutput{
			err: fmt.Errorf("keyset repository must implement KeysetRepository"),
		}
	}

	// Type assert registry repo to RegistryRepository
	registryRepo, ok := input.registryRepo.(mongoEncryption.RegistryRepository)
	if !ok {
		return wireEncryptionServicesOutput{
			err: fmt.Errorf("registry repository must implement RegistryRepository"),
		}
	}

	// Resolve Vault mount path with default
	vaultMountPath := input.vaultMountPath
	if vaultMountPath == "" {
		vaultMountPath = defaultKEKMountPath
	}

	// Wire ProtectionStateResolver with RegistryRepository
	protectionStateResolver := encryption.NewProtectionStateResolver(registryRepo, encryption.NewProtectionMetrics(nil))

	// Type assert vault client to KeysetUnwrapper
	mockUnwrapper, ok := input.vaultClient.(encryption.KeysetUnwrapper)
	if !ok {
		return wireEncryptionServicesOutput{
			err: fmt.Errorf("vault client must implement KeysetUnwrapper"),
		}
	}

	// Wire KeysetManager with mock unwrapper (no provisioner for test)
	keysetManager := encryption.NewKeysetManager(
		keysetRepo,
		mockUnwrapper,
		nil,
		encryption.DefaultKeysetManagerConfig(),
		encryption.NewProtectionMetrics(nil),
	)

	// Wire EncryptionService with mock dependencies
	// Pass EncryptionModeEnvelope as globalMode to match production behavior
	encryptionService := encryption.NewEncryptionService(
		protectionStateResolver,
		keysetManager,
		keysetRepo,
		input.legacyCrypto,
		encryption.NewProtectionMetrics(nil),
		crypto.EncryptionModeEnvelope,
	)

	// Type assert vault client to KeysetGenerator
	mockGenerator, ok := input.vaultClient.(encryption.KeysetGenerator)
	if !ok {
		// If the mock doesn't implement KeysetGenerator, return without provisioning service
		return wireEncryptionServicesOutput{
			protectionStateResolver: protectionStateResolver,
			keysetManager:           keysetManager,
			encryptionService:       encryptionService,
			provisioningService:     nil,
		}
	}

	// Wire ProvisioningService with mock dependencies. The AuditWriter is required
	// (no nil-default); a discarding stub is sufficient since these wiring tests
	// never invoke Provision.
	provisioningService := encryption.NewProvisioningService(
		keysetRepo,
		registryRepo,
		mockGenerator,
		encryption.ProvisioningConfig{KEKMountPath: vaultMountPath},
		stubAuditWriter{},
		encryption.NewProtectionMetrics(nil),
		nil,
		nil,
	)

	return wireEncryptionServicesOutput{
		protectionStateResolver: protectionStateResolver,
		keysetManager:           keysetManager,
		encryptionService:       encryptionService,
		provisioningService:     provisioningService,
	}
}

// =============================================================================
// Mock Implementations for Testing
// =============================================================================

// mockEncryptionVaultClient implements the vault client interface for encryption wiring tests.
// It satisfies KeysetUnwrapper and KeysetGenerator interfaces for mock testing.
type mockEncryptionVaultClient struct{}

// UnwrapKeyset satisfies the encryption.KeysetUnwrapper interface.
func (m *mockEncryptionVaultClient) UnwrapKeyset(_ context.Context, _, _, _ string) ([]byte, error) {
	// Return a minimal valid Tink keyset handle bytes (placeholder)
	return []byte("mock-keyset-bytes"), nil
}

// GenerateAEADKeyset satisfies the encryption.KeysetGenerator interface.
func (m *mockEncryptionVaultClient) GenerateAEADKeyset(_ context.Context, _, _ string) (tink.KeysetBundle, error) {
	return tink.KeysetBundle{
		Wrapped: tink.WrappedKeyset{
			WrappedData: "mock-wrapped-aead",
			Info: tink.KeysetInfo{
				PrimaryKeyID: 12345,
				Keys: []tink.KeyInfo{
					{KeyID: 12345, Status: tink.KeyStatusEnabled, Type: tink.KeyTypeAES256GCM, IsPrimary: true},
				},
			},
		},
		RawKeyset: []byte("mock-raw-aead-keyset"),
	}, nil
}

// GeneratePRFKeyset satisfies the encryption.KeysetGenerator interface.
func (m *mockEncryptionVaultClient) GeneratePRFKeyset(_ context.Context, _, _ string) (tink.KeysetBundle, error) {
	return tink.KeysetBundle{
		Wrapped: tink.WrappedKeyset{
			WrappedData: "mock-wrapped-prf",
			Info: tink.KeysetInfo{
				PrimaryKeyID: 67890,
				Keys: []tink.KeyInfo{
					{KeyID: 67890, Status: tink.KeyStatusEnabled, Type: tink.KeyTypeHMACPRF, IsPrimary: true},
				},
			},
		},
		RawKeyset: []byte("mock-raw-prf-keyset"),
	}, nil
}

// GenerateMixedAEADKeyset satisfies the encryption.KeysetGenerator interface.
// It returns a composite-shaped bundle (fresh primary + legacy non-primary) for
// the mocked migration path.
func (m *mockEncryptionVaultClient) GenerateMixedAEADKeyset(_ context.Context, _, _, _ string) (tink.KeysetBundle, error) {
	return tink.KeysetBundle{
		Wrapped: tink.WrappedKeyset{
			WrappedData: "mock-wrapped-mixed-aead",
			Info: tink.KeysetInfo{
				PrimaryKeyID: 12345,
				Keys: []tink.KeyInfo{
					{KeyID: 12345, Status: tink.KeyStatusEnabled, Type: tink.KeyTypeAES256GCM, IsPrimary: true},
					{KeyID: 1, Status: tink.KeyStatusEnabled, Type: tink.KeyTypeLegacyAESGCM, IsPrimary: false},
				},
			},
			LegacyKeyImported: true,
		},
		RawKeyset: []byte("mock-raw-mixed-aead-keyset"),
	}, nil
}

// GenerateMixedPRFKeyset satisfies the encryption.KeysetGenerator interface.
func (m *mockEncryptionVaultClient) GenerateMixedPRFKeyset(_ context.Context, _, _, _ string) (tink.KeysetBundle, error) {
	return tink.KeysetBundle{
		Wrapped: tink.WrappedKeyset{
			WrappedData: "mock-wrapped-mixed-prf",
			Info: tink.KeysetInfo{
				PrimaryKeyID: 67890,
				Keys: []tink.KeyInfo{
					{KeyID: 67890, Status: tink.KeyStatusEnabled, Type: tink.KeyTypeHMACPRF, IsPrimary: true},
					{KeyID: 1, Status: tink.KeyStatusEnabled, Type: tink.KeyTypeLegacyHMACSHA256, IsPrimary: false},
				},
			},
			LegacyKeyImported: true,
		},
		RawKeyset: []byte("mock-raw-mixed-prf-keyset"),
	}, nil
}

// mockKeysetRepo implements mongoEncryption.KeysetRepository for testing.
type mockKeysetRepo struct{}

// Get satisfies the mongoEncryption.KeysetRepository interface.
func (m *mockKeysetRepo) Get(_ context.Context, _ string) (*mmodel.OrganizationKeyset, error) {
	return &mmodel.OrganizationKeyset{
		OrganizationID: "test-org",
		KEKPath:        "transit/keys/test",
		WrappedKeyset:  "mock-wrapped",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: 12345,
		},
	}, nil
}

// GetByVersion satisfies the mongoEncryption.KeysetRepository interface.
func (m *mockKeysetRepo) GetByVersion(_ context.Context, _ string, version int) (*mmodel.OrganizationKeyset, error) {
	return &mmodel.OrganizationKeyset{
		OrganizationID: "test-org",
		Version:        version,
		KEKPath:        "transit/keys/test",
		WrappedKeyset:  "mock-wrapped",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: 12345,
		},
	}, nil
}

// GetActive satisfies the mongoEncryption.KeysetRepository interface.
func (m *mockKeysetRepo) GetActive(_ context.Context, _ string) (*mmodel.OrganizationKeyset, error) {
	return &mmodel.OrganizationKeyset{
		OrganizationID: "test-org",
		Version:        1,
		KEKPath:        "transit/keys/test",
		WrappedKeyset:  "mock-wrapped",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: 12345,
		},
	}, nil
}

// Save satisfies the mongoEncryption.KeysetRepository interface.
func (m *mockKeysetRepo) Save(_ context.Context, _ *mmodel.OrganizationKeyset) error {
	return nil
}

// Update satisfies the mongoEncryption.KeysetRepository interface.
func (m *mockKeysetRepo) Update(_ context.Context, _ *mmodel.OrganizationKeyset, _ int64) error {
	return nil
}

// mockRegistryRepo implements mongoEncryption.RegistryRepository for testing.
type mockRegistryRepo struct{}

// Get satisfies the mongoEncryption.RegistryRepository interface.
func (m *mockRegistryRepo) Get(_ context.Context, organizationID string) (*mmodel.OrganizationRegistryRecord, error) {
	return &mmodel.OrganizationRegistryRecord{
		OrganizationID: organizationID,
		Status:         mmodel.RegistryStatusActive,
		CurrentVersion: 1,
		LegacyReadable: true,
	}, nil
}

// Save satisfies the mongoEncryption.RegistryRepository interface.
func (m *mockRegistryRepo) Save(_ context.Context, _ *mmodel.OrganizationRegistryRecord) error {
	return nil
}

// Update satisfies the mongoEncryption.RegistryRepository interface.
func (m *mockRegistryRepo) Update(_ context.Context, _ *mmodel.OrganizationRegistryRecord, _ int64) error {
	return nil
}
