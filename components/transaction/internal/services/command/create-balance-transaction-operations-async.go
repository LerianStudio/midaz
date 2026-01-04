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
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
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

	t, err := uc.unmarshalQueueData(logger, data)
	if err != nil {
		return err
	}

	// Use the result outside the transaction
	var tran *transaction.Transaction

	// Wrap balance update, transaction creation, and operations in a single database transaction.
	// This prevents orphan transactions (transactions without operations) that occur when
	// transaction creation succeeds but operation creation fails.
	ctx, spanAtomic := tracer.Start(ctx, "command.create_balance_transaction_operations.atomic")
	defer spanAtomic.End()

	err = dbtx.RunInTransaction(ctx, uc.DBProvider, func(txCtx context.Context) error {
		// Step 1: Update balances (if not NOTED status)
		if err := uc.updateBalancesStep(txCtx, tracer, logger, data, t); err != nil {
			return err
		}

		// Step 2: Create or update transaction
		var txErr error

		tran, txErr = uc.createTransactionStep(txCtx, tracer, logger, t)
		if txErr != nil {
			return txErr
		}

		// Step 3: Create operations (PostgreSQL only - metadata moved outside transaction)
		if err := uc.createOperationsWithoutMetadata(txCtx, logger, tracer, tran.Operations); err != nil {
			return err
		}

		// Step 4: Queue metadata to outbox (INSIDE transaction for atomicity)
		return uc.queueMetadataToOutbox(txCtx, tran)
	})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanAtomic, "Atomic transaction failed", err)
		logger.Errorf("Atomic transaction failed: %v", err.Error())

		return err //nolint:wrapcheck // Errors from transaction callback are already typed
	}

	// Metadata creation is now handled via the outbox pattern inside the transaction.
	// The outbox worker will asynchronously process and write metadata to MongoDB.

	mruntime.SafeGoWithContextAndComponent(ctx, logger, "transaction", "send_transaction_events", mruntime.KeepRunning, func(ctx context.Context) {
		uc.SendTransactionEvents(ctx, tran)
	})

	mruntime.SafeGoWithContextAndComponent(ctx, logger, "transaction", "remove_transaction_from_redis", mruntime.KeepRunning, func(ctx context.Context) {
		uc.RemoveTransactionFromRedisQueue(ctx, logger, data.OrganizationID, data.LedgerID, tran.ID)
	})

	return nil
}

// unmarshalQueueData extracts transaction queue data from the message queue.
func (uc *UseCase) unmarshalQueueData(logger libLog.Logger, data mmodel.Queue) (transaction.TransactionQueue, error) {
	var t transaction.TransactionQueue

	for _, item := range data.QueueData {
		logger.Infof("Unmarshal account ID: %v", item.ID.String())

		if err := msgpack.Unmarshal(item.Value, &t); err != nil {
			logger.Errorf("failed to unmarshal response: %v", err.Error())
			return t, pkg.ValidateInternalError(err, reflect.TypeOf(transaction.Transaction{}).Name())
		}
	}

	assert.That(len(data.QueueData) > 0,
		"transaction queue_data must not be empty",
		"organization_id", data.OrganizationID,
		"ledger_id", data.LedgerID)
	assert.NotNil(t.Transaction,
		"transaction payload must not be nil",
		"organization_id", data.OrganizationID,
		"ledger_id", data.LedgerID)
	assert.NotNil(t.Validate,
		"transaction validate must not be nil",
		"organization_id", data.OrganizationID,
		"ledger_id", data.LedgerID)
	assert.That(t.Transaction.IDtoUUID() == data.QueueData[0].ID,
		"transaction ID must match queue data ID",
		"transaction_id", t.Transaction.ID,
		"queue_data_id", data.QueueData[0].ID)
	assert.That(t.Transaction.OrganizationID == data.OrganizationID.String(),
		"transaction organization ID must match queue organization ID",
		"transaction_organization_id", t.Transaction.OrganizationID,
		"queue_organization_id", data.OrganizationID)
	assert.That(t.Transaction.LedgerID == data.LedgerID.String(),
		"transaction ledger ID must match queue ledger ID",
		"transaction_ledger_id", t.Transaction.LedgerID,
		"queue_ledger_id", data.LedgerID)
	assert.That(assert.ValidTransactionStatus(t.Transaction.Status.Code),
		"invalid transaction status in queue payload",
		"status", t.Transaction.Status.Code,
		"transaction_id", t.Transaction.ID)

	return t, nil
}

