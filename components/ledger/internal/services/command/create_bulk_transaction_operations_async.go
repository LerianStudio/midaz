// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vmihailenco/msgpack/v5"
)

// bulkUpdateThreshold defines the minimum number of transactions required
// to use bulk update. Below this threshold, individual updates are used.
const bulkUpdateThreshold = 10

// asyncOperationTimeout defines the timeout for fire-and-forget async operations
// like sending events and cleaning up backup queues.
const asyncOperationTimeout = 30 * time.Second

// BulkResult contains the aggregated counts from a bulk transaction/operation processing.
type BulkResult struct {
	// Transaction insert counts
	TransactionsAttempted int64
	TransactionsInserted  int64
	TransactionsIgnored   int64

	// Transaction update counts (for status transitions)
	TransactionsUpdateAttempted int64
	TransactionsUpdated         int64

	// Operation counts
	OperationsAttempted int64
	OperationsInserted  int64
	OperationsIgnored   int64

	// Fallback tracking
	FallbackUsed  bool
	FallbackCount int64

	// InsertedTransactionIDs tracks which transactions were actually inserted (not duplicates).
	// Used to filter downstream operations like metadata creation and event publishing.
	InsertedTransactionIDs map[string]struct{}
}

// BulkMessageResult tracks the result for a single message in bulk processing.
type BulkMessageResult struct {
	Index   int
	Success bool
	Error   error
}

// CreateBulkTransactionOperationsAsync processes multiple transaction payloads in bulk.
// It extracts, sorts, and persists transactions and operations using bulk insert,
// then handles status transitions for pending transactions using bulk update.
//
// On failure, it falls back to individual processing using CreateBalanceTransactionOperationsAsync.
// Duplicates are treated as success (idempotent processing).
func (uc *UseCase) CreateBulkTransactionOperationsAsync(
	ctx context.Context,
	payloads []transaction.TransactionProcessingPayload,
) (*BulkResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_bulk_transaction_operations_async")
	defer span.End()

	result := &BulkResult{
		InsertedTransactionIDs: make(map[string]struct{}),
	}

	if len(payloads) == 0 {
		return result, nil
	}

	// Classify payloads into inserts and updates
	toInsert, toUpdate := uc.classifyAndExtractEntities(payloads)

	// Try bulk insert and update first
	// If bulk fails, fallback to individual processing
	if err := uc.performBulkInsertAndUpdate(ctx, logger, toInsert, toUpdate, result); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Bulk insert/update failed, falling back", err)
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Bulk insert/update failed, using fallback: %v", err))

		return uc.fallbackToIndividualProcessing(ctx, logger, payloads, result)
	}

	// Note: Balance updates are handled by BalanceSyncWorker asynchronously.
	// Hot balance was already updated atomically by Lua script during validation.
	// Cold balance persistence is scheduled via ZADD to schedule:balance-sync.

	// Process metadata and send events only for actually-inserted transactions
	// This ensures idempotency by skipping duplicates that were ignored during bulk insert
	uc.processMetadataAndEvents(ctx, logger, payloads, result.InsertedTransactionIDs)

	return result, nil
}

// extractOrgLedgerIDs extracts organization and ledger IDs from a payload.
func (uc *UseCase) extractOrgLedgerIDs(payload transaction.TransactionProcessingPayload) (uuid.UUID, uuid.UUID, error) {
	if payload.Transaction == nil {
		return uuid.Nil, uuid.Nil, errors.New("nil transaction in payload")
	}

	orgID, err := uuid.Parse(payload.Transaction.OrganizationID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid organization ID: %w", err)
	}

	ledgerID, err := uuid.Parse(payload.Transaction.LedgerID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid ledger ID: %w", err)
	}

	return orgID, ledgerID, nil
}

