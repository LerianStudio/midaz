package command

import (
	"context"
	"encoding/json"
	"os"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// SendBTOExecuteAsync func that send balances, transaction and operations to a queue to execute async.
func (uc *UseCase) SendBTOExecuteAsync(ctx context.Context, organizationID, ledgerID uuid.UUID, parseDSL *goldModel.Transaction, validate *goldModel.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a transaction operation telemetry entity
	op := uc.Telemetry.NewTransactionOperation("bto_async_send", tran.ID)

	// Add important attributes
	op.WithAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("asset_code", tran.AssetCode),
		attribute.Int("balance_count", len(blc)),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

	queueData := make([]mmodel.QueueData, 0)

	value := transaction.TransactionQueue{
		Validate:    validate,
		Balances:    blc,
		Transaction: tran,
		ParseDSL:    parseDSL,
	}

	marshal, err := json.Marshal(value)
	if err != nil {
		// Record error
		op.RecordError(ctx, "bto_marshal_error", err)
		op.End(ctx, "failed")

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

	// Add queue info to telemetry
	op.WithAttributes(
		attribute.String("exchange", exchange),
		attribute.String("routing_key", routingKey),
	)

	if _, err := uc.RabbitMQRepo.ProducerDefault(
		ctx,
		exchange,
		routingKey,
		queueMessage,
	); err != nil {
		// Record error
		op.RecordError(ctx, "bto_queue_send_error", err)
		op.End(ctx, "failed")

		logger.Errorf("Failed to send message: %s", err.Error())

		return
	}

	// Record business metrics
	if tran.Amount != nil {
		op.RecordBusinessMetric(ctx, "amount", float64(*tran.Amount))
	}

	// Mark operation as successful
	op.End(ctx, "success")
}
