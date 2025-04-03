package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ErrorType defines the type of error
type ErrorType string

const (
	// ErrorTypeAPI represents errors returned by the API
	ErrorTypeAPI ErrorType = "api_error"

	// ErrorTypeValidation represents validation errors
	ErrorTypeValidation ErrorType = "validation_error"

	// ErrorTypeAuthentication represents authentication errors
	ErrorTypeAuthentication ErrorType = "authentication_error"

	// ErrorTypeAuthorization represents authorization errors
	ErrorTypeAuthorization ErrorType = "authorization_error"

	// ErrorTypeEntityNotFound represents entity not found errors
	ErrorTypeEntityNotFound ErrorType = "entity_not_found_error"

	// ErrorTypeEntityConflict represents entity conflict errors
	ErrorTypeEntityConflict ErrorType = "entity_conflict_error"

	// ErrorTypeUnprocessableOperation represents unprocessable operation errors
	ErrorTypeUnprocessableOperation ErrorType = "unprocessable_operation_error"

	// ErrorTypeNetwork represents network-related errors
	ErrorTypeNetwork ErrorType = "network_error"

	// ErrorTypeTimeout represents timeout errors
	ErrorTypeTimeout ErrorType = "timeout_error"

	// ErrorTypeInternal represents internal SDK errors
	ErrorTypeInternal ErrorType = "internal_error"
)

// Error is the base interface for all errors in the SDK
type Error interface {
	error
	Type() ErrorType
}

// BaseError implements the common functionality for all error types
type BaseError struct {
	ErrorType  ErrorType
	Message    string
	Code       string
	EntityType string
	Title      string
}

// Error returns the error message
func (e *BaseError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s - %s", e.Code, e.Message)
	}

	return e.Message
}

// Type returns the error type
func (e *BaseError) Type() ErrorType {
	return e.ErrorType
}

// APIError represents errors returned by the API
type APIError struct {
	BaseError
	StatusCode int
	RequestID  string
	Details    map[string]any
}

// NewAPIError creates a new APIError
func NewAPIError(statusCode int, message string, requestID string, code string, details map[string]any) *APIError {
	return &APIError{
		BaseError: BaseError{
			ErrorType: ErrorTypeAPI,
			Message:   message,
			Code:      code,
		},
		StatusCode: statusCode,
		RequestID:  requestID,
		Details:    details,
	}
}

// Error returns a formatted error message
func (e *APIError) Error() string {
	baseError := e.BaseError.Error()

	if e.RequestID != "" {
		return fmt.Sprintf("API error (status: %d, request_id: %s): %s", e.StatusCode, e.RequestID, baseError)
	}

	return fmt.Sprintf("API error (status: %d): %s", e.StatusCode, baseError)
}

// ValidationError represents validation errors
type ValidationError struct {
	BaseError
	Fields  map[string]string
	Details map[string]any
}

// NewValidationError creates a new ValidationError
func NewValidationError(message string, code string, entityType string, fields map[string]string, details map[string]any) *ValidationError {
	return &ValidationError{
		BaseError: BaseError{
			ErrorType:  ErrorTypeValidation,
			Message:    message,
			Code:       code,
			EntityType: entityType,
			Title:      "Validation Error",
		},
		Fields:  fields,
		Details: details,
	}
}

// Error returns a formatted error message
func (e *ValidationError) Error() string {
	baseError := e.BaseError.Error()

	if len(e.Fields) > 0 {
		return fmt.Sprintf("Validation error: %s (Invalid fields: %v)", baseError, e.Fields)
	}

	return fmt.Sprintf("Validation error: %s", baseError)
}

// EntityNotFoundError represents entity not found errors
type EntityNotFoundError struct {
	BaseError
}

// NewEntityNotFoundError creates a new EntityNotFoundError
func NewEntityNotFoundError(message string, code string, entityType string) *EntityNotFoundError {
	title := "Entity Not Found"

	if entityType != "" {
		title = fmt.Sprintf("%s Not Found", entityType)
	}

	return &EntityNotFoundError{
		BaseError: BaseError{
			ErrorType:  ErrorTypeEntityNotFound,
			Message:    message,
			Code:       code,
			EntityType: entityType,
			Title:      title,
		},
	}
}

// EntityConflictError represents entity conflict errors
type EntityConflictError struct {
	BaseError
}

// NewEntityConflictError creates a new EntityConflictError
func NewEntityConflictError(message string, code string, entityType string) *EntityConflictError {
	title := "Entity Conflict"

	if entityType != "" {
		title = fmt.Sprintf("%s Conflict", entityType)
	}

	return &EntityConflictError{
		BaseError: BaseError{
			ErrorType:  ErrorTypeEntityConflict,
			Message:    message,
			Code:       code,
			EntityType: entityType,
			Title:      title,
		},
	}
}

