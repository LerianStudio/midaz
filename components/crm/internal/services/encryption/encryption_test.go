// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/tests/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Fakes and Helpers
// ---------------------------------------------------------------------------

// serviceTestRegistryRepo implements mongoEncryption.RegistryRepository for encryption service tests.
type serviceTestRegistryRepo struct {
	records map[string]*mmodel.OrganizationRegistryRecord
	err     error
}

func (f *serviceTestRegistryRepo) Get(_ context.Context, organizationID string) (*mmodel.OrganizationRegistryRecord, error) {
	if f.err != nil {
		return nil, f.err
	}

	record, ok := f.records[organizationID]
	if !ok {
		return nil, constant.ErrRegistryNotFound
	}

	return record, nil
}

func (f *serviceTestRegistryRepo) Save(_ context.Context, _ *mmodel.OrganizationRegistryRecord) error {
	return nil
}

func (f *serviceTestRegistryRepo) Update(_ context.Context, _ *mmodel.OrganizationRegistryRecord, _ int64) error {
	return nil
}

// serviceTestKeysetRepo implements mongoEncryption.KeysetRepository for encryption service tests.
type serviceTestKeysetRepo struct {
	keysets map[string]*mmodel.OrganizationKeyset
	err     error
}

func (f *serviceTestKeysetRepo) Get(_ context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	if f.err != nil {
		return nil, f.err
	}

	keyset, ok := f.keysets[organizationID]
	if !ok {
		return nil, errors.New("keyset not found")
	}

	return keyset, nil
}

func (f *serviceTestKeysetRepo) GetActive(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	return f.Get(ctx, organizationID)
}

func (f *serviceTestKeysetRepo) GetByVersion(_ context.Context, organizationID string, version int) (*mmodel.OrganizationKeyset, error) {
	if f.err != nil {
		return nil, f.err
	}

	keyset, ok := f.keysets[organizationID]
	if !ok || keyset.Version != version {
		return nil, mmodel.ErrKeysetNotFound
	}

	return keyset, nil
}

func (f *serviceTestKeysetRepo) Save(_ context.Context, _ *mmodel.OrganizationKeyset) error {
	return nil
}

func (f *serviceTestKeysetRepo) Update(_ context.Context, _ *mmodel.OrganizationKeyset, _ int64) error {
	return nil
}

// serviceTestKeysetUnwrapper implements KeysetUnwrapper for encryption service tests.
// It returns the keyset bytes based on the wrapped keyset marker.
type serviceTestKeysetUnwrapper struct {
	aeadKeyset []byte
	macKeyset  []byte
	err        error
}

func (f *serviceTestKeysetUnwrapper) UnwrapKeyset(_ context.Context, _ string, _ string, wrappedKeyset string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	// Determine which keyset to return based on the wrapped keyset value
	// In our test setup, we use "wrapped-aead" and "wrapped-hmac"
	if wrappedKeyset == "wrapped-hmac" {
		return f.macKeyset, nil
	}

	return f.aeadKeyset, nil
}

// newTestLegacyKeyMaterial creates a real LegacyKeyMaterial for testing.
// Uses the same keys as the characterization tests.
func newTestLegacyKeyMaterial(t *testing.T) *LegacyKeyMaterial {
	t.Helper()

	material, err := NewLegacyKeyMaterial(legacyEncryptHexKey, legacyHashKey)
	require.NoError(t, err)

	return material
}

// generateServiceTestKeysets creates real Tink keysets for encryption service testing.
func generateServiceTestKeysets(t *testing.T) ([]byte, []byte, uint32, uint32) {
	t.Helper()

	keysets := helpers.GenerateTinkKeysets(t)

	return keysets.AEADBytes, keysets.PRFBytes, keysets.AEADPrimaryKeyID, keysets.PRFPrimaryKeyID
}

// createEncryptionTestService creates a Service with test dependencies for envelope mode tests.
func createEncryptionTestService(t *testing.T, state ProtectionState, legacyKeys *LegacyKeyMaterial) (EncryptionService, *mmodel.OrganizationKeyset) {
	t.Helper()

	aeadBytes, prfBytes, aeadKeyID, prfKeyID := generateServiceTestKeysets(t)

	// Default the registry's readable versions to [keysetVersion] so version-routed
	// decrypt of the active marker passes the fail-closed gate, unless the test set
	// ReadableVersions explicitly (e.g. to exercise the fail-closed path).
	readableVersions := state.ReadableVersions
	if readableVersions == nil && state.CurrentKeysetVersion >= 1 {
		readableVersions = []int{state.CurrentKeysetVersion}
	}

	keyset := &mmodel.OrganizationKeyset{
		TenantID:       state.TenantID,
		OrganizationID: state.OrganizationID,
		Version:        1,
		KEKPath:        "test-kek",
		WrappedKeyset:  "wrapped-aead",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: aeadKeyID,
		},
		WrappedHMACKeyset: "wrapped-hmac",
		HMACKeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: prfKeyID, // PRF (search-token) keyset primary key ID
		},
	}

	keysetRepo := &serviceTestKeysetRepo{
		keysets: map[string]*mmodel.OrganizationKeyset{
			state.OrganizationID: keyset,
		},
	}

	unwrapper := &serviceTestKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  prfBytes,
	}

	keysetManager := NewKeysetManager(keysetRepo, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			state.OrganizationID: {
				TenantID:         state.TenantID,
				OrganizationID:   state.OrganizationID,
				Status:           mmodel.RegistryStatusActive,
				LegacyReadable:   state.CanReadLegacy,
				CurrentVersion:   state.CurrentKeysetVersion,
				ReadableVersions: readableVersions,
			},
		},
	}

	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, keysetManager, keysetRepo, legacyKeys, NewProtectionMetrics(nil))

	return svc, keyset
}

// ---------------------------------------------------------------------------
// Encrypt Tests
// ---------------------------------------------------------------------------

func TestService_Encrypt_EnvelopeMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, keyset := createEncryptionTestService(t, state, legacyKeys)

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "tax_id",
	}

	plaintext := "123-45-6789"

	ciphertext, err := svc.Encrypt(ctx, fieldCtx, plaintext)
	require.NoError(t, err)

	// Verify it has envelope marker
	assert.True(t, HasEnvelopeMarker(ciphertext), "ciphertext should have envelope marker")

	// Parse marker to verify format
	marker, hasMarker, err := ParseEnvelopeMarker(ciphertext)
	require.NoError(t, err)
	require.True(t, hasMarker)
	// The marker now carries the keyset VERSION (not the Tink primary key id).
	assert.Equal(t, uint32(keyset.Version), marker.Version)
}

func TestService_Encrypt_LegacyMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	legacyKeys := newTestLegacyKeyMaterial(t)

	// Create service with empty registry (no record = legacy mode)
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	// KeysetManager and KeysetReader can be nil for legacy mode since they won't be used
	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys, NewProtectionMetrics(nil))

	fieldCtx := FieldContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		RecordID:       "record-789",
		FieldName:      "document",
	}

	plaintext := "ABC123"

	ciphertext, err := svc.Encrypt(ctx, fieldCtx, plaintext)
	require.NoError(t, err)

	// Verify it does NOT have envelope marker
	assert.False(t, HasEnvelopeMarker(ciphertext), "legacy ciphertext should not have envelope marker")

	// Verify ciphertext can be decrypted back to original plaintext using service
	decrypted, err := svc.Decrypt(ctx, fieldCtx, ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestService_Encrypt_InvalidFieldContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	tests := []struct {
		name     string
		fieldCtx FieldContext
	}{
		{
			name: "empty tenant ID",
			fieldCtx: FieldContext{
				TenantID:       "",
				OrganizationID: "org-123",
				RecordID:       "record-456",
				FieldName:      "tax_id",
			},
		},
		{
			name: "empty organization ID",
			fieldCtx: FieldContext{
				TenantID:       "tenant-abc",
				OrganizationID: "",
				RecordID:       "record-456",
				FieldName:      "tax_id",
			},
		},
		{
			name: "empty record ID",
			fieldCtx: FieldContext{
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				RecordID:       "",
				FieldName:      "tax_id",
			},
		},
		{
			name: "empty field name",
			fieldCtx: FieldContext{
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				RecordID:       "record-456",
				FieldName:      "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := svc.Encrypt(ctx, tt.fieldCtx, "plaintext")
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrFieldContextInvalid)
		})
	}
}

func TestService_Encrypt_KeysetManagerError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create a keyset reader that returns an error
	keysetRepo := &serviceTestKeysetRepo{
		err: errors.New("keyset not found"),
	}

	unwrapper := &serviceTestKeysetUnwrapper{}

	keysetManager := NewKeysetManager(keysetRepo, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-123": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				Status:         mmodel.RegistryStatusActive,
				LegacyReadable: false,
				CurrentVersion: 1,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc := NewEncryptionService(stateResolver, keysetManager, keysetRepo, legacyKeys, NewProtectionMetrics(nil))

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "tax_id",
	}

	_, err := svc.Encrypt(ctx, fieldCtx, "plaintext")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// Decrypt Tests
// ---------------------------------------------------------------------------

func TestService_Decrypt_EnvelopeMarked(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "tax_id",
	}

	originalPlaintext := "123-45-6789"

	// Encrypt first
	ciphertext, err := svc.Encrypt(ctx, fieldCtx, originalPlaintext)
	require.NoError(t, err)
	require.True(t, HasEnvelopeMarker(ciphertext))

	// Decrypt
	decrypted, err := svc.Decrypt(ctx, fieldCtx, ciphertext)
	require.NoError(t, err)
	assert.Equal(t, originalPlaintext, decrypted)
}

