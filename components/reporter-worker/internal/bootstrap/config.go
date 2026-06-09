// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	clog "github.com/LerianStudio/lib-observability/log"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/readyz"
)

// Config holds the application's configurable parameters read from environment variables.
type Config struct {
	EnvName        string `env:"ENV_NAME"`
	LogLevel       string `env:"LOG_LEVEL"`
	DeploymentMode string `env:"DEPLOYMENT_MODE" default:"local"`
	HealthPort     string `env:"HEALTH_PORT" default:"4006"`
	// AllowInsecureTLS, when true, bypasses the production TLS enforcement
	// checks (REDIS_TLS, MULTI_TENANT_REDIS_TLS, OBJECT_STORAGE_DISABLE_SSL,
	// RABBITMQ AMQPS) and the SaaS-mode ValidateSaaSTLS gate. Mirrors the
	// lib-commons ALLOW_INSECURE_TLS opt-out semantics: truthy = bypass,
	// default false = enforce.
	AllowInsecureTLS                bool   `env:"ALLOW_INSECURE_TLS" default:"false"`
	RabbitURI                       string `env:"RABBITMQ_URI"`
	RabbitMQHost                    string `env:"RABBITMQ_HOST"`
	RabbitMQPortHost                string `env:"RABBITMQ_PORT_HOST"`
	RabbitMQPortAMQP                string `env:"RABBITMQ_PORT_AMQP"`
	RabbitMQUser                    string `env:"RABBITMQ_DEFAULT_USER"`
	RabbitMQPass                    string `env:"RABBITMQ_DEFAULT_PASS"`
	RabbitMQGenerateReportQueue     string `env:"RABBITMQ_GENERATE_REPORT_QUEUE"`
	RabbitMQNumWorkers              int    `env:"RABBITMQ_NUMBERS_OF_WORKERS"`
	RabbitMQHealthCheckURL          string `env:"RABBITMQ_HEALTH_CHECK_URL"`
	RabbitMQTLS                     bool   `env:"RABBITMQ_TLS" default:"false"`
	OtelServiceName                 string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName                 string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion              string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv               string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint         string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry                 bool   `env:"ENABLE_TELEMETRY"`
	OtelInsecureExporter            bool   `env:"OTEL_INSECURE_EXPORTER"`
	ObjectStorageEndpoint           string `env:"OBJECT_STORAGE_ENDPOINT"`
	ObjectStorageRegion             string `env:"OBJECT_STORAGE_REGION" default:"us-east-1"`
	ObjectStorageAccessKeyID        string `env:"OBJECT_STORAGE_ACCESS_KEY_ID"`
	ObjectStorageSecretKey          string `env:"OBJECT_STORAGE_SECRET_KEY"`
	ObjectStorageUsePathStyle       bool   `env:"OBJECT_STORAGE_USE_PATH_STYLE" default:"false"`
	ObjectStorageDisableSSL         bool   `env:"OBJECT_STORAGE_DISABLE_SSL" default:"false"`
	ObjectStorageBucket             string `env:"OBJECT_STORAGE_BUCKET" default:"reporter-storage"`
	MongoURI                        string `env:"MONGO_URI"`
	MongoDBHost                     string `env:"MONGO_HOST"`
	MongoDBName                     string `env:"MONGO_NAME"`
	MongoDBUser                     string `env:"MONGO_USER"`
	MongoDBPassword                 string `env:"MONGO_PASSWORD"`
	MongoDBPort                     string `env:"MONGO_PORT"`
	MongoDBParameters               string `env:"MONGO_PARAMETERS"`
	MaxPoolSize                     int    `env:"MONGO_MAX_POOL_SIZE"`
	MongoTLSCACert                  string `env:"MONGO_TLS_CA_CERT"`
	CryptoHashSecretKeyPluginCRM    string `env:"CRYPTO_HASH_SECRET_KEY_PLUGIN_CRM"`
	CryptoEncryptSecretKeyPluginCRM string `env:"CRYPTO_ENCRYPT_SECRET_KEY_PLUGIN_CRM"`
	PdfPoolWorkers                  int    `env:"PDF_POOL_WORKERS" default:"2"`
	PdfPoolTimeoutSeconds           int    `env:"PDF_TIMEOUT_SECONDS" default:"90"`
	RedisHost                       string `env:"REDIS_HOST"`
	RedisMasterName                 string `env:"REDIS_MASTER_NAME" default:""`
	RedisPassword                   string `env:"REDIS_PASSWORD"`
	RedisDB                         int    `env:"REDIS_DB" default:"0"`
	RedisProtocol                   int    `env:"REDIS_PROTOCOL" default:"3"`
	RedisTLS                        bool   `env:"REDIS_TLS" default:"false"`
	RedisCACert                     string `env:"REDIS_CA_CERT"`
	RedisUseGCPIAM                  bool   `env:"REDIS_USE_GCP_IAM" default:"false"`
	RedisServiceAccount             string `env:"REDIS_SERVICE_ACCOUNT" default:""`
	GoogleApplicationCredentials    string `env:"GOOGLE_APPLICATION_CREDENTIALS" default:""`
	RedisTokenLifeTime              int    `env:"REDIS_TOKEN_LIFETIME" default:"60"`
	RedisTokenRefreshDuration       int    `env:"REDIS_TOKEN_REFRESH_DURATION" default:"45"`
	// Fetcher dual-mode configuration envs
	FetcherEnabled         bool   `env:"FETCHER_ENABLED" default:"false"`
	FetcherURL             string `env:"FETCHER_URL"`
	AppEncKey              string `env:"APP_ENC_KEY"`
	FetcherStorageBucket   string `env:"FETCHER_STORAGE_BUCKET" default:"fetcher-storage"`
	FetcherStorageEndpoint string `env:"FETCHER_STORAGE_ENDPOINT"`
	// M2M auth (multi-tenant only)
	AWSRegion        string `env:"AWS_REGION"`
	M2MTargetService string `env:"M2M_TARGET_SERVICE" default:"fetcher"`
	AuthAddress      string `env:"PLUGIN_AUTH_ADDRESS"`
	AuthEnabled      bool   `env:"PLUGIN_AUTH_ENABLED"`
	// Outbound M2M application credentials for calling the Fetcher in
	// single-tenant deployments. Exchanged for a bearer token via plugin-auth
	// when PLUGIN_AUTH_ENABLED=true. Multi-tenant deployments resolve
	// credentials per-tenant from AWS Secrets Manager instead.
	ClientID     string `env:"FETCHER_M2M_CLIENT_ID"`
	ClientSecret string `env:"FETCHER_M2M_CLIENT_SECRET"`
	// Reconciler configuration
	ReconciliationIntervalMin int `env:"RECONCILIATION_INTERVAL_MIN" default:"5"`
	M2MCredentialCacheTTLSec  int `env:"M2M_CREDENTIAL_CACHE_TTL_SEC" default:"300"`
	M2MTokenCacheMarginSec    int `env:"M2M_TOKEN_CACHE_MARGIN_SEC" default:"60"`
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
	// Embedded extraction engine limits. A zero value for any field falls back
	// to the engine's DefaultLimits for that field, so the engine never runs
	// unbounded even if these are unset.
	EngineMaxDatasources         int   `env:"ENGINE_MAX_DATASOURCES" default:"10"`
	EngineMaxTablesPerDatasource int   `env:"ENGINE_MAX_TABLES_PER_DATASOURCE" default:"50"`
	EngineMaxFieldsPerTable      int   `env:"ENGINE_MAX_FIELDS_PER_TABLE" default:"200"`
	EngineMaxConcurrency         int   `env:"ENGINE_MAX_CONCURRENCY" default:"4"`
	EngineTimeoutSec             int   `env:"ENGINE_TIMEOUT_SEC" default:"300"`
	EngineMaxResultBytes         int64 `env:"ENGINE_MAX_RESULT_BYTES" default:"104857600"`
}

