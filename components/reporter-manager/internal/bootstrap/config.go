// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	httpIn "github.com/LerianStudio/midaz/v4/components/reporter-manager/internal/adapters/http/in"
	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/datasource"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/multitenant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/readyz"
	reportSeaweedFS "github.com/LerianStudio/midaz/v4/pkg/reporter/seaweedfs/report"
	templateSeaweedFS "github.com/LerianStudio/midaz/v4/pkg/reporter/seaweedfs/template"

	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/postgres"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/lib-observability/metrics"
	"go.opentelemetry.io/otel/metric"
)

// Config is the top-level configuration struct for the entire application.
type Config struct {
	// Service envs
	EnvName       string `env:"ENV_NAME"`
	ServerAddress string `env:"SERVER_ADDRESS"`
	LogLevel      string `env:"LOG_LEVEL"`
	// DeploymentMode is one of: saas | byoc | local. Echoed in /readyz responses.
	// Gate 4 will couple this to a SaaS-mode TLS enforcement at bootstrap.
	DeploymentMode string `env:"DEPLOYMENT_MODE" default:"local"`
	// Otel and telemetry configuration envs
	OtelServiceName         string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName         string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion      string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv       string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry         bool   `env:"ENABLE_TELEMETRY"`
	OtelInsecureExporter    bool   `env:"OTEL_INSECURE_EXPORTER"`
	// Mongo configuration envs
	MongoURI             string `env:"MONGO_URI"`
	MongoDBHost          string `env:"MONGO_HOST"`
	MongoDBName          string `env:"MONGO_NAME"`
	MongoDBUser          string `env:"MONGO_USER"`
	MongoDBPassword      string `env:"MONGO_PASSWORD"`
	MongoDBPort          string `env:"MONGO_PORT"`
	MongoDBParameters    string `env:"MONGO_PARAMETERS"`
	MongoMaxPoolSize     string `env:"MONGO_MAX_POOL_SIZE" default:"100"`
	MongoMinPoolSize     string `env:"MONGO_MIN_POOL_SIZE" default:"10"`
	MongoMaxConnIdleTime string `env:"MONGO_MAX_CONN_IDLE_TIME" default:"60s"`
	MongoTLSCACert       string `env:"MONGO_TLS_CA_CERT"`
	// Storage configuration envs (S3-compatible only)
	ObjectStorageEndpoint     string `env:"OBJECT_STORAGE_ENDPOINT"`
	ObjectStorageRegion       string `env:"OBJECT_STORAGE_REGION" default:"us-east-1"`
	ObjectStorageAccessKeyID  string `env:"OBJECT_STORAGE_ACCESS_KEY_ID"`
	ObjectStorageSecretKey    string `env:"OBJECT_STORAGE_SECRET_KEY"`
	ObjectStorageUsePathStyle bool   `env:"OBJECT_STORAGE_USE_PATH_STYLE" default:"false"`
	ObjectStorageDisableSSL   bool   `env:"OBJECT_STORAGE_DISABLE_SSL" default:"false"`
	ObjectStorageBucket       string `env:"OBJECT_STORAGE_BUCKET" default:"reporter-storage"` // Single bucket for templates/ and reports/ prefixes
	// RabbitMQ configuration envs
	RabbitURI                   string `env:"RABBITMQ_URI"`
	RabbitMQHost                string `env:"RABBITMQ_HOST"`
	RabbitMQHealthCheckURL      string `env:"RABBITMQ_HEALTH_CHECK_URL"`
	RabbitMQPortHost            string `env:"RABBITMQ_PORT_HOST"`
	RabbitMQPortAMQP            string `env:"RABBITMQ_PORT_AMQP"`
	RabbitMQUser                string `env:"RABBITMQ_DEFAULT_USER"`
	RabbitMQPass                string `env:"RABBITMQ_DEFAULT_PASS"`
	RabbitMQGenerateReportQueue string `env:"RABBITMQ_GENERATE_REPORT_QUEUE"`
	RabbitMQExchange            string `env:"RABBITMQ_EXCHANGE"`
	RabbitMQGenerateReportKey   string `env:"RABBITMQ_GENERATE_REPORT_KEY"`
	RabbitMQTLS                 bool   `env:"RABBITMQ_TLS" default:"false"`
	// Redis/Valkey configuration envs
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
	// Auth envs
	AuthAddress string `env:"PLUGIN_AUTH_ADDRESS"`
	AuthEnabled bool   `env:"PLUGIN_AUTH_ENABLED"`
	// CORS configuration envs
	CORSAllowedOrigins string `env:"CORS_ALLOWED_ORIGINS"`
	CORSAllowedMethods string `env:"CORS_ALLOWED_METHODS"`
	CORSAllowedHeaders string `env:"CORS_ALLOWED_HEADERS"`
	// Trusted proxies configuration
	TrustedProxies string `env:"TRUSTED_PROXIES"`
	// AllowInsecureTLS, when true, bypasses the production TLS enforcement
	// checks (REDIS_TLS, MULTI_TENANT_REDIS_TLS, OBJECT_STORAGE_DISABLE_SSL,
	// RABBITMQ AMQPS) and the SaaS-mode ValidateSaaSTLS gate. Mirrors the
	// lib-commons ALLOW_INSECURE_TLS opt-out semantics: truthy = bypass,
	// default false = enforce. Use only for non-production or transitional
	// environments that intentionally run plaintext dependencies.
	AllowInsecureTLS bool `env:"ALLOW_INSECURE_TLS" default:"false"`
	// Multi-tenant configuration envs
	MultiTenantEnabled                  bool   `env:"MULTI_TENANT_ENABLED" default:"false"`
	MultiTenantURL                      string `env:"MULTI_TENANT_URL"`
	MultiTenantEnvironment              string `env:"MULTI_TENANT_ENVIRONMENT" default:"staging"`
	MultiTenantRedisHost                string `env:"MULTI_TENANT_REDIS_HOST"`
	MultiTenantRedisPort                string `env:"MULTI_TENANT_REDIS_PORT" default:"6379"`
	MultiTenantRedisPassword            string `env:"MULTI_TENANT_REDIS_PASSWORD"`
	MultiTenantRedisTLS                 bool   `env:"MULTI_TENANT_REDIS_TLS" default:"false"`
	MultiTenantRedisCACert              string `env:"MULTI_TENANT_REDIS_CA_CERT"`
	MultiTenantMaxTenantPools           int    `env:"MULTI_TENANT_MAX_TENANT_POOLS" default:"100"`
	MultiTenantIdleTimeoutSec           int    `env:"MULTI_TENANT_IDLE_TIMEOUT_SEC" default:"300"`
	MultiTenantTimeout                  int    `env:"MULTI_TENANT_TIMEOUT" default:"30"`
	MultiTenantCircuitBreakerThreshold  int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD" default:"5"`
	MultiTenantCircuitBreakerTimeoutSec int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC" default:"30"`
	MultiTenantServiceAPIKey            string `env:"MULTI_TENANT_SERVICE_API_KEY"`
	MultiTenantCacheTTLSec              int    `env:"MULTI_TENANT_CACHE_TTL_SEC" default:"120"`
	MultiTenantAllowInsecureHTTP        bool   `env:"MULTI_TENANT_ALLOW_INSECURE_HTTP" default:"false"`
}

