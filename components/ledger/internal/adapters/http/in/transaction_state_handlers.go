// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"os"
	"strings"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// CommitTransaction method that commit transaction created before
//
//	@Summary		Commit a Transaction
//	@Description	Transitions a PENDING transaction to APPROVED, releasing held balances. Only PENDING transactions can be committed.
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string						true	"Ledger ID in UUID format"
//	@Param			transaction_id	path		string						true	"Transaction ID in UUID format"
//	@Success		201				{object}	transaction.Transaction		"Successfully committed transaction"
//	@Failure		400				{object}	mmodel.Error				"Invalid request or transaction cannot be committed"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Transaction not found"
//	@Failure		409				{object}	mmodel.Error				"Conflict: transaction is not in a state that allows this action"
//	@Failure		422				{object}	mmodel.Error				"Business validation failed (e.g. same account in sources and destinations)"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/commit [post]
func (handler *TransactionHandler) CommitTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

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

	_, span := tracer.Start(ctx, "handler.commit_transaction")
	defer span.End()

	tran, err := handler.Query.GetWriteBehindTransaction(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		tran, err = handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
		if err != nil {
			handleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

			return http.WithError(c, err)
		}
	}

	return handler.commitOrCancelTransaction(c, tran, constant.APPROVED)
}

// CancelTransaction method that cancel pre transaction created before
//
//	@Summary		Cancel a pre transaction
//	@Description	Transitions a PENDING transaction to CANCELED, reversing held reservations. Only PENDING transactions can be cancelled.
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string						true	"Ledger ID in UUID format"
//	@Param			transaction_id	path		string						true	"Transaction ID in UUID format"
//	@Success		201				{object}	transaction.Transaction		"Successfully cancelled transaction"
//	@Failure		400				{object}	mmodel.Error				"Invalid request or transaction cannot be cancelled"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Transaction not found"
//	@Failure		409				{object}	mmodel.Error				"Conflict: transaction is not in a state that allows this action"
//	@Failure		422				{object}	mmodel.Error				"Business validation failed (e.g. same account in sources and destinations)"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/cancel [post]
func (handler *TransactionHandler) CancelTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

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

	_, span := tracer.Start(ctx, "handler.cancel_transaction")
	defer span.End()

	tran, err := handler.Query.GetWriteBehindTransaction(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		tran, err = handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
		if err != nil {
			handleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

			return http.WithError(c, err)
		}
	}

	return handler.commitOrCancelTransaction(c, tran, constant.CANCELED)
}

// RevertTransaction method that revert transaction created before
//
//	@Summary		Revert a Transaction
//	@Description	Creates a mirror reversal transaction inverting all operations of the original. Only APPROVED, not-already-reverted transactions with all routes bidirectional can be reverted.
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string						true	"Ledger ID in UUID format"
//	@Param			transaction_id	path		string						true	"Transaction ID in UUID format"
//	@Success		201				{object}	transaction.Transaction		"Successfully reverted transaction"
//	@Failure		400				{object}	mmodel.Error				"Invalid request or transaction cannot be reverted"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Transaction not found"
//	@Failure		409				{object}	mmodel.Error				"Conflict: transaction already reverted, is itself a revert, or not in a revertable state"
//	@Failure		422				{object}	mmodel.Error				"Business validation failed (e.g. transaction cannot be reverted or route is not bidirectional)"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert [post]
func (handler *TransactionHandler) RevertTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

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

	_, span := tracer.Start(ctx, "handler.revert_transaction")
	defer span.End()

	parent, err := handler.Query.GetParentByTransactionID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Parent Transaction on query", err)

		return http.WithError(c, err)
	}

	if parent != nil {
		err = pkg.ValidateBusinessError(constant.ErrTransactionIDHasAlreadyParentTransaction, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction Has Already Parent Transaction", err)

		return http.WithError(c, err)
	}

	tran, err := handler.Query.GetTransactionWithOperationsByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

		return http.WithError(c, err)
	}

	if tran.ParentTransactionID != nil {
		err = pkg.ValidateBusinessError(constant.ErrTransactionIDIsAlreadyARevert, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction Has Already Parent Transaction", err)

		return http.WithError(c, err)
	}

	if tran.Status.Code != constant.APPROVED {
		err = pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction CantRevert Transaction", err)

		return http.WithError(c, err)
	}

	transactionReverted := tran.TransactionRevert()
	if transactionReverted.IsEmpty() {
		err = pkg.ValidateBusinessError(constant.ErrTransactionCantRevert, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction can't be reverted", err)

		return http.WithError(c, err)
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

			return http.WithError(c, parseValidationErr)
		}

		operationRoute, routeErr := handler.Query.GetOperationRouteByID(ctx, organizationID, ledgerID, nil, routeUUID)
		if routeErr != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to retrieve operation route for revert validation", routeErr)

			return http.WithError(c, routeErr)
		}

		if operationRoute != nil && operationRoute.OperationType != "bidirectional" {
			err = pkg.ValidateBusinessError(constant.ErrRouteNotBidirectional, "RevertTransaction")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Operation route is not bidirectional", err)

			return http.WithError(c, err)
		}
	}

	response := handler.createRevertTransaction(c, transactionReverted, constant.CREATED)

	return response
}

