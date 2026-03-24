// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libConstants "github.com/LerianStudio/lib-commons/v4/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v4/commons/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	attribute "go.opentelemetry.io/otel/attribute"
)

func resolveMessageHeaderID(headers amqp.Table) string {
	if raw, found := headers[libConstants.HeaderID]; found {
		switch value := raw.(type) {
		case string:
			if value != "" {
				return value
			}
		case []byte:
			if len(value) > 0 {
				return string(value)
			}
		}
	}

	generatedID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		generatedID = uuid.New()
	}

	return generatedID.String()
}

// ConsumerRepository provides an interface for Consumer related to rabbitmq.
// It defines methods for registering queues and running consumers.
type ConsumerRepository interface {
	Register(queueName string, handler QueueHandlerFunc)
	RunConsumers() error
}

// QueueHandlerFunc is a function that process a specific queue.
type QueueHandlerFunc func(ctx context.Context, body []byte) error

// BulkHandlerFunc is a function that processes a batch of messages.
// It receives the deliveries and returns:
// - results: per-message success/failure indicators for acknowledgment
// - error: if non-nil, indicates a complete bulk failure (fallback to individual processing)
type BulkHandlerFunc func(ctx context.Context, messages []amqp.Delivery) ([]BulkMessageResult, error)

// BulkMessageResult tracks the result for a single message in bulk processing.
type BulkMessageResult struct {
	Index   int   // Index in the original messages slice
	Success bool  // Whether processing succeeded (Ack) or failed (Nack)
	Error   error // Error if processing failed
}

// BulkConfig holds configuration for bulk message processing.
type BulkConfig struct {
	// Enabled indicates whether bulk mode is active.
	Enabled bool

	// Size is the number of messages to accumulate before flushing.
	Size int

	// FlushTimeout is the maximum duration to wait before flushing an incomplete batch.
	FlushTimeout time.Duration

	// FallbackEnabled indicates whether to fall back to individual processing on bulk failure.
	FallbackEnabled bool
}