// updateBalancesStep updates balances if the transaction status is not NOTED.
func (uc *UseCase) updateBalancesStep(ctx context.Context, tracer trace.Tracer, logger libLog.Logger, data mmodel.Queue, t transaction.TransactionQueue) error {
	if t.Transaction.Status.Code == constant.NOTED {
		return nil
	}

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.create_balance_transaction_operations.update_balances")
	defer spanUpdateBalances.End()

	logger.Infof("Trying to update balances")

	if err := uc.UpdateBalances(ctxProcessBalances, data.OrganizationID, data.LedgerID, t.Transaction.ID, *t.Validate, t.Balances); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "Failed to update balances", err)
		logger.Errorf("Failed to update balances: %v", err.Error())

		return err
	}

	return nil
}

// createTransactionStep creates or updates the transaction.
func (uc *UseCase) createTransactionStep(ctx context.Context, tracer trace.Tracer, logger libLog.Logger, t transaction.TransactionQueue) (*transaction.Transaction, error) {
	ctxProcessTransaction, spanUpdateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanUpdateTransaction.End()

	tran, err := uc.CreateOrUpdateTransaction(ctxProcessTransaction, logger, tracer, t)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateTransaction, "Failed to create or update transaction", err)
		logger.Errorf("Failed to create or update transaction: %v", err.Error())

		return nil, err
	}

	return tran, nil
}

// queueMetadataToOutbox queues transaction and operation metadata to the outbox for async processing.
func (uc *UseCase) queueMetadataToOutbox(ctx context.Context, tran *transaction.Transaction) error {
	if err := uc.queueTransactionMetadata(ctx, tran); err != nil {
		return err
	}

	return uc.queueOperationsMetadata(ctx, tran.Operations)
}

// queueTransactionMetadata queues transaction metadata to the outbox.
func (uc *UseCase) queueTransactionMetadata(ctx context.Context, tran *transaction.Transaction) error {
	if tran.Metadata == nil {
		return nil
	}

	entry, err := outbox.NewMetadataOutbox(tran.ID, outbox.EntityTypeTransaction, tran.Metadata)
	if err != nil {
		return fmt.Errorf("failed to create outbox entry for transaction: %w", err)
	}

	if err := uc.OutboxRepo.Create(ctx, entry); err != nil {
		// Idempotency: if entry already exists (e.g., RabbitMQ retry), skip silently
		if errors.Is(err, outbox.ErrDuplicateOutboxEntry) {
			if uc.Logger != nil {
				uc.Logger.Debugf("Outbox duplicate entry; skipping transaction metadata (transaction_id=%v)", tran.ID)
			}
			return nil
		}

		return fmt.Errorf("failed to queue transaction metadata to outbox: %w", err)
	}

	return nil
}

// queueOperationsMetadata queues operation metadata to the outbox.
func (uc *UseCase) queueOperationsMetadata(ctx context.Context, operations []*operation.Operation) error {
	for _, oper := range operations {
		if oper.Metadata == nil {
			continue
		}

		entry, err := outbox.NewMetadataOutbox(oper.ID, outbox.EntityTypeOperation, oper.Metadata)
		if err != nil {
			return fmt.Errorf("failed to create outbox entry for operation: %w", err)
		}

		if err := uc.OutboxRepo.Create(ctx, entry); err != nil {
			// Idempotency: if entry already exists (e.g., RabbitMQ retry), skip silently
			if errors.Is(err, outbox.ErrDuplicateOutboxEntry) {
				continue
			}

			return fmt.Errorf("failed to queue operation metadata to outbox: %w", err)
		}
	}

	return nil
}