func TestService_Decrypt_LegacyAllowed(t *testing.T) {
	t.Parallel()

	// An envelope-mode org with LegacyReadable=true carries any imported legacy
	// key inside its per-org composite keyset, so unmarked legacy ciphertext now
	// decrypts through the keyset (not the process-global legacyCrypto).
	factory := tink.NewKeysetFactory(identityKMS{})

	mixedAEAD, err := factory.GenerateMixedAEADKeyset(context.Background(), "transit", "crm-org-123", helperLegacyHexKey)
	require.NoError(t, err)

	mixedPRF, err := factory.GenerateMixedPRFKeyset(context.Background(), "transit", "crm-org-123", helperLegacySecret)
	require.NoError(t, err)

	km := newKeysetManagerForKeyset(t, "org-123", mixedAEAD, mixedPRF)

	// Unmarked legacy ciphertext written with the legacy hex key (nil AAD), which
	// the migrated org's composite keyset can decrypt.
	plaintext := "secret-value"
	legacyKeys := newTestLegacyKeyMaterial(t)
	encrypted, err := legacyKeys.Encrypt(&plaintext)
	require.NoError(t, err)
	require.NotNil(t, encrypted)

	// Create service with envelope mode but legacy read allowed
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-123": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				Status:         mmodel.RegistryStatusActive,
				LegacyReadable: true, // Legacy read allowed
				CurrentVersion: 1,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, km, nil, legacyKeys, NewProtectionMetrics(nil), crypto.EncryptionModeEnvelope)

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-abc")
	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "document",
	}

	// Decrypt legacy ciphertext (no envelope marker)
	decrypted, err := svc.Decrypt(ctx, fieldCtx, *encrypted)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestService_Decrypt_LegacyNotAllowed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	legacyKeys := newTestLegacyKeyMaterial(t)

	// Create actual legacy ciphertext
	cipherBytes, err := legacyKeys.aead.Encrypt([]byte("secret-value"), nil)
	require.NoError(t, err)
	legacyCiphertext := base64.StdEncoding.EncodeToString(cipherBytes)

	// Create service with envelope mode and legacy read NOT allowed
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-123": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				Status:         mmodel.RegistryStatusActive,
				LegacyReadable: false, // Legacy read NOT allowed
				CurrentVersion: 1,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys, NewProtectionMetrics(nil))

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "document",
	}

	// Try to decrypt legacy ciphertext - should fail
	_, err = svc.Decrypt(ctx, fieldCtx, legacyCiphertext)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLegacyReadNotAllowed)
}

func TestService_Decrypt_EnvelopeFailure_NoFallback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        true, // Even with legacy allowed
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, keyset := createEncryptionTestService(t, state, legacyKeys)

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "tax_id",
	}

	// Create a marked ciphertext with bogus payload for the active keyset version
	// (so it passes the readable-version gate and fails on AEAD decrypt instead).
	bogusMarkedCiphertext := FormatEnvelopeMarker(uint32(keyset.Version), []byte("invalid-ciphertext"))

	// Decryption should fail - NO fallback to legacy
	_, err := svc.Decrypt(ctx, fieldCtx, bogusMarkedCiphertext)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEnvelopeDecryptFailed)
}

func TestService_Decrypt_WrongAAD(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	// Encrypt with one field context
	encryptFieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "tax_id",
	}

	ciphertext, err := svc.Encrypt(ctx, encryptFieldCtx, "sensitive-data")
	require.NoError(t, err)

	// Try to decrypt with different field context (different AAD)
	decryptFieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "different_field", // Different field name = different AAD
	}

	_, err = svc.Decrypt(ctx, decryptFieldCtx, ciphertext)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEnvelopeDecryptFailed)
}

// ---------------------------------------------------------------------------
// GenerateSearchToken Tests
// ---------------------------------------------------------------------------

func TestService_GenerateSearchToken_EnvelopeMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	token, _, err := svc.GenerateSearchToken(ctx, searchCtx, "ABC123")
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// Token should be base64-encoded
	assert.NotContains(t, token, "legacy-hash")
}

func TestService_GenerateSearchToken_Deterministic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	// Generate same token twice
	token1, _, err := svc.GenerateSearchToken(ctx, searchCtx, "ABC123")
	require.NoError(t, err)

	token2, _, err := svc.GenerateSearchToken(ctx, searchCtx, "ABC123")
	require.NoError(t, err)

	// Tokens should be identical
	assert.Equal(t, token1, token2, "same input should produce same token")
}

func TestService_GenerateSearchToken_DifferentInputs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	token1, _, err := svc.GenerateSearchToken(ctx, searchCtx, "ABC123")
	require.NoError(t, err)

	token2, _, err := svc.GenerateSearchToken(ctx, searchCtx, "XYZ789")
	require.NoError(t, err)

	// Tokens should be different
	assert.NotEqual(t, token1, token2, "different inputs should produce different tokens")
}

func TestService_GenerateSearchToken_LegacyMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	legacyKeys := newTestLegacyKeyMaterial(t)

	// Create service with empty registry (no record = legacy mode)
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys, NewProtectionMetrics(nil))

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		FieldName:      "document",
	}

	normalizedValue := "ABC123"
	token, _, err := svc.GenerateSearchToken(ctx, searchCtx, normalizedValue)
	require.NoError(t, err)

	// Legacy mode uses HMAC-SHA256 hex token matching lib-commons format
	expectedToken := legacyKeys.GenerateHash(&normalizedValue)
	assert.Equal(t, expectedToken, token)
}

func TestService_GenerateSearchToken_InvalidContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	tests := []struct {
		name      string
		searchCtx SearchTokenContext
	}{
		{
			name: "empty tenant ID",
			searchCtx: SearchTokenContext{
				TenantID:       "",
				OrganizationID: "org-123",
				FieldName:      "document",
			},
		},
		{
			name: "empty organization ID",
			searchCtx: SearchTokenContext{
				TenantID:       "tenant-abc",
				OrganizationID: "",
				FieldName:      "document",
			},
		},
		{
			name: "empty field name",
			searchCtx: SearchTokenContext{
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				FieldName:      "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := svc.GenerateSearchToken(ctx, tt.searchCtx, "value")
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrSearchContextInvalid)
		})
	}
}

// ---------------------------------------------------------------------------
// MustUseEnvelope Tests
// ---------------------------------------------------------------------------

func TestService_MustUseEnvelope_EnvelopeMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-envelope": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-envelope",
				Status:         mmodel.RegistryStatusActive,
				LegacyReadable: false,
				CurrentVersion: 1,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, nil, nil, nil, NewProtectionMetrics(nil))

	mustUse, err := svc.MustUseEnvelope(ctx, "org-envelope")
	require.NoError(t, err)
	assert.True(t, mustUse)
}

func TestService_MustUseEnvelope_LegacyMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Empty registry = legacy mode (no record found)
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, nil, nil, nil, NewProtectionMetrics(nil))

	mustUse, err := svc.MustUseEnvelope(ctx, "org-legacy")
	require.NoError(t, err)
	assert.False(t, mustUse)
}

// ---------------------------------------------------------------------------
// Edge Cases
// ---------------------------------------------------------------------------

func TestService_Encrypt_EmptyPlaintext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "tax_id",
	}

	// Empty plaintext should still work
	ciphertext, err := svc.Encrypt(ctx, fieldCtx, "")
	require.NoError(t, err)
	assert.True(t, HasEnvelopeMarker(ciphertext))

	// And decrypt back to empty
	decrypted, err := svc.Decrypt(ctx, fieldCtx, ciphertext)
	require.NoError(t, err)
	assert.Equal(t, "", decrypted)
}

func TestService_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "tax_id",
	}

	_, err := svc.Encrypt(ctx, fieldCtx, "plaintext")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestService_Decrypt_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "tax_id",
	}

	_, err := svc.Decrypt(ctx, fieldCtx, "some-ciphertext")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestService_Decrypt_InvalidFieldContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	invalidFieldCtx := FieldContext{
		TenantID:       "",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "tax_id",
	}

	_, err := svc.Decrypt(ctx, invalidFieldCtx, "some-ciphertext")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFieldContextInvalid)
}

func TestService_Decrypt_MalformedEnvelopeMarker(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        true,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "tax_id",
	}

	// Malformed marker - has prefix but invalid format
	_, err := svc.Decrypt(ctx, fieldCtx, "tink:vnotanumber:payload")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse envelope marker")
}

func TestService_GenerateSearchToken_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	_, _, err := svc.GenerateSearchToken(ctx, searchCtx, "value")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestService_MustUseEnvelope_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-123": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				Status:         mmodel.RegistryStatusActive,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, nil, nil, nil, NewProtectionMetrics(nil))

	_, err := svc.MustUseEnvelope(ctx, "org-123")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestService_GetProtectionState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-123": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				Status:         mmodel.RegistryStatusActive,
				LegacyReadable: true,
				CurrentVersion: 2,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, nil, nil, nil, NewProtectionMetrics(nil))

	state, err := svc.GetProtectionState(ctx, "org-123")
	require.NoError(t, err)
	assert.True(t, state.MustUseEnvelope())
	assert.True(t, state.CanReadLegacy)
	assert.Equal(t, 2, state.CurrentKeysetVersion)
}

func TestService_GetProtectionState_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-123": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				Status:         mmodel.RegistryStatusActive,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, nil, nil, nil, NewProtectionMetrics(nil))

	_, err := svc.GetProtectionState(ctx, "org-123")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestService_GetKeysetInfo(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	keysetRepo := &serviceTestKeysetRepo{
		keysets: map[string]*mmodel.OrganizationKeyset{
			"org-123": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				KeysetInfo: mmodel.KeysetInfo{
					PrimaryKeyID: 12345,
				},
			},
		},
	}

	svc := NewEncryptionService(nil, nil, keysetRepo, nil, NewProtectionMetrics(nil))

	info, err := svc.GetKeysetInfo(ctx, "org-123")
	require.NoError(t, err)
	assert.Equal(t, uint32(12345), info.PrimaryKeyID)
}

func TestService_GetKeysetInfo_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	keysetRepo := &serviceTestKeysetRepo{
		keysets: map[string]*mmodel.OrganizationKeyset{
			"org-123": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
			},
		},
	}

	svc := NewEncryptionService(nil, nil, keysetRepo, nil, NewProtectionMetrics(nil))

	_, err := svc.GetKeysetInfo(ctx, "org-123")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestService_GetKeysetInfo_NotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	keysetRepo := &serviceTestKeysetRepo{
		keysets: map[string]*mmodel.OrganizationKeyset{},
	}

	svc := NewEncryptionService(nil, nil, keysetRepo, nil, NewProtectionMetrics(nil))

	_, err := svc.GetKeysetInfo(ctx, "org-nonexistent")
	require.Error(t, err)
}

