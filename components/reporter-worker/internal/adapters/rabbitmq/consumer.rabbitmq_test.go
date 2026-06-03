// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"testing"
	"time"

	pkgConstant "github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	pkgRabbitmq "github.com/LerianStudio/midaz/v3/components/reporter/pkg/rabbitmq"

	amqp091 "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
)

// Tests for functions that remain in the adapter package.
// Error classification, header parsing, and backoff tests have been moved to
// pkg/rabbitmq/ (error_classifier_test.go, headers_test.go) and pkg/ (backoff_test.go)
// where the implementations now live.

func TestDeliveryTelemetryAttributes_RedactsMessageBody(t *testing.T) {
	t.Parallel()

	message := amqp091.Delivery{
		Exchange:    "reports",
		RoutingKey:  "generate-report",
		ContentType: "application/json",
		Headers: amqp091.Table{
			pkgConstant.HeaderXTenantID: "tenant-a",
			"x-trace-id":                "trace-123",
		},
		Body: []byte(`{"secret":"12345678900"}`),
	}

	attrs := deliveryTelemetryAttributes(message)

	assert.Contains(t, attrs, attribute.String("app.request.rabbitmq.consumer.exchange", "reports"))
	assert.Contains(t, attrs, attribute.Int("app.request.rabbitmq.consumer.body_size_bytes", len(message.Body)))
	assert.Contains(t, attrs, attribute.Bool("app.request.rabbitmq.consumer.has_tenant_header", true))
	assert.NotContains(t, attrs, attribute.String("app.request.rabbitmq.consumer.message", string(message.Body)))
}

func TestConsumerRoutes_RetryConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 5, pkgConstant.MaxMessageRetries)
	assert.Equal(t, 1*time.Second, pkgConstant.RetryInitialBackoff)
	assert.Equal(t, 30*time.Second, pkgConstant.RetryMaxBackoff)
	assert.Equal(t, 500*time.Millisecond, pkgConstant.RetryJitterMax)
	assert.Equal(t, "x-retry-count", pkgConstant.RetryCountHeader)
	assert.Equal(t, "x-failure-reason", pkgConstant.RetryFailureReasonHeader)
}

func TestExtractRequestID(t *testing.T) {
	t.Parallel()

	t.Run("extracts existing request ID", func(t *testing.T) {
		t.Parallel()

		headers := amqp091.Table{"X-Request-Id": "req-abc-123"}
		got := extractRequestID(headers)
		assert.Equal(t, "req-abc-123", got)
	})

	t.Run("generates UUID when missing", func(t *testing.T) {
		t.Parallel()

		headers := amqp091.Table{}
		got := extractRequestID(headers)
		assert.NotEmpty(t, got)
	})

	t.Run("generates UUID when wrong type", func(t *testing.T) {
		t.Parallel()

		headers := amqp091.Table{"X-Request-Id": 42}
		got := extractRequestID(headers)
		assert.NotEmpty(t, got)
	})
}

func TestBuildMessageContext(t *testing.T) {
	t.Parallel()

	t.Run("creates context with request ID", func(t *testing.T) {
		t.Parallel()

		message := amqp091.Delivery{
			Headers: amqp091.Table{"X-Request-Id": "req-xyz"},
		}

		_, reqID := buildMessageContext(nil, message)
		assert.Equal(t, "req-xyz", reqID)
	})

	t.Run("handles nil headers", func(t *testing.T) {
		t.Parallel()

		message := amqp091.Delivery{Headers: nil}
		ctx, reqID := buildMessageContext(nil, message)
		assert.NotNil(t, ctx)
		assert.NotEmpty(t, reqID)
	})
}

func TestNoOpTenantResolver(t *testing.T) {
	t.Parallel()

	resolver := &NoOpTenantResolver{}

	t.Run("returns context unchanged", func(t *testing.T) {
		t.Parallel()

		ctx, err := resolver.Resolve(t.Context(), amqp091.Table{"X-Tenant-ID": "t1"})
		assert.NoError(t, err)
		assert.NotNil(t, ctx)
	})
}

// Verify that pkg/rabbitmq utilities are accessible from the adapter package.
func TestPkgRabbitmq_GetRetryCount_Integration(t *testing.T) {
	t.Parallel()

	headers := amqp091.Table{pkgConstant.RetryCountHeader: 3}
	assert.Equal(t, 3, pkgRabbitmq.GetRetryCount(headers))
}

func TestPkgRabbitmq_BuildRetryHeaders_Integration(t *testing.T) {
	t.Parallel()

	headers := pkgRabbitmq.BuildRetryHeaders(nil, 0, "retryable_error")
	assert.Equal(t, 1, headers[pkgConstant.RetryCountHeader])
	assert.Equal(t, "retryable_error", headers[pkgConstant.RetryFailureReasonHeader])
}
