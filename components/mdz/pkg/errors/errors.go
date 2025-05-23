package errors

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ErrorType represents the type of error
type ErrorType string

const (
	// ErrorTypeNetwork represents network-related errors
	ErrorTypeNetwork ErrorType = "network"
	// ErrorTypeAuth represents authentication errors
	ErrorTypeAuth ErrorType = "auth"
	// ErrorTypeValidation represents validation errors
	ErrorTypeValidation ErrorType = "validation"
	// ErrorTypeNotFound represents resource not found errors
	ErrorTypeNotFound ErrorType = "not_found"
	// ErrorTypeConflict represents conflict errors
	ErrorTypeConflict ErrorType = "conflict"
	// ErrorTypeInternal represents internal server errors
	ErrorTypeInternal ErrorType = "internal"
	// ErrorTypeTimeout represents timeout errors
	ErrorTypeTimeout ErrorType = "timeout"
	// ErrorTypeRateLimit represents rate limit errors
	ErrorTypeRateLimit ErrorType = "rate_limit"
)

// Error represents an enhanced error with additional context
type Error struct {
	Type        ErrorType
	Message     string
	Err         error
	StatusCode  int
	Retryable   bool
	RetryAfter  time.Duration
	Suggestions []string
	Context     map[string]string
}

// Error implements the error interface
func (e *Error) Error() string {
	var parts []string

	if e.Type != "" {
		parts = append(parts, fmt.Sprintf("[%s]", e.Type))
	}

	if e.Message != "" {
		parts = append(parts, e.Message)
	} else if e.Err != nil {
		parts = append(parts, e.Err.Error())
	}

	if len(e.Suggestions) > 0 {
		parts = append(parts, fmt.Sprintf("Suggestions: %s", strings.Join(e.Suggestions, "; ")))
	}

	return strings.Join(parts, " ")
}

// Unwrap returns the underlying error
func (e *Error) Unwrap() error {
	return e.Err
}

// Is checks if the error matches the target
func (e *Error) Is(target error) bool {
	if target == nil {
		return false
	}

	var targetErr *Error
	if errors.As(target, &targetErr) {
		return e.Type == targetErr.Type
	}

	return errors.Is(e.Err, target)
}

// New creates a new enhanced error
func New(errType ErrorType, message string) *Error {
	return &Error{
		Type:    errType,
		Message: message,
	}
}

// Wrap wraps an existing error with enhanced context
func Wrap(err error, errType ErrorType, message string) *Error {
	if err == nil {
		return nil
	}

	// If it's already an enhanced error, preserve the chain
	var enhancedErr *Error
	if errors.As(err, &enhancedErr) {
		enhancedErr.Message = message + ": " + enhancedErr.Message
		return enhancedErr
	}

	return &Error{
		Type:    errType,
		Message: message,
		Err:     err,
	}
}

// WithStatusCode adds HTTP status code to the error
func (e *Error) WithStatusCode(code int) *Error {
	e.StatusCode = code
	return e
}

// WithRetry marks the error as retryable with optional retry delay
func (e *Error) WithRetry(retryAfter time.Duration) *Error {
	e.Retryable = true
	e.RetryAfter = retryAfter

	return e
}

// WithSuggestions adds helpful suggestions for resolving the error
func (e *Error) WithSuggestions(suggestions ...string) *Error {
	e.Suggestions = append(e.Suggestions, suggestions...)
	return e
}

// WithContext adds additional context to the error
func (e *Error) WithContext(key, value string) *Error {
	if e.Context == nil {
		e.Context = make(map[string]string)
	}

	e.Context[key] = value

	return e
}

// FromHTTPResponse creates an error from HTTP response
func FromHTTPResponse(statusCode int, body string) *Error {
	err := &Error{
		StatusCode: statusCode,
		Message:    body,
	}

	switch statusCode {
	case http.StatusUnauthorized:
		err.Type = ErrorTypeAuth
		_ = err.WithSuggestions(
			"Check your authentication credentials",
			"Try running 'mdz login' to authenticate",
		)
	case http.StatusForbidden:
		err.Type = ErrorTypeAuth
		_ = err.WithSuggestions(
			"Check your permissions for this resource",
			"Contact your administrator for access",
		)
	case http.StatusNotFound:
		err.Type = ErrorTypeNotFound
		_ = err.WithSuggestions(
			"Verify the resource ID is correct",
			"Check if the resource exists using 'mdz <resource> list'",
		)
	case http.StatusConflict:
		err.Type = ErrorTypeConflict
		_ = err.WithSuggestions(
			"The resource may already exist",
			"Try using a different identifier",
		)
	case http.StatusTooManyRequests:
		err.Type = ErrorTypeRateLimit
		err.Retryable = true
		err.RetryAfter = 60 * time.Second
		_ = err.WithSuggestions(
			"You've exceeded the rate limit",
			"Wait a moment before retrying",
		)
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		err.Type = ErrorTypeInternal
		err.Retryable = true
		_ = err.WithSuggestions(
			"This appears to be a temporary server issue",
			"Try again in a few moments",
		)
	case http.StatusGatewayTimeout:
		err.Type = ErrorTypeTimeout
		err.Retryable = true
		_ = err.WithSuggestions(
			"The request timed out",
			"Try again with a smaller request or contact support",
		)
	default:
		if statusCode >= 400 && statusCode < 500 {
			err.Type = ErrorTypeValidation
		} else if statusCode >= 500 {
			err.Type = ErrorTypeInternal
			err.Retryable = true
		}
	}

	return err
}

// IsRetryable checks if an error is retryable
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	var enhancedErr *Error
	if errors.As(err, &enhancedErr) {
		return enhancedErr.Retryable
	}

	// Check for common retryable errors
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	return false
}

// GetRetryDelay returns the retry delay for an error
func GetRetryDelay(err error) time.Duration {
	var enhancedErr *Error
	if errors.As(err, &enhancedErr) && enhancedErr.RetryAfter > 0 {
		return enhancedErr.RetryAfter
	}

	// Default exponential backoff
	return 5 * time.Second
}

// GetSuggestions returns suggestions for fixing the error
func GetSuggestions(err error) []string {
	if err == nil {
		return nil
	}

	var enhancedErr *Error
	if errors.As(err, &enhancedErr) {
		return enhancedErr.Suggestions
	}

	return nil
}
