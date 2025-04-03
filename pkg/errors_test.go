package pkg

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/stretchr/testify/assert"
)

// \1 performs an operation
func TestEntityNotFoundError_Error(t *testing.T) {
	tests := []struct {
		name     string
		errorObj EntityNotFoundError
		expected string
	}{
		{
			name: "EntityType is not empty",
			errorObj: EntityNotFoundError{
				EntityType: "User",
			},
			expected: "Entity User not found",
		},
		{
			name: "Message is not empty",
			errorObj: EntityNotFoundError{
				Message: "Custom error message",
			},
			expected: "Custom error message",
		},
		{
			name: "Message is empty, but Err is set",
			errorObj: EntityNotFoundError{
				Err: errors.New("internal error"),
			},
			expected: "internal error",
		},
		{
			name: "Message and EntityType are empty, and Err is nil",
			errorObj: EntityNotFoundError{
				Err: nil,
			},
			expected: "entity not found",
		},
		{
			name: "Message is empty and EntityType is empty",
			errorObj: EntityNotFoundError{
				EntityType: "",
			},
			expected: "entity not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errorObj.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// \1 performs an operation
func TestEntityNotFoundError_Unwrap(t *testing.T) {
	innerErr := errors.New("inner error")
	err := EntityNotFoundError{
		Err: innerErr,
	}

	unwrapped := err.Unwrap()
	assert.Equal(t, innerErr, unwrapped)

	// Test with nil inner error
	err = EntityNotFoundError{
		Err: nil,
	}

	unwrapped = err.Unwrap()
	assert.Nil(t, unwrapped)
}

// \1 performs an operation
func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		ve       ValidationError
		expected string
	}{
		{
			name:     "When Code is non-empty",
			ve:       ValidationError{Code: "400", Message: "Bad Request"},
			expected: "400 - Bad Request",
		},
		{
			name:     "When Code is empty",
			ve:       ValidationError{Code: "", Message: "Bad Request"},
			expected: "Bad Request",
		},
		{
			name:     "When Code has only spaces",
			ve:       ValidationError{Code: "   ", Message: "Bad Request"},
			expected: "Bad Request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ve.Error()

			if result != tt.expected {
				t.Errorf("expected %q but got %q", tt.expected, result)
			}
		})
	}
}

// \1 performs an operation
func TestValidationError_Unwrap(t *testing.T) {
	innerErr := errors.New("validation inner error")
	err := ValidationError{
		Err: innerErr,
	}

	unwrapped := err.Unwrap()
	assert.Equal(t, innerErr, unwrapped)

	// Test with nil inner error
	err = ValidationError{
		Err: nil,
	}

	unwrapped = err.Unwrap()
	assert.Nil(t, unwrapped)
}

