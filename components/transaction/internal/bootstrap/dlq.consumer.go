// File: components/transaction/internal/bootstrap/dlq.consumer.go
package bootstrap

import (
	"context"
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

	// dlqQueueSuffix is the suffix for Dead Letter Queue names.
	dlqQueueSuffix = ".dlq"

	// dlqPollInterval is how often to check DLQ for new messages.
	dlqPollInterval = 10 * time.Second
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
	}

	return true
}

// processQueue processes messages from a single DLQ.
func (d *DLQConsumer) processQueue(ctx context.Context, dlqName, originalQueue string) {
	// Implementation in Task 1.3
	d.Logger.Debugf("DLQ_PROCESS_QUEUE: Processing %s for replay to %s", dlqName, originalQueue)
}
