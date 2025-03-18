package command

import (
	"context"
	"encoding/json"
	"time"

	"os"
	"strings"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
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
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctxLogTransaction, spanLogTransaction := tracer.Start(ctx, "command.transaction.log_transaction")
	defer spanLogTransaction.End()

	// Record operation metrics
	uc.recordBusinessMetrics(ctx, "transaction_audit_log_attempt",
		attribute.String("transaction_id", transactionID.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.Int("operation_count", len(operations)))

	if !isAuditLogEnabled() {
		logger.Infof("Audit logging not enabled. AUDIT_LOG_ENABLED='%s'", os.Getenv("AUDIT_LOG_ENABLED"))

		// Record skipped metrics
		uc.recordBusinessMetrics(ctx, "transaction_audit_log_skipped",
			attribute.String("transaction_id", transactionID.String()),
			attribute.String("reason", "disabled_in_config"))

		// Record duration with skipped status
		uc.recordTransactionDuration(ctx, startTime, "transaction_audit_log", "skipped",
			attribute.String("transaction_id", transactionID.String()),
			attribute.String("reason", "disabled_in_config"))

		return
	}

	queueData := make([]mmodel.QueueData, 0)
	marshalErrorCount := 0

	for _, o := range operations {
		oLog := o.ToLog()

		marshal, err := json.Marshal(oLog)
		if err != nil {
			// Record error
			uc.recordTransactionError(ctx, "audit_marshal_error",
				attribute.String("transaction_id", transactionID.String()),
				attribute.String("operation_id", o.ID),
				attribute.String("error_detail", err.Error()))

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
		// Record error
		uc.recordTransactionError(ctx, "audit_all_operations_marshal_error",
			attribute.String("transaction_id", transactionID.String()),
			attribute.Int("failed_operations", marshalErrorCount))

		// Record duration with error status
		uc.recordTransactionDuration(ctx, startTime, "transaction_audit_log", "error",
			attribute.String("transaction_id", transactionID.String()),
			attribute.String("error", "all_operations_marshal_failed"))

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

	if _, err := uc.RabbitMQRepo.ProducerDefault(
		ctxLogTransaction,
		exchange,
		routingKey,
		queueMessage,
	); err != nil {
		// Record error
		uc.recordTransactionError(ctx, "audit_queue_send_error",
			attribute.String("transaction_id", transactionID.String()),
			attribute.String("exchange", exchange),
			attribute.String("routing_key", routingKey),
			attribute.String("error_detail", err.Error()))

		// Record duration with error status
		uc.recordTransactionDuration(ctx, startTime, "transaction_audit_log", "error",
			attribute.String("transaction_id", transactionID.String()),
			attribute.String("error", "queue_send_error"))

		logger.Errorf("Failed to send audit message: %s", err.Error())
		mopentelemetry.HandleSpanError(&spanLogTransaction, "Failed to send audit message to queue", err)
		return
	}

	// Record partial success if some operations failed to marshal
	if marshalErrorCount > 0 {
		// Record partial success metrics
		uc.recordBusinessMetrics(ctx, "transaction_audit_log_partial",
			attribute.String("transaction_id", transactionID.String()),
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()),
			attribute.Int("successful_operations", len(queueData)),
			attribute.Int("failed_operations", marshalErrorCount))

		// Record duration with partial status
		uc.recordTransactionDuration(ctx, startTime, "transaction_audit_log", "partial",
			attribute.String("transaction_id", transactionID.String()),
			attribute.Int("successful_operations", len(queueData)),
			attribute.Int("failed_operations", marshalErrorCount))
	} else {
		// Record success metrics
		uc.recordBusinessMetrics(ctx, "transaction_audit_log_success",
			attribute.String("transaction_id", transactionID.String()),
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()),
			attribute.Int("operation_count", len(operations)))

		// Record duration with success status
		uc.recordTransactionDuration(ctx, startTime, "transaction_audit_log", "success",
			attribute.String("transaction_id", transactionID.String()),
			attribute.Int("operation_count", len(operations)))
	}
}

func isAuditLogEnabled() bool {
	envValue := strings.ToLower(strings.TrimSpace(os.Getenv("AUDIT_LOG_ENABLED")))
	return envValue != "false"
}
