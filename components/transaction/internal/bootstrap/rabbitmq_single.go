package bootstrap

import (
	"fmt"
	"net/url"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
)

// buildRabbitMQConnectionString constructs an AMQP connection string with optional vhost.
func buildRabbitMQConnectionString(uri, user, pass, host, port, vhost string) string {
	u := &url.URL{
		Scheme: uri,
		User:   url.UserPassword(user, pass),
		Host:   fmt.Sprintf("%s:%s", host, port),
	}
	if vhost != "" {
		u.RawPath = "/" + url.PathEscape(vhost)
		u.Path = "/" + vhost
	}

	return u.String()
}

// SingleTenantProducerResult contains the result of single-tenant producer initialization.
type SingleTenantProducerResult struct {
	Producer         rabbitmq.ProducerRepository
	ConnectionString string
}

// initSingleTenantProducer initializes the RabbitMQ producer for single-tenant mode.
// It creates a static RabbitMQ connection using the configured credentials.
func initSingleTenantProducer(cfg *Config, logger libLog.Logger) SingleTenantProducerResult {
	rabbitSource := buildRabbitMQConnectionString(
		cfg.RabbitURI, cfg.RabbitMQUser, cfg.RabbitMQPass, cfg.RabbitMQHost, cfg.RabbitMQPortHost, cfg.RabbitMQVHost)

	rabbitMQConnection := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: rabbitSource,
		HealthCheckURL:         cfg.RabbitMQHealthCheckURL,
		Host:                   cfg.RabbitMQHost,
		Port:                   cfg.RabbitMQPortAMQP,
		User:                   cfg.RabbitMQUser,
		Pass:                   cfg.RabbitMQPass,
		Queue:                  cfg.RabbitMQBalanceCreateQueue,
		Logger:                 logger,
	}

	producer := rabbitmq.NewProducerRabbitMQ(rabbitMQConnection)
	logger.Info("Single-tenant RabbitMQ producer initialized")

	return SingleTenantProducerResult{
		Producer:         producer,
		ConnectionString: rabbitSource,
	}
}

// initSingleTenantConsumer initializes the RabbitMQ consumer for single-tenant mode.
// It creates a MultiQueueConsumer with handlers for balance creation and BTO queues.
func initSingleTenantConsumer(
	cfg *Config,
	useCase *command.UseCase,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
) *MultiQueueConsumer {
	rabbitConsumerSource := buildRabbitMQConnectionString(
		cfg.RabbitURI, cfg.RabbitMQConsumerUser, cfg.RabbitMQConsumerPass, cfg.RabbitMQHost, cfg.RabbitMQPortHost, cfg.RabbitMQVHost)

	rabbitMQConsumerConnection := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: rabbitConsumerSource,
		HealthCheckURL:         cfg.RabbitMQHealthCheckURL,
		Host:                   cfg.RabbitMQHost,
		Port:                   cfg.RabbitMQPortAMQP,
		User:                   cfg.RabbitMQConsumerUser,
		Pass:                   cfg.RabbitMQConsumerPass,
		VHost:                  cfg.RabbitMQVHost,
		Queue:                  cfg.RabbitMQBalanceCreateQueue,
		Logger:                 logger,
	}

	routes := rabbitmq.NewConsumerRoutes(
		rabbitMQConsumerConnection,
		cfg.RabbitMQNumbersOfWorkers,
		cfg.RabbitMQNumbersOfPrefetch,
		logger,
		telemetry,
	)

	consumer := NewMultiQueueConsumer(routes, useCase)
	logger.Info("Single-tenant RabbitMQ consumer initialized")

	return consumer
}
