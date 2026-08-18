// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"os"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"

	// WriteTransaction routes the transaction to sync or async execution
	// based on the RABBITMQ_TRANSACTION_ASYNC environment variable.
	libLog "github.com/LerianStudio/lib-observability/v2/log"
)

func (uc *UseCase) WriteTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionInput *mtransaction.Transaction, validate *mtransaction.Responses, blc []*mmodel.Balance, blcAfter []*mmodel.Balance, tran *transaction.Transaction, attempts ...*mmodel.BalanceExecutionAttempt) (err error) {
	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "create_transaction", start, err)
	}()

	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		return uc.WriteTransactionAsync(ctx, organizationID, ledgerID, transactionInput, validate, blc, blcAfter, tran, attempts...)
	}

	return uc.WriteTransactionSync(ctx, organizationID, ledgerID, transactionInput, validate, blc, blcAfter, tran, attempts...)
}

// WriteTransactionAsync publishes the transaction payload to RabbitMQ
// for asynchronous processing. Falls back to direct DB write if queue fails.
func (uc *UseCase) WriteTransactionAsync(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionInput *mtransaction.Transaction, validate *mtransaction.Responses, blc []*mmodel.Balance, blcAfter []*mmodel.Balance, tran *transaction.Transaction, attempts ...*mmodel.BalanceExecutionAttempt) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.write_transaction_async")
	defer span.End()

	queueData := make([]mmodel.QueueData, 0, 1)

	effectMode, eventBalances, eventBalancesAfter := transactionEventEffect(tran, blc, blcAfter)
	value := transaction.TransactionProcessingPayload{
		Validate:              validate,
		Balances:              eventBalances,
		BalancesAfter:         eventBalancesAfter,
		Transaction:           tran,
		Input:                 transactionInput,
		Version:               "v2",
		EffectModeVersion:     mmodel.TransactionEffectModeVersion,
		EffectMode:            effectMode,
		OperationTypeOverride: transactionOperationTypeOverride(transactionInput),
		RevertRolloutMode:     tran.RevertRolloutMode,
		RevertRolloutToken:    tran.RevertRolloutToken,
		RedisGeneration:       tran.RedisGeneration,
	}
	applyExecutionAttemptToPayload(&value, attempts)

	marshal, err := msgpack.Marshal(value)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal transaction to JSON string", err)

		logger.Log(ctx, libLog.LevelError, "Failed to marshal validate to JSON string", libLog.Err(err))

		return err
	}

	queueData = append(queueData, mmodel.QueueData{
		ID:    tran.IDtoUUID(),
		Value: marshal,
	})

	queueMessage := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData:      queueData,
	}

	message, err := msgpack.Marshal(queueMessage)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal exchange message struct", err)

		logger.Log(ctx, libLog.LevelError, "Failed to marshal exchange message struct")

		return err
	}

	// ProducerDefaultWithContext handles scoped timeout internally.
	// If it fails, we fall back to direct DB write using the original context
	// which still has remaining HTTP timeout.
	if _, err := uc.RabbitMQRepo.ProducerDefaultWithContext(
		ctx,
		os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE"),
		os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY"),
		message,
	); err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to send message to queue", libLog.Err(err))

		// Use original context for fallback - it still has remaining HTTP timeout
		err = uc.CreateBalanceTransactionOperationsAsync(ctx, queueMessage)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to send message directly to database", err)

			logger.Log(ctx, libLog.LevelError, "Failed to send message directly to database", libLog.Err(err))

			return err
		}

		return nil
	}

	return nil
}

// WriteTransactionSync performs direct database writes for balance updates,
// transaction record creation, and operation records.
func (uc *UseCase) WriteTransactionSync(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionInput *mtransaction.Transaction, validate *mtransaction.Responses, blc []*mmodel.Balance, blcAfter []*mmodel.Balance, tran *transaction.Transaction, attempts ...*mmodel.BalanceExecutionAttempt) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.write_transaction_sync")
	defer span.End()

	queueData := make([]mmodel.QueueData, 0, 1)

	effectMode, eventBalances, eventBalancesAfter := transactionEventEffect(tran, blc, blcAfter)
	value := transaction.TransactionProcessingPayload{
		Validate:              validate,
		Balances:              eventBalances,
		BalancesAfter:         eventBalancesAfter,
		Transaction:           tran,
		Input:                 transactionInput,
		Version:               "v2",
		EffectModeVersion:     mmodel.TransactionEffectModeVersion,
		EffectMode:            effectMode,
		OperationTypeOverride: transactionOperationTypeOverride(transactionInput),
		RevertRolloutMode:     tran.RevertRolloutMode,
		RevertRolloutToken:    tran.RevertRolloutToken,
		RedisGeneration:       tran.RedisGeneration,
	}
	applyExecutionAttemptToPayload(&value, attempts)

	marshal, err := msgpack.Marshal(value)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal transaction to JSON string", err)

		logger.Log(ctx, libLog.LevelError, "Failed to marshal validate to JSON string", libLog.Err(err))

		return err
	}

	queueData = append(queueData, mmodel.QueueData{
		ID:    tran.IDtoUUID(),
		Value: marshal,
	})

	queueMessage := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData:      queueData,
	}

	err = uc.CreateBalanceTransactionOperationsAsync(ctx, queueMessage)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to send message directly to database", err)

		logger.Log(ctx, libLog.LevelError, "Failed to send message directly to database", libLog.Err(err))

		return err
	}

	return nil
}

func transactionEventEffect(
	tran *transaction.Transaction,
	balances, balancesAfter []*mmodel.Balance,
) (mmodel.TransactionEffectMode, []*mmodel.Balance, []*mmodel.Balance) {
	if tran != nil && tran.Status.Code == constant.NOTED {
		return mmodel.TransactionEffectAnnotationOnly, nil, nil
	}

	return mmodel.TransactionEffectBalanceMutation, balances, balancesAfter
}

func transactionOperationTypeOverride(transactionInput *mtransaction.Transaction) string {
	if transactionInput == nil {
		return ""
	}

	return transactionInput.OperationTypeOverride
}

func applyExecutionAttemptToPayload(payload *transaction.TransactionProcessingPayload, attempts []*mmodel.BalanceExecutionAttempt) {
	if payload == nil || len(attempts) == 0 || attempts[0] == nil {
		return
	}

	payload.AttemptOwner = attempts[0].Owner
	payload.ExpectedOutcome = attempts[0].Outcome
}
