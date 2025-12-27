// Package helpers provides test utilities for the Midaz integration test suite.
package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	// dlqSuffix is the suffix for Dead Letter Queue names
	dlqSuffix = ".dlq"

	// defaultDLQWaitTimeout is the default timeout for waiting for DLQ processing
	defaultDLQWaitTimeout = 2 * time.Minute

	// dlqPollInterval is how often to check DLQ message count
	dlqPollInterval = 5 * time.Second

	// httpClientTimeout is the timeout for HTTP requests to RabbitMQ Management API
	httpClientTimeout = 10 * time.Second

	// maxQueueNameLength is the maximum allowed length for queue names
	maxQueueNameLength = 255
)

var (
	// ErrUnexpectedStatusCode indicates RabbitMQ Management API returned a non-OK status
	ErrUnexpectedStatusCode = errors.New("unexpected status code from RabbitMQ Management API")

	// ErrDLQNotEmpty indicates DLQ still has messages after timeout
	ErrDLQNotEmpty = errors.New("DLQ not empty after timeout")

	// ErrInvalidQueueName indicates the queue name contains invalid characters.
	ErrInvalidQueueName = errors.New("invalid queue name: contains disallowed characters")

	// ErrQueueNameEmpty indicates an empty queue name was provided.
	ErrQueueNameEmpty = errors.New("queue name cannot be empty")

	// ErrQueueNameTooLong indicates the queue name exceeds maximum length.
	ErrQueueNameTooLong = errors.New("queue name too long")

	// ErrContextCancelledDLQ indicates the context was cancelled during DLQ operations.
	ErrContextCancelledDLQ = errors.New("context cancelled while waiting for DLQ")

	// ErrQueueValidationFailed indicates queue name validation failed.
	ErrQueueValidationFailed = errors.New("queue name validation failed")

	// queueNamePattern validates queue names to prevent URL path injection.
	// Only allows alphanumeric characters, hyphens, underscores, and dots.
	queueNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
)

// dlqQueueTypeMap maps DLQ queue name patterns to their type for explicit classification.
// Using a map with exact suffixes is more reliable than substring matching.
var dlqQueueTypeMap = map[string]string{
	"balance_create.dlq":  "balance_create",
	"balance-create.dlq":  "balance_create",
	"transaction_ops.dlq": "transaction_ops",
	"transaction-ops.dlq": "transaction_ops",
	"operations.dlq":      "transaction_ops",
}

// classifyDLQQueue returns the queue type for a DLQ queue name.
// Falls back to pattern matching if not in explicit map.
func classifyDLQQueue(queueName string) string {
	// Check explicit mapping first
	if queueType, ok := dlqQueueTypeMap[queueName]; ok {
		return queueType
	}

	// Fallback to pattern matching for unknown queues
	switch {
	case strings.Contains(queueName, "balance") && strings.Contains(queueName, "create"):
		return "balance_create"
	case strings.Contains(queueName, "transaction") || strings.Contains(queueName, "operation"):
		return "transaction_ops"
	default:
		return "unknown"
	}
}

// validateQueueName checks that a queue name is safe for URL construction.
// Returns error if the name contains characters that could cause URL injection.
//
//nolint:wrapcheck // Returns package-defined sentinel errors intentionally
func validateQueueName(queueName string) error {
	if queueName == "" {
		return ErrQueueNameEmpty
	}

	if len(queueName) > maxQueueNameLength {
		return ErrQueueNameTooLong
	}

	if !queueNamePattern.MatchString(queueName) {
		return fmt.Errorf("%w: pattern mismatch", ErrInvalidQueueName)
	}

	// Additional check: reject names that could escape URL path
	if strings.Contains(queueName, "..") || strings.HasPrefix(queueName, "/") {
		return fmt.Errorf("%w: path escape attempt", ErrInvalidQueueName)
	}

	return nil
}

// sleepWithContext waits for the specified duration or until context is cancelled.
// Returns true if the sleep completed, false if context was cancelled.
func sleepWithContext(ctx context.Context, duration time.Duration) bool {
	if duration <= 0 {
		return true
	}

	select {
	case <-ctx.Done():
		return false
	case <-time.After(duration):
		return true
	}
}

// BuildDLQName constructs the DLQ name for a given queue.
func BuildDLQName(queueName string) string {
	return queueName + dlqSuffix
}

// RabbitMQQueueInfo represents queue information from RabbitMQ Management API.
type RabbitMQQueueInfo struct {
	Name          string `json:"name"`
	Messages      int    `json:"messages"`
	MessagesReady int    `json:"messages_ready"`
	Consumers     int    `json:"consumers"`
}