// Validate checks that all required configuration fields are present
// and that optional numeric bounds are consistent.
// Returns a descriptive multi-error message listing all violations.
func (c *Config) Validate() error {
	var errs []string

	errs = c.validateRequiredFields(errs)
	errs = c.validateMongoPoolBounds(errs)
	errs = c.validateProductionConfig(errs)

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n- %s", strings.Join(errs, "\n- "))
	}

	return nil
}

// validateRequiredFields appends an error for each required configuration
// field that is empty and returns the accumulated slice.
func (c *Config) validateRequiredFields(errs []string) []string {
	required := []struct {
		value string
		name  string
	}{
		{c.ServerAddress, "SERVER_ADDRESS"},
		{c.RabbitMQHost, "RABBITMQ_HOST"},
		{c.RabbitMQPortAMQP, "RABBITMQ_PORT_AMQP"},
		{c.RabbitMQUser, "RABBITMQ_DEFAULT_USER"},
		{c.RabbitMQPass, "RABBITMQ_DEFAULT_PASS"},
		{c.RabbitMQGenerateReportQueue, "RABBITMQ_GENERATE_REPORT_QUEUE"},
		{c.RabbitMQExchange, "RABBITMQ_EXCHANGE"},
		{c.RabbitMQGenerateReportKey, "RABBITMQ_GENERATE_REPORT_KEY"},
		{c.RedisHost, "REDIS_HOST"},
	}
	if !c.MultiTenantEnabled {
		required = append(required,
			struct {
				value string
				name  string
			}{c.MongoDBHost, "MONGO_HOST"},
			struct {
				value string
				name  string
			}{c.MongoDBName, "MONGO_NAME"},
		)
	}

	errSlice := appendMissingManagerFields(required)
	errs = append(errs, errSlice...)
	errs = c.validateManagerMultiTenantFields(errs)
	errs = append(errs, validateManagerAbsoluteURL("RABBITMQ_HEALTH_CHECK_URL", c.RabbitMQHealthCheckURL, c.EnvName)...)

	return errs
}

// appendMissingManagerFields iterates over the required field list and returns
// an error string for each field whose value is empty.
func appendMissingManagerFields(required []struct {
	value string
	name  string
},
) []string {
	errs := make([]string, 0)

	for _, field := range required {
		if field.value == "" {
			errs = append(errs, field.name+" is required")
		}
	}

	return errs
}

// validateManagerMultiTenantFields enforces that multi-tenant-specific fields
// (URL, circuit breaker, API key) are properly configured when multi-tenancy is enabled.
func (c *Config) validateManagerMultiTenantFields(errs []string) []string {
	if !c.MultiTenantEnabled {
		return errs
	}

	if c.MultiTenantURL == "" {
		errs = append(errs, "MULTI_TENANT_URL is required when MULTI_TENANT_ENABLED=true")
	} else {
		errs = append(errs, validateManagerAbsoluteURL("MULTI_TENANT_URL", c.MultiTenantURL, c.EnvName)...)
	}

	if c.MultiTenantCircuitBreakerThreshold == 0 {
		errs = append(errs, "MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD must be > 0 when MULTI_TENANT_ENABLED=true (default: 5)")
	}

	if c.MultiTenantCircuitBreakerThreshold > 0 && c.MultiTenantCircuitBreakerTimeoutSec == 0 {
		errs = append(errs, "MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC must be > 0 when MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD > 0 (default: 30)")
	}

	if c.MultiTenantServiceAPIKey == "" {
		errs = append(errs, "MULTI_TENANT_SERVICE_API_KEY is required when MULTI_TENANT_ENABLED=true")
	}

	return errs
}

