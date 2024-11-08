package mrabbitmq

import (
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

func (crmq *ConsumerRabbitMQRepository) ConsumerDefault(response chan string) {
	crmq.conn.Logger.Infoln("init consumer message")

	message, err := crmq.conn.Channel.Consume(
		crmq.conn.Queue,
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

	forever := make(chan bool)

	go func() {
		for d := range message {
			//response <- string(d.Body[:])
			crmq.conn.Logger.Infof("message consumed: %s", d.Body)
		}
	}()

	<-forever
}
