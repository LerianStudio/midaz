// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// SendLogTransactionAuditQueue publishes the operations of an applied transaction to the audit exchange.
// Publication is an explicit opt-in: it happens only when AUDIT_LOG_ENABLED, trimmed and lowercased,
// equals "true"; any other value or its absence disables it. Failures are logged and never propagated.
func (uc *UseCase) SendLogTransactionAuditQueue(ctx context.Context, operations []*operation.Operation, organizationID, ledgerID, transactionID uuid.UUID) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	if !isAuditLogEnabled() {
		logger.Log(ctx, libLog.LevelDebug, "Audit logging not enabled",
			libLog.String("audit_log_enabled", os.Getenv("AUDIT_LOG_ENABLED")))

		return
	}

	ctxLogTransaction, spanLogTransaction := tracer.Start(ctx, "command.transaction.log_transaction")
	defer spanLogTransaction.End()

	queueData := make([]mmodel.QueueData, 0)

	for _, o := range operations {
		oLog := o.ToLog()

		marshal, err := json.Marshal(oLog)
		if err != nil {
			libOpentelemetry.HandleSpanError(spanLogTransaction, "Failed to marshal operation to JSON string", err)
			logger.Log(ctxLogTransaction, libLog.LevelError, "Failed to marshal operation to JSON string", libLog.Err(err))

			return
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
		libOpentelemetry.HandleSpanError(spanLogTransaction, "Failed to marshal exchange message struct", err)

		logger.Log(ctx, libLog.LevelError, "Failed to marshal exchange message struct")
	}

	if _, err := uc.RabbitMQRepo.ProducerDefault(
		ctxLogTransaction,
		os.Getenv("RABBITMQ_AUDIT_EXCHANGE"),
		os.Getenv("RABBITMQ_AUDIT_KEY"),
		message,
	); err != nil {
		libOpentelemetry.HandleSpanError(spanLogTransaction, "Failed to send message", err)
		logger.Log(ctxLogTransaction, libLog.LevelError, "Failed to send message", libLog.Err(err))

		return
	}
}

// sendLogTransactionAuditQueueAsync starts the audit publication in a goroutine only when the audit gate is enabled.
func (uc *UseCase) sendLogTransactionAuditQueueAsync(ctx context.Context, operations []*operation.Operation, organizationID, ledgerID, transactionID uuid.UUID) {
	if !isAuditLogEnabled() {
		return
	}

	go uc.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, transactionID)
}

// isAuditLogEnabled reports whether AUDIT_LOG_ENABLED, trimmed and lowercased, equals "true".
// Any other value, including an empty or unset variable, disables audit publication.
func isAuditLogEnabled() bool {
	envValue := strings.ToLower(strings.TrimSpace(os.Getenv("AUDIT_LOG_ENABLED")))
	return envValue == "true"
}