// validateManagerAbsoluteURL checks that rawURL is a valid absolute URL
// (scheme + host) and enforces HTTPS when envName is "production".
func validateManagerAbsoluteURL(name, rawURL, envName string) []string {
	if rawURL == "" {
		return nil
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return []string{name + " must be a valid absolute URL"}
	}

	if strings.EqualFold(envName, "production") && strings.EqualFold(parsedURL.Scheme, "http") {
		return []string{name + " must use HTTPS in production"}
	}

	return nil
}

// validateMongoPoolBounds checks that MongoDB connection pool size
// parameters are within allowed ranges and consistent with each other.
func (c *Config) validateMongoPoolBounds(errs []string) []string {
	maxPool, err := strconv.ParseUint(c.MongoMaxPoolSize, 10, 64)
	if err != nil && c.MongoMaxPoolSize != "" {
		errs = append(errs, "MONGO_MAX_POOL_SIZE must be a valid integer")
		return errs
	}

	minPool, err := strconv.ParseUint(c.MongoMinPoolSize, 10, 64)
	if err != nil && c.MongoMinPoolSize != "" {
		errs = append(errs, "MONGO_MIN_POOL_SIZE must be a valid integer")
		return errs
	}

	if maxPool > constant.MongoMaxPoolSizeUpperBound {
		errs = append(errs, fmt.Sprintf("MONGO_MAX_POOL_SIZE must not exceed %d", constant.MongoMaxPoolSizeUpperBound))
	}

	if maxPool > 0 && minPool > maxPool {
		errs = append(errs, "MONGO_MIN_POOL_SIZE must not exceed MONGO_MAX_POOL_SIZE")
	}

	return errs
}

// validateProductionConfig enforces stricter rules when EnvName is "production".
// Telemetry, authentication, and real credentials are required in production.
func (c *Config) validateProductionConfig(errs []string) []string {
	if c.EnvName != "production" {
		return errs
	}

	if !c.EnableTelemetry {
		errs = append(errs, "ENABLE_TELEMETRY must be true in production")
	}

	if !c.AuthEnabled {
		errs = append(errs, "PLUGIN_AUTH_ENABLED must be true in production")
	}

	secrets := []struct {
		value string
		name  string
	}{
		{c.MongoDBPassword, "MONGO_PASSWORD"},
		{c.RabbitMQPass, "RABBITMQ_DEFAULT_PASS"},
		{c.RedisPassword, "REDIS_PASSWORD"},
		{c.ObjectStorageSecretKey, "OBJECT_STORAGE_SECRET_KEY"},
	}

	for _, s := range secrets {
		if s.value == constant.DefaultPasswordPlaceholder {
			errs = append(errs, s.name+" must not use the default placeholder in production")
		}
	}

	// TLS enforcement is bypassable via ALLOW_INSECURE_TLS (mirrors
	// lib-commons semantics: truthy = bypass, default false = enforce).
	// Non-TLS production checks (telemetry, auth, secrets, CORS) are always
	// enforced regardless of this flag.
	if !c.AllowInsecureTLS {
		if !c.RedisTLS {
			errs = append(errs, "REDIS_TLS must be true in production")
		}

		if c.MultiTenantRedisHost != "" && !c.MultiTenantRedisTLS {
			errs = append(errs, "MULTI_TENANT_REDIS_TLS must be true in production when MULTI_TENANT_REDIS_HOST is configured")
		}

		if c.ObjectStorageDisableSSL {
			errs = append(errs, "OBJECT_STORAGE_DISABLE_SSL must be false in production")
		}

		if !usesSecureRabbitMQScheme(c.RabbitURI) {
			errs = append(errs, "RABBITMQ_URI must use AMQPS in production")
		}
	}

	errs = append(errs, validateManagerAbsoluteURL("OBJECT_STORAGE_ENDPOINT", c.ObjectStorageEndpoint, c.EnvName)...)

	errs = c.validateProductionCORS(errs)

	return errs
}

// usesSecureRabbitMQScheme returns true when the RabbitMQ URI uses the
// "amqps" scheme, indicating a TLS-encrypted connection.
func usesSecureRabbitMQScheme(rawValue string) bool {
	rawValue = strings.TrimSpace(strings.ToLower(rawValue))
	if rawValue == "" {
		return false
	}

	if strings.Contains(rawValue, "://") {
		parsedURL, err := url.Parse(rawValue)
		if err != nil {
			return false
		}

		rawValue = strings.ToLower(parsedURL.Scheme)
	}

	return rawValue == "amqps"
}

// validateProductionCORS enforces that CORS origins are explicitly configured
// in production. Wildcard (*) origins and empty origins are forbidden.
func (c *Config) validateProductionCORS(errs []string) []string {
	if c.CORSAllowedOrigins == "" {
		errs = append(errs, "CORS_ALLOWED_ORIGINS must not be empty in production")
		return errs
	}

	if strings.Contains(c.CORSAllowedOrigins, "*") {
		errs = append(errs, "CORS_ALLOWED_ORIGINS must not contain wildcard (*) in production")
	}

	origins := strings.Split(c.CORSAllowedOrigins, ",")
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin == "" || origin == "*" {
			continue
		}

		if strings.HasPrefix(origin, "http://") {
			errs = append(errs, "CORS_ALLOWED_ORIGINS must use HTTPS in production (found: "+origin+")")
		}
	}

	return errs
}

