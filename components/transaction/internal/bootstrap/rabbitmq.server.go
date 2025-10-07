// Package bootstrap provides application initialization and dependency injection for the transaction service.
// This file defines the RabbitMQ consumer component.
package bootstrap

import (
	"context"
	"encoding/json"
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/vmihailenco/msgpack/v5"
)

// MultiQueueConsumer represents a RabbitMQ consumer that processes multiple queues.
//
// This component handles asynchronous message processing from RabbitMQ queues:
//   - Balance creation queue: Processes account creation events from onboarding service
//   - BTO queue: Processes balance/transaction/operation updates
//
// The consumer uses multiple workers per queue for parallel processing.
type MultiQueueConsumer struct {
	consumerRoutes *rabbitmq.ConsumerRoutes
	UseCase        *command.UseCase
}

// NewMultiQueueConsumer creates a new RabbitMQ consumer instance.
//
// This function:
// 1. Creates the consumer struct
// 2. Registers handler functions for each queue
// 3. Maps queue names from environment variables to handler functions
//
// Registered Queues:
//   - RABBITMQ_BALANCE_CREATE_QUEUE: Account creation events
//   - RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE: Transaction processing
//
// Parameters:
//   - routes: ConsumerRoutes instance with queue configuration
//   - useCase: Command use case for business logic
//
// Returns:
//   - *MultiQueueConsumer: Configured consumer ready to run
func NewMultiQueueConsumer(routes *rabbitmq.ConsumerRoutes, useCase *command.UseCase) *MultiQueueConsumer {
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
//
// This method starts RabbitMQ consumers for all registered queues. Each queue
// runs with configured number of workers (RABBITMQ_NUMBERS_OF_WORKERS) and
// prefetch count (RABBITMQ_NUMBERS_OF_PREFETCH).
//
// The method blocks until shutdown signal is received.
//
// Parameters:
//   - l: Launcher instance (unused in current implementation)
//
// Returns:
//   - error: Error if consumer startup fails
func (mq *MultiQueueConsumer) Run(l *libCommons.Launcher) error {
	return mq.consumerRoutes.RunConsumers()
}

// handlerBalanceCreateQueue processes account creation events from the onboarding service.
//
// This handler:
// 1. Unmarshals JSON message to Queue struct
// 2. Calls CreateBalance to initialize account balances
// 3. Returns error if processing fails (message will be requeued)
//
// Message Format:
//   - JSON-encoded mmodel.Queue with account details
//   - Sent by onboarding service when accounts are created
//
// Processing:
//   - Creates initial balance entries for new accounts
//   - Initializes available and on-hold amounts to zero
//   - Sets up balance tracking for transaction processing
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - body: JSON message body from RabbitMQ
//
// Returns:
//   - error: nil on success, error if processing fails (triggers requeue)
//
// OpenTelemetry: Creates span "consumer.handler_balance_create_queue"
func (mq *MultiQueueConsumer) handlerBalanceCreateQueue(ctx context.Context, body []byte) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "consumer.handler_balance_create_queue")
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
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Error creating balance", err)

		logger.Errorf("Error creating balance: %v", err)

		return err
	}

	return nil
}

// handlerBTOQueue processes balance/transaction/operation updates from async queue.
//
// This handler:
// 1. Unmarshals msgpack message to Queue struct
// 2. Calls CreateBalanceTransactionOperationsAsync to process transaction
// 3. Returns error if processing fails (message will be requeued)
//
// Message Format:
//   - Msgpack-encoded mmodel.Queue with transaction data
//   - Sent by transaction service for async processing
//
// Processing:
//   - Updates account balances
//   - Creates transaction record
//   - Creates operation records
//   - Publishes events and audit logs
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - body: Msgpack message body from RabbitMQ
//
// Returns:
//   - error: nil on success, error if processing fails (triggers requeue)
//
// OpenTelemetry: Creates span "consumer.handler_balance_update"
func (mq *MultiQueueConsumer) handlerBTOQueue(ctx context.Context, body []byte) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "consumer.handler_balance_update")
	defer span.End()

	logger.Info("Processing message from balance_retry_queue_fifo")

	var message mmodel.Queue

	err := msgpack.Unmarshal(body, &message)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error unmarshalling message JSON", err)

		logger.Errorf("Error unmarshalling balance message JSON: %v", err)

		return err
	}

	logger.Infof("Transaction message consumed: %s", message.QueueData[0].ID)

	err = mq.UseCase.CreateBalanceTransactionOperationsAsync(ctx, message)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Error creating transaction", err)

		logger.Errorf("Error creating transaction: %v", err)

		return err
	}

	return nil
}
