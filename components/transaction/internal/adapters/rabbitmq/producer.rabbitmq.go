package rabbitmq

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libConstants "github.com/LerianStudio/lib-commons/v3/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	tenantmanager "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager"
	libRabbitmq "github.com/LerianStudio/lib-commons/v3/commons/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/trace"
)

// ProducerRepository provides an interface for Producer related to rabbitmq.
// // It defines methods for sending messages to a queue.
type ProducerRepository interface {
	ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error)
	CheckRabbitMQHealth() bool
}

// ProducerRabbitMQRepository is a rabbitmq implementation of the producer
type ProducerRabbitMQRepository struct {
	conn            *libRabbitmq.RabbitMQConnection
	rabbitMQPool    *tenantmanager.RabbitMQManager
	multiTenantMode bool
}

// NewProducerRabbitMQ returns a new instance of ProducerRabbitMQRepository using the given rabbitmq connection.
// For single-tenant mode, pass a connection and nil for the pool.
// For multi-tenant mode, pass nil for connection and a valid pool.
func NewProducerRabbitMQ(c *libRabbitmq.RabbitMQConnection) *ProducerRabbitMQRepository {
	prmq := &ProducerRabbitMQRepository{
		conn:            c,
		multiTenantMode: false,
	}

	_, err := c.GetNewConnect()
	if err != nil {
		panic("Failed to connect rabbitmq")
	}

	return prmq
}

// NewProducerRabbitMQMultiTenant returns a new instance of ProducerRabbitMQRepository for multi-tenant mode.
// Uses RabbitMQManager to get tenant-specific connections.
func NewProducerRabbitMQMultiTenant(pool *tenantmanager.RabbitMQManager) *ProducerRabbitMQRepository {
	return &ProducerRabbitMQRepository{
		rabbitMQPool:    pool,
		multiTenantMode: true,
	}
}

// CheckRabbitMQHealth checks the health of the rabbitmq connection.
// In multi-tenant mode, this checks if the pool is available and not closed.
// In single-tenant mode, this checks the static connection health.
func (prmq *ProducerRabbitMQRepository) CheckRabbitMQHealth() bool {
	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "false" {
		return true
	}

	// In multi-tenant mode, check pool availability
	if prmq.multiTenantMode {
		if prmq.rabbitMQPool == nil {
			return false
		}
		// Pool is available and not closed
		stats := prmq.rabbitMQPool.Stats()
		return !stats.Closed
	}

	// Single-tenant mode: check static connection
	return prmq.conn.HealthCheck()
}

// ProducerDefault sends a message to a RabbitMQ queue for further processing.
// In multi-tenant mode, it uses the RabbitMQ pool to get a tenant-specific channel.
// In single-tenant mode, it uses the static connection.
func (prmq *ProducerRabbitMQRepository) ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	logger.Infof("Init sent message to exchange: %s, key: %s", exchange, key)

	ctx, spanProducer := tracer.Start(ctx, "rabbitmq.producer.publish_message")
	defer spanProducer.End()

	var err error

	backoff := utils.InitialBackoff

	headers := amqp.Table{
		libConstants.HeaderID: reqId,
	}

	// Inject tenant ID if available in context (multi-tenant mode)
	tenantID := tenantmanager.GetTenantID(ctx)
	if tenantID != "" {
		headers["X-Tenant-ID"] = tenantID
	}

	libOpentelemetry.InjectTraceHeadersIntoQueue(ctx, (*map[string]any)(&headers))

	// Multi-tenant mode: use RabbitMQ pool to get tenant-specific channel
	if prmq.multiTenantMode {
		return prmq.publishMultiTenant(ctx, exchange, key, message, headers, tenantID, logger, &spanProducer)
	}

	// Single-tenant mode: use static connection
	for attempt := 0; attempt <= utils.MaxRetries; attempt++ {
		if err = prmq.conn.EnsureChannel(); err != nil {
			logger.Errorf("Failed to reopen channel: %v", err)

			sleepDuration := utils.FullJitter(backoff)
			logger.Infof("Retrying to reconnect in %v...", sleepDuration)
			time.Sleep(sleepDuration)

			backoff = utils.NextBackoff(backoff)

			continue
		}

		err = prmq.conn.Channel.Publish(
			exchange,
			key,
			false,
			false,
			amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Headers:      headers,
				Body:         message,
			},
		)
		if err == nil {
			logger.Infof("Messages sent successfully to exchange: %s, key: %s", exchange, key)

			return nil, nil
		}

		logger.Warnf("Failed to publish message to exchange: %s, key: %s, attempt %d/%d: %s", exchange, key, attempt+1, utils.MaxRetries+1, err)

		if attempt == utils.MaxRetries {
			libOpentelemetry.HandleSpanError(&spanProducer, "Failed to publish message after retries", err)

			logger.Errorf("Giving up after %d attempts: %s", utils.MaxRetries+1, err)

			return nil, err
		}

		sleepDuration := utils.FullJitter(backoff)
		logger.Infof("Retrying to publish message in %v (attempt %d)...", sleepDuration, attempt+2)
		time.Sleep(sleepDuration)

		backoff = utils.NextBackoff(backoff)
	}

	return nil, err
}

