// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	grpcIn "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/grpc/in"
	grpcOut "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/grpc/out"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redpanda"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
	brokerpkg "github.com/LerianStudio/midaz/v3/pkg/broker"
	brokersecurity "github.com/LerianStudio/midaz/v3/pkg/broker/security"
	pkgMongo "github.com/LerianStudio/midaz/v3/pkg/mongo"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const ApplicationName = "transaction"

// initLogger initializes the logger from options or creates a new one.
func initLogger(opts *Options) (libLog.Logger, error) {
	if opts != nil && opts.Logger != nil {
		return opts.Logger, nil
	}

	return libZap.InitializeLoggerWithError()
}

func resolveCircuitBreakerStateListener(
	opts *Options,
	metricStateListener libCircuitBreaker.StateChangeListener,
) libCircuitBreaker.StateChangeListener {
	if metricStateListener == nil {
		if opts != nil {
			return opts.CircuitBreakerStateListener
		}

		return nil
	}

	if opts != nil && opts.CircuitBreakerStateListener != nil {
		return &compositeStateListener{
			listeners: []libCircuitBreaker.StateChangeListener{
				metricStateListener,
				opts.CircuitBreakerStateListener,
			},
		}
	}

	return metricStateListener
}

func resolveBrokerOperationTimeout(rawTimeout string) time.Duration {
	operationTimeout := redpanda.DefaultOperationTimeout

	if rawTimeout != "" {
		if parsed, err := time.ParseDuration(rawTimeout); err == nil && parsed > 0 {
			operationTimeout = parsed
		}
	}

	return operationTimeout
}

func enforcePostgresSSLMode(envName, sslMode, envVar string) error {
	if brokersecurity.IsNonProductionEnvironment(envName) {
		return nil
	}

	if !strings.EqualFold(strings.TrimSpace(sslMode), "disable") {
		return nil
	}

	return fmt.Errorf("%s=disable is not allowed in production-like environments", envVar)
}

func newBalanceSyncWorker(
	cfg *Config,
	logger libLog.Logger,
	redisConnection *libRedis.RedisConnection,
	useCase *command.UseCase,
	balanceSyncWorkerEnabled bool,
) *BalanceSyncWorker {
	const defaultBalanceSyncMaxWorkers = 5

	balanceSyncMaxWorkers := cfg.BalanceSyncMaxWorkers
	if balanceSyncMaxWorkers <= 0 {
		balanceSyncMaxWorkers = defaultBalanceSyncMaxWorkers
		logger.Infof("BalanceSyncWorker using default: BALANCE_SYNC_MAX_WORKERS=%d", defaultBalanceSyncMaxWorkers)
	}

	if balanceSyncWorkerEnabled {
		balanceSyncWorker := NewBalanceSyncWorker(redisConnection, logger, useCase, balanceSyncMaxWorkers)
		logger.Infof("BalanceSyncWorker enabled with %d max workers.", balanceSyncMaxWorkers)

		return balanceSyncWorker
	}

	logger.Info("BalanceSyncWorker disabled.")

	return nil
}

// initShardRouting initializes the shard router and manager for Redis Cluster sharding (Phase 2A).
// Returns (nil, nil) when sharding is disabled (REDIS_SHARD_COUNT=0).
func initShardRouting(
	cfg *Config,
	logger libLog.Logger,
	redisConnection *libRedis.RedisConnection,
) (*shard.Router, *internalsharding.Manager) {
	if cfg.RedisShardCount <= 0 {
		logger.Info("Redis sharding disabled (REDIS_SHARD_COUNT=0)")

		return nil, nil
	}

	shardRouter := shard.NewRouter(cfg.RedisShardCount)
	logger.Infof("Redis sharding enabled: %d shards", cfg.RedisShardCount)

	shardManager := internalsharding.NewManager(redisConnection, shardRouter, logger, internalsharding.Config{})

	return shardRouter, shardManager
}

// brokerProducerResult holds the outputs of producer + circuit breaker initialization.
type brokerProducerResult struct {
	producer              redpanda.ProducerRepository
	circuitBreakerManager *CircuitBreakerManager
}

