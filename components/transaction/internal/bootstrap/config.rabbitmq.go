// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v3/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v3/commons/rabbitmq"
	libRedis "github.com/LerianStudio/lib-commons/v3/commons/redis"
	tmconsumer "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/consumer"
	tmrabbitmq "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	amqp "github.com/rabbitmq/amqp091-go"
)

// rabbitMQComponents holds all RabbitMQ-related components initialized during bootstrap.
// In single-tenant mode, multiQueueConsumer and circuitBreakerManager are populated.
// In multi-tenant mode, multiTenantConsumer is populated instead.
// The wireConsumer callback must be called after UseCase creation to complete the wiring.
type rabbitMQComponents struct {
	producerRepo          rabbitmq.ProducerRepository
	multiQueueConsumer    *MultiQueueConsumer
	multiTenantConsumer   *tmconsumer.MultiTenantConsumer
	circuitBreakerManager *CircuitBreakerManager

	// wireConsumer is a callback that wires the consumer with the UseCase.
	// Must be called after UseCase creation because the handler needs UseCase.
	// - Single-tenant: creates consumer connection, routes, and MultiQueueConsumer
	// - Multi-tenant: registers BTO handler on the MultiTenantConsumer
	wireConsumer func(useCase *command.UseCase)
}

// initRabbitMQ initializes RabbitMQ producer and consumer components.
// Dispatches to single-tenant or multi-tenant initialization based on Options.
func initRabbitMQ(
	opts *Options,
	cfg *Config,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
	redisConnection *libRedis.RedisConnection,
) (*rabbitMQComponents, error) {
	if opts != nil && opts.MultiTenantEnabled {
		return initMultiTenantRabbitMQ(opts, cfg, logger, redisConnection)
	}

	return initSingleTenantRabbitMQ(opts, cfg, logger, telemetry)
}

// initMultiTenantRabbitMQ initializes RabbitMQ in multi-tenant mode.
// Uses tmrabbitmq.Manager for per-tenant vhost connections with LRU eviction.
// No circuit breaker is needed; the Manager manages its own connection lifecycle.
func initMultiTenantRabbitMQ(
	opts *Options,
	cfg *Config,
	logger libLog.Logger,
	redisConnection *libRedis.RedisConnection,
) (*rabbitMQComponents, error) {
	logger.Info("Initializing multi-tenant RabbitMQ producer and consumer")

	if opts.TenantClient == nil {
		return nil, fmt.Errorf("TenantClient is required for multi-tenant RabbitMQ initialization")
	}

	tenantRabbitMQ := tmrabbitmq.NewManager(
		opts.TenantClient,
		opts.TenantServiceName,
		tmrabbitmq.WithLogger(logger),
		tmrabbitmq.WithModule(ApplicationName),
	)

	// Get Redis UniversalClient for tenant discovery cache (SMEMBERS on active tenants key)
	tenantDiscoveryRedisClient, err := redisConnection.GetClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get Redis client for multi-tenant consumer: %w", err)
	}

	prefetchCount := cfg.RabbitMQNumbersOfPrefetch
	if prefetchCount == 0 {
		prefetchCount = 10
	}

	syncInterval := utils.GetDurationSecondsWithDefault(cfg.RabbitMQMultiTenantSyncInterval, 30*time.Second)

	discoveryTimeout := 500 * time.Millisecond
	if cfg.RabbitMQMultiTenantDiscoveryTimeout > 0 {
		discoveryTimeout = time.Duration(cfg.RabbitMQMultiTenantDiscoveryTimeout) * time.Millisecond
	}

	mtConfig := tmconsumer.MultiTenantConfig{
		SyncInterval:     syncInterval,
		PrefetchCount:    prefetchCount,
		MultiTenantURL:   opts.TenantManagerURL,
		Service:          opts.TenantServiceName,
		Environment:      opts.TenantEnvironment,
		DiscoveryTimeout: discoveryTimeout,
	}

	consumer := tmconsumer.NewMultiTenantConsumer(tenantRabbitMQ, tenantDiscoveryRedisClient, mtConfig, logger)
	producer := rabbitmq.NewMultiTenantProducer(tenantRabbitMQ, logger)

	queueName := cfg.RabbitMQTransactionBalanceOperationQueue
	if queueName == "" {
		return nil, fmt.Errorf("RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE is required for multi-tenant consumer")
	}

	return &rabbitMQComponents{
		producerRepo:        producer,
		multiTenantConsumer: consumer,
		wireConsumer: func(useCase *command.UseCase) {
			consumer.Register(
				queueName,
				func(ctx context.Context, delivery amqp.Delivery) error {
					return handlerBTO(ctx, delivery.Body, useCase)
				},
			)
		},
	}, nil
}

