// Package pkg provides shared error types and error handling utilities for the Midaz ledger system.
// This file defines structured error types that map to HTTP status codes and business error codes,
// providing consistent error handling across all services.
package pkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

// Error Type Definitions
//
// This file defines a hierarchy of error types that correspond to different HTTP status codes
// and business scenarios. Each error type includes:
//   - EntityType: The type of entity involved (e.g., "Account", "Transaction")
//   - Title: A human-readable error title
//   - Message: A detailed error message for the client
//   - Code: A unique error code from pkg/constant/errors.go
//   - Err: The underlying error (for error wrapping)
//
// Error Type to HTTP Status Code Mapping:
//   - EntityNotFoundError       -> 404 Not Found
//   - ValidationError           -> 400 Bad Request
//   - EntityConflictError       -> 409 Conflict
//   - UnauthorizedError         -> 401 Unauthorized
//   - ForbiddenError            -> 403 Forbidden
//   - UnprocessableOperationError -> 422 Unprocessable Entity
//   - FailedPreconditionError   -> 412 Precondition Failed
//   - InternalServerError       -> 500 Internal Server Error
//   - HTTPError                 -> Variable (depends on HTTP client error)

// EntityNotFoundError indicates that a requested entity does not exist in the system.
//
// This error is used when:
//   - A database query returns no results for a given ID
//   - A cache lookup fails to find a cached entity
//   - A referenced entity (e.g., parent account, ledger) does not exist
//
// HTTP Status Code: 404 Not Found
//
// Example Usage:
//
//	if account == nil {
//	    return pkg.EntityNotFoundError{
//	        EntityType: "Account",
//	        Code:       constant.ErrAccountIDNotFound.Error(),
//	        Title:      "Account Not Found",
//	        Message:    "The provided account ID does not exist in our records.",
//	    }
//	}
type EntityNotFoundError struct {
	EntityType string `json:"entityType,omitempty"` // Type of entity that was not found (e.g., "Account", "Ledger")
	Title      string `json:"title,omitempty"`      // Human-readable error title
	Message    string `json:"message,omitempty"`    // Detailed error message for the client
	Code       string `json:"code,omitempty"`       // Unique error code from constant package
	Err        error  `json:"err,omitempty"`        // Underlying error for error wrapping
}

// Error implements the error interface for EntityNotFoundError.
//
// The error message is constructed with the following priority:
// 1. If Message is set, return Message
// 2. If EntityType is set, return "Entity {EntityType} not found"
// 3. If Err is set, return Err.Error()
// 4. Otherwise, return generic "entity not found"
//
// Returns:
//   - A string representation of the error
func (e EntityNotFoundError) Error() string {
	if strings.TrimSpace(e.Message) == "" {
		if strings.TrimSpace(e.EntityType) != "" {
			return fmt.Sprintf("Entity %s not found", e.EntityType)
		}

		if e.Err != nil && strings.TrimSpace(e.Message) == "" {
			return e.Err.Error()
		}

		return "entity not found"
	}

	return e.Message
}

// Unwrap implements the error unwrapping interface introduced in Go 1.13.
//
// This allows the use of errors.Is() and errors.As() to check for wrapped errors.
//
// Returns:
//   - The underlying error, or nil if no error is wrapped
func (e EntityNotFoundError) Unwrap() error {
	return e.Err
}

// ValidationError indicates that input validation has failed.
//
// This error is used when:
//   - Required fields are missing
//   - Field values don't meet format requirements
//   - Business rules are violated
//   - Data constraints are not satisfied
//
// HTTP Status Code: 400 Bad Request
//
// Example Usage:
//
//	if !isValidEmail(email) {
//	    return pkg.ValidationError{
//	        EntityType: "Account",
//	        Code:       constant.ErrBadRequest.Error(),
//	        Title:      "Invalid Email Format",
//	        Message:    "The email address provided is not in a valid format.",
//	    }
//	}
type ValidationError struct {
	EntityType string `json:"entityType,omitempty"` // Type of entity being validated
	Title      string `json:"title,omitempty"`      // Human-readable error title
	Message    string `json:"message,omitempty"`    // Detailed error message for the client
	Code       string `json:"code,omitempty"`       // Unique error code from constant package
	Err        error  `json:"err,omitempty"`        // Underlying error for error wrapping
}

// Error implements the error interface for ValidationError.
//
// If a Code is present, the error message includes both the code and message.
// Otherwise, only the message is returned.
//
// Returns:
//   - A string representation of the error
func (e ValidationError) Error() string {
	if strings.TrimSpace(e.Code) != "" {
		return fmt.Sprintf("%s - %s", e.Code, e.Message)
	}

	return e.Message
}

// Unwrap implements the error unwrapping interface for ValidationError.
//
// Returns:
//   - The underlying error, or nil if no error is wrapped
func (e ValidationError) Unwrap() error {
	return e.Err
}

// EntityConflictError indicates that an entity already exists and cannot be created again.
//
// This error is used when:
//   - Attempting to create an entity with a duplicate unique identifier
//   - Violating unique constraints (e.g., duplicate account alias, ledger name)
//   - Database unique constraint violations
//
// HTTP Status Code: 409 Conflict
//
// Example Usage:
//
//	if existingAccount != nil {
//	    return pkg.EntityConflictError{
//	        EntityType: "Account",
//	        Code:       constant.ErrAliasUnavailability.Error(),
//	        Title:      "Alias Unavailability",
//	        Message:    fmt.Sprintf("The alias %s is already in use.", alias),
//	    }
//	}
type EntityConflictError struct {
	EntityType string `json:"entityType,omitempty"` // Type of entity that conflicts
	Title      string `json:"title,omitempty"`      // Human-readable error title
	Message    string `json:"message,omitempty"`    // Detailed error message for the client
	Code       string `json:"code,omitempty"`       // Unique error code from constant package
	Err        error  `json:"err,omitempty"`        // Underlying error for error wrapping
}

// Error implements the error interface for EntityConflictError.
//
// Returns:
//   - The error message, or the underlying error's message if Message is empty
func (e EntityConflictError) Error() string {
	if e.Err != nil && strings.TrimSpace(e.Message) == "" {
		return e.Err.Error()
	}

	return e.Message
}

// Unwrap implements the error unwrapping interface for EntityConflictError.
//
// Returns:
//   - The underlying error, or nil if no error is wrapped
func (e EntityConflictError) Unwrap() error {
	return e.Err
}

