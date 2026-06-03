// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package shared

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/itestkit/addons/e2ekit"
)

// AppStartConfig configures how to start a Reporter application container.
type AppStartConfig struct {
	// Image is the Docker image name/tag to use when SkipBuild is true.
	Image string
	// SkipBuild determines whether to use a pre-built image (true) or build from Dockerfile (false).
	SkipBuild bool
}

// AppEnv holds the environment configuration needed to start Manager and Worker containers.
// It contains connection details for all infrastructure services.
type AppEnv struct {
	// Network is the Docker network name for container-to-container communication.
	Network string

	// MongoDB connection configuration (reporter metadata).
	MongoHost     string
	MongoPort     string
	MongoUser     string
	MongoPassword string

	// PostgreSQL connection configuration (midaz_onboarding datasource).
	PGHost     string
	PGPort     string
	PGUser     string
	PGPassword string

	// RabbitMQ connection configuration.
	RabbitHost           string
	RabbitPort           string
	RabbitManagementPort string
	RabbitUser           string
	RabbitPassword       string

	// Redis connection configuration.
	RedisHost     string
	RedisPassword string

	// S3/MinIO storage configuration.
	S3Endpoint    string
	S3Bucket      string
	S3AccessKeyID string
	S3SecretKey   string

	// Plugin CRM MongoDB connection configuration (separate instance).
	PluginCRMMongoHost     string
	PluginCRMMongoPort     string
	PluginCRMMongoUser     string
	PluginCRMMongoPassword string
}

// BuildAppEnv constructs environment variables from infrastructure endpoints.
// The infra HostPort() methods return network aliases when running in a shared Docker
// network, enabling direct container-to-container communication.
func BuildAppEnv(network string, env *E2EEnv) (*AppEnv, error) {
	mongoHost, mongoPort, err := env.Mongo.HostPort()
	if err != nil {
		return nil, fmt.Errorf("mongo host/port: %w", err)
	}

	pgHost, pgPort, err := env.Postgres.HostPort()
	if err != nil {
		return nil, fmt.Errorf("postgres host/port: %w", err)
	}

	rabbitHost, rabbitPort, err := env.RabbitMQ.HostPort()
	if err != nil {
		return nil, fmt.Errorf("rabbit host/port: %w", err)
	}

	redisHost, redisPort, err := env.Redis.HostPort()
	if err != nil {
		return nil, fmt.Errorf("redis host/port: %w", err)
	}

	// Use the MinIO host-accessible URL and convert to host.docker.internal for container access.
	// HostPort() returns the network alias (minio-reporter:9000) but container DNS resolution
	// can be unreliable. Using the mapped port via host.docker.internal is more robust.
	minioEndpoint, err := env.Minio.Endpoint()
	if err != nil {
		return nil, fmt.Errorf("minio endpoint: %w", err)
	}

	s3URL := minioEndpoint.URL // e.g., "http://localhost:59123"
	s3URL = strings.Replace(s3URL, "localhost", "host.docker.internal", 1)
	s3URL = strings.Replace(s3URL, "127.0.0.1", "host.docker.internal", 1)

	pluginCRMHost, pluginCRMPort, err := env.PluginCRMMongo.HostPort()
	if err != nil {
		return nil, fmt.Errorf("plugin_crm mongo host/port: %w", err)
	}

	// Redis expects host:port format for the REDIS_HOST env var.
	redisAddr := fmt.Sprintf("%s:%d", redisHost, redisPort)

	return &AppEnv{
		Network:                network,
		MongoHost:              mongoHost,
		MongoPort:              strconv.Itoa(mongoPort),
		MongoUser:              CoreInfraUsername,
		MongoPassword:          CoreInfraPassword,
		PGHost:                 pgHost,
		PGPort:                 strconv.Itoa(pgPort),
		PGUser:                 CoreInfraUsername,
		PGPassword:             CoreInfraPassword,
		RabbitHost:             rabbitHost,
		RabbitPort:             strconv.Itoa(rabbitPort),
		RabbitManagementPort:   "15672",
		RabbitUser:             CoreInfraUsername,
		RabbitPassword:         CoreInfraPassword,
		RedisHost:              redisAddr,
		RedisPassword:          CoreInfraPassword,
		S3Endpoint:             s3URL,
		S3Bucket:               env.Minio.Bucket(),
		S3AccessKeyID:          env.Minio.AccessKeyID(),
		S3SecretKey:            env.Minio.SecretAccessKey(),
		PluginCRMMongoHost:     pluginCRMHost,
		PluginCRMMongoPort:     strconv.Itoa(pluginCRMPort),
		PluginCRMMongoUser:     CoreInfraUsername,
		PluginCRMMongoPassword: PluginCRMPassword,
	}, nil
}

