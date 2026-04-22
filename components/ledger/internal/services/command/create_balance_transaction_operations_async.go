// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/otel/trace"
)

// CreateBalanceTransactionOperationsAsync processes transaction asynchronously.
// This is an append-only handler for transactions and operations:
// - Hot balance already updated atomically by Lua script during validation
// - Cold balance scheduled for async sync via sorted set (Lua script does ZADD)
// - Transaction and operations persisted to database
// - Events sent asynchronously
//
// Balance persistence is fully async via BalanceSyncWorker.
// The Lua script (balance_atomic_operation.lua) does ZADD to schedule:balance-sync
// when scheduleSync=1, which is the default for all balance-affecting transactions.
func (uc *UseCase) CreateBalanceTransactionOperationsAsync(ctx context.Context, data mmodel.Queue) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	var t transaction.TransactionProcessingPayload

	for _, item := range data.QueueData {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Unmarshal account ID: %v", item.ID.String()))

		err := msgpack.Unmarshal(item.Value, &t)
		if err != nil {
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("failed to unmarshal response: %v", err.Error()))

			return err
		}
	}

	if t.Transaction == nil {
		logger.Log(ctx, libLog.LevelError, "Transaction payload has nil Transaction field")

		return fmt.Errorf("transaction payload has nil Transaction field")
	}

	backupStatusForCleanup := utils.ExpectedBackupStatusForCleanup(t.Transaction.Status.Code, t.Validate)

	// Legacy payload compatibility: messages from v3.5.x lack the Version field.
	// Their balance persistence relied on UpdateBalances() in the consumer, which
	// was removed in v3.6.x (replaced by BalanceSyncWorker). Without this fallback,
	// balances for in-flight v3.5.x messages would never reach PostgreSQL.
	if t.Version == "" && t.Transaction.Status.Code != constant.NOTED {
		logger.Log(ctx, libLog.LevelWarn, "Legacy payload detected (no Version field), calling UpdateBalances for backward compatibility")

		if err := uc.UpdateBalances(ctx, data.OrganizationID, data.LedgerID, *t.Validate, t.Balances, t.BalancesAfter); err != nil {
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to update balances for legacy payload: %v", err))

			return err
		}
	}

	// Note: Balance updates are handled by BalanceSyncWorker asynchronously.
	// Hot balance was already updated atomically by Lua script during validation.
	// Cold balance persistence is scheduled via ZADD to schedule:balance-sync.

	ctxProcessTransaction, spanUpdateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanUpdateTransaction.End()

	tran, err := uc.CreateOrUpdateTransaction(ctxProcessTransaction, logger, tracer, t)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanUpdateTransaction, "Failed to create or update transaction", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create or update transaction: %v", err.Error()))

		return err
	}

	ctxProcessMetadata, spanCreateMetadata := tracer.Start(ctx, "command.create_balance_transaction_operations.create_metadata")
	defer spanCreateMetadata.End()

	err = uc.CreateMetadataAsync(ctxProcessMetadata, logger, tran.Metadata, tran.ID, reflect.TypeOf(transaction.Transaction{}).Name())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanCreateMetadata, "Failed to create metadata on transaction", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create metadata on transaction: %v", err.Error()))

		return err
	}

	ctxProcessOperation, spanCreateOperation := tracer.Start(ctx, "command.create_balance_transaction_operations.create_operation")
	defer spanCreateOperation.End()

	logger.Log(ctx, libLog.LevelInfo, "Trying to create new operations")

	for _, oper := range tran.Operations {
		if err := validateOperationDirection(ctx, logger, oper); err != nil {
			return err
		}

		_, err = uc.OperationRepo.Create(ctxProcessOperation, oper)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
				msg := fmt.Sprintf("Skipping to create operation, operation already exists: %v", oper.ID)

				libOpentelemetry.HandleSpanBusinessErrorEvent(spanCreateOperation, msg, err)

				logger.Log(ctx, libLog.LevelWarn, msg)

				continue
			} else {
				libOpentelemetry.HandleSpanBusinessErrorEvent(spanCreateOperation, "Failed to create operation", err)

				logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating operation: %v", err))

				return err
			}
		}

		err = uc.CreateMetadataAsync(ctx, logger, oper.Metadata, oper.ID, reflect.TypeOf(operation.Operation{}).Name())
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanCreateOperation, "Failed to create metadata on operation", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create metadata on operation: %v", err))

			return err
		}
	}

	go uc.SendTransactionEvents(ctx, tran)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Backup queue: cleaning up transaction %s after successful processing", tran.ID))

	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		if backupStatusForCleanup == "" {
			backupStatusForCleanup = utils.ExpectedBackupStatusForCleanup(tran.Status.Code, t.Validate)
		}

		go uc.RemoveTransactionFromRedisQueueIfStatus(ctx, logger, data.OrganizationID, data.LedgerID, tran.ID, backupStatusForCleanup)
	} else {
		go uc.RemoveTransactionFromRedisQueue(ctx, logger, data.OrganizationID, data.LedgerID, tran.ID)
	}

	uc.DeleteWriteBehindTransaction(ctx, data.OrganizationID, data.LedgerID, tran.ID)

	return nil
}

