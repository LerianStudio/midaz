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
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/otel/trace"
)

// BulkResult aggregates the results from bulk insert operations.
// It contains counts for both transactions and operations.
type BulkResult struct {
	// Transaction insert results
	TransactionsAttempted int64
	TransactionsInserted  int64
	TransactionsIgnored   int64

	// Operation insert results
	OperationsAttempted int64
	OperationsInserted  int64
	OperationsIgnored   int64

	// Fallback tracking
	FallbackUsed  bool
	FallbackCount int
}

// BulkMessageResult tracks the processing result for individual messages
// when fallback mode is used.
type BulkMessageResult struct {
	Index   int
	Success bool
	Error   error
}

// CreateBulkTransactionOperationsAsync processes multiple transaction messages in bulk.
// This method:
// 1. Unmarshals all payloads from the queue messages
// 2. Processes balances for each transaction (required for hot balance)
// 3. Extracts and sorts transactions/operations by ID (deadlock prevention)
// 4. Performs bulk INSERTs for transactions and operations
// 5. On bulk failure, falls back to individual processing if enabled
//
// The method returns a BulkResult with counts of attempted/inserted/ignored rows.
// Individual message acknowledgment is the responsibility of the caller.
func (uc *UseCase) CreateBulkTransactionOperationsAsync(
	ctx context.Context,
	messages []mmodel.Queue,
	fallbackEnabled bool,
) (*BulkResult, []BulkMessageResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_bulk_transaction_operations_async")
	defer span.End()

	result := &BulkResult{}
	messageResults := make([]BulkMessageResult, len(messages))

	// Initialize all message results as pending
	for i := range messageResults {
		messageResults[i] = BulkMessageResult{Index: i, Success: false}
	}

	if len(messages) == 0 {
		return result, messageResults, nil
	}

	logger.Log(ctx, libLog.LevelInfo, "Starting bulk transaction processing",
		libLog.Int("messageCount", len(messages)))

	// Phase 1: Unmarshal all payloads and process balances
	payloads, unmarshalResults := uc.unmarshalPayloads(ctx, logger, messages)

	// Always merge unmarshal results into messageResults (handles partial failures)
	for i, res := range unmarshalResults {
		if !res.Success {
			messageResults[i] = res
		}
	}

	// Count successful unmarshals
	successfulUnmarshals := 0

	for _, p := range payloads {
		if p != nil {
			successfulUnmarshals++
		}
	}

	if successfulUnmarshals == 0 {
		return result, messageResults, fmt.Errorf("all messages failed to unmarshal")
	}

	// Phase 2: Process balances for each transaction (required before insert)
	balanceResults := uc.processBalances(ctx, logger, tracer, messages, payloads)

	// Track which messages had balance failures
	validPayloadIndices := make([]int, 0, len(payloads))

	for i, res := range balanceResults {
		if !res.Success {
			messageResults[i] = res
		} else {
			validPayloadIndices = append(validPayloadIndices, i)
		}
	}

	if len(validPayloadIndices) == 0 {
		return result, messageResults, fmt.Errorf("all messages failed balance processing")
	}

	// Phase 3: Extract transactions and operations from valid payloads
	transactions, operations := uc.extractEntities(payloads, validPayloadIndices)

	// Phase 4: Sort by ID to prevent deadlocks
	sortTransactionsByID(transactions)
	sortOperationsByID(operations)

	// Phase 5: Attempt bulk insert
	bulkErr := uc.performBulkInsert(ctx, logger, tracer, transactions, operations, result)
	if bulkErr != nil {
		logger.Log(ctx, libLog.LevelWarn, "Bulk insert failed, checking fallback",
			libLog.Err(bulkErr),
			libLog.Bool("fallbackEnabled", fallbackEnabled))

		if !fallbackEnabled {
			// No fallback: mark all valid messages as failed
			for _, idx := range validPayloadIndices {
				messageResults[idx] = BulkMessageResult{Index: idx, Success: false, Error: bulkErr}
			}

			return result, messageResults, bulkErr
		}

		// Phase 6: Fallback to individual processing
		result.FallbackUsed = true
		fallbackResults := uc.processFallback(ctx, logger, tracer, messages, payloads, validPayloadIndices)

		for _, res := range fallbackResults {
			messageResults[res.Index] = res
			if res.Success {
				result.FallbackCount++
			}
		}

		return result, messageResults, nil
	}

	// Bulk insert succeeded: mark all valid messages as successful
	for _, idx := range validPayloadIndices {
		messageResults[idx] = BulkMessageResult{Index: idx, Success: true}
	}

	// Phase 7: Post-processing (metadata, events) - best effort
	uc.postProcessBulk(ctx, logger, payloads, validPayloadIndices, messages)

	logger.Log(ctx, libLog.LevelInfo, "Bulk transaction processing completed",
		libLog.Any("transactionsInserted", result.TransactionsInserted),
		libLog.Any("transactionsIgnored", result.TransactionsIgnored),
		libLog.Any("operationsInserted", result.OperationsInserted),
		libLog.Any("operationsIgnored", result.OperationsIgnored))

	return result, messageResults, nil
}

