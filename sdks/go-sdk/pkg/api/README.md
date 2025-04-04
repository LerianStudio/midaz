# API Package

The API package provides error handling utilities specifically for API interactions in the Midaz SDK.

## Usage

Import the package in your Go code:

```go
import "github.com/LerianStudio/midaz/sdks/go-sdk/pkg/api"
```

## Error Types

The package defines a comprehensive error hierarchy for handling different types of API errors:

### Base Error Interface

```go
type Error interface {
    error
    Type() ErrorType
}
```

### Error Types

The package defines several error types for different scenarios:

- `APIError`: Represents errors returned by the API
- `ValidationError`: Represents validation errors
- `EntityNotFoundError`: Represents entity not found errors
- `EntityConflictError`: Represents entity conflict errors
- `AuthenticationError`: Represents authentication errors
- `AuthorizationError`: Represents authorization errors
- `UnprocessableOperationError`: Represents unprocessable operation errors
- `NetworkError`: Represents network-related errors
- `TimeoutError`: Represents timeout errors
- `InternalError`: Represents internal SDK errors

### Error Type Constants

```go
const (
    ErrorTypeAPI                   ErrorType = "api_error"
    ErrorTypeValidation            ErrorType = "validation_error"
    ErrorTypeAuthentication        ErrorType = "authentication_error"
    ErrorTypeAuthorization         ErrorType = "authorization_error"
    ErrorTypeEntityNotFound        ErrorType = "entity_not_found_error"
    ErrorTypeEntityConflict        ErrorType = "entity_conflict_error"
    ErrorTypeUnprocessableOperation ErrorType = "unprocessable_operation_error"
    ErrorTypeNetwork               ErrorType = "network_error"
    ErrorTypeTimeout               ErrorType = "timeout_error"
    ErrorTypeInternal              ErrorType = "internal_error"
)
```

## Creating Errors

The package provides factory functions for creating each type of error:

```go
func NewAPIError(statusCode int, message string, requestID string, code string, details map[string]any) *APIError
func NewValidationError(message string, code string, entityType string, fields map[string]string, details map[string]any) *ValidationError
func NewEntityNotFoundError(message string, code string, entityType string) *EntityNotFoundError
func NewEntityConflictError(message string, code string, entityType string) *EntityConflictError
func NewAuthenticationError(message string, code string) *AuthenticationError
func NewAuthorizationError(message string, code string) *AuthorizationError
func NewUnprocessableOperationError(message string, code string, entityType string) *UnprocessableOperationError
func NewNetworkError(err error) *NetworkError
func NewTimeoutError(duration string) *TimeoutError
func NewInternalError(err error) *InternalError
```

## API Response Handling

### APIErrorResponse

The `APIErrorResponse` type represents the error response from the API:

```go
type APIErrorResponse struct {
    EntityType string            `json:"entityType,omitempty"`
    Title      string            `json:"title,omitempty"`
    Message    string            `json:"message,omitempty"`
    Code       string            `json:"code,omitempty"`
    Fields     map[string]string `json:"fields,omitempty"`
}
```

### ErrorFromResponse

Creates an appropriate error based on the HTTP response:

```go
func ErrorFromResponse(resp *http.Response, respBody []byte) Error
```

This function parses API error responses and creates the appropriate error type based on the HTTP status code and response body.

## Best Practices

1. Use the appropriate error type for each scenario
2. Check error types using the `Type()` method
3. Use `ErrorFromResponse` to handle API responses
4. Include detailed error information when creating errors
5. Handle specific error types in your application code
