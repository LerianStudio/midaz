package rabbitmq

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	attemptDisplayOffset = 2

	// publishConfirmTimeout defines how long the producer waits for broker
	// acknowledgment before considering a publish operation failed.
	// Set to 10s to allow temporary broker unavailability during chaos
	// scenarios while not blocking excessively during normal operations.
	// This value is critical for ensuring data consistency - without publisher
	// confirms, messages may be lost if the broker fails between accepting
	// the message and persisting it.
	publishConfirmTimeout = 10 * time.Second
)

// Static errors for publisher confirm scenarios.
// These provide consistent error types for error handling and testing.
var (
	// ErrNilConnection indicates the RabbitMQ connection is nil.
	ErrNilConnection = errors.New("rabbitmq connection is nil")

	// ErrConfirmChannelClosed indicates the confirmation channel was closed unexpectedly.
	ErrConfirmChannelClosed = errors.New("confirmation channel closed unexpectedly")

	// ErrBrokerNack indicates the broker rejected the message with a NACK.
	ErrBrokerNack = errors.New("broker returned NACK")

	// ErrConfirmTimeout indicates the publish confirmation timed out.
	ErrConfirmTimeout = errors.New("publish confirmation timed out")

	// ErrDeliveryTagReceived indicates a delivery tag was received with a broker NACK.
	ErrDeliveryTagReceived = errors.New("delivery tag received")
)

// ProducerRepository provides an interface for Producer related to rabbitmq.
// // It defines methods for sending messages to a queue.
type ProducerRepository interface {
	ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error)
	CheckRabbitMQHealth() bool
}

// ProducerRabbitMQRepository is a rabbitmq implementation of the producer
type ProducerRabbitMQRepository struct {
	conn *libRabbitmq.RabbitMQConnection
}

// NewProducerRabbitMQ returns a new instance of ProducerRabbitMQRepository using the given rabbitmq connection.
func NewProducerRabbitMQ(c *libRabbitmq.RabbitMQConnection) *ProducerRabbitMQRepository {
	assert.NotNil(c, "RabbitMQ connection must not be nil", "component", "TransactionProducer")

	conn, err := c.GetNewConnect()
	assert.NoError(err, "RabbitMQ connection required for TransactionProducer",
		"component", "TransactionProducer")
	assert.NotNil(conn, "RabbitMQ connection handle must not be nil", "component", "TransactionProducer")

	return &ProducerRabbitMQRepository{
		conn: c,
	}
}

// CheckRabbitMQHealth checks the health of the rabbitmq connection.
func (prmq *ProducerRabbitMQRepository) CheckRabbitMQHealth() bool {
	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "false" {
		return true
	}

	return prmq.conn.HealthCheck()
}

// isLastAttempt checks if the current attempt is the last retry attempt.
func isLastAttempt(attempt int) bool {
	return attempt == utils.MaxRetries
}

