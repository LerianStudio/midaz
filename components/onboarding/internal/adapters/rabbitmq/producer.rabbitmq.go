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

// ProducerRepository provides an interface for Producer related to rabbitmq.
// It is used to send messages to a queue.
type ProducerRepository interface {
	ProducerDefault(ctx context.Context, exchange, key string, message mmodel.Queue) (*string, error)
	CheckRabbitMQHealth() bool
}

// ProducerRabbitMQRepository is a rabbitmq implementation of the producer
type ProducerRabbitMQRepository struct {
	conn *libRabbitmq.RabbitMQConnection
}

// NewProducerRabbitMQ returns a new instance of ProducerRabbitMQRepository using the given rabbitmq connection.
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
		if attempt > 0 {
			sleepDuration := utils.FullJitter(backoff)
			logger.Infof("Retrying to publish message in %v (attempt %d)...", sleepDuration, attempt+2)

			select {
			case <-ctx.Done():
				logger.Warnf("Context canceled while waiting to retry: %v", ctx.Err())
				return nil, ctx.Err()
			case <-time.After(sleepDuration):
			}

			backoff = utils.NextBackoff(backoff)
		}

		if err = prmq.conn.EnsureChannel(); err != nil {
			logger.Errorf("Failed to reopen channel: %v", err)
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

	}

	return nil, err
}
