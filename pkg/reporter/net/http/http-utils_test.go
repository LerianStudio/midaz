// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateParameters_Defaults(t *testing.T) {
	t.Parallel()

	params := map[string]string{}

	result, err := ValidateParameters(params)
	require.NoError(t, err)

	// Check default values
	assert.Equal(t, 10, result.Limit)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, "desc", result.SortOrder)
	assert.Equal(t, "", result.OutputFormat)
	assert.Equal(t, "", result.Description)
	assert.Equal(t, "", result.Status)
	assert.Equal(t, "", result.Cursor)
	assert.False(t, result.UseMetadata)
	assert.Nil(t, result.Metadata)
}

func TestValidateParameters_AllParameters(t *testing.T) {
	t.Parallel()

	templateID := uuid.New()

	params := map[string]string{
		"outputFormat": "PDF",
		"description":  "Test description",
		"status":       "Finished",
		"templateId":   templateID.String(),
		"limit":        "20",
		"page":         "2",
		"sortOrder":    "asc",
		"createdAt":    "2024-01-15",
	}

	result, err := ValidateParameters(params)
	require.NoError(t, err)

	assert.Equal(t, "PDF", result.OutputFormat)
	assert.Equal(t, "Test description", result.Description)
	assert.Equal(t, "Finished", result.Status)
	assert.Equal(t, templateID, result.TemplateID)
	assert.Equal(t, 20, result.Limit)
	assert.Equal(t, 2, result.Page)
	assert.Equal(t, "asc", result.SortOrder)
	assert.Equal(t, 2024, result.CreatedAt.Year())
	assert.Equal(t, 1, int(result.CreatedAt.Month()))
	assert.Equal(t, 15, result.CreatedAt.Day())
}

func TestValidateParameters_Metadata(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"metadata.customField": "customValue",
	}

	result, err := ValidateParameters(params)
	require.NoError(t, err)

	assert.True(t, result.UseMetadata)
	assert.NotNil(t, result.Metadata)
}

func TestValidateParameters_InvalidOutputFormat(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"outputFormat": "INVALID_FORMAT",
	}

	_, err := ValidateParameters(params)
	require.Error(t, err)
}

func TestValidateParameters_ValidOutputFormats(t *testing.T) {
	t.Parallel()

	formats := []string{"PDF", "pdf", "HTML", "html", "CSV", "csv", "XML", "xml", "TXT", "txt"}

	for _, format := range formats {
		t.Run("Success - Format_"+format, func(t *testing.T) {
			t.Parallel()

			params := map[string]string{
				"outputFormat": format,
			}

			result, err := ValidateParameters(params)
			require.NoError(t, err)
			assert.Equal(t, format, result.OutputFormat)
		})
	}
}

func TestValidateParameters_InvalidSortOrder(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"sortOrder": "invalid",
	}

	_, err := ValidateParameters(params)
	require.Error(t, err)
}

func TestValidateParameters_ValidSortOrders(t *testing.T) {
	t.Parallel()

	sortOrders := []string{"asc", "ASC", "desc", "DESC", "Asc", "Desc"}

	for _, order := range sortOrders {
		t.Run("Success - SortOrder_"+order, func(t *testing.T) {
			t.Parallel()

			params := map[string]string{
				"sortOrder": order,
			}

			result, err := ValidateParameters(params)
			require.NoError(t, err)
			// Result is lowercased
			assert.Contains(t, []string{"asc", "desc"}, result.SortOrder)
		})
	}
}

func TestValidateParameters_PaginationLimitExceeded(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because t.Setenv is incompatible with parallel execution.
	t.Setenv("MAX_PAGINATION_LIMIT", "100")

	params := map[string]string{
		"limit": "150",
	}

	_, err := ValidateParameters(params)
	require.Error(t, err)
}

func TestValidateParameters_ValidCursor(t *testing.T) {
	t.Parallel()

	cursor := Cursor{
		ID:         "123",
		PointsNext: true,
	}
	cursorJSON, _ := json.Marshal(cursor)
	encodedCursor := base64.StdEncoding.EncodeToString(cursorJSON)

	params := map[string]string{
		"cursor": encodedCursor,
	}

	result, err := ValidateParameters(params)
	require.NoError(t, err)
	assert.Equal(t, encodedCursor, result.Cursor)
}

