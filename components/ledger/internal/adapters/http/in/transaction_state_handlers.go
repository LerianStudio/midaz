// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/revertclaim"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	transactionredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
	"github.com/LerianStudio/midaz/v4/pkg/skip"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// revertIdempotencyReplayedLogMessage is the Warn message the revert core records when the
// idempotency slot answers with a cached reverse instead of a new one.
const revertIdempotencyReplayedLogMessage = "Revert replayed a cached reverse transaction"

func newPodRequestToken() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("resolve rollout request owner: %w", err)
	}
	if hostname == "" {
		return "", fmt.Errorf("resolve rollout request owner: empty hostname")
	}

	return hostname + ":" + uuid.NewString(), nil
}

func (handler *TransactionHandler) acquireTransactionMutationLock(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	ttl time.Duration,
) (func() error, error) {
	if handler.Command == nil || handler.Command.TransactionRedisRepo == nil {
		if strings.TrimSpace(handler.RevertIdempotencyMode) == "" && handler.RevertUpdateFreeze == nil {
			return func() error { return nil }, nil
		}

		return nil, fmt.Errorf("transaction mutation lock repository not configured")
	}

	owner, err := newPodRequestToken()
	if err != nil {
		return nil, err
	}
	key := utils.PendingTransactionLockKey(organizationID, ledgerID, transactionID.String())
	acquired, err := handler.Command.TransactionRedisRepo.AcquireOwnedKey(ctx, key, owner, ttl)
	if err != nil {
		return nil, fmt.Errorf("acquire transaction mutation lock: %w", err)
	}
	if !acquired {
		return nil, pkg.ValidateBusinessError(constant.ErrPendingTransactionLocked, "ValidateTransactionNotPending")
	}

	return func() error {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		released, err := handler.Command.TransactionRedisRepo.ReleaseOwnedKey(releaseCtx, key, owner)
		if err != nil {
			return fmt.Errorf("release transaction mutation lock: %w", err)
		}
		if !released {
			return fmt.Errorf("release transaction mutation lock: ownership lost")
		}

		return nil
	}, nil
}

// NOT MOUNTED — the Fiber state wrappers below (CommitTransaction, CancelTransaction,
// RevertTransaction, UpdateTransaction) have no production registration: every v1 and v2
// commit/cancel/revert/update route terminates at the Huma shell, so covering a wrapper
// proves nothing about production.

// CommitTransaction method that commit transaction created before
func (handler *TransactionHandler) CommitTransaction(c fiber.Ctx) error {
	ctx := c.Context()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	transactionID, err := http.GetUUIDFromLocals(c, "transaction_id")
	if err != nil {
		return http.WithError(c, err)
	}

	tran, err := handler.commitTransaction(ctx, organizationID, ledgerID, transactionID, constant.APPROVED)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.Created(c, tran)
}

// commitTransaction is the transport-neutral commit/cancel core: it opens the same
// per-action span (commit_transaction / cancel_transaction, derived from the target
// status so the span names stay byte-identical to the pre-migration Fiber path),
// fetches the transaction (write-behind cache first, DB fallback), then delegates to the
// untouched commitOrCancelTransaction state machine.
func (handler *TransactionHandler) commitTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionStatus string) (
	result *transaction.Transaction,
	retErr error,
) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	spanName := "handler.commit_transaction"
	if transactionStatus == constant.CANCELED {
		spanName = "handler.cancel_transaction"
	}

	_, span := tracer.Start(ctx, spanName)
	defer span.End()

	releaseTransactionLock, err := handler.acquireTransactionMutationLock(ctx, organizationID, ledgerID, transactionID, 300)
	if err != nil {
		return nil, err
	}
	keepSuccessfulLock := false
	defer func() {
		if keepSuccessfulLock {
			return
		}
		if err := releaseTransactionLock(); err != nil {
			retErr = errors.Join(retErr, err)
			result = nil
		}
	}()

	primaryCtx := readrouting.WithPrimaryRead(ctx)
	tran, err := handler.Query.GetWriteBehindTransaction(primaryCtx, organizationID, ledgerID, transactionID)
	if err != nil {
		// Load the operations with the transaction: cancel needs them to unwind an
		// overdraft hold. The write-behind cache is cleared once the create persists,
		// so this fallback carries the transaction into commitOrCancelTransaction and
		// its annotateCanceledOverdraftAmounts step, which reads tran.Operations to
		// size the overdraft deficit. A row-only read leaves Operations empty and the
		// cancel restores the full hold to available instead of only the non-overdraft
		// portion.
		tran, err = handler.Query.GetTransactionWithOperationsByID(primaryCtx, organizationID, ledgerID, transactionID)
		if err != nil {
			handleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

			return nil, err
		}

		// FindWithOperations joins on operations, so a transaction with no rows comes
		// back as an empty value with no error. Fall back to the row-only read, which
		// reports not-found for a missing transaction and returns the real row for an
		// operation-less one — either way commitOrCancelTransaction never parses an
		// empty organization id.
		if tran == nil || tran.ID == "" {
			tran, err = handler.Query.GetTransactionByID(primaryCtx, organizationID, ledgerID, transactionID)
			if err != nil {
				handleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

				return nil, err
			}
		}
	}
	if tran.Status.Code != constant.PENDING {
		return nil, pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")
	}

	stateRepo := handler.Command.TransactionRepo
	if stateRepo == nil && handler.Query != nil {
		stateRepo = handler.Query.TransactionRepo
	}
	if stateRepo == nil {
		return nil, fmt.Errorf("transaction state repository not configured")
	}

	dbTx, err := stateRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin serialized transaction transition: %w", err)
	}
	defer func() { _ = dbTx.Rollback() }()

	locked, err := stateRepo.FindForUpdate(ctx, dbTx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}
	if locked.Status.Code != constant.PENDING {
		return nil, pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")
	}
	tran.Description = locked.Description
	tran.Body = locked.Body
	tran.Status = locked.Status

	result, retErr = handler.commitOrCancelTransaction(ctx, tran, transactionStatus, dbTx, func() {
		// Once Redis has moved balances, a later failure is ambiguous until the
		// durable backup/status handoff is reconciled. Retain the processing fence
		// instead of admitting an opposite transition immediately.
		keepSuccessfulLock = true
	})
	if retErr == nil {
		keepSuccessfulLock = false
	} else {
		var terminalConflict pkg.EntityConflictError
		if errors.As(retErr, &terminalConflict) &&
			terminalConflict.Code == constant.ErrCommitTransactionNotPending.Error() {
			// An opposite immutable outcome is not ambiguous: this call persisted
			// the winning terminal status before returning the conflict.
			keepSuccessfulLock = false
		}
	}

	return result, retErr
}

