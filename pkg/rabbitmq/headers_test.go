// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

// TestHeaderKeyLock pins the exact byte values of the AMQP retry/tenant header keys.
// In-flight messages carry these names; a rename silently breaks retry-count tracking
// and tenant-aware republish for already-queued messages. If this test fails, the
// header contract changed and every producer/consumer in the fleet is affected.
func TestHeaderKeyLock(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "x-retry-count", RetryCountHeader)
	assert.Equal(t, "x-failure-reason", RetryFailureReasonHeader)
	assert.Equal(t, "X-Tenant-ID", TenantIDHeader)
}

func TestRetryCountFromHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		headers amqp.Table
		want    int
	}{
		{"nil headers returns 0", nil, 0},
		{"missing header returns 0", amqp.Table{"other": "value"}, 0},
		{"int value", amqp.Table{RetryCountHeader: 3}, 3},
		{"int32 value", amqp.Table{RetryCountHeader: int32(2)}, 2},
		{"int64 value", amqp.Table{RetryCountHeader: int64(4)}, 4},
		{"float64 value", amqp.Table{RetryCountHeader: float64(1)}, 1},
		{"negative int returns 0", amqp.Table{RetryCountHeader: -1}, 0},
		{"negative int32 returns 0", amqp.Table{RetryCountHeader: int32(-5)}, 0},
		{"negative int64 returns 0", amqp.Table{RetryCountHeader: int64(-3)}, 0},
		{"negative float64 returns 0", amqp.Table{RetryCountHeader: float64(-2.5)}, 0},
		{"string value returns 0", amqp.Table{RetryCountHeader: "not-a-number"}, 0},
		{"zero returns 0", amqp.Table{RetryCountHeader: 0}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, RetryCountFromHeaders(tt.headers))
		})
	}
}

func TestBuildRetryHeaders(t *testing.T) {
	t.Parallel()

	t.Run("copies original headers and increments retry count", func(t *testing.T) {
		t.Parallel()

		original := amqp.Table{
			"traceparent":    "00-abc-def-01",
			"X-Request-Id":   "req-123",
			RetryCountHeader: 2,
		}

		result := BuildRetryHeaders(original, 2, "retryable_error")

		assert.Equal(t, 3, result[RetryCountHeader])
		assert.Equal(t, "retryable_error", result[RetryFailureReasonHeader])
		assert.Equal(t, "00-abc-def-01", result["traceparent"])
		assert.Equal(t, "req-123", result["X-Request-Id"])
	})

	t.Run("handles nil original headers", func(t *testing.T) {
		t.Parallel()

		result := BuildRetryHeaders(nil, 0, "unknown_error")

		assert.Equal(t, 1, result[RetryCountHeader])
		assert.Equal(t, "unknown_error", result[RetryFailureReasonHeader])
	})

	t.Run("does not mutate original headers", func(t *testing.T) {
		t.Parallel()

		original := amqp.Table{RetryCountHeader: 1}
		_ = BuildRetryHeaders(original, 1, "retryable_error")

		assert.Equal(t, 1, original[RetryCountHeader])
	})
}

func TestTenantIDFromHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		headers amqp.Table
		want    string
	}{
		{"nil headers", nil, ""},
		{"missing header", amqp.Table{"other": "val"}, ""},
		{"valid tenant", amqp.Table{TenantIDHeader: "t-1"}, "t-1"},
		{"empty string", amqp.Table{TenantIDHeader: ""}, ""},
		{"non-string type", amqp.Table{TenantIDHeader: 123}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, TenantIDFromHeaders(tt.headers))
		})
	}
}