func TestService_Encrypt_NilLegacyKeyMaterial(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Empty registry = legacy mode (no record found)
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	// Service with nil legacy key material
	svc := NewEncryptionService(stateResolver, nil, nil, nil, NewProtectionMetrics(nil))

	fieldCtx := FieldContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		RecordID:       "record-789",
		FieldName:      "document",
	}

	// Should fail with nil legacy key material
	_, err := svc.Encrypt(ctx, fieldCtx, "plaintext")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "legacy crypto is required")
}

func TestService_Decrypt_NilLegacyKeyMaterial(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Empty registry = legacy mode (no record found)
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	// Service with nil legacy key material
	svc := NewEncryptionService(stateResolver, nil, nil, nil, NewProtectionMetrics(nil))

	fieldCtx := FieldContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		RecordID:       "record-789",
		FieldName:      "document",
	}

	// Should fail with nil legacy key material (ciphertext without envelope marker)
	_, err := svc.Decrypt(ctx, fieldCtx, "some-non-envelope-ciphertext")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "legacy crypto is required")
}

func TestService_Encrypt_StateResolverError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	registryRepo := &serviceTestRegistryRepo{
		err: errors.New("registry error"),
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, nil, nil, nil, NewProtectionMetrics(nil))

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "tax_id",
	}

	_, err := svc.Encrypt(ctx, fieldCtx, "plaintext")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve protection state")
}

func TestService_GenerateSearchToken_StateResolverError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	registryRepo := &serviceTestRegistryRepo{
		err: errors.New("registry error"),
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, nil, nil, nil, NewProtectionMetrics(nil))

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	_, _, err := svc.GenerateSearchToken(ctx, searchCtx, "value")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve protection state")
}

// ---------------------------------------------------------------------------
// Global Mode Tests (Lazy Provisioning via Global Mode)
// ---------------------------------------------------------------------------

// TestService_Encrypt_GlobalModeEnvelope_TriggersLazyProvisioning tests that when
// globalMode is EncryptionModeEnvelope, Encrypt() calls encryptEnvelope() even when
// the organization has no registry record (would normally resolve to legacy mode).
//
// This ensures lazy provisioning is triggered via KeysetManager.fetchAndCache()
// instead of incorrectly falling back to legacy encryption.
func TestService_Encrypt_GlobalModeEnvelope_TriggersLazyProvisioning(t *testing.T) {
	t.Parallel()

	// Create context with tenant ID for auto-provisioning
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-abc")

	// Setup: Organization with NO registry record (ProtectionStateResolver returns legacy mode)
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	// Setup: KeysetManager with lazy provisioning enabled
	// The provisioner will be called when keyset is not found
	aeadBytes, macBytes, aeadKeyID, _ := generateServiceTestKeysets(t)

	provisionedKeyset := &mmodel.OrganizationKeyset{
		TenantID:       "tenant-abc",
		OrganizationID: "org-new",
		KEKPath:        "transit/keys/crm/org-new",
		WrappedKeyset:  "wrapped-aead",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: aeadKeyID,
		},
		WrappedHMACKeyset: "wrapped-hmac",
		HMACKeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: aeadKeyID,
		},
	}

	// KeysetRepo starts empty, provisioner will populate it
	keysetRepo := &serviceTestKeysetRepoWithProvisioning{
		keysets:           map[string]*mmodel.OrganizationKeyset{},
		provisionedKeyset: provisionedKeyset,
	}

	unwrapper := &serviceTestKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	// Create a mock provisioner that simulates lazy provisioning
	mockProvisioner := &mockProvisioningService{
		provisionedKeyset: provisionedKeyset,
		keysetRepo:        keysetRepo,
	}

	keysetManager := NewKeysetManager(keysetRepo, unwrapper, mockProvisioner, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	legacyKeys := newTestLegacyKeyMaterial(t)

	// KEY CHANGE: Pass globalMode = EncryptionModeEnvelope to constructor
	// This tells the service to use envelope encryption globally, triggering
	// lazy provisioning even when per-org registry does not exist
	svc := NewEncryptionService(stateResolver, keysetManager, keysetRepo, legacyKeys, NewProtectionMetrics(nil), crypto.EncryptionModeEnvelope)

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-new", // Organization with no existing registry
		RecordID:       "record-456",
		FieldName:      "tax_id",
	}

	plaintext := "sensitive-data-123"

	// Execute: Encrypt should use envelope mode (triggering lazy provisioning)
	// NOT legacy mode, even though ProtectionStateResolver returns legacy
	ciphertext, err := svc.Encrypt(ctx, fieldCtx, plaintext)
	require.NoError(t, err)

	// Verify: Result has envelope marker (proves envelope path was taken)
	assert.True(t, HasEnvelopeMarker(ciphertext), "ciphertext MUST have envelope marker when globalMode is envelope")

	// Verify: Legacy crypto was NOT used
	assert.NotContains(t, ciphertext, "legacy-encrypted", "legacy encryption MUST NOT be used when globalMode is envelope")

	// Verify: Provisioner was called (lazy provisioning triggered)
	assert.True(t, mockProvisioner.provisionCalled, "lazy provisioning MUST be triggered for non-provisioned org when globalMode is envelope")
}

// serviceTestKeysetRepoWithProvisioning extends serviceTestKeysetRepo to simulate
// lazy provisioning behavior (keyset appears after provisioner is called).
type serviceTestKeysetRepoWithProvisioning struct {
	keysets           map[string]*mmodel.OrganizationKeyset
	provisionedKeyset *mmodel.OrganizationKeyset
	provisioned       bool
}

func (f *serviceTestKeysetRepoWithProvisioning) Get(_ context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	keyset, ok := f.keysets[organizationID]
	if !ok {
		return nil, constant.ErrKeysetNotFound
	}

	return keyset, nil
}

func (f *serviceTestKeysetRepoWithProvisioning) GetActive(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	return f.Get(ctx, organizationID)
}

func (f *serviceTestKeysetRepoWithProvisioning) GetByVersion(_ context.Context, organizationID string, version int) (*mmodel.OrganizationKeyset, error) {
	keyset, ok := f.keysets[organizationID]
	if !ok || keyset.Version != version {
		return nil, mmodel.ErrKeysetNotFound
	}

	return keyset, nil
}

func (f *serviceTestKeysetRepoWithProvisioning) Save(_ context.Context, keyset *mmodel.OrganizationKeyset) error {
	f.keysets[keyset.OrganizationID] = keyset
	f.provisioned = true

	return nil
}

func (f *serviceTestKeysetRepoWithProvisioning) Update(_ context.Context, _ *mmodel.OrganizationKeyset, _ int64) error {
	return nil
}

// mockProvisioningService implements ProvisioningService for testing lazy provisioning behavior.
type mockProvisioningService struct {
	provisionedKeyset *mmodel.OrganizationKeyset
	keysetRepo        *serviceTestKeysetRepoWithProvisioning
	provisionCalled   bool
	err               error
}

func (m *mockProvisioningService) Provision(ctx context.Context, req ProvisionInput) (ProvisionResult, error) {
	if m.err != nil {
		return ProvisionResult{}, m.err
	}

	m.provisionCalled = true

	// Simulate provisioning by adding keyset to repo
	if m.keysetRepo != nil && m.provisionedKeyset != nil {
		keyset := *m.provisionedKeyset
		keyset.TenantID = req.TenantID
		keyset.OrganizationID = req.OrganizationID

		if err := m.keysetRepo.Save(ctx, &keyset); err != nil {
			return ProvisionResult{}, err
		}
	}

	return ProvisionResult{
		OrganizationID:   req.OrganizationID,
		KEKPath:          "transit/keys/crm/" + req.OrganizationID,
		AEADPrimaryKeyID: 12345,
		PRFPrimaryKeyID:  12346,
		RegistryStatus:   mmodel.RegistryStatusActive,
	}, nil
}

func (m *mockProvisioningService) GetProvisioningStatus(_ context.Context, _ string) (*mmodel.RegistryStatus, error) {
	status := mmodel.RegistryStatusActive
	return &status, nil
}

func (m *mockProvisioningService) IsProvisioned(_ context.Context, _ string) (bool, error) {
	return m.provisionCalled, nil
}

func (m *mockProvisioningService) IsActive(_ context.Context, _ string) (bool, error) {
	return m.provisionCalled, nil
}

// TestService_GenerateSearchToken_GlobalModeEnvelope_TriggersLazyProvisioning tests that when
// globalMode is EncryptionModeEnvelope, GenerateSearchToken() calls generateSearchTokenEnvelope()
// even when the organization has no registry record (would normally resolve to legacy mode).
//
// This ensures lazy provisioning is triggered via KeysetManager.GetPrimitives()
// instead of incorrectly falling back to legacy hash generation.
// ---------------------------------------------------------------------------
// Task 5 Routing Tests - KMS None / Legacy Mode with Imported Legacy Key
// ---------------------------------------------------------------------------

