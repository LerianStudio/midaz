package command

import (
	"context"
	"encoding/json"
	"os"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// SendAccountQueueTransaction sends an account-related transaction message to a RabbitMQ queue for further processing.
// It utilizes context for logger and tracer management and handles data serialization and queue message construction.
func (uc *UseCase) SendAccountQueueTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, account mmodel.Account) {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a new queue operation with telemetry
	queueOpID := "queue-" + account.ID
	op := uc.Telemetry.NewEntityOperation("queue", "send_account_transaction", queueOpID)

	// Add important attributes for telemetry
	op.WithAttributes(
		attribute.String("account_id", account.ID),
		attribute.String("account_type", account.Type),
		attribute.String("account_name", account.Name),
		attribute.String("asset_code", account.AssetCode),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	// Add optional attributes if they exist
	if account.PortfolioID != nil {
		op.WithAttribute("portfolio_id", *account.PortfolioID)
	}

	if account.SegmentID != nil {
		op.WithAttribute("segment_id", *account.SegmentID)
	}

	// Record system metric
	op.RecordSystemicMetric(ctx)

	// Start trace span for this operation
	ctx = op.StartTrace(ctx)

	defer func() {
		// End span will be done by op.End() at the end of the function
	}()

	queueData := make([]mmodel.QueueData, 0)

	marshal, err := json.Marshal(account)
	if err != nil {
		logger.Fatalf("Failed to marshal account to JSON string: %s", err.Error())

		// Record error
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "json_marshaling_error", err)
		op.End(ctx, "error")

		return
	}

	queueData = append(queueData, mmodel.QueueData{
		ID:    uuid.MustParse(account.ID),
		Value: marshal,
	})

	queueMessage := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		AccountID:      account.IDtoUUID(),
		QueueData:      queueData,
	}

	// Add queue information to telemetry
	op.WithAttributes(
		attribute.String("exchange", os.Getenv("RABBITMQ_EXCHANGE")),
		attribute.String("routing_key", os.Getenv("RABBITMQ_KEY")),
	)

	if _, err := uc.RabbitMQRepo.ProducerDefault(
		ctx,
		os.Getenv("RABBITMQ_EXCHANGE"),
		os.Getenv("RABBITMQ_KEY"),
		queueMessage,
	); err != nil {
		logger.Fatalf("Failed to send message: %s", err.Error())

		// Record error
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "rabbitmq_send_error", err)
		op.End(ctx, "error")

		return
	}

	logger.Infof("Account sent to transaction queue successfully")

	// Record successful completion
	op.End(ctx, "success")
}
