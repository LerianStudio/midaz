package sdk

import (
	"errors"
	"net/http"
	"strings"

	"demo-data/internal/domain/entities"
)

// HTTPError represents an HTTP error response
type HTTPError struct {
	StatusCode int
	Message    string
	Details    string
}

func (e *HTTPError) Error() string {
	return e.Message
}

// MapError maps SDK errors to domain errors
func MapError(err error) error {
	if err == nil {
		return nil
	}

	// Check if it's already a domain error
	if isDomainError(err) {
		return err
	}

	// Check for HTTP errors
	if httpErr, ok := err.(*HTTPError); ok {
		return mapHTTPError(httpErr)
	}

	// Map based on error message patterns
	return mapErrorMessage(err)
}

// isDomainError checks if the error is already a domain error
func isDomainError(err error) bool {
	switch err {
	case entities.ErrEntityNotFound,
		entities.ErrValidationFailed,
		entities.ErrAuthenticationFailed,
		entities.ErrRateLimitExceeded,
		entities.ErrConfigurationInvalid:
		return true
	default:
		return false
	}
}

// mapErrorMessage maps errors based on message content
func mapErrorMessage(err error) error {
	errMsg := strings.ToLower(err.Error())

	if isNotFoundError(errMsg) {
		return entities.ErrEntityNotFound
	}

	if isValidationError(errMsg) {
		return entities.ErrValidationFailed
	}

	if isAuthError(errMsg) {
		return entities.ErrAuthenticationFailed
	}

	if isRateLimitError(errMsg) {
		return entities.ErrRateLimitExceeded
	}

	if isTimeoutError(errMsg) {
		return errors.New("request timeout")
	}

	if isConnectionError(errMsg) {
		return errors.New("connection error")
	}

	return err
}

// Error checking helper functions
func isNotFoundError(msg string) bool {
	return strings.Contains(msg, "not found") || strings.Contains(msg, "404")
}

func isValidationError(msg string) bool {
	return strings.Contains(msg, "validation") ||
		strings.Contains(msg, "invalid") ||
		strings.Contains(msg, "400")
}

func isAuthError(msg string) bool {
	return strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "401") ||
		strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "403")
}

func isRateLimitError(msg string) bool {
	return strings.Contains(msg, "rate limit") || strings.Contains(msg, "429")
}

func isTimeoutError(msg string) bool {
	return strings.Contains(msg, "timeout")
}

func isConnectionError(msg string) bool {
	return strings.Contains(msg, "connection")
}

// mapHTTPError maps HTTP status codes to domain errors
func mapHTTPError(httpErr *HTTPError) error {
	switch httpErr.StatusCode {
	case http.StatusNotFound:
		return entities.ErrEntityNotFound
	case http.StatusBadRequest:
		return entities.ErrValidationFailed
	case http.StatusUnauthorized, http.StatusForbidden:
		return entities.ErrAuthenticationFailed
	case http.StatusTooManyRequests:
		return entities.ErrRateLimitExceeded
	case http.StatusInternalServerError:
		return errors.New("internal server error")
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return errors.New("service unavailable")
	default:
		return httpErr
	}
}

// IsRetryableError checks if an error is retryable
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific retryable errors
	switch err {
	case entities.ErrRateLimitExceeded:
		return true
	}

	// Check for HTTP errors
	if httpErr, ok := err.(*HTTPError); ok {
		return isRetryableHTTPStatus(httpErr.StatusCode)
	}

	// Check error message patterns
	errMsg := strings.ToLower(err.Error())

	return strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "connection") ||
		strings.Contains(errMsg, "temporary") ||
		strings.Contains(errMsg, "rate limit")
}

// isRetryableHTTPStatus checks if an HTTP status code is retryable
func isRetryableHTTPStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