func TestService_KMSNone_LegacyEncryptDecryptSearchUsesImportedLegacyKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Build legacyKeys with real Tink-backed legacy key material
	legacyKeys, err := NewLegacyKeyMaterial(legacyEncryptHexKey, legacyHashKey)
	require.NoError(t, err)

	// Build resolver with nil registry repo (pure legacy mode, no keyset)
	resolver := NewProtectionStateResolver(nil, NewProtectionMetrics(nil))

	// Build service with nil keyset manager and nil keyset repo (legacy-only mode)
	svc := NewEncryptionService(resolver, nil, nil, legacyKeys, NewProtectionMetrics(nil))

	fieldCtx := FieldContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		RecordID:       "record-789",
		FieldName:      "document",
	}

	plaintext := "crm-sensitive-value"

	// Test Encrypt
	ciphertext, err := svc.Encrypt(ctx, fieldCtx, plaintext)
	require.NoError(t, err)

	// Assert ciphertext does NOT have envelope marker
	assert.False(t, HasEnvelopeMarker(ciphertext), "legacy ciphertext should not have envelope marker")

	// Test Decrypt
	decrypted, err := svc.Decrypt(ctx, fieldCtx, ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)

	// Test GenerateSearchToken equals characterization HMAC hex token
	searchCtx := SearchTokenContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		FieldName:      "document",
	}

	token, _, err := svc.GenerateSearchToken(ctx, searchCtx, plaintext)
	require.NoError(t, err)

	// Compare with expected HMAC-SHA256 hex token
	expectedToken := legacyKeys.GenerateHash(&plaintext)
	assert.Equal(t, expectedToken, token)
}

func TestService_SearchRouting_LegacyTokenAndEnvelopeTokenDifferByMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	normalizedValue := "ABC123"

	// Build legacy mode service with imported legacy key
	legacyKeys, err := NewLegacyKeyMaterial(legacyEncryptHexKey, legacyHashKey)
	require.NoError(t, err)

	legacyResolver := NewProtectionStateResolver(nil, NewProtectionMetrics(nil))
	legacySvc := NewEncryptionService(legacyResolver, nil, nil, legacyKeys, NewProtectionMetrics(nil))

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	// Generate legacy token
	legacyToken, _, err := legacySvc.GenerateSearchToken(ctx, searchCtx, normalizedValue)
	require.NoError(t, err)

	// Assert legacy token equals expected hex HMAC
	expectedLegacyToken := legacyKeys.GenerateHash(&normalizedValue)
	assert.Equal(t, expectedLegacyToken, legacyToken)

	// Build envelope mode service
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	envelopeSvc, _ := createEncryptionTestService(t, state, legacyKeys)

	// Generate envelope token
	envelopeToken, _, err := envelopeSvc.GenerateSearchToken(ctx, searchCtx, normalizedValue)
	require.NoError(t, err)

	// Assert envelope token differs from legacy token
	assert.NotEqual(t, legacyToken, envelopeToken, "envelope and legacy tokens should differ")
}

func TestService_GenerateSearchToken_GlobalModeEnvelope_TriggersLazyProvisioning(t *testing.T) {
	t.Parallel()

	// Create context with tenant ID for auto-provisioning
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-abc")

	// Setup: Organization with NO registry record (ProtectionStateResolver returns legacy mode)
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	// Setup: KeysetManager with lazy provisioning enabled
	// The provisioner will be called when keyset is not found
	aeadBytes, macBytes, aeadKeyID, _ := generateServiceTestKeysets(t)

	provisionedKeyset := &mmodel.OrganizationKeyset{
		TenantID:       "tenant-abc",
		OrganizationID: "org-new",
		KEKPath:        "transit/keys/crm/org-new",
		WrappedKeyset:  "wrapped-aead",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: aeadKeyID,
		},
		WrappedHMACKeyset: "wrapped-hmac",
		HMACKeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: aeadKeyID,
		},
	}

	// KeysetRepo starts empty, provisioner will populate it
	keysetRepo := &serviceTestKeysetRepoWithProvisioning{
		keysets:           map[string]*mmodel.OrganizationKeyset{},
		provisionedKeyset: provisionedKeyset,
	}

	unwrapper := &serviceTestKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	// Create a mock provisioner that simulates lazy provisioning
	mockProvisioner := &mockProvisioningService{
		provisionedKeyset: provisionedKeyset,
		keysetRepo:        keysetRepo,
	}

	keysetManager := NewKeysetManager(keysetRepo, unwrapper, mockProvisioner, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	legacyKeys := newTestLegacyKeyMaterial(t)

	// KEY CHANGE: Pass globalMode = EncryptionModeEnvelope to constructor
	// This tells the service to use envelope encryption globally, triggering
	// lazy provisioning even when per-org registry does not exist
	svc := NewEncryptionService(stateResolver, keysetManager, keysetRepo, legacyKeys, NewProtectionMetrics(nil), crypto.EncryptionModeEnvelope)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-new", // Organization with no existing registry
		FieldName:      "document",
	}

	normalizedValue := "ABC123"

	// Execute: GenerateSearchToken should use envelope mode (triggering lazy provisioning)
	// NOT legacy mode, even though ProtectionStateResolver returns legacy
	token, _, err := svc.GenerateSearchToken(ctx, searchCtx, normalizedValue)
	require.NoError(t, err)

	// Verify: Token is not empty
	assert.NotEmpty(t, token, "search token MUST NOT be empty")

	// Verify: Legacy crypto hash was NOT used
	assert.NotContains(t, token, "legacy-hash", "legacy hash MUST NOT be used when globalMode is envelope")

	// Verify: Provisioner was called (lazy provisioning triggered)
	assert.True(t, mockProvisioner.provisionCalled, "lazy provisioning MUST be triggered for non-provisioned org when globalMode is envelope")
}

// ---------------------------------------------------------------------------
// GenerateSearchTokenCandidates Tests (T-2.1.1)
// ---------------------------------------------------------------------------

func TestService_GenerateSearchTokenCandidates_EnvelopeMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	tokens, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, "ABC123")
	require.NoError(t, err)
	assert.NotEmpty(t, tokens, "tokens slice MUST NOT be empty")

	// For a single-key keyset, we should get at least one token
	assert.GreaterOrEqual(t, len(tokens), 1, "MUST return at least one token for envelope mode")

	// All tokens should be non-empty base64-encoded strings
	for i, token := range tokens {
		assert.NotEmpty(t, token, "token at index %d MUST NOT be empty", i)
	}
}

func TestService_GenerateSearchTokenCandidates_LegacyMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	legacyKeys := newTestLegacyKeyMaterial(t)

	// Create service with empty registry (no record = legacy mode)
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys, NewProtectionMetrics(nil))

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		FieldName:      "document",
	}

	normalizedValue := "ABC123"
	tokens, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, normalizedValue)
	require.NoError(t, err)

	// Legacy mode MUST return exactly one token
	require.Len(t, tokens, 1, "legacy mode MUST return exactly one token")

	// Token MUST match the legacy hash
	expectedToken := legacyKeys.GenerateHash(&normalizedValue)
	assert.Equal(t, expectedToken, tokens[0], "legacy token MUST match GenerateHash output")
}

func TestService_GenerateSearchTokenCandidates_InvalidContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	tests := []struct {
		name      string
		searchCtx SearchTokenContext
	}{
		{
			name: "empty tenant ID",
			searchCtx: SearchTokenContext{
				TenantID:       "",
				OrganizationID: "org-123",
				FieldName:      "document",
			},
		},
		{
			name: "empty organization ID",
			searchCtx: SearchTokenContext{
				TenantID:       "tenant-abc",
				OrganizationID: "",
				FieldName:      "document",
			},
		},
		{
			name: "empty field name",
			searchCtx: SearchTokenContext{
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				FieldName:      "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := svc.GenerateSearchTokenCandidates(ctx, tt.searchCtx, "value")
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrSearchContextInvalid)
		})
	}
}

func TestService_GenerateSearchTokenCandidates_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	_, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, "value")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestService_GenerateSearchTokenCandidates_Deterministic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	// Generate tokens twice with same input
	tokens1, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, "ABC123")
	require.NoError(t, err)

	tokens2, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, "ABC123")
	require.NoError(t, err)

	// Token slices MUST be identical
	require.Equal(t, len(tokens1), len(tokens2), "token counts MUST be identical")

	for i := range tokens1 {
		assert.Equal(t, tokens1[i], tokens2[i], "token at index %d MUST be identical", i)
	}
}

func TestService_GenerateSearchTokenCandidates_DifferentInputs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	tokens1, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, "ABC123")
	require.NoError(t, err)

	tokens2, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, "XYZ789")
	require.NoError(t, err)

	// Tokens for different inputs MUST be different
	require.Equal(t, len(tokens1), len(tokens2), "token counts MUST be equal")

	for i := range tokens1 {
		assert.NotEqual(t, tokens1[i], tokens2[i], "tokens at index %d MUST differ for different inputs", i)
	}
}

func TestService_GenerateSearchTokenCandidates_GlobalModeEnvelope(t *testing.T) {
	t.Parallel()

	// Create context with tenant ID for auto-provisioning
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-abc")

	// Setup: Organization with NO registry record (ProtectionStateResolver returns legacy mode)
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	// Setup: KeysetManager with lazy provisioning enabled
	aeadBytes, macBytes, aeadKeyID, _ := generateServiceTestKeysets(t)

	provisionedKeyset := &mmodel.OrganizationKeyset{
		TenantID:       "tenant-abc",
		OrganizationID: "org-new",
		KEKPath:        "transit/keys/crm/org-new",
		WrappedKeyset:  "wrapped-aead",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: aeadKeyID,
		},
		WrappedHMACKeyset: "wrapped-hmac",
		HMACKeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: aeadKeyID,
		},
	}

	// KeysetRepo starts empty, provisioner will populate it
	keysetRepo := &serviceTestKeysetRepoWithProvisioning{
		keysets:           map[string]*mmodel.OrganizationKeyset{},
		provisionedKeyset: provisionedKeyset,
	}

	unwrapper := &serviceTestKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	mockProvisioner := &mockProvisioningService{
		provisionedKeyset: provisionedKeyset,
		keysetRepo:        keysetRepo,
	}

	keysetManager := NewKeysetManager(keysetRepo, unwrapper, mockProvisioner, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	legacyKeys := newTestLegacyKeyMaterial(t)

	// Use globalMode = EncryptionModeEnvelope to trigger envelope path
	svc := NewEncryptionService(stateResolver, keysetManager, keysetRepo, legacyKeys, NewProtectionMetrics(nil), crypto.EncryptionModeEnvelope)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-new",
		FieldName:      "document",
	}

	normalizedValue := "ABC123"

	// Execute: GenerateSearchTokenCandidates with global envelope mode
	tokens, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, normalizedValue)
	require.NoError(t, err)

	// Verify: Tokens slice is not empty
	assert.NotEmpty(t, tokens, "tokens slice MUST NOT be empty")

	// Verify: Provisioner was called (lazy provisioning triggered)
	assert.True(t, mockProvisioner.provisionCalled, "lazy provisioning MUST be triggered")
}