// CancelTransaction method that cancel pre transaction created before
func (handler *TransactionHandler) CancelTransaction(c fiber.Ctx) error {
	ctx := c.Context()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	transactionID, err := http.GetUUIDFromLocals(c, "transaction_id")
	if err != nil {
		return http.WithError(c, err)
	}

	tran, err := handler.commitTransaction(ctx, organizationID, ledgerID, transactionID, constant.CANCELED)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.Created(c, tran)
}

// RevertTransaction method that revert transaction created before. Unlike the live Huma
// terminal it drops the core's `replayed` flag instead of setting X-Idempotency-Replayed.
func (handler *TransactionHandler) RevertTransaction(c fiber.Ctx) error {
	ctx := c.Context()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	transactionID, err := http.GetUUIDFromLocals(c, "transaction_id")
	if err != nil {
		return http.WithError(c, err)
	}

	tran, _, err := handler.revertTransaction(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.Created(c, tran)
}

// revertTransaction fences every reversal by (organization, ledger, origin)
// in PostgreSQL primary before balance mutation. The reserved reverse ID is
// reused by Redis backup recovery, so a lost response cannot mint a second
// economic mutation. Bridge mode also participates in the released payload-
// hash Redis barrier; final mode only reads that legacy fence during rollout.
func (handler *TransactionHandler) revertTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (
	result *transaction.Transaction,
	replayed bool,
	retErr error,
) {
	ctx = readrouting.WithPrimaryRead(ctx)
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.revert_transaction")
	defer span.End()

	rolloutPhase, releaseRolloutLease, err := handler.acquireRevertRolloutRequest(ctx)
	if err != nil {
		return nil, false, err
	}
	if releaseRolloutLease != nil {
		defer func() {
			if err := releaseRolloutLease(); err != nil {
				retErr = errors.Join(retErr, err)
				if result != nil {
					result = nil
					replayed = false
				}
			}
		}()
	}
	durablePhaseZero := handler.activeRevertIdempotencyMode() == revertIdempotencyModeLegacy &&
		rolloutPhase == transactionredis.RevertUpdateFreezeActive

	finish := func(tran *transaction.Transaction, replayed bool, err error) (*transaction.Transaction, bool, error) {
		if replayed && err == nil {
			span.SetAttributes(attribute.Bool("app.response.idempotency_replayed", true))
			logger.Log(ctx, libLog.LevelWarn, revertIdempotencyReplayedLogMessage, libLog.String("transaction_id", transactionID.String()))
		}

		return tran, replayed, err
	}

	parent, err := handler.Query.GetParentByTransactionID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Parent Transaction on query", err)

		return nil, false, err
	}

	if parent != nil {
		if handler.activeRevertIdempotencyMode() == revertIdempotencyModeLegacy && !durablePhaseZero {
			return nil, false, pkg.ValidateBusinessError(constant.ErrTransactionIDHasAlreadyParentTransaction, "RevertTransaction")
		}
		if handler.Command == nil || handler.Command.RevertClaimRepo == nil {
			return nil, false, pkg.ValidateBusinessError(constant.ErrTransactionIDHasAlreadyParentTransaction, "RevertTransaction")
		}

		var adoptionLegacyKey *string
		if handler.activeRevertIdempotencyMode() == revertIdempotencyModeBridge || durablePhaseZero {
			legacyKey, keyErr := handler.legacyRevertBarrierKeyForOrigin(ctx, organizationID, ledgerID, transactionID)
			if keyErr != nil {
				return finish(nil, false, keyErr)
			}
			adoptionLegacyKey = &legacyKey
		}
		persisted, replayed, adoptErr := handler.adoptPersistedReverse(ctx, organizationID, ledgerID, transactionID,
			parent, adoptionLegacyKey)
		if adoptErr != nil {
			return finish(nil, false, adoptErr)
		}

		return finish(persisted, replayed, nil)
	}

	tran, err := handler.Query.GetTransactionWithOperationsByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

		return nil, false, err
	}

	if tran.ParentTransactionID != nil {
		err = pkg.ValidateBusinessError(constant.ErrTransactionIDIsAlreadyARevert, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction Has Already Parent Transaction", err)

		return nil, false, err
	}

	if tran.Status.Code != constant.APPROVED {
		err = pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction CantRevert Transaction", err)

		return nil, false, err
	}

	transactionReverted := tran.TransactionRevert()
	if transactionReverted.IsEmpty() {
		err = pkg.ValidateBusinessError(constant.ErrTransactionCantRevert, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction can't be reverted", err)

		return nil, false, err
	}

	// Validate bidirectional routes: operations with a route_id require
	// the referenced OperationRoute to have OperationType "bidirectional".
	for _, op := range tran.Operations {
		if op.RouteID == nil || *op.RouteID == "" {
			continue
		}

		routeUUID, parseErr := uuid.Parse(*op.RouteID)
		if parseErr != nil {
			parseValidationErr := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "RevertTransaction", "routeId")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid routeId format on operation during revert validation", parseValidationErr)

			return nil, false, parseValidationErr
		}

		operationRoute, routeErr := handler.Query.GetOperationRouteByID(ctx, organizationID, ledgerID, nil, routeUUID)
		if routeErr != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to retrieve operation route for revert validation", routeErr)

			return nil, false, routeErr
		}

		if operationRoute != nil && operationRoute.OperationType != "bidirectional" {
			err = pkg.ValidateBusinessError(constant.ErrRouteNotBidirectional, "RevertTransaction")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Operation route is not bidirectional", err)

			return nil, false, err
		}
	}

	if handler.activeRevertIdempotencyMode() == revertIdempotencyModeLegacy && !durablePhaseZero {
		params := &transactionPathParams{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			TransactionID:  transactionID,
		}

		return handler.createRevertTransaction(ctx, params, transactionReverted, constant.CREATED, "", http.ParseIdempotencyTTL(""))
	}

	legacyHash, err := legacyRevertIdempotencyHash(transactionReverted)
	if err != nil {
		return nil, false, err
	}

	legacyCached, legacyKey, err := handler.readLegacyRevert(ctx, organizationID, ledgerID, legacyHash)
	if err != nil {
		return nil, false, err
	}
	if legacyCached != nil {
		if reverseBelongsToOrigin(legacyCached, transactionID) {
			cachedID, parseErr := uuid.Parse(legacyCached.ID)
			if parseErr != nil {
				return nil, false, pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
			}

			var cachedLegacyKey *string
			if handler.activeRevertIdempotencyMode() == revertIdempotencyModeBridge || durablePhaseZero {
				cachedLegacyKey = &legacyKey
			}
			claim, acquired, claimErr := handler.Command.ClaimRevert(ctx, organizationID, ledgerID, transactionID,
				cachedID, cachedLegacyKey, nil)
			if claimErr != nil {
				return nil, false, claimErr
			}
			if claim.ReverseTransactionID != cachedID {
				return nil, false, pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
			}
			if acquired {
				reason := "legacy_revert_cached_before_primary_persistence"
				_ = handler.Command.MarkRevertClaim(ctx, organizationID, ledgerID, transactionID, cachedID, revertclaim.StateReconciliationRequired, &reason)
			}

			return finish(handler.resolveDurableRevertClaim(ctx, claim))
		}

		if handler.activeRevertIdempotencyMode() == revertIdempotencyModeBridge || durablePhaseZero {
			return nil, false, pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "RevertTransaction", transactionID.String())
		}
	}

	reverseID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		return nil, false, err
	}

	executionAttempt := &mmodel.BalanceExecutionAttempt{
		ExecutionKey: utils.TransactionBalanceExecutionKey(organizationID, ledgerID, reverseID),
		OutcomeKey:   utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, reverseID),
		Owner:        reverseID.String(),
		Outcome:      mmodel.TransactionOutcomeCommitted,
		Identity:     reverseID,
	}
	executionLeaseAcquired, executionLeaseErr := handler.Command.TransactionRedisRepo.AcquireOwnedKey(ctx,
		executionAttempt.ExecutionKey, executionAttempt.Owner, revertExecutionLeaseTTL)
	if executionLeaseErr != nil {
		return nil, false, fmt.Errorf("reserve revert balance execution attempt: %w", executionLeaseErr)
	}
	if !executionLeaseAcquired {
		return nil, false, pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "RevertTransaction", reverseID.String())
	}
	releaseUnclaimedAttempt := func() error {
		released, releaseErr := handler.Command.TransactionRedisRepo.ReleaseOwnedKey(ctx,
			executionAttempt.ExecutionKey, executionAttempt.Owner)
		if releaseErr != nil {
			return fmt.Errorf("release unclaimed revert balance execution attempt: %w", releaseErr)
		}
		if released {
			return nil
		}
		values, readErr := handler.Command.TransactionRedisRepo.MGet(ctx,
			[]string{executionAttempt.ExecutionKey, executionAttempt.ExecutionKey + ":owner"})
		if readErr != nil {
			return fmt.Errorf("verify unclaimed revert balance execution attempt release: %w", readErr)
		}
		if len(values) != 0 {
			return fmt.Errorf("release unclaimed revert balance execution attempt: ownership lost")
		}

		return nil
	}

	var claimedLegacyKey, claimedLegacyOwner *string
	if handler.activeRevertIdempotencyMode() == revertIdempotencyModeBridge || durablePhaseZero {
		claimedLegacyKey = &legacyKey
		owner := reverseID.String()
		claimedLegacyOwner = &owner
	}
	claim, acquired, err := handler.Command.ClaimRevert(ctx, organizationID, ledgerID, transactionID, reverseID,
		claimedLegacyKey, claimedLegacyOwner)
	if err != nil {
		return nil, false, errors.Join(err, releaseUnclaimedAttempt())
	}
	if claim == nil {
		return nil, false, errors.Join(fmt.Errorf("revert claim repository returned no claim"),
			releaseUnclaimedAttempt())
	}
	if !acquired {
		if releaseErr := releaseUnclaimedAttempt(); releaseErr != nil {
			return nil, false, releaseErr
		}
		if claim.State == revertclaim.StateClaimed || claim.State == revertclaim.StateRecovering {
			// Final pods can recover claims left by bridge pods. Recovery uses only
			// the exact legacy key persisted in the claim at acquisition time; it
			// never recalculates a key from an origin whose payload may have changed.
			// Final mode ignores a fence owned by a different reverse because legacy
			// collisions are no longer an admission barrier after bridge has drained.
			recovered, recoverErr := handler.recoverProvenPreMovementRevert(ctx, claim)
			if recoverErr != nil {
				return finish(nil, false, recoverErr)
			}
			if recovered {
				// Re-enter through the complete eligibility/claim path after the
				// recovery owner released the PostgreSQL claim last.
				return handler.revertTransaction(ctx, organizationID, ledgerID, transactionID)
			}
		}

		persisted, replayed, resolveErr := handler.resolveDurableRevertClaim(ctx, claim)
		if resolveErr != nil {
			return finish(nil, false, resolveErr)
		}

		return finish(persisted, replayed, nil)
	}

	// From this point the local string tracks only a legacy fence owned by this
	// request. The immutable key remains in the durable claim for every retry.
	legacyKey = ""
	if handler.activeRevertIdempotencyMode() == revertIdempotencyModeBridge || durablePhaseZero {
		var legacyReplay *transaction.Transaction
		legacyKey, legacyReplay, err = handler.acquireLegacyRevertBarrier(ctx, claim)
		if err != nil {
			if releaseErr := releaseUnclaimedAttempt(); releaseErr != nil {
				return nil, false, handler.requireRevertReconciliation(ctx, claim,
					"legacy_fence_acquire_attempt_cleanup_failed")
			}
			if releaseErr := handler.releaseFreshRevertClaim(ctx, claim, legacyKey, true); releaseErr != nil {
				return nil, false, handler.requireRevertReconciliation(ctx, claim, "legacy_fence_acquire_cleanup_failed")
			}

			return nil, false, err
		}
		if legacyReplay != nil {
			if releaseErr := releaseUnclaimedAttempt(); releaseErr != nil {
				return nil, false, handler.requireRevertReconciliation(ctx, claim,
					"legacy_replay_attempt_cleanup_failed")
			}
			released, releaseErr := handler.Command.ReleaseRevertClaim(ctx, organizationID, ledgerID,
				transactionID, claim.ReverseTransactionID)
			if releaseErr != nil || !released {
				return nil, false, handler.requireRevertReconciliation(ctx, claim,
					"legacy_replay_claim_cleanup_failed")
			}

			return nil, false, pkg.ValidateBusinessError(constant.ErrIdempotencyKey,
				"RevertTransaction", transactionID.String())
		}
	}

	execution := &revertExecutionState{}
	params := &transactionPathParams{
		OrganizationID:        organizationID,
		LedgerID:              ledgerID,
		TransactionID:         transactionID,
		ReservedTransactionID: claim.ReverseTransactionID,
		RevertExecution:       execution,
		ExecutionAttempt:      executionAttempt,
	}

	tranReverted, replayed, err := handler.createRevertTransaction(ctx, params, transactionReverted, constant.CREATED, "", http.ParseIdempotencyTTL(""), utils.RevertIdempotencyHashSource(transactionID))
	if err != nil {
		return nil, false, handler.failRevertClaim(ctx, claim, execution, legacyKey, err)
	}
	if replayed || !reverseMatchesClaim(tranReverted, claim) {
		// The durable claim is the authority. A fresh claimant can only find an
		// origin Redis replay when Redis and PostgreSQL disagree; returning that
		// cache value could expose another reverse or hide an unpersisted
		// movement. Preserve every barrier and reconcile from the atomic backup.
		return nil, false, handler.requireRevertReconciliation(ctx, claim, "origin_redis_replay_without_primary_child")
	}

	return finish(tranReverted, replayed, nil)
}

