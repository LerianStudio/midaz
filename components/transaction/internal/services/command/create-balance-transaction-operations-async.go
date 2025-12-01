package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/otel/trace"
)

// CreateBalanceTransactionOperationsAsync processes a transaction queue message asynchronously.
//
// This is the main worker function for processing transactions from RabbitMQ.
// It handles the complete transaction lifecycle: balance updates, transaction
// creation/update, operation creation, metadata storage, and event publishing.
//
// Processing Pipeline:
//
//	Step 1: Deserialize Queue Message
//	  - Unmarshal msgpack-encoded TransactionQueue from queue data
//	  - Extract transaction, balances, and validation results
//
//	Step 2: Update Balances (if not NOTED status)
//	  - Apply balance changes to affected accounts
//	  - NOTED transactions skip balance updates (audit-only)
//
//	Step 3: Create or Update Transaction
//	  - Insert new transaction record
//	  - Handle duplicate key (idempotency) by updating status
//	  - Map CREATED -> APPROVED, PENDING -> preserve body
//
//	Step 4: Create Transaction Metadata
//	  - Store transaction metadata in MongoDB
//	  - Link metadata to transaction by entity ID
//
//	Step 5: Create Operations
//	  - Create operation records for each balance affected
//	  - Skip duplicates (unique violation = already processed)
//	  - Store operation metadata in MongoDB
//
//	Step 6: Post-Processing (async)
//	  - Send transaction events to RabbitMQ (notifications)
//	  - Remove transaction from Redis pending queue
//
// Idempotency Handling:
//
// This function is idempotent - processing the same message multiple times
// produces the same result. This is achieved through:
//   - Unique constraints on transaction and operation IDs
//   - Skip-on-duplicate logic for already-processed records
//   - Status-based conditional updates
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - data: Queue message containing transaction data and organization context
//
// Returns:
//   - error: Processing error (triggers message retry/DLQ)
//
// Error Scenarios:
//   - Deserialization error: Corrupted queue message
//   - Balance update error: Insufficient funds, locked account
//   - Database error: PostgreSQL or MongoDB unavailable
//   - Unique violation (handled): Skip and continue processing
func (uc *UseCase) CreateBalanceTransactionOperationsAsync(ctx context.Context, data mmodel.Queue) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	var t transaction.TransactionQueue

	for _, item := range data.QueueData {
		logger.Infof("Unmarshal account ID: %v", item.ID.String())

		err := msgpack.Unmarshal(item.Value, &t)
		if err != nil {
			logger.Errorf("failed to unmarshal response: %v", err.Error())

			return err
		}
	}

	if t.Transaction.Status.Code != constant.NOTED {
		ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.create_balance_transaction_operations.update_balances")
		defer spanUpdateBalances.End()

		logger.Infof("Trying to update balances")

		err := uc.UpdateBalances(ctxProcessBalances, data.OrganizationID, data.LedgerID, *t.Validate, t.Balances)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "Failed to update balances", err)

			logger.Errorf("Failed to update balances: %v", err.Error())

			return err
		}
	}

	ctxProcessTransaction, spanUpdateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanUpdateTransaction.End()

	tran, err := uc.CreateOrUpdateTransaction(ctxProcessTransaction, logger, tracer, t)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateTransaction, "Failed to create or update transaction", err)

		logger.Errorf("Failed to create or update transaction: %v", err.Error())

		return err
	}

	ctxProcessMetadata, spanCreateMetadata := tracer.Start(ctx, "command.create_balance_transaction_operations.create_metadata")
	defer spanCreateMetadata.End()

	err = uc.CreateMetadataAsync(ctxProcessMetadata, logger, tran.Metadata, tran.ID, reflect.TypeOf(transaction.Transaction{}).Name())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateMetadata, "Failed to create metadata on transaction", err)

		logger.Errorf("Failed to create metadata on transaction: %v", err.Error())

		return err
	}

	ctxProcessOperation, spanCreateOperation := tracer.Start(ctx, "command.create_balance_transaction_operations.create_operation")
	defer spanCreateOperation.End()

	logger.Infof("Trying to create new operations")

	for _, oper := range tran.Operations {
		_, err = uc.OperationRepo.Create(ctxProcessOperation, oper)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
				msg := fmt.Sprintf("Skipping to create operation, operation already exists: %v", oper.ID)

				libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, msg, err)

				logger.Warnf(msg)

				continue
			} else {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, "Failed to create operation", err)

				logger.Errorf("Error creating operation: %v", err)

				return err
			}
		}

		err = uc.CreateMetadataAsync(ctx, logger, oper.Metadata, oper.ID, reflect.TypeOf(operation.Operation{}).Name())
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, "Failed to create metadata on operation", err)

			logger.Errorf("Failed to create metadata on operation: %v", err)

			return err
		}
	}

	go uc.SendTransactionEvents(ctx, tran)

	go uc.RemoveTransactionFromRedisQueue(ctx, logger, data.OrganizationID, data.LedgerID, tran.ID)

	return nil
}