// InitServers initiate http and grpc servers.
// Uses a cleanup stack pattern: if any initialization step fails, all previously
// opened connections are closed in reverse order to prevent resource leaks.
//
//nolint:gocognit,gocyclo // pre-existing complexity; refactoring planned as tech debt
func InitServers() (_ *Service, err error) {
	cfg, logger, err := initConfigAndLogger()
	if err != nil {
		return nil, err
	}

	// Gate 4: SaaS TLS enforcement. When DEPLOYMENT_MODE=saas, refuse to
	// start if any configured DSN lacks TLS. Runs BEFORE any connection
	// opens, so a misconfigured SaaS deployment cannot silently start
	// insecure (Monetarie incident).
	//
	// Bypassable via ALLOW_INSECURE_TLS (same flag/semantics as lib-commons:
	// truthy = bypass, default false = enforce) for transitional or
	// non-production deployments that intentionally run plaintext deps.
	if tlsErr := enforceManagerSaaSTLS(cfg); tlsErr != nil {
		return nil, fmt.Errorf("SaaS TLS enforcement failed: %w", tlsErr)
	}

	// Cleanup stack: on failure, close resources in reverse order
	var cleanups []func()

	defer func() {
		if err != nil {
			logger.Log(context.Background(), log.LevelInfo, "Initialization failed, cleaning up resources", log.Int("count", len(cleanups)))

			for i := len(cleanups) - 1; i >= 0; i-- {
				cleanups[i]()
			}
		}
	}()

	// Init OpenTelemetry to control logs and flows
	telemetry, telemetryCleanup, err := initTelemetry(cfg, logger)
	if err != nil {
		return nil, err
	}

	cleanups = append(cleanups, telemetryCleanup)

	tracer, err := telemetry.Tracer(cfg.OtelLibraryName)
	if err != nil {
		return nil, fmt.Errorf("failed to create tracer: %w", err)
	}

	if cfg.MultiTenantEnabled {
		logger.Log(context.Background(), log.LevelInfo, "Multi-tenant mode enabled — TenantMiddleware will be activated")
	} else {
		logger.Log(context.Background(), log.LevelInfo, "Running in SINGLE-TENANT MODE — TenantMiddleware disabled")
	}

	// Register multi-tenant OTel metrics (real instruments when MT is enabled, noop otherwise).
	// The metrics are registered with the OTel provider so they appear in dashboards.
	// They are not yet passed to services — that will come as the service layer evolves.
	var mtMetrics *multitenant.Metrics

	if cfg.MultiTenantEnabled {
		if telemetry != nil {
			meter, mErr := telemetry.Meter(cfg.OtelLibraryName)
			if mErr == nil {
				mtMetrics, _ = multitenant.NewMetrics(meter)
			}
		}

		if mtMetrics == nil {
			mtMetrics = multitenant.NoopMetrics()
		}
	} else {
		mtMetrics = multitenant.NoopMetrics()
	}

	_ = mtMetrics // metrics registered with OTel provider; not yet passed to services

	if cfg.MultiTenantEnabled {
		logger.Log(context.Background(), log.LevelInfo, "Multi-tenant metrics registered with OTel provider")
	} else {
		logger.Log(context.Background(), log.LevelDebug, "Multi-tenant metrics using no-op (single-tenant mode)")
	}

	// Register the canonical /readyz metric set on the same meter used by the
	// rest of the service. Independent of multi-tenant mode — readyz metrics
	// always emit (per Gate 5 contract). NewMetrics tolerates a nil meter
	// (returns a noop-backed Metrics) so a non-nil pointer is always returned.
	var readyzMetricsMeter metric.Meter

	if telemetry != nil {
		if meter, mErr := telemetry.Meter(cfg.OtelLibraryName); mErr == nil {
			readyzMetricsMeter = meter
		}
	}

	readyzMetrics, err := readyz.NewMetrics(readyzMetricsMeter)
	if err != nil {
		return nil, fmt.Errorf("failed to register readyz metrics: %w", err)
	}

	logger.Log(context.Background(), log.LevelInfo, "Readyz metrics registered with OTel provider", log.Bool("real_meter", readyzMetricsMeter != nil))

	// Build the datasource-health metric set. Same noop-fallback semantics
	// as readyz metrics.
	dsMetrics, err := pkg.NewDatasourceMetrics(readyzMetricsMeter)
	if err != nil {
		return nil, fmt.Errorf("failed to register datasource metrics: %w", err)
	}

	// Create single storage client for both templates and reports (using prefixes)
	storageClient, err := initStorage(cfg, logger)
	if err != nil {
		return nil, err
	}

	// Init MongoDB connection and repositories
	mongo, mongoCleanup, err := initMongoDB(cfg, logger)
	if err != nil {
		return nil, err
	}

	cleanups = append(cleanups, mongoCleanup)

	// Init RabbitMQ producer and connection monitor
	rabbit, rabbitCleanups, err := initRabbitMQ(cfg, logger)
	if err != nil {
		return nil, err
	}

	cleanups = append(cleanups, rabbitCleanups...)

	// Init Redis/Valkey connection
	redisConsumerRepository, redisConnection, redisCleanup, err := initRedis(cfg, logger)
	if err != nil {
		return nil, err
	}

	cleanups = append(cleanups, redisCleanup)

	// Schema discovery and validation run in-process (the remote Fetcher has
	// been retired). The immutable datasource registry (IDs, types, schema
	// lists, CRM/org-scope configuration) is always built from env config.
	//   - Single-tenant: the registry's lazily-connected pools are the schema
	//     source.
	//   - Multi-tenant: the registry supplies datasource metadata only; the
	//     schema source resolves the live per-tenant connection through the
	//     lib-commons tenant managers (built below).
	if cfg.MultiTenantEnabled {
		logger.Log(context.Background(), log.LevelInfo,
			"Datasource mode: DIRECT (in-process, multi-tenant per-tenant pools)")
	} else {
		logger.Log(context.Background(), log.LevelInfo,
			"Datasource mode: DIRECT (in-process, single-tenant env pools)")
	}

	externalDataSources := pkg.NewSafeDataSources(pkg.ExternalDatasourceConnectionsLazy(logger))

	// Register datasource pool cleanup for graceful shutdown. Under
	// multi-tenancy the registry entries are never connected by the manager
	// (per-tenant pools are owned by the tenant managers), so the per-entry
	// repository handles are nil and these closes are no-ops.
	cleanups = append(cleanups, func() {
		ctx := context.Background()
		logger.Log(ctx, log.LevelInfo, "Closing external datasource connection pools...")

		for name, ds := range externalDataSources.GetAll() {
			switch ds.DatabaseType {
			case pkg.PostgreSQLType:
				if ds.PostgresRepository != nil {
					if err := ds.PostgresRepository.CloseConnection(); err != nil {
						logger.Log(ctx, log.LevelError, "Failed to close PostgreSQL pool", log.String("datasource", name), log.String("error", err.Error()))
					} else {
						logger.Log(ctx, log.LevelInfo, "Closed PostgreSQL pool", log.String("datasource", name))
					}
				}
			case pkg.MongoDBType:
				if ds.MongoDBRepository != nil {
					if err := ds.MongoDBRepository.CloseConnection(ctx); err != nil {
						logger.Log(ctx, log.LevelError, "Failed to close MongoDB pool", log.String("datasource", name), log.String("error", err.Error()))
					} else {
						logger.Log(ctx, log.LevelInfo, "Closed MongoDB pool", log.String("datasource", name))
					}
				}
			}
		}

		logger.Log(ctx, log.LevelInfo, "External datasource pools closed")
	})

	// AuthClient is constructed here (instead of later in the HTTP-server
	// setup block) because it is reused for inbound auth middleware. In
	// disabled mode (PLUGIN_AUTH_ENABLED=false or empty address) the
	// constructor returns a stub that does not touch the network — safe to
	// build unconditionally.
	authClient := middleware.NewAuthClient(cfg.AuthAddress, cfg.AuthEnabled, &logger)

	// Create the in-process DataSourceProvider. Schema discovery and validation
	// always run locally (the remote Fetcher has been retired). Multi-tenant
	// mode resolves per-tenant connections through the lib-commons tenant
	// managers (fail-closed); single-tenant mode uses the env-configured pools.
	var dataSourceProvider datasource.DataSourceProvider

	if cfg.MultiTenantEnabled {
		tenantMongoManager, tenantPostgresManager, mtErr := initManagerSchemaTenantManagers(cfg, logger)
		if mtErr != nil {
			return nil, fmt.Errorf("failed to initialize multi-tenant schema discovery managers: %w", mtErr)
		}

		dataSourceProvider = datasource.NewMultiTenantDirectProvider(
			externalDataSources,
			tenantPostgresManager,
			tenantMongoManager,
			nil, // health checker: the manager runs no datasource health loop
			logger,
		)
	} else {
		provider, providerErr := datasource.NewProvider(datasource.ProviderConfig{
			SafeDataSources: externalDataSources,
		})
		if providerErr != nil {
			return nil, fmt.Errorf("failed to create datasource provider: %w", providerErr)
		}

		dataSourceProvider = provider
	}

	// Use same storage client for both templates and reports (repositories handle prefixes)
	templateStorageRepo := templateSeaweedFS.NewStorageRepository(storageClient)
	reportStorageRepo := reportSeaweedFS.NewStorageRepository(storageClient)

	// MetricsFactory carries the D6 domain operation emitter. Nil when
	// telemetry is disabled — RecordDomainOperation treats nil as a no-op.
	var domainMetricsFactory *metrics.MetricsFactory
	if telemetry != nil {
		domainMetricsFactory = telemetry.MetricsFactory
	}

	// Build service and handler instances
	templateHandler, reportHandler, dataSourceHandler, deadlineHandler, templateBuilderHandler, metricsHandler, notificationHandler, err := initHandlers(
		logger, tracer, domainMetricsFactory, cfg, mongo, rabbit.producer, templateStorageRepo, reportStorageRepo, externalDataSources, redisConsumerRepository, dataSourceProvider,
	)
	if err != nil {
		return nil, err
	}

	// HTTP server setup (authClient was already built above for the outbound
	// M2M providers and is reused here for inbound auth middleware).
	corsConfig := httpIn.CORSConfig{
		AllowedOrigins: cfg.CORSAllowedOrigins,
		AllowedMethods: cfg.CORSAllowedMethods,
		AllowedHeaders: cfg.CORSAllowedHeaders,
	}

	trustedProxies := parseTrustedProxies(cfg.TrustedProxies)

	tenantMiddleware, tenantManagerClient, _, tenantCleanup, err := initTenantMiddleware(cfg, logger)
	if err != nil {
		return nil, err
	}

	if tenantCleanup != nil {
		cleanups = append(cleanups, tenantCleanup)
	}

	// DrainState is shared with the SIGTERM listener launched in
	// (*Service).Run(): the listener calls StartDraining() so /readyz
	// short-circuits to 503 BEFORE lib-commons begins shutting the server
	// down. K8s and load balancers see the unready state and stop sending
	// new traffic while in-flight requests complete.
	drainState := &readyz.DrainState{}

	// SelfProbeState gates the /health endpoint. Created false; flipped to
	// true below by readyz.RunSelfProbe iff every dependency reports up.
	// Failure leaves it false, /health returns 503, and K8s livenessProbe
	// restarts the pod cleanly (no os.Exit — log collection stays intact).
	selfProbeState := &readyz.SelfProbeState{}

	readyzDeps := &httpIn.ManagerReadyzDeps{
		MongoConnection:     mongo.connection,
		RabbitMQConnection:  rabbit.connection,
		RedisConnection:     redisConnection,
		StorageClient:       storageClient,
		StorageEndpoint:     cfg.ObjectStorageEndpoint,
		TenantManagerClient: tenantManagerClient,
		MultiTenantEnabled:  cfg.MultiTenantEnabled,
		MongoURI:            cfg.MongoURI,
		RabbitURI:           cfg.RabbitURI,
		DrainState:          drainState,
		Version:             cfg.OtelServiceVersion,
		DeploymentMode:      cfg.DeploymentMode,
		Metrics:             readyzMetrics,
		SelfProbeState:      selfProbeState,
	}

	// dsMetrics is registered for completeness on the Manager side. The
	// Manager does not run the periodic health-checker loop (only the Worker
	// does), so dsMetrics is not consumed here yet — but registering the
	// instruments on the Manager's meter ensures dashboards see the metric
	// family from any process that exports to the collector.
	_ = dsMetrics

	httpApp := httpIn.NewRoutes(logger, telemetry, templateHandler, reportHandler, dataSourceHandler, deadlineHandler, templateBuilderHandler, metricsHandler, notificationHandler, authClient, readyzDeps, corsConfig, trustedProxies, tenantMiddleware)
	serverAPI := NewServer(cfg, httpApp, logger, telemetry)

	// Gate 7: startup self-probe. Runs every dep checker once, before the
	// HTTP server starts accepting traffic. The result gates the /health
	// endpoint via selfProbeState:
	//
	//   - Success → MarkHealthy() flips /health to 200; pod is fully alive.
	//   - Failure → state stays unhealthy; /health returns 503; K8s
	//     livenessProbe restarts the pod cleanly.
	//
	// Deliberately does NOT call os.Exit on failure — that would skip
	// telemetry flush and log shipping. The pod stays running long enough
	// for CloudWatch / Loki to capture the failure logs before K8s replaces
	// it.
	probeCtx := context.Background()
	probeCheckers := httpIn.BuildManagerCheckers(readyzDeps)

	if probeErr := readyz.RunSelfProbe(probeCtx, probeCheckers, readyzMetrics, logger); probeErr != nil {
		logger.Log(probeCtx, log.LevelError,
			"startup_self_probe_failed_letting_pod_stay_unhealthy",
			log.Err(probeErr))
	} else {
		selfProbeState.MarkHealthy()

		logger.Log(probeCtx, log.LevelInfo, "startup_self_probe_marked_healthy")
	}

	// Build consolidated shutdown cleanup from the same cleanup stack used for
	// init-failure recovery. Resources are closed in reverse initialization order
	// (Redis -> RabbitMQ -> MongoDB -> Telemetry). Telemetry is flushed last so
	// it captures any shutdown-related spans.
	shutdown := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			func(idx int) {
				defer func() {
					if r := recover(); r != nil {
						logger.Log(context.Background(), log.LevelError, "Cleanup panic", log.Int("index", idx), log.Any("recovered", r))
					}
				}()

				cleanups[idx]()
			}(i)
		}
	}

	return &Service{
		Server:         serverAPI,
		Logger:         logger,
		cleanup:        shutdown,
		drainState:     drainState,
		SelfProbeState: selfProbeState,
	}, nil
}

