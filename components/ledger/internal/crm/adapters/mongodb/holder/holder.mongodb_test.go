// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"context"
	"errors"
	"testing"

	encryption "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"
)

// ============================================================================
// buildHolderFilter Tests
// ============================================================================

func TestBuildHolderFilter_ExcludeDeleted(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	repo := &MongoDBRepository{
		FieldEncryptor: fe,
	}

	tests := []struct {
		name           string
		query          http.QueryHeader
		includeDeleted bool
		wantDeletedAt  bool
	}{
		{
			name:           "excludes_deleted_by_default",
			query:          http.QueryHeader{Limit: 10, Page: 1},
			includeDeleted: false,
			wantDeletedAt:  true,
		},
		{
			name:           "includes_deleted_when_flag_true",
			query:          http.QueryHeader{Limit: 10, Page: 1},
			includeDeleted: true,
			wantDeletedAt:  false,
		},
	}

	ctx := context.Background()
	orgID := "test-org-123"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filter, err := repo.buildHolderFilter(ctx, orgID, tt.query, tt.includeDeleted)

			require.NoError(t, err)
			require.NotNil(t, filter)

			hasDeletedAt := false
			for _, elem := range filter {
				if elem.Key == "deleted_at" {
					hasDeletedAt = true
					assert.Nil(t, elem.Value, "deleted_at filter should check for nil")
				}
			}

			assert.Equal(t, tt.wantDeletedAt, hasDeletedAt,
				"deleted_at filter presence should match expectation")
		})
	}
}

func TestBuildHolderFilter_WithExternalID(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	repo := &MongoDBRepository{
		FieldEncryptor: fe,
	}

	externalID := "EXT-12345"

	tests := []struct {
		name           string
		query          http.QueryHeader
		wantExternalID bool
		wantValue      string
	}{
		{
			name: "includes_external_id_when_provided",
			query: http.QueryHeader{
				Limit:      10,
				Page:       1,
				ExternalID: &externalID,
			},
			wantExternalID: true,
			wantValue:      externalID,
		},
		{
			name: "excludes_external_id_when_nil",
			query: http.QueryHeader{
				Limit:      10,
				Page:       1,
				ExternalID: nil,
			},
			wantExternalID: false,
		},
		{
			name: "excludes_external_id_when_empty_string",
			query: http.QueryHeader{
				Limit:      10,
				Page:       1,
				ExternalID: testutils.Ptr(""),
			},
			wantExternalID: false,
		},
	}

	ctx := context.Background()
	orgID := "test-org-123"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filter, err := repo.buildHolderFilter(ctx, orgID, tt.query, false)

			require.NoError(t, err)
			require.NotNil(t, filter)

			hasExternalID := false
			for _, elem := range filter {
				if elem.Key == "external_id" {
					hasExternalID = true
					assert.Equal(t, tt.wantValue, elem.Value,
						"external_id value should match")
				}
			}

			assert.Equal(t, tt.wantExternalID, hasExternalID,
				"external_id filter presence should match expectation")
		})
	}
}

func TestBuildHolderFilter_WithDocument(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	repo := &MongoDBRepository{
		FieldEncryptor: fe,
	}

	document := "12345678901"

	tests := []struct {
		name         string
		query        http.QueryHeader
		wantDocument bool
	}{
		{
			name: "includes_document_tokens_with_in_operator_when_provided",
			query: http.QueryHeader{
				Limit:    10,
				Page:     1,
				Document: &document,
			},
			wantDocument: true,
		},
		{
			name: "excludes_document_when_nil",
			query: http.QueryHeader{
				Limit:    10,
				Page:     1,
				Document: nil,
			},
			wantDocument: false,
		},
		{
			name: "excludes_document_when_empty_string",
			query: http.QueryHeader{
				Limit:    10,
				Page:     1,
				Document: testutils.Ptr(""),
			},
			wantDocument: false,
		},
	}

	ctx := context.Background()
	orgID := "test-org-123"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filter, err := repo.buildHolderFilter(ctx, orgID, tt.query, false)

			require.NoError(t, err)
			require.NotNil(t, filter)

			hasDocument := false
			for _, elem := range filter {
				if elem.Key == "search.document" {
					hasDocument = true

					// Value should be bson.M with $in operator containing token candidates
					inFilter, ok := elem.Value.(bson.M)
					require.True(t, ok, "search.document value should be bson.M")

					inValue, hasIn := inFilter["$in"]
					require.True(t, hasIn, "search.document should use $in operator")

					// Value should be a slice of tokens
					tokens, ok := inValue.([]string)
					require.True(t, ok, "$in value should be []string")
					require.NotEmpty(t, tokens, "token candidates should not be empty")

					// Tokens should not contain the original document (they are hashes)
					for _, token := range tokens {
						assert.NotEqual(t, document, token,
							"tokens should be hashes, not plaintext")
					}
				}
			}

			assert.Equal(t, tt.wantDocument, hasDocument,
				"search.document filter presence should match expectation")
		})
	}
}

