package bootstrap

import (
	"context"
	"encoding/json"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"os"
)

// MultiQueueConsumer represents a multi-queue consumer.
type MultiQueueConsumer struct {
	consumerRoutes *rabbitmq.ConsumerRoutes
	UseCase        *command.UseCase
}

// NewMultiQueueConsumer create a new instance of MultiQueueConsumer.
func NewMultiQueueConsumer(routes *rabbitmq.ConsumerRoutes, useCase *command.UseCase) *MultiQueueConsumer {
	consumer := &MultiQueueConsumer{
		consumerRoutes: routes,
		UseCase:        useCase,
	}

	// Registry handlers for each queue
	routes.Register(os.Getenv("RABBITMQ_QUEUE"), consumer.handlerBalanceQueue)

	return consumer
}

// Run starts consumers for all registered queues.
func (mq *MultiQueueConsumer) Run(l *pkg.Launcher) error {
	return mq.consumerRoutes.RunConsumers()
}

// handlerBalanceQueue processes messages from the audit queue, unmarshals the JSON, and creates logs in the system.
// It uses context for logging and tracing, and handles errors during unmarshalling or log creation.
func (mq *MultiQueueConsumer) handlerBalanceQueue(ctx context.Context, body []byte) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "consumer.handlerBalanceQueue")
	defer span.End()

	logger.Info("Processing message from transaction_balance_queue")

	var message mmodel.Queue

	err := json.Unmarshal(body, &message)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Error unmarshalling message JSON", err)

		logger.Errorf("Error unmarshalling transaction message JSON: %v", err)

		return err
	}

	logger.Infof("Account message consumed: %s", message.AccountID)

	err = mq.UseCase.CreateBalance(ctx, message)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Error creating balance", err)

		logger.Errorf("Error creating balance: %v", err)

		return err
	}

	return nil
}
