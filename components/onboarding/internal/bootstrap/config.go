// Package bootstrap provides application initialization and dependency injection for the onboarding service.
//
// This package implements the application bootstrap layer, which:
//   - Loads configuration from environment variables
//   - Initializes all infrastructure dependencies (PostgreSQL, MongoDB, RabbitMQ, Redis)
//   - Creates repository instances
//   - Wires up use cases with repositories (dependency injection)
//   - Creates HTTP handlers
//   - Configures middleware (auth, telemetry, logging)
//   - Starts the HTTP server
//
// The bootstrap follows the dependency injection pattern, creating all dependencies
// in the correct order and passing them to the components that need them.
package bootstrap

import (
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	httpin "github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/query"
)

// ApplicationName is the service identifier used for logging, tracing, and monitoring.
const ApplicationName = "onboarding"

// Config is the top-level configuration struct for the onboarding service.
//
// This struct contains all environment-based configuration values loaded at startup.
// It uses struct tags to map environment variables to fields.
//
// Configuration Categories:
//   - Server: HTTP server address
//   - PostgreSQL: Primary and replica database connections
//   - MongoDB: Metadata storage configuration
//   - RabbitMQ: Message queue configuration
//   - Redis: Caching and idempotency configuration
//   - OpenTelemetry: Observability and tracing
//   - Authentication: JWT/Casdoor integration
type Config struct {
	EnvName                      string `env:"ENV_NAME"`
	LogLevel                     string `env:"LOG_LEVEL"`
	ServerAddress                string `env:"SERVER_ADDRESS"`
	PrimaryDBHost                string `env:"DB_HOST"`
	PrimaryDBUser                string `env:"DB_USER"`
	PrimaryDBPassword            string `env:"DB_PASSWORD"`
	PrimaryDBName                string `env:"DB_NAME"`
	PrimaryDBPort                string `env:"DB_PORT"`
	PrimaryDBSSLMode             string `env:"DB_SSLMODE"`
	ReplicaDBHost                string `env:"DB_REPLICA_HOST"`
	ReplicaDBUser                string `env:"DB_REPLICA_USER"`
	ReplicaDBPassword            string `env:"DB_REPLICA_PASSWORD"`
	ReplicaDBName                string `env:"DB_REPLICA_NAME"`
	ReplicaDBPort                string `env:"DB_REPLICA_PORT"`
	ReplicaDBSSLMode             string `env:"DB_REPLICA_SSLMODE"`
	MaxOpenConnections           int    `env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConnections           int    `env:"DB_MAX_IDLE_CONNS"`
	MongoURI                     string `env:"MONGO_URI"`
	MongoDBHost                  string `env:"MONGO_HOST"`
	MongoDBName                  string `env:"MONGO_NAME"`
	MongoDBUser                  string `env:"MONGO_USER"`
	MongoDBPassword              string `env:"MONGO_PASSWORD"`
	MongoDBPort                  string `env:"MONGO_PORT"`
	MongoDBParameters            string `env:"MONGO_PARAMETERS"`
	MaxPoolSize                  int    `env:"MONGO_MAX_POOL_SIZE"`
	JWKAddress                   string `env:"CASDOOR_JWK_ADDRESS"`
	RabbitURI                    string `env:"RABBITMQ_URI"`
	RabbitMQHost                 string `env:"RABBITMQ_HOST"`
	RabbitMQPortHost             string `env:"RABBITMQ_PORT_HOST"`
	RabbitMQPortAMQP             string `env:"RABBITMQ_PORT_AMQP"`
	RabbitMQUser                 string `env:"RABBITMQ_DEFAULT_USER"`
	RabbitMQPass                 string `env:"RABBITMQ_DEFAULT_PASS"`
	RabbitMQExchange             string `env:"RABBITMQ_EXCHANGE"`
	RabbitMQHealthCheckURL       string `env:"RABBITMQ_HEALTH_CHECK_URL"`
	RabbitMQKey                  string `env:"RABBITMQ_KEY"`
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
	RedisProtocol                int    `env:"REDIS_DB" default:"3"`
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
}

// InitServers initializes the onboarding service with all dependencies.
//
// This function is the main bootstrap entry point that:
// 1. Loads configuration from environment variables
// 2. Initializes logger and telemetry
// 3. Creates database connections (PostgreSQL, MongoDB, Redis)
// 4. Creates RabbitMQ connection
// 5. Initializes all repositories
// 6. Wires up use cases (command and query)
// 7. Creates HTTP handlers
// 8. Configures authentication middleware
// 9. Sets up HTTP router
// 10. Returns the configured service ready to run
//
// Dependency Injection Flow:
//
//	Config → Connections → Repositories → Use Cases → Handlers → Router → Server
//
// The function panics on critical failures (database connections, configuration errors)
// as the service cannot function without these dependencies.
//
// Returns:
//   - *Service: Fully initialized service ready to run
//
// Panics:
//   - If configuration loading fails
//   - If database connections fail
//   - If RabbitMQ connection fails
func InitServers() *Service {
	cfg := &Config{}

	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		panic(err)
	}

	logger := libZap.InitializeLogger()

	telemetry := libOpentelemetry.InitializeTelemetry(&libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    logger,
	})

	postgreSourcePrimary := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.PrimaryDBHost, cfg.PrimaryDBUser, cfg.PrimaryDBPassword, cfg.PrimaryDBName, cfg.PrimaryDBPort, cfg.PrimaryDBSSLMode)

	postgreSourceReplica := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.ReplicaDBHost, cfg.ReplicaDBUser, cfg.ReplicaDBPassword, cfg.ReplicaDBName, cfg.ReplicaDBPort, cfg.ReplicaDBSSLMode)

	postgresConnection := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: postgreSourcePrimary,
		ConnectionStringReplica: postgreSourceReplica,
		PrimaryDBName:           cfg.PrimaryDBName,
		ReplicaDBName:           cfg.ReplicaDBName,
		Component:               ApplicationName,
		Logger:                  logger,
		MaxOpenConnections:      cfg.MaxOpenConnections,
		MaxIdleConnections:      cfg.MaxIdleConnections,
	}

	mongoSource := fmt.Sprintf("%s://%s:%s@%s:%s/",
		cfg.MongoURI, cfg.MongoDBUser, cfg.MongoDBPassword, cfg.MongoDBHost, cfg.MongoDBPort)

	if cfg.MaxPoolSize <= 0 {
		cfg.MaxPoolSize = 100
	}

	if cfg.MongoDBParameters != "" {
		mongoSource += "?" + cfg.MongoDBParameters
	}

	mongoConnection := &libMongo.MongoConnection{
		ConnectionStringSource: mongoSource,
		Database:               cfg.MongoDBName,
		Logger:                 logger,
		MaxPoolSize:            uint64(cfg.MaxPoolSize),
	}

	rabbitSource := fmt.Sprintf("%s://%s:%s@%s:%s",
		cfg.RabbitURI, cfg.RabbitMQUser, cfg.RabbitMQPass, cfg.RabbitMQHost, cfg.RabbitMQPortHost)

	rabbitMQConnection := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: rabbitSource,
		HealthCheckURL:         cfg.RabbitMQHealthCheckURL,
		Host:                   cfg.RabbitMQHost,
		Port:                   cfg.RabbitMQPortAMQP,
		User:                   cfg.RabbitMQUser,
		Pass:                   cfg.RabbitMQPass,
		Logger:                 logger,
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

	redisConsumerRepository := redis.NewConsumerRedis(redisConnection)

	organizationPostgreSQLRepository := organization.NewOrganizationPostgreSQLRepository(postgresConnection)
	ledgerPostgreSQLRepository := ledger.NewLedgerPostgreSQLRepository(postgresConnection)
	segmentPostgreSQLRepository := segment.NewSegmentPostgreSQLRepository(postgresConnection)
	portfolioPostgreSQLRepository := portfolio.NewPortfolioPostgreSQLRepository(postgresConnection)
	accountPostgreSQLRepository := account.NewAccountPostgreSQLRepository(postgresConnection)
	assetPostgreSQLRepository := asset.NewAssetPostgreSQLRepository(postgresConnection)
	accountTypePostgreSQLRepository := accounttype.NewAccountTypePostgreSQLRepository(postgresConnection)

	metadataMongoDBRepository := mongodb.NewMetadataMongoDBRepository(mongoConnection)

	producerRabbitMQRepository := rabbitmq.NewProducerRabbitMQ(rabbitMQConnection)

	commandUseCase := &command.UseCase{
		OrganizationRepo: organizationPostgreSQLRepository,
		LedgerRepo:       ledgerPostgreSQLRepository,
		SegmentRepo:      segmentPostgreSQLRepository,
		PortfolioRepo:    portfolioPostgreSQLRepository,
		AccountRepo:      accountPostgreSQLRepository,
		AssetRepo:        assetPostgreSQLRepository,
		AccountTypeRepo:  accountTypePostgreSQLRepository,
		MetadataRepo:     metadataMongoDBRepository,
		RabbitMQRepo:     producerRabbitMQRepository,
		RedisRepo:        redisConsumerRepository,
	}

	queryUseCase := &query.UseCase{
		OrganizationRepo: organizationPostgreSQLRepository,
		LedgerRepo:       ledgerPostgreSQLRepository,
		SegmentRepo:      segmentPostgreSQLRepository,
		PortfolioRepo:    portfolioPostgreSQLRepository,
		AccountRepo:      accountPostgreSQLRepository,
		AssetRepo:        assetPostgreSQLRepository,
		AccountTypeRepo:  accountTypePostgreSQLRepository,
		MetadataRepo:     metadataMongoDBRepository,
		RedisRepo:        redisConsumerRepository,
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
	}
}
