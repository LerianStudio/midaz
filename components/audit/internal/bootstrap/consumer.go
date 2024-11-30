package bootstrap

import (
	"context"
	"encoding/json"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/rabbitmq/transaction"
	"github.com/LerianStudio/midaz/components/audit/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mrabbitmq"
	"github.com/LerianStudio/midaz/pkg/net/http"
)

// Consumer is a rabbitmq implementation of the consumer
type Consumer struct {
	Conn    *mrabbitmq.RabbitMQConnection
	UseCase *services.UseCase
	mlog.Logger
	mopentelemetry.Telemetry
}

// NewConsumer returns a new instance of Consumer using the given rabbitmq connection.
func NewConsumer(conn *mrabbitmq.RabbitMQConnection, useCase *services.UseCase, logger mlog.Logger, telemetry *mopentelemetry.Telemetry) *Consumer {
	consumer := &Consumer{
		Conn:      conn,
		UseCase:   useCase,
		Logger:    logger,
		Telemetry: *telemetry,
	}

	_, err := conn.GetNewConnect()
	if err != nil {
		panic("Failed to connect consumer on RABBITMQ")
	}

	return consumer
}

func (c *Consumer) Run(l *pkg.Launcher) error {
	message, err := c.Conn.Channel.Consume(
		c.Conn.Queue,
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		c.Logger.Errorf("Failed to register a consumer: %s", err)
	}

	for d := range message {
		midazID := d.Headers["Midaz-Id"].(string)

		log := c.Logger.WithFields(
			http.HeaderMidazID, midazID,
			http.HeaderCorrelationID, d.CorrelationId,
		).WithDefaultMessageTemplate(midazID + " | ")

		ctx := pkg.ContextWithMidazID(context.Background(), midazID)
		ctx = pkg.ContextWithLogger(ctx, log)

		var transactionMessage transaction.Transaction

		err = json.Unmarshal(d.Body, &transactionMessage)
		if err != nil {
			c.Logger.Errorf("Error unmarshalling transaction message JSON: %v", err)
			continue
		}

		c.UseCase.CreateLog(ctx, transactionMessage)

		c.Logger.Infof("Message consumed: %s", transactionMessage.ID)
	}

	return nil
}
