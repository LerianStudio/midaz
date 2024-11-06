package mrabbitmq

import (
	"context"
	"github.com/LerianStudio/midaz/common/mrabbitmq"
	"github.com/streadway/amqp"
)

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

func (prmq *ProducerRabbitMQRepository) Producer(ctx context.Context, exchange, key, body string) (*string, error) {
	prmq.conn.Logger.Infoln("init sent message")

	err := prmq.conn.Channel.Publish(
		"transaction_operations_exchange",
		"transaction_operations_key",
		false,
		false,
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte("Mensagem de conta contábil"),
		})
	if err != nil {
		prmq.conn.Logger.Errorf("Failed to publish a message: %s", err)

		return nil, err
	}

	prmq.conn.Logger.Infoln("messages sent successfully")

	return nil, nil
}
