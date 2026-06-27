// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libCircuitBreaker "github.com/LerianStudio/lib-commons/v5/commons/circuitbreaker"
	libRedis "github.com/LerianStudio/lib-commons/v5/commons/redis"
	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	tmevent "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/event"
	tmmiddleware "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/middleware"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/postgres"
	tmredis "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/redis"
	"github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/tenantcache"
	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/lib-observability/metrics"
	libRuntime "github.com/LerianStudio/lib-observability/runtime"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libZap "github.com/LerianStudio/lib-observability/zap"
	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	onbRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/onboarding"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	tracerclient "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/composition"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	midazhttp "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const ApplicationName = "ledger"

// Config is the unified configuration struct for the ledger component.
// It merges all fields previously spread across onboarding, transaction, and ledger configs.
// Prefixed fields (Onb*/Txn*) map to domain-specific env vars; shared fields use common env vars.
type Config struct {
	// --- Shared fields ---
	ApplicationName string `env:"APPLICATION_NAME"`
	EnvName         string `env:"ENV_NAME"`
	LogLevel        string `env:"LOG_LEVEL"`
	Version         string `env:"VERSION"`
	DeploymentMode  string `env:"DEPLOYMENT_MODE"`

	// Server configuration - unified port for all APIs
	ServerAddress string `env:"SERVER_ADDRESS" default:":3002"`

	// OpenTelemetry configuration
	OtelServiceName         string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName         string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion      string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv       string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry         bool   `env:"ENABLE_TELEMETRY"`

	// Auth configuration
	AuthEnabled bool   `env:"PLUGIN_AUTH_ENABLED"`
	AuthHost    string `env:"PLUGIN_AUTH_HOST"`
	JWKAddress  string `env:"CASDOOR_JWK_ADDRESS"`

	// Redis configuration (shared across domains)
	// Defaults are applied programmatically by applyConfigDefaults after env loading.
	RedisHost                    string `env:"REDIS_HOST"`
	RedisMasterName              string `env:"REDIS_MASTER_NAME"`
	RedisPassword                string `env:"REDIS_PASSWORD"`
	RedisDB                      int    `env:"REDIS_DB"`
	RedisProtocol                int    `env:"REDIS_PROTOCOL"`
	RedisTLS                     bool   `env:"REDIS_TLS"`
	RedisCACert                  string `env:"REDIS_CA_CERT"`
	RedisUseGCPIAM               bool   `env:"REDIS_USE_GCP_IAM"`
	RedisServiceAccount          string `env:"REDIS_SERVICE_ACCOUNT"`
	GoogleApplicationCredentials string `env:"GOOGLE_APPLICATION_CREDENTIALS"`
	RedisTokenLifeTime           int    `env:"REDIS_TOKEN_LIFETIME"`
	RedisTokenRefreshDuration    int    `env:"REDIS_TOKEN_REFRESH_DURATION"`
	RedisPoolSize                int    `env:"REDIS_POOL_SIZE"`
	RedisMinIdleConns            int    `env:"REDIS_MIN_IDLE_CONNS"`
	RedisReadTimeout             int    `env:"REDIS_READ_TIMEOUT"`
	RedisWriteTimeout            int    `env:"REDIS_WRITE_TIMEOUT"`
	RedisDialTimeout             int    `env:"REDIS_DIAL_TIMEOUT"`
	RedisPoolTimeout             int    `env:"REDIS_POOL_TIMEOUT"`
	RedisMaxRetries              int    `env:"REDIS_MAX_RETRIES"`
	RedisMinRetryBackoff         int    `env:"REDIS_MIN_RETRY_BACKOFF"`
	RedisMaxRetryBackoff         int    `env:"REDIS_MAX_RETRY_BACKOFF"`

	// Multi-tenant configuration
	MultiTenantEnabled                     bool   `env:"MULTI_TENANT_ENABLED"`
	MultiTenantURL                         string `env:"MULTI_TENANT_URL"`
	MultiTenantCircuitBreakerThreshold     int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD"`
	MultiTenantCircuitBreakerTimeoutSec    int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC"`
	MultiTenantServiceAPIKey               string `env:"MULTI_TENANT_SERVICE_API_KEY"`
	MultiTenantConnectionsCheckIntervalSec int    `env:"MULTI_TENANT_CONNECTIONS_CHECK_INTERVAL_SEC"`
	MultiTenantCacheTTLSec                 int    `env:"MULTI_TENANT_CACHE_TTL_SEC" default:"120"` // seconds for tenant config cache TTL (0 = disabled)
	MultiTenantRedisHost                   string `env:"MULTI_TENANT_REDIS_HOST"`
	MultiTenantRedisPort                   string `env:"MULTI_TENANT_REDIS_PORT"`
	MultiTenantRedisPassword               string `env:"MULTI_TENANT_REDIS_PASSWORD"`
	MultiTenantRedisTLS                    bool   `env:"MULTI_TENANT_REDIS_TLS"`

	// --- Onboarding PostgreSQL fields (DB_ONBOARDING_* env tags) ---
	OnbPrefixedPrimaryDBHost     string `env:"DB_ONBOARDING_HOST"`
	OnbPrefixedPrimaryDBUser     string `env:"DB_ONBOARDING_USER"`
	OnbPrefixedPrimaryDBPassword string `env:"DB_ONBOARDING_PASSWORD"`
	OnbPrefixedPrimaryDBName     string `env:"DB_ONBOARDING_NAME"`
	OnbPrefixedPrimaryDBPort     string `env:"DB_ONBOARDING_PORT"`
	OnbPrefixedPrimaryDBSSLMode  string `env:"DB_ONBOARDING_SSLMODE"`

	OnbPrefixedReplicaDBHost     string `env:"DB_ONBOARDING_REPLICA_HOST"`
	OnbPrefixedReplicaDBUser     string `env:"DB_ONBOARDING_REPLICA_USER"`
	OnbPrefixedReplicaDBPassword string `env:"DB_ONBOARDING_REPLICA_PASSWORD"`
	OnbPrefixedReplicaDBName     string `env:"DB_ONBOARDING_REPLICA_NAME"`
	OnbPrefixedReplicaDBPort     string `env:"DB_ONBOARDING_REPLICA_PORT"`
	OnbPrefixedReplicaDBSSLMode  string `env:"DB_ONBOARDING_REPLICA_SSLMODE"`

	OnbPrefixedMaxOpenConnections int `env:"DB_ONBOARDING_MAX_OPEN_CONNS"`
	OnbPrefixedMaxIdleConnections int `env:"DB_ONBOARDING_MAX_IDLE_CONNS"`

	// --- Transaction PostgreSQL fields (DB_TRANSACTION_* env tags) ---
	TxnPrefixedPrimaryDBHost     string `env:"DB_TRANSACTION_HOST"`
	TxnPrefixedPrimaryDBUser     string `env:"DB_TRANSACTION_USER"`
	TxnPrefixedPrimaryDBPassword string `env:"DB_TRANSACTION_PASSWORD"`
	TxnPrefixedPrimaryDBName     string `env:"DB_TRANSACTION_NAME"`
	TxnPrefixedPrimaryDBPort     string `env:"DB_TRANSACTION_PORT"`
	TxnPrefixedPrimaryDBSSLMode  string `env:"DB_TRANSACTION_SSLMODE"`

	TxnPrefixedReplicaDBHost     string `env:"DB_TRANSACTION_REPLICA_HOST"`
	TxnPrefixedReplicaDBUser     string `env:"DB_TRANSACTION_REPLICA_USER"`
	TxnPrefixedReplicaDBPassword string `env:"DB_TRANSACTION_REPLICA_PASSWORD"`
	TxnPrefixedReplicaDBName     string `env:"DB_TRANSACTION_REPLICA_NAME"`
	TxnPrefixedReplicaDBPort     string `env:"DB_TRANSACTION_REPLICA_PORT"`
	TxnPrefixedReplicaDBSSLMode  string `env:"DB_TRANSACTION_REPLICA_SSLMODE"`

	TxnPrefixedMaxOpenConnections int `env:"DB_TRANSACTION_MAX_OPEN_CONNS"`
	TxnPrefixedMaxIdleConnections int `env:"DB_TRANSACTION_MAX_IDLE_CONNS"`

	// --- Onboarding MongoDB fields (MONGO_ONBOARDING_* env tags) ---
	OnbPrefixedMongoURI          string `env:"MONGO_ONBOARDING_URI"`
	OnbPrefixedMongoDBHost       string `env:"MONGO_ONBOARDING_HOST"`
	OnbPrefixedMongoDBName       string `env:"MONGO_ONBOARDING_NAME"`
	OnbPrefixedMongoDBUser       string `env:"MONGO_ONBOARDING_USER"`
	OnbPrefixedMongoDBPassword   string `env:"MONGO_ONBOARDING_PASSWORD"`
	OnbPrefixedMongoDBPort       string `env:"MONGO_ONBOARDING_PORT"`
	OnbPrefixedMongoDBParameters string `env:"MONGO_ONBOARDING_PARAMETERS"`
	OnbPrefixedMaxPoolSize       int    `env:"MONGO_ONBOARDING_MAX_POOL_SIZE"`
	OnbPrefixedMongoTLSCACert    string `env:"MONGO_ONBOARDING_TLS_CA_CERT"`

	// --- Transaction MongoDB fields (MONGO_TRANSACTION_* env tags) ---
	TxnPrefixedMongoURI          string `env:"MONGO_TRANSACTION_URI"`
	TxnPrefixedMongoDBHost       string `env:"MONGO_TRANSACTION_HOST"`
	TxnPrefixedMongoDBName       string `env:"MONGO_TRANSACTION_NAME"`
	TxnPrefixedMongoDBUser       string `env:"MONGO_TRANSACTION_USER"`
	TxnPrefixedMongoDBPassword   string `env:"MONGO_TRANSACTION_PASSWORD"`
	TxnPrefixedMongoDBPort       string `env:"MONGO_TRANSACTION_PORT"`
	TxnPrefixedMongoDBParameters string `env:"MONGO_TRANSACTION_PARAMETERS"`
	TxnPrefixedMaxPoolSize       int    `env:"MONGO_TRANSACTION_MAX_POOL_SIZE"`
	TxnPrefixedMongoTLSCACert    string `env:"MONGO_TRANSACTION_TLS_CA_CERT"`

	// --- CRM MongoDB fields (MONGO_CRM_* env tags) ---
	// CRM (holder/alias) collapsed into the unified ledger binary.
	CrmPrefixedMongoURI          string `env:"MONGO_CRM_URI"`
	CrmPrefixedMongoDBHost       string `env:"MONGO_CRM_HOST"`
	CrmPrefixedMongoDBName       string `env:"MONGO_CRM_NAME"`
	CrmPrefixedMongoDBUser       string `env:"MONGO_CRM_USER"`
	CrmPrefixedMongoDBPassword   string `env:"MONGO_CRM_PASSWORD"`
	CrmPrefixedMongoDBPort       string `env:"MONGO_CRM_PORT"`
	CrmPrefixedMongoDBParameters string `env:"MONGO_CRM_PARAMETERS"`
	CrmPrefixedMaxPoolSize       int    `env:"MONGO_CRM_MAX_POOL_SIZE"`
	CrmPrefixedMongoTLSCACert    string `env:"MONGO_CRM_TLS_CA_CERT"`

	// --- CRM crypto keys (holder/alias PII at-rest encryption) ---
	// These keep the BARE LCRYPTO_* env names (no CRM prefix) so the EXACT key
	// VALUES used by the standalone CRM service carry over unchanged. Changing
	// either value renders existing holder/alias PII undecryptable.
	CrmHashSecretKey    string `env:"LCRYPTO_HASH_SECRET_KEY"`
	CrmEncryptSecretKey string `env:"LCRYPTO_ENCRYPT_SECRET_KEY"`

	// --- CRM KMS / field-encryption (envelope) config ---
	// KMS_VENDOR selects the encryption mode: "none" (or empty) keeps legacy
	// lib-commons crypto; "hashicorp-vault" enables KMS-backed envelope encryption
	// for holder/instrument fields. The Vault fields are required only for envelope
	// mode and validated then; legacy mode ignores them.
	KMSVendor       string `env:"KMS_VENDOR"`
	VaultAddr       string `env:"KMS_VAULT_ADDR"`
	VaultRoleID     string `env:"KMS_VAULT_ROLE_ID"`
	VaultSecretID   string `env:"KMS_VAULT_SECRET_ID"`
	VaultMountPath  string `env:"KMS_VAULT_MOUNT_PATH"`
	VaultAuthMethod string `env:"KMS_VAULT_AUTH_METHOD"`

	// --- Fees MongoDB fields (MONGO_FEES_* env tags) ---
	// Fee/billing-package collections collapsed into the unified ledger binary
	// (P4). Namespaced MONGO_FEES_* to avoid colliding with the standalone fees
	// service's bare MONGO_* surface and ledger's MONGO_ONBOARDING_/MONGO_TRANSACTION_.
	FeesPrefixedMongoURI          string `env:"MONGO_FEES_URI"`
	FeesPrefixedMongoDBHost       string `env:"MONGO_FEES_HOST"`
	FeesPrefixedMongoDBName       string `env:"MONGO_FEES_NAME"`
	FeesPrefixedMongoDBUser       string `env:"MONGO_FEES_USER"`
	FeesPrefixedMongoDBPassword   string `env:"MONGO_FEES_PASSWORD"`
	FeesPrefixedMongoDBPort       string `env:"MONGO_FEES_PORT"`
	FeesPrefixedMongoDBParameters string `env:"MONGO_FEES_PARAMETERS"`
	FeesPrefixedMaxPoolSize       int    `env:"MONGO_FEES_MAX_POOL_SIZE"`
	FeesPrefixedMongoTLSCACert    string `env:"MONGO_FEES_TLS_CA_CERT"`

	// --- Fee engine config (FEES_* / DEFAULT_CURRENCY) ---
	// DEFAULT_CURRENCY keeps its bare env name (carried verbatim from the
	// standalone fees service) so existing deployments need no rename. It is the
	// fallback currency used by the fee calculation engine when a fee leg does
	// not specify one. Defaults to "USD" in applyConfigDefaults when unset.
	FeesDefaultCurrency string `env:"DEFAULT_CURRENCY"`

	// --- RabbitMQ (transaction domain only) ---
	RabbitURI                                string `env:"RABBITMQ_URI"`
	RabbitMQHost                             string `env:"RABBITMQ_HOST"`
	RabbitMQPortHost                         string `env:"RABBITMQ_PORT_HOST"`
	RabbitMQPortAMQP                         string `env:"RABBITMQ_PORT_AMQP"`
	RabbitMQUser                             string `env:"RABBITMQ_DEFAULT_USER"`
	RabbitMQPass                             string `env:"RABBITMQ_DEFAULT_PASS"`
	RabbitMQConsumerUser                     string `env:"RABBITMQ_CONSUMER_USER"`
	RabbitMQConsumerPass                     string `env:"RABBITMQ_CONSUMER_PASS"`
	RabbitMQVHost                            string `env:"RABBITMQ_VHOST"`
	RabbitMQNumbersOfWorkers                 int    `env:"RABBITMQ_NUMBERS_OF_WORKERS"`
	RabbitMQNumbersOfPrefetch                int    `env:"RABBITMQ_NUMBERS_OF_PREFETCH"`
	RabbitMQHealthCheckURL                   string `env:"RABBITMQ_HEALTH_CHECK_URL"`
	RabbitMQTLS                              bool   `env:"RABBITMQ_TLS"`
	RabbitMQTransactionBalanceOperationQueue string `env:"RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE"`

	// Circuit Breaker configuration for RabbitMQ
	RabbitMQCircuitBreakerConsecutiveFailures int    `env:"RABBITMQ_CIRCUIT_BREAKER_CONSECUTIVE_FAILURES"`
	RabbitMQCircuitBreakerFailureRatio        int    `env:"RABBITMQ_CIRCUIT_BREAKER_FAILURE_RATIO"`
	RabbitMQCircuitBreakerInterval            int    `env:"RABBITMQ_CIRCUIT_BREAKER_INTERVAL"`
	RabbitMQCircuitBreakerMaxRequests         int    `env:"RABBITMQ_CIRCUIT_BREAKER_MAX_REQUESTS"`
	RabbitMQCircuitBreakerMinRequests         int    `env:"RABBITMQ_CIRCUIT_BREAKER_MIN_REQUESTS"`
	RabbitMQCircuitBreakerTimeout             int    `env:"RABBITMQ_CIRCUIT_BREAKER_TIMEOUT"`
	RabbitMQCircuitBreakerHealthCheckInterval int    `env:"RABBITMQ_CIRCUIT_BREAKER_HEALTH_CHECK_INTERVAL"`
	RabbitMQCircuitBreakerHealthCheckTimeout  int    `env:"RABBITMQ_CIRCUIT_BREAKER_HEALTH_CHECK_TIMEOUT"`
	RabbitMQOperationTimeout                  string `env:"RABBITMQ_OPERATION_TIMEOUT"`
	RabbitMQTransactionAsync                  bool   `env:"RABBITMQ_TRANSACTION_ASYNC"`

	// Bulk mode activates only when RABBITMQ_TRANSACTION_ASYNC=true AND BulkRecorderEnabled=true.
	// Bulk size should match RabbitMQ prefetch for optimal performance (workers × prefetch).
	BulkRecorderEnabled          bool `env:"BULK_RECORDER_ENABLED"`
	BulkRecorderSize             int  `env:"BULK_RECORDER_SIZE"`
	BulkRecorderFlushTimeoutMs   int  `env:"BULK_RECORDER_FLUSH_TIMEOUT_MS"`
	BulkRecorderMaxRowsPerInsert int  `env:"BULK_RECORDER_MAX_ROWS_PER_INSERT"`

	// --- Balance/Worker fields ---
	BalanceSyncBatchSize      int `env:"BALANCE_SYNC_BATCH_SIZE"`
	BalanceSyncFlushTimeoutMs int `env:"BALANCE_SYNC_FLUSH_TIMEOUT_MS"`
	BalanceSyncPollIntervalMs int `env:"BALANCE_SYNC_POLL_INTERVAL_MS"`

	// --- Streaming (lib-streaming producer) ---
	// Default for all streaming knobs is OFF — a service with
	// STREAMING_ENABLED=false (or unset) injects a NoopEmitter and never
	// initialises the underlying transport. The pilot ships disabled-by-
	// default so that existing deployments are not broken by the new
	// dependency.
	StreamingEnabled           bool   `env:"STREAMING_ENABLED"`
	StreamingBrokers           string `env:"STREAMING_BROKERS"`
	StreamingClientID          string `env:"STREAMING_CLIENT_ID"`
	StreamingCloudEventsSource string `env:"STREAMING_CLOUDEVENTS_SOURCE"`
	StreamingCompression       string `env:"STREAMING_COMPRESSION"`
	StreamingRequiredAcks      string `env:"STREAMING_REQUIRED_ACKS"`
	StreamingBatchLingerMs     int    `env:"STREAMING_BATCH_LINGER_MS"`

	// --- Streaming SASL/TLS auth ---
	// When STREAMING_SASL_MECHANISM is empty (default) the producer connects
	// without authentication, matching the existing behaviour for local/dev
	// brokers. When set, the value must be one of PLAIN, SCRAM-SHA-256,
	// SCRAM-SHA-512 (case-insensitive); USERNAME and PASSWORD are then
	// required and BuildStreamingEmitter wires the matching franz-go
	// sasl.Mechanism into the lib-streaming Builder.
	//
	// SASL without TLS is rejected by lib-streaming with
	// ErrPlaintextSASLNotAllowed. STREAMING_ALLOW_PLAINTEXT_SASL=true is the
	// explicit unsafe opt-in for local/dev brokers that do not terminate
	// TLS. It must NOT be set in production: SASL credentials cross the
	// network in cleartext.
	StreamingSASLMechanism      string `env:"STREAMING_SASL_MECHANISM"`
	StreamingSASLUsername       string `env:"STREAMING_SASL_USERNAME"`
	StreamingSASLPassword       string `env:"STREAMING_SASL_PASSWORD"`
	StreamingAllowPlaintextSASL bool   `env:"STREAMING_ALLOW_PLAINTEXT_SASL"`

	// --- Tracer reservation client ---
	// TRACER_BASE_URL is the escape hatch for the tracer integration as a
	// whole: empty (the default) injects a nil TracerReserver so the
	// transaction create path stays unchanged. When set, the reservation HTTP
	// client is constructed and injected. The per-ledger advisory/enforce gate
	// is a tracer.mode setting read at the call site, not a global flag.
	// TracerTimeoutMs bounds each reservation call so a slow tracer cannot hold
	// the transaction create path open; it mirrors the tracer.timeoutMs setting
	// default (250ms) and is overridden per-ledger by the call site.
	// TracerTransport selects the reservation transport: "grpc" (default) or
	// "rest". gRPC is the seam's production transport; REST is retained as a
	// fallback, selectable by setting TRACER_TRANSPORT=rest. A deploy that wires
	// the tracer (TRACER_BASE_URL set) without overriding this now speaks gRPC,
	// so the tracer must expose its gRPC seam (TRACER_GRPC_PORT) and, under
	// TRACER_TLS_MODE=mtls, both ends need cert material.
	// TracerTLSMode secures the reservation seam: "mtls" presents a client
	// certificate and verifies the tracer's server certificate against the CA
	// (mutual TLS is the seam's identity — no shared secret); "mesh" (and the
	// empty default) speaks plaintext to a local service-mesh sidecar that
	// terminates mTLS. Under "mtls" the cert/key/CA paths are required when the
	// integration is on, enforced by buildTracerReserver.
	TracerBaseURL     string `env:"TRACER_BASE_URL"`
	TracerTimeoutMs   int    `env:"TRACER_TIMEOUT_MS"`
	TracerTransport   string `env:"TRACER_TRANSPORT"`
	TracerTLSMode     string `env:"TRACER_TLS_MODE"`
	TracerTLSCertFile string `env:"TRACER_TLS_CERT_FILE"`
	TracerTLSKeyFile  string `env:"TRACER_TLS_KEY_FILE"`
	TracerTLSCAFile   string `env:"TRACER_TLS_CA_FILE"`
}

