// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libConstants "github.com/LerianStudio/lib-commons/v4/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

func (handler *TransactionHandler) HandleAccountFields(entries []pkgTransaction.FromTo, isConcat bool) []pkgTransaction.FromTo {
	result := make([]pkgTransaction.FromTo, 0, len(entries))

	for i := range entries {
		var newAlias string
		if isConcat {
			newAlias = entries[i].ConcatAlias(i)
		} else {
			newAlias = entries[i].SplitAlias()
		}

		entries[i].AccountAlias = newAlias

		result = append(result, entries[i])
	}

	return result
}

func (handler *TransactionHandler) checkTransactionDate(ctx context.Context, logger libLog.Logger, transactionInput pkgTransaction.Transaction, transactionStatus string) (time.Time, error) {
	now := time.Now()
	transactionDate := now

	if transactionInput.TransactionDate != nil && !transactionInput.TransactionDate.IsZero() {
		if transactionInput.TransactionDate.After(now) {
			err := pkg.ValidateBusinessError(constant.ErrInvalidFutureTransactionDate, "validateTransactionDate")

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("transaction date cannot be a future date: %v", err.Error()))

			return time.Time{}, err
		} else if transactionStatus == constant.PENDING {
			err := pkg.ValidateBusinessError(constant.ErrInvalidPendingFutureTransactionDate, "validateTransactionDate")

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("pending transaction cannot be used together a transaction date: %v", err.Error()))

			return time.Time{}, err
		} else {
			transactionDate = transactionInput.TransactionDate.Time()
		}
	}

	return transactionDate, nil
}

func (handler *TransactionHandler) BuildOperations(
	ctx context.Context,
	balances []*mmodel.Balance,
	fromTo []pkgTransaction.FromTo,
	transactionInput pkgTransaction.Transaction,
	tran transaction.Transaction,
	validate *pkgTransaction.Responses,
	transactionDate time.Time,
	isAnnotation bool,
) ([]*operation.Operation, []*mmodel.Balance, error) {
	var operations []*operation.Operation

	var preBalances []*mmodel.Balance

	logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction_operations")
	defer span.End()

	for _, blc := range balances {
		for i := range fromTo {
			if blc.Alias == fromTo[i].AccountAlias {
				logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Creating operation for account id: %s and account alias: %s", blc.ID, blc.Alias))

				preBalances = append(preBalances, blc)

				amt, bat, err := pkgTransaction.ValidateFromToOperation(fromTo[i], *validate, blc.ToTransactionBalance())
				if err != nil {
					libOpentelemetry.HandleSpanError(span, "Failed to validate balances", err)

					logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate balance: %v", err.Error()))

					return nil, nil, err
				}

				amount := operation.Amount{
					Value: &amt.Value,
				}

				balance := operation.Balance{
					Available: &blc.Available,
					OnHold:    &blc.OnHold,
					Version:   &blc.Version,
				}

				balanceAfter := operation.Balance{
					Available: &bat.Available,
					OnHold:    &bat.OnHold,
					Version:   &bat.Version,
				}

				if isAnnotation {
					a := decimal.NewFromInt(0)
					balance.Available = &a
					balanceAfter.Available = &a

					o := decimal.NewFromInt(0)
					balance.OnHold = &o
					balanceAfter.OnHold = &o

					vBefore := int64(0)
					balance.Version = &vBefore
					vAfter := int64(0)
					balanceAfter.Version = &vAfter
				}

				description := fromTo[i].Description
				if libCommons.IsNilOrEmpty(&fromTo[i].Description) {
					description = transactionInput.Description
				}

				operationID, err := libCommons.GenerateUUIDv7()
				if err != nil {
					return nil, nil, err
				}

				operations = append(operations, &operation.Operation{
					ID:              operationID.String(),
					TransactionID:   tran.ID,
					Description:     description,
					Type:            amt.Operation,
					AssetCode:       transactionInput.Send.Asset,
					ChartOfAccounts: fromTo[i].ChartOfAccounts,
					Amount:          amount,
					Balance:         balance,
					BalanceAfter:    balanceAfter,
					BalanceID:       blc.ID,
					AccountID:       blc.AccountID,
					AccountAlias:    pkgTransaction.SplitAlias(blc.Alias),
					BalanceKey:      blc.Key,
					OrganizationID:  blc.OrganizationID,
					LedgerID:        blc.LedgerID,
					CreatedAt:       transactionDate,
					UpdatedAt:       time.Now(),
					Route:           fromTo[i].Route,
					Metadata:        fromTo[i].Metadata,
					BalanceAffected: !isAnnotation,
				})

				if err := metricFactory.RecordTransactionProcessed(
					ctx,
					attribute.String("organization_id", tran.OrganizationID),
					attribute.String("ledger_id", tran.LedgerID),
				); err != nil {
					libOpentelemetry.HandleSpanError(span, "Failed to record transaction processed metric", err)
				}
			}
		}
	}

	return operations, preBalances, nil
}

