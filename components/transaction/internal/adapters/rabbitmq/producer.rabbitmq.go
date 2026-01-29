package rabbitmq

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	amqp "github.com/rabbitmq/amqp091-go"
)

// ProducerRepository provides an interface for Producer related to rabbitmq.
// It defines methods for sending messages to a queue.
type ProducerRepository interface {
	ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error)
	CheckRabbitMQHealth() bool
}

// ProducerRabbitMQRepository is a rabbitmq implementation of the producer
type ProducerRabbitMQRepository struct {
	conn *libRabbitmq.RabbitMQConnection
}

// NewProducerRabbitMQ returns a new instance of ProducerRabbitMQRepository using the given rabbitmq connection.
func NewProducerRabbitMQ(c *libRabbitmq.RabbitMQConnection) *ProducerRabbitMQRepository {
	prmq := &ProducerRabbitMQRepository{
		conn: c,
	}

	_, err := c.GetNewConnect()
	if err != nil {
		panic("Failed to connect rabbitmq")
	}

	return prmq
}

// CheckRabbitMQHealth checks the health of the rabbitmq connection.
func (prmq *ProducerRabbitMQRepository) CheckRabbitMQHealth() bool {
	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "false" {
		return true
	}

	return prmq.conn.HealthCheck()
}

// ProducerDefault sends a message to a RabbitMQ queue for further processing.
func (prmq *ProducerRabbitMQRepository) ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	logger.Infof("Init sent message to exchange: %s, key: %s", exchange, key)

	ctx, spanProducer := tracer.Start(ctx, "rabbitmq.producer.publish_message")
	defer spanProducer.End()

	var err error

	backoff := utils.InitialBackoff()

	headers := amqp.Table{
		libConstants.HeaderID: reqId,
	}

	libOpentelemetry.InjectTraceHeadersIntoQueue(ctx, (*map[string]any)(&headers))

	// Timeout for each connection/publish attempt
	operationTimeout := utils.GetEnvDuration("RABBITMQ_OPERATION_TIMEOUT", 3*time.Second)

	maxRetries := utils.MaxRetries()
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Create a timeout context for this attempt
		attemptCtx, cancel := context.WithTimeout(ctx, operationTimeout)

		// Execute EnsureChannel with timeout
		errChan := make(chan error, 1)

		go func() {
			errChan <- prmq.conn.EnsureChannel()
		}()

		select {
		case err = <-errChan:
			cancel()

			if err != nil {
				logger.Errorf("Failed to reopen channel: %v", err)

				sleepDuration := utils.FullJitter(backoff)
				logger.Infof("Retrying to reconnect in %v...", sleepDuration)
				time.Sleep(sleepDuration)

				backoff = utils.NextBackoff(backoff)

				continue
			}
		case <-attemptCtx.Done():
			cancel()

			err = fmt.Errorf("timeout connecting to RabbitMQ after %v", operationTimeout)
			logger.Errorf("Failed to reopen channel: %v", err)

			sleepDuration := utils.FullJitter(backoff)
			logger.Infof("Retrying to reconnect in %v...", sleepDuration)
			time.Sleep(sleepDuration)

			backoff = utils.NextBackoff(backoff)

			continue
		}

		// Execute Publish with timeout
		attemptCtx, cancel = context.WithTimeout(ctx, operationTimeout)

		go func() {
			errChan <- prmq.conn.Channel.Publish(
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
		}()

		select {
		case err = <-errChan:
			cancel()

			if err == nil {
				logger.Infof("Messages sent successfully to exchange: %s, key: %s", exchange, key)

				return nil, nil
			}

			logger.Warnf("Failed to publish message to exchange: %s, key: %s, attempt %d/%d: %s", exchange, key, attempt+1, maxRetries+1, err)

			if attempt == maxRetries {
				libOpentelemetry.HandleSpanError(&spanProducer, "Failed to publish message after retries", err)

				logger.Errorf("Giving up after %d attempts: %s", maxRetries+1, err)

				return nil, err
			}

			sleepDuration := utils.FullJitter(backoff)
			logger.Infof("Retrying to publish message in %v (attempt %d)...", sleepDuration, attempt+2)
			time.Sleep(sleepDuration)

			backoff = utils.NextBackoff(backoff)
		case <-attemptCtx.Done():
			cancel()

			err = fmt.Errorf("timeout publishing to RabbitMQ after %v", operationTimeout)
			logger.Warnf("Failed to publish message to exchange: %s, key: %s, attempt %d/%d: %s", exchange, key, attempt+1, maxRetries+1, err)

			if attempt == maxRetries {
				libOpentelemetry.HandleSpanError(&spanProducer, "Failed to publish message after retries", err)

				logger.Errorf("Giving up after %d attempts: %s", maxRetries+1, err)

				return nil, err
			}

			sleepDuration := utils.FullJitter(backoff)
			logger.Infof("Retrying to publish message in %v (attempt %d)...", sleepDuration, attempt+2)
			time.Sleep(sleepDuration)

			backoff = utils.NextBackoff(backoff)
		}
	}

	return nil, err
}