// UpdateTransaction method that patch transaction created before
//
//	@Summary		Update a Transaction
//	@Description	Updates mutable transaction fields (description, metadata). Amounts, accounts, and status are immutable.
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string								false	"Request ID for tracing"
//	@Param			organization_id	path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string								true	"Ledger ID in UUID format"
//	@Param			transaction_id	path		string								true	"Transaction ID in UUID format"
//	@Param			transaction		body		transaction.UpdateTransactionInput	true	"Transaction Input"
//	@Success		200				{object}	transaction.Transaction				"Successfully updated transaction"
//	@Failure		400				{object}	mmodel.Error						"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error						"Forbidden access"
//	@Failure		404				{object}	mmodel.Error						"Transaction not found"
//	@Failure		500				{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id} [patch]
func (handler *TransactionHandler) UpdateTransaction(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_transaction")
	defer span.End()

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
	logSafePayload(ctx, logger, "Request to update a transaction", payload)

	recordSafePayloadAttributes(span, payload)

	_, err = handler.Command.UpdateTransaction(ctx, organizationID, ledgerID, transactionID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to update transaction on command", err)

		return http.WithError(c, err)
	}

	trans, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

		return http.WithError(c, err)
	}

	return http.OK(c, trans)
}

//nolint:gocyclo // State machine with branches per status × action combination; refactor candidate.
func (handler *TransactionHandler) commitOrCancelTransaction(c *fiber.Ctx, tran *transaction.Transaction, transactionStatus string) error {
	ctx := c.UserContext()
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

		return http.WithError(c, err)
	}

	if !success {
		err := pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction is locked", err)

		logger.Log(ctx, libLog.LevelWarn, "Transaction is locked", libLog.String("transaction_id", tran.ID), libLog.Err(err))

		return http.WithError(c, err)
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

		return http.WithError(c, err)
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

		return http.WithError(c, err)
	}

	ledgerSettings, err := handler.Query.GetParsedLedgerSettings(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get ledger settings", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get ledger settings", libLog.Err(err))

		deleteLockOnError()

		return http.WithError(c, err)
	}

	if ledgerSettings.Accounting.ValidateRoutes {
		mtransaction.PropagateRouteValidation(ctx, validate, transactionStatus)
	}

	action := constant.ActionCommit
	if transactionStatus == constant.CANCELED {
		action = constant.ActionCancel
	}

	balances, err := handler.Query.GetBalances(ctx, organizationID, ledgerID, validate.Aliases)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get balances", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get balances", libLog.Err(err))

		deleteLockOnError()

		return http.WithError(c, err)
	}

	balanceOps := buildBalanceOperations(ctx, organizationID, ledgerID, validate, balances)
	balanceOps = annotateCanceledOverdraftAmounts(balanceOps, tran)

	var companionFromTos []mtransaction.FromTo
	if transactionStatus == constant.CANCELED {
		balanceOps, companionFromTos, err = enrichOverdraftOperations(ctx, organizationID, ledgerID, balanceOps,
			validate, handler.Query.GetBalances)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to enrich canceled overdraft operations", err)
			logger.Log(ctx, libLog.LevelError, "Failed to enrich canceled overdraft operations", libLog.Err(err))

			deleteLockOnError()

			return http.WithError(c, err)
		}
	}

	routeCache, err := handler.Query.ValidateAccountingRules(ctx, organizationID, ledgerID, balanceOps, validate, action)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate accounting rules", err)
		logger.Log(ctx, libLog.LevelError, "Failed to validate accounting rules", libLog.Err(err))

		deleteLockOnError()

		return http.WithError(c, err)
	}

	ctxBackupSeed, spanBackupSeed := tracer.Start(ctx, "handler.commit_or_cancel_transaction.pre_seed_backup")

	if backupErr := handler.Command.SendTransactionToRedisQueue(ctxBackupSeed, organizationID, ledgerID, tran.IDtoUUID(), transactionInput, validate, transactionStatus, action, time.Now(), nil); backupErr != nil {
		libOpentelemetry.HandleSpanError(spanBackupSeed, "Failed to pre-seed transaction backup cache", backupErr)

		logger.Log(ctx, libLog.LevelError, "Failed to pre-seed commit/cancel transaction backup cache", libLog.Err(backupErr))

		spanBackupSeed.End()

		deleteLockOnError()

		return http.WithError(c, pkg.ValidateBusinessError(backupErr, constant.EntityTransaction))
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

		return http.WithError(c, err)
	}

	// Reservation phase two by transaction (F3-T15, PENDING lifecycle). The
	// PENDING create path reserved capacity but deferred the confirm/release to
	// this state transition; /commit and /cancel carry only the transaction id, so
	// the tracer is addressed by transaction id and flips every RESERVED
	// reservation the transaction holds. Non-blocking: a transport failure never
	// fails the request — the TTL reaper reconciles. The long-lived TTL hint set
	// at create-pending keeps these reservations alive until this transition.
	switch transactionStatus {
	case constant.APPROVED:
		handler.confirmReservationsByTransaction(ctx, span, logger, ledgerSettings.Tracer, tran.IDtoUUID())
	case constant.CANCELED:
		handler.releaseReservationsByTransaction(ctx, span, logger, ledgerSettings.Tracer, tran.IDtoUUID())
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

		return http.WithError(c, err)
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

		return http.WithError(c, err)
	}

	tenantCtx := tmcore.ContextWithTenantID(context.Background(), tmcore.GetTenantIDContext(ctx))

	go handler.Command.SendLogTransactionAuditQueue(tenantCtx, operations, organizationID, ledgerID, tran.IDtoUUID())

	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		go handler.Command.UpdateWriteBehindTransaction(tenantCtx, organizationID, ledgerID, tran)
	}

	return http.Created(c, tran)
}