// initManagerSchemaTenantManagers builds the per-tenant MongoDB and PostgreSQL
// managers that back the Manager's in-process multi-tenant schema discovery.
// They share one Tenant Manager client and the same MultiTenant* pool/idle
// knobs, so a tenant resolves the SAME credentials and pool ceiling on both
// backends — mirroring the worker's initMultiTenantManagers.
//
// Production templates and reports reference multi-tenant PostgreSQL and
// MongoDB datasources, so real per-tenant pools are required: there is no
// fail-closed stub and no fallback to a shared single-tenant pool. The managers
// resolve database-per-tenant from the tenant ID carried on the request
// context; an unresolvable tenant fails the schema request closed.
//
// Returns (nil, nil, nil) when multi-tenancy is disabled — single-tenant mode
// uses the env-configured datasource pools instead.
func initManagerSchemaTenantManagers(cfg *Config, logger log.Logger) (*tmmongo.Manager, *tmpostgres.Manager, error) {
	if !cfg.MultiTenantEnabled {
		return nil, nil, nil
	}

	if cfg.MultiTenantURL == "" {
		return nil, nil, fmt.Errorf("MULTI_TENANT_URL is required when MULTI_TENANT_ENABLED=true")
	}

	tmClient, err := newTenantManagerClient(cfg, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize tenant manager client for schema discovery: %w", err)
	}

	tenantMongoManager := tmmongo.NewManager(
		tmClient,
		constant.ApplicationName,
		tmmongo.WithModule(constant.ModuleManager),
		tmmongo.WithLogger(logger),
		tmmongo.WithMaxTenantPools(cfg.MultiTenantMaxTenantPools),
		tmmongo.WithIdleTimeout(time.Duration(cfg.MultiTenantIdleTimeoutSec)*time.Second),
	)

	tenantPostgresManager := tmpostgres.NewManager(
		tmClient,
		constant.ApplicationName,
		tmpostgres.WithModule(constant.ModuleManager),
		tmpostgres.WithLogger(logger),
		tmpostgres.WithMaxTenantPools(cfg.MultiTenantMaxTenantPools),
		tmpostgres.WithIdleTimeout(time.Duration(cfg.MultiTenantIdleTimeoutSec)*time.Second),
	)

	logger.Log(context.Background(), log.LevelInfo, "Manager: tenant schema discovery managers initialized (in-process multi-tenant)")

	return tenantMongoManager, tenantPostgresManager, nil
}

