// File: tests/helpers/dlq_test.go
package helpers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateQueueName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		queueName string
		wantErr   bool
		errType   error
	}{
		{
			name:      "valid simple name",
			queueName: "transactions",
			wantErr:   false,
		},
		{
			name:      "valid name with hyphen",
			queueName: "balance-updates",
			wantErr:   false,
		},
		{
			name:      "valid name with underscore",
			queueName: "balance_updates",
			wantErr:   false,
		},
		{
			name:      "valid name with dot",
			queueName: "balance.updates",
			wantErr:   false,
		},
		{
			name:      "valid name with numbers",
			queueName: "queue123",
			wantErr:   false,
		},
		{
			name:      "empty name rejected",
			queueName: "",
			wantErr:   true,
		},
		{
			name:      "path traversal rejected",
			queueName: "../etc/passwd",
			wantErr:   true,
			errType:   ErrInvalidQueueName,
		},
		{
			name:      "URL injection rejected",
			queueName: "queue%2F..%2Fetc",
			wantErr:   true,
			errType:   ErrInvalidQueueName,
		},
		{
			name:      "slash rejected",
			queueName: "queue/name",
			wantErr:   true,
			errType:   ErrInvalidQueueName,
		},
		{
			name:      "leading slash rejected",
			queueName: "/queue",
			wantErr:   true,
			errType:   ErrInvalidQueueName,
		},
		{
			name:      "double dot rejected",
			queueName: "queue..name",
			wantErr:   true,
			errType:   ErrInvalidQueueName,
		},
		{
			name:      "space rejected",
			queueName: "queue name",
			wantErr:   true,
			errType:   ErrInvalidQueueName,
		},
		{
			name:      "special chars rejected",
			queueName: "queue<script>",
			wantErr:   true,
			errType:   ErrInvalidQueueName,
		},
		{
			name:      "too long name rejected",
			queueName: string(make([]byte, 256)),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateQueueName(tt.queueName)

			if tt.wantErr {
				assert.Error(t, err, "Expected error for queue name: %s", tt.queueName)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				assert.NoError(t, err, "Expected no error for queue name: %s", tt.queueName)
			}
		})
	}
}

func TestGetDLQMessageCount_ValidationRejectsInvalidNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		queueName string
	}{
		{
			name:      "path traversal attack",
			queueName: "../../../etc/passwd",
		},
		{
			name:      "URL encoded attack",
			queueName: "queue%00name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The function should fail at validation before making any HTTP request
			count, err := GetDLQMessageCount(
				context.Background(),
				"http://localhost:15672", // Won't be called
				tt.queueName,
				"guest",
				"guest",
			)

			assert.Error(t, err, "Should reject invalid queue name")
			assert.Equal(t, 0, count, "Should return 0 on validation failure")
			assert.Contains(t, err.Error(), "validation", "Error should mention validation")
		})
	}
}

func TestBuildDLQName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		queueName string
		expected  string
	}{
		{
			name:      "simple queue",
			queueName: "transactions",
			expected:  "transactions.dlq",
		},
		{
			name:      "queue with hyphen",
			queueName: "balance-updates",
			expected:  "balance-updates.dlq",
		},
		{
			name:      "balance_updates queue",
			queueName: "balance_updates",
			expected:  "balance_updates.dlq",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := BuildDLQName(tt.queueName)
			assert.Equal(t, tt.expected, result)
		})
	}
}
