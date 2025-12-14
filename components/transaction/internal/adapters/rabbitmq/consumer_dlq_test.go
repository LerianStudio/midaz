package rabbitmq

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDLQConstants validates that DLQ-related constants are properly defined
// for Dead Letter Queue naming conventions.
func TestDLQConstants(t *testing.T) {
	t.Parallel()

	t.Run("dlqSuffix constant should equal .dlq", func(t *testing.T) {
		t.Parallel()

		// The dlqSuffix constant is used to construct Dead Letter Queue names
		// by appending this suffix to the original queue name.
		// Example: "transactions" -> "transactions.dlq"
		expected := ".dlq"
		assert.Equal(t, expected, dlqSuffix, "dlqSuffix constant should be '.dlq' for DLQ naming convention")
	})
}