// enforceManagerSaaSTLS runs the Gate-4 SaaS TLS enforcement for the Manager,
// unless ALLOW_INSECURE_TLS bypasses it. When cfg.AllowInsecureTLS is true the
// check is skipped entirely (returns nil) — mirroring the lib-commons
// ALLOW_INSECURE_TLS opt-out semantics (truthy = bypass, default false =
// enforce). Otherwise it delegates to readyz.ValidateSaaSTLS, which is itself a
// no-op outside DEPLOYMENT_MODE=saas.
func enforceManagerSaaSTLS(cfg *Config) error {
	if cfg.AllowInsecureTLS {
		return nil
	}

	return readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildManagerSaaSTLSDeps(cfg))
}

// buildManagerSaaSTLSDeps assembles the list of dependencies that
// ValidateSaaSTLS inspects for the Manager bootstrap. The list mirrors every
// outbound connection the Manager opens (Mongo, RabbitMQ, Redis, S3, Tenant
// Manager, Fetcher) plus the optional multi-tenant Redis when MT is enabled.
//
// Empty URIs are intentional and harmless: ValidateSaaSTLS skips deps whose
// URI is empty, so optional dependencies (Fetcher, multi-tenant Redis) auto
// disable themselves when not configured.
//
// Redis special case: this codebase does not have a single Redis URI; it has
// (host, port, REDIS_TLS bool) separately. We synthesize a "redis://" or
// "rediss://" URI here so DetectRedisTLS can read the scheme. Option A from
// the Gate 4 design — chosen to avoid introducing a new helper for a one-off
// shape (also keeps DetectRedisTLS contract uniform).
func buildManagerSaaSTLSDeps(cfg *Config) []readyz.SaaSTLSDep {
	deps := []readyz.SaaSTLSDep{
		{Name: "mongodb", URI: synthesizeMongoURI(cfg), DetectFn: readyz.DetectMongoTLS},
		{Name: "rabbitmq", URI: synthesizeRabbitMQURI(cfg), DetectFn: readyz.DetectAMQPTLS},
		{Name: "redis", URI: synthesizeRedisURI(cfg.RedisHost, cfg.RedisTLS), DetectFn: readyz.DetectRedisTLS},
		{Name: "storage", URI: cfg.ObjectStorageEndpoint, DetectFn: readyz.DetectS3TLS},
	}

	// Multi-tenant deps are only enforced when MT is actually enabled AND
	// the URL is configured. Operators with leftover MULTI_TENANT_URL set
	// but MULTI_TENANT_ENABLED=false should not have their bootstrap
	// blocked by tenant_manager TLS enforcement against an URL the service
	// will never call.
	if cfg.MultiTenantEnabled && cfg.MultiTenantURL != "" {
		deps = append(deps, readyz.SaaSTLSDep{
			Name:     "tenant_manager",
			URI:      cfg.MultiTenantURL,
			DetectFn: readyz.DetectHTTPUpstreamTLS,
		})

		// Multi-tenant Redis is a separate connection from the application
		// Redis and gets its own enforcement entry when the operator has
		// configured it.
		if cfg.MultiTenantRedisHost != "" {
			mtRedisHostPort := joinHostPort(cfg.MultiTenantRedisHost, cfg.MultiTenantRedisPort)
			deps = append(deps, readyz.SaaSTLSDep{
				Name:     "multi_tenant_redis",
				URI:      synthesizeRedisURI(mtRedisHostPort, cfg.MultiTenantRedisTLS),
				DetectFn: readyz.DetectRedisTLS,
			})
		}
	}

	return deps
}

