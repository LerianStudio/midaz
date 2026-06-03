// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validManagerConfig returns a Config with all required fields populated.
func validManagerConfig() *Config {
	return &Config{
		ServerAddress:               "localhost:4005",
		MongoDBHost:                 "localhost",
		MongoDBName:                 "reporter",
		MongoMaxPoolSize:            "100",
		MongoMinPoolSize:            "10",
		MongoMaxConnIdleTime:        "60s",
		RabbitURI:                   "amqps",
		RabbitMQHost:                "localhost",
		RabbitMQPortAMQP:            "5672",
		RabbitMQUser:                "guest",
		RabbitMQPass:                "guest",
		RabbitMQGenerateReportQueue: "reporter.generate-report.queue",
		RabbitMQExchange:            "reporter.generate-report.exchange",
		RabbitMQGenerateReportKey:   "reporter.generate-report.key",
		RedisHost:                   "localhost:6379",
		ObjectStorageEndpoint:       "http://localhost:8333",
	}
}

func TestConfig_Validate_ValidConfig(t *testing.T) {
	t.Parallel()

	cfg := validManagerConfig()
	err := cfg.Validate()
	require.NoError(t, err)
}

func TestConfig_Validate_AllFieldsMissing(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	err := cfg.Validate()
	require.Error(t, err)

	errMsg := err.Error()
	assert.Contains(t, errMsg, "config validation failed:")
	assert.Contains(t, errMsg, "SERVER_ADDRESS is required")
	assert.Contains(t, errMsg, "MONGO_HOST is required")
	assert.Contains(t, errMsg, "MONGO_NAME is required")
	assert.Contains(t, errMsg, "RABBITMQ_HOST is required")
	assert.Contains(t, errMsg, "RABBITMQ_PORT_AMQP is required")
	assert.Contains(t, errMsg, "RABBITMQ_DEFAULT_USER is required")
	assert.Contains(t, errMsg, "RABBITMQ_DEFAULT_PASS is required")
	assert.Contains(t, errMsg, "RABBITMQ_GENERATE_REPORT_QUEUE is required")
	assert.Contains(t, errMsg, "RABBITMQ_EXCHANGE is required")
	assert.Contains(t, errMsg, "RABBITMQ_GENERATE_REPORT_KEY is required")
	assert.Contains(t, errMsg, "REDIS_HOST is required")
}