// Options contains optional dependencies that can be injected by callers.
type Options struct {
	// Logger allows callers to provide a pre-configured logger.
	Logger libLog.Logger

	// CircuitBreakerStateListener receives notifications when circuit breaker state changes.
	CircuitBreakerStateListener libCircuitBreaker.StateChangeListener

	// TenantClient is the tenant manager client for multi-tenant mode.
	TenantClient *tmclient.Client

	// TenantCache is the shared process-local cache for tenant configurations.
	TenantCache *tenantcache.TenantCache

	// Multi-tenant configuration (resolved from Config during init).
	MultiTenantEnabled       bool
	TenantServiceName        string
	TenantManagerURL         string
	MultiTenantServiceAPIKey string
}

// InitServers initializes the unified ledger service.
func InitServers() (*Service, error) {
	return InitServersWithOptions(nil)
}

// validateBootAuthGates enforces the auth-presence invariants that must hold
// before any infrastructure opens. It runs before the logger exists, so it
// returns errors rather than logging. Two independent gates:
//
//  1. Multi-tenant mode requires auth in every environment — tenant identity
//     comes from the JWT, so disabling auth allows cross-tenant data access.
//  2. Production requires auth even single-tenant — lib-auth fail-opens when
//     disabled, so a production deploy that forgets PLUGIN_AUTH_ENABLED=true
//     would serve every business endpoint unauthenticated. Non-production
//     (local/dev/staging) keeps Warn-free boot for developer onboarding.
func validateBootAuthGates(cfg *Config) error {
	if cfg.MultiTenantEnabled && !cfg.AuthEnabled {
		return fmt.Errorf("MULTI_TENANT_ENABLED=true requires PLUGIN_AUTH_ENABLED=true; " +
			"running multi-tenant mode without authentication allows cross-tenant data access")
	}

	if strings.EqualFold(strings.TrimSpace(cfg.EnvName), "production") && !cfg.AuthEnabled {
		return fmt.Errorf("ENV_NAME=production requires PLUGIN_AUTH_ENABLED=true; " +
			"a single-tenant production deployment without authentication serves every endpoint unauthenticated")
	}

	return nil
}

