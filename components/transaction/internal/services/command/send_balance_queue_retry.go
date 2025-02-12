package command

import (
	"context"
	"encoding/json"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"os"
)

// SendBalanceQueueRetry func that send balances to queue when try to update and database isn't working.
func (uc *UseCase) SendBalanceQueueRetry(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctxBalanceFifo, spanBalanceFifo := tracer.Start(ctx, "command.send_balance_queue_retry")
	defer spanBalanceFifo.End()

	queueData := make([]mmodel.QueueData, 0)

	for _, b := range balances {
		marshal, err := json.Marshal(b)
		if err != nil {
			mopentelemetry.HandleSpanError(&spanBalanceFifo, "Failed to marshal balances to JSON string", err)

			logger.Fatalf("Failed to marshal balances to JSON string: %s", err.Error())

			return err
		}

		queueData = append(queueData, mmodel.QueueData{
			ID:    uuid.MustParse(b.ID),
			Value: marshal,
		})

	}

	queueMessage := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData:      queueData,
	}

	if _, err := uc.RabbitMQRepo.ProducerDefault(
		ctxBalanceFifo,
		os.Getenv("RABBITMQ_BALANCE_RETRY_EXCHANGE"),
		os.Getenv("RABBITMQ_BALANCE_RETRY_KEY"),
		queueMessage,
	); err != nil {
		mopentelemetry.HandleSpanError(&spanBalanceFifo, "Failed to send balances to queue", err)

		logger.Fatalf("Failed to send message: %s", err.Error())

		return err
	}

	spanBalanceFifo.End()

	return nil
}
