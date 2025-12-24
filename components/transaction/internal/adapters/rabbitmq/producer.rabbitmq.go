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

// isChannelError checks if the error indicates a broken channel that needs recreation.
// This is used to avoid recreating channels on every retry attempt, which can overwhelm
// RabbitMQ under burst load (channel thrashing prevention).
func isChannelError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	return strings.Contains(errStr, "channel") ||
		strings.Contains(errStr, "closed") ||
		strings.Contains(errStr, "not open")
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
// channel setup, confirm mode, publish, and confirmation handling (Ack/Nack/Timeout/Context).
// Each path requires distinct handling for proper observability and graceful degradation.
//
//nolint:gocognit,cyclop,gocyclo // Complexity is inherent to retry logic with multiple error scenarios:
func (prmq *ProducerRabbitMQRepository) ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	// TODO(review): Consider validating exchange/key length for defensive logging (reported by security-reviewer on 2025-12-14, severity: Low)
	// TODO(review): The *string return value is always nil - consider deprecating in future cleanup (reported by code-reviewer on 2025-12-14, severity: Low)
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	// TODO(review): Entry log emitted before nil/context checks - minor observability improvement possible (reported by code-reviewer on 2025-12-14, severity: Low)
	logger.Infof("Init sent message to exchange: %s, key: %s", exchange, key)

	ctx, spanProducer := tracer.Start(ctx, "rabbitmq.producer.publish_message")
	defer spanProducer.End()

	// Check for nil connection before attempting any operations
	if prmq.conn == nil {
		libOpentelemetry.HandleSpanError(&spanProducer, "RabbitMQ connection is nil", ErrNilConnection)
		logger.Errorf("Cannot publish message: %v", ErrNilConnection)

		return nil, pkg.ValidateInternalError(ErrNilConnection, "Producer")
	}

	var err error

	backoff := utils.InitialBackoff

	headers := amqp.Table{
		libConstants.HeaderID: reqId,
	}

	libOpentelemetry.InjectTraceHeadersIntoQueue(ctx, (*map[string]any)(&headers))

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

	// Ensure channel is available ONCE before retry loop.
	// This prevents channel thrashing under burst load where 110+ goroutines
	// × 5 retries = 550 potential channel operations can overwhelm RabbitMQ.
	if err = prmq.conn.EnsureChannel(); err != nil {
		libOpentelemetry.HandleSpanError(&spanProducer, "Failed to establish initial channel", err)
		logger.Errorf("Cannot publish message: failed to establish initial channel: %v", err)

		return nil, pkg.ValidateInternalError(err, "Producer")
	}

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

		// Only recreate channel if previous attempt had a channel-specific error.
		// This prevents channel thrashing - we only recreate when actually needed.
		if attempt > 0 && isChannelError(err) {
			if channelErr := prmq.conn.EnsureChannel(); channelErr != nil {
				logger.Errorf("Failed to recreate channel, attempt %d/%d: %v", attempt+1, utils.MaxRetries+1, channelErr)

				if isLastAttempt(attempt) {
					libOpentelemetry.HandleSpanError(&spanProducer, "Failed to establish channel after retries", channelErr)
					logger.Errorf("Giving up after %d attempts: failed to establish channel", utils.MaxRetries+1)

					return nil, pkg.ValidateInternalError(channelErr, "Producer")
				}

				err = channelErr
				sleepDuration := utils.FullJitter(backoff)
				logger.Infof("Retrying to reconnect in %v...", sleepDuration)
				time.Sleep(sleepDuration)

				backoff = utils.NextBackoff(backoff)

				continue
			}
		}

		// Enable publisher confirm mode on the channel.
		// This must be called on every iteration because EnsureChannel() may
		// return a new channel after connection recovery (idempotent per AMQP spec).
		if err = prmq.conn.Channel.Confirm(false); err != nil {
			logger.Errorf("Failed to enable confirm mode on channel, attempt %d/%d: %v", attempt+1, utils.MaxRetries+1, err)

			if isLastAttempt(attempt) {
				libOpentelemetry.HandleSpanError(&spanProducer, "Failed to enable confirm mode after retries", err)
				logger.Errorf("Giving up after %d attempts: failed to enable confirm mode", utils.MaxRetries+1)

				return nil, pkg.ValidateInternalError(err, "Producer")
			}

			sleepDuration := utils.FullJitter(backoff)
			logger.Infof("Retrying to enable confirms in %v...", sleepDuration)
			time.Sleep(sleepDuration)

			backoff = utils.NextBackoff(backoff)

			continue
		}

		// Create a channel to receive publish confirmations
		// Buffer size of 1 is sufficient since we wait for each confirmation
		confirms := prmq.conn.Channel.NotifyPublish(make(chan amqp.Confirmation, 1))

		// Publish the message
		err = prmq.conn.Channel.Publish(
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
		)
		if err != nil {
			logger.Warnf("Failed to publish message to exchange: %s, key: %s, attempt %d/%d: %s",
				exchange, key, attempt+1, utils.MaxRetries+1, err)

			if isLastAttempt(attempt) {
				libOpentelemetry.HandleSpanError(&spanProducer, "Failed to publish message after retries", err)
				logger.Errorf("Giving up after %d attempts: %s", utils.MaxRetries+1, err)

				return nil, pkg.ValidateInternalError(err, "Producer")
			}

			sleepDuration := utils.FullJitter(backoff)
			logger.Infof("Retrying to publish message in %v (attempt %d)...", sleepDuration, attempt+attemptDisplayOffset)
			time.Sleep(sleepDuration)

			backoff = utils.NextBackoff(backoff)

			continue
		}

		// Wait for broker confirmation with timeout
		// This is the critical section that ensures message persistence
		select {
		case confirmation, ok := <-confirms:
			if !ok {
				// Channel was closed - connection issue
				err = ErrConfirmChannelClosed

				logger.Warnf("Confirmation channel closed, attempt %d/%d", attempt+1, utils.MaxRetries+1)

				if isLastAttempt(attempt) {
					libOpentelemetry.HandleSpanError(&spanProducer, "Confirmation channel closed after retries", err)
					logger.Errorf("Giving up after %d attempts: confirmation channel closed", utils.MaxRetries+1)

					return nil, pkg.ValidateInternalError(err, "Producer")
				}

				sleepDuration := utils.FullJitter(backoff)
				logger.Infof("Retrying after confirmation channel closure in %v...", sleepDuration)
				time.Sleep(sleepDuration)

				backoff = utils.NextBackoff(backoff)

				continue
			}

			if confirmation.Ack {
				// SUCCESS: Broker confirmed message persistence
				// TODO(review): Add observability metrics for publisher confirms (success/nack/timeout counters, duration histogram) (reported by code-reviewer and business-logic-reviewer on 2025-12-14, severity: Low)
				// TODO(review): Consider logging delivery tag at DEBUG level only for production (reported by security-reviewer on 2025-12-14, severity: Low)
				// TODO(review): Consider using structured logging fields for better log aggregation (reported by business-logic-reviewer on 2025-12-14, severity: Low)
				logger.Infof("Message confirmed by broker (delivery tag: %d) to exchange: %s, key: %s",
					confirmation.DeliveryTag, exchange, key)

				return nil, nil
			}

			// NACK: Broker rejected the message
			err = errors.Join(ErrBrokerNack, ErrDeliveryTagReceived)
			logger.Warnf("Broker NACK received, attempt %d/%d: %v", attempt+1, utils.MaxRetries+1, err)

			if isLastAttempt(attempt) {
				libOpentelemetry.HandleSpanError(&spanProducer, "Broker NACK after retries", err)
				logger.Errorf("Giving up after %d attempts: broker NACK", utils.MaxRetries+1)

				return nil, pkg.ValidateInternalError(err, "Producer")
			}

			sleepDuration := utils.FullJitter(backoff)
			logger.Infof("Retrying after broker NACK in %v...", sleepDuration)
			time.Sleep(sleepDuration)

			backoff = utils.NextBackoff(backoff)

			continue

		case <-time.After(publishConfirmTimeout):
			// TIMEOUT: No confirmation received within timeout
			err = ErrConfirmTimeout
			logger.Warnf("Confirmation timeout, attempt %d/%d: %v", attempt+1, utils.MaxRetries+1, err)

			if isLastAttempt(attempt) {
				libOpentelemetry.HandleSpanError(&spanProducer, "Publish confirmation timeout after retries", err)
				logger.Errorf("Giving up after %d attempts: confirmation timeout", utils.MaxRetries+1)

				return nil, pkg.ValidateInternalError(err, "Producer")
			}

			sleepDuration := utils.FullJitter(backoff)
			logger.Infof("Retrying after confirmation timeout in %v...", sleepDuration)
			time.Sleep(sleepDuration)

			backoff = utils.NextBackoff(backoff)

			continue

		case <-ctx.Done():
			// Context cancelled during confirmation wait
			err = ctx.Err()
			libOpentelemetry.HandleSpanError(&spanProducer, "Context cancelled while waiting for confirmation", err)
			logger.Warnf("Confirmation wait cancelled by context: %v", err)

			return nil, pkg.ValidateInternalError(err, "Producer")
		}
	}

	return nil, pkg.ValidateInternalError(err, "Producer")
}
