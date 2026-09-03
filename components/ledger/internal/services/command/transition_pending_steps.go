// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"os"
	"strings"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/skip"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// loadPendingTransaction resolves the transaction the transition acts on, reading
// the write-behind cache first and falling back to the database.
func (uc *UseCase) loadPendingTransaction(ctx context.Context, span trace.Span, in PendingTransitionInput) (*transaction.Transaction, error) {
	tran, err := uc.TransactionReader.GetWriteBehindTransaction(ctx, in.OrganizationID, in.LedgerID, in.TransactionID)
	if err != nil {
		// Load the operations with the transaction: cancel needs them to unwind an
		// overdraft hold. The write-behind cache is cleared once the create persists,
		// so this fallback carries the transaction into the transition and its
		// annotateCanceledOverdraftAmounts step, which reads tran.Operations to
		// size the overdraft deficit. A row-only read leaves Operations empty and the
		// cancel restores the full hold to available instead of only the non-overdraft
		// portion.
		tran, err = uc.TransactionReader.GetTransactionWithOperationsByID(ctx, in.OrganizationID, in.LedgerID, in.TransactionID)
		if err != nil {
			spanattr.HandleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

			return nil, err
		}

		// FindWithOperations joins on operations, so a transaction with no rows comes
		// back as an empty value with no error. Fall back to the row-only read, which
		// reports not-found for a missing transaction and returns the real row for an
		// operation-less one — either way the transition never parses an empty
		// organization id.
		if tran == nil || tran.ID == "" {
			tran, err = uc.TransactionReader.GetTransactionByID(ctx, in.OrganizationID, in.LedgerID, in.TransactionID)
			if err != nil {
				spanattr.HandleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

				return nil, err
			}
		}
	}

	return tran, nil
}

// lockPendingTransaction claims the per-transaction Redis lock that serialises
// concurrent commit/cancel attempts. It returns the unlock closure the later steps
// call on their error branches; a lock already held is a 422.
func (uc *UseCase) lockPendingTransaction(ctx context.Context, span trace.Span, logger libLog.Logger, run *pendingTransitionRun) (func(), error) {
	lockPendingTransactionKey := utils.PendingTransactionLockKey(run.organizationID, run.ledgerID, run.tran.ID)

	ttl := time.Duration(300)

	success, err := uc.TransactionRedisRepo.SetNX(ctx, lockPendingTransactionKey, "", ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set on redis", err)

		logger.Log(ctx, libLog.LevelError, "Failed to set pending transaction lock on redis", libLog.Err(err))

		return nil, err
	}

	if !success {
		err := pkg.ValidateBusinessError(constant.ErrPendingTransactionLocked, "ValidateTransactionNotPending")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction is locked", err)

		logger.Log(ctx, libLog.LevelWarn, "Transaction is locked", libLog.String("transaction_id", run.tran.ID), libLog.Err(err))

		return nil, err
	}

	deleteLockOnError := func() {
		if delErr := uc.TransactionRedisRepo.Del(ctx, lockPendingTransactionKey); delErr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete pending transaction lock", delErr)

			logger.Log(ctx, libLog.LevelError, "Failed to delete pending transaction lock key", libLog.Err(delErr))
		}
	}

	return deleteLockOnError, nil
}

