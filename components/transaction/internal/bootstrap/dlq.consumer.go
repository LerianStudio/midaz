// File: components/transaction/internal/bootstrap/dlq.consumer.go
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mruntime"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
)

const (
	// dlqInitialBackoff is the initial delay before first DLQ message replay attempt.
	// Much longer than regular retries because infrastructure should have recovered.
	dlqInitialBackoff = 1 * time.Minute

	// dlqMaxBackoff is the maximum delay between DLQ replay attempts.
	dlqMaxBackoff = 30 * time.Minute

	// dlqMaxRetries is the maximum number of DLQ replay attempts per message.
	// Higher than regular maxRetries (4) because infrastructure should be stable.
	dlqMaxRetries = 10

	// healthCheckTimeout is the maximum time to wait for health check responses.
	healthCheckTimeout = 5 * time.Second

	// dlqQueueSuffix is the suffix for Dead Letter Queue names.
	dlqQueueSuffix = ".dlq"

	// dlqPollInterval is how often to check DLQ for new messages.
	dlqPollInterval = 10 * time.Second

	// publishConfirmTimeout is the maximum time to wait for RabbitMQ publish confirmation.
	publishConfirmTimeout = 10 * time.Second

	// dlqBatchSize is the number of messages to process in each DLQ batch
	dlqBatchSize = 10

	// dlqPrefetchCount is the RabbitMQ prefetch for DLQ consumer
	dlqPrefetchCount = 10
)

// DLQ header constants
const (
	dlqRetryCountHeader    = "x-dlq-retry-count"
	dlqOriginalQueueHeader = "x-dlq-original-queue"
	dlqReasonHeader        = "x-dlq-reason"
	dlqTimestampHeader     = "x-dlq-timestamp"
)

// dlqSafeHeadersAllowlist defines headers safe to replay (M7: Security - header sanitization)
var dlqSafeHeadersAllowlist = map[string]bool{
	"x-correlation-id":     true,
	"x-midaz-header-id":    true,
	"content-type":         true,
	"x-dlq-retry-count":    true,
	"x-dlq-original-queue": true,
	"x-dlq-timestamp":      true,
	"x-dlq-reason":         true,
	"x-dlq-error-type":     true,
}

// DLQConsumer processes messages from Dead Letter Queues after infrastructure recovery.
// It monitors DLQ queues, checks infrastructure health, and replays messages
// to their original queues for reprocessing.
type DLQConsumer struct {
	Logger              libLog.Logger
	RabbitMQConn        *libRabbitmq.RabbitMQConnection
	PostgresConn        *libPostgres.PostgresConnection
	RedisConn           *libRedis.RedisConnection
	QueueNames          []string        // Original queue names (DLQ names derived by adding suffix)
	validOriginalQueues map[string]bool // H8: Allowlist for security - prevent queue name injection
}

// NewDLQConsumer creates a new DLQ consumer instance.
func NewDLQConsumer(
	logger libLog.Logger,
	rabbitMQConn *libRabbitmq.RabbitMQConnection,
	postgresConn *libPostgres.PostgresConnection,
	redisConn *libRedis.RedisConnection,
	queueNames []string,
) *DLQConsumer {
	// M6: Validate empty QueueNames array
	if len(queueNames) == 0 {
		logger.Warn("DLQ_CONSUMER_INIT: No queue names provided, DLQ consumer will not process any queues")
	}

	// H8: Initialize allowlist for queue name validation (security)
	validQueues := make(map[string]bool, len(queueNames))
	for _, q := range queueNames {
		validQueues[q] = true
	}

	return &DLQConsumer{
		Logger:              logger,
		RabbitMQConn:        rabbitMQConn,
		PostgresConn:        postgresConn,
		RedisConn:           redisConn,
		QueueNames:          queueNames,
		validOriginalQueues: validQueues,
	}
}

