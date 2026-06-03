// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/stretchr/testify/assert"
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
				Message: "Package not found",
			},
			expected: "Package not found",
		},
		{
			name: "With entity type but no message",
			err: EntityNotFoundError{
				EntityType: "Package",
				Message:    "",
			},
			expected: "Entity Package not found",
		},
		{
			name: "With wrapped error but no message",
			err: EntityNotFoundError{
				Err:     errors.New("database error"),
				Message: "",
			},
			expected: "database error",
		},
		{
			name: "Empty error",
			err: EntityNotFoundError{
				Message: "",
			},
			expected: "entity not found",
		},
		{
			name: "Message with whitespace only",
			err: EntityNotFoundError{
				Message: "   ",
			},
			expected: "entity not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEntityNotFoundError_Unwrap(t *testing.T) {
	t.Parallel()

	innerErr := errors.New("inner error")
	err := EntityNotFoundError{
		Err: innerErr,
	}

	assert.Equal(t, innerErr, err.Unwrap())
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
				Code:    "FEE-0001",
				Message: "Validation failed",
			},
			expected: "FEE-0001 - Validation failed",
		},
		{
			name: "With message but no code",
			err: ValidationError{
				Message: "Validation failed",
			},
			expected: "Validation failed",
		},
		{
			name: "Empty error",
			err: ValidationError{
				Message: "",
			},
			expected: "",
		},
		{
			name: "Code with whitespace only",
			err: ValidationError{
				Code:    "   ",
				Message: "Test message",
			},
			expected: "Test message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidationError_Unwrap(t *testing.T) {
	t.Parallel()

	innerErr := errors.New("inner error")
	err := ValidationError{
		Err: innerErr,
	}

	assert.Equal(t, innerErr, err.Unwrap())
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
				Message: "Entity already exists",
			},
			expected: "Entity already exists",
		},
		{
			name: "With wrapped error but no message",
			err: EntityConflictError{
				Err:     errors.New("database conflict"),
				Message: "",
			},
			expected: "database conflict",
		},
		{
			name: "Empty error",
			err: EntityConflictError{
				Message: "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEntityConflictError_Unwrap(t *testing.T) {
	t.Parallel()

	innerErr := errors.New("inner error")
	err := EntityConflictError{
		Err: innerErr,
	}

	assert.Equal(t, innerErr, err.Unwrap())
}

func TestUnauthorizedError_Error(t *testing.T) {
	t.Parallel()

	err := UnauthorizedError{
		Message: "Authentication required",
	}

	assert.Equal(t, "Authentication required", err.Error())
}

func TestForbiddenError_Error(t *testing.T) {
	t.Parallel()

	err := ForbiddenError{
		Message: "Access forbidden",
	}

	assert.Equal(t, "Access forbidden", err.Error())
}

func TestUnprocessableOperationError_Error(t *testing.T) {
	t.Parallel()

	err := UnprocessableOperationError{
		Message: "Operation cannot be processed",
	}

	assert.Equal(t, "Operation cannot be processed", err.Error())
}

func TestHTTPError_Error(t *testing.T) {
	t.Parallel()

	err := HTTPError{
		Message: "HTTP request failed",
	}

	assert.Equal(t, "HTTP request failed", err.Error())
}

func TestFailedPreconditionError_Error(t *testing.T) {
	t.Parallel()

	err := FailedPreconditionError{
		Message: "Precondition failed",
	}

	assert.Equal(t, "Precondition failed", err.Error())
}

func TestInternalServerError_Error(t *testing.T) {
	t.Parallel()

	err := InternalServerError{
		Message: "Internal server error",
	}

	assert.Equal(t, "Internal server error", err.Error())
}

func TestResponseError_Error(t *testing.T) {
	t.Parallel()

	err := ResponseError{
		Message: "Response error message",
	}

	assert.Equal(t, "Response error message", err.Error())
}

func TestValidationKnownFieldsError_Error(t *testing.T) {
	t.Parallel()

	err := ValidationKnownFieldsError{
		Message: "Validation error",
		Fields: FieldValidations{
			"field1": "Field 1 is required",
			"field2": "Field 2 is invalid",
		},
	}

	assert.Equal(t, "Validation error", err.Error())
	assert.NotNil(t, err.Fields)
	assert.Len(t, err.Fields, 2)
}

func TestValidationUnknownFieldsError_Error(t *testing.T) {
	t.Parallel()

	err := ValidationUnknownFieldsError{
		Message: "Unknown fields error",
		Fields: UnknownFields{
			"unknownField1": "value1",
			"unknownField2": "value2",
		},
	}

	assert.Equal(t, "Unknown fields error", err.Error())
	assert.NotNil(t, err.Fields)
	assert.Len(t, err.Fields, 2)
}

func TestValidateInternalError(t *testing.T) {
	t.Parallel()

	innerErr := errors.New("database connection failed")
	entityType := "Package"

	result := ValidateInternalError(innerErr, entityType)

	assert.Error(t, result)
	internalErr, ok := result.(InternalServerError)
	assert.True(t, ok)
	assert.Equal(t, entityType, internalErr.EntityType)
	assert.Equal(t, constant.ErrInternalServer.Error(), internalErr.Code)
	assert.Equal(t, "Internal Server Error", internalErr.Title)
	assert.Equal(t, innerErr, internalErr.Err)
}

func TestValidateBadRequestFieldsError_UnknownFields(t *testing.T) {
	t.Parallel()

	unknownFields := map[string]any{
		"unknownField1": "value1",
		"unknownField2": "value2",
	}

	result := ValidateBadRequestFieldsError(
		map[string]string{},
		map[string]string{},
		"Package",
		unknownFields,
	)

	assert.Error(t, result)
	unknownErr, ok := result.(ValidationUnknownFieldsError)
	assert.True(t, ok)
	assert.Equal(t, "Package", unknownErr.EntityType)
	assert.Equal(t, constant.ErrUnexpectedFieldsInTheRequest.Error(), unknownErr.Code)
	assert.Equal(t, "Unexpected Fields in the Request", unknownErr.Title)
	assert.EqualValues(t, unknownFields, unknownErr.Fields)
	assert.Equal(t, "value1", unknownErr.Fields["unknownField1"])
	assert.Equal(t, "value2", unknownErr.Fields["unknownField2"])
}

func TestValidateBadRequestFieldsError_RequiredFields(t *testing.T) {
	t.Parallel()

	requiredFields := map[string]string{
		"field1": "Field 1 is required",
		"field2": "Field 2 is required",
	}

	result := ValidateBadRequestFieldsError(
		requiredFields,
		map[string]string{},
		"Package",
		map[string]any{},
	)

	assert.Error(t, result)
	knownErr, ok := result.(ValidationKnownFieldsError)
	assert.True(t, ok)
	assert.Equal(t, "Package", knownErr.EntityType)
	assert.Equal(t, constant.ErrMissingFieldsInRequest.Error(), knownErr.Code)
	assert.Equal(t, "Missing Fields in Request", knownErr.Title)
	assert.EqualValues(t, requiredFields, knownErr.Fields)
	assert.Equal(t, "Field 1 is required", knownErr.Fields["field1"])
	assert.Equal(t, "Field 2 is required", knownErr.Fields["field2"])
}

func TestValidateBadRequestFieldsError_KnownInvalidFields(t *testing.T) {
	t.Parallel()

	knownInvalidFields := map[string]string{
		"field1": "Field 1 is invalid",
		"field2": "Field 2 format is wrong",
	}

	result := ValidateBadRequestFieldsError(
		map[string]string{},
		knownInvalidFields,
		"Package",
		map[string]any{},
	)

	assert.Error(t, result)
	knownErr, ok := result.(ValidationKnownFieldsError)
	assert.True(t, ok)
	assert.Equal(t, "Package", knownErr.EntityType)
	assert.Equal(t, constant.ErrBadRequest.Error(), knownErr.Code)
	assert.Equal(t, "Bad Request", knownErr.Title)
	assert.EqualValues(t, knownInvalidFields, knownErr.Fields)
	assert.Equal(t, "Field 1 is invalid", knownErr.Fields["field1"])
	assert.Equal(t, "Field 2 format is wrong", knownErr.Fields["field2"])
}

func TestValidateBadRequestFieldsError_AllEmpty(t *testing.T) {
	t.Parallel()

	result := ValidateBadRequestFieldsError(
		map[string]string{},
		map[string]string{},
		"Package",
		map[string]any{},
	)

	assert.Error(t, result)
	assert.Contains(t, result.Error(), "expected knownInvalidFields, unknownFields and requiredFields to be non-empty")
}

func TestValidateBusinessError_KnownErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		entityType string
		args       []any
		check      func(*testing.T, error)
	}{
		{
			name:       "ErrCalculationFieldType",
			err:        constant.ErrCalculationFieldType,
			entityType: "Fee",
			check: func(t *testing.T, result error) {
				validationErr, ok := result.(ValidationError)
				assert.True(t, ok)
				assert.Equal(t, "Fee", validationErr.EntityType)
				assert.Equal(t, constant.ErrCalculationFieldType.Error(), validationErr.Code)
				assert.Equal(t, "Calculation field type invalid", validationErr.Title)
			},
		},
		{
			name:       "ErrEntityNotFound",
			err:        constant.ErrEntityNotFound,
			entityType: "Package",
			args:       []any{"package-id"},
			check: func(t *testing.T, result error) {
				notFoundErr, ok := result.(EntityNotFoundError)
				assert.True(t, ok)
				assert.Equal(t, "Package", notFoundErr.EntityType)
				assert.Equal(t, constant.ErrEntityNotFound.Error(), notFoundErr.Code)
				assert.Equal(t, "Entity Not Found", notFoundErr.Title)
			},
		},
		{
			name:       "ErrForbiddenAccessMidaz",
			err:        constant.ErrForbiddenAccessMidaz,
			entityType: "Fee",
			args:       []any{"account-alias"},
			check: func(t *testing.T, result error) {
				forbiddenErr, ok := result.(ForbiddenError)
				assert.True(t, ok)
				assert.Equal(t, "Fee", forbiddenErr.EntityType)
				assert.Equal(t, constant.ErrForbiddenAccessMidaz.Error(), forbiddenErr.Code)
			},
		},
		{
			name:       "ErrInvalidQueryParameter with args",
			err:        constant.ErrInvalidQueryParameter,
			entityType: "Query",
			args:       []any{"limit", "page"},
			check: func(t *testing.T, result error) {
				validationErr, ok := result.(ValidationError)
				assert.True(t, ok)
				assert.Contains(t, validationErr.Message, "limit")
				assert.Contains(t, validationErr.Message, "page")
			},
		},
		{
			name:       "ErrConvertToDecimal with field name",
			err:        constant.ErrConvertToDecimal,
			entityType: "Package",
			args:       []any{"minimumAmount"},
			check: func(t *testing.T, result error) {
				validationErr, ok := result.(ValidationError)
				assert.True(t, ok)
				assert.Contains(t, validationErr.Message, "minimumAmount")
				assert.Contains(t, validationErr.Message, "dot (.)")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateBusinessError(tt.err, tt.entityType, tt.args...)
			assert.Error(t, result)
			tt.check(t, result)
		})
	}
}