// CreateOrUpdateTransaction func that is responsible to create or update a transaction.
func (uc *UseCase) CreateOrUpdateTransaction(ctx context.Context, logger libLog.Logger, tracer trace.Tracer, t transaction.TransactionProcessingPayload) (*transaction.Transaction, error) {
	_, spanCreateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanCreateTransaction.End()

	logger.Log(ctx, libLog.LevelInfo, "Trying to create new transaction")

	tran := t.Transaction
	tran.Body = mtransaction.Transaction{}

	switch tran.Status.Code {
	case constant.CREATED:
		description := constant.APPROVED
		status := transaction.Status{
			Code:        description,
			Description: &description,
		}

		tran.Status = status
	case constant.PENDING:
		tran.Body = *t.Input
	}

	_, err := uc.TransactionRepo.Create(ctx, tran)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
			if t.Validate != nil && t.Validate.Pending && (tran.Status.Code == constant.APPROVED || tran.Status.Code == constant.CANCELED) {
				_, err = uc.UpdateTransactionStatus(ctx, tran)
				if err != nil {
					libOpentelemetry.HandleSpanBusinessErrorEvent(spanCreateTransaction, "Failed to update transaction", err)

					logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to update transaction with STATUS: %v by ID: %v", tran.Status.Code, tran.ID))

					return nil, err
				}
			}

			logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("skipping to create transaction, transaction already exists: %v", tran.ID))
		} else {
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanCreateTransaction, "Failed to create transaction on repo", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create transaction on repo: %v", err.Error()))

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

		if err := uc.TransactionMetadataRepo.Create(ctx, collection, &meta); err != nil {
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error into creating %s metadata: %v", collection, err))

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
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create balance transaction operations: %v", err))

		return err
	}

	return nil
}

// RemoveTransactionFromRedisQueue func that remove transaction from redis queue
func (uc *UseCase) RemoveTransactionFromRedisQueue(ctx context.Context, logger libLog.Logger, organizationID, ledgerID uuid.UUID, transactionID string) {
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID)

	if err := uc.TransactionRedisRepo.RemoveMessageFromQueue(ctx, transactionKey); err != nil {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Backup queue: failed to remove transaction %s: %s", transactionKey, err.Error()))
	} else {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Backup queue: transaction removed successfully from backup_queue:{transactions} with key %s", transactionKey))
	}
}

// RemoveTransactionFromRedisQueueIfStatus removes a backup entry only when the
// current queue payload still matches the expected transaction status.
//
// This prevents stale consumers from deleting a newer backup stage for the
// same transaction ID (e.g. late PENDING-create worker removing a newer
// APPROVED/CANCELED backup written by commit/cancel flow).
func (uc *UseCase) RemoveTransactionFromRedisQueueIfStatus(ctx context.Context, logger libLog.Logger, organizationID, ledgerID uuid.UUID, transactionID, expectedStatus string) {
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID)

	raw, err := uc.TransactionRedisRepo.ReadMessageFromQueue(ctx, transactionKey)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Backup queue: failed to read transaction %s before conditional cleanup: %v", transactionKey, err))
		return
	}

	var queue mmodel.TransactionRedisQueue
	if err := json.Unmarshal(raw, &queue); err != nil {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Backup queue: failed to decode transaction %s before conditional cleanup: %v", transactionKey, err))
		return
	}

	if !strings.EqualFold(queue.TransactionStatus, expectedStatus) {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
			"Backup queue: skip cleanup for transaction %s because status changed (expected=%s current=%s)",
			transactionKey,
			expectedStatus,
			queue.TransactionStatus,
		))

		return
	}

	uc.RemoveTransactionFromRedisQueue(ctx, logger, organizationID, ledgerID, transactionID)
}

