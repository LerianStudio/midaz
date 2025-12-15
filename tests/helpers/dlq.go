// File: tests/helpers/dlq.go
package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
)

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
// TODO(review): Add queue name validation to prevent URL path injection (queueName from env vars) - security-reviewer on 2025-12-14
func GetDLQMessageCount(ctx context.Context, mgmtURL, queueName, user, pass string) (int, error) {
	dlqName := BuildDLQName(queueName)
	url := fmt.Sprintf("%s/api/queues/%%2F/%s", mgmtURL, dlqName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(user, pass)

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to query RabbitMQ management API: %w", err)
	}
	defer resp.Body.Close()

	// 404 means queue doesn't exist (which is fine - no messages)
	if resp.StatusCode == http.StatusNotFound {
		return 0, nil
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var queueInfo RabbitMQQueueInfo
	if err := json.NewDecoder(resp.Body).Decode(&queueInfo); err != nil {
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
			return ctx.Err()
		default:
		}

		count, err := GetDLQMessageCount(ctx, mgmtURL, queueName, user, pass)
		if err != nil {
			// Log but continue - transient errors are expected during chaos
			// TODO(review): Use context-aware sleep to respect ctx.Done() - security-reviewer on 2025-12-14
			time.Sleep(dlqPollInterval)
			continue
		}

		if count == 0 {
			return nil
		}

		// TODO(review): Use context-aware sleep to respect ctx.Done() - security-reviewer on 2025-12-14
		time.Sleep(dlqPollInterval)
	}

	// Get final count for error message
	finalCount, _ := GetDLQMessageCount(ctx, mgmtURL, queueName, user, pass)

	return fmt.Errorf("DLQ %s still has %d messages after %v", BuildDLQName(queueName), finalCount, timeout)
}

// DLQCounts holds message counts for all DLQs used in chaos tests.
// TODO(review): Consider using map instead of struct fields if new queue types are added - code-reviewer on 2025-12-14
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
			return nil, fmt.Errorf("failed to get DLQ count for %s: %w", queueName, err)
		}

		counts.TotalDLQMessages += count

		// Map to named fields based on queue name pattern
		switch {
		case strings.Contains(queueName, "balance") && strings.Contains(queueName, "create"):
			counts.BalanceCreateDLQ = count
		case strings.Contains(queueName, "transaction") || strings.Contains(queueName, "operation"):
			counts.TransactionOpsDLQ = count
		}
	}

	return counts, nil
}
