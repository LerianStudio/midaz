package rabbitmq

import (
	"github.com/LerianStudio/midaz/common/mrabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

// ProducerRepository provides an interface for Producer related to rabbitmq.
//
//go:generate mockgen --destination=producer.mock.go --package=rabbitmq . ProducerRepository
type ProducerRepository interface {
	ProducerDefault(message string) (*string, error)
}

// ProducerRabbitMQRepository is a rabbitmq implementation of the producer
type ProducerRabbitMQRepository struct {
	conn *mrabbitmq.RabbitMQConnection
}

// NewProducerRabbitMQ returns a new instance of ProducerRabbitMQRepository using the given rabbitmq connection.
func NewProducerRabbitMQ(c *mrabbitmq.RabbitMQConnection) *ProducerRabbitMQRepository {
	prmq := &ProducerRabbitMQRepository{
		conn: c,
	}

	_, err := c.GetNewConnect()
	if err != nil {
		panic("Failed to connect rabbitmq")
	}

	return prmq
}

func (prmq *ProducerRabbitMQRepository) ProducerDefault(message string) (*string, error) {
	prmq.conn.Logger.Infoln("init sent message")

	err := prmq.conn.Channel.Publish(
		prmq.conn.Exchange,
		prmq.conn.Key,
		false,
		false,
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(message),
		})
	if err != nil {
		prmq.conn.Logger.Errorf("Failed to publish a message: %s", err)

		return nil, err
	}

	prmq.conn.Logger.Infoln("messages sent successfully")

	return nil, nil
}
