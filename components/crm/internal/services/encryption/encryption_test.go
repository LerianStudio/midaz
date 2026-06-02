// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Fakes and Helpers
// ---------------------------------------------------------------------------

// serviceTestRegistryReader implements RegistryReader for encryption service tests.
type serviceTestRegistryReader struct {
	records map[string]*mmodel.OrganizationRegistryRecord
	err     error
}

func (f *serviceTestRegistryReader) Get(_ context.Context, organizationID string) (*mmodel.OrganizationRegistryRecord, error) {
	if f.err != nil {
		return nil, f.err
	}

	record, ok := f.records[organizationID]
	if !ok {
		return nil, errors.New("registry not found")
	}

	return record, nil
}

// serviceTestKeysetReader implements KeysetReader for encryption service tests.
type serviceTestKeysetReader struct {
	keysets map[string]*mmodel.OrganizationKeyset
	err     error
}

func (f *serviceTestKeysetReader) Get(_ context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	if f.err != nil {
		return nil, f.err
	}

	keyset, ok := f.keysets[organizationID]
	if !ok {
		return nil, errors.New("keyset not found")
	}

	return keyset, nil
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

// fakeLegacyCrypto implements a basic legacy crypto interface for tests.
type fakeLegacyCrypto struct {
	encryptedValues map[string]string // plaintext -> ciphertext
	decryptedValues map[string]string // ciphertext -> plaintext
	hashValues      map[string]string // plaintext -> hash
	encryptErr      error
	decryptErr      error
}

func newFakeLegacyCrypto() *fakeLegacyCrypto {
	return &fakeLegacyCrypto{
		encryptedValues: make(map[string]string),
		decryptedValues: make(map[string]string),
		hashValues:      make(map[string]string),
	}
}

func (f *fakeLegacyCrypto) Encrypt(value *string) (*string, error) {
	if f.encryptErr != nil {
		return nil, f.encryptErr
	}

	if value == nil {
		return nil, nil
	}

	encrypted := "legacy-encrypted:" + *value
	f.encryptedValues[*value] = encrypted
	f.decryptedValues[encrypted] = *value

	return &encrypted, nil
}

func (f *fakeLegacyCrypto) Decrypt(value *string) (*string, error) {
	if f.decryptErr != nil {
		return nil, f.decryptErr
	}

	if value == nil {
		return nil, nil
	}

	decrypted, ok := f.decryptedValues[*value]
	if !ok {
		return nil, errors.New("cannot decrypt: unknown ciphertext")
	}

	return &decrypted, nil
}

func (f *fakeLegacyCrypto) GenerateHash(value *string) string {
	if value == nil {
		return ""
	}

	hash := "legacy-hash:" + *value
	f.hashValues[*value] = hash

	return hash
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
func createEncryptionTestService(t *testing.T, state ProtectionState, legacyCrypto LegacyCrypto) (EncryptionService, *mmodel.OrganizationKeyset) {
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

	keysetReader := &serviceTestKeysetReader{
		keysets: map[string]*mmodel.OrganizationKeyset{
			state.OrganizationID: keyset,
		},
	}

	unwrapper := &serviceTestKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	keysetManager := NewKeysetManager(keysetReader, unwrapper, DefaultKeysetManagerConfig())

	registryReader := &serviceTestRegistryReader{
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

	stateResolver := NewProtectionStateResolver(registryReader)

	svc := NewEncryptionService(stateResolver, keysetManager, keysetReader, legacyCrypto)

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

	legacyCrypto := newFakeLegacyCrypto()
	svc, keyset := createEncryptionTestService(t, state, legacyCrypto)

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
	state := ProtectionState{
		Mode:                 crypto.EncryptionModeLegacy,
		CanReadLegacy:        true,
		CurrentKeysetVersion: 0,
		OrganizationID:       "org-legacy",
		TenantID:             "tenant-legacy",
	}

	legacyCrypto := newFakeLegacyCrypto()

	// Create service with legacy-mode registry
	registryReader := &serviceTestRegistryReader{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			state.OrganizationID: {
				TenantID:       state.TenantID,
				OrganizationID: state.OrganizationID,
				Status:         mmodel.RegistryStatusLegacy,
				LegacyReadable: true,
				CurrentVersion: 0,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryReader)

	// KeysetManager and KeysetReader can be nil for legacy mode since they won't be used
	svc := NewEncryptionService(stateResolver, nil, nil, legacyCrypto)

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

	// Verify legacy crypto was used
	assert.Equal(t, "legacy-encrypted:ABC123", ciphertext)
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

	legacyCrypto := newFakeLegacyCrypto()
	svc, _ := createEncryptionTestService(t, state, legacyCrypto)

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
	keysetReader := &serviceTestKeysetReader{
		err: errors.New("keyset not found"),
	}

	unwrapper := &serviceTestKeysetUnwrapper{}

	keysetManager := NewKeysetManager(keysetReader, unwrapper, DefaultKeysetManagerConfig())

	registryReader := &serviceTestRegistryReader{
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
	stateResolver := NewProtectionStateResolver(registryReader)

	legacyCrypto := newFakeLegacyCrypto()
	svc := NewEncryptionService(stateResolver, keysetManager, keysetReader, legacyCrypto)

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

	legacyCrypto := newFakeLegacyCrypto()
	svc, _ := createEncryptionTestService(t, state, legacyCrypto)

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

	legacyCrypto := newFakeLegacyCrypto()

	// Pre-populate decryption mapping
	legacyCiphertext := "legacy-encrypted:secret-value"
	legacyCrypto.decryptedValues[legacyCiphertext] = "secret-value"

	// Create service with envelope mode but legacy read allowed
	registryReader := &serviceTestRegistryReader{
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
	stateResolver := NewProtectionStateResolver(registryReader)

	svc := NewEncryptionService(stateResolver, nil, nil, legacyCrypto)

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "document",
	}

	// Decrypt legacy ciphertext (no envelope marker)
	decrypted, err := svc.Decrypt(ctx, fieldCtx, legacyCiphertext)
	require.NoError(t, err)
	assert.Equal(t, "secret-value", decrypted)
}

func TestService_Decrypt_LegacyNotAllowed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	legacyCrypto := newFakeLegacyCrypto()
	legacyCiphertext := "legacy-encrypted:secret-value"

	// Create service with envelope mode and legacy read NOT allowed
	registryReader := &serviceTestRegistryReader{
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
	stateResolver := NewProtectionStateResolver(registryReader)

	svc := NewEncryptionService(stateResolver, nil, nil, legacyCrypto)

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "document",
	}

	// Try to decrypt legacy ciphertext - should fail
	_, err := svc.Decrypt(ctx, fieldCtx, legacyCiphertext)
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

	legacyCrypto := newFakeLegacyCrypto()
	svc, keyset := createEncryptionTestService(t, state, legacyCrypto)

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

	legacyCrypto := newFakeLegacyCrypto()
	svc, _ := createEncryptionTestService(t, state, legacyCrypto)

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

	legacyCrypto := newFakeLegacyCrypto()
	svc, _ := createEncryptionTestService(t, state, legacyCrypto)

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

	legacyCrypto := newFakeLegacyCrypto()
	svc, _ := createEncryptionTestService(t, state, legacyCrypto)

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

	legacyCrypto := newFakeLegacyCrypto()
	svc, _ := createEncryptionTestService(t, state, legacyCrypto)

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

	legacyCrypto := newFakeLegacyCrypto()

	// Create service with legacy mode
	registryReader := &serviceTestRegistryReader{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-legacy": {
				TenantID:       "tenant-legacy",
				OrganizationID: "org-legacy",
				Status:         mmodel.RegistryStatusLegacy,
				LegacyReadable: true,
				CurrentVersion: 0,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryReader)

	svc := NewEncryptionService(stateResolver, nil, nil, legacyCrypto)

	searchCtx := SearchTokenContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		FieldName:      "document",
	}

	token, err := svc.GenerateSearchToken(ctx, searchCtx, "ABC123")
	require.NoError(t, err)

	// Legacy mode should use legacy hash
	assert.Contains(t, token, "legacy-hash")
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

	legacyCrypto := newFakeLegacyCrypto()
	svc, _ := createEncryptionTestService(t, state, legacyCrypto)

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

	registryReader := &serviceTestRegistryReader{
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
	stateResolver := NewProtectionStateResolver(registryReader)

	svc := NewEncryptionService(stateResolver, nil, nil, nil)

	mustUse, err := svc.MustUseEnvelope(ctx, "org-envelope")
	require.NoError(t, err)
	assert.True(t, mustUse)
}

func TestService_MustUseEnvelope_LegacyMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	registryReader := &serviceTestRegistryReader{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-legacy": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-legacy",
				Status:         mmodel.RegistryStatusLegacy,
				LegacyReadable: true,
				CurrentVersion: 0,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryReader)

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

	legacyCrypto := newFakeLegacyCrypto()
	svc, _ := createEncryptionTestService(t, state, legacyCrypto)

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

	legacyCrypto := newFakeLegacyCrypto()
	svc, _ := createEncryptionTestService(t, state, legacyCrypto)

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

	legacyCrypto := newFakeLegacyCrypto()
	svc, _ := createEncryptionTestService(t, state, legacyCrypto)

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

	legacyCrypto := newFakeLegacyCrypto()
	svc, _ := createEncryptionTestService(t, state, legacyCrypto)

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

	legacyCrypto := newFakeLegacyCrypto()
	svc, _ := createEncryptionTestService(t, state, legacyCrypto)

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

	legacyCrypto := newFakeLegacyCrypto()
	svc, _ := createEncryptionTestService(t, state, legacyCrypto)

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

	registryReader := &serviceTestRegistryReader{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-123": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				Status:         mmodel.RegistryStatusActive,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryReader)

	svc := NewEncryptionService(stateResolver, nil, nil, nil)

	_, err := svc.MustUseEnvelope(ctx, "org-123")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestService_GetProtectionState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	registryReader := &serviceTestRegistryReader{
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
	stateResolver := NewProtectionStateResolver(registryReader)

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

	registryReader := &serviceTestRegistryReader{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-123": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				Status:         mmodel.RegistryStatusActive,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryReader)

	svc := NewEncryptionService(stateResolver, nil, nil, nil)

	_, err := svc.GetProtectionState(ctx, "org-123")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestService_GetKeysetInfo(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	keysetReader := &serviceTestKeysetReader{
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

	svc := NewEncryptionService(nil, nil, keysetReader, nil)

	info, err := svc.GetKeysetInfo(ctx, "org-123")
	require.NoError(t, err)
	assert.Equal(t, uint32(12345), info.PrimaryKeyID)
}

func TestService_GetKeysetInfo_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	keysetReader := &serviceTestKeysetReader{
		keysets: map[string]*mmodel.OrganizationKeyset{
			"org-123": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
			},
		},
	}

	svc := NewEncryptionService(nil, nil, keysetReader, nil)

	_, err := svc.GetKeysetInfo(ctx, "org-123")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestService_GetKeysetInfo_NotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	keysetReader := &serviceTestKeysetReader{
		keysets: map[string]*mmodel.OrganizationKeyset{},
	}

	svc := NewEncryptionService(nil, nil, keysetReader, nil)

	_, err := svc.GetKeysetInfo(ctx, "org-nonexistent")
	require.Error(t, err)
}

func TestService_Encrypt_LegacyMode_NilResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	legacyCrypto := &fakeLegacyCryptoNilResult{}

	registryReader := &serviceTestRegistryReader{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-legacy": {
				TenantID:       "tenant-legacy",
				OrganizationID: "org-legacy",
				Status:         mmodel.RegistryStatusLegacy,
				LegacyReadable: true,
				CurrentVersion: 0,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryReader)

	svc := NewEncryptionService(stateResolver, nil, nil, legacyCrypto)

	fieldCtx := FieldContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		RecordID:       "record-789",
		FieldName:      "document",
	}

	ciphertext, err := svc.Encrypt(ctx, fieldCtx, "plaintext")
	require.NoError(t, err)
	assert.Equal(t, "", ciphertext)
}

func TestService_Decrypt_LegacyMode_NilResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	legacyCrypto := &fakeLegacyCryptoNilResult{}

	registryReader := &serviceTestRegistryReader{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-legacy": {
				TenantID:       "tenant-legacy",
				OrganizationID: "org-legacy",
				Status:         mmodel.RegistryStatusLegacy,
				LegacyReadable: true,
				CurrentVersion: 0,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryReader)

	svc := NewEncryptionService(stateResolver, nil, nil, legacyCrypto)

	fieldCtx := FieldContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		RecordID:       "record-789",
		FieldName:      "document",
	}

	plaintext, err := svc.Decrypt(ctx, fieldCtx, "ciphertext")
	require.NoError(t, err)
	assert.Equal(t, "", plaintext)
}

// fakeLegacyCryptoNilResult returns nil from Encrypt/Decrypt to test nil handling.
type fakeLegacyCryptoNilResult struct{}

func (f *fakeLegacyCryptoNilResult) Encrypt(_ *string) (*string, error) {
	return nil, nil
}

func (f *fakeLegacyCryptoNilResult) Decrypt(_ *string) (*string, error) {
	return nil, nil
}

func (f *fakeLegacyCryptoNilResult) GenerateHash(_ *string) string {
	return ""
}

func TestService_Encrypt_StateResolverError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	registryReader := &serviceTestRegistryReader{
		err: errors.New("registry error"),
	}
	stateResolver := NewProtectionStateResolver(registryReader)

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

	registryReader := &serviceTestRegistryReader{
		err: errors.New("registry error"),
	}
	stateResolver := NewProtectionStateResolver(registryReader)

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
