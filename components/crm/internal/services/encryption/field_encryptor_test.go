// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFieldEncryptor_Interface verifies that FieldEncryptor interface exists
// and has the expected method signatures for use in repository adapters.
func TestFieldEncryptor_Interface(t *testing.T) {
	t.Parallel()

	// This test validates the interface contract exists.
	// It will fail until FieldEncryptor interface is implemented.
	var _ FieldEncryptor = (*fieldEncryptorAdapter)(nil)
}

// TestFieldEncryptor_EncryptField verifies EncryptField method exists
// and accepts proper FieldContext with all required fields.
func TestFieldEncryptor_EncryptField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		tenantID       string
		organizationID string
		recordID       string
		fieldName      string
		plaintext      string
		wantErr        bool
	}{
		{
			name:           "encrypts document field with valid context",
			tenantID:       "tenant-001",
			organizationID: uuid.New().String(),
			recordID:       uuid.New().String(),
			fieldName:      "document",
			plaintext:      "123.456.789-00",
			wantErr:        false,
		},
		{
			name:           "encrypts name field with valid context",
			tenantID:       "tenant-001",
			organizationID: uuid.New().String(),
			recordID:       uuid.New().String(),
			fieldName:      "name",
			plaintext:      "John Doe",
			wantErr:        false,
		},
		{
			name:           "encrypts contact.primary_email with nested field name",
			tenantID:       "tenant-001",
			organizationID: uuid.New().String(),
			recordID:       uuid.New().String(),
			fieldName:      "contact.primary_email",
			plaintext:      "john@example.com",
			wantErr:        false,
		},
		{
			name:           "returns error when tenant_id is empty",
			tenantID:       "",
			organizationID: uuid.New().String(),
			recordID:       uuid.New().String(),
			fieldName:      "document",
			plaintext:      "123.456.789-00",
			wantErr:        true,
		},
		{
			name:           "returns error when organization_id is empty",
			tenantID:       "tenant-001",
			organizationID: "",
			recordID:       uuid.New().String(),
			fieldName:      "document",
			plaintext:      "123.456.789-00",
			wantErr:        true,
		},
		{
			name:           "returns error when record_id is empty",
			tenantID:       "tenant-001",
			organizationID: uuid.New().String(),
			recordID:       "",
			fieldName:      "document",
			plaintext:      "123.456.789-00",
			wantErr:        true,
		},
		{
			name:           "returns error when field_name is empty",
			tenantID:       "tenant-001",
			organizationID: uuid.New().String(),
			recordID:       uuid.New().String(),
			fieldName:      "",
			plaintext:      "123.456.789-00",
			wantErr:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			// Create mock encryption service that returns predictable results
			mockSvc := &mockEncryptionService{
				encryptFunc: func(_ context.Context, fieldCtx FieldContext, plaintext string) (string, error) {
					if err := fieldCtx.Validate(); err != nil {
						return "", err
					}
					return "encrypted:" + plaintext, nil
				},
			}

			// Create the adapter - this will fail compilation until FieldEncryptor is implemented
			adapter := NewFieldEncryptorAdapter(mockSvc)

			// Build FieldContext
			fieldCtx := FieldContext{
				TenantID:       tc.tenantID,
				OrganizationID: tc.organizationID,
				RecordID:       tc.recordID,
				FieldName:      tc.fieldName,
			}

			// Call EncryptField
			result, err := adapter.EncryptField(ctx, fieldCtx, tc.plaintext)

			if tc.wantErr {
				require.Error(t, err)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, result)
				assert.Equal(t, "encrypted:"+tc.plaintext, result)
			}
		})
	}
}

