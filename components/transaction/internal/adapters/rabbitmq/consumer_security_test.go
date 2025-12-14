package rabbitmq

import (
	"errors"
	"testing"

	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

// TestSanitizeErrorForDLQ validates that error messages are sanitized
// to prevent information disclosure (CWE-209).
func TestSanitizeErrorForDLQ(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error returns unknown_error",
			err:      nil,
			expected: "unknown_error",
		},
		{
			name:     "stale balance error returns specific category",
			err:      constant.ErrStaleBalanceUpdateSkipped,
			expected: "stale_balance_version_conflict",
		},
		{
			name:     "wrapped stale balance error returns specific category",
			err:      errors.New("failed to update: " + constant.ErrStaleBalanceUpdateSkipped.Error()),
			expected: "processing_error", // wrapped error doesn't match errors.Is
		},
		{
			name:     "connection error returns database_connection_error",
			err:      errors.New("failed to establish connection to database"),
			expected: "database_connection_error",
		},
		{
			name:     "timeout error returns operation_timeout",
			err:      errors.New("context deadline exceeded: timeout waiting for response"),
			expected: "operation_timeout",
		},
		{
			name:     "validation error returns validation_error",
			err:      errors.New("validation failed: amount must be positive"),
			expected: "validation_error",
		},
		{
			name:     "not found error returns resource_not_found",
			err:      errors.New("account not found with ID 12345"),
			expected: "resource_not_found",
		},
		{
			name:     "duplicate error returns duplicate_entry",
			err:      errors.New("duplicate key violation: transaction_id"),
			expected: "duplicate_entry",
		},
		{
			name:     "permission error returns authorization_error",
			err:      errors.New("permission denied for user"),
			expected: "authorization_error",
		},
		{
			name:     "unauthorized error returns authorization_error",
			err:      errors.New("unauthorized access attempt"),
			expected: "authorization_error",
		},
		{
			name:     "generic error returns processing_error",
			err:      errors.New("something went wrong"),
			expected: "processing_error",
		},
		{
			name:     "SQL injection attempt is sanitized",
			err:      errors.New("pq: syntax error at or near \"DROP TABLE users\""),
			expected: "processing_error",
		},
		{
			name:     "internal path leak is sanitized",
			err:      errors.New("open /var/secrets/api_key.json: permission denied"),
			expected: "authorization_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := sanitizeErrorForDLQ(tt.err)
			assert.Equal(t, tt.expected, result, "sanitizeErrorForDLQ should return generic category")
			// Verify no sensitive data in result
			if tt.err != nil {
				assert.NotContains(t, result, tt.err.Error(), "sanitized result should not contain original error")
			}
		})
	}
}

// TestSanitizePanicForDLQ validates that panic values are sanitized
// to prevent information disclosure (CWE-209).
func TestSanitizePanicForDLQ(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		panicValue any
		expected   string
	}{
		{
			name:       "nil panic returns unknown_panic",
			panicValue: nil,
			expected:   "unknown_panic",
		},
		{
			name:       "nil pointer dereference",
			panicValue: "runtime error: invalid memory address or nil pointer dereference",
			expected:   "nil_pointer_dereference",
		},
		{
			name:       "index out of range",
			panicValue: "runtime error: index out of range [5] with length 3",
			expected:   "index_out_of_bounds",
		},
		{
			name:       "slice bounds error",
			panicValue: "runtime error: slice bounds out of range [:10] with capacity 5",
			expected:   "slice_bounds_error",
		},
		{
			name:       "map access error",
			panicValue: "assignment to entry in nil map",
			expected:   "map_access_error",
		},
		{
			name:       "channel operation error",
			panicValue: "send on closed channel",
			expected:   "channel_operation_error",
		},
		{
			name:       "generic runtime error",
			panicValue: "runtime error: some other issue",
			expected:   "runtime_error",
		},
		{
			name:       "unknown panic value",
			panicValue: "unexpected failure",
			expected:   "unhandled_panic",
		},
		{
			name:       "panic with error type",
			panicValue: errors.New("internal error with secrets"),
			expected:   "unhandled_panic",
		},
		{
			name:       "panic with stack trace is sanitized",
			panicValue: "panic: secret_key=abc123\ngoroutine 1 [running]:\nmain.main()\n\t/home/user/secrets/main.go:10",
			expected:   "unhandled_panic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := sanitizePanicForDLQ(tt.panicValue)
			assert.Equal(t, tt.expected, result, "sanitizePanicForDLQ should return generic category")
			// Verify no stack trace or sensitive data in result
			assert.NotContains(t, result, "goroutine", "sanitized result should not contain stack trace")
			assert.NotContains(t, result, "/", "sanitized result should not contain file paths")
		})
	}
}

