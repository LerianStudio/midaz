package mlog

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// sanitizeQueryParams Tests
// =============================================================================

func TestSanitizeQueryParams(t *testing.T) {
	// Note: url.Values.Encode() URL-encodes special characters,
	// so [REDACTED] becomes %5BREDACTED%5D in the output
	redactedEncoded := "%5BREDACTED%5D" // URL-encoded [REDACTED]

	tests := []struct {
		name        string
		input       string
		shouldMatch []string // Substrings that SHOULD be in output
		shouldNot   []string // Substrings that should NOT be in output
	}{
		{
			name:        "redacts token parameter",
			input:       "token=secret123&normal=value",
			shouldMatch: []string{redactedEncoded, "normal=value"},
			shouldNot:   []string{"secret123"},
		},
		{
			name:        "redacts api_key parameter",
			input:       "api_key=xyz789&page=1",
			shouldMatch: []string{redactedEncoded, "page=1"},
			shouldNot:   []string{"xyz789"},
		},
		{
			name:        "redacts apikey (no underscore)",
			input:       "apikey=abc123&limit=10",
			shouldMatch: []string{redactedEncoded, "limit=10"},
			shouldNot:   []string{"abc123"},
		},
		{
			name:        "redacts password parameter",
			input:       "password=hunter2&user=admin",
			shouldMatch: []string{redactedEncoded, "user=admin"},
			shouldNot:   []string{"hunter2"},
		},
		{
			name:        "redacts secret parameter",
			input:       "secret=mysecret&env=prod",
			shouldMatch: []string{redactedEncoded, "env=prod"},
			shouldNot:   []string{"mysecret"},
		},
		{
			name:        "redacts authorization parameter",
			input:       "authorization=Bearer+token123&id=1",
			shouldMatch: []string{redactedEncoded, "id=1"},
			shouldNot:   []string{"Bearer", "token123"},
		},
		{
			name:        "redacts access_token parameter",
			input:       "access_token=at_123&refresh_token=rt_456",
			shouldMatch: []string{redactedEncoded},
			shouldNot:   []string{"at_123", "rt_456"},
		},
		{
			name:        "redacts session_id parameter",
			input:       "session_id=sess_abc&user_id=123",
			shouldMatch: []string{redactedEncoded, "user_id=123"},
			shouldNot:   []string{"sess_abc"},
		},
		{
			name:        "redacts jwt parameter",
			input:       "jwt=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc&foo=bar",
			shouldMatch: []string{redactedEncoded, "foo=bar"},
			shouldNot:   []string{"eyJhbGciOiJIUzI1NiJ9"},
		},
		{
			name:        "redacts client_secret parameter",
			input:       "client_secret=cs_secret&client_id=ci_123",
			shouldMatch: []string{redactedEncoded, "client_id=ci_123"},
			shouldNot:   []string{"cs_secret"},
		},
		{
			name:        "redacts multiple sensitive params",
			input:       "token=abc&secret=xyz&password=123",
			shouldMatch: []string{redactedEncoded},
			shouldNot:   []string{"=abc", "=xyz", "=123"},
		},
		{
			name:        "preserves normal parameters",
			input:       "page=1&limit=10&sort=asc&filter=active",
			shouldMatch: []string{"page=1", "limit=10", "sort=asc", "filter=active"},
			shouldNot:   []string{redactedEncoded},
		},
		{
			name:        "handles empty string",
			input:       "",
			shouldMatch: nil,
			shouldNot:   nil,
		},
		{
			name:        "case insensitive matching - uppercase",
			input:       "TOKEN=secret&API_KEY=xyz",
			shouldMatch: []string{redactedEncoded},
			shouldNot:   []string{"=secret", "=xyz"},
		},
		{
			name:        "case insensitive matching - mixed case",
			input:       "Password=abc&Secret=def",
			shouldMatch: []string{redactedEncoded},
			shouldNot:   []string{"=abc", "=def"},
		},
		{
			name:        "handles URL-encoded values",
			input:       "token=secret%20value&normal=test",
			shouldMatch: []string{redactedEncoded, "normal=test"},
			shouldNot:   []string{"secret+value", "secret%20value"},
		},
		{
			name:        "handles special characters in values",
			input:       "name=value&token=abc+def%3D%3D&page=1",
			shouldMatch: []string{redactedEncoded, "page=1", "name=value"},
			shouldNot:   []string{"abc+def"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeQueryParams(tt.input)

			for _, match := range tt.shouldMatch {
				assert.Contains(t, result, match,
					"Expected output to contain %q, got %q", match, result)
			}

			for _, notMatch := range tt.shouldNot {
				assert.NotContains(t, result, notMatch,
					"Expected output NOT to contain %q, got %q", notMatch, result)
			}
		})
	}
}