func TestService_GenerateSearchTokenCandidates_StateResolverError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	registryRepo := &serviceTestRegistryRepo{
		err: errors.New("registry error"),
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, nil, nil, nil, NewProtectionMetrics(nil))

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	_, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, "value")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve protection state")
}

func TestService_GenerateSearchTokenCandidates_NilLegacyKeyMaterial(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Empty registry = legacy mode (no record found)
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	// Service with nil legacy key material
	svc := NewEncryptionService(stateResolver, nil, nil, nil, NewProtectionMetrics(nil))

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		FieldName:      "document",
	}

	// Should return empty slice when legacy key material is nil
	tokens, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, "value")
	require.NoError(t, err)

	// Legacy mode with nil crypto returns single empty string
	require.Len(t, tokens, 1)
	assert.Empty(t, tokens[0], "token MUST be empty when legacy crypto is nil")
}

// ---------------------------------------------------------------------------
// GenerateSearchTokenCandidates legacy∪envelope union Tests
// ---------------------------------------------------------------------------

// Migrated org (composite PRF keyset) that may read legacy gets envelope candidates
// plus the per-org keyset legacy hex token (over the bare value) as the final element.
func TestService_GenerateSearchTokenCandidates_MigratedOrgCanReadLegacy_ReturnsEnvelopePlusLegacy(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-abc")

	svc, _ := migratedOrgSearchService(t, "org-123", "tenant-abc", true)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	value := "ABC123"

	envelopeOnly, err := svc.generateSearchTokenCandidatesEnvelope(ctx, searchCtx, value, false)
	require.NoError(t, err)
	require.NotEmpty(t, envelopeOnly, "envelope baseline MUST NOT be empty")

	tokens, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, value)
	require.NoError(t, err)

	for i, want := range envelopeOnly {
		assert.Contains(t, tokens, want, "result MUST contain envelope PRF candidate at index %d", i)
	}

	// Legacy token MUST be the final element, computed over the bare value via the
	// per-org keyset (byte-identical to the indexed legacy token).
	wantLegacy := newTestLegacyKeyMaterial(t).GenerateHash(&value)
	require.Len(t, tokens, len(envelopeOnly)+1, "result MUST be envelope candidates plus exactly one legacy token")
	assert.Equal(t, wantLegacy, tokens[len(tokens)-1], "legacy token MUST be the FINAL element and computed over the bare value")
}

// Global envelope mode with an empty registry resolves to CanReadLegacy=true. When the
// per-org keyset is a migrated (composite) keyset, the keyset legacy token is unioned in.
func TestService_GenerateSearchTokenCandidates_GlobalEnvelopeModeCanReadLegacy_IncludesLegacy(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-abc")

	// Empty registry -> global envelope mode resolves CanReadLegacy=true.
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	// Migrated (composite) per-org keyset so LegacyHexTokenPRF is populated.
	factory := tink.NewKeysetFactory(identityKMS{})

	mixedAEAD, err := factory.GenerateMixedAEADKeyset(context.Background(), "transit", "crm-org-global", helperLegacyHexKey)
	require.NoError(t, err)

	mixedPRF, err := factory.GenerateMixedPRFKeyset(context.Background(), "transit", "crm-org-global", helperLegacySecret)
	require.NoError(t, err)

	keysetManager := newKeysetManagerForKeyset(t, "org-global", mixedAEAD, mixedPRF)

	spy := &spyLegacyCrypto{}

	svc := NewEncryptionService(stateResolver, keysetManager, nil, spy, NewProtectionMetrics(nil), crypto.EncryptionModeEnvelope)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-global",
		FieldName:      "document",
	}

	value := "ABC123"

	tokens, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, value)
	require.NoError(t, err)

	wantLegacy := newTestLegacyKeyMaterial(t).GenerateHash(&value)
	require.GreaterOrEqual(t, len(tokens), 2, "result MUST include at least one envelope candidate plus the legacy token")
	assert.Equal(t, wantLegacy, tokens[len(tokens)-1], "legacy token MUST be the FINAL element over the bare value")
	assert.False(t, spy.hashCalled, "process-global legacyCrypto MUST NOT be consulted on the envelope path")
}

// Envelope-resolved org that may NOT read legacy gets envelope candidates only.
func TestService_GenerateSearchTokenCandidates_BornEnvelopeNoLegacy_ReturnsEnvelopeOnly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-123",
		TenantID:             "tenant-abc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestService(t, state, legacyKeys)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	value := "ABC123"

	envelopeOnly, err := svc.(*encryptionService).generateSearchTokenCandidatesEnvelope(ctx, searchCtx, value, false)
	require.NoError(t, err)

	tokens, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, value)
	require.NoError(t, err)

	assert.Equal(t, envelopeOnly, tokens, "envelope-only org MUST NOT include a legacy token")

	wantLegacy := legacyKeys.GenerateHash(&value)
	assert.NotContains(t, tokens, wantLegacy, "result MUST NOT contain the legacy token when CanReadLegacy=false")
}

// migratedOrgSearchService builds an envelope-mode service for a MIGRATED org:
// a per-org composite PRF keyset (fresh primary + imported legacy HMAC secret) so
// GetPrimitives populates CachedPrimitives.LegacyHexTokenPRF. The legacyCrypto is an
// observable spy so tests can assert the process-global path is NEVER consulted on
// the envelope search candidate path. Returns the service and the spy.
func migratedOrgSearchService(t *testing.T, organizationID, tenantID string, canReadLegacy bool) (*encryptionService, *spyLegacyCrypto) {
	t.Helper()

	factory := tink.NewKeysetFactory(identityKMS{})

	mixedAEAD, err := factory.GenerateMixedAEADKeyset(context.Background(), "transit", "crm-"+organizationID, helperLegacyHexKey)
	require.NoError(t, err)

	mixedPRF, err := factory.GenerateMixedPRFKeyset(context.Background(), "transit", "crm-"+organizationID, helperLegacySecret)
	require.NoError(t, err)

	km := newKeysetManagerForKeyset(t, organizationID, mixedAEAD, mixedPRF)

	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			organizationID: {
				TenantID:       tenantID,
				OrganizationID: organizationID,
				Status:         mmodel.RegistryStatusActive,
				LegacyReadable: canReadLegacy,
				CurrentVersion: 1,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	spy := &spyLegacyCrypto{}
	svc := NewEncryptionService(stateResolver, km, nil, spy, NewProtectionMetrics(nil), crypto.EncryptionModeEnvelope)

	return svc.(*encryptionService), spy
}

// T-2.2.2: a MIGRATED org (composite PRF keyset, CanReadLegacy=true) MUST union the
// per-org keyset-derived legacy hex token (byte-identical to the indexed legacy token)
// as the final candidate, and MUST NOT consult the process-global legacyCrypto.
func TestService_GenerateSearchTokenCandidates_MigratedOrg_UsesKeysetLegacyToken_NotProcessGlobal(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-mig")

	svc, spy := migratedOrgSearchService(t, "org-mig", "tenant-mig", true)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-mig",
		OrganizationID: "org-mig",
		FieldName:      "document",
	}

	value := "ABC123"

	// MultiKeyPRF envelope candidates (canReadLegacy=false isolates the envelope set).
	envelopeOnly, err := svc.generateSearchTokenCandidatesEnvelope(ctx, searchCtx, value, false)
	require.NoError(t, err)
	require.NotEmpty(t, envelopeOnly, "envelope baseline MUST NOT be empty")

	tokens, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, value)
	require.NoError(t, err)

	// Envelope candidates preserved.
	for i, want := range envelopeOnly {
		assert.Contains(t, tokens, want, "result MUST contain envelope PRF candidate at index %d", i)
	}

	// Exactly one extra candidate: the keyset-derived legacy hex token.
	require.Len(t, tokens, len(envelopeOnly)+1, "result MUST be envelope candidates plus exactly one legacy token")

	// The keyset-derived legacy token is byte-identical to the indexed legacy token
	// (LegacyKeyMaterial uses the same secret as the imported keyset key).
	indexedLegacy := newTestLegacyKeyMaterial(t).GenerateHash(&value)
	assert.Equal(t, indexedLegacy, tokens[len(tokens)-1], "final candidate MUST equal the indexed legacy hex token")

	// The process-global legacyCrypto MUST NOT be consulted on the envelope path.
	assert.False(t, spy.hashCalled, "process-global legacyCrypto.GenerateHash MUST NOT be consulted on the envelope path")
	assert.NotContains(t, tokens, "spy-legacy-hash-must-not-be-used", "result MUST NOT contain the process-global spy token")
}

// T-2.2.2: an ENVELOPE-ONLY org (fresh PRF keyset, LegacyHexTokenPRF nil) with
// CanReadLegacy=true MUST produce envelope-only candidates and MUST NOT consult the
// process-global legacyCrypto (the org never wrote legacy tokens).
func TestService_GenerateSearchTokenCandidates_EnvelopeOnlyOrg_NoLegacyToken_NotProcessGlobal(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-env")

	factory := tink.NewKeysetFactory(identityKMS{})

	envAEAD, err := factory.GenerateAEADKeyset(context.Background(), "transit", "crm-org-env")
	require.NoError(t, err)

	envPRF, err := factory.GeneratePRFKeyset(context.Background(), "transit", "crm-org-env")
	require.NoError(t, err)

	km := newKeysetManagerForKeyset(t, "org-env", envAEAD, envPRF)

	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-env": {
				TenantID:       "tenant-env",
				OrganizationID: "org-env",
				Status:         mmodel.RegistryStatusActive,
				LegacyReadable: true,
				CurrentVersion: 1,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	spy := &spyLegacyCrypto{}
	svc := NewEncryptionService(stateResolver, km, nil, spy, NewProtectionMetrics(nil), crypto.EncryptionModeEnvelope).(*encryptionService)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-env",
		OrganizationID: "org-env",
		FieldName:      "document",
	}

	value := "ABC123"

	envelopeOnly, err := svc.generateSearchTokenCandidatesEnvelope(ctx, searchCtx, value, false)
	require.NoError(t, err)

	tokens, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, value)
	require.NoError(t, err)

	// No legacy token appended; envelope candidates only.
	assert.Equal(t, envelopeOnly, tokens, "envelope-only org MUST produce envelope candidates only")

	// Process-global legacyCrypto MUST NOT be consulted.
	assert.False(t, spy.hashCalled, "envelope-only org MUST NOT consult process-global legacyCrypto")
	assert.NotContains(t, tokens, "spy-legacy-hash-must-not-be-used", "result MUST NOT contain the process-global spy token")
}

