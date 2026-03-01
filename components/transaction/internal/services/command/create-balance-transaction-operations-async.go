// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vmihailenco/msgpack/v5"
	"go.mongodb.org/mongo-driver/mongo"
	"go.opentelemetry.io/otel/trace"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

var (
	errInvalidQueuePayloadEmpty      = errors.New("invalid queue payload: empty queue data")
	errInvalidPayloadNil             = errors.New("invalid transaction payload: payload is nil")
	errInvalidPayloadTransactionNil  = errors.New("invalid transaction payload: transaction is nil")
	errInvalidPayloadValidateNil     = errors.New("invalid transaction payload: validate is nil")
	errInvalidPayloadPendingInputNil = errors.New("invalid transaction payload: pending transaction input is nil")
	errInvalidPayloadBalancesNil     = errors.New("invalid transaction payload: balances is nil")
	errInvalidPayloadBalancesEmpty   = errors.New("invalid transaction payload: balances slice is empty")
)

// IsNonRetryablePayloadError reports whether err indicates malformed queue
// payload data that cannot succeed on retry and should be dropped.
func IsNonRetryablePayloadError(err error) bool {
	return errors.Is(err, errInvalidQueuePayloadEmpty) ||
		errors.Is(err, errInvalidPayloadNil) ||
		errors.Is(err, errInvalidPayloadTransactionNil) ||
		errors.Is(err, errInvalidPayloadValidateNil) ||
		errors.Is(err, errInvalidPayloadPendingInputNil) ||
		errors.Is(err, errInvalidPayloadBalancesNil) ||
		errors.Is(err, errInvalidPayloadBalancesEmpty)
}

type transactionBatchItem struct {
	organizationID uuid.UUID
	ledgerID       uuid.UUID
	payload        transaction.TransactionProcessingPayload
}

type balanceUpdateItem struct {
	organizationID uuid.UUID
	ledgerID       uuid.UUID
	transactionID  string
	validate       pkgTransaction.Responses
	balances       []*mmodel.Balance
}

type balanceBatchUpdateGroup struct {
	organizationID uuid.UUID
	ledgerID       uuid.UUID
	balances       []*mmodel.Balance
}

type sqlExecQueryTx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type sqlBatchTx interface {
	sqlExecQueryTx
	Commit() error
	Rollback() error
}

type batchTransactionRepository interface {
	BeginTx(ctx context.Context) (sqlBatchTx, error)
	CreateBatchWithTx(ctx context.Context, tx sqlExecQueryTx, transactions []*transaction.Transaction) error
}

type batchOperationRepository interface {
	CreateBatchWithTx(ctx context.Context, tx interface {
		ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	}, operations []*operation.Operation) error
}