// publishMultiTenant publishes a message using tenant-specific RabbitMQ connection from the pool.
func (prmq *ProducerRabbitMQRepository) publishMultiTenant(
	ctx context.Context,
	exchange, key string,
	message []byte,
	headers amqp.Table,
	tenantID string,
	logger libLog.Logger,
	spanProducer *trace.Span,
) (*string, error) {
	if tenantID == "" {
		err := fmt.Errorf("tenant ID is required in multi-tenant mode")
		libOpentelemetry.HandleSpanError(spanProducer, "tenant ID missing", err)
		return nil, err
	}

	if prmq.rabbitMQPool == nil {
		err := fmt.Errorf("RabbitMQ pool is not initialized")
		libOpentelemetry.HandleSpanError(spanProducer, "pool not initialized", err)
		return nil, err
	}

	logger.Infof("Multi-tenant mode: getting channel for tenant: %s", tenantID)

	backoff := utils.InitialBackoff

	for attempt := 0; attempt <= utils.MaxRetries; attempt++ {
		// Get tenant-specific channel from pool
		channel, err := prmq.rabbitMQPool.GetChannel(ctx, tenantID)
		if err != nil {
			logger.Errorf("Failed to get channel for tenant %s: %v", tenantID, err)

			sleepDuration := utils.FullJitter(backoff)
			logger.Infof("Retrying to get channel in %v...", sleepDuration)
			time.Sleep(sleepDuration)

			backoff = utils.NextBackoff(backoff)

			continue
		}

		// Publish message to tenant's vhost
		err = channel.Publish(
			exchange,
			key,
			false,
			false,
			amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Headers:      headers,
				Body:         message,
			},
		)

		// Close the channel after use (channels are cheap, connections are expensive)
		if closeErr := channel.Close(); closeErr != nil {
			logger.Warnf("Failed to close channel: %v", closeErr)
		}

		if err == nil {
			logger.Infof("Messages sent successfully to tenant %s, exchange: %s, key: %s", tenantID, exchange, key)
			return nil, nil
		}

		logger.Warnf("Failed to publish message to tenant %s, exchange: %s, key: %s, attempt %d/%d: %s",
			tenantID, exchange, key, attempt+1, utils.MaxRetries+1, err)

		if attempt == utils.MaxRetries {
			libOpentelemetry.HandleSpanError(spanProducer, "Failed to publish message after retries", err)
			logger.Errorf("Giving up after %d attempts: %s", utils.MaxRetries+1, err)
			return nil, err
		}

		sleepDuration := utils.FullJitter(backoff)
		logger.Infof("Retrying to publish message in %v (attempt %d)...", sleepDuration, attempt+2)
		time.Sleep(sleepDuration)

		backoff = utils.NextBackoff(backoff)
	}

	return nil, fmt.Errorf("failed to publish message after %d attempts", utils.MaxRetries+1)
}