// classifyAndExtractEntities separates payloads into transactions/operations to insert
// and transactions to update (status transitions).
// Creates copies of transactions to avoid mutating the original payloads,
// which are needed intact for fallback processing.
func (uc *UseCase) classifyAndExtractEntities(
	payloads []transaction.TransactionProcessingPayload,
) (toInsert *bulkInsertEntities, toUpdate *bulkUpdateEntities) {
	toInsert = &bulkInsertEntities{
		transactions: make([]*transaction.Transaction, 0, len(payloads)),
		operations:   make([]*operation.Operation, 0, len(payloads)*2), // Estimate 2 ops per transaction
	}

	toUpdate = &bulkUpdateEntities{
		transactions: make([]*transaction.Transaction, 0),
	}

	for _, payload := range payloads {
		if payload.Transaction == nil {
			continue
		}

		// Create a shallow copy to avoid mutating the original payload
		// The original is needed intact for fallbackToIndividualProcessing
		txCopy := *payload.Transaction

		// Determine if this is an insert or an update
		if uc.isStatusTransition(payload) {
			// Status transition: PENDING -> APPROVED/CANCELED
			// Clear body for status updates - not needed for UPDATE query
			txCopy.Body = mtransaction.Transaction{}
			toUpdate.transactions = append(toUpdate.transactions, &txCopy)

			// Also collect operations for status transitions (commit/cancel flows)
			// These operations were generated by BuildOperations and MUST be persisted
			for _, op := range payload.Transaction.Operations {
				if op != nil {
					toInsert.operations = append(toInsert.operations, op)
				}
			}
		} else {
			// Normal insert - prepareTransactionForInsert handles body based on status
			uc.prepareTransactionForInsert(&txCopy)
			toInsert.transactions = append(toInsert.transactions, &txCopy)

			// Collect operations for this transaction (operations are not mutated)
			for _, op := range payload.Transaction.Operations {
				if op != nil {
					toInsert.operations = append(toInsert.operations, op)
				}
			}
		}
	}

	// Sort for deadlock prevention
	sortTransactionsByID(toInsert.transactions)
	sortOperationsByID(toInsert.operations)
	sortTransactionsByID(toUpdate.transactions)

	return toInsert, toUpdate
}

// bulkInsertEntities holds entities to be inserted in bulk.
type bulkInsertEntities struct {
	transactions []*transaction.Transaction
	operations   []*operation.Operation
}

// bulkUpdateEntities holds entities to be updated in bulk.
type bulkUpdateEntities struct {
	transactions []*transaction.Transaction
}

// isStatusTransition checks if a payload represents a status transition
// (existing PENDING transaction being updated to APPROVED or CANCELED).
func (uc *UseCase) isStatusTransition(payload transaction.TransactionProcessingPayload) bool {
	if payload.Transaction == nil || payload.Validate == nil {
		return false
	}

	// If Validate.Pending is true and status is APPROVED or CANCELED,
	// this is a status transition from PENDING
	status := payload.Transaction.Status.Code

	return payload.Validate.Pending && (status == constant.APPROVED || status == constant.CANCELED)
}

// prepareTransactionForInsert prepares a transaction for bulk insert.
// Handles status transitions and body clearing based on transaction status.
func (uc *UseCase) prepareTransactionForInsert(tx *transaction.Transaction) {
	switch tx.Status.Code {
	case constant.CREATED:
		// CREATED transactions are auto-approved and body is not needed
		tx.Body = mtransaction.Transaction{}
		description := constant.APPROVED
		tx.Status = transaction.Status{
			Code:        description,
			Description: &description,
		}
	case constant.PENDING:
		// Keep PENDING status and body for pending transactions
		// Body is required for later approval/cancellation processing
	default:
		// Clear body for other statuses (e.g., NOTED)
		tx.Body = mtransaction.Transaction{}
	}
}