func TestValidateParameters_InvalidCursor(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"cursor": "invalid-cursor-not-base64",
	}

	_, err := ValidateParameters(params)
	require.Error(t, err)
}

func TestValidateParameters_InvalidTemplateID(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"templateId": "not-a-uuid",
	}

	result, err := ValidateParameters(params)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "0257")
}

func TestValidateParameters_InvalidLimit(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"limit": "not-a-number",
	}

	result, err := ValidateParameters(params)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "0082")
}

func TestValidateParameters_InvalidPage(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"page": "not-a-number",
	}

	result, err := ValidateParameters(params)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "0082")
}

func TestQueryHeader_ToOffsetPagination(t *testing.T) {
	t.Parallel()

	qh := &QueryHeader{
		Limit:     20,
		Page:      3,
		SortOrder: "asc",
		Alias:     "test_alias",
		Cursor:    "some_cursor",
	}

	pagination := qh.ToOffsetPagination()

	assert.Equal(t, 20, pagination.Limit)
	assert.Equal(t, 3, pagination.Page)
	assert.Equal(t, "asc", pagination.SortOrder)
	assert.Equal(t, "test_alias", pagination.Alias)
	// Cursor is not included in ToOffsetPagination
	assert.Empty(t, pagination.Cursor)
}

func TestPagination_Struct(t *testing.T) {
	t.Parallel()

	pagination := Pagination{
		Limit:     10,
		Page:      1,
		Cursor:    "cursor",
		SortOrder: "desc",
		Alias:     "alias",
	}

	assert.Equal(t, 10, pagination.Limit)
	assert.Equal(t, 1, pagination.Page)
	assert.Equal(t, "cursor", pagination.Cursor)
	assert.Equal(t, "desc", pagination.SortOrder)
	assert.Equal(t, "alias", pagination.Alias)
}

func TestHeaderConstants(t *testing.T) {
	t.Parallel()

	// Test that constants are defined
	assert.Equal(t, "User-Agent", headerUserAgent)
	assert.Equal(t, ".tpl", fileExtension)
	assert.Equal(t, "X-TTL", idempotencyTTL)
}

// TestQueryParam_Helper verifies that queryParam checks snake_case first,
// then falls back to camelCase, and returns false when neither key exists.
// TestNormalizeParams verifies that normalizeParams converts camelCase query
// parameter keys to snake_case while preserving already-correct keys and
// giving snake_case precedence when both formats are present.
func TestNormalizeParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  map[string]string
		expect map[string]string
	}{
		{
			name:   "camelCase keys are converted to snake_case",
			input:  map[string]string{"outputFormat": "PDF", "sortOrder": "asc", "templateId": "abc", "createdAt": "2024-01-01"},
			expect: map[string]string{"output_format": "PDF", "sort_order": "asc", "template_id": "abc", "created_at": "2024-01-01"},
		},
		{
			name:   "snake_case keys are preserved unchanged",
			input:  map[string]string{"output_format": "PDF", "sort_order": "asc"},
			expect: map[string]string{"output_format": "PDF", "sort_order": "asc"},
		},
		{
			name:   "snake_case wins when both formats present",
			input:  map[string]string{"output_format": "CSV", "outputFormat": "PDF"},
			expect: map[string]string{"output_format": "CSV"},
		},
		{
			name:   "non-aliased keys are preserved",
			input:  map[string]string{"limit": "10", "page": "1", "status": "Finished", "description": "test"},
			expect: map[string]string{"limit": "10", "page": "1", "status": "Finished", "description": "test"},
		},
		{
			name:   "metadata keys are preserved",
			input:  map[string]string{"metadata.field": "val"},
			expect: map[string]string{"metadata.field": "val"},
		},
		{
			name:   "empty map returns empty map",
			input:  map[string]string{},
			expect: map[string]string{},
		},
		{
			name:   "mixed aliased and non-aliased keys",
			input:  map[string]string{"outputFormat": "HTML", "limit": "5", "sortOrder": "desc"},
			expect: map[string]string{"output_format": "HTML", "limit": "5", "sort_order": "desc"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := normalizeParams(tt.input)
			assert.Equal(t, tt.expect, result)
		})
	}
}

