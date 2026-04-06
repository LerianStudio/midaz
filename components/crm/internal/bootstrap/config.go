// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libCrypto "github.com/LerianStudio/lib-commons/v4/commons/crypto"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v4/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v4/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services"
	pkgMongo "github.com/LerianStudio/midaz/v3/pkg/mongo"
)

// Config is the top level configuration struct for the entire application.
type Config struct {
	EnvName                                string `env:"ENV_NAME"`
	ProtoAddress                           string `env:"PROTO_ADDRESS"`
	ServerAddress                          string `env:"SERVER_ADDRESS"`
	LogLevel                               string `env:"LOG_LEVEL"`
	OtelServiceName                        string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName                        string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion                     string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv                      string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint                string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry                        bool   `env:"ENABLE_TELEMETRY"`
	MongoURI                               string `env:"MONGO_URI"`
	MongoDBHost                            string `env:"MONGO_HOST"`
	MongoDBName                            string `env:"MONGO_NAME"`
	MongoDBUser                            string `env:"MONGO_USER"`
	MongoDBPassword                        string `env:"MONGO_PASSWORD"`
	MongoDBPort                            string `env:"MONGO_PORT"`
	MongoDBParameters                      string `env:"MONGO_PARAMETERS"`
	MaxPoolSize                            int    `env:"MONGO_MAX_POOL_SIZE"`
	HashSecretKey                          string `env:"LCRYPTO_HASH_SECRET_KEY"`
	EncryptSecretKey                       string `env:"LCRYPTO_ENCRYPT_SECRET_KEY"`
	AuthAddress                            string `env:"PLUGIN_AUTH_ADDRESS"`
	AuthEnabled                            bool   `env:"PLUGIN_AUTH_ENABLED"`
	MultiTenantEnabled                     bool   `env:"MULTI_TENANT_ENABLED"`
	MultiTenantURL                         string `env:"MULTI_TENANT_URL"`
	MultiTenantTimeout                     int    `env:"MULTI_TENANT_TIMEOUT"`                     // seconds (HTTP client timeout)
	MultiTenantIdleTimeoutSec              int    `env:"MULTI_TENANT_IDLE_TIMEOUT_SEC"`            // seconds before idle connection eviction
	MultiTenantMaxTenantPools              int    `env:"MULTI_TENANT_MAX_TENANT_POOLS"`            // max concurrent tenant pools
	MultiTenantCircuitBreakerThreshold     int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD"`   // failures before circuit opens
	MultiTenantCircuitBreakerTimeoutSec    int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC"` // seconds before circuit resets
	MultiTenantServiceAPIKey               string `env:"MULTI_TENANT_SERVICE_API_KEY"`
	MultiTenantConnectionsCheckIntervalSec int    `env:"MULTI_TENANT_CONNECTIONS_CHECK_INTERVAL_SEC"` // seconds between tenant config revalidation checks
	MultiTenantCacheTTLSec                 int    `env:"MULTI_TENANT_CACHE_TTL_SEC" default:"120"`    // seconds for tenant config cache TTL (0 = disabled)
	MultiTenantRedisHost                   string `env:"MULTI_TENANT_REDIS_HOST"`
	MultiTenantRedisPort                   string `env:"MULTI_TENANT_REDIS_PORT"`
	MultiTenantRedisPassword               string `env:"MULTI_TENANT_REDIS_PASSWORD"`
	MultiTenantRedisTLS                    bool   `env:"MULTI_TENANT_REDIS_TLS"`
	ApplicationName                        string `env:"APPLICATION_NAME"`
}

// Options contains optional dependencies that can be injected by callers.
type Options struct {
	Logger libLog.Logger
}

// InitServers initiate http and grpc servers.
func InitServers() (*Service, error) {
	return InitServersWithOptions(nil)
}

