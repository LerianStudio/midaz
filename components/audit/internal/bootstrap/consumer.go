package bootstrap

import (
	"context"
	"encoding/json"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"os"

	"github.com/LerianStudio/midaz/components/audit/internal/adapters/rabbitmq"
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
	routes.Register(os.Getenv("RABBITMQ_QUEUE"), consumer.handleAuditQueue)

	return consumer
}

// Run starts consumers for all registered queues.
func (mq *MultiQueueConsumer) Run(l *pkg.Launcher) error {
	return mq.consumerRoutes.RunConsumers()
}

// handleAuditQueue process messages from queue.
func (mq *MultiQueueConsumer) handleAuditQueue(ctx context.Context, body []byte) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "consumer.handleAuditQueue")
	defer span.End()

	logger.Info("Processing message from queue")

	var message mmodel.Queue

	err := json.Unmarshal(body, &message)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Error unmarshalling message JSON", err)

		logger.Errorf("Error unmarshalling transaction message JSON: %v", err)

		return err
	}

	logger.Infof("Message consumed: %s", message.AuditID)

	err = mq.UseCase.CreateLog(ctx, message.OrganizationID, message.LedgerID, message.AuditID, message.QueueData)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Error creating log", err)

		logger.Errorf("Error creating log: %v", err)

		return err
	}

	return nil
}