// initProducerWithCircuitBreaker creates the producer and wraps it with a circuit breaker.
func initProducerWithCircuitBreaker(
	cfg *Config,
	opts *Options,
	logger libLog.Logger,
	brokers []string,
	telemetry *libOpentelemetry.Telemetry,
) (*brokerProducerResult, error) {
	linger := time.Duration(cfg.RedpandaProducerLingerMS) * time.Millisecond
	securityConfig := redpanda.ClientSecurityConfig{
		TLSEnabled:            cfg.RedpandaTLSEnabled,
		TLSInsecureSkipVerify: cfg.RedpandaTLSInsecureSkipVerify,
		TLSCAFile:             cfg.RedpandaTLSCAFile,
		SASLEnabled:           cfg.RedpandaSASLEnabled,
		SASLMechanism:         cfg.RedpandaSASLMechanism,
		SASLUsername:          cfg.RedpandaSASLUsername,
		SASLPassword:          cfg.RedpandaSASLPassword,
	}

	rawProducer, err := redpanda.NewProducerRedpandaWithSecurity(
		brokers,
		linger,
		cfg.RedpandaMaxBufferedRecords,
		cfg.TransactionAsync,
		securityConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redpanda producer: %w", err)
	}

	var metricStateListener libCircuitBreaker.StateChangeListener
	if telemetry != nil && telemetry.MetricsFactory != nil {
		metricStateListener, err = redpanda.NewMetricStateListener(telemetry.MetricsFactory)
		if err != nil {
			if closeErr := rawProducer.Close(); closeErr != nil {
				logger.Warnf("Failed to close producer during cleanup: %v", closeErr)
			}

			return nil, fmt.Errorf("failed to create metric state listener: %w", err)
		}
	} else {
		logger.Warn("Telemetry metrics factory unavailable; circuit breaker metrics listener disabled")
	}

	stateListener := resolveCircuitBreakerStateListener(opts, metricStateListener)
	operationTimeout := resolveBrokerOperationTimeout(cfg.BrokerOperationTimeout)

	cbConfig := redpanda.CircuitBreakerConfig{
		ConsecutiveFailures: utils.GetUint32FromIntWithDefault(cfg.BrokerCircuitBreakerConsecutiveFailures, 15),
		FailureRatio:        utils.GetFloat64FromIntPercentWithDefault(cfg.BrokerCircuitBreakerFailureRatio, 0.5),
		Interval:            utils.GetDurationSecondsWithDefault(cfg.BrokerCircuitBreakerInterval, 2*time.Minute),
		MaxRequests:         utils.GetUint32FromIntWithDefault(cfg.BrokerCircuitBreakerMaxRequests, 3),
		MinRequests:         utils.GetUint32FromIntWithDefault(cfg.BrokerCircuitBreakerMinRequests, 10),
		Timeout:             utils.GetDurationSecondsWithDefault(cfg.BrokerCircuitBreakerTimeout, 30*time.Second),
		HealthCheckInterval: utils.GetDurationSecondsWithDefault(cfg.BrokerCircuitBreakerHealthCheckInterval, 30*time.Second),
		OperationTimeout:    operationTimeout,
	}

	circuitBreakerManager, err := NewCircuitBreakerManager(logger, cbConfig, stateListener)
	if err != nil {
		if closeErr := rawProducer.Close(); closeErr != nil {
			logger.Warnf("Failed to close producer during cleanup: %v", closeErr)
		}

		return nil, fmt.Errorf("failed to create circuit breaker manager: %w", err)
	}

	circuitBreakerManager.SetHealthChecker(rawProducer)

	producerRepo, err := redpanda.NewCircuitBreakerProducer(
		rawProducer,
		circuitBreakerManager.Manager,
		logger,
		cbConfig.OperationTimeout,
	)
	if err != nil {
		if closeErr := rawProducer.Close(); closeErr != nil {
			logger.Warnf("Failed to close producer during cleanup: %v", closeErr)
		}

		return nil, fmt.Errorf("failed to create circuit breaker producer: %w", err)
	}

	return &brokerProducerResult{
		producer:              producerRepo,
		circuitBreakerManager: circuitBreakerManager,
	}, nil
}

func newShardRebalanceWorker(
	cfg *Config,
	logger libLog.Logger,
	shardManager *internalsharding.Manager,
	shardRouter *shard.Router,
	enabled bool,
) *ShardRebalanceWorker {
	if !enabled {
		logger.Info("ShardRebalanceWorker disabled.")

		return nil
	}

	if shardManager == nil || shardRouter == nil {
		logger.Info("ShardRebalanceWorker disabled: sharding manager/router unavailable")

		return nil
	}

	interval := time.Duration(cfg.ShardRebalanceIntervalSeconds) * time.Second
	window := time.Duration(cfg.ShardRebalanceWindowSeconds) * time.Second
	threshold := float64(cfg.ShardRebalanceThresholdPercent) / 100.0
	candidateLimit := cfg.ShardRebalanceCandidateLimit
	isolationShare := float64(cfg.ShardRebalanceIsolationSharePercent) / 100.0
	isolationMinLoad := cfg.ShardRebalanceIsolationMinLoad

	worker := NewShardRebalanceWorker(
		logger,
		shardManager,
		shardRouter,
		interval,
		window,
		threshold,
		candidateLimit,
		isolationShare,
		isolationMinLoad,
	)
	logger.Infof(
		"ShardRebalanceWorker enabled interval=%s window=%s threshold=%.2f candidate_limit=%d isolation_share=%.2f isolation_min_load=%d",
		interval,
		window,
		threshold,
		candidateLimit,
		isolationShare,
		isolationMinLoad,
	)

	return worker
}