// \1 performs an operation
func TestEntityConflictError_Error(t *testing.T) {
	tests := []struct {
		name     string
		errorObj EntityConflictError
		expected string
	}{
		{
			name: "Message is not empty",
			errorObj: EntityConflictError{
				Message: "Custom error message",
			},
			expected: "Custom error message",
		},
		{
			name: "Message is empty, but Err is set",
			errorObj: EntityConflictError{
				Err: errors.New("internal error"),
			},
			expected: "internal error",
		},
		{
			name: "Message is empty and Err is nil",
			errorObj: EntityConflictError{
				Err: nil,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errorObj.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// \1 performs an operation
func TestEntityConflictError_Unwrap(t *testing.T) {
	innerErr := errors.New("conflict inner error")
	err := EntityConflictError{
		Err: innerErr,
	}

	unwrapped := err.Unwrap()
	assert.Equal(t, innerErr, unwrapped)

	// Test with nil inner error
	err = EntityConflictError{
		Err: nil,
	}

	unwrapped = err.Unwrap()
	assert.Nil(t, unwrapped)
}

// \1 performs an operation
func TestUnauthorizedError_Error(t *testing.T) {
	tests := []struct {
		name     string
		errorObj UnauthorizedError
		expected string
	}{
		{
			name: "With message",
			errorObj: UnauthorizedError{
				Message: "Unauthorized access",
			},
			expected: "Unauthorized access",
		},
		{
			name: "Empty message",
			errorObj: UnauthorizedError{
				Message: "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errorObj.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// \1 performs an operation
func TestForbiddenError_Error(t *testing.T) {
	tests := []struct {
		name     string
		errorObj ForbiddenError
		expected string
	}{
		{
			name: "With message",
			errorObj: ForbiddenError{
				Message: "Forbidden access",
			},
			expected: "Forbidden access",
		},
		{
			name: "Empty message",
			errorObj: ForbiddenError{
				Message: "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errorObj.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// \1 performs an operation
func TestUnprocessableOperationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		errorObj UnprocessableOperationError
		expected string
	}{
		{
			name: "With message",
			errorObj: UnprocessableOperationError{
				Message: "Unprocessable operation",
			},
			expected: "Unprocessable operation",
		},
		{
			name: "Empty message",
			errorObj: UnprocessableOperationError{
				Message: "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errorObj.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// \1 performs an operation
func TestHTTPError_Error(t *testing.T) {
	tests := []struct {
		name     string
		errorObj HTTPError
		expected string
	}{
		{
			name: "With message",
			errorObj: HTTPError{
				Message: "HTTP error occurred",
			},
			expected: "HTTP error occurred",
		},
		{
			name: "Empty message",
			errorObj: HTTPError{
				Message: "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errorObj.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// \1 performs an operation
func TestFailedPreconditionError_Error(t *testing.T) {
	tests := []struct {
		name     string
		errorObj FailedPreconditionError
		expected string
	}{
		{
			name: "With message",
			errorObj: FailedPreconditionError{
				Message: "Failed precondition",
			},
			expected: "Failed precondition",
		},
		{
			name: "Empty message",
			errorObj: FailedPreconditionError{
				Message: "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errorObj.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// \1 performs an operation
func TestInternalServerError_Error(t *testing.T) {
	tests := []struct {
		name     string
		errorObj InternalServerError
		expected string
	}{
		{
			name: "With message",
			errorObj: InternalServerError{
				Message: "Internal server error",
			},
			expected: "Internal server error",
		},
		{
			name: "Empty message",
			errorObj: InternalServerError{
				Message: "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errorObj.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// \1 performs an operation
func TestResponseError_Error(t *testing.T) {
	tests := []struct {
		name     string
		errorObj ResponseError
		expected string
	}{
		{
			name: "With message and code",
			errorObj: ResponseError{
				Message: "Response error",
				Code:    "500",
			},
			expected: "Response error",
		},
		{
			name: "With message, no code",
			errorObj: ResponseError{
				Message: "Response error",
			},
			expected: "Response error",
		},
		{
			name: "Empty message, with code",
			errorObj: ResponseError{
				Code: "404",
			},
			expected: "",
		},
		{
			name:     "Empty message, no code",
			errorObj: ResponseError{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errorObj.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// \1 performs an operation
func TestValidationKnownFieldsError_Error(t *testing.T) {
	tests := []struct {
		name     string
		errorObj ValidationKnownFieldsError
		expected string
	}{
		{
			name: "With message",
			errorObj: ValidationKnownFieldsError{
				Message: "Validation known fields error",
				Fields: FieldValidations{
					"email": "Invalid email format",
				},
			},
			expected: "Validation known fields error",
		},
		{
			name: "Empty message",
			errorObj: ValidationKnownFieldsError{
				Message: "",
				Fields: FieldValidations{
					"email": "Invalid email format",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errorObj.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// \1 performs an operation
func TestValidationUnknownFieldsError_Error(t *testing.T) {
	tests := []struct {
		name     string
		errorObj ValidationUnknownFieldsError
		expected string
	}{
		{
			name: "With message",
			errorObj: ValidationUnknownFieldsError{
				Message: "Validation unknown fields error",
				Fields: UnknownFields{
					"unknown_field": "value",
				},
			},
			expected: "Validation unknown fields error",
		},
		{
			name: "Empty message",
			errorObj: ValidationUnknownFieldsError{
				Message: "",
				Fields: UnknownFields{
					"unknown_field": "value",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errorObj.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// \1 performs an operation
func TestValidateBadRequestFieldsError(t *testing.T) {
	tests := []struct {
		name               string
		requiredFields     map[string]string
		knownInvalidFields map[string]string
		entityType         string
		unknownFields      map[string]any
		expectedError      bool
		expectedErrorType  string
	}{
		{
			name:               "All fields are empty",
			requiredFields:     map[string]string{},
			knownInvalidFields: map[string]string{},
			entityType:         "User",
			unknownFields:      map[string]any{},
			expectedError:      true,
			expectedErrorType:  "errorString",
		},
		{
			name:               "Unknown fields present",
			requiredFields:     map[string]string{},
			knownInvalidFields: map[string]string{},
			entityType:         "User",
			unknownFields:      map[string]any{"unknown_field": "value"},
			expectedError:      true,
			expectedErrorType:  "ValidationUnknownFieldsError",
		},
		{
			name:               "Required fields missing",
			requiredFields:     map[string]string{"name": "Name is required"},
			knownInvalidFields: map[string]string{},
			entityType:         "User",
			unknownFields:      map[string]any{},
			expectedError:      true,
			expectedErrorType:  "ValidationKnownFieldsError",
		},
		{
			name:               "Known invalid fields",
			requiredFields:     map[string]string{},
			knownInvalidFields: map[string]string{"email": "Invalid email format"},
			entityType:         "User",
			unknownFields:      map[string]any{},
			expectedError:      true,
			expectedErrorType:  "ValidationKnownFieldsError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateBadRequestFieldsError(tt.requiredFields, tt.knownInvalidFields, tt.entityType, tt.unknownFields)

			assert.NotNil(t, result, "Expected an error but got nil")

			switch tt.expectedErrorType {
			case "errorString":
				_, ok := result.(error)
				assert.True(t, ok, "Expected a generic error")
			case "ValidationUnknownFieldsError":
				_, ok := result.(ValidationUnknownFieldsError)
				assert.True(t, ok, "Expected ValidationUnknownFieldsError")
			case "ValidationKnownFieldsError":
				_, ok := result.(ValidationKnownFieldsError)
				assert.True(t, ok, "Expected ValidationKnownFieldsError")
			}
		})
	}
}

// \1 performs an operation
func TestValidateBusinessError(t *testing.T) {
	// Create a simple mock for ValidateBusinessError
	mockValidateBusinessError := func(err error, entityType string, args ...interface{}) error {
		switch err.Error() {
		case "duplicate_ledger":
			return &EntityConflictError{
				EntityType: entityType,
				Message:    "Entity conflict error",
			}
		case "transaction_value_mismatch":
			return &ValidationError{
				EntityType: entityType,
				Message:    "Validation error",
			}
		default:
			return err
		}
	}

	tests := []struct {
		name       string
		err        error
		entityType string
		args       []interface{}
		expected   interface{}
	}{
		{
			name:       "entity conflict error",
			err:        errors.New("duplicate_ledger"),
			entityType: "User",
			args:       []interface{}{},
			expected:   &EntityConflictError{},
		},
		{
			name:       "transaction value mismatch error",
			err:        errors.New("transaction_value_mismatch"),
			entityType: "Transaction",
			args:       []interface{}{},
			expected:   &ValidationError{},
		},
		{
			name:       "error not mapped",
			err:        errors.New("custom error"),
			entityType: "Custom",
			args:       []interface{}{},
			expected:   errors.New("custom error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mockValidateBusinessError(tt.err, tt.entityType, tt.args...)

			if _, ok := tt.expected.(error); ok && tt.err.Error() == "custom error" {
				assert.Equal(t, tt.err.Error(), result.Error())
			} else {
				assert.IsType(t, tt.expected, result)
			}
		})
	}
}

// TestRealValidateBusinessError tests the actual ValidateBusinessError function with a few common error cases
func TestRealValidateBusinessError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		entityType   string
		args         []interface{}
		expectedType interface{}
	}{
		{
			name:         "entity not found error",
			err:          constant.ErrEntityNotFound,
			entityType:   "User",
			args:         []interface{}{},
			expectedType: EntityNotFoundError{},
		},
		{
			name:         "unmodifiable field error",
			err:          constant.ErrUnmodifiableField,
			entityType:   "Transaction",
			args:         []interface{}{},
			expectedType: ValidationError{},
		},
		{
			name:         "duplicate ledger error",
			err:          constant.ErrDuplicateLedger,
			entityType:   "Ledger",
			args:         []interface{}{"TestLedger", "TestDivision"},
			expectedType: EntityConflictError{},
		},
		{
			name:         "unknown error",
			err:          errors.New("unknown_error"),
			entityType:   "Custom",
			args:         []interface{}{},
			expectedType: errors.New(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateBusinessError(tt.err, tt.entityType, tt.args...)

			// For unknown errors, we expect the original error to be returned
			if tt.err.Error() == "unknown_error" {
				assert.Equal(t, tt.err, result)
				return
			}

			// For known errors, check the type
			switch expected := tt.expectedType.(type) {
			case EntityNotFoundError:
				actual, ok := result.(EntityNotFoundError)
				assert.True(t, ok, "Expected EntityNotFoundError")
				assert.Equal(t, tt.entityType, actual.EntityType)
				assert.NotEmpty(t, actual.Title)
				assert.NotEmpty(t, actual.Message)
			case ValidationError:
				actual, ok := result.(ValidationError)
				assert.True(t, ok, "Expected ValidationError")
				assert.Equal(t, tt.entityType, actual.EntityType)
				assert.NotEmpty(t, actual.Title)
				assert.NotEmpty(t, actual.Message)
			case EntityConflictError:
				actual, ok := result.(EntityConflictError)
				assert.True(t, ok, "Expected EntityConflictError")
				assert.Equal(t, tt.entityType, actual.EntityType)
				assert.NotEmpty(t, actual.Title)
				assert.NotEmpty(t, actual.Message)
			default:
				assert.Equal(t, expected, result)
			}
		})
	}
}

// \1 performs an operation
func TestValidateInternalError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		entityType string
	}{
		{
			name:       "with error",
			err:        errors.New("internal error"),
			entityType: "User",
		},
		{
			name:       "with nil error",
			err:        nil,
			entityType: "User",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateInternalError(tt.err, tt.entityType)

			// Check that we got a non-nil result
			assert.NotNil(t, result)

			// Check that it's the right type
			internalErr, ok := result.(InternalServerError)
			assert.True(t, ok)

			// Check the fields
			assert.Equal(t, tt.entityType, internalErr.EntityType)
			assert.Equal(t, "Internal Server Error", internalErr.Title)
			assert.NotEmpty(t, internalErr.Message)

			// Check if Err is nil or not nil as expected
			if tt.err == nil {
				assert.Nil(t, internalErr.Err)
			} else {
				assert.NotNil(t, internalErr.Err)
				assert.Equal(t, tt.err.Error(), internalErr.Err.Error())
			}
		})
	}
}
