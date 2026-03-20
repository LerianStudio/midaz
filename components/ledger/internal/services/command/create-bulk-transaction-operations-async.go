// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vmihailenco/msgpack/v5"
)

// bulkUpdateThreshold defines the minimum number of transactions required
// to use bulk update. Below this threshold, individual updates are used.
const bulkUpdateThreshold = 10

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

	result := &BulkResult{}

	if len(payloads) == 0 {
		return result, nil
	}

	// Classify payloads into inserts and updates
	toInsert, toUpdate := uc.classifyAndExtractEntities(payloads)

	// Try bulk insert and update first (before updating balances)
	// If bulk fails, fallback will handle balance updates
	if err := uc.performBulkInsertAndUpdate(ctx, logger, toInsert, toUpdate, result); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Bulk insert/update failed, falling back", err)
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Bulk insert/update failed, using fallback: %v", err))

		return uc.fallbackToIndividualProcessing(ctx, logger, payloads, result)
	}

	// Bulk insert succeeded - now update balances
	// This order prevents double balance updates if we had to fallback
	if err := uc.updateBalancesForPayloads(ctx, logger, payloads); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update balances after bulk insert", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to update balances after bulk insert: %v", err))

		// Note: Transactions are already persisted, but balances failed
		// This is a partial success - log but don't fail the entire operation
		// The balance sync worker will reconcile eventually
	}

	// Process metadata and send events
	uc.processMetadataAndEvents(ctx, logger, payloads)

	return result, nil
}

// updateBalancesForPayloads updates balances for all payloads that require it.
func (uc *UseCase) updateBalancesForPayloads(
	ctx context.Context,
	logger libLog.Logger,
	payloads []transaction.TransactionProcessingPayload,
) error {
	for i, payload := range payloads {
		if payload.Transaction == nil {
			continue
		}

		if payload.Transaction.Status.Code == constant.NOTED {
			continue
		}

		if payload.Validate == nil {
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Payload %d has nil Validate field, skipping balance update", i))

			continue
		}

		orgID, ledgerID, err := uc.extractOrgLedgerIDs(payload)
		if err != nil {
			return fmt.Errorf("payload %d: %w", i, err)
		}

		if err := uc.UpdateBalances(ctx, orgID, ledgerID, *payload.Validate, payload.Balances, payload.BalancesAfter); err != nil {
			return fmt.Errorf("payload %d balance update: %w", i, err)
		}
	}

	return nil
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

		tx := payload.Transaction
		tx.Body = pkgTransaction.Transaction{}

		// Determine if this is an insert or an update
		if uc.isStatusTransition(payload) {
			// Status transition: PENDING -> APPROVED/CANCELED
			toUpdate.transactions = append(toUpdate.transactions, tx)
		} else {
			// Normal insert
			uc.prepareTransactionForInsert(tx)
			toInsert.transactions = append(toInsert.transactions, tx)

			// Collect operations for this transaction
			for _, op := range tx.Operations {
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
func (uc *UseCase) prepareTransactionForInsert(tx *transaction.Transaction) {
	switch tx.Status.Code {
	case constant.CREATED:
		description := constant.APPROVED
		tx.Status = transaction.Status{
			Code:        description,
			Description: &description,
		}
	case constant.PENDING:
		// Keep PENDING status and body for pending transactions
	}
}

// performBulkInsertAndUpdate performs bulk insert for new entities and bulk update for status transitions.
func (uc *UseCase) performBulkInsertAndUpdate(
	ctx context.Context,
	logger libLog.Logger,
	toInsert *bulkInsertEntities,
	toUpdate *bulkUpdateEntities,
	result *BulkResult,
) error {
	// Bulk insert transactions
	if len(toInsert.transactions) > 0 {
		if err := uc.bulkInsertTransactions(ctx, logger, toInsert.transactions, result); err != nil {
			return err
		}
	}

	// Bulk insert operations
	if len(toInsert.operations) > 0 {
		if err := uc.bulkInsertOperations(ctx, logger, toInsert.operations, result); err != nil {
			return err
		}
	}

	// Bulk update transactions (status transitions)
	if len(toUpdate.transactions) > 0 {
		if err := uc.performBulkStatusUpdate(ctx, logger, toUpdate.transactions, result); err != nil {
			return err
		}
	}

	return nil
}

// bulkInsertTransactions inserts transactions in bulk.
func (uc *UseCase) bulkInsertTransactions(
	ctx context.Context,
	logger libLog.Logger,
	transactions []*transaction.Transaction,
	result *BulkResult,
) error {
	result.TransactionsAttempted = int64(len(transactions))

	insertResult, err := uc.TransactionRepo.CreateBulk(ctx, transactions)
	if err != nil {
		return fmt.Errorf("bulk insert transactions: %w", err)
	}

	result.TransactionsInserted = insertResult.Inserted
	result.TransactionsIgnored = insertResult.Ignored

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
		"Bulk inserted transactions: attempted=%d, inserted=%d, ignored=%d",
		result.TransactionsAttempted, result.TransactionsInserted, result.TransactionsIgnored,
	))

	return nil
}

// bulkInsertOperations inserts operations in bulk.
func (uc *UseCase) bulkInsertOperations(
	ctx context.Context,
	logger libLog.Logger,
	operations []*operation.Operation,
	result *BulkResult,
) error {
	result.OperationsAttempted = int64(len(operations))

	insertResult, err := uc.OperationRepo.CreateBulk(ctx, operations)
	if err != nil {
		return fmt.Errorf("bulk insert operations: %w", err)
	}

	result.OperationsInserted = insertResult.Inserted
	result.OperationsIgnored = insertResult.Ignored

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
		"Bulk inserted operations: attempted=%d, inserted=%d, ignored=%d",
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
func (uc *UseCase) individualUpdateTransactionStatus(
	ctx context.Context,
	logger libLog.Logger,
	transactions []*transaction.Transaction,
	result *BulkResult,
) error {
	var updated int64

	for _, tx := range transactions {
		_, err := uc.UpdateTransactionStatus(ctx, tx)
		if err != nil {
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf(
				"Failed to update transaction %s status: %v", tx.ID, err,
			))

			continue
		}

		updated++
	}

	result.TransactionsUpdated = updated

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
		"Individual updated transactions: attempted=%d, updated=%d",
		result.TransactionsUpdateAttempted, updated,
	))

	return nil
}

