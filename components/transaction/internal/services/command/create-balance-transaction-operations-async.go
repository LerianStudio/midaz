// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"reflect"
	"time"

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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vmihailenco/msgpack/v5"
	"go.mongodb.org/mongo-driver/mongo"
	"go.opentelemetry.io/otel/trace"
)

// CreateBalanceTransactionOperationsAsync func that is responsible to create all transactions at the same async.
func (uc *UseCase) CreateBalanceTransactionOperationsAsync(ctx context.Context, data mmodel.Queue) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	if len(data.QueueData) == 0 {
		return fmt.Errorf("invalid queue payload: empty queue data")
	}

	if len(data.QueueData) > 1 {
		return fmt.Errorf("invalid queue payload: expected exactly 1 queue data item, got %d", len(data.QueueData))
	}

	var t transaction.TransactionProcessingPayload

	item := data.QueueData[0]
	logger.Infof("Unmarshal account ID: %v", item.ID.String())

	err := msgpack.Unmarshal(item.Value, &t)
	if err != nil {
		logger.Errorf("failed to unmarshal response: %v", err.Error())

		return err
	}

	if t.Transaction == nil {
		return fmt.Errorf("invalid transaction payload: transaction is nil")
	}

	if t.Validate == nil {
		return fmt.Errorf("invalid transaction payload: validate is nil")
	}

	if t.Transaction.Status.Code == constant.PENDING && t.Input == nil {
		return fmt.Errorf("invalid transaction payload: pending transaction input is nil")
	}

	// NOTED transactions do not mutate balances, so balance payload is optional.
	if t.Transaction.Status.Code != constant.NOTED {
		if err := validateBalancesNotNil(t.Balances); err != nil {
			return err
		}

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
	go func() {
		sideEffectCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()

		uc.SendTransactionEvents(sideEffectCtx, tran)
	}()

	logger.Infof("Backup queue: cleaning up transaction %s after successful processing", tran.ID)

	// touchedShards is not passed because the async payload
	// (TransactionProcessingPayload) does not carry shard IDs — they were
	// resolved during the original sync Redis Lua execution and are not
	// serialized into the broker message. RemoveTransactionFromRedisQueue
	// computes the pre-flight shard deterministically using the same FNV hash
	// as SendTransactionToRedisQueue, so only 1 shard + legacy key is cleaned.
	go func() {
		sideEffectCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()

		uc.RemoveTransactionFromRedisQueue(sideEffectCtx, logger, data.OrganizationID, data.LedgerID, tran.ID)
	}()

	return nil
}

func validateOperationsNotNil(operations []*operation.Operation) error {
	for i, op := range operations {
		if op == nil {
			return fmt.Errorf("nil operation at index %d", i)
		}
	}

	return nil
}

func validateBalancesNotNil(balances []*mmodel.Balance) error {
	if balances == nil {
		return fmt.Errorf("invalid transaction payload: balances is nil")
	}

	for i, balance := range balances {
		if balance == nil {
			return fmt.Errorf("invalid transaction payload: nil balance at index %d", i)
		}
	}

	return nil
}

// CreateOrUpdateTransaction func that is responsible to create or update a transaction.
func (uc *UseCase) CreateOrUpdateTransaction(ctx context.Context, logger libLog.Logger, tracer trace.Tracer, t transaction.TransactionProcessingPayload) (*transaction.Transaction, error) {
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
			return nil, fmt.Errorf("invalid transaction payload: pending transaction input is nil")
		}

		tran.Body = *t.Input
	}

	_, err := uc.TransactionRepo.Create(ctx, tran)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
			if pendingValidate && (tran.Status.Code == constant.APPROVED || tran.Status.Code == constant.CANCELED) {
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

// CreateMetadataAsync func that create metadata into operations
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
			if mongo.IsDuplicateKeyError(err) {
				return nil
			}

			logger.Errorf("Error into creating %s metadata: %v", collection, err)

			return err
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
func (uc *UseCase) UpdateTransactionBackupOperations(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionID string, operations []*operation.Operation) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction_backup_operations")
	defer span.End()

	// Update the backup entry for this transaction. When sharding is enabled,
	// the pre-flight backup lives on the deterministic FNV-hashed shard (same
	// logic as SendTransactionToRedisQueue), so we only need to check that
	// shard plus the legacy key -- not all N shards.
	candidateKeys := make([]string, 0, 2)

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