// ManagerEnv returns the complete set of environment variables required by the Manager container.
func (e *AppEnv) ManagerEnv() map[string]string {
	portStr := strconv.Itoa(ManagerAPIPort)

	return map[string]string{
		// Service configuration
		"ENV_NAME":       "test",
		"LOG_LEVEL":      "debug",
		"SERVER_ADDRESS": ":" + portStr,

		// MongoDB (reporter metadata)
		"MONGO_URI":      "mongodb",
		"MONGO_HOST":     e.MongoHost,
		"MONGO_PORT":     e.MongoPort,
		"MONGO_USER":     e.MongoUser,
		"MONGO_PASSWORD": e.MongoPassword,
		"MONGO_NAME":     "reporter-db",

		// RabbitMQ
		"RABBITMQ_URI":                   "amqp",
		"RABBITMQ_HOST":                  e.RabbitHost,
		"RABBITMQ_PORT_AMQP":             e.RabbitPort,
		"RABBITMQ_PORT_HOST":             e.RabbitManagementPort,
		"RABBITMQ_HEALTH_CHECK_URL":      fmt.Sprintf("http://%s:%s", e.RabbitHost, e.RabbitManagementPort),
		"RABBITMQ_DEFAULT_USER":          e.RabbitUser,
		"RABBITMQ_DEFAULT_PASS":          e.RabbitPassword,
		"RABBITMQ_GENERATE_REPORT_QUEUE": RabbitQueue,
		"RABBITMQ_EXCHANGE":              RabbitExchange,
		"RABBITMQ_GENERATE_REPORT_KEY":   RabbitRoutingKey,

		// Redis
		"REDIS_HOST":     e.RedisHost,
		"REDIS_PASSWORD": e.RedisPassword,
		"REDIS_DB":       "0",

		// S3-compatible object storage (MinIO)
		"OBJECT_STORAGE_ENDPOINT":       e.S3Endpoint,
		"OBJECT_STORAGE_REGION":         "us-east-1",
		"OBJECT_STORAGE_ACCESS_KEY_ID":  e.S3AccessKeyID,
		"OBJECT_STORAGE_SECRET_KEY":     e.S3SecretKey,
		"OBJECT_STORAGE_BUCKET":         e.S3Bucket,
		"OBJECT_STORAGE_USE_PATH_STYLE": "true",
		"OBJECT_STORAGE_DISABLE_SSL":    "true",

		// Telemetry and auth
		"ENABLE_TELEMETRY":     "false",
		"OTEL_LIBRARY_NAME":    "reporter",
		"MULTI_TENANT_ENABLED": "false",

		// External data sources (Manager needs these for template validation and data source listing)
		"DATASOURCE_MIDAZ_ONBOARDING_CONFIG_NAME": DSMidazOnboarding,
		"DATASOURCE_MIDAZ_ONBOARDING_HOST":        e.PGHost,
		"DATASOURCE_MIDAZ_ONBOARDING_PORT":        e.PGPort,
		"DATASOURCE_MIDAZ_ONBOARDING_USER":        e.PGUser,
		"DATASOURCE_MIDAZ_ONBOARDING_PASSWORD":    e.PGPassword,
		"DATASOURCE_MIDAZ_ONBOARDING_DATABASE":    DSMidazOnboarding,
		"DATASOURCE_MIDAZ_ONBOARDING_TYPE":        "postgresql",
		"DATASOURCE_MIDAZ_ONBOARDING_SSLMODE":     "disable",

		"DATASOURCE_MIDAZ_TRANSACTION_CONFIG_NAME": DSMidazTransaction,
		"DATASOURCE_MIDAZ_TRANSACTION_HOST":        e.PGHost,
		"DATASOURCE_MIDAZ_TRANSACTION_PORT":        e.PGPort,
		"DATASOURCE_MIDAZ_TRANSACTION_USER":        e.PGUser,
		"DATASOURCE_MIDAZ_TRANSACTION_PASSWORD":    e.PGPassword,
		"DATASOURCE_MIDAZ_TRANSACTION_DATABASE":    DSMidazOnboarding, // shares PG instance with midaz_onboarding
		"DATASOURCE_MIDAZ_TRANSACTION_TYPE":        "postgresql",
		"DATASOURCE_MIDAZ_TRANSACTION_SSLMODE":     "disable",

		"DATASOURCE_PLUGIN_CRM_CONFIG_NAME":           DSPluginCRM,
		"DATASOURCE_PLUGIN_CRM_HOST":                  e.PluginCRMMongoHost,
		"DATASOURCE_PLUGIN_CRM_PORT":                  e.PluginCRMMongoPort,
		"DATASOURCE_PLUGIN_CRM_USER":                  e.PluginCRMMongoUser,
		"DATASOURCE_PLUGIN_CRM_PASSWORD":              e.PluginCRMMongoPassword,
		"DATASOURCE_PLUGIN_CRM_DATABASE":              DSPluginCRM,
		"DATASOURCE_PLUGIN_CRM_TYPE":                  "mongodb",
		"DATASOURCE_PLUGIN_CRM_OPTIONS":               "authSource=admin",
		"DATASOURCE_PLUGIN_CRM_MIDAZ_ORGANIZATION_ID": PluginCRMMidazOrgID,
	}
}