// AuthenticationError represents authentication errors
type AuthenticationError struct {
	BaseError
}

// NewAuthenticationError creates a new AuthenticationError
func NewAuthenticationError(message string, code string) *AuthenticationError {
	return &AuthenticationError{
		BaseError: BaseError{
			ErrorType: ErrorTypeAuthentication,
			Message:   message,
			Code:      code,
			Title:     "Authentication Error",
		},
	}
}

// AuthorizationError represents authorization errors
type AuthorizationError struct {
	BaseError
}

// NewAuthorizationError creates a new AuthorizationError
func NewAuthorizationError(message string, code string) *AuthorizationError {
	return &AuthorizationError{
		BaseError: BaseError{
			ErrorType: ErrorTypeAuthorization,
			Message:   message,
			Code:      code,
			Title:     "Authorization Error",
		},
	}
}

// UnprocessableOperationError represents unprocessable operation errors
type UnprocessableOperationError struct {
	BaseError
}

// NewUnprocessableOperationError creates a new UnprocessableOperationError
func NewUnprocessableOperationError(message string, code string, entityType string) *UnprocessableOperationError {
	return &UnprocessableOperationError{
		BaseError: BaseError{
			ErrorType:  ErrorTypeUnprocessableOperation,
			Message:    message,
			Code:       code,
			EntityType: entityType,
			Title:      "Unprocessable Operation",
		},
	}
}

// NetworkError represents network-related errors
type NetworkError struct {
	BaseError
	Err error
}

// NewNetworkError creates a new NetworkError
func NewNetworkError(err error) *NetworkError {
	return &NetworkError{
		BaseError: BaseError{
			ErrorType: ErrorTypeNetwork,
			Message:   fmt.Sprintf("Network error: %s", err.Error()),
			Title:     "Network Error",
		},
		Err: err,
	}
}

// TimeoutError represents timeout errors
type TimeoutError struct {
	BaseError
	Duration string
}

// NewTimeoutError creates a new TimeoutError
func NewTimeoutError(duration string) *TimeoutError {
	return &TimeoutError{
		BaseError: BaseError{
			ErrorType: ErrorTypeTimeout,
			Message:   fmt.Sprintf("Request timed out after %s", duration),
			Title:     "Timeout Error",
		},
		Duration: duration,
	}
}

// InternalError represents internal SDK errors
type InternalError struct {
	BaseError
	Err error
}

// NewInternalError creates a new InternalError
func NewInternalError(err error) *InternalError {
	return &InternalError{
		BaseError: BaseError{
			ErrorType: ErrorTypeInternal,
			Message:   fmt.Sprintf("Internal error: %s", err.Error()),
			Title:     "Internal Error",
		},
		Err: err,
	}
}

// APIErrorResponse represents the error response from the API
type APIErrorResponse struct {
	EntityType string            `json:"entityType,omitempty"`
	Title      string            `json:"title,omitempty"`
	Message    string            `json:"message,omitempty"`
	Code       string            `json:"code,omitempty"`
	Fields     map[string]string `json:"fields,omitempty"`
}

// ErrorFromResponse creates an appropriate error based on the HTTP response
func ErrorFromResponse(resp *http.Response, respBody []byte) Error {
	statusCode := resp.StatusCode
	requestID := resp.Header.Get("X-Request-Id")

	// Try to parse the response as a structured error
	var errorResp APIErrorResponse
	err := json.Unmarshal(respBody, &errorResp)

	// If we can't parse it as a structured error, use the raw body as the message
	message := string(respBody)

	if err == nil && errorResp.Message != "" {
		message = errorResp.Message
	}

	// If we still don't have a message, use the HTTP status text
	if message == "" {
		message = http.StatusText(statusCode)
	}

	// Map status codes to appropriate error types
	switch statusCode {
	case http.StatusUnauthorized:
		return NewAuthenticationError(message, errorResp.Code)

	case http.StatusForbidden:
		return NewAuthorizationError(message, errorResp.Code)

	case http.StatusNotFound:
		return NewEntityNotFoundError(message, errorResp.Code, errorResp.EntityType)

	case http.StatusConflict:
		return NewEntityConflictError(message, errorResp.Code, errorResp.EntityType)

	case http.StatusBadRequest:
		return NewValidationError(message, errorResp.Code, errorResp.EntityType, errorResp.Fields, nil)

	case http.StatusUnprocessableEntity:
		return NewUnprocessableOperationError(message, errorResp.Code, errorResp.EntityType)

	default:
		return NewAPIError(statusCode, message, requestID, errorResp.Code, nil)
	}
}