// SendTransactionToRedisQueue func that send transaction to redis queue.
// When balances is non-nil (e.g. commit/cancel flows), the snapshot is included
// directly in the backup message so the Redis consumer can retry without relying
// on the Lua script to populate them.
func (uc *UseCase) SendTransactionToRedisQueue(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionInput mtransaction.Transaction, validate *mtransaction.Responses, transactionStatus, action string, transactionDate time.Time, balances []*mmodel.Balance) error {
	logger, _, reqId, _ := libCommons.NewTrackingFromContext(ctx)
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

	// Scope protection: a transaction that targets any internal-scope
	// balance (e.g. auto-created overdraft reserves) MUST be rejected
	// BEFORE the transaction is published to the Redis queue. This keeps
	// system-managed balances out of the user-initiated mutation path
	// across every entry point (HTTP, gRPC, DSL).
	for _, b := range balances {
		if b != nil && b.Settings != nil && b.Settings.BalanceScope == mmodel.BalanceScopeInternal {
			err := pkg.ValidateBusinessError(constant.ErrDirectOperationOnInternalBalance, reflect.TypeOf(mmodel.Balance{}).Name(), b.Alias)
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Rejected transaction targeting internal balance alias=%s key=%s", b.Alias, b.Key))

			return err
		}
	}

	utils.SanitizeAccountAliases(&transactionInput)

	var balanceRedis []mmodel.BalanceRedis

	if balances != nil {
		balanceRedis = make([]mmodel.BalanceRedis, 0, len(balances))

		for _, b := range balances {
			allowSending := 0
			if b.AllowSending {
				allowSending = 1
			}

			allowReceiving := 0
			if b.AllowReceiving {
				allowReceiving = 1
			}

			balanceRedis = append(balanceRedis, mmodel.BalanceRedis{
				ID:             b.ID,
				Alias:          b.Alias,
				Key:            b.Key,
				AccountID:      b.AccountID,
				AssetCode:      b.AssetCode,
				Available:      b.Available,
				OnHold:         b.OnHold,
				Version:        b.Version,
				AccountType:    b.AccountType,
				AllowSending:   allowSending,
				AllowReceiving: allowReceiving,
			})
		}
	}

	queue := mmodel.TransactionRedisQueue{
		HeaderID:          reqId,
		OrganizationID:    organizationID,
		LedgerID:          ledgerID,
		TransactionID:     transactionID,
		TransactionInput:  transactionInput,
		Balances:          balanceRedis,
		TTL:               time.Now(),
		Validate:          validate,
		TransactionStatus: transactionStatus,
		Action:            action,
		TransactionDate:   transactionDate,
	}

	raw, err := json.Marshal(queue)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to marshal transaction to json string: %s", err.Error()))

		return constant.ErrTransactionBackupCacheMarshalFailed
	}

	err = uc.TransactionRedisRepo.AddMessageToQueue(ctx, transactionKey, raw)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to send transaction to redis queue: %s", err.Error()))

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
func (uc *UseCase) UpdateTransactionBackupOperations(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionID string, operations []*operation.Operation, actionOverride ...string) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction_backup_operations")
	defer span.End()

	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID)

	raw, err := uc.TransactionRedisRepo.ReadMessageFromQueue(ctx, transactionKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read transaction backup for operations update", err)
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to read transaction backup for operations update: %v", err))

		return
	}

	var queue mmodel.TransactionRedisQueue
	if err := json.Unmarshal(raw, &queue); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to unmarshal transaction backup for operations update", err)
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to unmarshal transaction backup for operations update: %v", err))

		return
	}

	redisOps := make([]mmodel.OperationRedis, 0, len(operations))
	for _, op := range operations {
		redisOps = append(redisOps, op.ToRedis())
	}

	queue.Operations = redisOps

	if len(actionOverride) > 0 && actionOverride[0] != "" {
		queue.Action = actionOverride[0]
	}

	updated, err := json.Marshal(queue)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal updated transaction backup", err)
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to marshal updated transaction backup: %v", err))

		return
	}

	if err := uc.TransactionRedisRepo.AddMessageToQueue(ctx, transactionKey, updated); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to write updated transaction backup with operations", err)
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to write updated transaction backup with operations: %v", err))
	}
}

// validateOperationDirection checks the direction field of an operation.
// Empty direction is allowed with a warning (v3.5.3 messages lack this field).
// Non-empty direction must be one of the valid values ("debit", "credit").
func validateOperationDirection(ctx context.Context, logger libLog.Logger, oper *operation.Operation) error {
	if oper.Direction == "" {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf(
			"Operation %s has empty direction, may be from pre-migration message", oper.ID))

		return nil
	}

	switch strings.ToLower(oper.Direction) {
	case "debit", "credit":
		return nil
	default:
		return fmt.Errorf("operation %s has invalid direction %q: must be 'debit' or 'credit'", oper.ID, oper.Direction)
	}
}
