// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libRuntime "github.com/LerianStudio/lib-observability/v2/runtime"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/otel/trace"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
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
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	var t transaction.TransactionProcessingPayload

	for _, item := range data.QueueData {
		err := msgpack.Unmarshal(item.Value, &t)
		if err != nil {
			logger.Log(ctx, libLog.LevelError, "failed to unmarshal response", libLog.Err(err))

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
			logger.Log(ctx, libLog.LevelError, "Failed to update balances for legacy payload", libLog.Err(err))

			return err
		}
	}

	// Note: Balance updates are handled by BalanceSyncWorker asynchronously.
	// Hot balance was already updated atomically by Lua script during validation.
	// Cold balance persistence is scheduled via ZADD to schedule:balance-sync.

	ctxProcessTransaction, spanUpdateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanUpdateTransaction.End()

	tran, phase, err := uc.CreateOrUpdateTransaction(ctxProcessTransaction, logger, tracer, t)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanUpdateTransaction, "Failed to create or update transaction", err)

		logger.Log(ctx, libLog.LevelError, "Failed to create or update transaction", libLog.Err(err))

		return err
	}

	ctxProcessMetadata, spanCreateMetadata := tracer.Start(ctx, "command.create_balance_transaction_operations.create_metadata")
	defer spanCreateMetadata.End()

	err = uc.CreateMetadataAsync(ctxProcessMetadata, logger, tran.Metadata, tran.ID, constant.EntityTransaction)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanCreateMetadata, "Failed to create metadata on transaction", err)

		logger.Log(ctx, libLog.LevelError, "Failed to create metadata on transaction", libLog.Err(err))

		return err
	}

	ctxProcessOperation, spanCreateOperation := tracer.Start(ctx, "command.create_balance_transaction_operations.create_operation")
	defer spanCreateOperation.End()

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

				logger.Log(ctx, libLog.LevelError, "Error creating operation", libLog.Err(err))

				return err
			}
		}

		err = uc.CreateMetadataAsync(ctx, logger, oper.Metadata, oper.ID, constant.EntityOperation)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanCreateOperation, "Failed to create metadata on operation", err)

			logger.Log(ctx, libLog.LevelError, "Failed to create metadata on operation", libLog.Err(err))

			return err
		}
	}

	managedPersistence, err := uc.FinalizeDurableTransactionPersistence(ctx, data.OrganizationID, data.LedgerID, t)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed terminal transaction persistence handoff",
			libLog.String("transaction_id", tran.ID), libLog.Err(err))

		return err
	}

	// Send events asynchronously with context that preserves trace but survives parent cancellation.
	// Each emitter gets its own timeout budget so a slow earlier emitter cannot starve later ones.
	go func() {
		base := context.WithoutCancel(ctx)

		runWithTimeout := func(fn func(context.Context)) {
			emitCtx, cancel := context.WithTimeout(base, asyncOperationTimeout)
			defer cancel()

			fn(emitCtx)
		}

		var wg sync.WaitGroup

		wg.Add(4)

		go func() {
			defer wg.Done()

			runWithTimeout(func(c context.Context) { uc.SendTransactionEvents(c, tran, phase) })
		}()
		go func() { defer wg.Done(); runWithTimeout(func(c context.Context) { uc.SendOverdraftEvents(c, tran) }) }()
		go func() {
			defer wg.Done()

			runWithTimeout(func(c context.Context) { uc.SendBalanceChangedEvents(c, tran) })
		}()
		// Billing is the newest emitter and is wrapped in panic recovery here.
		// The three sibling goroutines above lack recovery as a pre-existing,
		// separately tracked follow-up — do not wrap them in this change.
		go func() {
			defer wg.Done()
			defer libRuntime.RecoverWithPolicyAndContext(base, logger, "transaction", "send-active-account-billing", libRuntime.KeepRunning)

			runWithTimeout(func(c context.Context) { uc.SendActiveAccountBillingEvents(c, tran, phase) })
		}()

		wg.Wait()
	}()

	if managedPersistence {
		// The exact backup and outcome were already removed atomically above.
	} else {
		if backupStatusForCleanup == "" {
			backupStatusForCleanup = utils.ExpectedBackupStatusForCleanup(tran.Status.Code, t.Validate)
		}

		go func() {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), asyncOperationTimeout)
			defer cancel()
			uc.RemoveTransactionFromRedisQueueIfStatus(cleanupCtx, logger, data.OrganizationID, data.LedgerID,
				tran.ID, backupStatusForCleanup, t.AttemptOwner, t.ExpectedOutcome)
		}()
	}

	uc.DeleteWriteBehindTransaction(ctx, data.OrganizationID, data.LedgerID, tran.ID)

	return nil
}