// InitServersWithOptions initializes the CRM service with optional dependency injection.
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

		logger, err = libZap.New(libZap.Config{
			Environment:     resolveLoggerEnvironment(cfg.EnvName),
			Level:           cfg.LogLevel,
			OTelLibraryName: cfg.OtelLibraryName,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to initialize logger: %w", err)
		}
	}

	if cfg.MultiTenantEnabled {
		logger.Log(context.Background(), libLog.LevelInfo, "Multi-tenant mode ENABLED")
	} else {
		logger.Log(context.Background(), libLog.LevelInfo, "Running in SINGLE-TENANT MODE")
	}

	if cfg.MultiTenantEnabled && !cfg.AuthEnabled {
		return nil, fmt.Errorf("MULTI_TENANT_ENABLED=true requires PLUGIN_AUTH_ENABLED=true")
	}

	// Init Open telemetry to control logs and flows
	telemetry, err := libOpentelemetry.NewTelemetry(libOpentelemetry.TelemetryConfig{
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

	// Register telemetry providers as process-global so that the otelzap bridge
	// (installed in the logger core) can forward log records to the OTLP exporter.
	if err := telemetry.ApplyGlobals(); err != nil {
		return nil, fmt.Errorf("failed to apply telemetry globals: %w", err)
	}

	mongoConnection, err := initMongoConnection(cfg, logger)
	if err != nil {
		return nil, err
	}

	dataSecurity := &libCrypto.Crypto{
		HashSecretKey:    cfg.HashSecretKey,
		EncryptSecretKey: cfg.EncryptSecretKey,
		Logger:           logger,
	}

	err = dataSecurity.InitializeCipher()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cipher: %w", err)
	}

	holderMongoDBRepository, err := holder.NewMongoDBRepository(mongoConnection, dataSecurity)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize holder repository: %w", err)
	}

	aliasMongoDBRepository, err := alias.NewMongoDBRepository(mongoConnection, dataSecurity)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize alias repository: %w", err)
	}

	useCases := &services.UseCase{
		HolderRepo: holderMongoDBRepository,
		AliasRepo:  aliasMongoDBRepository,
	}

	holderHandler := &in.HolderHandler{
		Service: useCases,
	}

	aliasHandler := &in.AliasHandler{
		Service: useCases,
	}

	auth := middleware.NewAuthClient(cfg.AuthAddress, cfg.AuthEnabled, nil)

	tenantMiddleware, eventListener, err := initTenantMiddleware(cfg, logger, telemetry)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tenant middleware: %w", err)
	}

	httpApp := in.NewRouter(logger, telemetry, auth, tenantMiddleware, holderHandler, aliasHandler)
	serverAPI := NewServer(cfg, httpApp, logger, telemetry)

	return &Service{
		Server:        serverAPI,
		EventListener: eventListener,
		Logger:        logger,
	}, nil
}

func initMongoConnection(cfg *Config, logger libLog.Logger) (*libMongo.Client, error) {
	mongoPort, mongoParameters := pkgMongo.ExtractMongoPortAndParameters(cfg.MongoDBPort, cfg.MongoDBParameters, logger)

	hasStaticMongo := strings.TrimSpace(cfg.MongoURI) != "" || strings.TrimSpace(cfg.MongoDBHost) != ""
	if !hasStaticMongo {
		if cfg.MultiTenantEnabled {
			logger.Log(context.Background(), libLog.LevelInfo, "No static MongoDB configuration; multi-tenant mode will use tenant-specific connections")

			return nil, nil
		}

		return nil, fmt.Errorf("mongo configuration is required in single-tenant mode")
	}

	mongoURI, err := resolveMongoURI(cfg, mongoPort, mongoParameters)
	if err != nil {
		return nil, err
	}

	if cfg.MaxPoolSize <= 0 {
		logger.Log(context.Background(), libLog.LevelInfo, fmt.Sprintf("MaxPoolSize invalid (%d); defaulting to 100", cfg.MaxPoolSize))
		cfg.MaxPoolSize = 100
	}

	mongoConnection, err := libMongo.NewClient(context.Background(), libMongo.Config{
		URI:         mongoURI,
		Database:    cfg.MongoDBName,
		MaxPoolSize: uint64(cfg.MaxPoolSize), // #nosec G115 -- guarded by <= 0 check above
		Logger:      logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize mongodb client: %w", err)
	}

	return mongoConnection, nil
}

func resolveMongoURI(cfg *Config, mongoPort, mongoParameters string) (string, error) {
	rawURI := strings.TrimSpace(cfg.MongoURI)

	switch {
	case rawURI == "", rawURI == "mongodb", rawURI == "mongodb+srv":
		query, err := url.ParseQuery(mongoParameters)
		if err != nil {
			return "", fmt.Errorf("failed to parse mongodb parameters: %w", err)
		}

		scheme := rawURI
		if scheme == "" {
			scheme = "mongodb"
		}

		mongoURI, buildErr := libMongo.BuildURI(libMongo.URIConfig{
			Scheme:   scheme,
			Username: cfg.MongoDBUser,
			Password: cfg.MongoDBPassword,
			Host:     cfg.MongoDBHost,
			Port:     mongoPort,
			Query:    query,
		})
		if buildErr != nil {
			return "", fmt.Errorf("failed to build mongodb uri: %w", buildErr)
		}

		return mongoURI, nil
	case strings.Contains(rawURI, "://"):
		return rawURI, nil
	default:
		return "", fmt.Errorf("invalid MONGO_URI format: expected full URI or legacy scheme value")
	}
}

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
