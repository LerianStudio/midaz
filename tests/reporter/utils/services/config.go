// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"os"

	"github.com/LerianStudio/midaz/v3/tests/reporter/utils/containers"
)

// ServiceConfig holds configuration derived from test infrastructure.
type ServiceConfig struct {
	// MongoDB
	MongoURI      string
	MongoHost     string
	MongoPort     string
	MongoUser     string
	MongoPassword string
	MongoDatabase string

	// RabbitMQ
	RabbitURL      string
	RabbitHost     string
	RabbitPort     string
	RabbitMgmtPort string
	RabbitUser     string
	RabbitPassword string

	// S3/SeaweedFS
	S3Endpoint  string
	S3Region    string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string

	// Redis/Valkey
	RedisHost     string
	RedisPort     string
	RedisPassword string

	// Manager
	ServerAddress string
	AuthEnabled   bool
}

// NewConfigFromInfrastructure creates a ServiceConfig from running test containers.
func NewConfigFromInfrastructure(infra *containers.TestInfrastructure) *ServiceConfig {
	cfg := &ServiceConfig{
		MongoUser:      containers.MongoUser,
		MongoPassword:  containers.MongoPassword,
		MongoDatabase:  containers.MongoDatabase,
		RabbitUser:     containers.RabbitUser,
		RabbitPassword: containers.RabbitPassword,
		S3Region:       containers.SeaweedRegion,
		S3AccessKey:    containers.SeaweedAccessKey,
		S3SecretKey:    containers.SeaweedSecretKey,
		S3Bucket:       containers.SeaweedBucket,
		RedisPassword:  containers.ValkeyPassword,
		ServerAddress:  "127.0.0.1:0", // Dynamic port
		AuthEnabled:    false,         // Disable auth for tests
	}

	if infra.MongoDB != nil {
		cfg.MongoURI = infra.MongoDB.ConnectionString
		cfg.MongoHost = infra.MongoDB.Host
		cfg.MongoPort = infra.MongoDB.Port
	}

	if infra.RabbitMQ != nil {
		cfg.RabbitURL = infra.RabbitMQ.AmqpURL
		cfg.RabbitHost = infra.RabbitMQ.Host
		cfg.RabbitPort = infra.RabbitMQ.AmqpPort
		cfg.RabbitMgmtPort = infra.RabbitMQ.MgmtPort
	}

	if infra.SeaweedFS != nil {
		cfg.S3Endpoint = infra.SeaweedFS.S3Endpoint
	}

	if infra.Valkey != nil {
		cfg.RedisHost = infra.Valkey.Host
		cfg.RedisPort = infra.Valkey.Port
	}

	return cfg
}

// ApplyManagerEnv sets environment variables for Manager service.
//

func (c *ServiceConfig) ApplyManagerEnv() {
	// Service
	os.Setenv("ENV_NAME", "test")
	os.Setenv("SERVER_ADDRESS", c.ServerAddress)
	os.Setenv("LOG_LEVEL", "error") // Reduce noise in tests

	// MongoDB
	os.Setenv("MONGO_URI", "mongodb")
	os.Setenv("MONGO_HOST", c.MongoHost)
	os.Setenv("MONGO_PORT", c.MongoPort)
	os.Setenv("MONGO_USER", c.MongoUser)
	os.Setenv("MONGO_PASSWORD", c.MongoPassword)
	os.Setenv("MONGO_NAME", c.MongoDatabase)

	// RabbitMQ
	os.Setenv("RABBITMQ_URI", "amqp")
	os.Setenv("RABBITMQ_HOST", c.RabbitHost)
	os.Setenv("RABBITMQ_PORT_AMQP", c.RabbitPort)
	os.Setenv("RABBITMQ_PORT_HOST", c.RabbitMgmtPort)
	os.Setenv("RABBITMQ_DEFAULT_USER", c.RabbitUser)
	os.Setenv("RABBITMQ_DEFAULT_PASS", c.RabbitPassword)
	os.Setenv("RABBITMQ_GENERATE_REPORT_QUEUE", containers.QueueGenerateReport)
	os.Setenv("RABBITMQ_EXCHANGE", containers.ExchangeGenerateReport)
	os.Setenv("RABBITMQ_GENERATE_REPORT_KEY", containers.RoutingKeyGenerateReport)
	os.Setenv("RABBITMQ_HEALTH_CHECK_URL", "http://"+c.RabbitHost+":"+c.RabbitMgmtPort)

	// S3/SeaweedFS
	os.Setenv("OBJECT_STORAGE_ENDPOINT", c.S3Endpoint)
	os.Setenv("OBJECT_STORAGE_REGION", c.S3Region)
	os.Setenv("OBJECT_STORAGE_ACCESS_KEY_ID", c.S3AccessKey)
	os.Setenv("OBJECT_STORAGE_SECRET_KEY", c.S3SecretKey)
	os.Setenv("OBJECT_STORAGE_BUCKET", c.S3Bucket)
	os.Setenv("OBJECT_STORAGE_USE_PATH_STYLE", "true")
	os.Setenv("OBJECT_STORAGE_DISABLE_SSL", "true")

	// Redis/Valkey
	os.Setenv("REDIS_HOST", c.RedisHost+":"+c.RedisPort)
	os.Setenv("REDIS_PASSWORD", c.RedisPassword)
	os.Setenv("REDIS_DB", "0")

	// Auth (disabled for tests)
	os.Setenv("PLUGIN_AUTH_ENABLED", "false")
	os.Setenv("PLUGIN_AUTH_ADDRESS", "")

	// Telemetry (disabled for tests)
	os.Setenv("ENABLE_TELEMETRY", "false")
	os.Setenv("OTEL_LIBRARY_NAME", "reporter")
}