// GetDLQMessageCount queries RabbitMQ Management API for DLQ message count.
// Returns the number of messages in the DLQ, or 0 if the queue doesn't exist.
// Validates queue name to prevent URL path injection attacks.
func GetDLQMessageCount(ctx context.Context, mgmtURL, queueName, user, pass string) (int, error) {
	// Security: Validate queue name to prevent URL path injection
	if err := validateQueueName(queueName); err != nil {
		//nolint:wrapcheck // Wrapping with package-defined sentinel error
		return 0, fmt.Errorf("%w: %w", ErrQueueValidationFailed, err)
	}

	dlqName := BuildDLQName(queueName)
	// URL-encode the queue name to handle special characters safely
	encodedDLQName := url.PathEscape(dlqName)
	apiURL := fmt.Sprintf("%s/api/queues/%%2F/%s", mgmtURL, encodedDLQName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(user, pass)

	client := &http.Client{Timeout: httpClientTimeout}

	resp, err := client.Do(req)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return 0, fmt.Errorf("failed to query RabbitMQ management API: %w", err)
	}
	defer resp.Body.Close()

	// 404 means queue doesn't exist (which is fine - no messages)
	if resp.StatusCode == http.StatusNotFound {
		return 0, nil
	}

	if resp.StatusCode != http.StatusOK {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return 0, fmt.Errorf("%w: %d", ErrUnexpectedStatusCode, resp.StatusCode)
	}

	var queueInfo RabbitMQQueueInfo
	if err := json.NewDecoder(resp.Body).Decode(&queueInfo); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return 0, fmt.Errorf("failed to decode queue info: %w", err)
	}

	return queueInfo.Messages, nil
}

// WaitForDLQEmpty waits until the DLQ has zero messages or timeout.
// This is useful after chaos tests to wait for DLQ consumer to replay all messages.
// TODO(review): Add unit tests with HTTP mocking for GetDLQMessageCount, WaitForDLQEmpty, GetAllDLQCounts - code-reviewer on 2025-12-14
// TODO(review): Consider logging or tracking consecutive errors to fail faster on persistent issues - code-reviewer on 2025-12-14
func WaitForDLQEmpty(ctx context.Context, mgmtURL, queueName, user, pass string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = defaultDLQWaitTimeout
	}

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			//nolint:wrapcheck // Wrapping with package-defined sentinel error
			return fmt.Errorf("%w: %w", ErrContextCancelledDLQ, ctx.Err())
		default:
		}

		count, err := GetDLQMessageCount(ctx, mgmtURL, queueName, user, pass)
		if err != nil {
			// Log but continue - transient errors are expected during chaos
			// Use context-aware sleep to respect graceful shutdown
			if !sleepWithContext(ctx, dlqPollInterval) {
				//nolint:wrapcheck // Wrapping with package-defined sentinel error
				return fmt.Errorf("%w: %w", ErrContextCancelledDLQ, ctx.Err())
			}

			continue
		}

		if count == 0 {
			return nil
		}

		// Context-aware sleep between poll attempts
		if !sleepWithContext(ctx, dlqPollInterval) {
			//nolint:wrapcheck // Wrapping with package-defined sentinel error
			return fmt.Errorf("%w: %w", ErrContextCancelledDLQ, ctx.Err())
		}
	}

	// Get final count for error message
	finalCount, _ := GetDLQMessageCount(ctx, mgmtURL, queueName, user, pass)

	//nolint:wrapcheck // Error already wrapped with context for test helpers
	return fmt.Errorf("%w: %s still has %d messages after %v", ErrDLQNotEmpty, BuildDLQName(queueName), finalCount, timeout)
}

// DLQCounts holds message counts for all DLQs used in chaos tests.
// NOTE: Using struct fields intentionally for type safety and explicit field access.
// A map[string]int would be more flexible but loses compile-time checking.
// Current approach is acceptable for 2-3 queue types; refactor to map if >5 types needed.
type DLQCounts struct {
	BalanceCreateDLQ  int
	TransactionOpsDLQ int
	TotalDLQMessages  int
}

// GetAllDLQCounts retrieves message counts from all relevant DLQs.
func GetAllDLQCounts(ctx context.Context, mgmtURL, user, pass string, queueNames []string) (*DLQCounts, error) {
	counts := &DLQCounts{}

	for _, queueName := range queueNames {
		count, err := GetDLQMessageCount(ctx, mgmtURL, queueName, user, pass)
		if err != nil {
			//nolint:wrapcheck // Error already wrapped with context for test helpers
			return nil, fmt.Errorf("failed to get DLQ count for %s: %w", queueName, err)
		}

		counts.TotalDLQMessages += count

		// Map to named fields based on queue name using explicit classification
		queueType := classifyDLQQueue(queueName)
		switch queueType {
		case "balance_create":
			counts.BalanceCreateDLQ = count
		case "transaction_ops":
			counts.TransactionOpsDLQ = count
		}
	}

	return counts, nil
}