// performBulkInsertAndUpdate performs bulk insert for new entities and bulk update for status transitions.
// Inserts (transactions + operations) are wrapped in a single DB transaction for atomicity.
// Status updates are performed separately as they operate on existing rows.
func (uc *UseCase) performBulkInsertAndUpdate(
	ctx context.Context,
	logger libLog.Logger,
	toInsert *bulkInsertEntities,
	toUpdate *bulkUpdateEntities,
	result *BulkResult,
) error {
	// Atomic bulk insert for transactions and operations
	if len(toInsert.transactions) > 0 || len(toInsert.operations) > 0 {
		if err := uc.atomicBulkInsert(ctx, logger, toInsert, result); err != nil {
			return err
		}
	}

	// Bulk update transactions (status transitions) - separate from inserts
	// These operate on existing rows and don't need to be atomic with inserts
	if len(toUpdate.transactions) > 0 {
		if err := uc.performBulkStatusUpdate(ctx, logger, toUpdate.transactions, result); err != nil {
			return err
		}
	}

	return nil
}

// atomicBulkInsert inserts transactions and operations in a single database transaction.
// If either insert fails, the entire operation is rolled back to prevent partial state.
func (uc *UseCase) atomicBulkInsert(
	ctx context.Context,
	logger libLog.Logger,
	toInsert *bulkInsertEntities,
	result *BulkResult,
) error {
	// Begin database transaction
	dbTx, err := uc.TransactionRepo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin database transaction: %w", err)
	}

	// Ensure rollback on panic or error
	committed := false

	defer func() {
		if !committed {
			if rbErr := dbTx.Rollback(); rbErr != nil {
				logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to rollback transaction: %v", rbErr))
			}
		}
	}()

	// Bulk insert transactions within the DB transaction
	if len(toInsert.transactions) > 0 {
		if err := uc.bulkInsertTransactionsTx(ctx, logger, dbTx, toInsert.transactions, result); err != nil {
			return err
		}
	}

	// Bulk insert operations within the same DB transaction
	if len(toInsert.operations) > 0 {
		if err := uc.bulkInsertOperationsTx(ctx, logger, dbTx, toInsert.operations, result); err != nil {
			return err
		}
	}

	// Commit the transaction
	if err := dbTx.Commit(); err != nil {
		return fmt.Errorf("failed to commit database transaction: %w", err)
	}

	committed = true

	return nil
}

// bulkInsertTransactionsTx inserts transactions in bulk using a provided database transaction.
// This enables atomic multi-table operations with operations insert.
// Populates result.InsertedTransactionIDs with IDs of actually-inserted transactions.
func (uc *UseCase) bulkInsertTransactionsTx(
	ctx context.Context,
	logger libLog.Logger,
	dbTx repository.DBExecutor,
	transactions []*transaction.Transaction,
	result *BulkResult,
) error {
	result.TransactionsAttempted = int64(len(transactions))

	insertResult, err := uc.TransactionRepo.CreateBulkTx(ctx, dbTx, transactions)
	if err != nil {
		return fmt.Errorf("bulk insert transactions: %w", err)
	}

	result.TransactionsInserted = insertResult.Inserted
	result.TransactionsIgnored = insertResult.Ignored

	// Populate inserted IDs for downstream filtering (metadata, events)
	for _, id := range insertResult.InsertedIDs {
		result.InsertedTransactionIDs[id] = struct{}{}
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
		"Bulk inserted transactions (tx): attempted=%d, inserted=%d, ignored=%d",
		result.TransactionsAttempted, result.TransactionsInserted, result.TransactionsIgnored,
	))

	return nil
}

// bulkInsertOperationsTx inserts operations in bulk using a provided database transaction.
// This enables atomic multi-table operations with transactions insert.
func (uc *UseCase) bulkInsertOperationsTx(
	ctx context.Context,
	logger libLog.Logger,
	dbTx repository.DBExecutor,
	operations []*operation.Operation,
	result *BulkResult,
) error {
	result.OperationsAttempted = int64(len(operations))

	insertResult, err := uc.OperationRepo.CreateBulkTx(ctx, dbTx, operations)
	if err != nil {
		return fmt.Errorf("bulk insert operations: %w", err)
	}

	result.OperationsInserted = insertResult.Inserted
	result.OperationsIgnored = insertResult.Ignored

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
		"Bulk inserted operations (tx): attempted=%d, inserted=%d, ignored=%d",
		result.OperationsAttempted, result.OperationsInserted, result.OperationsIgnored,
	))

	return nil
}

