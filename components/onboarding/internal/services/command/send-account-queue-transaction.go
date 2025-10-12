package command

import (
	"context"
	"encoding/json"
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// SendAccountQueueTransaction publishes an account creation event to RabbitMQ for the transaction service.
//
// This function implements the event-driven architecture pattern, publishing account creation events
// from the onboarding service to the transaction service. This decouples the two services and enables
// the transaction service to initialize balance tracking structures for newly created accounts.
//
// The message is published to RabbitMQ using the exchange and routing key configured in environment
// variables (RABBITMQ_EXCHANGE and RABBITMQ_KEY). The transaction service consumes these messages
// and creates the necessary balance records in Redis and PostgreSQL.
//
// Flow:
// 1. Serialize account data to JSON
// 2. Wrap in a queue message structure with context (org, ledger, account IDs)
// 3. Publish to configured RabbitMQ exchange/routing key
// 4. Transaction service consumer processes the message asynchronously
//
// Note: This function uses Fatalf on errors, which terminates the process. This is intentional
// as failure to publish account creation events would leave the system in an inconsistent state
// where accounts exist in onboarding but have no balance tracking in the transaction service.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning the account
//   - ledgerID: The UUID of the ledger containing the account
//   - account: The complete account entity to be published
func (uc *UseCase) SendAccountQueueTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, account mmodel.Account) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxLogTransaction, spanLogTransaction := tracer.Start(ctx, "command.send_account_queue_transaction")
	defer spanLogTransaction.End()

	queueData := make([]mmodel.QueueData, 0)

	// Step 1: Serialize the account to JSON for transmission
	marshal, err := json.Marshal(account)
	if err != nil {
		logger.Fatalf("Failed to marshal account to JSON string: %s", err.Error())
	}

	// Step 2: Wrap account JSON in queue data structure
	queueData = append(queueData, mmodel.QueueData{
		ID:    uuid.MustParse(account.ID),
		Value: marshal,
	})

	// Step 3: Construct queue message with organizational context
	queueMessage := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		AccountID:      account.IDtoUUID(),
		QueueData:      queueData,
	}

	// Step 4: Publish to RabbitMQ exchange/routing key from environment config.
	// Failure here is fatal as it leaves system in inconsistent state.
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
