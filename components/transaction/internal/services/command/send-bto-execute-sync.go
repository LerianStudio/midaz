package command

import (
	"context"
	"encoding/json"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// SendBTOExecuteSync func that send balances, transaction and operations to a queue to execute sync.
func (uc *UseCase) SendBTOExecuteSync(ctx context.Context, organizationID, ledgerID uuid.UUID, parseDSL *libTransaction.Transaction, validate *libTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) (*transaction.Transaction, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctxSendBTOSync, spanSendBTOQueue := tracer.Start(ctx, "command.send_bto_execute_sync")
	defer spanSendBTOQueue.End()

	queueData := make([]mmodel.QueueData, 0)

	value := transaction.TransactionQueue{
		Validate:    validate,
		Balances:    blc,
		Transaction: tran,
		ParseDSL:    parseDSL,
	}

	marshal, err := json.Marshal(value)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanSendBTOQueue, "Failed to marshal transaction to JSON string", err)

		logger.Fatalf("Failed to marshal validate to JSON string: %s", err.Error())
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

	tran, err = uc.CreateBalanceTransactionOperationsAsync(ctxSendBTOSync, queueMessage)
	if err != nil {
		logger.Errorf("Failed to create balance transaction operations: %v", err)

		return nil, err
	}

	return tran, nil
}
