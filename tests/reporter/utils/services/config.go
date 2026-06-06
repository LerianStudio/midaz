// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/LerianStudio/midaz/v4/tests/reporter/utils/chaos"
	"github.com/LerianStudio/midaz/v4/tests/reporter/utils/containers"
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

	// Onboarding datasource (PostgreSQL). Registered with the worker/manager via
	// DATASOURCE_ONBOARDING_* env so report rendering can fetch template
	// variables. Empty when no Postgres container is present (e.g. chaos suite
	// before this was wired), in which case the env block is skipped and the
	// report path keeps its prior "data source not found" behavior.
	OnboardingConfigName string
	OnboardingHost       string
	OnboardingPort       string
	OnboardingUser       string
	OnboardingPassword   string
	OnboardingDatabase   string

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

	if infra.Postgres != nil {
		cfg.OnboardingConfigName = containers.OnboardingConfigName
		cfg.OnboardingHost = infra.Postgres.Host
		cfg.OnboardingPort = infra.Postgres.Port
		cfg.OnboardingUser = infra.Postgres.User
		cfg.OnboardingPassword = infra.Postgres.Password
		cfg.OnboardingDatabase = infra.Postgres.Database
	}

	// When Toxiproxy is running (chaos suite only), route datastore traffic
	// through the proxy listeners so injected toxics actually reach the
	// in-process Manager/Worker. Without this, services dial the datastores
	// directly and fault injection is a silent no-op. Integration and fuzzy
	// suites never start Toxiproxy, so they keep the direct addresses above.
	applyToxiproxyEndpoints(cfg, infra)

	return cfg
}

// applyToxiproxyEndpoints overrides datastore addresses with their Toxiproxy
// listener endpoints when the proxy is available, so injected toxics actually
// reach the in-process services.
//
// RabbitMQ is deliberately left on its direct address. The RabbitMQ chaos tests
// split into two groups: toxic-injection tests assert only that HTTP endpoints
// stay RabbitMQ-independent and recover (they pass whether or not AMQP routes
// through the proxy), while container-restart tests (ConnectionClosed,
// QueueFull, MessageLoss, ...) need the broker connection to recover cleanly
// after a stop/start — routing AMQP through the proxy breaks that recovery.
// Keeping AMQP direct satisfies both: the rabbit proxy still exists for toxic
// injection, but the service's own connection is never funneled through it.
func applyToxiproxyEndpoints(cfg *ServiceConfig, infra *containers.TestInfrastructure) {
	if infra.Toxiproxy == nil {
		return
	}

	// Background context is fine: this is one-shot setup at suite startup and
	// the mapped ports are read from an already-running container.
	endpoints, err := infra.GetToxiproxyEndpoints(context.Background())
	if err != nil {
		// Non-fatal: fall back to direct addresses. The chaos tests guard on
		// proxy availability and skip if a proxy is missing.
		fmt.Fprintf(os.Stderr, "Warning: failed to resolve Toxiproxy endpoints, using direct addresses: %v\n", err)
		return
	}

	if addr, ok := endpoints[chaos.ProxyNameMongoDB]; ok {
		if host, port, splitErr := net.SplitHostPort(addr); splitErr == nil {
			cfg.MongoHost = host
			cfg.MongoPort = port
		}
	}

	if addr, ok := endpoints[chaos.ProxyNameValkey]; ok {
		if host, port, splitErr := net.SplitHostPort(addr); splitErr == nil {
			cfg.RedisHost = host
			cfg.RedisPort = port
		}
	}

	if addr, ok := endpoints[chaos.ProxyNameSeaweedFS]; ok {
		cfg.S3Endpoint = "http://" + addr
	}
}

// onboardingDatasourceEnv returns the DATASOURCE_ONBOARDING_* entries that
// register the onboarding PostgreSQL datasource with a service (DIRECT mode,
// FETCHER_ENABLED=false). The worker scans os.Environ() for the
// DATASOURCE_{NAME}_CONFIG_NAME marker (see pkg/reporter/datasource-config.go),
// so CONFIG_NAME must be present and equal to "midaz_onboarding" for templates
// and filters keyed on midaz_onboarding to resolve.
//
// Returns nil when no Postgres datasource is configured, so suites without a
// Postgres container are unaffected.
func (c *ServiceConfig) onboardingDatasourceEnv() []string {
	if c.OnboardingConfigName == "" {
		return nil
	}

	return []string{
		"DATASOURCE_ONBOARDING_CONFIG_NAME=" + c.OnboardingConfigName,
		"DATASOURCE_ONBOARDING_HOST=" + c.OnboardingHost,
		"DATASOURCE_ONBOARDING_PORT=" + c.OnboardingPort,
		"DATASOURCE_ONBOARDING_USER=" + c.OnboardingUser,
		"DATASOURCE_ONBOARDING_PASSWORD=" + c.OnboardingPassword,
		"DATASOURCE_ONBOARDING_DATABASE=" + c.OnboardingDatabase,
		"DATASOURCE_ONBOARDING_TYPE=postgresql",
		"DATASOURCE_ONBOARDING_SSLMODE=disable",
	}
}

// applyOnboardingDatasourceEnv sets the DATASOURCE_ONBOARDING_* variables via
// os.Setenv for in-process service startup paths.
func (c *ServiceConfig) applyOnboardingDatasourceEnv() {
	for _, kv := range c.onboardingDatasourceEnv() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			os.Setenv(k, v)
		}
	}
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

	// Onboarding datasource (DIRECT mode) so report rendering can fetch data.
	c.applyOnboardingDatasourceEnv()
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

	// Onboarding datasource (DIRECT mode) so report rendering can fetch data.
	c.applyOnboardingDatasourceEnv()
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
		"DATASOURCE_ONBOARDING_CONFIG_NAME", "DATASOURCE_ONBOARDING_HOST",
		"DATASOURCE_ONBOARDING_PORT", "DATASOURCE_ONBOARDING_USER",
		"DATASOURCE_ONBOARDING_PASSWORD", "DATASOURCE_ONBOARDING_DATABASE",
		"DATASOURCE_ONBOARDING_TYPE", "DATASOURCE_ONBOARDING_SSLMODE",
	}

	for _, v := range envVars {
		_ = os.Unsetenv(v)
	}
}