// Config is the top level configuration struct for the entire application.
// Supports prefixed env vars (DB_TRANSACTION_*) with fallback to non-prefixed (DB_*) for backward compatibility.
type Config struct {
	EnvName  string `env:"ENV_NAME" default:"development"`
	LogLevel string `env:"LOG_LEVEL"`
	Version  string `env:"VERSION" default:"v3"`

	// Server address - prefixed for unified ledger deployment
	PrefixedServerAddress string `env:"SERVER_ADDRESS_TRANSACTION"`
	ServerAddress         string `env:"SERVER_ADDRESS"`

	// PostgreSQL Primary - prefixed vars for unified ledger deployment
	PrefixedPrimaryDBHost     string `env:"DB_TRANSACTION_HOST"`
	PrefixedPrimaryDBUser     string `env:"DB_TRANSACTION_USER"`
	PrefixedPrimaryDBPassword string `env:"DB_TRANSACTION_PASSWORD"`
	PrefixedPrimaryDBName     string `env:"DB_TRANSACTION_NAME"`
	PrefixedPrimaryDBPort     string `env:"DB_TRANSACTION_PORT"`
	PrefixedPrimaryDBSSLMode  string `env:"DB_TRANSACTION_SSLMODE"`

	// PostgreSQL Primary - fallback vars for standalone deployment
	PrimaryDBHost     string `env:"DB_HOST"`
	PrimaryDBUser     string `env:"DB_USER"`
	PrimaryDBPassword string `env:"DB_PASSWORD"`
	PrimaryDBName     string `env:"DB_NAME"`
	PrimaryDBPort     string `env:"DB_PORT"`
	PrimaryDBSSLMode  string `env:"DB_SSLMODE"`

	// PostgreSQL Replica - prefixed vars for unified ledger deployment
	PrefixedReplicaDBHost     string `env:"DB_TRANSACTION_REPLICA_HOST"`
	PrefixedReplicaDBUser     string `env:"DB_TRANSACTION_REPLICA_USER"`
	PrefixedReplicaDBPassword string `env:"DB_TRANSACTION_REPLICA_PASSWORD"`
	PrefixedReplicaDBName     string `env:"DB_TRANSACTION_REPLICA_NAME"`
	PrefixedReplicaDBPort     string `env:"DB_TRANSACTION_REPLICA_PORT"`
	PrefixedReplicaDBSSLMode  string `env:"DB_TRANSACTION_REPLICA_SSLMODE"`

	// PostgreSQL Replica - fallback vars for standalone deployment
	ReplicaDBHost     string `env:"DB_REPLICA_HOST"`
	ReplicaDBUser     string `env:"DB_REPLICA_USER"`
	ReplicaDBPassword string `env:"DB_REPLICA_PASSWORD"`
	ReplicaDBName     string `env:"DB_REPLICA_NAME"`
	ReplicaDBPort     string `env:"DB_REPLICA_PORT"`
	ReplicaDBSSLMode  string `env:"DB_REPLICA_SSLMODE"`

	// PostgreSQL connection pool - prefixed with fallback
	PrefixedMaxOpenConnections int `env:"DB_TRANSACTION_MAX_OPEN_CONNS"`
	PrefixedMaxIdleConnections int `env:"DB_TRANSACTION_MAX_IDLE_CONNS"`
	MaxOpenConnections         int `env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConnections         int `env:"DB_MAX_IDLE_CONNS"`

	// MongoDB - prefixed vars for unified ledger deployment
	PrefixedMongoURI          string `env:"MONGO_TRANSACTION_URI"`
	PrefixedMongoDBHost       string `env:"MONGO_TRANSACTION_HOST"`
	PrefixedMongoDBName       string `env:"MONGO_TRANSACTION_NAME"`
	PrefixedMongoDBUser       string `env:"MONGO_TRANSACTION_USER"`
	PrefixedMongoDBPassword   string `env:"MONGO_TRANSACTION_PASSWORD"`
	PrefixedMongoDBPort       string `env:"MONGO_TRANSACTION_PORT"`
	PrefixedMongoDBParameters string `env:"MONGO_TRANSACTION_PARAMETERS"`
	PrefixedMaxPoolSize       int    `env:"MONGO_TRANSACTION_MAX_POOL_SIZE"`

	// MongoDB - fallback vars for standalone deployment
	MongoURI                            string `env:"MONGO_URI"`
	MongoDBHost                         string `env:"MONGO_HOST"`
	MongoDBName                         string `env:"MONGO_NAME"`
	MongoDBUser                         string `env:"MONGO_USER"`
	MongoDBPassword                     string `env:"MONGO_PASSWORD"`
	MongoDBPort                         string `env:"MONGO_PORT"`
	MongoDBParameters                   string `env:"MONGO_PARAMETERS"`
	MaxPoolSize                         int    `env:"MONGO_MAX_POOL_SIZE"`
	CasdoorAddress                      string `env:"CASDOOR_ADDRESS"`
	CasdoorClientID                     string `env:"CASDOOR_CLIENT_ID"`
	CasdoorClientSecret                 string `env:"CASDOOR_CLIENT_SECRET"`
	CasdoorOrganizationName             string `env:"CASDOOR_ORGANIZATION_NAME"`
	CasdoorApplicationName              string `env:"CASDOOR_APPLICATION_NAME"`
	CasdoorModelName                    string `env:"CASDOOR_MODEL_NAME"`
	JWKAddress                          string `env:"CASDOOR_JWK_ADDRESS"`
	RedpandaBrokers                     string `env:"REDPANDA_BROKERS" default:"127.0.0.1:9092"`
	RedpandaBalanceCreateTopic          string `env:"REDPANDA_BALANCE_CREATE_TOPIC" default:"ledger.balance.create"`
	RedpandaBalanceOperationsTopic      string `env:"REDPANDA_BALANCE_OPS_TOPIC" default:"ledger.balance.operations"`
	RedpandaEventsTopic                 string `env:"REDPANDA_EVENTS_TOPIC" default:"ledger.transaction.events"`
	RedpandaAuditTopic                  string `env:"REDPANDA_AUDIT_TOPIC" default:"ledger.audit.log"`
	RedpandaConsumerGroup               string `env:"REDPANDA_CONSUMER_GROUP" default:"midaz-balance-projector"`
	RedpandaNumbersOfWorkers            int    `env:"REDPANDA_NUMBERS_OF_WORKERS" default:"5"`
	RedpandaFetchMaxBytes               int    `env:"REDPANDA_FETCH_MAX_BYTES" default:"50000000"`
	RedpandaMaxRetryAttempts            int    `env:"REDPANDA_MAX_RETRY_ATTEMPTS" default:"3"`
	RedpandaProducerLingerMS            int    `env:"REDPANDA_PRODUCER_LINGER_MS" default:"5"`
	RedpandaMaxBufferedRecords          int    `env:"REDPANDA_MAX_BUFFERED_RECORDS" default:"10000"`
	RedpandaTLSEnabled                  bool   `env:"REDPANDA_TLS_ENABLED" default:"false"`
	RedpandaTLSInsecureSkipVerify       bool   `env:"REDPANDA_TLS_INSECURE_SKIP_VERIFY" default:"false"`
	RedpandaTLSCAFile                   string `env:"REDPANDA_TLS_CA_FILE"`
	RedpandaSASLEnabled                 bool   `env:"REDPANDA_SASL_ENABLED" default:"false"`
	RedpandaSASLMechanism               string `env:"REDPANDA_SASL_MECHANISM" default:"SCRAM-SHA-256"`
	RedpandaSASLUsername                string `env:"REDPANDA_SASL_USERNAME"`
	RedpandaSASLPassword                string `env:"REDPANDA_SASL_PASSWORD"`
	TransactionEventsEnabled            bool   `env:"TRANSACTION_EVENTS_ENABLED" default:"true"`
	AuditLogEnabled                     bool   `env:"AUDIT_LOG_ENABLED" default:"true"`
	OtelServiceName                     string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName                     string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion                  string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv                   string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint             string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry                     bool   `env:"ENABLE_TELEMETRY"`
	RedisHost                           string `env:"REDIS_HOST"`
	RedisMasterName                     string `env:"REDIS_MASTER_NAME" default:""`
	RedisPassword                       string `env:"REDIS_PASSWORD"`
	RedisDB                             int    `env:"REDIS_DB" default:"0"`
	RedisProtocol                       int    `env:"REDIS_PROTOCOL" default:"3"`
	RedisTLS                            bool   `env:"REDIS_TLS" default:"false"`
	RedisCACert                         string `env:"REDIS_CA_CERT"`
	RedisUseGCPIAM                      bool   `env:"REDIS_USE_GCP_IAM" default:"false"`
	RedisServiceAccount                 string `env:"REDIS_SERVICE_ACCOUNT" default:""`
	GoogleApplicationCredentials        string `env:"GOOGLE_APPLICATION_CREDENTIALS" default:""`
	RedisTokenLifeTime                  int    `env:"REDIS_TOKEN_LIFETIME" default:"60"`
	RedisTokenRefreshDuration           int    `env:"REDIS_TOKEN_REFRESH_DURATION" default:"45"`
	RedisPoolSize                       int    `env:"REDIS_POOL_SIZE" default:"10"`
	RedisMinIdleConns                   int    `env:"REDIS_MIN_IDLE_CONNS" default:"0"`
	RedisReadTimeout                    int    `env:"REDIS_READ_TIMEOUT" default:"3"`
	RedisWriteTimeout                   int    `env:"REDIS_WRITE_TIMEOUT" default:"3"`
	RedisDialTimeout                    int    `env:"REDIS_DIAL_TIMEOUT" default:"5"`
	RedisPoolTimeout                    int    `env:"REDIS_POOL_TIMEOUT" default:"2"`
	RedisMaxRetries                     int    `env:"REDIS_MAX_RETRIES" default:"3"`
	RedisMinRetryBackoff                int    `env:"REDIS_MIN_RETRY_BACKOFF" default:"8"`
	RedisMaxRetryBackoff                int    `env:"REDIS_MAX_RETRY_BACKOFF" default:"1"`
	AuthEnabled                         bool   `env:"PLUGIN_AUTH_ENABLED"`
	AuthHost                            string `env:"PLUGIN_AUTH_HOST"`
	ProtoAddress                        string `env:"PROTO_ADDRESS"`
	AuthorizerEnabled                   bool   `env:"AUTHORIZER_ENABLED" default:"false"`
	AuthorizerHost                      string `env:"AUTHORIZER_HOST" default:"127.0.0.1"`
	AuthorizerPort                      string `env:"AUTHORIZER_PORT" default:"50051"`
	AuthorizerTimeoutMS                 int    `env:"AUTHORIZER_TIMEOUT_MS" default:"100"`
	AuthorizerUseStreaming              bool   `env:"AUTHORIZER_USE_STREAMING" default:"false"`
	AuthorizerGRPCTLSEnabled            bool   `env:"AUTHORIZER_GRPC_TLS_ENABLED" default:"false"`
	BalanceSyncWorkerEnabled            bool   `env:"BALANCE_SYNC_WORKER_ENABLED" default:"true"`
	BalanceSyncMaxWorkers               int    `env:"BALANCE_SYNC_MAX_WORKERS"`
	ShardRebalanceWorkerEnabled         bool   `env:"SHARD_REBALANCE_WORKER_ENABLED" default:"false"`
	ShardRebalanceIntervalSeconds       int    `env:"SHARD_REBALANCE_INTERVAL_SECONDS" default:"5"`
	ShardRebalanceWindowSeconds         int    `env:"SHARD_REBALANCE_WINDOW_SECONDS" default:"60"`
	ShardRebalanceThresholdPercent      int    `env:"SHARD_REBALANCE_THRESHOLD_PERCENT" default:"150"`
	ShardRebalanceCandidateLimit        int    `env:"SHARD_REBALANCE_CANDIDATE_LIMIT" default:"8"`
	ShardRebalanceIsolationSharePercent int    `env:"SHARD_REBALANCE_ISOLATION_SHARE_PERCENT" default:"70"`
	ShardRebalanceIsolationMinLoad      int64  `env:"SHARD_REBALANCE_ISOLATION_MIN_LOAD" default:"250"`

	// Transaction async mode - when true, transactions are published to Redpanda for async processing.
	// Resolved once at startup and injected into UseCase to avoid per-request os.Getenv overhead.
	TransactionAsync bool `env:"TRANSACTION_ASYNC" default:"false"`

	// Redis Cluster sharding (Phase 2A)
	// Set REDIS_SHARD_COUNT > 0 to enable per-shard Lua execution.
	// Default 0 = sharding disabled (legacy single-slot mode).
	RedisShardCount int `env:"REDIS_SHARD_COUNT" default:"0"`

	// Circuit Breaker configuration for producer
	BrokerCircuitBreakerConsecutiveFailures int    `env:"BROKER_CIRCUIT_BREAKER_CONSECUTIVE_FAILURES" default:"15"`
	BrokerCircuitBreakerFailureRatio        int    `env:"BROKER_CIRCUIT_BREAKER_FAILURE_RATIO" default:"50"`
	BrokerCircuitBreakerInterval            int    `env:"BROKER_CIRCUIT_BREAKER_INTERVAL" default:"120"`
	BrokerCircuitBreakerMaxRequests         int    `env:"BROKER_CIRCUIT_BREAKER_MAX_REQUESTS" default:"3"`
	BrokerCircuitBreakerMinRequests         int    `env:"BROKER_CIRCUIT_BREAKER_MIN_REQUESTS" default:"10"`
	BrokerCircuitBreakerTimeout             int    `env:"BROKER_CIRCUIT_BREAKER_TIMEOUT" default:"30"`
	BrokerCircuitBreakerHealthCheckInterval int    `env:"BROKER_CIRCUIT_BREAKER_HEALTH_CHECK_INTERVAL" default:"30"`
	BrokerOperationTimeout                  string `env:"BROKER_OPERATION_TIMEOUT" default:"3s"`
}

// Options contains optional dependencies that can be injected by callers.
type Options struct {
	// Logger allows callers to provide a pre-configured logger, avoiding double
	// initialization when the cmd/app wants to handle bootstrap errors.
	Logger libLog.Logger

	// CircuitBreakerStateListener receives notifications when circuit breaker state changes.
	// This is optional - pass nil if you don't need state change notifications.
	CircuitBreakerStateListener libCircuitBreaker.StateChangeListener
}

// InitServers initiate http and grpc servers.
func InitServers() (*Service, error) {
	return InitServersWithOptions(nil)
}

// InitServersWithOptions initiates http and grpc servers with optional dependency injection.
func InitServersWithOptions(opts *Options) (*Service, error) {
	cfg := &Config{}

	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config from environment variables: %w", err)
	}

	var cleanupFuncs []func()
	success := false

	defer func() {
		if success {
			return
		}

		for i := len(cleanupFuncs) - 1; i >= 0; i-- {
			cleanupFuncs[i]()
		}
	}()

	logger, err := initLogger(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	warnings, err := brokersecurity.ValidateRuntimeConfig(brokersecurity.RuntimeConfig{
		Environment:           cfg.EnvName,
		TLSEnabled:            cfg.RedpandaTLSEnabled,
		TLSInsecureSkipVerify: cfg.RedpandaTLSInsecureSkipVerify,
		SASLEnabled:           cfg.RedpandaSASLEnabled,
	})
	if err != nil {
		return nil, err
	}

	for _, warning := range warnings {
		logger.Warnf("Redpanda security warning: %s (ENV_NAME=%s)", warning, cfg.EnvName)
	}

	deprecatedBrokerEnvs := brokerpkg.DeprecatedBrokerEnvVariables(os.Environ())
	if len(deprecatedBrokerEnvs) > 0 {
		logger.Warnf(
			"Deprecated broker environment variables detected (ignored by this version): %s. Regenerate .env from .env.example and remove deprecated entries.",
			strings.Join(deprecatedBrokerEnvs, ", "),
		)
	}

	// BalanceSyncWorkerEnabled defaults to true via struct tag
	balanceSyncWorkerEnabled := cfg.BalanceSyncWorkerEnabled
	logger.Infof("BalanceSyncWorker: BALANCE_SYNC_WORKER_ENABLED=%v", balanceSyncWorkerEnabled)

	shardRebalanceWorkerEnabled := cfg.ShardRebalanceWorkerEnabled
	logger.Infof("ShardRebalanceWorker: SHARD_REBALANCE_WORKER_ENABLED=%v", shardRebalanceWorkerEnabled)

	telemetry, err := libOpentelemetry.InitializeTelemetryWithError(&libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	cleanupFuncs = append(cleanupFuncs, func() {
		telemetry.ShutdownTelemetry()
	})

	// Apply fallback for prefixed env vars (unified ledger) to non-prefixed (standalone)
	dbHost := utils.EnvFallback(cfg.PrefixedPrimaryDBHost, cfg.PrimaryDBHost)
	dbUser := utils.EnvFallback(cfg.PrefixedPrimaryDBUser, cfg.PrimaryDBUser)
	dbPassword := utils.EnvFallback(cfg.PrefixedPrimaryDBPassword, cfg.PrimaryDBPassword)
	dbName := utils.EnvFallback(cfg.PrefixedPrimaryDBName, cfg.PrimaryDBName)
	dbPort := utils.EnvFallback(cfg.PrefixedPrimaryDBPort, cfg.PrimaryDBPort)
	dbSSLMode := strings.TrimSpace(utils.EnvFallback(cfg.PrefixedPrimaryDBSSLMode, cfg.PrimaryDBSSLMode))
	if dbSSLMode == "" {
		dbSSLMode = "disable"
	}

	if err := enforcePostgresSSLMode(cfg.EnvName, dbSSLMode, "DB_TRANSACTION_SSLMODE"); err != nil {
		return nil, err
	}

	dbReplicaHost := utils.EnvFallback(cfg.PrefixedReplicaDBHost, cfg.ReplicaDBHost)
	dbReplicaUser := utils.EnvFallback(cfg.PrefixedReplicaDBUser, cfg.ReplicaDBUser)
	dbReplicaPassword := utils.EnvFallback(cfg.PrefixedReplicaDBPassword, cfg.ReplicaDBPassword)
	dbReplicaName := utils.EnvFallback(cfg.PrefixedReplicaDBName, cfg.ReplicaDBName)
	dbReplicaPort := utils.EnvFallback(cfg.PrefixedReplicaDBPort, cfg.ReplicaDBPort)
	dbReplicaSSLMode := strings.TrimSpace(utils.EnvFallback(cfg.PrefixedReplicaDBSSLMode, cfg.ReplicaDBSSLMode))
	if dbReplicaSSLMode == "" {
		dbReplicaSSLMode = dbSSLMode
	}

	if err := enforcePostgresSSLMode(cfg.EnvName, dbReplicaSSLMode, "DB_TRANSACTION_REPLICA_SSLMODE"); err != nil {
		return nil, err
	}

	maxOpenConns := utils.EnvFallbackInt(cfg.PrefixedMaxOpenConnections, cfg.MaxOpenConnections)
	maxIdleConns := utils.EnvFallbackInt(cfg.PrefixedMaxIdleConnections, cfg.MaxIdleConnections)

	postgreSourcePrimary := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		dbHost, dbUser, dbPassword, dbName, dbPort, dbSSLMode)

	postgreSourceReplica := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		dbReplicaHost, dbReplicaUser, dbReplicaPassword, dbReplicaName, dbReplicaPort, dbReplicaSSLMode)

	postgresConnection := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: postgreSourcePrimary,
		ConnectionStringReplica: postgreSourceReplica,
		PrimaryDBName:           dbName,
		ReplicaDBName:           dbReplicaName,
		Component:               ApplicationName,
		Logger:                  logger,
		MaxOpenConnections:      maxOpenConns,
		MaxIdleConnections:      maxIdleConns,
	}

	cleanupFuncs = append(cleanupFuncs, func() {
		if postgresConnection.ConnectionDB != nil {
			if closeErr := (*postgresConnection.ConnectionDB).Close(); closeErr != nil {
				logger.Warnf("Failed to close transaction PostgreSQL connection during cleanup: %v", closeErr)
			}
		}
	})

	// Apply fallback for MongoDB prefixed env vars
	mongoURI := utils.EnvFallback(cfg.PrefixedMongoURI, cfg.MongoURI)
	mongoHost := utils.EnvFallback(cfg.PrefixedMongoDBHost, cfg.MongoDBHost)
	mongoName := utils.EnvFallback(cfg.PrefixedMongoDBName, cfg.MongoDBName)
	mongoUser := utils.EnvFallback(cfg.PrefixedMongoDBUser, cfg.MongoDBUser)
	mongoPassword := utils.EnvFallback(cfg.PrefixedMongoDBPassword, cfg.MongoDBPassword)
	mongoPortRaw := utils.EnvFallback(cfg.PrefixedMongoDBPort, cfg.MongoDBPort)
	mongoParametersRaw := utils.EnvFallback(cfg.PrefixedMongoDBParameters, cfg.MongoDBParameters)
	mongoPoolSize := utils.EnvFallbackInt(cfg.PrefixedMaxPoolSize, cfg.MaxPoolSize)

	// Extract port and parameters for MongoDB connection (handles backward compatibility)
	mongoPort, mongoParameters := pkgMongo.ExtractMongoPortAndParameters(mongoPortRaw, mongoParametersRaw, logger)

	// Build MongoDB connection string using centralized utility (ensures correct format)
	mongoSource := libMongo.BuildConnectionString(
		mongoURI, mongoUser, mongoPassword, mongoHost, mongoPort, mongoParameters, logger)

	// Safe conversion: use uint64 with default, only assign if positive
	var mongoMaxPoolSize uint64 = 100
	if mongoPoolSize > 0 {
		mongoMaxPoolSize = uint64(mongoPoolSize)
	}

	mongoConnection := &libMongo.MongoConnection{
		ConnectionStringSource: mongoSource,
		Database:               mongoName,
		Logger:                 logger,
		MaxPoolSize:            mongoMaxPoolSize,
	}

	cleanupFuncs = append(cleanupFuncs, func() {
		if mongoConnection.DB != nil {
			if closeErr := mongoConnection.DB.Disconnect(context.Background()); closeErr != nil {
				logger.Warnf("Failed to disconnect transaction MongoDB client during cleanup: %v", closeErr)
			}
		}
	})

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
		PoolSize:                     cfg.RedisPoolSize,
		MinIdleConns:                 cfg.RedisMinIdleConns,
		ReadTimeout:                  time.Duration(cfg.RedisReadTimeout) * time.Second,
		WriteTimeout:                 time.Duration(cfg.RedisWriteTimeout) * time.Second,
		DialTimeout:                  time.Duration(cfg.RedisDialTimeout) * time.Second,
		PoolTimeout:                  time.Duration(cfg.RedisPoolTimeout) * time.Second,
		MaxRetries:                   cfg.RedisMaxRetries,
		MinRetryBackoff:              time.Duration(cfg.RedisMinRetryBackoff) * time.Millisecond,
		MaxRetryBackoff:              time.Duration(cfg.RedisMaxRetryBackoff) * time.Second,
	}

	cleanupFuncs = append(cleanupFuncs, func() {
		if closeErr := redisConnection.Close(); closeErr != nil {
			logger.Warnf("Failed to close transaction Redis connection during cleanup: %v", closeErr)
		}
	})

	shardRouter, shardManager := initShardRouting(cfg, logger, redisConnection)

	redisConsumerRepository, err := redis.NewConsumerRedis(redisConnection, balanceSyncWorkerEnabled, shardRouter)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize redis: %w", err)
	}

	transactionPostgreSQLRepository := transaction.NewTransactionPostgreSQLRepository(postgresConnection)
	operationPostgreSQLRepository := operation.NewOperationPostgreSQLRepository(postgresConnection)
	assetRatePostgreSQLRepository := assetrate.NewAssetRatePostgreSQLRepository(postgresConnection)
	balancePostgreSQLRepository := balance.NewBalancePostgreSQLRepository(postgresConnection)
	operationRoutePostgreSQLRepository := operationroute.NewOperationRoutePostgreSQLRepository(postgresConnection)
	transactionRoutePostgreSQLRepository := transactionroute.NewTransactionRoutePostgreSQLRepository(postgresConnection)

	metadataMongoDBRepository := mongodb.NewMetadataMongoDBRepository(mongoConnection)

	// Ensure indexes also for known base collections on fresh installs
	ctxEnsureIndexes, cancelEnsureIndexes := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelEnsureIndexes()

	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "entity_id", Value: 1}},
		Options: options.Index().
			SetUnique(false),
	}

	collections := []string{"operation", "transaction", "operation_route", "transaction_route"}
	for _, collection := range collections {
		if err := mongoConnection.EnsureIndexes(ctxEnsureIndexes, collection, indexModel); err != nil {
			logger.Warnf("Failed to ensure indexes for collection %s: %v", collection, err)
		}
	}

	seedBrokers := redpanda.ParseSeedBrokers(cfg.RedpandaBrokers)

	producerResult, err := initProducerWithCircuitBreaker(cfg, opts, logger, seedBrokers, telemetry)
	if err != nil {
		return nil, err
	}

	cleanupFuncs = append(cleanupFuncs, func() {
		if producerResult.circuitBreakerManager != nil {
			producerResult.circuitBreakerManager.Stop()
		}

		if producerResult.producer != nil {
			if closeErr := producerResult.producer.Close(); closeErr != nil {
				logger.Warnf("Failed to close transaction producer during cleanup: %v", closeErr)
			}
		}
	})

	producerRepository := producerResult.producer
	circuitBreakerManager := producerResult.circuitBreakerManager

	useCase := &command.UseCase{
		TransactionRepo:        transactionPostgreSQLRepository,
		OperationRepo:          operationPostgreSQLRepository,
		AssetRateRepo:          assetRatePostgreSQLRepository,
		BalanceRepo:            balancePostgreSQLRepository,
		OperationRouteRepo:     operationRoutePostgreSQLRepository,
		TransactionRouteRepo:   transactionRoutePostgreSQLRepository,
		MetadataRepo:           metadataMongoDBRepository,
		BrokerRepo:             producerRepository,
		RedisRepo:              redisConsumerRepository,
		ShardRouter:            shardRouter,
		ShardManager:           shardManager,
		BalanceOperationsTopic: cfg.RedpandaBalanceOperationsTopic,
		BalanceCreateTopic:     cfg.RedpandaBalanceCreateTopic,
		EventsTopic:            cfg.RedpandaEventsTopic,
		EventsEnabled:          cfg.TransactionEventsEnabled,
		AuditTopic:             cfg.RedpandaAuditTopic,
		AuditLogEnabled:        cfg.AuditLogEnabled,
		TransactionAsync:       cfg.TransactionAsync,
		Version:                cfg.Version,
	}

	queryUseCase := &query.UseCase{
		TransactionRepo:      transactionPostgreSQLRepository,
		OperationRepo:        operationPostgreSQLRepository,
		AssetRateRepo:        assetRatePostgreSQLRepository,
		BalanceRepo:          balancePostgreSQLRepository,
		OperationRouteRepo:   operationRoutePostgreSQLRepository,
		TransactionRouteRepo: transactionRoutePostgreSQLRepository,
		MetadataRepo:         metadataMongoDBRepository,
		RedisRepo:            redisConsumerRepository,
		ShardRouter:          shardRouter,
		ShardManager:         shardManager,
	}

	authorizerClient, err := grpcOut.NewAuthorizerGRPC(
		grpcOut.AuthorizerConfig{
			Enabled:    cfg.AuthorizerEnabled,
			Host:       cfg.AuthorizerHost,
			Port:       cfg.AuthorizerPort,
			Timeout:    time.Duration(cfg.AuthorizerTimeoutMS) * time.Millisecond,
			Streaming:  cfg.AuthorizerUseStreaming,
			TLSEnabled: cfg.AuthorizerGRPCTLSEnabled,
		},
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize authorizer client: %w", err)
	}

	cleanupFuncs = append(cleanupFuncs, func() {
		if closeErr := authorizerClient.Close(); closeErr != nil {
			logger.Warnf("Failed to close transaction authorizer connection during cleanup: %v", closeErr)
		}
	})

	queryUseCase.Authorizer = authorizerClient
	useCase.Authorizer = authorizerClient

	transactionHandler := &in.TransactionHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	operationHandler := &in.OperationHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	assetRateHandler := &in.AssetRateHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	balanceHandler := &in.BalanceHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	operationRouteHandler := &in.OperationRouteHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	transactionRouteHandler := &in.TransactionRouteHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	routes := redpanda.NewConsumerRoutesWithSecurity(
		seedBrokers,
		cfg.RedpandaConsumerGroup,
		cfg.RedpandaNumbersOfWorkers,
		cfg.RedpandaFetchMaxBytes,
		logger,
		telemetry,
		redpanda.ClientSecurityConfig{
			TLSEnabled:            cfg.RedpandaTLSEnabled,
			TLSInsecureSkipVerify: cfg.RedpandaTLSInsecureSkipVerify,
			TLSCAFile:             cfg.RedpandaTLSCAFile,
			SASLEnabled:           cfg.RedpandaSASLEnabled,
			SASLMechanism:         cfg.RedpandaSASLMechanism,
			SASLUsername:          cfg.RedpandaSASLUsername,
			SASLPassword:          cfg.RedpandaSASLPassword,
		},
		cfg.RedpandaMaxRetryAttempts,
	)

	multiQueueConsumer := NewMultiQueueConsumer(routes, useCase)

	auth := middleware.NewAuthClient(cfg.AuthHost, cfg.AuthEnabled, &logger)

	app := in.NewRouter(logger, telemetry, auth, transactionHandler, operationHandler, assetRateHandler, balanceHandler, operationRouteHandler, transactionRouteHandler)

	server := NewServer(cfg, app, logger, telemetry)

	if cfg.ProtoAddress == "" || cfg.ProtoAddress == ":" {
		cfg.ProtoAddress = ":3011"

		logger.Warn("PROTO_ADDRESS not set or invalid, using default: :3011")
	}

	grpcApp := grpcIn.NewRouterGRPC(logger, telemetry, auth, useCase, queryUseCase)
	serverGRPC := NewServerGRPC(cfg, grpcApp, logger, telemetry)

	redisConsumer := NewRedisQueueConsumer(logger, *transactionHandler)

	balanceSyncWorker := newBalanceSyncWorker(cfg, logger, redisConnection, useCase, balanceSyncWorkerEnabled)
	shardRebalanceWorker := newShardRebalanceWorker(cfg, logger, shardManager, shardRouter, shardRebalanceWorkerEnabled)
	resolvedShardRebalanceWorkerEnabled := shardRebalanceWorker != nil

	service := &Service{
		Server:                      server,
		ServerGRPC:                  serverGRPC,
		MultiQueueConsumer:          multiQueueConsumer,
		RedisQueueConsumer:          redisConsumer,
		BalanceSyncWorker:           balanceSyncWorker,
		BalanceSyncWorkerEnabled:    balanceSyncWorkerEnabled,
		ShardRebalanceWorker:        shardRebalanceWorker,
		ShardRebalanceWorkerEnabled: resolvedShardRebalanceWorkerEnabled,
		CircuitBreakerManager:       circuitBreakerManager,
		Logger:                      logger,
		Ports: Ports{
			BalancePort:  useCase,
			MetadataPort: metadataMongoDBRepository,
		},
		authorizerCloser:        authorizerClient,
		auth:                    auth,
		transactionHandler:      transactionHandler,
		operationHandler:        operationHandler,
		assetRateHandler:        assetRateHandler,
		balanceHandler:          balanceHandler,
		operationRouteHandler:   operationRouteHandler,
		transactionRouteHandler: transactionRouteHandler,
		brokerProducer:          producerResult.producer,
		telemetry:               telemetry,
		postgresConnection:      postgresConnection,
		mongoConnection:         mongoConnection,
		redisConnection:         redisConnection,
	}

	success = true

	return service, nil
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
