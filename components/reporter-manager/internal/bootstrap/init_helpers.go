// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	httpIn "github.com/LerianStudio/midaz/v3/components/reporter-manager/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/reporter-manager/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/reporter-manager/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/reporter-manager/internal/services"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/ctxutil"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/datasource"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/deadline"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/report"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/template"
	reportSeaweedFS "github.com/LerianStudio/midaz/v3/components/reporter/pkg/seaweedfs/report"
	templateSeaweedFS "github.com/LerianStudio/midaz/v3/components/reporter/pkg/seaweedfs/template"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/storage"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	libRabbitmq "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	tmrabbitmq "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/rabbitmq"
	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/lib-observability/zap"
	amqp091 "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/trace"

	mongoDB "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb"
	libRedis "github.com/LerianStudio/midaz/v3/components/reporter/pkg/redis"
)

// mongoResources holds MongoDB-related resources created during initialization.
type mongoResources struct {
	connection   *mongoDB.MongoConnection
	deadlineRepo *deadline.DeadlineMongoDBRepository
	templateRepo *template.TemplateMongoDBRepository
	reportRepo   *report.ReportMongoDBRepository
}

// rabbitResources holds RabbitMQ-related resources created during initialization.
type rabbitResources struct {
	connection *libRabbitmq.RabbitMQConnection
	producer   *rabbitmq.ProducerRabbitMQRepository
}

