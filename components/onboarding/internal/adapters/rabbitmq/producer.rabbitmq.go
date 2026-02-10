package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	amqp "github.com/rabbitmq/amqp091-go"
)

// ProducerRepository provides an interface for Producer related to rabbitmq.
// It is used to send messages to a queue.
type ProducerRepository interface {
	ProducerDefault(ctx context.Context, exchange, key string, message mmodel.Queue) (*string, error)
	CheckRabbitMQHealth() bool
}

// ProducerRabbitMQRepository is a rabbitmq implementation of the producer
type ProducerRabbitMQRepository struct {
	conn             *libRabbitmq.RabbitMQConnection
	mu               sync.Mutex
	confirmedChannel *amqp.Channel
}

// NewProducerRabbitMQ returns a new instance of ProducerRabbitMQRepository using the given rabbitmq connection.
func NewProducerRabbitMQ(c *libRabbitmq.RabbitMQConnection) (*ProducerRabbitMQRepository, error) {
	prmq := &ProducerRabbitMQRepository{
		conn: c,
	}

	_, err := c.GetNewConnect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}

	return prmq, nil
}

// CheckRabbitMQHealth checks the health of the rabbitmq connection.
func (prmq *ProducerRabbitMQRepository) CheckRabbitMQHealth() bool {
	return prmq.conn.HealthCheck()
}

// ProducerDefault sends a message to a RabbitMQ queue for further processing.
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

		prmq.mu.Lock()
		needsConfirm := prmq.conn.Channel != prmq.confirmedChannel
		prmq.mu.Unlock()

		if needsConfirm {
			if err = prmq.conn.Channel.Confirm(false); err != nil {
				logger.Warnf("Failed to put channel in confirm mode: %v", err)

				sleepDuration := utils.FullJitter(backoff)
				logger.Infof("Retrying in %v...", sleepDuration)
				time.Sleep(sleepDuration)

				backoff = utils.NextBackoff(backoff)

				continue
			}

			prmq.mu.Lock()
			prmq.confirmedChannel = prmq.conn.Channel
			prmq.mu.Unlock()
		}

		publishConfirmation, err := prmq.conn.Channel.PublishWithDeferredConfirmWithContext(
			ctx,
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
			publishConfirmationCtx, cancel := context.WithTimeout(ctx, time.Second*5)
			ackConfirmed, waitErr := publishConfirmation.WaitContext(publishConfirmationCtx)

			cancel() // Call cancel immediately after use, not via defer

			if waitErr != nil || !ackConfirmed {
				logger.Warnf("Failed to wait for publish confirmation: %v", waitErr)

				return nil, waitErr
			}

			logger.Infof("Messages sent successfully to exchange: %s, key: %s", exchange, key)

			return nil, nil
		} else {
			logger.Warnf("Failed to publish message to exchange: %s, key: %s, attempt %d/%d: %s", exchange, key, attempt+1, utils.MaxRetries+1, err)
		}

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