// Run starts the DLQ consumer loop.
func (d *DLQConsumer) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d.Logger.Info("DLQConsumer started - monitoring Dead Letter Queues for message replay")

	ticker := time.NewTicker(dlqPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.Logger.Info("DLQConsumer: shutting down...")
			return nil

		case <-ticker.C:
			d.processDLQMessages(ctx)
		}
	}
}

// processDLQMessages checks all DLQ queues and replays messages if infrastructure is healthy.
func (d *DLQConsumer) processDLQMessages(ctx context.Context) {
	// Check infrastructure health before attempting replay
	if !d.isInfrastructureHealthy(ctx) {
		d.Logger.Debug("DLQ_HEALTH_CHECK_FAILED: Infrastructure not ready, skipping DLQ processing")
		return
	}

	for _, queueName := range d.QueueNames {
		dlqName := queueName + dlqQueueSuffix

		mruntime.SafeGoWithContextAndComponent(ctx, d.Logger, "transaction", "dlq_consumer_"+dlqName, mruntime.KeepRunning, func(ctx context.Context) {
			d.processQueue(ctx, dlqName, queueName)
		})
	}
}

// isInfrastructureHealthy checks if PostgreSQL and Redis are available.
func (d *DLQConsumer) isInfrastructureHealthy(ctx context.Context) bool {
	hasHealthyInfra := false

	// H3: Apply timeout to health checks
	healthCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	// Check PostgreSQL
	if d.PostgresConn != nil {
		db, err := d.PostgresConn.GetDB()
		if err != nil {
			d.Logger.Warnf("DLQ_HEALTH_CHECK: PostgreSQL connection failed: %v", err)
			return false
		}

		if err := db.PingContext(healthCtx); err != nil {
			d.Logger.Warnf("DLQ_HEALTH_CHECK: PostgreSQL unhealthy: %v", err)
			return false
		}

		hasHealthyInfra = true
	}

	// Check Redis
	if d.RedisConn != nil {
		rds, err := d.RedisConn.GetClient(healthCtx)
		if err != nil {
			d.Logger.Warnf("DLQ_HEALTH_CHECK: Redis connection failed: %v", err)
			return false
		}

		if err := rds.Ping(healthCtx).Err(); err != nil {
			d.Logger.Warnf("DLQ_HEALTH_CHECK: Redis unhealthy: %v", err)
			return false
		}

		hasHealthyInfra = true
	}

	if !hasHealthyInfra {
		d.Logger.Warn("DLQ_HEALTH_CHECK: No infrastructure connections available")
	}

	return hasHealthyInfra
}

// calculateDLQBackoff returns the delay before the next DLQ replay attempt.
// Uses tiered backoff with predefined intervals: 1min, 5min, 15min, 30min (capped).
// This is longer than regular retry backoff because DLQ processing
// happens after infrastructure recovery.
func calculateDLQBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return dlqInitialBackoff
	}

	// Tiered backoff with predefined intervals
	// Attempt 1: 1min, 2: 5min, 3: 15min, 4+: 30min (max)
	// TODO(review): Consider extracting backoff tier durations to named constants (Low severity - code style)
	switch attempt {
	case 1:
		return 1 * time.Minute
	case 2:
		return 5 * time.Minute
	case 3:
		return 15 * time.Minute
	default:
		return dlqMaxBackoff
	}
}

