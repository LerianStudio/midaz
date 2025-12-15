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

// Infrastructure error patterns for different failure types
var (
	postgresPatterns = []string{
		"connection refused",
		"connection reset",
		"connection closed",
		"no connection to the server",
		"server closed the connection",
		"connection timed out",
		"could not connect to server",
		"connection does not exist",
	}

	redisPatterns = []string{
		"redis",
		"valkey",
	}

	timeoutPatterns = []string{
		"timeout",
		"deadline exceeded",
		"context deadline exceeded",
		"context canceled",
	}

	rabbitmqPatterns = []string{
		"rabbitmq",
		"amqp",
	}
)

// containsAnyPattern checks if s contains any of the specified patterns.
func containsAnyPattern(s string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(s, pattern) {
			return true
		}
	}

	return false
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

	// Check against all infrastructure error patterns
	return containsAnyPattern(errStr, postgresPatterns) ||
		containsAnyPattern(errStr, redisPatterns) ||
		containsAnyPattern(errStr, timeoutPatterns) ||
		containsAnyPattern(errStr, rabbitmqPatterns)
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
