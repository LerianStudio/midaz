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

func (f *serviceTestKeysetUnwrapper) UnwrapKeyset(_ context.Context, _ string, wrappedKeyset string) ([]byte, error) {
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

	// Generate AEAD keyset
	aeadGen := tink.NewAEADKeysetGenerator()
	aeadHandle, aeadBytes, err := aeadGen.Generate()
	require.NoError(t, err)

	aeadInfo, err := aeadGen.ExtractInfo(aeadHandle)
	require.NoError(t, err)

	// Generate MAC keyset
	macGen := tink.NewMACKeysetGenerator()
	macHandle, macBytes, err := macGen.Generate()
	require.NoError(t, err)

	macInfo, err := macGen.ExtractInfo(macHandle)
	require.NoError(t, err)

	return aeadBytes, macBytes, aeadInfo.PrimaryKeyID, macInfo.PrimaryKeyID
}

// createEncryptionTestService creates a Service with test dependencies for envelope mode tests.
func createEncryptionTestService(t *testing.T, state ProtectionState, legacyKeys *LegacyKeyMaterial) (EncryptionService, *mmodel.OrganizationKeyset) {
	t.Helper()

	aeadBytes, macBytes, aeadKeyID, _ := generateServiceTestKeysets(t)

	keyset := &mmodel.OrganizationKeyset{
		TenantID:       state.TenantID,
		OrganizationID: state.OrganizationID,
		KEKPath:        "test-kek",
		WrappedKeyset:  "wrapped-aead",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: aeadKeyID,
		},
		WrappedHMACKeyset: "wrapped-hmac",
		HMACKeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: aeadKeyID, // Using same ID for simplicity
		},
	}

	keysetRepo := &serviceTestKeysetRepo{
		keysets: map[string]*mmodel.OrganizationKeyset{
			state.OrganizationID: keyset,
		},
	}

	unwrapper := &serviceTestKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	keysetManager := NewKeysetManager(keysetRepo, unwrapper, nil, DefaultKeysetManagerConfig())

	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			state.OrganizationID: {
				TenantID:       state.TenantID,
				OrganizationID: state.OrganizationID,
				Status:         mmodel.RegistryStatusActive,
				LegacyReadable: state.CanReadLegacy,
				CurrentVersion: state.CurrentKeysetVersion,
			},
		},
	}

	stateResolver := NewProtectionStateResolver(registryRepo)

	svc := NewEncryptionService(stateResolver, keysetManager, keysetRepo, legacyKeys)

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
	assert.Equal(t, keyset.KeysetInfo.PrimaryKeyID, marker.KeyID)
}

