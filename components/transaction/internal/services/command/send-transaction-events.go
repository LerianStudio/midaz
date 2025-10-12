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
	// Source identifies the system generating events ("midaz")
	Source string = "midaz"
	// EventType categorizes the event as a transaction event
	EventType string = "transaction"
)

// SendTransactionEvents publishes transaction state change events to RabbitMQ.
//
// This function implements event sourcing patterns, publishing transaction lifecycle
// events (APPROVED, PENDING, CANCELED, NOTED) to downstream consumers. External systems
// can subscribe to these events for:
// - Real-time notifications
// - Webhooks to customer systems
// - Analytics and reporting pipelines
// - Audit logging
// - Replication to data warehouses
//
// Event Structure:
// - source: "midaz"
// - eventType: "transaction"
// - action: Transaction status (APPROVED/PENDING/CANCELED/NOTED)
// - payload: Complete transaction JSON
//
// Routing Key Format: "midaz.transaction.{STATUS}"
// Examples: "midaz.transaction.APPROVED", "midaz.transaction.PENDING"
//
// This runs asynchronously (via goroutine) to avoid blocking transaction processing.
// Failures are logged but don't fail the transaction.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - tran: The transaction entity to publish events for
func (uc *UseCase) SendTransactionEvents(ctx context.Context, tran *transaction.Transaction) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	// Check if transaction events are enabled via environment variable
	if !isTransactionEventEnabled() {
		logger.Infof("Transaction event not enabled. RABBITMQ_TRANSACTION_EVENTS_ENABLED='%s'", os.Getenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED"))
		return
	}

	ctxSendTransactionEvents, spanTransactionEvents := tracer.Start(ctx, "command.send_transaction_events_async")
	defer spanTransactionEvents.End()

	// Serialize transaction to JSON payload
	payload, err := json.Marshal(tran)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanTransactionEvents, "Failed to marshal transaction to JSON string", err)

		logger.Errorf("Failed to marshal transaction to JSON string: %s", err.Error())
	}

	// Construct event envelope with metadata
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

	// Build routing key for RabbitMQ topic exchange: "midaz.transaction.{STATUS}"
	var key strings.Builder

	key.WriteString(Source)
	key.WriteString(".")
	key.WriteString(EventType)
	key.WriteString(".")
	key.WriteString(tran.Status.Code)

	logger.Infof("Sending transaction events to key: %s", key)

	// Serialize event envelope
	message, err := json.Marshal(event)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanTransactionEvents, "Failed to marshal exchange message struct", err)

		logger.Errorf("Failed to marshal exchange message struct")
	}

	// Publish to RabbitMQ exchange (non-fatal if it fails)
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
// Returns true unless explicitly set to "false" in environment.
func isTransactionEventEnabled() bool {
	envValue := strings.ToLower(strings.TrimSpace(os.Getenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED")))
	return envValue != "false"
}
