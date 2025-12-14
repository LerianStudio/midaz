package rabbitmq

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mruntime"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// maxRetries is the maximum number of delivery attempts (including first delivery)
// before rejecting as a poison message to prevent infinite retry loops.
const maxRetries = 4

// retryCountHeader is the custom header used to track retry attempts.
// We use a custom header instead of RabbitMQ's x-death because x-death
// is only populated when messages go through a Dead Letter Exchange, not on
// direct Nack requeues. Our custom header provides accurate retry tracking.
const retryCountHeader = "x-midaz-retry-count"

// ConsumerRepository provides an interface for Consumer related to rabbitmq.
// It defines methods for registering queues and running consumers.
type ConsumerRepository interface {
	Register(queueName string, handler QueueHandlerFunc)
	RunConsumers() error
}

// getRetryCount extracts the retry count from our custom retry tracking header.
// This header is set and incremented by this consumer on each republish.
// Returns 0 for first delivery (header not present).
func getRetryCount(headers amqp.Table) int {
	if val, ok := headers[retryCountHeader].(int32); ok {
		return int(val)
	}
	// Check int64 for compatibility
	if val, ok := headers[retryCountHeader].(int64); ok {
		return int(val)
	}

	return 0
}

// copyHeaders creates a deep copy of amqp.Table for safe header modification
func copyHeaders(src amqp.Table) amqp.Table {
	if src == nil {
		return amqp.Table{}
	}

	dst := make(amqp.Table, len(src))
	for k, v := range src {
		dst[k] = v
	}

	return dst
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
	assert.NoError(err, "RabbitMQ connection must succeed during initialization",
		"component", "rabbitmq_consumer",
		"workers", numbersOfWorkers,
		"prefetch", numbersOfPrefetch)

	return cr
}

// Register add a new queue to handler.
func (cr *ConsumerRoutes) Register(queueName string, handler QueueHandlerFunc) {
	cr.routes[queueName] = handler
}

// RunConsumers init consume for all registry queues.
//
//nolint:gocognit // Complexity from panic recovery, backoff, and reconnection logic is necessary for resilience
func (cr *ConsumerRoutes) RunConsumers() error {
	for queueName, handler := range cr.routes {
		cr.Infof("Initializing consumer for queue: %s", queueName)

		// Capture loop variables before SafeGo
		queue := queueName
		queueHandler := handler

		mruntime.SafeGo(cr.Logger, "rabbitmq_consumer_"+queue, mruntime.KeepRunning, func() {
			backoff := utils.InitialBackoff

			for {
				// Wrap each iteration in an anonymous function with panic recovery
				shouldContinue := func() bool {
					defer mruntime.RecoverAndLog(cr.Logger, "rabbitmq_consumer_loop_"+queue)

					if err := cr.conn.EnsureChannel(); err != nil {
						cr.Errorf("[Consumer %s] failed to ensure channel: %v", queue, err)

						sleepDuration := utils.FullJitter(backoff)
						cr.Infof("[Consumer %s] retrying EnsureChannel in %v...", queue, sleepDuration)
						time.Sleep(sleepDuration)

						backoff = utils.NextBackoff(backoff)

						return true
					}

					if err := cr.conn.Channel.Qos(
						cr.NumbersOfPrefetch,
						0,
						false,
					); err != nil {
						cr.Errorf("[Consumer %s] failed to set QoS: %v", queue, err)

						sleepDuration := utils.FullJitter(backoff)
						cr.Infof("[Consumer %s] retrying QoS in %v...", queue, sleepDuration)
						time.Sleep(sleepDuration)

						backoff = utils.NextBackoff(backoff)

						return true
					}

					messages, err := cr.conn.Channel.Consume(
						queue,
						"",
						false,
						false,
						false,
						false,
						nil,
					)
					if err != nil {
						cr.Errorf("[Consumer %s] failed to start consuming: %v", queue, err)

						sleepDuration := utils.FullJitter(backoff)
						cr.Infof("[Consumer %s] retrying Consume in %v...", queue, sleepDuration)
						time.Sleep(sleepDuration)

						backoff = utils.NextBackoff(backoff)

						return true
					}

					cr.Infof("[Consumer %s] consuming started", queue)

					backoff = utils.InitialBackoff

					notifyClose := make(chan *amqp.Error, 1)
					cr.conn.Channel.NotifyClose(notifyClose)

					for i := 0; i < cr.NumbersOfWorkers; i++ {
						workerID := i

						mruntime.SafeGo(cr.Logger, "rabbitmq_worker_"+queue, mruntime.KeepRunning, func() {
							cr.startWorker(workerID, queue, queueHandler, messages)
						})
					}

					if errClose := <-notifyClose; errClose != nil {
						cr.Warnf("[Consumer %s] channel closed: %v", queue, errClose)
					} else {
						cr.Warnf("[Consumer %s] channel closed: no error info", queue)
					}

					cr.Warnf("[Consumer %s] restarting...", queue)

					return true
				}()

				if !shouldContinue {
					break
				}
			}
		})
	}

	return nil
}