// WorkerEnv returns the complete set of environment variables required by the Worker container.
// This includes DATASOURCE_* variables for midaz_onboarding (PostgreSQL) and plugin_crm (MongoDB).
func (e *AppEnv) WorkerEnv() map[string]string {
	return map[string]string{
		// Service configuration
		"ENV_NAME":    "test",
		"LOG_LEVEL":   "debug",
		"HEALTH_PORT": strconv.Itoa(WorkerHealthPort),

		// MongoDB (reporter metadata)
		"MONGO_URI":      "mongodb",
		"MONGO_HOST":     e.MongoHost,
		"MONGO_PORT":     e.MongoPort,
		"MONGO_USER":     e.MongoUser,
		"MONGO_PASSWORD": e.MongoPassword,
		"MONGO_NAME":     "reporter-db",

		// RabbitMQ
		"RABBITMQ_URI":                   "amqp",
		"RABBITMQ_HOST":                  e.RabbitHost,
		"RABBITMQ_PORT_AMQP":             e.RabbitPort,
		"RABBITMQ_PORT_HOST":             e.RabbitManagementPort,
		"RABBITMQ_HEALTH_CHECK_URL":      fmt.Sprintf("http://%s:%s", e.RabbitHost, e.RabbitManagementPort),
		"RABBITMQ_DEFAULT_USER":          e.RabbitUser,
		"RABBITMQ_DEFAULT_PASS":          e.RabbitPassword,
		"RABBITMQ_GENERATE_REPORT_QUEUE": RabbitQueue,
		"RABBITMQ_NUMBERS_OF_WORKERS":    "1",

		// Redis
		"REDIS_HOST":     e.RedisHost,
		"REDIS_PASSWORD": e.RedisPassword,
		"REDIS_DB":       "0",

		// S3-compatible object storage (MinIO)
		"OBJECT_STORAGE_ENDPOINT":       e.S3Endpoint,
		"OBJECT_STORAGE_REGION":         "us-east-1",
		"OBJECT_STORAGE_ACCESS_KEY_ID":  e.S3AccessKeyID,
		"OBJECT_STORAGE_SECRET_KEY":     e.S3SecretKey,
		"OBJECT_STORAGE_BUCKET":         e.S3Bucket,
		"OBJECT_STORAGE_USE_PATH_STYLE": "true",
		"OBJECT_STORAGE_DISABLE_SSL":    "true",

		// DATASOURCE: midaz_onboarding (PostgreSQL)
		"DATASOURCE_MIDAZ_ONBOARDING_CONFIG_NAME": DSMidazOnboarding,
		"DATASOURCE_MIDAZ_ONBOARDING_HOST":        e.PGHost,
		"DATASOURCE_MIDAZ_ONBOARDING_PORT":        e.PGPort,
		"DATASOURCE_MIDAZ_ONBOARDING_USER":        e.PGUser,
		"DATASOURCE_MIDAZ_ONBOARDING_PASSWORD":    e.PGPassword,
		"DATASOURCE_MIDAZ_ONBOARDING_DATABASE":    DSMidazOnboarding,
		"DATASOURCE_MIDAZ_ONBOARDING_TYPE":        "postgresql",
		"DATASOURCE_MIDAZ_ONBOARDING_SSLMODE":     "disable",

		// DATASOURCE: midaz_transaction (PostgreSQL — shares PG instance with midaz_onboarding)
		"DATASOURCE_MIDAZ_TRANSACTION_CONFIG_NAME": DSMidazTransaction,
		"DATASOURCE_MIDAZ_TRANSACTION_HOST":        e.PGHost,
		"DATASOURCE_MIDAZ_TRANSACTION_PORT":        e.PGPort,
		"DATASOURCE_MIDAZ_TRANSACTION_USER":        e.PGUser,
		"DATASOURCE_MIDAZ_TRANSACTION_PASSWORD":    e.PGPassword,
		"DATASOURCE_MIDAZ_TRANSACTION_DATABASE":    DSMidazOnboarding, // shares PG instance with midaz_onboarding
		"DATASOURCE_MIDAZ_TRANSACTION_TYPE":        "postgresql",
		"DATASOURCE_MIDAZ_TRANSACTION_SSLMODE":     "disable",

		// DATASOURCE: plugin_crm (MongoDB)
		"DATASOURCE_PLUGIN_CRM_CONFIG_NAME":           DSPluginCRM,
		"DATASOURCE_PLUGIN_CRM_HOST":                  e.PluginCRMMongoHost,
		"DATASOURCE_PLUGIN_CRM_PORT":                  e.PluginCRMMongoPort,
		"DATASOURCE_PLUGIN_CRM_USER":                  e.PluginCRMMongoUser,
		"DATASOURCE_PLUGIN_CRM_PASSWORD":              e.PluginCRMMongoPassword,
		"DATASOURCE_PLUGIN_CRM_DATABASE":              DSPluginCRM,
		"DATASOURCE_PLUGIN_CRM_TYPE":                  "mongodb",
		"DATASOURCE_PLUGIN_CRM_OPTIONS":               "authSource=admin",
		"DATASOURCE_PLUGIN_CRM_MIDAZ_ORGANIZATION_ID": PluginCRMMidazOrgID,

		// PDF generation
		"PDF_POOL_WORKERS":    "1",
		"PDF_TIMEOUT_SECONDS": "90",

		// Telemetry and auth
		"ENABLE_TELEMETRY":     "false",
		"OTEL_LIBRARY_NAME":    "reporter",
		"MULTI_TENANT_ENABLED": "false",

		// Crypto keys for plugin_crm PII decryption (must match keys used to encrypt seed data).
		"CRYPTO_HASH_SECRET_KEY_PLUGIN_CRM":    TestCryptoHashKey,
		"CRYPTO_ENCRYPT_SECRET_KEY_PLUGIN_CRM": TestCryptoEncryptKey,
	}
}

