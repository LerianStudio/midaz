// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/constant"
)

func TestEntityNotFoundError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      EntityNotFoundError
		expected string
	}{
		{
			name: "With message",
			err: EntityNotFoundError{
				Message: "custom message",
			},
			expected: "custom message",
		},
		{
			name: "With entity type, no message",
			err: EntityNotFoundError{
				EntityType: "Template",
			},
			expected: "Entity Template not found",
		},
		{
			name: "With wrapped error, no message",
			err: EntityNotFoundError{
				Err: errors.New("underlying error"),
			},
			expected: "underlying error",
		},
		{
			name:     "Empty - default message",
			err:      EntityNotFoundError{},
			expected: "entity not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestEntityNotFoundError_Unwrap(t *testing.T) {
	t.Parallel()

	wrappedErr := errors.New("wrapped error")
	err := EntityNotFoundError{
		Err: wrappedErr,
	}

	assert.Equal(t, wrappedErr, err.Unwrap())
}

func TestValidationError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      ValidationError
		expected string
	}{
		{
			name: "With code and message",
			err: ValidationError{
				Code:    "ERR001",
				Message: "validation failed",
			},
			expected: "ERR001 - validation failed",
		},
		{
			name: "Message only",
			err: ValidationError{
				Message: "validation failed",
			},
			expected: "validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestValidationError_Unwrap(t *testing.T) {
	t.Parallel()

	wrappedErr := errors.New("wrapped error")
	err := ValidationError{
		Err: wrappedErr,
	}

	assert.Equal(t, wrappedErr, err.Unwrap())
}

func Test_validationErrorJSONSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		validationError ValidationError
		expectedJSON    map[string]any
	}{
		{
			name: "Full ValidationError with all fields",
			validationError: ValidationError{
				EntityType: "template",
				Title:      "Validation Failed",
				Message:    "The template is invalid",
				Code:       "VAL001",
			},
			expectedJSON: map[string]any{
				"entityType": "template",
				"title":      "Validation Failed",
				"message":    "The template is invalid",
				"code":       "VAL001",
			},
		},
		{
			name: "ValidationError with only Code and Message",
			validationError: ValidationError{
				Code:    "ERR123",
				Message: "Invalid input",
			},
			expectedJSON: map[string]any{
				"code":    "ERR123",
				"message": "Invalid input",
			},
		},
		{
			name: "ValidationError with empty fields (omitempty behavior)",
			validationError: ValidationError{
				Code: "CODE001",
			},
			expectedJSON: map[string]any{
				"code": "CODE001",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Marshal to JSON
			jsonBytes, err := json.Marshal(tt.validationError)
			require.NoError(t, err)

			// Unmarshal to map for flexible comparison
			var result map[string]any
			err = json.Unmarshal(jsonBytes, &result)
			require.NoError(t, err)

			// Verify each expected field
			for key, expectedValue := range tt.expectedJSON {
				assert.Equal(t, expectedValue, result[key], "Field %s should match", key)
			}

			// Verify no unexpected fields beyond what we expect
			for key := range result {
				_, exists := tt.expectedJSON[key]
				assert.True(t, exists, "Unexpected field %s in JSON output", key)
			}
		})
	}
}

