package command

import (
	"context"
	"encoding/json"
	"time"

	"os"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// SendBTOExecuteAsync func that send balances, transaction and operations to a queue to execute async.
func (uc *UseCase) SendBTOExecuteAsync(ctx context.Context, organizationID, ledgerID uuid.UUID, parseDSL *goldModel.Transaction, validate *goldModel.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctxSendBTOQueue, spanSendBTOQueue := tracer.Start(ctx, "command.send_bto_execute_async")
	defer spanSendBTOQueue.End()

	// Record operation metrics
	uc.recordBusinessMetrics(ctx, "bto_async_send_attempt",
		attribute.String("transaction_id", tran.ID),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("asset_code", tran.AssetCode),
		attribute.Int("balance_count", len(blc)))

	queueData := make([]mmodel.QueueData, 0)

	value := transaction.TransactionQueue{
		Validate:    validate,
		Balances:    blc,
		Transaction: tran,
		ParseDSL:    parseDSL,
	}

	marshal, err := json.Marshal(value)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanSendBTOQueue, "Failed to marshal transaction to JSON string", err)

		// Record error
		uc.recordTransactionError(ctx, "bto_marshal_error",
			attribute.String("transaction_id", tran.ID),
			attribute.String("error_detail", err.Error()))

		// Record duration with error status
		uc.recordTransactionDuration(ctx, startTime, "bto_async_send", "error",
			attribute.String("transaction_id", tran.ID),
			attribute.String("error", "marshal_error"))

		logger.Fatalf("Failed to marshal validate to JSON string: %s", err.Error())
		return
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

	exchange := os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE")
	routingKey := os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY")

	if _, err := uc.RabbitMQRepo.ProducerDefault(
		ctxSendBTOQueue,
		exchange,
		routingKey,
		queueMessage,
	); err != nil {
		mopentelemetry.HandleSpanError(&spanSendBTOQueue, "Failed to send BTO to queue", err)

		// Record error
		uc.recordTransactionError(ctx, "bto_queue_send_error",
			attribute.String("transaction_id", tran.ID),
			attribute.String("exchange", exchange),
			attribute.String("routing_key", routingKey),
			attribute.String("error_detail", err.Error()))

		// Record duration with error status
		uc.recordTransactionDuration(ctx, startTime, "bto_async_send", "error",
			attribute.String("transaction_id", tran.ID),
			attribute.String("error", "queue_send_error"))

		logger.Errorf("Failed to send message: %s", err.Error())
		return
	}

	// Record success metrics
	uc.recordBusinessMetrics(ctx, "bto_async_send_success",
		attribute.String("transaction_id", tran.ID),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("asset_code", tran.AssetCode),
		attribute.Int("balance_count", len(blc)))

	// Record duration with success status
	uc.recordTransactionDuration(ctx, startTime, "bto_async_send", "success",
		attribute.String("transaction_id", tran.ID),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()))
}