// CreateOrUpdateTransaction func that is responsible to create or update a transaction.
//
// The string return value carries the lifecycle phase the call resolved
// to, used by SendTransactionEvents to pick the corresponding
// lib-streaming event_type:
//
//   - TransactionLifecyclePhaseCreated — fresh insert via
//     TransactionRepo.Create (L193 success). Emits transaction.posted
//     when ParentTransactionID is nil, transaction.reverted otherwise.
//   - TransactionLifecyclePhaseUpdated — status transition via the
//     unique-violation idempotency branch
//     (UpdateTransactionStatus, L198 success). Emits
//     transaction.committed when Status.Code is APPROVED,
//     transaction.canceled when CANCELED.
//   - TransactionLifecyclePhaseNoop — no state change occurred (e.g.
//     unique violation with no status transition). Callers must NOT
//     emit a lifecycle event in this phase.
//
// Tracking the phase explicitly inside this function — rather than
// inferring it from CreatedAt vs UpdatedAt downstream — keeps the
// branch decision pinned to the actual code path that ran. Inference
// would be fragile because both timestamps may be touched by DB
// triggers or msgpack roundtrips before SendTransactionEvents runs.
func (uc *UseCase) CreateOrUpdateTransaction(ctx context.Context, logger libLog.Logger, tracer trace.Tracer, t transaction.TransactionProcessingPayload) (*transaction.Transaction, string, error) {
	_, spanCreateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanCreateTransaction.End()

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

					logger.Log(ctx, libLog.LevelWarn, "Failed to update transaction status",
						libLog.String("status", tran.Status.Code), libLog.String("transaction_id", tran.ID))

					return nil, TransactionLifecyclePhaseNoop, err
				}

				// Status transition succeeded via the idempotency branch.
				return tran, TransactionLifecyclePhaseUpdated, nil
			}

			// Unique violation with no eligible status transition.
			// Caller should NOT emit a lifecycle event for this path
			// (no state change observed on this attempt).
			return tran, TransactionLifecyclePhaseNoop, nil
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(spanCreateTransaction, "Failed to create transaction on repo", err)

		logger.Log(ctx, libLog.LevelError, "Failed to create transaction on repo", libLog.Err(err))

		return nil, TransactionLifecyclePhaseNoop, err
	}

	// Fresh insert succeeded.
	return tran, TransactionLifecyclePhaseCreated, nil
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
			logger.Log(ctx, libLog.LevelError, "Error creating metadata",
				libLog.String("collection", collection), libLog.Err(err))

			return err
		}
	}

	return nil
}

// CreateBTOSync func that create balance transaction operations synchronously
func (uc *UseCase) CreateBTOSync(ctx context.Context, data mmodel.Queue) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_balance_transaction_operations.create_bto_sync")
	defer span.End()

	err := uc.CreateBalanceTransactionOperationsAsync(ctx, data)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to create balance transaction operations", libLog.Err(err))

		return err
	}

	return nil
}

// RemoveTransactionFromRedisQueueIfStatus removes a backup entry only when the
// current queue payload still matches the expected transaction status.
//
// This prevents stale consumers from deleting a newer backup stage for the
// same transaction ID (e.g. late PENDING-create worker removing a newer
// APPROVED/CANCELED backup written by commit/cancel flow).
func (uc *UseCase) RemoveTransactionFromRedisQueueIfStatus(
	ctx context.Context,
	logger libLog.Logger,
	organizationID, ledgerID uuid.UUID,
	transactionID, expectedStatus, expectedOwner, expectedOutcome string,
) {
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID)

	removed, err := uc.TransactionRedisRepo.RemoveMessageFromQueueIfStatus(ctx, transactionKey,
		expectedStatus, expectedOwner, expectedOutcome, false)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Backup queue: failed conditional transaction cleanup",
			libLog.String("transaction_key", transactionKey), libLog.Err(err))

		return
	}
	if !removed {
		logger.Log(ctx, libLog.LevelDebug, "Backup queue: skip cleanup because transaction status changed",
			libLog.String("transaction_key", transactionKey),
			libLog.String("expected_status", expectedStatus))
	}
}

type TransactionBackupSeedOptions struct {
	ExecutionAttempt   *mmodel.BalanceExecutionAttempt
	RevertRolloutMode  string
	RevertRolloutToken string
}

