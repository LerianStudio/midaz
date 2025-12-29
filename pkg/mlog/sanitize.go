package mlog

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// Constants for bounds checking and sanitization.
const (
	maxHeaderLength    = 256
	maxQueryLength     = 4096 // TODO(review): Add length limit on query string before parsing
	maxPathLength      = 2048
	maxCustomKeys      = 50
	maxCustomKeyLen    = 64
	maxCustomValueLen  = 1024
	maxErrorMessageLen = 500
)

// TODO(review): Export bounds constants (MaxCustomKeys, MaxCustomKeyLen, MaxCustomValueLen) for API discoverability

// sensitiveQueryParams contains query parameter names that should be redacted.
var sensitiveQueryParams = map[string]struct{}{
	"token":         {},
	"api_key":       {},
	"apikey":        {},
	"key":           {},
	"secret":        {},
	"password":      {},
	"passwd":        {},
	"pwd":           {},
	"auth":          {},
	"authorization": {},
	"access_token":  {},
	"refresh_token": {},
	"bearer":        {},
	"credential":    {},
	"credentials":   {},
	// Additional sensitive parameters
	"client_secret": {},
	"private_key":   {},
	"jwt":           {},
	"id_token":      {},
	"session":       {},
	"session_id":    {},
	"sid":           {},
	"otp":           {},
	"code":          {},
	"pin":           {},
	"signature":     {},
	"sig":           {},
	"nonce":         {},
}

// piiPatterns contains regex patterns for detecting PII in error messages.
// Note: UUIDs are intentionally NOT included as entity IDs are needed for debugging.
var piiPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`), // Email
	regexp.MustCompile(`\b\d{3}[-.]?\d{3}[-.]?\d{4}\b`),                       // Phone number
	regexp.MustCompile(`\b\d{3}[-]?\d{2}[-]?\d{4}\b`),                         // SSN
	regexp.MustCompile(`\b\d{13,16}\b`),                                       // Credit card
}

// structuralPatterns contains patterns for structural data that should be redacted.
var structuralPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(postgres|postgresql|mysql|mongodb|redis|amqp|rabbitmq)://[^\s]+`), // Connection strings
	regexp.MustCompile(`(/[a-zA-Z0-9_/-]+)+\.(go|json|yaml|yml|env|conf|cfg|key|pem|crt)`),     // File paths with sensitive extensions
	regexp.MustCompile(`(?i)(driver|dsn|connection)[=:]\s*[^\s;]+`),                            // Driver/DSN prefixes
	regexp.MustCompile(`(?i)-----BEGIN[^-]+-----[\s\S]*?-----END[^-]+-----`),                   // PEM blocks
}

// sanitizeQueryParams redacts sensitive query parameters from a query string.
func sanitizeQueryParams(queryString string) string {
	if queryString == "" {
		return ""
	}

	// Truncate extremely long query strings before parsing
	if len(queryString) > maxQueryLength {
		queryString = queryString[:maxQueryLength]
	}

	values, err := url.ParseQuery(queryString)
	if err != nil {
		return "[invalid_query]"
	}

	for key := range values {
		lowerKey := strings.ToLower(key)
		if _, sensitive := sensitiveQueryParams[lowerKey]; sensitive {
			values.Set(key, "[REDACTED]")
		}
	}

	return values.Encode()
}

// sanitizeErrorMessage removes potential PII and structural data from error messages.
func sanitizeErrorMessage(message string) string {
	if message == "" {
		return ""
	}

	// Truncate to max length
	if len(message) > maxErrorMessageLen {
		message = message[:maxErrorMessageLen] + "...[truncated]"
	}

	// Redact PII patterns
	for _, pattern := range piiPatterns {
		message = pattern.ReplaceAllString(message, "[REDACTED]")
	}

	// Redact structural patterns (connection strings, file paths, etc.)
	for _, pattern := range structuralPatterns {
		message = pattern.ReplaceAllString(message, "[REDACTED]")
	}

	return message
}

// sanitizeHeader truncates and sanitizes header values.
func sanitizeHeader(header string) string {
	if header == "" {
		return ""
	}

	if len(header) > maxHeaderLength {
		return header[:maxHeaderLength] + "...[truncated]"
	}

	return header
}

// sanitizePath sanitizes a URL path by removing control characters and limiting length.
func sanitizePath(path string) string {
	if path == "" {
		return ""
	}

	// Remove control characters
	var builder strings.Builder
	builder.Grow(len(path))

	for _, r := range path {
		if !unicode.IsControl(r) {
			builder.WriteRune(r)
		}
	}

	path = builder.String()

	// Truncate to max length
	if len(path) > maxPathLength {
		return path[:maxPathLength] + "...[truncated]"
	}

	return path
}

// anonymizeIP anonymizes an IP address by zeroing the last octet (IPv4)
// or the last 80 bits (IPv6) for privacy compliance.
// Uses net.ParseIP for proper handling of all IP formats.
func anonymizeIP(ip string) string {
	if ip == "" {
		return ""
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "invalid"
	}

	// Check for IPv4 (including IPv4-mapped IPv6)
	if ipv4 := parsed.To4(); ipv4 != nil {
		ipv4[3] = 0
		return ipv4.String()
	}

	// IPv6: zero last 80 bits (last 10 bytes)
	for i := 6; i < 16; i++ {
		parsed[i] = 0
	}

	return parsed.String()
}

// hashIdempotencyKey hashes an idempotency key to prevent analysis of business patterns.
func hashIdempotencyKey(key string) string {
	if key == "" {
		return ""
	}

	hash := sha256.Sum256([]byte(key))
	// Return first 16 characters of hex-encoded hash (64 bits)
	return hex.EncodeToString(hash[:8])
}
