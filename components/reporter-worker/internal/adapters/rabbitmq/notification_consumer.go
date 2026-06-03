// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"

	"github.com/LerianStudio/reporter/pkg/ctxutil"
	pkgHTTP "github.com/LerianStudio/reporter/pkg/net/http"

	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// NotificationHandler defines the contract for processing Fetcher notification messages.
// This is implemented by services.UseCase.ProcessFetcherNotification.
type NotificationHandler interface {
	ProcessFetcherNotification(ctx context.Context, body []byte) error
}

// NotificationConsumerHandler wraps a NotificationHandler to provide RabbitMQ
// message handling with OpenTelemetry instrumentation. This handler is registered
// with the bootstrap consumer to process messages from the fetcher-notification queue.
type NotificationConsumerHandler struct {
	handler NotificationHandler
	logger  log.Logger
}

// NewNotificationConsumerHandler creates a handler that can be registered with
// ConsumerRoutes or MultiQueueConsumer for the fetcher-notification queue.
func NewNotificationConsumerHandler(handler NotificationHandler, logger log.Logger) *NotificationConsumerHandler {
	return &NotificationConsumerHandler{
		handler: handler,
		logger:  logger,
	}
}

// Handle processes a RabbitMQ message body from the fetcher-notification queue.
// It creates an OTel span and delegates to the NotificationHandler.
// This method signature matches pkgRabbitmq.QueueHandlerFunc: func(ctx, []byte) error.
func (h *NotificationConsumerHandler) Handle(ctx context.Context, body []byte) error {
	reqID := ctxutil.HeaderIDFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.notification.fetcher_completion")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))

	h.logger.Log(ctx, log.LevelInfo, "Received Fetcher notification message")

	err := h.handler.ProcessFetcherNotification(ctx, body)
	if err != nil {
		if pkgHTTP.IsBusinessError(err) {
			libOtel.HandleSpanBusinessErrorEvent(span, "Error processing Fetcher notification.", err)
		} else {
			libOtel.HandleSpanError(span, "Error processing Fetcher notification.", err)
		}

		h.logger.Log(ctx, log.LevelError, "Error processing Fetcher notification", log.Err(err))

		return err
	}

	return nil
}
