// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"net/url"
	"os"

	libLog "github.com/LerianStudio/lib-observability/log"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
)

// Multi-tenant canonical defaults.
//
// These constants are the single source of truth for MULTI_TENANT_* default
// values. lib-commons v4 SetConfigFromEnvVars does not honor `envDefault`
// tags, so defaults are applied in code via ApplyMultiTenantDefaults after
// env loading completes.
const (
	defaultMultiTenantRedisPort                   = "6379"
	defaultMultiTenantMaxTenantPools              = 100
	defaultMultiTenantIdleTimeoutSec              = 300
	defaultMultiTenantTimeout                     = 30
	defaultMultiTenantCircuitBreakerThreshold     = 5
	defaultMultiTenantCircuitBreakerTimeoutSec    = 30
	defaultMultiTenantCacheTTLSec                 = 120
	defaultMultiTenantConnectionsCheckIntervalSec = 30
)

// ApplyMultiTenantDefaults fills in canonical default values for any MULTI_TENANT_*
// or ApplicationName field left at its zero value after libCommons.SetConfigFromEnvVars.
//
// This exists because lib-commons v4 intentionally does not read `envDefault`
// struct tags — it only populates fields from the `env` tag, falling back to
// the Go zero value when the env var is unset. Keeping defaults here gives us
// a single, testable source of truth and avoids silent drift.
func ApplyMultiTenantDefaults(cfg *Config) {
	if cfg == nil {
		return
	}

	if cfg.ApplicationName == "" {
		cfg.ApplicationName = "tracer"
	}

	if cfg.MultiTenantRedisPort == "" {
		cfg.MultiTenantRedisPort = defaultMultiTenantRedisPort
	}

	if cfg.MultiTenantMaxTenantPools == 0 {
		cfg.MultiTenantMaxTenantPools = defaultMultiTenantMaxTenantPools
	}

	if cfg.MultiTenantIdleTimeoutSec == 0 {
		cfg.MultiTenantIdleTimeoutSec = defaultMultiTenantIdleTimeoutSec
	}

	if cfg.MultiTenantTimeout == 0 {
		cfg.MultiTenantTimeout = defaultMultiTenantTimeout
	}

	if cfg.MultiTenantCircuitBreakerThreshold == 0 {
		cfg.MultiTenantCircuitBreakerThreshold = defaultMultiTenantCircuitBreakerThreshold
	}

	if cfg.MultiTenantCircuitBreakerTimeoutSec == 0 {
		cfg.MultiTenantCircuitBreakerTimeoutSec = defaultMultiTenantCircuitBreakerTimeoutSec
	}

	if cfg.MultiTenantCacheTTLSec == 0 {
		cfg.MultiTenantCacheTTLSec = defaultMultiTenantCacheTTLSec
	}

	if cfg.MultiTenantConnectionsCheckIntervalSec == 0 {
		cfg.MultiTenantConnectionsCheckIntervalSec = defaultMultiTenantConnectionsCheckIntervalSec
	}

	// Security default: TLS is ON unless the operator explicitly disabled it.
	// lib-commons SetConfigFromEnvVars cannot distinguish "unset" from "false",
	// so probe the raw env var via os.LookupEnv. Operators who need cleartext
	// Redis (local dev only) must set MULTI_TENANT_REDIS_TLS=false explicitly
	// and accept the WARN-at-boot banner from ValidateMultiTenantConfig.
	if _, present := os.LookupEnv("MULTI_TENANT_REDIS_TLS"); !present {
		cfg.MultiTenantRedisTLS = true
	}
}

