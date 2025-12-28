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
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/dbtx"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mruntime"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/otel/trace"
)

// CreateBalanceTransactionOperationsAsync creates all transactions atomically using a database transaction.
// This ensures that balance updates, transaction creation, and operation creation either
// all succeed or all fail together, preventing orphan transactions.
func (uc *UseCase) CreateBalanceTransactionOperationsAsync(ctx context.Context, data mmodel.Queue) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	var t transaction.TransactionQueue

	for _, item := range data.QueueData {
		logger.Infof("Unmarshal account ID: %v", item.ID.String())

		err := msgpack.Unmarshal(item.Value, &t)
		if err != nil {
			logger.Errorf("failed to unmarshal response: %v", err.Error())

			return pkg.ValidateInternalError(err, reflect.TypeOf(transaction.Transaction{}).Name())
		}
	}

	// Use the result outside the transaction
	var tran *transaction.Transaction

	// Wrap balance update, transaction creation, and operations in a single database transaction.
	// This prevents orphan transactions (transactions without operations) that occur when
	// transaction creation succeeds but operation creation fails.
	ctx, spanAtomic := tracer.Start(ctx, "command.create_balance_transaction_operations.atomic")
	defer spanAtomic.End()

	err := dbtx.RunInTransaction(ctx, uc.DBProvider, func(txCtx context.Context) error {
		// Step 1: Update balances (if not NOTED status)
		if t.Transaction.Status.Code != constant.NOTED {
			ctxProcessBalances, spanUpdateBalances := tracer.Start(txCtx, "command.create_balance_transaction_operations.update_balances")
			defer spanUpdateBalances.End()

			logger.Infof("Trying to update balances")

			if err := uc.UpdateBalances(ctxProcessBalances, data.OrganizationID, data.LedgerID, *t.Validate, t.Balances); err != nil {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "Failed to update balances", err)
				logger.Errorf("Failed to update balances: %v", err.Error())
				return err
			}
		}

		// Step 2: Create or update transaction
		ctxProcessTransaction, spanUpdateTransaction := tracer.Start(txCtx, "command.create_balance_transaction_operations.create_transaction")
		defer spanUpdateTransaction.End()

		var err error
		tran, err = uc.CreateOrUpdateTransaction(ctxProcessTransaction, logger, tracer, t)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateTransaction, "Failed to create or update transaction", err)
			logger.Errorf("Failed to create or update transaction: %v", err.Error())
			return err
		}

		// Step 3: Create operations (PostgreSQL only - metadata moved outside transaction)
		if err := uc.createOperationsWithoutMetadata(txCtx, logger, tracer, tran.Operations); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanAtomic, "Atomic transaction failed", err)
		logger.Errorf("Atomic transaction failed: %v", err.Error())
		return err
	}

	// All MongoDB metadata creation happens after PostgreSQL transaction commits.
	// This prevents orphaned metadata in MongoDB if the PostgreSQL transaction rolls back.
	// If metadata creation fails, the core transaction is already committed successfully.

	// Create transaction metadata
	ctxProcessMetadata, spanCreateMetadata := tracer.Start(ctx, "command.create_balance_transaction_operations.create_metadata")
	defer spanCreateMetadata.End()

	if err := uc.CreateMetadataAsync(ctxProcessMetadata, logger, tran.Metadata, tran.ID, reflect.TypeOf(transaction.Transaction{}).Name()); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateMetadata, "Failed to create metadata on transaction", err)
		// Transaction is already committed - log warning but don't fail the operation
		logger.Warnf("Transaction %s committed successfully but metadata creation failed: %v", tran.ID, err)
		// TODO(review): Consider adding to retry queue or reconciliation job
	}

	// Create operation metadata
	for _, oper := range tran.Operations {
		if err := uc.CreateMetadataAsync(ctx, logger, oper.Metadata, oper.ID, reflect.TypeOf(operation.Operation{}).Name()); err != nil {
			// Log warning but don't fail - core operation is already committed
			logger.Warnf("Operation %s committed successfully but metadata creation failed: %v", oper.ID, err)
			// TODO(review): Consider adding to retry queue or reconciliation job
		}
	}

	mruntime.SafeGoWithContextAndComponent(ctx, logger, "transaction", "send_transaction_events", mruntime.KeepRunning, func(ctx context.Context) {
		uc.SendTransactionEvents(ctx, tran)
	})

	mruntime.SafeGoWithContextAndComponent(ctx, logger, "transaction", "remove_transaction_from_redis", mruntime.KeepRunning, func(ctx context.Context) {
		uc.RemoveTransactionFromRedisQueue(ctx, logger, data.OrganizationID, data.LedgerID, tran.ID)
	})

	return nil
}

