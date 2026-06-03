// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package bootstrap

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerConfig_MultiTenantEnabled_DefaultFalse(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	assert.False(t, cfg.MultiTenantEnabled)
}

func TestWorkerConfig_MultiTenant_ValidWithoutURLWhenDisabled(t *testing.T) {
	t.Parallel()
	// Documents that a fully-configured worker validates cleanly with
	// MultiTenantEnabled=false and MultiTenantURL empty — the backward
	// compatibility contract: no tenant vars required in single-tenant mode.
	cfg := validWorkerConfig()
	cfg.MultiTenantEnabled = false
	cfg.MultiTenantURL = ""

	assert.NoError(t, cfg.Validate(),
		"worker config must validate with MultiTenantEnabled=false (no tenant vars required)")
}

func TestWorkerConfig_MultiTenant_ErrorWhenEnabledWithoutURL(t *testing.T) {
	t.Parallel()
	cfg := validWorkerConfig()
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = ""
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 30

	err := cfg.Validate()
	require.Error(t, err, "Validate() must return error when MultiTenantEnabled=true and MultiTenantURL is empty")
	assert.Contains(t, err.Error(), "MULTI_TENANT_URL is required when MULTI_TENANT_ENABLED=true")
}

func TestWorkerConfig_MultiTenant_ValidWhenEnabledWithURL(t *testing.T) {
	t.Parallel()
	cfg := validWorkerConfig()
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "http://tenant-manager:8080"
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 30
	cfg.RedisHost = "redis:6379" // Required for multi-tenant mode

	assert.NoError(t, cfg.Validate(),
		"Validate() must pass when MultiTenantEnabled=true and MultiTenantURL is set")
}

func TestWorkerConfig_MultiTenant_DoesNotRequireStaticMongoConfig(t *testing.T) {
	t.Parallel()

	cfg := validWorkerConfig()
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "http://tenant-manager:8080"
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 30
	cfg.RedisHost = "redis:6379"
	cfg.MongoDBHost = ""
	cfg.MongoDBName = ""

	assert.NoError(t, cfg.Validate(),
		"Validate() must pass in multi-tenant mode without static Mongo configuration")
}

func TestWorkerConfig_MultiTenant_ProductionRejectsHTTPURL(t *testing.T) {
	t.Parallel()
	cfg := validWorkerConfig()
	cfg.EnvName = "production"
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "http://tenant-manager:8080"
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 30
	cfg.RedisHost = "redis:6379"

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MULTI_TENANT_URL must use HTTPS in production")
}

func TestWorkerConfig_MultiTenant_ErrorWhenCircuitBreakerThresholdZero(t *testing.T) {
	t.Parallel()
	cfg := validWorkerConfig()
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "http://tenant-manager:8080"
	cfg.MultiTenantCircuitBreakerThreshold = 0

	err := cfg.Validate()
	require.Error(t, err,
		"Validate() must return error when MultiTenantEnabled=true and CircuitBreakerThreshold=0")
	assert.Contains(t, err.Error(),
		"MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD must be > 0 when MULTI_TENANT_ENABLED=true")
}

func TestWorkerConfig_MultiTenant_ErrorWhenCircuitBreakerTimeoutZero(t *testing.T) {
	t.Parallel()
	cfg := validWorkerConfig()
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "http://tenant-manager:8080"
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 0

	err := cfg.Validate()
	require.Error(t, err,
		"Validate() must return error when CircuitBreakerThreshold>0 and CircuitBreakerTimeoutSec=0")
	assert.Contains(t, err.Error(),
		"MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC must be > 0 when MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD > 0")
}

func TestWorkerConfig_MultiTenant_CanonicalFieldsExist(t *testing.T) {
	t.Parallel()
	// Verify all 12 canonical multi-tenant fields exist and have correct zero/default values.
	cfg := Config{}
	assert.False(t, cfg.MultiTenantEnabled)
	assert.Empty(t, cfg.MultiTenantURL)
	assert.Empty(t, cfg.MultiTenantEnvironment)
	assert.Zero(t, cfg.MultiTenantMaxTenantPools)
	assert.Zero(t, cfg.MultiTenantIdleTimeoutSec)
	assert.Zero(t, cfg.MultiTenantCircuitBreakerThreshold)
	assert.Zero(t, cfg.MultiTenantCircuitBreakerTimeoutSec)
	assert.Empty(t, cfg.MultiTenantServiceAPIKey)
	// New canonical fields
	assert.Empty(t, cfg.MultiTenantRedisHost)
	assert.Empty(t, cfg.MultiTenantRedisPort)
	assert.Empty(t, cfg.MultiTenantRedisPassword)
	assert.Zero(t, cfg.MultiTenantTimeout)
	assert.Zero(t, cfg.MultiTenantCacheTTLSec)
}

