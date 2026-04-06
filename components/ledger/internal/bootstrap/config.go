// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libCircuitBreaker "github.com/LerianStudio/lib-commons/v4/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	libRedis "github.com/LerianStudio/lib-commons/v4/commons/redis"
	tmclient "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	tmevent "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/event"
	tmmiddleware "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/middleware"
	tmmongo "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/postgres"
	tmredis "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/redis"
	"github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/tenantcache"
	libZap "github.com/LerianStudio/lib-commons/v4/commons/zap"
	httpin "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/http/in"
	onbRedis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/onboarding"
	txRedis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	midazhttp "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const ApplicationName = "ledger"

// Config is the unified configuration struct for the ledger component.
// It merges all fields previously spread across onboarding, transaction, and ledger configs.
// Prefixed fields (Onb*/Txn*) map to domain-specific env vars; shared fields use common env vars.
type Config struct {
	// --- Shared fields ---
	ApplicationName string `env:"APPLICATION_NAME"`
	EnvName         string `env:"ENV_NAME"`
	LogLevel        string `env:"LOG_LEVEL"`
	Version         string `env:"VERSION"`

	// Server configuration - unified port for all APIs
	ServerAddress string `env:"SERVER_ADDRESS" envDefault:":3002"`

	// OpenTelemetry configuration
	OtelServiceName         string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName         string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion      string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv       string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry         bool   `env:"ENABLE_TELEMETRY"`

	// Auth configuration
	AuthEnabled bool   `env:"PLUGIN_AUTH_ENABLED"`
	AuthHost    string `env:"PLUGIN_AUTH_HOST"`
	JWKAddress  string `env:"CASDOOR_JWK_ADDRESS"`

	// Redis configuration (shared across domains)
	// Defaults are applied programmatically by applyConfigDefaults after env loading.
	RedisHost                    string `env:"REDIS_HOST"`
	RedisMasterName              string `env:"REDIS_MASTER_NAME"`
	RedisPassword                string `env:"REDIS_PASSWORD"`
	RedisDB                      int    `env:"REDIS_DB"`
	RedisProtocol                int    `env:"REDIS_PROTOCOL"`
	RedisTLS                     bool   `env:"REDIS_TLS"`
	RedisCACert                  string `env:"REDIS_CA_CERT"`
	RedisUseGCPIAM               bool   `env:"REDIS_USE_GCP_IAM"`
	RedisServiceAccount          string `env:"REDIS_SERVICE_ACCOUNT"`
	GoogleApplicationCredentials string `env:"GOOGLE_APPLICATION_CREDENTIALS"`
	RedisTokenLifeTime           int    `env:"REDIS_TOKEN_LIFETIME"`
	RedisTokenRefreshDuration    int    `env:"REDIS_TOKEN_REFRESH_DURATION"`
	RedisPoolSize                int    `env:"REDIS_POOL_SIZE"`
	RedisMinIdleConns            int    `env:"REDIS_MIN_IDLE_CONNS"`
	RedisReadTimeout             int    `env:"REDIS_READ_TIMEOUT"`
	RedisWriteTimeout            int    `env:"REDIS_WRITE_TIMEOUT"`
	RedisDialTimeout             int    `env:"REDIS_DIAL_TIMEOUT"`
	RedisPoolTimeout             int    `env:"REDIS_POOL_TIMEOUT"`
	RedisMaxRetries              int    `env:"REDIS_MAX_RETRIES"`
	RedisMinRetryBackoff         int    `env:"REDIS_MIN_RETRY_BACKOFF"`
	RedisMaxRetryBackoff         int    `env:"REDIS_MAX_RETRY_BACKOFF"`

	// Multi-tenant configuration
	MultiTenantEnabled                     bool   `env:"MULTI_TENANT_ENABLED"`
	MultiTenantURL                         string `env:"MULTI_TENANT_URL"`
	MultiTenantCircuitBreakerThreshold     int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD"`
	MultiTenantCircuitBreakerTimeoutSec    int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC"`
	MultiTenantServiceAPIKey               string `env:"MULTI_TENANT_SERVICE_API_KEY"`
	MultiTenantConnectionsCheckIntervalSec int    `env:"MULTI_TENANT_CONNECTIONS_CHECK_INTERVAL_SEC"`
	MultiTenantCacheTTLSec                 int    `env:"MULTI_TENANT_CACHE_TTL_SEC" default:"120"` // seconds for tenant config cache TTL (0 = disabled)
	MultiTenantRedisHost                   string `env:"MULTI_TENANT_REDIS_HOST"`
	MultiTenantRedisPort                   string `env:"MULTI_TENANT_REDIS_PORT"`
	MultiTenantRedisPassword               string `env:"MULTI_TENANT_REDIS_PASSWORD"`
	MultiTenantRedisTLS                    bool   `env:"MULTI_TENANT_REDIS_TLS"`

	// --- Onboarding PostgreSQL fields (DB_ONBOARDING_* env tags) ---
	OnbPrefixedPrimaryDBHost     string `env:"DB_ONBOARDING_HOST"`
	OnbPrefixedPrimaryDBUser     string `env:"DB_ONBOARDING_USER"`
	OnbPrefixedPrimaryDBPassword string `env:"DB_ONBOARDING_PASSWORD"`
	OnbPrefixedPrimaryDBName     string `env:"DB_ONBOARDING_NAME"`
	OnbPrefixedPrimaryDBPort     string `env:"DB_ONBOARDING_PORT"`
	OnbPrefixedPrimaryDBSSLMode  string `env:"DB_ONBOARDING_SSLMODE"`

	OnbPrefixedReplicaDBHost     string `env:"DB_ONBOARDING_REPLICA_HOST"`
	OnbPrefixedReplicaDBUser     string `env:"DB_ONBOARDING_REPLICA_USER"`
	OnbPrefixedReplicaDBPassword string `env:"DB_ONBOARDING_REPLICA_PASSWORD"`
	OnbPrefixedReplicaDBName     string `env:"DB_ONBOARDING_REPLICA_NAME"`
	OnbPrefixedReplicaDBPort     string `env:"DB_ONBOARDING_REPLICA_PORT"`
	OnbPrefixedReplicaDBSSLMode  string `env:"DB_ONBOARDING_REPLICA_SSLMODE"`

	OnbPrefixedMaxOpenConnections int `env:"DB_ONBOARDING_MAX_OPEN_CONNS"`
	OnbPrefixedMaxIdleConnections int `env:"DB_ONBOARDING_MAX_IDLE_CONNS"`

	// --- Transaction PostgreSQL fields (DB_TRANSACTION_* env tags) ---
	TxnPrefixedPrimaryDBHost     string `env:"DB_TRANSACTION_HOST"`
	TxnPrefixedPrimaryDBUser     string `env:"DB_TRANSACTION_USER"`
	TxnPrefixedPrimaryDBPassword string `env:"DB_TRANSACTION_PASSWORD"`
	TxnPrefixedPrimaryDBName     string `env:"DB_TRANSACTION_NAME"`
	TxnPrefixedPrimaryDBPort     string `env:"DB_TRANSACTION_PORT"`
	TxnPrefixedPrimaryDBSSLMode  string `env:"DB_TRANSACTION_SSLMODE"`

	TxnPrefixedReplicaDBHost     string `env:"DB_TRANSACTION_REPLICA_HOST"`
	TxnPrefixedReplicaDBUser     string `env:"DB_TRANSACTION_REPLICA_USER"`
	TxnPrefixedReplicaDBPassword string `env:"DB_TRANSACTION_REPLICA_PASSWORD"`
	TxnPrefixedReplicaDBName     string `env:"DB_TRANSACTION_REPLICA_NAME"`
	TxnPrefixedReplicaDBPort     string `env:"DB_TRANSACTION_REPLICA_PORT"`
	TxnPrefixedReplicaDBSSLMode  string `env:"DB_TRANSACTION_REPLICA_SSLMODE"`

	TxnPrefixedMaxOpenConnections int `env:"DB_TRANSACTION_MAX_OPEN_CONNS"`
	TxnPrefixedMaxIdleConnections int `env:"DB_TRANSACTION_MAX_IDLE_CONNS"`

	// --- Onboarding MongoDB fields (MONGO_ONBOARDING_* env tags) ---
	OnbPrefixedMongoURI          string `env:"MONGO_ONBOARDING_URI"`
	OnbPrefixedMongoDBHost       string `env:"MONGO_ONBOARDING_HOST"`
	OnbPrefixedMongoDBName       string `env:"MONGO_ONBOARDING_NAME"`
	OnbPrefixedMongoDBUser       string `env:"MONGO_ONBOARDING_USER"`
	OnbPrefixedMongoDBPassword   string `env:"MONGO_ONBOARDING_PASSWORD"`
	OnbPrefixedMongoDBPort       string `env:"MONGO_ONBOARDING_PORT"`
	OnbPrefixedMongoDBParameters string `env:"MONGO_ONBOARDING_PARAMETERS"`
	OnbPrefixedMaxPoolSize       int    `env:"MONGO_ONBOARDING_MAX_POOL_SIZE"`

	// --- Transaction MongoDB fields (MONGO_TRANSACTION_* env tags) ---
	TxnPrefixedMongoURI          string `env:"MONGO_TRANSACTION_URI"`
	TxnPrefixedMongoDBHost       string `env:"MONGO_TRANSACTION_HOST"`
	TxnPrefixedMongoDBName       string `env:"MONGO_TRANSACTION_NAME"`
	TxnPrefixedMongoDBUser       string `env:"MONGO_TRANSACTION_USER"`
	TxnPrefixedMongoDBPassword   string `env:"MONGO_TRANSACTION_PASSWORD"`
	TxnPrefixedMongoDBPort       string `env:"MONGO_TRANSACTION_PORT"`
	TxnPrefixedMongoDBParameters string `env:"MONGO_TRANSACTION_PARAMETERS"`
	TxnPrefixedMaxPoolSize       int    `env:"MONGO_TRANSACTION_MAX_POOL_SIZE"`

	// --- RabbitMQ (transaction domain only) ---
	RabbitURI                                string `env:"RABBITMQ_URI"`
	RabbitMQHost                             string `env:"RABBITMQ_HOST"`
	RabbitMQPortHost                         string `env:"RABBITMQ_PORT_HOST"`
	RabbitMQPortAMQP                         string `env:"RABBITMQ_PORT_AMQP"`
	RabbitMQUser                             string `env:"RABBITMQ_DEFAULT_USER"`
	RabbitMQPass                             string `env:"RABBITMQ_DEFAULT_PASS"`
	RabbitMQConsumerUser                     string `env:"RABBITMQ_CONSUMER_USER"`
	RabbitMQConsumerPass                     string `env:"RABBITMQ_CONSUMER_PASS"`
	RabbitMQVHost                            string `env:"RABBITMQ_VHOST"`
	RabbitMQNumbersOfWorkers                 int    `env:"RABBITMQ_NUMBERS_OF_WORKERS"`
	RabbitMQNumbersOfPrefetch                int    `env:"RABBITMQ_NUMBERS_OF_PREFETCH"`
	RabbitMQHealthCheckURL                   string `env:"RABBITMQ_HEALTH_CHECK_URL"`
	RabbitMQTLS                              bool   `env:"RABBITMQ_TLS"`
	RabbitMQTransactionBalanceOperationQueue string `env:"RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE"`

	// Circuit Breaker configuration for RabbitMQ
	RabbitMQCircuitBreakerConsecutiveFailures int    `env:"RABBITMQ_CIRCUIT_BREAKER_CONSECUTIVE_FAILURES"`
	RabbitMQCircuitBreakerFailureRatio        int    `env:"RABBITMQ_CIRCUIT_BREAKER_FAILURE_RATIO"`
	RabbitMQCircuitBreakerInterval            int    `env:"RABBITMQ_CIRCUIT_BREAKER_INTERVAL"`
	RabbitMQCircuitBreakerMaxRequests         int    `env:"RABBITMQ_CIRCUIT_BREAKER_MAX_REQUESTS"`
	RabbitMQCircuitBreakerMinRequests         int    `env:"RABBITMQ_CIRCUIT_BREAKER_MIN_REQUESTS"`
	RabbitMQCircuitBreakerTimeout             int    `env:"RABBITMQ_CIRCUIT_BREAKER_TIMEOUT"`
	RabbitMQCircuitBreakerHealthCheckInterval int    `env:"RABBITMQ_CIRCUIT_BREAKER_HEALTH_CHECK_INTERVAL"`
	RabbitMQCircuitBreakerHealthCheckTimeout  int    `env:"RABBITMQ_CIRCUIT_BREAKER_HEALTH_CHECK_TIMEOUT"`
	RabbitMQOperationTimeout                  string `env:"RABBITMQ_OPERATION_TIMEOUT"`
	RabbitMQTransactionAsync                  bool   `env:"RABBITMQ_TRANSACTION_ASYNC"`

	// Bulk mode activates only when RABBITMQ_TRANSACTION_ASYNC=true AND BulkRecorderEnabled=true.
	// Bulk size should match RabbitMQ prefetch for optimal performance (workers × prefetch).
	BulkRecorderEnabled          bool `env:"BULK_RECORDER_ENABLED"`
	BulkRecorderSize             int  `env:"BULK_RECORDER_SIZE"`
	BulkRecorderFlushTimeoutMs   int  `env:"BULK_RECORDER_FLUSH_TIMEOUT_MS"`
	BulkRecorderMaxRowsPerInsert int  `env:"BULK_RECORDER_MAX_ROWS_PER_INSERT"`

	// --- Balance/Worker fields ---
	BalanceSyncBatchSize      int `env:"BALANCE_SYNC_BATCH_SIZE"`
	BalanceSyncFlushTimeoutMs int `env:"BALANCE_SYNC_FLUSH_TIMEOUT_MS"`
	BalanceSyncPollIntervalMs int `env:"BALANCE_SYNC_POLL_INTERVAL_MS"`

	// --- Settings ---
	SettingsCacheTTL string `env:"SETTINGS_CACHE_TTL"`
}

