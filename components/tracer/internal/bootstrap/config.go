// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	authMiddleware "github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	tmpostgres "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/postgres"
	libLog "github.com/LerianStudio/lib-observability/log"
	libMetrics "github.com/LerianStudio/lib-observability/metrics"
	libRuntime "github.com/LerianStudio/lib-observability/runtime"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	libZap "github.com/LerianStudio/lib-observability/zap"
	"google.golang.org/grpc"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	grpcin "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in"
	httpMiddleware "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/observability"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/metrics"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/resilience"
)

// Config is the top level configuration struct for the entire application.
type Config struct {
	ServerAddress string `env:"SERVER_ADDRESS"`
	// TracerGRPCPort is the listen address for the reservation gRPC seam (e.g.
	// ":4021"). When empty (the default) the gRPC server is NOT started — the
	// transport is opt-in during the Phase-1 rollout. Transport security follows
	// TRACER_TLS_MODE: in "mtls" the gRPC server requires+verifies a client cert.
	TracerGRPCPort string `env:"TRACER_GRPC_PORT"`
	// TracerTLSMode selects how the reservation seam is secured. "mtls"
	// (Epic 1.3) makes the app load its own cert/key/CA and require+verify a
	// client cert on BOTH the gRPC and the Fiber listeners — the verified mTLS
	// peer is the seam credential (no shared secret). "mesh" lets a service-mesh
	// sidecar (Istio/Linkerd) terminate mTLS, so the app listens plaintext and
	// skips its own TLS. Empty/unset behaves like "mesh" (plaintext) so the
	// Phase-1 toggle default and local dev keep working without cert material.
	TracerTLSMode string `env:"TRACER_TLS_MODE"`
	// TracerTLSCertFile / TracerTLSKeyFile are the PEM paths for the tracer's
	// OWN server certificate and private key, presented on both transports in
	// "mtls" mode. Required (non-empty) when TracerTLSMode=mtls.
	TracerTLSCertFile string `env:"TRACER_TLS_CERT_FILE"`
	TracerTLSKeyFile  string `env:"TRACER_TLS_KEY_FILE"`
	// TracerTLSClientCAFile is the PEM bundle of CA certificate(s) used to
	// verify client certificates the ledger presents. Required (non-empty) when
	// TracerTLSMode=mtls — without it the server cannot enforce
	// RequireAndVerifyClientCert.
	TracerTLSClientCAFile string `env:"TRACER_TLS_CLIENT_CA_FILE"`

	LogLevel                string `env:"LOG_LEVEL"`
	OtelServiceName         string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName         string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion      string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv       string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry         bool   `env:"ENABLE_TELEMETRY"`
	// DeploymentMode is echoed in /readyz responses and (Gate 4) gates
	// SaaS-mode TLS validation at bootstrap. Valid values: "saas", "byoc",
	// "local". Default "local" — applied via ApplyDeploymentDefaults
	// because lib-commons v4 does not honor envDefault tags.
	DeploymentMode string `env:"DEPLOYMENT_MODE"`
	DBHost         string `env:"DB_HOST"`
	DBUser         string `env:"DB_USER"`
	DBPassword     string `env:"DB_PASSWORD"`
	DBName         string `env:"DB_NAME"`
	DBPort         string `env:"DB_PORT"`
	DBSSLMode      string `env:"DB_SSL_MODE"`
	MigrationPath  string `env:"MIGRATIONS_PATH"`
	// DBMaxOpenConns / DBMaxIdleConns bound the per-pool sql.DB sizes for the
	// primary AND replica pools opened by libPostgres.New. When 0 (default) the
	// lib-commons defaults apply (25 max-open, 10 max-idle each → up to 50
	// connections per service instance). These knobs exist primarily for the
	// integration test suite, which restarts the service repeatedly via
	// RestartServerWithConfig and would otherwise exhaust the testcontainer's
	// max_connections=100 (default 50/restart × N restarts ≫ 100). Production
	// should leave both unset to preserve the lib-commons defaults.
	DBMaxOpenConns int `env:"DB_MAX_OPEN_CONNS"`
	DBMaxIdleConns int `env:"DB_MAX_IDLE_CONNS"`

	// Authentication
	APIKey               string `env:"API_KEY"`
	APIKeyEnabled        bool   `env:"API_KEY_ENABLED"`
	APIKeyOnlyValidation bool   `env:"API_KEY_ENABLED_ONLY_VALIDATION"`
	// APIKeyLabel is recorded as the audit actor ID for requests authenticated
	// via API key. Defaults to "tracer-default" when unset, so audit rows always
	// carry a non-empty actor identifier instead of falling back to "svc_tracer".
	APIKeyLabel       string `env:"API_KEY_LABEL"`
	PluginAuthAddress string `env:"PLUGIN_AUTH_ADDRESS"`
	PluginAuthEnabled bool   `env:"PLUGIN_AUTH_ENABLED"`

	// Application identity
	// ApplicationName is the module identifier used when registering with the
	// multi-tenant Tenant Manager. Default: "tracer" (applied in ApplyMultiTenantDefaults).
	ApplicationName string `env:"APPLICATION_NAME"`

	// Multi-Tenant
	// All MULTI_TENANT_* defaults are applied by ApplyMultiTenantDefaults after
	// libCommons.SetConfigFromEnvVars runs. lib-commons v4 does not honor
	// `envDefault` tags; defaults are applied in code to preserve a single
	// source of truth. Defaults: MULTI_TENANT_ENABLED=false,
	// MULTI_TENANT_REDIS_PORT=6379, MULTI_TENANT_REDIS_TLS=false,
	// MULTI_TENANT_MAX_TENANT_POOLS=100, MULTI_TENANT_IDLE_TIMEOUT_SEC=300,
	// MULTI_TENANT_TIMEOUT=30, MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD=5,
	// MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC=30, MULTI_TENANT_CACHE_TTL_SEC=120,
	// MULTI_TENANT_CONNECTIONS_CHECK_INTERVAL_SEC=30.
	MultiTenantEnabled        bool   `env:"MULTI_TENANT_ENABLED"`
	MultiTenantURL            string `env:"MULTI_TENANT_URL"`
	MultiTenantRedisHost      string `env:"MULTI_TENANT_REDIS_HOST"`
	MultiTenantRedisPort      string `env:"MULTI_TENANT_REDIS_PORT"`
	MultiTenantRedisPassword  string `env:"MULTI_TENANT_REDIS_PASSWORD"`
	MultiTenantRedisTLS       bool   `env:"MULTI_TENANT_REDIS_TLS"`
	MultiTenantMaxTenantPools int    `env:"MULTI_TENANT_MAX_TENANT_POOLS"`
	// MultiTenantMaxOpenConnsPerTenant / MultiTenantMaxIdleConnsPerTenant cap the
	// per-tenant primary+replica pool sizes inside tmpostgres.Manager. When 0
	// (default) lib-commons applies its fallback (25 max-open, 5 max-idle per
	// tenant). Tracer's MT integration tests reboot the service repeatedly with
	// MAX_TENANT_POOLS=100, so without these knobs each reboot can lay claim to
	// hundreds of connections against the testcontainer's max_connections=100.
	// Production should leave both unset.
	MultiTenantMaxOpenConnsPerTenant       int    `env:"MULTI_TENANT_MAX_OPEN_CONNS_PER_TENANT"`
	MultiTenantMaxIdleConnsPerTenant       int    `env:"MULTI_TENANT_MAX_IDLE_CONNS_PER_TENANT"`
	MultiTenantIdleTimeoutSec              int    `env:"MULTI_TENANT_IDLE_TIMEOUT_SEC"`
	MultiTenantTimeout                     int    `env:"MULTI_TENANT_TIMEOUT"`
	MultiTenantCircuitBreakerThreshold     int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD"`
	MultiTenantCircuitBreakerTimeoutSec    int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC"`
	MultiTenantServiceAPIKey               string `env:"MULTI_TENANT_SERVICE_API_KEY"`
	MultiTenantCacheTTLSec                 int    `env:"MULTI_TENANT_CACHE_TTL_SEC"`
	MultiTenantConnectionsCheckIntervalSec int    `env:"MULTI_TENANT_CONNECTIONS_CHECK_INTERVAL_SEC"`
	// MultiTenantAllowInsecureHTTP must be explicitly true for the service to
	// accept an http:// MULTI_TENANT_URL. lib-commons v4 rejects cleartext HTTP
	// by default; without this flag the service fails fast rather than silently
	// sending the service API key in plaintext. Never set in production.
	MultiTenantAllowInsecureHTTP bool `env:"MULTI_TENANT_ALLOW_INSECURE_HTTP"`

	// CORS
	CORSAllowedOrigins string `env:"CORS_ALLOWED_ORIGINS"`

	// TrustedProxyCIDRs is a comma-separated list of CIDR ranges identifying
	// the reverse proxies / load balancers in front of tracer. It governs how
	// the audit client IP is derived from X-Forwarded-For (see
	// middleware.ClientIPMiddlewareWithTrustedProxies). When empty (the default)
	// XFF is ignored entirely and the socket peer IP is recorded, so a client
	// cannot forge the audit IP. The list is parsed ONCE at boot by
	// parseTrustedProxyCIDRs; a malformed entry fails boot.
	TrustedProxyCIDRs string `env:"TRUSTED_PROXY_CIDRS"`

	// CEL Expression Engine
	CELCostLimit string `env:"CEL_COST_LIMIT"`

	// Rule Evaluation Feature Flags
	DefaultDecisionWhenNoMatch string `env:"DEFAULT_DECISION_WHEN_NO_MATCH"`
	MaxRulesPerRequest         string `env:"MAX_RULES_PER_REQUEST"`

	// Usage Counter Cleanup Worker
	// CleanupWorkerEnabled enables/disables the background cleanup worker (default: false)
	// Set CLEANUP_WORKER_ENABLED=true in environment to enable the worker
	CleanupWorkerEnabled bool `env:"CLEANUP_WORKER_ENABLED"`
	// CleanupIntervalHours is the interval between cleanup runs in hours (default: 24)
	CleanupIntervalHours string `env:"CLEANUP_INTERVAL_HOURS"`

	// Reservation Reaper Worker
	// ReservationReaperEnabled enables/disables the background reservation reaper (default: false).
	// Set RESERVATION_REAPER_ENABLED=true to release expired two-phase reservations.
	ReservationReaperEnabled bool `env:"RESERVATION_REAPER_ENABLED"`
	// ReservationReaperIntervalSeconds is the sub-minute interval between reaper sweeps in seconds (default: 30).
	ReservationReaperIntervalSeconds string `env:"RESERVATION_REAPER_INTERVAL_SECONDS"`
	// ReservationLongLivedTTLHours is the lifetime granted to a PENDING-transaction
	// reservation (the longLived reserve hint, R18), in hours (default: 720 = 30 days).
	// Direct-transaction reservations use a fixed short TTL and ignore this knob.
	ReservationLongLivedTTLHours string `env:"RESERVATION_LONG_LIVED_TTL_HOURS"`

	// Rule Sync Worker
	// RuleSyncPollIntervalSeconds is how often the worker polls for rule changes (default: 10)
	RuleSyncPollIntervalSeconds string `env:"RULE_SYNC_POLL_INTERVAL_SECONDS"`
	// RuleSyncStalenessThresholdSeconds is when the cache is considered stale for health checks (default: 50)
	RuleSyncStalenessThresholdSeconds string `env:"RULE_SYNC_STALENESS_THRESHOLD_SECONDS"`
	// RuleSyncOverlapBufferSeconds is the overlap buffer for delta queries in seconds (default: 2)
	RuleSyncOverlapBufferSeconds string `env:"RULE_SYNC_OVERLAP_BUFFER_SECONDS"`

	// ReadyzDrainGraceSeconds controls how long /readyz returns 503 after
	// SIGTERM before the HTTP server begins shutting down. Sized for K8s
	// readinessProbe defaults (periodSeconds=5 × failureThreshold=2 = 10s)
	// plus a small buffer. Default 12s when unset or non-positive, applied
	// in drainGracePeriod (lib-commons v4 SetConfigFromEnvVars does not
	// honour envDefault tags).
	ReadyzDrainGraceSeconds int `env:"READYZ_DRAIN_GRACE_SECONDS"`

	// ReadyzCacheStalenessThresholdSeconds is the operator-tunable rule_cache
	// staleness threshold used by the /readyz handler (H3). When the cache's
	// staleness exceeds this, /readyz reports the rule_cache check as
	// "degraded" → aggregate "unhealthy" → 503. Default 300s (5 min) so a
	// brief Postgres failover does not trigger a fleet-wide pod restart for
	// a fraud-prevention service. A non-positive value falls back to the
	// default; bootstrap applies the value via
	// HealthChecker.SetCacheStalenessThreshold.
	ReadyzCacheStalenessThresholdSeconds int `env:"READYZ_CACHE_STALENESS_THRESHOLD_SECONDS"`
}

