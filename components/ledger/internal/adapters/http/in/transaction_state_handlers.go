// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"os"
	"strings"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/skip"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// revertIdempotencyReplayedLogMessage is the Warn message the revert core records when the
// idempotency slot answers with a cached reverse instead of a new one.
const revertIdempotencyReplayedLogMessage = "Revert replayed a cached reverse transaction"

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
func (handler *TransactionHandler) commitTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionStatus string) (*transaction.Transaction, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	spanName := "handler.commit_transaction"
	if transactionStatus == constant.CANCELED {
		spanName = "handler.cancel_transaction"
	}

	_, span := tracer.Start(ctx, spanName)
	defer span.End()

	tran, err := handler.Query.GetWriteBehindTransaction(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		// Load the operations with the transaction: cancel needs them to unwind an
		// overdraft hold. The write-behind cache is cleared once the create persists,
		// so this fallback carries the transaction into commitOrCancelTransaction and
		// its annotateCanceledOverdraftAmounts step, which reads tran.Operations to
		// size the overdraft deficit. A row-only read leaves Operations empty and the
		// cancel restores the full hold to available instead of only the non-overdraft
		// portion.
		tran, err = handler.Query.GetTransactionWithOperationsByID(ctx, organizationID, ledgerID, transactionID)
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
			tran, err = handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
			if err != nil {
				handleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

				return nil, err
			}
		}
	}

	return handler.commitOrCancelTransaction(ctx, tran, transactionStatus)
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

// KNOWN DEFECT — REVERT IDEMPOTENCY IS NOT SCOPED BY ORIGIN.
//
// Revert sends no X-Idempotency header, so CreateOrCheckTransactionIdempotency falls back to
// key = HashSHA256(preimage), and with no override resolveIdempotencyHashSource serialises the
// reversal payload. TransactionRevert() copies only the origin's economic content
// (description, asset, amount, legs, route, metadata) and NEVER the origin id, so two
// economically-identical origins in the same ledger derive the SAME key and share ONE slot:
// the second revert loses the SetNX, is handed the FIRST origin's cached reverse, and answers
// 201 while its own origin is never reverted. Silently — no error, no distinguishable status.
//
// The fix is an origin-scoped preimage. It is deliberately NOT applied here: v1 revert is
// released, and changing the preimage changes the Redis key shape, so a revert retried across
// a rolling-deploy boundary would land on a different slot and could double-revert. It re-lands
// together with the idempotency keyspace separation, which re-shapes the key anyway, behind a
// dual-write/dual-read migration — one coordinated deploy window instead of two.
//
// Until then the ONLY control is detection: the replayed flag below, its Warn, and the
// X-Idempotency-Replayed header the transports project. Do not treat that as a fix.
// The reproduction is preserved, skipped, in
// TestIntegration_TransactionV2Revert_IdempotencyNotScopedByOrigin_KnownDefect.
//
// revertTransaction is the transport-neutral revert core: it runs the full revert
// eligibility gate (no-parent, not-already-a-revert, APPROVED status, non-empty reversal,
// all bidirectional routes) then delegates to the untouched createRevertTransaction core.
// The parent transaction id passed to createRevertTransaction is the reverted
// transaction's id (from the route), so the reversal links back to its origin. Revert
// sends no idempotency headers, so the key is empty (the core keys on the reversal hash)
// and the TTL defaults to ParseIdempotencyTTL("") == 300s — byte-identical to the
// pre-migration Fiber path, which reached executeCreateTransaction and read
// GetIdempotencyKeyAndTTL(c) (an absent X-TTL defaults to 300, never 0). A hardcoded 0
// would make the Redis idempotency slot permanent. It returns the idempotency `replayed`
// flag alongside the reverse transaction so the transport sets X-Idempotency-Replayed
// itself.
func (handler *TransactionHandler) revertTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.revert_transaction")
	defer span.End()

	parent, err := handler.Query.GetParentByTransactionID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Parent Transaction on query", err)

		return nil, false, err
	}

	if parent != nil {
		err = pkg.ValidateBusinessError(constant.ErrTransactionIDHasAlreadyParentTransaction, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction Has Already Parent Transaction", err)

		return nil, false, err
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

	params := &transactionPathParams{OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: transactionID}

	tranReverted, replayed, err := handler.createRevertTransaction(ctx, params, transactionReverted, constant.CREATED, "", http.ParseIdempotencyTTL(""))
	if err != nil {
		return nil, false, err
	}

	if replayed {
		// A replay is an outcome this span observed, not an input, so it belongs outside the
		// app.request.* namespace (T4). It is also not an error: the span stays green.
		span.SetAttributes(attribute.Bool("app.response.idempotency_replayed", true))

		// Warn — deliberately louder than the create paths, which treat a replay as routine.
		// A create replay is what the caller asked for: they sent X-Idempotency, so a cached
		// answer is the contract. Revert carries no caller key, so nobody asked for this one;
		// it means the caller's revert did NOT happen and the 201 alone cannot tell them so.
		// While the origin-agnostic key above stands, the cached reverse may not
		// even belong to this origin, so this is the only operator-visible trace of the
		// defect — Debug, typically not collected in production, could not carry it.
		logger.Log(ctx, libLog.LevelWarn, revertIdempotencyReplayedLogMessage, libLog.String("transaction_id", transactionID.String()))
	}

	return tranReverted, replayed, nil
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

// updateTransaction is the transport-neutral update core: it logs the safe payload,
// runs command.UpdateTransaction, then re-reads the transaction via query.GetTransactionByID
// (mutable fields only — amounts/accounts/status are immutable).
func (handler *TransactionHandler) updateTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, payload *transaction.UpdateTransactionInput) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_transaction")
	defer span.End()

	logSafePayload(ctx, logger, "Request to update a transaction", payload)

	recordSafePayloadAttributes(span, payload)

	_, err := handler.Command.UpdateTransaction(ctx, organizationID, ledgerID, transactionID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to update transaction on command", err)

		return nil, err
	}

	trans, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

		return nil, err
	}

	return trans, nil
}

