// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"

	// SendLogTransactionAuditQueue sends transaction audit log data to a message queue for processing and storage.
	// ctx is the request-scoped context for cancellation and deadlines.
	// operations is the list of operations to be logged in the audit queue.
	// organizationID is the UUID of the associated organization.
	// ledgerID is the UUID of the ledger linked to the transaction.
	// transactionID is the UUID of the transaction being logged.
	libLog "github.com/LerianStudio/lib-observability/log"
)

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

func isAuditLogEnabled() bool {
	envValue := strings.ToLower(strings.TrimSpace(os.Getenv("AUDIT_LOG_ENABLED")))
	return envValue != "false"
}