// initSingleTenantRabbitMQ initializes RabbitMQ in single-tenant mode.
// Uses a shared connection with circuit breaker protection for the producer,
// and a separate consumer connection with dedicated credentials.
func initSingleTenantRabbitMQ(
	opts *Options,
	cfg *Config,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
) (*rabbitMQComponents, error) {
	// Producer connection
	rabbitSource := buildRabbitMQConnectionString(
		cfg.RabbitURI, cfg.RabbitMQUser, cfg.RabbitMQPass, cfg.RabbitMQHost, cfg.RabbitMQPortHost, cfg.RabbitMQVHost)

	rabbitMQConnection := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: rabbitSource,
		HealthCheckURL:         cfg.RabbitMQHealthCheckURL,
		Host:                   cfg.RabbitMQHost,
		Port:                   cfg.RabbitMQPortAMQP,
		User:                   cfg.RabbitMQUser,
		Pass:                   cfg.RabbitMQPass,
		VHost:                  cfg.RabbitMQVHost,
		Logger:                 logger,
	}

	rawProducerRabbitMQ, err := rabbitmq.NewProducerRabbitMQ(rabbitMQConnection)
	if err != nil {
		return nil, fmt.Errorf("failed to create RabbitMQ producer: %w", err)
	}

	// Circuit breaker observability
	metricStateListener, err := rabbitmq.NewMetricStateListener(telemetry.MetricsFactory)
	if err != nil {
		if closeErr := rawProducerRabbitMQ.Close(); closeErr != nil {
			logger.Warnf("Failed to close RabbitMQ producer during cleanup: %v", closeErr)
		}

		return nil, fmt.Errorf("failed to create metric state listener: %w", err)
	}

	var stateListener libCircuitBreaker.StateChangeListener
	if opts != nil && opts.CircuitBreakerStateListener != nil {
		stateListener = &compositeStateListener{
			listeners: []libCircuitBreaker.StateChangeListener{
				metricStateListener,
				opts.CircuitBreakerStateListener,
			},
		}
	} else {
		stateListener = metricStateListener
	}

	// Circuit breaker configuration with safe defaults
	operationTimeout := rabbitmq.DefaultOperationTimeout

	if cfg.RabbitMQOperationTimeout != "" {
		if parsed, err := time.ParseDuration(cfg.RabbitMQOperationTimeout); err == nil && parsed > 0 {
			operationTimeout = parsed
		}
	}

	cbConfig := rabbitmq.CircuitBreakerConfig{
		ConsecutiveFailures: utils.GetUint32FromIntWithDefault(cfg.RabbitMQCircuitBreakerConsecutiveFailures, 15),
		FailureRatio:        utils.GetFloat64FromIntPercentWithDefault(cfg.RabbitMQCircuitBreakerFailureRatio, 0.5),
		Interval:            utils.GetDurationSecondsWithDefault(cfg.RabbitMQCircuitBreakerInterval, 2*time.Minute),
		MaxRequests:         utils.GetUint32FromIntWithDefault(cfg.RabbitMQCircuitBreakerMaxRequests, 3),
		MinRequests:         utils.GetUint32FromIntWithDefault(cfg.RabbitMQCircuitBreakerMinRequests, 10),
		Timeout:             utils.GetDurationSecondsWithDefault(cfg.RabbitMQCircuitBreakerTimeout, 30*time.Second),
		HealthCheckInterval: utils.GetDurationSecondsWithDefault(cfg.RabbitMQCircuitBreakerHealthCheckInterval, 30*time.Second),
		HealthCheckTimeout:  utils.GetDurationSecondsWithDefault(cfg.RabbitMQCircuitBreakerHealthCheckTimeout, 10*time.Second),
		OperationTimeout:    operationTimeout,
	}

	circuitBreakerManager, err := NewCircuitBreakerManager(logger, rabbitMQConnection, cbConfig, stateListener)
	if err != nil {
		if closeErr := rawProducerRabbitMQ.Close(); closeErr != nil {
			logger.Warnf("Failed to close RabbitMQ producer during cleanup: %v", closeErr)
		}

		return nil, fmt.Errorf("failed to create circuit breaker manager: %w", err)
	}

	producerRabbitMQRepository, err := rabbitmq.NewCircuitBreakerProducer(
		rawProducerRabbitMQ,
		circuitBreakerManager.Manager,
		logger,
		cbConfig.OperationTimeout,
	)
	if err != nil {
		if closeErr := rawProducerRabbitMQ.Close(); closeErr != nil {
			logger.Warnf("Failed to close RabbitMQ producer during cleanup: %v", closeErr)
		}

		return nil, fmt.Errorf("failed to create circuit breaker producer: %w", err)
	}

	rmq := &rabbitMQComponents{
		producerRepo:          producerRabbitMQRepository,
		circuitBreakerManager: circuitBreakerManager,
	}

	// wireConsumer creates the single-tenant consumer with dedicated connection credentials.
	// Deferred to after UseCase creation because NewMultiQueueConsumer registers the handler internally.
	rmq.wireConsumer = func(useCase *command.UseCase) {
		rabbitConsumerSource := buildRabbitMQConnectionString(
			cfg.RabbitURI, cfg.RabbitMQConsumerUser, cfg.RabbitMQConsumerPass,
			cfg.RabbitMQHost, cfg.RabbitMQPortHost, cfg.RabbitMQVHost)

		rabbitMQConsumerConnection := &libRabbitmq.RabbitMQConnection{
			ConnectionStringSource: rabbitConsumerSource,
			HealthCheckURL:         cfg.RabbitMQHealthCheckURL,
			Host:                   cfg.RabbitMQHost,
			Port:                   cfg.RabbitMQPortAMQP,
			User:                   cfg.RabbitMQConsumerUser,
			Pass:                   cfg.RabbitMQConsumerPass,
			VHost:                  cfg.RabbitMQVHost,
			Logger:                 logger,
		}

		routes := rabbitmq.NewConsumerRoutes(
			rabbitMQConsumerConnection,
			cfg.RabbitMQNumbersOfWorkers,
			cfg.RabbitMQNumbersOfPrefetch,
			logger,
			telemetry,
		)

		rmq.multiQueueConsumer = NewMultiQueueConsumer(routes, useCase)
	}

	return rmq, nil
}

// compositeStateListener fans out state change notifications to multiple listeners.
type compositeStateListener struct {
	listeners []libCircuitBreaker.StateChangeListener
}

// OnStateChange notifies all registered listeners of the state change.
func (c *compositeStateListener) OnStateChange(serviceName string, from, to libCircuitBreaker.State) {
	for _, listener := range c.listeners {
		listener.OnStateChange(serviceName, from, to)
	}
}
