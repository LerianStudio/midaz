package holderlink

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v3/tests/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// ============================================================================
// buildHolderLinkFilter Tests
// ============================================================================

func TestBuildHolderLinkFilter_ExcludeDeleted(t *testing.T) {
	t.Parallel()

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

			filter, err := buildHolderLinkFilter(tt.query, tt.includeDeleted)

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

func TestBuildHolderLinkFilter_WithHolderID(t *testing.T) {
	t.Parallel()

	holderID := uuid.New()
	holderIDStr := holderID.String()

	tests := []struct {
		name         string
		query        http.QueryHeader
		wantHolderID bool
		wantValue    uuid.UUID
	}{
		{
			name: "includes_holder_id_when_provided",
			query: http.QueryHeader{
				Limit:    10,
				Page:     1,
				HolderID: &holderIDStr,
			},
			wantHolderID: true,
			wantValue:    holderID,
		},
		{
			name: "excludes_holder_id_when_nil",
			query: http.QueryHeader{
				Limit:    10,
				Page:     1,
				HolderID: nil,
			},
			wantHolderID: false,
		},
		{
			name: "excludes_holder_id_when_empty_string",
			query: http.QueryHeader{
				Limit:    10,
				Page:     1,
				HolderID: testutils.Ptr(""),
			},
			wantHolderID: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filter, err := buildHolderLinkFilter(tt.query, false)

			require.NoError(t, err)
			require.NotNil(t, filter)

			hasHolderID := false
			for _, elem := range filter {
				if elem.Key == "holder_id" {
					hasHolderID = true
					assert.Equal(t, tt.wantValue, elem.Value,
						"holder_id value should match parsed UUID")
				}
			}

			assert.Equal(t, tt.wantHolderID, hasHolderID,
				"holder_id filter presence should match expectation")
		})
	}
}

func TestBuildHolderLinkFilter_WithInvalidHolderID(t *testing.T) {
	t.Parallel()

	invalidUUID := "not-a-valid-uuid"
	query := http.QueryHeader{
		Limit:    10,
		Page:     1,
		HolderID: &invalidUUID,
	}

	filter, err := buildHolderLinkFilter(query, false)

	require.Error(t, err, "should return error for invalid holder_id UUID")
	assert.Nil(t, filter)
	assert.Contains(t, err.Error(), "holder_id",
		"error message should mention holder_id parameter")
}

func TestBuildHolderLinkFilter_WithMetadata(t *testing.T) {
	t.Parallel()

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

			filter, err := buildHolderLinkFilter(query, false)

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

func TestBuildHolderLinkFilter_CombinedFilters(t *testing.T) {
	t.Parallel()

	holderID := uuid.New()
	holderIDStr := holderID.String()

	query := http.QueryHeader{
		Limit:    10,
		Page:     1,
		HolderID: &holderIDStr,
		Metadata: &bson.M{
			"metadata.region": "us-east",
		},
	}

	filter, err := buildHolderLinkFilter(query, false)

	require.NoError(t, err)
	require.NotNil(t, filter)

	// Should have deleted_at, holder_id, and metadata.region
	expectedKeys := map[string]bool{
		"deleted_at":      false,
		"holder_id":       false,
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
}

// ============================================================================
// getDuplicateKeyErrorType Tests
// ============================================================================

func TestGetDuplicateKeyErrorType_DuplicateHolderLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		errMessage string
		wantType   string
		wantFound  bool
	}{
		{
			name:       "detects_alias_link_type_unique_index",
			errMessage: `E11000 duplicate key error collection: test.holder_links index: alias_id_link_type_unique dup key: { alias_id: UUID("..."), link_type: "LEGAL_REPRESENTATIVE" }`,
			wantType:   "duplicate_holder_link",
			wantFound:  true,
		},
		{
			name:       "detects_from_key_pattern_alias_and_link_type",
			errMessage: `E11000 duplicate key error collection: test.holder_links dup key: { alias_id: UUID("..."), link_type: "..." }`,
			wantType:   "duplicate_holder_link",
			wantFound:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := createWriteException(11000, tt.errMessage)

			errorType, found := getDuplicateKeyErrorType(err)

			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantType, errorType)
		})
	}
}