// initConfigAndLogger loads configuration from environment variables, validates it,
// and initializes the structured logger.
func initConfigAndLogger() (*Config, log.Logger, error) {
	cfg := &Config{}
	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, nil, fmt.Errorf("failed to load config from env vars: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}

	logger, err := zap.New(zap.Config{
		Environment:     zap.EnvironmentLocal,
		OTelLibraryName: "reporter",
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	return cfg, logger, nil
}

// initTelemetry initializes OpenTelemetry tracing and returns the telemetry instance
// along with a cleanup function that shuts down the telemetry provider.
func initTelemetry(cfg *Config, logger log.Logger) (*libOtel.Telemetry, func(), error) {
	telemetry, err := libOtel.NewTelemetry(libOtel.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		InsecureExporter:          cfg.OtelInsecureExporter,
		Logger:                    logger,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	if err = telemetry.ApplyGlobals(); err != nil {
		return nil, nil, fmt.Errorf("failed to apply telemetry globals: %w", err)
	}

	cleanup := func() {
		logger.Log(context.Background(), log.LevelInfo, "Cleanup: shutting down telemetry")
		telemetry.ShutdownTelemetry()
	}

	return telemetry, cleanup, nil
}

// initStorage creates the S3-compatible object storage client used for both
// template and report file storage (differentiated by key prefix).
func initStorage(cfg *Config, logger log.Logger) (storage.ObjectStorage, error) {
	storageConfig := storage.Config{
		Bucket:            cfg.ObjectStorageBucket,
		S3Endpoint:        cfg.ObjectStorageEndpoint,
		S3Region:          cfg.ObjectStorageRegion,
		S3AccessKeyID:     cfg.ObjectStorageAccessKeyID,
		S3SecretAccessKey: cfg.ObjectStorageSecretKey,
		S3UsePathStyle:    cfg.ObjectStorageUsePathStyle,
		S3DisableSSL:      cfg.ObjectStorageDisableSSL,
	}

	ctx := ctxutil.ContextWithLogger(context.Background(), logger)

	storageClient, err := storage.NewStorageClient(ctx, storageConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage client: %w", err)
	}

	logger.Log(context.Background(), log.LevelInfo, "Storage initialized", log.String("bucket", cfg.ObjectStorageBucket))

	return storageClient, nil
}

// initMongoDB establishes the MongoDB connection, creates template and report
// repositories, ensures indexes exist, and returns a cleanup function that
// disconnects the client.
func initMongoDB(cfg *Config, logger log.Logger) (*mongoResources, func(), error) {
	escapedPass := url.QueryEscape(cfg.MongoDBPassword)
	mongoSource := fmt.Sprintf("%s://%s:%s@%s:%s",
		cfg.MongoURI, cfg.MongoDBUser, escapedPass, cfg.MongoDBHost, cfg.MongoDBPort)

	if cfg.MongoDBParameters != "" {
		mongoSource += "/?" + cfg.MongoDBParameters
	}

	mongoMaxPoolSize, _ := strconv.ParseUint(cfg.MongoMaxPoolSize, 10, 64)
	if mongoMaxPoolSize == 0 {
		mongoMaxPoolSize = constant.MongoDefaultMaxPoolSize
	}

	logger.Log(context.Background(), log.LevelInfo, "MongoDB connecting", log.String("dsn", pkg.RedactConnectionString(mongoSource)))

	var mongoTLS *libMongo.TLSConfig
	if cfg.MongoTLSCACert != "" {
		mongoTLS = &libMongo.TLSConfig{CACertBase64: cfg.MongoTLSCACert}
	}

	mongoConnection := &mongoDB.MongoConnection{
		ConnectionStringSource: mongoSource,
		Database:               cfg.MongoDBName,
		Logger:                 logger,
		MaxPoolSize:            mongoMaxPoolSize,
		TLS:                    mongoTLS,
	}

	var (
		deadlineMongoDBRepository *deadline.DeadlineMongoDBRepository
		templateMongoDBRepository *template.TemplateMongoDBRepository
		reportMongoDBRepository   *report.ReportMongoDBRepository
		err                       error
	)

	if cfg.MultiTenantEnabled {
		deadlineMongoDBRepository, err = deadline.NewDeadlineMongoDBRepositoryLazy(mongoConnection)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize deadline mongodb repository: %w", err)
		}

		templateMongoDBRepository, err = template.NewTemplateMongoDBRepositoryLazy(mongoConnection)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize template mongodb repository: %w", err)
		}

		reportMongoDBRepository, err = report.NewReportMongoDBRepositoryLazy(mongoConnection)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize report mongodb repository: %w", err)
		}
	} else {
		deadlineMongoDBRepository, err = deadline.NewDeadlineMongoDBRepository(mongoConnection)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize deadline mongodb repository: %w", err)
		}

		templateMongoDBRepository, err = template.NewTemplateMongoDBRepository(mongoConnection)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize template mongodb repository: %w", err)
		}

		reportMongoDBRepository, err = report.NewReportMongoDBRepository(mongoConnection)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize report mongodb repository: %w", err)
		}

		// Create MongoDB indexes only for single-tenant mode.
		logger.Log(context.Background(), log.LevelInfo, "Ensuring MongoDB indexes exist for templates and reports...")

		ctx := ctxutil.ContextWithLogger(context.Background(), logger)

		if err = templateMongoDBRepository.EnsureIndexes(ctx); err != nil {
			return nil, nil, fmt.Errorf("failed to ensure template indexes: %w", err)
		}

		if err = reportMongoDBRepository.EnsureIndexes(ctx); err != nil {
			return nil, nil, fmt.Errorf("failed to ensure report indexes: %w", err)
		}

		if err = deadlineMongoDBRepository.EnsureIndexes(ctx); err != nil {
			return nil, nil, fmt.Errorf("failed to ensure deadline indexes: %w", err)
		}
	}

	cleanup := func() {
		if mongoConnection != nil {
			logger.Log(context.Background(), log.LevelInfo, "Cleanup: disconnecting MongoDB")

			if disconnectErr := mongoConnection.Close(); disconnectErr != nil {
				logger.Log(context.Background(), log.LevelError, "Cleanup: failed to disconnect MongoDB", log.Err(disconnectErr))
			}
		}
	}

	return &mongoResources{
		connection:   mongoConnection,
		deadlineRepo: deadlineMongoDBRepository,
		templateRepo: templateMongoDBRepository,
		reportRepo:   reportMongoDBRepository,
	}, cleanup, nil
}

