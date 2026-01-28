package rabbitmq

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	amqp "github.com/rabbitmq/amqp091-go"
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

// RunConsumers init consume for all registry queues.
func (cr *ConsumerRoutes) RunConsumers() error {
	for queueName, handler := range cr.routes {
		cr.Infof("Initializing consumer for queue: %s", queueName)

		go func(queueName string, handler QueueHandlerFunc) {
			backoff := utils.InitialBackoff()

			for {
				if err := cr.conn.EnsureChannel(); err != nil {
					cr.Errorf("[Consumer %s] failed to ensure channel: %v", queueName, err)

					sleepDuration := utils.FullJitter(backoff)
					cr.Infof("[Consumer %s] retrying EnsureChannel in %v...", queueName, sleepDuration)
					time.Sleep(sleepDuration)

					backoff = utils.NextBackoff(backoff)

					continue
				}

				if err := cr.conn.Channel.Qos(
					cr.NumbersOfPrefetch,
					0,
					false,
				); err != nil {
					cr.Errorf("[Consumer %s] failed to set QoS: %v", queueName, err)

					sleepDuration := utils.FullJitter(backoff)
					cr.Infof("[Consumer %s] retrying QoS in %v...", queueName, sleepDuration)
					time.Sleep(sleepDuration)

					backoff = utils.NextBackoff(backoff)

					continue
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
					cr.Errorf("[Consumer %s] failed to start consuming: %v", queueName, err)

					sleepDuration := utils.FullJitter(backoff)
					cr.Infof("[Consumer %s] retrying Consume in %v...", queueName, sleepDuration)
					time.Sleep(sleepDuration)

					backoff = utils.NextBackoff(backoff)

					continue
				}

				cr.Infof("[Consumer %s] consuming started", queueName)

				backoff = utils.InitialBackoff()

				notifyClose := make(chan *amqp.Error, 1)
				cr.conn.Channel.NotifyClose(notifyClose)

				for i := 0; i < cr.NumbersOfWorkers; i++ {
					go cr.startWorker(i, queueName, handler, messages)
				}

				if errClose := <-notifyClose; errClose != nil {
					cr.Warnf("[Consumer %s] channel closed: %v", queueName, errClose)
				} else {
					cr.Warnf("[Consumer %s] channel closed: no error info", queueName)
				}

				cr.Warnf("[Consumer %s] restarting...", queueName)
			}
		}(queueName, handler)
	}

	return nil
}

// startWorker starts a worker that processes messages from the queue.
func (cr *ConsumerRoutes) startWorker(workerID int, queue string, handlerFunc QueueHandlerFunc, messages <-chan amqp.Delivery) {
	for msg := range messages {
		midazID, found := msg.Headers[libConstants.HeaderID]
		if !found {
			midazID = libCommons.GenerateUUIDv7().String()
		}

		midazIDStr, ok := midazID.(string)
		if !ok {
			midazIDStr = libCommons.GenerateUUIDv7().String()
		}

		log := cr.Logger.WithFields(
			libConstants.HeaderID, midazIDStr,
		).WithDefaultMessageTemplate(midazIDStr + libConstants.LoggerDefaultSeparator)

		ctx := libCommons.ContextWithLogger(
			libCommons.ContextWithHeaderID(context.Background(), midazIDStr),
			log,
		)

		ctx = libOpentelemetry.ExtractTraceContextFromQueueHeaders(ctx, msg.Headers)

		logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)
		ctx, spanConsumer := tracer.Start(ctx, "rabbitmq.consumer.process_message")

		ctx = libCommons.ContextWithSpanAttributes(ctx, attribute.String("app.request.request_id", reqId))

		err := libOpentelemetry.SetSpanAttributesFromStruct(&spanConsumer, "app.request.rabbitmq.consumer.message", msg.Body)
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanConsumer, "Failed to convert message to JSON string", err)
		}

		err = handlerFunc(ctx, msg.Body)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanConsumer, "Error processing message from queue", err)
			spanConsumer.End()
			logger.Errorf("Worker %d: Error processing message from queue %s: %v", workerID, queue, err)

			_ = msg.Nack(false, true)

			continue
		}

		spanConsumer.End()

		_ = msg.Ack(false)
	}

	cr.Warnf("[Consumer %s] worker %d stopped (channel closed)", queue, workerID)
}
