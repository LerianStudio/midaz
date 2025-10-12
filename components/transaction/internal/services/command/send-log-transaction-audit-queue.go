package command

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// SendLogTransactionAuditQueue publishes transaction audit logs to RabbitMQ.
//
// This function sends immutable audit records to a separate audit queue for:
// - Compliance and regulatory reporting
// - Forensic analysis and investigation
// - Long-term audit trail preservation
// - Tamper-evident transaction logging
//
// Each operation (debit/credit) in the transaction is serialized and queued
// individually, providing granular audit trails. The audit system can consume
// these events and store them in append-only audit storage (e.g., TimescaleDB,
// S3, or blockchain-based audit logs).
//
// Note: Uses Fatalf on errors (terminates process). This is intentional as failure
// to publish audit logs could create compliance violations. For production systems
// that can tolerate audit loss, consider downgrading to Errorf.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - operations: List of operations (debits/credits) to audit
//   - organizationID: Organization UUID for audit context
//   - ledgerID: Ledger UUID for audit context
//   - transactionID: Transaction UUID being audited
func (uc *UseCase) SendLogTransactionAuditQueue(ctx context.Context, operations []*operation.Operation, organizationID, ledgerID, transactionID uuid.UUID) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	// Check if audit logging is enabled via environment variable
	if !isAuditLogEnabled() {
		logger.Infof("Audit logging not enabled. AUDIT_LOG_ENABLED='%s'", os.Getenv("AUDIT_LOG_ENABLED"))
		return
	}

	ctxLogTransaction, spanLogTransaction := tracer.Start(ctx, "command.transaction.log_transaction")
	defer spanLogTransaction.End()

	queueData := make([]mmodel.QueueData, 0)

	// Serialize each operation to audit log format
	for _, o := range operations {
		oLog := o.ToLog()

		marshal, err := json.Marshal(oLog)
		if err != nil {
			logger.Fatalf("Failed to marshal operation to JSON string: %s", err.Error())
		}

		queueData = append(queueData, mmodel.QueueData{
			ID:    uuid.MustParse(o.ID),
			Value: marshal,
		})
	}

	// Construct audit queue message
	queueMessage := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		AuditID:        transactionID,
		QueueData:      queueData,
	}

	message, err := json.Marshal(queueMessage)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanLogTransaction, "Failed to marshal exchange message struct", err)

		logger.Errorf("Failed to marshal exchange message struct")
	}

	// Publish to audit queue (fatal on failure for compliance)
	if _, err := uc.RabbitMQRepo.ProducerDefault(
		ctxLogTransaction,
		os.Getenv("RABBITMQ_AUDIT_EXCHANGE"),
		os.Getenv("RABBITMQ_AUDIT_KEY"),
		message,
	); err != nil {
		logger.Fatalf("Failed to send message: %s", err.Error())
	}
}

// isAuditLogEnabled checks if audit logging is enabled via environment configuration.
//
// Returns true unless explicitly set to "false".
func isAuditLogEnabled() bool {
	envValue := strings.ToLower(strings.TrimSpace(os.Getenv("AUDIT_LOG_ENABLED")))
	return envValue != "false"
}
