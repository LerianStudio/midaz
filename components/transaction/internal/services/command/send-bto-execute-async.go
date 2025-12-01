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

// TransactionExecute routes transaction execution to sync or async processing.
//
// This function is the main entry point for transaction execution after validation.
// It checks the RABBITMQ_TRANSACTION_ASYNC environment variable to determine
// whether to process transactions via message queue (async) or directly (sync).
//
// Execution Modes:
//
//	Async (RABBITMQ_TRANSACTION_ASYNC=true):
//	  - Sends transaction to RabbitMQ for background processing
//	  - Returns immediately after queue publish (non-blocking)
//	  - Higher throughput, eventual consistency
//	  - Fallback to sync if queue publish fails
//
//	Sync (RABBITMQ_TRANSACTION_ASYNC=false or unset):
//	  - Processes transaction directly in the request
//	  - Blocks until completion (blocking)
//	  - Immediate consistency, lower throughput
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - parseDSL: Parsed transaction DSL containing send/distribute rules
//   - validate: Validation results from DSL parsing
//   - blc: Slice of balances affected by this transaction
//   - tran: Transaction record to process
//
// Returns:
//   - error: Execution or queue publish error
//
// Environment Variables:
//
//	RABBITMQ_TRANSACTION_ASYNC=true|false (default: false)
func (uc *UseCase) TransactionExecute(ctx context.Context, organizationID, ledgerID uuid.UUID, parseDSL *libTransaction.Transaction, validate *libTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		return uc.SendBTOExecuteAsync(ctx, organizationID, ledgerID, parseDSL, validate, blc, tran)
	} else {
		return uc.CreateBTOExecuteSync(ctx, organizationID, ledgerID, parseDSL, validate, blc, tran)
	}
}

// SendBTOExecuteAsync sends balance, transaction, and operations to RabbitMQ for async processing.
//
// This function publishes transaction data to a RabbitMQ exchange for background
// processing by worker consumers. It provides high-throughput transaction handling
// by offloading the actual persistence work to dedicated workers.
//
// Message Publishing Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for distributed tracing
//
//	Step 2: Build Queue Message
//	  - Package transaction data into TransactionQueue struct
//	  - Include validation results, balances, and parsed DSL
//	  - Serialize using msgpack for efficient transport
//
//	Step 3: Wrap in Queue Envelope
//	  - Add organization and ledger context
//	  - Include transaction ID for routing/tracking
//
//	Step 4: Publish to RabbitMQ
//	  - Send to configured exchange with routing key
//	  - On failure: Fall back to synchronous processing
//
// Fallback Behavior:
//
// If RabbitMQ publish fails, the function automatically falls back to
// synchronous processing via CreateBalanceTransactionOperationsAsync.
// This ensures transactions are never lost due to queue unavailability.
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for message routing
//   - ledgerID: Ledger scope for message routing
//   - parseDSL: Parsed transaction DSL for worker processing
//   - validate: Validation results from DSL parsing
//   - blc: Slice of balances to update
//   - tran: Transaction record to persist
//
// Returns:
//   - error: Serialization error or fallback processing error
//
// Environment Variables:
//
//	RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE: Exchange name
//	RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY: Routing key
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

// CreateBTOExecuteSync processes balance, transaction, and operations synchronously.
//
// This function provides synchronous transaction processing when async mode is
// disabled or unavailable. It uses the same processing logic as the async path
// but executes immediately in the request context.
//
// Synchronous Processing:
//
// While this method is named "sync", it still uses the CreateBalanceTransactionOperationsAsync
// function internally. The "sync" refers to the calling pattern (blocking, in-request)
// rather than the processing implementation.
//
// Use Cases:
//   - Development/testing environments without RabbitMQ
//   - Low-volume deployments where queue overhead isn't beneficial
//   - Fallback when async processing fails
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - parseDSL: Parsed transaction DSL containing send/distribute rules
//   - validate: Validation results from DSL parsing
//   - blc: Slice of balances affected by this transaction
//   - tran: Transaction record to process
//
// Returns:
//   - error: Processing or database error
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