// minAPIKeyLength is the minimum recommended length for API keys.
const minAPIKeyLength = 32

// celCompilerAdapter wraps cel.Adapter to satisfy command.ExpressionCompiler interface.
type celCompilerAdapter struct {
	adapter *cel.Adapter
}

// Compile wraps cel.Adapter.Compile to return any instead of *cel.CompiledProgram.
func (c *celCompilerAdapter) Compile(ctx context.Context, expression string) (any, error) {
	return c.adapter.Compile(ctx, expression)
}

// parseCELCostLimit parses the CEL cost limit from string to uint64.
// Returns default value (10000) if empty.
// Returns error if value is invalid or zero.
func parseCELCostLimit(s string) (uint64, error) {
	const defaultValue uint64 = 10000

	if s == "" {
		return defaultValue, nil
	}

	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid CEL_COST_LIMIT value '%s': %w", s, err)
	}

	if v == 0 {
		return 0, fmt.Errorf("CEL_COST_LIMIT must be positive, got 0")
	}

	return v, nil
}

// parseDefaultDecision parses the default decision from string.
// Returns model.DecisionAllow if empty or "ALLOW".
// Returns model.DecisionDeny if "DENY".
// Returns error if value is invalid.
//
// NOTE: REVIEW is intentionally excluded as a default decision because
// defaulting to manual review would overwhelm human reviewers when no
// rules match, which is the common case for legitimate transactions.
func parseDefaultDecision(s string) (model.Decision, error) {
	switch s {
	case "", "ALLOW":
		return model.DecisionAllow, nil
	case "DENY":
		return model.DecisionDeny, nil
	default:
		return "", fmt.Errorf("invalid DEFAULT_DECISION_WHEN_NO_MATCH value '%s': must be ALLOW or DENY", s)
	}
}

// parseMaxRulesPerRequest parses the max rules per request from string to int.
// Returns default value (1000) if empty.
// Returns error if value is invalid, non-positive, or exceeds maximum.
func parseMaxRulesPerRequest(s string) (int, error) {
	const defaultValue = 1000

	// maxAllowed limits MaxRulesPerRequest to prevent resource exhaustion.
	// 100,000 rules is a reasonable upper bound that balances flexibility
	// with DoS protection. At this limit, memory and CPU usage remain bounded.
	const maxAllowed = 100000

	if s == "" {
		return defaultValue, nil
	}

	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid MAX_RULES_PER_REQUEST value '%s': %w", s, err)
	}

	if v <= 0 {
		return 0, fmt.Errorf("MAX_RULES_PER_REQUEST must be positive, got %d", v)
	}

	if v > maxAllowed {
		return 0, fmt.Errorf("MAX_RULES_PER_REQUEST exceeds maximum allowed (%d), got %d", maxAllowed, v)
	}

	return v, nil
}

// parseCleanupIntervalHours parses the cleanup interval from string to time.Duration.
// Returns default value (24 hours) if empty.
// Returns error if value is invalid, non-positive, or exceeds maximum.
func parseCleanupIntervalHours(s string) (time.Duration, error) {
	const defaultHours = 24

	// maxAllowedHours limits cleanup interval to 1 year (8760 hours).
	// This prevents misconfiguration that could effectively disable cleanup.
	const maxAllowedHours = 8760

	if s == "" {
		return time.Duration(defaultHours) * time.Hour, nil
	}

	hours, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid CLEANUP_INTERVAL_HOURS value '%s': %w", s, err)
	}

	if hours <= 0 {
		return 0, fmt.Errorf("CLEANUP_INTERVAL_HOURS must be positive, got %d", hours)
	}

	if hours > maxAllowedHours {
		return 0, fmt.Errorf("CLEANUP_INTERVAL_HOURS exceeds maximum allowed (%d hours = 1 year), got %d", maxAllowedHours, hours)
	}

	return time.Duration(hours) * time.Hour, nil
}

// parseReservationReaperIntervalSeconds parses the reaper sweep interval from
// string to time.Duration. Returns the 30s default when empty. The reaper is a
// sub-minute worker, so the unit is seconds (NOT hours like the cleanup worker).
// Returns an error if the value is invalid, non-positive, or exceeds 1 hour —
// a reaper interval longer than an hour defeats the point of a TTL backstop.
func parseReservationReaperIntervalSeconds(s string) (time.Duration, error) {
	// maxAllowedSeconds caps the reaper interval at 1 hour. Beyond that the TTL
	// backstop is effectively disabled; the operator should use
	// RESERVATION_REAPER_ENABLED=false to turn it off explicitly instead.
	const maxAllowedSeconds = 3600

	if s == "" {
		return workers.DefaultReservationReaperInterval, nil
	}

	seconds, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid RESERVATION_REAPER_INTERVAL_SECONDS value '%s': %w", s, err)
	}

	if seconds <= 0 {
		return 0, fmt.Errorf("RESERVATION_REAPER_INTERVAL_SECONDS must be positive, got %d", seconds)
	}

	if seconds > maxAllowedSeconds {
		return 0, fmt.Errorf("RESERVATION_REAPER_INTERVAL_SECONDS exceeds maximum allowed (%d seconds = 1 hour), got %d", maxAllowedSeconds, seconds)
	}

	return time.Duration(seconds) * time.Second, nil
}

// parseReservationLongLivedTTLHours parses the long-lived reservation TTL from
// string to time.Duration. Returns 0 when empty so the service applies its own
// default (defaultLongLivedReservationTTL, 30 days). The unit is hours because a
// long-lived pending reservation spans days, not seconds (unlike the reaper
// interval). Returns an error if the value is invalid, non-positive, or exceeds
// 1 year — beyond that the reaper effectively never converges an abandoned pending.
func parseReservationLongLivedTTLHours(s string) (time.Duration, error) {
	// maxAllowedHours caps the long-lived TTL at 1 year (8760 hours), mirroring the
	// cleanup-interval ceiling: past it the reaper would hold an abandoned pending's
	// capacity for an operationally unbounded time.
	const maxAllowedHours = 8760

	if s == "" {
		return 0, nil
	}

	hours, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid RESERVATION_LONG_LIVED_TTL_HOURS value '%s': %w", s, err)
	}

	if hours <= 0 {
		return 0, fmt.Errorf("RESERVATION_LONG_LIVED_TTL_HOURS must be positive, got %d", hours)
	}

	if hours > maxAllowedHours {
		return 0, fmt.Errorf("RESERVATION_LONG_LIVED_TTL_HOURS exceeds maximum allowed (%d hours = 1 year), got %d", maxAllowedHours, hours)
	}

	return time.Duration(hours) * time.Hour, nil
}

// parseRuleSyncPollInterval parses the poll interval from string to time.Duration.
// Returns default value (10 seconds) if empty.
// Returns error if value is invalid, non-positive, or exceeds maximum.
func parseRuleSyncPollInterval(s string) (time.Duration, error) {
	const (
		defaultSeconds    = 10
		maxAllowedSeconds = 3600
	)

	if s == "" {
		return time.Duration(defaultSeconds) * time.Second, nil
	}

	seconds, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid RULE_SYNC_POLL_INTERVAL_SECONDS value '%s': %w", s, err)
	}

	if seconds <= 0 {
		return 0, fmt.Errorf("RULE_SYNC_POLL_INTERVAL_SECONDS must be positive, got %d", seconds)
	}

	if seconds > maxAllowedSeconds {
		return 0, fmt.Errorf("RULE_SYNC_POLL_INTERVAL_SECONDS exceeds maximum allowed (%d seconds = 1 hour), got %d", maxAllowedSeconds, seconds)
	}

	return time.Duration(seconds) * time.Second, nil
}

// parseRuleSyncStalenessThreshold parses the staleness threshold from string to time.Duration.
// Returns default value (50 seconds) if empty.
// Returns error if value is invalid, non-positive, or exceeds maximum.
func parseRuleSyncStalenessThreshold(s string) (time.Duration, error) {
	const (
		defaultSeconds    = 50
		maxAllowedSeconds = 3600
	)

	if s == "" {
		return time.Duration(defaultSeconds) * time.Second, nil
	}

	seconds, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid RULE_SYNC_STALENESS_THRESHOLD_SECONDS value '%s': %w", s, err)
	}

	if seconds <= 0 {
		return 0, fmt.Errorf("RULE_SYNC_STALENESS_THRESHOLD_SECONDS must be positive, got %d", seconds)
	}

	if seconds > maxAllowedSeconds {
		return 0, fmt.Errorf("RULE_SYNC_STALENESS_THRESHOLD_SECONDS exceeds maximum allowed (%d seconds = 1 hour), got %d", maxAllowedSeconds, seconds)
	}

	return time.Duration(seconds) * time.Second, nil
}

// parseRuleSyncOverlapBuffer parses the overlap buffer from string to time.Duration.
// Returns default value (2 seconds) if empty.
// Returns error if value is invalid, negative, or exceeds maximum.
// Zero is allowed (no overlap buffer).
func parseRuleSyncOverlapBuffer(s string) (time.Duration, error) {
	const (
		defaultSeconds    = 2
		maxAllowedSeconds = 60
	)

	if s == "" {
		return time.Duration(defaultSeconds) * time.Second, nil
	}

	seconds, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid RULE_SYNC_OVERLAP_BUFFER_SECONDS value '%s': %w", s, err)
	}

	if seconds < 0 {
		return 0, fmt.Errorf("RULE_SYNC_OVERLAP_BUFFER_SECONDS must be non-negative, got %d", seconds)
	}

	if seconds > maxAllowedSeconds {
		return 0, fmt.Errorf("RULE_SYNC_OVERLAP_BUFFER_SECONDS exceeds maximum allowed (%d seconds), got %d", maxAllowedSeconds, seconds)
	}

	return time.Duration(seconds) * time.Second, nil
}

