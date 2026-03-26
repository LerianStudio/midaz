// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v4/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/lib-commons/v4/commons/opentelemetry/metrics"
	libRabbitmq "github.com/LerianStudio/lib-commons/v4/commons/rabbitmq"
	tmconsumer "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/consumer"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	tmmongo "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/postgres"
	tmrabbitmq "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
)

// shouldUseBulkMode determines if bulk processing should be used for RabbitMQ message consumption.
// Bulk mode activates only when both conditions are met:
//   - RABBITMQ_TRANSACTION_ASYNC=true (async mode enabled)
//   - BULK_RECORDER_ENABLED=true (bulk mode not disabled)
//
// When bulk mode is active, messages are accumulated by BulkCollector and processed in batches.
// When inactive, messages are processed individually as before (backward compatible).
func shouldUseBulkMode(cfg *Config) bool {
	return cfg.RabbitMQTransactionAsync && cfg.BulkRecorderEnabled
}

// logBulkConfiguration logs the bulk recorder configuration at startup.
// Helps operators understand the current bulk mode settings.
func logBulkConfiguration(ctx context.Context, logger libLog.Logger, cfg *Config) {
	bulkMode := shouldUseBulkMode(cfg)

	logger.Log(ctx, libLog.LevelInfo, "Bulk recorder configuration",
		libLog.Bool("bulk_mode_active", bulkMode),
		libLog.Bool("rabbitmq_transaction_async", cfg.RabbitMQTransactionAsync),
		libLog.Bool("bulk_recorder_enabled", cfg.BulkRecorderEnabled),
		libLog.Int("bulk_recorder_size", cfg.BulkRecorderSize),
		libLog.Int("bulk_recorder_flush_timeout_ms", cfg.BulkRecorderFlushTimeoutMs),
		libLog.Int("bulk_recorder_max_rows_per_insert", cfg.BulkRecorderMaxRowsPerInsert),
	)

	if bulkMode {
		logger.Log(ctx, libLog.LevelInfo, "Bulk mode is ACTIVE: messages will be accumulated and processed in batches",
			libLog.Int("bulk_size", cfg.BulkRecorderSize),
			libLog.Int("flush_timeout_ms", cfg.BulkRecorderFlushTimeoutMs),
		)
	} else {
		if !cfg.RabbitMQTransactionAsync {
			logger.Log(ctx, libLog.LevelInfo, "Bulk mode is INACTIVE: RABBITMQ_TRANSACTION_ASYNC is not true")
		} else {
			logger.Log(ctx, libLog.LevelInfo, "Bulk mode is INACTIVE: BULK_RECORDER_ENABLED is false")
		}
	}
}

// rabbitMQComponents holds all RabbitMQ-related components initialized during bootstrap.
// In single-tenant mode, multiQueueConsumer and circuitBreakerManager are populated.
// In multi-tenant mode, multiTenantConsumer is populated instead.
// The wireConsumer callback must be called after UseCase creation to complete the wiring.
type rabbitMQComponents struct {
	producerRepo          rabbitmq.ProducerRepository
	multiQueueConsumer    *MultiQueueConsumer
	multiTenantConsumer   *tmconsumer.MultiTenantConsumer
	circuitBreakerManager *CircuitBreakerManager
	pgManager             *tmpostgres.Manager      // nil in single-tenant mode; used by consumer handler for per-tenant PG resolution
	mongoManager          *tmmongo.Manager          // nil in single-tenant mode; used by consumer handler for per-tenant Mongo resolution
	rabbitmqManager       *tmrabbitmq.Manager       // nil in single-tenant mode; used by event dispatcher to close tenant RabbitMQ connections
	metricsFactory        *metrics.MetricsFactory   // nil in single-tenant mode or when telemetry disabled; used for tenant metrics emission

	// wireConsumer is a callback that wires the consumer with the UseCase.
	// Must be called after UseCase creation because the handler needs UseCase.
	// - Single-tenant: creates consumer connection, routes, and MultiQueueConsumer
	// - Multi-tenant: registers BTO handler on the MultiTenantConsumer
	wireConsumer func(useCase *command.UseCase) error
}

