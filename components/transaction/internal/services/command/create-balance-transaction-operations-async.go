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

// CreateBalanceTransactionOperationsAsync processes a transaction queue message and atomically updates balances.
//
// This is the **MOST CRITICAL FUNCTION** in the transaction service and the financial core of Midaz.
// It orchestrates the complete transaction processing pipeline with atomic balance updates.
//
// Transaction Processing Flow:
// 1. Deserialize transaction from msgpack queue message
// 2. Update account balances atomically using Redis Lua scripts (for non-NOTED transactions)
// 3. Create or update transaction record in PostgreSQL
// 4. Create transaction metadata in MongoDB
// 5. Create operation records (debits/credits) in PostgreSQL
// 6. Create operation metadata in MongoDB
// 7. Publish transaction events to RabbitMQ for downstream systems
// 8. Remove processed transaction from Redis queue
//
// Critical Financial Guarantees:
// - Balance updates use Redis Lua scripts for atomicity (no race conditions)
// - Optimistic locking (version field) prevents concurrent modification issues
// - NOTED transactions bypass balance updates (informational only)
// - Idempotency: duplicate operations are safely skipped (unique constraint check)
// - All database operations are within implicit transactions for consistency
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - data: Queue message containing transaction data from Redis
//
// Returns:
//   - error: Balance update failures, persistence errors, or validation failures
func (uc *UseCase) CreateBalanceTransactionOperationsAsync(ctx context.Context, data mmodel.Queue) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	var t transaction.TransactionQueue

	// Step 1: Deserialize transaction queue message from msgpack format.
	// Msgpack is used for efficient binary serialization in Redis.
	for _, item := range data.QueueData {
		logger.Infof("Unmarshal account ID: %v", item.ID.String())

		err := msgpack.Unmarshal(item.Value, &t)
		if err != nil {
			logger.Errorf("failed to unmarshal response: %v", err.Error())

			return err
		}
	}

	// Step 2: Update account balances atomically unless transaction is NOTED.
	// NOTED transactions are informational only and don't affect balances.
	// This uses Redis Lua scripts to ensure atomic balance updates across multiple accounts.
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

	// Step 3: Create or update transaction record in PostgreSQL.
	// Handles both initial creation and status updates (e.g., PENDING -> APPROVED).
	ctxProcessTransaction, spanUpdateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanUpdateTransaction.End()

	tran, err := uc.CreateOrUpdateTransaction(ctxProcessTransaction, logger, tracer, t)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateTransaction, "Failed to create or update transaction", err)

		logger.Errorf("Failed to create or update transaction: %v", err.Error())

		return err
	}

	// Step 4: Store transaction metadata in MongoDB
	ctxProcessMetadata, spanCreateMetadata := tracer.Start(ctx, "command.create_balance_transaction_operations.create_metadata")
	defer spanCreateMetadata.End()

	err = uc.CreateMetadataAsync(ctxProcessMetadata, logger, tran.Metadata, tran.ID, reflect.TypeOf(transaction.Transaction{}).Name())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateMetadata, "Failed to create metadata on transaction", err)

		logger.Errorf("Failed to create metadata on transaction: %v", err.Error())

		return err
	}

	// Step 5: Create operation records (individual debits and credits).
	// Each transaction consists of multiple operations representing the double-entry accounting.
	// Duplicate operations are safely skipped using unique constraint checking.
	ctxProcessOperation, spanCreateOperation := tracer.Start(ctx, "command.create_balance_transaction_operations.create_operation")
	defer spanCreateOperation.End()

	logger.Infof("Trying to create new operations")

	for _, oper := range tran.Operations {
		_, err = uc.OperationRepo.Create(ctxProcessOperation, oper)
		if err != nil {
			var pgErr *pgconn.PgError
			// Handle idempotent operation creation: skip if already exists
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

		// Step 6: Store operation metadata in MongoDB
		err = uc.CreateMetadataAsync(ctx, logger, oper.Metadata, oper.ID, reflect.TypeOf(operation.Operation{}).Name())
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, "Failed to create metadata on operation", err)

			logger.Errorf("Failed to create metadata on operation: %v", err)

			return err
		}
	}

	// Step 7: Publish transaction events to RabbitMQ for downstream consumers (async, non-blocking)
	go uc.SendTransactionEvents(ctx, tran)

	// Step 8: Remove processed transaction from Redis queue (async cleanup)
	go uc.RemoveTransactionFromRedisQueue(ctx, logger, data.OrganizationID, data.LedgerID, tran.ID)

	return nil
}

