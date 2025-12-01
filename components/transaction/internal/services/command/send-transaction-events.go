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
	// Source identifies the origin system for transaction events.
	Source string = "midaz"

	// EventType identifies the type of event being published.
	EventType string = "transaction"
)

// SendTransactionEvents publishes transaction lifecycle events to RabbitMQ.
//
// Transaction events enable external systems to react to transaction state changes.
// This supports event-driven architectures where downstream services need to:
//   - Update external accounting systems
//   - Trigger notifications to users
//   - Update analytics and reporting dashboards
//   - Synchronize with partner systems
//
// Event Structure:
//
//	{
//	  "source": "midaz",
//	  "event_type": "transaction",
//	  "action": "APPROVED",      // Transaction status: APPROVED, CANCELED, etc.
//	  "timestamp": "2024-01-15T10:30:00Z",
//	  "version": "1.0.0",
//	  "organization_id": "...",
//	  "ledger_id": "...",
//	  "payload": {...}          // Full transaction data
//	}
//
// Routing Keys:
//
// Events are published with routing keys following the pattern:
//
//	midaz.transaction.{status}
//
// Examples:
//   - midaz.transaction.APPROVED
//   - midaz.transaction.CANCELED
//   - midaz.transaction.PENDING
//
// This allows consumers to subscribe to specific transaction outcomes.
//
// Publishing Process:
//
//	Step 1: Check Configuration
//	  - Return early if RABBITMQ_TRANSACTION_EVENTS_ENABLED=false
//	  - Default is enabled (any value except "false")
//
//	Step 2: Build Event Payload
//	  - Serialize transaction to JSON
//	  - Wrap in event envelope with metadata
//
//	Step 3: Build Routing Key
//	  - Construct key: source.eventType.status
//	  - Example: midaz.transaction.APPROVED
//
//	Step 4: Publish to RabbitMQ
//	  - Send to configured events exchange
//	  - Use constructed routing key
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - tran: Completed transaction to publish as event
//
// Note: This function is called asynchronously (via goroutine) and does not
// return errors. Publishing failures are logged but don't affect the transaction.
//
// Environment Variables:
//
//	RABBITMQ_TRANSACTION_EVENTS_ENABLED=true|false (default: true)
//	RABBITMQ_TRANSACTION_EVENTS_EXCHANGE: Exchange name for events
//	VERSION: Application version for event metadata
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
// Returns true unless RABBITMQ_TRANSACTION_EVENTS_ENABLED is explicitly set to "false".
// This default-enabled behavior ensures events are published unless explicitly disabled.
func isTransactionEventEnabled() bool {
	envValue := strings.ToLower(strings.TrimSpace(os.Getenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED")))
	return envValue != "false"
}