// initRabbitMQ establishes the RabbitMQ connection, creates the producer,
// starts the background connection monitor, and returns cleanup functions for
// the monitor and the connection itself.
//
// In multi-tenant mode (Layer 1 + Layer 2):
// - Layer 1: Uses tmrabbitmq.Manager for per-tenant vhost isolation
// - Layer 2: X-Tenant-ID header injection is handled in the producer (preserved in both modes)
//
// In single-tenant mode: Uses the static connection with retry logic.
func initRabbitMQ(cfg *Config, logger log.Logger) (*rabbitResources, []func(), error) {
	rabbitSource := fmt.Sprintf("%s://%s:%s@%s:%s",
		cfg.RabbitURI, cfg.RabbitMQUser, cfg.RabbitMQPass, cfg.RabbitMQHost, cfg.RabbitMQPortAMQP)
	logger.Log(context.Background(), log.LevelInfo, "RabbitMQ connecting", log.String("dsn", pkg.RedactConnectionString(rabbitSource)))

	rabbitMQConnection := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: rabbitSource,
		HealthCheckURL:         cfg.RabbitMQHealthCheckURL,
		Host:                   cfg.RabbitMQHost,
		Port:                   cfg.RabbitMQPortHost,
		User:                   cfg.RabbitMQUser,
		Pass:                   cfg.RabbitMQPass,
		Queue:                  cfg.RabbitMQGenerateReportQueue,
		AllowInsecureHealthCheck: strings.HasPrefix(strings.ToLower(cfg.RabbitMQHealthCheckURL), "http://") &&
			strings.ToLower(cfg.EnvName) != "production",
		Logger: logger,
	}

	var (
		producerRabbitMQRepository *rabbitmq.ProducerRabbitMQRepository
		rabbitMQManager            *tmrabbitmq.Manager
	)

	// Multi-tenant mode: use tmrabbitmq.Manager for per-tenant vhost isolation (Layer 1)

	if cfg.MultiTenantEnabled && cfg.MultiTenantURL != "" {
		logger.Log(context.Background(), log.LevelInfo, "RabbitMQ: initializing multi-tenant producer with vhost isolation")

		tmClient, err := newTenantManagerClient(cfg, logger)
		if err != nil {
			return nil, nil, fmt.Errorf("RabbitMQ: failed to initialize tenant manager client: %w", err)
		}

		rmqOpts := []tmrabbitmq.Option{
			tmrabbitmq.WithModule(constant.ModuleManager),
			tmrabbitmq.WithLogger(logger),
			tmrabbitmq.WithMaxTenantPools(cfg.MultiTenantMaxTenantPools),
			tmrabbitmq.WithIdleTimeout(time.Duration(cfg.MultiTenantIdleTimeoutSec) * time.Second),
		}

		if cfg.RabbitMQTLS {
			rmqOpts = append(rmqOpts, tmrabbitmq.WithTLS())
		}

		rabbitMQManager = tmrabbitmq.NewManager(
			tmClient,
			constant.ApplicationName,
			rmqOpts...,
		)

		// Wrap the tmrabbitmq.Manager to satisfy our interface
		producerRabbitMQRepository = rabbitmq.NewProducerRabbitMQMultiTenant(
			newRabbitMQManagerAdapter(rabbitMQManager),
		)

		logger.Log(context.Background(), log.LevelInfo, "RabbitMQ: multi-tenant producer initialized with tmrabbitmq.Manager")
	} else {
		// Single-tenant mode: use static connection
		producerRabbitMQRepository = rabbitmq.NewProducerRabbitMQ(rabbitMQConnection)
	}

	// Start background RabbitMQ connection monitor only in single-tenant mode.
	// In multi-tenant mode, the static rabbitMQConnection is never used — the
	// tmrabbitmq.Manager maintains its own per-tenant connections. Starting the
	// monitor would trigger a false "dead connection" warning every 10s because
	// the static connection object is never initialized (Connected=false, Connection=nil).
	var rabbitMQMonitor *RabbitMQMonitor

	cleanups := make([]func(), 0, 4)

	if rabbitMQManager == nil {
		// Single-tenant: monitor the static connection
		rabbitMQMonitor = NewRabbitMQMonitor(rabbitMQConnection, logger)
		rabbitMQMonitor.Start()
		logger.Log(context.Background(), log.LevelInfo, "RabbitMQ background connection monitor started")

		cleanups = append(cleanups, func() {
			logger.Log(context.Background(), log.LevelInfo, "Cleanup: stopping RabbitMQ connection monitor")
			rabbitMQMonitor.Stop()
		})
		cleanups = append(cleanups, func() {
			logger.Log(context.Background(), log.LevelInfo, "Cleanup: closing RabbitMQ connection")

			if rabbitMQConnection.Channel != nil {
				if closeErr := rabbitMQConnection.Channel.Close(); closeErr != nil {
					logger.Log(context.Background(), log.LevelError, "Cleanup: failed to close RabbitMQ channel", log.Err(closeErr))
				}
			}

			if rabbitMQConnection.Connection != nil && !rabbitMQConnection.Connection.IsClosed() {
				if closeErr := rabbitMQConnection.Connection.Close(); closeErr != nil {
					logger.Log(context.Background(), log.LevelError, "Cleanup: failed to close RabbitMQ connection", log.Err(closeErr))
				}
			}
		})
	} else {
		logger.Log(context.Background(), log.LevelInfo, "RabbitMQ connection monitor skipped (multi-tenant mode uses per-tenant connections)")
	}

	// Add cleanup for multi-tenant RabbitMQ manager (no-op when nil)
	if rabbitMQManager != nil {
		cleanups = append(cleanups, func() {
			logger.Log(context.Background(), log.LevelInfo, "Cleanup: closing multi-tenant RabbitMQ manager")

			if closeErr := rabbitMQManager.Close(context.Background()); closeErr != nil {
				logger.Log(context.Background(), log.LevelError, "Cleanup: failed to close RabbitMQ manager", log.Err(closeErr))
			}
		})
	}

	return &rabbitResources{
		connection: rabbitMQConnection,
		producer:   producerRabbitMQRepository,
	}, cleanups, nil
}

