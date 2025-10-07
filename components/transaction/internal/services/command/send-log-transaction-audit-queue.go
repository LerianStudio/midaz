// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

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
// This method implements audit logging by publishing operation details to a dedicated
// audit queue for compliance, reporting, and forensic analysis. It:
// 1. Checks if audit logging is enabled (AUDIT_LOG_ENABLED)
// 2. Converts operations to log format (ToLog method)
// 3. Marshals operations to JSON
// 4. Publishes to audit exchange
// 5. Uses logger.Fatalf on failures (CRITICAL BUG - will crash server)
//
// Audit Log Contents:
//   - All operations in the transaction
//   - Account IDs and aliases
//   - Amounts and balance changes
//   - Timestamps and metadata
//
// **CRITICAL BUG:**
//   - Uses logger.Fatalf() which crashes the entire application
//   - Should return errors instead of calling os.Exit(1)
//   - See BUGS.md for details
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - operations: List of operations to audit log
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - transactionID: UUID of the transaction being logged
//
// OpenTelemetry: Creates span "command.transaction.log_transaction"
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

// isAuditLogEnabled checks if audit logging is enabled.
//
// Reads AUDIT_LOG_ENABLED environment variable.
// Returns true unless explicitly set to "false".
//
// Returns:
//   - bool: true if enabled (default), false if explicitly disabled
func isAuditLogEnabled() bool {
	envValue := strings.ToLower(strings.TrimSpace(os.Getenv("AUDIT_LOG_ENABLED")))
	return envValue != "false"
}