// performBulkStatusUpdate handles status transitions for transactions.
// Uses bulk update for large batches and individual updates for small batches.
func (uc *UseCase) performBulkStatusUpdate(
	ctx context.Context,
	logger libLog.Logger,
	transactions []*transaction.Transaction,
	result *BulkResult,
) error {
	result.TransactionsUpdateAttempted = int64(len(transactions))

	if len(transactions) >= bulkUpdateThreshold {
		return uc.bulkUpdateTransactionStatus(ctx, logger, transactions, result)
	}

	return uc.individualUpdateTransactionStatus(ctx, logger, transactions, result)
}

// bulkUpdateTransactionStatus uses bulk update for status transitions.
func (uc *UseCase) bulkUpdateTransactionStatus(
	ctx context.Context,
	logger libLog.Logger,
	transactions []*transaction.Transaction,
	result *BulkResult,
) error {
	updateResult, err := uc.TransactionRepo.UpdateBulk(ctx, transactions)
	if err != nil {
		return fmt.Errorf("bulk update transactions: %w", err)
	}

	result.TransactionsUpdated = updateResult.Updated

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
		"Bulk updated transactions: attempted=%d, updated=%d, unchanged=%d",
		result.TransactionsUpdateAttempted, updateResult.Updated, updateResult.Unchanged,
	))

	return nil
}

// individualUpdateTransactionStatus uses individual updates for small batches.
// Returns an aggregated error if any updates failed.
func (uc *UseCase) individualUpdateTransactionStatus(
	ctx context.Context,
	logger libLog.Logger,
	transactions []*transaction.Transaction,
	result *BulkResult,
) error {
	var updated int64

	var failureCount int64

	for _, tx := range transactions {
		_, err := uc.UpdateTransactionStatus(ctx, tx)
		if err != nil {
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf(
				"Failed to update transaction %s status: %v", tx.ID, err,
			))

			failureCount++

			continue
		}

		updated++
	}

	result.TransactionsUpdated = updated

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
		"Individual updated transactions: attempted=%d, updated=%d, failed=%d",
		result.TransactionsUpdateAttempted, updated, failureCount,
	))

	if failureCount > 0 {
		return fmt.Errorf("failed to update %d of %d transactions", failureCount, len(transactions))
	}

	return nil
}

// processMetadataAndEvents creates metadata and sends events for payloads that were actually inserted.
// Skips duplicate transactions (those not in insertedTxIDs) to ensure idempotency.
// If insertedTxIDs is nil or empty, processes all payloads (fallback behavior).
func (uc *UseCase) processMetadataAndEvents(
	ctx context.Context,
	logger libLog.Logger,
	payloads []transaction.TransactionProcessingPayload,
	insertedTxIDs map[string]struct{},
) {
	// Create all metadata in bulk (reduces N round-trips to 1 per collection)
	uc.processMetadataAndEventsBulk(ctx, logger, payloads, insertedTxIDs)

	// Process events and cleanup for each inserted transaction
	for _, payload := range payloads {
		if payload.Transaction == nil {
			continue
		}

		tx := payload.Transaction

		// Skip if this transaction was not actually inserted (duplicate)
		// If insertedTxIDs is empty, process all (fallback or status-update scenarios)
		if len(insertedTxIDs) > 0 {
			if _, wasInserted := insertedTxIDs[tx.ID]; !wasInserted {
				logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf(
					"Skipping events for duplicate transaction %s", tx.ID))

				continue
			}
		}

		// Send events asynchronously with context that preserves trace but survives parent cancellation
		go func() {
			opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), asyncOperationTimeout)
			defer cancel()

			uc.SendTransactionEvents(opCtx, tx)
		}()

		// Clean up backup queue with context that preserves trace but survives parent cancellation
		orgID, ledgerID, err := uc.extractOrgLedgerIDs(payload)
		if err == nil {
			go func(orgID, ledgerID uuid.UUID, txID string) {
				opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), asyncOperationTimeout)
				defer cancel()

				uc.RemoveTransactionFromRedisQueue(opCtx, logger, orgID, ledgerID, txID)
			}(orgID, ledgerID, tx.ID)

			go func(orgID, ledgerID uuid.UUID, txID string) {
				opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), asyncOperationTimeout)
				defer cancel()

				uc.DeleteWriteBehindTransaction(opCtx, orgID, ledgerID, txID)
			}(orgID, ledgerID, tx.ID)
		}
	}
}

