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

// TestBuildDLQName validates the buildDLQName helper function that constructs
// Dead Letter Queue names by appending the dlqSuffix to the original queue name.
func TestBuildDLQName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		queueName string
		expected  string
	}{
		{
			name:      "standard queue name",
			queueName: "transactions",
			expected:  "transactions.dlq",
		},
		{
			name:      "hyphenated queue name",
			queueName: "balance-updates",
			expected:  "balance-updates.dlq",
		},
		{
			name:      "empty queue name",
			queueName: "",
			expected:  ".dlq",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := buildDLQName(tt.queueName)
			assert.Equal(t, tt.expected, result, "buildDLQName should append dlqSuffix to queue name")
		})
	}
}
