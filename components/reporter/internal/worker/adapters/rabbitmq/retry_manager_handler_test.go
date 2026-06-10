// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"testing"
	"time"

	pkgErr "github.com/LerianStudio/midaz/v4/pkg"

	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	pkgRabbitmq "github.com/LerianStudio/midaz/v4/pkg/reporter/rabbitmq"

	"github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	amqp091 "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

// testAcknowledger is a test double for amqp091.Acknowledger.
type testAcknowledger struct {
	acked   bool
	nacked  bool
	requeue bool
	ackErr  error
	nackErr error
}

func (f *testAcknowledger) Ack(_ uint64, _ bool) error {
	f.acked = true
	return f.ackErr
}

func (f *testAcknowledger) Nack(_ uint64, _ bool, requeue bool) error {
	f.nacked = true
	f.requeue = requeue
	return f.nackErr
}

func (f *testAcknowledger) Reject(_ uint64, requeue bool) error {
	f.requeue = requeue
	return nil
}

// testChannel implements RabbitMQConnectionChannel for testing.
type testChannel struct {
	publishErr error
	published  bool
}

func (f *testChannel) Publish(_, _ string, _, _ bool, _ amqp091.Publishing) error {
	f.published = true
	return f.publishErr
}

// testManager implements RabbitMQManagerConsumerInterface for testing.
type testManager struct {
	channel    RabbitMQConnectionChannel
	connErr    error
	calledWith string
}

func (f *testManager) GetConnection(_ context.Context, tenantID string) (RabbitMQConnectionChannel, error) {
	f.calledWith = tenantID
	return f.channel, f.connErr
}

func buildRetryManager(conn *rabbitmq.RabbitMQConnection, manager RabbitMQManagerConsumerInterface) *ConsumerRetryManager {
	return &ConsumerRetryManager{
		classifier:      pkgRabbitmq.NewDefaultErrorClassifier(),
		backoff:         func(int) time.Duration { return 0 },
		conn:            conn,
		rabbitMQManager: manager,
		maxRetries:      pkgConstant.MaxMessageRetries,
		logger:          log.NewNop(),
		telemetry:       libOtel.Telemetry{},
	}
}

func buildDelivery(ack *testAcknowledger, headers amqp091.Table) amqp091.Delivery {
	return amqp091.Delivery{
		Acknowledger: ack,
		Exchange:     "test-exchange",
		RoutingKey:   "test-routing-key",
		ContentType:  "application/json",
		Headers:      headers,
		Body:         []byte(`{"test":"data"}`),
	}
}

func testSpan() noop.Span {
	tracer := noop.NewTracerProvider().Tracer("test")
	_, span := tracer.Start(context.Background(), "test")

	return span.(noop.Span)
}

func TestHandleFailure_NonRetryableError_SendsToDLQ(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	rm := buildRetryManager(nil, nil)
	message := buildDelivery(ack, nil)
	span := testSpan()

	nonRetryableErr := pkgErr.ValidationError{
		Code:    "VAL-001",
		Title:   "Validation Error",
		Message: "invalid field",
	}

	rm.HandleFailure(context.Background(), 1, "test-queue", message, nonRetryableErr, 0, span)

	assert.True(t, ack.nacked, "message should be nacked to DLQ for non-retryable error")
	assert.False(t, ack.requeue, "message should not be requeued")
}

func TestHandleFailure_MaxRetriesExceeded_SendsToDLQ(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	rm := buildRetryManager(nil, nil)
	message := buildDelivery(ack, nil)
	span := testSpan()

	rm.HandleFailure(context.Background(), 1, "test-queue", message, errors.New("timeout"), pkgConstant.MaxMessageRetries, span)

	assert.True(t, ack.nacked, "message should be nacked to DLQ when max retries exceeded")
	assert.False(t, ack.requeue)
}

func TestHandleFailure_RetryableError_MultiTenant_RepublishesAndAcks(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	channel := &testChannel{}
	manager := &testManager{channel: channel}
	rm := buildRetryManager(nil, manager)
	message := buildDelivery(ack, amqp091.Table{pkgConstant.HeaderXTenantID: "tenant-e"})
	span := testSpan()

	rm.HandleFailure(context.Background(), 1, "test-queue", message, errors.New("transient error"), 0, span)

	assert.True(t, channel.published, "message should be republished via multi-tenant channel")
	assert.True(t, ack.acked, "message should be acked after successful republish")
}

func TestHandleFailure_AckFailsAfterRepublish_DoesNotPanic(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{ackErr: errors.New("ack failed")}
	channel := &testChannel{}
	manager := &testManager{channel: channel}
	rm := buildRetryManager(nil, manager)
	message := buildDelivery(ack, amqp091.Table{pkgConstant.HeaderXTenantID: "tenant-f"})
	span := testSpan()

	require.NotPanics(t, func() {
		rm.HandleFailure(context.Background(), 1, "test-queue", message, errors.New("transient"), 0, span)
	})

	assert.True(t, channel.published)
	assert.True(t, ack.acked, "ack was attempted")
}