type batchBalanceRepository interface {
	BalancesUpdateWithTx(ctx context.Context, tx sqlExecQueryTx, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error
}

type processedTransactionBatchItem struct {
	organizationID   uuid.UUID
	ledgerID         uuid.UUID
	decisionContract pkgTransaction.DecisionContract
	transaction      *transaction.Transaction
}

// sideEffectItem holds only the immutable data needed by post-commit goroutines.
// Deep-copied from processedTransactionBatchItem to avoid sharing mutable pointers
// across concurrent goroutines after the batch function returns on timeout.
type sideEffectItem struct {
	organizationID   uuid.UUID
	ledgerID         uuid.UUID
	transactionID    string
	decisionContract pkgTransaction.DecisionContract
	transaction      transaction.Transaction // value copy, not pointer
}

const (
	// defaultSideEffectTimeout is the default timeout for fire-and-forget
	// post-commit side effects (event publishing, Redis cleanup).
	defaultSideEffectTimeout = 10 * time.Second

	batchSideEffectsWaitTimeout = 2 * time.Second
	batchSideEffectsMaxWorkers  = 16
	batchPersistMaxRetries      = 3
	batchPersistRetryDelay      = 20 * time.Millisecond
)

func (uc *UseCase) batchSideEffectsTimeout() time.Duration {
	if uc == nil || uc.BatchSideEffectsTimeout <= 0 {
		return batchSideEffectsWaitTimeout
	}

	return uc.BatchSideEffectsTimeout
}

// CreateBalanceTransactionOperationsAsync func that is responsible to create all transactions at the same async.
//
//nolint:gocyclo,cyclop,funlen
func (uc *UseCase) CreateBalanceTransactionOperationsAsync(ctx context.Context, data mmodel.Queue) (retErr error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	decisionContract := pkgTransaction.DefaultDecisionContract()

	var lifecycleTran *transaction.Transaction

	suppressEventEmission := false

	defer func() {
		if retErr == nil || lifecycleTran == nil || suppressEventEmission {
			return
		}

		uc.dispatchDecisionLifecycleEvent(
			ctx,
			lifecycleTran,
			decisionContract,
			pkgTransaction.DecisionLifecycleActionPostingFailed,
			defaultSideEffectTimeout,
		)
	}()

	if len(data.QueueData) == 0 {
		return errInvalidQueuePayloadEmpty
	}

	if len(data.QueueData) > 1 {
		return fmt.Errorf("invalid queue payload: expected exactly 1 queue data item, got %d", len(data.QueueData)) //nolint:err113
	}

	var t transaction.TransactionProcessingPayload

	item := data.QueueData[0]
	logger.Infof("Unmarshal account ID: %v", item.ID.String())

	err := msgpack.Unmarshal(item.Value, &t)
	if err != nil {
		logger.Errorf("failed to unmarshal response: %v", err.Error())

		return fmt.Errorf("failed to unmarshal queue item: %w", err)
	}

	if err := validateTransactionProcessingPayload(&t); err != nil {
		return err
	}

	decisionContract = t.DecisionContract
	lifecycleTran = t.Transaction

	// NOTED transactions do not mutate balances, so balance payload is optional.
	if t.Transaction.Status.Code != constant.NOTED {
		if err := validateBalancesNotNil(t.Balances); err != nil {
			return err
		}
	}

	ctxProcessTransaction, spanUpdateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanUpdateTransaction.End()

	tran, duplicate, err := uc.CreateOrUpdateTransaction(ctxProcessTransaction, logger, tracer, t)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateTransaction, "Failed to create or update transaction", err)

		logger.Errorf("Failed to create or update transaction: %v", err.Error())

		return err
	}

	suppressEventEmission = duplicate
	if duplicate {
		logger.Infof("Duplicate async transaction replay detected: transaction_id=%s (running reconciliation without event emission)", tran.ID)
	}

	lifecycleTran = tran

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

	ctxProcessMetadata, spanCreateMetadata := tracer.Start(ctx, "command.create_balance_transaction_operations.create_metadata")
	defer spanCreateMetadata.End()

	err = uc.CreateMetadataAsync(ctxProcessMetadata, logger, tran.Metadata, tran.ID, reflect.TypeOf(transaction.Transaction{}).Name())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateMetadata, "Failed to create metadata on transaction", err)

		logger.Errorf("Failed to create metadata on transaction: %v", err.Error())

		return err
	}

	ctxProcessOperation, spanCreateOperation := tracer.Start(ctx, "command.create_balance_transaction_operations.create_operations_batch")
	defer spanCreateOperation.End()

	logger.Infof("Trying to create %d operations (batch insert)", len(tran.Operations))

	if err := validateOperationsNotNil(tran.Operations); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, "Invalid operations payload", err)

		logger.Errorf("Invalid operations payload: %v", err)

		return err
	}

	// Batch insert all operations in a single multi-row INSERT statement.
	// Duplicates are silently skipped via ON CONFLICT DO NOTHING.
	if len(tran.Operations) > 0 {
		err = uc.OperationRepo.CreateBatch(ctxProcessOperation, tran.Operations)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, "Failed to batch create operations", err)

			logger.Errorf("Failed to batch create operations: %v", err)

			return err
		}
	}

	// Create metadata for each operation (MongoDB — separate from PG batch)
	for _, oper := range tran.Operations {
		err = uc.CreateMetadataAsync(ctx, logger, oper.Metadata, oper.ID, reflect.TypeOf(operation.Operation{}).Name())
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, "Failed to create metadata on operation", err)

			logger.Errorf("Failed to create metadata on operation: %v", err)

			return err
		}
	}

	// Fire-and-forget goroutines for post-commit side effects.
	// Use a detached context so request cancellation does not drop side effects
	// after the main transaction was already persisted.
	// Concurrency is bounded by upstream consumer workers and partitioning,
	// so the maximum concurrent goroutines here remains controlled.
	// An explicit semaphore is unnecessary for this stage.
	if !suppressEventEmission {
		go func() {
			sideEffectCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second) //nolint:mnd
			defer cancel()

			uc.SendDecisionLifecycleEvent(sideEffectCtx, tran, t.DecisionContract, pkgTransaction.DecisionLifecycleActionPostingCompleted)
			uc.SendTransactionEvents(sideEffectCtx, tran)
		}()
	}

	logger.Infof("Backup queue: cleaning up transaction %s after successful processing", tran.ID)

	touchedShards := uc.touchedShardsFromOperations(tran.Operations)

	go func() {
		sideEffectCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second) //nolint:mnd
		defer cancel()

		uc.RemoveTransactionFromRedisQueue(sideEffectCtx, logger, data.OrganizationID, data.LedgerID, tran.ID, touchedShards...)
	}()

	return nil
}

