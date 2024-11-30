package rabbitmq

import (
	"context"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mrabbitmq"
	"github.com/LerianStudio/midaz/pkg/net/http"
)

// ConsumerRepository provides an interface for Consumer related to rabbitmq.
//
//go:generate mockgen --destination=consumer.mock.go --package=rabbitmq . ConsumerRepository
type ConsumerRepository interface {
	Register(queueName string, handler QueueHandlerFunc)
	RunConsumers() error
}

// QueueHandlerFunc is a function that process a specific queue.
type QueueHandlerFunc func(ctx context.Context, body []byte) error

// ConsumerRoutes struct
type ConsumerRoutes struct {
	conn   *mrabbitmq.RabbitMQConnection
	routes map[string]QueueHandlerFunc
	mlog.Logger
	mopentelemetry.Telemetry
}

// NewConsumerRoutes creates a new instance of ConsumerRoutes.
func NewConsumerRoutes(conn *mrabbitmq.RabbitMQConnection, logger mlog.Logger, telemetry *mopentelemetry.Telemetry) *ConsumerRoutes {
	cr := &ConsumerRoutes{
		conn:      conn,
		routes:    make(map[string]QueueHandlerFunc),
		Logger:    logger,
		Telemetry: *telemetry,
	}

	_, err := conn.GetNewConnect()
	if err != nil {
		panic("Failed to connect rabbitmq")
	}

	return cr
}

// Register add a new queue to handler.
func (cr *ConsumerRoutes) Register(queueName string, handler QueueHandlerFunc) {
	cr.routes[queueName] = handler
}

// RunConsumers init consume for all registry queues.
func (cr *ConsumerRoutes) RunConsumers() error {
	for queueName, handler := range cr.routes {
		cr.Logger.Infof("Initializing consumer for queue: %s", queueName)

		messages, err := cr.conn.Channel.Consume(
			queueName,
			"",
			true,
			false,
			false,
			false,
			nil,
		)
		if err != nil {
			return err
		}

		go func(queue string, handlerFunc QueueHandlerFunc) {
			for msg := range messages {
				midazID, found := msg.Headers["Midaz-Id"]
				if !found {
					midazID = pkg.GenerateUUIDv7().String()
				}

				correlationID := msg.CorrelationId
				if correlationID == "" {
					correlationID = pkg.GenerateUUIDv7().String()
				}

				log := cr.Logger.WithFields(
					http.HeaderMidazID, midazID.(string),
					http.HeaderCorrelationID, correlationID,
				).WithDefaultMessageTemplate(midazID.(string) + " | ")

				ctx := pkg.ContextWithLogger(pkg.ContextWithMidazID(context.Background(), midazID.(string)), log)

				err := handlerFunc(ctx, msg.Body)
				if err != nil {
					cr.Logger.Errorf("Error processing message from queue %s: %v", queue, err)
				}
			}
		}(queueName, handler)
	}

	return nil
}
