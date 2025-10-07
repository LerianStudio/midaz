// Package rabbitmq provides RabbitMQ integration for the onboarding service.
//
// This package implements message queue operations for asynchronous communication
// between the onboarding service and the transaction service. It uses RabbitMQ
// for reliable message delivery with retry logic and exponential backoff.
package rabbitmq

import (
	"context"
	"encoding/json"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	amqp "github.com/rabbitmq/amqp091-go"
)

// ProducerRepository provides an interface for RabbitMQ message publishing operations.
//
// This interface defines methods for sending messages to RabbitMQ exchanges with
// health checking capabilities. It abstracts RabbitMQ-specific implementation details.
type ProducerRepository interface {
	ProducerDefault(ctx context.Context, exchange, key string, message mmodel.Queue) (*string, error)
	CheckRabbitMQHealth() bool
}

// ProducerRabbitMQRepository is a RabbitMQ implementation of the ProducerRepository interface.
//
// This struct provides concrete RabbitMQ-based message publishing with automatic retry logic,
// exponential backoff, and channel recovery. It ensures reliable message delivery even when
// RabbitMQ connections are temporarily unavailable.
type ProducerRabbitMQRepository struct {
	conn *libRabbitmq.RabbitMQConnection
}

// NewProducerRabbitMQ creates a new RabbitMQ producer repository instance.
//
// This constructor initializes the RabbitMQ connection and panics if the connection fails.
// The panic is intentional as the service cannot function without message queue connectivity.
//
// Parameters:
//   - c: RabbitMQ connection configuration
//
// Returns:
//   - *ProducerRabbitMQRepository: Initialized producer
//
// Panics:
//   - If RabbitMQ connection fails
func NewProducerRabbitMQ(c *libRabbitmq.RabbitMQConnection) *ProducerRabbitMQRepository {
	prmq := &ProducerRabbitMQRepository{
		conn: c,
	}

	_, err := c.GetNewConnect()
	if err != nil {
		panic("Failed to connect rabbitmq")
	}

	return prmq
}

// CheckRabbitMQHealth checks the health status of the RabbitMQ connection.
//
// This method verifies that RabbitMQ is reachable and accepting connections.
// It's used during service startup and health check endpoints.
//
// Returns:
//   - bool: true if RabbitMQ is healthy, false otherwise
func (prmq *ProducerRabbitMQRepository) CheckRabbitMQHealth() bool {
	return prmq.conn.HealthCheck()
}

// ProducerDefault publishes a message to RabbitMQ with automatic retry and exponential backoff.
//
// This method implements reliable message publishing with:
//   - Automatic retry on failure (up to MaxRetries attempts)
//   - Exponential backoff with full jitter
//   - Channel recovery on connection failures
//   - Persistent message delivery mode
//   - OpenTelemetry trace context propagation
//
// Retry Logic:
//   - MaxRetries: 5 attempts
//   - InitialBackoff: 500ms
//   - MaxBackoff: 10s
//   - BackoffFactor: 2.0 (exponential)
//   - Jitter: Full jitter to prevent thundering herd
//
// Message Properties:
//   - ContentType: application/json
//   - DeliveryMode: Persistent (survives broker restarts)
//   - Headers: Includes request ID and trace context
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - exchange: RabbitMQ exchange name
//   - key: Routing key for message routing
//   - queueMessage: Message payload to send
//
// Returns:
//   - *string: nil (unused)
//   - error: nil on success, error after all retries exhausted
//
// OpenTelemetry: Creates span "rabbitmq.producer.publish_message"
func (prmq *ProducerRabbitMQRepository) ProducerDefault(ctx context.Context, exchange, key string, queueMessage mmodel.Queue) (*string, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	logger.Infof("Init sent message")

	ctx, spanProducer := tracer.Start(ctx, "rabbitmq.producer.publish_message")
	defer spanProducer.End()

	var err error

	backoff := utils.InitialBackoff

	message, err := json.Marshal(queueMessage)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanProducer, "Failed to marshal queue message struct", err)

		logger.Errorf("Failed to marshal queue message struct")

		return nil, err
	}

	headers := amqp.Table{
		libConstants.HeaderID: reqId,
	}

	libOpentelemetry.InjectTraceHeadersIntoQueue(ctx, (*map[string]any)(&headers))

	for attempt := 0; attempt <= utils.MaxRetries; attempt++ {
		if err = prmq.conn.EnsureChannel(); err != nil {
			logger.Errorf("Failed to reopen channel: %v", err)

			sleepDuration := utils.FullJitter(backoff)
			logger.Infof("Retrying to reconnect in %v...", sleepDuration)
			time.Sleep(sleepDuration)

			backoff = utils.NextBackoff(backoff)

			continue
		}

		err = prmq.conn.Channel.Publish(
			exchange,
			key,
			false,
			false,
			amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Headers:      headers,
				Body:         message,
			},
		)
		if err == nil {
			logger.Infof("Messages sent successfully to exchange: %s, key: %s", exchange, key)

			return nil, nil
		}

		logger.Warnf("Failed to publish message to exchange: %s, key: %s, attempt %d/%d: %s", exchange, key, attempt+1, utils.MaxRetries+1, err)

		if attempt == utils.MaxRetries {
			libOpentelemetry.HandleSpanError(&spanProducer, "Failed to publish message after retries", err)

			logger.Errorf("Giving up after %d attempts: %s", utils.MaxRetries+1, err)

			return nil, err
		}

		sleepDuration := utils.FullJitter(backoff)
		logger.Infof("Retrying to publish message in %v (attempt %d)...", sleepDuration, attempt+2)
		time.Sleep(sleepDuration)

		backoff = utils.NextBackoff(backoff)
	}

	return nil, err
}