// ConsumerRoutes struct
type ConsumerRoutes struct {
	conn              *libRabbitmq.RabbitMQConnection
	routes            map[string]QueueHandlerFunc
	bulkRoutes        map[string]BulkHandlerFunc
	bulkConfig        *BulkConfig
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
		bulkRoutes:        make(map[string]BulkHandlerFunc),
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

// ConfigureBulk sets the bulk processing configuration.
// Must be called before RunConsumers if bulk mode is desired.
func (cr *ConsumerRoutes) ConfigureBulk(cfg *BulkConfig) {
	cr.bulkConfig = cfg
}

// BulkConfig returns the current bulk configuration.
func (cr *ConsumerRoutes) BulkConfig() *BulkConfig {
	return cr.bulkConfig
}

// IsBulkModeEnabled returns true if bulk mode is configured and enabled.
func (cr *ConsumerRoutes) IsBulkModeEnabled() bool {
	return cr.bulkConfig != nil && cr.bulkConfig.Enabled
}

// Register add a new queue to handler.
func (cr *ConsumerRoutes) Register(queueName string, handler QueueHandlerFunc) {
	cr.routes[queueName] = handler
}

// RegisterBulk registers a bulk handler for a queue.
// The bulk handler will be used when bulk mode is enabled.
// The individual handler (from Register) serves as fallback.
func (cr *ConsumerRoutes) RegisterBulk(queueName string, handler BulkHandlerFunc) {
	cr.bulkRoutes[queueName] = handler
}

// RunConsumers init consume for all registry queues.
func (cr *ConsumerRoutes) RunConsumers() error {
	for queueName, handler := range cr.routes {
		cr.Log(context.Background(), libLog.LevelInfo, "Initializing consumer for queue", libLog.String("queue", queueName))

		// Check if bulk mode should be used for this queue
		bulkHandler, hasBulkHandler := cr.bulkRoutes[queueName]
		useBulkMode := cr.IsBulkModeEnabled() && hasBulkHandler

		cr.logConsumerMode(queueName, useBulkMode, hasBulkHandler)

		go cr.runConsumerLoop(queueName, handler, bulkHandler, useBulkMode)
	}

	return nil
}

// logConsumerMode logs the processing mode for a queue.
func (cr *ConsumerRoutes) logConsumerMode(queueName string, useBulkMode, hasBulkHandler bool) {
	if useBulkMode {
		cr.Log(context.Background(), libLog.LevelInfo, "Bulk mode ENABLED for queue",
			libLog.String("queue", queueName),
			libLog.Int("bulk_size", cr.bulkConfig.Size),
			libLog.Any("flush_timeout", cr.bulkConfig.FlushTimeout),
		)
	} else {
		cr.Log(context.Background(), libLog.LevelInfo, "Individual processing mode for queue",
			libLog.String("queue", queueName),
			libLog.Bool("bulk_config_enabled", cr.IsBulkModeEnabled()),
			libLog.Bool("has_bulk_handler", hasBulkHandler),
		)
	}
}

// runConsumerLoop runs the main consumer loop for a queue with automatic reconnection.
func (cr *ConsumerRoutes) runConsumerLoop(queueName string, handler QueueHandlerFunc, bulkHandler BulkHandlerFunc, useBulkMode bool) {
	backoff := utils.InitialBackoff
	bgCtx := context.Background()

	for {
		messages, shouldRetry := cr.setupChannelAndConsume(bgCtx, queueName, &backoff)
		if shouldRetry {
			continue
		}

		cr.Log(bgCtx, libLog.LevelInfo, "consuming started", libLog.String("queue", queueName))

		backoff = utils.InitialBackoff

		notifyClose := make(chan *amqp.Error, 1)
		cr.conn.Channel.NotifyClose(notifyClose)

		cr.startWorkers(bgCtx, queueName, handler, bulkHandler, useBulkMode, messages)

		cr.waitForChannelClose(bgCtx, queueName, notifyClose)
	}
}

// setupChannelAndConsume sets up the channel and starts consuming.
// Returns the messages channel and whether retry is needed.
func (cr *ConsumerRoutes) setupChannelAndConsume(ctx context.Context, queueName string, backoff *time.Duration) (<-chan amqp.Delivery, bool) {
	if err := cr.conn.EnsureChannel(); err != nil {
		cr.logAndSleep(ctx, "failed to ensure channel", "retrying EnsureChannel", queueName, err, backoff)

		return nil, true
	}

	if err := cr.conn.Channel.Qos(cr.NumbersOfPrefetch, 0, false); err != nil {
		cr.logAndSleep(ctx, "failed to set QoS", "retrying QoS", queueName, err, backoff)

		return nil, true
	}

	messages, err := cr.conn.Channel.Consume(queueName, "", false, false, false, false, nil)
	if err != nil {
		cr.logAndSleep(ctx, "failed to start consuming", "retrying Consume", queueName, err, backoff)

		return nil, true
	}

	return messages, false
}

// logAndSleep logs an error, sleeps with backoff, and updates the backoff value.
func (cr *ConsumerRoutes) logAndSleep(ctx context.Context, errMsg, retryMsg, queueName string, err error, backoff *time.Duration) {
	cr.Log(ctx, libLog.LevelError, errMsg, libLog.String("queue", queueName), libLog.Err(err))

	sleepDuration := utils.FullJitter(*backoff)
	cr.Log(ctx, libLog.LevelInfo, retryMsg, libLog.String("queue", queueName), libLog.Any("sleepDuration", sleepDuration))
	time.Sleep(sleepDuration)

	*backoff = utils.NextBackoff(*backoff)
}

// startWorkers starts the appropriate workers based on bulk mode configuration.
func (cr *ConsumerRoutes) startWorkers(ctx context.Context, queueName string, handler QueueHandlerFunc, bulkHandler BulkHandlerFunc, useBulkMode bool, messages <-chan amqp.Delivery) {
	if useBulkMode {
		// Start a single bulk worker that uses BulkCollector
		go cr.startBulkWorker(ctx, queueName, handler, bulkHandler, messages)
	} else {
		// Start individual workers (original behavior)
		for i := 0; i < cr.NumbersOfWorkers; i++ {
			go cr.startWorker(i, queueName, handler, messages)
		}
	}
}

// waitForChannelClose waits for the channel to close and logs the event.
func (cr *ConsumerRoutes) waitForChannelClose(ctx context.Context, queueName string, notifyClose <-chan *amqp.Error) {
	if errClose := <-notifyClose; errClose != nil {
		cr.Log(ctx, libLog.LevelWarn, "channel closed", libLog.String("queue", queueName), libLog.Err(errClose))
	} else {
		cr.Log(ctx, libLog.LevelWarn, "channel closed: no error info", libLog.String("queue", queueName))
	}

	cr.Log(ctx, libLog.LevelWarn, "restarting consumer", libLog.String("queue", queueName))
}

// startWorker starts a worker that processes messages from the queue.
func (cr *ConsumerRoutes) startWorker(workerID int, queue string, handlerFunc QueueHandlerFunc, messages <-chan amqp.Delivery) {
	for msg := range messages {
		midazID := resolveMessageHeaderID(msg.Headers)

		log := cr.With(
			libLog.String(libConstants.HeaderID, midazID),
		)

		ctx := libCommons.ContextWithLogger(
			libCommons.ContextWithHeaderID(context.Background(), midazID),
			log,
		)

		ctx = libCommons.ContextWithHeaderID(ctx, midazID)
		ctx = libOpentelemetry.ExtractTraceContextFromQueueHeaders(ctx, msg.Headers)

		logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)
		ctx, spanConsumer := tracer.Start(ctx, "rabbitmq.consumer.process_message")

		ctx = libCommons.ContextWithSpanAttributes(ctx, attribute.String("app.request.request_id", reqId))

		err := libOpentelemetry.SetSpanAttributesFromValue(spanConsumer, "app.request.rabbitmq.consumer.message", msg.Body, nil)
		if err != nil {
			libOpentelemetry.HandleSpanError(spanConsumer, "Failed to convert message to JSON string", err)
		}

		err = handlerFunc(ctx, msg.Body)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanConsumer, "Error processing message from queue", err)
			spanConsumer.End()
			logger.Log(ctx, libLog.LevelError, "Error processing message from queue", libLog.Int("workerID", workerID), libLog.String("queue", queue), libLog.Err(err))

			_ = msg.Nack(false, true)

			continue
		}

		spanConsumer.End()

		_ = msg.Ack(false)
	}

	cr.Log(context.Background(), libLog.LevelWarn, "worker stopped (channel closed)", libLog.String("queue", queue), libLog.Int("workerID", workerID))
}

