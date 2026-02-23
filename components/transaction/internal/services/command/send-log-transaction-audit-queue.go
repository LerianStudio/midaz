// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// SendLogTransactionAuditQueue sends transaction audit log data to a message queue for processing and storage.
// ctx is the request-scoped context for cancellation and deadlines.
// operations is the list of operations to be logged in the audit queue.
// organizationID is the UUID of the associated organization.
// ledgerID is the UUID of the ledger linked to the transaction.
// transactionID is the UUID of the transaction being logged.
func (uc *UseCase) SendLogTransactionAuditQueue(ctx context.Context, operations []*operation.Operation, organizationID, ledgerID, transactionID uuid.UUID) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	if !uc.AuditLogEnabled {
		logger.Info("Audit logging is disabled")
		return
	}

	if uc.BrokerRepo == nil {
		logger.Errorf("Failed to send audit message: broker repository is not configured")
		return
	}

	ctxLogTransaction, spanLogTransaction := tracer.Start(ctx, "command.transaction.log_transaction")
	defer spanLogTransaction.End()

	queueData := make([]mmodel.QueueData, 0)

	for i, o := range operations {
		if o == nil {
			logger.Errorf("Invalid audit operation payload: nil operation at index %d", i)
			return
		}

		oLog := o.ToLog()

		marshal, err := json.Marshal(oLog)
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanLogTransaction, "Failed to marshal operation to JSON string", err)
			logger.Errorf("Failed to marshal operation to JSON string: %s", err.Error())
			return
		}

		opID, err := uuid.Parse(o.ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanLogTransaction, "Invalid operation UUID for audit payload", err)
			logger.Errorf("Invalid operation UUID for audit payload: %s", err.Error())
			return
		}

		queueData = append(queueData, mmodel.QueueData{
			ID:    opID,
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
		libOpentelemetry.HandleSpanError(&spanLogTransaction, "Failed to marshal audit message struct", err)

		logger.Errorf("Failed to marshal audit message struct")

		return
	}

	if _, err := uc.BrokerRepo.ProducerDefault(
		ctxLogTransaction,
		uc.AuditTopic,
		transactionID.String(),
		message,
	); err != nil {
		logger.Errorf("Failed to send audit message: %s", err.Error())
	}
}