// CreateOrUpdateTransaction func that is responsible to create or update a transaction.
func (uc *UseCase) CreateOrUpdateTransaction(ctx context.Context, logger libLog.Logger, tracer trace.Tracer, t transaction.TransactionQueue) (*transaction.Transaction, error) {
	_, spanCreateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanCreateTransaction.End()

	logger.Infof("Trying to create new transaction")

	tran := t.Transaction

	tran.Body = pkgTransaction.Transaction{}
	if t.ParseDSL != nil && !t.ParseDSL.IsEmpty() {
		// Preserve the parsed DSL for metadata fallback and revert support when available.
		tran.Body = *t.ParseDSL
	}

	switch tran.Status.Code {
	case constant.CREATED:
		description := constant.APPROVED
		status := transaction.Status{
			Code:        description,
			Description: &description,
		}

		tran.Status = status
	case constant.PENDING:
		// Body already populated from ParseDSL when present.
	case constant.APPROVED, constant.CANCELED, constant.NOTED:
		// Status already set upstream (commit/cancel/annotation flows).
	default:
		assert.Never("unhandled transaction status code in CreateOrUpdateTransaction",
			"status_code", tran.Status.Code,
			"transaction_id", tran.ID)
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
// When a pending transaction is committed/canceled and encounters a unique constraint violation,
// this function updates the status AND ensures the original transaction's Operations array is preserved.
// The Operations (RELEASE/DEBIT) will be created by createOperationsWithoutMetadata() which is called
// after this function returns, using the operations from the original queue transaction.
//
// State machine for status transitions:
//   - PENDING -> CANCELED: RELEASE operations (OnHold -= amount, Available += amount)
//   - PENDING -> APPROVED: DEBIT operations (OnHold -= amount)
func (uc *UseCase) handleCreateTransactionError(ctx context.Context, span *trace.Span, logger libLog.Logger, tran *transaction.Transaction, pending bool, err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != constant.UniqueViolationCode {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create transaction on repo", err)
		logger.Errorf("Failed to create transaction on repo: %v", err.Error())

		return pkg.ValidateInternalError(err, reflect.TypeOf(transaction.Transaction{}).Name())
	}

	if pending && (tran.Status.Code == constant.APPROVED || tran.Status.Code == constant.CANCELED) {
		// Validate that operations exist from the original queue transaction.
		// For PENDING -> APPROVED/CANCELED transitions, operations (RELEASE/DEBIT) MUST be present
		// to properly update the balance state machine (OnHold adjustments).
		assert.That(len(tran.Operations) > 0,
			"operations must not be empty for pending->approved/canceled status transition",
			"transaction_id", tran.ID,
			"status", tran.Status.Code,
			"pending_flag", pending)

		// Preserve the operation count before status update for logging.
		// These operations (RELEASE/DEBIT) from the original queue transaction will be created
		// by createOperationsWithoutMetadata() after this function returns successfully.
		operationCount := len(tran.Operations)

		logger.Infof("Updating pending transaction %v to %v with %d operations to be created",
			tran.ID, tran.Status.Code, operationCount)

		_, err = uc.UpdateTransactionStatus(ctx, tran)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction", err)
			logger.Warnf("Failed to update transaction with STATUS: %v by ID: %v", tran.Status.Code, tran.ID)

			return pkg.ValidateInternalError(err, reflect.TypeOf(transaction.Transaction{}).Name())
		}

		// IMPORTANT: tran.Operations is preserved from the original queue transaction.
		// The caller (CreateOrUpdateTransaction) returns this tran, and createOperationsWithoutMetadata()
		// will use tran.Operations to create the RELEASE/DEBIT operations in the database.
		// This ensures the balance state machine transitions are properly recorded:
		// - CANCELED: RELEASE operations restore funds from OnHold to Available
		// - APPROVED: DEBIT operations finalize the deduction from OnHold
		logger.Infof("Transaction %v status updated to %v, %d operations will be created",
			tran.ID, tran.Status.Code, operationCount)

		// Status transition handled; do not emit the generic "skipping to create" log below.
		return nil
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
func (uc *UseCase) SendTransactionToRedisQueue(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, parserDSL pkgTransaction.Transaction, validate *pkgTransaction.Responses, transactionStatus string, transactionDate time.Time) error {
	logger, _, reqId, _ := libCommons.NewTrackingFromContext(ctx)
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

	utils.SanitizeAccountAliases(&parserDSL)

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
		logger.Errorf("Failed to marshal transaction to json string: %s", err.Error())

		return constant.ErrTransactionBackupCacheMarshalFailed
	}

	err = uc.RedisRepo.AddMessageToQueue(ctx, transactionKey, raw)
	if err != nil {
		logger.Errorf("Failed to send transaction to redis queue: %s", err.Error())

		return constant.ErrTransactionBackupCacheFailed
	}

	return nil
}

// createOperationsWithoutMetadata creates operation records in PostgreSQL without metadata.
// Metadata creation is handled separately outside the database transaction.
// Note: The repository uses ON CONFLICT (id) DO NOTHING, so duplicates are handled gracefully
// without errors - the insert simply returns the record with rowsAffected=0.
func (uc *UseCase) createOperationsWithoutMetadata(ctx context.Context, logger libLog.Logger, tracer trace.Tracer, operations []*operation.Operation) error {
	ctxProcessOperation, spanCreateOperation := tracer.Start(ctx, "command.create_balance_transaction_operations.create_operation")
	defer spanCreateOperation.End()

	logger.Infof("Trying to create new operations")

	for _, oper := range operations {
		_, err := uc.OperationRepo.Create(ctxProcessOperation, oper)
		if err != nil {
			// Safety net: isUniqueViolation check kept as fallback, though ON CONFLICT
			// should handle duplicates without raising errors.
			if uc.isUniqueViolation(err) {
				logger.Warnf("Skipping operation (unique violation fallback), operation already exists: %v", oper.ID)
				continue
			}

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, "Failed to create operation", err)
			logger.Errorf("Error creating operation: %v", err)

			return pkg.ValidateInternalError(err, reflect.TypeOf(operation.Operation{}).Name())
		}
	}

	return nil
}