// parseTrustedProxyCIDRs parses the comma-separated TRUSTED_PROXY_CIDRS value
// into a slice of *net.IPNet. Empty/blank entries (and an all-whitespace input)
// are skipped, so an empty string and a trailing comma both yield a nil slice
// with no error. A malformed CIDR fails the parse with an actionable error that
// names the env var and the offending value so the operator can fix it before
// the service ever records an audit row. Parsed ONCE at boot; the result is
// handed to the client-IP middleware.
func parseTrustedProxyCIDRs(s string) ([]*net.IPNet, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}

	parts := strings.Split(s, ",")
	nets := make([]*net.IPNet, 0, len(parts))

	for _, raw := range parts {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}

		_, network, err := net.ParseCIDR(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid TRUSTED_PROXY_CIDRS entry %q (expected CIDR notation like 10.0.0.0/8): %w", entry, err)
		}

		nets = append(nets, network)
	}

	if len(nets) == 0 {
		return nil, nil
	}

	return nets, nil
}

// LoadCleanupWorkerConfig creates a UsageCleanupWorkerConfig from environment configuration.
// Returns nil config if cleanup worker is disabled.
// Returns error if config or logger is nil, or if config values are invalid.
func LoadCleanupWorkerConfig(ctx context.Context, cfg *Config, logger libLog.Logger) (*workers.UsageCleanupWorkerConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	// Check if cleanup worker is disabled
	// Note: default value for bool is false, so cleanup worker is disabled by default
	// Set CLEANUP_WORKER_ENABLED=true to enable the background cleanup worker
	if !cfg.CleanupWorkerEnabled {
		logger.With(
			libLog.String("config", "CLEANUP_WORKER_ENABLED"),
		).Log(ctx, libLog.LevelInfo, "Usage counter cleanup worker is DISABLED")

		return nil, nil
	}

	cleanupInterval, err := parseCleanupIntervalHours(cfg.CleanupIntervalHours)
	if err != nil {
		return nil, fmt.Errorf("invalid CLEANUP_INTERVAL_HOURS: %w", err)
	}

	logger.With(
		libLog.String("cleanup_interval", cleanupInterval.String()),
	).Log(ctx, libLog.LevelInfo, "Usage counter cleanup worker configuration loaded")

	return &workers.UsageCleanupWorkerConfig{
		CleanupInterval: cleanupInterval,
	}, nil
}

// LoadReservationReaperConfig creates a ReservationReaperWorkerConfig from
// environment configuration. Returns a nil config (no error) when the reaper is
// disabled (RESERVATION_REAPER_ENABLED=false, the default) so the caller can
// propagate the "disabled" signal end-to-end exactly like LoadCleanupWorkerConfig.
// Returns an error if config or logger is nil, or if the interval is invalid.
func LoadReservationReaperConfig(ctx context.Context, cfg *Config, logger libLog.Logger) (*workers.ReservationReaperWorkerConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	if !cfg.ReservationReaperEnabled {
		logger.With(
			libLog.String("config", "RESERVATION_REAPER_ENABLED"),
		).Log(ctx, libLog.LevelInfo, "Reservation reaper worker is DISABLED")

		return nil, nil
	}

	reapInterval, err := parseReservationReaperIntervalSeconds(cfg.ReservationReaperIntervalSeconds)
	if err != nil {
		return nil, fmt.Errorf("invalid RESERVATION_REAPER_INTERVAL_SECONDS: %w", err)
	}

	logger.With(
		libLog.String("reap_interval", reapInterval.String()),
	).Log(ctx, libLog.LevelInfo, "Reservation reaper worker configuration loaded")

	return &workers.ReservationReaperWorkerConfig{
		ReapInterval: reapInterval,
	}, nil
}

// LoadRuleSyncWorkerConfig creates a RuleSyncWorkerConfig from environment configuration.
// Returns error if config or logger is nil, or if config values are invalid.
func LoadRuleSyncWorkerConfig(ctx context.Context, cfg *Config, logger libLog.Logger) (*workers.RuleSyncWorkerConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	pollInterval, err := parseRuleSyncPollInterval(cfg.RuleSyncPollIntervalSeconds)
	if err != nil {
		return nil, fmt.Errorf("invalid RULE_SYNC_POLL_INTERVAL_SECONDS: %w", err)
	}

	stalenessThreshold, err := parseRuleSyncStalenessThreshold(cfg.RuleSyncStalenessThresholdSeconds)
	if err != nil {
		return nil, fmt.Errorf("invalid RULE_SYNC_STALENESS_THRESHOLD_SECONDS: %w", err)
	}

	overlapBuffer, err := parseRuleSyncOverlapBuffer(cfg.RuleSyncOverlapBufferSeconds)
	if err != nil {
		return nil, fmt.Errorf("invalid RULE_SYNC_OVERLAP_BUFFER_SECONDS: %w", err)
	}

	if stalenessThreshold < pollInterval {
		return nil, fmt.Errorf("invalid configuration: RULE_SYNC_STALENESS_THRESHOLD_SECONDS (%s) must be >= RULE_SYNC_POLL_INTERVAL_SECONDS (%s)",
			stalenessThreshold, pollInterval)
	}

	logger.With(
		libLog.String("poll_interval", pollInterval.String()),
		libLog.String("staleness_threshold", stalenessThreshold.String()),
		libLog.String("overlap_buffer", overlapBuffer.String()),
	).Log(ctx, libLog.LevelInfo, "Rule sync worker configuration loaded")

	return &workers.RuleSyncWorkerConfig{
		PollInterval:       pollInterval,
		StalenessThreshold: stalenessThreshold,
		OverlapBuffer:      overlapBuffer,
	}, nil
}

// LoadEvaluationConfig creates an EvaluationConfig from environment configuration.
// Returns error if config or logger is nil, or if any value is invalid.
// Logs a warning if using default ALLOW decision (fail-open behavior).
func LoadEvaluationConfig(ctx context.Context, cfg *Config, logger libLog.Logger) (*query.EvaluationConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	defaultDecision, err := parseDefaultDecision(cfg.DefaultDecisionWhenNoMatch)
	if err != nil {
		return nil, fmt.Errorf("invalid DEFAULT_DECISION_WHEN_NO_MATCH: %w", err)
	}

	// Warn if using default ALLOW (fail-open) - operator should be aware
	if cfg.DefaultDecisionWhenNoMatch == "" {
		logger.With(
			libLog.String("config", "DEFAULT_DECISION_WHEN_NO_MATCH"),
			libLog.String("default_value", "ALLOW"),
		).Log(ctx, libLog.LevelWarn, "Using default ALLOW decision when no rules match (fail-open)")
	}

	maxRules, err := parseMaxRulesPerRequest(cfg.MaxRulesPerRequest)
	if err != nil {
		return nil, fmt.Errorf("invalid MAX_RULES_PER_REQUEST: %w", err)
	}

	return &query.EvaluationConfig{
		DefaultDecisionWhenNoMatch: defaultDecision,
		MaxRulesPerRequest:         maxRules,
	}, nil
}

// initCELAdapter initializes the CEL expression engine with configuration.
func initCELAdapter(cfg *Config, logger libLog.Logger) (*cel.Adapter, error) {
	celCostLimit, err := parseCELCostLimit(cfg.CELCostLimit)
	if err != nil {
		return nil, fmt.Errorf("invalid CEL cost limit configuration: %w", err)
	}

	adapter, err := cel.NewAdapter(cel.AdapterConfig{
		CostLimit: celCostLimit,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL adapter: %w", err)
	}

	return adapter, nil
}

// ValidateAuthConfig validates the authentication configuration.
// It warns when auth is disabled (operator should be aware).
// It fails if auth is enabled but key is missing.
// It warns if the key is too short (security best practice).
//
// M16: additionally rejects the combination of API_KEY_ENABLED=true with
// CORS_ALLOWED_ORIGINS="*". The CORS policy allows the X-API-Key header with
// AllowCredentials=false, meaning ANY browser on ANY origin can trigger
// authenticated API calls once it learns the key. Combined with a wildcard
// origin this becomes a CSRF-style attack where a malicious site exfiltrates
// data via legitimate-looking X-API-Key calls. Block at boot.
func ValidateAuthConfig(ctx context.Context, cfg *Config, logger libLog.Logger) error {
	// Warn if auth is disabled (operator should be aware)
	if !cfg.APIKeyEnabled {
		logger.With(libLog.String("config", "API_KEY_ENABLED")).Log(ctx, libLog.LevelWarn, "API Key authentication is DISABLED")
		return nil
	}

	// Fail if auth is enabled but key is missing
	if cfg.APIKey == "" {
		return fmt.Errorf("API_KEY must be set when API_KEY_ENABLED=true")
	}

	// Warn if key is too short (security best practice)
	if len(cfg.APIKey) < minAPIKeyLength {
		logger.With(libLog.Int("min_length", minAPIKeyLength), libLog.Int("actual_length", len(cfg.APIKey))).Log(ctx, libLog.LevelWarn, "API_KEY should be at least 32 characters")
	}

	// M16: fail-fast on API_KEY_ENABLED=true + wildcard CORS. See function
	// comment for attack rationale.
	if cfg.CORSAllowedOrigins == "*" {
		return fmt.Errorf(
			"API_KEY_ENABLED=true is incompatible with CORS_ALLOWED_ORIGINS=\"*\": " +
				"any malicious website can invoke authenticated API calls once the key leaks. " +
				"Restrict CORS_ALLOWED_ORIGINS to a concrete allow-list (e.g. https://app.example.com)")
	}

	return nil
}

// ValidateAccessManagerConfig validates the Access Manager plugin configuration.
// It warns when plugin auth is disabled (operator should be aware).
// It fails if plugin auth is enabled but the address is missing.
func ValidateAccessManagerConfig(ctx context.Context, cfg *Config, logger libLog.Logger) error {
	if !cfg.PluginAuthEnabled {
		logger.With(libLog.String("config", "PLUGIN_AUTH_ENABLED")).Log(ctx, libLog.LevelWarn, "Access Manager plugin authentication is DISABLED")
		return nil
	}

	if cfg.PluginAuthAddress == "" {
		return fmt.Errorf("PLUGIN_AUTH_ADDRESS must be set when PLUGIN_AUTH_ENABLED=true")
	}

	return nil
}

// quoteLibpqValue wraps a libpq keyword/value DSN value in single quotes and
// escapes embedded single quotes and backslashes per the libpq spec. This
// prevents values containing spaces, quotes, or backslashes (e.g. complex
// passwords) from being misparsed by pgx/libpq, which would otherwise corrupt
// the connection string and either fail boot or — worse — connect to the
// wrong database.
func quoteLibpqValue(v string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v)

	return "'" + escaped + "'"
}

