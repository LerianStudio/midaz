package command

import (
	"context"
	"encoding/json"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"os"
	"strings"
	"time"
)

const (
	Source    string = "midaz"
	EventType string = "transaction"
)

func (uc *UseCase) SendTransactionEvents(ctx context.Context, tran *transaction.Transaction) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	if !isTransactionEventEnabled() {
		logger.Infof("Transaction event not enabled. RABBITMQ_TRANSACTION_EVENTS_ENABLED='%s'", os.Getenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED"))
		return
	}

	ctxSendTransactionEvents, spanTransactionEvents := tracer.Start(ctx, "command.send_transaction_events_async")
	defer spanTransactionEvents.End()

	payload, err := json.Marshal(tran)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanTransactionEvents, "Failed to marshal transaction to JSON string", err)

		logger.Errorf("Failed to marshal transaction to JSON string: %s", err.Error())
	}

	event := mmodel.Event{
		Source:         Source,
		EventType:      EventType,
		Action:         tran.Status.Code,
		TimeStamp:      time.Now(),
		Version:        os.Getenv("VERSION"),
		OrganizationID: tran.OrganizationID,
		LedgerID:       tran.LedgerID,
		Payload:        payload,
	}

	var key strings.Builder

	key.WriteString(Source)
	key.WriteString(".")
	key.WriteString(EventType)
	key.WriteString(".")
	key.WriteString(tran.Status.Code)

	logger.Infof("Sending transaction events to key: %s", key)

	message, err := json.Marshal(event)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanTransactionEvents, "Failed to marshal exchange message struct", err)

		logger.Errorf("Failed to marshal exchange message struct")
	}

	if _, err := uc.RabbitMQRepo.ProducerDefault(
		ctxSendTransactionEvents,
		os.Getenv("RABBITMQ_TRANSACTION_EVENTS_EXCHANGE"),
		key.String(),
		message,
	); err != nil {
		libOpentelemetry.HandleSpanError(&spanTransactionEvents, "Failed to send transaction events to exchange", err)

		logger.Errorf("Failed to send message: %s", err.Error())
	}
}

func isTransactionEventEnabled() bool {
	envValue := strings.ToLower(strings.TrimSpace(os.Getenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED")))
	return envValue != "false"
}
