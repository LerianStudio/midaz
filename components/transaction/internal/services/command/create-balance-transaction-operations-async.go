// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/otel/trace"
)

// CreateBalanceTransactionOperationsAsync processes queued transaction data (balances, transaction, operations).
//
// This method is the worker function that processes transaction queue messages. It:
// 1. Unmarshals TransactionQueue from msgpack format
// 2. Updates account balances (unless transaction is NOTED status)
// 3. Creates or updates the transaction record
// 4. Creates transaction metadata
// 5. Creates all operation records
// 6. Creates operation metadata
// 7. Publishes transaction events (async)
// 8. Removes transaction from Redis queue (async)
//
// Processing Flow:
//   - For APPROVED/CANCELED: Update balances, create transaction, create operations
//   - For NOTED: Skip balance updates, create transaction, create operations
//   - For PENDING: Update to APPROVED, create operations
//
// Idempotency:
//   - Handles duplicate transaction creation (unique violation)
//   - Handles duplicate operation creation (unique violation)
//   - Updates transaction status if already exists
//
// Async Cleanup:
//   - Publishes transaction events in goroutine (fire-and-forget)
//   - Removes from Redis queue in goroutine (fire-and-forget)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - data: Queue message with transaction data
//
// Returns:
//   - error: nil on success, error if processing fails
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

// CreateOrUpdateTransaction creates or updates a transaction with idempotent handling.
//
// This helper function implements transaction creation with duplicate handling:
// 1. Attempts to create transaction
// 2. If unique violation (transaction already exists):
//   - For PENDING transactions: Updates status to APPROVED/CANCELED
//   - For other statuses: Logs and continues (idempotent)
//
// 3. Returns the transaction
//
// Status Handling:
//   - CREATED: Changes to APPROVED (validation complete)
//   - PENDING: Stores DSL body, updates status later
//
// Idempotency:
//   - Unique violation on ID means transaction already processed
//   - Updates status if needed (PENDING → APPROVED/CANCELED)
//   - Logs duplicate creation attempts
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - logger: Logger instance
//   - tracer: Tracer instance
//   - t: TransactionQueue with transaction data
//
// Returns:
//   - *transaction.Transaction: Created or existing transaction
//   - error: Database error if creation/update fails
//
// OpenTelemetry: Creates span "command.create_balance_transaction_operations.create_transaction"
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

// CreateMetadataAsync creates metadata for transactions or operations in MongoDB.
//
// This helper function validates and persists metadata during async processing.
// It's similar to CreateMetadata but designed for the async worker context.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - logger: Logger instance
//   - metadata: Key-value metadata map (nil if no metadata)
//   - ID: Entity UUID
//   - collection: Entity type (Transaction, Operation)
//
// Returns:
//   - error: nil on success, validation or creation error
func (uc *UseCase) CreateMetadataAsync(ctx context.Context, logger libLog.Logger, metadata map[string]any, ID string, collection string) error {
	if metadata != nil {
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

// CreateBTOSync is a wrapper that processes BTO data synchronously.
//
// This method calls CreateBalanceTransactionOperationsAsync (despite the name, it runs sync
// when called directly). It's used as an entry point for sync processing.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - data: Queue message with transaction data
//
// Returns:
//   - error: nil on success, error if processing fails
//
// OpenTelemetry: Creates span "command.create_balance_transaction_operations.create_bto_sync"
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

// RemoveTransactionFromRedisQueue removes a processed transaction from the Redis queue.
//
// This cleanup function removes transaction data from Redis after successful processing.
// It's called asynchronously (in a goroutine) and errors are logged but not returned.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - logger: Logger instance
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - transactionID: UUID of the transaction to remove
func (uc *UseCase) RemoveTransactionFromRedisQueue(ctx context.Context, logger libLog.Logger, organizationID, ledgerID uuid.UUID, transactionID string) {
	transactionKey := libCommons.TransactionInternalKey(organizationID, ledgerID, transactionID)

	if err := uc.RedisRepo.RemoveMessageFromQueue(ctx, transactionKey); err != nil {
		logger.Warnf("err to remove message on redis: %s", err.Error())
	} else {
		logger.Infof("message removed from redis successfully: %s", transactionKey)
	}
}

// SendTransactionToRedisQueue stores transaction data in Redis for tracking and recovery.
//
// This method stores transaction processing state in Redis for:
//   - Monitoring pending transactions
//   - Recovery after failures
//   - Debugging transaction processing
//   - Tracking transaction lifecycle
//
// The stored data includes:
//   - Request ID for correlation
//   - Transaction IDs (organization, ledger, transaction)
//   - Parsed DSL specification
//   - Validation responses
//   - Transaction status and date
//   - TTL timestamp
//
// Errors are logged but not returned (fire-and-forget pattern).
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - transactionID: UUID of the transaction
//   - parserDSL: Parsed DSL transaction specification
//   - validate: Validation responses
//   - transactionStatus: Current transaction status
//   - transactionDate: Transaction creation date
func (uc *UseCase) SendTransactionToRedisQueue(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, parserDSL libTransaction.Transaction, validate *libTransaction.Responses, transactionStatus string, transactionDate time.Time) {
	logger, _, reqId, _ := libCommons.NewTrackingFromContext(ctx)
	transactionKey := libCommons.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

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