func TestSanitizeQueryParams_InvalidQuery(t *testing.T) {
	// Invalid query string should return error indicator
	result := sanitizeQueryParams("%ZZ%invalid%%")
	assert.Equal(t, "[invalid_query]", result)
}

func TestSanitizeQueryParams_LongQuery(t *testing.T) {
	// Create a query string longer than maxQueryLength
	longValue := strings.Repeat("a", maxQueryLength+100)
	input := "key=" + longValue

	result := sanitizeQueryParams(input)

	// Should truncate and still parse
	assert.NotEqual(t, "[invalid_query]", result)
}

// =============================================================================
// sanitizeErrorMessage Tests
// =============================================================================

func TestSanitizeErrorMessage(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldMatch []string // Substrings that SHOULD be in output
		shouldNot   []string // Substrings that should NOT be in output
	}{
		{
			name:        "redacts postgres connection string",
			input:       "pq: connection failed: postgres://user:pass@host:5432/db",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"user:pass", "host:5432"},
		},
		{
			name:        "redacts postgresql connection string",
			input:       "error connecting to postgresql://admin:secret@localhost/mydb",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"admin:secret", "localhost/mydb"},
		},
		{
			name:        "redacts mysql connection string",
			input:       "connection error: mysql://root:password@127.0.0.1:3306/db",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"root:password"},
		},
		{
			name:        "redacts mongodb connection string",
			input:       "failed to connect: mongodb://admin:secret@cluster.mongodb.net/db",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"admin:secret"},
		},
		{
			name:        "redacts redis connection string",
			input:       "redis error: redis://user:pass@redis.host:6379/0",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"user:pass"},
		},
		{
			name:        "redacts amqp/rabbitmq connection string",
			input:       "amqp connection failed: amqp://guest:guest@rabbitmq.local:5672/vhost",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"guest:guest"},
		},
		{
			name:        "redacts file paths with .go extension",
			input:       "failed to open /home/user/app/internal/handler.go",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"/home/user"},
		},
		{
			name:        "redacts file paths with .json extension",
			input:       "cannot read /var/config/secrets.json",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"/var/config"},
		},
		{
			name:        "redacts file paths with .yaml extension",
			input:       "error parsing /etc/app/config.yaml",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"/etc/app"},
		},
		{
			name:        "redacts file paths with .env extension",
			input:       "missing file /app/.env",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"/app/.env"},
		},
		{
			name:        "redacts file paths with .pem extension",
			input:       "certificate error at /etc/ssl/private/key.pem",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"/etc/ssl/private"},
		},
		{
			name:        "redacts email addresses",
			input:       "user not found: john.doe@example.com",
			shouldMatch: []string{"[REDACTED]", "user not found"},
			shouldNot:   []string{"john.doe@example.com"},
		},
		{
			name:        "redacts phone numbers",
			input:       "invalid phone: 123-456-7890",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"123-456-7890"},
		},
		{
			name:        "redacts phone numbers with dots",
			input:       "contact: 123.456.7890",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"123.456.7890"},
		},
		{
			name:        "redacts SSN pattern",
			input:       "SSN validation failed: 123-45-6789",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"123-45-6789"},
		},
		{
			name:        "redacts credit card numbers (16 digits)",
			input:       "invalid card: 4111111111111111",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"4111111111111111"},
		},
		{
			name:        "redacts credit card with hyphen separators",
			input:       "card: 4111-1111-1111-1111",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"4111-1111-1111-1111"},
		},
		{
			name:        "redacts credit card with space separators",
			input:       "card: 4111 1111 1111 1111",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"4111 1111 1111 1111"},
		},
		{
			name:        "redacts phone numbers with spaces",
			input:       "contact: 123 456 7890",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"123 456 7890"},
		},
		{
			name:        "redacts SSN with spaces",
			input:       "SSN: 123 45 6789",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"123 45 6789"},
		},
		{
			name:        "redacts DSN/driver strings",
			input:       "error: driver=postgres dsn=host=localhost user=admin",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"host=localhost"},
		},
		{
			name:        "redacts PEM blocks",
			input:       "key error: -----BEGIN RSA PRIVATE KEY-----\nMIIBOgI...\n-----END RSA PRIVATE KEY-----",
			shouldMatch: []string{"[REDACTED]"},
			shouldNot:   []string{"MIIBOgI"},
		},
		{
			name:        "preserves normal error messages",
			input:       "validation failed: amount must be positive",
			shouldMatch: []string{"validation failed", "amount must be positive"},
			shouldNot:   nil,
		},
		{
			name:        "handles empty message",
			input:       "",
			shouldMatch: nil,
			shouldNot:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeErrorMessage(tt.input)

			for _, match := range tt.shouldMatch {
				assert.Contains(t, result, match,
					"Expected output to contain %q", match)
			}

			for _, notMatch := range tt.shouldNot {
				assert.NotContains(t, result, notMatch,
					"Expected output NOT to contain %q", notMatch)
			}
		})
	}
}

