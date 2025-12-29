// Package bootstrap provides initialization and configuration for the CRM service.
package bootstrap

import (
	"fmt"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libCrypto "github.com/LerianStudio/lib-commons/v2/commons/crypto"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	holderlink "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder-link"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
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
	MaxPoolSize             int    `env:"MONGO_MAX_POOL_SIZE"`
	HashSecretKey           string `env:"LCRYPTO_HASH_SECRET_KEY"`
	EncryptSecretKey        string `env:"LCRYPTO_ENCRYPT_SECRET_KEY"`
	AuthAddress             string `env:"PLUGIN_AUTH_ADDRESS"`
	AuthEnabled             bool   `env:"PLUGIN_AUTH_ENABLED"`
	LicenseKey              string `env:"LICENSE_KEY"`
	OrganizationIDs         string `env:"ORGANIZATION_IDS"`
}

// Validate validates the configuration and panics with clear error messages if invalid.
// This method should be called immediately after loading configuration from environment.
func (cfg *Config) Validate() {
	// Server configuration
	assert.NotEmpty(cfg.ServerAddress, "SERVER_ADDRESS is required",
		"field", "ServerAddress")

	// MongoDB configuration
	assert.NotEmpty(cfg.MongoDBHost, "MONGO_HOST is required",
		"field", "MongoDBHost")
	assert.NotEmpty(cfg.MongoDBName, "MONGO_NAME is required",
		"field", "MongoDBName")
	assert.That(assert.ValidPort(cfg.MongoDBPort), "MONGO_PORT must be valid port (1-65535)",
		"field", "MongoDBPort", "value", cfg.MongoDBPort)
	assert.That(assert.InRangeInt(cfg.MaxPoolSize, 1, 1000), "MONGO_MAX_POOL_SIZE must be 1-1000",
		"field", "MaxPoolSize", "value", cfg.MaxPoolSize)

	// Crypto configuration (required for data security)
	assert.NotEmpty(cfg.HashSecretKey, "LCRYPTO_HASH_SECRET_KEY is required",
		"field", "HashSecretKey")
	assert.NotEmpty(cfg.EncryptSecretKey, "LCRYPTO_ENCRYPT_SECRET_KEY is required",
		"field", "EncryptSecretKey")
}

// InitServers initiate http and grpc servers.
func InitServers() *Service {
	cfg := &Config{}

	err := libCommons.SetConfigFromEnvVars(cfg)
	assert.NoError(err, "configuration required for CRM",
		"package", "bootstrap",
		"function", "InitServers")

	// Validate configuration before proceeding
	cfg.Validate()

	logger := libZap.InitializeLogger()

	// Init Open telemetry to control logs and flows
	telemetry := libOpentelemetry.InitializeTelemetry(&libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    logger,
	})

	// Mongo DB
	mongoSource := fmt.Sprintf("%s://%s:%s@%s:%s",
		cfg.MongoURI, cfg.MongoDBUser, cfg.MongoDBPassword, cfg.MongoDBHost, cfg.MongoDBPort)

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
	assert.NoError(err, "cipher initialization required for CRM",
		"package", "bootstrap",
		"function", "InitServers",
		"component", "crypto")

	holderMongoDBRepository := holder.NewMongoDBRepository(mongoConnection, dataSecurity)
	aliasMongoDBRepository := alias.NewMongoDBRepository(mongoConnection, dataSecurity)
	holderLinkMongoDBRepository := holderlink.NewMongoDBRepository(mongoConnection)

	useCases := &services.UseCase{
		HolderRepo:     holderMongoDBRepository,
		AliasRepo:      aliasMongoDBRepository,
		HolderLinkRepo: holderLinkMongoDBRepository,
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
	}
}
