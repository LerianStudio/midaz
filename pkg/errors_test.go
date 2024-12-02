package pkg

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func TestEntityConflictError_Error(t *testing.T) {
	tests := []struct {
		name     string
		errorObj EntityConflictError
		expected string
	}{
		{
			name: "Err is not nil and Message is empty",
			errorObj: EntityConflictError{
				Err:     errors.New("underlying error"),
				Message: "",
			},
			expected: "underlying error",
		},
		{
			name: "Message is not empty, Err is nil",
			errorObj: EntityConflictError{
				Message: "Conflict occurred",
			},
			expected: "Conflict occurred",
		},
		{
			name: "Message is empty and Err is nil",
			errorObj: EntityConflictError{
				Message: "",
			},
			expected: "",
		},
		{
			name: "Err is nil and Message is whitespace",
			errorObj: EntityConflictError{
				Message: "   ",
			},
			expected: "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errorObj.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateBadRequestFieldsError(t *testing.T) {
	tests := []struct {
		name               string
		requiredFields     map[string]string
		knownInvalidFields map[string]string
		unknownFields      map[string]any
		entityType         string
		expectedError      error
	}{
		{
			name:               "All fields are empty",
			requiredFields:     map[string]string{},
			knownInvalidFields: map[string]string{},
			unknownFields:      map[string]any{},
			entityType:         "Entity1",
			expectedError:      errors.New("expected knownInvalidFields, unknownFields and requiredFields to be non-empty"),
		},
		{
			name:               "Unknown fields present",
			requiredFields:     map[string]string{},
			knownInvalidFields: map[string]string{},
			unknownFields:      map[string]any{"field1": "value1"},
			entityType:         "Entity2",
			expectedError: ValidationUnknownFieldsError{
				EntityType: "Entity2",
				Code:       "0053",
				Title:      "Unexpected Fields in the Request",
				Message:    "The request body contains more fields than expected. Please send only the allowed fields as per the documentation. The unexpected fields are listed in the fields object.",
				Fields:     map[string]any{"field1": "value1"},
			},
		},
		{
			name:               "Required fields missing",
			requiredFields:     map[string]string{"field1": "value1"},
			knownInvalidFields: map[string]string{},
			unknownFields:      map[string]any{},
			entityType:         "Entity3",
			expectedError: ValidationKnownFieldsError{
				EntityType: "Entity3",
				Code:       "0009",
				Title:      "Missing Fields in Request",
				Message:    "Your request is missing one or more required fields. Please refer to the documentation to ensure all necessary fields are included in your request.",
				Fields:     map[string]string{"field1": "value1"},
			},
		},
		{
			name:               "Known invalid fields",
			requiredFields:     map[string]string{},
			knownInvalidFields: map[string]string{"field2": "value2"},
			unknownFields:      map[string]any{},
			entityType:         "Entity4",
			expectedError: ValidationKnownFieldsError{
				EntityType: "Entity4",
				Code:       "0047",
				Title:      "Bad Request",
				Message:    "The server could not understand the request due to malformed syntax. Please check the listed fields and try again.",
				Fields:     map[string]string{"field2": "value2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateBadRequestFieldsError(tt.requiredFields, tt.knownInvalidFields, tt.entityType, tt.unknownFields)
			assert.Equal(t, tt.expectedError, result)
		})
	}
}

func TestValidateBusinessError(t *testing.T) {
	type args struct {
		err        error
		entityType string
		args       []any
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateBusinessError(tt.args.err, tt.args.entityType, tt.args.args...); (err != nil) != tt.wantErr {
				t.Errorf("ValidateBusinessError() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