func TestValidateBusinessError_UnknownError(t *testing.T) {
	t.Parallel()

	unknownErr := errors.New("unknown error")
	result := ValidateBusinessError(unknownErr, "Package")

	assert.Equal(t, unknownErr, result)
}

func TestValidateBusinessError_AllErrorTypes(t *testing.T) {
	t.Parallel()

	errorConstants := []struct {
		err        error
		entityType string
		args       []any
	}{
		{constant.ErrCalculationFieldType, "Fee", nil},
		{constant.ErrInvalidQueryParameter, "Query", []any{"param1"}},
		{constant.ErrInvalidDateFormat, "Date", nil},
		{constant.ErrInvalidFinalDate, "Date", nil},
		{constant.ErrDateRangeExceedsLimit, "Date", []any{12}},
		{constant.ErrInvalidDateRange, "Date", nil},
		{constant.ErrPaginationLimitExceeded, "Pagination", []any{100}},
		{constant.ErrInvalidSortOrder, "Sort", nil},
		{constant.ErrEntityNotFound, "Package", []any{"id"}},
		{constant.ErrPriorityInvalid, "Fee", nil},
		{constant.ErrFindAccountOnMidaz, "Account", []any{"alias"}},
		{constant.ErrMinAmountGreaterThanMaxAmount, "Package", nil},
		{constant.ErrInvalidPathParameter, "Path", []any{"id"}},
		{constant.ErrNothingToUpdate, "Package", nil},
		{constant.ErrInvalidHeaderParameter, "Header", []any{"X-Org-Id"}},
		{constant.ErrHeaderParameterRequired, "Header", []any{"X-Org-Id"}},
		{constant.ErrDuplicatePackage, "Package", nil},
		{constant.ErrInvalidTransactionType, "Transaction", []any{"from"}},
		{constant.ErrCalculateFee, "Fee", nil},
		{constant.ErrCalculationRequired, "Fee", []any{"fee1"}},
		{constant.ErrPriorityOne, "Fee", []any{"fee1"}},
		{constant.ErrAppRuleFlatFeeAndPercentual, "Fee", []any{"fee1"}},
		{constant.ErrAppRuleMaxBetweenTypes, "Fee", []any{"fee1"}},
		{constant.ErrCalculationTypePercentual, "Fee", []any{"fee1"}},
		{constant.ErrCalculationTypeFlatFee, "Fee", []any{"fee1"}},
		{constant.ErrFeeFieldsRequired, "Fee", nil},
		{constant.ErrCalculationFieldOfFeeRequired, "Fee", nil},
		{constant.ErrReferenceAmountInvalid, "Fee", nil},
		{constant.ErrAppRuleInvalid, "Fee", nil},
		{constant.ErrCalculationTypeInvalid, "Fee", nil},
		{constant.ErrMaxAmountLessThanMinAmount, "Package", nil},
		{constant.ErrFilterPackage, "Package", nil},
		{constant.ErrPackageRange, "Package", nil},
		{constant.ErrValidateDistributeTransactionValue, "Transaction", nil},
		{constant.ErrInvalidSegmentID, "Segment", nil},
		{constant.ErrInvalidLedgerID, "Ledger", nil},
		{constant.ErrConvertToDecimal, "Amount", []any{"field"}},
		{constant.ErrIsDeductibleFrom, "Fee", []any{"fee1"}},
		{constant.ErrApplicationRule, "Fee", []any{"error"}},
		{constant.ErrForbiddenAccessMidaz, "Midaz", []any{"account"}},
		{constant.ErrCalculationValuePercentage, "Fee", []any{"fee1"}},
		{constant.ErrCalculationValueFlatFee, "Fee", []any{"100", "fee1"}},
		{constant.ErrAccessMidaz, "Midaz", []any{"account"}},
		{constant.ErrDeductibleCalculationValuePercentage, "Fee", []any{"fee1"}},
		{constant.ErrDeductibleCalculationValueFlatFee, "Fee", []any{"100", "fee1"}},
		{constant.ErrInvalidQueryParameterPage, "Pagination", nil},
	}

	for _, ec := range errorConstants {
		t.Run(ec.err.Error(), func(t *testing.T) {
			result := ValidateBusinessError(ec.err, ec.entityType, ec.args...)
			assert.Error(t, result)
			assert.NotNil(t, result)
		})
	}
}