// TestCopyHeadersSafe validates that only allowlisted headers are copied
// to prevent sensitive data propagation (CWE-200).
func TestCopyHeadersSafe(t *testing.T) {
	t.Parallel()

	t.Run("nil source returns empty table", func(t *testing.T) {
		t.Parallel()

		result := copyHeadersSafe(nil)
		assert.NotNil(t, result, "result should not be nil")
		assert.Empty(t, result, "result should be empty for nil source")
	})

	t.Run("empty source returns empty table", func(t *testing.T) {
		t.Parallel()

		result := copyHeadersSafe(amqp.Table{})
		assert.NotNil(t, result, "result should not be nil")
		assert.Empty(t, result, "result should be empty for empty source")
	})

	t.Run("allowlisted headers are copied", func(t *testing.T) {
		t.Parallel()

		src := amqp.Table{
			"x-correlation-id":    "test-correlation-123",
			"x-midaz-header-id":   "test-header-456",
			"content-type":        "application/json",
			retryCountHeader:      int32(2),
			libConstants.HeaderID: "test-midaz-id",
		}

		result := copyHeadersSafe(src)

		assert.Equal(t, "test-correlation-123", result["x-correlation-id"])
		assert.Equal(t, "test-header-456", result["x-midaz-header-id"])
		assert.Equal(t, "application/json", result["content-type"])
		assert.Equal(t, int32(2), result[retryCountHeader])
		assert.Equal(t, "test-midaz-id", result[libConstants.HeaderID])
		assert.Len(t, result, 5, "all allowlisted headers should be copied")
	})

	t.Run("sensitive headers are filtered out", func(t *testing.T) {
		t.Parallel()

		src := amqp.Table{
			// Allowlisted
			"x-correlation-id": "safe-value",
			// Sensitive - should be filtered
			"authorization":      "Bearer secret-token-12345",
			"x-api-key":          "sk_live_secret_key",
			"x-user-id":          "user_12345",
			"x-session-token":    "session_secret",
			"x-internal-path":    "/var/secrets/config.json",
			"x-database-query":   "SELECT * FROM users WHERE password='secret'",
			"cookie":             "session=abc123",
			"x-forwarded-for":    "192.168.1.100",
			"x-real-ip":          "10.0.0.50",
			"x-custom-sensitive": "internal-only-value",
		}

		result := copyHeadersSafe(src)

		// Verify safe header is present
		assert.Equal(t, "safe-value", result["x-correlation-id"])

		// Verify sensitive headers are NOT present
		assert.NotContains(t, result, "authorization", "authorization header should be filtered")
		assert.NotContains(t, result, "x-api-key", "x-api-key header should be filtered")
		assert.NotContains(t, result, "x-user-id", "x-user-id header should be filtered")
		assert.NotContains(t, result, "x-session-token", "x-session-token header should be filtered")
		assert.NotContains(t, result, "x-internal-path", "x-internal-path header should be filtered")
		assert.NotContains(t, result, "x-database-query", "x-database-query header should be filtered")
		assert.NotContains(t, result, "cookie", "cookie header should be filtered")
		assert.NotContains(t, result, "x-forwarded-for", "x-forwarded-for header should be filtered")
		assert.NotContains(t, result, "x-real-ip", "x-real-ip header should be filtered")
		assert.NotContains(t, result, "x-custom-sensitive", "custom sensitive headers should be filtered")

		assert.Len(t, result, 1, "only allowlisted headers should be present")
	})

	t.Run("mixed headers - only safe ones pass through", func(t *testing.T) {
		t.Parallel()

		src := amqp.Table{
			"x-correlation-id":    "corr-123",
			"content-type":        "application/json",
			retryCountHeader:      int32(1),
			"authorization":       "Bearer token",
			"x-secret":            "should-not-appear",
			libConstants.HeaderID: "midaz-id-789",
		}

		result := copyHeadersSafe(src)

		// Count should be exactly the allowlisted ones
		assert.Len(t, result, 4)
		assert.Contains(t, result, "x-correlation-id")
		assert.Contains(t, result, "content-type")
		assert.Contains(t, result, retryCountHeader)
		assert.Contains(t, result, libConstants.HeaderID)

		// Sensitive should NOT be present
		assert.NotContains(t, result, "authorization")
		assert.NotContains(t, result, "x-secret")
	})

	t.Run("original table is not modified", func(t *testing.T) {
		t.Parallel()

		src := amqp.Table{
			"x-correlation-id": "original-value",
			"authorization":    "secret-token",
		}

		result := copyHeadersSafe(src)

		// Modify the result
		result["x-correlation-id"] = "modified-value"
		result["new-key"] = "new-value"

		// Original should be unchanged
		assert.Equal(t, "original-value", src["x-correlation-id"])
		assert.Equal(t, "secret-token", src["authorization"])
		assert.NotContains(t, src, "new-key")
	})
}

// TestSafeHeadersAllowlist validates the allowlist configuration.
func TestSafeHeadersAllowlist(t *testing.T) {
	t.Parallel()

	t.Run("allowlist contains expected safe headers", func(t *testing.T) {
		t.Parallel()

		expectedHeaders := []string{
			"x-correlation-id",
			"x-midaz-header-id",
			"content-type",
			retryCountHeader,
			libConstants.HeaderID,
		}

		for _, header := range expectedHeaders {
			assert.True(t, safeHeadersAllowlist[header],
				"header %q should be in allowlist", header)
		}
	})

	t.Run("allowlist excludes known sensitive headers", func(t *testing.T) {
		t.Parallel()

		sensitiveHeaders := []string{
			"authorization",
			"cookie",
			"x-api-key",
			"x-session-token",
			"x-forwarded-for",
			"x-real-ip",
		}

		for _, header := range sensitiveHeaders {
			assert.False(t, safeHeadersAllowlist[header],
				"header %q should NOT be in allowlist", header)
		}
	})
}