// CreateOrUpdateTransaction func that is responsible to create or update a transaction.
func (uc *UseCase) CreateOrUpdateTransaction(ctx context.Context, logger libLog.Logger, tracer trace.Tracer, t transaction.TransactionQueue) (*transaction.Transaction, error) {
	_, spanCreateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanCreateTransaction.End()

	logger.Infof("Trying to create new transaction")

	tran := t.Transaction
	tran.Body = pkgTransaction.Transaction{}

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
		if handleErr := uc.handleCreateTransactionError(ctx, &spanCreateTransaction, logger, tran, t.Validate.Pending, err); handleErr != nil {
			return nil, handleErr
		}

		// Transaction already exists (unique violation handled). Return the transaction so callers can
		// continue safely (e.g., avoid nil dereference) and decide what to do next.
		return tran, nil
	}

	return tran, nil
}

// handleCreateTransactionError handles errors during transaction creation, including unique violations.
func (uc *UseCase) handleCreateTransactionError(ctx context.Context, span *trace.Span, logger libLog.Logger, tran *transaction.Transaction, pending bool, err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != constant.UniqueViolationCode {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create transaction on repo", err)
		logger.Errorf("Failed to create transaction on repo: %v", err.Error())

		return pkg.ValidateInternalError(err, reflect.TypeOf(transaction.Transaction{}).Name())
	}

	if pending && (tran.Status.Code == constant.APPROVED || tran.Status.Code == constant.CANCELED) {
		_, err = uc.UpdateTransactionStatus(ctx, tran)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction", err)
			logger.Warnf("Failed to update transaction with STATUS: %v by ID: %v", tran.Status.Code, tran.ID)

			return pkg.ValidateInternalError(err, reflect.TypeOf(transaction.Transaction{}).Name())
		}
	}

	logger.Infof("skipping to create transaction, transaction already exists: %v", tran.ID)

	return nil
}

// CreateMetadataAsync func that create metadata into operations
func (uc *UseCase) CreateMetadataAsync(ctx context.Context, logger libLog.Logger, metadata map[string]any, id string, collection string) error {
	if metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   id,
			EntityName: collection,
			Data:       metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, collection, &meta); err != nil {
			logger.Errorf("Error into creating %s metadata: %v", collection, err)

			return pkg.ValidateInternalError(err, collection)
		}
	}

	return nil
}

// CreateBTOSync func that create balance transaction operations synchronously
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

// RemoveTransactionFromRedisQueue func that remove transaction from redis queue
func (uc *UseCase) RemoveTransactionFromRedisQueue(ctx context.Context, logger libLog.Logger, organizationID, ledgerID uuid.UUID, transactionID string) {
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID)

	if err := uc.RedisRepo.RemoveMessageFromQueue(ctx, transactionKey); err != nil {
		logger.Warnf("err to remove message on redis: %s", err.Error())
	} else {
		logger.Infof("message removed from redis successfully: %s", transactionKey)
	}
}

// SendTransactionToRedisQueue func that send transaction to redis queue
func (uc *UseCase) SendTransactionToRedisQueue(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, parserDSL pkgTransaction.Transaction, validate *pkgTransaction.Responses, transactionStatus string, transactionDate time.Time) {
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

// createOperationsWithoutMetadata creates operation records in PostgreSQL without metadata.
// Metadata creation is handled separately outside the database transaction.
func (uc *UseCase) createOperationsWithoutMetadata(ctx context.Context, logger libLog.Logger, tracer trace.Tracer, operations []*operation.Operation) error {
	ctxProcessOperation, spanCreateOperation := tracer.Start(ctx, "command.create_balance_transaction_operations.create_operation")
	defer spanCreateOperation.End()

	logger.Infof("Trying to create new operations")

	for _, oper := range operations {
		_, err := uc.OperationRepo.Create(ctxProcessOperation, oper)
		if err != nil {
			if uc.isUniqueViolation(err) {
				msg := fmt.Sprintf("Skipping to create operation, operation already exists: %v", oper.ID)
				libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, msg, err)
				logger.Warnf(msg)

				continue
			}

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, "Failed to create operation", err)
			logger.Errorf("Error creating operation: %v", err)

			return pkg.ValidateInternalError(err, reflect.TypeOf(operation.Operation{}).Name())
		}
	}

	return nil
}

// createOperations creates all operations for a transaction including metadata.
// This is the legacy method that includes metadata creation inline.
// Use createOperationsWithoutMetadata for transaction-aware operations.
func (uc *UseCase) createOperations(ctx context.Context, logger libLog.Logger, tracer trace.Tracer, operations []*operation.Operation) error {
	// First create PostgreSQL operation records
	if err := uc.createOperationsWithoutMetadata(ctx, logger, tracer, operations); err != nil {
		return err
	}

	// Then create MongoDB metadata
	for _, oper := range operations {
		if err := uc.CreateMetadataAsync(ctx, logger, oper.Metadata, oper.ID, reflect.TypeOf(operation.Operation{}).Name()); err != nil {
			logger.Errorf("Failed to create metadata on operation: %v", err)
			return err
		}
	}

	return nil
}
