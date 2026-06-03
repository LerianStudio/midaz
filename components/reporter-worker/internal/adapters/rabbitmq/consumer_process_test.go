// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"testing"

	pkgConstant "github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	pkgRabbitmq "github.com/LerianStudio/midaz/v3/components/reporter/pkg/rabbitmq"

	libRabbitMQ "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	amqp091 "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsumerRoutes_Register(t *testing.T) {
	t.Parallel()

	cr := &ConsumerRoutes{
		routes: make(map[string]pkgRabbitmq.QueueHandlerFunc),
	}

	handler := func(_ context.Context, _ []byte) error { return nil }
	cr.Register("test-queue", handler)

	assert.Len(t, cr.routes, 1)
	assert.Contains(t, cr.routes, "test-queue")
}

func TestConsumerRoutes_Register_MultipleQueues(t *testing.T) {
	t.Parallel()

	cr := &ConsumerRoutes{
		routes: make(map[string]pkgRabbitmq.QueueHandlerFunc),
	}

	cr.Register("queue-1", func(_ context.Context, _ []byte) error { return nil })
	cr.Register("queue-2", func(_ context.Context, _ []byte) error { return nil })

	assert.Len(t, cr.routes, 2)
}

func TestConsumerRoutes_Info(t *testing.T) {
	t.Parallel()

	cr := &ConsumerRoutes{
		Logger: log.NewNop(),
	}

	require.NotPanics(t, func() {
		cr.Info("test message")
	})
}

func TestConsumerRoutes_ConsumeMessages_NilChannel(t *testing.T) {
	t.Parallel()

	conn := &libRabbitMQ.RabbitMQConnection{Channel: nil}
	cr := &ConsumerRoutes{conn: conn}

	_, err := cr.consumeMessages("test-queue")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rabbitmq channel is nil")
}

func TestConsumerRoutes_SetupQos_NilChannel(t *testing.T) {
	t.Parallel()

	conn := &libRabbitMQ.RabbitMQConnection{Channel: nil}
	cr := &ConsumerRoutes{conn: conn}

	err := cr.setupQos()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rabbitmq channel is nil")
}

func TestConsumerRoutes_ProcessMessage_SuccessfulHandler(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}

	cr := &ConsumerRoutes{
		Logger:         log.NewNop(),
		Telemetry:      libOtel.Telemetry{},
		tenantResolver: &NoOpTenantResolver{},
		retryManager:   buildRetryManager(nil, nil),
	}

	message := amqp091.Delivery{
		Acknowledger: ack,
		Headers:      amqp091.Table{"X-Request-Id": "req-001"},
		Body:         []byte(`{"test":"data"}`),
	}

	handler := func(_ context.Context, _ []byte) error { return nil }

	cr.processMessage(1, "test-queue", handler, message)

	assert.True(t, ack.acked, "message should be acked after successful handler")
}

func TestConsumerRoutes_ProcessMessage_HandlerError_TriggersRetry(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	channel := &testChannel{}
	manager := &testManager{channel: channel}

	cr := &ConsumerRoutes{
		Logger:         log.NewNop(),
		Telemetry:      libOtel.Telemetry{},
		tenantResolver: &NoOpTenantResolver{},
		retryManager:   buildRetryManager(nil, manager),
	}

	message := amqp091.Delivery{
		Acknowledger: ack,
		Headers:      amqp091.Table{pkgConstant.HeaderXTenantID: "tenant-x"},
		Body:         []byte(`{"test":"data"}`),
	}

	handler := func(_ context.Context, _ []byte) error {
		return errors.New("processing failed")
	}

	cr.processMessage(1, "test-queue", handler, message)

	assert.True(t, ack.nacked || ack.acked, "retry manager should have processed the failure")
}

func TestConsumerRoutes_ProcessMessage_TenantResolverError_TriggersRetry(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}

	failingResolver := &failingTenantResolver{err: errors.New("tenant DB unavailable")}

	conn := &libRabbitMQ.RabbitMQConnection{Channel: nil}
	cr := &ConsumerRoutes{
		Logger:         log.NewNop(),
		Telemetry:      libOtel.Telemetry{},
		tenantResolver: failingResolver,
		retryManager:   buildRetryManager(conn, nil),
	}

	message := amqp091.Delivery{
		Acknowledger: ack,
		Headers:      amqp091.Table{},
		Body:         []byte(`{"test":"data"}`),
	}

	handler := func(_ context.Context, _ []byte) error {
		t.Fatal("handler should not be called when tenant resolution fails")
		return nil
	}

	cr.processMessage(1, "test-queue", handler, message)

	assert.True(t, ack.nacked, "message should be nacked when tenant resolution fails")
}

// failingTenantResolver always returns an error.
type failingTenantResolver struct {
	err error
}

func (f *failingTenantResolver) Resolve(ctx context.Context, _ amqp091.Table) (context.Context, error) {
	return ctx, f.err
}