// StartManager starts the Manager HTTP API container using e2ekit.
// The container exposes port 4005 and waits for the /health endpoint.
func StartManager(t *testing.T, ctx context.Context, env *AppEnv, cfg AppStartConfig) (*e2ekit.RunningApp, error) { //nolint:thelper // t can be nil when called from TestMain
	if t != nil {
		t.Helper()
	}

	builder := e2ekit.New(t).
		WithContext(ctx).
		ExposePort(ManagerAPIPort).
		WithEnv(env.ManagerEnv()).
		WithWait(e2ekit.WaitHTTP(ManagerAPIPort, "/health", 120*time.Second))

	if env.Network != "" {
		builder = builder.WithNetworks(env.Network)
	}

	if cfg.SkipBuild {
		builder = builder.WithImage(cfg.Image)
	} else {
		builder = builder.WithDockerfile(e2ekit.BuildConfig{
			ContextDir: e2ekit.ProjectRoot(),
			Dockerfile: "components/manager/Dockerfile",
			Tag:        cfg.Image,
			Secrets: []e2ekit.BuildSecret{
				{ID: "github_token", Env: "GITHUB_TOKEN"},
			},
		})
	}

	return builder.Run()
}

// StartWorker starts the Worker message consumer container using e2ekit.
// The container exposes port 4006 for health checks.
func StartWorker(t *testing.T, ctx context.Context, env *AppEnv, cfg AppStartConfig) (*e2ekit.RunningApp, error) { //nolint:thelper // t can be nil when called from TestMain
	if t != nil {
		t.Helper()
	}

	builder := e2ekit.New(t).
		WithContext(ctx).
		ExposePort(WorkerHealthPort).
		WithEnv(env.WorkerEnv()).
		WithShmSize(2 * 1024 * 1024 * 1024). // 2GB shared memory for Chromium PDF generation
		WithWait(e2ekit.WaitHTTP(WorkerHealthPort, "/health", 120*time.Second))

	if env.Network != "" {
		builder = builder.WithNetworks(env.Network)
	}

	if cfg.SkipBuild {
		builder = builder.WithImage(cfg.Image)
	} else {
		builder = builder.WithDockerfile(e2ekit.BuildConfig{
			ContextDir: e2ekit.ProjectRoot(),
			Dockerfile: "components/worker/Dockerfile",
			Tag:        cfg.Image,
			Secrets: []e2ekit.BuildSecret{
				{ID: "github_token", Env: "GITHUB_TOKEN"},
			},
		})
	}

	return builder.Run()
}

// resolveManagerConfig builds AppStartConfig from environment variables.
func resolveManagerConfig() AppStartConfig {
	cfg := AppStartConfig{}

	if strings.EqualFold(os.Getenv("E2E_SKIP_BUILD"), "true") {
		cfg.SkipBuild = true
	}

	if img := os.Getenv("MANAGER_IMAGE"); img != "" {
		cfg.Image = img
	}

	return cfg
}

// resolveWorkerConfig builds AppStartConfig from environment variables.
func resolveWorkerConfig() AppStartConfig {
	cfg := AppStartConfig{}

	if strings.EqualFold(os.Getenv("E2E_SKIP_BUILD"), "true") {
		cfg.SkipBuild = true
	}

	if img := os.Getenv("WORKER_IMAGE"); img != "" {
		cfg.Image = img
	}

	return cfg
}