// processQueue processes messages from a single DLQ with backoff between retries.
func (d *DLQConsumer) processQueue(ctx context.Context, dlqName, originalQueue string) {
	// H6: Set up proper context with header ID before calling NewTrackingFromContext
	correlationID := libCommons.GenerateUUIDv7().String()

	log := d.Logger.WithFields(
		libConstants.HeaderID, correlationID,
	).WithDefaultMessageTemplate(correlationID + " | ")

	ctx = libCommons.ContextWithLogger(
		libCommons.ContextWithHeaderID(ctx, correlationID),
		log,
	)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "dlq.consumer.process_queue")
	defer span.End()

	span.SetAttributes(
		attribute.String("dlq.queue_name", dlqName),
		attribute.String("dlq.original_queue", originalQueue),
	)

	// H7: Create dedicated channel for this goroutine (fixes race condition)
	ch, err := d.RabbitMQConn.Connection.Channel()
	if err != nil {
		logger.Errorf("DLQ_PROCESS_QUEUE: Failed to get channel for %s: %v", dlqName, err)
		return
	}
	defer ch.Close()

	// Declare DLQ if it doesn't exist (idempotent)
	_, err = ch.QueueDeclare(
		dlqName,
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		nil,   // arguments
	)
	if err != nil {
		logger.Errorf("DLQ_PROCESS_QUEUE: Failed to declare %s: %v", dlqName, err)
		return
	}

	// Set QoS for controlled processing
	if err := ch.Qos(dlqPrefetchCount, 0, false); err != nil {
		logger.Errorf("DLQ_PROCESS_QUEUE: Failed to set QoS for %s: %v", dlqName, err)
		return
	}

	// Get messages from DLQ (non-blocking check)
	msgs, err := ch.Consume(
		dlqName,
		"dlq-consumer-"+dlqName, // consumer tag
		false,                   // autoAck
		false,                   // exclusive
		false,                   // noLocal
		false,                   // noWait
		nil,                     // args
	)
	if err != nil {
		logger.Errorf("DLQ_PROCESS_QUEUE: Failed to consume from %s: %v", dlqName, err)
		return
	}

	// H2: Add defer to cancel consumer before returning (resource leak fix)
	defer func() {
		if err := ch.Cancel("dlq-consumer-"+dlqName, false); err != nil {
			logger.Warnf("DLQ_PROCESS_QUEUE: Failed to cancel consumer: %v", err)
		}
	}()

	processed := 0

	for {
		select {
		case <-ctx.Done():
			logger.Infof("DLQ_PROCESS_QUEUE: Context cancelled, processed %d messages from %s", processed, dlqName)
			return

		case msg, ok := <-msgs:
			if !ok {
				logger.Infof("DLQ_PROCESS_QUEUE: Channel closed, processed %d messages from %s", processed, dlqName)
				return
			}

			// Get original queue from headers, fallback to derived name
			origQueue := getOriginalQueue(msg.Headers)
			if origQueue == "" {
				origQueue = originalQueue
			}

			// H8: SECURITY - Validate queue name against allowlist to prevent injection
			if !d.validOriginalQueues[origQueue] {
				logger.Errorf("DLQ_SECURITY_VIOLATION: Invalid original queue in header: %s (expected one of: %v)",
					origQueue, d.QueueNames)
				span.SetAttributes(
					attribute.Bool("dlq.security_violation", true),
					attribute.String("dlq.invalid_queue", origQueue),
				)
				// Ack to remove malicious message from DLQ
				if err := msg.Ack(false); err != nil {
					logger.Warnf("Failed to ack invalid DLQ message: %v", err)
				}

				continue
			}

			dlqRetryCount := getDLQRetryCount(msg.Headers)

			// Check if we should wait (backoff) before replaying
			backoffDuration := calculateDLQBackoff(dlqRetryCount)
			logger.Infof("DLQ_REPLAY_ATTEMPT: queue=%s, dlq_retry=%d, backoff=%v",
				dlqName, dlqRetryCount, backoffDuration)

			// M4: Use helper function for backoff check (reduces complexity)
			if d.shouldDeferReplay(&msg, dlqRetryCount, backoffDuration, logger, dlqName) {
				if err := msg.Nack(false, true); err != nil {
					logger.Warnf("DLQ_REPLAY_DEFERRED: Failed to nack: %v", err)
				}

				continue
			}

			// Replay the message
			if err := d.replayMessageToOriginalQueue(ctx, &msg, origQueue, dlqRetryCount); err != nil {
				logger.Errorf("DLQ_REPLAY_ERROR: Failed to replay message from %s: %v", dlqName, err)
				// Nack with requeue so we retry later
				if nackErr := msg.Nack(false, true); nackErr != nil {
					logger.Warnf("DLQ_REPLAY_ERROR: Failed to nack message: %v", nackErr)
				}

				continue
			}

			processed++
			if processed >= dlqBatchSize {
				logger.Infof("DLQ_PROCESS_QUEUE: Batch complete, processed %d messages from %s", processed, dlqName)
				return
			}

		default:
			// No more messages in queue
			if processed > 0 {
				logger.Infof("DLQ_PROCESS_QUEUE: Completed, processed %d messages from %s", processed, dlqName)
			}

			return
		}
	}
}

