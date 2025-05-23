package errors

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		expected string
	}{
		{
			name: "basic error",
			err: &Error{
				Type:    ErrorTypeNetwork,
				Message: "connection failed",
			},
			expected: "[network] connection failed",
		},
		{
			name: "error with suggestions",
			err: &Error{
				Type:        ErrorTypeAuth,
				Message:     "authentication failed",
				Suggestions: []string{"Check credentials", "Try logging in again"},
			},
			expected: "[auth] authentication failed Suggestions: Check credentials; Try logging in again",
		},
		{
			name: "wrapped error",
			err: &Error{
				Type: ErrorTypeInternal,
				Err:  errors.New("underlying error"),
			},
			expected: "[internal] underlying error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestError_Is(t *testing.T) {
	baseErr := errors.New("base error")
	err1 := &Error{Type: ErrorTypeNetwork, Err: baseErr}
	err2 := &Error{Type: ErrorTypeNetwork}
	err3 := &Error{Type: ErrorTypeAuth}

	assert.True(t, err1.Is(err2))
	assert.False(t, err1.Is(err3))
	assert.True(t, errors.Is(err1, baseErr))
}

func TestWrap(t *testing.T) {
	baseErr := errors.New("base error")

	wrapped := Wrap(baseErr, ErrorTypeNetwork, "network operation failed")
	assert.NotNil(t, wrapped)
	assert.Equal(t, ErrorTypeNetwork, wrapped.Type)
	assert.Equal(t, "network operation failed", wrapped.Message)
	assert.Equal(t, baseErr, wrapped.Err)

	// Test nil error
	assert.Nil(t, Wrap(nil, ErrorTypeNetwork, "message"))

	// Test wrapping enhanced error
	doubleWrapped := Wrap(wrapped, ErrorTypeInternal, "internal error")
	assert.Equal(t, "internal error: network operation failed", doubleWrapped.Message)
}

func TestFromHTTPResponse(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		body           string
		expectedType   ErrorType
		expectedRetry  bool
		hasSuggestions bool
	}{
		{
			name:           "unauthorized",
			statusCode:     http.StatusUnauthorized,
			body:           "unauthorized",
			expectedType:   ErrorTypeAuth,
			expectedRetry:  false,
			hasSuggestions: true,
		},
		{
			name:           "not found",
			statusCode:     http.StatusNotFound,
			body:           "resource not found",
			expectedType:   ErrorTypeNotFound,
			expectedRetry:  false,
			hasSuggestions: true,
		},
		{
			name:           "rate limit",
			statusCode:     http.StatusTooManyRequests,
			body:           "rate limit exceeded",
			expectedType:   ErrorTypeRateLimit,
			expectedRetry:  true,
			hasSuggestions: true,
		},
		{
			name:           "server error",
			statusCode:     http.StatusInternalServerError,
			body:           "internal server error",
			expectedType:   ErrorTypeInternal,
			expectedRetry:  true,
			hasSuggestions: true,
		},
		{
			name:           "timeout",
			statusCode:     http.StatusGatewayTimeout,
			body:           "gateway timeout",
			expectedType:   ErrorTypeTimeout,
			expectedRetry:  true,
			hasSuggestions: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FromHTTPResponse(tt.statusCode, tt.body)

			assert.Equal(t, tt.expectedType, err.Type)
			assert.Equal(t, tt.statusCode, err.StatusCode)
			assert.Equal(t, tt.body, err.Message)
			assert.Equal(t, tt.expectedRetry, err.Retryable)

			if tt.hasSuggestions {
				assert.NotEmpty(t, err.Suggestions)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	assert.True(t, IsRetryable(&Error{Retryable: true}))
	assert.False(t, IsRetryable(&Error{Retryable: false}))
	assert.True(t, IsRetryable(context.DeadlineExceeded))
	assert.True(t, IsRetryable(context.Canceled))
	assert.False(t, IsRetryable(errors.New("regular error")))
	assert.False(t, IsRetryable(nil))
}

func TestGetRetryDelay(t *testing.T) {
	// Error with specific retry delay
	err := &Error{RetryAfter: 10 * time.Second}
	assert.Equal(t, 10*time.Second, GetRetryDelay(err))

	// Error without retry delay
	assert.Equal(t, 5*time.Second, GetRetryDelay(errors.New("regular error")))
}

func TestError_Fluent(t *testing.T) {
	err := New(ErrorTypeValidation, "validation failed").
		WithStatusCode(http.StatusBadRequest).
		WithRetry(10*time.Second).
		WithSuggestions("Check input format", "Refer to documentation").
		WithContext("field", "email").
		WithContext("value", "invalid@")

	assert.Equal(t, ErrorTypeValidation, err.Type)
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.True(t, err.Retryable)
	assert.Equal(t, 10*time.Second, err.RetryAfter)
	assert.Len(t, err.Suggestions, 2)
	assert.Equal(t, "email", err.Context["field"])
	assert.Equal(t, "invalid@", err.Context["value"])
}
