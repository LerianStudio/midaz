// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	libRedis "github.com/LerianStudio/lib-commons/v3/commons/redis"
	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	grpcout "github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/grpc/out"
	httpin "github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mgrpc"
)

const ApplicationName = "onboarding"

// envFallback returns the prefixed value if not empty, otherwise returns the fallback value.
func envFallback(prefixed, fallback string) string {
	if prefixed != "" {
		return prefixed
	}

	return fallback
}

// envFallbackInt returns the prefixed value if not zero, otherwise returns the fallback value.
func envFallbackInt(prefixed, fallback int) int {
	if prefixed != 0 {
		return prefixed
	}

	return fallback
}

// Config is the top level configuration struct for the entire application.
// Supports prefixed env vars (DB_ONBOARDING_*) with fallback to non-prefixed (DB_*) for backward compatibility.
type Config struct {
	EnvName  string `env:"ENV_NAME"`
	LogLevel string `env:"LOG_LEVEL"`

	// Server address - prefixed for unified ledger deployment
	PrefixedServerAddress string `env:"SERVER_ADDRESS_ONBOARDING"`
	ServerAddress         string `env:"SERVER_ADDRESS"`

	// PostgreSQL Primary - prefixed vars for unified ledger deployment
	PrefixedPrimaryDBHost     string `env:"DB_ONBOARDING_HOST"`
	PrefixedPrimaryDBUser     string `env:"DB_ONBOARDING_USER"`
	PrefixedPrimaryDBPassword string `env:"DB_ONBOARDING_PASSWORD"`
	PrefixedPrimaryDBName     string `env:"DB_ONBOARDING_NAME"`
	PrefixedPrimaryDBPort     string `env:"DB_ONBOARDING_PORT"`
	PrefixedPrimaryDBSSLMode  string `env:"DB_ONBOARDING_SSLMODE"`

	// PostgreSQL Primary - fallback vars for standalone deployment
	PrimaryDBHost     string `env:"DB_HOST"`
	PrimaryDBUser     string `env:"DB_USER"`
	PrimaryDBPassword string `env:"DB_PASSWORD"`
	PrimaryDBName     string `env:"DB_NAME"`
	PrimaryDBPort     string `env:"DB_PORT"`
	PrimaryDBSSLMode  string `env:"DB_SSLMODE"`

	// PostgreSQL Replica - prefixed vars for unified ledger deployment
	PrefixedReplicaDBHost     string `env:"DB_ONBOARDING_REPLICA_HOST"`
	PrefixedReplicaDBUser     string `env:"DB_ONBOARDING_REPLICA_USER"`
	PrefixedReplicaDBPassword string `env:"DB_ONBOARDING_REPLICA_PASSWORD"`
	PrefixedReplicaDBName     string `env:"DB_ONBOARDING_REPLICA_NAME"`
	PrefixedReplicaDBPort     string `env:"DB_ONBOARDING_REPLICA_PORT"`
	PrefixedReplicaDBSSLMode  string `env:"DB_ONBOARDING_REPLICA_SSLMODE"`

	// PostgreSQL Replica - fallback vars for standalone deployment
	ReplicaDBHost     string `env:"DB_REPLICA_HOST"`
	ReplicaDBUser     string `env:"DB_REPLICA_USER"`
	ReplicaDBPassword string `env:"DB_REPLICA_PASSWORD"`
	ReplicaDBName     string `env:"DB_REPLICA_NAME"`
	ReplicaDBPort     string `env:"DB_REPLICA_PORT"`
	ReplicaDBSSLMode  string `env:"DB_REPLICA_SSLMODE"`

	// PostgreSQL connection pool - prefixed with fallback
	PrefixedMaxOpenConnections int `env:"DB_ONBOARDING_MAX_OPEN_CONNS"`
	PrefixedMaxIdleConnections int `env:"DB_ONBOARDING_MAX_IDLE_CONNS"`
	MaxOpenConnections         int `env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConnections         int `env:"DB_MAX_IDLE_CONNS"`

	// MongoDB - prefixed vars for unified ledger deployment
	PrefixedMongoURI          string `env:"MONGO_ONBOARDING_URI"`
	PrefixedMongoDBHost       string `env:"MONGO_ONBOARDING_HOST"`
	PrefixedMongoDBName       string `env:"MONGO_ONBOARDING_NAME"`
	PrefixedMongoDBUser       string `env:"MONGO_ONBOARDING_USER"`
	PrefixedMongoDBPassword   string `env:"MONGO_ONBOARDING_PASSWORD"`
	PrefixedMongoDBPort       string `env:"MONGO_ONBOARDING_PORT"`
	PrefixedMongoDBParameters string `env:"MONGO_ONBOARDING_PARAMETERS"`
	PrefixedMaxPoolSize       int    `env:"MONGO_ONBOARDING_MAX_POOL_SIZE"`

	// MongoDB - fallback vars for standalone deployment
	MongoURI                     string `env:"MONGO_URI"`
	MongoDBHost                  string `env:"MONGO_HOST"`
	MongoDBName                  string `env:"MONGO_NAME"`
	MongoDBUser                  string `env:"MONGO_USER"`
	MongoDBPassword              string `env:"MONGO_PASSWORD"`
	MongoDBPort                  string `env:"MONGO_PORT"`
	MongoDBParameters            string `env:"MONGO_PARAMETERS"`
	MaxPoolSize                  int    `env:"MONGO_MAX_POOL_SIZE"`
	JWKAddress                   string `env:"CASDOOR_JWK_ADDRESS"`
	OtelServiceName              string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName              string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion           string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv            string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint      string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry              bool   `env:"ENABLE_TELEMETRY"`
	RedisHost                    string `env:"REDIS_HOST"`
	RedisMasterName              string `env:"REDIS_MASTER_NAME" default:""`
	RedisPassword                string `env:"REDIS_PASSWORD"`
	RedisDB                      int    `env:"REDIS_DB" default:"0"`
	RedisProtocol                int    `env:"REDIS_PROTOCOL" default:"3"`
	RedisTLS                     bool   `env:"REDIS_TLS" default:"false"`
	RedisCACert                  string `env:"REDIS_CA_CERT"`
	RedisUseGCPIAM               bool   `env:"REDIS_USE_GCP_IAM" default:"false"`
	RedisServiceAccount          string `env:"REDIS_SERVICE_ACCOUNT" default:""`
	GoogleApplicationCredentials string `env:"GOOGLE_APPLICATION_CREDENTIALS" default:""`
	RedisTokenLifeTime           int    `env:"REDIS_TOKEN_LIFETIME" default:"60"`
	RedisTokenRefreshDuration    int    `env:"REDIS_TOKEN_REFRESH_DURATION" default:"45"`
	RedisPoolSize                int    `env:"REDIS_POOL_SIZE" default:"10"`
	RedisMinIdleConns            int    `env:"REDIS_MIN_IDLE_CONNS" default:"0"`
	RedisReadTimeout             int    `env:"REDIS_READ_TIMEOUT" default:"3"`
	RedisWriteTimeout            int    `env:"REDIS_WRITE_TIMEOUT" default:"3"`
	RedisDialTimeout             int    `env:"REDIS_DIAL_TIMEOUT" default:"5"`
	RedisPoolTimeout             int    `env:"REDIS_POOL_TIMEOUT" default:"2"`
	RedisMaxRetries              int    `env:"REDIS_MAX_RETRIES" default:"3"`
	RedisMinRetryBackoff         int    `env:"REDIS_MIN_RETRY_BACKOFF" default:"8"`
	RedisMaxRetryBackoff         int    `env:"REDIS_MAX_RETRY_BACKOFF" default:"1"`
	AuthEnabled                  bool   `env:"PLUGIN_AUTH_ENABLED"`
	AuthHost                     string `env:"PLUGIN_AUTH_HOST"`
	TransactionGRPCAddress       string `env:"TRANSACTION_GRPC_ADDRESS"`
	TransactionGRPCPort          string `env:"TRANSACTION_GRPC_PORT"`

	// SettingsCacheTTL is the TTL for cached ledger settings.
	// Format: Go duration string (e.g., "5m", "1h", "30s"). Default: 5m.
	SettingsCacheTTL string `env:"SETTINGS_CACHE_TTL"`
}

// Options contains optional dependencies that can be injected when running
// in unified ledger mode. When nil, defaults to gRPC-based communication.
type Options struct {
	// Logger allows callers (e.g. cmd/app) to provide a pre-configured logger,
	// avoiding double initialization and ensuring consistent output.
	Logger libLog.Logger

	// UnifiedMode indicates the service is running as part of the unified ledger.
	// When true, all ports must be provided for in-process communication.
	// When false (or Options is nil), uses gRPC adapters for remote communication.
	UnifiedMode bool

	// BalancePort is the transaction module's BalancePort for direct calls.
	// Required when UnifiedMode is true.
	// This is typically the transaction.UseCase which implements mbootstrap.BalancePort.
	BalancePort mbootstrap.BalancePort

	// Multi-tenant configuration (only used in unified ledger mode).
	MultiTenantEnabled bool
	TenantClient       *tmclient.Client
	TenantServiceName  string
	TenantEnvironment  string
}

// InitServers initiate http and grpc servers using default gRPC communication.
func InitServers() (*Service, error) {
	return InitServersWithOptions(nil)
}

// InitServersWithOptions initiates http servers with optional dependency injection.
// When opts is nil or opts.UnifiedMode is false, uses gRPC for balance operations.
// When opts.UnifiedMode is true, uses direct in-process calls (unified ledger mode).
func InitServersWithOptions(opts *Options) (*Service, error) {
	cfg := &Config{}

	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config from environment variables: %w", err)
	}

	var logger libLog.Logger
	if opts != nil && opts.Logger != nil {
		logger = opts.Logger
	} else {
		var err error

		logger, err = libZap.InitializeLoggerWithError()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize logger: %w", err)
		}
	}

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

	// PostgreSQL: single-tenant or multi-tenant (decided internally)
	pg, err := initPostgres(opts, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize PostgreSQL: %w", err)
	}

	// MongoDB: single-tenant or multi-tenant (decided internally)
	mgo, err := initMongo(opts, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MongoDB: %w", err)
	}

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

	redisConsumerRepository, err := redis.NewConsumerRedis(redisConnection)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize redis: %w", err)
	}

	organizationPostgreSQLRepository := pg.organizationRepo
	ledgerPostgreSQLRepository := pg.ledgerRepo
	segmentPostgreSQLRepository := pg.segmentRepo
	portfolioPostgreSQLRepository := pg.portfolioRepo
	accountPostgreSQLRepository := pg.accountRepo
	assetPostgreSQLRepository := pg.assetRepo
	accountTypePostgreSQLRepository := pg.accountTypeRepo

	// Choose balance port based on UnifiedMode:
	// - If UnifiedMode is true, validate and use provided ports for in-process calls
	// - Otherwise, use gRPC adapter to call the separate transaction service
	var balancePort mbootstrap.BalancePort

	if opts != nil && opts.UnifiedMode {
		if opts.BalancePort == nil {
			return nil, fmt.Errorf("unified mode requires BalancePort to be provided")
		}

		logger.Info("Running in UNIFIED MODE - using direct balance port (in-process calls)")

		balancePort = opts.BalancePort
	} else {
		if cfg.TransactionGRPCAddress == "" {
			cfg.TransactionGRPCAddress = "midaz-transaction"

			logger.Warn("TRANSACTION_GRPC_ADDRESS not set, using default: midaz-transaction")
		}

		if cfg.TransactionGRPCPort == "" {
			cfg.TransactionGRPCPort = "3011"

			logger.Warn("TRANSACTION_GRPC_PORT not set, using default: 3011")
		}

		grpcConnection := &mgrpc.GRPCConnection{
			Addr:   fmt.Sprintf("%s:%s", cfg.TransactionGRPCAddress, cfg.TransactionGRPCPort),
			Logger: logger,
		}

		logger.Info("Running in MICROSERVICES MODE - using gRPC balance adapter (network calls)")

		balancePort = grpcout.NewBalanceAdapter(grpcConnection)
	}

	commandUseCase := &command.UseCase{
		OrganizationRepo: organizationPostgreSQLRepository,
		LedgerRepo:       ledgerPostgreSQLRepository,
		SegmentRepo:      segmentPostgreSQLRepository,
		PortfolioRepo:    portfolioPostgreSQLRepository,
		AccountRepo:      accountPostgreSQLRepository,
		AssetRepo:        assetPostgreSQLRepository,
		AccountTypeRepo:  accountTypePostgreSQLRepository,
		MetadataRepo:     mgo.metadataRepo,
		RedisRepo:        redisConsumerRepository,
		BalancePort:      balancePort,
	}

	// Parse settings cache TTL from config (default: 5m via query.DefaultSettingsCacheTTL)
	var settingsCacheTTL time.Duration

	if cfg.SettingsCacheTTL != "" {
		if parsed, err := time.ParseDuration(cfg.SettingsCacheTTL); err == nil && parsed > 0 {
			settingsCacheTTL = parsed
			logger.Infof("Settings cache TTL configured: %v", settingsCacheTTL)
		} else {
			logger.Warnf("Invalid SETTINGS_CACHE_TTL value '%s', using default", cfg.SettingsCacheTTL)
		}
	}

	queryUseCase := &query.UseCase{
		OrganizationRepo: organizationPostgreSQLRepository,
		LedgerRepo:       ledgerPostgreSQLRepository,
		SegmentRepo:      segmentPostgreSQLRepository,
		PortfolioRepo:    portfolioPostgreSQLRepository,
		AccountRepo:      accountPostgreSQLRepository,
		AssetRepo:        assetPostgreSQLRepository,
		AccountTypeRepo:  accountTypePostgreSQLRepository,
		MetadataRepo:     mgo.metadataRepo,
		RedisRepo:        redisConsumerRepository,
		SettingsCacheTTL: settingsCacheTTL,
	}

	accountHandler := &httpin.AccountHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	portfolioHandler := &httpin.PortfolioHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	ledgerHandler := &httpin.LedgerHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	assetHandler := &httpin.AssetHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	organizationHandler := &httpin.OrganizationHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	segmentHandler := &httpin.SegmentHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	accountTypeHandler := &httpin.AccountTypeHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	auth := middleware.NewAuthClient(cfg.AuthHost, cfg.AuthEnabled, &logger)

	httpApp := httpin.NewRouter(logger, telemetry, auth, accountHandler, portfolioHandler, ledgerHandler, assetHandler, organizationHandler, segmentHandler, accountTypeHandler)

	serverAPI := NewServer(cfg, httpApp, logger, telemetry)

	return &Service{
		Server: serverAPI,
		Logger: logger,
		Ports: Ports{
			MetadataPort: mgo.metadataRepo,
			SettingsPort: queryUseCase,
		},
		auth:                auth,
		accountHandler:      accountHandler,
		portfolioHandler:    portfolioHandler,
		ledgerHandler:       ledgerHandler,
		assetHandler:        assetHandler,
		organizationHandler: organizationHandler,
		segmentHandler:      segmentHandler,
		accountTypeHandler:  accountTypeHandler,
	}, nil
}
