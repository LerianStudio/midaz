// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v3/tests/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// ============================================================================
// buildHolderFilter Tests
// ============================================================================

func TestBuildHolderFilter_ExcludeDeleted(t *testing.T) {
	t.Parallel()

	crypto := testutils.SetupCrypto(t)
	repo := &MongoDBRepository{
		DataSecurity: crypto,
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filter, err := repo.buildHolderFilter(tt.query, tt.includeDeleted)

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

	crypto := testutils.SetupCrypto(t)
	repo := &MongoDBRepository{
		DataSecurity: crypto,
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filter, err := repo.buildHolderFilter(tt.query, false)

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

	crypto := testutils.SetupCrypto(t)
	repo := &MongoDBRepository{
		DataSecurity: crypto,
	}

	document := "12345678901"

	tests := []struct {
		name         string
		query        http.QueryHeader
		wantDocument bool
	}{
		{
			name: "includes_document_hash_when_provided",
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filter, err := repo.buildHolderFilter(tt.query, false)

			require.NoError(t, err)
			require.NotNil(t, filter)

			hasDocument := false
			for _, elem := range filter {
				if elem.Key == "search.document" {
					hasDocument = true
					// Value should be a hash, not the original document
					assert.NotEqual(t, document, elem.Value,
						"document should be hashed, not plaintext")
				}
			}

			assert.Equal(t, tt.wantDocument, hasDocument,
				"search.document filter presence should match expectation")
		})
	}
}

func TestBuildHolderFilter_WithMetadata(t *testing.T) {
	t.Parallel()

	crypto := testutils.SetupCrypto(t)
	repo := &MongoDBRepository{
		DataSecurity: crypto,
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			query := http.QueryHeader{
				Limit:    10,
				Page:     1,
				Metadata: tt.metadata,
			}

			filter, err := repo.buildHolderFilter(query, false)

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

	crypto := testutils.SetupCrypto(t)
	repo := &MongoDBRepository{
		DataSecurity: crypto,
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			query := http.QueryHeader{
				Limit:    10,
				Page:     1,
				Metadata: tt.metadata,
			}

			filter, err := repo.buildHolderFilter(query, false)

			require.Error(t, err, "should return error for invalid metadata")
			assert.Nil(t, filter)
			assert.Contains(t, err.Error(), tt.errContains,
				"error should contain expected code")
		})
	}
}

func TestBuildHolderFilter_CombinedFilters(t *testing.T) {
	t.Parallel()

	crypto := testutils.SetupCrypto(t)
	repo := &MongoDBRepository{
		DataSecurity: crypto,
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

	filter, err := repo.buildHolderFilter(query, false)

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

func TestBuildHolderFilter_HashGeneration(t *testing.T) {
	t.Parallel()

	crypto := testutils.SetupCrypto(t)
	repo := &MongoDBRepository{
		DataSecurity: crypto,
	}

	// Test that document hash is generated consistently
	document := "12345678901"
	expectedHash := crypto.GenerateHash(&document)

	query := http.QueryHeader{
		Limit:    10,
		Page:     1,
		Document: &document,
	}

	filter, err := repo.buildHolderFilter(query, false)
	require.NoError(t, err)

	// Find the document hash in the filter
	var foundHash string
	for _, elem := range filter {
		if elem.Key == "search.document" {
			foundHash = elem.Value.(string)
			break
		}
	}

	assert.Equal(t, expectedHash, foundHash, "document hash should match expected")

	// Verify hash is deterministic - same input produces same hash
	filter2, err := repo.buildHolderFilter(query, false)
	require.NoError(t, err)

	var foundHash2 string
	for _, elem := range filter2 {
		if elem.Key == "search.document" {
			foundHash2 = elem.Value.(string)
			break
		}
	}

	assert.Equal(t, foundHash, foundHash2, "hash should be deterministic")
}

func TestBuildHolderFilter_EmptyQuery(t *testing.T) {
	t.Parallel()

	crypto := testutils.SetupCrypto(t)
	repo := &MongoDBRepository{
		DataSecurity: crypto,
	}

	query := http.QueryHeader{
		Limit: 10,
		Page:  1,
	}

	filter, err := repo.buildHolderFilter(query, false)

	require.NoError(t, err)
	require.NotNil(t, filter)

	// Should only have deleted_at filter
	assert.Len(t, filter, 1, "empty query should only have deleted_at filter")
	assert.Equal(t, "deleted_at", filter[0].Key)
	assert.Nil(t, filter[0].Value)
}