// CreateBalanceTransactionOperationsBatch processes multiple queue messages in one consumer flush.
// It batches operation INSERTs across all payloads to reduce PostgreSQL round-trips while
// preserving per-transaction validation and side effects.
//
//nolint:gocyclo,cyclop
func (uc *UseCase) CreateBalanceTransactionOperationsBatch(ctx context.Context, queues []mmodel.Queue) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	if len(queues) == 0 {
		return nil
	}

	txRepo, txRepoOK := uc.TransactionRepo.(batchTransactionRepository)
	opRepo, opRepoOK := uc.OperationRepo.(batchOperationRepository)
	balanceRepo, balanceRepoOK := uc.BalanceRepo.(batchBalanceRepository)

	if !txRepoOK || !opRepoOK || !balanceRepoOK {
		logger.Warnf(
			"Batch optimization unavailable (requires PostgreSQL repositories). Falling back to per-item processing: tx_repo=%t op_repo=%t balance_repo=%t",
			txRepoOK,
			opRepoOK,
			balanceRepoOK,
		)

		var batchErrs []error

		for i, queue := range queues {
			if err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue); err != nil {
				batchErrs = append(batchErrs, fmt.Errorf("batch item %d failed: %w", i, err))
			}
		}

		return errors.Join(batchErrs...)
	}

	batchItems, parseSkipped := parseBatchItems(logger, queues)

	if len(batchItems) == 0 {
		logger.Warnf("Batch: all items were invalid, nothing to process")
		return nil
	}

	classified := classifyBatchItems(logger, batchItems)
	skippedItems := parseSkipped + classified.skippedItems

	persistedTransactionIDs, err := uc.findPersistedBatchTransactionIDs(ctx, classified.processedTransactions)
	if err != nil {
		return fmt.Errorf("batch: failed to resolve already persisted transactions: %w", err)
	}

	if len(persistedTransactionIDs) > 0 {
		classified = filterPersistedBatchTransactions(classified, persistedTransactionIDs)
		logger.Warnf("Batch: %d transaction(s) already persisted; suppressing duplicate balance mutation and event emission", len(persistedTransactionIDs))
	}

	if len(classified.transactionsToCreate) == 0 {
		uc.runBatchDuplicateCleanup(ctx, logger, classified.duplicateTransactions)

		if skippedItems > 0 {
			return fmt.Errorf("batch: all %d item(s) failed validation (dead-letter candidates)", skippedItems) //nolint:err113
		}

		logger.Warnf("Batch: no valid transactions to persist after validation")

		return nil
	}

	balanceUpdates := classified.balanceUpdates
	transactionsToCreate := classified.transactionsToCreate
	transactionsToStatusUpdate := classified.transactionsToStatusUpdate
	allOperations := classified.allOperations
	processedTransactions := classified.processedTransactions

	ctxPersist, spanPersist := tracer.Start(ctx, "command.create_balance_transaction_operations_batch.persist_single_tx")
	defer spanPersist.End()

	for attempt := 1; attempt <= batchPersistMaxRetries; attempt++ {
		err := uc.persistBatchPayloadInTx(
			ctxPersist,
			logger,
			tracer,
			txRepo,
			balanceRepo,
			opRepo,
			balanceUpdates,
			transactionsToCreate,
			allOperations,
		)
		if err == nil {
			break
		}

		if !isRetryableBatchPersistError(err) || attempt >= batchPersistMaxRetries {
			libOpentelemetry.HandleSpanError(&spanPersist, "Failed to persist batch payload", err)
			logger.Errorf("Failed to persist batch payload: %v", err)
			uc.emitBatchPostingFailed(ctx, logger, processedTransactions)

			return err
		}

		waitDelay := time.Duration(attempt*attempt) * batchPersistRetryDelay
		logger.Warnf("Retrying batch persistence after transient database error (attempt %d/%d, delay=%s): %v", attempt, batchPersistMaxRetries, waitDelay, err)

		select {
		case <-ctxPersist.Done():
			return ctxPersist.Err()
		case <-time.After(waitDelay):
		}
	}

	uc.runBatchPostCommit(ctx, logger, transactionsToStatusUpdate, processedTransactions, allOperations)
	uc.runBatchDuplicateCleanup(ctx, logger, classified.duplicateTransactions)

	return nil
}

func (uc *UseCase) emitBatchPostingFailed(ctx context.Context, logger libLog.Logger, processedTransactions []processedTransactionBatchItem) {
	if len(processedTransactions) == 0 {
		return
	}

	for _, item := range processedTransactions {
		if item.transaction == nil {
			continue
		}

		uc.dispatchDecisionLifecycleEvent(
			ctx,
			item.transaction,
			item.decisionContract,
			pkgTransaction.DecisionLifecycleActionPostingFailed,
			defaultSideEffectTimeout,
		)
	}

	logger.Warnf("Batch post-commit: emitted posting_failed lifecycle events for %d transaction(s)", len(processedTransactions))
}

// parseBatchItems parses and validates queue messages into batch items.
// Returns valid items and count of skipped items.
func parseBatchItems(logger libLog.Logger, queues []mmodel.Queue) ([]transactionBatchItem, int) {
	batchItems := make([]transactionBatchItem, 0, len(queues))
	skippedItems := 0

	for _, queue := range queues {
		if len(queue.QueueData) == 0 {
			logger.Errorf("Batch: skipping queue with empty queue data (organization_id=%s ledger_id=%s)", queue.OrganizationID, queue.LedgerID)

			skippedItems++

			continue
		}

		for _, item := range queue.QueueData {
			var payload transaction.TransactionProcessingPayload

			err := msgpack.Unmarshal(item.Value, &payload)
			if err != nil {
				logger.Errorf("Batch: skipping item due to unmarshal failure (item_id=%s): %v", item.ID.String(), err)

				skippedItems++

				continue
			}

			if err := validateTransactionProcessingPayload(&payload); err != nil {
				logger.Errorf("Batch: skipping item due to invalid payload (item_id=%s): %v", item.ID.String(), err)

				skippedItems++

				continue
			}

			batchItems = append(batchItems, transactionBatchItem{
				organizationID: queue.OrganizationID,
				ledgerID:       queue.LedgerID,
				payload:        payload,
			})
		}
	}

	if skippedItems > 0 {
		logger.Warnf("Batch: %d item(s) skipped due to validation/unmarshal errors (dead-letter candidates)", skippedItems)
	}

	return batchItems, skippedItems
}

// batchClassification holds the categorized results of validating and classifying batch items.
type batchClassification struct {
	balanceUpdates             []balanceUpdateItem
	transactionsToCreate       []*transaction.Transaction
	transactionsToStatusUpdate []*transaction.Transaction
	allOperations              []*operation.Operation
	processedTransactions      []processedTransactionBatchItem
	duplicateTransactions      []processedTransactionBatchItem
	skippedItems               int
}