// TestFieldEncryptor_DecryptField verifies DecryptField method exists
// and properly decrypts values using FieldContext.
func TestFieldEncryptor_DecryptField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		tenantID       string
		organizationID string
		recordID       string
		fieldName      string
		ciphertext     string
		expected       string
		wantErr        bool
	}{
		{
			name:           "decrypts document field with valid context",
			tenantID:       "tenant-001",
			organizationID: uuid.New().String(),
			recordID:       uuid.New().String(),
			fieldName:      "document",
			ciphertext:     "encrypted:123.456.789-00",
			expected:       "123.456.789-00",
			wantErr:        false,
		},
		{
			name:           "decrypts name field with valid context",
			tenantID:       "tenant-001",
			organizationID: uuid.New().String(),
			recordID:       uuid.New().String(),
			fieldName:      "name",
			ciphertext:     "encrypted:John Doe",
			expected:       "John Doe",
			wantErr:        false,
		},
		{
			name:           "returns error when tenant_id is empty",
			tenantID:       "",
			organizationID: uuid.New().String(),
			recordID:       uuid.New().String(),
			fieldName:      "document",
			ciphertext:     "encrypted:123.456.789-00",
			expected:       "",
			wantErr:        true,
		},
		{
			name:           "returns error when field_name is empty",
			tenantID:       "tenant-001",
			organizationID: uuid.New().String(),
			recordID:       uuid.New().String(),
			fieldName:      "",
			ciphertext:     "encrypted:123.456.789-00",
			expected:       "",
			wantErr:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			// Create mock encryption service
			mockSvc := &mockEncryptionService{
				decryptFunc: func(_ context.Context, fieldCtx FieldContext, ciphertext string) (string, error) {
					if err := fieldCtx.Validate(); err != nil {
						return "", err
					}
					// Simple mock: remove "encrypted:" prefix
					if len(ciphertext) > 10 && ciphertext[:10] == "encrypted:" {
						return ciphertext[10:], nil
					}
					return ciphertext, nil
				},
			}

			// Create the adapter
			adapter := NewFieldEncryptorAdapter(mockSvc)

			// Build FieldContext
			fieldCtx := FieldContext{
				TenantID:       tc.tenantID,
				OrganizationID: tc.organizationID,
				RecordID:       tc.recordID,
				FieldName:      tc.fieldName,
			}

			// Call DecryptField
			result, err := adapter.DecryptField(ctx, fieldCtx, tc.ciphertext)

			if tc.wantErr {
				require.Error(t, err)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

// TestFieldEncryptor_GenerateSearchToken verifies GenerateSearchToken method exists
// and uses SearchTokenContext (without RecordID) for token generation.
func TestFieldEncryptor_GenerateSearchToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		tenantID        string
		organizationID  string
		fieldName       string
		normalizedValue string
		wantErr         bool
	}{
		{
			name:            "generates search token for document field",
			tenantID:        "tenant-001",
			organizationID:  uuid.New().String(),
			fieldName:       "document",
			normalizedValue: "12345678900",
			wantErr:         false,
		},
		{
			name:            "generates search token for tax_id field",
			tenantID:        "tenant-001",
			organizationID:  uuid.New().String(),
			fieldName:       "tax_id",
			normalizedValue: "98765432100",
			wantErr:         false,
		},
		{
			name:            "returns error when tenant_id is empty",
			tenantID:        "",
			organizationID:  uuid.New().String(),
			fieldName:       "document",
			normalizedValue: "12345678900",
			wantErr:         true,
		},
		{
			name:            "returns error when organization_id is empty",
			tenantID:        "tenant-001",
			organizationID:  "",
			fieldName:       "document",
			normalizedValue: "12345678900",
			wantErr:         true,
		},
		{
			name:            "returns error when field_name is empty",
			tenantID:        "tenant-001",
			organizationID:  uuid.New().String(),
			fieldName:       "",
			normalizedValue: "12345678900",
			wantErr:         true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			// Create mock encryption service
			mockSvc := &mockEncryptionService{
				generateSearchTokenFunc: func(_ context.Context, searchCtx SearchTokenContext, normalizedValue string) (string, error) {
					if err := searchCtx.Validate(); err != nil {
						return "", err
					}
					return "token:" + normalizedValue, nil
				},
			}

			// Create the adapter
			adapter := NewFieldEncryptorAdapter(mockSvc)

			// Build SearchTokenContext (note: no RecordID)
			searchCtx := SearchTokenContext{
				TenantID:       tc.tenantID,
				OrganizationID: tc.organizationID,
				FieldName:      tc.fieldName,
			}

			// Call GenerateSearchToken
			result, err := adapter.GenerateSearchToken(ctx, searchCtx, tc.normalizedValue)

			if tc.wantErr {
				require.Error(t, err)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, result)
				assert.Equal(t, "token:"+tc.normalizedValue, result)
			}
		})
	}
}

// TestFieldEncryptor_ContextBinding verifies that FieldContext properly binds
// tenant_id, organization_id, record_id, and field_name for AAD construction.
func TestFieldEncryptor_ContextBinding(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tenantID := "test-tenant"
	organizationID := uuid.New().String()
	recordID := uuid.New().String()
	fieldName := "document"

	var capturedFieldCtx FieldContext

	mockSvc := &mockEncryptionService{
		encryptFunc: func(_ context.Context, fieldCtx FieldContext, plaintext string) (string, error) {
			capturedFieldCtx = fieldCtx
			return "encrypted:" + plaintext, nil
		},
	}

	adapter := NewFieldEncryptorAdapter(mockSvc)

	fieldCtx := FieldContext{
		TenantID:       tenantID,
		OrganizationID: organizationID,
		RecordID:       recordID,
		FieldName:      fieldName,
	}

	_, err := adapter.EncryptField(ctx, fieldCtx, "test-plaintext")
	require.NoError(t, err)

	// Verify all context fields are properly passed through
	assert.Equal(t, tenantID, capturedFieldCtx.TenantID)
	assert.Equal(t, organizationID, capturedFieldCtx.OrganizationID)
	assert.Equal(t, recordID, capturedFieldCtx.RecordID)
	assert.Equal(t, fieldName, capturedFieldCtx.FieldName)

	// Verify canonical AAD format
	expectedAAD := []byte("tenant:" + tenantID + ":org:" + organizationID + ":record:" + recordID + ":field:" + fieldName)
	assert.Equal(t, expectedAAD, capturedFieldCtx.CanonicalAAD())
}

