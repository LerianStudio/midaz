package rabbitmq

import (
	"context"
	"encoding/json"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libConstants "github.com/LerianStudio/lib-commons/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/commons/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	amqp "github.com/rabbitmq/amqp091-go"
	"math/rand"
	"time"
)

const (
	maxRetries     = 5
	initialBackoff = 500 * time.Millisecond
	maxBackoff     = 10 * time.Second
	backoffFactor  = 2.0
	jitterFactor   = 0.3
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

func (prmq *ProducerRabbitMQRepository) ProducerDefault(ctx context.Context, exchange, key string, queueMessage mmodel.Queue) (*string, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	logger.Infof("Init sent message")

	_, spanProducer := tracer.Start(ctx, "rabbitmq.producer.publish_message")
	defer spanProducer.End()

	var err error

	backoff := initialBackoff

	message, err := json.Marshal(queueMessage)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanProducer, "Failed to marshal queue message struct", err)

		logger.Errorf("Failed to marshal queue message struct")

		return nil, err
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		err = prmq.conn.Channel.Publish(
			exchange,
			key,
			false,
			false,
			amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Headers: amqp.Table{
					libConstants.HeaderID: libCommons.NewHeaderIDFromContext(ctx),
				},
				Body: message,
			},
		)

		if err == nil {
			logger.Infof("Messages sent successfully to exchange: %s, key: %s", exchange, key)

			return nil, nil
		}

		logger.Warnf("Failed to publish message to exchange: %s, key: %s, attempt %d/%d: %s", exchange, key, attempt+1, maxRetries+1, err)

		if attempt == maxRetries {
			libOpentelemetry.HandleSpanError(&spanProducer, "Failed to publish message after retries", err)

			logger.Errorf("Giving up after %d attempts: %s", maxRetries+1, err)

			return nil, err
		}

		// #nosec G404
		jitter := time.Duration(rand.Float64() * jitterFactor * float64(backoff))

		sleepDuration := backoff + jitter
		if sleepDuration > maxBackoff {
			sleepDuration = maxBackoff
		}

		logger.Infof("Retrying to publish message in %v (attempt %d)...", sleepDuration, attempt+2)

		time.Sleep(sleepDuration)

		backoff = time.Duration(float64(backoff) * backoffFactor)
	}

	return nil, err
}
