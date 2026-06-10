// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"testing"
	"time"

	libRabbitmq "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	pkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgRabbitmq "github.com/LerianStudio/midaz/v4/pkg/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
)

// spyChannel captures Publish calls so tests can assert republish behavior without
// a real RabbitMQ connection.
type spyChannel struct {
	publishCalled  bool
	publishHeaders amqp.Table
	publishErr     error
}

func (s *spyChannel) Publish(_ string, _ string, _ bool, _ bool, msg amqp.Publishing) error {
	s.publishCalled = true
	s.publishHeaders = msg.Headers

	return s.publishErr
}

// newTestRetryManager builds a ConsumerRetryManager wired with the generic engine,
// a no-op sleep, and a spy channel for republish capture.
func newTestRetryManager(channel publishChannel) *ConsumerRetryManager {
	return &ConsumerRetryManager{
		classifier:  pkgRabbitmq.NewDefaultClassifier(),
		backoff:     func(int) time.Duration { return 0 },
		channelFunc: func() publishChannel { return channel },
		maxRetries:  maxMessageRetries,
		logger:      testLogger,
	}
}

func TestRetryManager_PermanentError_NacksToDLQ(t *testing.T) {
	t.Parallel()

	ack := &mockAcknowledger{}
	delivery := amqp.Delivery{
		Acknowledger: ack,
		DeliveryTag:  1,
		Body:         []byte("msg"),
		Headers:      amqp.Table{},
	}

	channel := &spyChannel{}
	rm := newTestRetryManager(channel)

	// Business error → permanent → immediate DLQ nack (requeue=false).
	permErr := pkg.ValidateBusinessError(constant.ErrInsufficientAccountBalance, "Balance")

	rm.HandleFailure(context.Background(), 0, "transaction.queue", delivery, permErr, 0, noop.Span{})

	assert.True(t, ack.nackCalled, "permanent error should nack")
	assert.False(t, ack.nackRequeue, "permanent error must nack to DLQ (requeue=false), not requeue")
	assert.False(t, channel.publishCalled, "permanent error must not republish")
}

func TestRetryManager_TransientUnderBudget_RepublishesWithIncrementedHeader(t *testing.T) {
	t.Parallel()

	ack := &mockAcknowledger{}
	delivery := amqp.Delivery{
		Acknowledger: ack,
		DeliveryTag:  2,
		Body:         []byte("msg"),
		Headers:      amqp.Table{pkgRabbitmq.RetryCountHeader: 1},
	}

	channel := &spyChannel{}
	rm := newTestRetryManager(channel)

	transientErr := errors.New("transient infra failure")

	rm.HandleFailure(context.Background(), 0, "transaction.queue", delivery, transientErr, 1, noop.Span{})

	assert.True(t, channel.publishCalled, "transient error under budget should republish")
	assert.Equal(t, 2, channel.publishHeaders[pkgRabbitmq.RetryCountHeader], "retry count header should be incremented")
	assert.True(t, ack.ackCalled, "original delivery should be acked after republish")
	assert.False(t, ack.nackCalled, "successful republish should not nack")
}

func TestRetryManager_TransientAtMaxRetries_NacksToDLQ(t *testing.T) {
	t.Parallel()

	ack := &mockAcknowledger{}
	delivery := amqp.Delivery{
		Acknowledger: ack,
		DeliveryTag:  3,
		Body:         []byte("msg"),
		Headers:      amqp.Table{pkgRabbitmq.RetryCountHeader: maxMessageRetries},
	}

	channel := &spyChannel{}
	rm := newTestRetryManager(channel)

	transientErr := errors.New("transient infra failure")

	rm.HandleFailure(context.Background(), 0, "transaction.queue", delivery, transientErr, maxMessageRetries, noop.Span{})

	assert.False(t, channel.publishCalled, "at max retries should not republish")
	assert.True(t, ack.nackCalled, "at max retries should nack to DLQ")
	assert.False(t, ack.nackRequeue, "at max retries must nack to DLQ (requeue=false)")
}

// TestNewConsumerRoutes_ReturnsErrorOnConnectionFailure ensures the constructor returns
// an error (not a panic) when the RabbitMQ connection cannot be established.
func TestNewConsumerRoutes_ReturnsErrorOnConnectionFailure(t *testing.T) {
	t.Parallel()

	conn := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: "amqp://invalid:invalid@127.0.0.1:1/",
		Host:                   "127.0.0.1",
		Port:                   "1",
		Logger:                 testLogger,
	}

	telemetry := &libOpentelemetry.Telemetry{}

	cr, err := NewConsumerRoutes(conn, 5, 10, testLogger, telemetry)

	assert.Error(t, err, "NewConsumerRoutes should return an error on connection failure")
	assert.Nil(t, cr, "ConsumerRoutes should be nil on connection failure")
}