// TestFieldEncryptor_SearchTokenContextBinding verifies that SearchTokenContext
// properly binds tenant_id, organization_id, and field_name (without record_id).
func TestFieldEncryptor_SearchTokenContextBinding(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tenantID := "test-tenant"
	organizationID := uuid.New().String()
	fieldName := "document"
	normalizedValue := "12345678900"

	var capturedSearchCtx SearchTokenContext

	mockSvc := &mockEncryptionService{
		generateSearchTokenFunc: func(_ context.Context, searchCtx SearchTokenContext, _ string) (string, error) {
			capturedSearchCtx = searchCtx
			return "token", nil
		},
	}

	adapter := NewFieldEncryptorAdapter(mockSvc)

	searchCtx := SearchTokenContext{
		TenantID:       tenantID,
		OrganizationID: organizationID,
		FieldName:      fieldName,
	}

	_, err := adapter.GenerateSearchToken(ctx, searchCtx, normalizedValue)
	require.NoError(t, err)

	// Verify context fields (note: no RecordID)
	assert.Equal(t, tenantID, capturedSearchCtx.TenantID)
	assert.Equal(t, organizationID, capturedSearchCtx.OrganizationID)
	assert.Equal(t, fieldName, capturedSearchCtx.FieldName)

	// Verify canonical input format
	expectedInput := []byte("tenant:" + tenantID + ":org:" + organizationID + ":field:" + fieldName + ":" + normalizedValue)
	assert.Equal(t, expectedInput, capturedSearchCtx.CanonicalInput(normalizedValue))
}

// TestFieldEncryptor_GenerateSearchTokenCandidates verifies GenerateSearchTokenCandidates
// method exists and properly delegates to EncryptionService.
func TestFieldEncryptor_GenerateSearchTokenCandidates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		tenantID        string
		organizationID  string
		fieldName       string
		normalizedValue string
		mockTokens      []string
		mockErr         error
		wantTokens      []string
		wantErr         bool
	}{
		{
			name:            "returns multiple tokens for key rotation support",
			tenantID:        "tenant-001",
			organizationID:  uuid.New().String(),
			fieldName:       "document",
			normalizedValue: "12345678900",
			mockTokens:      []string{"token_key1", "token_key2", "token_key3"},
			mockErr:         nil,
			wantTokens:      []string{"token_key1", "token_key2", "token_key3"},
			wantErr:         false,
		},
		{
			name:            "returns single token when only one key enabled",
			tenantID:        "tenant-001",
			organizationID:  uuid.New().String(),
			fieldName:       "tax_id",
			normalizedValue: "98765432100",
			mockTokens:      []string{"single_token"},
			mockErr:         nil,
			wantTokens:      []string{"single_token"},
			wantErr:         false,
		},
		{
			name:            "propagates error from EncryptionService",
			tenantID:        "tenant-001",
			organizationID:  uuid.New().String(),
			fieldName:       "document",
			normalizedValue: "12345678900",
			mockTokens:      nil,
			mockErr:         errors.New("MAC primitive construction failed"),
			wantTokens:      nil,
			wantErr:         true,
		},
		{
			name:            "returns error when tenant_id is empty",
			tenantID:        "",
			organizationID:  uuid.New().String(),
			fieldName:       "document",
			normalizedValue: "12345678900",
			mockTokens:      nil,
			mockErr:         nil,
			wantTokens:      nil,
			wantErr:         true,
		},
		{
			name:            "returns error when organization_id is empty",
			tenantID:        "tenant-001",
			organizationID:  "",
			fieldName:       "document",
			normalizedValue: "12345678900",
			mockTokens:      nil,
			mockErr:         nil,
			wantTokens:      nil,
			wantErr:         true,
		},
		{
			name:            "returns error when field_name is empty",
			tenantID:        "tenant-001",
			organizationID:  uuid.New().String(),
			fieldName:       "",
			normalizedValue: "12345678900",
			mockTokens:      nil,
			mockErr:         nil,
			wantTokens:      nil,
			wantErr:         true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			// Create mock encryption service
			mockSvc := &mockEncryptionService{
				generateSearchTokenCandidatesFunc: func(_ context.Context, searchCtx SearchTokenContext, _ string) ([]string, error) {
					if err := searchCtx.Validate(); err != nil {
						return nil, err
					}
					if tc.mockErr != nil {
						return nil, tc.mockErr
					}
					return tc.mockTokens, nil
				},
			}

			// Create the adapter
			adapter := NewFieldEncryptorAdapter(mockSvc)

			// Build SearchTokenContext (note: no RecordID)
			searchCtx := SearchTokenContext{
				TenantID:       tc.tenantID,
				OrganizationID: tc.organizationID,
				FieldName:      tc.fieldName,
			}

			// Call GenerateSearchTokenCandidates
			result, err := adapter.GenerateSearchTokenCandidates(ctx, searchCtx, tc.normalizedValue)

			if tc.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantTokens, result)
			}
		})
	}
}