// startBulkWorker starts a bulk worker that uses BulkCollector to accumulate messages.
// Messages are processed in batches for improved throughput.
// Falls back to individual processing on bulk failure if fallback is enabled.
func (cr *ConsumerRoutes) startBulkWorker(
	ctx context.Context,
	queue string,
	individualHandler QueueHandlerFunc,
	bulkHandler BulkHandlerFunc,
	messages <-chan amqp.Delivery,
) {
	collector := NewBulkCollector(cr.bulkConfig.Size, cr.bulkConfig.FlushTimeout)

	// Set the flush callback that processes the bulk
	collector.SetFlushCallback(func(flushCtx context.Context, deliveries []amqp.Delivery) error {
		return cr.processBulkFlush(flushCtx, queue, deliveries, bulkHandler)
	})

	// Set error handler for fallback processing
	if cr.bulkConfig.FallbackEnabled {
		collector.SetFlushErrorHandler(func(errCtx context.Context, deliveries []amqp.Delivery, err error) {
			cr.Log(errCtx, libLog.LevelWarn, "Bulk processing failed, using fallback",
				libLog.String("queue", queue),
				libLog.Int("message_count", len(deliveries)),
				libLog.Err(err),
			)
			cr.processFallback(errCtx, queue, deliveries, individualHandler)
		})
	}

	// Start a goroutine to feed messages to the collector
	go func() {
		for msg := range messages {
			if err := collector.Add(msg); err != nil {
				cr.Log(ctx, libLog.LevelError, "Failed to add message to bulk collector",
					libLog.String("queue", queue),
					libLog.Err(err),
				)
				// If we can't add to collector, process individually as fallback
				if cr.bulkConfig.FallbackEnabled {
					cr.processIndividualMessage(ctx, queue, msg, individualHandler)
				} else {
					_ = msg.Nack(false, true)
				}
			}
		}

		// Channel closed, stop the collector (this will trigger final flush)
		collector.Stop()
	}()

	// Run the collector's main loop (blocks until context cancelled or stopped)
	if err := collector.Start(ctx); err != nil {
		cr.Log(ctx, libLog.LevelWarn, "Bulk collector stopped",
			libLog.String("queue", queue),
			libLog.Err(err),
		)
	}

	cr.Log(ctx, libLog.LevelWarn, "bulk worker stopped (channel closed)", libLog.String("queue", queue))
}