// classifyBatchItems validates and categorizes batch items for persistence.
//
//nolint:gocyclo,cyclop
func classifyBatchItems(logger libLog.Logger, batchItems []transactionBatchItem) batchClassification {
	result := batchClassification{
		balanceUpdates:             make([]balanceUpdateItem, 0, len(batchItems)),
		transactionsToCreate:       make([]*transaction.Transaction, 0, len(batchItems)),
		transactionsToStatusUpdate: make([]*transaction.Transaction, 0, len(batchItems)),
		allOperations:              make([]*operation.Operation, 0, len(batchItems)*2), //nolint:mnd
		processedTransactions:      make([]processedTransactionBatchItem, 0, len(batchItems)),
		duplicateTransactions:      make([]processedTransactionBatchItem, 0),
	}

	seenTransactions := make(map[string]struct{}, len(batchItems))

	for _, item := range batchItems {
		payload := item.payload
		tran := payload.Transaction
		transactionID := strings.TrimSpace(tran.ID)
		pendingValidate := payload.Validate != nil && payload.Validate.Pending

		if transactionID != "" {
			if _, exists := seenTransactions[transactionID]; exists {
				logger.Warnf("Batch: duplicate transaction replay detected in same flush, suppressing duplicate persistence transaction_id=%s", transactionID)

				result.duplicateTransactions = append(result.duplicateTransactions, processedTransactionBatchItem{
					organizationID:   item.organizationID,
					ledgerID:         item.ledgerID,
					decisionContract: payload.DecisionContract,
					transaction:      tran,
				})

				continue
			}

			seenTransactions[transactionID] = struct{}{}
		}

		// Validate ALL inputs before adding to any list — ensures that if an
		// item is skipped, its balance updates and transactions are not orphaned.
		if tran.Status.Code != constant.NOTED {
			if err := validateBalancesNotNil(payload.Balances); err != nil {
				logger.Errorf("Batch: skipping transaction %s due to invalid balances (dead-letter candidate): %v", tran.ID, err)

				result.skippedItems++

				continue
			}
		}

		if tran.Status.Code == constant.PENDING && payload.Input == nil {
			logger.Errorf("Batch: skipping pending transaction %s with nil input (dead-letter candidate)", tran.ID)

			result.skippedItems++

			continue
		}

		if err := validateOperationsNotNil(tran.Operations); err != nil {
			logger.Errorf("Batch: skipping transaction %s due to invalid operations (dead-letter candidate): %v", tran.ID, err)

			result.skippedItems++

			continue
		}

		// All validation passed — safe to add to lists.
		if tran.Status.Code != constant.NOTED {
			result.balanceUpdates = append(result.balanceUpdates, balanceUpdateItem{
				organizationID: item.organizationID,
				ledgerID:       item.ledgerID,
				transactionID:  transactionID,
				validate:       *payload.Validate,
				balances:       payload.Balances,
			})
		}

		tran.Body = pkgTransaction.Transaction{}

		switch tran.Status.Code {
		case constant.CREATED:
			description := constant.APPROVED
			tran.Status = transaction.Status{Code: description, Description: &description}
		case constant.PENDING:
			tran.Body = *payload.Input
		}

		if pendingValidate && (tran.Status.Code == constant.APPROVED || tran.Status.Code == constant.CANCELED) {
			result.transactionsToStatusUpdate = append(result.transactionsToStatusUpdate, tran)
		}

		result.transactionsToCreate = append(result.transactionsToCreate, tran)

		if len(tran.Operations) > 0 {
			result.allOperations = append(result.allOperations, tran.Operations...)
		}

		result.processedTransactions = append(result.processedTransactions, processedTransactionBatchItem{
			organizationID:   item.organizationID,
			ledgerID:         item.ledgerID,
			decisionContract: payload.DecisionContract,
			transaction:      tran,
		})
	}

	if result.skippedItems > 0 {
		logger.Warnf("Batch: total %d item(s) skipped during processing (dead-letter candidates)", result.skippedItems)
	}

	return result
}

//nolint:gocyclo,cyclop
func (uc *UseCase) findPersistedBatchTransactionIDs(ctx context.Context, items []processedTransactionBatchItem) (map[string]struct{}, error) {
	persisted := make(map[string]struct{})
	if uc == nil || uc.TransactionRepo == nil || len(items) == 0 {
		return persisted, nil
	}

	type scopedIDs struct {
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		ids            []uuid.UUID
		seen           map[uuid.UUID]struct{}
	}

	scopes := make(map[string]*scopedIDs)

	for _, item := range items {
		if item.transaction == nil {
			continue
		}

		transactionID := strings.TrimSpace(item.transaction.ID)
		if transactionID == "" {
			continue
		}

		parsedID, err := uuid.Parse(transactionID)
		if err != nil {
			continue
		}

		scopeKey := item.organizationID.String() + "|" + item.ledgerID.String()

		scope, ok := scopes[scopeKey]
		if !ok {
			scope = &scopedIDs{
				organizationID: item.organizationID,
				ledgerID:       item.ledgerID,
				ids:            make([]uuid.UUID, 0, len(items)),
				seen:           make(map[uuid.UUID]struct{}),
			}
			scopes[scopeKey] = scope
		}

		if _, exists := scope.seen[parsedID]; exists {
			continue
		}

		scope.seen[parsedID] = struct{}{}
		scope.ids = append(scope.ids, parsedID)
	}

	for _, scope := range scopes {
		if len(scope.ids) == 0 {
			continue
		}

		existing, err := uc.TransactionRepo.ListByIDs(ctx, scope.organizationID, scope.ledgerID, scope.ids)
		if err != nil {
			return nil, err
		}

		for _, tx := range existing {
			if tx == nil {
				continue
			}

			if txID := strings.TrimSpace(tx.ID); txID != "" {
				persisted[txID] = struct{}{}
			}
		}
	}

	return persisted, nil
}