func TestConfig_Validate_SingleFieldMissing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		modify      func(cfg *Config)
		expectedErr string
	}{
		{
			name:        "missing ServerAddress",
			modify:      func(cfg *Config) { cfg.ServerAddress = "" },
			expectedErr: "SERVER_ADDRESS is required",
		},
		{
			name:        "missing MongoDBHost",
			modify:      func(cfg *Config) { cfg.MongoDBHost = "" },
			expectedErr: "MONGO_HOST is required",
		},
		{
			name:        "missing MongoDBName",
			modify:      func(cfg *Config) { cfg.MongoDBName = "" },
			expectedErr: "MONGO_NAME is required",
		},
		{
			name:        "missing RabbitMQHost",
			modify:      func(cfg *Config) { cfg.RabbitMQHost = "" },
			expectedErr: "RABBITMQ_HOST is required",
		},
		{
			name:        "missing RabbitMQPortAMQP",
			modify:      func(cfg *Config) { cfg.RabbitMQPortAMQP = "" },
			expectedErr: "RABBITMQ_PORT_AMQP is required",
		},
		{
			name:        "missing RabbitMQUser",
			modify:      func(cfg *Config) { cfg.RabbitMQUser = "" },
			expectedErr: "RABBITMQ_DEFAULT_USER is required",
		},
		{
			name:        "missing RabbitMQPass",
			modify:      func(cfg *Config) { cfg.RabbitMQPass = "" },
			expectedErr: "RABBITMQ_DEFAULT_PASS is required",
		},
		{
			name:        "missing RabbitMQGenerateReportQueue",
			modify:      func(cfg *Config) { cfg.RabbitMQGenerateReportQueue = "" },
			expectedErr: "RABBITMQ_GENERATE_REPORT_QUEUE is required",
		},
		{
			name:        "missing RabbitMQExchange",
			modify:      func(cfg *Config) { cfg.RabbitMQExchange = "" },
			expectedErr: "RABBITMQ_EXCHANGE is required",
		},
		{
			name:        "missing RabbitMQGenerateReportKey",
			modify:      func(cfg *Config) { cfg.RabbitMQGenerateReportKey = "" },
			expectedErr: "RABBITMQ_GENERATE_REPORT_KEY is required",
		},
		{
			name:        "missing RedisHost",
			modify:      func(cfg *Config) { cfg.RedisHost = "" },
			expectedErr: "REDIS_HOST is required",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := validManagerConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestConfig_Validate_MultipleFieldsMissing(t *testing.T) {
	t.Parallel()

	cfg := validManagerConfig()
	cfg.ServerAddress = ""
	cfg.RedisHost = ""
	cfg.MongoDBHost = ""

	err := cfg.Validate()
	require.Error(t, err)

	errMsg := err.Error()
	assert.Contains(t, errMsg, "SERVER_ADDRESS is required")
	assert.Contains(t, errMsg, "REDIS_HOST is required")
	assert.Contains(t, errMsg, "MONGO_HOST is required")

	// Verify multi-line format
	lines := strings.Split(errMsg, "\n")
	assert.GreaterOrEqual(t, len(lines), 4) // header + 3 errors
}

func TestConfig_Validate_OptionalFieldsCanBeEmpty(t *testing.T) {
	t.Parallel()

	cfg := validManagerConfig()
	// These fields are optional and should not cause validation errors
	cfg.EnvName = ""
	cfg.LogLevel = ""
	cfg.MongoDBPassword = ""
	cfg.MongoDBParameters = ""
	cfg.RedisPassword = ""
	cfg.AuthAddress = ""

	err := cfg.Validate()
	require.NoError(t, err)
}

func TestConfig_Validate_ProductionRejectsHTTPRabbitHealthURL(t *testing.T) {
	t.Parallel()

	cfg := validManagerConfig()
	cfg.EnvName = "production"
	cfg.EnableTelemetry = true
	cfg.AuthEnabled = true
	cfg.RabbitURI = "amqps"
	cfg.MongoDBPassword = "mongo-secret"
	cfg.RabbitMQPass = "rabbit-secret"
	cfg.RedisPassword = "redis-secret"
	cfg.ObjectStorageSecretKey = "object-storage-secret"
	cfg.CORSAllowedOrigins = "https://example.com"
	cfg.RabbitMQHealthCheckURL = "http://rabbitmq:15672"

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RABBITMQ_HEALTH_CHECK_URL must use HTTPS in production")
}

func TestConfig_Validate_ProductionRequiresSecureStorageAndRedis(t *testing.T) {
	t.Parallel()

	cfg := validManagerConfig()
	cfg.EnvName = "production"
	cfg.EnableTelemetry = true
	cfg.AuthEnabled = true
	cfg.RabbitURI = "amqps"
	cfg.MongoDBPassword = "mongo-secret"
	cfg.RabbitMQPass = "rabbit-secret"
	cfg.RedisPassword = "redis-secret"
	cfg.ObjectStorageSecretKey = "object-storage-secret"
	cfg.CORSAllowedOrigins = "https://example.com"
	cfg.RedisTLS = false
	cfg.ObjectStorageDisableSSL = true
	cfg.ObjectStorageEndpoint = "http://minio.internal:9000"

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "REDIS_TLS must be true in production")
	assert.Contains(t, err.Error(), "OBJECT_STORAGE_DISABLE_SSL must be false in production")
	assert.Contains(t, err.Error(), "OBJECT_STORAGE_ENDPOINT must use HTTPS in production")
}

func TestConfig_Validate_MultiTenantRequiresServiceAPIKey(t *testing.T) {
	t.Parallel()

	cfg := validManagerConfig()
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "http://tenant-manager:8080"
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 30
	cfg.MultiTenantServiceAPIKey = ""

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MULTI_TENANT_SERVICE_API_KEY is required when MULTI_TENANT_ENABLED=true")
}

func TestConfig_Validate_MultiTenantValidWithServiceAPIKey(t *testing.T) {
	t.Parallel()

	cfg := validManagerConfig()
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "http://tenant-manager:8080"
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 30
	cfg.MultiTenantServiceAPIKey = "test-api-key"

	err := cfg.Validate()
	require.NoError(t, err)
}

func TestConfig_Validate_ProductionRequiresSecureRabbitMQScheme(t *testing.T) {
	t.Parallel()

	cfg := validManagerConfig()
	cfg.EnvName = "production"
	cfg.EnableTelemetry = true
	cfg.AuthEnabled = true
	cfg.MongoDBPassword = "mongo-secret"
	cfg.RabbitMQPass = "rabbit-secret"
	cfg.RedisPassword = "redis-secret"
	cfg.ObjectStorageSecretKey = "object-storage-secret"
	cfg.CORSAllowedOrigins = "https://example.com"
	cfg.RabbitURI = "amqp"

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RABBITMQ_URI must use AMQPS in production")
}

// productionInsecureTLSConfig builds a production Config that satisfies all
// non-TLS production requirements (telemetry, auth, secrets, CORS) but runs
// every dependency over plaintext: REDIS_TLS=false, OBJECT_STORAGE_DISABLE_SSL=true,
// and an amqp:// (non-TLS) RabbitMQ URI. Used by the ALLOW_INSECURE_TLS opt-out
// tests.
func productionInsecureTLSConfig() *Config {
	cfg := validManagerConfig()
	cfg.EnvName = "production"
	cfg.EnableTelemetry = true
	cfg.AuthEnabled = true
	cfg.MongoDBPassword = "mongo-secret"
	cfg.RabbitMQPass = "rabbit-secret"
	cfg.RedisPassword = "redis-secret"
	cfg.ObjectStorageSecretKey = "object-storage-secret"
	cfg.CORSAllowedOrigins = "https://example.com"
	// HTTPS endpoint: the OBJECT_STORAGE_ENDPOINT scheme check is a URL
	// validation independent of the ALLOW_INSECURE_TLS toggle, so keep it
	// valid to isolate the toggle-gated TLS checks under test.
	cfg.ObjectStorageEndpoint = "https://s3.example.com"
	// Plaintext dependencies gated by ALLOW_INSECURE_TLS.
	cfg.RabbitURI = "amqp"
	cfg.RedisTLS = false
	cfg.ObjectStorageDisableSSL = true
	cfg.MultiTenantRedisHost = "mt-valkey:6379"
	cfg.MultiTenantRedisTLS = false

	return cfg
}

// TestConfig_Validate_ProductionTLSEnforcedWhenAllowInsecureTLSFalse verifies
// that production TLS requirements still fail validation when ALLOW_INSECURE_TLS
// is unset/false (the secure default). This is the RED guard for the opt-out:
// the gate must stay closed by default.
func TestConfig_Validate_ProductionTLSEnforcedWhenAllowInsecureTLSFalse(t *testing.T) {
	t.Parallel()

	cfg := productionInsecureTLSConfig()
	cfg.AllowInsecureTLS = false

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "REDIS_TLS must be true in production")
	assert.Contains(t, err.Error(), "MULTI_TENANT_REDIS_TLS must be true in production")
	assert.Contains(t, err.Error(), "OBJECT_STORAGE_DISABLE_SSL must be false in production")
	assert.Contains(t, err.Error(), "RABBITMQ_URI must use AMQPS in production")
}

