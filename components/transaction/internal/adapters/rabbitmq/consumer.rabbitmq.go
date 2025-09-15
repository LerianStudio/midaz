package rabbitmq

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
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

// RunConsumers  init consume for all registry queues.
func (cr *ConsumerRoutes) RunConsumers() error {
	// For each registered queue, spawn a resilient manager that will
	// reconnect and re-subscribe if the channel/connection drops.
	for queueName, handler := range cr.routes {
		q := queueName

		h := handler
		go cr.runQueueConsumers(q, h)
	}

	return nil
}

// runQueueConsumers manages a single queue subscription with automatic reconnection.
func (cr *ConsumerRoutes) runQueueConsumers(queueName string, handler QueueHandlerFunc) {
	for {
		cr.Infof("Initializing consumer for queue: %s", queueName)

		// Ensure we have a valid channel; only attempt reconnect when broker is healthy.
		if cr.conn.Channel == nil {
			if !cr.conn.HealthCheck() {
				cr.Warnf("RabbitMQ unhealthy while initializing consumer for %s; waiting...", queueName)
				<-time.After(750 * time.Millisecond)

				continue
			}

			if _, err := cr.conn.GetNewConnect(); err != nil {
				cr.Warnf("RabbitMQ not ready for queue %s: %v", queueName, err)
				<-time.After(750 * time.Millisecond)

				continue
			}
		}

		if err := cr.conn.Channel.Qos(cr.NumbersOfPrefetch, 0, false); err != nil {
			cr.Errorf("Failed to set QoS for %s: %v", queueName, err)
			// Force reconnect on QoS error by clearing stale channel state.
			cr.conn.Channel = nil

			cr.conn.Connected = false
			if cr.conn.HealthCheck() {
				_, _ = cr.conn.GetNewConnect()
			}

			<-time.After(750 * time.Millisecond)

			continue
		}

		deliveries, err := cr.conn.Channel.Consume(
			queueName,
			"",
			false,
			false,
			false,
			false,
			nil,
		)
		if err != nil {
			cr.Errorf("Failed to consume from %s: %v", queueName, err)
			// Force reconnect on consume error by clearing stale channel state.
			cr.conn.Channel = nil

			cr.conn.Connected = false
			if cr.conn.HealthCheck() {
				_, _ = cr.conn.GetNewConnect()
			}

			<-time.After(750 * time.Millisecond)

			continue
		}

		// Watch for channel closure to re-subscribe.
		notifyClosed := make(chan *amqp.Error, 1)
		cr.conn.Channel.NotifyClose(notifyClosed)

		for i := 0; i < cr.NumbersOfWorkers; i++ {
			go func(workerID int, queue string, handlerFunc QueueHandlerFunc, msgs <-chan amqp.Delivery) {
				for msg := range msgs {
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

					ctx = libCommons.ContextWithHeaderID(ctx, midazID.(string))
					ctx = libOpentelemetry.ExtractTraceContextFromQueueHeaders(ctx, msg.Headers)

					logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)
					ctx, spanConsumer := tracer.Start(ctx, "rabbitmq.consumer.process_message")

					ctx = libCommons.ContextWithSpanAttributes(ctx, attribute.String("app.request.request_id", reqId))

					if err := libOpentelemetry.SetSpanAttributesFromStruct(&spanConsumer, "app.request.rabbitmq.consumer.message", msg.Body); err != nil {
						libOpentelemetry.HandleSpanError(&spanConsumer, "Failed to convert message to JSON string", err)
					}

					if err := handlerFunc(ctx, msg.Body); err != nil {
						libOpentelemetry.HandleSpanBusinessErrorEvent(&spanConsumer, "Error processing message from queue", err)
						spanConsumer.End()
						logger.Errorf("Worker %d: Error processing message from queue %s: %v", workerID, queue, err)

						_ = msg.Nack(false, true)

						continue
					}

					spanConsumer.End()

					_ = msg.Ack(false)
				}
			}(i, queueName, handler, deliveries)
		}

		// Block until the channel is closed, then loop to reconnect.
		if err := <-notifyClosed; err != nil {
			cr.Warnf("RabbitMQ channel closed for %s: %v", queueName, err)
		} else {
			cr.Warnf("RabbitMQ channel closed for %s", queueName)
		}

		// Attempt to reconnect before next iteration, but only if broker is healthy.
		cr.conn.Channel = nil

		cr.conn.Connected = false
		if cr.conn.HealthCheck() {
			_, _ = cr.conn.GetNewConnect()
		}

		<-time.After(750 * time.Millisecond)
	}
}