// T-2.2.2: a migrated org with CanReadLegacy=false MUST produce envelope-only
// candidates with no legacy token, regardless of the keyset carrying a legacy key.
func TestService_GenerateSearchTokenCandidates_MigratedOrg_CanReadLegacyFalse_NoLegacyToken(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-mig2")

	svc, spy := migratedOrgSearchService(t, "org-mig2", "tenant-mig2", false)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-mig2",
		OrganizationID: "org-mig2",
		FieldName:      "document",
	}

	value := "ABC123"

	envelopeOnly, err := svc.generateSearchTokenCandidatesEnvelope(ctx, searchCtx, value, false)
	require.NoError(t, err)

	tokens, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, value)
	require.NoError(t, err)

	assert.Equal(t, envelopeOnly, tokens, "CanReadLegacy=false MUST produce envelope candidates only")

	indexedLegacy := newTestLegacyKeyMaterial(t).GenerateHash(&value)
	assert.NotContains(t, tokens, indexedLegacy, "no legacy candidate when CanReadLegacy=false")
	assert.False(t, spy.hashCalled, "legacyCrypto MUST NOT be consulted when CanReadLegacy=false")
}

// T-2.2.2: an envelope-only org (fresh PRF keyset, LegacyHexTokenPRF nil) with
// CanReadLegacy=true and NO process-global legacy crypto wired produces envelope-only
// candidates WITHOUT error: the per-org keyset carries no legacy key, so there is no
// legacy candidate to produce and the process-global legacyCrypto is never consulted.
func TestService_GenerateSearchTokenCandidates_EnvelopeOnlyNilLegacyCrypto_EnvelopeOnly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-123": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				Status:         mmodel.RegistryStatusActive,
				LegacyReadable: true,
				CurrentVersion: 1,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	aeadBytes, macBytes, aeadKeyID, prfKeyID := generateServiceTestKeysets(t)

	keyset := &mmodel.OrganizationKeyset{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		KEKPath:        "test-kek",
		WrappedKeyset:  "wrapped-aead",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: aeadKeyID,
		},
		WrappedHMACKeyset: "wrapped-hmac",
		HMACKeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: prfKeyID,
		},
	}

	keysetRepo := &serviceTestKeysetRepo{
		keysets: map[string]*mmodel.OrganizationKeyset{
			"org-123": keyset,
		},
	}

	unwrapper := &serviceTestKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	keysetManager := NewKeysetManager(keysetRepo, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	// nil legacyCrypto (true nil interface). It MUST NOT be consulted on this path.
	svc := NewEncryptionService(stateResolver, keysetManager, keysetRepo, nil, NewProtectionMetrics(nil), crypto.EncryptionModeEnvelope)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	value := "ABC123"

	envelopeOnly, err := svc.(*encryptionService).generateSearchTokenCandidatesEnvelope(ctx, searchCtx, value, false)
	require.NoError(t, err)

	tokens, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, value)
	require.NoError(t, err, "envelope-only org with nil legacyCrypto MUST NOT error")
	assert.Equal(t, envelopeOnly, tokens, "envelope-only org MUST produce envelope candidates only")
}

// A protection-state resolution failure on the global envelope path is propagated unchanged.
func TestService_GenerateSearchTokenCandidates_ResolveError_Propagated(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	registryRepo := &serviceTestRegistryRepo{
		err: errors.New("registry error"),
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	aeadBytes, macBytes, aeadKeyID, _ := generateServiceTestKeysets(t)

	keyset := &mmodel.OrganizationKeyset{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		KEKPath:        "test-kek",
		WrappedKeyset:  "wrapped-aead",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: aeadKeyID,
		},
		WrappedHMACKeyset: "wrapped-hmac",
		HMACKeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: aeadKeyID,
		},
	}

	keysetRepo := &serviceTestKeysetRepo{
		keysets: map[string]*mmodel.OrganizationKeyset{
			"org-123": keyset,
		},
	}

	unwrapper := &serviceTestKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	keysetManager := NewKeysetManager(keysetRepo, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	legacyKeys := newTestLegacyKeyMaterial(t)

	svc := NewEncryptionService(stateResolver, keysetManager, keysetRepo, legacyKeys, NewProtectionMetrics(nil), crypto.EncryptionModeEnvelope)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	_, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, "ABC123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve protection state")
}

// A pure-legacy org still returns the single-element legacy-token slice, unchanged by the union.
func TestService_GenerateSearchTokenCandidates_PureLegacyOrg_Unchanged(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	legacyKeys := newTestLegacyKeyMaterial(t)

	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys, NewProtectionMetrics(nil))

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		FieldName:      "document",
	}

	value := "ABC123"

	tokens, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, value)
	require.NoError(t, err)

	require.Len(t, tokens, 1, "pure-legacy org MUST return exactly one token")
	assert.Equal(t, legacyKeys.GenerateHash(&value), tokens[0], "single legacy token MUST match GenerateHash over bare value")
}

// TestService_GenerateSearchToken_Envelope_ReturnsPRFKeyVersion verifies that the
// envelope path returns a non-zero keyVersion equal to the provisioned PRF keyset
// primary key ID, and that the token base64url-decodes to a RAW 32-byte PRF value
func TestService_GenerateSearchToken_Envelope_ReturnsPRFKeyVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-prf",
		TenantID:             "tenant-prf",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, keyset := createEncryptionTestService(t, state, legacyKeys)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-prf",
		OrganizationID: "org-prf",
		FieldName:      "document",
	}

	token, keyVersion, err := svc.GenerateSearchToken(ctx, searchCtx, "ABC123")
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// keyVersion must equal the provisioned PRF (HMAC) keyset primary key ID.
	assert.NotZero(t, keyVersion, "envelope search token must carry a non-zero PRF key version")
	assert.Equal(t, keyset.HMACKeysetInfo.PrimaryKeyID, keyVersion)

	// PRF output is RAW (no Tink key-id prefix) and fixed at 32 bytes.
	raw, decErr := base64.URLEncoding.DecodeString(token)
	require.NoError(t, decErr)
	assert.Len(t, raw, 32, "PRF token must decode to RAW 32 bytes")
}

// TestService_GenerateSearchToken_Legacy_ReturnsZeroKeyVersion verifies that the
// true legacy-hash branch returns keyVersion == 0.
func TestService_GenerateSearchToken_Legacy_ReturnsZeroKeyVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	legacyKeys := newTestLegacyKeyMaterial(t)

	// Empty registry (no record) -> legacy mode.
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys, NewProtectionMetrics(nil))

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		FieldName:      "document",
	}

	normalizedValue := "ABC123"
	token, keyVersion, err := svc.GenerateSearchToken(ctx, searchCtx, normalizedValue)
	require.NoError(t, err)

	assert.Zero(t, keyVersion, "legacy-hash branch must return key version 0")
	assert.Equal(t, legacyKeys.GenerateHash(&normalizedValue), token)
}

// TestService_GenerateSearchTokenCandidates_MigratedOrg_PreservesLegacyUnion
// verifies the legacy∪envelope union for a MIGRATED org (composite PRF keyset +
// CanReadLegacy): the PRF candidate(s) AND a trailing keyset-sourced legacy
// bare-value token (byte-identical to the indexed legacy token).
func TestService_GenerateSearchTokenCandidates_MigratedOrg_PreservesLegacyUnion(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-migrated")

	svc, spy := migratedOrgSearchService(t, "org-migrated", "tenant-migrated", true)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-migrated",
		OrganizationID: "org-migrated",
		FieldName:      "document",
	}

	normalizedValue := "ABC123"
	tokens, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, normalizedValue)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(tokens), 2, "must include at least one PRF candidate plus the legacy token")

	// Trailing element must be the keyset-sourced legacy bare-value token
	// (byte-identical to the indexed legacy token).
	legacyToken := newTestLegacyKeyMaterial(t).GenerateHash(&normalizedValue)
	assert.Equal(t, legacyToken, tokens[len(tokens)-1], "last candidate must be the legacy bare-value token")
	assert.False(t, spy.hashCalled, "process-global legacyCrypto MUST NOT be consulted")

	// Leading PRF candidate(s) must be RAW 32-byte PRF values, distinct from the legacy hex token.
	raw, decErr := base64.URLEncoding.DecodeString(tokens[0])
	require.NoError(t, decErr)
	assert.Len(t, raw, 32)
	assert.NotEqual(t, legacyToken, tokens[0])
}

// ---------------------------------------------------------------------------
// T-2.2.3: end-to-end legacy-token search match
// ---------------------------------------------------------------------------