// synthesizeMongoURI returns the Mongo URI used solely for TLS-posture
// detection by ValidateSaaSTLS. When MONGO_URI is provided directly, it is
// returned as-is. When the operator instead configures split fields
// (MONGO_HOST, MONGO_PORT, etc.) we construct a representative
// "mongodb://" URI from them so DetectMongoTLS can inspect the scheme.
//
// MONGO_PARAMETERS handling: when the operator declares connection options
// via MongoDBParameters (e.g. "tls=true&authSource=admin") rather than
// embedding them in MONGO_URI, the parameters are appended as a query
// string. Without this, an operator who runs Mongo over TLS via split
// fields would be incorrectly classified as non-TLS by DetectMongoTLS and
// blocked at SaaS bootstrap.
//
// The synthesized URI is never used to dial Mongo — connection bootstrap
// has its own builder. We intentionally use the non-TLS scheme here:
// SaaS TLS enforcement requires operators to set MONGO_URI explicitly
// with a TLS-implicit scheme (mongodb+srv://) or with tls=true. Empty
// MongoDBHost yields an empty string, which ValidateSaaSTLS skips.
func synthesizeMongoURI(cfg *Config) string {
	if strings.TrimSpace(cfg.MongoURI) != "" {
		return cfg.MongoURI
	}

	host := strings.TrimSpace(cfg.MongoDBHost)
	if host == "" {
		return ""
	}

	port := strings.TrimSpace(cfg.MongoDBPort)
	user := strings.TrimSpace(cfg.MongoDBUser)
	pass := strings.TrimSpace(cfg.MongoDBPassword)
	dbName := strings.TrimSpace(cfg.MongoDBName)
	params := strings.TrimSpace(cfg.MongoDBParameters)

	hostPort := host
	if port != "" && !strings.Contains(host, ":") {
		hostPort = host + ":" + port
	}

	// url.URL.String() percent-encodes userinfo via url.UserPassword/url.User
	// so reserved characters (@, :, /, ?) in MONGO_USER / MONGO_PASSWORD do
	// not corrupt the synthesized URI. Raw concatenation would produce e.g.
	// "mongodb://user:p@ss@host" which url.Parse re-tokenizes incorrectly,
	// causing DetectMongoTLS to fail and SaaS bootstrap to reject a
	// deployment whose actual connection (using split fields) would succeed.
	u := &url.URL{
		Scheme: "mongodb",
		Host:   hostPort,
	}

	switch {
	case user != "" && pass != "":
		u.User = url.UserPassword(user, pass)
	case user != "":
		u.User = url.User(user)
	}

	if dbName != "" {
		u.Path = "/" + dbName
	}

	if params != "" {
		u.RawQuery = params
	}

	return u.String()
}