// UnauthorizedError indicates that authentication is required but was not provided.
//
// This error is used when:
//   - No authentication token is present in the request
//   - The authentication token is invalid or expired
//   - The token cannot be validated
//
// HTTP Status Code: 401 Unauthorized
//
// Example Usage:
//
//	if token == "" {
//	    return pkg.UnauthorizedError{
//	        Code:    constant.ErrTokenMissing.Error(),
//	        Title:   "Token Missing",
//	        Message: "A valid token must be provided in the request header.",
//	    }
//	}
type UnauthorizedError struct {
	EntityType string `json:"entityType,omitempty"` // Type of entity requiring authentication
	Title      string `json:"title,omitempty"`      // Human-readable error title
	Message    string `json:"message,omitempty"`    // Detailed error message for the client
	Code       string `json:"code,omitempty"`       // Unique error code from constant package
	Err        error  `json:"err,omitempty"`        // Underlying error for error wrapping
}

// Error implements the error interface for UnauthorizedError.
//
// Returns:
//   - The error message
func (e UnauthorizedError) Error() string {
	return e.Message
}

// ForbiddenError indicates that the authenticated user lacks sufficient privileges.
//
// This error is used when:
//   - The user is authenticated but doesn't have required permissions
//   - Role-based access control denies the operation
//   - Resource-level permissions prevent the action
//
// HTTP Status Code: 403 Forbidden
//
// Example Usage:
//
//	if !hasPermission(user, "delete:account") {
//	    return pkg.ForbiddenError{
//	        Code:    constant.ErrInsufficientPrivileges.Error(),
//	        Title:   "Insufficient Privileges",
//	        Message: "You do not have the necessary permissions to perform this action.",
//	    }
//	}
type ForbiddenError struct {
	EntityType string `json:"entityType,omitempty"` // Type of entity being accessed
	Title      string `json:"title,omitempty"`      // Human-readable error title
	Message    string `json:"message,omitempty"`    // Detailed error message for the client
	Code       string `json:"code,omitempty"`       // Unique error code from constant package
	Err        error  `json:"err,omitempty"`        // Underlying error for error wrapping
}

// Error implements the error interface for ForbiddenError.
//
// Returns:
//   - The error message
func (e ForbiddenError) Error() string {
	return e.Message
}

// UnprocessableOperationError indicates that the operation is semantically invalid.
//
// This error is used when:
//   - The request is well-formed but semantically incorrect
//   - Business logic prevents the operation (e.g., insufficient funds)
//   - The operation violates business rules
//
// HTTP Status Code: 422 Unprocessable Entity
//
// Example Usage:
//
//	if balance < amount {
//	    return pkg.UnprocessableOperationError{
//	        Code:    constant.ErrInsufficientFunds.Error(),
//	        Title:   "Insufficient Funds",
//	        Message: "The transaction could not be completed due to insufficient funds.",
//	    }
//	}
type UnprocessableOperationError struct {
	EntityType string `json:"entityType,omitempty"` // Type of entity involved in the operation
	Title      string `json:"title,omitempty"`      // Human-readable error title
	Message    string `json:"message,omitempty"`    // Detailed error message for the client
	Code       string `json:"code,omitempty"`       // Unique error code from constant package
	Err        error  `json:"err,omitempty"`        // Underlying error for error wrapping
}

// Error implements the error interface for UnprocessableOperationError.
//
// Returns:
//   - The error message
func (e UnprocessableOperationError) Error() string {
	return e.Message
}

// HTTPError indicates an error occurred during an HTTP client request.
//
// This error is used when:
//   - HTTP client requests fail
//   - External API calls return errors
//   - Network issues occur during HTTP communication
//
// HTTP Status Code: Variable (depends on the HTTP client error)
//
// Example Usage:
//
//	resp, err := http.Get(url)
//	if err != nil {
//	    return pkg.HTTPError{
//	        Code:    constant.ErrInternalServer.Error(),
//	        Title:   "HTTP Request Failed",
//	        Message: "Failed to communicate with external service.",
//	        Err:     err,
//	    }
//	}
type HTTPError struct {
	EntityType string `json:"entityType,omitempty"` // Type of entity involved in the HTTP request
	Title      string `json:"title,omitempty"`      // Human-readable error title
	Message    string `json:"message,omitempty"`    // Detailed error message for the client
	Code       string `json:"code,omitempty"`       // Unique error code from constant package
	Err        error  `json:"err,omitempty"`        // Underlying error for error wrapping
}

// Error implements the error interface for HTTPError.
//
// Returns:
//   - The error message
func (e HTTPError) Error() string {
	return e.Message
}

// FailedPreconditionError indicates that a required precondition was not met.
//
// This error is used when:
//   - System configuration is incomplete or invalid
//   - Required services are not available
//   - Preconditions for an operation are not satisfied
//
// HTTP Status Code: 412 Precondition Failed
//
// Example Usage:
//
//	if enforcer == nil {
//	    return pkg.FailedPreconditionError{
//	        Code:    constant.ErrPermissionEnforcement.Error(),
//	        Title:   "Permission Enforcement Error",
//	        Message: "The enforcer is not configured properly.",
//	    }
//	}
type FailedPreconditionError struct {
	EntityType string `json:"entityType,omitempty"` // Type of entity involved in the operation
	Title      string `json:"title,omitempty"`      // Human-readable error title
	Message    string `json:"message,omitempty"`    // Detailed error message for the client
	Code       string `json:"code,omitempty"`       // Unique error code from constant package
	Err        error  `json:"err,omitempty"`        // Underlying error for error wrapping
}

// Error implements the error interface for FailedPreconditionError.
//
// Returns:
//   - The error message
func (e FailedPreconditionError) Error() string {
	return e.Message
}

// InternalServerError indicates an unexpected server-side error.
//
// This error is used when:
//   - Unexpected exceptions occur
//   - System failures happen
//   - Database connections fail unexpectedly
//   - Message broker is unavailable
//
// HTTP Status Code: 500 Internal Server Error
//
// Example Usage:
//
//	if err := database.Connect(); err != nil {
//	    return pkg.InternalServerError{
//	        Code:    constant.ErrInternalServer.Error(),
//	        Title:   "Internal Server Error",
//	        Message: "The server encountered an unexpected error.",
//	        Err:     err,
//	    }
//	}
type InternalServerError struct {
	EntityType string `json:"entityType,omitempty"` // Type of entity involved when error occurred
	Title      string `json:"title,omitempty"`      // Human-readable error title
	Message    string `json:"message,omitempty"`    // Detailed error message for the client
	Code       string `json:"code,omitempty"`       // Unique error code from constant package
	Err        error  `json:"err,omitempty"`        // Underlying error for error wrapping
}

// Error implements the error interface for InternalServerError.
//
// Returns:
//   - The error message
func (e InternalServerError) Error() string {
	return e.Message
}

// ResponseError is a generic error structure used to return errors to API clients.
//
// This is the base error type that can be serialized to JSON for HTTP responses.
// Other error types are typically converted to ResponseError before being sent to clients.
//
// Example Usage:
//
//	return pkg.ResponseError{
//	    Code:    "0047",
//	    Title:   "Bad Request",
//	    Message: "The request could not be processed.",
//	}
type ResponseError struct {
	EntityType string `json:"entityType,omitempty"` // Type of entity involved in the error
	Title      string `json:"title,omitempty"`      // Human-readable error title
	Message    string `json:"message,omitempty"`    // Detailed error message for the client
	Code       string `json:"code,omitempty"`       // Unique error code from constant package
	Err        error  `json:"err,omitempty"`        // Underlying error (not serialized to JSON)
}