// CreateOrUpdateTransaction creates a new transaction or updates an existing one.
//
// This function handles transaction persistence with idempotency support.
// If a transaction already exists (unique key violation), it updates the
// status instead of failing, supporting retry scenarios.
//
// Status Transitions:
//
//	CREATED -> APPROVED: Normal transaction flow
//	PENDING -> preserve body: Transaction awaiting confirmation
//	APPROVED/CANCELED (retry): Update existing pending transaction
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - logger: Logger instance for structured logging
//   - tracer: OpenTelemetry tracer for distributed tracing
//   - t: TransactionQueue containing transaction data and validation results
//
// Returns:
//   - *transaction.Transaction: Created or updated transaction
//   - error: Database or validation error
//
// Error Scenarios:
//   - Database error: PostgreSQL unavailable
//   - Unique violation (handled): Updates existing transaction status
func (uc *UseCase) CreateOrUpdateTransaction(ctx context.Context, logger libLog.Logger, tracer trace.Tracer, t transaction.TransactionQueue) (*transaction.Transaction, error) {
	_, spanCreateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanCreateTransaction.End()

	logger.Infof("Trying to create new transaction")

	tran := t.Transaction
	tran.Body = libTransaction.Transaction{}

	switch tran.Status.Code {
	case constant.CREATED:
		description := constant.APPROVED
		status := transaction.Status{
			Code:        description,
			Description: &description,
		}

		tran.Status = status
	case constant.PENDING:
		tran.Body = *t.ParseDSL
	}

	_, err := uc.TransactionRepo.Create(ctx, tran)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
			if t.Validate.Pending && (tran.Status.Code == constant.APPROVED || tran.Status.Code == constant.CANCELED) {
				_, err = uc.UpdateTransactionStatus(ctx, tran)
				if err != nil {
					libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateTransaction, "Failed to update transaction", err)

					logger.Warnf("Failed to update transaction with STATUS: %v by ID: %v", tran.Status.Code, tran.ID)

					return nil, err
				}
			}

			logger.Infof("skipping to create transaction, transaction already exists: %v", tran.ID)
		} else {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateTransaction, "Failed to create transaction on repo", err)

			logger.Errorf("Failed to create transaction on repo: %v", err.Error())

			return nil, err
		}
	}

	return tran, nil
}

// CreateMetadataAsync creates metadata for an entity asynchronously.
//
// Metadata provides extensible key-value storage for transactions and operations,
// allowing clients to attach custom data without schema changes.
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - logger: Logger instance for error reporting
//   - metadata: Key-value metadata map (nil = no-op)
//   - ID: Entity ID to associate metadata with
//   - collection: MongoDB collection name (Transaction or Operation)
//
// Returns:
//   - error: MongoDB operation error
func (uc *UseCase) CreateMetadataAsync(ctx context.Context, logger libLog.Logger, metadata map[string]any, ID string, collection string) error {
	if metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   ID,
			EntityName: collection,
			Data:       metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, collection, &meta); err != nil {
			logger.Errorf("Error into creating %s metadata: %v", collection, err)

			return err
		}
	}

	return nil
}

