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

// validWorkerConfig returns a Config with all required fields populated.
func validWorkerConfig() *Config {
	return &Config{
		RabbitURI:                   "amqps",
		RabbitMQHost:                "localhost",
		RabbitMQPortAMQP:            "5672",
		RabbitMQUser:                "guest",
		RabbitMQPass:                "guest",
		RabbitMQGenerateReportQueue: "reporter.generate-report.queue",
		MongoDBHost:                 "localhost",
		MongoDBName:                 "reporter",
		ObjectStorageEndpoint:       "http://localhost:8333",
	}
}

func TestConfig_Validate_ValidConfig(t *testing.T) {
	t.Parallel()

	cfg := validWorkerConfig()
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
	assert.Contains(t, errMsg, "RABBITMQ_HOST is required")
	assert.Contains(t, errMsg, "RABBITMQ_PORT_AMQP is required")
	assert.Contains(t, errMsg, "RABBITMQ_DEFAULT_USER is required")
	assert.Contains(t, errMsg, "RABBITMQ_DEFAULT_PASS is required")
	assert.Contains(t, errMsg, "RABBITMQ_GENERATE_REPORT_QUEUE is required")
	assert.Contains(t, errMsg, "MONGO_HOST is required")
	assert.Contains(t, errMsg, "MONGO_NAME is required")
}

func TestConfig_Validate_SingleFieldMissing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		modify      func(cfg *Config)
		expectedErr string
	}{
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
			name:        "missing MongoDBHost",
			modify:      func(cfg *Config) { cfg.MongoDBHost = "" },
			expectedErr: "MONGO_HOST is required",
		},
		{
			name:        "missing MongoDBName",
			modify:      func(cfg *Config) { cfg.MongoDBName = "" },
			expectedErr: "MONGO_NAME is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := validWorkerConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestConfig_Validate_MultipleFieldsMissing(t *testing.T) {
	t.Parallel()

	cfg := validWorkerConfig()
	cfg.RabbitMQHost = ""
	cfg.MongoDBHost = ""

	err := cfg.Validate()
	require.Error(t, err)

	errMsg := err.Error()
	assert.Contains(t, errMsg, "RABBITMQ_HOST is required")
	assert.Contains(t, errMsg, "MONGO_HOST is required")

	// Verify multi-line format
	lines := strings.Split(errMsg, "\n")
	assert.GreaterOrEqual(t, len(lines), 3) // header + 2 errors
}

