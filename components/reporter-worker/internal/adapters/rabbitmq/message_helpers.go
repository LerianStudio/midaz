// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"

	pkgConstant "github.com/LerianStudio/reporter/pkg/constant"

	"github.com/LerianStudio/lib-commons/v5/commons"
	constant "github.com/LerianStudio/lib-commons/v5/commons/constants"
	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"

	"go.opentelemetry.io/otel/attribute"
)

// buildMessageContext creates a context with request ID and logger from a message.
func buildMessageContext(logger log.Logger, message amqp091.Delivery) (context.Context, string) {
	if message.Headers == nil {
		message.Headers = amqp091.Table{}
	}

	requestIDStr := extractRequestID(message.Headers)

	logWithFields := logger
	if logWithFields != nil {
		logWithFields = logWithFields.With(log.String(constant.HeaderID, requestIDStr))
	}

	ctx := libObservability.ContextWithLogger(
		libObservability.ContextWithHeaderID(context.Background(), requestIDStr),
		logWithFields,
	)

	return ctx, requestIDStr
}

// extractRequestID reads or generates a request ID from message headers.
func extractRequestID(headers amqp091.Table) string {
	if val, found := headers[constant.HeaderID]; found {
		if str, ok := val.(string); ok {
			return str
		}
	}

	generatedID, err := commons.GenerateUUIDv7()
	if err != nil {
		generatedID = uuid.New()
	}

	return generatedID.String()
}

// deliveryTelemetryAttributes extracts span attributes from a message delivery.
func deliveryTelemetryAttributes(message amqp091.Delivery) []attribute.KeyValue {
	_, hasTenantHeader := message.Headers[pkgConstant.HeaderXTenantID]

	return []attribute.KeyValue{
		attribute.String("app.request.rabbitmq.consumer.exchange", message.Exchange),
		attribute.String("app.request.rabbitmq.consumer.routing_key", message.RoutingKey),
		attribute.String("app.request.rabbitmq.consumer.content_type", message.ContentType),
		attribute.Int("app.request.rabbitmq.consumer.body_size_bytes", len(message.Body)),
		attribute.Int("app.request.rabbitmq.consumer.header_count", len(message.Headers)),
		attribute.Bool("app.request.rabbitmq.consumer.has_tenant_header", hasTenantHeader),
	}
}

// recoverWorkerPanic recovers from panics in worker goroutines.
func recoverWorkerPanic(logger log.Logger, workerID int, queue string) {
	if r := recover(); r != nil {
		if logger == nil {
			return
		}

		logger.Log(context.Background(), log.LevelError, "Panic recovered in RabbitMQ worker",
			log.Int("worker_id", workerID),
			log.String("queue", queue),
			log.Any("panic", r),
		)
	}
}