func TestService_Encrypt_LegacyMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	legacyKeys := newTestLegacyKeyMaterial(t)

	// Create service with empty registry (no record = legacy mode)
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo)

	// KeysetManager and KeysetReader can be nil for legacy mode since they won't be used
	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys)

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

	keysetManager := NewKeysetManager(keysetRepo, unwrapper, nil, DefaultKeysetManagerConfig())

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
	stateResolver := NewProtectionStateResolver(registryRepo)

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc := NewEncryptionService(stateResolver, keysetManager, keysetRepo, legacyKeys)

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

	ctx := context.Background()

	legacyKeys := newTestLegacyKeyMaterial(t)

	// Create actual legacy ciphertext using Tink-backed legacy crypto
	plaintext := "secret-value"
	cipherBytes, err := legacyKeys.aead.Encrypt([]byte(plaintext), nil)
	require.NoError(t, err)
	legacyCiphertext := base64.StdEncoding.EncodeToString(cipherBytes)

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
	stateResolver := NewProtectionStateResolver(registryRepo)

	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys)

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "document",
	}

	// Decrypt legacy ciphertext (no envelope marker)
	decrypted, err := svc.Decrypt(ctx, fieldCtx, legacyCiphertext)
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
	stateResolver := NewProtectionStateResolver(registryRepo)

	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys)

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

	// Create a marked ciphertext with bogus payload
	bogusMarkedCiphertext := FormatEnvelopeMarker(keyset.KeysetInfo.PrimaryKeyID, []byte("invalid-ciphertext"))

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

	token, err := svc.GenerateSearchToken(ctx, searchCtx, "ABC123")
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
	token1, err := svc.GenerateSearchToken(ctx, searchCtx, "ABC123")
	require.NoError(t, err)

	token2, err := svc.GenerateSearchToken(ctx, searchCtx, "ABC123")
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

	token1, err := svc.GenerateSearchToken(ctx, searchCtx, "ABC123")
	require.NoError(t, err)

	token2, err := svc.GenerateSearchToken(ctx, searchCtx, "XYZ789")
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
	stateResolver := NewProtectionStateResolver(registryRepo)

	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		FieldName:      "document",
	}

	normalizedValue := "ABC123"
	token, err := svc.GenerateSearchToken(ctx, searchCtx, normalizedValue)
	require.NoError(t, err)

	// Legacy mode uses Tink-backed HMAC-SHA256 hex token matching lib-commons format
	expectedToken := legacyKeys.legacySearchToken(normalizedValue)
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

			_, err := svc.GenerateSearchToken(ctx, tt.searchCtx, "value")
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
	stateResolver := NewProtectionStateResolver(registryRepo)

	svc := NewEncryptionService(stateResolver, nil, nil, nil)

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
	stateResolver := NewProtectionStateResolver(registryRepo)

	svc := NewEncryptionService(stateResolver, nil, nil, nil)

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

	_, err := svc.GenerateSearchToken(ctx, searchCtx, "value")
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
	stateResolver := NewProtectionStateResolver(registryRepo)

	svc := NewEncryptionService(stateResolver, nil, nil, nil)

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
	stateResolver := NewProtectionStateResolver(registryRepo)

	svc := NewEncryptionService(stateResolver, nil, nil, nil)

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
	stateResolver := NewProtectionStateResolver(registryRepo)

	svc := NewEncryptionService(stateResolver, nil, nil, nil)

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

	svc := NewEncryptionService(nil, nil, keysetRepo, nil)

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

	svc := NewEncryptionService(nil, nil, keysetRepo, nil)

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

	svc := NewEncryptionService(nil, nil, keysetRepo, nil)

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
	stateResolver := NewProtectionStateResolver(registryRepo)

	// Service with nil legacy key material
	svc := NewEncryptionService(stateResolver, nil, nil, nil)

	fieldCtx := FieldContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		RecordID:       "record-789",
		FieldName:      "document",
	}

	// Should fail with nil legacy key material
	_, err := svc.Encrypt(ctx, fieldCtx, "plaintext")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "legacy key material is required")
}

func TestService_Decrypt_NilLegacyKeyMaterial(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Empty registry = legacy mode (no record found)
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{},
	}
	stateResolver := NewProtectionStateResolver(registryRepo)

	// Service with nil legacy key material
	svc := NewEncryptionService(stateResolver, nil, nil, nil)

	fieldCtx := FieldContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		RecordID:       "record-789",
		FieldName:      "document",
	}

	// Should fail with nil legacy key material (ciphertext without envelope marker)
	_, err := svc.Decrypt(ctx, fieldCtx, "some-non-envelope-ciphertext")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "legacy key material is required")
}

func TestService_Encrypt_StateResolverError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	registryRepo := &serviceTestRegistryRepo{
		err: errors.New("registry error"),
	}
	stateResolver := NewProtectionStateResolver(registryRepo)

	svc := NewEncryptionService(stateResolver, nil, nil, nil)

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
	stateResolver := NewProtectionStateResolver(registryRepo)

	svc := NewEncryptionService(stateResolver, nil, nil, nil)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	_, err := svc.GenerateSearchToken(ctx, searchCtx, "value")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve protection state")
}

