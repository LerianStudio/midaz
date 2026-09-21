// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

type MultiQueueConsumer struct {
	consumerRoutes *rabbitmq.ConsumerRoutes
	UseCase        *command.UseCase
	metricsFactory *metrics.MetricsFactory
	dispatcher     *rabbitTransactionDispatcher
}

// NewMultiQueueConsumer create a new instance of MultiQueueConsumer.
// The metricsFactory parameter can be nil when telemetry is disabled.
func NewMultiQueueConsumer(routes *rabbitmq.ConsumerRoutes, useCase *command.UseCase, bulkRequired bool, metricsFactory *metrics.MetricsFactory) (*MultiQueueConsumer, error) {
	if routes == nil {
		return nil, fmt.Errorf("rabbitmq consumer routes are not configured")
	}

	dispatcher, err := newRabbitTransactionDispatcher(useCase, rabbitEngineSingleTenant, bulkRequired, metricsFactory)
	if err != nil {
		return nil, fmt.Errorf("configure rabbitmq transaction dispatcher: %w", err)
	}

	consumer := &MultiQueueConsumer{
		consumerRoutes: routes,
		UseCase:        useCase,
		metricsFactory: metricsFactory,
		dispatcher:     dispatcher,
	}

	queueName := os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE")

	// Register individual handler (used for non-bulk mode and as fallback)
	routes.Register(queueName, consumer.handlerBTOQueue)

	// Register bulk handler (used when bulk mode is enabled)
	routes.RegisterBulk(queueName, consumer.handlerBTOBulkQueue)

	return consumer, nil
}

// Run starts consumers for all registered queues.
func (mq *MultiQueueConsumer) Run(l *libCommons.Launcher) error {
	return mq.consumerRoutes.RunConsumers()
}

// handlerBTOQueue processes messages from the balance fifo queue, unmarshal the JSON, and update balances on database.
func (mq *MultiQueueConsumer) handlerBTOQueue(ctx context.Context, body []byte) error {
	if mq == nil || mq.dispatcher == nil {
		return errRabbitTransactionCompleterNotConfigured
	}

	return mq.dispatcher.handle(ctx, body)
}

// handlerBTOBulkQueue processes a batch of messages from the balance queue.
// Returns per-message results for acknowledgment handling.
func (mq *MultiQueueConsumer) handlerBTOBulkQueue(ctx context.Context, messages []amqp.Delivery) ([]rabbitmq.BulkMessageResult, error) {
	if mq == nil || mq.dispatcher == nil {
		return nil, errRabbitTransactionBulkCompleterNotConfigured
	}

	return mq.dispatcher.handleBulk(ctx, messages)
}
