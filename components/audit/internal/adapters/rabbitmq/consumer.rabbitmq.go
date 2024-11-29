package rabbitmq

import (
	"context"
	"encoding/json"

	"github.com/LerianStudio/midaz/components/audit/internal/adapters/rabbitmq/transaction"
	"github.com/LerianStudio/midaz/components/audit/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mrabbitmq"
	"github.com/LerianStudio/midaz/pkg/net/http"
)

// ConsumerRepository provides an interface for Consumer related to rabbitmq.
//
//go:generate mockgen --destination=consumer.mock.go --package=rabbitmq . ConsumerRepository
type ConsumerRepository interface {
	ConsumerAudit()
}

// ConsumerRabbitMQRepository is a rabbitmq implementation of the consumer
type ConsumerRabbitMQRepository struct {
	conn *mrabbitmq.RabbitMQConnection
	uc   *services.UseCase
}

// NewConsumerRabbitMQ returns a new instance of ConsumerRabbitMQRepository using the given rabbitmq connection.
func NewConsumerRabbitMQ(c *mrabbitmq.RabbitMQConnection, uc *services.UseCase) *ConsumerRabbitMQRepository {
	crmq := &ConsumerRabbitMQRepository{
		conn: c,
		uc:   uc,
	}

	_, err := c.GetNewConnect()
	if err != nil {
		panic("Failed to connect rabbitmq")
	}

	return crmq
}

func (crmq *ConsumerRabbitMQRepository) ConsumerAudit() {
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
			midazID := d.Headers["Midaz-Id"].(string)

			l := crmq.conn.Logger.WithFields(
				http.HeaderMidazID, midazID,
				http.HeaderCorrelationID, d.CorrelationId,
			).WithDefaultMessageTemplate(midazID + " | ")

			ctx := pkg.ContextWithMidazID(context.Background(), midazID)
			ctx = pkg.ContextWithLogger(ctx, l)

			var transactionMessage transaction.Transaction

			err = json.Unmarshal(d.Body, &transactionMessage)
			if err != nil {
				crmq.conn.Logger.Errorf("Error unmarshalling transaction message JSON: %v", err)
				return
			}

			crmq.uc.CreateLog(ctx, transactionMessage)

			crmq.conn.Logger.Infof("Message consumed: %s", transactionMessage.ID)
		}
	}()

	<-forever
}
