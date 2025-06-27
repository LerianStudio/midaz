package bootstrap

import (
	"context"
	"encoding/json"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/consumer/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/consumer/internal/services/commands"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"os"
	"os/signal"
	"syscall"
)

// MultiQueueConsumer represents a multi-queue app.
type MultiQueueConsumer struct {
	consumerRoutes *rabbitmq.ConsumerRoutes
	UseCase        *commands.UseCase
}

// NewMultiQueueConsumer create a new instance of MultiQueueConsumer.
func NewMultiQueueConsumer(routes *rabbitmq.ConsumerRoutes, useCase *commands.UseCase) *MultiQueueConsumer {
	consumer := &MultiQueueConsumer{
		consumerRoutes: routes,
		UseCase:        useCase,
	}

	// Registry handlers for each queue
	routes.Register(os.Getenv("RABBITMQ_BALANCE_CREATE_QUEUE"), consumer.handlerBalanceCreateQueue)
	routes.Register(os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE"), consumer.handlerBTOQueue)

	return consumer
}

// Run starts consumers for all registered queues.
func (mq *MultiQueueConsumer) Run(l *libCommons.Launcher) error {
	err := mq.consumerRoutes.RunConsumers()
	if err != nil {
		return err
	}

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	return nil
}

// handlerBalanceCreateQueue processes messages from the audit queue, unmarshal the JSON, and creates balances on database.
func (mq *MultiQueueConsumer) handlerBalanceCreateQueue(ctx context.Context, body []byte) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "app.handler_balance_create_queue")
	defer span.End()

	logger.Info("Processing message from transaction_balance_queue")

	var message mmodel.Queue

	err := json.Unmarshal(body, &message)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error unmarshalling message JSON", err)

		logger.Errorf("Error unmarshalling accounts message JSON: %v", err)

		return err
	}

	logger.Infof("Account message consumed: %s", message.AccountID)

	err = mq.UseCase.CreateBalance(ctx, message)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error creating balance", err)

		logger.Errorf("Error creating balance: %v", err)

		return err
	}

	return nil
}

// handlerBTOQueue processes messages from the balance fifo queue, unmarshal the JSON, and update balances on database.
func (mq *MultiQueueConsumer) handlerBTOQueue(ctx context.Context, body []byte) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "app.handler_balance_update")
	defer span.End()

	logger.Info("Processing message from balance_retry_queue_fifo")

	var message mmodel.Queue

	err := json.Unmarshal(body, &message)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error unmarshalling message JSON", err)

		logger.Errorf("Error unmarshalling balance message JSON: %v", err)

		return err
	}

	logger.Infof("Transaction message consumed: %s", message.QueueData[0].ID)

	err = mq.UseCase.CreateBalanceTransactionOperationsAsync(ctx, message)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error creating transaction", err)

		logger.Errorf("Error creating transaction: %v", err)

		return err
	}

	return nil
}