// rabbitMQManagerAdapter wraps tmrabbitmq.Manager to satisfy the RabbitMQManagerInterface.
type rabbitMQManagerAdapter struct {
	manager *tmrabbitmq.Manager
}

func newRabbitMQManagerAdapter(manager *tmrabbitmq.Manager) *rabbitMQManagerAdapter {
	return &rabbitMQManagerAdapter{manager: manager}
}

// GetChannel wraps tmrabbitmq.Manager.GetChannel and converts the returned *amqp091.Channel
// to our RabbitMQChannel interface.
func (a *rabbitMQManagerAdapter) GetChannel(ctx context.Context, tenantID string) (rabbitmq.RabbitMQChannel, error) {
	channel, err := a.manager.GetChannel(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	return &amqpChannelAdapter{channel: channel}, nil
}

// amqpChannelAdapter wraps *amqp091.Channel to implement RabbitMQChannel interface.
type amqpChannelAdapter struct {
	channel *amqp091.Channel
}

func (a *amqpChannelAdapter) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp091.Publishing) error {
	return a.channel.PublishWithContext(ctx, exchange, key, mandatory, immediate, msg)
}

func (a *amqpChannelAdapter) Close() error {
	return a.channel.Close()
}

// initRedis establishes the Redis/Valkey connection and returns the consumer
// repository along with a cleanup function that closes the connection.
func initRedis(cfg *Config, logger log.Logger) (*redis.RedisConsumerRepository, *libRedis.RedisConnection, func(), error) {
	redisConnection := &libRedis.RedisConnection{
		Address:                      strings.Split(cfg.RedisHost, ","),
		Password:                     cfg.RedisPassword,
		DB:                           cfg.RedisDB,
		Protocol:                     cfg.RedisProtocol,
		MasterName:                   cfg.RedisMasterName,
		UseTLS:                       cfg.RedisTLS,
		CACert:                       cfg.RedisCACert,
		UseGCPIAMAuth:                cfg.RedisUseGCPIAM,
		ServiceAccount:               cfg.RedisServiceAccount,
		GoogleApplicationCredentials: cfg.GoogleApplicationCredentials,
		TokenLifeTime:                time.Duration(cfg.RedisTokenLifeTime) * time.Minute,
		RefreshDuration:              time.Duration(cfg.RedisTokenRefreshDuration) * time.Minute,
		Logger:                       logger,
	}

	redisConsumerRepository, err := redis.NewConsumerRedis(redisConnection)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to initialize redis connection: %w", err)
	}

	cleanup := func() {
		logger.Log(context.Background(), log.LevelInfo, "Cleanup: closing Redis connection")

		if closeErr := redisConnection.Close(); closeErr != nil {
			logger.Log(context.Background(), log.LevelError, "Cleanup: failed to close Redis connection", log.Err(closeErr))
		}
	}

	return redisConsumerRepository, redisConnection, cleanup, nil
}