// Error implements the error interface for ResponseError.
//
// Returns:
//   - The error message
func (r ResponseError) Error() string {
	return r.Message
}

// ValidationKnownFieldsError indicates validation failures for known/expected fields.
//
// This error is used when:
//   - Required fields are missing from the request
//   - Known fields have invalid values
//   - Field-specific validation rules are violated
//
// The Fields map contains field names as keys and validation error messages as values.
//
// HTTP Status Code: 400 Bad Request
//
// Example Usage:
//
//	return pkg.ValidationKnownFieldsError{
//	    Code:    constant.ErrMissingFieldsInRequest.Error(),
//	    Title:   "Missing Fields in Request",
//	    Message: "Your request is missing required fields.",
//	    Fields: pkg.FieldValidations{
//	        "email": "Email is required",
//	        "name":  "Name must be at least 3 characters",
//	    },
//	}
type ValidationKnownFieldsError struct {
	EntityType string           `json:"entityType,omitempty"` // Type of entity being validated
	Title      string           `json:"title,omitempty"`      // Human-readable error title
	Message    string           `json:"message,omitempty"`    // Detailed error message for the client
	Code       string           `json:"code,omitempty"`       // Unique error code from constant package
	Err        error            `json:"err,omitempty"`        // Underlying error for error wrapping
	Fields     FieldValidations `json:"fields,omitempty"`     // Map of field names to validation errors
}

// Error implements the error interface for ValidationKnownFieldsError.
//
// Returns:
//   - The error message
func (r ValidationKnownFieldsError) Error() string {
	return r.Message
}

// FieldValidations is a map of field names to their validation error messages.
//
// Example:
//
//	fields := pkg.FieldValidations{
//	    "email": "Invalid email format",
//	    "age":   "Must be a positive integer",
//	}
type FieldValidations map[string]string

// ValidationUnknownFieldsError indicates that the request contains unexpected fields.
//
// This error is used when:
//   - The request body contains fields not defined in the API schema
//   - Extra fields are present that should not be there
//   - The client is sending more data than expected
//
// The Fields map contains the unexpected field names as keys and their values.
//
// HTTP Status Code: 400 Bad Request
//
// Example Usage:
//
//	return pkg.ValidationUnknownFieldsError{
//	    Code:    constant.ErrUnexpectedFieldsInTheRequest.Error(),
//	    Title:   "Unexpected Fields in Request",
//	    Message: "The request contains fields that are not expected.",
//	    Fields: pkg.UnknownFields{
//	        "extra_field": "some value",
//	        "another_field": 123,
//	    },
//	}
type ValidationUnknownFieldsError struct {
	EntityType string        `json:"entityType,omitempty"` // Type of entity being validated
	Title      string        `json:"title,omitempty"`      // Human-readable error title
	Message    string        `json:"message,omitempty"`    // Detailed error message for the client
	Code       string        `json:"code,omitempty"`       // Unique error code from constant package
	Err        error         `json:"err,omitempty"`        // Underlying error for error wrapping
	Fields     UnknownFields `json:"fields,omitempty"`     // Map of unexpected field names to their values
}

// Error implements the error interface for ValidationUnknownFieldsError.
//
// Returns:
//   - The error message
func (r ValidationUnknownFieldsError) Error() string {
	return r.Message
}

// UnknownFields is a map of unexpected field names to their values.
//
// Example:
//
//	fields := pkg.UnknownFields{
//	    "unexpected_field": "value",
//	    "another_field":    42,
//	}
type UnknownFields map[string]any

// Error Validation and Creation Functions
//
// The following functions provide utilities for creating and validating errors
// with consistent formatting and appropriate error codes.

// ValidateInternalError creates an InternalServerError from a generic error.
//
// This function wraps unexpected errors (e.g., database failures, panic recovery)
// into a structured InternalServerError that can be safely returned to clients.
// The underlying error is preserved for logging and debugging purposes.
//
// Parameters:
//   - err: The underlying error that caused the internal server error
//   - entityType: The type of entity being processed when the error occurred
//
// Returns:
//   - An InternalServerError with code 0046, generic user-facing message, and wrapped error
//
// Example Usage:
//
//	result, err := database.Query(...)
//	if err != nil {
//	    return nil, pkg.ValidateInternalError(err, "Account")
//	}
func ValidateInternalError(err error, entityType string) error {
	return InternalServerError{
		EntityType: entityType,
		Code:       constant.ErrInternalServer.Error(),
		Title:      "Internal Server Error",
		Message:    "The server encountered an unexpected error. Please try again later or contact support.",
		Err:        err,
	}
}

// ValidateUnmarshallingError creates a ResponseError from JSON unmarshalling failures.
//
// This function handles JSON parsing errors and provides user-friendly error messages.
// It specifically detects json.UnmarshalTypeError to provide detailed information about
// type mismatches (e.g., "expected string but got number").
//
// Parameters:
//   - err: The unmarshalling error from json.Unmarshal
//
// Returns:
//   - A ResponseError with code 0094 and a descriptive message about the parsing failure
//
// Example Usage:
//
//	var request CreateAccountRequest
//	if err := json.Unmarshal(body, &request); err != nil {
//	    return pkg.ValidateUnmarshallingError(err)
//	}
func ValidateUnmarshallingError(err error) error {
	message := err.Error()

	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) {
		field := ute.Field
		expected := ute.Type.String()
		actual := ute.Value
		message = fmt.Sprintf("invalid value for field '%s': expected type '%s', but got '%s'", field, expected, actual)
	}

	return ResponseError{
		Code:    constant.ErrInvalidRequestBody.Error(),
		Title:   "Unmarshalling error",
		Message: message,
	}
}