// SendTransactionToRedisQueue func that send transaction to redis queue.
// When balances is non-nil (e.g. commit/cancel flows), the snapshot is included
// directly in the backup message so the Redis consumer can retry without relying
// on the Lua script to populate them.
func (uc *UseCase) SendTransactionToRedisQueue(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionInput mtransaction.Transaction, validate *mtransaction.Responses, transactionStatus, action string, transactionDate time.Time, balances []*mmodel.Balance, parentTransactionID *uuid.UUID, options ...TransactionBackupSeedOptions) error {
	logger, _, reqId, _ := libObservability.NewTrackingFromContext(ctx)
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

	// Scope protection: a transaction that targets any internal-scope
	// balance (e.g. auto-created overdraft reserves) MUST be rejected
	// BEFORE the transaction is published to the Redis queue. This keeps
	// system-managed balances out of the user-initiated mutation path
	// for every caller of this use case, not just the HTTP handler.
	for _, b := range balances {
		if b != nil && b.Settings != nil && b.Settings.BalanceScope == mmodel.BalanceScopeInternal {
			logger.Log(ctx, libLog.LevelWarn, "Rejected transaction targeting internal balance",
				libLog.String("event", "rejected_internal_balance_transaction"))

			return pkg.ValidateBusinessError(constant.ErrDirectOperationOnInternalBalance, constant.EntityBalance, b.Alias)
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
	if parentTransactionID != nil && *parentTransactionID != uuid.Nil {
		queue.ParentTransactionID = parentTransactionID
	}
	var executionAttempt *mmodel.BalanceExecutionAttempt
	if len(options) > 0 {
		executionAttempt = options[0].ExecutionAttempt
		queue.RevertRolloutMode = options[0].RevertRolloutMode
		queue.RevertRolloutToken = options[0].RevertRolloutToken
	}
	validRolloutMode := queue.RevertRolloutMode == "legacy" || queue.RevertRolloutMode == "bridge"
	if (queue.RevertRolloutToken == "") != (queue.RevertRolloutMode == "") ||
		(queue.RevertRolloutToken != "" && (!validRolloutMode || queue.ParentTransactionID == nil ||
			transactionInput.IsEmpty() || executionAttempt == nil || executionAttempt.Owner != transactionID.String() ||
			executionAttempt.Outcome != mmodel.TransactionOutcomeCommitted)) {
		return fmt.Errorf("rollout revert backup requires exact generation, origin, input, and committed outcome owner")
	}
	if executionAttempt != nil {
		queue.AttemptOwner = executionAttempt.Owner
		queue.ExpectedOutcome = executionAttempt.Outcome
	}

	raw, err := json.Marshal(queue)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to marshal transaction to json string", libLog.Err(err))

		return constant.ErrTransactionBackupCacheMarshalFailed
	}

	if executionAttempt != nil {
		err = uc.TransactionRedisRepo.SeedTransactionBackup(ctx, organizationID, ledgerID, transactionID, raw, *executionAttempt)
	} else {
		err = uc.TransactionRedisRepo.AddMessageToQueue(ctx, transactionKey, raw)
	}
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to send transaction to redis queue", libLog.Err(err))

		return constant.ErrTransactionBackupCacheFailed
	}

	return nil
}

// UpdateTransactionBackupOperations atomically enriches the existing Redis
// backup with the materialized operation IDs. For an economic execution
// attempt, Redis verifies the immutable Lua outcome before changing the
// envelope, so a post-movement backup is never replaced by stale HTTP state.
func (uc *UseCase) UpdateTransactionBackupOperations(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	operations []*operation.Operation,
	action string,
	attempt *mmodel.BalanceExecutionAttempt,
) ([]*operation.Operation, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction_backup_operations")
	defer span.End()

	redisOps := make([]mmodel.OperationRedis, 0, len(operations))
	for _, op := range operations {
		redisOps = append(redisOps, op.ToRedis())
	}

	canonicalRedisOps, err := uc.TransactionRedisRepo.EnrichTransactionBackup(ctx, organizationID, ledgerID, transactionID,
		redisOps, action, attempt)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to enrich transaction backup with operations", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to enrich transaction backup with operations", libLog.Err(err))

		return nil, err
	}

	canonicalOperations := make([]*operation.Operation, 0, len(canonicalRedisOps))
	for _, redisOperation := range canonicalRedisOps {
		canonicalOperations = append(canonicalOperations, operation.OperationFromRedis(redisOperation))
	}

	return canonicalOperations, nil
}

// validateOperationDirection checks the direction field of an operation.
// Empty direction is allowed with a warning (v3.5.3 messages lack this field).
// Non-empty direction must be one of the valid values ("debit", "credit").
func validateOperationDirection(ctx context.Context, logger libLog.Logger, oper *operation.Operation) error {
	if oper.Direction == "" {
		logger.Log(ctx, libLog.LevelWarn, "Operation has empty direction, may be from pre-migration message",
			libLog.String("operation_id", oper.ID))

		return nil
	}

	switch strings.ToLower(oper.Direction) {
	case "debit", "credit":
		return nil
	default:
		return fmt.Errorf("operation %s has invalid direction %q: must be 'debit' or 'credit'", oper.ID, oper.Direction)
	}
}
