// Package command implements write operations (commands) for the onboarding service.
// This file contains command implementation.

package command

import (
	"context"
	"encoding/json"
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// SendAccountQueueTransaction sends an account creation event to RabbitMQ for transaction service processing.
//
// This method publishes account creation events to RabbitMQ so the transaction service can:
//   - Initialize balances for the new account
//   - Set up balance tracking structures
//   - Create cache entries for the account
//   - Enable the account for transaction processing
//
// The message is sent to the configured RabbitMQ exchange and routing key, which routes
// it to the transaction service's queue for async processing.
//
// Message Structure:
//   - OrganizationID: Organization context
//   - LedgerID: Ledger context
//   - AccountID: The created account's ID
//   - QueueData: Array containing serialized account data
//
// Parameters:
//   - ctx: Context for tracing and logging
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - account: The created account to send
//
// Configuration:
//   - RABBITMQ_EXCHANGE: Environment variable for exchange name
//   - RABBITMQ_KEY: Environment variable for routing key
//
// Error Handling:
//   - Uses logger.Fatalf on errors, which terminates the application
//   - This is a CRITICAL BUG - should return error instead
//   - See BUGS.md for details
//
// Example:
//
//	account := &mmodel.Account{...}
//	uc.SendAccountQueueTransaction(ctx, orgID, ledgerID, *account)
//	// Account event is sent to RabbitMQ
//	// Transaction service will process it asynchronously
//
// OpenTelemetry:
//   - Creates span "command.send_account_queue_transaction"
//
// CRITICAL BUG WARNING:
//
//	This function uses logger.Fatalf which will crash the entire application on failure.
//	This should be changed to return errors gracefully. See BUGS.md issue #1.
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