// getTimestampHeader extracts the DLQ timestamp from headers.
func getTimestampHeader(headers amqp.Table) int64 {
	if val, ok := headers[dlqTimestampHeader].(int64); ok {
		return val
	}

	return 0
}

// M8: getValidatedTimestamp validates timestamp from headers to prevent manipulation (security)
func getValidatedTimestamp(headers amqp.Table) int64 {
	timestamp := getTimestampHeader(headers)
	if timestamp <= 0 {
		return 0 // Treat as unset
	}

	now := time.Now().Unix()

	// Reject timestamps more than 1 hour in the future (clock skew allowance)
	if timestamp > now+3600 {
		return now // Use current time as fallback
	}

	// Reject timestamps older than 30 days (stale message protection)
	thirtyDaysAgo := now - (30 * 24 * 60 * 60)
	if timestamp < thirtyDaysAgo {
		return thirtyDaysAgo // Cap at 30 days ago
	}

	return timestamp
}

// M4: shouldDeferReplay extracts complex backoff check into helper function
func (d *DLQConsumer) shouldDeferReplay(msg *amqp.Delivery, dlqRetryCount int, backoffDuration time.Duration, logger libLog.Logger, dlqName string) bool {
	if dlqRetryCount == 0 {
		return false // First attempt, replay immediately
	}

	timestamp := getValidatedTimestamp(msg.Headers)
	if timestamp == 0 {
		return false // No timestamp, can't determine elapsed time
	}

	elapsed := time.Since(time.Unix(timestamp, 0))
	if elapsed < backoffDuration {
		logger.Debugf("DLQ_REPLAY_DEFERRED: queue=%s, dlq_retry=%d, elapsed=%v, required=%v",
			dlqName, dlqRetryCount, elapsed, backoffDuration)

		return true
	}

	return false
}

// getDLQRetryCount extracts the DLQ retry count from message headers.
// Returns 0 if header is missing or invalid.
// M3: Use `any` instead of `interface{}`
func getDLQRetryCount(headers map[string]any) int {
	if val, ok := headers[dlqRetryCountHeader].(int32); ok {
		return int(val)
	}

	if val, ok := headers[dlqRetryCountHeader].(int64); ok {
		return int(val)
	}

	return 0
}

// getOriginalQueue extracts the original queue name from DLQ message headers.
// Returns empty string if header is missing.
// M3: Use `any` instead of `interface{}`
func getOriginalQueue(headers map[string]any) string {
	if val, ok := headers[dlqOriginalQueueHeader].(string); ok {
		return val
	}

	return ""
}

