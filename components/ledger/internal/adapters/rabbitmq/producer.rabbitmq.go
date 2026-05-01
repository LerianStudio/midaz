// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"fmt"
	"os"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

// ProducerRepository provides an interface for Producer related to rabbitmq.
// It defines methods for sending messages to a queue.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=producer.rabbitmq.go -destination=producer.rabbitmq_mock.go -package=rabbitmq
type ProducerRepository interface {
	ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error)
	// ProducerDefaultWithContext sends message with explicit context timeout control.
	// The context deadline/timeout controls how long to wait for RabbitMQ connection.
	ProducerDefaultWithContext(ctx context.Context, exchange, key string, message []byte) (*string, error)
	CheckRabbitMQHealth() bool
	// Close releases any resources held by the producer (AMQP channel and connection).
	// Safe to call multiple times or on nil receivers.
	Close() error
}

// ProducerRabbitMQRepository is a rabbitmq implementation of the producer
type ProducerRabbitMQRepository struct {
	conn *libRabbitmq.RabbitMQConnection
}

// NewProducerRabbitMQ returns a new instance of ProducerRabbitMQRepository using the given rabbitmq connection.
// Returns an error if the connection cannot be established.
func NewProducerRabbitMQ(c *libRabbitmq.RabbitMQConnection) (*ProducerRabbitMQRepository, error) {
	if c == nil {
		return nil, fmt.Errorf("rabbitmq connection cannot be nil")
	}

	prmq := &ProducerRabbitMQRepository{
		conn: c,
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

// ProducerDefault sends a message to a RabbitMQ queue for further processing.
func (prmq *ProducerRabbitMQRepository) ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	logger.Log(ctx, libLog.LevelInfo, "Init sent message", libLog.String("exchange", exchange), libLog.String("key", key))

	ctx, spanProducer := tracer.Start(ctx, "rabbitmq.producer.publish_message")
	defer spanProducer.End()

	headers := amqp.Table{
		libConstants.HeaderID: reqId,
	}

	libOpentelemetry.InjectTraceHeadersIntoQueue(ctx, (*map[string]any)(&headers))

	if err := prmq.conn.EnsureChannel(); err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to ensure channel", libLog.Err(err))
		libOpentelemetry.HandleSpanError(spanProducer, "Failed to ensure channel", err)

		return nil, err
	}

	// Use ChannelSnapshot to get a consistent channel reference under lock,
	// avoiding a TOCTOU race where another goroutine's reconnection could
	// replace the channel between EnsureChannel and Publish.
	ch := prmq.conn.ChannelSnapshot()
	if ch == nil {
		err := fmt.Errorf("rabbitmq channel unavailable after ensure")
		logger.Log(ctx, libLog.LevelError, "Channel snapshot returned nil", libLog.Err(err))
		libOpentelemetry.HandleSpanError(spanProducer, "Channel snapshot returned nil", err)

		return nil, err
	}

	err := ch.Publish(
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
		logger.Log(ctx, libLog.LevelError, "Failed to publish message", libLog.String("exchange", exchange), libLog.String("key", key), libLog.Err(err))
		libOpentelemetry.HandleSpanError(spanProducer, "Failed to publish message", err)

		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, "Message sent successfully", libLog.String("exchange", exchange), libLog.String("key", key))

	return nil, nil
}

// ProducerDefaultWithContext sends a message to RabbitMQ with context-aware timeout.
// Uses EnsureChannelWithContext to respect context deadline for connection attempts.
func (prmq *ProducerRabbitMQRepository) ProducerDefaultWithContext(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	logger.Log(ctx, libLog.LevelInfo, "Init sent message with context", libLog.String("exchange", exchange), libLog.String("key", key))

	ctx, spanProducer := tracer.Start(ctx, "rabbitmq.producer.publish_message_with_context")
	defer spanProducer.End()

	headers := amqp.Table{
		libConstants.HeaderID: reqId,
	}

	libOpentelemetry.InjectTraceHeadersIntoQueue(ctx, (*map[string]any)(&headers))

	if err := prmq.conn.EnsureChannelContext(ctx); err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to ensure channel with context", libLog.Err(err))
		libOpentelemetry.HandleSpanError(spanProducer, "Failed to ensure channel with context", err)

		return nil, err
	}

	// Use ChannelSnapshot to get a consistent channel reference under lock,
	// avoiding a TOCTOU race where another goroutine's reconnection could
	// replace the channel between EnsureChannelContext and Publish.
	ch := prmq.conn.ChannelSnapshot()
	if ch == nil {
		err := fmt.Errorf("rabbitmq channel unavailable after ensure")
		logger.Log(ctx, libLog.LevelError, "Channel snapshot returned nil", libLog.Err(err))
		libOpentelemetry.HandleSpanError(spanProducer, "Channel snapshot returned nil", err)

		return nil, err
	}

	err := ch.Publish(
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
		logger.Log(ctx, libLog.LevelError, "Failed to publish message", libLog.String("exchange", exchange), libLog.String("key", key), libLog.Err(err))
		libOpentelemetry.HandleSpanError(spanProducer, "Failed to publish message", err)

		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, "Message sent successfully with context", libLog.String("exchange", exchange), libLog.String("key", key))

	return nil, nil
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
