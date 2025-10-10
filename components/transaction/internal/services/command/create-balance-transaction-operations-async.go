// Package command implements write operations (commands) for the transaction service.
// This file contains the asynchronous command for creating balances, transactions, and operations.
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

// CreateBalanceTransactionOperationsAsync processes a queued message to create balances, a transaction, and operations.
//
// This function is the primary worker for asynchronously processing transaction data.
// It orchestrates the creation of all necessary records, including updating balances,
// creating the transaction and its operations, and storing metadata. The function
// also handles idempotency by checking for duplicate records.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - data: The queue message containing the transaction data.
//
// Returns:
//   - error: An error if any step of the processing fails.
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
			}

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, "Failed to create operation", err)

			logger.Errorf("Error creating operation: %v", err)

			return err
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

// CreateOrUpdateTransaction creates a new transaction or updates an existing one, with idempotency handling.
//
// This function attempts to create a new transaction record. If a unique violation
// error occurs, it checks if the transaction was in a PENDING state and updates its
// status accordingly. This ensures that transactions are processed idempotently.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - logger: The logger instance.
//   - tracer: The tracer instance.
//   - t: The transaction data from the queue.
//
// Returns:
//   - *transaction.Transaction: The created or updated transaction.
//   - error: An error if the creation or update fails.
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

// CreateMetadataAsync creates metadata for an entity in MongoDB.
//
// This helper function is designed for use in asynchronous command handlers.
// It validates and persists metadata for a given entity, such as a transaction
// or an operation.
//
// Parameters:
//   - ctx: The context for tracing and logging.
//   - logger: The logger instance.
//   - metadata: A map of key-value pairs to be stored.
//   - ID: The UUID string of the entity.
//   - collection: The name of the entity, used as the collection name in MongoDB.
//
// Returns:
//   - error: An error if the validation or persistence fails.
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

// CreateBTOSync is a synchronous wrapper for CreateBalanceTransactionOperationsAsync.
//
// This function provides a synchronous entry point for processing transaction data,
// primarily for use cases where immediate processing is required instead of queuing.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - data: The queue message containing the transaction data.
//
// Returns:
//   - error: An error if the processing fails.
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
// This function is called as a fire-and-forget goroutine to clean up the Redis
// queue after a transaction has been successfully processed. Any errors that occur
// are logged but do not affect the overall transaction flow.
//
// Parameters:
//   - ctx: The context for tracing and logging.
//   - logger: The logger instance.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - transactionID: The UUID of the transaction to remove.
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
// This function is used to store the state of a transaction in Redis, which can be
// useful for monitoring, debugging, and recovering from failures. The operation
// is fire-and-forget, and any errors are logged without interrupting the main flow.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - transactionID: The UUID of the transaction.
//   - parserDSL: The parsed DSL specification of the transaction.
//   - validate: The validation responses for the transaction.
//   - transactionStatus: The current status of the transaction.
//   - transactionDate: The creation date of the transaction.
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