//nolint:gocyclo,cyclop
func filterPersistedBatchTransactions(classified batchClassification, persisted map[string]struct{}) batchClassification {
	if len(persisted) == 0 {
		return classified
	}

	isPersisted := func(transactionID string) bool {
		_, ok := persisted[strings.TrimSpace(transactionID)]
		return ok
	}

	filteredBalanceUpdates := make([]balanceUpdateItem, 0, len(classified.balanceUpdates))
	for _, update := range classified.balanceUpdates {
		if isPersisted(update.transactionID) {
			continue
		}

		filteredBalanceUpdates = append(filteredBalanceUpdates, update)
	}

	filteredTransactionsToCreate := make([]*transaction.Transaction, 0, len(classified.transactionsToCreate))
	for _, tx := range classified.transactionsToCreate {
		if tx == nil || isPersisted(tx.ID) {
			continue
		}

		filteredTransactionsToCreate = append(filteredTransactionsToCreate, tx)
	}

	filteredTransactionsToStatusUpdate := make([]*transaction.Transaction, 0, len(classified.transactionsToStatusUpdate))
	for _, tx := range classified.transactionsToStatusUpdate {
		if tx == nil || isPersisted(tx.ID) {
			continue
		}

		filteredTransactionsToStatusUpdate = append(filteredTransactionsToStatusUpdate, tx)
	}

	filteredOperations := make([]*operation.Operation, 0, len(classified.allOperations))
	for _, op := range classified.allOperations {
		if op == nil || isPersisted(op.TransactionID) {
			continue
		}

		filteredOperations = append(filteredOperations, op)
	}

	filteredProcessed := make([]processedTransactionBatchItem, 0, len(classified.processedTransactions))
	duplicates := make([]processedTransactionBatchItem, 0, len(classified.duplicateTransactions)+len(classified.processedTransactions))
	duplicates = append(duplicates, classified.duplicateTransactions...)

	for _, item := range classified.processedTransactions {
		if item.transaction == nil {
			continue
		}

		if isPersisted(item.transaction.ID) {
			duplicates = append(duplicates, item)
			continue
		}

		filteredProcessed = append(filteredProcessed, item)
	}

	classified.balanceUpdates = filteredBalanceUpdates
	classified.transactionsToCreate = filteredTransactionsToCreate
	classified.transactionsToStatusUpdate = filteredTransactionsToStatusUpdate
	classified.allOperations = filteredOperations
	classified.processedTransactions = filteredProcessed
	classified.duplicateTransactions = duplicates

	return classified
}

// runBatchPostCommit executes post-commit operations: status updates, metadata persistence,
// and async side effects (event publishing, Redis cleanup).
func (uc *UseCase) runBatchPostCommit(
	ctx context.Context,
	logger libLog.Logger,
	transactionsToStatusUpdate []*transaction.Transaction,
	processedTransactions []processedTransactionBatchItem,
	allOperations []*operation.Operation,
) {
	for _, tran := range transactionsToStatusUpdate {
		if _, err := uc.UpdateTransactionStatus(ctx, tran); err != nil {
			logger.Warnf("Batch post-commit: failed to update transaction status transaction_id=%s status=%s err=%v", tran.ID, tran.Status.Code, err)
		}
	}

	for _, item := range processedTransactions {
		if err := uc.CreateMetadataAsync(ctx, logger, item.transaction.Metadata, item.transaction.ID, reflect.TypeOf(transaction.Transaction{}).Name()); err != nil {
			logger.Warnf("Batch post-commit: failed to create transaction metadata transaction_id=%s err=%v", item.transaction.ID, err)
		}
	}

	for _, oper := range allOperations {
		if err := uc.CreateMetadataAsync(ctx, logger, oper.Metadata, oper.ID, reflect.TypeOf(operation.Operation{}).Name()); err != nil {
			logger.Warnf("Batch post-commit: failed to create operation metadata operation_id=%s err=%v", oper.ID, err)
		}
	}

	uc.runBatchSideEffects(ctx, logger, processedTransactions)
}

// runBatchSideEffects runs post-commit side effects (event publishing, Redis cleanup) concurrently.
func (uc *UseCase) runBatchSideEffects(ctx context.Context, logger libLog.Logger, processedTransactions []processedTransactionBatchItem) {
	if len(processedTransactions) == 0 {
		return
	}

	workerCount := batchSideEffectsMaxWorkers
	if len(processedTransactions) < workerCount {
		workerCount = len(processedTransactions)
	}

	// Deep-copy transaction data into value types so goroutines don't share
	// mutable pointers with the caller after the function returns on timeout.
	jobs := make(chan sideEffectItem, len(processedTransactions))
	for _, item := range processedTransactions {
		jobs <- sideEffectItem{
			organizationID:   item.organizationID,
			ledgerID:         item.ledgerID,
			transactionID:    item.transaction.ID,
			decisionContract: item.decisionContract,
			transaction:      *item.transaction, // value copy
		}
	}

	close(jobs)

	var sideEffectsWG sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		sideEffectsWG.Add(1)

		go func() {
			defer sideEffectsWG.Done()

			for item := range jobs {
				func() {
					sideEffectCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second) //nolint:mnd
					defer cancel()

					tran := item.transaction // local copy for the goroutine
					touchedShards := uc.touchedShardsFromOperations(tran.Operations)
					uc.SendDecisionLifecycleEvent(sideEffectCtx, &tran, item.decisionContract, pkgTransaction.DecisionLifecycleActionPostingCompleted)
					uc.SendTransactionEvents(sideEffectCtx, &tran)
					uc.RemoveTransactionFromRedisQueue(sideEffectCtx, logger, item.organizationID, item.ledgerID, item.transactionID, touchedShards...)
				}()
			}
		}()
	}

	sideEffectsDone := make(chan struct{})

	go func() {
		sideEffectsWG.Wait()
		close(sideEffectsDone)
	}()

	select {
	case <-sideEffectsDone:
	case <-time.After(uc.batchSideEffectsTimeout()):
		logger.Warnf("Batch post-commit side effects still running after %s; continuing without blocking worker", uc.batchSideEffectsTimeout())
	}
}

func (uc *UseCase) runBatchDuplicateCleanup(ctx context.Context, logger libLog.Logger, duplicates []processedTransactionBatchItem) {
	if len(duplicates) == 0 {
		return
	}

	for _, item := range duplicates {
		if item.transaction == nil || strings.TrimSpace(item.transaction.ID) == "" {
			continue
		}

		touchedShards := uc.touchedShardsFromOperations(item.transaction.Operations)
		sideEffectCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second) //nolint:mnd
		uc.RemoveTransactionFromRedisQueue(sideEffectCtx, logger, item.organizationID, item.ledgerID, item.transaction.ID, touchedShards...)
		cancel()
	}
}