// ---------------------------------------------------------------------------
// Global Mode Tests (ST-002-01: Lazy Provisioning via Global Mode)
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
	stateResolver := NewProtectionStateResolver(registryRepo)

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

	keysetManager := NewKeysetManager(keysetRepo, unwrapper, mockProvisioner, DefaultKeysetManagerConfig())

	legacyKeys := newTestLegacyKeyMaterial(t)

	// KEY CHANGE: Pass globalMode = EncryptionModeEnvelope to constructor
	// This tells the service to use envelope encryption globally, triggering
	// lazy provisioning even when per-org registry does not exist
	svc := NewEncryptionService(stateResolver, keysetManager, keysetRepo, legacyKeys, crypto.EncryptionModeEnvelope)

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
		MACPrimaryKeyID:  12346,
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

	// Build resolver with nil registry repo (KMS_VENDOR=none scenario)
	resolver := NewProtectionStateResolver(nil)

	// Build service with nil keyset manager and nil keyset repo (legacy-only mode)
	svc := NewEncryptionService(resolver, nil, nil, legacyKeys)

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

	token, err := svc.GenerateSearchToken(ctx, searchCtx, plaintext)
	require.NoError(t, err)

	// Compare with expected HMAC-SHA256 hex token
	expectedToken := legacyKeys.legacySearchToken(plaintext)
	assert.Equal(t, expectedToken, token)
}

func TestService_SearchRouting_LegacyTokenAndEnvelopeTokenDifferByMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	normalizedValue := "ABC123"

	// Build legacy mode service with imported legacy key
	legacyKeys, err := NewLegacyKeyMaterial(legacyEncryptHexKey, legacyHashKey)
	require.NoError(t, err)

	legacyResolver := NewProtectionStateResolver(nil)
	legacySvc := NewEncryptionService(legacyResolver, nil, nil, legacyKeys)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		FieldName:      "document",
	}

	// Generate legacy token
	legacyToken, err := legacySvc.GenerateSearchToken(ctx, searchCtx, normalizedValue)
	require.NoError(t, err)

	// Assert legacy token equals expected hex HMAC
	expectedLegacyToken := legacyKeys.legacySearchToken(normalizedValue)
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
	envelopeToken, err := envelopeSvc.GenerateSearchToken(ctx, searchCtx, normalizedValue)
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
	stateResolver := NewProtectionStateResolver(registryRepo)

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

	keysetManager := NewKeysetManager(keysetRepo, unwrapper, mockProvisioner, DefaultKeysetManagerConfig())

	legacyKeys := newTestLegacyKeyMaterial(t)

	// KEY CHANGE: Pass globalMode = EncryptionModeEnvelope to constructor
	// This tells the service to use envelope encryption globally, triggering
	// lazy provisioning even when per-org registry does not exist
	svc := NewEncryptionService(stateResolver, keysetManager, keysetRepo, legacyKeys, crypto.EncryptionModeEnvelope)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-new", // Organization with no existing registry
		FieldName:      "document",
	}

	normalizedValue := "ABC123"

	// Execute: GenerateSearchToken should use envelope mode (triggering lazy provisioning)
	// NOT legacy mode, even though ProtectionStateResolver returns legacy
	token, err := svc.GenerateSearchToken(ctx, searchCtx, normalizedValue)
	require.NoError(t, err)

	// Verify: Token is not empty
	assert.NotEmpty(t, token, "search token MUST NOT be empty")

	// Verify: Legacy crypto hash was NOT used
	assert.NotContains(t, token, "legacy-hash", "legacy hash MUST NOT be used when globalMode is envelope")

	// Verify: Provisioner was called (lazy provisioning triggered)
	assert.True(t, mockProvisioner.provisionCalled, "lazy provisioning MUST be triggered for non-provisioned org when globalMode is envelope")
}