// preparePendingTransition validates the persisted body, resolves the ledger
// settings and the honored per-call tracer skip, loads the balances behind the
// validated aliases, builds and enriches the balance operations and enforces the
// ledger's accounting routes. Every failure releases the lock.
func (uc *UseCase) preparePendingTransition(ctx context.Context, span trace.Span, logger libLog.Logger, run *pendingTransitionRun, unlock func()) error {
	transactionInput := run.tran.Body

	mtransaction.ApplyDefaultBalanceKeys(transactionInput.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(transactionInput.Send.Distribute.To)

	var fromTo []mtransaction.FromTo

	fromTo = append(fromTo, mtransaction.MutateConcatAliases(transactionInput.Send.Source.From)...)
	to := mtransaction.MutateConcatAliases(transactionInput.Send.Distribute.To)

	if run.status != constant.CANCELED {
		fromTo = append(fromTo, to...)
	}

	if run.tran.Status.Code != constant.PENDING {
		err := pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction is not pending", err)

		logger.Log(ctx, libLog.LevelWarn, "Transaction is not pending", libLog.String("transaction_id", run.tran.ID), libLog.Err(err))

		unlock()

		return err
	}

	// No fee seam here (P4-T13). tran.Body was persisted by the create path, which
	// already applied fees and persisted the fee legs as real operations. So
	// transactionInput == tran.Body is already fee-inclusive, and this validate runs
	// over the fee-inclusive shape. Calling applyFees on commit/cancel would charge
	// the fee a second time (double-charge). Cancel routes the held legs — including
	// fees — back via the cancel/refund path (P4-T14), not a re-charge. Do NOT call
	// applyFees.
	validate, err := mtransaction.ValidateSendSourceAndDistribute(ctx, transactionInput, run.status)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to validate send source and distribute", libLog.Err(err))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		unlock()

		return err
	}

	ledgerSettings, err := uc.TransactionReader.GetParsedLedgerSettings(ctx, run.organizationID, run.ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get ledger settings", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get ledger settings", libLog.Err(err))

		unlock()

		return err
	}

	if ledgerSettings.Accounting.ValidateRoutes {
		mtransaction.PropagateRouteValidation(ctx, validate, run.status)
	}

	// Re-resolve the per-call tracer skip from the persisted body so an honored
	// create-time skip also short-circuits the by-transaction confirm/release
	// below, instead of relocating the gRPC cost from create to this transition.
	// Authorization was already enforced at create, so a no-longer-permitted skip
	// (the opt-in was revoked between create and commit) is treated as
	// not-honored here — the error is intentionally discarded, never a 422.
	honoredTracerSkip, _ := skip.ResolveSkipFor("tracer", run.tran.Body.Skip != nil && run.tran.Body.Skip.Tracer, ledgerSettings.Overrides.AllowTracerSkip)

	action := constant.ActionCommit
	if run.status == constant.CANCELED {
		action = constant.ActionCancel
	}

	// Route ONLY the pre-write balance reads to the primary via a dedicated ctx:
	// their result seeds the authoritative balance via the NX-seed, so a stale
	// replica read here corrupts money. The mark lives on readCtx, scoped to the
	// direct balance read and the overdraft-enrichment read; the unmarked
	// ctx flows to everything else (validation, Redis seed, balance processing,
	// write) so those keep their default routing. The flag governs the effect; the
	// mark is unconditional.
	readCtx := readrouting.WithPrimaryRead(ctx)

	balances, err := uc.TransactionReader.GetBalances(readCtx, run.organizationID, run.ledgerID, validate.Aliases)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get balances", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get balances", libLog.Err(err))

		unlock()

		return err
	}

	balanceOps := BuildBalanceOperations(ctx, run.organizationID, run.ledgerID, validate, balances)
	balanceOps = AnnotateCanceledOverdraftAmounts(balanceOps, run.tran)

	// Both transitions move funds on the overdrafted balance, so both need the
	// companion mirrored: a cancel restores the held capacity, and a commit posts
	// the destination credit that repays outstanding overdraft. Enriching here is
	// what puts the companion leg in front of ValidateAccountingRules (so the
	// route's overdraft rubric is enforced), into the atomic batch (so the
	// companion balance moves in lock-step with the primary's repayment) and into
	// BuildOperations (so the overdraft leg is persisted).
	var companionFromTos []mtransaction.FromTo

	if run.status == constant.APPROVED || run.status == constant.CANCELED {
		balanceOps, companionFromTos, err = EnrichOverdraftOperations(readCtx, run.organizationID, run.ledgerID, balanceOps,
			validate, uc.TransactionReader.GetBalances)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to enrich overdraft operations", err)
			logger.Log(ctx, libLog.LevelError, "Failed to enrich overdraft operations", libLog.Err(err))

			unlock()

			return err
		}
	}

	routeCache, err := uc.TransactionReader.ValidateAccountingRules(ctx, run.organizationID, run.ledgerID, balanceOps, validate, action)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate accounting rules", err)
		logger.Log(ctx, libLog.LevelError, "Failed to validate accounting rules", libLog.Err(err))

		unlock()

		return err
	}

	run.input = transactionInput
	run.fromTo = fromTo
	run.validate = validate
	run.ledgerSettings = ledgerSettings
	run.honoredTracerSkip = honoredTracerSkip
	run.action = action
	run.balanceOps = balanceOps
	run.companionFromTos = companionFromTos
	run.routeCache = routeCache

	return nil
}