func (uc *UseCase) touchedShardsFromOperations(operations []*operation.Operation) []int {
	if uc == nil || uc.ShardRouter == nil || len(operations) == 0 {
		return nil
	}

	shardCount := uc.ShardRouter.ShardCount()
	unique := make(map[int]struct{})

	for _, op := range operations {
		if op == nil {
			continue
		}

		shardID := uc.ShardRouter.ResolveBalance(op.AccountAlias, op.BalanceKey)
		if shardID < 0 || shardID >= shardCount {
			continue
		}

		unique[shardID] = struct{}{}
	}

	if len(unique) == 0 {
		return nil
	}

	shards := make([]int, 0, len(unique))
	for shardID := range unique {
		shards = append(shards, shardID)
	}

	sort.Ints(shards)

	return shards
}

func (uc *UseCase) persistBatchPayloadInTx(
	ctx context.Context,
	logger libLog.Logger,
	tracer trace.Tracer,
	txRepo batchTransactionRepository,
	balanceRepo batchBalanceRepository,
	opRepo batchOperationRepository,
	balanceUpdates []balanceUpdateItem,
	transactionsToCreate []*transaction.Transaction,
	allOperations []*operation.Operation,
) error {
	dbTx, err := txRepo.BeginTx(ctx)
	if err != nil {
		return err
	}

	committed := false

	defer func() {
		if !committed {
			if rbErr := dbTx.Rollback(); rbErr != nil {
				logger.Errorf("Failed to rollback batch transaction: %v", rbErr)
			}
		}
	}()

	aggregatedBalanceUpdates, err := uc.buildBatchBalanceUpdates(balanceUpdates)
	if err != nil {
		return err
	}

	for _, updateGroup := range aggregatedBalanceUpdates {
		ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.create_balance_transaction_operations_batch.update_balances")

		balancesToUpdate := uc.filterStaleBalances(ctxProcessBalances, updateGroup.organizationID, updateGroup.ledgerID, updateGroup.balances, logger)
		if len(balancesToUpdate) == 0 {
			spanUpdateBalances.End()
			continue
		}

		err := balanceRepo.BalancesUpdateWithTx(ctxProcessBalances, dbTx, updateGroup.organizationID, updateGroup.ledgerID, balancesToUpdate)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "Failed to update balances", err)
			spanUpdateBalances.End()

			return err
		}

		spanUpdateBalances.End()
	}

	if err := txRepo.CreateBatchWithTx(ctx, dbTx, transactionsToCreate); err != nil {
		return err
	}

	if len(allOperations) > 0 {
		ctxProcessOperation, spanCreateOperation := tracer.Start(ctx, "command.create_balance_transaction_operations_batch.create_operations_batch")

		err := opRepo.CreateBatchWithTx(ctxProcessOperation, dbTx, allOperations)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, "Failed to batch create operations", err)
			spanCreateOperation.End()

			return err
		}

		spanCreateOperation.End()
	}

	if err := dbTx.Commit(); err != nil {
		return err
	}

	committed = true

	return nil
}

func isRetryableBatchPersistError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == "40P01" || pgErr.Code == "40001"
}

func (uc *UseCase) buildBatchBalanceUpdates(balanceUpdates []balanceUpdateItem) ([]balanceBatchUpdateGroup, error) {
	if len(balanceUpdates) == 0 {
		return nil, nil
	}

	type scopedState struct {
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		balancesByID   map[string]*mmodel.Balance
	}

	states := make(map[string]*scopedState)

	for _, update := range balanceUpdates {
		scopeKey := update.organizationID.String() + "|" + update.ledgerID.String()

		state, ok := states[scopeKey]
		if !ok {
			state = &scopedState{
				organizationID: update.organizationID,
				ledgerID:       update.ledgerID,
				balancesByID:   make(map[string]*mmodel.Balance, len(update.balances)),
			}
			states[scopeKey] = state
		}

		fromTo := make(map[string]pkgTransaction.Amount, len(update.validate.From)+len(update.validate.To))
		for alias, amount := range update.validate.From {
			fromTo[alias] = amount
		}

		for alias, amount := range update.validate.To {
			fromTo[alias] = amount
		}

		for _, current := range update.balances {
			if current == nil {
				continue
			}

			// Always use the accumulated state as the base when a balance was
			// already seen in this batch. This ensures that every transaction's
			// mutations are applied cumulatively — never skipped because of a
			// stale version number carried from the original Redis snapshot.
			//
			// Previous logic compared current.Version vs existing.Version and
			// would skip (continue) when current.Version < existing.Version,
			// silently dropping the operation. Under 100K TPS batching, multiple
			// transactions in the same batch frequently touch the same balance,
			// all carrying the same snapshot version. After the first transaction
			// increments the version, subsequent ones were being dropped.
			baseBalance := current
			if existing, exists := state.balancesByID[current.ID]; exists && existing != nil {
				baseBalance = existing
			}

			updatedAmounts, err := pkgTransaction.OperateBalances(fromTo[current.Alias], *baseBalance.ToTransactionBalance())
			if err != nil {
				return nil, err
			}

			state.balancesByID[current.ID] = &mmodel.Balance{
				ID:             baseBalance.ID,
				OrganizationID: baseBalance.OrganizationID,
				LedgerID:       baseBalance.LedgerID,
				AccountID:      baseBalance.AccountID,
				Alias:          baseBalance.Alias,
				Key:            baseBalance.Key,
				AssetCode:      baseBalance.AssetCode,
				Available:      updatedAmounts.Available,
				OnHold:         updatedAmounts.OnHold,
				Version:        updatedAmounts.Version,
				AccountType:    baseBalance.AccountType,
				AllowSending:   baseBalance.AllowSending,
				AllowReceiving: baseBalance.AllowReceiving,
			}
		}
	}

	groups := make([]balanceBatchUpdateGroup, 0, len(states))
	for _, state := range states {
		balances := make([]*mmodel.Balance, 0, len(state.balancesByID))
		for _, updatedBalance := range state.balancesByID {
			balances = append(balances, updatedBalance)
		}

		groups = append(groups, balanceBatchUpdateGroup{
			organizationID: state.organizationID,
			ledgerID:       state.ledgerID,
			balances:       balances,
		})
	}

	return groups, nil
}

