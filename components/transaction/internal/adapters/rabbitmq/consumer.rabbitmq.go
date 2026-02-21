// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"strings"
	"sync"
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
	ctx      context.Context
	cancel   context.CancelFunc
	stopOnce sync.Once
}

// NewConsumerRoutes creates a new instance of ConsumerRoutes.
func NewConsumerRoutes(conn *libRabbitmq.RabbitMQConnection, numbersOfWorkers int, numbersOfPrefetch int, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry) *ConsumerRoutes {
	if numbersOfWorkers == 0 {
		numbersOfWorkers = 5
	}

	if numbersOfPrefetch == 0 {
		numbersOfPrefetch = 10
	}

	runCtx, cancel := context.WithCancel(context.Background())

	cr := &ConsumerRoutes{
		conn:              conn,
		routes:            make(map[string]QueueHandlerFunc),
		NumbersOfWorkers:  numbersOfWorkers,
		NumbersOfPrefetch: numbersOfWorkers * numbersOfPrefetch,
		Logger:            logger,
		Telemetry:         *telemetry,
		ctx:               runCtx,
		cancel:            cancel,
	}

	_, err := conn.GetNewConnect()
	if err != nil {
		panic("Failed to connect rabbitmq")
	}

	return cr
}

// Stop requests all consumer goroutines to stop retrying/reconnecting.
func (cr *ConsumerRoutes) Stop() {
	if cr == nil || cr.cancel == nil {
		return
	}

	cr.stopOnce.Do(func() {
		cr.cancel()
	})
}

func (cr *ConsumerRoutes) sleepWithContext(duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-cr.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (cr *ConsumerRoutes) shouldStopConsumer(queueName string) bool {
	if cr.ctx == nil {
		return false
	}

	select {
	case <-cr.ctx.Done():
		cr.Infof("[Consumer %s] stopping retry loop", queueName)
		return true
	default:
		return false
	}
}

func (cr *ConsumerRoutes) waitAndIncreaseBackoff(queueName, stage string, backoff *time.Duration) bool {
	sleepDuration := utils.FullJitter(*backoff)
	cr.Infof("[Consumer %s] retrying %s in %v...", queueName, stage, sleepDuration)

	if !cr.sleepWithContext(sleepDuration) {
		cr.Infof("[Consumer %s] stopped while waiting to retry %s", queueName, stage)
		return false
	}

	*backoff = utils.NextBackoff(*backoff)

	return true
}

func (cr *ConsumerRoutes) ensureChannelReady(queueName string, backoff *time.Duration) bool {
	if err := cr.conn.EnsureChannel(); err != nil {
		cr.Errorf("[Consumer %s] failed to ensure channel: %v", queueName, err)
		return cr.waitAndIncreaseBackoff(queueName, "EnsureChannel", backoff)
	}

	return true
}

func (cr *ConsumerRoutes) configureQoS(queueName string, backoff *time.Duration) bool {
	// Defense-in-depth: EnsureChannel guarantees non-nil Channel on success,
	// but guard against unexpected races or future refactors.
	if cr.conn.Channel == nil {
		cr.Errorf("[Consumer %s] channel is nil after EnsureChannel", queueName)
		return cr.waitAndIncreaseBackoff(queueName, "QoS (nil channel)", backoff)
	}

	if err := cr.conn.Channel.Qos(
		cr.NumbersOfPrefetch,
		0,
		false,
	); err != nil {
		cr.Errorf("[Consumer %s] failed to set QoS: %v", queueName, err)
		return cr.waitAndIncreaseBackoff(queueName, "QoS", backoff)
	}

	return true
}

func (cr *ConsumerRoutes) startConsuming(queueName string, backoff *time.Duration) (<-chan amqp.Delivery, bool) {
	// Defense-in-depth: EnsureChannel guarantees non-nil Channel on success,
	// but guard against unexpected races or future refactors.
	if cr.conn.Channel == nil {
		cr.Errorf("[Consumer %s] channel is nil after EnsureChannel", queueName)
		return nil, cr.waitAndIncreaseBackoff(queueName, "Consume (nil channel)", backoff)
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
		return nil, cr.waitAndIncreaseBackoff(queueName, "Consume", backoff)
	}

	return messages, true
}

func (cr *ConsumerRoutes) launchWorkers(queueName string, handler QueueHandlerFunc, messages <-chan amqp.Delivery) {
	for i := 0; i < cr.NumbersOfWorkers; i++ {
		go cr.startWorker(i, queueName, handler, messages)
	}
}

func (cr *ConsumerRoutes) waitForRestart(queueName string, notifyClose <-chan *amqp.Error) bool {
	select {
	case <-cr.ctx.Done():
		cr.Infof("[Consumer %s] stopping after cancellation", queueName)
		return false
	case errClose := <-notifyClose:
		if errClose != nil {
			cr.Warnf("[Consumer %s] channel closed: %v", queueName, errClose)
		} else {
			cr.Warnf("[Consumer %s] channel closed: no error info", queueName)
		}
	}

	return true
}

func (cr *ConsumerRoutes) runConsumer(queueName string, handler QueueHandlerFunc) {
	backoff := utils.InitialBackoff

	for {
		if cr.shouldStopConsumer(queueName) {
			return
		}

		if !cr.ensureChannelReady(queueName, &backoff) {
			return
		}

		if !cr.configureQoS(queueName, &backoff) {
			return
		}

		messages, ok := cr.startConsuming(queueName, &backoff)
		if !ok {
			return
		}

		cr.Infof("[Consumer %s] consuming started", queueName)

		backoff = utils.InitialBackoff

		notifyClose := make(chan *amqp.Error, 1)

		// Defense-in-depth: EnsureChannel guarantees non-nil Channel on success,
		// but guard against unexpected races or future refactors.
		if cr.conn.Channel == nil {
			cr.Errorf("[Consumer %s] channel is nil before NotifyClose", queueName)

			continue
		}

		cr.conn.Channel.NotifyClose(notifyClose)

		cr.launchWorkers(queueName, handler, messages)

		if !cr.waitForRestart(queueName, notifyClose) {
			return
		}

		cr.Warnf("[Consumer %s] restarting...", queueName)
	}
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
			cr.runConsumer(queueName, handler)
		}(queueName, handler)
	}

	return nil
}

// startWorker starts a worker that processes messages from the queue.
func (cr *ConsumerRoutes) startWorker(workerID int, queue string, handlerFunc QueueHandlerFunc, messages <-chan amqp.Delivery) {
	for msg := range messages {
		midazID := libCommons.GenerateUUIDv7().String()

		if rawID, found := msg.Headers[libConstants.HeaderID]; found {
			if parsedID, ok := rawID.(string); ok {
				parsedID = strings.TrimSpace(parsedID)
				if parsedID != "" {
					midazID = parsedID
				}
			}
		}

		log := cr.Logger.WithFields(
			libConstants.HeaderID, midazID,
		).WithDefaultMessageTemplate(midazID + libConstants.LoggerDefaultSeparator)

		ctx := libCommons.ContextWithLogger(
			libCommons.ContextWithHeaderID(context.Background(), midazID),
			log,
		)

		ctx = libCommons.ContextWithHeaderID(ctx, midazID)
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