func TestBuildHolderFilter_WithMetadata(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	repo := &MongoDBRepository{
		FieldEncryptor: fe,
	}

	tests := []struct {
		name         string
		metadata     *bson.M
		wantKeys     []string
		wantValues   map[string]any
		expectFilter bool
	}{
		{
			name: "includes_single_metadata_key",
			metadata: &bson.M{
				"metadata.source": "api",
			},
			wantKeys:     []string{"metadata.source"},
			wantValues:   map[string]any{"metadata.source": "api"},
			expectFilter: true,
		},
		{
			name: "includes_multiple_metadata_keys",
			metadata: &bson.M{
				"metadata.key1": "value1",
				"metadata.key2": "value2",
			},
			wantKeys: []string{"metadata.key1", "metadata.key2"},
			wantValues: map[string]any{
				"metadata.key1": "value1",
				"metadata.key2": "value2",
			},
			expectFilter: true,
		},
		{
			name:         "excludes_metadata_when_nil",
			metadata:     nil,
			wantKeys:     nil,
			expectFilter: false,
		},
		{
			name:         "excludes_metadata_when_empty",
			metadata:     &bson.M{},
			wantKeys:     nil,
			expectFilter: false,
		},
	}

	ctx := context.Background()
	orgID := "test-org-123"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			query := http.QueryHeader{
				Limit:    10,
				Page:     1,
				Metadata: tt.metadata,
			}

			filter, err := repo.buildHolderFilter(ctx, orgID, query, false)

			require.NoError(t, err)
			require.NotNil(t, filter)

			// Collect found metadata keys
			foundKeys := make(map[string]any)
			for _, elem := range filter {
				if elem.Key != "deleted_at" {
					foundKeys[elem.Key] = elem.Value
				}
			}

			if tt.expectFilter {
				for _, key := range tt.wantKeys {
					assert.Contains(t, foundKeys, key,
						"filter should contain metadata key: %s", key)
					if tt.wantValues != nil {
						assert.Equal(t, tt.wantValues[key], foundKeys[key],
							"metadata value should match for key: %s", key)
					}
				}
			} else {
				// Only deleted_at should be present
				assert.Empty(t, foundKeys,
					"no metadata keys should be in filter when metadata is nil/empty")
			}
		})
	}
}

func TestBuildHolderFilter_InvalidMetadata(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	repo := &MongoDBRepository{
		FieldEncryptor: fe,
	}

	tests := []struct {
		name        string
		metadata    *bson.M
		errContains string
	}{
		{
			name: "rejects_nested_object_metadata",
			metadata: &bson.M{
				"metadata.nested": map[string]any{"invalid": "nested"},
			},
			errContains: "0067", // ErrInvalidMetadataNesting
		},
		{
			name: "rejects_array_metadata",
			metadata: &bson.M{
				"metadata.array": []string{"a", "b"},
			},
			errContains: "0047", // ErrBadRequest for array values
		},
	}

	ctx := context.Background()
	orgID := "test-org-123"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			query := http.QueryHeader{
				Limit:    10,
				Page:     1,
				Metadata: tt.metadata,
			}

			filter, err := repo.buildHolderFilter(ctx, orgID, query, false)

			require.Error(t, err, "should return error for invalid metadata")
			assert.Nil(t, filter)
			assert.Contains(t, err.Error(), tt.errContains,
				"error should contain expected code")
		})
	}
}