func validateTransactionProcessingPayload(payload *transaction.TransactionProcessingPayload) error {
	if payload == nil {
		return errInvalidPayloadNil
	}

	if payload.DecisionContract.IsZero() {
		payload.DecisionContract = pkgTransaction.DefaultDecisionContract()
	} else {
		payload.DecisionContract = payload.DecisionContract.Normalize()
	}

	if payload.Transaction == nil {
		return errInvalidPayloadTransactionNil
	}

	if payload.Validate == nil {
		return errInvalidPayloadValidateNil
	}

	if payload.Transaction.Status.Code == constant.PENDING && payload.Input == nil {
		return errInvalidPayloadPendingInputNil
	}

	return nil
}

func validateOperationsNotNil(operations []*operation.Operation) error {
	for i, op := range operations {
		if op == nil {
			return fmt.Errorf("nil operation at index %d", i) //nolint:err113
		}

		if op.BalanceID != "" {
			if _, err := uuid.Parse(op.BalanceID); err != nil {
				return fmt.Errorf("invalid balance_id at operation index %d: %w", i, err)
			}
		}
	}

	return nil
}

func validateBalancesNotNil(balances []*mmodel.Balance) error {
	if balances == nil {
		return errInvalidPayloadBalancesNil
	}

	if len(balances) == 0 {
		return errInvalidPayloadBalancesEmpty
	}

	for i, balance := range balances {
		if balance == nil {
			return fmt.Errorf("invalid transaction payload: nil balance at index %d", i) //nolint:err113
		}
	}

	return nil
}

// CreateOrUpdateTransaction func that is responsible to create or update a transaction.
func (uc *UseCase) CreateOrUpdateTransaction(ctx context.Context, logger libLog.Logger, tracer trace.Tracer, t transaction.TransactionProcessingPayload) (*transaction.Transaction, bool, error) {
	_, spanCreateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanCreateTransaction.End()

	logger.Infof("Trying to create new transaction")

	tran := t.Transaction
	tran.Body = pkgTransaction.Transaction{}
	pendingValidate := t.Validate != nil && t.Validate.Pending

	switch tran.Status.Code {
	case constant.CREATED:
		description := constant.APPROVED
		status := transaction.Status{
			Code:        description,
			Description: &description,
		}

		tran.Status = status
	case constant.PENDING:
		if t.Input == nil {
			return nil, false, errInvalidPayloadPendingInputNil
		}

		tran.Body = *t.Input
	}

	created, err := uc.TransactionRepo.Create(ctx, tran)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateTransaction, "Failed to create transaction on repo", err)

		logger.Errorf("Failed to create transaction on repo: %v", err.Error())

		return nil, false, err
	}

	if created == nil {
		if pendingValidate && (tran.Status.Code == constant.APPROVED || tran.Status.Code == constant.CANCELED) {
			_, err = uc.UpdateTransactionStatus(ctx, tran)
			if err != nil {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateTransaction, "Failed to update transaction", err)

				logger.Warnf("Failed to update transaction with STATUS: %v by ID: %v", tran.Status.Code, tran.ID)

				return nil, false, err
			}
		}

		logger.Infof("skipping to create transaction, transaction already exists: %v", tran.ID)

		return tran, true, nil
	}

	return tran, false, nil
}

// CreateMetadataAsync func that create metadata into operations.
func (uc *UseCase) CreateMetadataAsync(ctx context.Context, logger libLog.Logger, metadata map[string]any, entityID, collection string) error {
	if metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   entityID,
			EntityName: collection,
			Data:       metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, collection, &meta); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return nil
			}

			logger.Errorf("Error into creating %s metadata: %v", collection, err)

			return err
		}
	}

	return nil
}

// CreateBTOSync func that create balance transaction operations synchronously.
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

// preFlightShard computes the deterministic shard index for a transaction's
// pre-flight backup using the same FNV hash as SendTransactionToRedisQueue.
func preFlightShard(transactionID string, shardCount int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(transactionID))

	return int(h.Sum32()) % shardCount
}