// UpdateTransaction method that patch transaction created before
func (handler *TransactionHandler) UpdateTransaction(p any, c fiber.Ctx) error {
	ctx := c.Context()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	transactionID, err := http.GetUUIDFromLocals(c, "transaction_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload := p.(*transaction.UpdateTransactionInput)

	trans, err := handler.updateTransaction(ctx, organizationID, ledgerID, transactionID, payload)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, trans)
}

// updateTransaction is the transport-neutral update core. Its PostgreSQL row
// lock covers the status decision and writes so PENDING cannot become APPROVED
// inside the update's decision.
func (handler *TransactionHandler) updateTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, payload *transaction.UpdateTransactionInput) (
	result *transaction.Transaction,
	retErr error,
) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_transaction")
	defer span.End()

	logSafePayload(ctx, logger, "Request to update a transaction", payload)

	recordSafePayloadAttributes(span, payload)

	_, err := handler.Command.UpdateTransactionSerialized(ctx, organizationID, ledgerID, transactionID, payload,
		handler.acquireApprovedUpdateRolloutRequest)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to update transaction on command", err)

		return nil, err
	}

	result, err = handler.Query.GetTransactionByID(readrouting.WithPrimaryRead(ctx), organizationID, ledgerID, transactionID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

		return nil, err
	}

	return result, nil
}