// TestValidateParameters_SnakeCaseParams verifies that ValidateParameters
// accepts the new snake_case query parameter format.
func TestValidateParameters_SnakeCaseParams(t *testing.T) {
	t.Parallel()

	templateID := uuid.New()

	params := map[string]string{
		"output_format": "PDF",
		"description":   "Test description",
		"status":        "Finished",
		"template_id":   templateID.String(),
		"limit":         "20",
		"page":          "2",
		"sort_order":    "asc",
		"created_at":    "2024-01-15",
	}

	result, err := ValidateParameters(params)
	require.NoError(t, err)

	assert.Equal(t, "PDF", result.OutputFormat)
	assert.Equal(t, "Test description", result.Description)
	assert.Equal(t, "Finished", result.Status)
	assert.Equal(t, templateID, result.TemplateID)
	assert.Equal(t, 20, result.Limit)
	assert.Equal(t, 2, result.Page)
	assert.Equal(t, "asc", result.SortOrder)
	assert.Equal(t, 2024, result.CreatedAt.Year())
	assert.Equal(t, 1, int(result.CreatedAt.Month()))
	assert.Equal(t, 15, result.CreatedAt.Day())
}

// TestValidateParameters_CamelCaseBackwardsCompat verifies that camelCase
// query parameters still work (backwards compatibility).
func TestValidateParameters_CamelCaseBackwardsCompat(t *testing.T) {
	t.Parallel()

	templateID := uuid.New()

	params := map[string]string{
		"outputFormat": "HTML",
		"templateId":   templateID.String(),
		"sortOrder":    "desc",
		"createdAt":    "2025-06-30",
	}

	result, err := ValidateParameters(params)
	require.NoError(t, err)

	assert.Equal(t, "HTML", result.OutputFormat)
	assert.Equal(t, templateID, result.TemplateID)
	assert.Equal(t, "desc", result.SortOrder)
	assert.Equal(t, 2025, result.CreatedAt.Year())
	assert.Equal(t, 6, int(result.CreatedAt.Month()))
	assert.Equal(t, 30, result.CreatedAt.Day())
}

// TestValidateParameters_SnakeCasePrecedence verifies that when both snake_case
// and camelCase keys are present, the snake_case value takes precedence.
func TestValidateParameters_SnakeCasePrecedence(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"output_format": "CSV",
		"outputFormat":  "PDF",
		"sort_order":    "asc",
		"sortOrder":     "desc",
	}

	result, err := ValidateParameters(params)
	require.NoError(t, err)

	assert.Equal(t, "CSV", result.OutputFormat, "snake_case output_format should take precedence")
	assert.Equal(t, "asc", result.SortOrder, "snake_case sort_order should take precedence")
}

// TestValidateParameters_InvalidSnakeCaseOutputFormat verifies that validation
// errors work the same way with snake_case parameter names.
func TestValidateParameters_InvalidSnakeCaseOutputFormat(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"output_format": "INVALID_FORMAT",
	}

	_, err := ValidateParameters(params)
	require.Error(t, err)
}

// TestValidateParameters_InvalidSnakeCaseSortOrder verifies that validation
// errors work the same way with snake_case sort_order parameter.
func TestValidateParameters_InvalidSnakeCaseSortOrder(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"sort_order": "invalid",
	}

	_, err := ValidateParameters(params)
	require.Error(t, err)
}

// TestValidateParameters_InvalidSnakeCaseTemplateID verifies that an invalid
// template_id (snake_case) returns a validation error.
func TestValidateParameters_InvalidSnakeCaseTemplateID(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"template_id": "not-a-uuid",
	}

	result, err := ValidateParameters(params)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "0257")
}

// ---------------------------------------------------------------------------
// GetFileFromHeader tests
// ---------------------------------------------------------------------------

func TestGetFileFromHeader_InvalidExtension(t *testing.T) {
	t.Parallel()

	header := createTestFileHeader(t, "template.txt", []byte("content"))

	_, err := GetFileFromHeader(header)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0249")
}

func TestGetFileFromHeader_EmptyFile(t *testing.T) {
	t.Parallel()

	header := createTestFileHeader(t, "template.tpl", []byte{})

	_, err := GetFileFromHeader(header)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0253")
}

func TestGetFileFromHeader_ValidFile(t *testing.T) {
	t.Parallel()

	content := []byte("<html>{{name}}</html>")
	header := createTestFileHeader(t, "template.tpl", content)

	result, err := GetFileFromHeader(header)
	require.NoError(t, err)
	assert.Equal(t, string(content), result)
}