func TestHandleFailure_RepublishFails_NacksToDLQ(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	channel := &testChannel{publishErr: errors.New("publish failed")}
	manager := &testManager{channel: channel}
	rm := buildRetryManager(nil, manager)
	message := buildDelivery(ack, amqp091.Table{pkgConstant.HeaderXTenantID: "tenant-g"})
	span := testSpan()

	rm.HandleFailure(context.Background(), 1, "test-queue", message, errors.New("transient"), 0, span)

	assert.True(t, ack.nacked, "message should be nacked to DLQ when republish fails")
	assert.False(t, ack.acked, "message should NOT be acked when republish fails")
}

func TestRepublishSingleTenant_NilChannel_SendsToDLQ(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	conn := &rabbitmq.RabbitMQConnection{Channel: nil}
	rm := buildRetryManager(conn, nil)
	message := buildDelivery(ack, amqp091.Table{})
	span := testSpan()

	err := rm.republishSingleTenant(context.Background(), 1, "test-queue", message, amqp091.Publishing{Body: message.Body}, span)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel is nil")
	assert.True(t, ack.nacked, "message should be nacked to DLQ when channel is nil")
}

func TestRepublishMultiTenant_NoTenantID_SendsToDLQ(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	manager := &testManager{}
	rm := buildRetryManager(nil, manager)
	message := buildDelivery(ack, amqp091.Table{})
	span := testSpan()

	err := rm.republishMultiTenant(context.Background(), 1, "test-queue", message, amqp091.Publishing{Body: message.Body}, span)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing tenant ID")
	assert.True(t, ack.nacked)
}

func TestRepublishMultiTenant_GetConnectionError_SendsToDLQ(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	manager := &testManager{connErr: errors.New("vhost not found")}
	rm := buildRetryManager(nil, manager)
	message := buildDelivery(ack, amqp091.Table{pkgConstant.HeaderXTenantID: "tenant-a"})
	span := testSpan()

	err := rm.republishMultiTenant(context.Background(), 1, "test-queue", message, amqp091.Publishing{Body: message.Body}, span)

	require.Error(t, err)
	assert.Equal(t, "tenant-a", manager.calledWith)
	assert.True(t, ack.nacked)
}

func TestRepublishMultiTenant_PublishError_SendsToDLQ(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	channel := &testChannel{publishErr: errors.New("channel closed")}
	manager := &testManager{channel: channel}
	rm := buildRetryManager(nil, manager)
	message := buildDelivery(ack, amqp091.Table{pkgConstant.HeaderXTenantID: "tenant-b"})
	span := testSpan()

	err := rm.republishMultiTenant(context.Background(), 1, "test-queue", message, amqp091.Publishing{Body: message.Body}, span)

	require.Error(t, err)
	assert.True(t, ack.nacked)
}

func TestRepublishMultiTenant_Success(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	channel := &testChannel{}
	manager := &testManager{channel: channel}
	rm := buildRetryManager(nil, manager)
	message := buildDelivery(ack, amqp091.Table{pkgConstant.HeaderXTenantID: "tenant-c"})
	span := testSpan()

	err := rm.republishMultiTenant(context.Background(), 1, "test-queue", message, amqp091.Publishing{Body: message.Body}, span)

	require.NoError(t, err)
	assert.True(t, channel.published)
	assert.False(t, ack.nacked)
	assert.Equal(t, "tenant-c", manager.calledWith)
}

func TestRepublish_RoutesToMultiTenant_WhenManagerSet(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	channel := &testChannel{}
	manager := &testManager{channel: channel}
	rm := buildRetryManager(nil, manager)
	message := buildDelivery(ack, amqp091.Table{pkgConstant.HeaderXTenantID: "tenant-d"})
	span := testSpan()

	err := rm.republish(context.Background(), 1, "test-queue", message, amqp091.Table{}, span)

	require.NoError(t, err)
	assert.Equal(t, "tenant-d", manager.calledWith)
}

func TestRepublish_RoutesToSingleTenant_WhenManagerNil(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	conn := &rabbitmq.RabbitMQConnection{Channel: nil}
	rm := buildRetryManager(conn, nil)
	message := buildDelivery(ack, amqp091.Table{})
	span := testSpan()

	err := rm.republish(context.Background(), 1, "test-queue", message, amqp091.Table{}, span)

	require.Error(t, err, "should fail with nil channel in single-tenant mode")
	assert.True(t, ack.nacked)
}

func TestNackToDLQ_Success(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{}
	rm := buildRetryManager(nil, nil)
	message := buildDelivery(ack, nil)

	rm.nackToDLQ(context.Background(), 1, "test-queue", message)

	assert.True(t, ack.nacked)
	assert.False(t, ack.requeue, "DLQ nack must not requeue")
}

func TestNackToDLQ_NackError_DoesNotPanic(t *testing.T) {
	t.Parallel()

	ack := &testAcknowledger{nackErr: errors.New("nack failed")}
	rm := buildRetryManager(nil, nil)
	message := buildDelivery(ack, nil)

	require.NotPanics(t, func() {
		rm.nackToDLQ(context.Background(), 1, "test-queue", message)
	})
}

func TestLogPublishFailure_DoesNotPanic(t *testing.T) {
	t.Parallel()

	rm := buildRetryManager(nil, nil)
	span := testSpan()

	require.NotPanics(t, func() {
		rm.logPublishFailure(context.Background(), 1, "test-queue", errors.New("publish failed"), span)
	})
}
