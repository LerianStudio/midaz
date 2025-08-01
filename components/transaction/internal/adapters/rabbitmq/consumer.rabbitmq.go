package rabbitmq

import (
	"context"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	attribute "go.opentelemetry.io/otel/attribute"
)

// ConsumerRepository provides an interface for Consumer related to rabbitmq.
// It defines methods for registering queues and running consumers.
type ConsumerRepository interface {
	Register(queueName string, handler QueueHandlerFunc)
	RunConsumers() error
}

// QueueHandlerFunc is a function that process a specific queue.
type QueueHandlerFunc func(ctx context.Context, body []byte) error

// ConsumerRoutes struct
type ConsumerRoutes struct {
	conn              *libRabbitmq.RabbitMQConnection
	routes            map[string]QueueHandlerFunc
	NumbersOfWorkers  int
	NumbersOfPrefetch int
	libLog.Logger
	libOpentelemetry.Telemetry
}

// NewConsumerRoutes creates a new instance of ConsumerRoutes.
func NewConsumerRoutes(conn *libRabbitmq.RabbitMQConnection, numbersOfWorkers int, numbersOfPrefetch int, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry) *ConsumerRoutes {
	if numbersOfWorkers == 0 {
		numbersOfWorkers = 5
	}

	if numbersOfPrefetch == 0 {
		numbersOfPrefetch = 10
	}

	cr := &ConsumerRoutes{
		conn:              conn,
		routes:            make(map[string]QueueHandlerFunc),
		NumbersOfWorkers:  numbersOfWorkers,
		NumbersOfPrefetch: numbersOfWorkers * numbersOfPrefetch,
		Logger:            logger,
		Telemetry:         *telemetry,
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

// RunConsumers  init consume for all registry queues.
func (cr *ConsumerRoutes) RunConsumers() error {
	for queueName, handler := range cr.routes {
		cr.Infof("Initializing consumer for queue: %s", queueName)

		err := cr.conn.Channel.Qos(
			cr.NumbersOfPrefetch,
			0,
			false,
		)
		if err != nil {
			return err
		}

		messages, err := cr.conn.Channel.Consume(
			queueName,
			"",
			false,
			false,
			false,
			false,
			nil,
		)
		if err != nil {
			return err
		}

		for i := 0; i < cr.NumbersOfWorkers; i++ {
			go func(workerID int, queue string, handlerFunc QueueHandlerFunc) {
				for msg := range messages {
					midazID, found := msg.Headers[libConstants.HeaderID]
					if !found {
						midazID = libCommons.GenerateUUIDv7().String()
					}

					log := cr.Logger.WithFields(
						libConstants.HeaderID, midazID.(string),
					).WithDefaultMessageTemplate(midazID.(string) + libConstants.LoggerDefaultSeparator)

					ctx := libCommons.ContextWithLogger(
						libCommons.ContextWithHeaderID(context.Background(), midazID.(string)),
						log,
					)

					ctx = libOpentelemetry.ExtractTraceContextFromQueueHeaders(ctx, msg.Headers)

					tracer := libCommons.NewTracerFromContext(ctx)
					ctx, spanConsumer := tracer.Start(ctx, "rabbitmq.consumer.process_message")

					spanConsumer.SetAttributes(
						attribute.String("app.request.rabbitmq.consumer.request_id", midazID.(string)),
					)

					err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&spanConsumer, "app.request.rabbitmq.consumer.message", strings.ToValidUTF8(string(msg.Body), "�"))
					if err != nil {
						libOpentelemetry.HandleSpanError(&spanConsumer, "Failed to convert message to JSON string", err)
					}

					err = handlerFunc(ctx, msg.Body)
					if err != nil {
						libOpentelemetry.HandleSpanError(&spanConsumer, "Error processing message from queue", err)

						spanConsumer.End()

						cr.Errorf("Worker %d: Error processing message from queue %s: %v", workerID, queue, err)

						_ = msg.Nack(false, true)

						continue
					}

					spanConsumer.End()

					_ = msg.Ack(false)
				}
			}(i, queueName, handler)
		}
	}

	return nil
}
