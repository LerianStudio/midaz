package bootstrap

import (
	"context"
	"encoding/json"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"

	"github.com/LerianStudio/midaz/components/audit/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/rabbitmq/transaction"
	"github.com/LerianStudio/midaz/components/audit/internal/services"
	"github.com/LerianStudio/midaz/pkg"
)

// MultiQueueConsumer represents a multi-queue consumer.
type MultiQueueConsumer struct {
	consumerRoutes *rabbitmq.ConsumerRoutes
	UseCase        *services.UseCase
}

// NewMultiQueueConsumer create a new instance of MultiQueueConsumer.
func NewMultiQueueConsumer(routes *rabbitmq.ConsumerRoutes, useCase *services.UseCase) *MultiQueueConsumer {
	consumer := &MultiQueueConsumer{
		consumerRoutes: routes,
		UseCase:        useCase,
	}

	// Registry handlers for each queue
	routes.Register("audit_queue", consumer.handleAuditQueue)
	routes.Register("transaction_operations_queue", consumer.handleTransactionQueue)

	return consumer
}

// Run starts consumers for all registered queues.
func (mq *MultiQueueConsumer) Run(l *pkg.Launcher) error {
	return mq.consumerRoutes.RunConsumers()
}

// handleAuditQueue process messages from "audit_queue".
func (mq *MultiQueueConsumer) handleAuditQueue(ctx context.Context, body []byte) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "consumer.handleAuditQueue")
	defer span.End()

	logger.Info("Processing message from audit_queue")

	var transactionMessage transaction.Transaction

	err := json.Unmarshal(body, &transactionMessage)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Error unmarshalling transaction message JSON: %v", err)
		logger.Errorf("Error unmarshalling transaction message JSON: %v", err)
		return err
	}

	logger.Infof("Message consumed: %s", transactionMessage.ID)

	mq.UseCase.CreateLog(ctx, transactionMessage)

	return nil
}

// handleTransactionQueue process messages from "transaction_operations_queue".
func (mq *MultiQueueConsumer) handleTransactionQueue(ctx context.Context, body []byte) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "consumer.handleTransactionQueue")
	defer span.End()

	logger.Info("Processing message from queue_transactions")

	logger.Info(string(body))

	return nil
}