// buildPostgresDSN composes the libpq keyword/value DSN tracer hands to
// lib-commons at boot. Extracted so the /readyz handler can read the same
// DSN to populate the postgres `tls` field — see SetPostgresDSN. The
// sslmode default lives here (lib-commons SetConfigFromEnvVars does not
// honor envDefault tags), so /readyz reports `tls=false` for unconfigured
// dev environments instead of nil.
//
// Each component is escaped via quoteLibpqValue so secrets containing libpq
// metacharacters (spaces, single quotes, backslashes) do not break parsing
// (CVE-class issue surfaced by code review).
func buildPostgresDSN(cfg *Config) string {
	if cfg == nil {
		return ""
	}

	sslMode := cfg.DBSSLMode
	if sslMode == "" {
		sslMode = "disable" // Default for local development; use "require" in production
	}

	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		quoteLibpqValue(cfg.DBHost),
		quoteLibpqValue(cfg.DBUser),
		quoteLibpqValue(cfg.DBPassword),
		quoteLibpqValue(cfg.DBName),
		quoteLibpqValue(cfg.DBPort),
		quoteLibpqValue(sslMode),
	)
}

// initPostgresConnection creates and connects a PostgreSQL connection pool.
// The ctx is used to bound migration and connect attempts so a wedged advisory
// lock or unreachable database cannot hang the boot sequence indefinitely.
func initPostgresConnection(ctx context.Context, cfg *Config, logger libLog.Logger) (*libPostgres.Client, error) {
	postgresSQLSource := buildPostgresDSN(cfg)

	// Run all migrations (functions + schema) in a single pass via lib-commons.
	// lib-commons's Migrator emits the `postgres.migrate_up` OpenTelemetry span
	// automatically and owns its own DB connection, so the pooled libPostgres
	// client below is independent of the migration lifecycle.
	//
	// AllowMultiStatements must stay false because migrations 000001-000003 install
	// PL/pgSQL functions using dollar-quoted bodies ($$...$$); golang-migrate's
	// multi-statement mode splits on semicolons and corrupts dollar-quoted blocks.
	//
	// INVARIANT: migrations run BEFORE libPostgres.New so the runtime pool
	// lifecycle is decoupled from migration errors — there is no pool to close
	// on migration failure, which simplifies the fail-fast path above.
	if cfg.MigrationPath != "" {
		migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
			PrimaryDSN:           postgresSQLSource,
			DatabaseName:         cfg.DBName,
			MigrationsPath:       cfg.MigrationPath,
			AllowMultiStatements: false,
			Logger:               logger,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create migrator: %w", err)
		}

		// Bound the migration run so a wedged advisory lock (two boot instances
		// racing, or an orphaned lock from a killed boot) surfaces as a clean
		// timeout error rather than hanging past any liveness/startup probe
		// window. 5 minutes is generous for the 16 migrations this service
		// ships today while still letting K8s or the operator intervene.
		migCtx, migCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer migCancel()

		if err := migrator.Up(migCtx); err != nil {
			return nil, fmt.Errorf("failed to apply migrations: %w", err)
		}

		// "up-to-date" truthfully covers both the "applied N new migrations"
		// path and the no-op path where lib-commons swallowed migrate.ErrNoChange
		// (e.g. a rolling-deploy boot against a DB a peer already migrated).
		// lib-commons itself emits a structured log for the work that actually
		// happened, so this line is a boot-sequence marker, not a duplicate.
		logger.Log(ctx, libLog.LevelInfo, "Migrations up-to-date")
	} else {
		// Silent skip is an ops footgun: a misconfigured deployment (MIGRATIONS_PATH
		// unset or empty-stringed by a templating bug) would boot successfully
		// against whatever schema the DB happens to have. Warn-log it so the
		// condition is visible in the startup trail.
		logger.Log(ctx, libLog.LevelWarn,
			"MIGRATIONS_PATH is empty, skipping migration runner (service will boot against existing schema)")
	}

	postgresConn, err := libPostgres.New(libPostgres.Config{
		PrimaryDSN: postgresSQLSource,
		ReplicaDSN: postgresSQLSource,
		Logger:     logger,
		// MaxOpenConnections / MaxIdleConnections default to 0 in production
		// (env vars unset), which lets lib-commons apply its own defaults
		// (25/10). Integration tests set DB_MAX_OPEN_CONNS=5, DB_MAX_IDLE_CONNS=2
		// to keep cumulative connections (across primary+replica × repeated
		// RestartServerWithConfig invocations) safely below the testcontainer's
		// max_connections=100.
		MaxOpenConnections: cfg.DBMaxOpenConns,
		MaxIdleConnections: cfg.DBMaxIdleConns,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create PostgreSQL client: %w", err)
	}

	if err := postgresConn.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	return postgresConn, nil
}

// initRuleService creates the rule service with all its dependencies.
// The cacheWriter parameter is optional (nil-safe); when provided, activate and
// deactivate commands will synchronously update the in-memory cache after a
// successful persistence commit.
// The txBeginner is shared with the limit lifecycle commands and the validation
// service so the rule lifecycle commands persist the status/update and the
// audit event atomically via executeInTx.
func initRuleService(ruleRepo *postgres.Repository, celAdapter *cel.Adapter, auditWriter command.AuditWriter, cacheWriter command.RuleCacheWriter, clk clock.Clock, txBeginner pgdb.TxBeginner) (*services.RuleService, error) {
	celCompiler := &celCompilerAdapter{adapter: celAdapter}

	// Inject audit writer and cache writer into Rule commands
	createRuleCmd, err := command.NewCreateRuleCommand(ruleRepo, celCompiler, clk, auditWriter, txBeginner)
	if err != nil {
		return nil, fmt.Errorf("failed to construct CreateRuleCommand: %w", err)
	}

	updateRuleCmd, err := command.NewUpdateRuleCommand(ruleRepo, celCompiler, clk, auditWriter, txBeginner)
	if err != nil {
		return nil, fmt.Errorf("failed to construct UpdateRuleCommand: %w", err)
	}

	activateRuleCmd, err := command.NewActivateRuleService(ruleRepo, celCompiler, clk, auditWriter, cacheWriter, txBeginner)
	if err != nil {
		return nil, fmt.Errorf("failed to create activate rule service: %w", err)
	}

	deactivateRuleCmd, err := command.NewDeactivateRuleService(ruleRepo, clk, auditWriter, cacheWriter, txBeginner)
	if err != nil {
		return nil, fmt.Errorf("failed to create deactivate rule service: %w", err)
	}

	draftRuleCmd, err := command.NewDraftRuleService(ruleRepo, clk, auditWriter, txBeginner)
	if err != nil {
		return nil, fmt.Errorf("failed to create draft rule service: %w", err)
	}

	deleteRuleCmd, err := command.NewDeleteRuleService(ruleRepo, auditWriter, txBeginner)
	if err != nil {
		return nil, fmt.Errorf("failed to create delete rule service: %w", err)
	}

	getRuleQuery := query.NewGetRuleQuery(ruleRepo)
	listRulesQuery := query.NewListRulesQuery(ruleRepo)

	return services.NewRuleService(createRuleCmd, updateRuleCmd, activateRuleCmd, deactivateRuleCmd, draftRuleCmd, deleteRuleCmd, getRuleQuery, listRulesQuery), nil
}

// initEvaluateRulesQuery creates the rule evaluation query with all its dependencies.
// The activeRulesRepo parameter accepts any ActiveRulesRepository implementation
// (e.g., *postgres.Repository for direct DB reads, or *cache.CacheAdapter for in-memory reads).
func initEvaluateRulesQuery(activeRulesRepo query.ActiveRulesRepository, celAdapter *cel.Adapter, evalConfig *query.EvaluationConfig) (*query.EvaluateRulesQuery, error) {
	ruleEvaluator, err := query.NewRuleEvaluator(celAdapter)
	if err != nil {
		return nil, fmt.Errorf("failed to create rule evaluator: %w", err)
	}

	completeEvaluator, err := query.NewCompleteEvaluator(ruleEvaluator)
	if err != nil {
		return nil, fmt.Errorf("failed to create complete evaluator: %w", err)
	}

	getActiveRulesQuery, err := query.NewGetActiveRulesQuery(activeRulesRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to create get active rules query: %w", err)
	}

	evaluateRulesQuery, err := query.NewEvaluateRulesQuery(getActiveRulesQuery, completeEvaluator, evalConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create evaluate rules query: %w", err)
	}

	return evaluateRulesQuery, nil
}

// initTxBeginner builds the shared TxBeginnerAdapter used by every transactional
// service path (rule/limit lifecycle commands + validation service). The adapter
// is context-aware: BeginTx resolves the per-tenant pool via tmcore.GetPGContext(ctx)
// when present (multi-tenant mode) and falls back to this static pool otherwise
// (single-tenant mode). No conditional wiring is needed — both modes share the
// same adapter, and the request context drives the per-call pool selection.
//
// In strict MT mode (enabledMT == true) the adapter refuses to fall
// back to the root pool when the context carries no tenant — mirroring
// PostgresConnectionAdapter so a missing ContextWithPG fails closed instead
// of silently opening a tx on the default pool (C5).
//
// See pgdb.TxBeginnerAdapter.BeginTx for the resolution order.
func initTxBeginner(ctx context.Context, postgresConn *libPostgres.Client, enabledMT bool) (pgdb.TxBeginner, error) {
	dbConn, err := postgresConn.Resolver(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection for transactions: %w", err)
	}

	txBeginner := pgdb.NewTxBeginnerAdapter(dbConn)
	if txBeginner == nil {
		return nil, fmt.Errorf("failed to create transaction beginner adapter: connection is nil")
	}

	txBeginner.SetMultiTenantEnabled(enabledMT)

	return txBeginner, nil
}

// limitServiceDeps holds the dependencies created during limit service initialization.
type limitServiceDeps struct {
	service          *services.LimitService
	usageCounterRepo *postgres.UsageCounterRepository
	limitRepo        *postgres.LimitRepository
}