// unmarshalPayloads unmarshals all queue messages into TransactionProcessingPayload.
func (uc *UseCase) unmarshalPayloads(
	ctx context.Context,
	logger libLog.Logger,
	messages []mmodel.Queue,
) ([]*transaction.TransactionProcessingPayload, []BulkMessageResult) {
	payloads := make([]*transaction.TransactionProcessingPayload, len(messages))
	results := make([]BulkMessageResult, len(messages))

	for i, msg := range messages {
		results[i] = BulkMessageResult{Index: i, Success: true}

		// Validate exactly one queue item per message
		if len(msg.QueueData) != 1 {
			logger.Log(ctx, libLog.LevelError, "Invalid QueueData count in message",
				libLog.Int("index", i),
				libLog.Int("count", len(msg.QueueData)))

			results[i] = BulkMessageResult{Index: i, Success: false, Error: fmt.Errorf("expected 1 queue item, got %d", len(msg.QueueData))}
			payloads[i] = nil

			continue
		}

		var payload transaction.TransactionProcessingPayload

		if err := msgpack.Unmarshal(msg.QueueData[0].Value, &payload); err != nil {
			logger.Log(ctx, libLog.LevelError, "Failed to unmarshal queue message",
				libLog.Int("index", i),
				libLog.Err(err))

			results[i] = BulkMessageResult{Index: i, Success: false, Error: err}
			payloads[i] = nil

			continue
		}

		payloads[i] = &payload
	}

	return payloads, results
}

// processBalances processes balance updates for each transaction.
func (uc *UseCase) processBalances(
	ctx context.Context,
	logger libLog.Logger,
	tracer trace.Tracer,
	messages []mmodel.Queue,
	payloads []*transaction.TransactionProcessingPayload,
) []BulkMessageResult {
	results := make([]BulkMessageResult, len(payloads))

	for i, payload := range payloads {
		results[i] = BulkMessageResult{Index: i, Success: true}

		if payload == nil {
			results[i] = BulkMessageResult{Index: i, Success: false, Error: errors.New("nil payload")}
			continue
		}

		// Skip balance update for NOTED transactions
		if payload.Transaction != nil && payload.Transaction.Status.Code == constant.NOTED {
			continue
		}

		if payload.Validate == nil {
			results[i] = BulkMessageResult{Index: i, Success: false, Error: errors.New("nil Validate field")}
			continue
		}

		ctxBalance, spanBalance := tracer.Start(ctx, "command.bulk.update_balances")

		err := uc.UpdateBalances(ctxBalance, messages[i].OrganizationID, messages[i].LedgerID,
			*payload.Validate, payload.Balances, payload.BalancesAfter)

		spanBalance.End()

		if err != nil {
			logger.Log(ctx, libLog.LevelError, "Failed to update balances in bulk",
				libLog.Int("index", i),
				libLog.Err(err))

			results[i] = BulkMessageResult{Index: i, Success: false, Error: err}
		}
	}

	return results
}

// extractEntities extracts transactions and operations from valid payloads.
func (uc *UseCase) extractEntities(
	payloads []*transaction.TransactionProcessingPayload,
	validIndices []int,
) ([]*transaction.Transaction, []*operation.Operation) {
	transactions := make([]*transaction.Transaction, 0, len(validIndices))
	operations := make([]*operation.Operation, 0, len(validIndices)*5) // Estimate 5 ops per transaction

	for _, idx := range validIndices {
		payload := payloads[idx]
		if payload == nil || payload.Transaction == nil {
			continue
		}

		tran := payload.Transaction

		// Clear body for storage (same as individual processing)
		tran.Body = pkgTransaction.Transaction{}

		// Update status for CREATED transactions
		if tran.Status.Code == constant.CREATED {
			description := constant.APPROVED
			tran.Status = transaction.Status{
				Code:        description,
				Description: &description,
			}
		} else if tran.Status.Code == constant.PENDING && payload.Input != nil {
			tran.Body = *payload.Input
		}

		transactions = append(transactions, tran)

		// Collect operations
		for _, op := range tran.Operations {
			if op != nil {
				operations = append(operations, op)
			}
		}
	}

	return transactions, operations
}

