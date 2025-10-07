// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

package command

import (
	"context"
	"os"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

// TransactionExecute routes transaction processing to sync or async execution based on configuration.
//
// This method determines the execution mode based on the RABBITMQ_TRANSACTION_ASYNC environment variable:
//   - "true": Async execution via RabbitMQ queue (SendBTOExecuteAsync)
//   - "false" or unset: Sync execution directly to database (CreateBTOExecuteSync)
//
// Async Mode:
//   - Publishes transaction to RabbitMQ for async processing
//   - Returns immediately after queue publish
//   - Worker processes balance/transaction/operations later
//   - Better for high throughput, eventual consistency
//
// Sync Mode:
//   - Processes balance/transaction/operations immediately
//   - Returns after database commit
//   - Immediate consistency, lower throughput
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - parseDSL: Parsed DSL transaction specification
//   - validate: Validation responses with calculated amounts
//   - blc: Account balances involved in transaction
//   - tran: Transaction to process
//
// Returns:
//   - error: nil on success, error if processing fails
func (uc *UseCase) TransactionExecute(ctx context.Context, organizationID, ledgerID uuid.UUID, parseDSL *libTransaction.Transaction, validate *libTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		return uc.SendBTOExecuteAsync(ctx, organizationID, ledgerID, parseDSL, validate, blc, tran)
	} else {
		return uc.CreateBTOExecuteSync(ctx, organizationID, ledgerID, parseDSL, validate, blc, tran)
	}
}

// SendBTOExecuteAsync publishes transaction data to RabbitMQ for async processing.
//
// This method implements async transaction processing by:
// 1. Packaging balances, transaction, and DSL into TransactionQueue
// 2. Serializing to msgpack format (efficient binary serialization)
// 3. Publishing to RabbitMQ exchange
// 4. Falling back to sync processing if queue publish fails
//
// Async Processing Flow:
//  1. Transaction validated and created in PENDING status
//  2. BTO (Balance-Transaction-Operation) data published to queue
//  3. Worker consumes message from queue
//  4. Worker updates balances, creates operations
//  5. Worker updates transaction status to APPROVED
//
// Fallback Behavior:
//   - If RabbitMQ publish fails, processes synchronously
//   - Ensures transaction is never lost
//   - Logs warning about fallback
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - parseDSL: Parsed DSL transaction specification
//   - validate: Validation responses with calculated amounts
//   - blc: Account balances involved in transaction
//   - tran: Transaction to process
//
// Returns:
//   - error: nil on success, error if both queue and fallback fail
//
// OpenTelemetry: Creates span "command.send_bto_execute_async"
func (uc *UseCase) SendBTOExecuteAsync(ctx context.Context, organizationID, ledgerID uuid.UUID, parseDSL *libTransaction.Transaction, validate *libTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxSendBTOQueue, spanSendBTOQueue := tracer.Start(ctx, "command.send_bto_execute_async")
	defer spanSendBTOQueue.End()

	queueData := make([]mmodel.QueueData, 0)

	value := transaction.TransactionQueue{
		Validate:    validate,
		Balances:    blc,
		Transaction: tran,
		ParseDSL:    parseDSL,
	}

	marshal, err := msgpack.Marshal(value)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanSendBTOQueue, "Failed to marshal transaction to JSON string", err)

		logger.Errorf("Failed to marshal validate to JSON string: %s", err.Error())

		return err
	}

	queueData = append(queueData, mmodel.QueueData{
		ID:    tran.IDtoUUID(),
		Value: marshal,
	})

	queueMessage := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData:      queueData,
	}

	message, err := msgpack.Marshal(queueMessage)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanSendBTOQueue, "Failed to marshal exchange message struct", err)

		logger.Errorf("Failed to marshal exchange message struct")

		return err
	}

	if _, err := uc.RabbitMQRepo.ProducerDefault(
		ctxSendBTOQueue,
		os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE"),
		os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY"),
		message,
	); err != nil {
		logger.Warnf("Failed to send message to queue: %s", err.Error())

		logger.Infof("Trying to send message directly to database: %s", tran.ID)

		err = uc.CreateBalanceTransactionOperationsAsync(ctxSendBTOQueue, queueMessage)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanSendBTOQueue, "Failed to send message directly to database", err)

			logger.Errorf("Failed to send message directly to database: %s", err.Error())

			return err
		}

		logger.Infof("transaction updated successfully directly to database: %s", tran.ID)

		return nil
	}

	logger.Infof("Transaction send successfully to queue: %s", tran.ID)

	return nil
}

// CreateBTOExecuteSync processes transaction synchronously (direct database execution).
//
// This method implements sync transaction processing by:
// 1. Packaging balances, transaction, and DSL into TransactionQueue
// 2. Calling CreateBalanceTransactionOperationsAsync directly (despite name, runs sync)
// 3. Updating balances and creating operations in the same request context
// 4. Returning after database commit
//
// Sync Processing Flow:
//  1. Transaction validated and created in PENDING status
//  2. BTO data processed immediately
//  3. Balances updated in database
//  4. Operations created in database
//  5. Transaction status updated to APPROVED
//  6. Returns to caller with completed transaction
//
// Use Cases:
//   - When immediate consistency is required
//   - When RabbitMQ is not available
//   - When async processing is disabled
//   - For testing and development
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - parseDSL: Parsed DSL transaction specification
//   - validate: Validation responses with calculated amounts
//   - blc: Account balances involved in transaction
//   - tran: Transaction to process
//
// Returns:
//   - error: nil on success, error if processing fails
//
// OpenTelemetry: Creates span "command.create_bto_execute_sync"
func (uc *UseCase) CreateBTOExecuteSync(ctx context.Context, organizationID, ledgerID uuid.UUID, parseDSL *libTransaction.Transaction, validate *libTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxSendBTODirect, spanSendBTODirect := tracer.Start(ctx, "command.create_bto_execute_sync")
	defer spanSendBTODirect.End()

	queueData := make([]mmodel.QueueData, 0)

	value := transaction.TransactionQueue{
		Validate:    validate,
		Balances:    blc,
		Transaction: tran,
		ParseDSL:    parseDSL,
	}

	marshal, err := msgpack.Marshal(value)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanSendBTODirect, "Failed to marshal transaction to JSON string", err)

		logger.Errorf("Failed to marshal validate to JSON string: %s", err.Error())

		return err
	}

	queueData = append(queueData, mmodel.QueueData{
		ID:    tran.IDtoUUID(),
		Value: marshal,
	})

	queueMessage := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData:      queueData,
	}

	err = uc.CreateBalanceTransactionOperationsAsync(ctxSendBTODirect, queueMessage)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanSendBTODirect, "Failed to send message directly to database", err)

		logger.Errorf("Failed to send message directly to database: %s", err.Error())

		return err
	}

	logger.Infof("Transaction updated successfully directly in database: %s", tran.ID)

	return nil
}