// initLimitService creates the limit service with all its dependencies.
// The shared pgdb.Connection carries the H11 strict-mode flag so a missing
// tenant pool fails fast in MT mode rather than silently using root (M1).
// The txBeginner is shared with the validation service so the limit lifecycle
// commands persist the status/update and the audit event atomically.
func initLimitService(pgConn pgdb.Connection, auditWriter command.AuditWriter, clk clock.Clock, txBeginner pgdb.TxBeginner) (*limitServiceDeps, error) {
	limitRepo := postgres.NewLimitRepositoryWithConnection(pgConn)

	usageCounterRepo := postgres.NewUsageCounterRepositoryWithConnection(pgConn)

	createLimitCmd, err := command.NewCreateLimitCommand(limitRepo, clk, auditWriter, txBeginner)
	if err != nil {
		return nil, fmt.Errorf("failed to construct CreateLimitCommand: %w", err)
	}

	updateLimitCmd, err := command.NewUpdateLimitCommand(limitRepo, clk, auditWriter, txBeginner)
	if err != nil {
		return nil, fmt.Errorf("failed to create update limit command: %w", err)
	}

	activateLimitCmd, err := command.NewActivateLimitCommand(limitRepo, clk, auditWriter, txBeginner)
	if err != nil {
		return nil, fmt.Errorf("failed to create activate limit command: %w", err)
	}

	deactivateLimitCmd, err := command.NewDeactivateLimitCommand(limitRepo, clk, auditWriter, txBeginner)
	if err != nil {
		return nil, fmt.Errorf("failed to create deactivate limit command: %w", err)
	}

	draftLimitCmd, err := command.NewDraftLimitCommand(limitRepo, clk, auditWriter, txBeginner)
	if err != nil {
		return nil, fmt.Errorf("failed to create draft limit command: %w", err)
	}

	deleteLimitCmd, err := command.NewDeleteLimitCommand(limitRepo, clk, auditWriter, txBeginner)
	if err != nil {
		return nil, fmt.Errorf("failed to create delete limit command: %w", err)
	}

	getLimitQuery := query.NewGetLimitQuery(limitRepo)

	listLimitsQuery, err := query.NewListLimitsQuery(limitRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to create list limits query: %w", err)
	}

	service := services.NewLimitService(createLimitCmd, updateLimitCmd, activateLimitCmd, deactivateLimitCmd, draftLimitCmd, deleteLimitCmd, getLimitQuery, listLimitsQuery, usageCounterRepo)

	return &limitServiceDeps{
		service:          service,
		usageCounterRepo: usageCounterRepo,
		limitRepo:        limitRepo,
	}, nil
}

// initHTTPServer creates the HTTP server with all services wired together.
// Extracted from InitServers to reduce cyclomatic complexity.
//
// mtComponents is non-nil only when MULTI_TENANT_ENABLED=true; it supplies the
// PG pool manager and worker supervisor that the TenantMiddleware needs to
// resolve tenant-specific connections and spawn lazy workers.
//
// The txBeginner is shared with the limit lifecycle commands (see initLimitService)
// so all transactional persistence paths use the same underlying connection adapter.
func initHTTPServer(
	ctx context.Context,
	cfg *Config,
	pgConn pgdb.Connection,
	limitDeps *limitServiceDeps,
	evaluateRulesQuery *query.EvaluateRulesQuery,
	auditWriter *command.RecordAuditEventCommand,
	auditEventRepo *postgres.AuditEventRepository,
	ruleService *services.RuleService,
	healthChecker *in.HealthChecker,
	logger libLog.Logger,
	telemetry *libOtel.Telemetry,
	clk clock.Clock,
	mtComponents *componentsMT,
	mtMetrics metrics.MultiTenantMetrics,
	txBeginner pgdb.TxBeginner,
) (*HTTPServer, *services.ReservationService, error) {
	_ = ctx // reserved for future ctx-aware initialization (e.g., when NewValidationService takes ctx)
	// Init Transaction Validation repository and queries
	transactionValidationRepo := postgres.NewTransactionValidationRepositoryWithConnection(pgConn)
	getTransactionValidationQuery := query.NewGetTransactionValidationQuery(transactionValidationRepo)
	listTransactionValidationsQuery := query.NewListTransactionValidationsQuery(transactionValidationRepo)

	// Init LimitChecker for ValidationService
	limitChecker, err := query.NewLimitChecker(limitDeps.limitRepo, limitDeps.usageCounterRepo, clk)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create limit checker: %w", err)
	}

	// Init ValidationService with audit writer for SOX/GLBA compliance
	// Pass transactionValidationRepo for both command (insert) and query (FindByRequestID) operations
	//
	// Note: NewValidationService internally constructs a default
	// no-op metrics.NewMultiTenantMetrics(false, nil, nil); cross-package signature
	// change would cascade into supervisor.go + 4 test sites in metrics_test.
	validationService, err := services.NewValidationService(txBeginner, evaluateRulesQuery, limitChecker, transactionValidationRepo, transactionValidationRepo, auditWriter, clk) //nolint:contextcheck
	if err != nil {
		return nil, nil, err
	}

	// Attach multi-tenant metrics sink. In single-tenant mode this is the
	// no-op implementation (zero overhead on the Validate hot path); in MT
	// mode it emits tenant_messages_processed_total keyed by tenant_id and
	// module.
	if mtMetrics != nil {
		// Note: same NewMultiTenantMetrics cross-package issue
		// as NewValidationService above. See note at L899.
		validationService.SetMultiTenantMetrics(mtMetrics) //nolint:contextcheck
	}

	// Init Transaction Validation service facade
	transactionValidationService, err := services.NewTransactionValidationService(getTransactionValidationQuery, listTransactionValidationsQuery)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create transaction validation service: %w", err)
	}

	// Init Reservation service (two-phase capacity hold). It reuses the limit
	// checker as the limit resolver and the shared audit writer / txBeginner so
	// the reserve/confirm/release counter moves commit atomically with their
	// audit rows — the same atomicity discipline as the validate path.
	reservationRepo := postgres.NewUsageReservationRepositoryWithConnection(limitDeps.usageCounterRepo)

	longLivedTTL, err := parseReservationLongLivedTTLHours(cfg.ReservationLongLivedTTLHours)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse reservation long-lived TTL: %w", err)
	}

	reservationService, err := services.NewReservationServiceWithLongLivedTTL(txBeginner, limitChecker, reservationRepo, auditWriter, clk, longLivedTTL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create reservation service: %w", err)
	}

	// Init Audit Event service (read-only per SOX/GLBA requirements)
	auditEventService, err := initAuditEventService(auditEventRepo)
	if err != nil {
		return nil, nil, err
	}

	// Parse the trusted-proxy CIDR set ONCE at boot. A malformed entry fails
	// boot here (actionable error names TRUSTED_PROXY_CIDRS) rather than
	// silently recording forgeable audit IPs at runtime.
	trustedProxyCIDRs, err := parseTrustedProxyCIDRs(cfg.TrustedProxyCIDRs)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid trusted proxy configuration: %w", err)
	}

	// Route configuration with CORS settings. Authentication is handled
	// per-route by AuthGuard, which has its own configuration.
	routeConfig := &in.RouteConfig{
		CORSAllowedOrigins:   cfg.CORSAllowedOrigins,
		APIKeyOnlyValidation: cfg.APIKeyOnlyValidation,
		TrustedProxyCIDRs:    trustedProxyCIDRs,
	}

	// Create auth guard with all authentication configuration.
	authClient := authMiddleware.NewAuthClient(cfg.PluginAuthAddress, cfg.PluginAuthEnabled, &logger)
	// Note: NewAuthGuard builds APIKeyAuth which is a Fiber
	// handler closure; ctx propagation would require refactoring the Fiber
	// middleware API surface to take ctx, which it deliberately doesn't (Fiber
	// uses its own request-scoped UserContext that's set per-request).
	authGuard := httpMiddleware.NewAuthGuard(httpMiddleware.AuthGuardConfig{ //nolint:contextcheck
		APIKey:            cfg.APIKey,
		APIKeyEnabled:     cfg.APIKeyEnabled,
		APIKeyLabel:       cfg.APIKeyLabel,
		PluginAuthEnabled: cfg.PluginAuthEnabled,
		AppName:           trcConstant.ApplicationName,
	}, authClient)

	// Extract multi-tenant bits for the middleware registration. In
	// single-tenant mode both stay nil and NewRoutes skips the tenant
	// middleware block entirely. In multi-tenant mode pgManager drives the
	// guard `enabledMT && pgManager != nil` inside NewRoutes — the
	// middleware only registers when both conditions hold, which keeps the
	// path testable with a nil pgManager while preserving the production
	// invariant that MT=true ⇒ pgManager must be wired.
	var (
		pgManager        *tmpostgres.Manager
		workerSupervisor in.WorkerEnsurer
	)

	if mtComponents != nil {
		pgManager = mtComponents.pgManager
		workerSupervisor = mtComponents.supervisor
	}

	// Note: NewRoutes wires ReadyzHandler which is a Fiber
	// handler closure that receives ctx per-request via c.UserContext();
	// passing boot-time ctx here is conceptually wrong (boot ctx outlives
	// individual request lifecycles).
	httpApp, err := in.NewRoutes(in.RoutesDeps{ //nolint:contextcheck
		Logger:                       logger,
		Telemetry:                    telemetry,
		HealthChecker:                healthChecker,
		Cfg:                          routeConfig,
		RuleService:                  ruleService,
		LimitService:                 limitDeps.service,
		ValidationService:            validationService,
		ReservationService:           reservationService,
		TransactionValidationService: transactionValidationService,
		AuditEventService:            auditEventService,
		Guard:                        authGuard,
		Clock:                        clk,
		MultiTenantEnabled:           cfg.MultiTenantEnabled,
		PgManager:                    pgManager,
		Supervisor:                   workerSupervisor,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create routes: %w", err)
	}

	// Secure the REST reservation seam per TRACER_TLS_MODE: mtls ⇒ a verifying
	// *tls.Config, mesh/unset ⇒ nil (plaintext, sidecar terminates). Same builder
	// the gRPC server uses, so both transports share one posture.
	seamTLS, err := buildSeamTLSConfig(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build reservation seam TLS config: %w", err)
	}

	httpServer, err := NewHTTPServer(cfg, httpApp, seamTLS, logger, telemetry)
	if err != nil {
		return nil, nil, err
	}

	return httpServer, reservationService, nil
}

// initGRPCServer builds the opt-in reservation gRPC server. It returns nil (no
// error) when TRACER_GRPC_PORT is unset, so the gRPC transport stays off unless
// an operator configures it. Transport security follows TRACER_TLS_MODE (Epic
// 1.3): mtls ⇒ the server requires+verifies a client cert (reservation seam
// unreachable without one); mesh/unset ⇒ plaintext (sidecar terminates). The
// server delegates to the SAME reservationService the REST handler uses; clk
// drives the reserve timestamp-window check identically to the REST path.
func initGRPCServer(
	cfg *Config,
	reservationService *services.ReservationService,
	pgManager *tmpostgres.Manager,
	clk clock.Clock,
	logger libLog.Logger,
	telemetry *libOtel.Telemetry,
) (*GRPCServer, error) {
	if cfg.TracerGRPCPort == "" {
		return nil, nil
	}

	reservationServer, err := grpcin.NewReservationServer(reservationService, clk)
	if err != nil {
		return nil, fmt.Errorf("failed to create reservation gRPC server: %w", err)
	}

	// Same seam TLS posture as the REST listener so the two transports cannot
	// diverge. nil in mesh/unset mode ⇒ plaintext gRPC.
	seamTLS, err := buildSeamTLSConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build reservation seam TLS config: %w", err)
	}

	// Resolve the per-tenant pool from the trusted x-tenant-id metadata the
	// ledger forwards over the mTLS/mesh-verified connection. In single-tenant
	// mode the resolver is a no-op and the interceptor passes through.
	tenantResolver := seamtenant.NewResolver(pgManager, cfg.MultiTenantEnabled)

	var tenantInterceptor grpc.UnaryServerInterceptor
	if tenantResolver.Active() {
		tenantInterceptor = grpcin.TenantUnaryInterceptor(tenantResolver)
	}

	grpcServer, err := NewGRPCServer(cfg.TracerGRPCPort, reservationServer, seamTLS, tenantInterceptor, logger, telemetry)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC server: %w", err)
	}

	return grpcServer, nil
}