// InitWorker initializes and configures the application's dependencies and returns the Service instance.
// Uses a CleanupManager: if any initialization step fails, all previously
// opened connections are closed in reverse order (LIFO) to prevent resource leaks.
func InitWorker() (_ *Service, err error) {
	cfg, logger, cfgErr := loadConfigAndLogger()
	if cfgErr != nil {
		return nil, cfgErr
	}

	// Gate 4: SaaS TLS enforcement. When DEPLOYMENT_MODE=saas, refuse to
	// start if any configured DSN lacks TLS. Runs BEFORE any connection
	// opens (Mongo, RabbitMQ, Redis, Storage, Fetcher, Tenant Manager) so a
	// misconfigured SaaS deployment cannot silently start insecure.
	//
	// Bypassable via ALLOW_INSECURE_TLS (same flag/semantics as lib-commons:
	// truthy = bypass, default false = enforce).
	if tlsErr := enforceWorkerSaaSTLS(cfg); tlsErr != nil {
		return nil, fmt.Errorf("SaaS TLS enforcement failed: %w", tlsErr)
	}

	logWorkerMode(cfg, logger)

	tmClient, tenantMongoManager, tenantPostgresManager, err := initMultiTenantManagers(cfg, logger)
	if err != nil {
		return nil, err
	}

	cleanups := NewCleanupManager(logger)

	defer func() {
		if err != nil {
			cleanups.ExecuteAll()
		}
	}()

	appendWorkerTenantPostgresCleanup(logger, tenantPostgresManager, cleanups)

	deps, err := initWorkerDependencies(cfg, logger, tenantMongoManager, tenantPostgresManager, cleanups)
	if err != nil {
		return nil, err
	}

	if cfg.MultiTenantEnabled && cfg.MultiTenantURL != "" {
		return initMultiTenantWorkerService(cfg, logger, tmClient, tenantMongoManager, deps, cleanups)
	}

	return initSingleTenantWorkerService(cfg, logger, deps, cleanups)
}