// startWorker starts a worker that processes messages from the queue.
//
//nolint:cyclop // Complexity from panic recovery with span events and message ack/nack handling is necessary for safety
func (cr *ConsumerRoutes) startWorker(workerID int, queue string, handlerFunc QueueHandlerFunc, messages <-chan amqp.Delivery) {
	for msg := range messages {
		func() {
			// Safely extract HeaderID - handle both string and []byte types
			midazID := libCommons.GenerateUUIDv7().String()

			if raw, ok := msg.Headers[libConstants.HeaderID]; ok {
				switch v := raw.(type) {
				case string:
					midazID = v
				case []byte:
					midazID = string(v)
				}
			}

			log := cr.Logger.WithFields(
				libConstants.HeaderID, midazID,
			).WithDefaultMessageTemplate(midazID + libConstants.LoggerDefaultSeparator)

			ctx := libCommons.ContextWithLogger(
				libCommons.ContextWithHeaderID(context.Background(), midazID),
				log,
			)

			ctx = libOpentelemetry.ExtractTraceContextFromQueueHeaders(ctx, msg.Headers)

			logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)
			ctx, spanConsumer := tracer.Start(ctx, "rabbitmq.consumer.process_message")

			ctx = libCommons.ContextWithSpanAttributes(ctx, attribute.String("app.request.request_id", reqId))

			defer spanConsumer.End()

			// Panic recovery with span event recording and poison message handling
			// Uses custom x-midaz-retry-count header to track retries since x-death
			// header is only populated when messages go through DLX, not on Nack requeue.
			// Does NOT re-panic so the worker survives and continues processing.
			defer func() {
				if r := recover(); r != nil {
					stack := debug.Stack()
					retryCount := getRetryCount(msg.Headers)

					spanConsumer.AddEvent("panic.recovered", trace.WithAttributes(
						attribute.String("panic.value", fmt.Sprintf("%v", r)),
						attribute.String("panic.stack", string(stack)),
						attribute.String("rabbitmq.queue", queue),
						attribute.Int("rabbitmq.worker_id", workerID),
						attribute.Int("rabbitmq.retry_count", retryCount),
					))

					logger.Errorf("Worker %d: panic recovered while processing message from queue %s: %v\n%s",
						workerID, queue, r, string(stack))

					// Check if message has exceeded max retry attempts (poison message)
					if retryCount >= maxRetries-1 {
						logger.Errorf("Worker %d: poison message rejected after %d delivery attempts: %v",
							workerID, retryCount+1, r)
						// Reject without requeue - message will go to DLX if configured
						if err := msg.Reject(false); err != nil {
							logger.Warnf("Worker %d: failed to reject poison message: %v", workerID, err)
						}
					} else {
						logger.Warnf("Worker %d: redelivering message (delivery %d of %d max): %v",
							workerID, retryCount+1, maxRetries, r)

						// Republish message with incremented retry counter
						headers := copyHeaders(msg.Headers)
						headers[retryCountHeader] = int32(retryCount + 1)

						ch, err := cr.conn.Connection.Channel()
						if err != nil {
							logger.Errorf("Worker %d: failed to get channel for republish: %v", workerID, err)
							// Fallback: Nack without header (will retry indefinitely, but worker is already panicking)
							if nackErr := msg.Nack(false, true); nackErr != nil {
								logger.Warnf("Worker %d: failed to nack message: %v", workerID, nackErr)
							}
						} else {
							defer ch.Close()

							err = ch.Publish(
								"",    // exchange (use default)
								queue, // routing key (queue name)
								false, // mandatory
								false, // immediate
								amqp.Publishing{
									Headers:      headers,
									Body:         msg.Body,
									ContentType:  msg.ContentType,
									DeliveryMode: amqp.Persistent,
								},
							)
							if err != nil {
								logger.Errorf("Worker %d: failed to republish message: %v", workerID, err)
								// Fallback: Nack
								if nackErr := msg.Nack(false, true); nackErr != nil {
									logger.Warnf("Worker %d: failed to nack message: %v", workerID, nackErr)
								}
							} else {
								// Successfully republished - Ack original to remove from queue
								if err := msg.Ack(false); err != nil {
									logger.Warnf("Worker %d: failed to ack original message after republish: %v", workerID, err)
								}
							}
						}
					}

					// Record panic to span and metrics manually so worker can survive and continue
					mruntime.RecordPanicToSpanWithComponent(ctx, r, stack, "rabbitmq_consumer", "worker_"+queue)
				}
			}()

			err := libOpentelemetry.SetSpanAttributesFromStruct(&spanConsumer, "app.request.rabbitmq.consumer.message", msg.Body)
			if err != nil {
				libOpentelemetry.HandleSpanError(&spanConsumer, "Failed to convert message to JSON string", err)
			}

			err = handlerFunc(ctx, msg.Body)
			if err != nil {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&spanConsumer, "Error processing message from queue", err)
				logger.Errorf("Worker %d: Error processing message from queue %s: %v", workerID, queue, err)

				// Nack(multiple=false, requeue=true)
				if nackErr := msg.Nack(false, true); nackErr != nil {
					logger.Warnf("Worker %d: failed to nack message: %v", workerID, nackErr)
				}

				return
			}

			if ackErr := msg.Ack(false); ackErr != nil {
				logger.Warnf("Worker %d: failed to ack message (may cause redelivery): %v", workerID, ackErr)
			}
		}()
	}

	cr.Warnf("[Consumer %s] worker %d stopped (channel closed)", queue, workerID)
}