func TestSanitizeErrorMessage_Truncation(t *testing.T) {
	// Create a message longer than maxErrorMessageLen
	longMessage := strings.Repeat("error ", maxErrorMessageLen)

	result := sanitizeErrorMessage(longMessage)

	assert.LessOrEqual(t, len(result), maxErrorMessageLen+20) // Allow for truncation suffix
	assert.Contains(t, result, "[truncated]")
}

func TestSanitizeErrorMessage_MultiplePatterns(t *testing.T) {
	// Message with multiple sensitive patterns
	input := "error at /app/config.json: postgres://user:pass@host:5432/db - contact admin@example.com"

	result := sanitizeErrorMessage(input)

	assert.NotContains(t, result, "/app/config")
	assert.NotContains(t, result, "user:pass")
	assert.NotContains(t, result, "admin@example.com")
	// Should have multiple [REDACTED] markers
	assert.GreaterOrEqual(t, strings.Count(result, "[REDACTED]"), 2)
}

// =============================================================================
// sanitizeHeader Tests
// =============================================================================

func TestSanitizeHeader(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		maxLen   bool // If true, check for truncation
	}{
		{
			name:     "returns normal header unchanged",
			input:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
			expected: "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "preserves short header",
			input:    "application/json",
			expected: "application/json",
		},
		{
			name:   "truncates long header",
			input:  strings.Repeat("a", 500),
			maxLen: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHeader(tt.input)

			if tt.maxLen {
				assert.LessOrEqual(t, len(result), maxHeaderLength+20) // Allow for truncation suffix
				assert.Contains(t, result, "[truncated]")
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestSanitizeHeader_TruncationIncludesSuffix(t *testing.T) {
	longHeader := strings.Repeat("x", 500)
	result := sanitizeHeader(longHeader)

	assert.Contains(t, result, "...[truncated]")
	assert.LessOrEqual(t, len(result), maxHeaderLength+len("...[truncated]"))
}

// =============================================================================
// sanitizePath Tests
// =============================================================================

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "returns normal path unchanged",
			input:    "/v1/organizations/123/ledgers/456",
			expected: "/v1/organizations/123/ledgers/456",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "removes control characters",
			input:    "/path\x00with\x0Dcontrol\x0Achars",
			expected: "/pathwithcontrolchars",
		},
		{
			name:     "removes null byte",
			input:    "/test\x00path",
			expected: "/testpath",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizePath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizePath_Truncation(t *testing.T) {
	longPath := "/" + strings.Repeat("segment/", maxPathLength/8)
	result := sanitizePath(longPath)

	assert.LessOrEqual(t, len(result), maxPathLength+20) // Allow for truncation suffix
	assert.Contains(t, result, "[truncated]")
}

// =============================================================================
// anonymizeIP Tests
// =============================================================================

func TestAnonymizeIP(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// IPv4 tests
		{
			name:     "anonymizes IPv4 address",
			input:    "192.168.1.123",
			expected: "192.168.1.0",
		},
		{
			name:     "anonymizes IPv4 localhost",
			input:    "127.0.0.1",
			expected: "127.0.0.0",
		},
		{
			name:     "anonymizes public IPv4",
			input:    "8.8.8.8",
			expected: "8.8.8.0",
		},
		{
			name:     "anonymizes IPv4 with last octet 255",
			input:    "10.0.0.255",
			expected: "10.0.0.0",
		},
		{
			name:     "anonymizes IPv4 with last octet 0",
			input:    "172.16.0.0",
			expected: "172.16.0.0",
		},

		// IPv6 tests
		{
			name:     "anonymizes full IPv6",
			input:    "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			expected: "2001:db8:85a3::", // Last 80 bits zeroed
		},
		{
			name:     "anonymizes compressed IPv6",
			input:    "2001:db8::1",
			expected: "2001:db8::",
		},
		{
			name:     "anonymizes IPv6 loopback",
			input:    "::1",
			expected: "::",
		},
		{
			name:     "anonymizes IPv6 with zones",
			input:    "fe80::1",
			expected: "fe80::",
		},

		// Edge cases
		{
			name:     "handles invalid IP",
			input:    "not-an-ip",
			expected: "invalid",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "handles malformed IP",
			input:    "192.168.1.256", // Invalid octet
			expected: "invalid",
		},
		{
			name:     "handles partial IP",
			input:    "192.168.1",
			expected: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := anonymizeIP(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAnonymizeIP_IPv4MappedIPv6(t *testing.T) {
	// IPv4-mapped IPv6 addresses should be treated as IPv4
	result := anonymizeIP("::ffff:192.168.1.123")
	// Should anonymize the IPv4 part
	assert.Equal(t, "192.168.1.0", result)
}

func TestAnonymizeIP_PreservesNetworkPrefix(t *testing.T) {
	// Verify that the network prefix is preserved for debugging
	// while the host portion is zeroed for privacy

	ipv4Result := anonymizeIP("203.0.113.42")
	// Should preserve /24 network prefix
	assert.True(t, strings.HasPrefix(ipv4Result, "203.0.113."))
	assert.Equal(t, "203.0.113.0", ipv4Result)

	ipv6Result := anonymizeIP("2001:db8:85a3:1234:5678:9abc:def0:1234")
	// Should preserve /48 network prefix
	assert.True(t, strings.HasPrefix(ipv6Result, "2001:db8:85a3::"))
}

// =============================================================================
// hashIdempotencyKey Tests
// =============================================================================

func TestHashIdempotencyKey(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "hashes normal key",
			input: "user-123-txn-456",
		},
		{
			name:  "hashes UUID-like key",
			input: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:  "hashes compound key",
			input: "org_abc:ledger_def:txn_ghi",
		},
		{
			name:  "hashes key with special characters",
			input: "key!@#$%^&*()_+-=[]{}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hashIdempotencyKey(tt.input)

			// Verify hash properties
			assert.NotEmpty(t, result)
			// Original key should NOT be in result
			assert.NotContains(t, result, tt.input)
			// Hash should be consistent
			result2 := hashIdempotencyKey(tt.input)
			assert.Equal(t, result, result2)
		})
	}
}

func TestHashIdempotencyKey_EmptyInput(t *testing.T) {
	result := hashIdempotencyKey("")
	assert.Empty(t, result)
}

func TestHashIdempotencyKey_Consistency(t *testing.T) {
	// Same input should always produce same output
	key := "test-idempotency-key-12345"

	hash1 := hashIdempotencyKey(key)
	hash2 := hashIdempotencyKey(key)
	hash3 := hashIdempotencyKey(key)

	assert.Equal(t, hash1, hash2)
	assert.Equal(t, hash2, hash3)
}

func TestHashIdempotencyKey_Uniqueness(t *testing.T) {
	// Different inputs should produce different outputs
	keys := []string{
		"key-1",
		"key-2",
		"key-3",
		"KEY-1", // Case sensitive
		"key_1", // Different separator
	}

	hashes := make(map[string]bool)
	for _, key := range keys {
		hash := hashIdempotencyKey(key)
		assert.False(t, hashes[hash],
			"Hash collision detected for key %q", key)
		hashes[hash] = true
	}
}

func TestHashIdempotencyKey_Format(t *testing.T) {
	result := hashIdempotencyKey("test-key")

	// The implementation returns first 16 hex chars (8 bytes = 64 bits)
	// Verify it's a valid hex string
	assert.NotEmpty(t, result)
	for _, c := range result {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"Expected hex character, got %c", c)
	}
}

func TestHashIdempotencyKey_DoesNotExposePIIPatterns(t *testing.T) {
	// Keys that might contain PII-like patterns
	piiKeys := []string{
		"email:john@example.com:order:123",
		"phone:1234567890:txn:456",
		"ssn:123-45-6789:account:789",
	}

	for _, key := range piiKeys {
		result := hashIdempotencyKey(key)
		// None of the PII patterns should be visible in the hash
		assert.NotContains(t, result, "john")
		assert.NotContains(t, result, "example")
		assert.NotContains(t, result, "123456")
	}
}

// =============================================================================
// Bounds Constants Tests
// =============================================================================

func TestBoundsConstants(t *testing.T) {
	// Verify constants are reasonable
	assert.Equal(t, 256, maxHeaderLength)
	assert.Equal(t, 4096, maxQueryLength)
	assert.Equal(t, 2048, maxPathLength)
	assert.Equal(t, 50, maxCustomKeys)
	assert.Equal(t, 64, maxCustomKeyLen)
	assert.Equal(t, 1024, maxCustomValueLen)
	assert.Equal(t, 500, maxErrorMessageLen)
}