func TestConfig_ValidateProductionConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		modify      func(cfg *Config)
		expectErr   bool
		errContains []string
	}{
		{
			name: "valid production config",
			modify: func(cfg *Config) {
				cfg.EnvName = "production"
				cfg.EnableTelemetry = true
				cfg.MongoDBPassword = "real-secret"
				cfg.RabbitMQPass = "real-secret"
				cfg.ObjectStorageSecretKey = "real-secret"
				cfg.ObjectStorageEndpoint = "https://storage.example.com"
				cfg.CryptoHashSecretKeyPluginCRM = "real-secret"
				cfg.CryptoEncryptSecretKeyPluginCRM = "real-secret"
			},
			expectErr: false,
		},
		{
			name: "production requires telemetry enabled",
			modify: func(cfg *Config) {
				cfg.EnvName = "production"
				cfg.EnableTelemetry = false
				cfg.MongoDBPassword = "real-secret"
				cfg.RabbitMQPass = "real-secret"
				cfg.ObjectStorageSecretKey = "real-secret"
				cfg.CryptoHashSecretKeyPluginCRM = "real-secret"
				cfg.CryptoEncryptSecretKeyPluginCRM = "real-secret"
			},
			expectErr:   true,
			errContains: []string{"ENABLE_TELEMETRY must be true in production"},
		},
		{
			name: "production rejects default placeholder password",
			modify: func(cfg *Config) {
				cfg.EnvName = "production"
				cfg.EnableTelemetry = true
				cfg.MongoDBPassword = "CHANGE_ME"
				cfg.RabbitMQPass = "real-secret"
				cfg.ObjectStorageSecretKey = "real-secret"
				cfg.CryptoHashSecretKeyPluginCRM = "real-secret"
				cfg.CryptoEncryptSecretKeyPluginCRM = "real-secret"
			},
			expectErr:   true,
			errContains: []string{"MONGO_PASSWORD must not use the default placeholder in production"},
		},
		{
			name: "production rejects empty secrets",
			modify: func(cfg *Config) {
				cfg.EnvName = "production"
				cfg.EnableTelemetry = true
				cfg.MongoDBPassword = ""
				cfg.RabbitMQPass = ""
				cfg.ObjectStorageSecretKey = "real-secret"
				cfg.CryptoHashSecretKeyPluginCRM = "real-secret"
				cfg.CryptoEncryptSecretKeyPluginCRM = "real-secret"
			},
			expectErr:   true,
			errContains: []string{"MONGO_PASSWORD must not be empty in production", "RABBITMQ_DEFAULT_PASS must not be empty in production"},
		},
		{
			name: "production rejects empty crypto keys",
			modify: func(cfg *Config) {
				cfg.EnvName = "production"
				cfg.EnableTelemetry = true
				cfg.MongoDBPassword = "real-secret"
				cfg.RabbitMQPass = "real-secret"
				cfg.ObjectStorageSecretKey = "real-secret"
				cfg.CryptoHashSecretKeyPluginCRM = ""
				cfg.CryptoEncryptSecretKeyPluginCRM = ""
			},
			expectErr: true,
			errContains: []string{
				"CRYPTO_HASH_SECRET_KEY_PLUGIN_CRM must not be empty in production",
				"CRYPTO_ENCRYPT_SECRET_KEY_PLUGIN_CRM must not be empty in production",
			},
		},
		{
			name: "non-production skips production validation",
			modify: func(cfg *Config) {
				cfg.EnvName = "staging"
				cfg.EnableTelemetry = false
				cfg.MongoDBPassword = "CHANGE_ME"
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := validWorkerConfig()
			tt.modify(cfg)

			err := cfg.Validate()

			if tt.expectErr {
				require.Error(t, err)
				for _, expected := range tt.errContains {
					assert.Contains(t, err.Error(), expected)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_Validate_OptionalFieldsCanBeEmpty(t *testing.T) {
	t.Parallel()

	cfg := validWorkerConfig()
	// These fields are optional and should not cause validation errors
	cfg.EnvName = ""
	cfg.LogLevel = ""
	cfg.MongoDBPassword = ""
	cfg.MongoDBParameters = ""
	cfg.RabbitMQHealthCheckURL = ""

	err := cfg.Validate()
	require.NoError(t, err)
}

func TestConfig_Validate_ProductionRejectsHTTPRabbitHealthURL(t *testing.T) {
	t.Parallel()

	cfg := validWorkerConfig()
	cfg.EnvName = "production"
	cfg.EnableTelemetry = true
	cfg.RabbitURI = "amqps"
	cfg.MongoDBPassword = "mongo-secret"
	cfg.RabbitMQPass = "rabbit-secret"
	cfg.ObjectStorageSecretKey = "object-storage-secret"
	cfg.CryptoHashSecretKeyPluginCRM = "hash-secret"
	cfg.CryptoEncryptSecretKeyPluginCRM = "encrypt-secret"
	cfg.RabbitMQHealthCheckURL = "http://rabbitmq:15672"

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RABBITMQ_HEALTH_CHECK_URL must use HTTPS in production")
}

func TestConfig_Validate_ProductionRequiresSecureStorageAndRedisWhenMultiTenant(t *testing.T) {
	t.Parallel()

	cfg := validWorkerConfig()
	cfg.EnvName = "production"
	cfg.EnableTelemetry = true
	cfg.RabbitURI = "amqps"
	cfg.MongoDBPassword = "mongo-secret"
	cfg.RabbitMQPass = "rabbit-secret"
	cfg.ObjectStorageSecretKey = "object-storage-secret"
	cfg.CryptoHashSecretKeyPluginCRM = "hash-secret"
	cfg.CryptoEncryptSecretKeyPluginCRM = "encrypt-secret"
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "https://tenant-manager.internal"
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 30
	cfg.MultiTenantServiceAPIKey = "test-api-key"
	cfg.RedisHost = "redis:6379"
	cfg.RedisTLS = false
	cfg.RedisPassword = ""
	cfg.ObjectStorageDisableSSL = true
	cfg.ObjectStorageEndpoint = "http://minio.internal:9000"

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "REDIS_TLS must be true in production when MULTI_TENANT_ENABLED=true")
	assert.Contains(t, err.Error(), "REDIS_PASSWORD must not be empty in production when MULTI_TENANT_ENABLED=true and REDIS_USE_GCP_IAM=false")
	assert.Contains(t, err.Error(), "OBJECT_STORAGE_DISABLE_SSL must be false in production")
	assert.Contains(t, err.Error(), "OBJECT_STORAGE_ENDPOINT must use HTTPS in production")
}

func TestConfig_Validate_MultiTenantRequiresServiceAPIKey(t *testing.T) {
	t.Parallel()

	cfg := validWorkerConfig()
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "http://tenant-manager:8080"
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 30
	cfg.RedisHost = "redis:6379"
	cfg.MultiTenantServiceAPIKey = ""

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MULTI_TENANT_SERVICE_API_KEY is required when MULTI_TENANT_ENABLED=true")
}

func TestConfig_Validate_MultiTenantValidWithServiceAPIKey(t *testing.T) {
	t.Parallel()

	cfg := validWorkerConfig()
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "http://tenant-manager:8080"
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 30
	cfg.RedisHost = "redis:6379"
	cfg.MultiTenantServiceAPIKey = "test-api-key"

	err := cfg.Validate()
	require.NoError(t, err)
}

func TestConfig_Validate_ProductionRequiresSecureRabbitMQScheme(t *testing.T) {
	t.Parallel()

	cfg := validWorkerConfig()
	cfg.EnvName = "production"
	cfg.EnableTelemetry = true
	cfg.MongoDBPassword = "mongo-secret"
	cfg.RabbitMQPass = "rabbit-secret"
	cfg.ObjectStorageSecretKey = "object-storage-secret"
	cfg.CryptoHashSecretKeyPluginCRM = "hash-secret"
	cfg.CryptoEncryptSecretKeyPluginCRM = "encrypt-secret"
	cfg.RabbitURI = "amqp"

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RABBITMQ_URI must use AMQPS in production")
}

// productionInsecureTLSWorkerConfig builds a production multi-tenant Worker
// Config that satisfies all non-TLS production requirements but runs every
// dependency over plaintext (REDIS_TLS=false, OBJECT_STORAGE_DISABLE_SSL=true,
// amqp:// RabbitMQ URI). Used by the ALLOW_INSECURE_TLS opt-out tests.
func productionInsecureTLSWorkerConfig() *Config {
	cfg := validWorkerConfig()
	cfg.EnvName = "production"
	cfg.EnableTelemetry = true
	cfg.MongoDBPassword = "mongo-secret"
	cfg.RabbitMQPass = "rabbit-secret"
	cfg.ObjectStorageSecretKey = "object-storage-secret"
	cfg.CryptoHashSecretKeyPluginCRM = "hash-secret"
	cfg.CryptoEncryptSecretKeyPluginCRM = "encrypt-secret"
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "https://tenant-manager.internal"
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 30
	cfg.MultiTenantServiceAPIKey = "test-api-key"
	cfg.RedisHost = "redis:6379"
	cfg.RedisPassword = "redis-secret"
	// HTTPS endpoint: the OBJECT_STORAGE_ENDPOINT scheme check is independent
	// of the ALLOW_INSECURE_TLS toggle, so keep it valid to isolate the
	// toggle-gated TLS checks.
	cfg.ObjectStorageEndpoint = "https://storage.example.com"
	// Plaintext dependencies gated by ALLOW_INSECURE_TLS.
	cfg.RabbitURI = "amqp"
	cfg.RedisTLS = false
	cfg.ObjectStorageDisableSSL = true
	cfg.MultiTenantRedisHost = "mt-redis:6379"
	cfg.MultiTenantRedisTLS = false

	return cfg
}

// TestConfig_Validate_ProductionTLSEnforcedWhenAllowInsecureTLSFalse verifies
// that production TLS requirements still fail validation when ALLOW_INSECURE_TLS
// is unset/false (secure default).
func TestConfig_Validate_ProductionTLSEnforcedWhenAllowInsecureTLSFalse(t *testing.T) {
	t.Parallel()

	cfg := productionInsecureTLSWorkerConfig()
	cfg.AllowInsecureTLS = false

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "REDIS_TLS must be true in production when MULTI_TENANT_ENABLED=true")
	assert.Contains(t, err.Error(), "MULTI_TENANT_REDIS_TLS must be true in production")
	assert.Contains(t, err.Error(), "OBJECT_STORAGE_DISABLE_SSL must be false in production")
	assert.Contains(t, err.Error(), "RABBITMQ_URI must use AMQPS in production")
}

// TestConfig_Validate_ProductionTLSBypassedWhenAllowInsecureTLSTrue verifies
// that ALLOW_INSECURE_TLS=true bypasses every TLS-related production requirement.
func TestConfig_Validate_ProductionTLSBypassedWhenAllowInsecureTLSTrue(t *testing.T) {
	t.Parallel()

	cfg := productionInsecureTLSWorkerConfig()
	cfg.AllowInsecureTLS = true

	err := cfg.Validate()
	require.NoError(t, err, "ALLOW_INSECURE_TLS=true should bypass production TLS enforcement")
}

// TestConfig_Validate_NonTLSProductionChecksStillEnforcedWithAllowInsecureTLS
// verifies that ALLOW_INSECURE_TLS only relaxes TLS checks — non-TLS production
// requirements (telemetry, secrets, REDIS_PASSWORD) stay enforced.
func TestConfig_Validate_NonTLSProductionChecksStillEnforcedWithAllowInsecureTLS(t *testing.T) {
	t.Parallel()

	cfg := productionInsecureTLSWorkerConfig()
	cfg.AllowInsecureTLS = true
	cfg.EnableTelemetry = false
	cfg.RedisPassword = ""

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ENABLE_TELEMETRY must be true in production")
	assert.Contains(t, err.Error(), "REDIS_PASSWORD must not be empty in production")
}

// TestEnforceWorkerSaaSTLS_SkippedWhenAllowInsecureTLSTrue verifies that the
// Gate-4 SaaS TLS enforcement is skipped when ALLOW_INSECURE_TLS=true.
func TestEnforceWorkerSaaSTLS_SkippedWhenAllowInsecureTLSTrue(t *testing.T) {
	t.Parallel()

	cfg := validWorkerConfig()
	cfg.DeploymentMode = "saas"
	cfg.RabbitURI = "amqp://reporter-user:secret@rabbitmq.example.com:5672/"
	cfg.RedisTLS = false
	cfg.RedisHost = "valkey.example.com:6379"
	cfg.ObjectStorageEndpoint = "http://seaweedfs:8333"

	// Sanity: without the flag, enforcement rejects this plaintext SaaS config.
	cfg.AllowInsecureTLS = false
	require.Error(t, enforceWorkerSaaSTLS(cfg),
		"plaintext SaaS config must be rejected when ALLOW_INSECURE_TLS=false")

	// With the flag, enforcement is skipped entirely.
	cfg.AllowInsecureTLS = true
	require.NoError(t, enforceWorkerSaaSTLS(cfg),
		"ALLOW_INSECURE_TLS=true should skip SaaS TLS enforcement")
}