// ProducerDefault sends a message to a RabbitMQ queue for further processing.
// It uses publisher confirms to guarantee the message was persisted by the broker
// before returning success. This ensures HTTP 201 responses are only sent after
// the broker has acknowledged message persistence, preventing message loss during
// infrastructure failures.
//
// NOTE: On confirmation timeout, the message may have already been persisted by the broker.
// Retries may result in duplicate messages (at-least-once delivery semantic).
// Consumer-side idempotency (via unique constraints) handles this gracefully.
//
// ARCHITECTURE: Each call creates a dedicated channel (per-request pattern) to avoid
// contention when 100+ goroutines publish concurrently. The underlying connection is
// shared, but channels are isolated per request for thread safety.
//
//nolint:cyclop // Complexity is inherent to retry logic with context checks and backoff
func (prmq *ProducerRabbitMQRepository) ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	logger.Infof("Init sent message to exchange: %s, key: %s", exchange, key)

	ctx, spanProducer := tracer.Start(ctx, "rabbitmq.producer.publish_message")
	defer spanProducer.End()

	// Check for nil connection before attempting any operations
	if prmq.conn == nil {
		libOpentelemetry.HandleSpanError(&spanProducer, "RabbitMQ connection is nil", ErrNilConnection)
		logger.Errorf("Cannot publish message: %v", ErrNilConnection)

		return nil, pkg.ValidateInternalError(ErrNilConnection, "Producer")
	}

	// Check context cancellation before attempting channel setup
	select {
	case <-ctx.Done():
		err := ctx.Err()
		libOpentelemetry.HandleSpanError(&spanProducer, "Context cancelled before channel setup", err)
		logger.Warnf("Publish cancelled by context before channel setup: %v", err)

		return nil, pkg.ValidateInternalError(err, "Producer")
	default:
		// Continue with channel setup
	}

	// Ensure the underlying connection is healthy before creating per-request channels.
	// EnsureChannel() handles connection recovery if the connection was lost.
	// This is called once at the start to validate connection state.
	if err := prmq.conn.EnsureChannel(); err != nil {
		libOpentelemetry.HandleSpanError(&spanProducer, "Failed to ensure connection is healthy", err)
		logger.Errorf("Cannot publish message: connection unhealthy: %v", err)

		return nil, pkg.ValidateInternalError(err, "Producer")
	}

	// Verify the underlying connection is available for creating per-request channels
	if prmq.conn.Connection == nil {
		libOpentelemetry.HandleSpanError(&spanProducer, "RabbitMQ underlying connection is nil", ErrNilConnection)
		logger.Errorf("Cannot publish message: underlying connection is nil")

		return nil, pkg.ValidateInternalError(ErrNilConnection, "Producer")
	}

	var lastErr error

	backoff := utils.InitialBackoff

	headers := amqp.Table{
		libConstants.HeaderID: reqId,
	}

	libOpentelemetry.InjectTraceHeadersIntoQueue(ctx, (*map[string]any)(&headers))

	for attempt := 0; attempt <= utils.MaxRetries; attempt++ {
		// Check context cancellation before each attempt
		select {
		case <-ctx.Done():
			err := ctx.Err()
			libOpentelemetry.HandleSpanError(&spanProducer, "Context cancelled during publish", err)
			logger.Warnf("Publish cancelled by context: %v", err)

			return nil, pkg.ValidateInternalError(err, "Producer")
		default:
			// Continue with publish attempt
		}

		// Create a dedicated channel for this publish operation (per-request pattern).
		// This avoids contention when 100+ concurrent goroutines publish simultaneously.
		// Each goroutine gets its own channel, preventing race conditions on Confirm/Publish.
		ch, err := prmq.conn.Connection.Channel()
		if err != nil {
			logger.Warnf("Failed to create channel, attempt %d/%d: %v", attempt+1, utils.MaxRetries+1, err)
			lastErr = err

			if isLastAttempt(attempt) {
				libOpentelemetry.HandleSpanError(&spanProducer, "Failed to create channel after retries", err)
				logger.Errorf("Giving up after %d attempts: failed to create channel", utils.MaxRetries+1)

				return nil, pkg.ValidateInternalError(err, "Producer")
			}

			// Try to recover the connection before next attempt
			if reconnErr := prmq.conn.EnsureChannel(); reconnErr != nil {
				logger.Warnf("Connection recovery also failed: %v", reconnErr)
			}

			sleepDuration := utils.FullJitter(backoff)
			logger.Infof("Retrying channel creation in %v...", sleepDuration)
			time.Sleep(sleepDuration)

			backoff = utils.NextBackoff(backoff)

			continue
		}

		// Try to publish with this channel
		isFinal, publishErr := prmq.publishWithChannel(ctx, ch, exchange, key, headers, message)
		if publishErr == nil {
			// Success - message confirmed by broker
			logger.Infof("Message confirmed by broker to exchange: %s, key: %s", exchange, key)
			return nil, nil
		}

		lastErr = publishErr

		// Check if this is a final error (like context cancelled) that shouldn't be retried
		if isFinal {
			libOpentelemetry.HandleSpanError(&spanProducer, "Publish failed with final error", publishErr)
			logger.Errorf("Publish failed (no retry): %v", publishErr)

			return nil, pkg.ValidateInternalError(publishErr, "Producer")
		}

		// Log the retriable error
		logger.Warnf("Publish attempt %d/%d failed: %v", attempt+1, utils.MaxRetries+1, publishErr)

		if isLastAttempt(attempt) {
			libOpentelemetry.HandleSpanError(&spanProducer, "Publish failed after all retries", publishErr)
			logger.Errorf("Giving up after %d attempts: %v", utils.MaxRetries+1, publishErr)

			return nil, pkg.ValidateInternalError(publishErr, "Producer")
		}

		// Wait before retry with exponential backoff and jitter
		sleepDuration := utils.FullJitter(backoff)
		logger.Infof("Retrying publish in %v (attempt %d)...", sleepDuration, attempt+attemptDisplayOffset)
		time.Sleep(sleepDuration)

		backoff = utils.NextBackoff(backoff)
	}

	return nil, pkg.ValidateInternalError(lastErr, "Producer")
}

// publishWithChannel performs a single publish attempt on the given channel.
// It always closes the channel before returning, regardless of success or failure.
// Returns (true, nil) on success, (true, err) for final errors that shouldn't be retried,
// or (false, err) for retriable errors.
//
//nolint:wrapcheck // Internal helper - caller wraps errors with pkg.ValidateInternalError
func (prmq *ProducerRabbitMQRepository) publishWithChannel(
	ctx context.Context,
	ch *amqp.Channel,
	exchange, key string,
	headers amqp.Table,
	message []byte,
) (isFinal bool, err error) {
	// Always close the channel when done - each request gets a fresh channel.
	// Errors on close are intentionally ignored as the channel is being discarded anyway.
	defer func() { _ = ch.Close() }()

	// Enable publisher confirm mode on this channel
	if err := ch.Confirm(false); err != nil {
		return false, err
	}

	// Create a channel to receive publish confirmations
	// Buffer size of 1 is sufficient since we wait for each confirmation
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	// Publish the message
	if err := ch.Publish(
		exchange,
		key,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Headers:      headers,
			Body:         message,
		},
	); err != nil {
		return false, err
	}

	// Wait for broker confirmation with timeout
	// This is the critical section that ensures message persistence
	select {
	case confirmation, ok := <-confirms:
		if !ok {
			// Channel was closed - connection issue
			return false, ErrConfirmChannelClosed
		}

		if confirmation.Ack {
			// SUCCESS: Broker confirmed message persistence
			return true, nil
		}

		// NACK: Broker rejected the message
		return false, ErrBrokerNack

	case <-time.After(publishConfirmTimeout):
		// TIMEOUT: No confirmation received within timeout
		return false, ErrConfirmTimeout

	case <-ctx.Done():
		// Context cancelled during confirmation wait - don't retry
		return true, ctx.Err()
	}
}