func TestBuildHolderFilter_CombinedFilters(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	repo := &MongoDBRepository{
		FieldEncryptor: fe,
	}

	externalID := "EXT-COMBINED"
	document := "99988877766"

	query := http.QueryHeader{
		Limit:      10,
		Page:       1,
		ExternalID: &externalID,
		Document:   &document,
		Metadata: &bson.M{
			"metadata.region": "us-east",
		},
	}

	ctx := context.Background()
	orgID := "test-org-123"

	filter, err := repo.buildHolderFilter(ctx, orgID, query, false)

	require.NoError(t, err)
	require.NotNil(t, filter)

	// Should have deleted_at, external_id, search.document, and metadata.region
	expectedKeys := map[string]bool{
		"deleted_at":      false,
		"external_id":     false,
		"search.document": false,
		"metadata.region": false,
	}

	for _, elem := range filter {
		if _, exists := expectedKeys[elem.Key]; exists {
			expectedKeys[elem.Key] = true
		}
	}

	for key, found := range expectedKeys {
		assert.True(t, found, "filter should contain key: %s", key)
	}

	// Verify exact count
	assert.Len(t, filter, len(expectedKeys), "filter should have exactly %d keys", len(expectedKeys))
}

func TestBuildHolderFilter_TokenCandidatesGeneration(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	repo := &MongoDBRepository{
		FieldEncryptor: fe,
	}

	// Test that document token candidates are generated consistently
	document := "12345678901"
	expectedToken := testutils.TestLegacySearchToken(document)

	query := http.QueryHeader{
		Limit:    10,
		Page:     1,
		Document: &document,
	}

	ctx := context.Background()
	orgID := "test-org-123"

	filter, err := repo.buildHolderFilter(ctx, orgID, query, false)
	require.NoError(t, err)

	// Find the document filter with $in operator
	var foundTokens []string
	for _, elem := range filter {
		if elem.Key == "search.document" {
			inFilter, ok := elem.Value.(bson.M)
			require.True(t, ok, "search.document value should be bson.M")

			inValue, hasIn := inFilter["$in"]
			require.True(t, hasIn, "search.document should use $in operator")

			foundTokens, ok = inValue.([]string)
			require.True(t, ok, "$in value should be []string")

			break
		}
	}

	require.NotEmpty(t, foundTokens, "token candidates should not be empty")
	assert.Contains(t, foundTokens, expectedToken, "token candidates should contain expected legacy token")

	// Verify token generation is deterministic - same input produces same tokens
	filter2, err := repo.buildHolderFilter(ctx, orgID, query, false)
	require.NoError(t, err)

	var foundTokens2 []string
	for _, elem := range filter2 {
		if elem.Key == "search.document" {
			inFilter := elem.Value.(bson.M)
			foundTokens2 = inFilter["$in"].([]string)

			break
		}
	}

	assert.Equal(t, foundTokens, foundTokens2, "token candidates should be deterministic")
}

func TestBuildHolderFilter_EmptyQuery(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	repo := &MongoDBRepository{
		FieldEncryptor: fe,
	}

	query := http.QueryHeader{
		Limit: 10,
		Page:  1,
	}

	ctx := context.Background()
	orgID := "test-org-123"

	filter, err := repo.buildHolderFilter(ctx, orgID, query, false)

	require.NoError(t, err)
	require.NotNil(t, filter)

	// Should only have deleted_at filter
	assert.Len(t, filter, 1, "empty query should only have deleted_at filter")
	assert.Equal(t, "deleted_at", filter[0].Key)
	assert.Nil(t, filter[0].Value)
}

// mockFieldEncryptorWithError implements FieldEncryptor for error testing scenarios.
type mockFieldEncryptorWithError struct {
	searchTokenCandidatesErr error
}

func (m *mockFieldEncryptorWithError) EncryptField(_ context.Context, _ encryption.FieldContext, plaintext string) (string, error) {
	return plaintext, nil
}

func (m *mockFieldEncryptorWithError) DecryptField(_ context.Context, _ encryption.FieldContext, ciphertext string) (string, error) {
	return ciphertext, nil
}

func (m *mockFieldEncryptorWithError) GenerateSearchToken(_ context.Context, _ encryption.SearchTokenContext, _ string) (string, uint32, error) {
	return "mock-token", 0, nil
}

func (m *mockFieldEncryptorWithError) GenerateSearchTokenCandidates(_ context.Context, _ encryption.SearchTokenContext, _ string) ([]string, error) {
	if m.searchTokenCandidatesErr != nil {
		return nil, m.searchTokenCandidatesErr
	}

	return []string{"mock-token"}, nil
}

