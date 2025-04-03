package rabbitmq

import (
	"context"
	"encoding/json"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libConstants "github.com/LerianStudio/lib-commons/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/commons/rabbitmq"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	amqp "github.com/rabbitmq/amqp091-go"
)

// ProducerRepository provides an interface for Producer related to rabbitmq.
// // It defines methods for sending messages to a queue.
type ProducerRepository interface {
	ProducerDefault(ctx context.Context, exchange, key string, message mmodel.Queue) (*string, error)
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

// func (prmq *ProducerRabbitMQRepository) ProducerDefault(ctx context.Context, exchange, key string, queueMessage mmodel.Queue) (*string, error) { performs an operation
func (prmq *ProducerRabbitMQRepository) ProducerDefault(ctx context.Context, exchange, key string, queueMessage mmodel.Queue) (*string, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	logger.Infof("Init sent message")

	_, spanProducer := tracer.Start(ctx, "rabbitmq.producer.publish_message")

	defer spanProducer.End()

	message, err := json.Marshal(queueMessage)

	if err != nil {
		libOpentelemetry.HandleSpanError(&spanProducer, "Failed to marshal queue message struct", err)

		logger.Errorf("Failed to marshal queue message struct")

		return nil, err
	}

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
		})

	if err != nil {
		libOpentelemetry.HandleSpanError(&spanProducer, "Failed to marshal queue message struct", err)

		logger.Errorf("Failed to publish message: %s", err)

		return nil, err
	}

	logger.Infoln("Messages sent successfully")

	return nil, nil
}
