package metricskit

import (
	"strings"
	"sync"
)

// ErrorCategory represents a classification of error types.
type ErrorCategory string

const (
	ErrorCategoryTimeout     ErrorCategory = "timeout"
	ErrorCategoryConnection  ErrorCategory = "connection"
	ErrorCategoryRefused     ErrorCategory = "refused"
	ErrorCategoryReset       ErrorCategory = "reset"
	ErrorCategoryDNS         ErrorCategory = "dns"
	ErrorCategoryTLS         ErrorCategory = "tls"
	ErrorCategoryServerError ErrorCategory = "server_error"
	ErrorCategoryCanceled    ErrorCategory = "canceled"
	ErrorCategoryUnknown     ErrorCategory = "unknown"
)

// ErrorClassifier categorizes error messages into standard categories.
// Thread-safe for concurrent use.
type ErrorClassifier struct {
	mu       sync.Mutex
	counts   map[ErrorCategory]int
	patterns map[ErrorCategory][]string
}

// NewErrorClassifier creates an ErrorClassifier with default patterns.
func NewErrorClassifier() *ErrorClassifier {
	return &ErrorClassifier{
		counts: make(map[ErrorCategory]int),
		patterns: map[ErrorCategory][]string{
			ErrorCategoryTimeout: {
				"timeout", "deadline exceeded", "context deadline",
				"i/o timeout", "read timeout", "write timeout",
			},
			ErrorCategoryConnection: {
				"connection refused", "no such host", "network is unreachable",
				"connection reset", "broken pipe", "connection closed",
			},
			ErrorCategoryRefused: {
				"connection refused", "actively refused",
			},
			ErrorCategoryReset: {
				"connection reset", "reset by peer", "broken pipe",
			},
			ErrorCategoryDNS: {
				"no such host", "dns", "lookup failed", "name resolution",
			},
			ErrorCategoryTLS: {
				"tls", "certificate", "x509", "ssl",
			},
			ErrorCategoryServerError: {
				"500", "502", "503", "504", "internal server error",
				"bad gateway", "service unavailable", "gateway timeout",
			},
			ErrorCategoryCanceled: {
				"context canceled", "request canceled", "operation was canceled",
			},
		},
	}
}

// RecordError classifies and records an error message.
func (ec *ErrorClassifier) RecordError(errMsg string) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	category := ec.classify(errMsg)
	ec.counts[category]++
}

// classify determines the category for an error message (must hold lock).
func (ec *ErrorClassifier) classify(errMsg string) ErrorCategory {
	lower := strings.ToLower(errMsg)

	// Check patterns in priority order
	priorityOrder := []ErrorCategory{
		ErrorCategoryTimeout,
		ErrorCategoryRefused,
		ErrorCategoryReset,
		ErrorCategoryDNS,
		ErrorCategoryTLS,
		ErrorCategoryConnection,
		ErrorCategoryServerError,
		ErrorCategoryCanceled,
	}

	for _, category := range priorityOrder {
		for _, pattern := range ec.patterns[category] {
			if strings.Contains(lower, pattern) {
				return category
			}
		}
	}

	return ErrorCategoryUnknown
}

// GetCategoryCounts returns a copy of error counts by category.
func (ec *ErrorClassifier) GetCategoryCounts() map[ErrorCategory]int {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	result := make(map[ErrorCategory]int, len(ec.counts))
	for k, v := range ec.counts {
		result[k] = v
	}

	return result
}

// Clone creates a deep copy of the ErrorClassifier.
func (ec *ErrorClassifier) Clone() *ErrorClassifier {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	counts := make(map[ErrorCategory]int, len(ec.counts))
	for k, v := range ec.counts {
		counts[k] = v
	}

	patterns := make(map[ErrorCategory][]string, len(ec.patterns))
	for k, v := range ec.patterns {
		patternsCopy := make([]string, len(v))
		copy(patternsCopy, v)
		patterns[k] = patternsCopy
	}

	return &ErrorClassifier{
		counts:   counts,
		patterns: patterns,
	}
}

// Reset clears all recorded error counts.
func (ec *ErrorClassifier) Reset() {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.counts = make(map[ErrorCategory]int)
}

// AddPattern adds a custom pattern for a category.
func (ec *ErrorClassifier) AddPattern(category ErrorCategory, pattern string) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.patterns[category] = append(ec.patterns[category], strings.ToLower(pattern))
}
