// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for sending an account to the transaction queue.
package command

import (
	"context"
	"encoding/json"
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// SendAccountQueueTransaction sends an account creation event to RabbitMQ.
//
// This function publishes an event to a RabbitMQ queue, allowing the transaction
// service to asynchronously process the new account and initialize its balance.
//
// The message contains the organization, ledger, and account IDs, along with the
// serialized account data.
//
// Parameters:
//   - ctx: The context for tracing and logging.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - account: The account data to be sent in the message.
//
// FIXME: This function uses logger.Fatalf, which will terminate the application
// if marshaling the account to JSON or publishing the message to RabbitMQ fails.
// This is a critical issue and should be refactored to return an error instead.
func (uc *UseCase) SendAccountQueueTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, account mmodel.Account) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxLogTransaction, spanLogTransaction := tracer.Start(ctx, "command.send_account_queue_transaction")
	defer spanLogTransaction.End()

	queueData := make([]mmodel.QueueData, 0)

	marshal, err := json.Marshal(account)
	if err != nil {
		logger.Fatalf("Failed to marshal account to JSON string: %s", err.Error())
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

	if _, err := uc.RabbitMQRepo.ProducerDefault(
		ctxLogTransaction,
		os.Getenv("RABBITMQ_EXCHANGE"),
		os.Getenv("RABBITMQ_KEY"),
		queueMessage,
	); err != nil {
		logger.Fatalf("Failed to send message: %s", err.Error())
	}

	logger.Infof("Account sent to transaction queue successfully")
}
