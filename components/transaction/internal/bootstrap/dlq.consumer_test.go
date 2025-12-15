// File: components/transaction/internal/bootstrap/dlq.consumer_test.go
package bootstrap

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDLQConsumerConstants(t *testing.T) {
	t.Parallel()

	t.Run("DLQ consumer constants have expected values", func(t *testing.T) {
		t.Parallel()

		// DLQ retry backoff should be much longer than regular retries
		// because infrastructure should be stable after recovery
		assert.Equal(t, 1*time.Minute, dlqInitialBackoff,
			"dlqInitialBackoff should be 1 minute for first DLQ retry")
		assert.Equal(t, 30*time.Minute, dlqMaxBackoff,
			"dlqMaxBackoff should be 30 minutes for DLQ max retry interval")
		assert.Equal(t, 10, dlqMaxRetries,
			"dlqMaxRetries should be 10 (higher than regular retries since infrastructure should be stable)")
		// M2: healthCheckInterval constant was removed as unused (dlqPollInterval serves the same purpose)
	})
}

func TestDLQQueueNames(t *testing.T) {
	t.Parallel()

	t.Run("DLQ queue names follow naming convention", func(t *testing.T) {
		t.Parallel()

		// Based on existing queue names in rabbitmq.server.go
		// The DLQ names are derived from environment variables + ".dlq" suffix
		assert.Contains(t, dlqQueueSuffix, ".dlq",
			"DLQ suffix should be '.dlq'")
	})
}

func TestDLQConsumer_HealthCheckConstants(t *testing.T) {
	t.Parallel()

	t.Run("health check timeout should be reasonable", func(t *testing.T) {
		t.Parallel()

		// Health check should timeout quickly to not block DLQ processing
		assert.Equal(t, 5*time.Second, healthCheckTimeout,
			"healthCheckTimeout should be 5 seconds")
	})
}

func TestDLQRetryBackoffCalculation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		attempt       int
		expectedDelay time.Duration
	}{
		{
			name:          "zero attempt should return initial backoff",
			attempt:       0,
			expectedDelay: 1 * time.Minute, // dlqInitialBackoff
		},
		{
			name:          "negative attempt should return initial backoff",
			attempt:       -1,
			expectedDelay: 1 * time.Minute, // dlqInitialBackoff
		},
		{
			name:          "first DLQ retry",
			attempt:       1,
			expectedDelay: 1 * time.Minute,
		},
		{
			name:          "second DLQ retry",
			attempt:       2,
			expectedDelay: 5 * time.Minute,
		},
		{
			name:          "third DLQ retry",
			attempt:       3,
			expectedDelay: 15 * time.Minute,
		},
		{
			name:          "fourth and beyond should cap at max",
			attempt:       4,
			expectedDelay: 30 * time.Minute,
		},
		{
			name:          "tenth retry should still be capped",
			attempt:       10,
			expectedDelay: 30 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			delay := calculateDLQBackoff(tt.attempt)
			assert.Equal(t, tt.expectedDelay, delay,
				"DLQ backoff for attempt %d should be %v", tt.attempt, tt.expectedDelay)
		})
	}
}

func TestDLQHeaderParsing(t *testing.T) {
	t.Parallel()

	t.Run("getDLQRetryCount extracts retry count from headers", func(t *testing.T) {
		t.Parallel()

		headers := map[string]interface{}{
			"x-dlq-retry-count": int32(3),
		}
		count := getDLQRetryCount(headers)
		assert.Equal(t, 3, count, "Should extract retry count from x-dlq-retry-count header")
	})

	t.Run("getDLQRetryCount returns 0 for missing header", func(t *testing.T) {
		t.Parallel()

		headers := map[string]interface{}{}
		count := getDLQRetryCount(headers)
		assert.Equal(t, 0, count, "Should return 0 when header is missing")
	})

	t.Run("getOriginalQueue extracts original queue name", func(t *testing.T) {
		t.Parallel()

		headers := map[string]interface{}{
			"x-dlq-original-queue": "balance_updates",
		}
		queue := getOriginalQueue(headers)
		assert.Equal(t, "balance_updates", queue, "Should extract original queue from headers")
	})
}

func TestDLQProcessingConstants(t *testing.T) {
	t.Parallel()

	t.Run("DLQ batch size should be reasonable", func(t *testing.T) {
		t.Parallel()

		// Process messages in small batches to avoid overwhelming the system
		assert.Equal(t, 10, dlqBatchSize,
			"dlqBatchSize should be 10 to process in manageable chunks")
	})

	t.Run("DLQ prefetch count should match batch size", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, 10, dlqPrefetchCount,
			"dlqPrefetchCount should match batch size for efficient processing")
	})
}
