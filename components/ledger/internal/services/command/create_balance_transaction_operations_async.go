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
	"github.com/vmihailenco/msgpack/v5"

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

	_, terminal, preflightErr := uc.preflightOutcomeBackedTransaction(ctx, data.OrganizationID, data.LedgerID, &t)
	if preflightErr != nil {
		return fmt.Errorf("validate current Redis economic outcome before PostgreSQL persistence: %w", preflightErr)
	}

	if terminal {
		if _, replayErr := uc.FinalizeDurableTransactionPersistence(ctx, data.OrganizationID, data.LedgerID, t); replayErr != nil {
			return replayErr
		}

		uc.DeleteWriteBehindTransaction(ctx, data.OrganizationID, data.LedgerID, t.Transaction.ID)

		return nil
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

	ctxProcessTransaction, spanUpdateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.persist_atomic")
	defer spanUpdateTransaction.End()

	tran, phase, err := uc.persistTransactionAndOperationsAtomic(ctxProcessTransaction, t)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanUpdateTransaction, "Failed to persist transaction and operations atomically", err)

		logger.Log(ctx, libLog.LevelError, "Failed to persist transaction and operations atomically", libLog.Err(err))

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

	for _, oper := range tran.Operations {
		err = uc.CreateMetadataAsync(ctx, logger, oper.Metadata, oper.ID, constant.EntityOperation)
		if err != nil {
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

	// When persistence is managed, the exact backup and outcome were already
	// removed atomically above and no queue cleanup is needed.
	if !managedPersistence {
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

func actionForTransactionPayload(payload transaction.TransactionProcessingPayload) string {
	if payload.Action != "" {
		return payload.Action
	}

	if payload.Transaction == nil {
		return ""
	}

	if payload.Transaction.ParentTransactionID != nil {
		return constant.ActionRevert
	}

	return mtransaction.StatusToAction(payload.Transaction.Status.Code)
}

// persistTransactionAndOperationsAtomic makes the terminal transaction row and
// its complete operation set visible at one PostgreSQL commit boundary. This is
// the individual-consumer equivalent of the bulk path's atomic persistence.
// Duplicate inserts are idempotent: a pending-to-terminal redelivery performs
// the status compare-and-set inside the same transaction, while an already
// terminal redelivery only replays the operation inserts before committing a
// no-op.
//
//nolint:gocognit,gocyclo // Atomic persistence covers transaction, operations, and metadata legs; refactor candidate.
func (uc *UseCase) persistTransactionAndOperationsAtomic(
	ctx context.Context,
	payload transaction.TransactionProcessingPayload,
) (_ *transaction.Transaction, phase string, retErr error) {
	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)

	tran := payload.Transaction
	if tran == nil {
		return nil, TransactionLifecyclePhaseNoop, fmt.Errorf("transaction payload is required")
	}

	tran.Body = mtransaction.Transaction{}
	switch tran.Status.Code {
	case constant.CREATED:
		description := constant.APPROVED
		tran.Status = transaction.Status{Code: description, Description: &description}
	case constant.PENDING:
		if payload.Input == nil {
			return nil, TransactionLifecyclePhaseNoop, fmt.Errorf("pending transaction input is required")
		}

		tran.Body = *payload.Input
	}

	for _, oper := range tran.Operations {
		if oper == nil {
			return nil, TransactionLifecyclePhaseNoop, fmt.Errorf("transaction operation is required")
		}

		if err := validateOperationDirection(ctx, logger, oper); err != nil {
			return nil, TransactionLifecyclePhaseNoop, err
		}
	}

	dbTx, err := uc.TransactionRepo.BeginTx(ctx)
	if err != nil {
		return nil, TransactionLifecyclePhaseNoop, fmt.Errorf("begin atomic transaction persistence: %w", err)
	}

	committed := false

	defer func() {
		if committed {
			return
		}

		if rollbackErr := dbTx.Rollback(); rollbackErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("rollback atomic transaction persistence: %w", rollbackErr))
		}
	}()

	insertResult, err := uc.TransactionRepo.CreateBulkTx(ctx, dbTx, []*transaction.Transaction{tran})
	if err != nil {
		return nil, TransactionLifecyclePhaseNoop, fmt.Errorf("persist transaction atomically: %w", err)
	}

	if insertResult == nil || insertResult.Inserted+insertResult.Ignored != 1 {
		return nil, TransactionLifecyclePhaseNoop, fmt.Errorf("persist transaction atomically: invalid insert result")
	}

	phase = TransactionLifecyclePhaseNoop
	if insertResult.Inserted == 1 {
		phase = TransactionLifecyclePhaseCreated
	}

	if len(tran.Operations) > 0 {
		operationResult, createErr := uc.OperationRepo.CreateBulkTx(ctx, dbTx, tran.Operations)
		if createErr != nil {
			return nil, TransactionLifecyclePhaseNoop, fmt.Errorf("persist transaction operations atomically: %w", createErr)
		}

		if operationResult == nil || operationResult.Inserted+operationResult.Ignored != int64(len(tran.Operations)) {
			return nil, TransactionLifecyclePhaseNoop, fmt.Errorf("persist transaction operations atomically: invalid insert result")
		}

		if phase == TransactionLifecyclePhaseCreated && operationResult.Inserted != int64(len(tran.Operations)) {
			return nil, TransactionLifecyclePhaseNoop, fmt.Errorf("persist transaction operations atomically: fresh transaction operation conflict")
		}
	}

	if insertResult.Ignored == 1 && uc.isStatusTransition(payload) {
		organizationID, ledgerID, transactionID, identityErr := terminalTransactionIdentity(tran)
		if identityErr != nil {
			return nil, TransactionLifecyclePhaseNoop, identityErr
		}

		if _, updateErr := uc.TransactionRepo.UpdateStatusFromPendingTx(ctx, dbTx,
			organizationID, ledgerID, transactionID, &transaction.Transaction{Status: tran.Status}); updateErr != nil {
			return nil, TransactionLifecyclePhaseNoop, fmt.Errorf("persist terminal transaction status atomically: %w", updateErr)
		}

		phase = TransactionLifecyclePhaseUpdated
	}

	if err := dbTx.Commit(); err != nil {
		return nil, TransactionLifecyclePhaseNoop, fmt.Errorf("commit atomic transaction persistence: %w", err)
	}

	committed = true

	return tran, phase, nil
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
	ExecutionAttempt     *mmodel.BalanceExecutionAttempt
	ExpectedEconomicPlan *mmodel.ExpectedEconomicPlan
	RevertRolloutMode    string
	RevertRolloutToken   string
	RevertLegacyFenceKey string
	RedisGeneration      string
}

