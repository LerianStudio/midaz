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
		assert.Equal(t, 30*time.Second, healthCheckInterval,
			"healthCheckInterval should be 30 seconds between health polls")
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