// TestConfig_Validate_ProductionTLSBypassedWhenAllowInsecureTLSTrue verifies
// that setting ALLOW_INSECURE_TLS=true bypasses every TLS-related production
// requirement, so an otherwise-valid plaintext production config passes.
func TestConfig_Validate_ProductionTLSBypassedWhenAllowInsecureTLSTrue(t *testing.T) {
	t.Parallel()

	cfg := productionInsecureTLSConfig()
	cfg.AllowInsecureTLS = true

	err := cfg.Validate()
	require.NoError(t, err, "ALLOW_INSECURE_TLS=true should bypass production TLS enforcement")
}

// TestConfig_Validate_NonTLSProductionChecksStillEnforcedWithAllowInsecureTLS
// verifies that the ALLOW_INSECURE_TLS opt-out only relaxes TLS checks — the
// non-TLS production requirements (telemetry, auth, secrets, CORS) remain
// enforced even when the flag is true.
func TestConfig_Validate_NonTLSProductionChecksStillEnforcedWithAllowInsecureTLS(t *testing.T) {
	t.Parallel()

	cfg := productionInsecureTLSConfig()
	cfg.AllowInsecureTLS = true
	cfg.EnableTelemetry = false
	cfg.AuthEnabled = false
	cfg.CORSAllowedOrigins = ""

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ENABLE_TELEMETRY must be true in production")
	assert.Contains(t, err.Error(), "PLUGIN_AUTH_ENABLED must be true in production")
	assert.Contains(t, err.Error(), "CORS_ALLOWED_ORIGINS must not be empty in production")
}

// TestEnforceManagerSaaSTLS_SkippedWhenAllowInsecureTLSTrue verifies that the
// Gate-4 SaaS TLS enforcement is skipped when ALLOW_INSECURE_TLS=true, even for
// a DEPLOYMENT_MODE=saas config whose dependencies are all plaintext (which
// would otherwise be rejected by ValidateSaaSTLS).
func TestEnforceManagerSaaSTLS_SkippedWhenAllowInsecureTLSTrue(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSManagerConfig()
	cfg.RabbitURI = "amqp://reporter-user:secret@rabbitmq.example.com:5672/"
	cfg.RedisTLS = false
	cfg.ObjectStorageEndpoint = "http://seaweedfs:8333"

	// Sanity: without the flag, enforcement rejects this plaintext SaaS config.
	cfg.AllowInsecureTLS = false
	require.Error(t, enforceManagerSaaSTLS(cfg),
		"plaintext SaaS config must be rejected when ALLOW_INSECURE_TLS=false")

	// With the flag, enforcement is skipped entirely.
	cfg.AllowInsecureTLS = true
	require.NoError(t, enforceManagerSaaSTLS(cfg),
		"ALLOW_INSECURE_TLS=true should skip SaaS TLS enforcement")
}