func TestRecoverWorkerPanic_RecoversPanic(t *testing.T) {
	t.Parallel()

	logger := log.NewNop()

	recovered := make(chan bool, 1)

	go func() {
		defer func() {
			recovered <- true
		}()
		defer recoverWorkerPanic(logger, 1, "test-queue")
		panic("test panic")
	}()

	assert.True(t, <-recovered, "panic should be recovered")
}

func TestRecoverWorkerPanic_NilLogger_DoesNotPanic(t *testing.T) {
	t.Parallel()

	recovered := make(chan bool, 1)

	go func() {
		defer func() {
			recovered <- true
		}()
		defer recoverWorkerPanic(nil, 1, "test-queue")
		panic("test panic with nil logger")
	}()

	assert.True(t, <-recovered)
}

func TestRecoverWorkerPanic_NoPanic_DoesNothing(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		recoverWorkerPanic(log.NewNop(), 1, "test-queue")
	})
}

func TestRunConsumers_NilChannel_ReturnsError(t *testing.T) {
	t.Parallel()

	conn := &libRabbitMQ.RabbitMQConnection{Channel: nil}
	cr := &ConsumerRoutes{
		conn:   conn,
		routes: make(map[string]pkgRabbitmq.QueueHandlerFunc),
		Logger: log.NewNop(),
	}

	cr.Register("test-queue", func(_ context.Context, _ []byte) error { return nil })

	err := cr.RunConsumers(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rabbitmq channel is nil")
}

func TestDeliveryTelemetryAttributes_NoTenantHeader(t *testing.T) {
	t.Parallel()

	message := amqp091.Delivery{
		Exchange:    "reports",
		RoutingKey:  "generate",
		ContentType: "application/json",
		Headers:     amqp091.Table{},
		Body:        []byte(`{}`),
	}

	attrs := deliveryTelemetryAttributes(message)

	found := false
	for _, a := range attrs {
		if string(a.Key) == "app.request.rabbitmq.consumer.has_tenant_header" {
			assert.False(t, a.Value.AsBool())
			found = true
		}
	}

	assert.True(t, found, "should include has_tenant_header attribute")
}

func TestExtractRequestID_NilHeaders(t *testing.T) {
	t.Parallel()

	got := extractRequestID(nil)
	assert.NotEmpty(t, got, "should generate UUID when headers are nil")
}

func TestBuildMessageContext_WithLogger(t *testing.T) {
	t.Parallel()

	logger := log.NewNop()
	message := amqp091.Delivery{
		Headers: amqp091.Table{"X-Request-Id": "req-logger-test"},
	}

	ctx, reqID := buildMessageContext(logger, message)
	assert.NotNil(t, ctx)
	assert.Equal(t, "req-logger-test", reqID)
}

func TestConsumerRoutes_RunConsumers_EmptyRoutes_NoError(t *testing.T) {
	t.Parallel()

	conn := &libRabbitMQ.RabbitMQConnection{Channel: nil}
	cr := &ConsumerRoutes{
		conn:   conn,
		routes: make(map[string]pkgRabbitmq.QueueHandlerFunc),
		Logger: log.NewNop(),
	}

	err := cr.RunConsumers(context.Background(), nil)
	require.NoError(t, err)
}

func TestConsumerRoutes_ProcessMessage_WithRetryHeaders(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}

	cr := &ConsumerRoutes{
		Logger:         log.NewNop(),
		Telemetry:      libOtel.Telemetry{},
		tenantResolver: &NoOpTenantResolver{},
		retryManager:   buildRetryManager(nil, nil),
	}

	message := amqp091.Delivery{
		Acknowledger: ack,
		Headers: amqp091.Table{
			"X-Request-Id":                       "req-retry",
			pkgConstant.RetryCountHeader:         3,
			pkgConstant.RetryFailureReasonHeader: "transient_error",
		},
		Body: []byte(`{}`),
	}

	handler := func(_ context.Context, _ []byte) error { return nil }

	require.NotPanics(t, func() {
		cr.processMessage(1, "test-queue", handler, message)
	})

	assert.True(t, ack.acked)
}

func TestConsumerRoutes_ProcessMessage_PermanentTenantError_NacksToDLQ(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}

	permanentErr := fmt.Errorf("tenant not found: %w", errors.New("permanent"))
	resolver := &failingTenantResolver{err: permanentErr}

	// Provide a nil-channel conn so republishSingleTenant hits the nil guard instead of panicking
	conn := &libRabbitMQ.RabbitMQConnection{Channel: nil}
	cr := &ConsumerRoutes{
		Logger:         log.NewNop(),
		Telemetry:      libOtel.Telemetry{},
		tenantResolver: resolver,
		retryManager:   buildRetryManager(conn, nil),
	}

	message := amqp091.Delivery{
		Acknowledger: ack,
		Headers:      amqp091.Table{},
		Body:         []byte(`{}`),
	}

	handler := func(_ context.Context, _ []byte) error { return nil }
	cr.processMessage(1, "test-queue", handler, message)

	assert.True(t, ack.nacked, "permanent tenant error should nack to DLQ")
}
