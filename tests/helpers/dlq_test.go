// File: tests/helpers/dlq_test.go
package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDLQQueueNameBuilder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		queueName   string
		expectedDLQ string
	}{
		{
			name:        "balance_updates queue",
			queueName:   "balance_updates",
			expectedDLQ: "balance_updates.dlq",
		},
		{
			name:        "transactions queue",
			queueName:   "transactions",
			expectedDLQ: "transactions.dlq",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dlqName := BuildDLQName(tt.queueName)
			assert.Equal(t, tt.expectedDLQ, dlqName)
		})
	}
}