// fallbackToIndividualProcessing processes payloads individually when bulk fails.
// Returns the result and an error if all payloads failed.
func (uc *UseCase) fallbackToIndividualProcessing(
	ctx context.Context,
	logger libLog.Logger,
	payloads []transaction.TransactionProcessingPayload,
	result *BulkResult,
) (*BulkResult, error) {
	result.FallbackUsed = true

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Using fallback processing for %d payloads", len(payloads)))

	var successCount int64

	var lastErr error

	for i, payload := range payloads {
		if payload.Transaction == nil {
			continue
		}

		queueData, err := uc.buildQueueDataFromPayload(payload)
		if err != nil {
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Fallback: failed to build queue data for payload %d: %v", i, err))

			lastErr = err

			continue
		}

		err = uc.CreateBalanceTransactionOperationsAsync(ctx, queueData)
		if err != nil {
			// Check for duplicate - treat as success
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
				logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Payload %d is duplicate, treating as success", i))

				successCount++

				continue
			}

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Fallback processing failed for payload %d: %v", i, err))

			lastErr = err

			continue
		}

		successCount++
	}

	result.FallbackCount = successCount

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
		"Fallback processing complete: %d/%d succeeded",
		successCount, len(payloads),
	))

	// Return error if any payload failed (partial failure)
	if successCount != int64(len(payloads)) && lastErr != nil {
		return result, fmt.Errorf("partial fallback processing failed: %d/%d succeeded: %w", successCount, len(payloads), lastErr)
	}

	return result, nil
}

// buildQueueDataFromPayload constructs mmodel.Queue from a payload for fallback processing.
// Encodes the payload using msgpack as expected by CreateBalanceTransactionOperationsAsync.
// Returns an error if the payload cannot be encoded or has invalid IDs.
func (uc *UseCase) buildQueueDataFromPayload(payload transaction.TransactionProcessingPayload) (mmodel.Queue, error) {
	if payload.Transaction == nil {
		return mmodel.Queue{}, errors.New("nil transaction in payload")
	}

	orgID, err := uuid.Parse(payload.Transaction.OrganizationID)
	if err != nil {
		return mmodel.Queue{}, fmt.Errorf("invalid organization ID: %w", err)
	}

	ledgerID, err := uuid.Parse(payload.Transaction.LedgerID)
	if err != nil {
		return mmodel.Queue{}, fmt.Errorf("invalid ledger ID: %w", err)
	}

	// Encode the payload using msgpack
	encodedPayload, err := msgpack.Marshal(payload)
	if err != nil {
		return mmodel.Queue{}, fmt.Errorf("failed to encode payload: %w", err)
	}

	return mmodel.Queue{
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		QueueData: []mmodel.QueueData{
			{
				ID:    uuid.New(),
				Value: encodedPayload,
			},
		},
	}, nil
}

// sortTransactionsByID sorts transactions slice by ID for deadlock prevention.
func sortTransactionsByID(transactions []*transaction.Transaction) {
	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].ID < transactions[j].ID
	})
}

// sortOperationsByID sorts operations slice by ID for deadlock prevention.
func sortOperationsByID(operations []*operation.Operation) {
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].ID < operations[j].ID
	})
}