// ValidateMultiTenantConfig validates the multi-tenant configuration block.
//
// When MULTI_TENANT_ENABLED=false (single-tenant mode), the function is a
// no-op and returns nil regardless of the other MULTI_TENANT_* fields.
//
// When MULTI_TENANT_ENABLED=true, the function enforces fail-fast validation
// on the three required fields: MULTI_TENANT_URL, MULTI_TENANT_SERVICE_API_KEY,
// and MULTI_TENANT_REDIS_HOST. On success, it emits a single info log line
// "Multi-tenant mode enabled" with safely redacted field values — only the URL
// host (not the full URL, never credentials) is exposed. The service API key
// and redis password are never logged.
func ValidateMultiTenantConfig(ctx context.Context, cfg *Config, logger libLog.Logger) error {
	if cfg == nil {
		return fmt.Errorf("multi-tenant config: cfg is required: %w", constant.ErrMTConfigRequired)
	}

	// Single-tenant mode is a documented no-op; check that BEFORE the logger
	// requirement so callers in single-tenant mode never need to wire a
	// logger here.
	if !cfg.MultiTenantEnabled {
		return nil
	}

	if logger == nil {
		return fmt.Errorf("multi-tenant config: logger is required: %w", constant.ErrMTLoggerRequired)
	}

	if cfg.MultiTenantURL == "" {
		return fmt.Errorf("MULTI_TENANT_URL must be set when MULTI_TENANT_ENABLED=true: %w", constant.ErrMTURLRequired)
	}

	// Validate URL syntax at boot, not just presence: values like "foo" or
	// "https://" pass an emptiness check but fail later when the
	// tenant-manager client tries to dial. Require a non-empty scheme + host.
	u, err := url.Parse(cfg.MultiTenantURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf(
			"MULTI_TENANT_URL must be a valid absolute URL with scheme and host (got %q): %w",
			cfg.MultiTenantURL, constant.ErrMTURLInvalid)
	}

	if cfg.MultiTenantServiceAPIKey == "" {
		return fmt.Errorf(
			"MULTI_TENANT_SERVICE_API_KEY must be set when MULTI_TENANT_ENABLED=true: %w",
			constant.ErrMTServiceAPIKeyRequired)
	}

	if cfg.MultiTenantRedisHost == "" {
		return fmt.Errorf(
			"MULTI_TENANT_REDIS_HOST must be set when MULTI_TENANT_ENABLED=true: %w",
			constant.ErrMTRedisHostRequired)
	}

	// Security: TenantMiddleware uses jwt.ParseUnverified to extract the
	// tenantId claim, delegating signature verification to the upstream auth
	// layer (the Access Manager plugin). When PLUGIN_AUTH_ENABLED=false there
	// is no signature check anywhere in the chain — any caller with a valid
	// API key could mint an unsigned JWT claiming any tenantId and reach any
	// tenant's data. Block this combination at boot.
	if !cfg.PluginAuthEnabled {
		return fmt.Errorf(
			"MULTI_TENANT_ENABLED=true requires PLUGIN_AUTH_ENABLED=true "+
				"(API-key-only auth cannot verify JWT signatures, enabling cross-tenant forgery): %w",
			constant.ErrMTPluginAuthRequired)
	}

	// Security: APIKeyOnlyValidation lets the /v1/validations endpoint bypass
	// plugin auth (API key only). In MT that creates the same forgery hole as
	// PLUGIN_AUTH_ENABLED=false but scoped to the hottest endpoint. Block.
	if cfg.APIKeyOnlyValidation {
		return fmt.Errorf(
			"MULTI_TENANT_ENABLED=true is incompatible with API_KEY_ENABLED_ONLY_VALIDATION=true "+
				"(validation endpoint must go through plugin auth for JWT signature verification): %w",
			constant.ErrMTAPIKeyOnlyValidationConfl)
	}

	logger.With(
		libLog.String("tenant_manager_host", redactURLHost(cfg.MultiTenantURL)),
		libLog.String("redis_host", cfg.MultiTenantRedisHost),
		libLog.String("redis_port", cfg.MultiTenantRedisPort),
		libLog.Any("redis_tls", cfg.MultiTenantRedisTLS),
		libLog.Int("max_tenant_pools", cfg.MultiTenantMaxTenantPools),
		libLog.Int("cache_ttl_sec", cfg.MultiTenantCacheTTLSec),
	).Log(ctx, libLog.LevelInfo, "Multi-tenant mode enabled")

	return nil
}

// redactURLHost returns only the host component of a URL for safe logging.
// If the input is not a parseable URL, it returns "<unparseable>" to avoid
// leaking any path/query/credentials that may be embedded in the raw value.
func redactURLHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "<unparseable>"
	}

	return u.Host
}