// commitPendingBalances pre-seeds the backup queue and runs the atomic balance
// mutation. A Lua failure removes the seed and releases the lock.
func (uc *UseCase) commitPendingBalances(ctx context.Context, span trace.Span, logger libLog.Logger, run *pendingTransitionRun, unlock func()) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctxBackupSeed, spanBackupSeed := tracer.Start(ctx, "handler.commit_or_cancel_transaction.pre_seed_backup")

	if backupErr := uc.SendTransactionToRedisQueue(ctxBackupSeed, run.organizationID, run.ledgerID, run.tran.IDtoUUID(), run.input, run.validate, run.status, run.action, time.Now(), nil); backupErr != nil {
		libOpentelemetry.HandleSpanError(spanBackupSeed, "Failed to pre-seed transaction backup cache", backupErr)

		logger.Log(ctx, libLog.LevelError, "Failed to pre-seed commit/cancel transaction backup cache", libLog.Err(backupErr))

		spanBackupSeed.End()

		unlock()

		return pkg.ValidateBusinessError(backupErr, constant.EntityTransaction)
	}

	spanBackupSeed.End()

	result, err := uc.ProcessBalanceOperations(ctx, ProcessBalanceOperationsInput{
		OrganizationID:    run.organizationID,
		LedgerID:          run.ledgerID,
		TransactionID:     run.tran.IDtoUUID(),
		TransactionInput:  nil, // State transitions skip balance-rule re-validation
		Validate:          run.validate,
		BalanceOperations: run.balanceOps,
		TransactionStatus: run.status,
	})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to process balance operations", err)
		logger.Log(ctx, libLog.LevelError, "Failed to process balance operations", libLog.Err(err))

		uc.RemoveTransactionFromRedisQueue(ctx, logger, run.organizationID, run.ledgerID, run.tran.IDtoUUID().String())

		unlock()

		return err
	}

	run.result = result

	return nil
}

// finalizePendingTransition turns the committed balance result into the updated
// transaction row: it splices the split-alias legs and the overdraft companions
// into fromTo, builds the operation records, refreshes the backup entry and writes
// the transaction.
func (uc *UseCase) finalizePendingTransition(ctx context.Context, span trace.Span, logger libLog.Logger, run *pendingTransitionRun, unlock func()) (*transaction.Transaction, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	balancesBefore, balancesAfter := run.result.Before, run.result.After

	run.fromTo = append(run.fromTo, mtransaction.MutateSplitAliases(run.input.Send.Source.From)...)
	to := mtransaction.MutateSplitAliases(run.input.Send.Distribute.To)

	if run.status != constant.CANCELED {
		run.fromTo = append(run.fromTo, to...)
	}

	run.fromTo = append(run.fromTo, run.companionFromTos...)

	run.tran.UpdatedAt = time.Now()
	run.tran.Status = transaction.Status{
		Code:        run.status,
		Description: &run.status,
	}

	operations, preBalances, err := uc.BuildOperations(ctx, balancesBefore, balancesAfter, run.fromTo, run.input, *run.tran, run.validate, time.Now(), false, run.ledgerSettings.Accounting.ValidateRoutes, run.routeCache, run.action)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build operations", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build operations", libLog.Err(err))

		unlock()

		return nil, err
	}

	run.tran.Source = GetAliasWithoutKey(FilterCompanionAliases(run.validate.Sources))
	run.tran.Destination = GetAliasWithoutKey(FilterCompanionAliases(run.validate.Destinations))
	run.tran.Operations = operations

	ctxBackup, spanBackup := tracer.Start(ctx, "handler.commit_or_cancel_transaction.send_to_redis_queue")

	if backupErr := uc.SendTransactionToRedisQueue(ctxBackup, run.organizationID, run.ledgerID, run.tran.IDtoUUID(), run.input, run.validate, run.status, run.action, time.Now(), preBalances); backupErr != nil {
		libOpentelemetry.HandleSpanError(spanBackup, "Failed to send transaction to backup cache", backupErr)

		logger.Log(ctx, libLog.LevelWarn, "Failed to send commit/cancel transaction to backup cache", libLog.Err(backupErr))
	}

	spanBackup.End()

	// Materialize operation IDs in the backup entry to prevent duplicate
	// operations if the Redis backup consumer replays this transaction.
	// Without this, BuildOperations() generates new UUIDs on replay.
	uc.UpdateTransactionBackupOperations(ctx, run.organizationID, run.ledgerID, run.tran.IDtoUUID().String(), operations, run.action)

	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		_, err = uc.UpdateTransactionStatus(ctx, run.tran)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction status synchronously", err)

			logger.Log(ctx, libLog.LevelError, "Failed to update transaction status synchronously", libLog.String("transaction_id", run.tran.ID), libLog.Err(err))
		}
	}

	// Past this point the lock is not released: the balance has already moved, so a
	// failure below is reconciled by the backup queue, not by retrying the transition.
	err = uc.WriteTransaction(ctx, run.organizationID, run.ledgerID, &run.input, run.validate, preBalances, balancesAfter, run.tran)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, "failed to update BTO")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "failed to update BTO", err)

		logger.Log(ctx, libLog.LevelError, "Failed to update BTO", libLog.String("transaction_id", run.tran.ID), libLog.Err(err))

		return nil, err
	}

	tenantCtx := tmcore.ContextWithTenantID(context.Background(), tmcore.GetTenantIDContext(ctx))

	go uc.SendLogTransactionAuditQueue(tenantCtx, operations, run.organizationID, run.ledgerID, run.tran.IDtoUUID())

	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		go uc.UpdateWriteBehindTransaction(tenantCtx, run.organizationID, run.ledgerID, run.tran)
	}

	return run.tran, nil
}
