// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

package command

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

const (
	// Source identifies the origin of transaction events.
	Source string = "midaz"

	// EventType identifies the type of event being published.
	EventType string = "transaction"
)

// SendTransactionEvents publishes transaction status change events to RabbitMQ.
//
// This method implements event-driven architecture by publishing transaction events
// to external consumers (e.g., webhooks, analytics, notifications). It:
// 1. Checks if transaction events are enabled (RABBITMQ_TRANSACTION_EVENTS_ENABLED)
// 2. Marshals transaction to JSON payload
// 3. Creates event envelope with metadata
// 4. Publishes to RabbitMQ exchange with dynamic routing key
// 5. Does not return errors (fire-and-forget pattern)
//
// Event Structure:
//   - source: "midaz"
//   - event_type: "transaction"
//   - action: Transaction status (APPROVED, CANCELED, etc.)
//   - timestamp: Event creation time
//   - version: Service version
//   - organization_id, ledger_id: Identifiers
//   - payload: Full transaction data
//
// Routing Key Format:
//   - "midaz.transaction.{STATUS}" (e.g., "midaz.transaction.APPROVED")
//   - Enables consumers to subscribe to specific transaction statuses
//
// Fire-and-Forget:
//   - Errors are logged but not returned
//   - Transaction processing continues even if event publish fails
//   - Events are best-effort delivery
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - tran: Transaction to publish event for
//
// OpenTelemetry: Creates span "command.send_transaction_events_async"
func (uc *UseCase) SendTransactionEvents(ctx context.Context, tran *transaction.Transaction) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

// isTransactionEventEnabled checks if transaction event publishing is enabled.
//
// Reads RABBITMQ_TRANSACTION_EVENTS_ENABLED environment variable.
// Returns true unless explicitly set to "false".
//
// Returns:
//   - bool: true if enabled (default), false if explicitly disabled
func isTransactionEventEnabled() bool {
	envValue := strings.ToLower(strings.TrimSpace(os.Getenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED")))
	return envValue != "false"
}