func TestBuildHolderFilter_GenerateSearchTokenCandidatesError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("keyset not found for organization")

	mockFE := &mockFieldEncryptorWithError{
		searchTokenCandidatesErr: expectedErr,
	}

	repo := &MongoDBRepository{
		FieldEncryptor: mockFE,
	}

	document := "12345678901"
	query := http.QueryHeader{
		Limit:    10,
		Page:     1,
		Document: &document,
	}

	ctx := context.Background()
	orgID := "test-org-123"

	filter, err := repo.buildHolderFilter(ctx, orgID, query, false)

	require.Error(t, err, "should return error when GenerateSearchTokenCandidates fails")
	assert.Nil(t, filter)
	assert.ErrorIs(t, err, expectedErr, "should propagate the original error")
}

// ============================================================================
// Multi-Token Key Rotation Tests
// ============================================================================

// mockFieldEncryptorMultiToken implements FieldEncryptor that returns configurable
// multiple tokens to simulate key rotation scenarios where multiple HMAC keys are active.
type mockFieldEncryptorMultiToken struct {
	tokens []string
}

func (m *mockFieldEncryptorMultiToken) EncryptField(_ context.Context, _ encryption.FieldContext, plaintext string) (string, error) {
	return "encrypted-" + plaintext, nil
}

func (m *mockFieldEncryptorMultiToken) DecryptField(_ context.Context, _ encryption.FieldContext, ciphertext string) (string, error) {
	return ciphertext, nil
}

func (m *mockFieldEncryptorMultiToken) GenerateSearchToken(_ context.Context, _ encryption.SearchTokenContext, _ string) (string, uint32, error) {
	if len(m.tokens) > 0 {
		return m.tokens[0], 0, nil
	}

	return "mock-token", 0, nil
}

func (m *mockFieldEncryptorMultiToken) GenerateSearchTokenCandidates(_ context.Context, _ encryption.SearchTokenContext, _ string) ([]string, error) {
	return m.tokens, nil
}

func TestBuildHolderFilter_MultiTokenKeyRotation(t *testing.T) {
	t.Parallel()

	// Create mock that returns multiple tokens simulating key rotation scenario
	// where old key, current key, and new key are all enabled
	mockFE := &mockFieldEncryptorMultiToken{
		tokens: []string{"token-old-key", "token-current-key", "token-new-key"},
	}

	repo := &MongoDBRepository{
		FieldEncryptor: mockFE,
	}

	document := "12345678901"
	query := http.QueryHeader{
		Limit:    10,
		Page:     1,
		Document: &document,
	}

	ctx := context.Background()
	orgID := "test-org-123"

	filter, err := repo.buildHolderFilter(ctx, orgID, query, false)
	require.NoError(t, err)
	require.NotNil(t, filter)

	// Find search.document filter
	var inFilter bson.M
	for _, elem := range filter {
		if elem.Key == "search.document" {
			inFilter = elem.Value.(bson.M)

			break
		}
	}

	require.NotNil(t, inFilter, "search.document filter should be present")

	tokens := inFilter["$in"].([]string)
	assert.Len(t, tokens, 3, "should contain all three rotation tokens")
	assert.Contains(t, tokens, "token-old-key", "should contain old key token")
	assert.Contains(t, tokens, "token-current-key", "should contain current key token")
	assert.Contains(t, tokens, "token-new-key", "should contain new key token")
}

func TestBuildHolderFilter_MultiTokenKeyRotation_OrderPreserved(t *testing.T) {
	t.Parallel()

	// Verify that token order is preserved (important for deterministic query plans)
	orderedTokens := []string{"first-token", "second-token", "third-token", "fourth-token"}

	mockFE := &mockFieldEncryptorMultiToken{
		tokens: orderedTokens,
	}

	repo := &MongoDBRepository{
		FieldEncryptor: mockFE,
	}

	document := "99988877766"
	query := http.QueryHeader{
		Limit:    10,
		Page:     1,
		Document: &document,
	}

	ctx := context.Background()
	orgID := "test-org-456"

	filter, err := repo.buildHolderFilter(ctx, orgID, query, false)
	require.NoError(t, err)

	// Extract tokens from filter
	var foundTokens []string
	for _, elem := range filter {
		if elem.Key == "search.document" {
			inFilter := elem.Value.(bson.M)
			foundTokens = inFilter["$in"].([]string)

			break
		}
	}

	require.Len(t, foundTokens, len(orderedTokens))
	assert.Equal(t, orderedTokens, foundTokens, "token order should be preserved for deterministic queries")
}