// ---------------------------------------------------------------------------
// ReadMultipartFile tests
// ---------------------------------------------------------------------------

func TestReadMultipartFile_Success(t *testing.T) {
	t.Parallel()

	content := []byte("file content for reading")
	header := createTestFileHeader(t, "test.tpl", content)

	result, err := ReadMultipartFile(header)
	require.NoError(t, err)
	assert.Equal(t, content, result)
}

func TestReadMultipartFile_EmptyFile(t *testing.T) {
	t.Parallel()

	header := createTestFileHeader(t, "empty.tpl", []byte{})

	result, err := ReadMultipartFile(header)
	require.NoError(t, err)
	assert.Empty(t, result)
}

// createTestFileHeader creates a multipart.FileHeader for testing purposes.
func createTestFileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)

	_, err = part.Write(content)
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	reader := multipart.NewReader(body, writer.Boundary())

	form, err := reader.ReadForm(int64(body.Len()) + 1024)
	require.NoError(t, err)

	files := form.File["file"]
	require.NotEmpty(t, files)

	return files[0]
}

// ---------------------------------------------------------------------------
// validatePagination edge case tests
// ---------------------------------------------------------------------------

func TestValidatePagination_InvalidCursorDecode(t *testing.T) {
	t.Parallel()

	// Non-base64 cursor string should fail decoding
	err := validatePagination("not-valid-base64!@#$", "desc", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0082")
}

func TestValidatePagination_ValidBase64ButInvalidJSON(t *testing.T) {
	t.Parallel()

	// Valid base64 but invalid JSON inside
	invalidJSON := base64.StdEncoding.EncodeToString([]byte("not json"))
	err := validatePagination(invalidJSON, "desc", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0082")
}

// TestValidateParameters_QueryParamParseErrors verifies that ValidateParameters
// returns a validation error for non-numeric limit/page values and out-of-bounds
// values instead of silently defaulting.
// REFACTOR-005A: These tests must FAIL against the current code.
func TestValidateParameters_QueryParamParseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		params      map[string]string
		wantErr     bool
		errContains string
	}{
		{
			name:        "non-numeric limit returns error",
			params:      map[string]string{"limit": "abc"},
			wantErr:     true,
			errContains: "0082",
		},
		{
			name:        "non-numeric page returns error",
			params:      map[string]string{"page": "xyz"},
			wantErr:     true,
			errContains: "0082",
		},
		{
			name:        "negative limit returns error",
			params:      map[string]string{"limit": "-1"},
			wantErr:     true,
			errContains: "0082",
		},
		{
			name:        "zero page returns error",
			params:      map[string]string{"page": "0"},
			wantErr:     true,
			errContains: "0082",
		},
		{
			name:        "zero limit returns error",
			params:      map[string]string{"limit": "0"},
			wantErr:     true,
			errContains: "0082",
		},
		{
			name:        "negative page returns error",
			params:      map[string]string{"page": "-5"},
			wantErr:     true,
			errContains: "0082",
		},
		{
			name:        "float limit returns error",
			params:      map[string]string{"limit": "10.5"},
			wantErr:     true,
			errContains: "0082",
		},
		{
			name:        "float page returns error",
			params:      map[string]string{"page": "1.5"},
			wantErr:     true,
			errContains: "0082",
		},
		{
			name:    "valid limit and page succeeds",
			params:  map[string]string{"limit": "10", "page": "1"},
			wantErr: false,
		},
		{
			name:    "valid large page succeeds",
			params:  map[string]string{"limit": "25", "page": "100"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := ValidateParameters(tt.params)

			if tt.wantErr {
				require.Error(t, err, "expected error for params %v but got nil", tt.params)
				assert.Contains(t, err.Error(), tt.errContains,
					"error should contain code %q, got: %s", tt.errContains, err.Error())
				assert.Nil(t, result, "result should be nil when error is returned")
			} else {
				require.NoError(t, err, "unexpected error for params %v: %v", tt.params, err)
				assert.NotNil(t, result, "result should not be nil for valid params")
				assert.Greater(t, result.Limit, 0, "limit must be positive")
				assert.GreaterOrEqual(t, result.Page, 1, "page must be >= 1")
			}
		})
	}
}