func TestEntityConflictError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      EntityConflictError
		expected string
	}{
		{
			name: "With message",
			err: EntityConflictError{
				Message: "conflict detected",
			},
			expected: "conflict detected",
		},
		{
			name: "With wrapped error, no message",
			err: EntityConflictError{
				Err: errors.New("db conflict"),
			},
			expected: "db conflict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestEntityConflictError_Unwrap(t *testing.T) {
	t.Parallel()

	wrappedErr := errors.New("wrapped error")
	err := EntityConflictError{
		Err: wrappedErr,
	}

	assert.Equal(t, wrappedErr, err.Unwrap())
}

func TestUnauthorizedError_Error(t *testing.T) {
	t.Parallel()

	err := UnauthorizedError{
		Message: "unauthorized access",
	}
	assert.Equal(t, "unauthorized access", err.Error())
}

func TestForbiddenError_Error(t *testing.T) {
	t.Parallel()

	err := ForbiddenError{
		Message: "forbidden action",
	}
	assert.Equal(t, "forbidden action", err.Error())
}

func TestUnprocessableOperationError_Error(t *testing.T) {
	t.Parallel()

	err := UnprocessableOperationError{
		Message: "cannot process",
	}
	assert.Equal(t, "cannot process", err.Error())
}

func TestHTTPError_Error(t *testing.T) {
	t.Parallel()

	err := HTTPError{
		Message: "http error",
	}
	assert.Equal(t, "http error", err.Error())
}

func TestFailedPreconditionError_Error(t *testing.T) {
	t.Parallel()

	err := FailedPreconditionError{
		Message: "precondition failed",
	}
	assert.Equal(t, "precondition failed", err.Error())
}

func TestInternalServerError_Error(t *testing.T) {
	t.Parallel()

	err := InternalServerError{
		Message: "internal error",
	}
	assert.Equal(t, "internal error", err.Error())
}

func TestResponseError_Error(t *testing.T) {
	t.Parallel()

	err := ResponseError{
		Code:    500,
		Title:   "Internal Error",
		Message: "something went wrong",
	}
	assert.Equal(t, "something went wrong", err.Error())
}

func TestValidationKnownFieldsError_Error(t *testing.T) {
	t.Parallel()

	err := ValidationKnownFieldsError{
		Message: "field validation error",
		Fields: FieldValidations{
			"name": "required",
		},
	}
	assert.Equal(t, "field validation error", err.Error())
}

func TestValidationUnknownFieldsError_Error(t *testing.T) {
	t.Parallel()

	err := ValidationUnknownFieldsError{
		Message: "unknown fields error",
		Fields: UnknownFields{
			"extra_field": "not allowed",
		},
	}
	assert.Equal(t, "unknown fields error", err.Error())
}

func TestValidateInternalError(t *testing.T) {
	t.Parallel()

	originalErr := errors.New("database connection failed")
	err := ValidateInternalError(originalErr, "report")

	internalErr, ok := err.(InternalServerError)
	assert.True(t, ok)
	assert.Equal(t, "report", internalErr.EntityType)
	assert.Equal(t, constant.ErrInternalServer.Error(), internalErr.Code)
	assert.Equal(t, "Internal Server Error", internalErr.Title)
	assert.Contains(t, internalErr.Message, "unexpected error")
}

func TestValidateBadRequestFieldsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		requiredFields     map[string]string
		knownInvalidFields map[string]string
		entityType         string
		unknownFields      map[string]any
		expectedType       string
	}{
		{
			name:               "Unknown fields error",
			requiredFields:     nil,
			knownInvalidFields: nil,
			entityType:         "template",
			unknownFields:      map[string]any{"extra": "value"},
			expectedType:       "ValidationUnknownFieldsError",
		},
		{
			name:               "Required fields error",
			requiredFields:     map[string]string{"name": "required"},
			knownInvalidFields: nil,
			entityType:         "template",
			unknownFields:      nil,
			expectedType:       "ValidationKnownFieldsError",
		},
		{
			name:               "Known invalid fields error",
			requiredFields:     nil,
			knownInvalidFields: map[string]string{"format": "invalid"},
			entityType:         "template",
			unknownFields:      nil,
			expectedType:       "ValidationKnownFieldsError",
		},
		{
			name:               "All empty - returns error",
			requiredFields:     nil,
			knownInvalidFields: nil,
			entityType:         "template",
			unknownFields:      nil,
			expectedType:       "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateBadRequestFieldsError(tt.requiredFields, tt.knownInvalidFields, tt.entityType, tt.unknownFields)
			assert.NotNil(t, err)

			switch tt.expectedType {
			case "ValidationUnknownFieldsError":
				_, ok := err.(ValidationUnknownFieldsError)
				assert.True(t, ok)
			case "ValidationKnownFieldsError":
				_, ok := err.(ValidationKnownFieldsError)
				assert.True(t, ok)
			case "error":
				assert.Contains(t, err.Error(), "expected")
			}
		})
	}
}