// synthesizeRabbitMQURI returns the RabbitMQ URI used solely for TLS-posture
// detection by ValidateSaaSTLS. When RABBITMQ_URI is provided directly, it
// is returned as-is. When the operator instead configures split fields
// (RABBITMQ_HOST, RABBITMQ_PORT_AMQP, etc.) we construct a representative
// "amqp://" or "amqps://" URI from them so DetectAMQPTLS can inspect the
// scheme.
//
// Scheme selection mirrors the Redis path: when RABBITMQ_TLS=true the
// synthesized URI uses amqps:// so DetectAMQPTLS classifies the
// deployment as TLS-enforced. Operators using split fields with
// RABBITMQ_TLS=true would otherwise be blocked by SaaS enforcement
// against a synthesized amqp:// URI, even though their runtime
// connection is TLS.
//
// The synthesized URI is never used to dial RabbitMQ. Empty RabbitMQHost
// yields an empty string, which ValidateSaaSTLS skips.
func synthesizeRabbitMQURI(cfg *Config) string {
	if strings.TrimSpace(cfg.RabbitURI) != "" {
		return cfg.RabbitURI
	}

	host := strings.TrimSpace(cfg.RabbitMQHost)
	if host == "" {
		return ""
	}

	port := strings.TrimSpace(cfg.RabbitMQPortAMQP)
	user := strings.TrimSpace(cfg.RabbitMQUser)
	pass := strings.TrimSpace(cfg.RabbitMQPass)

	hostPort := host
	if port != "" && !strings.Contains(host, ":") {
		hostPort = host + ":" + port
	}

	scheme := "amqp"
	if cfg.RabbitMQTLS {
		scheme = "amqps"
	}

	// url.URL.String() percent-encodes userinfo via url.UserPassword/url.User
	// so reserved characters (@, :, /, ?) in RABBITMQ_DEFAULT_USER /
	// RABBITMQ_DEFAULT_PASS do not corrupt the synthesized URI. Without this
	// DetectAMQPTLS would either fail to parse or misclassify the scheme
	// because url.Parse mis-tokenizes raw-concatenated credentials.
	u := &url.URL{
		Scheme: scheme,
		Host:   hostPort,
		Path:   "/",
	}

	switch {
	case user != "" && pass != "":
		u.User = url.UserPassword(user, pass)
	case user != "":
		u.User = url.User(user)
	}

	return u.String()
}

// synthesizeRedisURI builds a Redis URI from (host[:port], tls bool) so that
// DetectRedisTLS — which only consults the URL scheme — can determine TLS
// posture. Empty host returns an empty string (so ValidateSaaSTLS skips the
// dep). The resulting URI is not used to dial Redis; it exists solely for
// scheme inspection.
func synthesizeRedisURI(hostPort string, tls bool) string {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return ""
	}

	scheme := "redis"
	if tls {
		scheme = "rediss"
	}

	return scheme + "://" + hostPort
}

// joinHostPort joins host and port with a colon when both are non-empty.
// When port is empty, host is returned as-is (the host string may already
// contain a port, e.g., "valkey:6380"). When host is empty, returns "".
func joinHostPort(host, port string) string {
	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)

	if host == "" {
		return ""
	}

	if port == "" {
		return host
	}

	if strings.Contains(host, ":") {
		return host
	}

	return host + ":" + port
}

// parseTrustedProxies splits the comma-separated trusted proxies string into
// a cleaned slice, omitting empty entries.
func parseTrustedProxies(raw string) []string {
	if raw == "" {
		return nil
	}

	var proxies []string

	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			proxies = append(proxies, p)
		}
	}

	return proxies
}