// Options contains optional dependencies that can be injected by callers.
type Options struct {
	// Logger allows callers to provide a pre-configured logger.
	Logger libLog.Logger

	// CircuitBreakerStateListener receives notifications when circuit breaker state changes.
	CircuitBreakerStateListener libCircuitBreaker.StateChangeListener

	// TenantClient is the tenant manager client for multi-tenant mode.
	TenantClient *tmclient.Client

	// TenantCache is the shared process-local cache for tenant configurations.
	TenantCache *tenantcache.TenantCache

	// Multi-tenant configuration (resolved from Config during init).
	MultiTenantEnabled       bool
	TenantServiceName        string
	TenantManagerURL         string
	MultiTenantServiceAPIKey string
}

// InitServers initializes the unified ledger service.
func InitServers() (*Service, error) {
	return InitServersWithOptions(nil)
}

// InitServersWithOptions initializes the unified ledger service with optional dependency injection.
// It directly initializes all infrastructure (PG, Mongo, Redis, RabbitMQ) instead of delegating
// to onboarding/transaction sub-modules.
func InitServersWithOptions(opts *Options) (*Service, error) {
	cfg := &Config{}

	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config from environment variables: %w", err)
	}

	applyConfigDefaults(cfg)

	if cfg.MultiTenantEnabled && !cfg.AuthEnabled {
		return nil, fmt.Errorf("MULTI_TENANT_ENABLED=true requires PLUGIN_AUTH_ENABLED=true; " +
			"running multi-tenant mode without authentication allows cross-tenant data access")
	}

	// Logger: use injected or create fresh
	var baseLogger libLog.Logger
	if opts != nil && opts.Logger != nil {
		baseLogger = opts.Logger
	} else {
		var err error

		baseLogger, err = libZap.New(libZap.Config{
			Environment:     resolveLoggerEnvironment(cfg.EnvName),
			Level:           cfg.LogLevel,
			OTelLibraryName: cfg.OtelLibraryName,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to initialize logger: %w", err)
		}
	}

	// Generate startup ID for tracing initialization issues
	startupID := uuid.New().String()

	logger := baseLogger.With(
		libLog.String("component", ApplicationName),
		libLog.String("startup_id", startupID),
	)

	logger.Log(context.Background(), libLog.LevelInfo, "Starting unified ledger component",
		libLog.String("version", cfg.Version),
		libLog.String("env", cfg.EnvName),
	)

	// Telemetry
	telemetry, err := libOpentelemetry.NewTelemetry(libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    baseLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	// Register telemetry providers as process-global so that the otelzap bridge
	// (installed in the logger core) can forward log records to the OTLP exporter.
	if err := telemetry.ApplyGlobals(); err != nil {
		return nil, fmt.Errorf("failed to apply telemetry globals: %w", err)
	}

	// Multi-tenant client setup
	var tenantClient *tmclient.Client

	var tenantServiceName string

	if opts != nil && opts.TenantClient != nil {
		tenantClient = opts.TenantClient
		tenantServiceName = strings.TrimSpace(cfg.ApplicationName)
	} else {
		var err error

		tenantClient, tenantServiceName, err = initTenantClient(cfg, logger)
		if err != nil {
			return nil, err
		}
	}

	// Validate TenantServiceName early so that workers fail fast on misconfiguration
	if cfg.MultiTenantEnabled {
		tenantServiceName = strings.TrimSpace(tenantServiceName)
		if tenantServiceName == "" {
			return nil, fmt.Errorf("TenantServiceName must not be empty when multi-tenant is enabled")
		}
	}

	// Build internal options struct used by init functions
	internalOpts := &Options{
		MultiTenantEnabled:          cfg.MultiTenantEnabled,
		TenantClient:                tenantClient,
		TenantServiceName:           tenantServiceName,
		TenantManagerURL:            strings.TrimSpace(cfg.MultiTenantURL),
		MultiTenantServiceAPIKey:    strings.TrimSpace(cfg.MultiTenantServiceAPIKey),
		CircuitBreakerStateListener: nil,
	}
	if opts != nil {
		internalOpts.CircuitBreakerStateListener = opts.CircuitBreakerStateListener
	}

	// === Tenant Cache (multi-tenant only) ===
	// The dispatcher and event listener are created AFTER infrastructure init (PG, Mongo, RabbitMQ)
	// so that they can receive the infrastructure managers for closing tenant connections on
	// suspend/delete events.

	var tenantCache *tenantcache.TenantCache

	var tenantLoader *tenantcache.TenantLoader

	var eventListener *tmevent.TenantEventListener

	var cacheTTL time.Duration

	if cfg.MultiTenantEnabled && tenantClient != nil {
		tenantCache = tenantcache.NewTenantCache()

		cacheTTL = time.Duration(cfg.MultiTenantCacheTTLSec) * time.Second
		tenantLoader = tenantcache.NewTenantLoader(tenantClient, tenantCache, tenantServiceName, cacheTTL, logger)

		internalOpts.TenantCache = tenantCache
	}

	// === Infrastructure initialization ===

	// Cleanup helper: on error, close resources in reverse order of creation.
	var cleanups []func()

	addCleanup := func(fn func()) { cleanups = append(cleanups, fn) }
	doCleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	// 1. Onboarding PostgreSQL → 7 repos
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing onboarding PostgreSQL...")

	onbPG, err := initOnboardingPostgres(internalOpts, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize onboarding PostgreSQL: %w", err)
	}

	addCleanup(func() { _ = onbPG.connection.Close() })

	// 2. Transaction PostgreSQL → 6 repos
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing transaction PostgreSQL...")

	txnPG, err := initTransactionPostgres(internalOpts, cfg, logger)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize transaction PostgreSQL: %w", err)
	}

	addCleanup(func() { _ = txnPG.connection.Close() })

	// 3. Onboarding MongoDB → metadata repo
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing onboarding MongoDB...")

	onbMgo, err := initOnboardingMongo(internalOpts, cfg, logger)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize onboarding MongoDB: %w", err)
	}

	if onbMgo.connection != nil {
		addCleanup(func() { _ = onbMgo.connection.Close(context.Background()) })
	}

	// 4. Transaction MongoDB → metadata repo
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing transaction MongoDB...")

	txnMgo, err := initTransactionMongo(internalOpts, cfg, logger)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize transaction MongoDB: %w", err)
	}

	if txnMgo.connection != nil {
		addCleanup(func() { _ = txnMgo.connection.Close(context.Background()) })
	}

	// 5. Redis (shared connection, two consumer repos)
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing Redis...")

	redisConnection, err := initRedisConnection(cfg, logger)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize Redis: %w", err)
	}

	addCleanup(func() { _ = redisConnection.Close() })

	onbRedisRepo, err := onbRedis.NewConsumerRedis(redisConnection)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize onboarding redis consumer: %w", err)
	}

	txnRedisRepo, err := txRedis.NewConsumerRedis(redisConnection)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize transaction redis consumer: %w", err)
	}

	// 6. RabbitMQ → producer + consumer
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing RabbitMQ...")

	rmq, err := initRabbitMQ(internalOpts, cfg, logger, telemetry)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize RabbitMQ: %w", err)
	}

	if rmq != nil && rmq.producerRepo != nil {
		addCleanup(func() { _ = rmq.producerRepo.Close() })
	}

	// Pass PG and Mongo managers to RabbitMQ components for per-message tenant resolution
	if rmq != nil {
		rmq.pgManager = txnPG.pgManager
		rmq.mongoManager = txnMgo.mongoManager
	}

	// === Event Dispatcher & Listener (multi-tenant only) ===
	// Created AFTER infrastructure init so the dispatcher receives the PG, Mongo, and RabbitMQ
	// managers. This allows removeTenant to close infrastructure connections when a tenant is
	// suspended, deleted, or disassociated.
	if cfg.MultiTenantEnabled && tenantCache != nil && tenantLoader != nil {
		dispatcherOpts := []tmevent.DispatcherOption{
			tmevent.WithDispatcherLogger(logger),
			tmevent.WithCacheTTL(cacheTTL),
			tmevent.WithOnTenantAdded(func(ctx context.Context, tenantID string) {
				if tenantClient != nil {
					_ = tenantClient.InvalidateConfig(ctx, tenantID, tenantServiceName)
				}

				if rmq != nil && rmq.multiTenantConsumer != nil {
					rmq.multiTenantConsumer.EnsureConsumerStarted(ctx, tenantID)
				}
			}),
			tmevent.WithOnTenantRemoved(func(ctx context.Context, tenantID string) {
				if rmq != nil && rmq.multiTenantConsumer != nil {
					rmq.multiTenantConsumer.StopConsumer(tenantID)
				}

				// Close ALL postgres managers (onboarding + transaction)
				if onbPG.pgManager != nil {
					if err := onbPG.pgManager.CloseConnection(ctx, tenantID); err != nil {
						logger.Log(ctx, libLog.LevelWarn, "failed to close onboarding PG connection",
							libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
					}
				}

				if txnPG.pgManager != nil {
					if err := txnPG.pgManager.CloseConnection(ctx, tenantID); err != nil {
						logger.Log(ctx, libLog.LevelWarn, "failed to close transaction PG connection",
							libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
					}
				}

				// Close ALL mongo managers (onboarding + transaction)
				if onbMgo.mongoManager != nil {
					if err := onbMgo.mongoManager.CloseConnection(ctx, tenantID); err != nil {
						logger.Log(ctx, libLog.LevelWarn, "failed to close onboarding Mongo connection",
							libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
					}
				}

				if txnMgo.mongoManager != nil {
					if err := txnMgo.mongoManager.CloseConnection(ctx, tenantID); err != nil {
						logger.Log(ctx, libLog.LevelWarn, "failed to close transaction Mongo connection",
							libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
					}
				}

				// Close RabbitMQ
				if rmq != nil && rmq.rabbitmqManager != nil {
					if err := rmq.rabbitmqManager.CloseConnection(ctx, tenantID); err != nil {
						logger.Log(ctx, libLog.LevelWarn, "failed to close RabbitMQ connection",
							libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
					}
				}

				// Invalidate pmClient internal cache so lazy-load fetches fresh state from tenant-manager
				if tenantClient != nil {
					if err := tenantClient.InvalidateConfig(ctx, tenantID, tenantServiceName); err != nil {
						logger.Log(ctx, libLog.LevelWarn, "failed to invalidate tenant config cache",
							libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
					}
				}

				logger.Log(ctx, libLog.LevelInfo, "tenant evicted: all connections and caches invalidated",
					libLog.String("tenant_id", tenantID))
			}),
		}

		dispatcher := tmevent.NewEventDispatcher(
			tenantCache,
			tenantLoader,
			tenantServiceName,
			dispatcherOpts...,
		)

		tenantLoader.SetOnTenantLoaded(func(ctx context.Context, tenantID string) {
			if rmq != nil && rmq.multiTenantConsumer != nil {
				rmq.multiTenantConsumer.EnsureConsumerStarted(ctx, tenantID)
			}
		})

		// Create Redis client for tenant-manager Pub/Sub when configured
		if cfg.MultiTenantRedisHost != "" {
			tmRedisClient, tmRedisErr := tmredis.NewTenantPubSubRedisClient(context.Background(), tmredis.TenantPubSubRedisConfig{
				Host:     cfg.MultiTenantRedisHost,
				Port:     strings.TrimSpace(cfg.MultiTenantRedisPort),
				Password: cfg.MultiTenantRedisPassword,
				TLS:      cfg.MultiTenantRedisTLS,
			})
			if tmRedisErr != nil {
				doCleanup()
				return nil, fmt.Errorf("failed to initialize tenant-manager Redis for Pub/Sub: %w", tmRedisErr)
			}

			var listenerErr error

			eventListener, listenerErr = tmevent.NewTenantEventListener(
				tmRedisClient,
				dispatcher.HandleEvent,
				tmevent.WithListenerLogger(logger),
				tmevent.WithService(tenantServiceName),
			)
			if listenerErr != nil {
				doCleanup()
				return nil, fmt.Errorf("failed to create tenant event listener: %w", listenerErr)
			}

			logger.Log(context.Background(), libLog.LevelInfo, "Tenant event listener configured",
				libLog.String("redis_host", cfg.MultiTenantRedisHost),
				libLog.String("service", tenantServiceName),
			)
		}
	}

	// === Use cases ===

	settingsCacheTTL := resolveSettingsCacheTTL(cfg, logger)

	commandUseCase := &command.UseCase{
		// Onboarding domain
		OrganizationRepo:       onbPG.organizationRepo,
		LedgerRepo:             onbPG.ledgerRepo,
		SegmentRepo:            onbPG.segmentRepo,
		PortfolioRepo:          onbPG.portfolioRepo,
		AccountRepo:            onbPG.accountRepo,
		AssetRepo:              onbPG.assetRepo,
		AccountTypeRepo:        onbPG.accountTypeRepo,
		OnboardingMetadataRepo: onbMgo.metadataRepo,
		OnboardingRedisRepo:    onbRedisRepo,
		// Transaction domain
		TransactionRepo:         txnPG.transactionRepo,
		OperationRepo:           txnPG.operationRepo,
		AssetRateRepo:           txnPG.assetRateRepo,
		BalanceRepo:             txnPG.balanceRepo,
		OperationRouteRepo:      txnPG.operationRouteRepo,
		TransactionRouteRepo:    txnPG.transactionRouteRepo,
		TransactionMetadataRepo: txnMgo.metadataRepo,
		RabbitMQRepo:            rmq.producerRepo,
		TransactionRedisRepo:    txnRedisRepo,
		// Settings
		SettingsCacheTTL: settingsCacheTTL,
	}

	queryUseCase := &query.UseCase{
		// Onboarding domain
		OrganizationRepo:       onbPG.organizationRepo,
		LedgerRepo:             onbPG.ledgerRepo,
		SegmentRepo:            onbPG.segmentRepo,
		PortfolioRepo:          onbPG.portfolioRepo,
		AccountRepo:            onbPG.accountRepo,
		AssetRepo:              onbPG.assetRepo,
		AccountTypeRepo:        onbPG.accountTypeRepo,
		OnboardingMetadataRepo: onbMgo.metadataRepo,
		OnboardingRedisRepo:    onbRedisRepo,
		// Transaction domain
		TransactionRepo:         txnPG.transactionRepo,
		OperationRepo:           txnPG.operationRepo,
		AssetRateRepo:           txnPG.assetRateRepo,
		BalanceRepo:             txnPG.balanceRepo,
		OperationRouteRepo:      txnPG.operationRouteRepo,
		TransactionRouteRepo:    txnPG.transactionRouteRepo,
		TransactionMetadataRepo: txnMgo.metadataRepo,
		RabbitMQRepo:            rmq.producerRepo,
		TransactionRedisRepo:    txnRedisRepo,
		// Settings
		SettingsCacheTTL: settingsCacheTTL,
	}

	// Wire consumer with UseCase (registers handler or creates MultiQueueConsumer)
	if err := rmq.wireConsumer(commandUseCase); err != nil {
		doCleanup()
		return nil, err
	}

	// === Handlers ===

	// Onboarding handlers
	accountHandler := &httpin.AccountHandler{Command: commandUseCase, Query: queryUseCase}
	portfolioHandler := &httpin.PortfolioHandler{Command: commandUseCase, Query: queryUseCase}
	ledgerHandler := &httpin.LedgerHandler{Command: commandUseCase, Query: queryUseCase}
	assetHandler := &httpin.AssetHandler{Command: commandUseCase, Query: queryUseCase}
	organizationHandler := &httpin.OrganizationHandler{Command: commandUseCase, Query: queryUseCase}
	segmentHandler := &httpin.SegmentHandler{Command: commandUseCase, Query: queryUseCase}
	accountTypeHandler := &httpin.AccountTypeHandler{Command: commandUseCase, Query: queryUseCase}

	// Transaction handlers
	transactionHandler := &httpin.TransactionHandler{Command: commandUseCase, Query: queryUseCase}
	operationHandler := &httpin.OperationHandler{Command: commandUseCase, Query: queryUseCase}
	assetRateHandler := &httpin.AssetRateHandler{Command: commandUseCase, Query: queryUseCase}
	balanceHandler := &httpin.BalanceHandler{Command: commandUseCase, Query: queryUseCase}
	operationRouteHandler := &httpin.OperationRouteHandler{Command: commandUseCase, Query: queryUseCase}
	transactionRouteHandler := &httpin.TransactionRouteHandler{Command: commandUseCase, Query: queryUseCase}

	// Metadata index handler (ledger-specific)
	metadataIndexHandler := &httpin.MetadataIndexHandler{
		OnboardingMetadataRepo:  onbMgo.metadataRepo,
		TransactionMetadataRepo: txnMgo.metadataRepo,
		OnboardingMongoManager:  onbMgo.mongoManager,
		TransactionMongoManager: txnMgo.mongoManager,
	}

	// Auth
	auth := middleware.NewAuthClient(cfg.AuthHost, cfg.AuthEnabled, nil)

	// === Multi-tenant middleware ===

	routeSetup, err := buildUnifiedRouteSetup(cfg, logger, onbPG.pgManager, txnPG.pgManager, onbMgo.mongoManager, txnMgo.mongoManager, tenantCache, tenantLoader)
	if err != nil {
		doCleanup()
		return nil, err
	}

	// === Route registrars ===

	onboardingRouteRegistrar := func(router fiber.Router) {
		httpin.RegisterOnboardingRoutesToApp(router, auth, accountHandler, portfolioHandler, ledgerHandler, assetHandler, organizationHandler, segmentHandler, accountTypeHandler, routeSetup.onboardingRouteOptions)
	}

	transactionRouteRegistrar := func(router fiber.Router) {
		httpin.RegisterTransactionRoutesToApp(router, auth, transactionHandler, operationHandler, assetRateHandler, balanceHandler, operationRouteHandler, transactionRouteHandler, routeSetup.transactionRouteOptions)
	}

	ledgerRouteRegistrar := httpin.CreateRouteRegistrar(auth, metadataIndexHandler, routeSetup.ledgerRouteOptions)

	logger.Log(context.Background(), libLog.LevelInfo, "Creating unified HTTP server on "+cfg.ServerAddress)

	// === Unified server ===

	unifiedServer := NewUnifiedServer(
		cfg.ServerAddress,
		logger,
		telemetry,
		onboardingRouteRegistrar,
		transactionRouteRegistrar,
		ledgerRouteRegistrar,
	)

	// === Workers ===

	// RedisQueueConsumer: multi-tenant or single-tenant
	var redisConsumer *RedisQueueConsumer
	if cfg.MultiTenantEnabled && tenantCache != nil {
		redisConsumer = NewRedisQueueConsumerMultiTenant(logger, *transactionHandler, true, tenantCache, txnPG.pgManager, tenantServiceName)
	} else {
		redisConsumer = NewRedisQueueConsumer(logger, *transactionHandler)
	}

	// BalanceSyncWorker: multi-tenant or single-tenant
	balanceSyncWorker := initBalanceSyncWorker(internalOpts, cfg, logger, commandUseCase, txnPG.pgManager, tenantServiceName)

	logger.Log(context.Background(), libLog.LevelInfo, "Unified ledger component started successfully with single-port mode",
		libLog.String("version", cfg.Version),
		libLog.String("env", cfg.EnvName),
		libLog.String("server_address", cfg.ServerAddress),
	)

	return &Service{
		UnifiedServer:         unifiedServer,
		MultiQueueConsumer:    rmq.multiQueueConsumer,
		MultiTenantConsumer:   rmq.multiTenantConsumer,
		RedisQueueConsumer:    redisConsumer,
		BalanceSyncWorker:     balanceSyncWorker,
		EventListener:         eventListener,
		CircuitBreakerManager: rmq.circuitBreakerManager,
		Logger:                logger,
		Telemetry:             telemetry,
		metricsFactory:        rmq.metricsFactory,
	}, nil
}

// resolveLoggerEnvironment maps an env name to a libZap environment constant.
func resolveLoggerEnvironment(env string) libZap.Environment {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case string(libZap.EnvironmentProduction):
		return libZap.EnvironmentProduction
	case string(libZap.EnvironmentStaging):
		return libZap.EnvironmentStaging
	case string(libZap.EnvironmentUAT):
		return libZap.EnvironmentUAT
	case string(libZap.EnvironmentLocal):
		return libZap.EnvironmentLocal
	default:
		return libZap.EnvironmentDevelopment
	}
}

// resolveSettingsCacheTTL parses the SETTINGS_CACHE_TTL configuration value.
func resolveSettingsCacheTTL(cfg *Config, logger libLog.Logger) time.Duration {
	const defaultSettingsCacheTTL = 5 * time.Minute

	if cfg.SettingsCacheTTL == "" {
		return defaultSettingsCacheTTL
	}

	parsed, err := time.ParseDuration(cfg.SettingsCacheTTL)
	if err != nil || parsed <= 0 {
		logger.Log(context.Background(), libLog.LevelWarn, fmt.Sprintf("Invalid SETTINGS_CACHE_TTL value '%s', using default %v", cfg.SettingsCacheTTL, defaultSettingsCacheTTL))

		return defaultSettingsCacheTTL
	}

	logger.Log(context.Background(), libLog.LevelInfo, fmt.Sprintf("Settings cache TTL configured: %v", parsed))

	return parsed
}

// buildRedisConfig creates a Redis configuration from Config fields.
func buildRedisConfig(cfg *Config, logger libLog.Logger) (libRedis.Config, error) {
	redisAddresses := strings.Split(cfg.RedisHost, ",")

	if len(redisAddresses) == 0 || strings.TrimSpace(redisAddresses[0]) == "" {
		return libRedis.Config{}, fmt.Errorf("redis host is required")
	}

	topology := libRedis.Topology{}
	if cfg.RedisMasterName != "" {
		topology.Sentinel = &libRedis.SentinelTopology{Addresses: redisAddresses, MasterName: cfg.RedisMasterName}
	} else if len(redisAddresses) > 1 {
		topology.Cluster = &libRedis.ClusterTopology{Addresses: redisAddresses}
	} else {
		topology.Standalone = &libRedis.StandaloneTopology{Address: redisAddresses[0]}
	}

	var tlsCfg *libRedis.TLSConfig
	if cfg.RedisTLS {
		tlsCfg = &libRedis.TLSConfig{CACertBase64: cfg.RedisCACert}
	}

	auth := libRedis.Auth{}
	if cfg.RedisUseGCPIAM {
		auth = libRedis.Auth{GCPIAM: &libRedis.GCPIAMAuth{
			CredentialsBase64: cfg.GoogleApplicationCredentials,
			ServiceAccount:    cfg.RedisServiceAccount,
			TokenLifetime:     time.Duration(cfg.RedisTokenLifeTime) * time.Minute,
			RefreshEvery:      time.Duration(cfg.RedisTokenRefreshDuration) * time.Minute,
		}}
	} else if cfg.RedisPassword != "" {
		auth = libRedis.Auth{StaticPassword: &libRedis.StaticPasswordAuth{Password: cfg.RedisPassword}}
	}

	return libRedis.Config{
		Topology: topology,
		TLS:      tlsCfg,
		Auth:     auth,
		Options: libRedis.ConnectionOptions{
			DB:              cfg.RedisDB,
			Protocol:        cfg.RedisProtocol,
			PoolSize:        cfg.RedisPoolSize,
			MinIdleConns:    cfg.RedisMinIdleConns,
			ReadTimeout:     time.Duration(cfg.RedisReadTimeout) * time.Second,
			WriteTimeout:    time.Duration(cfg.RedisWriteTimeout) * time.Second,
			DialTimeout:     time.Duration(cfg.RedisDialTimeout) * time.Second,
			PoolTimeout:     time.Duration(cfg.RedisPoolTimeout) * time.Second,
			MaxRetries:      cfg.RedisMaxRetries,
			MinRetryBackoff: time.Duration(cfg.RedisMinRetryBackoff) * time.Millisecond,
			MaxRetryBackoff: time.Duration(cfg.RedisMaxRetryBackoff) * time.Second,
		},
		Logger: logger,
	}, nil
}

// initRedisConnection creates the shared Redis connection used by both onboarding and transaction.
func initRedisConnection(cfg *Config, logger libLog.Logger) (*libRedis.Client, error) {
	redisConfig, err := buildRedisConfig(cfg, logger)
	if err != nil {
		return nil, err
	}

	redisConnection, err := libRedis.New(context.Background(), redisConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize redis client: %w", err)
	}

	return redisConnection, nil
}

// initBalanceSyncWorker creates the balance sync worker (multi-tenant or single-tenant).
func initBalanceSyncWorker(opts *Options, cfg *Config, logger libLog.Logger, commandUC *command.UseCase, pgManager *tmpostgres.Manager, tenantServiceName string) *BalanceSyncWorker {
	syncCfg := BalanceSyncConfig{
		BatchSize:      cfg.BalanceSyncBatchSize,
		FlushTimeoutMs: cfg.BalanceSyncFlushTimeoutMs,
		PollIntervalMs: cfg.BalanceSyncPollIntervalMs,
	}

	var balanceSyncWorker *BalanceSyncWorker

	if opts != nil && opts.MultiTenantEnabled && opts.TenantCache != nil {
		balanceSyncWorker = NewBalanceSyncWorkerMT(logger, commandUC, syncCfg, true, opts.TenantCache, pgManager, tenantServiceName)
	} else {
		balanceSyncWorker = NewBalanceSyncWorker(logger, commandUC, syncCfg)
	}

	// Log the effective config (after defaults applied by the constructor).
	effectiveCfg := balanceSyncWorker.syncConfig
	logger.Log(context.Background(), libLog.LevelInfo, "BalanceSyncWorker enabled",
		libLog.Int("batch_size", effectiveCfg.BatchSize),
		libLog.Int("flush_timeout_ms", effectiveCfg.FlushTimeoutMs),
		libLog.Int("poll_interval_ms", effectiveCfg.PollIntervalMs),
	)

	return balanceSyncWorker
}

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
	} else {
		u.Path = "/"
	}

	return u.String()
}

// initTenantClient creates the multi-tenant client when multi-tenant mode is enabled.
// Returns the client, the resolved tenantServiceName, and error.
// Returns (nil, serviceName, nil) when multi-tenant is disabled.
func initTenantClient(cfg *Config, logger libLog.Logger) (*tmclient.Client, string, error) {
	tenantServiceName := cfg.ApplicationName

	if !cfg.MultiTenantEnabled {
		return nil, tenantServiceName, nil
	}

	tenantManagerURL := strings.TrimSpace(cfg.MultiTenantURL)
	if tenantManagerURL == "" {
		return nil, "", fmt.Errorf("MULTI_TENANT_URL is required when MULTI_TENANT_ENABLED=true")
	}

	tenantServiceName = strings.TrimSpace(cfg.ApplicationName)
	if tenantServiceName == "" {
		return nil, "", fmt.Errorf("APPLICATION_NAME is required when MULTI_TENANT_ENABLED=true")
	}

	tenantManagerAPIKey := strings.TrimSpace(cfg.MultiTenantServiceAPIKey)
	if tenantManagerAPIKey == "" {
		return nil, "", fmt.Errorf("MULTI_TENANT_SERVICE_API_KEY is required when MULTI_TENANT_ENABLED=true")
	}

	// Apply safe defaults for circuit breaker when not configured
	cbThreshold := cfg.MultiTenantCircuitBreakerThreshold
	if cbThreshold <= 0 {
		cbThreshold = 5
	}

	cbTimeoutSec := cfg.MultiTenantCircuitBreakerTimeoutSec
	if cbTimeoutSec <= 0 {
		cbTimeoutSec = 30
	}

	clientOpts := []tmclient.ClientOption{
		tmclient.WithServiceAPIKey(tenantManagerAPIKey),
		tmclient.WithCircuitBreaker(cbThreshold, time.Duration(cbTimeoutSec)*time.Second),
	}

	if cfg.MultiTenantCacheTTLSec >= 0 {
		clientOpts = append(clientOpts, tmclient.WithCacheTTL(time.Duration(cfg.MultiTenantCacheTTLSec)*time.Second))
	}

	if allowInsecureMultiTenantHTTP(tenantManagerURL, cfg.EnvName) {
		clientOpts = append(clientOpts, tmclient.WithAllowInsecureHTTP())
	}

	tenantClient, err := tmclient.NewClient(
		tenantManagerURL,
		logger,
		clientOpts...,
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to initialize tenant manager client: %w", err)
	}

	logger.Log(context.Background(), libLog.LevelInfo, "Multi-tenant mode enabled",
		libLog.String("service", tenantServiceName),
		libLog.Bool("tenant_manager_configured", true),
	)

	return tenantClient, tenantServiceName, nil
}

type unifiedRouteSetup struct {
	onboardingRouteOptions  *midazhttp.ProtectedRouteOptions
	transactionRouteOptions *midazhttp.ProtectedRouteOptions
	ledgerRouteOptions      *midazhttp.ProtectedRouteOptions
}

func buildUnifiedRouteSetup(
	cfg *Config,
	logger libLog.Logger,
	onboardingPGManager *tmpostgres.Manager,
	transactionPGManager *tmpostgres.Manager,
	onboardingMongoManager *tmmongo.Manager,
	transactionMongoManager *tmmongo.Manager,
	tenantCache *tenantcache.TenantCache,
	tenantLoader *tenantcache.TenantLoader,
) (*unifiedRouteSetup, error) {
	setup := &unifiedRouteSetup{}
	if !cfg.MultiTenantEnabled {
		return setup, nil
	}

	if onboardingPGManager == nil {
		return nil, fmt.Errorf("onboarding multi-tenant PostgreSQL manager not available")
	}

	if onboardingMongoManager == nil {
		return nil, fmt.Errorf("onboarding multi-tenant MongoDB manager not available")
	}

	if transactionPGManager == nil {
		return nil, fmt.Errorf("transaction multi-tenant PostgreSQL manager not available")
	}

	if transactionMongoManager == nil {
		return nil, fmt.Errorf("transaction multi-tenant MongoDB manager not available")
	}

	// Build unified tenant middleware with all module managers (PG + Mongo)
	tenantMiddleware := tmmiddleware.NewTenantMiddleware(
		tmmiddleware.WithPG(onboardingPGManager, constant.ModuleOnboarding),
		tmmiddleware.WithPG(transactionPGManager, constant.ModuleTransaction),
		tmmiddleware.WithMB(onboardingMongoManager, constant.ModuleOnboarding),
		tmmiddleware.WithMB(transactionMongoManager, constant.ModuleTransaction),
		tmmiddleware.WithTenantCache(tenantCache),
		tmmiddleware.WithTenantLoader(tenantLoader),
	)

	logger.Log(context.Background(), libLog.LevelInfo, "Tenant middleware configured",
		libLog.String("modules", "onboarding,transaction"),
	)

	authAssertion := midazhttp.MarkTrustedAuthAssertion()

	setup.onboardingRouteOptions = &midazhttp.ProtectedRouteOptions{
		PostAuthMiddlewares: []fiber.Handler{authAssertion, tenantMiddleware.WithTenantDB},
	}

	setup.transactionRouteOptions = &midazhttp.ProtectedRouteOptions{
		PostAuthMiddlewares: []fiber.Handler{authAssertion, tenantMiddleware.WithTenantDB},
	}

	setup.ledgerRouteOptions = &midazhttp.ProtectedRouteOptions{
		PostAuthMiddlewares: []fiber.Handler{authAssertion},
	}

	return setup, nil
}

// midazErrorMapper converts tenant-manager errors into Midaz-specific HTTP responses.
// It uses the standard midazhttp response helpers to ensure a consistent error format
// across all Midaz endpoints (code/title/message JSON envelope).
func midazErrorMapper(c *fiber.Ctx, err error, tenantID string) error {
	if err == nil {
		return nil
	}

	// Tenant suspended or purged → 403 (same semantics as tenant-manager /connections)
	var suspErr *tmcore.TenantSuspendedError
	if errors.As(err, &suspErr) {
		return midazhttp.Forbidden(c,
			constant.ErrTenantServiceSuspended.Error(),
			"Service Suspended",
			fmt.Sprintf("service is %s for tenant %s", suspErr.Status, tenantID),
		)
	}

	// Tenant not found → 404
	if errors.Is(err, tmcore.ErrTenantNotFound) {
		return midazhttp.NotFound(c,
			constant.ErrTenantNotFound.Error(),
			"Tenant Not Found",
			fmt.Sprintf("tenant not found: %s", tenantID),
		)
	}

	// Tenant not provisioned → 422
	if tmcore.IsTenantNotProvisionedError(err) {
		return midazhttp.UnprocessableEntity(c,
			constant.ErrTenantNotProvisioned.Error(),
			"Tenant Not Provisioned",
			"Database schema not initialized for this tenant. Contact your administrator.",
		)
	}

	// Unknown error → 503
	return midazhttp.ServiceUnavailable(c,
		constant.ErrTenantServiceUnavailable.Error(),
		"Tenant Service Unavailable",
		fmt.Sprintf("failed to resolve tenant %s: %s", tenantID, err.Error()),
	)
}

// applyConfigDefaults sets sensible defaults for Config fields that remain at their
// zero value after SetConfigFromEnvVars. This replaces the inert `default` struct tags
// which are not interpreted by SetConfigFromEnvVars.
func applyConfigDefaults(cfg *Config) {
	intDefault := func(field *int, fallback int) {
		if *field == 0 {
			*field = fallback
		}
	}

	intDefault(&cfg.RedisProtocol, 3)
	intDefault(&cfg.RedisTokenLifeTime, 60)
	intDefault(&cfg.RedisTokenRefreshDuration, 45)
	intDefault(&cfg.RedisPoolSize, 10)
	intDefault(&cfg.RedisReadTimeout, 3)
	intDefault(&cfg.RedisWriteTimeout, 3)
	intDefault(&cfg.RedisDialTimeout, 5)
	intDefault(&cfg.RedisPoolTimeout, 2)
	intDefault(&cfg.RedisMaxRetries, 3)
	intDefault(&cfg.RedisMinRetryBackoff, 8)
	intDefault(&cfg.RedisMaxRetryBackoff, 1)

	// Bulk Recorder defaults
	// BulkRecorderEnabled defaults to true when the env var is not set or empty.
	// This treats both unset and empty string as "use default" for safer behavior.
	// Explicit "true"/"false" values are parsed by SetConfigFromEnvVars before this runs.
	if os.Getenv("BULK_RECORDER_ENABLED") == "" {
		cfg.BulkRecorderEnabled = true
	}

	// BulkRecorderFlushTimeoutMs defaults to 100ms.
	intDefault(&cfg.BulkRecorderFlushTimeoutMs, 100)

	// BulkRecorderMaxRowsPerInsert defaults to 1000 (safe under PostgreSQL 65,535 param limit).
	intDefault(&cfg.BulkRecorderMaxRowsPerInsert, 1000)

	// BulkRecorderSize: derive from workers × prefetch if not explicitly set.
	// This ensures bulk size matches RabbitMQ prefetch for optimal performance.
	if cfg.BulkRecorderSize == 0 {
		workers := cfg.RabbitMQNumbersOfWorkers
		if workers == 0 {
			workers = 5 // default workers
		}

		prefetch := cfg.RabbitMQNumbersOfPrefetch
		if prefetch == 0 {
			prefetch = 10 // default prefetch
		}

		cfg.BulkRecorderSize = workers * prefetch
	}

	// Balance Sync Worker defaults (dual-trigger)
	intDefault(&cfg.BalanceSyncBatchSize, 50)
	intDefault(&cfg.BalanceSyncFlushTimeoutMs, 500)
	intDefault(&cfg.BalanceSyncPollIntervalMs, 50)
}