// SendTransactionToRedisQueue func that send transaction to redis queue.
// When balances is non-nil (e.g. commit/cancel flows), the snapshot is included
// directly in the backup message so the Redis consumer can retry without relying
// on the Lua script to populate them.
//
//nolint:gocognit,gocyclo // Queue handoff branches per backup and rollout state; refactor candidate.
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

	effectMode := mmodel.TransactionEffectBalanceMutation
	if transactionStatus == constant.NOTED {
		effectMode = mmodel.TransactionEffectAnnotationOnly
	}

	queue := mmodel.TransactionRedisQueue{
		HeaderID:              reqId,
		OrganizationID:        organizationID,
		LedgerID:              ledgerID,
		TransactionID:         transactionID,
		TransactionInput:      transactionInput,
		Balances:              balanceRedis,
		TTL:                   time.Now(),
		Validate:              validate,
		TransactionStatus:     transactionStatus,
		Action:                action,
		EffectModeVersion:     mmodel.TransactionEffectModeVersion,
		EffectMode:            effectMode,
		OperationTypeOverride: transactionInput.OperationTypeOverride,
		TransactionDate:       transactionDate,
	}
	if parentTransactionID != nil && *parentTransactionID != uuid.Nil {
		queue.ParentTransactionID = parentTransactionID
	}

	if len(options) > 1 {
		return fmt.Errorf("at most one transaction backup seed options value is supported")
	}

	var executionAttempt *mmodel.BalanceExecutionAttempt
	if len(options) > 0 {
		executionAttempt = options[0].ExecutionAttempt
		queue.RevertRolloutMode = options[0].RevertRolloutMode
		queue.RevertRolloutToken = options[0].RevertRolloutToken
		queue.RevertLegacyFenceKey = options[0].RevertLegacyFenceKey
		queue.RedisGeneration = options[0].RedisGeneration
		queue.ExpectedEconomicPlan = options[0].ExpectedEconomicPlan
	}

	if queue.ExpectedEconomicPlan != nil {
		if err := mmodel.ValidateExpectedEconomicPlan(queue.ExpectedEconomicPlan); err != nil {
			return fmt.Errorf("validate transaction expected economic plan: %w", err)
		}
	}

	validRolloutMode := queue.RevertRolloutMode == constant.RevertRolloutModeLegacy || queue.RevertRolloutMode == constant.RevertRolloutModeBridge
	if (queue.RevertRolloutToken == "") != (queue.RevertRolloutMode == "") ||
		(queue.RevertRolloutToken != "" && (!validRolloutMode || queue.RevertLegacyFenceKey == "" || queue.ParentTransactionID == nil ||
			transactionInput.IsEmpty() || executionAttempt == nil || executionAttempt.Owner != transactionID.String() ||
			executionAttempt.Outcome != mmodel.TransactionOutcomeCommitted || queue.RedisGeneration == "")) {
		return fmt.Errorf("rollout revert backup requires exact generation, origin, input, and committed outcome owner")
	}

	if executionAttempt != nil && executionAttempt.RedisGeneration != queue.RedisGeneration {
		return fmt.Errorf("transaction backup Redis generation mismatch")
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
// attempt, Redis verifies the immutable Lua outcome and returns its complete
// balance snapshot; both the operation multiset and balances must match before
// persistence. A post-movement envelope is never replaced by stale HTTP state.
func (uc *UseCase) UpdateTransactionBackupOperations(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	operations []*operation.Operation,
	balancesAfter []mmodel.BalanceRedis,
	action string,
	attempt *mmodel.BalanceExecutionAttempt,
	expected ...mmodel.TransactionEconomicContext,
) ([]*operation.Operation, bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction_backup_operations")
	defer span.End()

	redisOps := make([]mmodel.OperationRedis, 0, len(operations))
	for _, op := range operations {
		if op == nil {
			return nil, false, fmt.Errorf("transaction economic operation is required")
		}

		redisOps = append(redisOps, op.ToRedis())
	}

	if len(expected) != 1 || expected[0].TransactionStatus == "" ||
		expected[0].TransactionAmount == "" || expected[0].TransactionAssetCode == "" {
		return nil, false, fmt.Errorf("one transaction economic context is required")
	}

	proof := expected[0]
	proof.Action = action
	ctx = mmodel.WithTransactionEconomicContext(ctx, proof)

	canonicalRedisOps, canonicalBalancesAfter, terminal, err := uc.TransactionRedisRepo.EnrichTransactionBackup(ctx, organizationID, ledgerID, transactionID,
		redisOps, action, attempt)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to enrich transaction backup with operations", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to enrich transaction backup with operations", libLog.Err(err))

		return nil, false, err
	}

	operationEffectMatches := sameRedisEconomicOperationMultiset(
		organizationID, ledgerID, transactionID, redisOps, canonicalRedisOps,
	)
	if attempt == nil && proof.TransactionStatus == constant.NOTED && !operationEffectMatches {
		return nil, false, fmt.Errorf("transaction annotation operation effect differs from its authoritative Redis envelope")
	}

	if attempt != nil && (!operationEffectMatches || !mmodel.RedisBalanceSetEconomicComplete(balancesAfter) ||
		!mmodel.RedisBalanceSetEconomicComplete(canonicalBalancesAfter) ||
		!mmodel.RedisBalanceSetEconomicEqual(balancesAfter, canonicalBalancesAfter)) {
		return nil, false, fmt.Errorf("transaction economic effect differs from its authoritative Redis envelope")
	}

	canonicalOperations := make([]*operation.Operation, 0, len(canonicalRedisOps))
	for _, redisOperation := range canonicalRedisOps {
		canonicalOperations = append(canonicalOperations, operation.OperationFromRedis(redisOperation))
	}

	return canonicalOperations, terminal, nil
}

func sameRedisEconomicOperationMultiset(
	organizationID, ledgerID, transactionID uuid.UUID,
	left, right []mmodel.OperationRedis,
) bool {
	if len(left) == 0 || len(left) != len(right) {
		return false
	}

	used := make([]bool, len(right))

	for _, candidate := range left {
		if candidate.TransactionID != transactionID.String() ||
			candidate.OrganizationID != organizationID.String() || candidate.LedgerID != ledgerID.String() ||
			!mmodel.RedisOperationEconomicComplete(candidate) {
			return false
		}

		matched := false

		for index, canonical := range right {
			if used[index] || canonical.TransactionID != transactionID.String() ||
				canonical.OrganizationID != organizationID.String() || canonical.LedgerID != ledgerID.String() ||
				!mmodel.RedisOperationEconomicComplete(canonical) ||
				!operation.RedisEconomicEffectEqual(candidate, canonical) {
				continue
			}

			used[index] = true
			matched = true

			break
		}

		if !matched {
			return false
		}
	}

	return true
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