// TestFieldEncryptor_GenerateSearchTokenCandidates_ContextBinding verifies that
// SearchTokenContext is properly passed through to EncryptionService.
func TestFieldEncryptor_GenerateSearchTokenCandidates_ContextBinding(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tenantID := "test-tenant"
	organizationID := uuid.New().String()
	fieldName := "document"
	normalizedValue := "12345678900"

	var capturedSearchCtx SearchTokenContext
	var capturedNormalizedValue string

	mockSvc := &mockEncryptionService{
		generateSearchTokenCandidatesFunc: func(_ context.Context, searchCtx SearchTokenContext, normValue string) ([]string, error) {
			capturedSearchCtx = searchCtx
			capturedNormalizedValue = normValue
			return []string{"token1", "token2"}, nil
		},
	}

	adapter := NewFieldEncryptorAdapter(mockSvc)

	searchCtx := SearchTokenContext{
		TenantID:       tenantID,
		OrganizationID: organizationID,
		FieldName:      fieldName,
	}

	tokens, err := adapter.GenerateSearchTokenCandidates(ctx, searchCtx, normalizedValue)
	require.NoError(t, err)
	assert.Len(t, tokens, 2)

	// Verify context fields are properly passed through
	assert.Equal(t, tenantID, capturedSearchCtx.TenantID)
	assert.Equal(t, organizationID, capturedSearchCtx.OrganizationID)
	assert.Equal(t, fieldName, capturedSearchCtx.FieldName)
	assert.Equal(t, normalizedValue, capturedNormalizedValue)
}

// mockEncryptionService is a test mock for EncryptionService.
// This mock is defined here to test the FieldEncryptor adapter.
type mockEncryptionService struct {
	encryptFunc                       func(ctx context.Context, fieldCtx FieldContext, plaintext string) (string, error)
	decryptFunc                       func(ctx context.Context, fieldCtx FieldContext, ciphertext string) (string, error)
	generateSearchTokenFunc           func(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) (string, error)
	generateSearchTokenCandidatesFunc func(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) ([]string, error)
}

func (m *mockEncryptionService) Encrypt(ctx context.Context, fieldCtx FieldContext, plaintext string) (string, error) {
	if m.encryptFunc != nil {
		return m.encryptFunc(ctx, fieldCtx, plaintext)
	}
	return "", nil
}

func (m *mockEncryptionService) Decrypt(ctx context.Context, fieldCtx FieldContext, ciphertext string) (string, error) {
	if m.decryptFunc != nil {
		return m.decryptFunc(ctx, fieldCtx, ciphertext)
	}
	return "", nil
}

func (m *mockEncryptionService) GenerateSearchToken(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) (string, error) {
	if m.generateSearchTokenFunc != nil {
		return m.generateSearchTokenFunc(ctx, searchCtx, normalizedValue)
	}
	return "", nil
}

func (m *mockEncryptionService) GenerateSearchTokenCandidates(ctx context.Context, searchCtx SearchTokenContext, normalizedValue string) ([]string, error) {
	if m.generateSearchTokenCandidatesFunc != nil {
		return m.generateSearchTokenCandidatesFunc(ctx, searchCtx, normalizedValue)
	}
	return nil, nil
}

func (m *mockEncryptionService) MustUseEnvelope(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *mockEncryptionService) GetProtectionState(_ context.Context, _ string) (ProtectionState, error) {
	return ProtectionState{}, nil
}

func (m *mockEncryptionService) GetKeysetInfo(_ context.Context, _ string) (*mmodel.KeysetInfo, error) {
	return nil, nil
}