// initRabbitMQ initializes RabbitMQ producer and consumer components.
// Dispatches to single-tenant or multi-tenant initialization based on Options.
func initRabbitMQ(
	opts *Options,
	cfg *Config,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
) (*rabbitMQComponents, error) {
	if opts != nil && opts.MultiTenantEnabled {
		return initMultiTenantRabbitMQ(opts, cfg, logger, telemetry)
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
	telemetry *libOpentelemetry.Telemetry,
) (*rabbitMQComponents, error) {
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing multi-tenant RabbitMQ producer and consumer")

	// Log bulk recorder configuration at startup
	logBulkConfiguration(context.Background(), logger, cfg)

	if opts.TenantClient == nil {
		return nil, fmt.Errorf("tenant client is required for multi-tenant RabbitMQ initialization")
	}

	rmqOpts := []tmrabbitmq.Option{
		tmrabbitmq.WithLogger(logger),
		tmrabbitmq.WithModule(ApplicationName),
	}

	if cfg.RabbitMQTLS {
		rmqOpts = append(rmqOpts, tmrabbitmq.WithTLS())
	}

	tenantRabbitMQ := tmrabbitmq.NewManager(
		opts.TenantClient,
		opts.TenantServiceName,
		rmqOpts...,
	)

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
		SyncInterval:      syncInterval,
		PrefetchCount:     prefetchCount,
		MultiTenantURL:    opts.TenantManagerURL,
		ServiceAPIKey:     opts.MultiTenantServiceAPIKey,
		Service:           opts.TenantServiceName,
		Environment:       opts.TenantEnvironment,
		DiscoveryTimeout:  discoveryTimeout,
		AllowInsecureHTTP: allowInsecureMultiTenantHTTP(opts.TenantManagerURL, cfg.EnvName),
	}

	consumer, err := tmconsumer.NewMultiTenantConsumerWithError(
		mtConfig,
		logger,
		tmconsumer.WithRabbitMQ(tenantRabbitMQ),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize multi-tenant consumer: %w", err)
	}

	producer := rabbitmq.NewMultiTenantProducer(tenantRabbitMQ, logger)

	queueName := cfg.RabbitMQTransactionBalanceOperationQueue
	if queueName == "" {
		return nil, fmt.Errorf("RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE is required for multi-tenant consumer")
	}

	// Store metricsFactory for tenant metrics emission (nil-safe: checked before use)
	var metricsFactory *metrics.MetricsFactory
	if telemetry != nil {
		metricsFactory = telemetry.MetricsFactory
	}

	rmqComponents := &rabbitMQComponents{
		producerRepo:        producer,
		multiTenantConsumer: consumer,
		rabbitmqManager:     tenantRabbitMQ,
		metricsFactory:      metricsFactory,
	}

	// wireConsumer registers the BTO handler on the MultiTenantConsumer.
	// The closure captures rmqComponents by pointer so that pgManager and mongoManager
	// can be set after initRabbitMQ returns (they are initialized in initPostgres/initMongo
	// and wired in config.go before wireConsumer is called).
	rmqComponents.wireConsumer = func(useCase *command.UseCase) error {
		if err := consumer.Register(
			queueName,
			func(ctx context.Context, delivery amqp.Delivery) error {
				ctx, err := resolveTenantConnections(ctx, rmqComponents)
				if err != nil {
					logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to resolve tenant connections for consumer message: %v", err))
					return err
				}

				if err := handlerBTO(ctx, delivery.Body, useCase); err != nil {
					logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to process consumer message: %v", err))
					return err
				}

				// Emit message processed metric after successful handler execution
				tenantID := tmcore.GetTenantIDFromContext(ctx)
				if rmqComponents.metricsFactory != nil && tenantID != "" {
					counter, counterErr := rmqComponents.metricsFactory.Counter(utils.TenantMessagesProcessedTotal)
					if counterErr == nil {
						if metricErr := counter.WithAttributes(attribute.String("tenant", tenantID)).AddOne(ctx); metricErr != nil {
							logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("failed to increment metric %v: %v", utils.TenantMessagesProcessedTotal, metricErr))
						}
					} else {
						logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("failed to create metric counter %v: %v", utils.TenantMessagesProcessedTotal, counterErr))
					}
				}

				return nil
			},
		); err != nil {
			return fmt.Errorf("failed to register rabbitmq consumer for queue %s: %w", queueName, err)
		}

		return nil
	}

	return rmqComponents, nil
}