// initHandlers creates all HTTP handler instances with their service dependencies.
func initHandlers(
	logger log.Logger,
	tracer trace.Tracer,
	cfg *Config,
	mongo *mongoResources,
	rabbitProducer *rabbitmq.ProducerRabbitMQRepository,
	templateStorageRepo templateSeaweedFS.Repository,
	reportStorageRepo reportSeaweedFS.Repository,
	externalDataSources *pkg.SafeDataSources,
	redisConsumerRepository *redis.RedisConsumerRepository,
	dataSourceProvider datasource.DataSourceProvider,
) (*httpIn.TemplateHandler, *httpIn.ReportHandler, *httpIn.DataSourceHandler, *httpIn.DeadlineHandler, *httpIn.TemplateBuilderHandler, *httpIn.MetricsHandler, *httpIn.NotificationHandler, error) {
	templateHandler, err := httpIn.NewTemplateHandler(&services.UseCase{
		Logger:              logger,
		Tracer:              tracer,
		TemplateRepo:        mongo.templateRepo,
		TemplateSeaweedFS:   templateStorageRepo,
		ExternalDataSources: externalDataSources,
		DataSourceProvider:  dataSourceProvider,
		RedisRepo:           redisConsumerRepository,
		DeadlineRepo:        mongo.deadlineRepo,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize template handler: %w", err)
	}

	reportHandler, err := httpIn.NewReportHandler(&services.UseCase{
		Logger:                    logger,
		Tracer:                    tracer,
		ReportRepo:                mongo.reportRepo,
		RabbitMQRepo:              rabbitProducer,
		TemplateRepo:              mongo.templateRepo,
		ReportSeaweedFS:           reportStorageRepo,
		ExternalDataSources:       externalDataSources,
		DataSourceProvider:        dataSourceProvider,
		RedisRepo:                 redisConsumerRepository,
		RabbitMQExchange:          cfg.RabbitMQExchange,
		RabbitMQGenerateReportKey: cfg.RabbitMQGenerateReportKey,
		FetcherEnabled:            cfg.FetcherEnabled,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize report handler: %w", err)
	}

	dataSourceHandler, err := httpIn.NewDataSourceHandler(&services.UseCase{
		Logger:              logger,
		Tracer:              tracer,
		ExternalDataSources: externalDataSources,
		DataSourceProvider:  dataSourceProvider,
		RedisRepo:           redisConsumerRepository,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize data source handler: %w", err)
	}

	deadlineHandler, err := httpIn.NewDeadlineHandler(&services.UseCase{
		Logger:       logger,
		Tracer:       tracer,
		DeadlineRepo: mongo.deadlineRepo,
		TemplateRepo: mongo.templateRepo,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize deadline handler: %w", err)
	}

	templateBuilderHandler, err := httpIn.NewTemplateBuilderHandler(&services.UseCase{
		Logger: logger,
		Tracer: tracer,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize template builder handler: %w", err)
	}

	metricsHandler, err := httpIn.NewMetricsHandler(&services.UseCase{
		Logger:              logger,
		Tracer:              tracer,
		TemplateRepo:        mongo.templateRepo,
		ReportRepo:          mongo.reportRepo,
		ExternalDataSources: externalDataSources,
		DataSourceProvider:  dataSourceProvider,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize metrics handler: %w", err)
	}

	notificationHandler, err := httpIn.NewNotificationHandler(&services.UseCase{
		Logger:       logger,
		Tracer:       tracer,
		DeadlineRepo: mongo.deadlineRepo,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize notification handler: %w", err)
	}

	return templateHandler, reportHandler, dataSourceHandler, deadlineHandler, templateBuilderHandler, metricsHandler, notificationHandler, nil
}
