package command

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// SendLogTransactionAuditQueue sends transaction audit log data to a message queue for processing and storage.
// ctx is the request-scoped context for cancellation and deadlines.
// operations is the list of operations to be logged in the audit queue.
// organizationID is the UUID of the associated organization.
// ledgerID is the UUID of the ledger linked to the transaction.
// transactionID is the UUID of the transaction being logged.
func (uc *UseCase) SendLogTransactionAuditQueue(ctx context.Context, operations []*operation.Operation, organizationID, ledgerID, transactionID uuid.UUID) {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a transaction audit log operation telemetry entity
	op := uc.Telemetry.NewTransactionOperation("transaction_audit_log", transactionID.String())

	// Add important attributes
	op.WithAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.Int("operation_count", len(operations)),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

	if !isAuditLogEnabled() {
		logger.Infof("Audit logging not enabled. AUDIT_LOG_ENABLED='%s'", os.Getenv("AUDIT_LOG_ENABLED"))

		// Add skipped reason to telemetry
		op.WithAttribute("reason", "disabled_in_config")
		op.End(ctx, "skipped")

		return
	}

	queueData := make([]mmodel.QueueData, 0)
	marshalErrorCount := 0

	for _, o := range operations {
		oLog := o.ToLog()

		marshal, err := json.Marshal(oLog)
		if err != nil {
			// Record error for marshal but continue with other operations
			op.RecordError(ctx, "audit_marshal_error", err)
			op.WithAttribute("operation_id", o.ID)

			marshalErrorCount++

			logger.Errorf("Failed to marshal operation to JSON string: %s", err.Error())

			continue // Continue with other operations rather than failing entirely
		}

		queueData = append(queueData, mmodel.QueueData{
			ID:    uuid.MustParse(o.ID),
			Value: marshal,
		})
	}

	// If all operations failed to marshal, record total failure
	if len(operations) > 0 && len(queueData) == 0 {
		// Update error metrics
		op.WithAttributes(
			attribute.Int("failed_operations", marshalErrorCount),
		)
		op.End(ctx, "failed")

		logger.Errorf("All operations failed to marshal for transaction: %s", transactionID.String())

		return
	}

	queueMessage := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		AuditID:        transactionID,
		QueueData:      queueData,
	}

	exchange := os.Getenv("RABBITMQ_AUDIT_EXCHANGE")
	routingKey := os.Getenv("RABBITMQ_AUDIT_KEY")

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
		// Record queue send error
		op.RecordError(ctx, "audit_queue_send_error", err)
		op.End(ctx, "failed")

		logger.Errorf("Failed to send audit message: %s", err.Error())

		return
	}

	// Add success/partial metrics and end the operation
	if marshalErrorCount > 0 {
		// Update with partial success metrics
		op.WithAttributes(
			attribute.Int("successful_operations", len(queueData)),
			attribute.Int("failed_operations", marshalErrorCount),
		)
		op.End(ctx, "partial_success")
	} else {
		// Complete success
		op.End(ctx, "success")
	}
}

func isAuditLogEnabled() bool {
	envValue := strings.ToLower(strings.TrimSpace(os.Getenv("AUDIT_LOG_ENABLED")))
	return envValue != "false"
}