func (handler *TransactionHandler) acquireApprovedUpdateRolloutRequest(ctx context.Context, status string) (func() error, error) {
	if status != constant.APPROVED {
		return nil, nil
	}

	mode := handler.activeRevertIdempotencyMode()
	if handler.RevertUpdateFreeze == nil {
		if mode == revertIdempotencyModeBridge || mode == revertIdempotencyModeFinal {
			return nil, pkg.ValidateBusinessError(constant.ErrRevertRolloutFreezeRequired, constant.EntityTransaction)
		}

		return nil, nil
	}

	token, err := newPodRequestToken()
	if err != nil {
		return nil, err
	}
	admitted, frozen, leaseHeld, err := handler.RevertUpdateFreeze.AcquireApprovedUpdate(ctx, mode, token)
	if err != nil {
		acquireErr := fmt.Errorf("acquire revert rollout approved-update lease: %w", err)

		return nil, releaseAmbiguousRolloutAdmission(ctx, acquireErr, func(releaseCtx context.Context) error {
			return handler.RevertUpdateFreeze.ReleaseApprovedUpdate(releaseCtx, token)
		})
	}
	if frozen {
		return nil, pkg.ValidateBusinessError(constant.ErrActionNotPermitted, "TransactionUpdateDuringRevertRollout")
	}
	if !admitted {
		return nil, pkg.ValidateBusinessError(constant.ErrRevertRolloutFreezeRequired, constant.EntityTransaction)
	}
	if !leaseHeld {
		return nil, nil
	}

	return func() error {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		if err := handler.RevertUpdateFreeze.ReleaseApprovedUpdate(releaseCtx, token); err != nil {
			return fmt.Errorf("release revert rollout approved-update lease: %w", err)
		}

		return nil
	}, nil
}