// ValidateBadRequestFieldsError creates appropriate validation errors based on field issues.
//
// This function analyzes different types of field validation failures and returns the most
// appropriate error type with detailed field-level information. It prioritizes error types
// in the following order:
// 1. Unknown fields (fields not in the API schema)
// 2. Missing required fields
// 3. Invalid known fields (format/validation errors)
//
// Parameters:
//   - requiredFields: Map of missing required field names to error messages
//   - knownInvalidFields: Map of invalid known field names to validation error messages
//   - entityType: The type of entity being validated (e.g., "Account", "Transaction")
//   - unknownFields: Map of unexpected field names to their values
//
// Returns:
//   - ValidationUnknownFieldsError if unknown fields are present (code 0053)
//   - ValidationKnownFieldsError if required fields are missing (code 0009)
//   - ValidationKnownFieldsError if known fields are invalid (code 0047)
//   - Generic error if all maps are empty (should not happen in normal usage)
//
// Example Usage:
//
//	requiredFields := map[string]string{
//	    "name": "Name is required",
//	}
//	knownInvalidFields := map[string]string{
//	    "email": "Invalid email format",
//	}
//	unknownFields := map[string]any{
//	    "extra_field": "value",
//	}
//	return pkg.ValidateBadRequestFieldsError(requiredFields, knownInvalidFields, "Account", unknownFields)
func ValidateBadRequestFieldsError(requiredFields, knownInvalidFields map[string]string, entityType string, unknownFields map[string]any) error {
	if len(unknownFields) == 0 && len(knownInvalidFields) == 0 && len(requiredFields) == 0 {
		return errors.New("expected knownInvalidFields, unknownFields and requiredFields to be non-empty")
	}

	if len(unknownFields) > 0 {
		return ValidationUnknownFieldsError{
			EntityType: entityType,
			Code:       constant.ErrUnexpectedFieldsInTheRequest.Error(),
			Title:      "Unexpected Fields in the Request",
			Message:    "The request body contains more fields than expected. Please send only the allowed fields as per the documentation. The unexpected fields are listed in the fields object.",
			Fields:     unknownFields,
		}
	}

	if len(requiredFields) > 0 {
		return ValidationKnownFieldsError{
			EntityType: entityType,
			Code:       constant.ErrMissingFieldsInRequest.Error(),
			Title:      "Missing Fields in Request",
			Message:    "Your request is missing one or more required fields. Please refer to the documentation to ensure all necessary fields are included in your request.",
			Fields:     requiredFields,
		}
	}

	return ValidationKnownFieldsError{
		EntityType: entityType,
		Code:       constant.ErrBadRequest.Error(),
		Title:      "Bad Request",
		Message:    "The server could not understand the request due to malformed syntax. Please check the listed fields and try again.",
		Fields:     knownInvalidFields,
	}
}

