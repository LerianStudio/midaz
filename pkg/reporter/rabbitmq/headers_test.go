// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRetryCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		headers amqp.Table
		want    int
	}{
		{
			name:    "nil headers returns 0",
			headers: nil,
			want:    0,
		},
		{
			name:    "missing header returns 0",
			headers: amqp.Table{"other": "value"},
			want:    0,
		},
		{
			name:    "int value",
			headers: amqp.Table{constant.RetryCountHeader: 3},
			want:    3,
		},
		{
			name:    "int32 value",
			headers: amqp.Table{constant.RetryCountHeader: int32(2)},
			want:    2,
		},
		{
			name:    "int64 value",
			headers: amqp.Table{constant.RetryCountHeader: int64(4)},
			want:    4,
		},
		{
			name:    "float64 value",
			headers: amqp.Table{constant.RetryCountHeader: float64(1)},
			want:    1,
		},
		{
			name:    "negative int returns 0",
			headers: amqp.Table{constant.RetryCountHeader: -1},
			want:    0,
		},
		{
			name:    "negative int32 returns 0",
			headers: amqp.Table{constant.RetryCountHeader: int32(-5)},
			want:    0,
		},
		{
			name:    "negative int64 returns 0",
			headers: amqp.Table{constant.RetryCountHeader: int64(-3)},
			want:    0,
		},
		{
			name:    "negative float64 returns 0",
			headers: amqp.Table{constant.RetryCountHeader: float64(-2.5)},
			want:    0,
		},
		{
			name:    "string value returns 0",
			headers: amqp.Table{constant.RetryCountHeader: "not-a-number"},
			want:    0,
		},
		{
			name:    "zero returns 0",
			headers: amqp.Table{constant.RetryCountHeader: 0},
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := GetRetryCount(tt.headers)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildRetryHeaders(t *testing.T) {
	t.Parallel()

	t.Run("copies original headers and increments retry count", func(t *testing.T) {
		t.Parallel()

		original := amqp.Table{
			"traceparent":             "00-abc-def-01",
			"X-Request-Id":            "req-123",
			constant.RetryCountHeader: 2,
		}

		result := BuildRetryHeaders(original, 2, "retryable_error")

		assert.Equal(t, 3, result[constant.RetryCountHeader])
		assert.Equal(t, "retryable_error", result[constant.RetryFailureReasonHeader])
		assert.Equal(t, "00-abc-def-01", result["traceparent"])
		assert.Equal(t, "req-123", result["X-Request-Id"])
	})

	t.Run("handles nil original headers", func(t *testing.T) {
		t.Parallel()

		result := BuildRetryHeaders(nil, 0, "unknown_error")

		assert.Equal(t, 1, result[constant.RetryCountHeader])
		assert.Equal(t, "unknown_error", result[constant.RetryFailureReasonHeader])
	})

	t.Run("does not mutate original headers", func(t *testing.T) {
		t.Parallel()

		original := amqp.Table{constant.RetryCountHeader: 1}
		_ = BuildRetryHeaders(original, 1, "retryable_error")

		assert.Equal(t, 1, original[constant.RetryCountHeader])
	})
}

func TestNewProducerHeaders(t *testing.T) {
	t.Parallel()

	t.Run("sets request ID and retry count", func(t *testing.T) {
		t.Parallel()

		headers := NewProducerHeaders("req-abc", "")

		assert.Equal(t, "req-abc", headers["X-Request-Id"])
		assert.Equal(t, 0, headers[constant.RetryCountHeader])
		_, hasTenant := headers[constant.HeaderXTenantID]
		assert.False(t, hasTenant)
	})

	t.Run("includes tenant ID when present", func(t *testing.T) {
		t.Parallel()

		headers := NewProducerHeaders("req-def", "tenant-42")

		assert.Equal(t, "tenant-42", headers[constant.HeaderXTenantID])
		assert.Equal(t, "req-def", headers["X-Request-Id"])
	})
}

func TestExtractTenantID(t *testing.T) {
	t.Parallel()

	t.Run("extracts tenant ID from headers", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		headers := amqp.Table{constant.HeaderXTenantID: "tenant-99"}

		newCtx := ExtractTenantID(ctx, headers)

		tenantID := tmcore.GetTenantIDContext(newCtx)
		require.Equal(t, "tenant-99", tenantID)
	})

	t.Run("nil headers returns unchanged context", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		newCtx := ExtractTenantID(ctx, nil)

		assert.Equal(t, ctx, newCtx)
	})

	t.Run("missing header returns unchanged context", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		headers := amqp.Table{"other": "value"}

		newCtx := ExtractTenantID(ctx, headers)

		tenantID := tmcore.GetTenantIDContext(newCtx)
		assert.Empty(t, tenantID)
	})

	t.Run("empty string returns unchanged context", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		headers := amqp.Table{constant.HeaderXTenantID: ""}

		newCtx := ExtractTenantID(ctx, headers)

		tenantID := tmcore.GetTenantIDContext(newCtx)
		assert.Empty(t, tenantID)
	})

	t.Run("non-string header returns unchanged context", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		headers := amqp.Table{constant.HeaderXTenantID: 42}

		newCtx := ExtractTenantID(ctx, headers)

		tenantID := tmcore.GetTenantIDContext(newCtx)
		assert.Empty(t, tenantID)
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
		{"valid tenant", amqp.Table{constant.HeaderXTenantID: "t-1"}, "t-1"},
		{"empty string", amqp.Table{constant.HeaderXTenantID: ""}, ""},
		{"non-string type", amqp.Table{constant.HeaderXTenantID: 123}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, TenantIDFromHeaders(tt.headers))
		})
	}
}
