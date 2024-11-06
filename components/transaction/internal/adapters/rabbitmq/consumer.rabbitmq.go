package mrabbitmq

import (
	"context"
	"github.com/LerianStudio/midaz/common/mrabbitmq"
)

// ConsumerRabbitMQRepository is a rabbitmq implementation of the consumer
type ConsumerRabbitMQRepository struct {
	conn *mrabbitmq.RabbitMQConnection
}

// NewConsumerRabbitMQ returns a new instance of ConsumerRabbitMQRepository using the given rabbitmq connection.
func NewConsumerRabbitMQ(c *mrabbitmq.RabbitMQConnection) *ConsumerRabbitMQRepository {
	crmq := &ConsumerRabbitMQRepository{
		conn: c,
	}

	_, err := c.GetNewConnect()
	if err != nil {
		panic("Failed to connect rabbitmq")
	}

	return crmq
}

func (crmq *ConsumerRabbitMQRepository) Consumer(ctx context.Context, queue string, response chan string) {
	crmq.conn.Logger.Infoln("init consumer message")

	message, err := crmq.conn.Channel.Consume(
		"transaction_operations_queue",
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		crmq.conn.Logger.Errorf("Failed to register a consumer: %s", err)
	}

	for d := range message {
		crmq.conn.Logger.Infof("message consumed: %s", d.Body)

		response <- string(d.Body[:])
	}
}
