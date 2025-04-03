package command

import (
	"context"
	"encoding/json"
	"os"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// SendAccountQueueTransaction sends an account-related transaction message to a RabbitMQ queue for further processing.
// It utilizes context for logger and tracer management and handles data serialization and queue message construction.
func (uc *UseCase) SendAccountQueueTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, account mmodel.Account) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

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
