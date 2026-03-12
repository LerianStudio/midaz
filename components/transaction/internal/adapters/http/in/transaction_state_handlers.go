// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// CommitTransaction method that commit transaction created before
//
//	@Summary		Commit a Transaction
//	@Description	Commit a previously created transaction
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction_id	path		string	true	"Transaction ID"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid request or transaction cannot be reverted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		409				{object}	mmodel.Error	"Transaction already has a parent transaction"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/commit [Post]
func (handler *TransactionHandler) CommitTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve transaction on query", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error()))

			return http.WithError(c, err)
		}
	}

	return handler.commitOrCancelTransaction(c, tran, constant.APPROVED)
}

// CancelTransaction method that cancel pre transaction created before
//
//	@Summary		Cancel a pre transaction
//	@Description	Cancel a previously created pre transaction
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction_id	path		string	true	"Transaction ID"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid request or transaction cannot be reverted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		409				{object}	mmodel.Error	"Transaction already has a parent transaction"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/cancel [Post]
func (handler *TransactionHandler) CancelTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve transaction on query", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error()))

			return http.WithError(c, err)
		}
	}

	return handler.commitOrCancelTransaction(c, tran, constant.CANCELED)
}

// RevertTransaction method that revert transaction created before
//
//	@Summary		Revert a Transaction
//	@Description	Revert a Transaction with Transaction ID only
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction_id	path		string	true	"Transaction ID"
//	@Success		200				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid request or transaction cannot be reverted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		409				{object}	mmodel.Error	"Transaction already has a parent transaction"
//	@Failure		422				{object}	mmodel.Error	"Unprocessable Entity, validation errors"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert [post]
func (handler *TransactionHandler) RevertTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve Parent Transaction on query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to retrieve Parent Transaction with ID: %s, Error: %s", transactionID.String(), err.Error()))

		return http.WithError(c, err)
	}

	if parent != nil {
		err = pkg.ValidateBusinessError(constant.ErrTransactionIDHasAlreadyParentTransaction, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction Has Already Parent Transaction", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Transaction Has Already Parent Transaction with ID: %s, Error: %s", transactionID.String(), err))

		return http.WithError(c, err)
	}

	tran, err := handler.Query.GetTransactionWithOperationsByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve transaction on query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error()))

		return http.WithError(c, err)
	}

	if tran.ParentTransactionID != nil {
		err = pkg.ValidateBusinessError(constant.ErrTransactionIDIsAlreadyARevert, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction Has Already Parent Transaction", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Transaction Has Already Parent Transaction with ID: %s, Error: %s", transactionID.String(), err))

		return http.WithError(c, err)
	}

	if tran.Status.Code != constant.APPROVED {
		err = pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction CantRevert Transaction", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Transaction CantRevert Transaction with ID: %s, Error: %s", transactionID.String(), err))

		return http.WithError(c, err)
	}

	transactionReverted := tran.TransactionRevert()
	if transactionReverted.IsEmpty() {
		err = pkg.ValidateBusinessError(constant.ErrTransactionCantRevert, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction can't be reverted", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Parent Transaction can't be reverted with ID: %s, Error: %s", transactionID.String(), err))

		return http.WithError(c, err)
	}

	response := handler.createTransaction(c, transactionReverted, constant.CREATED)

	return response
}

// UpdateTransaction method that patch transaction created before
//
//	@Summary		Update a Transaction
//	@Description	Update a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string					true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string					false	"Request ID"
//	@Param			organization_id	path		string					true	"Organization ID"
//	@Param			ledger_id		path		string					true	"Ledger ID"
//	@Param			transaction_id	path		string					true	"Transaction ID"
//	@Param			transaction		body		transaction.UpdateTransactionInput	true	"Transaction Input"
//	@Success		200				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id} [patch]
func (handler *TransactionHandler) UpdateTransaction(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Initiating update of Transaction with Organization ID: %s, Ledger ID: %s and ID: %s", organizationID.String(), ledgerID.String(), transactionID.String()))

	payload := p.(*transaction.UpdateTransactionInput)
	logSafePayload(ctx, logger, "Request to update a transaction", payload)

	recordSafePayloadAttributes(span, payload)

	_, err = handler.Command.UpdateTransaction(ctx, organizationID, ledgerID, transactionID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction on command", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to update Transaction with ID: %s, Error: %s", transactionID.String(), err.Error()))

		return http.WithError(c, err)
	}

	trans, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve transaction on query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error()))

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully updated Transaction with Organization ID: %s, Ledger ID: %s and ID: %s", organizationID.String(), ledgerID.String(), transactionID.String()))

	return http.OK(c, trans)
}