// ValidateBusinessError maps business error codes to structured error types with user-friendly messages.
//
// This function is the central error mapping function in Midaz. It takes error codes from
// the constant package and converts them into appropriate error types (EntityNotFoundError,
// ValidationError, EntityConflictError, etc.) with:
//   - User-friendly titles and messages
//   - Appropriate HTTP status codes (via error type)
//   - Support for message formatting with args
//
// The function maintains a comprehensive map of all 124+ error codes to their corresponding
// error types and messages. If an error code is not found in the map, the original error
// is returned unchanged.
//
// Parameters:
//   - err: The error code from pkg/constant/errors.go (e.g., constant.ErrAccountIDNotFound)
//   - entityType: The type of entity related to the error (e.g., "Account", "Transaction")
//   - args: Optional arguments for fmt.Sprintf formatting in error messages
//
// Returns:
//   - A structured error type (EntityNotFoundError, ValidationError, etc.) with appropriate
//     code, title, and message, or the original error if not found in the map
//
// Example Usage:
//
//	if account == nil {
//	    return pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, "Account")
//	}
//
//	// With formatting arguments
//	return pkg.ValidateBusinessError(constant.ErrAliasUnavailability, "Account", aliasName)
//
// Error Type Mapping:
//   - EntityNotFoundError: For "not found" errors (404)
//   - ValidationError: For validation and constraint errors (400)
//   - EntityConflictError: For duplicate/conflict errors (409)
//   - UnauthorizedError: For authentication errors (401)
//   - ForbiddenError: For authorization errors (403)
//   - UnprocessableOperationError: For business logic errors (422)
//   - FailedPreconditionError: For precondition failures (412)
//   - InternalServerError: For system errors (500)
func ValidateBusinessError(err error, entityType string, args ...any) error {
	errorMap := map[error]error{
		constant.ErrDuplicateLedger: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicateLedger.Error(),
			Title:      "Duplicate Ledger Error",
			Message:    fmt.Sprintf("A ledger with the name %v already exists in the division %v. Please rename the ledger or choose a different division to attach it to.", args...),
		},
		constant.ErrLedgerNameConflict: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrLedgerNameConflict.Error(),
			Title:      "Ledger Name Conflict",
			Message:    fmt.Sprintf("A ledger named %v already exists in your organization. Please rename the ledger, or if you want to use the same name, consider creating a new ledger for a different division.", args...),
		},
		constant.ErrAssetNameOrCodeDuplicate: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrAssetNameOrCodeDuplicate.Error(),
			Title:      "Asset Name or Code Duplicate",
			Message:    "An asset with the same name or code already exists in your ledger. Please modify the name or code of your new asset.",
		},
		constant.ErrCodeUppercaseRequirement: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCodeUppercaseRequirement.Error(),
			Title:      "Code Uppercase Requirement",
			Message:    "The code must be in uppercase. Please ensure that the code is in uppercase format and try again.",
		},
		constant.ErrCurrencyCodeStandardCompliance: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCurrencyCodeStandardCompliance.Error(),
			Title:      "Currency Code Standard Compliance",
			Message:    "Currency-type assets must comply with the ISO-4217 standard. Please use a currency code that conforms to ISO-4217 guidelines.",
		},
		constant.ErrUnmodifiableField: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrUnmodifiableField.Error(),
			Title:      "Unmodifiable Field Error",
			Message:    "Your request includes a field that cannot be modified. Please review your request and try again, removing any uneditable fields. Please refer to the documentation for guidance.",
		},
		constant.ErrEntityNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrEntityNotFound.Error(),
			Title:      "Entity Not Found",
			Message:    "No entity was found for the given ID. Please make sure to use the correct ID for the entity you are trying to manage.",
		},
		constant.ErrActionNotPermitted: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrActionNotPermitted.Error(),
			Title:      "Action Not Permitted",
			Message:    "The action you are attempting is not allowed in the current environment. Please refer to the documentation for guidance.",
		},
		constant.ErrAccountTypeImmutable: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountTypeImmutable.Error(),
			Title:      "Account Type Immutable",
			Message:    "The account type specified cannot be modified. Please ensure the correct account type is being used and try again.",
		},
		constant.ErrInactiveAccountType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInactiveAccountType.Error(),
			Title:      "Inactive Account Type Error",
			Message:    "The account type specified cannot be set to INACTIVE. Please ensure the correct account type is being used and try again.",
		},
		constant.ErrAccountBalanceDeletion: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountBalanceDeletion.Error(),
			Title:      "Account Balance Deletion Error",
			Message:    "An account or sub-account cannot be deleted if it has a remaining balance. Please ensure all remaining balances are transferred to another account before attempting to delete.",
		},
		constant.ErrResourceAlreadyDeleted: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrResourceAlreadyDeleted.Error(),
			Title:      "Resource Already Deleted",
			Message:    "The resource you are trying to delete has already been deleted. Ensure you are using the correct ID and try again.",
		},
		constant.ErrSegmentIDInactive: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrSegmentIDInactive.Error(),
			Title:      "Segment ID Inactive",
			Message:    "The Segment ID you are attempting to use is inactive. Please use another Segment ID and try again.",
		},
		constant.ErrDuplicateSegmentName: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicateSegmentName.Error(),
			Title:      "Duplicate Segment Name Error",
			Message:    fmt.Sprintf("A segment with the name %v already exists for this ledger ID %v. Please try again with a different ledger or name.", args...),
		},
		constant.ErrBalanceRemainingDeletion: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrBalanceRemainingDeletion.Error(),
			Title:      "Balance Remaining Deletion Error",
			Message:    "The asset cannot be deleted because there is a remaining balance. Please ensure all balances are cleared before attempting to delete again.",
		},
		constant.ErrInvalidScriptFormat: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrInvalidScriptFormat.Error(),
			Title:      "Invalid Script Format Error",
			Message:    "The script provided in your request is invalid or in an unsupported format. Please verify the script format and try again.",
		},
		constant.ErrInsufficientFunds: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrInsufficientFunds.Error(),
			Title:      "Insufficient Funds Error",
			Message:    "The transaction could not be completed due to insufficient funds in the account. Please add sufficient funds to your account and try again.",
		},
		constant.ErrAccountIneligibility: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrAccountIneligibility.Error(),
			Title:      "Account Ineligibility Error",
			Message:    "One or more accounts listed in the transaction are not eligible to participate. Please review the account statuses and try again.",
		},
		constant.ErrAliasUnavailability: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrAliasUnavailability.Error(),
			Title:      "Alias Unavailability Error",
			Message:    fmt.Sprintf("The alias %v is already in use. Please choose a different alias and try again.", args...),
		},
		constant.ErrParentTransactionIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrParentTransactionIDNotFound.Error(),
			Title:      "Parent Transaction ID Not Found",
			Message:    fmt.Sprintf("The parentTransactionId %v does not correspond to any existing transaction. Please review the ID and try again.", args...),
		},
		constant.ErrImmutableField: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrImmutableField.Error(),
			Title:      "Immutable Field Error",
			Message:    fmt.Sprintf("The %v field cannot be modified. Please remove this field from your request and try again.", args...),
		},
		constant.ErrTransactionTimingRestriction: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionTimingRestriction.Error(),
			Title:      "Transaction Timing Restriction",
			Message:    fmt.Sprintf("You can only perform another transaction using %v of %f from %v to %v after %v. Please wait until the specified time to try again.", args...),
		},
		constant.ErrAccountStatusTransactionRestriction: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountStatusTransactionRestriction.Error(),
			Title:      "Account Status Transaction Restriction",
			Message:    "The current statuses of the source and/or destination accounts do not permit transactions. Change the account status(es) and try again.",
		},
		constant.ErrInsufficientAccountBalance: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInsufficientAccountBalance.Error(),
			Title:      "Insufficient Account Balance Error",
			Message:    fmt.Sprintf("The account %v does not have sufficient balance. Please try again with an amount that is less than or equal to the available balance.", args...),
		},
		constant.ErrTransactionMethodRestriction: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionMethodRestriction.Error(),
			Title:      "Transaction Method Restriction",
			Message:    fmt.Sprintf("Transactions involving %v are not permitted for the specified source and/or destination. Please try again using accounts that allow transactions with %v.", args...),
		},
		constant.ErrDuplicateTransactionTemplateCode: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicateTransactionTemplateCode.Error(),
			Title:      "Duplicate Transaction Template Code Error",
			Message:    fmt.Sprintf("A transaction template with the code %v already exists for your ledger. Please use a different code and try again.", args...),
		},
		constant.ErrDuplicateAssetPair: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicateAssetPair.Error(),
			Title:      "Duplicate Asset Pair Error",
			Message:    fmt.Sprintf("A pair for the assets %v%v already exists with the ID %v. Please update the existing entry instead of creating a new one.", args...),
		},
		constant.ErrInvalidParentAccountID: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidParentAccountID.Error(),
			Title:      "Invalid Parent Account ID",
			Message:    "The specified parent account ID does not exist. Please verify the ID is correct and attempt your request again.",
		},
		constant.ErrMismatchedAssetCode: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMismatchedAssetCode.Error(),
			Title:      "Mismatched Asset Code",
			Message:    "The parent account ID you provided is associated with a different asset code than the one specified in your request. Please make sure the asset code matches that of the parent account, or use a different parent account ID and try again.",
		},
		constant.ErrChartTypeNotFound: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrChartTypeNotFound.Error(),
			Title:      "Chart Type Not Found",
			Message:    fmt.Sprintf("The chart type %v does not exist. Please provide a valid chart type and refer to the documentation if you have any questions.", args...),
		},
		constant.ErrInvalidCountryCode: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidCountryCode.Error(),
			Title:      "Invalid Country Code",
			Message:    "The provided country code in the 'address.country' field does not conform to the ISO-3166 alpha-2 standard. Please provide a valid alpha-2 country code.",
		},
		constant.ErrInvalidCodeFormat: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidCodeFormat.Error(),
			Title:      "Invalid Code Format",
			Message:    "The 'code' field must be alphanumeric, in upper case, and must contain at least one letter. Please provide a valid code.",
		},
		constant.ErrAssetCodeNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAssetCodeNotFound.Error(),
			Title:      "Asset Code Not Found",
			Message:    "The provided asset code does not exist in our records. Please verify the asset code and try again.",
		},
		constant.ErrPortfolioIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrPortfolioIDNotFound.Error(),
			Title:      "Portfolio ID Not Found",
			Message:    "The provided portfolio ID does not exist in our records. Please verify the portfolio ID and try again.",
		},
		constant.ErrSegmentIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrSegmentIDNotFound.Error(),
			Title:      "Segment ID Not Found",
			Message:    "The provided segment ID does not exist in our records. Please verify the segment ID and try again.",
		},
		constant.ErrLedgerIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrLedgerIDNotFound.Error(),
			Title:      "Ledger ID Not Found",
			Message:    "The provided ledger ID does not exist in our records. Please verify the ledger ID and try again.",
		},
		constant.ErrOrganizationIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrOrganizationIDNotFound.Error(),
			Title:      "Organization ID Not Found",
			Message:    "The provided organization ID does not exist in our records. Please verify the organization ID and try again.",
		},
		constant.ErrParentOrganizationIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrParentOrganizationIDNotFound.Error(),
			Title:      "Parent Organization ID Not Found",
			Message:    "The provided parent organization ID does not exist in our records. Please verify the parent organization ID and try again.",
		},
		constant.ErrInvalidType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidType.Error(),
			Title:      "Invalid Type",
			Message:    "The provided 'type' is not valid. Accepted types are currency, crypto, commodities, or others. Please provide a valid type.",
		},
		constant.ErrTokenMissing: UnauthorizedError{
			EntityType: entityType,
			Code:       constant.ErrTokenMissing.Error(),
			Title:      "Token Missing",
			Message:    "A valid token must be provided in the request header. Please include a token and try again.",
		},
		constant.ErrInvalidToken: UnauthorizedError{
			EntityType: entityType,
			Code:       constant.ErrInvalidToken.Error(),
			Title:      "Invalid Token",
			Message:    "The provided token is expired, invalid or malformed. Please provide a valid token and try again.",
		},
		constant.ErrInsufficientPrivileges: ForbiddenError{
			EntityType: entityType,
			Code:       constant.ErrInsufficientPrivileges.Error(),
			Title:      "Insufficient Privileges",
			Message:    "You do not have the necessary permissions to perform this action. Please contact your administrator if you believe this is an error.",
		},
		constant.ErrPermissionEnforcement: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrPermissionEnforcement.Error(),
			Title:      "Permission Enforcement Error",
			Message:    "The enforcer is not configured properly. Please contact your administrator if you believe this is an error.",
		},
		constant.ErrJWKFetch: FailedPreconditionError{
			EntityType: entityType,
			Code:       constant.ErrJWKFetch.Error(),
			Title:      "JWK Fetch Error",
			Message:    "The JWK keys could not be fetched from the source. Please verify the source environment variable configuration and try again.",
		},
		constant.ErrInvalidDSLFileFormat: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidDSLFileFormat.Error(),
			Title:      "Invalid DSL File Format",
			Message:    fmt.Sprintf("The submitted DSL file %v is in an incorrect format. Please ensure that the file follows the expected structure and syntax.", args...),
		},
		constant.ErrEmptyDSLFile: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrEmptyDSLFile.Error(),
			Title:      "Empty DSL File",
			Message:    fmt.Sprintf("The submitted DSL file %v is empty. Please provide a valid file with content.", args...),
		},
		constant.ErrMetadataKeyLengthExceeded: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMetadataKeyLengthExceeded.Error(),
			Title:      "Metadata Key Length Exceeded",
			Message:    fmt.Sprintf("The metadata key %v exceeds the maximum allowed length of %v characters. Please use a shorter key.", args...),
		},
		constant.ErrMetadataValueLengthExceeded: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMetadataValueLengthExceeded.Error(),
			Title:      "Metadata Value Length Exceeded",
			Message:    fmt.Sprintf("The metadata value %v exceeds the maximum allowed length of %v characters. Please use a shorter value.", args...),
		},
		constant.ErrAccountIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAccountIDNotFound.Error(),
			Title:      "Account ID Not Found",
			Message:    "The provided account ID does not exist in our records. Please verify the account ID and try again.",
		},
		constant.ErrIDsNotFoundForAccounts: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrIDsNotFoundForAccounts.Error(),
			Title:      "IDs Not Found for Accounts",
			Message:    "No accounts were found for the provided IDs. Please verify the IDs and try again.",
		},
		constant.ErrAssetIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAssetIDNotFound.Error(),
			Title:      "Asset ID Not Found",
			Message:    "The provided asset ID does not exist in our records. Please verify the asset ID and try again.",
		},
		constant.ErrNoAssetsFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoAssetsFound.Error(),
			Title:      "No Assets Found",
			Message:    "No assets were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrNoSegmentsFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoSegmentsFound.Error(),
			Title:      "No Segments Found",
			Message:    "No segments were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrNoPortfoliosFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoPortfoliosFound.Error(),
			Title:      "No Portfolios Found",
			Message:    "No portfolios were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrNoOrganizationsFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoOrganizationsFound.Error(),
			Title:      "No Organizations Found",
			Message:    "No organizations were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrNoLedgersFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoLedgersFound.Error(),
			Title:      "No Ledgers Found",
			Message:    "No ledgers were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrBalanceUpdateFailed: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrBalanceUpdateFailed.Error(),
			Title:      "Balance Update Failed",
			Message:    "The balance could not be updated for the specified account ID. Please verify the account ID and try again.",
		},
		constant.ErrNoAccountIDsProvided: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoAccountIDsProvided.Error(),
			Title:      "No Account IDs Provided",
			Message:    "No account IDs were provided for the balance update. Please provide valid account IDs and try again.",
		},
		constant.ErrFailedToRetrieveAccountsByAliases: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrFailedToRetrieveAccountsByAliases.Error(),
			Title:      "Failed To Retrieve Accounts By Aliases",
			Message:    "The accounts could not be retrieved using the specified aliases. Please verify the aliases for accuracy and try again.",
		},
		constant.ErrNoAccountsFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoAccountsFound.Error(),
			Title:      "No Accounts Found",
			Message:    "No accounts were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrInvalidPathParameter: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidPathParameter.Error(),
			Title:      "Invalid Path Parameter",
			Message:    fmt.Sprintf("One or more path parameters are in an incorrect format. Please check the following parameters %v and ensure they meet the required format before trying again.", args...),
		},
		constant.ErrInvalidAccountType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidAccountType.Error(),
			Title:      "Invalid Account Type",
			Message:    "The provided 'type' is not valid.",
		},
		constant.ErrInvalidMetadataNesting: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidMetadataNesting.Error(),
			Title:      "Invalid Metadata Nesting",
			Message:    fmt.Sprintf("The metadata object cannot contain nested values. Please ensure that the value %v is not nested and try again.", args...),
		},
		constant.ErrOperationIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrOperationIDNotFound.Error(),
			Title:      "Operation ID Not Found",
			Message:    "The provided operation ID does not exist in our records. Please verify the operation ID and try again.",
		},
		constant.ErrNoOperationsFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoOperationsFound.Error(),
			Title:      "No Operations Found",
			Message:    "No operations were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrTransactionIDNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrTransactionIDNotFound.Error(),
			Title:      "Transaction ID Not Found",
			Message:    "The provided transaction ID does not exist in our records. Please verify the transaction ID and try again.",
		},
		constant.ErrNoTransactionsFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoTransactionsFound.Error(),
			Title:      "No Transactions Found",
			Message:    "No transactions were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrInvalidTransactionType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidTransactionType.Error(),
			Title:      "Invalid Transaction Type",
			Message:    fmt.Sprintf("Only one transaction type ('amount', 'share', or 'remaining') must be specified in the '%v' field for each entry. Please review your input and try again.", args...),
		},
		constant.ErrTransactionValueMismatch: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionValueMismatch.Error(),
			Title:      "Transaction Value Mismatch",
			Message:    "The values for the source, the destination, or both do not match the specified transaction amount. Please verify the values and try again.",
		},
		constant.ErrForbiddenExternalAccountManipulation: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrForbiddenExternalAccountManipulation.Error(),
			Title:      "External Account Modification Prohibited",
			Message:    "Accounts of type 'external' cannot be deleted or modified as they are used for traceability with external systems. Please review your request and ensure operations are only performed on internal accounts.",
		},
		constant.ErrAuditRecordNotRetrieved: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAuditRecordNotRetrieved.Error(),
			Title:      "Audit Record Not Retrieved",
			Message:    fmt.Sprintf("The record %v could not be retrieved for audit. Please verify that the submitted data is correct and try again.", args...),
		},
		constant.ErrAuditTreeRecordNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAuditTreeRecordNotFound.Error(),
			Title:      "Audit Tree Record Not Found",
			Message:    fmt.Sprintf("The record %v does not exist in the audit tree. Please ensure the audit tree is available and try again.", args...),
		},
		constant.ErrInvalidDateFormat: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidDateFormat.Error(),
			Title:      "Invalid Date Format Error",
			Message:    "The 'initialDate', 'finalDate', or both are in the incorrect format. Please use the 'yyyy-mm-dd' format and try again.",
		},
		constant.ErrInvalidFinalDate: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidFinalDate.Error(),
			Title:      "Invalid Final Date Error",
			Message:    "The 'finalDate' cannot be earlier than the 'initialDate'. Please verify the dates and try again.",
		},
		constant.ErrDateRangeExceedsLimit: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrDateRangeExceedsLimit.Error(),
			Title:      "Date Range Exceeds Limit Error",
			Message:    fmt.Sprintf("The range between 'initialDate' and 'finalDate' exceeds the permitted limit of %v months. Please adjust the dates and try again.", args...),
		},
		constant.ErrPaginationLimitExceeded: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrPaginationLimitExceeded.Error(),
			Title:      "Pagination Limit Exceeded",
			Message:    fmt.Sprintf("The pagination limit exceeds the maximum allowed of %v items per page. Please verify the limit and try again.", args...),
		},
		constant.ErrInvalidSortOrder: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidSortOrder.Error(),
			Title:      "Invalid Sort Order",
			Message:    "The 'sort_order' field must be 'asc' or 'desc'. Please provide a valid sort order and try again.",
		},
		constant.ErrInvalidQueryParameter: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidQueryParameter.Error(),
			Title:      "Invalid Query Parameter",
			Message:    fmt.Sprintf("One or more query parameters are in an incorrect format. Please check the following parameters '%v' and ensure they meet the required format before trying again.", args...),
		},
		constant.ErrInvalidDateRange: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidDateRange.Error(),
			Title:      "Invalid Date Range Error",
			Message:    "Both 'initialDate' and 'finalDate' fields are required and must be in the 'yyyy-mm-dd' format. Please provide valid dates and try again.",
		},
		constant.ErrIdempotencyKey: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrIdempotencyKey.Error(),
			Title:      "Duplicate Idempotency Key",
			Message:    fmt.Sprintf("The idempotency key %v is already in use. Please provide a unique key and try again.", args...),
		},
		constant.ErrAccountAliasNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAccountAliasNotFound.Error(),
			Title:      "Account Alias Not Found",
			Message:    "The provided account Alias does not exist in our records. Please verify the account Alias and try again.",
		},
		constant.ErrLockVersionAccountBalance: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrLockVersionAccountBalance.Error(),
			Title:      "Race condition detected",
			Message:    "A race condition was detected while processing your request. Please try again",
		},
		constant.ErrTransactionIDHasAlreadyParentTransaction: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionIDHasAlreadyParentTransaction.Error(),
			Title:      "Transaction Revert already exist",
			Message:    "Transaction revert already exists. Please try again.",
		},
		constant.ErrTransactionIDIsAlreadyARevert: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionIDIsAlreadyARevert.Error(),
			Title:      "Transaction is already a reversal",
			Message:    "Transaction is already a reversal. Please try again",
		},
		constant.ErrTransactionCantRevert: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionCantRevert.Error(),
			Title:      "Transaction can't be reverted",
			Message:    "Transaction can't be reverted. Please try again",
		},
		constant.ErrTransactionAmbiguous: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionAmbiguous.Error(),
			Title:      "Transaction ambiguous account",
			Message:    "Transaction can't use the same account in sources and destinations",
		},
		constant.ErrBalancesCantDeleted: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrBalancesCantDeleted.Error(),
			Title:      "Balance cannot be deleted",
			Message:    "Balance cannot be deleted because it still has funds in it.",
		},
		constant.ErrParentIDSameID: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrParentIDSameID.Error(),
			Title:      "ID cannot be used as the parent ID",
			Message:    "The provided ID cannot be used as the parent ID. Please choose a different one.",
		},
		constant.ErrMessageBrokerUnavailable: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrMessageBrokerUnavailable.Error(),
			Title:      "Message Broker Unavailable",
			Message:    "The server encountered an unexpected error while connecting to Message Broker. Please try again later or contact support.",
		},
		constant.ErrAccountAliasInvalid: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrAccountAliasInvalid.Error(),
			Title:      "Invalid Account Alias",
			Message:    "The alias contains invalid characters. Please verify the alias value and try again.",
		},
		constant.ErrOnHoldExternalAccount: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrOnHoldExternalAccount.Error(),
			Title:      "Invalid Pending Transaction",
			Message:    "External accounts cannot be used for pending transactions in source operations. Please check the accounts and try again.",
		},
		constant.ErrCommitTransactionNotPending: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrCommitTransactionNotPending.Error(),
			Title:      "Invalid Transaction Status",
			Message:    "The transaction status does not allow the requested action. Please check the transaction status.",
		},
		constant.ErrOverFlowInt64: InternalServerError{
			EntityType: entityType,
			Code:       constant.ErrOverFlowInt64.Error(),
			Title:      "Overflow Error",
			Message:    "The request could not be completed due to an overflow. Please check the values, and try again.",
		},
		constant.ErrOperationRouteTitleAlreadyExists: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrOperationRouteTitleAlreadyExists.Error(),
			Title:      "Operation Route Title Already Exists",
			Message:    "The 'title' provided already exists for the 'type' provided. Please redefine the operation route title.",
		},
		constant.ErrOperationRouteNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrOperationRouteNotFound.Error(),
			Title:      "Operation Route Not Found",
			Message:    "The provided operation route does not exist in our records. Please verify the operation route and try again.",
		},
		constant.ErrNoOperationRoutesFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoOperationRoutesFound.Error(),
			Title:      "No Operation Routes Found",
			Message:    "No operation routes were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrInvalidOperationRouteType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidOperationRouteType.Error(),
			Title:      "Invalid Operation Route Type",
			Message:    "The provided 'type' is not valid. Accepted types are 'debit' or 'credit'. Please provide a valid type.",
		},
		constant.ErrMissingOperationRoutes: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrMissingOperationRoutes.Error(),
			Title:      "Missing Operation Routes in Request",
			Message:    "Your request must include at least one operation route of each type (debit and credit). Please refer to the documentation to ensure these fields are properly populated.",
		},
		constant.ErrTransactionRouteNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrTransactionRouteNotFound.Error(),
			Title:      "Transaction Route Not Found",
			Message:    "The provided transaction route does not exist in our records. Please verify the transaction route and try again.",
		},
		constant.ErrNoTransactionRoutesFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoTransactionRoutesFound.Error(),
			Title:      "No Transaction Routes Found",
			Message:    "No transaction routes were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrOperationRouteLinkedToTransactionRoutes: UnprocessableOperationError{
			EntityType: entityType,
			Code:       constant.ErrOperationRouteLinkedToTransactionRoutes.Error(),
			Title:      "Operation Route Linked to Transaction Routes",
			Message:    "The operation route cannot be deleted because it is linked to one or more transaction routes. Please remove the operation route from all transaction routes before attempting to delete it.",
		},
		constant.ErrInvalidAccountRuleType: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidAccountRuleType.Error(),
			Title:      "Invalid Account Rule Type",
			Message:    "The provided 'account.ruleType' is not valid. Accepted types are 'alias' or 'account_type'. Please provide a valid rule type.",
		},
		constant.ErrInvalidAccountRuleValue: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidAccountRuleValue.Error(),
			Title:      "Invalid Account Rule Value",
			Message:    "The provided 'account.validIf' is not valid. Please provide a string for 'alias' or an array of strings for 'account_type'.",
		},
		constant.ErrInvalidAccountingRoute: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidAccountingRoute.Error(),
			Title:      "Invalid Accounting Route",
			Message:    "The transaction does not comply with the defined accounting route rules. Please verify that the transaction matches the expected operation types and account validation rules.",
		},
		constant.ErrTransactionRouteNotInformed: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrTransactionRouteNotInformed.Error(),
			Title:      "Transaction Route Not Informed",
			Message:    "The transaction route is not informed. Please inform the transaction route for this transaction.",
		},
		constant.ErrInvalidTransactionRouteID: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidTransactionRouteID.Error(),
			Title:      "Invalid Transaction Route ID",
			Message:    "The provided transaction route ID is not a valid UUID format. Please provide a valid UUID for the transaction route.",
		},
		constant.ErrAccountingRouteCountMismatch: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountingRouteCountMismatch.Error(),
			Title:      "Accounting Route Count Mismatch",
			Message:    fmt.Sprintf("The operation routes count does not match the transaction route cache. Expected %v source routes and %v destination routes, but found %v source routes and %v destination routes in the transaction route.", args...),
		},
		constant.ErrAccountingRouteNotFound: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountingRouteNotFound.Error(),
			Title:      "Accounting Route Not Found",
			Message:    fmt.Sprintf("The operation route ID '%v' was not found in the transaction route cache for operation '%v'. Please verify the route configuration.", args...),
		},
		constant.ErrAccountingAliasValidationFailed: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountingAliasValidationFailed.Error(),
			Title:      "Accounting Alias Validation Failed",
			Message:    fmt.Sprintf("The operation alias '%v' does not match the expected alias '%v' defined in the accounting route rule.", args...),
		},
		constant.ErrAccountingAccountTypeValidationFailed: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountingAccountTypeValidationFailed.Error(),
			Title:      "Accounting Account Type Validation Failed",
			Message:    fmt.Sprintf("The account type '%v' does not match any of the expected account types %v defined in the accounting route rule.", args...),
		},
		constant.ErrInvalidAccountTypeKeyValue: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidAccountTypeKeyValue.Error(),
			Title:      "Invalid Characters",
			Message:    "The field 'keyValue' contains invalid characters. Use only letters, numbers, underscores and hyphens.",
		},
		constant.ErrDuplicateAccountTypeKeyValue: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicateAccountTypeKeyValue.Error(),
			Title:      "Duplicate Account Type Key Value Error",
			Message:    "An account type with the specified key value already exists for this organization and ledger. Please use a different key value or update the existing account type.",
		},
		constant.ErrAccountTypeNotFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrAccountTypeNotFound.Error(),
			Title:      "Account Type Not Found Error",
			Message:    "The account type you are trying to access does not exist or has been removed.",
		},
		constant.ErrNoAccountTypesFound: EntityNotFoundError{
			EntityType: entityType,
			Code:       constant.ErrNoAccountTypesFound.Error(),
			Title:      "No Account Types Found",
			Message:    "No account types were found in the search. Please review the search criteria and try again.",
		},
		constant.ErrInvalidFutureTransactionDate: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidFutureTransactionDate.Error(),
			Title:      "Invalid Future Date Error",
			Message:    "The 'transactionDate' cannot be a future date. Please provide a valid date.",
		},
		constant.ErrInvalidPendingFutureTransactionDate: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidPendingFutureTransactionDate.Error(),
			Title:      "Invalid Field for Pending Transaction Error",
			Message:    "Pending transactions do not support the 'transactionDate' field. To proceed, please remove it from your request.",
		},
		constant.ErrDuplicatedAliasKeyValue: EntityConflictError{
			EntityType: entityType,
			Code:       constant.ErrDuplicatedAliasKeyValue.Error(),
			Title:      "Duplicated Alias Key Value Error",
			Message:    "An account alias with the specified key value already exists for this organization and ledger. Please use a different key value.",
		},
		constant.ErrAdditionalBalanceNotAllowed: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAdditionalBalanceNotAllowed.Error(),
			Title:      "Additional Balance Creation Not Allowed",
			Message:    "Additional balances are not allowed for external account type.",
		},
	}

	if mappedError, found := errorMap[err]; found {
		return mappedError
	}

	return err
}