func (handler *TransactionHandler) enforceRevertUpdateFreeze(ctx context.Context, status string) error {
	if status != constant.APPROVED {
		return nil
	}

	mode := handler.activeRevertIdempotencyMode()
	if handler.RevertUpdateFreeze == nil {
		if mode == revertIdempotencyModeBridge || mode == revertIdempotencyModeFinal {
			return pkg.ValidateBusinessError(constant.ErrRevertRolloutFreezeRequired, constant.EntityTransaction)
		}

		return nil
	}

	frozen, ready, err := handler.RevertUpdateFreeze.ApprovedUpdatePolicy(ctx, mode)
	if err != nil {
		return fmt.Errorf("read revert rollout update policy: %w", err)
	}
	if frozen {
		return pkg.ValidateBusinessError(constant.ErrActionNotPermitted, "TransactionUpdateDuringRevertRollout")
	}
	if !ready {
		return pkg.ValidateBusinessError(constant.ErrRevertRolloutFreezeRequired, constant.EntityTransaction)
	}

	return nil
}

// commitOrCancelTransaction is the transport-neutral state-transition core. The
// caller holds the PostgreSQL row lock across its Redis movement and terminal
// CAS, and both Fiber wrappers and Huma shells use this same state machine.
//
//nolint:gocyclo // State machine with branches per status × action combination; refactor candidate.
func (handler *TransactionHandler) commitOrCancelTransaction(
	ctx context.Context,
	tran *transaction.Transaction,
	transactionStatus string,
	dbTx repository.DBTransaction,
	markMovementApplied func(),
) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.commit_or_cancel_transaction")
	defer span.End()

	organizationID := uuid.MustParse(tran.OrganizationID)
	ledgerID := uuid.MustParse(tran.LedgerID)

	transactionInput := tran.Body

	mtransaction.ApplyDefaultBalanceKeys(transactionInput.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(transactionInput.Send.Distribute.To)

	var fromTo []mtransaction.FromTo

	fromTo = append(fromTo, mtransaction.MutateConcatAliases(transactionInput.Send.Source.From)...)
	to := mtransaction.MutateConcatAliases(transactionInput.Send.Distribute.To)

	if transactionStatus != constant.CANCELED {
		fromTo = append(fromTo, to...)
	}

	if tran.Status.Code != constant.PENDING {
		err := pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction is not pending", err)

		logger.Log(ctx, libLog.LevelWarn, "Transaction is not pending", libLog.String("transaction_id", tran.ID), libLog.Err(err))

		return nil, err
	}

	// No fee seam here (P4-T13). tran.Body was persisted by the create path
	// (executeCreateTransaction), which already applied fees and persisted the
	// fee legs as real operations. So transactionInput == tran.Body is already
	// fee-inclusive, and this validate runs over the fee-inclusive shape.
	// Calling applyFees on commit/cancel would charge the fee a second time
	// (double-charge). Cancel routes the held legs — including fees — back via
	// the cancel/refund path (P4-T14), not a re-charge. Do NOT call applyFees.
	validate, err := mtransaction.ValidateSendSourceAndDistribute(ctx, transactionInput, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to validate send source and distribute", libLog.Err(err))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		return nil, err
	}
	ledgerSettings, err := handler.Query.GetParsedLedgerSettings(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get ledger settings", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get ledger settings", libLog.Err(err))

		return nil, err
	}

	if ledgerSettings.Accounting.ValidateRoutes {
		mtransaction.PropagateRouteValidation(ctx, validate, transactionStatus)
	}

	expectedOutcome := economicOutcomeForStatus(transactionStatus)
	executionKey := utils.TransactionBalanceExecutionKey(organizationID, ledgerID, tran.IDtoUUID())
	outcomeKey := utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, tran.IDtoUUID())
	queuedAttempt, err := handler.readTransactionLifecycleOutcome(ctx, organizationID, ledgerID, tran.IDtoUUID())
	if err != nil {
		return nil, transactionLifecycleReconciliationError()
	}
	persistedOutcome, err := handler.readBalanceExecutionOutcome(ctx, organizationID, ledgerID, tran.IDtoUUID())
	if err != nil {
		return nil, transactionLifecycleReconciliationError()
	}

	var result *mmodel.BalanceAtomicResult
	var executionAttempt *mmodel.BalanceExecutionAttempt
	if persistedOutcome != nil {
		if queuedAttempt == nil || queuedAttempt.AttemptOwner != persistedOutcome.Owner ||
			queuedAttempt.ExpectedOutcome != persistedOutcome.Outcome {
			if markMovementApplied != nil {
				markMovementApplied()
			}

			return nil, transactionLifecycleReconciliationError()
		}

		if persistedOutcome.Outcome != expectedOutcome {
			if markMovementApplied != nil {
				markMovementApplied()
			}

			persistedStatus := constant.APPROVED
			if persistedOutcome.Outcome == mmodel.TransactionOutcomeAborted {
				persistedStatus = constant.CANCELED
			}
			tran.Status = transaction.Status{Code: persistedStatus, Description: &persistedStatus}
			if _, err := handler.Command.UpdateTransactionStatusTx(ctx, dbTx, tran); err != nil {
				return nil, err
			}
			if err := dbTx.Commit(); err != nil {
				return nil, fmt.Errorf("commit recovered transaction outcome: %w", err)
			}

			return nil, pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")
		}

		result = balanceAtomicResultFromOutcome(persistedOutcome, organizationID, ledgerID)
		if result == nil {
			if markMovementApplied != nil {
				markMovementApplied()
			}

			return nil, transactionLifecycleReconciliationError()
		}
		executionAttempt = &mmodel.BalanceExecutionAttempt{
			ExecutionKey: executionKey,
			OutcomeKey:   outcomeKey,
			Owner:        persistedOutcome.Owner,
			Outcome:      persistedOutcome.Outcome,
			Identity:     tran.IDtoUUID(),
		}
	} else if queuedAttempt != nil {
		// New fenced seeds carry the exact owner and desired economic outcome.
		// A legacy/unowned seed is never inferred safe from missing Redis data.
		if queuedAttempt.AttemptOwner == "" || queuedAttempt.ExpectedOutcome == "" {
			if markMovementApplied != nil {
				markMovementApplied()
			}

			return nil, transactionLifecycleReconciliationError()
		}

		attemptValues, readErr := handler.Command.TransactionRedisRepo.MGet(ctx,
			[]string{executionKey, executionKey + ":owner"})
		if readErr != nil || len(attemptValues) != 0 {
			if markMovementApplied != nil {
				markMovementApplied()
			}

			return nil, transactionLifecycleReconciliationError()
		}

		// The exact owned execution attempt and immutable outcome are absent.
		// Any delayed Lua now fails its owner check before movement, so this is a
		// proven pre-movement attempt and its seed may be replaced. Elapsed time
		// alone is never release evidence.
		removed, cleanupErr := handler.Command.TransactionRedisRepo.RemoveMessageFromQueueIfStatus(ctx,
			utils.TransactionInternalKey(organizationID, ledgerID, tran.ID), queuedAttempt.TransactionStatus,
			queuedAttempt.AttemptOwner, queuedAttempt.ExpectedOutcome, true)
		if cleanupErr != nil || !removed {
			return nil, transactionLifecycleReconciliationError()
		}
		queuedAttempt = nil
	}

	// Re-resolve the per-call tracer skip from the persisted body so an honored
	// create-time skip also short-circuits the by-transaction confirm/release
	// below, instead of relocating the gRPC cost from create to this transition.
	// Authorization was already enforced at create, so a no-longer-permitted skip
	// (the opt-in was revoked between create and commit) is treated as
	// not-honored here — the error is intentionally discarded, never a 422.
	honoredTracerSkip, _ := skip.ResolveSkipFor("tracer", tran.Body.Skip != nil && tran.Body.Skip.Tracer, ledgerSettings.Overrides.AllowTracerSkip)

	action := constant.ActionCommit
	if transactionStatus == constant.CANCELED {
		action = constant.ActionCancel
	}

	// Route ONLY the pre-write balance reads to the primary via a dedicated ctx:
	// their result seeds the authoritative balance via the NX-seed, so a stale
	// replica read here corrupts money. The mark lives on readCtx, scoped to the
	// direct balance read and the cancel overdraft-enrichment read; the unmarked
	// ctx flows to everything else (validation, Redis seed, balance processing,
	// write) so those keep their default routing. The flag governs the effect; the
	// mark is unconditional.
	readCtx := readrouting.WithPrimaryRead(ctx)

	balances, err := handler.Query.GetBalances(readCtx, organizationID, ledgerID, validate.Aliases)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get balances", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get balances", libLog.Err(err))

		return nil, err
	}

	balanceOps := buildBalanceOperations(ctx, organizationID, ledgerID, validate, balances)
	balanceOps = annotateCanceledOverdraftAmounts(balanceOps, tran)

	var companionFromTos []mtransaction.FromTo
	if transactionStatus == constant.CANCELED {
		balanceOps, companionFromTos, err = enrichOverdraftOperations(readCtx, organizationID, ledgerID, balanceOps,
			validate, handler.Query.GetBalances)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to enrich canceled overdraft operations", err)
			logger.Log(ctx, libLog.LevelError, "Failed to enrich canceled overdraft operations", libLog.Err(err))

			return nil, err
		}
	}

	routeCache, err := handler.Query.ValidateAccountingRules(ctx, organizationID, ledgerID, balanceOps, validate, action)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate accounting rules", err)
		logger.Log(ctx, libLog.LevelError, "Failed to validate accounting rules", libLog.Err(err))

		return nil, err
	}

	if result == nil {
		attemptOwner, ownerErr := newPodRequestToken()
		if ownerErr != nil {
			return nil, ownerErr
		}
		attempt := &mmodel.BalanceExecutionAttempt{
			ExecutionKey: executionKey,
			OutcomeKey:   outcomeKey,
			Owner:        attemptOwner,
			Outcome:      expectedOutcome,
			Identity:     tran.IDtoUUID(),
		}
		executionAttempt = attempt
		acquired, acquireErr := handler.Command.TransactionRedisRepo.AcquireOwnedKey(ctx,
			executionKey, attemptOwner, balanceExecutionLeaseTTL)
		if acquireErr != nil || !acquired {
			return nil, transactionLifecycleReconciliationError()
		}

		ctxBackupSeed, spanBackupSeed := tracer.Start(ctx, "handler.commit_or_cancel_transaction.pre_seed_backup")
		if backupErr := handler.Command.SendTransactionToRedisQueue(ctxBackupSeed, organizationID, ledgerID,
			tran.IDtoUUID(), transactionInput, validate, transactionStatus, action, time.Now(), nil, nil, attempt); backupErr != nil {
			libOpentelemetry.HandleSpanError(spanBackupSeed, "Failed to pre-seed transaction backup cache", backupErr)
			logger.Log(ctx, libLog.LevelError, "Failed to pre-seed commit/cancel transaction backup cache", libLog.Err(backupErr))
			spanBackupSeed.End()

			released, releaseErr := handler.Command.TransactionRedisRepo.ReleaseOwnedKey(ctx, executionKey, attemptOwner)
			if releaseErr != nil || !released {
				return nil, transactionLifecycleReconciliationError()
			}

			return nil, pkg.ValidateBusinessError(backupErr, constant.EntityTransaction)
		}
		spanBackupSeed.End()

		result, err = handler.Command.ProcessBalanceOperations(ctx, command.ProcessBalanceOperationsInput{
			OrganizationID:    organizationID,
			LedgerID:          ledgerID,
			TransactionID:     tran.IDtoUUID(),
			TransactionInput:  nil, // State transitions skip balance-rule re-validation
			Validate:          validate,
			BalanceOperations: balanceOps,
			TransactionStatus: transactionStatus,
			ExecutionAttempt:  attempt,
		})
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to process balance operations", err)
			logger.Log(ctx, libLog.LevelError, "Failed to process balance operations", libLog.Err(err))

			persistedOutcome, outcomeErr := handler.readBalanceExecutionOutcome(ctx, organizationID, ledgerID, tran.IDtoUUID())
			if outcomeErr != nil || persistedOutcome == nil || persistedOutcome.Owner != attemptOwner ||
				persistedOutcome.Outcome != expectedOutcome {
				if markMovementApplied != nil {
					markMovementApplied()
				}

				return nil, transactionLifecycleReconciliationError()
			}
			result = balanceAtomicResultFromOutcome(persistedOutcome, organizationID, ledgerID)
			if result == nil {
				if markMovementApplied != nil {
					markMovementApplied()
				}

				return nil, transactionLifecycleReconciliationError()
			}
		}
	}
	if markMovementApplied != nil {
		markMovementApplied()
	}

	tran.UpdatedAt = time.Now()
	tran.Status = transaction.Status{
		Code:        transactionStatus,
		Description: &transactionStatus,
	}
	if _, err := handler.Command.UpdateTransactionStatusTx(ctx, dbTx, tran); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to CAS transaction status", err)

		return nil, err
	}
	if err := dbTx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to commit transaction status", err)

		return nil, fmt.Errorf("commit serialized transaction transition: %w", err)
	}

	// Reservation phase two by transaction (F3-T15, PENDING lifecycle). The
	// PENDING create path reserved capacity but deferred the confirm/release to
	// this state transition; /commit and /cancel carry only the transaction id, so
	// the tracer is addressed by transaction id and flips every RESERVED
	// reservation the transaction holds. Non-blocking: a transport failure never
	// fails the request. Expiry safely drains a lost release after cancel, while a
	// lost confirm after commit remains a known usage-undercount gap. The
	// long-lived TTL hint set at create-pending keeps these reservations alive
	// until this transition.
	switch transactionStatus {
	case constant.APPROVED:
		handler.confirmReservationsByTransaction(ctx, span, logger, ledgerSettings.Tracer, tran.IDtoUUID(), honoredTracerSkip)
	case constant.CANCELED:
		handler.releaseReservationsByTransaction(ctx, span, logger, ledgerSettings.Tracer, tran.IDtoUUID(), honoredTracerSkip)
	}

	balancesBefore, balancesAfter := result.Before, result.After

	fromTo = append(fromTo, mtransaction.MutateSplitAliases(transactionInput.Send.Source.From)...)
	to = mtransaction.MutateSplitAliases(transactionInput.Send.Distribute.To)

	if transactionStatus != constant.CANCELED {
		fromTo = append(fromTo, to...)
	}

	fromTo = append(fromTo, companionFromTos...)

	operations, preBalances, err := handler.BuildOperations(ctx, balancesBefore, balancesAfter, fromTo, transactionInput, *tran, validate, time.Now(), false, ledgerSettings.Accounting.ValidateRoutes, routeCache, action)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build operations", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build operations", libLog.Err(err))

		return nil, err
	}

	tran.Source = getAliasWithoutKey(filterCompanionAliases(validate.Sources))
	tran.Destination = getAliasWithoutKey(filterCompanionAliases(validate.Destinations))

	// The balance Lua already enriched the pre-seeded envelope with the exact
	// before/after snapshots. Add operation IDs through an owner/outcome CAS;
	// replacing this record would destroy the authoritative post-Lua proof.
	operations, err = handler.Command.UpdateTransactionBackupOperations(ctx, organizationID, ledgerID,
		tran.IDtoUUID(), operations, action, executionAttempt)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to durably bind lifecycle operations to balance outcome", err)
		logger.Log(ctx, libLog.LevelError, "Failed to durably bind lifecycle operations to balance outcome",
			libLog.String("transaction_id", tran.ID), libLog.Err(err))

		return nil, err
	}
	tran.Operations = operations

	err = handler.Command.WriteTransaction(ctx, organizationID, ledgerID, &transactionInput, validate, preBalances,
		balancesAfter, tran, executionAttempt)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, "failed to update BTO")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "failed to update BTO", err)

		logger.Log(ctx, libLog.LevelError, "Failed to update BTO", libLog.String("transaction_id", tran.ID), libLog.Err(err))

		return nil, err
	}
	tenantCtx := tmcore.ContextWithTenantID(context.Background(), tmcore.GetTenantIDContext(ctx))

	go handler.Command.SendLogTransactionAuditQueue(tenantCtx, operations, organizationID, ledgerID, tran.IDtoUUID())

	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		go handler.Command.UpdateWriteBehindTransaction(tenantCtx, organizationID, ledgerID, tran)
	}

	return tran, nil
}