// processMetadataAndEvents creates metadata and sends events for all payloads.
func (uc *UseCase) processMetadataAndEvents(
	ctx context.Context,
	logger libLog.Logger,
	payloads []transaction.TransactionProcessingPayload,
) {
	for _, payload := range payloads {
		if payload.Transaction == nil {
			continue
		}

		tx := payload.Transaction

		// Create transaction metadata
		if err := uc.CreateMetadataAsync(ctx, logger, tx.Metadata, tx.ID, reflect.TypeOf(transaction.Transaction{}).Name()); err != nil {
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to create transaction metadata: %v", err))
		}

		// Create operation metadata
		for _, op := range tx.Operations {
			if op == nil {
				continue
			}

			if err := uc.CreateMetadataAsync(ctx, logger, op.Metadata, op.ID, reflect.TypeOf(operation.Operation{}).Name()); err != nil {
				logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to create operation metadata: %v", err))
			}
		}

		// Send events asynchronously
		go uc.SendTransactionEvents(ctx, tx)

		// Clean up backup queue
		orgID, ledgerID, err := uc.extractOrgLedgerIDs(payload)
		if err == nil {
			go uc.RemoveTransactionFromRedisQueue(ctx, logger, orgID, ledgerID, tx.ID)
			go uc.DeleteWriteBehindTransaction(ctx, orgID, ledgerID, tx.ID)
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

	// Return error only if all payloads failed
	if successCount == 0 && lastErr != nil {
		return result, fmt.Errorf("all fallback processing failed: %w", lastErr)
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