func TestWorkerConfig_MultiTenant_NewFieldDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		field    string
		envKey   string
		expected string
	}{
		{
			name:     "MULTI_TENANT_REDIS_PORT has default 6379",
			field:    "MultiTenantRedisPort",
			envKey:   "MULTI_TENANT_REDIS_PORT",
			expected: "6379",
		},
		{
			name:     "MULTI_TENANT_TIMEOUT has default 30",
			field:    "MultiTenantTimeout",
			envKey:   "MULTI_TENANT_TIMEOUT",
			expected: "30",
		},
		{
			name:     "MULTI_TENANT_CACHE_TTL_SEC has default 120",
			field:    "MultiTenantCacheTTLSec",
			envKey:   "MULTI_TENANT_CACHE_TTL_SEC",
			expected: "120",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Verify the struct tag contains the expected env key and default via reflection.
			typ := reflect.TypeOf(Config{})
			f, ok := typ.FieldByName(tc.field)
			require.True(t, ok, "field %s must exist in Config struct", tc.field)
			assert.Equal(t, tc.envKey, f.Tag.Get("env"), "env tag for %s", tc.field)
			assert.Equal(t, tc.expected, f.Tag.Get("default"), "default tag for %s", tc.field)
		})
	}
}

func TestWorkerConfig_MultiTenant_NewFieldsDoNotBreakValidation(t *testing.T) {
	t.Parallel()

	cfg := validWorkerConfig()
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "http://tenant-manager:8080"
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 30
	cfg.MultiTenantServiceAPIKey = "test-key"
	cfg.RedisHost = "redis:6379"
	// New fields populated
	cfg.MultiTenantRedisHost = "mt-redis:6379"
	cfg.MultiTenantRedisPort = "6379"
	cfg.MultiTenantRedisPassword = "secret"
	cfg.MultiTenantTimeout = 30
	cfg.MultiTenantCacheTTLSec = 120

	assert.NoError(t, cfg.Validate(),
		"Validate() must pass when all multi-tenant fields including new canonical fields are populated")
}

func TestWorkerConfig_MultiTenant_NewFieldsOptionalWhenEnabled(t *testing.T) {
	t.Parallel()

	cfg := validWorkerConfig()
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "http://tenant-manager:8080"
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 30
	cfg.MultiTenantServiceAPIKey = "test-key"
	cfg.RedisHost = "redis:6379"
	// New fields left at zero values — they have sensible defaults
	cfg.MultiTenantRedisHost = ""
	cfg.MultiTenantRedisPort = ""
	cfg.MultiTenantRedisPassword = ""
	cfg.MultiTenantTimeout = 0
	cfg.MultiTenantCacheTTLSec = 0

	assert.NoError(t, cfg.Validate(),
		"Validate() must pass when new multi-tenant fields are empty (they have sensible defaults)")
}

// TestWorkerConfig_MultiTenant_RedisFieldsExist verifies that Redis config fields
// exist for tenant discovery cache (required when MultiTenantEnabled=true).
func TestWorkerConfig_MultiTenant_RedisFieldsExist(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	// Zero-value checks for Redis fields
	assert.Empty(t, cfg.RedisHost)
	assert.Empty(t, cfg.RedisPassword)
	assert.Zero(t, cfg.RedisDB)
	assert.Zero(t, cfg.RedisProtocol)
}

// TestWorkerConfig_MultiTenant_ErrorWhenEnabledWithoutRedis verifies that
// validation fails when MultiTenantEnabled=true but RedisHost is empty.
func TestWorkerConfig_MultiTenant_ErrorWhenEnabledWithoutRedis(t *testing.T) {
	t.Parallel()
	cfg := validWorkerConfig()
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "http://tenant-manager:8080"
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 30
	cfg.RedisHost = "" // Missing required Redis host

	err := cfg.Validate()
	require.Error(t, err, "Validate() must return error when MultiTenantEnabled=true and RedisHost is empty")
	assert.Contains(t, err.Error(), "REDIS_HOST is required when MULTI_TENANT_ENABLED=true")
}

// TestWorkerConfig_MultiTenant_ValidWhenRedisConfigured verifies that
// validation passes when multi-tenant mode is enabled and Redis is configured.
func TestWorkerConfig_MultiTenant_ValidWhenRedisConfigured(t *testing.T) {
	t.Parallel()
	cfg := validWorkerConfig()
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "http://tenant-manager:8080"
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 30
	cfg.RedisHost = "redis:6379"
	cfg.RedisDB = 0
	cfg.RedisProtocol = 3

	err := cfg.Validate()
	require.NoError(t, err, "Validate() must pass when MultiTenantEnabled=true with Redis configured")
}

// TestWorkerConfig_SingleTenant_NoRedisRequired verifies that Redis is NOT
// required when MultiTenantEnabled=false (backward compatibility).
func TestWorkerConfig_SingleTenant_NoRedisRequired(t *testing.T) {
	t.Parallel()
	cfg := validWorkerConfig()
	cfg.MultiTenantEnabled = false
	cfg.RedisHost = ""

	err := cfg.Validate()
	require.NoError(t, err, "Validate() must pass when MultiTenantEnabled=false even without Redis")
}