func TestValidateBusinessError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          error
		entityType   string
		args         []any
		expectedType string
	}{
		{
			name:         "Entity not found",
			err:          constant.ErrEntityNotFound,
			entityType:   "template",
			args:         []any{"template"},
			expectedType: "EntityNotFoundError",
		},
		{
			name:         "Invalid query parameter",
			err:          constant.ErrInvalidQueryParameter,
			entityType:   "report",
			args:         []any{"limit"},
			expectedType: "ValidationError",
		},
		{
			name:         "Missing required fields",
			err:          constant.ErrMissingRequiredFields,
			entityType:   "template",
			args:         nil,
			expectedType: "ValidationError",
		},
		{
			name:         "Invalid file format",
			err:          constant.ErrInvalidFileFormat,
			entityType:   "template",
			args:         nil,
			expectedType: "ValidationError",
		},
		{
			name:         "Invalid output format",
			err:          constant.ErrInvalidOutputFormat,
			entityType:   "template",
			args:         nil,
			expectedType: "ValidationError",
		},
		{
			name:         "Empty file",
			err:          constant.ErrEmptyFile,
			entityType:   "template",
			args:         nil,
			expectedType: "ValidationError",
		},
		{
			name:         "Report status not finished",
			err:          constant.ErrReportStatusNotFinished,
			entityType:   "report",
			args:         nil,
			expectedType: "ValidationError",
		},
		{
			name:         "Missing data source",
			err:          constant.ErrMissingDataSource,
			entityType:   "report",
			args:         []any{"unknown_db"},
			expectedType: "ValidationError",
		},
		{
			name:         "Script tag detected",
			err:          constant.ErrScriptTagDetected,
			entityType:   "template",
			args:         nil,
			expectedType: "ValidationError",
		},
		{
			name:         "Schema ambiguous",
			err:          constant.ErrSchemaAmbiguous,
			entityType:   "template",
			args:         []any{"users", "public, custom"},
			expectedType: "ValidationError",
		},
		{
			name:         "Unknown error - returns original",
			err:          errors.New("unknown error"),
			entityType:   "template",
			args:         nil,
			expectedType: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := ValidateBusinessError(tt.err, tt.entityType, tt.args...)
			assert.NotNil(t, result)

			switch tt.expectedType {
			case "EntityNotFoundError":
				_, ok := result.(EntityNotFoundError)
				assert.True(t, ok)
			case "ValidationError":
				_, ok := result.(ValidationError)
				assert.True(t, ok)
			case "error":
				assert.Equal(t, tt.err, result)
			}
		})
	}
}

func TestValidateBusinessError_AllMappedErrors(t *testing.T) {
	t.Parallel()

	// Test all mapped errors to ensure they return correct types
	mappedErrors := []error{
		constant.ErrInvalidDateFormat,
		constant.ErrInvalidFinalDate,
		constant.ErrDateRangeExceedsLimit,
		constant.ErrInvalidDateRange,
		constant.ErrPaginationLimitExceeded,
		constant.ErrInvalidSortOrder,
		constant.ErrMetadataKeyLengthExceeded,
		constant.ErrMetadataValueLengthExceeded,
		constant.ErrInvalidMetadataNesting,
		constant.ErrInvalidHeaderParameter,
		constant.ErrInvalidFileUploaded,
		constant.ErrFileContentInvalid,
		constant.ErrInvalidMapFields,
		constant.ErrInvalidPathParameter,
		constant.ErrOutputFormatWithoutTemplateFile,
		constant.ErrInvalidTemplateID,
		constant.ErrInvalidLedgerIDList,
		constant.ErrMissingTableFields,
		constant.ErrMissingSchemaTable,
		constant.ErrDecryptionData,
		constant.ErrCommunicateSeaweedFS,
		constant.ErrSchemaNotFound,
		constant.ErrTableNotFoundInSchema,
		constant.ErrDatabaseNotRegistered,
		constant.ErrDataSourceNotFound,
		constant.ErrDataSourceUnavailable,
		constant.ErrSchemaValidationFailed,
		constant.ErrExtractionJobFailed,
	}

	for _, err := range mappedErrors {
		t.Run(err.Error(), func(t *testing.T) {
			t.Parallel()

			result := ValidateBusinessError(err, "test", "arg1", "arg2")
			assert.NotNil(t, result)
			// All mapped errors should return a different type than the original
			assert.NotEqual(t, err, result)
		})
	}
}