// performBulkInsert executes bulk inserts for transactions and operations.
func (uc *UseCase) performBulkInsert(
	ctx context.Context,
	logger libLog.Logger,
	tracer trace.Tracer,
	transactions []*transaction.Transaction,
	operations []*operation.Operation,
	result *BulkResult,
) error {
	// Bulk insert transactions
	ctxTxInsert, spanTxInsert := tracer.Start(ctx, "command.bulk.insert_transactions")

	txResult, err := uc.TransactionRepo.CreateBulk(ctxTxInsert, transactions)

	spanTxInsert.End()

	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Bulk transaction insert failed", libLog.Err(err))

		return fmt.Errorf("bulk transaction insert failed: %w", err)
	}

	result.TransactionsAttempted = txResult.Attempted
	result.TransactionsInserted = txResult.Inserted
	result.TransactionsIgnored = txResult.Ignored

	logger.Log(ctx, libLog.LevelDebug, "Bulk transaction insert completed",
		libLog.Any("attempted", txResult.Attempted),
		libLog.Any("inserted", txResult.Inserted),
		libLog.Any("ignored", txResult.Ignored))

	// Bulk insert operations
	ctxOpInsert, spanOpInsert := tracer.Start(ctx, "command.bulk.insert_operations")

	opResult, err := uc.OperationRepo.CreateBulk(ctxOpInsert, operations)

	spanOpInsert.End()

	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Bulk operation insert failed", libLog.Err(err))

		return fmt.Errorf("bulk operation insert failed: %w", err)
	}

	result.OperationsAttempted = opResult.Attempted
	result.OperationsInserted = opResult.Inserted
	result.OperationsIgnored = opResult.Ignored

	logger.Log(ctx, libLog.LevelDebug, "Bulk operation insert completed",
		libLog.Any("attempted", opResult.Attempted),
		libLog.Any("inserted", opResult.Inserted),
		libLog.Any("ignored", opResult.Ignored))

	return nil
}

// processFallback processes each message individually when bulk insert fails.
func (uc *UseCase) processFallback(
	ctx context.Context,
	logger libLog.Logger,
	tracer trace.Tracer,
	messages []mmodel.Queue,
	payloads []*transaction.TransactionProcessingPayload,
	validIndices []int,
) []BulkMessageResult {
	results := make([]BulkMessageResult, 0, len(validIndices))

	logger.Log(ctx, libLog.LevelInfo, "Starting fallback to individual processing",
		libLog.Int("messageCount", len(validIndices)))

	for _, idx := range validIndices {
		payload := payloads[idx]
		msg := messages[idx]

		if payload == nil || payload.Transaction == nil {
			results = append(results, BulkMessageResult{Index: idx, Success: false, Error: errors.New("nil payload")})
			continue
		}

		ctxFallback, spanFallback := tracer.Start(ctx, "command.bulk.fallback_individual")

		// Try to create transaction individually
		tran, err := uc.createTransactionIndividually(ctxFallback, logger, tracer, payload)

		spanFallback.End()

		if err != nil {
			// Check if it's a duplicate error (idempotency - treat as success)
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
				logger.Log(ctx, libLog.LevelInfo, "Fallback: transaction already exists (duplicate)",
					libLog.Int("index", idx),
					libLog.String("transactionID", payload.Transaction.ID))

				results = append(results, BulkMessageResult{Index: idx, Success: true})

				continue
			}

			logger.Log(ctx, libLog.LevelError, "Fallback: failed to create transaction",
				libLog.Int("index", idx),
				libLog.Err(err))

			results = append(results, BulkMessageResult{Index: idx, Success: false, Error: err})

			continue
		}

		// Create operations individually
		allOpsSuccess := true

		for _, op := range tran.Operations {
			// Defensive nil check to prevent panic
			if op == nil {
				continue
			}

			_, opErr := uc.OperationRepo.Create(ctx, op)
			if opErr != nil {
				var pgErr *pgconn.PgError
				if errors.As(opErr, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
					// Duplicate operation is fine (idempotency)
					continue
				}

				logger.Log(ctx, libLog.LevelError, "Fallback: failed to create operation",
					libLog.Int("index", idx),
					libLog.String("operationID", op.ID),
					libLog.Err(opErr))

				allOpsSuccess = false

				break
			}
		}

		if allOpsSuccess {
			results = append(results, BulkMessageResult{Index: idx, Success: true})

			// Post-processing for successful fallback
			// Use detached context so async work completes even if request context is canceled
			asyncCtx := context.WithoutCancel(ctx)
			go uc.SendTransactionEvents(asyncCtx, tran)
			go uc.RemoveTransactionFromRedisQueue(asyncCtx, logger, msg.OrganizationID, msg.LedgerID, tran.ID)
			go uc.DeleteWriteBehindTransaction(asyncCtx, msg.OrganizationID, msg.LedgerID, tran.ID)
		} else {
			results = append(results, BulkMessageResult{Index: idx, Success: false, Error: errors.New("operation insert failed")})
		}
	}

	return results
}