// resolveTenantConnections enriches the context with per-tenant PostgreSQL and MongoDB
// connections for the current message. The tenant ID must already be present in ctx
// (set by MultiTenantConsumer.handleMessage via tmcore.ContextWithTenantID).
//
// Graceful degradation:
//   - Missing tenant ID: returns ctx unchanged (single-tenant fallback).
//   - Nil pgManager/mongoManager: skips that resolution (not configured).
//   - Connection error: returns error so the message is nacked and retried.
func resolveTenantConnections(ctx context.Context, rmq *rabbitMQComponents) (context.Context, error) {
	tenantID := tmcore.GetTenantIDFromContext(ctx)
	if tenantID == "" {
		return ctx, fmt.Errorf("missing tenant context in multi-tenant consumer")
	}

	if rmq.pgManager != nil {
		db, err := rmq.pgManager.GetDB(ctx, tenantID)
		if err != nil {
			emitTenantCounter(ctx, rmq.metricsFactory, utils.TenantConnectionErrorsTotal, tenantID, "postgresql")

			return ctx, fmt.Errorf("failed to resolve tenant PG connection for %s: %w", tenantID, err)
		}

		emitTenantCounter(ctx, rmq.metricsFactory, utils.TenantConnectionsTotal, tenantID, "postgresql")

		// Store the tenant PG connection in the generic tenant context key.
		ctx = tmcore.ContextWithTenantPGConnection(ctx, db)
	}

	if rmq.mongoManager != nil {
		mongoDB, err := rmq.mongoManager.GetDatabaseForTenant(ctx, tenantID)
		if err != nil {
			emitTenantCounter(ctx, rmq.metricsFactory, utils.TenantConnectionErrorsTotal, tenantID, "mongodb")

			return ctx, fmt.Errorf("failed to resolve tenant Mongo connection for %s: %w", tenantID, err)
		}

		emitTenantCounter(ctx, rmq.metricsFactory, utils.TenantConnectionsTotal, tenantID, "mongodb")

		ctx = tmcore.ContextWithTenantMongo(ctx, mongoDB)
	}

	return ctx, nil
}

func emitTenantCounter(ctx context.Context, factory *metrics.MetricsFactory, metricName metrics.Metric, tenantID, db string) {
	if factory == nil {
		return
	}

	counter, err := factory.Counter(metricName)
	if err != nil {
		return
	}

	_ = counter.WithAttributes(attribute.String("tenant", tenantID), attribute.String("db", db)).AddOne(ctx)
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
	logCtx := context.Background()

	// Log bulk recorder configuration at startup
	logBulkConfiguration(logCtx, logger, cfg)

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
			logger.Log(logCtx, libLog.LevelWarn, fmt.Sprintf("Failed to close RabbitMQ producer during cleanup: %v", closeErr))
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
			logger.Log(logCtx, libLog.LevelWarn, fmt.Sprintf("Failed to close RabbitMQ producer during cleanup: %v", closeErr))
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
			logger.Log(logCtx, libLog.LevelWarn, fmt.Sprintf("Failed to close RabbitMQ producer during cleanup: %v", closeErr))
		}

		return nil, fmt.Errorf("failed to create circuit breaker producer: %w", err)
	}

	rmq := &rabbitMQComponents{
		producerRepo:          producerRabbitMQRepository,
		circuitBreakerManager: circuitBreakerManager,
	}

	// wireConsumer creates the single-tenant consumer with dedicated connection credentials.
	// Deferred to after UseCase creation because NewMultiQueueConsumer registers the handler internally.
	rmq.wireConsumer = func(useCase *command.UseCase) error {
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

		// Configure bulk processing if enabled
		if shouldUseBulkMode(cfg) {
			routes.ConfigureBulk(&rabbitmq.BulkConfig{
				Enabled:      true,
				Size:         cfg.BulkRecorderSize,
				FlushTimeout: time.Duration(cfg.BulkRecorderFlushTimeoutMs) * time.Millisecond,
			})

			logger.Log(context.Background(), libLog.LevelInfo, "Bulk mode configured for consumer",
				libLog.Int("bulk_size", cfg.BulkRecorderSize),
				libLog.Int("flush_timeout_ms", cfg.BulkRecorderFlushTimeoutMs),
			)
		}

		rmq.multiQueueConsumer = NewMultiQueueConsumer(routes, useCase, telemetry.MetricsFactory)

		return nil
	}

	return rmq, nil
}

// allowInsecureMultiTenantHTTP returns true when the tenant-manager URL uses plain HTTP
// and the environment is a non-production environment (local, development, test).
// This mirrors the CRM pattern in config.tenant.go for consistent behavior.
func allowInsecureMultiTenantHTTP(tenantManagerURL, envName string) bool {
	parsedURL, err := url.Parse(tenantManagerURL)
	if err != nil {
		return false
	}

	scheme := strings.ToLower(strings.TrimSpace(parsedURL.Scheme))
	if scheme != "http" {
		return false
	}

	switch strings.ToLower(strings.TrimSpace(envName)) {
	case "local", "development", "dev", "test", "testing", "staging":
		return true
	default:
		return false
	}
}

// compositeStateListener fans out state change notifications to multiple listeners.
type compositeStateListener struct {
	listeners []libCircuitBreaker.StateChangeListener
}

// OnStateChange notifies all registered listeners of the state change.
func (c *compositeStateListener) OnStateChange(ctx context.Context, serviceName string, from, to libCircuitBreaker.State) {
	for _, listener := range c.listeners {
		listener.OnStateChange(ctx, serviceName, from, to)
	}
}
