package tests

import (
	"errors"
	"testing"

	"demo-data/internal/adapters/secondary/sdk"
	"demo-data/internal/domain/entities"
)

// TestErrorMapping tests the error mapping functionality
func TestErrorMapping(t *testing.T) {
	t.Run("maps domain errors correctly", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    error
			expected error
		}{
			{
				name:     "entity not found",
				input:    entities.ErrEntityNotFound,
				expected: entities.ErrEntityNotFound,
			},
			{
				name:     "validation failed",
				input:    entities.ErrValidationFailed,
				expected: entities.ErrValidationFailed,
			},
			{
				name:     "authentication failed",
				input:    entities.ErrAuthenticationFailed,
				expected: entities.ErrAuthenticationFailed,
			},
			{
				name:     "rate limit exceeded",
				input:    entities.ErrRateLimitExceeded,
				expected: entities.ErrRateLimitExceeded,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := sdk.MapError(tc.input)
				if result != tc.expected {
					t.Errorf("expected %v, got %v", tc.expected, result)
				}
			})
		}
	})

	t.Run("maps HTTP errors correctly", func(t *testing.T) {
		testCases := []struct {
			name       string
			statusCode int
			message    string
			expected   error
		}{
			{
				name:       "404 not found",
				statusCode: 404,
				message:    "Resource not found",
				expected:   entities.ErrEntityNotFound,
			},
			{
				name:       "400 bad request",
				statusCode: 400,
				message:    "Invalid request",
				expected:   entities.ErrValidationFailed,
			},
			{
				name:       "401 unauthorized",
				statusCode: 401,
				message:    "Unauthorized",
				expected:   entities.ErrAuthenticationFailed,
			},
			{
				name:       "403 forbidden",
				statusCode: 403,
				message:    "Forbidden",
				expected:   entities.ErrAuthenticationFailed,
			},
			{
				name:       "429 rate limit",
				statusCode: 429,
				message:    "Too many requests",
				expected:   entities.ErrRateLimitExceeded,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				httpErr := &sdk.HTTPError{
					StatusCode: tc.statusCode,
					Message:    tc.message,
				}

				result := sdk.MapError(httpErr)
				if result != tc.expected {
					t.Errorf("expected %v, got %v", tc.expected, result)
				}
			})
		}
	})

	t.Run("maps error message patterns correctly", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    error
			expected error
		}{
			{
				name:     "not found message",
				input:    errors.New("resource not found"),
				expected: entities.ErrEntityNotFound,
			},
			{
				name:     "validation message",
				input:    errors.New("validation failed"),
				expected: entities.ErrValidationFailed,
			},
			{
				name:     "unauthorized message",
				input:    errors.New("unauthorized access"),
				expected: entities.ErrAuthenticationFailed,
			},
			{
				name:     "rate limit message",
				input:    errors.New("rate limit exceeded"),
				expected: entities.ErrRateLimitExceeded,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := sdk.MapError(tc.input)
				if result != tc.expected {
					t.Errorf("expected %v, got %v", tc.expected, result)
				}
			})
		}
	})

	t.Run("handles nil error", func(t *testing.T) {
		result := sdk.MapError(nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})
}

// TestIsRetryableError tests the retryable error detection
func TestIsRetryableError(t *testing.T) {
	t.Run("identifies retryable errors", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    error
			expected bool
		}{
			{
				name:     "rate limit error",
				input:    entities.ErrRateLimitExceeded,
				expected: true,
			},
			{
				name:     "timeout error",
				input:    errors.New("request timeout"),
				expected: true,
			},
			{
				name:     "connection error",
				input:    errors.New("connection failed"),
				expected: true,
			},
			{
				name:     "temporary error",
				input:    errors.New("temporary failure"),
				expected: true,
			},
			{
				name:     "validation error",
				input:    entities.ErrValidationFailed,
				expected: false,
			},
			{
				name:     "authentication error",
				input:    entities.ErrAuthenticationFailed,
				expected: false,
			},
			{
				name:     "not found error",
				input:    entities.ErrEntityNotFound,
				expected: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := sdk.IsRetryableError(tc.input)
				if result != tc.expected {
					t.Errorf("expected %v, got %v", tc.expected, result)
				}
			})
		}
	})

	t.Run("identifies retryable HTTP errors", func(t *testing.T) {
		testCases := []struct {
			name       string
			statusCode int
			expected   bool
		}{
			{name: "408 timeout", statusCode: 408, expected: true},
			{name: "429 rate limit", statusCode: 429, expected: true},
			{name: "500 internal error", statusCode: 500, expected: true},
			{name: "502 bad gateway", statusCode: 502, expected: true},
			{name: "503 service unavailable", statusCode: 503, expected: true},
			{name: "504 gateway timeout", statusCode: 504, expected: true},
			{name: "400 bad request", statusCode: 400, expected: false},
			{name: "401 unauthorized", statusCode: 401, expected: false},
			{name: "404 not found", statusCode: 404, expected: false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				httpErr := &sdk.HTTPError{
					StatusCode: tc.statusCode,
					Message:    "Test error",
				}

				result := sdk.IsRetryableError(httpErr)
				if result != tc.expected {
					t.Errorf("expected %v, got %v", tc.expected, result)
				}
			})
		}
	})

	t.Run("handles nil error", func(t *testing.T) {
		result := sdk.IsRetryableError(nil)
		if result != false {
			t.Errorf("expected false for nil error, got %v", result)
		}
	})
}