// RemoveTransactionFromRedisQueue removes backup entries for a transaction.
// When sharding is enabled and touchedShards is provided, only cleans those
// specific shard backup queues plus the legacy key. When touchedShards is
// empty (or sharding is disabled), computes the pre-flight shard using the
// same FNV hash as SendTransactionToRedisQueue and only cleans that shard
// plus the legacy key, avoiding an O(N) iteration over all shards.
//
//nolint:nestif
func (uc *UseCase) RemoveTransactionFromRedisQueue(ctx context.Context, logger libLog.Logger, organizationID, ledgerID uuid.UUID, transactionID string, touchedShards ...int) {
	if uc.ShardRouter != nil {
		shards := touchedShards
		if len(shards) == 0 {
			// No hint provided: compute the pre-flight shard deterministically
			// (same FNV hash as SendTransactionToRedisQueue) instead of iterating
			// all shards. This reduces Redis HDEL calls from N to 1.
			shards = []int{preFlightShard(transactionID, uc.ShardRouter.ShardCount())}
		}

		for _, shardID := range shards {
			shardKey := utils.TransactionShardKey(shardID, organizationID, ledgerID, transactionID)
			if err := uc.RedisRepo.RemoveMessageFromQueue(ctx, shardKey); err != nil {
				logger.Warnf("Backup queue: failed to remove shard %d key %s: %v", shardID, shardKey, err)
			} else {
				logger.Infof("Backup queue: removed transaction from shard %d with key %s", shardID, shardKey)
			}
		}

		legacyKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID)
		if err := uc.RedisRepo.RemoveMessageFromQueue(ctx, legacyKey); err != nil {
			logger.Warnf("Backup queue: failed to remove legacy key %s: %v", legacyKey, err)
		} else {
			logger.Infof("Backup queue: removed legacy transaction key %s", legacyKey)
		}
	} else {
		transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID)

		if err := uc.RedisRepo.RemoveMessageFromQueue(ctx, transactionKey); err != nil {
			logger.Warnf("Backup queue: failed to remove transaction %s: %s", transactionKey, err.Error())
		} else {
			logger.Infof("Backup queue: transaction removed with key %s", transactionKey)
		}
	}
}

// SendTransactionToRedisQueue writes a pre-flight transaction backup to Redis.
// When sharding is enabled, uses shard 0 as the control shard for pre-flight
// backups. The Lua script writes its own per-shard backups during execution.
func (uc *UseCase) SendTransactionToRedisQueue(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionInput pkgTransaction.Transaction, validate *pkgTransaction.Responses, transactionStatus string, transactionDate time.Time) error {
	logger, _, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	var transactionKey string

	if uc.ShardRouter != nil {
		// Distribute pre-flight backups across shards using a deterministic hash of
		// the transaction ID. The same transaction always maps to the same shard
		// (idempotency is preserved), but different transactions spread load evenly
		// across all shards instead of funnelling everything through shard 0.
		shard := preFlightShard(transactionID.String(), uc.ShardRouter.ShardCount())
		transactionKey = utils.TransactionShardKey(shard, organizationID, ledgerID, transactionID.String())
	} else {
		transactionKey = utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
	}

	utils.SanitizeAccountAliases(&transactionInput)

	queue := mmodel.TransactionRedisQueue{
		HeaderID:          reqId,
		OrganizationID:    organizationID,
		LedgerID:          ledgerID,
		TransactionID:     transactionID,
		TransactionInput:  transactionInput,
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

// UpdateTransactionBackupOperations updates the Redis backup queue entry
// for a transaction to include the materialized operations. This ensures
// that if the cron consumer reprocesses this backup, it uses the exact same
// operation IDs that were returned to the user.
//
// This is a best-effort operation: failures are logged but do not block
// the main transaction flow.
func (uc *UseCase) UpdateTransactionBackupOperations(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionID string, balances []*mmodel.Balance, operations []*operation.Operation) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction_backup_operations")
	defer span.End()

	// Update the backup entry for this transaction. When sharding is enabled,
	// the pre-flight backup lives on the deterministic FNV-hashed shard (same
	// logic as SendTransactionToRedisQueue), so we only need to check that
	// shard plus the legacy key -- not all N shards.
	candidateKeys := make([]string, 0, 2) //nolint:mnd

	if uc.ShardRouter != nil {
		shard := preFlightShard(transactionID, uc.ShardRouter.ShardCount())
		candidateKeys = append(candidateKeys, utils.TransactionShardKey(shard, organizationID, ledgerID, transactionID))
		candidateKeys = append(candidateKeys, utils.TransactionInternalKey(organizationID, ledgerID, transactionID))
	} else {
		candidateKeys = append(candidateKeys, utils.TransactionInternalKey(organizationID, ledgerID, transactionID))
	}

	var queue mmodel.TransactionRedisQueue

	redisOps := make([]mmodel.OperationRedis, 0, len(operations))
	for _, op := range operations {
		if op == nil {
			logger.Warn("Skipping nil operation while updating transaction backup operations")
			continue
		}

		redisOps = append(redisOps, op.ToRedis())
	}

	updatedCount := 0

	var lastErr error

	for _, transactionKey := range candidateKeys {
		raw, readErr := uc.RedisRepo.ReadMessageFromQueue(ctx, transactionKey)
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if err := json.Unmarshal(raw, &queue); err != nil {
			lastErr = err
			logger.Warnf("Failed to unmarshal transaction backup for key %s: %v", transactionKey, err)

			continue
		}

		queue.Operations = redisOps

		if len(balances) > 0 {
			redisBalances := make([]mmodel.BalanceRedis, 0, len(balances))
			for _, balance := range balances {
				if balance == nil {
					logger.Warn("Skipping nil balance while updating transaction backup operations")
					continue
				}

				redisBalances = append(redisBalances, mmodel.ToBalanceRedis(balance, balance.Alias))
			}

			queue.Balances = redisBalances
		}

		updated, err := json.Marshal(queue)
		if err != nil {
			lastErr = err
			logger.Warnf("Failed to marshal updated transaction backup for key %s: %v", transactionKey, err)

			continue
		}

		if err := uc.RedisRepo.AddMessageToQueue(ctx, transactionKey, updated); err != nil {
			lastErr = err
			logger.Warnf("Failed to write updated transaction backup for key %s: %v", transactionKey, err)

			continue
		}

		updatedCount++
	}

	if updatedCount == 0 {
		if lastErr != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to update transaction backup operations on any key", lastErr)
			logger.Warnf("Failed to update transaction backup operations for keys %v: %v", candidateKeys, lastErr)
		} else {
			logger.Warnf("No candidate backup keys found for transaction backup operations update")
		}
	}
}