func logWorkerMode(cfg *Config, logger clog.Logger) {
	if cfg.MultiTenantEnabled {
		logger.Log(context.Background(), clog.LevelInfo, "Worker: multi-tenant mode enabled")
		return
	}

	logger.Log(context.Background(), clog.LevelInfo, "Worker: single-tenant mode enabled")
}

// enforceWorkerSaaSTLS runs the Gate-4 SaaS TLS enforcement for the Worker,
// unless ALLOW_INSECURE_TLS bypasses it. When cfg.AllowInsecureTLS is true the
// check is skipped entirely (returns nil) — mirroring the lib-commons
// ALLOW_INSECURE_TLS opt-out semantics (truthy = bypass, default false =
// enforce). Otherwise it delegates to readyz.ValidateSaaSTLS, which is itself a
// no-op outside DEPLOYMENT_MODE=saas.
func enforceWorkerSaaSTLS(cfg *Config) error {
	if cfg.AllowInsecureTLS {
		return nil
	}

	return readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildWorkerSaaSTLSDeps(cfg))
}

// buildWorkerSaaSTLSDeps assembles the list of dependencies that
// ValidateSaaSTLS inspects for the Worker bootstrap. The list mirrors every
// outbound connection the Worker actually opens, gated on the same flags
// the runtime uses to decide whether to construct each connection.
//
// Empty URIs are intentional and harmless: ValidateSaaSTLS skips deps whose
// URI is empty, so optional dependencies (Fetcher, multi-tenant Redis,
// fetcher_storage) auto-disable themselves when not configured.
//
// Redis special case: this codebase does not have a single Redis URI; it has
// (host, port, REDIS_TLS bool) separately. We synthesize a "redis://" or
// "rediss://" URI here so DetectRedisTLS can read the scheme. Option A from
// the Gate 4 design — chosen to avoid introducing a new helper for a one-off
// shape (also keeps DetectRedisTLS contract uniform).
//
// Redis is conditionally enforced: the Worker only constructs a Redis
// connection when FETCHER_ENABLED=true (reconciler distributed lock) OR
// MULTI_TENANT_ENABLED=true (per-tenant Redis client + tenant
// event-listener). When both are false the Worker never dials Redis, so a
// leftover plaintext REDIS_HOST in the environment must NOT block SaaS
// bootstrap. This mirrors the redisRequired gate in BuildWorkerCheckers
// (health-server.go) so the readyz aggregation and SaaS enforcement see
// the same picture of which dependencies are actually live.
func buildWorkerSaaSTLSDeps(cfg *Config) []readyz.SaaSTLSDep {
	deps := []readyz.SaaSTLSDep{
		{Name: "mongodb", URI: synthesizeWorkerMongoURI(cfg), DetectFn: readyz.DetectMongoTLS},
		{Name: "rabbitmq", URI: synthesizeWorkerRabbitMQURI(cfg), DetectFn: readyz.DetectAMQPTLS},
		{Name: "storage", URI: cfg.ObjectStorageEndpoint, DetectFn: readyz.DetectS3TLS},
	}

	// Redis is only required when the Worker actually opens it. Mirrors
	// redisRequired = FetcherEnabled || MultiTenantEnabled in
	// BuildWorkerCheckers.
	if cfg.FetcherEnabled || cfg.MultiTenantEnabled {
		deps = append(deps, readyz.SaaSTLSDep{
			Name:     "redis",
			URI:      synthesizeWorkerRedisURI(cfg.RedisHost, cfg.RedisTLS),
			DetectFn: readyz.DetectRedisTLS,
		})
	}

	// Fetcher upstream URL + the Fetcher-specific S3 storage are only enforced
	// when Fetcher is enabled. When disabled, both URLs may be unset and must
	// not block SaaS bootstrap.
	if cfg.FetcherEnabled {
		deps = append(deps,
			readyz.SaaSTLSDep{Name: "fetcher", URI: cfg.FetcherURL, DetectFn: readyz.DetectHTTPUpstreamTLS},
			readyz.SaaSTLSDep{Name: "fetcher_storage", URI: cfg.FetcherStorageEndpoint, DetectFn: readyz.DetectS3TLS},
		)
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
			mtRedisHostPort := joinWorkerHostPort(cfg.MultiTenantRedisHost, cfg.MultiTenantRedisPort)
			deps = append(deps, readyz.SaaSTLSDep{
				Name:     "multi_tenant_redis",
				URI:      synthesizeWorkerRedisURI(mtRedisHostPort, cfg.MultiTenantRedisTLS),
				DetectFn: readyz.DetectRedisTLS,
			})
		}
	}

	return deps
}

