package commands

import (
	"context"
	"encoding/json"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/consumer/internal/adapters/postgresql/transaction"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"os"
	"strings"
	"time"
)

const (
	Source                          string = "midaz"
	EventType                       string = "transaction"
	RabbitTransactionEventsExchange string = "RABBITMQ_TRANSACTION_EVENTS_EXCHANGE"
	RabbitTransactionEventsEnabled  string = "RABBITMQ_TRANSACTION_EVENTS_ENABLED"
)

func (uc *UseCase) SendTransactionEvents(ctx context.Context, tran *transaction.Transaction) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	if !isTransactionEventEnabled() {
		logger.Infof("Transaction event not enabled. value='%s'", os.Getenv(RabbitTransactionEventsEnabled))

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
		TimeStamp:      time.Now().String(),
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
		os.Getenv(RabbitTransactionEventsExchange),
		key.String(),
		message,
	); err != nil {
		libOpentelemetry.HandleSpanError(&spanTransactionEvents, "Failed to send transaction events to exchange", err)

		logger.Errorf("Failed to send message: %s", err.Error())
	}
}

func isTransactionEventEnabled() bool {
	envValue := strings.ToLower(strings.TrimSpace(os.Getenv(RabbitTransactionEventsEnabled)))
	return envValue != "false"
}
