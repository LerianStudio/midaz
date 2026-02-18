// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"fmt"
	"os"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

// ProducerRepository provides an interface for Producer related to rabbitmq.
// It defines methods for sending messages to a queue.
type ProducerRepository interface {
	ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error)
	// ProducerDefaultWithContext sends message with explicit context timeout control.
	// The context deadline/timeout controls how long to wait for RabbitMQ connection.
	ProducerDefaultWithContext(ctx context.Context, exchange, key string, message []byte) (*string, error)
	CheckRabbitMQHealth() bool
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

	return prmq.conn.HealthCheck()
}

// ProducerDefault sends a message to a RabbitMQ queue for further processing.
func (prmq *ProducerRabbitMQRepository) ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	logger.Infof("Init sent message to exchange: %s, key: %s", exchange, key)

	ctx, spanProducer := tracer.Start(ctx, "rabbitmq.producer.publish_message")
	defer spanProducer.End()

	headers := amqp.Table{
		libConstants.HeaderID: reqId,
	}

	libOpentelemetry.InjectTraceHeadersIntoQueue(ctx, (*map[string]any)(&headers))

	if err := prmq.conn.EnsureChannel(); err != nil {
		logger.Errorf("Failed to ensure channel: %v", err)
		libOpentelemetry.HandleSpanError(&spanProducer, "Failed to ensure channel", err)

		return nil, err
	}

	err := prmq.conn.Channel.Publish(
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
		logger.Errorf("Failed to publish message to exchange: %s, key: %s: %v", exchange, key, err)
		libOpentelemetry.HandleSpanError(&spanProducer, "Failed to publish message", err)

		return nil, err
	}

	logger.Infof("Messages sent successfully to exchange: %s, key: %s", exchange, key)

	return nil, nil
}

// ProducerDefaultWithContext sends a message to RabbitMQ with context-aware timeout.
// Uses EnsureChannelWithContext to respect context deadline for connection attempts.
func (prmq *ProducerRabbitMQRepository) ProducerDefaultWithContext(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	logger.Infof("Init sent message to exchange: %s, key: %s (with context)", exchange, key)

	ctx, spanProducer := tracer.Start(ctx, "rabbitmq.producer.publish_message_with_context")
	defer spanProducer.End()

	headers := amqp.Table{
		libConstants.HeaderID: reqId,
	}

	libOpentelemetry.InjectTraceHeadersIntoQueue(ctx, (*map[string]any)(&headers))

	if err := prmq.conn.EnsureChannelWithContext(ctx); err != nil {
		logger.Errorf("Failed to ensure channel with context: %v", err)
		libOpentelemetry.HandleSpanError(&spanProducer, "Failed to ensure channel with context", err)

		return nil, err
	}

	err := prmq.conn.Channel.Publish(
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
		logger.Errorf("Failed to publish message to exchange: %s, key: %s: %v", exchange, key, err)
		libOpentelemetry.HandleSpanError(&spanProducer, "Failed to publish message", err)

		return nil, err
	}

	logger.Infof("Messages sent successfully to exchange: %s, key: %s (with context)", exchange, key)

	return nil, nil
}
