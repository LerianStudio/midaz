// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	libConstants "github.com/LerianStudio/lib-commons/v7/commons/constants"
	libRabbitmq "github.com/LerianStudio/lib-commons/v7/commons/rabbitmq"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	amqp "github.com/rabbitmq/amqp091-go"
)

// ProducerRepository provides an interface for Producer related to rabbitmq.
// It defines methods for sending messages to a queue.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=producer.rabbitmq.go -destination=producer.rabbitmq_mock.go -package=rabbitmq
type ProducerRepository interface {
	// ProducerDefault publishes a persistent message and returns only after the
	// broker confirms it. A nack, a channel closed before the confirmation, or a
	// wait that outlives the caller's deadline or the producer's confirmation
	// ceiling is returned as an error.
	ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error)
	CheckRabbitMQHealth() bool
	// Close releases any resources held by the producer (AMQP channel and connection).
	// Safe to call multiple times or on nil receivers.
	Close() error
}

// ProducerRabbitMQRepository is a rabbitmq implementation of the producer
type ProducerRabbitMQRepository struct {
	conn           *libRabbitmq.RabbitMQConnection
	confirmTimeout time.Duration

	// confirmMu guards confirmingChannel, the last shared channel put in
	// confirm mode; a channel recreated by a reconnect is a different pointer.
	confirmMu         sync.Mutex
	confirmingChannel *amqp.Channel
}

// NewProducerRabbitMQ returns a new instance of ProducerRabbitMQRepository using the given rabbitmq connection.
// Returns an error if the connection cannot be established.
func NewProducerRabbitMQ(c *libRabbitmq.RabbitMQConnection, opts ...ProducerOption) (*ProducerRabbitMQRepository, error) {
	if c == nil {
		return nil, fmt.Errorf("rabbitmq connection cannot be nil")
	}

	prmq := &ProducerRabbitMQRepository{
		conn:           c,
		confirmTimeout: newProducerSettings(opts).confirmTimeout,
	}

	_, err := c.GetNewConnect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}

	return prmq, nil
}

// CheckRabbitMQHealth checks the health of the rabbitmq connection.
func (prmq *ProducerRabbitMQRepository) CheckRabbitMQHealth() bool {
	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "false" {
		return true
	}

	healthy, err := prmq.conn.HealthCheck()
	if err != nil {
		return false
	}

	return healthy
}

// ProducerDefault publishes on the connection's shared channel. Concurrent
// publishers keep their own confirmation through the per-message deferred
// confirmation, so the channel is never serialized around the wait.
func (prmq *ProducerRabbitMQRepository) ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	// Rebind ctx: the publish span's trace context is injected into the message
	// headers below so the consumer can continue the trace.
	ctx, spanProducer := tracer.Start(ctx, "rabbitmq.producer.publish_message")
	defer spanProducer.End()

	headers := amqp.Table{
		libConstants.HeaderID: reqId,
	}

	libOpentelemetry.InjectTraceHeadersIntoQueue(ctx, (*map[string]any)(&headers))

	if err := prmq.conn.EnsureChannel(); err != nil {
		libOpentelemetry.HandleSpanError(spanProducer, "Failed to ensure channel", err)

		return nil, err
	}

	// Use ChannelSnapshot to get a consistent channel reference under lock,
	// avoiding a TOCTOU race where another goroutine's reconnection could
	// replace the channel between EnsureChannel and Publish.
	ch := prmq.conn.ChannelSnapshot()
	if ch == nil {
		err := fmt.Errorf("rabbitmq channel unavailable after ensure")
		libOpentelemetry.HandleSpanError(spanProducer, "Channel snapshot returned nil", err)

		return nil, err
	}

	publishCtx, cancel := context.WithTimeout(ctx, prmq.confirmTimeout)
	defer cancel()

	if err := prmq.ensureConfirmMode(ch); err != nil {
		libOpentelemetry.HandleSpanError(spanProducer, "Failed to enable publisher confirms", err)

		return nil, err
	}

	confirmation, err := ch.PublishWithDeferredConfirmWithContext(
		publishCtx,
		exchange,
		key,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Headers:      headers,
			Body:         message,
		},
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanProducer, "Failed to publish message", err)

		return nil, err
	}

	if err := awaitDeferredConfirmation(publishCtx, confirmation, ch.IsClosed); err != nil {
		libOpentelemetry.HandleSpanError(spanProducer, "Publish was not confirmed by the broker", err)

		return nil, err
	}

	return nil, nil
}

func (prmq *ProducerRabbitMQRepository) ensureConfirmMode(ch *amqp.Channel) error {
	prmq.confirmMu.Lock()
	defer prmq.confirmMu.Unlock()

	if prmq.confirmingChannel == ch {
		return nil
	}

	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("enable publisher confirms: %w", err)
	}

	prmq.confirmingChannel = ch

	return nil
}

// Close releases AMQP channel and connection resources.
// Safe to call multiple times or on nil receivers.
// Returns the first error encountered, but attempts to close both channel and connection.
func (prmq *ProducerRabbitMQRepository) Close() error {
	if prmq == nil || prmq.conn == nil {
		return nil
	}

	var firstErr error

	// Close channel first
	if prmq.conn.Channel != nil {
		if err := prmq.conn.Channel.Close(); err != nil {
			firstErr = fmt.Errorf("failed to close AMQP channel: %w", err)
		}
	}

	// Close connection
	if prmq.conn.Connection != nil {
		if err := prmq.conn.Connection.Close(); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("failed to close AMQP connection: %w", err)
			}
		}
	}

	return firstErr
}