// CreateBTOSync processes a transaction synchronously (blocking).
//
// This is a wrapper around CreateBalanceTransactionOperationsAsync for
// synchronous execution paths. Used when RABBITMQ_TRANSACTION_ASYNC=false.
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - data: Queue message containing transaction data
//
// Returns:
//   - error: Processing error
func (uc *UseCase) CreateBTOSync(ctx context.Context, data mmodel.Queue) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_balance_transaction_operations.create_bto_sync")
	defer span.End()

	err := uc.CreateBalanceTransactionOperationsAsync(ctx, data)
	if err != nil {
		logger.Errorf("Failed to create balance transaction operations: %v", err)

		return err
	}

	return nil
}

// RemoveTransactionFromRedisQueue removes a processed transaction from the Redis pending queue.
//
// After successful processing, transactions are removed from the Redis queue
// to prevent reprocessing and free up memory. This is called asynchronously
// to avoid blocking the main processing flow.
//
// Parameters:
//   - ctx: Request context (may be background context for async)
//   - logger: Logger instance for error reporting
//   - organizationID: Organization scope for queue key
//   - ledgerID: Ledger scope for queue key
//   - transactionID: Transaction ID to remove
func (uc *UseCase) RemoveTransactionFromRedisQueue(ctx context.Context, logger libLog.Logger, organizationID, ledgerID uuid.UUID, transactionID string) {
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID)

	if err := uc.RedisRepo.RemoveMessageFromQueue(ctx, transactionKey); err != nil {
		logger.Warnf("err to remove message on redis: %s", err.Error())
	} else {
		logger.Infof("message removed from redis successfully: %s", transactionKey)
	}
}

// SendTransactionToRedisQueue adds a transaction to the Redis pending queue.
//
// Transactions are queued in Redis for tracking and recovery purposes.
// If the message queue fails, the transaction can be recovered from Redis
// and reprocessed, ensuring no transactions are lost.
//
// Queue Entry Structure:
//
//	{
//	  "header_id": "request-id",
//	  "organization_id": "...",
//	  "ledger_id": "...",
//	  "transaction_id": "...",
//	  "parser_dsl": {...},
//	  "validate": {...},
//	  "ttl": "timestamp",
//	  "transaction_status": "CREATED",
//	  "transaction_date": "timestamp"
//	}
//
// Parameters:
//   - ctx: Request context with request ID for tracing
//   - organizationID: Organization scope for queue key
//   - ledgerID: Ledger scope for queue key
//   - transactionID: Unique transaction identifier
//   - parserDSL: Parsed transaction DSL for reprocessing
//   - validate: Validation results from transaction parsing
//   - transactionStatus: Current transaction status
//   - transactionDate: Transaction creation timestamp
func (uc *UseCase) SendTransactionToRedisQueue(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, parserDSL libTransaction.Transaction, validate *libTransaction.Responses, transactionStatus string, transactionDate time.Time) {
	logger, _, reqId, _ := libCommons.NewTrackingFromContext(ctx)
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

	queue := mmodel.TransactionRedisQueue{
		HeaderID:          reqId,
		OrganizationID:    organizationID,
		LedgerID:          ledgerID,
		TransactionID:     transactionID,
		ParserDSL:         parserDSL,
		TTL:               time.Now(),
		Validate:          validate,
		TransactionStatus: transactionStatus,
		TransactionDate:   transactionDate,
	}

	raw, err := json.Marshal(queue)
	if err != nil {
		logger.Warnf("Failed to marshal transaction to json string: %s", err.Error())
	}

	err = uc.RedisRepo.AddMessageToQueue(ctx, transactionKey, raw)
	if err != nil {
		logger.Warnf("Failed to send transaction to redis queue: %s", err.Error())
	}
}