func TestGetDuplicateKeyErrorType_PrimaryHolderExists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		errMessage string
		wantType   string
		wantFound  bool
	}{
		{
			name:       "detects_primary_holder_unique_index",
			errMessage: `E11000 duplicate key error collection: test.holder_links index: alias_id_primary_holder_unique dup key: { alias_id: UUID("...") }`,
			wantType:   "primary_holder_exists",
			wantFound:  true,
		},
		{
			name:       "detects_primary_holder_from_message_content",
			errMessage: `E11000 duplicate key error collection: test.holder_links dup key: { alias_id: UUID("...") } PRIMARY_HOLDER`,
			wantType:   "primary_holder_exists",
			wantFound:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := createWriteException(11000, tt.errMessage)

			errorType, found := getDuplicateKeyErrorType(err)

			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantType, errorType)
		})
	}
}

func TestGetDuplicateKeyErrorType_NonWriteException(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		wantType  string
		wantFound bool
	}{
		{
			name:      "returns_empty_for_nil_error",
			err:       nil,
			wantType:  "",
			wantFound: false,
		},
		{
			name:      "returns_empty_for_generic_error",
			err:       assert.AnError,
			wantType:  "",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			errorType, found := getDuplicateKeyErrorType(tt.err)

			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantType, errorType)
		})
	}
}

func TestGetDuplicateKeyErrorType_NonDuplicateCode(t *testing.T) {
	t.Parallel()

	// Error code that is not 11000 or 11001
	err := createWriteException(12345, "some other error")

	errorType, found := getDuplicateKeyErrorType(err)

	assert.False(t, found, "should not detect duplicate key for non-11000/11001 codes")
	assert.Empty(t, errorType)
}

func TestGetDuplicateKeyErrorType_UnknownDuplicateKey(t *testing.T) {
	t.Parallel()

	// Duplicate key error but doesn't match known patterns
	err := createWriteException(11000, `E11000 duplicate key error collection: test.holder_links index: some_other_index dup key: { some_field: "value" }`)

	errorType, found := getDuplicateKeyErrorType(err)

	assert.False(t, found, "should not match unknown index patterns")
	assert.Empty(t, errorType)
}

// ============================================================================
// checkErrorByIndexName Tests
// ============================================================================

func TestCheckErrorByIndexName_AliasLinkTypeUnique(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		errMsg    string
		wantType  string
		wantFound bool
	}{
		{
			name:      "detects_alias_id_link_type_unique_index",
			errMsg:    `E11000 duplicate key error index: alias_id_link_type_unique`,
			wantType:  "duplicate_holder_link",
			wantFound: true,
		},
		{
			name:      "detects_in_longer_message",
			errMsg:    `E11000 duplicate key error collection: test_db.holder_links_org-123 index: alias_id_link_type_unique dup key: { alias_id: UUID("abc"), link_type: "LEGAL_REPRESENTATIVE" }`,
			wantType:  "duplicate_holder_link",
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, found := checkErrorByIndexName(tt.errMsg)

			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantType, result)
		})
	}
}

func TestCheckErrorByIndexName_PrimaryHolderUnique(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		errMsg    string
		wantType  string
		wantFound bool
	}{
		{
			name:      "detects_alias_id_primary_holder_unique_index",
			errMsg:    `E11000 duplicate key error index: alias_id_primary_holder_unique`,
			wantType:  "primary_holder_exists",
			wantFound: true,
		},
		{
			name:      "detects_in_longer_message",
			errMsg:    `E11000 duplicate key error collection: test_db.holder_links_org-456 index: alias_id_primary_holder_unique dup key: { alias_id: UUID("xyz") }`,
			wantType:  "primary_holder_exists",
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, found := checkErrorByIndexName(tt.errMsg)

			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantType, result)
		})
	}
}

