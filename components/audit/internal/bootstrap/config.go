package bootstrap

import (
	"fmt"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/grpc"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/audit/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mmongo"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mrabbitmq"
	"github.com/LerianStudio/midaz/pkg/mtrillian"
	"github.com/LerianStudio/midaz/pkg/mzap"
)

const ApplicationName = "audit"

// Config is the top level configuration struct for the entire application.
type Config struct {
	EnvName                 string `env:"ENV_NAME"`
	LogLevel                string `env:"LOG_LEVEL"`
	JWKAddress              string `env:"CASDOOR_JWK_ADDRESS"`
	CasdoorAddress          string `env:"CASDOOR_ADDRESS"`
	CasdoorClientID         string `env:"CASDOOR_CLIENT_ID"`
	CasdoorClientSecret     string `env:"CASDOOR_CLIENT_SECRET"`
	CasdoorOrganizationName string `env:"CASDOOR_ORGANIZATION_NAME"`
	CasdoorApplicationName  string `env:"CASDOOR_APPLICATION_NAME"`
	CasdoorEnforcerName     string `env:"CASDOOR_ENFORCER_NAME"`
	OtelServiceName         string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName         string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion      string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv       string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	ServerAddress           string `env:"SERVER_ADDRESS"`
	MongoDBHost             string `env:"MONGO_HOST"`
	MongoDBName             string `env:"MONGO_NAME"`
	MongoDBUser             string `env:"MONGO_USER"`
	MongoDBPassword         string `env:"MONGO_PASSWORD"`
	MongoDBPort             string `env:"MONGO_PORT"`
	RabbitMQHost            string `env:"RABBITMQ_HOST"`
	RabbitMQPortHost        string `env:"RABBITMQ_PORT_HOST"`
	RabbitMQPortAMQP        string `env:"RABBITMQ_PORT_AMPQ"`
	RabbitMQUser            string `env:"RABBITMQ_DEFAULT_USER"`
	RabbitMQPass            string `env:"RABBITMQ_DEFAULT_PASS"`
	RabbitMQExchange        string `env:"RABBITMQ_EXCHANGE"`
	RabbitMQKey             string `env:"RABBITMQ_KEY"`
	RabbitMQQueue           string `env:"RABBITMQ_QUEUE"`
	TrillianHost            string `env:"TRILLIAN_HOST"`
	TrillianPort            string `env:"TRILLIAN_GRPC_PORT"`
}

// InitServers initiate http and grpc servers.
func InitServers() (*services.UseCase, mlog.Logger, *mopentelemetry.Telemetry) {
	cfg := &Config{}

	if err := pkg.SetConfigFromEnvVars(cfg); err != nil {
		panic(err)
	}

	logger := mzap.InitializeLogger()

	telemetry := &mopentelemetry.Telemetry{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
	}

	rabbitSource := fmt.Sprintf("amqp://%s:%s@%s:%s",
		cfg.RabbitMQUser, cfg.RabbitMQPass, cfg.RabbitMQHost, cfg.RabbitMQPortHost)

	rabbitMQConnection := &mrabbitmq.RabbitMQConnection{
		ConnectionStringSource: rabbitSource,
		Host:                   cfg.RabbitMQHost,
		Port:                   cfg.RabbitMQPortAMQP,
		User:                   cfg.RabbitMQUser,
		Pass:                   cfg.RabbitMQPass,
		Exchange:               cfg.RabbitMQExchange,
		Key:                    cfg.RabbitMQKey,
		Queue:                  cfg.RabbitMQQueue,
		Logger:                 logger,
	}

	trillianSource := fmt.Sprintf("%s:%s", cfg.TrillianHost, cfg.TrillianPort)

	trillianConnection := &mtrillian.TrillianConnection{
		Addr:   trillianSource,
		Logger: logger,
	}

	mongoSource := fmt.Sprintf("mongodb://%s:%s@%s:%s",
		cfg.MongoDBUser, cfg.MongoDBPassword, cfg.MongoDBHost, cfg.MongoDBPort)

	mongoAuditConnection := &mmongo.MongoConnection{
		ConnectionStringSource: mongoSource,
		Database:               cfg.MongoDBName,
		Logger:                 logger,
	}

	consumerRabbitMQRepository := rabbitmq.NewConsumerRabbitMQ(rabbitMQConnection)

	trillianRepository := grpc.NewTrillianRepository(trillianConnection)

	auditMongoDBRepository := audit.NewAuditMongoDBRepository(mongoAuditConnection)

	useCase := &services.UseCase{
		RabbitMQRepo: consumerRabbitMQRepository,
		TrillianRepo: trillianRepository,
		AuditRepo:    auditMongoDBRepository,
	}

	return useCase, logger, telemetry
}