func (handler *TransactionHandler) createTransaction(c *fiber.Ctx, transactionInput pkgTransaction.Transaction, transactionStatus string) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	parentID := uuid.Nil
	if c.Locals("transaction_id") != nil {
		parentID, err = http.GetUUIDFromLocals(c, "transaction_id")
		if err != nil {
			return http.WithError(c, err)
		}
	}

	transactionID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate transaction id", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to generate transaction id: %v", err))

		return http.WithError(c, pkg.InternalServerError{
			Code:    "INTERNAL_SERVER_ERROR",
			Title:   "Internal Server Error",
			Message: "Failed to generate transaction id",
			Err:     err,
		})
	}

	c.Set(libConstants.IdempotencyReplayed, "false")

	transactionDate, err := handler.checkTransactionDate(ctx, logger, transactionInput, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to check transaction date", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to check transaction date: %v", err))

		return http.WithError(c, err)
	}

	recordSafePayloadAttributes(span, transactionInput)

	if transactionInput.Send.Value.LessThanOrEqual(decimal.Zero) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, reflect.TypeOf(transaction.Transaction{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid transaction with non-positive value", err)

		logger.Log(ctx, libLog.LevelWarn, "Transaction value must be greater than zero")

		return http.WithError(c, err)
	}

	handler.ApplyDefaultBalanceKeys(transactionInput.Send.Source.From)
	handler.ApplyDefaultBalanceKeys(transactionInput.Send.Distribute.To)

	var fromTo []pkgTransaction.FromTo

	fromTo = append(fromTo, handler.HandleAccountFields(transactionInput.Send.Source.From, true)...)
	to := handler.HandleAccountFields(transactionInput.Send.Distribute.To, true)

	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, to...)
	}

	ctxIdempotency, spanIdempotency := tracer.Start(ctx, "handler.create_transaction_idempotency")

	ts, _ := libCommons.StructToJSONString(transactionInput)
	hash := libCommons.HashSHA256(ts)
	key, ttl := http.GetIdempotencyKeyAndTTL(c)

	value, internalKey, err := handler.Command.CreateOrCheckIdempotencyKey(ctxIdempotency, organizationID, ledgerID, key, hash, ttl)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanIdempotency, "Error on create or check redis idempotency key", err)
		spanIdempotency.End()

		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Error on create or check redis idempotency key: %v", err.Error()))

		return http.WithError(c, err)
	} else if !libCommons.IsNilOrEmpty(value) {
		t := transaction.Transaction{}
		if err = json.Unmarshal([]byte(*value), &t); err != nil {
			libOpentelemetry.HandleSpanError(spanIdempotency, "Error to deserialization idempotency transaction json on redis", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to deserialization idempotency transaction json on redis: %v", err))
			spanIdempotency.End()

			return http.WithError(c, err)
		}

		spanIdempotency.End()
		c.Set(libConstants.IdempotencyReplayed, "true")

		return http.Created(c, t)
	}

	spanIdempotency.End()

	validate, err := pkgTransaction.ValidateSendSourceAndDistribute(ctx, transactionInput, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to validate send source and distribute: %v", err.Error()))

		err = pkg.HandleKnownBusinessValidationErrors(err)

		_ = handler.Command.RedisRepo.Del(ctx, *internalKey)

		return http.WithError(c, err)
	}

	ctxSendTransactionToRedisQueue, spanSendTransactionToRedisQueue := tracer.Start(ctx, "handler.create_transaction.send_transaction_to_redis_queue")

	err = handler.Command.SendTransactionToRedisQueue(ctxSendTransactionToRedisQueue, organizationID, ledgerID, transactionID, transactionInput, validate, transactionStatus, transactionDate, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanSendTransactionToRedisQueue, "Failed to send transaction to backup cache", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to send transaction to backup cache: %v", err.Error()))

		if errors.Is(err, constant.ErrTransactionBackupCacheMarshalFailed) {
			_ = handler.Command.RedisRepo.Del(ctxSendTransactionToRedisQueue, *internalKey)
		}

		spanSendTransactionToRedisQueue.End()

		return http.WithError(c, pkg.ValidateBusinessError(err, reflect.TypeOf(transaction.Transaction{}).Name()))
	}

	spanSendTransactionToRedisQueue.End()

	_, spanGetBalances := tracer.Start(ctx, "handler.create_transaction.get_balances")

	balancesBefore, balancesAfter, err := handler.Query.GetBalances(ctx, organizationID, ledgerID, transactionID, &transactionInput, validate, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanGetBalances, "Failed to get balances", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get balances: %v", err.Error()))

		_ = handler.Command.RedisRepo.Del(ctx, *internalKey)

		handler.Command.RemoveTransactionFromRedisQueue(ctx, logger, organizationID, ledgerID, transactionID.String())
		spanGetBalances.End()

		return http.WithError(c, err)
	}

	spanGetBalances.End()

	fromTo = append(fromTo, handler.HandleAccountFields(transactionInput.Send.Source.From, false)...)
	to = handler.HandleAccountFields(transactionInput.Send.Distribute.To, false)

	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, to...)
	}

	var parentTransactionID *string

	if parentID != uuid.Nil {
		str := parentID.String()
		parentTransactionID = &str
	}

	tran := &transaction.Transaction{
		ID:                       transactionID.String(),
		ParentTransactionID:      parentTransactionID,
		OrganizationID:           organizationID.String(),
		LedgerID:                 ledgerID.String(),
		Description:              transactionInput.Description,
		Amount:                   &transactionInput.Send.Value,
		AssetCode:                transactionInput.Send.Asset,
		ChartOfAccountsGroupName: transactionInput.ChartOfAccountsGroupName,
		CreatedAt:                transactionDate,
		UpdatedAt:                time.Now(),
		Route:                    transactionInput.Route,
		Metadata:                 transactionInput.Metadata,
		Status: transaction.Status{
			Code:        transactionStatus,
			Description: &transactionStatus,
		},
	}

	operations, _, err := handler.BuildOperations(ctx, balancesBefore, fromTo, transactionInput, *tran, validate, transactionDate, transactionStatus == constant.NOTED)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to validate balances", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate balance: %v", err.Error()))

		return http.WithError(c, err)
	}

	tran.Source = getAliasWithoutKey(validate.Sources)
	tran.Destination = getAliasWithoutKey(validate.Destinations)
	tran.Operations = operations

	handler.Command.UpdateTransactionBackupOperations(ctx, organizationID, ledgerID, transactionID.String(), operations)

	originalStatus := tran.Status

	if transactionStatus == constant.CREATED {
		approved := constant.APPROVED
		tran.Status = transaction.Status{Code: approved, Description: &approved}
	}

	handler.Command.CreateWriteBehindTransaction(ctx, organizationID, ledgerID, tran, transactionInput)

	err = handler.Command.WriteTransaction(ctx, organizationID, ledgerID, &transactionInput, validate, balancesBefore, balancesAfter, tran)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, "failed to update BTO")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "failed to update BTO", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("failed to update BTO - transaction: %s - Error: %v", tran.ID, err))

		return http.WithError(c, err)
	}

	tran.Status = originalStatus

	go handler.Command.SetValueOnExistingIdempotencyKey(ctx, organizationID, ledgerID, key, hash, *tran, ttl)

	go handler.Command.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, tran.IDtoUUID())

	return http.Created(c, tran)
}

func (handler *TransactionHandler) ApplyDefaultBalanceKeys(entries []pkgTransaction.FromTo) {
	for i := range entries {
		if entries[i].BalanceKey == "" {
			entries[i].BalanceKey = constant.DefaultBalanceKey
		}
	}
}

func getAliasWithoutKey(array []string) []string {
	result := make([]string, len(array))

	for i, str := range array {
		parts := strings.Split(str, "#")
		result[i] = parts[0]
	}

	return result
}