func TestBuildHolderFilter_MultiTokenKeyRotation_SingleToken(t *testing.T) {
	t.Parallel()

	// Edge case: single token (no rotation in progress)
	mockFE := &mockFieldEncryptorMultiToken{
		tokens: []string{"single-active-key-token"},
	}

	repo := &MongoDBRepository{
		FieldEncryptor: mockFE,
	}

	document := "55544433322"
	query := http.QueryHeader{
		Limit:    10,
		Page:     1,
		Document: &document,
	}

	ctx := context.Background()
	orgID := "test-org-single"

	filter, err := repo.buildHolderFilter(ctx, orgID, query, false)
	require.NoError(t, err)

	// Extract tokens from filter
	var foundTokens []string
	for _, elem := range filter {
		if elem.Key == "search.document" {
			inFilter := elem.Value.(bson.M)
			foundTokens = inFilter["$in"].([]string)

			break
		}
	}

	require.Len(t, foundTokens, 1, "should work with single token (no rotation)")
	assert.Equal(t, "single-active-key-token", foundTokens[0])
}

// allowedRepositoryInputKeys is the exact, closed set of span attribute keys the
// presence-attribute helper is permitted to emit. It is the regression guard that
// prevents the helper from ever leaking a serialized-value key such as
// "app.request.repository_input.document".
var allowedRepositoryInputKeys = map[attribute.Key]struct{}{
	"app.request.repository_input.has_metadata":       {},
	"app.request.repository_input.has_external_id":    {},
	"app.request.repository_input.has_contact":        {},
	"app.request.repository_input.has_addresses":      {},
	"app.request.repository_input.has_natural_person": {},
	"app.request.repository_input.has_legal_person":   {},
}

func TestRepositoryInputAttributes(t *testing.T) {
	externalID := "ext-123"

	tests := []struct {
		name     string
		model    *MongoDBModel
		expected map[attribute.Key]bool
	}{
		{
			name: "fully populated model yields all true",
			model: &MongoDBModel{
				Metadata:      map[string]any{"k": "v"},
				ExternalID:    &externalID,
				Contact:       &ContactMongoDBModel{},
				Addresses:     &AddressesMongoDBModel{},
				NaturalPerson: &NaturalPersonMongoDBModel{},
				LegalPerson:   &LegalPersonMongoDBModel{},
			},
			expected: map[attribute.Key]bool{
				"app.request.repository_input.has_metadata":       true,
				"app.request.repository_input.has_external_id":    true,
				"app.request.repository_input.has_contact":        true,
				"app.request.repository_input.has_addresses":      true,
				"app.request.repository_input.has_natural_person": true,
				"app.request.repository_input.has_legal_person":   true,
			},
		},
		{
			name:  "zero-value model yields all false",
			model: &MongoDBModel{},
			expected: map[attribute.Key]bool{
				"app.request.repository_input.has_metadata":       false,
				"app.request.repository_input.has_external_id":    false,
				"app.request.repository_input.has_contact":        false,
				"app.request.repository_input.has_addresses":      false,
				"app.request.repository_input.has_natural_person": false,
				"app.request.repository_input.has_legal_person":   false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := repositoryInputAttributes(tt.model)

			// Regression guard: the returned key set is exactly the six allowed
			// presence keys — nothing more, nothing less. A serialized-value leak
			// would introduce an unexpected key and fail here.
			seen := make(map[attribute.Key]struct{}, len(attrs))

			for _, a := range attrs {
				_, allowed := allowedRepositoryInputKeys[a.Key]
				assert.Truef(t, allowed, "unexpected span attribute key emitted: %q", a.Key)

				_, dup := seen[a.Key]
				assert.Falsef(t, dup, "duplicate span attribute key emitted: %q", a.Key)

				seen[a.Key] = struct{}{}

				want, ok := tt.expected[a.Key]
				assert.Truef(t, ok, "key not in expected map: %q", a.Key)
				assert.Equalf(t, want, a.Value.AsBool(), "value mismatch for key %q", a.Key)
			}

			assert.Len(t, attrs, len(allowedRepositoryInputKeys), "helper must emit exactly the allowed key count")
			assert.Len(t, seen, len(allowedRepositoryInputKeys), "helper must emit each allowed key exactly once")
		})
	}
}