// HandleKnownBusinessValidationErrors handles specific known business validation errors.
//
// This function provides special handling for certain business validation errors that
// require specific entity type context. It's primarily used for transaction validation
// errors where the entity type should be "ValidateSendSourceAndDistribute" for clarity.
//
// Currently handles:
//   - ErrTransactionAmbiguous: When a transaction uses the same account in sources and destinations
//   - ErrTransactionValueMismatch: When transaction values don't balance correctly
//
// Parameters:
//   - err: The error to be checked and potentially mapped
//
// Returns:
//   - A structured error if the error matches known patterns, or the original error otherwise
//
// Example Usage:
//
//	if err := validateTransaction(tx); err != nil {
//	    return pkg.HandleKnownBusinessValidationErrors(err)
//	}
//
// Note: This function is typically used in transaction validation logic where specific
// error context is needed. For most cases, use ValidateBusinessError directly.
func HandleKnownBusinessValidationErrors(err error) error {
	switch {
	case err.Error() == constant.ErrTransactionAmbiguous.Error():
		return ValidateBusinessError(constant.ErrTransactionAmbiguous, "ValidateSendSourceAndDistribute")
	case err.Error() == constant.ErrTransactionValueMismatch.Error():
		return ValidateBusinessError(constant.ErrTransactionValueMismatch, "ValidateSendSourceAndDistribute")
	default:
		return err
	}
}