// replayMessageToOriginalQueue republishes a DLQ message to its original queue.
// Updates retry count header and logs the replay attempt.
func (d *DLQConsumer) replayMessageToOriginalQueue(ctx context.Context, msg *amqp.Delivery, originalQueue string, dlqRetryCount int) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "dlq.consumer.replay_message")
	defer span.End()

	span.SetAttributes(
		attribute.String("dlq.original_queue", originalQueue),
		attribute.Int("dlq.retry_count", dlqRetryCount),
		attribute.String("dlq.reason", getStringHeader(msg.Headers, dlqReasonHeader)),
	)

	// Check if max DLQ retries exceeded
	if dlqRetryCount >= dlqMaxRetries {
		logger.Errorf("DLQ_REPLAY_FAILED: Max DLQ retries exceeded (%d/%d) - message will be permanently lost - queue=%s",
			dlqRetryCount, dlqMaxRetries, originalQueue)

		// H4: Add span attributes for alerting on permanent message loss
		span.SetAttributes(
			attribute.Bool("dlq.message_lost", true),
			attribute.String("dlq.loss_reason", "max_retries_exceeded"),
			attribute.Int("dlq.retry_count", dlqRetryCount),
			attribute.String("dlq.original_queue", originalQueue),
		)

		// Ack to remove from DLQ - message is lost but we prevent infinite loops
		if err := msg.Ack(false); err != nil {
			logger.Warnf("DLQ_REPLAY_FAILED: Failed to ack expired DLQ message: %v", err)
		}

		return fmt.Errorf("max DLQ retries exceeded: %d/%d", dlqRetryCount, dlqMaxRetries)
	}

	// M7: SECURITY - Prepare headers for replay with allowlist (only copy safe headers)
	headers := make(amqp.Table)

	for k, v := range msg.Headers {
		// Only copy allowlisted headers
		if dlqSafeHeadersAllowlist[k] {
			headers[k] = v
		}
	}

	// TODO(review): Add bounds check for int32 conversion to prevent overflow (gosec G115, Low severity)
	headers[dlqRetryCountHeader] = int32(dlqRetryCount + 1)
	// Reset the regular retry count so message gets fresh retry attempts
	delete(headers, "x-midaz-retry-count")

	ch, err := d.RabbitMQConn.Connection.Channel()
	if err != nil {
		logger.Errorf("DLQ_REPLAY_FAILED: Failed to get channel: %v", err)
		return fmt.Errorf("failed to get channel for DLQ replay: %w", err)
	}
	defer ch.Close()

	// Enable publisher confirms for reliable replay
	if err = ch.Confirm(false); err != nil {
		logger.Errorf("DLQ_REPLAY_FAILED: Failed to enable confirm mode: %v", err)
		return fmt.Errorf("failed to enable confirm mode for DLQ replay: %w", err)
	}

	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	err = ch.Publish(
		"",            // exchange (default)
		originalQueue, // routing key (queue name)
		false,         // mandatory
		false,         // immediate
		amqp.Publishing{
			Headers:      headers,
			Body:         msg.Body,
			ContentType:  msg.ContentType,
			DeliveryMode: amqp.Persistent,
		},
	)
	if err != nil {
		logger.Errorf("DLQ_REPLAY_FAILED: Failed to publish to original queue: %v", err)
		return fmt.Errorf("failed to publish DLQ message to %s: %w", originalQueue, err)
	}

	// Wait for broker confirmation
	// TODO(review): Consider adding ctx.Done() case for faster shutdown response (Low severity - efficiency)
	select {
	case confirmation, ok := <-confirms:
		if !ok {
			logger.Errorf("DLQ_REPLAY_FAILED: Confirmation channel closed")
			return errors.New("confirmation channel closed during DLQ replay")
		}

		if confirmation.Ack {
			logger.Infof("DLQ_REPLAY_SUCCESS: message replayed to %s (DLQ retry %d/%d)",
				originalQueue, dlqRetryCount+1, dlqMaxRetries)
			// Ack the DLQ message since we successfully replayed it
			if err := msg.Ack(false); err != nil {
				logger.Warnf("DLQ_REPLAY_WARNING: Failed to ack DLQ message after successful replay: %v", err)
			}

			return nil
		}

		logger.Errorf("DLQ_REPLAY_FAILED: Broker NACK'd message")

		return errors.New("broker NACK'd DLQ replay message")

	case <-time.After(publishConfirmTimeout):
		logger.Errorf("DLQ_REPLAY_FAILED: Confirmation timeout")
		return fmt.Errorf("DLQ replay confirmation timed out after %v", publishConfirmTimeout)
	}
}

// getStringHeader extracts a string header value from amqp.Table.
func getStringHeader(headers amqp.Table, key string) string {
	if val, ok := headers[key].(string); ok {
		return val
	}

	return ""
}
