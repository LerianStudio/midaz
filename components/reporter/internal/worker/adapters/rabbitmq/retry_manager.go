// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"fmt"

	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	pkgReporterRabbitmq "github.com/LerianStudio/midaz/v4/pkg/reporter/rabbitmq"

	pkgRabbitmq "github.com/LerianStudio/midaz/v4/pkg/rabbitmq"

	"github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/trace"
)

// ConsumerRetryManager encapsulates retry logic for failed consumer messages.
// It supplies the reporter's classifier and tenant-aware republish hook to the
// generic pkg/rabbitmq retry engine, which owns the classify/backoff/Ack flow.
type ConsumerRetryManager struct {
	classifier      pkgReporterRabbitmq.ErrorClassifier
	backoff         pkgRabbitmq.BackoffFunc
	conn            *rabbitmq.RabbitMQConnection     // single-tenant channel
	rabbitMQManager RabbitMQManagerConsumerInterface // multi-tenant channel (nil in ST)
	maxRetries      int
	logger          log.Logger
	telemetry       libOtel.Telemetry
}

// NewConsumerRetryManager creates a new retry manager with the given dependencies.
func NewConsumerRetryManager(
	classifier pkgReporterRabbitmq.ErrorClassifier,
	backoff pkgRabbitmq.BackoffFunc,
	conn *rabbitmq.RabbitMQConnection,
	rabbitMQManager RabbitMQManagerConsumerInterface,
	logger log.Logger,
	telemetry libOtel.Telemetry,
) *ConsumerRetryManager {
	return &ConsumerRetryManager{
		classifier:      classifier,
		backoff:         backoff,
		conn:            conn,
		rabbitMQManager: rabbitMQManager,
		maxRetries:      pkgConstant.MaxMessageRetries,
		logger:          logger,
		telemetry:       telemetry,
	}
}

// HandleFailure determines whether a failed message should be retried or sent to DLQ.
// It delegates to the generic retry engine, wiring the reporter's tenant-aware
// republish path in as the engine's republish hook.
func (rm *ConsumerRetryManager) HandleFailure(ctx context.Context, workerID int, queue string, message amqp091.Delivery, err error, retryCount int, span trace.Span) {
	engine := pkgRabbitmq.NewRetryManager(pkgRabbitmq.Config{
		Classifier: rm.classifier,
		Backoff:    rm.backoff,
		Republish:  rm.republish,
		MaxRetries: rm.maxRetries,
		Logger:     rm.logger,
	})

	engine.HandleFailure(ctx, workerID, queue, message, err, retryCount, span)
}

// republish sends the message back to the exchange with updated headers.
// Uses tenant-specific channel in multi-tenant mode, static channel otherwise.
// On failure it has already nacked the message to the DLQ and recorded the span,
// so the engine leaves the delivery unacked.
func (rm *ConsumerRetryManager) republish(ctx context.Context, workerID int, queue string, message amqp091.Delivery, headers amqp091.Table, span trace.Span) error {
	publishing := amqp091.Publishing{
		ContentType:  message.ContentType,
		DeliveryMode: amqp091.Persistent,
		Headers:      headers,
		Body:         message.Body,
	}

	if rm.rabbitMQManager != nil {
		return rm.republishMultiTenant(ctx, workerID, queue, message, publishing, span)
	}

	return rm.republishSingleTenant(ctx, workerID, queue, message, publishing, span)
}

// republishMultiTenant republishes the message via tenant-specific vhost channel.
func (rm *ConsumerRetryManager) republishMultiTenant(ctx context.Context, workerID int, queue string, message amqp091.Delivery, publishing amqp091.Publishing, span trace.Span) error {
	tenantID := pkgReporterRabbitmq.TenantIDFromHeaders(message.Headers)
	if tenantID == "" {
		rm.logger.Log(ctx, log.LevelError, "No tenant ID in message headers, cannot republish to correct vhost; sending to DLQ",
			log.Int("worker_id", workerID),
			log.String("queue", queue),
		)
		libOtel.HandleSpanError(span, "No tenant ID for retry republish, routing to DLQ", fmt.Errorf("missing tenant ID in message headers"))
		rm.nackToDLQ(ctx, workerID, queue, message)

		return fmt.Errorf("missing tenant ID")
	}

	tenantChannel, chanErr := rm.rabbitMQManager.GetConnection(ctx, tenantID)
	if chanErr != nil {
		rm.logger.Log(ctx, log.LevelError, "Failed to get tenant channel for retry republish; sending to DLQ",
			log.Int("worker_id", workerID),
			log.String("tenant_id", tenantID),
			log.String("queue", queue),
			log.Err(chanErr),
		)
		libOtel.HandleSpanError(span, "Failed to get tenant channel for retry, routing to DLQ", chanErr)
		rm.nackToDLQ(ctx, workerID, queue, message)

		return chanErr
	}

	publishErr := tenantChannel.Publish(message.Exchange, message.RoutingKey, false, false, publishing)
	if publishErr != nil {
		rm.logPublishFailure(ctx, workerID, queue, publishErr, span)
		rm.nackToDLQ(ctx, workerID, queue, message)

		return publishErr
	}

	return nil
}

// republishSingleTenant republishes the message via the static RabbitMQ connection channel.
func (rm *ConsumerRetryManager) republishSingleTenant(ctx context.Context, workerID int, queue string, message amqp091.Delivery, publishing amqp091.Publishing, span trace.Span) error {
	if rm.conn.Channel == nil {
		rm.logger.Log(ctx, log.LevelError, "Channel is nil, cannot republish for retry; sending to DLQ",
			log.Int("worker_id", workerID),
			log.String("queue", queue),
		)
		libOtel.HandleSpanError(span, "Channel nil, cannot republish for retry, routing to DLQ", fmt.Errorf("rabbitmq channel is nil"))
		rm.nackToDLQ(ctx, workerID, queue, message)

		return fmt.Errorf("channel is nil")
	}

	publishErr := rm.conn.Channel.Publish(message.Exchange, message.RoutingKey, false, false, publishing)
	if publishErr != nil {
		rm.logPublishFailure(ctx, workerID, queue, publishErr, span)
		rm.nackToDLQ(ctx, workerID, queue, message)

		return publishErr
	}

	return nil
}

// nackToDLQ sends a message to the dead-letter queue via Nack without requeue.
func (rm *ConsumerRetryManager) nackToDLQ(ctx context.Context, workerID int, queue string, message amqp091.Delivery) {
	pkgRabbitmq.NackToDLQ(ctx, rm.logger, workerID, queue, message)
}

// logPublishFailure logs republish failure and records it in the OTel span.
func (rm *ConsumerRetryManager) logPublishFailure(ctx context.Context, workerID int, queue string, publishErr error, span trace.Span) {
	rm.logger.Log(ctx, log.LevelError, "Failed to republish message for retry; sending to DLQ",
		log.Int("worker_id", workerID),
		log.String("queue", queue),
		log.Err(publishErr),
	)
	libOtel.HandleSpanError(span, "Failed to republish for retry, routing to DLQ", publishErr)
}