func TestCheckErrorByIndexName_UnknownIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		errMsg string
	}{
		{
			name:   "unknown_index_name",
			errMsg: `E11000 duplicate key error index: some_other_index`,
		},
		{
			name:   "no_index_name",
			errMsg: `E11000 duplicate key error collection: test.holder_links`,
		},
		{
			name:   "partial_match_should_not_work",
			errMsg: `E11000 error index: alias_id_link_type`, // missing "_unique"
		},
		{
			name:   "empty_message",
			errMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, found := checkErrorByIndexName(tt.errMsg)

			assert.False(t, found, "should not match unknown patterns")
			assert.Empty(t, result)
		})
	}
}

// ============================================================================
// checkErrorByKeyPatternFromMessage Tests
// ============================================================================

func TestCheckErrorByKeyPatternFromMessage_DuplicateHolderLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		errMsg    string
		wantType  string
		wantFound bool
	}{
		{
			name:      "detects_alias_id_and_link_type_in_dup_key",
			errMsg:    `E11000 duplicate key error dup key: { alias_id: UUID("abc"), link_type: "LEGAL_REP" }`,
			wantType:  "duplicate_holder_link",
			wantFound: true,
		},
		{
			name:      "detects_with_different_order",
			errMsg:    `E11000 duplicate key error dup key: { link_type: "RESPONSIBLE", alias_id: UUID("xyz") }`,
			wantType:  "duplicate_holder_link",
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, found := checkErrorByKeyPatternFromMessage(tt.errMsg)

			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantType, result)
		})
	}
}

func TestCheckErrorByKeyPatternFromMessage_PrimaryHolder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		errMsg    string
		wantType  string
		wantFound bool
	}{
		{
			name:      "detects_primary_holder_in_message",
			errMsg:    `E11000 duplicate key error dup key: { alias_id: UUID("abc") } PRIMARY_HOLDER`,
			wantType:  "primary_holder_exists",
			wantFound: true,
		},
		{
			name:      "detects_primary_holder_link_type_constant",
			errMsg:    `E11000 duplicate key error for PRIMARY_HOLDER dup key: { alias_id: UUID("xyz") }`,
			wantType:  "primary_holder_exists",
			wantFound: true,
		},
		{
			name:      "detects_alias_id_primary_holder_unique_in_dup_key_section",
			errMsg:    `E11000 duplicate key error dup key: { alias_id: UUID("abc") } alias_id_primary_holder_unique`,
			wantType:  "primary_holder_exists",
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, found := checkErrorByKeyPatternFromMessage(tt.errMsg)

			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantType, result)
		})
	}
}

func TestCheckErrorByKeyPatternFromMessage_NoDupKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		errMsg string
	}{
		{
			name:   "no_dup_key_marker",
			errMsg: `E11000 duplicate key error collection: test.holder_links`,
		},
		{
			name:   "empty_message",
			errMsg: "",
		},
		{
			name:   "only_alias_id_no_dup_key",
			errMsg: `error with alias_id and link_type but no dup key marker`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, found := checkErrorByKeyPatternFromMessage(tt.errMsg)

			assert.False(t, found, "should not match when 'dup key:' is missing")
			assert.Empty(t, result)
		})
	}
}

func TestCheckErrorByKeyPatternFromMessage_UnknownPattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		errMsg string
	}{
		{
			name:   "only_alias_id_no_link_type",
			errMsg: `E11000 duplicate key error dup key: { alias_id: UUID("abc") }`,
		},
		{
			name:   "only_link_type_no_alias_id",
			errMsg: `E11000 duplicate key error dup key: { link_type: "LEGAL_REP" }`,
		},
		{
			name:   "different_fields",
			errMsg: `E11000 duplicate key error dup key: { some_field: "value" }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, found := checkErrorByKeyPatternFromMessage(tt.errMsg)

			assert.False(t, found, "should not match unknown patterns")
			assert.Empty(t, result)
		})
	}
}

// ============================================================================
// Test Helpers
// ============================================================================

// createWriteException creates a mongo.WriteException with a single WriteError for testing.
func createWriteException(code int, message string) mongo.WriteException {
	return mongo.WriteException{
		WriteErrors: []mongo.WriteError{
			{
				Code:    code,
				Message: message,
			},
		},
	}
}