// CreateOrUpdateTransaction persists or updates a transaction record with idempotent handling.
//
// This function handles the transaction lifecycle transitions:
// - CREATED status: Transition to APPROVED when balance updates succeed
// - PENDING status: Preserve DSL body for potential commit/cancel operations
// - Existing transactions: Update status if transitioning from PENDING to APPROVED/CANCELED
//
// Idempotency guarantees:
// - Duplicate creation attempts (unique constraint violation) are safely handled
// - For pending transactions, status updates are applied when committing/canceling
// - Transaction body preserves the Gold DSL for pending transactions
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - logger: Structured logger for operation tracking
//   - tracer: OpenTelemetry tracer for distributed tracing
//   - t: Transaction queue data including validation results and parsed DSL
//
// Returns:
//   - *transaction.Transaction: The created or updated transaction entity
//   - error: Persistence errors or status update failures
func (uc *UseCase) CreateOrUpdateTransaction(ctx context.Context, logger libLog.Logger, tracer trace.Tracer, t transaction.TransactionQueue) (*transaction.Transaction, error) {
	_, spanCreateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanCreateTransaction.End()

	logger.Infof("Trying to create new transaction")

	tran := t.Transaction
	tran.Body = libTransaction.Transaction{}

	// Determine transaction status and body based on current state
	switch tran.Status.Code {
	case constant.CREATED:
		// Transition CREATED -> APPROVED after successful balance updates
		description := constant.APPROVED
		status := transaction.Status{
			Code:        description,
			Description: &description,
		}

		tran.Status = status
	case constant.PENDING:
		// Preserve DSL body for pending transactions (needed for commit/cancel)
		tran.Body = *t.ParseDSL
	}

	// Attempt to create transaction
	_, err := uc.TransactionRepo.Create(ctx, tran)
	if err != nil {
		var pgErr *pgconn.PgError
		// Handle idempotent creation: transaction may already exist
		if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
			// If this was a pending transaction being committed/canceled, update its status
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

// CreateMetadataAsync stores metadata for transactions or operations in MongoDB.
//
// This function is called during async transaction processing to store custom
// metadata for both transactions and their individual operations. It enforces
// a maximum key length of 100 characters to prevent metadata bloat.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - logger: Structured logger for operation tracking
//   - metadata: Custom key-value pairs to store (can be nil)
//   - ID: The entity ID (transaction or operation UUID string)
//   - collection: The entity type ("Transaction" or "Operation")
//
// Returns:
//   - error: Metadata validation or persistence errors
func (uc *UseCase) CreateMetadataAsync(ctx context.Context, logger libLog.Logger, metadata map[string]any, ID string, collection string) error {
	if metadata != nil {
		// Validate metadata key length (max 100 characters)
		if err := libCommons.CheckMetadataKeyAndValueLength(100, metadata); err != nil {
			logger.Errorf("Error checking metadata key and value length: %v", err)

			return err
		}

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

// CreateBTOSync synchronously processes balance-transaction-operations from a queue message.
//
// This is a synchronous wrapper around CreateBalanceTransactionOperationsAsync,
// used when immediate transaction processing is required rather than async Redis queue processing.
// Typically called when testing or when synchronous confirmation is needed.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - data: Queue message containing transaction data
//
// Returns:
//   - error: Any error from the async processing pipeline
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

// RemoveTransactionFromRedisQueue removes a processed transaction from the Redis processing queue.
//
// After successful transaction processing (balance updates + persistence), the transaction
// message is removed from the Redis queue to prevent reprocessing. This implements
// at-least-once delivery semantics with idempotency for safety.
//
// The transaction key follows the format: "transaction:{transactions}:orgID:ledgerID:transactionID"
// with curly braces ensuring all related keys hash to the same Redis Cluster slot.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - logger: Structured logger for operation tracking
//   - organizationID: Organization UUID for key composition
//   - ledgerID: Ledger UUID for key composition
//   - transactionID: Transaction UUID string for key composition
func (uc *UseCase) RemoveTransactionFromRedisQueue(ctx context.Context, logger libLog.Logger, organizationID, ledgerID uuid.UUID, transactionID string) {
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID)

	if err := uc.RedisRepo.RemoveMessageFromQueue(ctx, transactionKey); err != nil {
		logger.Warnf("err to remove message on redis: %s", err.Error())
	} else {
		logger.Infof("message removed from redis successfully: %s", transactionKey)
	}
}

// SendTransactionToRedisQueue queues a transaction for async processing in Redis.
//
// This function implements the write-behind cache pattern, queuing transactions in Redis
// for async processing by worker consumers. The transaction is validated and enriched
// with balance data before being added to the queue.
//
// Redis Queue Pattern:
// 1. Transaction is serialized to JSON and stored in Redis with a generated key
// 2. Worker consumers poll Redis and process transactions via CreateBalanceTransactionOperationsAsync
// 3. After processing, the transaction is removed from the queue
// 4. This decouples transaction validation from balance updates for high throughput
//
// The queue key ensures Redis Cluster slot affinity: "transaction:{transactions}:orgID:ledgerID:txID"
//
// Parameters:
//   - ctx: Request context for tracing
//   - organizationID: Organization UUID for transaction context
//   - ledgerID: Ledger UUID for transaction context
//   - transactionID: Transaction UUID being queued
//   - parserDSL: Parsed Gold DSL transaction structure
//   - validate: Transaction validation results (account eligibility, balance checks)
//   - transactionStatus: Current transaction status (CREATED, PENDING, etc.)
//   - transactionDate: Effective date for the transaction
func (uc *UseCase) SendTransactionToRedisQueue(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, parserDSL libTransaction.Transaction, validate *libTransaction.Responses, transactionStatus string, transactionDate time.Time) {
	logger, _, reqId, _ := libCommons.NewTrackingFromContext(ctx)
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

	// Construct queue message with all transaction context
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

	// Serialize to JSON for Redis storage
	raw, err := json.Marshal(queue)
	if err != nil {
		logger.Warnf("Failed to marshal transaction to json string: %s", err.Error())
	}

	// Add to Redis queue for async processing
	err = uc.RedisRepo.AddMessageToQueue(ctx, transactionKey, raw)
	if err != nil {
		logger.Warnf("Failed to send transaction to redis queue: %s", err.Error())
	}
}
