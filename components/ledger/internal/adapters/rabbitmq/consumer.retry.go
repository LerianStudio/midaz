// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"fmt"
	"time"

	libBackoff "github.com/LerianStudio/lib-commons/v7/commons/backoff"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/trace"

	pkgRabbitmq "github.com/LerianStudio/midaz/v4/pkg/rabbitmq"
)

// maxMessageRetries bounds the number of republish attempts before a transient
// failure is dead-lettered. Three attempts is enough to ride out brief infra
// blips without unbounded redelivery; the durable copy lives in the Redis backup
// hash, so DLQ routing is flow-control, not durability.
const maxMessageRetries = 3

// consumerRetryInitialBackoff is the base delay for the exponential backoff that
// paces transient-failure republishes; consumerRetryMaxBackoff caps that delay so
// a long-lived outage does not stretch the inter-attempt wait without bound.
const (
	consumerRetryInitialBackoff = 1 * time.Second
	consumerRetryMaxBackoff     = 30 * time.Second
)

// publishChannel abstracts the AMQP channel surface needed to republish a message
// for retry. The lib-commons connection channel satisfies it; tests inject a spy.
type publishChannel interface {
	Publish(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}

// ConsumerRetryManager classifies failed transaction-consumer messages and routes
// them through the generic pkg/rabbitmq retry engine. It supplies the engine with a
// single-tenant republish hook bound to the consumer's channel; this consumer has no
// multi-tenant republish path (MT is not wired here).
//
// Each consumer generation builds its own manager bound to the channel that generation
// consumes from, so a message is republished on the channel that delivered it and the
// manager never reads the connection's shared channel field.
type ConsumerRetryManager struct {
	classifier pkgRabbitmq.ErrorClassifier
	backoff    pkgRabbitmq.BackoffFunc
	channel    publishChannel
	maxRetries int
	logger     libLog.Logger
}

// NewConsumerRetryManager builds a retry manager for the single-tenant transaction
// consumer. channel is the consumer generation's AMQP channel used for republish; a nil
// channel routes every republish to the DLQ. A typed-nil *amqp.Channel is collapsed to
// an untyped nil so the republish nil check holds (a typed-nil would slip past `== nil`
// and panic on Publish).
func NewConsumerRetryManager(channel publishChannel, logger libLog.Logger) *ConsumerRetryManager {
	if amqpChannel, ok := channel.(*amqp.Channel); ok && amqpChannel == nil {
		channel = nil
	}

	return &ConsumerRetryManager{
		classifier: pkgRabbitmq.NewDefaultClassifier(),
		backoff: func(attempt int) time.Duration {
			return min(
				libBackoff.ExponentialWithJitter(consumerRetryInitialBackoff, attempt),
				consumerRetryMaxBackoff,
			)
		},
		channel:    channel,
		maxRetries: maxMessageRetries,
		logger:     logger,
	}
}

// HandleFailure delegates to the generic retry engine, wiring the single-tenant
// republish path in as the engine's republish hook. Permanent (business) errors and
// retry-count exhaustion route to the DLQ; transient errors under budget are
// republished with incremented headers and the original delivery is Acked.
func (rm *ConsumerRetryManager) HandleFailure(ctx context.Context, workerID int, queue string, message amqp.Delivery, err error, retryCount int, span trace.Span) {
	engine := pkgRabbitmq.NewRetryManager(pkgRabbitmq.Config{
		Classifier: rm.classifier,
		Backoff:    rm.backoff,
		Republish:  rm.republish,
		MaxRetries: rm.maxRetries,
		Logger:     rm.logger,
	})

	engine.HandleFailure(ctx, workerID, queue, message, err, retryCount, span)
}

// republish sends the message back to its origin exchange with updated retry headers
// via the generation's channel. On failure it dead-letters the message and records
// the span, returning the error so the engine leaves the delivery unacked.
func (rm *ConsumerRetryManager) republish(ctx context.Context, workerID int, queue string, message amqp.Delivery, headers amqp.Table, span trace.Span) error {
	channel := rm.channel
	if channel == nil {
		rm.logger.Log(
			ctx, libLog.LevelError, "Channel is nil, cannot republish for retry; sending to DLQ",
			libLog.Int("worker_id", workerID),
			libLog.String("queue", queue),
		)
		libOpentelemetry.HandleSpanError(span, "Channel nil, cannot republish for retry, routing to DLQ", fmt.Errorf("rabbitmq channel is nil"))
		pkgRabbitmq.NackToDLQ(ctx, rm.logger, workerID, queue, message)

		return fmt.Errorf("channel is nil")
	}

	publishing := amqp.Publishing{
		ContentType:  message.ContentType,
		DeliveryMode: amqp.Persistent,
		Headers:      headers,
		Body:         message.Body,
	}

	if publishErr := channel.Publish(message.Exchange, message.RoutingKey, false, false, publishing); publishErr != nil {
		rm.logger.Log(
			ctx, libLog.LevelError, "Failed to republish message for retry; sending to DLQ",
			libLog.Int("worker_id", workerID),
			libLog.String("queue", queue),
			libLog.Err(publishErr),
		)
		libOpentelemetry.HandleSpanError(span, "Failed to republish for retry, routing to DLQ", publishErr)
		pkgRabbitmq.NackToDLQ(ctx, rm.logger, workerID, queue, message)

		return publishErr
	}

	return nil
}
