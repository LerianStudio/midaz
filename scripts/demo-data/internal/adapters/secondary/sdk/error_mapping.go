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
	switch err {
	case entities.ErrEntityNotFound,
		entities.ErrValidationFailed,
		entities.ErrAuthenticationFailed,
		entities.ErrRateLimitExceeded,
		entities.ErrConfigurationInvalid:
		return err
	}

	// Check for HTTP errors
	if httpErr, ok := err.(*HTTPError); ok {
		return mapHTTPError(httpErr)
	}

	// Check for common error patterns in error message
	errMsg := strings.ToLower(err.Error())

	switch {
	case strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "404"):
		return entities.ErrEntityNotFound
	case strings.Contains(errMsg, "validation") || strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "400"):
		return entities.ErrValidationFailed
	case strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "401") || strings.Contains(errMsg, "forbidden") || strings.Contains(errMsg, "403"):
		return entities.ErrAuthenticationFailed
	case strings.Contains(errMsg, "rate limit") || strings.Contains(errMsg, "429"):
		return entities.ErrRateLimitExceeded
	case strings.Contains(errMsg, "timeout"):
		return errors.New("request timeout")
	case strings.Contains(errMsg, "connection"):
		return errors.New("connection error")
	default:
		return err
	}
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