// processBulkFlush processes a batch of messages using the bulk handler.
// On success, acknowledges all messages with a single bulk ack.
// Returns error if bulk processing fails (error handler will be called for fallback).
func (cr *ConsumerRoutes) processBulkFlush(
	ctx context.Context,
	queue string,
	deliveries []amqp.Delivery,
	bulkHandler BulkHandlerFunc,
) error {
	if len(deliveries) == 0 {
		return nil
	}

	startTime := time.Now()

	// Build context with trace information from first message
	bulkCtx := cr.buildBulkContext(ctx, deliveries)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(bulkCtx)

	bulkCtx, span := tracer.Start(bulkCtx, "rabbitmq.consumer.process_bulk")
	defer span.End()

	span.SetAttributes(
		attribute.Int("bulk.size", len(deliveries)),
		attribute.String("bulk.queue", queue),
	)

	logger.Log(bulkCtx, libLog.LevelInfo, "Processing bulk",
		libLog.String("queue", queue),
		libLog.Int("message_count", len(deliveries)),
	)

	// Call the bulk handler
	results, err := bulkHandler(bulkCtx, deliveries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Bulk processing failed", err)
		logger.Log(bulkCtx, libLog.LevelError, "Bulk processing failed",
			libLog.String("queue", queue),
			libLog.Int("message_count", len(deliveries)),
			libLog.Err(err),
		)

		return err
	}

	// Process results and acknowledge messages
	cr.acknowledgeByResults(bulkCtx, deliveries, results, logger, queue)

	duration := time.Since(startTime)
	span.SetAttributes(attribute.Float64("bulk.duration_ms", float64(duration.Milliseconds())))

	logger.Log(bulkCtx, libLog.LevelInfo, "Bulk processing completed",
		libLog.String("queue", queue),
		libLog.Int("message_count", len(deliveries)),
		libLog.Any("duration", duration),
	)

	return nil
}

