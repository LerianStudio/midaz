// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/internal/testutil"
)

// TestValidateMultiTenantConfig_DisabledPasses verifies that validation is a
// no-op when MULTI_TENANT_ENABLED=false: all other MULTI_TENANT_* fields may be
// empty without triggering an error.
func TestValidateMultiTenantConfig_DisabledPasses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "all fields empty",
			cfg:  &Config{MultiTenantEnabled: false},
		},
		{
			name: "URL set but disabled",
			cfg: &Config{
				MultiTenantEnabled: false,
				MultiTenantURL:     "http://tenant-manager:8080",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := testutil.NewMockLogger()
			err := ValidateMultiTenantConfig(t.Context(), tc.cfg, logger)
			require.NoError(t, err)
		})
	}
}

// TestValidateMultiTenantConfig_EnabledMissingURL verifies the fail-fast path
// when multi-tenant mode is enabled without MULTI_TENANT_URL.
func TestValidateMultiTenantConfig_EnabledMissingURL(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()
	cfg := &Config{
		MultiTenantEnabled:       true,
		MultiTenantURL:           "",
		MultiTenantServiceAPIKey: "svc-api-key",
		MultiTenantRedisHost:     "redis.example.com",
	}

	err := ValidateMultiTenantConfig(t.Context(), cfg, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MULTI_TENANT_URL")
}

// TestValidateMultiTenantConfig_EnabledMissingServiceAPIKey verifies the
// fail-fast path when MULTI_TENANT_SERVICE_API_KEY is missing.
func TestValidateMultiTenantConfig_EnabledMissingServiceAPIKey(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()
	cfg := &Config{
		MultiTenantEnabled:       true,
		MultiTenantURL:           "http://tenant-manager:8080",
		MultiTenantServiceAPIKey: "",
		MultiTenantRedisHost:     "redis.example.com",
	}

	err := ValidateMultiTenantConfig(t.Context(), cfg, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MULTI_TENANT_SERVICE_API_KEY")
}

// TestValidateMultiTenantConfig_EnabledMissingRedisHost verifies the fail-fast
// path when MULTI_TENANT_REDIS_HOST is missing.
func TestValidateMultiTenantConfig_EnabledMissingRedisHost(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()
	cfg := &Config{
		MultiTenantEnabled:       true,
		MultiTenantURL:           "http://tenant-manager:8080",
		MultiTenantServiceAPIKey: "svc-api-key",
		MultiTenantRedisHost:     "",
	}

	err := ValidateMultiTenantConfig(t.Context(), cfg, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MULTI_TENANT_REDIS_HOST")
}

// TestValidateMultiTenantConfig_BlocksAPIKeyOnlyMode verifies the MT fail-fast
// path when PluginAuthEnabled is false. In API-key-only mode TenantMiddleware
// uses jwt.ParseUnverified against an unsigned token, so any caller with a
// valid API key could forge a JWT with any tenantId. The bootstrap MUST reject
// this combination rather than run in a configuration that enables
// cross-tenant forgery.
func TestValidateMultiTenantConfig_BlocksAPIKeyOnlyMode(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()
	cfg := &Config{
		MultiTenantEnabled:       true,
		MultiTenantURL:           "http://tenant-manager:8080",
		MultiTenantServiceAPIKey: "svc-api-key",
		MultiTenantRedisHost:     "redis.example.com",
		PluginAuthEnabled:        false,
	}

	err := ValidateMultiTenantConfig(t.Context(), cfg, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PLUGIN_AUTH_ENABLED",
		"error must mention PLUGIN_AUTH_ENABLED so operators know which knob to flip")
}

// TestValidateMultiTenantConfig_BlocksAPIKeyEnabledOnlyValidation verifies the
// MT fail-fast path when APIKeyOnlyValidation is true. That mode lets the
// validation endpoint bypass plugin auth — in MT that is a JWT-forgery hole
// because TenantMiddleware trusts the upstream-validated JWT signature.
func TestValidateMultiTenantConfig_BlocksAPIKeyEnabledOnlyValidation(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()
	cfg := &Config{
		MultiTenantEnabled:       true,
		MultiTenantURL:           "http://tenant-manager:8080",
		MultiTenantServiceAPIKey: "svc-api-key",
		MultiTenantRedisHost:     "redis.example.com",
		PluginAuthEnabled:        true,
		APIKeyOnlyValidation:     true,
	}

	err := ValidateMultiTenantConfig(t.Context(), cfg, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API_KEY_ENABLED_ONLY_VALIDATION",
		"error must mention API_KEY_ENABLED_ONLY_VALIDATION so operators know which knob to flip")
}

// TestValidateMultiTenantConfig_EnabledAllFieldsValid verifies the happy path:
// all required fields present returns nil and logs the "enabled" info line.
// The service API key and redis password MUST NOT appear in log fields.
func TestValidateMultiTenantConfig_EnabledAllFieldsValid(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()
	cfg := &Config{
		MultiTenantEnabled:       true,
		MultiTenantURL:           "http://tenant-manager.lerian.internal:8080/api",
		MultiTenantServiceAPIKey: "super-secret-service-api-key",
		MultiTenantRedisHost:     "redis.lerian.internal",
		MultiTenantRedisPassword: "super-secret-redis-password",
		// MT requires plugin auth (JWT signature verification). See
		// TestValidateMultiTenantConfig_BlocksAPIKeyOnlyMode for the rationale.
		PluginAuthEnabled: true,
	}

	err := ValidateMultiTenantConfig(t.Context(), cfg, logger)
	require.NoError(t, err)

	require.NotEmpty(t, logger.Calls, "expected at least one info log line")

	var foundEnabledLog bool

	for _, call := range logger.Calls {
		if call.Message != "Multi-tenant mode enabled" {
			continue
		}

		foundEnabledLog = true

		fieldMap := testutil.FieldsToMap(call.Fields)
		for k, v := range fieldMap {
			strVal, _ := v.(string)
			assert.NotContains(t, strVal, "super-secret-service-api-key",
				"service API key must not be logged (field=%s)", k)
			assert.NotContains(t, strVal, "super-secret-redis-password",
				"redis password must not be logged (field=%s)", k)
		}
	}

	assert.True(t, foundEnabledLog, "expected 'Multi-tenant mode enabled' log line")
}

// TestConfig_ApplicationNameDefault verifies that when APPLICATION_NAME is
// unset, the ApplicationName field defaults to "tracer" after the bootstrap
// default step.
func TestConfig_ApplicationNameDefault(t *testing.T) {
	t.Setenv("APPLICATION_NAME", "")

	cfg := &Config{}
	require.NoError(t, libCommons.SetConfigFromEnvVars(cfg))

	ApplyMultiTenantDefaults(cfg)

	assert.Equal(t, "tracer", cfg.ApplicationName)
}

// TestConfig_MultiTenantDefaults verifies the canonical defaults for the
// MULTI_TENANT_* configuration block after bootstrap default application.
//
// NOTE: MULTI_TENANT_REDIS_TLS defaults to true when unset (H14 security
// default). To keep this test deterministic across environments that may
// leak env vars, we Setenv every MULTI_TENANT_* key to empty — which means
// os.LookupEnv("MULTI_TENANT_REDIS_TLS") returns true and the empty-string
// value parses as false. The unset-true behavior is verified separately in
// TestConfig_MultiTenantRedisTLSDefaultsTrueWhenUnset.
func TestConfig_MultiTenantDefaults(t *testing.T) {
	// Clear all MULTI_TENANT_* env vars to exercise default path.
	envVars := []string{
		"MULTI_TENANT_ENABLED",
		"MULTI_TENANT_URL",
		"MULTI_TENANT_REDIS_HOST",
		"MULTI_TENANT_REDIS_PORT",
		"MULTI_TENANT_REDIS_PASSWORD",
		"MULTI_TENANT_REDIS_TLS",
		"MULTI_TENANT_MAX_TENANT_POOLS",
		"MULTI_TENANT_IDLE_TIMEOUT_SEC",
		"MULTI_TENANT_TIMEOUT",
		"MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD",
		"MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC",
		"MULTI_TENANT_SERVICE_API_KEY",
		"MULTI_TENANT_CACHE_TTL_SEC",
		"MULTI_TENANT_CONNECTIONS_CHECK_INTERVAL_SEC",
	}
	for _, ev := range envVars {
		t.Setenv(ev, "")
	}

	cfg := &Config{}
	require.NoError(t, libCommons.SetConfigFromEnvVars(cfg))

	ApplyMultiTenantDefaults(cfg)

	assert.False(t, cfg.MultiTenantEnabled, "MULTI_TENANT_ENABLED default=false")
	assert.Equal(t, "6379", cfg.MultiTenantRedisPort, "MULTI_TENANT_REDIS_PORT default=6379")
	assert.Equal(t, 100, cfg.MultiTenantMaxTenantPools, "MULTI_TENANT_MAX_TENANT_POOLS default=100")
	assert.Equal(t, 300, cfg.MultiTenantIdleTimeoutSec, "MULTI_TENANT_IDLE_TIMEOUT_SEC default=300")
	assert.Equal(t, 30, cfg.MultiTenantTimeout, "MULTI_TENANT_TIMEOUT default=30")
	assert.Equal(t, 5, cfg.MultiTenantCircuitBreakerThreshold, "MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD default=5")
	assert.Equal(t, 30, cfg.MultiTenantCircuitBreakerTimeoutSec, "MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC default=30")
	assert.Equal(t, 120, cfg.MultiTenantCacheTTLSec, "MULTI_TENANT_CACHE_TTL_SEC default=120")
	assert.Equal(t, 30, cfg.MultiTenantConnectionsCheckIntervalSec, "MULTI_TENANT_CONNECTIONS_CHECK_INTERVAL_SEC default=30")
}

// TestConfig_MultiTenantRedisTLSDefaultsTrueWhenUnset verifies H14: when
// MULTI_TENANT_REDIS_TLS is completely unset (not merely empty), the secure
// default is TLS=true. Operators who want cleartext Redis must set the env
// var explicitly — at which point the WARN banner from ValidateMultiTenantConfig
// fires as before.
//
// Cannot use t.Setenv with "" here — that would make os.LookupEnv return true
// and bypass the default branch we are testing. We must actually Unsetenv and
// restore on cleanup.
func TestConfig_MultiTenantRedisTLSDefaultsTrueWhenUnset(t *testing.T) {
	// Save the original value so the test is safe to run in parallel with a
	// CI environment that pre-sets MULTI_TENANT_REDIS_TLS.
	const key = "MULTI_TENANT_REDIS_TLS"

	original, wasSet := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))

	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(key, original)
		} else {
			_ = os.Unsetenv(key)
		}
	})

	cfg := &Config{}
	require.NoError(t, libCommons.SetConfigFromEnvVars(cfg))

	ApplyMultiTenantDefaults(cfg)

	assert.True(t, cfg.MultiTenantRedisTLS,
		"MULTI_TENANT_REDIS_TLS must default to true when the env var is unset (secure default)")
}

// TestConfig_MultiTenantRedisTLSExplicitFalseIsRespected verifies H14: when
// the operator explicitly sets MULTI_TENANT_REDIS_TLS=false (knowing it emits
// a WARN banner), the default is NOT re-applied on top.
func TestConfig_MultiTenantRedisTLSExplicitFalseIsRespected(t *testing.T) {
	t.Setenv("MULTI_TENANT_REDIS_TLS", "false")

	cfg := &Config{}
	require.NoError(t, libCommons.SetConfigFromEnvVars(cfg))

	ApplyMultiTenantDefaults(cfg)

	assert.False(t, cfg.MultiTenantRedisTLS,
		"explicit MULTI_TENANT_REDIS_TLS=false must be preserved (operator knows the risk)")
}