// commitOrCancelTransaction is the transport-neutral state-transition core (the tracer
// two-phase confirm/release-by-transaction, balance ProcessBalanceOperations, backup
// seeding, and BuildOperations/WriteTransaction). It is called by BOTH the Fiber
// wrappers and the Huma shells; the ~275-line body is untouched (no reordered side-
// effects). It returns the updated transaction so each transport writes its own
// response.
//
//nolint:gocyclo // State machine with branches per status × action combination; refactor candidate.
func (handler *TransactionHandler) commitOrCancelTransaction(ctx context.Context, tran *transaction.Transaction, transactionStatus string) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.commit_or_cancel_transaction")
	defer span.End()

	organizationID := uuid.MustParse(tran.OrganizationID)
	ledgerID := uuid.MustParse(tran.LedgerID)

	lockPendingTransactionKey := utils.PendingTransactionLockKey(organizationID, ledgerID, tran.ID)

	ttl := time.Duration(300)

	success, err := handler.Command.TransactionRedisRepo.SetNX(ctx, lockPendingTransactionKey, "", ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set on redis", err)

		logger.Log(ctx, libLog.LevelError, "Failed to set pending transaction lock on redis", libLog.Err(err))

		return nil, err
	}

	if !success {
		err := pkg.ValidateBusinessError(constant.ErrPendingTransactionLocked, "ValidateTransactionNotPending")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction is locked", err)

		logger.Log(ctx, libLog.LevelWarn, "Transaction is locked", libLog.String("transaction_id", tran.ID), libLog.Err(err))

		return nil, err
	}

	deleteLockOnError := func() {
		if delErr := handler.Command.TransactionRedisRepo.Del(ctx, lockPendingTransactionKey); delErr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete pending transaction lock", delErr)

			logger.Log(ctx, libLog.LevelError, "Failed to delete pending transaction lock key", libLog.Err(delErr))
		}
	}

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

		deleteLockOnError()

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

		deleteLockOnError()

		return nil, err
	}

	ledgerSettings, err := handler.Query.GetParsedLedgerSettings(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get ledger settings", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get ledger settings", libLog.Err(err))

		deleteLockOnError()

		return nil, err
	}

	if ledgerSettings.Accounting.ValidateRoutes {
		mtransaction.PropagateRouteValidation(ctx, validate, transactionStatus)
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

		deleteLockOnError()

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

			deleteLockOnError()

			return nil, err
		}
	}

	routeCache, err := handler.Query.ValidateAccountingRules(ctx, organizationID, ledgerID, balanceOps, validate, action)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate accounting rules", err)
		logger.Log(ctx, libLog.LevelError, "Failed to validate accounting rules", libLog.Err(err))

		deleteLockOnError()

		return nil, err
	}

	ctxBackupSeed, spanBackupSeed := tracer.Start(ctx, "handler.commit_or_cancel_transaction.pre_seed_backup")

	if backupErr := handler.Command.SendTransactionToRedisQueue(ctxBackupSeed, organizationID, ledgerID, tran.IDtoUUID(), transactionInput, validate, transactionStatus, action, time.Now(), nil); backupErr != nil {
		libOpentelemetry.HandleSpanError(spanBackupSeed, "Failed to pre-seed transaction backup cache", backupErr)

		logger.Log(ctx, libLog.LevelError, "Failed to pre-seed commit/cancel transaction backup cache", libLog.Err(backupErr))

		spanBackupSeed.End()

		deleteLockOnError()

		return nil, pkg.ValidateBusinessError(backupErr, constant.EntityTransaction)
	}

	spanBackupSeed.End()

	result, err := handler.Command.ProcessBalanceOperations(ctx, command.ProcessBalanceOperationsInput{
		OrganizationID:    organizationID,
		LedgerID:          ledgerID,
		TransactionID:     tran.IDtoUUID(),
		TransactionInput:  nil, // State transitions skip balance-rule re-validation
		Validate:          validate,
		BalanceOperations: balanceOps,
		TransactionStatus: transactionStatus,
	})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to process balance operations", err)
		logger.Log(ctx, libLog.LevelError, "Failed to process balance operations", libLog.Err(err))

		handler.Command.RemoveTransactionFromRedisQueue(ctx, logger, organizationID, ledgerID, tran.IDtoUUID().String())

		deleteLockOnError()

		return nil, err
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

	tran.UpdatedAt = time.Now()
	tran.Status = transaction.Status{
		Code:        transactionStatus,
		Description: &transactionStatus,
	}

	operations, preBalances, err := handler.BuildOperations(ctx, balancesBefore, balancesAfter, fromTo, transactionInput, *tran, validate, time.Now(), false, ledgerSettings.Accounting.ValidateRoutes, routeCache, action)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build operations", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build operations", libLog.Err(err))

		deleteLockOnError()

		return nil, err
	}

	tran.Source = getAliasWithoutKey(filterCompanionAliases(validate.Sources))
	tran.Destination = getAliasWithoutKey(filterCompanionAliases(validate.Destinations))
	tran.Operations = operations

	ctxBackup, spanBackup := tracer.Start(ctx, "handler.commit_or_cancel_transaction.send_to_redis_queue")

	if backupErr := handler.Command.SendTransactionToRedisQueue(ctxBackup, organizationID, ledgerID, tran.IDtoUUID(), transactionInput, validate, transactionStatus, action, time.Now(), preBalances); backupErr != nil {
		libOpentelemetry.HandleSpanError(spanBackup, "Failed to send transaction to backup cache", backupErr)

		logger.Log(ctx, libLog.LevelWarn, "Failed to send commit/cancel transaction to backup cache", libLog.Err(backupErr))
	}

	spanBackup.End()

	// Materialize operation IDs in the backup entry to prevent duplicate
	// operations if the Redis backup consumer replays this transaction.
	// Without this, BuildOperations() generates new UUIDs on replay.
	handler.Command.UpdateTransactionBackupOperations(ctx, organizationID, ledgerID, tran.IDtoUUID().String(), operations, action)

	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		_, err = handler.Command.UpdateTransactionStatus(ctx, tran)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction status synchronously", err)

			logger.Log(ctx, libLog.LevelError, "Failed to update transaction status synchronously", libLog.String("transaction_id", tran.ID), libLog.Err(err))
		}
	}

	err = handler.Command.WriteTransaction(ctx, organizationID, ledgerID, &transactionInput, validate, preBalances, balancesAfter, tran)
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