// initCleanupWorker creates the usage cleanup worker if enabled.
func initCleanupWorker(ctx context.Context, cfg *Config, usageCounterRepo *postgres.UsageCounterRepository, logger libLog.Logger, clk clock.Clock) (*workers.UsageCleanupWorker, error) {
	cleanupWorkerConfig, err := LoadCleanupWorkerConfig(ctx, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("invalid cleanup worker configuration: %w", err)
	}

	if cleanupWorkerConfig == nil {
		return nil, nil
	}

	// tenantID is empty in single-tenant mode; the supervisor passes the real tenantID in MT mode.
	cleanupWorker, err := workers.NewUsageCleanupWorker(usageCounterRepo, *cleanupWorkerConfig, logger, clk, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create cleanup worker: %w", err)
	}

	logger.With(
		libLog.String("component", "cleanup_worker"),
		libLog.String("cleanup_interval", cleanupWorkerConfig.CleanupInterval.String()),
	).Log(ctx, libLog.LevelInfo, "Usage cleanup worker initialized")

	return cleanupWorker, nil
}

// buildMultiTenantStack assembles the multi-tenant metrics sink and the
// multi-tenant components in a single call. In single-tenant mode the metrics
// sink is a zero-cost no-op and mtComponents stays nil; both modes share the
// same downstream wiring path. Extracted from InitServers to keep the main
// flow under the gocyclo budget.
func buildMultiTenantStack(
	ctx context.Context,
	cfg *Config,
	logger libLog.Logger,
	telemetry *libOtel.Telemetry,
	ruleCache *cache.RuleCache,
	ruleSyncRepo *postgres.RuleSyncRepository,
	limitDeps *limitServiceDeps,
	celAdapter *cel.Adapter,
	clk clock.Clock,
) (*componentsMT, metrics.MultiTenantMetrics, error) {
	var mtFactory *libMetrics.MetricsFactory
	if telemetry != nil {
		mtFactory = telemetry.MetricsFactory
	}

	// NewMultiTenantMetrics lives in services/metrics (cross-package) and has
	// 8 callers across production + tests; threading ctx through it would
	// cascade into NewValidationService, supervisor.go, and 4 test sites.
	// Out of scope here; the function's internal log line uses
	// context.Background() but is guarded behind `enabled=true && factory=nil`
	// (rare fallback path that should never fire in production).
	mtMetrics := metrics.NewMultiTenantMetrics(cfg.MultiTenantEnabled, mtFactory, logger) //nolint:contextcheck // see comment above

	mtComponents, err := initMultiTenant(ctx, cfg, logger, ruleCache, ruleSyncRepo, limitDeps, celAdapter, clk, mtMetrics)
	if err != nil {
		return nil, nil, err
	}

	return mtComponents, mtMetrics, nil
}

