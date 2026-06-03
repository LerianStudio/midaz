// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
)

func TestEntityNotFoundError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      EntityNotFoundError
		expected string
	}{
		{
			name: "Success - returns message when set",
			err: EntityNotFoundError{
				Message: "custom message",
			},
			expected: "custom message",
		},
		{
			name: "Success - returns entity type message when message empty",
			err: EntityNotFoundError{
				EntityType: "User",
			},
			expected: "Entity User not found",
		},
		{
			name: "Success - returns wrapped error when message and entity empty",
			err: EntityNotFoundError{
				Err: errors.New("wrapped error"),
			},
			expected: "wrapped error",
		},
		{
			name:     "Success - returns default message when all empty",
			err:      EntityNotFoundError{},
			expected: "entity not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.err.Error()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestEntityNotFoundError_Unwrap(t *testing.T) {
	wrappedErr := errors.New("original error")
	err := EntityNotFoundError{Err: wrappedErr}

	assert.Equal(t, wrappedErr, err.Unwrap())
}

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      ValidationError
		expected string
	}{
		{
			name: "Success - returns code and message when code set",
			err: ValidationError{
				Code:    "ERR001",
				Message: "validation failed",
			},
			expected: "ERR001 - validation failed",
		},
		{
			name: "Success - returns only message when code empty",
			err: ValidationError{
				Message: "validation failed",
			},
			expected: "validation failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.err.Error()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestValidationError_Unwrap(t *testing.T) {
	wrappedErr := errors.New("original error")
	err := ValidationError{Err: wrappedErr}

	assert.Equal(t, wrappedErr, err.Unwrap())
}

func TestEntityConflictError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      EntityConflictError
		expected string
	}{
		{
			name: "Success - returns message when set",
			err: EntityConflictError{
				Message: "conflict message",
			},
			expected: "conflict message",
		},
		{
			name: "Success - returns wrapped error when message empty",
			err: EntityConflictError{
				Err: errors.New("wrapped error"),
			},
			expected: "wrapped error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.err.Error()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestEntityConflictError_Unwrap(t *testing.T) {
	wrappedErr := errors.New("original error")
	err := EntityConflictError{Err: wrappedErr}

	assert.Equal(t, wrappedErr, err.Unwrap())
}

func TestErrorTypes_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "UnauthorizedError",
			err:      UnauthorizedError{Message: "unauthorized"},
			expected: "unauthorized",
		},
		{
			name:     "ForbiddenError",
			err:      ForbiddenError{Message: "forbidden"},
			expected: "forbidden",
		},
		{
			name:     "UnprocessableOperationError",
			err:      UnprocessableOperationError{Message: "unprocessable"},
			expected: "unprocessable",
		},
		{
			name:     "HTTPError",
			err:      HTTPError{Message: "http error"},
			expected: "http error",
		},
		{
			name:     "FailedPreconditionError",
			err:      FailedPreconditionError{Message: "precondition failed"},
			expected: "precondition failed",
		},
		{
			name:     "InternalServerError",
			err:      InternalServerError{Message: "internal error"},
			expected: "internal error",
		},
		{
			name:     "ResponseError",
			err:      ResponseError{Message: "response error", Code: 500},
			expected: "response error",
		},
		{
			name:     "ValidationKnownFieldsError",
			err:      ValidationKnownFieldsError{Message: "known fields error", Fields: FieldValidations{"field1": "invalid"}},
			expected: "known fields error",
		},
		{
			name:     "ValidationUnknownFieldsError",
			err:      ValidationUnknownFieldsError{Message: "unknown fields error", Fields: UnknownFields{"field1": "unexpected"}},
			expected: "unknown fields error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.err.Error())
		})
	}
}

func TestValidateInternalError(t *testing.T) {
	originalErr := errors.New("database error")
	err := ValidateInternalError(originalErr, "User")

	internalErr, ok := err.(InternalServerError)
	assert.True(t, ok)
	assert.Equal(t, "User", internalErr.EntityType)
	assert.Equal(t, constant.ErrInternalServer.Error(), internalErr.Code)
	assert.Equal(t, "Internal Server Error", internalErr.Title)
	assert.Equal(t, originalErr, internalErr.Err)
}

func TestValidateBadRequestFieldsError(t *testing.T) {
	tests := []struct {
		name               string
		requiredFields     map[string]string
		knownInvalidFields map[string]string
		unknownFields      map[string]any
		entityType         string
		expectedType       string
		expectedCode       string
	}{
		{
			name:         "Error - returns error when all fields empty",
			entityType:   "User",
			expectedType: "error",
			expectedCode: "",
		},
		{
			name:          "Success - returns unknown fields error",
			unknownFields: map[string]any{"extra": "value"},
			entityType:    "User",
			expectedType:  "ValidationUnknownFieldsError",
			expectedCode:  constant.ErrUnexpectedFieldsInTheRequest.Error(),
		},
		{
			name:           "Success - returns required fields error",
			requiredFields: map[string]string{"name": "required"},
			entityType:     "User",
			expectedType:   "ValidationKnownFieldsError",
			expectedCode:   constant.ErrMissingFieldsInRequest.Error(),
		},
		{
			name:               "Success - returns known invalid fields error",
			knownInvalidFields: map[string]string{"email": "invalid format"},
			entityType:         "User",
			expectedType:       "ValidationKnownFieldsError",
			expectedCode:       constant.ErrBadRequest.Error(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBadRequestFieldsError(tc.requiredFields, tc.knownInvalidFields, tc.entityType, tc.unknownFields)

			assert.Error(t, err)

			switch tc.expectedType {
			case "ValidationUnknownFieldsError":
				unknownErr, ok := err.(ValidationUnknownFieldsError)
				assert.True(t, ok)
				assert.Equal(t, tc.expectedCode, unknownErr.Code)
			case "ValidationKnownFieldsError":
				knownErr, ok := err.(ValidationKnownFieldsError)
				assert.True(t, ok)
				assert.Equal(t, tc.expectedCode, knownErr.Code)
			case "error":
				// Generic error case - verify it's not a validation error type
				_, isUnknown := err.(ValidationUnknownFieldsError)
				_, isKnown := err.(ValidationKnownFieldsError)
				assert.False(t, isUnknown, "should not be ValidationUnknownFieldsError")
				assert.False(t, isKnown, "should not be ValidationKnownFieldsError")
				assert.NotEmpty(t, err.Error(), "error message should not be empty")
			}
		})
	}
}

func TestValidateBusinessError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		entityType   string
		args         []any
		expectedType string
	}{
		{
			name:         "Success - returns ValidationError for calculation field type",
			err:          constant.ErrCalculationFieldType,
			entityType:   "Transaction",
			expectedType: "ValidationError",
		},
		{
			name:         "Success - returns original error when not mapped",
			err:          errors.New("unknown error"),
			entityType:   "User",
			expectedType: "error",
		},
		{
			name:         "Success - returns ValidationError for invalid query parameter",
			err:          constant.ErrInvalidQueryParameter,
			entityType:   "Transaction",
			args:         []any{"status"},
			expectedType: "ValidationError",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ValidateBusinessError(tc.err, tc.entityType, tc.args...)

			switch tc.expectedType {
			case "ValidationError":
				_, ok := result.(ValidationError)
				assert.True(t, ok)
			case "EntityNotFoundError":
				_, ok := result.(EntityNotFoundError)
				assert.True(t, ok)
			case "error":
				assert.Equal(t, tc.err, result)
			}
		})
	}
}
