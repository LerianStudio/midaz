// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libCrypto "github.com/LerianStudio/lib-commons/v2/commons/crypto"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services"
	pkgMongo "github.com/LerianStudio/midaz/v3/pkg/mongo"
)

// Config is the top level configuration struct for the entire application.
type Config struct {
	EnvName                 string `env:"ENV_NAME"`
	ProtoAddress            string `env:"PROTO_ADDRESS"`
	ServerAddress           string `env:"SERVER_ADDRESS"`
	LogLevel                string `env:"LOG_LEVEL"`
	OtelServiceName         string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName         string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion      string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv       string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry         bool   `env:"ENABLE_TELEMETRY"`
	MongoURI                string `env:"MONGO_URI"`
	MongoDBHost             string `env:"MONGO_HOST"`
	MongoDBName             string `env:"MONGO_NAME"`
	MongoDBUser             string `env:"MONGO_USER"`
	MongoDBPassword         string `env:"MONGO_PASSWORD"`
	MongoDBPort             string `env:"MONGO_PORT"`
	MongoDBParameters       string `env:"MONGO_PARAMETERS"`
	MaxPoolSize             int    `env:"MONGO_MAX_POOL_SIZE"`
	HashSecretKey           string `env:"LCRYPTO_HASH_SECRET_KEY"`
	EncryptSecretKey        string `env:"LCRYPTO_ENCRYPT_SECRET_KEY"`
	AuthAddress             string `env:"PLUGIN_AUTH_ADDRESS"`
	AuthEnabled             bool   `env:"PLUGIN_AUTH_ENABLED"`
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

		logger, err = libZap.InitializeLoggerWithError()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize logger: %w", err)
		}
	}

	// Init Open telemetry to control logs and flows
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

	// Mongo DB
	// Extract port and parameters for MongoDB connection (handles backward compatibility)
	mongoPort, mongoParameters := pkgMongo.ExtractMongoPortAndParameters(cfg.MongoDBPort, cfg.MongoDBParameters, logger)

	// Build MongoDB connection string using centralized utility (ensures correct format)
	mongoSource := libMongo.BuildConnectionString(
		cfg.MongoURI, cfg.MongoDBUser, cfg.MongoDBPassword, cfg.MongoDBHost, mongoPort, mongoParameters, logger)

	if cfg.MaxPoolSize <= 0 {
		cfg.MaxPoolSize = 100
	}

	mongoConnection := &libMongo.MongoConnection{
		ConnectionStringSource: mongoSource,
		Database:               cfg.MongoDBName,
		Logger:                 logger,
		MaxPoolSize:            uint64(cfg.MaxPoolSize),
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

	auth := middleware.NewAuthClient(cfg.AuthAddress, cfg.AuthEnabled, &logger)

	httpApp := in.NewRouter(logger, telemetry, auth, holderHandler, aliasHandler)
	serverAPI := NewServer(cfg, httpApp, logger, telemetry)

	return &Service{
		Server: serverAPI,
		Logger: logger,
	}, nil
}