// InitServersWithOptions initializes the unified ledger service with optional dependency injection.
// It directly initializes all infrastructure (PG, Mongo, Redis, RabbitMQ) instead of delegating
// to onboarding/transaction sub-modules.
//
//nolint:gocognit,gocyclo // Will be refactored into smaller initialization functions.
func InitServersWithOptions(opts *Options) (*Service, error) {
	cfg := &Config{}

	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config from environment variables: %w", err)
	}

	applyConfigDefaults(cfg)

	if err := validateBootAuthGates(cfg); err != nil {
		return nil, err
	}

	// Logger: use injected or create fresh
	var baseLogger libLog.Logger
	if opts != nil && opts.Logger != nil {
		baseLogger = opts.Logger
	} else {
		var err error

		baseLogger, err = libZap.New(libZap.Config{
			Environment:     resolveLoggerEnvironment(cfg.EnvName),
			Level:           cfg.LogLevel,
			OTelLibraryName: cfg.OtelLibraryName,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to initialize logger: %w", err)
		}
	}

	// Generate startup ID for tracing initialization issues
	startupID := uuid.New().String()

	logger := baseLogger.With(
		libLog.String("component", ApplicationName),
		libLog.String("startup_id", startupID),
	)

	logger.Log(context.Background(), libLog.LevelInfo, "Starting unified ledger component",
		libLog.String("version", cfg.Version),
		libLog.String("env", cfg.EnvName),
	)

	// Telemetry
	telemetry, err := libOpentelemetry.NewTelemetry(libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    baseLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	// Domain-metrics factory (D6): derived once from telemetry (nil when
	// telemetry is disabled) and shared across every use case so
	// utils.RecordDomainOperation emits the bounded domain_operations_total /
	// domain_operation_duration_ms series. Also threaded into CRM field-encryption
	// wiring below (protection metrics) and reused for the readyz handler.
	var metricsFactory *metrics.MetricsFactory
	if telemetry != nil {
		metricsFactory = telemetry.MetricsFactory
	}

	// Register telemetry providers as process-global so that the otelzap bridge
	// (installed in the logger core) can forward log records to the OTLP exporter.
	if err := telemetry.ApplyGlobals(); err != nil {
		return nil, fmt.Errorf("failed to apply telemetry globals: %w", err)
	}

	// Multi-tenant client setup
	var tenantClient *tmclient.Client

	var tenantServiceName string

	if opts != nil && opts.TenantClient != nil {
		tenantClient = opts.TenantClient
		tenantServiceName = strings.TrimSpace(cfg.ApplicationName)
	} else {
		var err error

		tenantClient, tenantServiceName, err = initTenantClient(cfg, logger)
		if err != nil {
			return nil, err
		}
	}

	// Validate TenantServiceName early so that workers fail fast on misconfiguration
	if cfg.MultiTenantEnabled {
		tenantServiceName = strings.TrimSpace(tenantServiceName)
		if tenantServiceName == "" {
			return nil, fmt.Errorf("TenantServiceName must not be empty when multi-tenant is enabled")
		}
	}

	// Build internal options struct used by init functions
	internalOpts := &Options{
		MultiTenantEnabled:          cfg.MultiTenantEnabled,
		TenantClient:                tenantClient,
		TenantServiceName:           tenantServiceName,
		TenantManagerURL:            strings.TrimSpace(cfg.MultiTenantURL),
		MultiTenantServiceAPIKey:    strings.TrimSpace(cfg.MultiTenantServiceAPIKey),
		CircuitBreakerStateListener: nil,
	}
	if opts != nil {
		internalOpts.CircuitBreakerStateListener = opts.CircuitBreakerStateListener
	}

	// === Tenant Cache (multi-tenant only) ===
	// The dispatcher and event listener are created AFTER infrastructure init (PG, Mongo, RabbitMQ)
	// so that they can receive the infrastructure managers for closing tenant connections on
	// suspend/delete events.

	var tenantCache *tenantcache.TenantCache

	var tenantLoader *tenantcache.TenantLoader

	var eventListener *tmevent.TenantEventListener

	var cacheTTL time.Duration

	if cfg.MultiTenantEnabled && tenantClient != nil {
		tenantCache = tenantcache.NewTenantCache()

		cacheTTL = time.Duration(cfg.MultiTenantCacheTTLSec) * time.Second
		tenantLoader = tenantcache.NewTenantLoader(tenantClient, tenantCache, tenantServiceName, cacheTTL, logger)

		internalOpts.TenantCache = tenantCache
	}

	// === Infrastructure initialization ===

	// Cleanup helper: on error, close resources in reverse order of creation.
	var cleanups []func()

	addCleanup := func(fn func()) { cleanups = append(cleanups, fn) }
	doCleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	// 1. Onboarding PostgreSQL → 7 repos
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing onboarding PostgreSQL...")

	onbPG, err := initOnboardingPostgres(internalOpts, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize onboarding PostgreSQL: %w", err)
	}

	addCleanup(func() { _ = onbPG.connection.Close() })

	// 2. Transaction PostgreSQL → 6 repos
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing transaction PostgreSQL...")

	txnPG, err := initTransactionPostgres(internalOpts, cfg, logger)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize transaction PostgreSQL: %w", err)
	}

	addCleanup(func() { _ = txnPG.connection.Close() })

	// 3. Onboarding MongoDB → metadata repo
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing onboarding MongoDB...")

	onbMgo, err := initOnboardingMongo(internalOpts, cfg, logger)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize onboarding MongoDB: %w", err)
	}

	if onbMgo.connection != nil {
		addCleanup(func() { _ = onbMgo.connection.Close(context.Background()) })
	}

	// 4. Transaction MongoDB → metadata repo
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing transaction MongoDB...")

	txnMgo, err := initTransactionMongo(internalOpts, cfg, logger)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize transaction MongoDB: %w", err)
	}

	if txnMgo.connection != nil {
		addCleanup(func() { _ = txnMgo.connection.Close(context.Background()) })
	}

	// 4b. CRM MongoDB → holder/instrument repos + handlers (collapsed from the
	// standalone CRM service in P3). Single-tenant builds a static client;
	// multi-tenant builds a 3rd crm-api tenant Mongo manager. Field encryption
	// (legacy or KMS envelope) is wired here and injected into the repos.
	// metricsFactory (derived from telemetry above) feeds the encryption
	// protection-metrics seam; the seam is nil-safe if telemetry is disabled.
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing CRM MongoDB...")

	crmMgo, err := initCRM(internalOpts, cfg, metricsFactory, logger)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize CRM MongoDB: %w", err)
	}

	if crmMgo.connection != nil {
		addCleanup(func() { _ = crmMgo.connection.Close(context.Background()) })
	}

	// 4c. Fees MongoDB → pack/billing-package repos (collapsed from the
	// standalone plugin-fees service in P4). The constructors ensure the 11
	// compound indexes on the static connection's DB at startup. In MT mode a
	// fee tenant Mongo manager (module plugin-fees) is also built; per-request
	// DB resolution lands on tmcore via the route-scoped middleware.
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing fees MongoDB...")

	feeMgo, err := initFeesMongo(internalOpts, cfg, logger)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize fees MongoDB: %w", err)
	}

	if feeMgo.connection != nil {
		addCleanup(func() { _ = feeMgo.connection.Close(context.Background()) })
	}

	// 5. Redis (shared connection, two consumer repos)
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing Redis...")

	redisConnection, err := initRedisConnection(cfg, logger)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize Redis: %w", err)
	}

	addCleanup(func() { _ = redisConnection.Close() })

	onbRedisRepo, err := onbRedis.NewConsumerRedis(redisConnection)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize onboarding redis consumer: %w", err)
	}

	txnRedisRepo, err := txRedis.NewConsumerRedis(redisConnection)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize transaction redis consumer: %w", err)
	}

	// 6. RabbitMQ → producer + consumer
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing RabbitMQ...")

	rmq, err := initRabbitMQ(internalOpts, cfg, logger, telemetry)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize RabbitMQ: %w", err)
	}

	if rmq != nil && rmq.producerRepo != nil {
		addCleanup(func() { _ = rmq.producerRepo.Close() })
	}

	// Pass PG and Mongo managers to RabbitMQ components for per-message tenant resolution
	if rmq != nil {
		rmq.pgManager = txnPG.pgManager
		rmq.mongoManager = txnMgo.mongoManager
	}

	// === Event Dispatcher & Listener (multi-tenant only) ===
	// Created AFTER infrastructure init so the dispatcher receives the PG, Mongo, and RabbitMQ
	// managers. This allows removeTenant to close infrastructure connections when a tenant is
	// suspended, deleted, or disassociated.
	if cfg.MultiTenantEnabled && tenantCache != nil && tenantLoader != nil {
		dispatcherOpts := []tmevent.DispatcherOption{
			tmevent.WithDispatcherLogger(logger),
			tmevent.WithCacheTTL(cacheTTL),
			// Ownership signal for lib-commons' tenant.cache.invalidate handling: report a
			// tenant as owned when midaz has a live consumer goroutine for it. Stats()
			// reflects live goroutines, so this stays accurate even when the tier-1 cache
			// entry has TTL-expired (the default cache-based check would miss it and skip
			// the reload that restarts the consumer, leaving it stopped).
			tmevent.WithTenantOwnershipChecker(func(tenantID string) bool {
				if rmq == nil || rmq.multiTenantConsumer == nil {
					return false
				}

				for _, id := range rmq.multiTenantConsumer.Stats().TenantIDs {
					if id == tenantID {
						return true
					}
				}

				return false
			}),
			tmevent.WithOnTenantAdded(func(ctx context.Context, tenantID string) {
				if tenantClient != nil {
					_ = tenantClient.InvalidateConfig(ctx, tenantID, tenantServiceName)
				}

				if rmq != nil && rmq.multiTenantConsumer != nil {
					rmq.multiTenantConsumer.EnsureConsumerStarted(ctx, tenantID)
				}
			}),
			tmevent.WithOnTenantRemoved(func(ctx context.Context, tenantID string) {
				if rmq != nil && rmq.multiTenantConsumer != nil {
					rmq.multiTenantConsumer.StopConsumer(tenantID)
				}

				// Close ALL postgres managers (onboarding + transaction)
				if onbPG.pgManager != nil {
					if err := onbPG.pgManager.CloseConnection(ctx, tenantID); err != nil {
						logger.Log(ctx, libLog.LevelWarn, "failed to close onboarding PG connection",
							libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
					}
				}

				if txnPG.pgManager != nil {
					if err := txnPG.pgManager.CloseConnection(ctx, tenantID); err != nil {
						logger.Log(ctx, libLog.LevelWarn, "failed to close transaction PG connection",
							libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
					}
				}

				// Close ALL mongo managers (onboarding + transaction + crm-api)
				if onbMgo.mongoManager != nil {
					if err := onbMgo.mongoManager.CloseConnection(ctx, tenantID); err != nil {
						logger.Log(ctx, libLog.LevelWarn, "failed to close onboarding Mongo connection",
							libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
					}
				}

				if txnMgo.mongoManager != nil {
					if err := txnMgo.mongoManager.CloseConnection(ctx, tenantID); err != nil {
						logger.Log(ctx, libLog.LevelWarn, "failed to close transaction Mongo connection",
							libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
					}
				}

				if crmMgo.mongoManager != nil {
					if err := crmMgo.mongoManager.CloseConnection(ctx, tenantID); err != nil {
						logger.Log(ctx, libLog.LevelWarn, "failed to close crm-api Mongo connection",
							libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
					}
				}

				if feeMgo.mongoManager != nil {
					if err := feeMgo.mongoManager.CloseConnection(ctx, tenantID); err != nil {
						logger.Log(ctx, libLog.LevelWarn, "failed to close plugin-fees Mongo connection",
							libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
					}
				}

				// Close RabbitMQ
				if rmq != nil && rmq.rabbitmqManager != nil {
					if err := rmq.rabbitmqManager.CloseConnection(ctx, tenantID); err != nil {
						logger.Log(ctx, libLog.LevelWarn, "failed to close RabbitMQ connection",
							libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
					}
				}

				// Invalidate pmClient internal cache so lazy-load fetches fresh state from tenant-manager
				if tenantClient != nil {
					if err := tenantClient.InvalidateConfig(ctx, tenantID, tenantServiceName); err != nil {
						logger.Log(ctx, libLog.LevelWarn, "failed to invalidate tenant config cache",
							libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
					}
				}

				logger.Log(ctx, libLog.LevelInfo, "tenant evicted: all connections and caches invalidated",
					libLog.String("tenant_id", tenantID))
			}),
		}

		dispatcher := tmevent.NewEventDispatcher(
			tenantCache,
			tenantLoader,
			tenantServiceName,
			dispatcherOpts...,
		)

		tenantLoader.SetOnTenantLoaded(func(ctx context.Context, tenantID string) {
			if rmq != nil && rmq.multiTenantConsumer != nil {
				rmq.multiTenantConsumer.EnsureConsumerStarted(ctx, tenantID)
			}
		})

		// Create Redis client for tenant-manager Pub/Sub when configured
		if cfg.MultiTenantRedisHost != "" {
			tmRedisClient, tmRedisErr := tmredis.NewTenantPubSubRedisClient(context.Background(), tmredis.TenantPubSubRedisConfig{
				Host:     cfg.MultiTenantRedisHost,
				Port:     strings.TrimSpace(cfg.MultiTenantRedisPort),
				Password: cfg.MultiTenantRedisPassword,
				TLS:      cfg.MultiTenantRedisTLS,
			})
			if tmRedisErr != nil {
				doCleanup()
				return nil, fmt.Errorf("failed to initialize tenant-manager Redis for Pub/Sub: %w", tmRedisErr)
			}

			var listenerErr error

			eventListener, listenerErr = tmevent.NewTenantEventListener(
				tmRedisClient,
				dispatcher.HandleEvent,
				tmevent.WithListenerLogger(logger),
				tmevent.WithService(tenantServiceName),
			)
			if listenerErr != nil {
				doCleanup()
				return nil, fmt.Errorf("failed to create tenant event listener: %w", listenerErr)
			}

			logger.Log(context.Background(), libLog.LevelInfo, "Tenant event listener configured",
				libLog.String("redis_host", cfg.MultiTenantRedisHost),
				libLog.String("service", tenantServiceName),
			)
		}
	}

	// === Streaming producer (lib-streaming) ===
	// Built before the UseCase so the Emitter is available for injection.
	// When STREAMING_ENABLED=false (the documented default for this pilot)
	// the helper returns a NoopEmitter and a no-op closer, preserving full
	// backward compatibility with existing deployments.

	streamingEmitter, streamingClose, err := BuildStreamingEmitter(context.Background(), cfg, logger, telemetry)
	if err != nil {
		doCleanup()

		return nil, fmt.Errorf("failed to initialize streaming emitter: %w", err)
	}

	if streamingClose != nil {
		addCleanup(func() { _ = streamingClose() })
	}

	// === Use cases ===

	// Arm lib-observability's panic-observability trident so every
	// libRuntime.SafeGo* call site (e.g. the Redis backup-consumer replay
	// goroutines) emits the panic_recovered_total counter when a goroutine
	// panics. Without this Init, SafeGo still recovers + logs + records the
	// span event, but the metric is a no-op binary-wide. Called once here,
	// after the metrics factory is resolved and before the workers spawn.
	// Idempotent — subsequent calls are no-ops.
	if metricsFactory != nil {
		libRuntime.InitPanicMetrics(metricsFactory, logger)
	}

	commandUseCase := &command.UseCase{
		// Onboarding domain
		OrganizationRepo:       onbPG.organizationRepo,
		LedgerRepo:             onbPG.ledgerRepo,
		SegmentRepo:            onbPG.segmentRepo,
		PortfolioRepo:          onbPG.portfolioRepo,
		AccountRepo:            onbPG.accountRepo,
		AssetRepo:              onbPG.assetRepo,
		AccountTypeRepo:        onbPG.accountTypeRepo,
		OnboardingMetadataRepo: onbMgo.metadataRepo,
		OnboardingRedisRepo:    onbRedisRepo,
		// Transaction domain
		TransactionRepo:         txnPG.transactionRepo,
		OperationRepo:           txnPG.operationRepo,
		AssetRateRepo:           txnPG.assetRateRepo,
		BalanceRepo:             txnPG.balanceRepo,
		OperationRouteRepo:      txnPG.operationRouteRepo,
		TransactionRouteRepo:    txnPG.transactionRouteRepo,
		TransactionMetadataRepo: txnMgo.metadataRepo,
		RabbitMQRepo:            rmq.producerRepo,
		TransactionRedisRepo:    txnRedisRepo,
		// Streaming
		Streaming: streamingEmitter,
		// Observability (D6)
		MetricsFactory: metricsFactory,
	}

	queryUseCase := &query.UseCase{
		// Onboarding domain
		OrganizationRepo:       onbPG.organizationRepo,
		LedgerRepo:             onbPG.ledgerRepo,
		SegmentRepo:            onbPG.segmentRepo,
		PortfolioRepo:          onbPG.portfolioRepo,
		AccountRepo:            onbPG.accountRepo,
		AssetRepo:              onbPG.assetRepo,
		AccountTypeRepo:        onbPG.accountTypeRepo,
		OnboardingMetadataRepo: onbMgo.metadataRepo,
		OnboardingRedisRepo:    onbRedisRepo,
		// Transaction domain
		TransactionRepo:         txnPG.transactionRepo,
		OperationRepo:           txnPG.operationRepo,
		AssetRateRepo:           txnPG.assetRateRepo,
		BalanceRepo:             txnPG.balanceRepo,
		OperationRouteRepo:      txnPG.operationRouteRepo,
		TransactionRouteRepo:    txnPG.transactionRouteRepo,
		TransactionMetadataRepo: txnMgo.metadataRepo,
		RabbitMQRepo:            rmq.producerRepo,
		TransactionRedisRepo:    txnRedisRepo,
		// Observability (D6)
		MetricsFactory: metricsFactory,
	}

	// === Holder ownership wiring (F1) ===
	// The command UseCase reads cached settings and asserts holder existence
	// through narrow ports so it never imports the query or CRM packages.
	// HolderReader adapts the CRM holder service; SettingsReader is satisfied
	// directly by the query UseCase (signatures match); HolderProvisioner is
	// satisfied directly by the CRM holder service's CreateHolderWithID.
	commandUseCase.HolderReader = holderReaderAdapter{service: crmMgo.holderHandler.Service}
	commandUseCase.SettingsReader = queryUseCase
	commandUseCase.HolderProvisioner = crmMgo.holderHandler.Service

	// === CRM domain metrics (D6) ===
	// The holder and instrument handlers share the SAME CRM use-case instance,
	// so setting the factory once covers every CRM entrypoint.
	crmMgo.holderHandler.Service.MetricsFactory = metricsFactory

	// === CRM idempotency ===
	// Reuse the already-constructed transaction Redis repo (it satisfies the
	// narrow crmservices.IdempotencyRepo port structurally). No second Redis
	// client. Set once on the shared CRM use case.
	crmMgo.holderHandler.Service.Idempotency = txnRedisRepo

	// === CRM instrument referential validation ===
	// CreateInstrument verifies the body-supplied ledger_id/account_id exist in
	// the request org via a narrow adapter over the ledger query use case, so
	// CRM never imports the query package. Set once on the shared CRM use case.
	crmMgo.holderHandler.Service.LedgerAccounts = ledgerAccountReaderAdapter{query: queryUseCase}

	// === Fee use cases ===
	// Built from the fee Mongo slice + the ledger query.UseCase so fee
	// account/segment/count reads run in-process. HTTP route mounting is
	// deferred to the next chunk (P4-T10/T17); here the fee use cases are only
	// constructed + held so they are not dead code.
	fees, err := initFees(feeMgo, queryUseCase, cfg, logger)
	if err != nil {
		doCleanup()
		return nil, fmt.Errorf("failed to initialize fee use cases: %w", err)
	}

	// === Fee domain metrics (D6) ===
	fees.useCase.MetricsFactory = metricsFactory
	fees.billingPackageService.MetricsFactory = metricsFactory
	fees.billingCalculateService.MetricsFactory = metricsFactory

	// Cache the per-(org,ledger) fee-package set so a transaction create on a
	// ledger with no fee packages skips the Mongo lookup; invalidated on package CUD.
	fees.useCase.PackageCache = txnRedisRepo

	// Wire consumer with UseCase (registers handler or creates MultiQueueConsumer)
	if err := rmq.wireConsumer(commandUseCase); err != nil {
		doCleanup()
		return nil, err
	}

	// === Handlers ===

	// Onboarding handlers
	accountHandler := &httpin.AccountHandler{Command: commandUseCase, Query: queryUseCase}
	portfolioHandler := &httpin.PortfolioHandler{Command: commandUseCase, Query: queryUseCase}
	ledgerHandler := &httpin.LedgerHandler{Command: commandUseCase, Query: queryUseCase}
	assetHandler := &httpin.AssetHandler{Command: commandUseCase, Query: queryUseCase}
	organizationHandler := &httpin.OrganizationHandler{Command: commandUseCase, Query: queryUseCase}
	segmentHandler := &httpin.SegmentHandler{Command: commandUseCase, Query: queryUseCase}
	accountTypeHandler := &httpin.AccountTypeHandler{Command: commandUseCase, Query: queryUseCase}

	// === Tracer reservation client ===
	// Built before the handler so the reserver is available for injection.
	// When TRACER_BASE_URL is empty (the documented default) the helper returns
	// a nil TracerReserver and the create path stays unchanged, mirroring the
	// streaming NoopEmitter escape hatch.
	tracerReserver, err := buildTracerReserver(cfg, logger)
	if err != nil {
		doCleanup()

		return nil, fmt.Errorf("failed to initialize tracer reservation client: %w", err)
	}

	// Resolve the optional SIGTERM teardown hook for the tracer transport.
	// The gRPC client holds a persistent grpc.ClientConn and exposes
	// Close() error; the REST client does not implement the interface, so
	// tracerClose stays nil and Run() registers no teardown app for it.
	var tracerClose func() error
	if closer, ok := tracerReserver.(interface{ Close() error }); ok {
		tracerClose = closer.Close

		// Register the transport teardown in the startup cleanup stack so the
		// gRPC ClientConn is closed if a later startup step (route setup, readyz
		// handler) fails — not only on the happy SIGTERM path via Run().
		addCleanup(func() { _ = tracerClose() })
	}

	// Transaction handlers
	transactionHandler := &httpin.TransactionHandler{
		Command:            commandUseCase,
		Query:              queryUseCase,
		FeeApplier:         fees.useCase,
		TracerReserver:     tracerReserver,
		FeesMongoManager:   feeMgo.mongoManager,
		MultiTenantEnabled: cfg.MultiTenantEnabled,
	}
	operationHandler := &httpin.OperationHandler{Command: commandUseCase, Query: queryUseCase}
	assetRateHandler := &httpin.AssetRateHandler{Command: commandUseCase, Query: queryUseCase}
	balanceHandler := &httpin.BalanceHandler{Command: commandUseCase, Query: queryUseCase}
	operationRouteHandler := &httpin.OperationRouteHandler{Command: commandUseCase, Query: queryUseCase}
	transactionRouteHandler := &httpin.TransactionRouteHandler{Command: commandUseCase, Query: queryUseCase}

	// Metadata index handler (ledger-specific)
	metadataIndexHandler := &httpin.MetadataIndexHandler{
		OnboardingMetadataRepo:  onbMgo.metadataRepo,
		TransactionMetadataRepo: txnMgo.metadataRepo,
		OnboardingMongoManager:  onbMgo.mongoManager,
		TransactionMongoManager: txnMgo.mongoManager,
	}

	// Auth
	auth := middleware.NewAuthClient(cfg.AuthHost, cfg.AuthEnabled, nil)

	// === Multi-tenant middleware ===

	routeSetup, err := buildUnifiedRouteSetup(cfg, logger, onbPG.pgManager, txnPG.pgManager, onbMgo.mongoManager, txnMgo.mongoManager, crmMgo.mongoManager, feeMgo.mongoManager, tenantCache, tenantLoader)
	if err != nil {
		doCleanup()
		return nil, err
	}

	// === Route registrars ===

	onboardingRouteRegistrar := func(router fiber.Router) {
		httpin.RegisterOnboardingRoutesToApp(router, auth, accountHandler, portfolioHandler, ledgerHandler, assetHandler, organizationHandler, segmentHandler, accountTypeHandler, routeSetup.onboardingRouteOptions)
	}

	transactionRouteRegistrar := func(router fiber.Router) {
		httpin.RegisterTransactionRoutesToApp(router, auth, transactionHandler, operationHandler, assetRateHandler, balanceHandler, operationRouteHandler, transactionRouteHandler, routeSetup.transactionRouteOptions)
	}

	ledgerRouteRegistrar := httpin.CreateRouteRegistrar(auth, metadataIndexHandler, routeSetup.ledgerRouteOptions)

	// CRM uses the SAME auth client as ledger; only the authz resource namespace
	// differs (plugin-crm, encoded in the route definitions). The CRM-scoped
	// tenant middleware travels via routeSetup.crmRouteOptions. The
	// holder-accounts handler reads ledger accounts through a thin adapter over
	// the query UseCase so the CRM HTTP layer never imports ledger internals.
	holderAccountsHandler := &httpin.HolderAccountsHandler{
		Reader: holderAccountsReaderAdapter{query: queryUseCase},
	}
	crmRouteRegistrar := httpin.CreateCRMRouteRegistrar(auth, crmMgo.holderHandler, crmMgo.instrumentHandler, holderAccountsHandler, crmMgo.encryptionHandler, crmMgo.auditHandler, routeSetup.crmRouteOptions)

	// Fee/billing handlers wire directly to the in-process fee use cases built by
	// initFees (no reconstruction). The fee UseCase satisfies both the package CRUD
	// and fee-estimate handler interfaces.
	feePackageHandler := &httpin.PackageHandler{Service: fees.useCase}
	feeHandler := &httpin.FeeHandler{Service: fees.useCase}
	billingPackageHandler := &httpin.BillingPackageHandler{Service: fees.billingPackageService}
	billingCalculateHandler := &httpin.BillingCalculateHandler{Service: fees.billingCalculateService}

	// Fees uses the SAME auth client as ledger; only the authz resource namespace
	// differs (plugin-fees, encoded in the route definitions). The fees-scoped
	// tenant middleware travels via routeSetup.feesRouteOptions.
	feesRouteRegistrar := httpin.CreateFeesRouteRegistrar(auth, feePackageHandler, feeHandler, billingPackageHandler, billingCalculateHandler, routeSetup.feesRouteOptions)

	logger.Log(context.Background(), libLog.LevelInfo, "Fee routes mounted on unified server",
		libLog.String("default_currency", fees.useCase.DefaultCurrency()),
	)

	// Composition reuses the SAME account-create and instrument-create use-case
	// instances the onboarding and CRM registrars already use — it composes them,
	// it never reimplements them. The cross-store composition tenant middleware
	// travels via routeSetup.compositionRouteOptions so it applies ONLY to the
	// composition route.
	compositionService := composition.NewService(commandUseCase, crmMgo.instrumentHandler.Service)
	compositionHandler := &httpin.CompositionHandler{Service: compositionService}
	compositionRouteRegistrar := httpin.CreateCompositionRouteRegistrar(auth, compositionHandler, routeSetup.compositionRouteOptions)

	logger.Log(context.Background(), libLog.LevelInfo, "Creating unified HTTP server on "+cfg.ServerAddress)

	// === Readyz handler ===

	// metricsFactory derived once with the use cases (nil-safe: checked inside handler).
	readyzHandler, err := buildReadyzHandler(cfg, logger, redisConnection, onbPG, txnPG, onbMgo, txnMgo, crmMgo, feeMgo, rmq, metricsFactory)
	if err != nil {
		doCleanup()

		return nil, fmt.Errorf("failed to build readiness handler: %w", err)
	}

	// === Unified server ===

	unifiedServer := NewUnifiedServer(
		cfg.ServerAddress,
		cfg.Version,
		logger,
		telemetry,
		readyzHandler,
		onboardingRouteRegistrar,
		transactionRouteRegistrar,
		ledgerRouteRegistrar,
		crmRouteRegistrar,
		feesRouteRegistrar,
		compositionRouteRegistrar,
	)

	// === Workers ===

	// RedisQueueConsumer: multi-tenant or single-tenant
	var redisConsumer *RedisQueueConsumer
	if cfg.MultiTenantEnabled && tenantCache != nil {
		redisConsumer = NewRedisQueueConsumerMultiTenant(logger, *transactionHandler, true, tenantCache, txnPG.pgManager)
	} else {
		redisConsumer = NewRedisQueueConsumer(logger, *transactionHandler)
	}

	// The quarantine repository is the durable sink for poison backup records;
	// the metrics factory powers the backup-queue observability gauges/counter.
	redisConsumer.
		WithQuarantineRepository(txnPG.quarantineRepo).
		WithMetricsFactory(metricsFactory)

	// BalanceSyncWorker: multi-tenant or single-tenant
	balanceSyncWorker := initBalanceSyncWorker(internalOpts, cfg, logger, commandUseCase, txnPG.pgManager, tenantServiceName)
	balanceSyncWorker.WithMetricsFactory(metricsFactory)

	// Legacy drainer: drains pre-v3.6.2 ZSET entries (balance-sync key with seconds/microsecond scores).
	// Uses relaxed timing (longer flush timeout, longer idle wait) since it only drains a finite backlog.
	legacyDrainer := NewLegacyBalanceSyncDrainer(logger, commandUseCase, BalanceSyncConfig{
		BatchSize:      cfg.BalanceSyncBatchSize,
		FlushTimeoutMs: 2000,
		PollIntervalMs: 1000,
	})

	logger.Log(context.Background(), libLog.LevelInfo, "Unified ledger component started successfully with single-port mode",
		libLog.String("version", cfg.Version),
		libLog.String("env", cfg.EnvName),
		libLog.String("server_address", cfg.ServerAddress),
	)

	return &Service{
		UnifiedServer:            unifiedServer,
		MultiQueueConsumer:       rmq.multiQueueConsumer,
		MultiTenantConsumer:      rmq.multiTenantConsumer,
		RedisQueueConsumer:       redisConsumer,
		BalanceSyncWorker:        balanceSyncWorker,
		LegacyBalanceSyncDrainer: legacyDrainer,
		EventListener:            eventListener,
		CircuitBreakerManager:    rmq.circuitBreakerManager,
		Logger:                   logger,
		Telemetry:                telemetry,
		metricsFactory:           rmq.metricsFactory,
		StreamingClose:           streamingClose,
		StreamingEnabled:         cfg.StreamingEnabled,
		TracerClose:              tracerClose,
	}, nil
}

// resolveLoggerEnvironment maps an env name to a libZap environment constant.
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

// buildRedisConfig creates a Redis configuration from Config fields.
func buildRedisConfig(cfg *Config, logger libLog.Logger) (libRedis.Config, error) {
	redisAddresses := strings.Split(cfg.RedisHost, ",")

	if len(redisAddresses) == 0 || strings.TrimSpace(redisAddresses[0]) == "" {
		return libRedis.Config{}, fmt.Errorf("redis host is required")
	}

	topology := libRedis.Topology{}
	if cfg.RedisMasterName != "" {
		topology.Sentinel = &libRedis.SentinelTopology{Addresses: redisAddresses, MasterName: cfg.RedisMasterName}
	} else if len(redisAddresses) > 1 {
		topology.Cluster = &libRedis.ClusterTopology{Addresses: redisAddresses}
	} else {
		topology.Standalone = &libRedis.StandaloneTopology{Address: redisAddresses[0]}
	}

	var tlsCfg *libRedis.TLSConfig
	if cfg.RedisTLS {
		tlsCfg = &libRedis.TLSConfig{CACertBase64: cfg.RedisCACert}
	}

	auth := libRedis.Auth{}
	if cfg.RedisUseGCPIAM {
		auth = libRedis.Auth{GCPIAM: &libRedis.GCPIAMAuth{
			CredentialsBase64: cfg.GoogleApplicationCredentials,
			ServiceAccount:    cfg.RedisServiceAccount,
			TokenLifetime:     time.Duration(cfg.RedisTokenLifeTime) * time.Minute,
			RefreshEvery:      time.Duration(cfg.RedisTokenRefreshDuration) * time.Minute,
		}}
	} else if cfg.RedisPassword != "" {
		auth = libRedis.Auth{StaticPassword: &libRedis.StaticPasswordAuth{Password: cfg.RedisPassword}}
	}

	return libRedis.Config{
		Topology: topology,
		TLS:      tlsCfg,
		Auth:     auth,
		Options: libRedis.ConnectionOptions{
			DB:              cfg.RedisDB,
			Protocol:        cfg.RedisProtocol,
			PoolSize:        cfg.RedisPoolSize,
			MinIdleConns:    cfg.RedisMinIdleConns,
			ReadTimeout:     time.Duration(cfg.RedisReadTimeout) * time.Second,
			WriteTimeout:    time.Duration(cfg.RedisWriteTimeout) * time.Second,
			DialTimeout:     time.Duration(cfg.RedisDialTimeout) * time.Second,
			PoolTimeout:     time.Duration(cfg.RedisPoolTimeout) * time.Second,
			MaxRetries:      cfg.RedisMaxRetries,
			MinRetryBackoff: time.Duration(cfg.RedisMinRetryBackoff) * time.Millisecond,
			MaxRetryBackoff: time.Duration(cfg.RedisMaxRetryBackoff) * time.Second,
		},
		Logger: logger,
	}, nil
}

// initRedisConnection creates the shared Redis connection used by both onboarding and transaction.
func initRedisConnection(cfg *Config, logger libLog.Logger) (*libRedis.Client, error) {
	redisConfig, err := buildRedisConfig(cfg, logger)
	if err != nil {
		return nil, err
	}

	redisConnection, err := libRedis.New(context.Background(), redisConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize redis client: %w", err)
	}

	return redisConnection, nil
}

// initBalanceSyncWorker creates the balance sync worker (multi-tenant or single-tenant).
func initBalanceSyncWorker(opts *Options, cfg *Config, logger libLog.Logger, commandUC *command.UseCase, pgManager *tmpostgres.Manager, tenantServiceName string) *BalanceSyncWorker {
	syncCfg := BalanceSyncConfig{
		BatchSize:      cfg.BalanceSyncBatchSize,
		FlushTimeoutMs: cfg.BalanceSyncFlushTimeoutMs,
		PollIntervalMs: cfg.BalanceSyncPollIntervalMs,
	}

	var balanceSyncWorker *BalanceSyncWorker

	if opts != nil && opts.MultiTenantEnabled && opts.TenantCache != nil {
		balanceSyncWorker = NewBalanceSyncWorkerMT(logger, commandUC, syncCfg, true, opts.TenantCache, pgManager, tenantServiceName)
	} else {
		balanceSyncWorker = NewBalanceSyncWorker(logger, commandUC, syncCfg)
	}

	// Log the effective config (after defaults applied by the constructor).
	effectiveCfg := balanceSyncWorker.syncConfig
	logger.Log(context.Background(), libLog.LevelInfo, "BalanceSyncWorker enabled",
		libLog.Int("batch_size", effectiveCfg.BatchSize),
		libLog.Int("flush_timeout_ms", effectiveCfg.FlushTimeoutMs),
		libLog.Int("poll_interval_ms", effectiveCfg.PollIntervalMs),
	)

	return balanceSyncWorker
}

// buildRabbitMQConnectionString constructs an AMQP connection string with optional vhost.
func buildRabbitMQConnectionString(uri, user, pass, host, port, vhost string) string {
	u := &url.URL{
		Scheme: uri,
		User:   url.UserPassword(user, pass),
		Host:   fmt.Sprintf("%s:%s", host, port),
	}
	if vhost != "" {
		u.RawPath = "/" + url.PathEscape(vhost)
		u.Path = "/" + vhost
	} else {
		u.Path = "/"
	}

	return u.String()
}

// initTenantClient creates the multi-tenant client when multi-tenant mode is enabled.
// Returns the client, the resolved tenantServiceName, and error.
// Returns (nil, serviceName, nil) when multi-tenant is disabled.
func initTenantClient(cfg *Config, logger libLog.Logger) (*tmclient.Client, string, error) {
	tenantServiceName := cfg.ApplicationName

	if !cfg.MultiTenantEnabled {
		return nil, tenantServiceName, nil
	}

	tenantManagerURL := strings.TrimSpace(cfg.MultiTenantURL)
	if tenantManagerURL == "" {
		return nil, "", fmt.Errorf("MULTI_TENANT_URL is required when MULTI_TENANT_ENABLED=true")
	}

	tenantServiceName = strings.TrimSpace(cfg.ApplicationName)
	if tenantServiceName == "" {
		return nil, "", fmt.Errorf("APPLICATION_NAME is required when MULTI_TENANT_ENABLED=true")
	}

	tenantManagerAPIKey := strings.TrimSpace(cfg.MultiTenantServiceAPIKey)
	if tenantManagerAPIKey == "" {
		return nil, "", fmt.Errorf("MULTI_TENANT_SERVICE_API_KEY is required when MULTI_TENANT_ENABLED=true")
	}

	// Apply safe defaults for circuit breaker when not configured
	cbThreshold := cfg.MultiTenantCircuitBreakerThreshold
	if cbThreshold <= 0 {
		cbThreshold = 5
	}

	cbTimeoutSec := cfg.MultiTenantCircuitBreakerTimeoutSec
	if cbTimeoutSec <= 0 {
		cbTimeoutSec = 30
	}

	clientOpts := []tmclient.ClientOption{
		tmclient.WithServiceAPIKey(tenantManagerAPIKey),
		tmclient.WithCircuitBreaker(cbThreshold, time.Duration(cbTimeoutSec)*time.Second),
	}

	if cfg.MultiTenantCacheTTLSec >= 0 {
		clientOpts = append(clientOpts, tmclient.WithCacheTTL(time.Duration(cfg.MultiTenantCacheTTLSec)*time.Second))
	}

	if allowInsecureMultiTenantHTTP(tenantManagerURL, cfg.EnvName) {
		clientOpts = append(clientOpts, tmclient.WithAllowInsecureHTTP())
	}

	tenantClient, err := tmclient.NewClient(
		tenantManagerURL,
		logger,
		clientOpts...,
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to initialize tenant manager client: %w", err)
	}

	logger.Log(context.Background(), libLog.LevelInfo, "Multi-tenant mode enabled",
		libLog.String("service", tenantServiceName),
		libLog.Bool("tenant_manager_configured", true),
	)

	return tenantClient, tenantServiceName, nil
}

type unifiedRouteSetup struct {
	onboardingRouteOptions  *midazhttp.ProtectedRouteOptions
	transactionRouteOptions *midazhttp.ProtectedRouteOptions
	ledgerRouteOptions      *midazhttp.ProtectedRouteOptions
	crmRouteOptions         *midazhttp.ProtectedRouteOptions
	feesRouteOptions        *midazhttp.ProtectedRouteOptions
	compositionRouteOptions *midazhttp.ProtectedRouteOptions
}

func buildUnifiedRouteSetup(
	cfg *Config,
	logger libLog.Logger,
	onboardingPGManager *tmpostgres.Manager,
	transactionPGManager *tmpostgres.Manager,
	onboardingMongoManager *tmmongo.Manager,
	transactionMongoManager *tmmongo.Manager,
	crmMongoManager *tmmongo.Manager,
	feesMongoManager *tmmongo.Manager,
	tenantCache *tenantcache.TenantCache,
	tenantLoader *tenantcache.TenantLoader,
) (*unifiedRouteSetup, error) {
	setup := &unifiedRouteSetup{}
	if !cfg.MultiTenantEnabled {
		return setup, nil
	}

	if onboardingPGManager == nil {
		return nil, fmt.Errorf("onboarding multi-tenant PostgreSQL manager not available")
	}

	if onboardingMongoManager == nil {
		return nil, fmt.Errorf("onboarding multi-tenant MongoDB manager not available")
	}

	if transactionPGManager == nil {
		return nil, fmt.Errorf("transaction multi-tenant PostgreSQL manager not available")
	}

	if transactionMongoManager == nil {
		return nil, fmt.Errorf("transaction multi-tenant MongoDB manager not available")
	}

	if crmMongoManager == nil {
		return nil, fmt.Errorf("crm multi-tenant MongoDB manager not available")
	}

	if feesMongoManager == nil {
		return nil, fmt.Errorf("fees multi-tenant MongoDB manager not available")
	}

	// Build the onboarding+transaction tenant middleware. CRM is DELIBERATELY
	// excluded here: it gets its own instance below.
	tenantMiddleware := tmmiddleware.NewTenantMiddleware(
		tmmiddleware.WithPG(onboardingPGManager, constant.ModuleOnboarding),
		tmmiddleware.WithPG(transactionPGManager, constant.ModuleTransaction),
		tmmiddleware.WithMB(onboardingMongoManager, constant.ModuleOnboarding),
		tmmiddleware.WithMB(transactionMongoManager, constant.ModuleTransaction),
		tmmiddleware.WithTenantCache(tenantCache),
		tmmiddleware.WithTenantLoader(tenantLoader),
	)

	// CRM tenant middleware is a SEPARATE instance carrying ONLY the crm-api
	// Mongo manager. This is the isolation-critical step: the CRM
	// WithTenantDB MUST be attached only to CRM routes via crmRouteOptions
	// below. Mounting it on the onboarding/transaction middleware (or globally
	// via f.Use) would overwrite the tenant Mongo that ledger handlers resolve,
	// leaking one tenant's CRM DB into a concurrent ledger request.
	//
	// WithMB is called WITHOUT a module name (single-manager mode) on purpose:
	// the CRM holder/alias repos read tmcore.GetMBContext(ctx) on the GENERIC
	// key (they predate module-keyed resolution). A module-keyed WithMB would
	// write the crm-api key while the repos read the generic key, so MT CRM
	// requests would fail DB resolution. Because this middleware instance only
	// runs on CRM routes, writing the generic key here cannot collide with the
	// module-keyed onboarding/transaction injection on ledger routes — isolation
	// is preserved by route scoping. (The manager itself still carries
	// WithModule(ModuleCRM) for tenant-manager DB resolution; that is a separate
	// concern from the request-context key.) The same tenantCache/tenantLoader
	// are reused (no second cache/loader).
	crmTenantMiddleware := tmmiddleware.NewTenantMiddleware(
		tmmiddleware.WithMB(crmMongoManager),
		tmmiddleware.WithTenantCache(tenantCache),
		tmmiddleware.WithTenantLoader(tenantLoader),
	)

	// Fees tenant middleware is its own SEPARATE instance carrying ONLY the
	// plugin-fees Mongo manager, for the same isolation reason as CRM: mounting
	// the fee WithTenantDB on the onboarding/transaction middleware (or globally)
	// would overwrite the tenant Mongo that ledger handlers resolve. It is
	// attached only to fee routes via feesRouteOptions below.
	//
	// WithMB is called WITHOUT a module name (single-manager mode) on purpose:
	// the fee pack/billing_package repos read tmcore.GetMBContext(ctx) on the
	// GENERIC key (the standalone fees service ran single-module — it registered
	// its manager under the SERVICE name with no WithModule). A module-keyed
	// WithMB would write the plugin-fees key while the repos read the generic
	// key, so MT fee requests would fail DB resolution. Route scoping keeps the
	// generic-key write from colliding with the module-keyed onboarding/
	// transaction injection on ledger routes. (The manager itself still carries
	// WithModule(ModuleFees) for tenant-manager DB resolution; that is a separate
	// concern from the request-context key.) Same cache/loader are reused.
	feesTenantMiddleware := tmmiddleware.NewTenantMiddleware(
		tmmiddleware.WithMB(feesMongoManager),
		tmmiddleware.WithTenantCache(tenantCache),
		tmmiddleware.WithTenantLoader(tenantLoader),
	)

	// Composition tenant middleware is its own SEPARATE instance spanning BOTH
	// stores the holder-account composition touches: the onboarding PostgreSQL
	// (module-keyed) for the account write AND the CRM Mongo (generic key) for
	// the instrument write. It is attached ONLY to composition routes via
	// compositionRouteOptions below — never global, never on ledger routes.
	// Mounting it globally (or on the onboarding/transaction middleware) would
	// bleed the generic CRM Mongo key onto ledger routes and overwrite the
	// tenant Mongo that ledger handlers resolve, leaking one tenant's CRM DB
	// into a concurrent ledger request — the precise cross-store leak this
	// instance exists to prevent.
	//
	// WithPG carries constant.ModuleOnboarding because composition writes the
	// account through the onboarding account repo, which resolves the
	// module-keyed PG context. WithMB is called WITHOUT a module name
	// (single-manager mode), matching the CRM block above: the CRM instrument
	// repo reads tmcore.GetMBContext(ctx) on the GENERIC key. Route scoping
	// keeps that generic-key write from colliding with the module-keyed
	// onboarding/transaction injection on ledger routes. The transaction PG
	// manager is DELIBERATELY excluded: composition writes the onboarding
	// account and the CRM instrument only and never touches the transaction PG.
	// Same tenantCache/tenantLoader are reused (no second cache/loader).
	compositionTenantMiddleware := tmmiddleware.NewTenantMiddleware(
		tmmiddleware.WithPG(onboardingPGManager, constant.ModuleOnboarding),
		tmmiddleware.WithMB(crmMongoManager),
		tmmiddleware.WithTenantCache(tenantCache),
		tmmiddleware.WithTenantLoader(tenantLoader),
	)

	logger.Log(context.Background(), libLog.LevelInfo, "Tenant middleware configured",
		libLog.String("modules", "onboarding,transaction,crm-api,plugin-fees"),
	)

	authAssertion := midazhttp.MarkTrustedAuthAssertion()

	setup.onboardingRouteOptions = &midazhttp.ProtectedRouteOptions{
		PostAuthMiddlewares: []fiber.Handler{authAssertion, tenantMiddleware.WithTenantDB},
	}

	setup.transactionRouteOptions = &midazhttp.ProtectedRouteOptions{
		PostAuthMiddlewares: []fiber.Handler{authAssertion, tenantMiddleware.WithTenantDB},
	}

	setup.ledgerRouteOptions = &midazhttp.ProtectedRouteOptions{
		PostAuthMiddlewares: []fiber.Handler{authAssertion},
	}

	// CRM routes get the CRM-only tenant middleware instance.
	setup.crmRouteOptions = &midazhttp.ProtectedRouteOptions{
		PostAuthMiddlewares: []fiber.Handler{authAssertion, crmTenantMiddleware.WithTenantDB},
	}

	// Fee routes get the fees-only tenant middleware instance. The next chunk
	// (P4-T10) consumes feesRouteOptions when it mounts the fee RouteRegistrar.
	setup.feesRouteOptions = &midazhttp.ProtectedRouteOptions{
		PostAuthMiddlewares: []fiber.Handler{authAssertion, feesTenantMiddleware.WithTenantDB},
	}

	// Composition routes get the cross-store composition tenant middleware
	// instance, scoping the onboarding-PG + CRM-Mongo injection to composition
	// routes only.
	setup.compositionRouteOptions = &midazhttp.ProtectedRouteOptions{
		PostAuthMiddlewares: []fiber.Handler{authAssertion, compositionTenantMiddleware.WithTenantDB},
	}

	return setup, nil
}

// midazErrorMapper converts tenant-manager errors into Midaz-specific HTTP responses.
// It uses the standard midazhttp response helpers to ensure a consistent error format
// across all Midaz endpoints (code/title/message JSON envelope).
//
//nolint:unused // Will be wired into the multi-tenant middleware error handler.
func midazErrorMapper(c *fiber.Ctx, err error, tenantID string) error {
	if err == nil {
		return nil
	}

	// Tenant suspended or purged → 403 (same semantics as tenant-manager /connections)
	var suspErr *tmcore.TenantSuspendedError
	if errors.As(err, &suspErr) {
		return midazhttp.Forbidden(c,
			constant.ErrTenantServiceSuspended.Error(),
			"Service Suspended",
			fmt.Sprintf("service is %s for tenant %s", suspErr.Status, tenantID),
		)
	}

	// Tenant not found → 404
	if errors.Is(err, tmcore.ErrTenantNotFound) {
		return midazhttp.NotFound(c,
			constant.ErrTenantNotFound.Error(),
			"Tenant Not Found",
			fmt.Sprintf("tenant not found: %s", tenantID),
		)
	}

	// Tenant not provisioned → 422
	if tmcore.IsTenantNotProvisionedError(err) {
		return midazhttp.UnprocessableEntity(c,
			constant.ErrTenantNotProvisioned.Error(),
			"Tenant Not Provisioned",
			"Database schema not initialized for this tenant. Contact your administrator.",
		)
	}

	// Unknown error → 503
	return midazhttp.ServiceUnavailable(c,
		constant.ErrTenantServiceUnavailable.Error(),
		"Tenant Service Unavailable",
		fmt.Sprintf("failed to resolve tenant %s: %s", tenantID, err.Error()),
	)
}

// applyConfigDefaults sets sensible defaults for Config fields that remain at their
// zero value after SetConfigFromEnvVars. This replaces the inert `default` struct tags
// which are not interpreted by SetConfigFromEnvVars.
func applyConfigDefaults(cfg *Config) {
	intDefault := func(field *int, fallback int) {
		if *field == 0 {
			*field = fallback
		}
	}

	intDefault(&cfg.RedisProtocol, 3)
	intDefault(&cfg.RedisTokenLifeTime, 60)
	intDefault(&cfg.RedisTokenRefreshDuration, 45)
	intDefault(&cfg.RedisPoolSize, 10)
	intDefault(&cfg.RedisReadTimeout, 3)
	intDefault(&cfg.RedisWriteTimeout, 3)
	intDefault(&cfg.RedisDialTimeout, 5)
	intDefault(&cfg.RedisPoolTimeout, 2)
	intDefault(&cfg.RedisMaxRetries, 3)
	intDefault(&cfg.RedisMinRetryBackoff, 8)
	intDefault(&cfg.RedisMaxRetryBackoff, 1)

	// Bulk Recorder defaults
	// BulkRecorderEnabled defaults to true when the env var is not set or empty.
	// This treats both unset and empty string as "use default" for safer behavior.
	// Explicit "true"/"false" values are parsed by SetConfigFromEnvVars before this runs.
	if os.Getenv("BULK_RECORDER_ENABLED") == "" {
		cfg.BulkRecorderEnabled = true
	}

	// BulkRecorderFlushTimeoutMs defaults to 100ms.
	intDefault(&cfg.BulkRecorderFlushTimeoutMs, 100)

	// BulkRecorderMaxRowsPerInsert defaults to 1000 (safe under PostgreSQL 65,535 param limit).
	intDefault(&cfg.BulkRecorderMaxRowsPerInsert, 1000)

	// BulkRecorderSize: derive from workers × prefetch if not explicitly set.
	// This ensures bulk size matches RabbitMQ prefetch for optimal performance.
	if cfg.BulkRecorderSize == 0 {
		workers := cfg.RabbitMQNumbersOfWorkers
		if workers == 0 {
			workers = 5 // default workers
		}

		prefetch := cfg.RabbitMQNumbersOfPrefetch
		if prefetch == 0 {
			prefetch = 10 // default prefetch
		}

		cfg.BulkRecorderSize = workers * prefetch
	}

	// Balance Sync Worker defaults (dual-trigger)
	intDefault(&cfg.BalanceSyncBatchSize, 50)
	intDefault(&cfg.BalanceSyncFlushTimeoutMs, 500)
	intDefault(&cfg.BalanceSyncPollIntervalMs, 50)

	// Fee engine default currency. The standalone fees service shipped with no
	// hard default (DEFAULT_CURRENCY was required env); the unified binary must
	// not fail fee construction when the var is unset, so fall back to "USD".
	if strings.TrimSpace(cfg.FeesDefaultCurrency) == "" {
		cfg.FeesDefaultCurrency = "USD"
	}
}

// buildTracerReserver constructs the tracer reservation HTTP client when the
// integration is configured (TRACER_BASE_URL set), or returns a nil
// TracerReserver when it is not. Returning the interface type (rather than the
// concrete *tracerclient.TracerClient) keeps the disabled case a genuine nil
// interface so the call-site nil guard short-circuits correctly.
//
// This is pure DI: it wires the transport, not behavior. The per-ledger
// advisory/enforce gate and the fail-posture branch live at the reserve anchor.
func buildTracerReserver(cfg *Config, logger libLog.Logger) (httpin.TracerReserver, error) {
	baseURL := strings.TrimSpace(cfg.TracerBaseURL)
	if baseURL == "" {
		logger.Log(context.Background(), libLog.LevelInfo, "Tracer reservation integration disabled (TRACER_BASE_URL unset)")

		return nil, nil
	}

	// Fail-fast guard: identity on the reservation seam is mutual TLS (the
	// verified peer IS the credential — no shared secret). The discriminator is
	// the transport's security, NOT tenancy: with the integration on and
	// TRACER_TLS_MODE=mtls, the cert/key/CA material is mandatory, so a
	// misconfigured deploy fails at boot rather than dialing an unverified seam.
	// "mesh" trusts a local sidecar to originate mTLS (no app cert material).
	// buildSeamClientTLSConfig names the failing knob; in mesh/empty mode it
	// returns a nil config and both transports dial plaintext.
	tlsConfig, err := buildSeamClientTLSConfig(cfg, seamServerName(baseURL))
	if err != nil {
		return nil, err
	}

	transport := strings.ToLower(strings.TrimSpace(cfg.TracerTransport))
	if transport == "" {
		transport = tracerTransportGRPC
	}

	switch transport {
	case tracerTransportGRPC:
		return buildTracerGRPCReserver(cfg, baseURL, tlsConfig, logger)
	case tracerTransportREST:
		return buildTracerRESTReserver(cfg, baseURL, tlsConfig, logger)
	default:
		return nil, fmt.Errorf("invalid TRACER_TRANSPORT %q: expected %q or %q", cfg.TracerTransport, tracerTransportGRPC, tracerTransportREST)
	}
}

// Tracer reservation transports selected by TRACER_TRANSPORT.
const (
	tracerTransportGRPC = "grpc"
	tracerTransportREST = "rest"
)

// buildTracerRESTReserver wires the HTTP reservation client. When tlsConfig is
// non-nil (TRACER_TLS_MODE=mtls) it is applied to the client's transport so the
// REST seam presents the ledger's client cert and verifies the tracer's server
// cert; a nil config (mesh/empty mode) leaves the default plaintext transport.
func buildTracerRESTReserver(cfg *Config, baseURL string, tlsConfig *tls.Config, logger libLog.Logger) (httpin.TracerReserver, error) {
	logger.Log(context.Background(), libLog.LevelInfo, "Tracer reservation transport selected",
		libLog.String("transport", tracerTransportREST))

	opts := []tracerclient.TracerClientOption{}
	if cfg.TracerTimeoutMs > 0 {
		opts = append(opts, tracerclient.WithOperationTimeout(time.Duration(cfg.TracerTimeoutMs)*time.Millisecond))
	}

	if tlsConfig != nil {
		opts = append(opts, tracerclient.WithTLSConfig(tlsConfig))
	}

	client, err := tracerclient.NewTracerClient(baseURL, opts...)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// buildTracerGRPCReserver wires the gRPC reservation client. When tlsConfig is
// non-nil (TRACER_TLS_MODE=mtls) it is injected as transport credentials
// (credentials.NewTLS) so the gRPC seam presents the ledger's client cert and
// verifies the tracer's server cert; a nil config (mesh/empty mode) leaves the
// client's default insecure transport for a sidecar to secure. The target is the
// same TRACER_BASE_URL value, stripped of any scheme so grpc.NewClient receives
// a host:port authority.
func buildTracerGRPCReserver(cfg *Config, baseURL string, tlsConfig *tls.Config, logger libLog.Logger) (httpin.TracerReserver, error) {
	logger.Log(context.Background(), libLog.LevelInfo, "Tracer reservation transport selected",
		libLog.String("transport", tracerTransportGRPC))

	target := stripURLScheme(baseURL)

	opts := []tracerclient.TracerGRPCClientOption{}
	if cfg.TracerTimeoutMs > 0 {
		opts = append(opts, tracerclient.WithGRPCOperationTimeout(time.Duration(cfg.TracerTimeoutMs)*time.Millisecond))
	}

	if tlsConfig != nil {
		opts = append(opts, tracerclient.WithGRPCDialOptions(grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))))
	}

	client, err := tracerclient.NewTracerGRPCClient(target, opts...)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// stripURLScheme removes an http:// or https:// scheme from a tracer endpoint so
// the same TRACER_BASE_URL value feeds both the REST client (full URL) and the
// gRPC client (host:port authority).
func stripURLScheme(endpoint string) string {
	if i := strings.Index(endpoint, "://"); i >= 0 {
		return endpoint[i+len("://"):]
	}

	return endpoint
}

// seamServerName extracts the host (no port) the tracer's server certificate is
// verified against in mtls mode. It tolerates both a full URL (https://host:port)
// and a bare host:port authority; a parse failure falls back to the raw value so
// the TLS layer surfaces the mismatch rather than this helper silently dropping
// it.
func seamServerName(endpoint string) string {
	hostPort := stripURLScheme(endpoint)

	if host, _, err := net.SplitHostPort(hostPort); err == nil {
		return host
	}

	return hostPort
}