// createTransactionIndividually creates a single transaction using the existing pattern.
func (uc *UseCase) createTransactionIndividually(
	ctx context.Context,
	logger libLog.Logger,
	tracer trace.Tracer,
	payload *transaction.TransactionProcessingPayload,
) (*transaction.Transaction, error) {
	tran, err := uc.CreateOrUpdateTransaction(ctx, logger, tracer, *payload)
	if err != nil {
		return nil, err
	}

	// Create metadata for transaction
	if err := uc.CreateMetadataAsync(ctx, logger, tran.Metadata, tran.ID, reflect.TypeOf(transaction.Transaction{}).Name()); err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Fallback: failed to create transaction metadata",
			libLog.String("transactionID", tran.ID),
			libLog.Err(err))
		// Continue despite metadata error
	}

	return tran, nil
}

// postProcessBulk handles metadata creation and event sending for bulk-inserted transactions.
func (uc *UseCase) postProcessBulk(
	ctx context.Context,
	logger libLog.Logger,
	payloads []*transaction.TransactionProcessingPayload,
	validIndices []int,
	messages []mmodel.Queue,
) {
	for _, idx := range validIndices {
		payload := payloads[idx]
		msg := messages[idx]

		if payload == nil || payload.Transaction == nil {
			continue
		}

		tran := payload.Transaction

		// Create metadata (best effort)
		if err := uc.CreateMetadataAsync(ctx, logger, tran.Metadata, tran.ID, reflect.TypeOf(transaction.Transaction{}).Name()); err != nil {
			logger.Log(ctx, libLog.LevelWarn, "Bulk: failed to create transaction metadata",
				libLog.String("transactionID", tran.ID),
				libLog.Err(err))
		}

		// Create operation metadata
		for _, op := range tran.Operations {
			if op != nil && op.Metadata != nil {
				if err := uc.CreateMetadataAsync(ctx, logger, op.Metadata, op.ID, reflect.TypeOf(operation.Operation{}).Name()); err != nil {
					logger.Log(ctx, libLog.LevelWarn, "Bulk: failed to create operation metadata",
						libLog.String("operationID", op.ID),
						libLog.Err(err))
				}
			}
		}

		// Async cleanup and events
		// Use detached context so async work completes even if request context is canceled
		asyncCtx := context.WithoutCancel(ctx)
		go uc.SendTransactionEvents(asyncCtx, tran)
		go uc.RemoveTransactionFromRedisQueue(asyncCtx, logger, msg.OrganizationID, msg.LedgerID, tran.ID)
		go uc.DeleteWriteBehindTransaction(asyncCtx, msg.OrganizationID, msg.LedgerID, tran.ID)
	}
}

// sortTransactionsByID sorts transactions by ID to prevent deadlocks during bulk insert.
// Nil elements are sorted to the beginning of the slice (defensive programming).
func sortTransactionsByID(transactions []*transaction.Transaction) {
	sort.Slice(transactions, func(i, j int) bool {
		// Defensive nil checks to prevent panic
		if transactions[i] == nil {
			return true // nil sorts first
		}

		if transactions[j] == nil {
			return false
		}

		return transactions[i].ID < transactions[j].ID
	})
}

// sortOperationsByID sorts operations by ID to prevent deadlocks during bulk insert.
// Nil elements are sorted to the beginning of the slice (defensive programming).
func sortOperationsByID(operations []*operation.Operation) {
	sort.Slice(operations, func(i, j int) bool {
		// Defensive nil checks to prevent panic
		if operations[i] == nil {
			return true // nil sorts first
		}

		if operations[j] == nil {
			return false
		}

		return operations[i].ID < operations[j].ID
	})
}
