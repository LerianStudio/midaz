package service

import (
	"github.com/LerianStudio/midaz/common"
)

// Config is the top level configuration struct for the entire application.
type Config struct {
	EnvName                 string `env:"ENV_NAME"`
	ServerAddress           string `env:"SERVER_ADDRESS"`
	ProtoAddress            string `env:"PROTO_ADDRESS"`
	PrimaryDBHost           string `env:"DB_HOST"`
	PrimaryDBUser           string `env:"DB_USER"`
	PrimaryDBPassword       string `env:"DB_PASSWORD"`
	PrimaryDBName           string `env:"DB_NAME"`
	PrimaryDBPort           string `env:"DB_PORT"`
	ReplicaDBHost           string `env:"DB_REPLICA_HOST"`
	ReplicaDBUser           string `env:"DB_REPLICA_USER"`
	ReplicaDBPassword       string `env:"DB_REPLICA_PASSWORD"`
	ReplicaDBName           string `env:"DB_REPLICA_NAME"`
	ReplicaDBPort           string `env:"DB_REPLICA_PORT"`
	MongoDBHost             string `env:"MONGO_HOST"`
	MongoDBName             string `env:"MONGO_NAME"`
	MongoDBUser             string `env:"MONGO_USER"`
	MongoDBPassword         string `env:"MONGO_PASSWORD"`
	MongoDBPort             string `env:"MONGO_PORT"`
	CasdoorAddress          string `env:"CASDOOR_ADDRESS"`
	CasdoorClientID         string `env:"CASDOOR_CLIENT_ID"`
	CasdoorClientSecret     string `env:"CASDOOR_CLIENT_SECRET"`
	CasdoorOrganizationName string `env:"CASDOOR_ORGANIZATION_NAME"`
	CasdoorApplicationName  string `env:"CASDOOR_APPLICATION_NAME"`
	CasdoorEnforcerName     string `env:"CASDOOR_ENFORCER_NAME"`
	JWKAddress              string `env:"CASDOOR_JWK_ADDRESS"`
	RabbitMQHost            string `env:"RABBITMQ_HOST"`
	RabbitMQPortHost        string `env:"RABBITMQ_PORT_HOST"`
	RabbitMQPortAMQP        string `env:"RABBITMQ_PORT_AMPQ"`
	RabbitMQUser            string `env:"RABBITMQ_DEFAULT_USER"`
	RabbitMQPass            string `env:"RABBITMQ_DEFAULT_PASS"`
	RabbitMQExchange        string `env:"RABBITMQ_EXCHANGE"`
	RabbitMQKey             string `env:"RABBITMQ_KEY"`
	RabbitMQQueue           string `env:"RABBITMQ_QUEUE"`
	OtelServiceName         string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName         string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion      string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv       string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
}

// NewConfig creates an instance of Config.
func NewConfig() *Config {
	cfg := &Config{}

	if err := common.SetConfigFromEnvVars(cfg); err != nil {
		panic(err)
	}

	return cfg
}
