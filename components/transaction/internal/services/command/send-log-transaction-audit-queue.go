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

// SendLogTransactionAuditQueue sends transaction audit data to RabbitMQ for compliance logging.
//
// Financial regulations require comprehensive audit trails for all transactions.
// This function publishes operation-level details to an audit queue, where they're
// consumed by audit services for long-term storage and compliance reporting.
//
// Audit Data Structure:
//
// Each operation is converted to an audit log format containing:
//   - Operation ID and type (debit/credit)
//   - Account and balance information
//   - Amount and asset details
//   - Timestamps and organization context
//
// Publishing Process:
//
//	Step 1: Check Configuration
//	  - Return early if AUDIT_LOG_ENABLED=false
//	  - Default is enabled (any value except "false")
//
//	Step 2: Build Audit Queue Message
//	  - Convert each operation to audit log format
//	  - Serialize operations as JSON
//	  - Package into queue message with context
//
//	Step 3: Publish to RabbitMQ
//	  - Send to configured audit exchange
//	  - Use transaction ID for message correlation
//
// Compliance Requirements:
//
// This audit trail supports:
//   - SOX compliance (Sarbanes-Oxley)
//   - PCI-DSS requirements for financial data
//   - General ledger reconciliation
//   - Fraud investigation and forensics
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - operations: Slice of operations to audit (all from same transaction)
//   - organizationID: Organization scope for audit records
//   - ledgerID: Ledger scope for audit records
//   - transactionID: Transaction ID for audit correlation
//
// Note: This function does not return errors. Audit failures are logged but
// do not affect transaction processing. Audit data can be recovered from
// the database if queue publishing fails.
//
// Environment Variables:
//
//	AUDIT_LOG_ENABLED=true|false (default: true)
//	RABBITMQ_AUDIT_EXCHANGE: Exchange name for audit messages
//	RABBITMQ_AUDIT_KEY: Routing key for audit messages
func (uc *UseCase) SendLogTransactionAuditQueue(ctx context.Context, operations []*operation.Operation, organizationID, ledgerID, transactionID uuid.UUID) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	if !isAuditLogEnabled() {
		logger.Infof("Audit logging not enabled. AUDIT_LOG_ENABLED='%s'", os.Getenv("AUDIT_LOG_ENABLED"))
		return
	}

	ctxLogTransaction, spanLogTransaction := tracer.Start(ctx, "command.transaction.log_transaction")
	defer spanLogTransaction.End()

	queueData := make([]mmodel.QueueData, 0)

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

	if _, err := uc.RabbitMQRepo.ProducerDefault(
		ctxLogTransaction,
		os.Getenv("RABBITMQ_AUDIT_EXCHANGE"),
		os.Getenv("RABBITMQ_AUDIT_KEY"),
		message,
	); err != nil {
		logger.Fatalf("Failed to send message: %s", err.Error())
	}
}

// isAuditLogEnabled checks if audit logging is enabled via environment variable.
//
// Returns true unless AUDIT_LOG_ENABLED is explicitly set to "false".
// This default-enabled behavior ensures audit trails are captured unless
// explicitly disabled (e.g., in development environments).
func isAuditLogEnabled() bool {
	envValue := strings.ToLower(strings.TrimSpace(os.Getenv("AUDIT_LOG_ENABLED")))
	return envValue != "false"
}
