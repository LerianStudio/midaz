package rabbitmq

import (
	"context"
	"os"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	amqp "github.com/rabbitmq/amqp091-go"
)

// ProducerRepository provides an interface for Producer related to rabbitmq.
// // It defines methods for sending messages to a queue.
type ProducerRepository interface {
	ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error)
	CheckRabbitMQHealth() bool
}

// ProducerRabbitMQRepository is a rabbitmq implementation of the producer
type ProducerRabbitMQRepository struct {
	conn   *libRabbitmq.RabbitMQConnection
	logger libLog.Logger
}

// NewProducerRabbitMQ returns a new instance of ProducerRabbitMQRepository using the given rabbitmq connection.
func NewProducerRabbitMQ(c *libRabbitmq.RabbitMQConnection, logger libLog.Logger) *ProducerRabbitMQRepository {
	prmq := &ProducerRabbitMQRepository{
		conn:   c,
		logger: logger,
	}

	_, err := c.GetNewConnect()
	if err != nil {
		panic("Failed to connect rabbitmq")
	}

	return prmq
}

// CheckRabbitMQHealth checks the health of the rabbitmq connection.
func (prmq *ProducerRabbitMQRepository) CheckRabbitMQHealth() bool {
	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "false" {
		return true
	}

	return prmq.conn.HealthCheck()
}

// ProducerDefault sends a message to a RabbitMQ queue for further processing.
func (prmq *ProducerRabbitMQRepository) ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	logger.Infof("Init sent message to exchange: %s, key: %s", exchange, key)

	ctx, spanProducer := tracer.Start(ctx, "rabbitmq.producer.publish_message")
	defer spanProducer.End()

	var err error

	backoff := utils.InitialBackoff

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
			false, // mandatory
			false, // immediate
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