// acknowledgeByResults acknowledges messages based on processing results.
// Uses bulk ack if all messages succeeded, otherwise individual ack/nack.
func (cr *ConsumerRoutes) acknowledgeByResults(
	ctx context.Context,
	deliveries []amqp.Delivery,
	results []BulkMessageResult,
	logger libLog.Logger,
	queue string,
) {
	// If no results provided, assume all succeeded (backward compatible)
	if len(results) == 0 {
		// Bulk ack on the last message (acknowledges all messages up to this delivery tag)
		lastMsg := deliveries[len(deliveries)-1]
		if err := lastMsg.Ack(true); err != nil {
			logger.Log(ctx, libLog.LevelError, "Failed to bulk ack messages",
				libLog.String("queue", queue),
				libLog.Int("message_count", len(deliveries)),
				libLog.Err(err),
			)
		}

		return
	}

	// Check if all messages succeeded
	allSucceeded := true

	for _, result := range results {
		if !result.Success {
			allSucceeded = false

			break
		}
	}

	if allSucceeded {
		// Bulk ack on the last message
		lastMsg := deliveries[len(deliveries)-1]
		if err := lastMsg.Ack(true); err != nil {
			logger.Log(ctx, libLog.LevelError, "Failed to bulk ack messages",
				libLog.String("queue", queue),
				libLog.Int("message_count", len(deliveries)),
				libLog.Err(err),
			)
		}

		return
	}

	// Mixed results: individual ack/nack
	resultMap := make(map[int]BulkMessageResult, len(results))
	for _, r := range results {
		resultMap[r.Index] = r
	}

	for i, delivery := range deliveries {
		result, hasResult := resultMap[i]
		if hasResult && !result.Success {
			// Failed: nack with requeue
			if err := delivery.Nack(false, true); err != nil {
				logger.Log(ctx, libLog.LevelError, "Failed to nack message",
					libLog.String("queue", queue),
					libLog.Int("index", i),
					libLog.Err(err),
				)
			}
		} else {
			// Succeeded or no result (treat as success): ack
			if err := delivery.Ack(false); err != nil {
				logger.Log(ctx, libLog.LevelError, "Failed to ack message",
					libLog.String("queue", queue),
					libLog.Int("index", i),
					libLog.Err(err),
				)
			}
		}
	}
}

// buildBulkContext creates a context for bulk processing with trace information.
func (cr *ConsumerRoutes) buildBulkContext(ctx context.Context, deliveries []amqp.Delivery) context.Context {
	if len(deliveries) == 0 {
		return ctx
	}

	// Use the first message's trace context as the parent
	firstMsg := deliveries[0]
	midazID := resolveMessageHeaderID(firstMsg.Headers)

	log := cr.With(
		libLog.String(libConstants.HeaderID, midazID),
		libLog.Int("bulk_size", len(deliveries)),
	)

	bulkCtx := libCommons.ContextWithLogger(
		libCommons.ContextWithHeaderID(ctx, midazID),
		log,
	)

	bulkCtx = libOpentelemetry.ExtractTraceContextFromQueueHeaders(bulkCtx, firstMsg.Headers)

	return bulkCtx
}

// processFallback processes messages individually when bulk processing fails.
func (cr *ConsumerRoutes) processFallback(
	ctx context.Context,
	queue string,
	deliveries []amqp.Delivery,
	individualHandler QueueHandlerFunc,
) {
	logger := cr.Logger

	logger.Log(ctx, libLog.LevelInfo, "Starting fallback processing",
		libLog.String("queue", queue),
		libLog.Int("message_count", len(deliveries)),
	)

	for _, delivery := range deliveries {
		cr.processIndividualMessage(ctx, queue, delivery, individualHandler)
	}

	logger.Log(ctx, libLog.LevelInfo, "Fallback processing completed",
		libLog.String("queue", queue),
		libLog.Int("message_count", len(deliveries)),
	)
}

// processIndividualMessage processes a single message using the individual handler.
func (cr *ConsumerRoutes) processIndividualMessage(
	ctx context.Context,
	queue string,
	msg amqp.Delivery,
	handler QueueHandlerFunc,
) {
	midazID := resolveMessageHeaderID(msg.Headers)

	log := cr.With(
		libLog.String(libConstants.HeaderID, midazID),
	)

	msgCtx := libCommons.ContextWithLogger(
		libCommons.ContextWithHeaderID(ctx, midazID),
		log,
	)

	msgCtx = libOpentelemetry.ExtractTraceContextFromQueueHeaders(msgCtx, msg.Headers)

	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(msgCtx)

	msgCtx, span := tracer.Start(msgCtx, "rabbitmq.consumer.process_message_fallback")
	defer span.End()

	msgCtx = libCommons.ContextWithSpanAttributes(msgCtx, attribute.String("app.request.request_id", reqID))

	err := handler(msgCtx, msg.Body)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Error processing message (fallback)", err)
		logger.Log(msgCtx, libLog.LevelError, "Error processing message (fallback)",
			libLog.String("queue", queue),
			libLog.Err(err),
		)

		_ = msg.Nack(false, true)

		return
	}

	_ = msg.Ack(false)
}