// TestService_GenerateSearchTokenCandidates_LegacyIndexedValueIsFoundEndToEnd proves
// the full read-path contract for a value that was indexed under the OLD scheme.
//
// Scenario: before migration the process-global legacy hashing wrote
// search.<field> = hex-over-bare-value HMAC for value V. After the org is migrated
// (its per-org composite keyset is built with the SAME legacy secret), a search for
// V calls GenerateSearchTokenCandidates — the exact function whose result feeds the
// MongoDB `$in` filter (holder_query.mongodb.go:144-150, alias_query.mongodb.go:179-228).
//
// The test asserts:
//  1. The candidate set CONTAINS the historically-indexed legacy token byte-for-byte
//     (so a `$in` query against the pre-migration row matches), and
//  2. The candidate set also contains the current envelope/MultiKeyPRF candidate(s)
//     (so newly indexed rows match too), and
//  3. An envelope-only org (never wrote legacy rows) produces NO legacy candidate, so
//     it would never falsely match on the legacy token.
//
// This is the end-to-end proof that migrated orgs can still find their pre-migration
// rows while envelope-only orgs are unaffected.
func TestService_GenerateSearchTokenCandidates_LegacyIndexedValueIsFoundEndToEnd(t *testing.T) {
	t.Parallel()

	value := "123-45-6789"

	// STEP 1 — simulate the historically-indexed token. This is exactly what the
	// pre-migration process-global legacy hashing wrote into search.<field> for V:
	// the lowercase-hex HMAC over the BARE normalized value. LegacyKeyMaterial uses
	// the same legacy secret the migrated org's composite keyset imports.
	indexedLegacyToken := newTestLegacyKeyMaterial(t).GenerateHash(&value)
	require.NotEmpty(t, indexedLegacyToken, "historically-indexed legacy token must be non-empty")

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-e2e",
		OrganizationID: "org-e2e",
		FieldName:      "document",
	}

	t.Run("migrated org finds the historically-indexed row plus current rows", func(t *testing.T) {
		t.Parallel()

		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-e2e")

		// MIGRATED org: composite keyset built with the SAME legacy secret.
		svc, spy := migratedOrgSearchService(t, "org-e2e", "tenant-e2e", true)

		// Isolate the envelope/MultiKeyPRF candidate set (canReadLegacy=false path).
		envelopeCandidates, err := svc.generateSearchTokenCandidatesEnvelope(ctx, searchCtx, value, false)
		require.NoError(t, err)
		require.NotEmpty(t, envelopeCandidates, "envelope candidate set must not be empty")

		// READ PATH: the exact slice that feeds the `$in` filter.
		candidates, err := svc.GenerateSearchTokenCandidates(ctx, searchCtx, value)
		require.NoError(t, err)

		// (1) The `$in` set contains the historically-indexed token -> old row matches.
		assert.Contains(t, candidates, indexedLegacyToken,
			"candidate set must contain the historically-indexed legacy token so the pre-migration row is found")

		// (2) The `$in` set also contains the current envelope candidate(s) -> new rows match.
		for i, envToken := range envelopeCandidates {
			assert.Contains(t, candidates, envToken,
				"candidate set must contain the current envelope candidate at index %d", i)
		}

		// The legacy token is derived from the per-org keyset, never the process-global crypto.
		assert.False(t, spy.hashCalled,
			"process-global legacyCrypto MUST NOT be consulted on the envelope read path")
	})

	t.Run("envelope-only org produces no legacy candidate", func(t *testing.T) {
		t.Parallel()

		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-e2e-env")

		// ENVELOPE-ONLY org: fresh PRF keyset, no imported legacy key, never wrote legacy rows.
		factory := tink.NewKeysetFactory(identityKMS{})

		envAEAD, err := factory.GenerateAEADKeyset(context.Background(), "transit", "crm-org-e2e-env")
		require.NoError(t, err)

		envPRF, err := factory.GeneratePRFKeyset(context.Background(), "transit", "crm-org-e2e-env")
		require.NoError(t, err)

		km := newKeysetManagerForKeyset(t, "org-e2e-env", envAEAD, envPRF)

		registryRepo := &serviceTestRegistryRepo{
			records: map[string]*mmodel.OrganizationRegistryRecord{
				"org-e2e-env": {
					TenantID:       "tenant-e2e-env",
					OrganizationID: "org-e2e-env",
					Status:         mmodel.RegistryStatusActive,
					LegacyReadable: true, // even with legacy reads allowed, no legacy key exists
					CurrentVersion: 1,
				},
			},
		}
		stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

		spy := &spyLegacyCrypto{}
		svc := NewEncryptionService(stateResolver, km, nil, spy, NewProtectionMetrics(nil), crypto.EncryptionModeEnvelope)

		envSearchCtx := SearchTokenContext{
			TenantID:       "tenant-e2e-env",
			OrganizationID: "org-e2e-env",
			FieldName:      "document",
		}

		candidates, err := svc.GenerateSearchTokenCandidates(ctx, envSearchCtx, value)
		require.NoError(t, err)

		// The legacy token is absent -> the env-only org never falsely matches a legacy row.
		assert.NotContains(t, candidates, indexedLegacyToken,
			"envelope-only org MUST NOT produce the legacy token (it never wrote legacy rows)")
		assert.False(t, spy.hashCalled,
			"envelope-only org MUST NOT consult the process-global legacyCrypto")
	})
}

// ---------------------------------------------------------------------------
// Keyset-backed legacy decrypt helper (T-2.1.1)
// ---------------------------------------------------------------------------

const helperLegacyHexKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

const helperLegacySecret = "legacy-hash-secret"

// newKeysetManagerForKeyset builds a KeysetManager backed by the given composite
// keyset bundles for organizationID. The fake unwrapper returns the real raw
// keyset bytes so GetPrimitives produces working primitives.
func newKeysetManagerForKeyset(t *testing.T, organizationID string, aead, prf tink.KeysetBundle) *KeysetManager {
	t.Helper()

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    organizationID,
			KEKPath:           "crm-" + organizationID,
			KEKMountPath:      "transit",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
			KeysetInfo:        convertTestKeysetInfo(aead.Wrapped.Info),
			HMACKeysetInfo:    convertTestKeysetInfo(prf.Wrapped.Info),
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aead.RawKeyset,
		prfKeyset:  prf.RawKeyset,
	}

	return NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))
}

// newServiceWithKeysetManager builds an *encryptionService wired to the given
// KeysetManager. The helper under test reads only keysetManager, so the other
// dependencies are intentionally minimal.
func newServiceWithKeysetManager(km *KeysetManager) *encryptionService {
	return &encryptionService{
		keysetManager: km,
		metrics:       NewProtectionMetrics(nil),
	}
}

// TestService_DecryptLegacyFromKeyset_MigratedOrg proves that an unmarked legacy
// ciphertext (written with the legacy hex key, nil AAD) decrypts through the
// per-org composite AEAD primitive, and FAILS for an envelope-only keyset that
// has no legacy key. The helper never consults process-global s.legacyCrypto
// (it is left nil here).
func TestService_DecryptLegacyFromKeyset_MigratedOrg(t *testing.T) {
	t.Parallel()

	factory := tink.NewKeysetFactory(identityKMS{})

	// Migrated org: composite keyset (fresh primary + imported legacy AES-GCM).
	mixedAEAD, err := factory.GenerateMixedAEADKeyset(context.Background(), "transit", "crm-org-migrated", helperLegacyHexKey)
	require.NoError(t, err)

	mixedPRF, err := factory.GenerateMixedPRFKeyset(context.Background(), "transit", "crm-org-migrated", helperLegacySecret)
	require.NoError(t, err)

	// Envelope-only org: fresh keyset with NO legacy key imported.
	envAEAD, err := factory.GenerateAEADKeyset(context.Background(), "transit", "crm-org-envelope")
	require.NoError(t, err)

	envPRF, err := factory.GeneratePRFKeyset(context.Background(), "transit", "crm-org-envelope")
	require.NoError(t, err)

	// Produce unmarked legacy ciphertext using the legacy hex key directly,
	// mirroring how legacy data was written (base64(nonce||ct||tag), nil AAD).
	plaintext := "migrated-secret-value"
	legacyKeys := newTestLegacyKeyMaterial(t)
	encrypted, err := legacyKeys.Encrypt(&plaintext)
	require.NoError(t, err)
	require.NotNil(t, encrypted)

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-migrated")

	t.Run("migrated org composite keyset decrypts legacy ciphertext", func(t *testing.T) {
		km := newKeysetManagerForKeyset(t, "org-migrated", mixedAEAD, mixedPRF)
		svc := newServiceWithKeysetManager(km)

		fieldCtx := FieldContext{
			TenantID:       "tenant-migrated",
			OrganizationID: "org-migrated",
			RecordID:       "rec-1",
			FieldName:      "document",
		}

		got, derr := svc.decryptLegacyFromKeyset(ctx, fieldCtx, *encrypted)
		require.NoError(t, derr)
		assert.Equal(t, plaintext, got)
	})

	t.Run("envelope-only keyset fails to decrypt legacy ciphertext", func(t *testing.T) {
		km := newKeysetManagerForKeyset(t, "org-envelope", envAEAD, envPRF)
		svc := newServiceWithKeysetManager(km)

		fieldCtx := FieldContext{
			TenantID:       "tenant-migrated",
			OrganizationID: "org-envelope",
			RecordID:       "rec-1",
			FieldName:      "document",
		}

		_, derr := svc.decryptLegacyFromKeyset(ctx, fieldCtx, *encrypted)
		require.Error(t, derr, "envelope-only keyset must not silently succeed")
	})
}

// TestService_DecryptLegacyFromKeyset_DecodeFailure proves a non-base64 input is
// rejected with a wrapped error and no panic.
func TestService_DecryptLegacyFromKeyset_DecodeFailure(t *testing.T) {
	t.Parallel()

	factory := tink.NewKeysetFactory(identityKMS{})

	mixedAEAD, err := factory.GenerateMixedAEADKeyset(context.Background(), "transit", "crm-org-migrated", helperLegacyHexKey)
	require.NoError(t, err)

	mixedPRF, err := factory.GenerateMixedPRFKeyset(context.Background(), "transit", "crm-org-migrated", helperLegacySecret)
	require.NoError(t, err)

	km := newKeysetManagerForKeyset(t, "org-migrated", mixedAEAD, mixedPRF)
	svc := newServiceWithKeysetManager(km)

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-migrated")
	fieldCtx := FieldContext{
		TenantID:       "tenant-migrated",
		OrganizationID: "org-migrated",
		RecordID:       "rec-1",
		FieldName:      "document",
	}

	_, derr := svc.decryptLegacyFromKeyset(ctx, fieldCtx, "not-valid-base64!!!")
	require.Error(t, derr)
}