func (handler *TransactionHandler) commitOrCancelTransaction(c *fiber.Ctx, tran *transaction.Transaction, transactionStatus string) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.commit_or_cancel_transaction")
	defer span.End()

	organizationID := uuid.MustParse(tran.OrganizationID)
	ledgerID := uuid.MustParse(tran.LedgerID)

	lockPendingTransactionKey := utils.PendingTransactionLockKey(organizationID, ledgerID, tran.ID)

	ttl := time.Duration(300)

	success, err := handler.Command.RedisRepo.SetNX(ctx, lockPendingTransactionKey, "", ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set on redis", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to set on redis: %v", err))

		return http.WithError(c, err)
	}

	if !success {
		err := pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction is locked", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed, Transaction: %s is locked, Error: %s", tran.ID, err.Error()))

		return http.WithError(c, err)
	}

	deleteLockOnError := func() {
		if delErr := handler.Command.RedisRepo.Del(ctx, lockPendingTransactionKey); delErr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete pending transaction lock", delErr)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to delete pending transaction lock key: %v", delErr))
		}
	}

	transactionInput := tran.Body

	handler.ApplyDefaultBalanceKeys(transactionInput.Send.Source.From)
	handler.ApplyDefaultBalanceKeys(transactionInput.Send.Distribute.To)

	var fromTo []pkgTransaction.FromTo

	fromTo = append(fromTo, handler.HandleAccountFields(transactionInput.Send.Source.From, true)...)
	to := handler.HandleAccountFields(transactionInput.Send.Distribute.To, true)

	if transactionStatus != constant.CANCELED {
		fromTo = append(fromTo, to...)
	}

	if tran.Status.Code != constant.PENDING {
		err := pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction is not pending", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed, Transaction: %s is not pending, Error: %s", tran.ID, err.Error()))

		deleteLockOnError()

		return http.WithError(c, err)
	}

	validate, err := pkgTransaction.ValidateSendSourceAndDistribute(ctx, transactionInput, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate send source and distribute: %s", err.Error()))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		deleteLockOnError()

		return http.WithError(c, err)
	}

	_, spanGetBalances := tracer.Start(ctx, "handler.create_transaction.get_balances")

	balancesBefore, balancesAfter, err := handler.Query.GetBalances(ctx, organizationID, ledgerID, tran.IDtoUUID(), nil, validate, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanGetBalances, "Failed to get balances", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get balances: %v", err.Error()))
		spanGetBalances.End()

		deleteLockOnError()

		return http.WithError(c, err)
	}

	fromTo = append(fromTo, handler.HandleAccountFields(transactionInput.Send.Source.From, false)...)
	to = handler.HandleAccountFields(transactionInput.Send.Distribute.To, false)

	if transactionStatus != constant.CANCELED {
		fromTo = append(fromTo, to...)
	}

	tran.UpdatedAt = time.Now()
	tran.Status = transaction.Status{
		Code:        transactionStatus,
		Description: &transactionStatus,
	}

	operations, preBalances, err := handler.BuildOperations(ctx, balancesBefore, fromTo, transactionInput, *tran, validate, time.Now(), false)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to validate balances", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate balance: %v", err.Error()))

		return http.WithError(c, err)
	}

	tran.Source = getAliasWithoutKey(validate.Sources)
	tran.Destination = getAliasWithoutKey(validate.Destinations)
	tran.Operations = operations

	ctxBackup, spanBackup := tracer.Start(ctx, "handler.commit_or_cancel_transaction.send_to_redis_queue")

	if backupErr := handler.Command.SendTransactionToRedisQueue(ctxBackup, organizationID, ledgerID, tran.IDtoUUID(), transactionInput, validate, transactionStatus, time.Now(), preBalances); backupErr != nil {
		libOpentelemetry.HandleSpanError(spanBackup, "Failed to send transaction to backup cache", backupErr)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to send commit/cancel transaction to backup cache: %v", backupErr))
	}

	spanBackup.End()

	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		_, err = handler.Command.UpdateTransactionStatus(ctx, tran)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction status synchronously", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to update transaction status synchronously for Transaction: %s, Error: %s", tran.ID, err.Error()))
		}
	}

	err = handler.Command.WriteTransaction(ctx, organizationID, ledgerID, &transactionInput, validate, preBalances, balancesAfter, tran)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, "failed to update BTO")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "failed to update BTO", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("failed to update BTO - transaction: %s - Error: %v", tran.ID, err))

		return http.WithError(c, err)
	}

	go handler.Command.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, tran.IDtoUUID())

	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		go handler.Command.UpdateWriteBehindTransaction(context.Background(), organizationID, ledgerID, tran)
	}

	return http.Created(c, tran)
}
