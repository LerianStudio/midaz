// File: components/transaction/internal/bootstrap/dlq.consumer.go
package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
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

	// healthCheckInterval is how often to poll infrastructure health before replaying.
	healthCheckInterval = 30 * time.Second

	// healthCheckTimeout is the maximum time to wait for health check responses.
	healthCheckTimeout = 5 * time.Second

	// dlqQueueSuffix is the suffix for Dead Letter Queue names.
	dlqQueueSuffix = ".dlq"

	// dlqPollInterval is how often to check DLQ for new messages.
	dlqPollInterval = 10 * time.Second

	// publishConfirmTimeout is the maximum time to wait for RabbitMQ publish confirmation.
	publishConfirmTimeout = 10 * time.Second
)

// DLQ header constants
const (
	dlqRetryCountHeader    = "x-dlq-retry-count"
	dlqOriginalQueueHeader = "x-dlq-original-queue"
	dlqReasonHeader        = "x-dlq-reason"
	dlqTimestampHeader     = "x-dlq-timestamp"
)

// DLQConsumer processes messages from Dead Letter Queues after infrastructure recovery.
// It monitors DLQ queues, checks infrastructure health, and replays messages
// to their original queues for reprocessing.
type DLQConsumer struct {
	Logger           libLog.Logger
	RabbitMQConn     *libRabbitmq.RabbitMQConnection
	PostgresConn     *libPostgres.PostgresConnection
	RedisConn        *libRedis.RedisConnection
	QueueNames       []string // Original queue names (DLQ names derived by adding suffix)
	originalHandlers map[string]func(ctx context.Context, body []byte) error
}

// NewDLQConsumer creates a new DLQ consumer instance.
func NewDLQConsumer(
	logger libLog.Logger,
	rabbitMQConn *libRabbitmq.RabbitMQConnection,
	postgresConn *libPostgres.PostgresConnection,
	redisConn *libRedis.RedisConnection,
	queueNames []string,
) *DLQConsumer {
	return &DLQConsumer{
		Logger:           logger,
		RabbitMQConn:     rabbitMQConn,
		PostgresConn:     postgresConn,
		RedisConn:        redisConn,
		QueueNames:       queueNames,
		originalHandlers: make(map[string]func(ctx context.Context, body []byte) error),
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

	// Check PostgreSQL
	if d.PostgresConn != nil {
		db, err := d.PostgresConn.GetDB()
		if err != nil {
			d.Logger.Warnf("DLQ_HEALTH_CHECK: PostgreSQL connection failed: %v", err)
			return false
		}
		if err := db.PingContext(ctx); err != nil {
			d.Logger.Warnf("DLQ_HEALTH_CHECK: PostgreSQL unhealthy: %v", err)
			return false
		}
		hasHealthyInfra = true
	}

	// Check Redis
	if d.RedisConn != nil {
		rds, err := d.RedisConn.GetClient(ctx)
		if err != nil {
			d.Logger.Warnf("DLQ_HEALTH_CHECK: Redis connection failed: %v", err)
			return false
		}
		if err := rds.Ping(ctx).Err(); err != nil {
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

// processQueue processes messages from a single DLQ.
func (d *DLQConsumer) processQueue(_ context.Context, dlqName, originalQueue string) {
	// TODO(task-1.3): Implement DLQ message replay logic
	d.Logger.Debugf("DLQ_PROCESS_QUEUE: Processing %s for replay to %s", dlqName, originalQueue)
}

// getDLQRetryCount extracts the DLQ retry count from message headers.
// Returns 0 if header is missing or invalid.
func getDLQRetryCount(headers map[string]interface{}) int {
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
func getOriginalQueue(headers map[string]interface{}) string {
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
		// Ack to remove from DLQ - message is lost but we prevent infinite loops
		if err := msg.Ack(false); err != nil {
			logger.Warnf("DLQ_REPLAY_FAILED: Failed to ack expired DLQ message: %v", err)
		}
		return fmt.Errorf("max DLQ retries exceeded: %d/%d", dlqRetryCount, dlqMaxRetries)
	}

	// Prepare headers for replay - reset regular retry count, increment DLQ retry count
	headers := make(amqp.Table)
	for k, v := range msg.Headers {
		headers[k] = v
	}
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
	select {
	case confirmation, ok := <-confirms:
		if !ok {
			logger.Errorf("DLQ_REPLAY_FAILED: Confirmation channel closed")
			return fmt.Errorf("confirmation channel closed during DLQ replay")
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
		return fmt.Errorf("broker NACK'd DLQ replay message")

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