func TestValidateUnmarshallingError_StandardError(t *testing.T) {
	t.Parallel()

	err := errors.New("invalid json")
	result := ValidateUnmarshallingError(err)

	assert.Error(t, result)
	responseErr, ok := result.(ResponseError)
	assert.True(t, ok)
	assert.Equal(t, constant.ErrInvalidRequestBody.Error(), responseErr.Code)
	assert.Equal(t, "Unmarshalling error", responseErr.Title)
	assert.Equal(t, "invalid json", responseErr.Message)
}

func TestValidateUnmarshallingError_UnmarshalTypeError(t *testing.T) {
	t.Parallel()

	type TestStruct struct {
		Field1 int    `json:"field1"`
		Field2 string `json:"field2"`
	}

	var result TestStruct
	jsonData := `{"field1": "invalid", "field2": 123}`
	err := json.Unmarshal([]byte(jsonData), &result)

	assert.Error(t, err)
	unmarshalledErr := ValidateUnmarshallingError(err)

	assert.Error(t, unmarshalledErr)
	responseErr, ok := unmarshalledErr.(ResponseError)
	assert.True(t, ok)
	assert.Equal(t, constant.ErrInvalidRequestBody.Error(), responseErr.Code)
	assert.Equal(t, "Unmarshalling error", responseErr.Title)
	assert.Contains(t, responseErr.Message, "field1")
	assert.Contains(t, responseErr.Message, "int")
}

func TestValidateUnmarshallingError_JSONSyntaxError(t *testing.T) {
	t.Parallel()

	type TestStruct struct {
		Field1 string `json:"field1"`
	}

	var result TestStruct
	jsonData := `{"field1": invalid}`
	err := json.Unmarshal([]byte(jsonData), &result)

	assert.Error(t, err)
	unmarshalledErr := ValidateUnmarshallingError(err)

	assert.Error(t, unmarshalledErr)
	responseErr, ok := unmarshalledErr.(ResponseError)
	assert.True(t, ok)
	assert.Equal(t, constant.ErrInvalidRequestBody.Error(), responseErr.Code)
}