// ApplyWorkerEnv sets environment variables for Worker service.
//

func (c *ServiceConfig) ApplyWorkerEnv() {
	// Service
	os.Setenv("ENV_NAME", "test")
	os.Setenv("LOG_LEVEL", "error")

	// MongoDB
	os.Setenv("MONGO_URI", "mongodb")
	os.Setenv("MONGO_HOST", c.MongoHost)
	os.Setenv("MONGO_PORT", c.MongoPort)
	os.Setenv("MONGO_USER", c.MongoUser)
	os.Setenv("MONGO_PASSWORD", c.MongoPassword)
	os.Setenv("MONGO_NAME", c.MongoDatabase)

	// RabbitMQ
	os.Setenv("RABBITMQ_URI", "amqp")
	os.Setenv("RABBITMQ_HOST", c.RabbitHost)
	os.Setenv("RABBITMQ_PORT_AMQP", c.RabbitPort)
	os.Setenv("RABBITMQ_PORT_HOST", c.RabbitMgmtPort)
	os.Setenv("RABBITMQ_DEFAULT_USER", c.RabbitUser)
	os.Setenv("RABBITMQ_DEFAULT_PASS", c.RabbitPassword)
	os.Setenv("RABBITMQ_GENERATE_REPORT_QUEUE", containers.QueueGenerateReport)
	os.Setenv("RABBITMQ_HEALTH_CHECK_URL", "http://"+c.RabbitHost+":"+c.RabbitMgmtPort)
	os.Setenv("RABBITMQ_NUMBERS_OF_WORKERS", "2") // Fewer workers for tests

	// S3/SeaweedFS
	os.Setenv("OBJECT_STORAGE_ENDPOINT", c.S3Endpoint)
	os.Setenv("OBJECT_STORAGE_REGION", c.S3Region)
	os.Setenv("OBJECT_STORAGE_ACCESS_KEY_ID", c.S3AccessKey)
	os.Setenv("OBJECT_STORAGE_SECRET_KEY", c.S3SecretKey)
	os.Setenv("OBJECT_STORAGE_BUCKET", c.S3Bucket)
	os.Setenv("OBJECT_STORAGE_USE_PATH_STYLE", "true")
	os.Setenv("OBJECT_STORAGE_DISABLE_SSL", "true")

	// PDF Pool (minimal for tests)
	os.Setenv("PDF_POOL_WORKERS", "1")
	os.Setenv("PDF_TIMEOUT_SECONDS", "30")

	// Telemetry (disabled for tests)
	os.Setenv("ENABLE_TELEMETRY", "false")
	os.Setenv("OTEL_LIBRARY_NAME", "reporter")
}

// ClearEnv removes all environment variables set by ApplyManagerEnv/ApplyWorkerEnv.
func ClearEnv() {
	envVars := []string{
		"ENV_NAME", "SERVER_ADDRESS", "LOG_LEVEL",
		"MONGO_URI", "MONGO_HOST", "MONGO_PORT", "MONGO_USER", "MONGO_PASSWORD", "MONGO_NAME",
		"RABBITMQ_URI", "RABBITMQ_HOST", "RABBITMQ_PORT_AMQP", "RABBITMQ_PORT_HOST",
		"RABBITMQ_DEFAULT_USER", "RABBITMQ_DEFAULT_PASS", "RABBITMQ_GENERATE_REPORT_QUEUE",
		"RABBITMQ_EXCHANGE", "RABBITMQ_GENERATE_REPORT_KEY",
		"RABBITMQ_HEALTH_CHECK_URL", "RABBITMQ_NUMBERS_OF_WORKERS",
		"OBJECT_STORAGE_ENDPOINT", "OBJECT_STORAGE_REGION", "OBJECT_STORAGE_ACCESS_KEY_ID",
		"OBJECT_STORAGE_SECRET_KEY", "OBJECT_STORAGE_BUCKET", "OBJECT_STORAGE_USE_PATH_STYLE",
		"OBJECT_STORAGE_DISABLE_SSL",
		"REDIS_HOST", "REDIS_PASSWORD", "REDIS_DB",
		"PLUGIN_AUTH_ENABLED", "PLUGIN_AUTH_ADDRESS",
		"PDF_POOL_WORKERS", "PDF_TIMEOUT_SECONDS",
		"ENABLE_TELEMETRY", "OTEL_LIBRARY_NAME",
	}

	for _, v := range envVars {
		_ = os.Unsetenv(v)
	}
}