// initMultiTenant constructs the multi-tenant component set when
// MULTI_TENANT_ENABLED=true, otherwise returns (nil, nil). Extracted from
// InitServers to keep the main flow readable and to push the MT-specific
// dependency assembly behind one focused function.
//
// When it returns non-nil components, it also spawns a fire-and-forget
// goroutine that runs the supervisor's InitialTenantSync. The goroutine's
// outcome does not block boot: workers still spawn lazily via the
// TenantMiddleware hook on the first request for each tenant.
func initMultiTenant(
	ctx context.Context,
	cfg *Config,
	logger libLog.Logger,
	ruleCache *cache.RuleCache,
	ruleSyncRepo *postgres.RuleSyncRepository,
	limitDeps *limitServiceDeps,
	celAdapter *cel.Adapter,
	clk clock.Clock,
	mtMetrics metrics.MultiTenantMetrics,
) (*componentsMT, error) {
	if !cfg.MultiTenantEnabled {
		return nil, nil
	}

	syncCfg, err := LoadRuleSyncWorkerConfig(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	cleanupCfg, err := LoadCleanupWorkerConfig(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	// H8: LoadCleanupWorkerConfig returns nil when CLEANUP_WORKER_ENABLED=false.
	// Previously we overwrote the nil with DefaultUsageCleanupWorkerConfig,
	// which silently re-enabled per-tenant cleanup. Propagate the "disabled"
	// signal end-to-end instead: nil means the supervisor must not spawn the
	// cleanup worker (only the sync worker runs).
	var resolvedCleanup workers.UsageCleanupWorkerConfig

	cleanupEnabled := cleanupCfg != nil

	if cleanupEnabled {
		resolvedCleanup = *cleanupCfg
	}

	syncCBConfig := workers.DefaultSyncCircuitBreakerConfig()

	// M4: compiler creation lives inside buildComponentsMT now. Pass
	// the CELAdapter directly; the wiring helper owns the adapter literal.
	//
	// Note: buildComponentsMT builds worker closures
	// (per-tenant sync + cleanup) that run on their own goroutine schedules
	// and own their own ctx lifetime; threading boot ctx in is conceptually
	// wrong (the workers must outlive boot).
	components, err := buildComponentsMT(cfg, logger, //nolint:contextcheck
		wiringDepsMT{
			SyncRepo:             ruleSyncRepo,
			UsageRepo:            limitDeps.usageCounterRepo,
			CELAdapter:           celAdapter,
			SyncConfig:           *syncCfg,
			CleanupConfig:        resolvedCleanup,
			CleanupWorkerEnabled: cleanupEnabled,
			CBTemplate: workers.CircuitBreakerTemplate{
				NamePrefix:    syncCBConfig.Name,
				MaxRequests:   syncCBConfig.MaxRequests,
				Interval:      syncCBConfig.Interval,
				Timeout:       syncCBConfig.Timeout,
				FailureThresh: syncCBConfig.FailureThresh,
				FailureRatio:  syncCBConfig.FailureRatio,
				MinRequests:   syncCBConfig.MinRequests,
			},
		},
		workers.WorkerSupervisorDeps{
			RuleCache:  ruleCache,
			Clock:      clk,
			Logger:     logger,
			MaxTenants: cfg.MultiTenantMaxTenantPools,
			Service:    cfg.ApplicationName,
			Metrics:    mtMetrics,
		},
	)
	if err != nil {
		return nil, err
	}

	// M23: Defensive eviction of the empty-tenant ("") bucket after all MT
	// wiring has succeeded. conditionalWarmUpCache already does this earlier
	// in the boot sequence, but any code path between warm-up and here that
	// touches the cache via a tenant-less context (background tasks, tests
	// that share the same RuleCache) would re-populate the empty bucket.
	// Re-evicting here is idempotent and guarantees the MT invariant
	// "empty-tenant bucket is never populated" at the end of bootstrap.
	ruleCache.EvictTenant("")

	go runInitialTenantSync(ctx, logger, components.supervisor)

	return components, nil
}

// runInitialTenantSync invokes the supervisor's InitialTenantSync under a
// bounded timeout and logs (but does not propagate) any failure. Separated
// from initMultiTenant so it can be exercised directly in unit tests without
// re-running the whole wiring helper.
func runInitialTenantSync(ctx context.Context, logger libLog.Logger, sup *workers.WorkerSupervisor) {
	// Detach cancellation from the parent boot ctx (which is cancelled once
	// boot returns) but preserve trace/values; bound the sync itself to 30s.
	syncCtx, syncCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer syncCancel()

	if err := sup.InitialTenantSync(syncCtx); err != nil {
		logger.With(
			libLog.String("operation", "bootstrap.initial_tenant_sync"),
			libLog.String("error.message", err.Error()),
		).Log(syncCtx, libLog.LevelWarn, "Initial tenant sync incomplete; workers will spawn lazily")
	}
}

// initWorkers initializes all background workers and assembles the Service.
// Extracted from InitServers to reduce cyclomatic complexity.
//
// In multi-tenant mode (mtComponents != nil), the singleton cleanup + sync
// workers are intentionally skipped — the supervisor spawns per-tenant workers
// in response to tenant lifecycle events. Both sets of workers accept an empty
// tenantID in single-tenant mode, so the repos and configs remain identical.
func initWorkers(
	ctx context.Context,
	cfg *Config,
	limitDeps *limitServiceDeps,
	syncWorker *workers.RuleSyncWorker,
	serverAPI *HTTPServer,
	grpcServer *GRPCServer,
	postgresConn *libPostgres.Client,
	healthChecker *in.HealthChecker,
	logger libLog.Logger,
	clk clock.Clock,
	mtComponents *componentsMT,
) (*Service, error) {
	svc := &Service{
		HTTPServer:    serverAPI,
		grpcServer:    grpcServer,
		Logger:        logger,
		postgresConn:  postgresConn,
		healthChecker: healthChecker,
		config:        cfg,
	}

	if mtComponents != nil {
		svc.pgManager = mtComponents.pgManager
		svc.supervisor = mtComponents.supervisor
		svc.eventListener = mtComponents.eventListener
		svc.tmClient = mtComponents.tmClient

		return svc, nil
	}

	cleanupWorker, err := initCleanupWorker(ctx, cfg, limitDeps.usageCounterRepo, logger, clk)
	if err != nil {
		return nil, err
	}

	svc.cleanupWorker = cleanupWorker
	svc.syncWorker = syncWorker

	return svc, nil
}

// initSyncWorker creates the rule sync worker.
func initSyncWorker(
	ctx context.Context,
	cfg *Config,
	ruleCache *cache.RuleCache,
	syncRepo *postgres.RuleSyncRepository,
	celAdapter *cel.Adapter,
	logger libLog.Logger,
) (*workers.RuleSyncWorker, error) {
	syncWorkerConfig, err := LoadRuleSyncWorkerConfig(ctx, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("invalid rule sync worker configuration: %w", err)
	}

	// celCompilerAdapter satisfies workers.ExpressionCompiler (Compile returns (any, error))
	compiler := &celCompilerAdapter{adapter: celAdapter}

	// Configure circuit breaker for DB poll resilience.
	// Note: gobreaker's CircuitBreaker wraps callback closures
	// that execute per-call with their own ctx; boot ctx is the wrong lifecycle.
	cbConfig := workers.DefaultSyncCircuitBreakerConfig()
	cb := resilience.NewCircuitBreaker(cbConfig, logger) //nolint:contextcheck

	// tenantID is empty in single-tenant mode; the supervisor passes the real tenantID in MT mode.
	syncWorker, err := workers.NewRuleSyncWorker(ruleCache, syncRepo, compiler, *syncWorkerConfig, logger, cb, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create rule sync worker: %w", err)
	}

	logger.With(
		libLog.String("component", "rule_sync_worker"),
		libLog.String("poll_interval", syncWorkerConfig.PollInterval.String()),
		libLog.String("staleness_threshold", syncWorkerConfig.StalenessThreshold.String()),
		libLog.String("overlap_buffer", syncWorkerConfig.OverlapBuffer.String()),
		libLog.Any("circuit_breaker.failure_threshold", cbConfig.FailureThresh),
		libLog.String("circuit_breaker.timeout", cbConfig.Timeout.String()),
	).Log(ctx, libLog.LevelInfo, "Rule sync worker initialized with circuit breaker")

	return syncWorker, nil
}

// initAuditEventService initializes the audit event service with all required queries.
// Extracted to reduce cyclomatic complexity of InitServers.
func initAuditEventService(auditEventRepo *postgres.AuditEventRepository) (*services.AuditEventService, error) {
	getAuditEventQuery, err := query.NewGetAuditEventQuery(auditEventRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to create get audit event query: %w", err)
	}

	listAuditEventsQuery, err := query.NewListAuditEventsQuery(auditEventRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to create list audit events query: %w", err)
	}

	verifyAuditEventQuery, err := query.NewVerifyAuditEventQuery(auditEventRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to create verify audit event query: %w", err)
	}

	auditEventService, err := services.NewAuditEventService(getAuditEventQuery, listAuditEventsQuery, verifyAuditEventQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit event service: %w", err)
	}

	return auditEventService, nil
}

// conditionalWarmUpCache runs the bootstrap-time rule cache warm-up in
// single-tenant mode only. In multi-tenant mode warm-up would execute against
// context.Background() — which carries no tenant — and populate the empty-
// tenant ("") bucket that no request ever reads from (H15). The per-tenant
// RuleSyncWorker performs the equivalent load on its first cycle for each
// tenant instead. As a defensive measure, the empty bucket is evicted so no
// earlier code path can leave stale rules behind.
func conditionalWarmUpCache(
	ctx context.Context,
	cfg *Config,
	ruleCache *cache.RuleCache,
	ruleSyncRepo cache.RuleSyncRepository,
	compiler cache.ExpressionCompiler,
	logger libLog.Logger,
	clk clock.Clock,
) error {
	if cfg.MultiTenantEnabled {
		ruleCache.EvictTenant("")

		logger.Log(ctx, libLog.LevelInfo,
			"MT mode: skipping bootstrap cache warmup (handled per-tenant by RuleSyncWorker)")

		return nil
	}

	rulesLoaded, warmUpDuration, err := cache.WarmUp(ctx, ruleCache, ruleSyncRepo, compiler, logger, clk)
	if err != nil {
		return fmt.Errorf("failed to warm up rule cache: %w", err)
	}

	logger.With(
		libLog.Int("rules_loaded", rulesLoaded),
		libLog.Any("warmup_duration", warmUpDuration),
	).Log(ctx, libLog.LevelInfo, "Rule cache warmed up")

	return nil
}

// initCoreInfra initializes logger, validates auth config, and sets up OpenTelemetry.
func initCoreInfra(ctx context.Context, cfg *Config) (libLog.Logger, *libOtel.Telemetry, error) {
	zapEnv := libZap.Environment(cfg.OtelDeploymentEnv)
	if zapEnv == "" {
		zapEnv = libZap.EnvironmentDevelopment
	}

	zapLogger, err := libZap.New(libZap.Config{
		Environment:     zapEnv,
		Level:           cfg.LogLevel,
		OTelLibraryName: "tracer",
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	var logger libLog.Logger = zapLogger

	// Validate authentication configuration (fail-fast if misconfigured)
	if err := ValidateAuthConfig(ctx, cfg, logger); err != nil {
		return nil, nil, fmt.Errorf("invalid auth configuration: %w", err)
	}

	// Validate Access Manager plugin configuration (fail-fast if misconfigured)
	if err := ValidateAccessManagerConfig(ctx, cfg, logger); err != nil {
		return nil, nil, fmt.Errorf("invalid access manager configuration: %w", err)
	}

	// Cross-check: at least one auth mechanism must be enabled outside local
	// deployments (fail-fast; the per-mechanism validators above only Warn)
	if err := ValidateAuthPresence(ctx, cfg, logger); err != nil {
		return nil, nil, fmt.Errorf("auth presence: %w", err)
	}

	// Validate multi-tenant configuration (fail-fast if misconfigured)
	if err := ValidateMultiTenantConfig(ctx, cfg, logger); err != nil {
		return nil, nil, fmt.Errorf("invalid multi-tenant configuration: %w", err)
	}

	// Gate 4: enforce TLS posture for SaaS deployments BEFORE any external
	// connection opens (postgres, OTel exporter, tenant manager, redis).
	// Centralized in ValidateSaaSTLS so the enforcement surface is one
	// function with one call site — anti-pattern N6 (scattered inline
	// checks) is structurally absent.
	if err := ValidateSaaSTLS(cfg); err != nil {
		return nil, nil, fmt.Errorf("TLS enforcement: %w", err)
	}

	// Init OpenTelemetry via lib-commons helper (per Ring standards)
	telemetry, err := libOtel.NewTelemetry(libOtel.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    logger,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	// C2: arm lib-commons' panic-observability trident so every
	// runtime.SafeGo* call site (workers/supervisor.go, readyz.go) emits
	// the canonical panic_recovered_total counter when a goroutine panics.
	// Without this Init, SafeGo still recovers + logs, but the metric is a
	// no-op. Idempotent — subsequent calls are safe.
	if telemetry != nil {
		libRuntime.InitPanicMetrics(telemetry.MetricsFactory, logger)
	}

	return logger, telemetry, nil
}

// initClock creates a clock instance based on environment configuration.
// If MOCK_TIME env var is set to a valid RFC3339 timestamp, returns a MockClock
// with that fixed time (for integration tests). Otherwise, returns a RealClock.
//
// This allows integration tests to simulate specific times (e.g., 22:00 for nighttime
// PIX limits, Black Friday dates for custom periods) without restarting the server
// multiple times or waiting for real time to pass.
//
// SECURITY: MOCK_TIME is read once at server boot. It cannot be modified via HTTP
// requests, preventing timestamp injection attacks. In production, MOCK_TIME should
// never be set, ensuring the system always uses real time.
func initClock() clock.Clock {
	mockTime := os.Getenv("MOCK_TIME")
	if mockTime == "" {
		return clock.New()
	}

	t, err := time.Parse(time.RFC3339, mockTime)
	if err != nil {
		// Invalid format: fall back to real clock and log warning
		// Don't fail server startup due to misconfigured test env var
		fmt.Fprintf(os.Stderr, "WARNING: Invalid MOCK_TIME format '%s' (expected RFC3339), using real clock\n", mockTime)
		return clock.New()
	}

	fmt.Fprintf(os.Stderr, "INFO: Using MOCK_TIME=%s (test mode)\n", mockTime)

	return clock.NewFixedClock(t)
}

// InitServers initiate http and grpc servers. The ctx flows through
// initPostgresConnection (bounds migration + pool connect) and through the
// rule-cache WarmUp (bounds cache hydration). Other bootstrap log-only
// context.Background() calls in this package remain as pre-existing debt and
// are rerouted separately.
func InitServers(ctx context.Context) (*Service, error) {
	cfg := &Config{}

	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, err
	}

	// Apply canonical defaults for ApplicationName and MULTI_TENANT_* fields.
	// Required because lib-commons v4 SetConfigFromEnvVars does not honor
	// `envDefault` struct tags; this is the single source of truth for defaults.
	ApplyMultiTenantDefaults(cfg)

	logger, telemetry, err := initCoreInfra(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Emit the single-tenant banner when MT is off. The MT-enabled banner is
	// emitted with redacted config details by ValidateMultiTenantConfig (called
	// from initCoreInfra above); duplicating it here would produce two
	// near-identical lines in the boot log, so we only cover the else branch.
	if !cfg.MultiTenantEnabled {
		logger.Log(ctx, libLog.LevelInfo, "Running in SINGLE-TENANT MODE")
	}

	// Init PostgreSQL connection pool
	postgresConn, err := initPostgresConnection(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	// Track initialization success; if initialization fails after Connect(),
	// the deferred cleanup will mark the connection as disconnected to prevent
	// partial resource leaks and avoid using a partially-initialized connection.
	initSuccess := false

	defer func() {
		if !initSuccess && postgresConn != nil {
			_ = postgresConn.Close()
		}
	}()

	// Init health checker for readiness probe. The /readyz cycle is single-
	// tenant only — the cache probe always runs the global staleness/ready
	// check. In multi-tenant deployments per-tenant cache health is surfaced
	// via the tenant_consumers_active metric, NOT /readyz. version +
	// deploymentMode are echoed in /readyz responses; deploymentMode defaults
	// to "local" when unset (used for SaaS TLS enforcement).
	healthChecker := buildHealthChecker(cfg, postgresConn)

	// Build the OTel-backed metrics recorder for /readyz + selfprobe and wire
	// it into the health checker. The factory bridges OTel instruments to the
	// process-default Prometheus registry via UnderscoreEscapingWithoutSuffixes
	// so the canonical metric names (readyz_check_duration_ms,
	// readyz_check_status, selfprobe_result) surface unchanged on /metrics —
	// dashboards and SLO alerts that hard-code those names keep working
	// across the migration. A failure here is non-fatal: the bootstrap
	// continues with a no-op recorder so probe semantics still gate /health,
	// only metric emission is silenced.
	readyzRecorder := buildReadyzRecorder(ctx, logger)
	healthChecker.SetReadyzRecorder(readyzRecorder)

	// Build the single pgdb.Connection adapter and toggle strict MT mode ONCE.
	// every repository now shares this adapter, so tenant resolution lives in
	// one place (M1) instead of being duplicated as getDB helpers across six
	// repos. In strict MT mode the adapter refuses to fall back to the root
	// pool when the request context carries no tenant (H11).
	pgConn := pgdb.NewPostgresConnectionAdapter(postgresConn)
	pgConn.SetMultiTenantEnabled(cfg.MultiTenantEnabled)

	// Init CEL expression engine
	celAdapter, err := initCELAdapter(cfg, logger)
	if err != nil {
		return nil, err
	}

	// Init Rule repository (shared by rule service and evaluation)
	ruleRepo := postgres.NewRepositoryWithConnection(pgConn)

	// Init Audit Event repository and command (needed by Rule/Limit commands and ValidationService)
	auditEventRepo := postgres.NewAuditEventRepositoryWithConnection(pgConn)
	auditWriter := command.NewRecordAuditEventCommand(auditEventRepo)

	// Init Clock, Rule Cache (warm up + CEL compile), and the single-tenant
	// sync worker. Extracted into initRuleCacheStack to keep InitServers under
	// the gocyclo budget; behavior and ordering are unchanged.
	clk, ruleCache, ruleSyncRepo, syncWorker, err := initRuleCacheStack(ctx, cfg, pgConn, celAdapter, logger)
	if err != nil {
		return nil, err
	}

	txBeginner, err := initTxBeginner(ctx, postgresConn, cfg.MultiTenantEnabled)
	if err != nil {
		return nil, err
	}

	// Init Rule service with audit writer and rule cache for synchronous cache updates
	ruleService, err := initRuleService(ruleRepo, celAdapter, auditWriter, ruleCache, clk, txBeginner)
	if err != nil {
		return nil, err
	}

	cacheAdapter, err := cache.NewCacheAdapter(ruleCache)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache adapter: %w", err)
	}

	healthChecker.SetCacheHealthProvider(ruleCache)

	// Init Rule Evaluation components
	evalConfig, err := LoadEvaluationConfig(ctx, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("invalid evaluation configuration: %w", err)
	}

	evaluateRulesQuery, err := initEvaluateRulesQuery(cacheAdapter, celAdapter, evalConfig)
	if err != nil {
		return nil, err
	}

	// Init Limit service with audit writer for SOX/GLBA compliance
	limitDeps, err := initLimitService(pgConn, auditWriter, clk, txBeginner)
	if err != nil {
		return nil, err
	}

	mtComponents, mtMetrics, err := buildMultiTenantStack(ctx, cfg, logger, telemetry, ruleCache, ruleSyncRepo, limitDeps, celAdapter, clk)
	if err != nil {
		return nil, err
	}

	// Init HTTP server with all services. mtComponents is nil in single-tenant
	// mode; the HTTP server builder threads pgManager + supervisor through to
	// the TenantMiddleware when non-nil.
	serverAPI, reservationService, err := initHTTPServer(ctx, cfg, pgConn, limitDeps, evaluateRulesQuery, auditWriter, auditEventRepo, ruleService, healthChecker, logger, telemetry, clk, mtComponents, mtMetrics, txBeginner)
	if err != nil {
		return nil, err
	}

	// Init background workers (conditional on MT mode inside initWorkers).
	// finalizeStartup also builds the opt-in reservation gRPC server and runs the
	// startup self-probe BEFORE the HTTP server begins accepting traffic; folded
	// into one helper to keep InitServers under the gocyclo budget.
	svc, err := finalizeStartup(ctx, cfg, limitDeps, syncWorker, serverAPI, reservationService, postgresConn, healthChecker, logger, telemetry, clk, mtComponents)
	if err != nil {
		return nil, err
	}

	// Mark initialization as successful; defer cleanup will not close the connection.
	initSuccess = true

	return svc, nil
}

// initRuleCacheStack builds the clock, rule cache, rule-sync repository, and
// (single-tenant only) sync worker. Extracted from InitServers to keep the
// composition root under the gocyclo budget; behavior and ordering are
// unchanged.
//
// The cache warmup runs under a 30s timeout context that is created and
// cancelled inside this helper. The warmup context never escapes the helper,
// so cancelling it here (rather than at InitServers teardown) is observably
// equivalent.
//
// M19: the singleton sync worker is skipped in MT mode — the supervisor spawns
// a per-tenant sync worker for each active tenant, so the singleton would never
// be registered and its allocation (+config load) would be dead work on boot.
func initRuleCacheStack(
	ctx context.Context,
	cfg *Config,
	pgConn pgdb.Connection,
	celAdapter *cel.Adapter,
	logger libLog.Logger,
) (clock.Clock, *cache.RuleCache, *postgres.RuleSyncRepository, *workers.RuleSyncWorker, error) {
	// Init Clock (supports MOCK_TIME for integration tests)
	clk := initClock()

	// Init Rule Cache: warm up from database, compile CEL expressions, wire into evaluation path
	ruleCache := cache.NewRuleCache(clk)
	ruleSyncRepo := postgres.NewRuleSyncRepositoryWithConnection(pgConn)

	warmUpCtx, warmUpCancel := context.WithTimeout(ctx, 30*time.Second)
	defer warmUpCancel()

	cacheCompiler := &celCompilerAdapter{adapter: celAdapter}

	if err := conditionalWarmUpCache(warmUpCtx, cfg, ruleCache, ruleSyncRepo, cacheCompiler, logger, clk); err != nil {
		return nil, nil, nil, nil, err
	}

	// Init sync worker for background polling (cross-instance consistency).
	var syncWorker *workers.RuleSyncWorker

	if !cfg.MultiTenantEnabled {
		var err error

		syncWorker, err = initSyncWorker(ctx, cfg, ruleCache, ruleSyncRepo, celAdapter, logger)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	}

	return clk, ruleCache, ruleSyncRepo, syncWorker, nil
}

// finalizeStartup wires workers, runs the one-shot startup self-probe, and
// returns the assembled Service. Combining initWorkers + executeStartupSelfProbe
// here keeps InitServers under the gocyclo budget while keeping the boot order
// (workers first, then probe) explicit.
//
// On self-probe failure the Service is discarded — the caller's deferred
// cleanup closes the postgres pool and logs the failure. K8s observes the
// pod exit and restarts it; /health stays 503 the whole time.
func finalizeStartup(
	ctx context.Context,
	cfg *Config,
	limitDeps *limitServiceDeps,
	syncWorker *workers.RuleSyncWorker,
	serverAPI *HTTPServer,
	reservationService *services.ReservationService,
	postgresConn *libPostgres.Client,
	healthChecker *in.HealthChecker,
	logger libLog.Logger,
	telemetry *libOtel.Telemetry,
	clk clock.Clock,
	mtComponents *componentsMT,
) (*Service, error) {
	var pgManager *tmpostgres.Manager
	if mtComponents != nil {
		pgManager = mtComponents.pgManager
	}

	grpcServer, err := initGRPCServer(cfg, reservationService, pgManager, clk, logger, telemetry)
	if err != nil {
		return nil, err
	}

	svc, err := initWorkers(ctx, cfg, limitDeps, syncWorker, serverAPI, grpcServer, postgresConn, healthChecker, logger, clk, mtComponents)
	if err != nil {
		return nil, err
	}

	if err := executeStartupSelfProbe(ctx, cfg, healthChecker, logger); err != nil {
		return nil, err
	}

	return svc, nil
}

// buildReadyzRecorder constructs the OTel-backed metrics recorder used by
// the /readyz handler and the startup self-probe. The recorder shares its
// MetricsFactory with the process-default Prometheus registry via the OTel
// → Prometheus exporter, so the canonical metric names continue to surface
// on /metrics for operator scrape consumers.
//
// A construction failure is non-fatal — bootstrap falls back to a no-op
// recorder so /readyz and the self-probe keep functioning end-to-end. The
// error is logged at Warn level so operators notice the missing metric
// stream without crashlooping the pod.
func buildReadyzRecorder(ctx context.Context, logger libLog.Logger) *observability.Recorder {
	// Note: NewPrometheusBackedFactory builds Prometheus
	// HTTP-handler closures that serve scrape requests on their own ctx;
	// boot ctx is the wrong lifecycle.
	factory, _, err := observability.NewPrometheusBackedFactory(nil, logger) //nolint:contextcheck
	if err != nil {
		if logger != nil {
			logger.With(
				libLog.String("operation", "bootstrap.readyz.recorder"),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelWarn,
				"Falling back to no-op /readyz recorder")
		}

		return observability.NewNopRecorder()
	}

	return observability.NewRecorder(factory, logger)
}

// executeStartupSelfProbe wires the /health gate and runs the one-shot
// startup probe. Extracted from InitServers to keep the bootstrap composition
// root under the gocyclo budget — the helper itself is straight-line code.
//
// The /readyz cycle is single-tenant only: the self-probe checks postgres +
// rule_cache regardless of MULTI_TENANT_ENABLED. Multi-tenant per-tenant
// readiness is surfaced via metrics, not /readyz.
//
// buildHealthChecker constructs the *in.HealthChecker for the /readyz +
// /health stack and applies every bootstrap-side knob in one place:
//
//   - postgres TLS posture sources (DSN + sslmode parser);
//   - MT mode flag (H1: in MT mode the /readyz cycle skips the rule_cache
//     probe — the cache is per-tenant and the K8s probe carries no
//     tenant context);
//   - operator-tunable cache staleness threshold
//     (H3: READYZ_CACHE_STALENESS_THRESHOLD_SECONDS).
//
// Extracting this out of InitServers keeps the main bootstrap flow under
// the gocyclo budget while making the readyz wiring testable in isolation.
func buildHealthChecker(cfg *Config, postgresConn *libPostgres.Client) *in.HealthChecker {
	hc := in.NewHealthChecker(postgresConn, cfg.OtelServiceVersion, resolveDeploymentMode(cfg))

	// Wire TLS posture sources for the postgres /readyz probe. The DSN is
	// the same one handed to lib-commons; the detector parses sslmode without
	// touching the live connection (anti-pattern N5).
	hc.SetPostgresDSN(buildPostgresDSN(cfg))
	hc.SetPostgresTLSDetector(detectPostgresTLS)

	// H1: tell the /readyz handler whether MT mode is on so the rule_cache
	// probe is gated correctly. Mirrors the boot self-probe behaviour in
	// buildSelfProbeChecks.
	hc.SetMultiTenantEnabled(cfg.MultiTenantEnabled)

	// H3: apply the operator-tunable cache-staleness threshold. Non-positive
	// values are rejected by the setter — the production-grade default is
	// preserved.
	if secs := cfg.ReadyzCacheStalenessThresholdSeconds; secs > 0 {
		hc.SetCacheStalenessThreshold(time.Duration(secs) * time.Second)
	}

	return hc
}

// Failure here is a hard boot failure: the operator cares which dep was
// unreachable, so the wrapped error preserves the dep names from RunSelfProbe.
func executeStartupSelfProbe(ctx context.Context, cfg *Config, healthChecker *in.HealthChecker, logger libLog.Logger) error {
	// Wire the /health self-probe gate so the LivenessHandler returns 503
	// until RunSelfProbe completes successfully. Done here (not at package
	// init) so tests that exercise the handler in isolation can run without
	// the bootstrap chain.
	in.SetSelfProbeGate(IsSelfProbeOK)

	// Detach cancellation from the parent boot ctx (cancelled once boot
	// returns) but preserve trace/values; bound the probe itself to 30s.
	probeCtx, probeCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer probeCancel()

	mt := cfg != nil && cfg.MultiTenantEnabled

	if err := RunSelfProbe(probeCtx, buildSelfProbeChecks(healthChecker, mt), healthChecker.ReadyzRecorder(), logger); err != nil {
		return fmt.Errorf("startup self-probe failed: %w", err)
	}

	return nil
}

// buildSelfProbeChecks assembles the SelfProbeChecks map used by RunSelfProbe.
// Reuses the same probe contracts that /readyz uses so the boot-time check
// and the per-request check exercise identical code paths — no risk of "boot
// passes but readyz fails" drift.
//
// /readyz is single-tenant only. In multi-tenant mode the empty-tenant cache
// bucket is intentionally NOT warmed at boot — InitialTenantSync runs
// asynchronously and populates per-tenant buckets. Including the rule_cache
// self-probe in MT mode would therefore guarantee a startup probe failure
// and crashloop the pod. We omit it in MT mode; per-tenant readiness is
// surfaced via the tenant_consumers_active metric, not /readyz.
//
// An empty map (no deps) is a degenerate but valid case — RunSelfProbe
// trivially passes when nothing is required.
func buildSelfProbeChecks(hc *in.HealthChecker, enabledMT bool) SelfProbeChecks {
	checks := SelfProbeChecks{}

	if hc != nil {
		checks["postgres"] = newPostgresSelfProbe(hc)

		if !enabledMT {
			checks["rule_cache"] = newRuleCacheSelfProbe(hc)
		}
	}

	return checks
}