// synthesizeWorkerMongoURI returns the Mongo URI used solely for TLS-posture
// detection by ValidateSaaSTLS. When MONGO_URI is provided directly it is
// returned unchanged. When only split fields (MONGO_HOST, MONGO_PORT, etc.)
// are configured we construct a representative "mongodb://" URI so
// DetectMongoTLS can inspect the scheme.
//
// MONGO_PARAMETERS handling: when the operator declares connection options
// via MongoDBParameters (e.g. "tls=true&authSource=admin") rather than
// embedding them in MONGO_URI, the parameters are appended as a query
// string. Without this, an operator who runs Mongo over TLS via split
// fields would be incorrectly classified as non-TLS by DetectMongoTLS and
// blocked at SaaS bootstrap.
//
// The synthesized URI is never used to dial Mongo. Non-TLS scheme is
// chosen by default: SaaS TLS enforcement requires operators to set
// MONGO_URI explicitly with a TLS-implicit scheme (mongodb+srv://) or
// with tls=true. Empty MongoDBHost yields an empty string, which
// ValidateSaaSTLS skips.
func synthesizeWorkerMongoURI(cfg *Config) string {
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

// synthesizeWorkerRabbitMQURI returns the RabbitMQ URI used solely for
// TLS-posture detection by ValidateSaaSTLS. When RABBITMQ_URI is provided
// directly it is returned unchanged. When only split fields are configured
// we construct a representative "amqp://" or "amqps://" URI so
// DetectAMQPTLS can inspect the scheme.
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
func synthesizeWorkerRabbitMQURI(cfg *Config) string {
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

// synthesizeWorkerRedisURI builds a Redis URI from (host[:port], tls bool) so
// that DetectRedisTLS — which only consults the URL scheme — can determine
// TLS posture. Empty host returns an empty string (ValidateSaaSTLS skips the
// dep). The resulting URI is not used to dial Redis; it exists solely for
// scheme inspection.
func synthesizeWorkerRedisURI(hostPort string, tls bool) string {
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

// joinWorkerHostPort joins host and port with a colon when both are non-empty.
// When port is empty, host is returned as-is (it may already contain a port).
// When host is empty, returns "".
func joinWorkerHostPort(host, port string) string {
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
