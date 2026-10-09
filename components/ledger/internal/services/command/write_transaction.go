// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// WriteTransaction persists an annotation before the caller answers. It writes
// directly to the database whatever RABBITMQ_TRANSACTION_ASYNC says: the legacy
// queue publish has no broker confirmation, so a successful publish would not
// prove the annotation is durable.
func (uc *UseCase) WriteTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionInput *mtransaction.Transaction, validate *mtransaction.Responses, blc []*mmodel.Balance, blcAfter []*mmodel.Balance, tran *transaction.Transaction) (err error) {
	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "create_transaction", start, err)
	}()

	return uc.WriteTransactionSync(ctx, organizationID, ledgerID, transactionInput, validate, blc, blcAfter, tran)
}

// WriteTransactionSync performs direct database writes for balance updates,
// transaction record creation, and operation records.
func (uc *UseCase) WriteTransactionSync(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionInput *mtransaction.Transaction, validate *mtransaction.Responses, blc []*mmodel.Balance, blcAfter []*mmodel.Balance, tran *transaction.Transaction) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.write_transaction_sync")
	defer span.End()

	queueData := make([]mmodel.QueueData, 0, 1)

	value := transaction.TransactionProcessingPayload{
		Validate:      validate,
		Balances:      blc,
		BalancesAfter: blcAfter,
		Transaction:   tran,
		Input:         transactionInput,
		Version:       "v2",
	}

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
		recordCommandError(ctx, span, logger, "Failed to send message directly to database", err)

		return err
	}

	return nil
}