// TestService_DecryptLegacyFromKeyset_GetPrimitivesFailure proves a GetPrimitives
// failure is propagated as a wrapped error.
func TestService_DecryptLegacyFromKeyset_GetPrimitivesFailure(t *testing.T) {
	t.Parallel()

	unwrapper := &fakeKeysetUnwrapper{err: errors.New("unwrap boom")}
	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-broken",
			KEKPath:           "crm-org-broken",
			KEKMountPath:      "transit",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	km := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))
	svc := newServiceWithKeysetManager(km)

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-broken")
	fieldCtx := FieldContext{
		TenantID:       "tenant-broken",
		OrganizationID: "org-broken",
		RecordID:       "rec-1",
		FieldName:      "document",
	}

	plaintext := "v"
	legacyKeys := newTestLegacyKeyMaterial(t)
	encrypted, err := legacyKeys.Encrypt(&plaintext)
	require.NoError(t, err)

	_, derr := svc.decryptLegacyFromKeyset(ctx, fieldCtx, *encrypted)
	require.Error(t, derr)
}

// TestService_DecryptLegacyFromKeyset_ContextCancelled proves a cancelled context
// short-circuits before any crypto work, returning the context error.
func TestService_DecryptLegacyFromKeyset_ContextCancelled(t *testing.T) {
	t.Parallel()

	factory := tink.NewKeysetFactory(identityKMS{})

	mixedAEAD, err := factory.GenerateMixedAEADKeyset(context.Background(), "transit", "crm-org-migrated", helperLegacyHexKey)
	require.NoError(t, err)

	mixedPRF, err := factory.GenerateMixedPRFKeyset(context.Background(), "transit", "crm-org-migrated", helperLegacySecret)
	require.NoError(t, err)

	km := newKeysetManagerForKeyset(t, "org-migrated", mixedAEAD, mixedPRF)
	svc := newServiceWithKeysetManager(km)

	ctx, cancel := context.WithCancel(tmcore.ContextWithTenantID(context.Background(), "tenant-migrated"))
	cancel()

	fieldCtx := FieldContext{
		TenantID:       "tenant-migrated",
		OrganizationID: "org-migrated",
		RecordID:       "rec-1",
		FieldName:      "document",
	}

	_, derr := svc.decryptLegacyFromKeyset(ctx, fieldCtx, "irrelevant")
	require.ErrorIs(t, derr, context.Canceled)
}

// spyLegacyCrypto is an observable LegacyCrypto double. Decrypt records that it
// was invoked and always fails, so a test can assert the production path did NOT
// route through process-global legacy crypto.
type spyLegacyCrypto struct {
	decryptCalled bool
	hashCalled    bool
}

func (s *spyLegacyCrypto) Encrypt(plaintext *string) (*string, error) {
	return plaintext, nil
}

func (s *spyLegacyCrypto) Decrypt(_ *string) (*string, error) {
	s.decryptCalled = true
	return nil, errors.New("spy legacy decrypt must not be called")
}

func (s *spyLegacyCrypto) GenerateHash(_ *string) string {
	s.hashCalled = true
	return "spy-legacy-hash-must-not-be-used"
}

// TestService_Decrypt_LegacyRouting proves decryptLegacy routes unmarked legacy
// ciphertext by per-org protection state (state.MustUseEnvelope), not by the
// process-global legacyCrypto:
//
//	(a) envelope/migrated org decrypts via the per-org composite KEYSET; the
//	    observable legacyCrypto double is NEVER consulted;
//	(b) legacy-mode org (no keyset, KMS vendor none shape) decrypts via legacyCrypto;
//	(c) CanReadLegacy=false short-circuits with ErrLegacyReadNotAllowed before any
//	    decrypt branch (neither keyset nor legacyCrypto is consulted).
func TestService_Decrypt_LegacyRouting(t *testing.T) {
	t.Parallel()

	// Unmarked legacy ciphertext written with the legacy hex key, nil AAD.
	plaintext := "routed-secret-value"
	legacyKeys := newTestLegacyKeyMaterial(t)
	encrypted, err := legacyKeys.Encrypt(&plaintext)
	require.NoError(t, err)
	require.NotNil(t, encrypted)

	fieldCtx := FieldContext{
		TenantID:       "tenant-route",
		OrganizationID: "org-migrated",
		RecordID:       "rec-1",
		FieldName:      "document",
	}

	t.Run("envelope org routes to keyset, legacyCrypto not consulted", func(t *testing.T) {
		t.Parallel()

		factory := tink.NewKeysetFactory(identityKMS{})

		mixedAEAD, gerr := factory.GenerateMixedAEADKeyset(context.Background(), "transit", "crm-org-migrated", helperLegacyHexKey)
		require.NoError(t, gerr)

		mixedPRF, gerr := factory.GenerateMixedPRFKeyset(context.Background(), "transit", "crm-org-migrated", helperLegacySecret)
		require.NoError(t, gerr)

		km := newKeysetManagerForKeyset(t, "org-migrated", mixedAEAD, mixedPRF)

		registryRepo := &serviceTestRegistryRepo{
			records: map[string]*mmodel.OrganizationRegistryRecord{
				"org-migrated": {
					TenantID:       "tenant-route",
					OrganizationID: "org-migrated",
					Status:         mmodel.RegistryStatusActive,
					LegacyReadable: true,
					CurrentVersion: 1,
				},
			},
		}
		stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

		spy := &spyLegacyCrypto{}
		svc := NewEncryptionService(stateResolver, km, nil, spy, NewProtectionMetrics(nil), crypto.EncryptionModeEnvelope)

		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-route")

		got, derr := svc.Decrypt(ctx, fieldCtx, *encrypted)
		require.NoError(t, derr)
		assert.Equal(t, plaintext, got)
		assert.False(t, spy.decryptCalled, "envelope/migrated org must decrypt via keyset, not legacyCrypto")
	})

	t.Run("legacy-mode org routes to legacyCrypto", func(t *testing.T) {
		t.Parallel()

		// No registry record -> legacy mode (KMS vendor none shape), nil keyset manager.
		registryRepo := &serviceTestRegistryRepo{
			records: map[string]*mmodel.OrganizationRegistryRecord{},
		}
		stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

		svc := NewEncryptionService(stateResolver, nil, nil, newTestLegacyKeyMaterial(t), NewProtectionMetrics(nil))

		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-route")

		got, derr := svc.Decrypt(ctx, fieldCtx, *encrypted)
		require.NoError(t, derr)
		assert.Equal(t, plaintext, got)
	})

	t.Run("envelope-only org fails and never consults legacyCrypto", func(t *testing.T) {
		t.Parallel()

		// Born-envelope org: keyset has NO legacy key. Even with CanReadLegacy
		// allowed, an unmarked legacy ciphertext routes to the per-org keyset
		// (MustUseEnvelope) and MUST fail, never falling back to the
		// process-global legacyCrypto (the org never wrote legacy data).
		factory := tink.NewKeysetFactory(identityKMS{})

		envAEAD, gerr := factory.GenerateAEADKeyset(context.Background(), "transit", "crm-org-envelope")
		require.NoError(t, gerr)

		envPRF, gerr := factory.GeneratePRFKeyset(context.Background(), "transit", "crm-org-envelope")
		require.NoError(t, gerr)

		km := newKeysetManagerForKeyset(t, "org-envelope", envAEAD, envPRF)

		registryRepo := &serviceTestRegistryRepo{
			records: map[string]*mmodel.OrganizationRegistryRecord{
				"org-envelope": {
					TenantID:       "tenant-route",
					OrganizationID: "org-envelope",
					Status:         mmodel.RegistryStatusActive,
					LegacyReadable: true,
					CurrentVersion: 1,
				},
			},
		}
		stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

		spy := &spyLegacyCrypto{}
		svc := NewEncryptionService(stateResolver, km, nil, spy, NewProtectionMetrics(nil), crypto.EncryptionModeEnvelope)

		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-route")

		envelopeOnlyFieldCtx := FieldContext{
			TenantID:       "tenant-route",
			OrganizationID: "org-envelope",
			RecordID:       "rec-1",
			FieldName:      "document",
		}

		_, derr := svc.Decrypt(ctx, envelopeOnlyFieldCtx, *encrypted)
		require.Error(t, derr, "envelope-only org must not decrypt unmarked legacy ciphertext")
		assert.False(t, spy.decryptCalled, "envelope-only org must route to keyset, never to process-global legacyCrypto")
	})

	t.Run("CanReadLegacy false short-circuits before any decrypt", func(t *testing.T) {
		t.Parallel()

		factory := tink.NewKeysetFactory(identityKMS{})

		mixedAEAD, gerr := factory.GenerateMixedAEADKeyset(context.Background(), "transit", "crm-org-migrated", helperLegacyHexKey)
		require.NoError(t, gerr)

		mixedPRF, gerr := factory.GenerateMixedPRFKeyset(context.Background(), "transit", "crm-org-migrated", helperLegacySecret)
		require.NoError(t, gerr)

		km := newKeysetManagerForKeyset(t, "org-migrated", mixedAEAD, mixedPRF)

		registryRepo := &serviceTestRegistryRepo{
			records: map[string]*mmodel.OrganizationRegistryRecord{
				"org-migrated": {
					TenantID:       "tenant-route",
					OrganizationID: "org-migrated",
					Status:         mmodel.RegistryStatusActive,
					LegacyReadable: false,
					CurrentVersion: 1,
				},
			},
		}
		stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

		spy := &spyLegacyCrypto{}
		svc := NewEncryptionService(stateResolver, km, nil, spy, NewProtectionMetrics(nil), crypto.EncryptionModeEnvelope)

		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-route")

		_, derr := svc.Decrypt(ctx, fieldCtx, *encrypted)
		require.Error(t, derr)
		assert.ErrorIs(t, derr, ErrLegacyReadNotAllowed)
		assert.False(t, spy.decryptCalled, "rejection must short-circuit before any decrypt branch")
	})
}
