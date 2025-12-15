package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/vmihailenco/msgpack/v5"
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
	routes.Register(os.Getenv("RABBITMQ_BALANCE_CREATE_QUEUE"), consumer.handlerBalanceCreateQueue)
	routes.Register(os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE"), consumer.handlerBTOQueue)

	return consumer
}

// Run starts consumers for all registered queues.
func (mq *MultiQueueConsumer) Run(l *libCommons.Launcher) error {
	if err := mq.consumerRoutes.RunConsumers(); err != nil {
		return fmt.Errorf("failed to run consumers: %w", err)
	}

	return nil
}

// isInfrastructureError detects if an error is caused by infrastructure failure
// (PostgreSQL, Redis, RabbitMQ connection issues) that should trigger retries.
// Returns true for retriable infrastructure errors, false for validation errors
// or business logic errors that should fail fast.
func isInfrastructureError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// PostgreSQL connection errors
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "connection closed") ||
		strings.Contains(errStr, "no connection to the server") ||
		strings.Contains(errStr, "server closed the connection") ||
		strings.Contains(errStr, "connection timed out") ||
		strings.Contains(errStr, "could not connect to server") ||
		strings.Contains(errStr, "connection does not exist") {
		return true
	}

	// Redis/Valkey errors
	if strings.Contains(errStr, "redis") ||
		strings.Contains(errStr, "valkey") {
		return true
	}

	// Timeout and deadline errors
	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "context deadline exceeded") ||
		strings.Contains(errStr, "context canceled") {
		return true
	}

	// RabbitMQ errors
	if strings.Contains(errStr, "rabbitmq") ||
		strings.Contains(errStr, "amqp") {
		return true
	}

	return false
}

// handlerBalanceCreateQueue processes messages from the audit queue, unmarshal the JSON, and creates balances on database.
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

		return fmt.Errorf("failed to unmarshal balance create queue message: %w", err)
	}

	logger.Infof("Account message consumed: %s", message.AccountID)

	err = mq.UseCase.CreateBalance(ctx, message)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Error creating balance", err)

		// Log infrastructure vs business errors differently for debugging
		if isInfrastructureError(err) {
			logger.Errorf("Infrastructure error creating balance (will retry): %v", err)
			return fmt.Errorf("infrastructure failure during balance creation for account %s: %w", message.AccountID, err)
		}

		logger.Errorf("Business error creating balance: %v", err)

		return fmt.Errorf("failed to create balance for account %s: %w", message.AccountID, err)
	}

	return nil
}

// handlerBTOQueue processes messages from the balance fifo queue, unmarshal the JSON, and update balances on database.
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

		return fmt.Errorf("failed to unmarshal balance transaction operation message: %w", err)
	}

	logger.Infof("Transaction message consumed: %s", message.QueueData[0].ID)

	err = mq.UseCase.CreateBalanceTransactionOperationsAsync(ctx, message)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Error creating transaction", err)

		// Log infrastructure vs business errors differently for debugging
		if isInfrastructureError(err) {
			logger.Errorf("Infrastructure error creating transaction (will retry): %v", err)
			return fmt.Errorf("infrastructure failure during balance operation for transaction %s: %w", message.QueueData[0].ID, err)
		}

		logger.Errorf("Business error creating transaction: %v", err)

		return fmt.Errorf("failed to create balance transaction operations for transaction %s: %w", message.QueueData[0].ID, err)
	}

	return nil
}
