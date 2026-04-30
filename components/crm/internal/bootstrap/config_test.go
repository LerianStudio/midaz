// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"net/http/httptest"
	"reflect"
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_MultiTenantFields(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		fieldType string
		envTag    string
	}{
		{
			name:      "has MultiTenantEnabled bool field",
			fieldName: "MultiTenantEnabled",
			fieldType: "bool",
			envTag:    "MULTI_TENANT_ENABLED",
		},
		{
			name:      "has MultiTenantURL string field",
			fieldName: "MultiTenantURL",
			fieldType: "string",
			envTag:    "MULTI_TENANT_URL",
		},
		{
			name:      "has MultiTenantTimeout int field",
			fieldName: "MultiTenantTimeout",
			fieldType: "int",
			envTag:    "MULTI_TENANT_TIMEOUT",
		},
		{
			name:      "has MultiTenantIdleTimeoutSec int field",
			fieldName: "MultiTenantIdleTimeoutSec",
			fieldType: "int",
			envTag:    "MULTI_TENANT_IDLE_TIMEOUT_SEC",
		},
		{
			name:      "has MultiTenantMaxTenantPools int field",
			fieldName: "MultiTenantMaxTenantPools",
			fieldType: "int",
			envTag:    "MULTI_TENANT_MAX_TENANT_POOLS",
		},
		{
			name:      "has MultiTenantCircuitBreakerThreshold int field",
			fieldName: "MultiTenantCircuitBreakerThreshold",
			fieldType: "int",
			envTag:    "MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD",
		},
		{
			name:      "has MultiTenantCircuitBreakerTimeoutSec int field",
			fieldName: "MultiTenantCircuitBreakerTimeoutSec",
			fieldType: "int",
			envTag:    "MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC",
		},
	}

	configType := reflect.TypeOf(Config{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, found := configType.FieldByName(tt.fieldName)
			require.True(t, found, "Config struct must have field %s", tt.fieldName)

			assert.Equal(t, tt.fieldType, field.Type.String(),
				"field %s must be of type %s", tt.fieldName, tt.fieldType)

			envValue := field.Tag.Get("env")
			assert.Equal(t, tt.envTag, envValue,
				"field %s must have env tag %q", tt.fieldName, tt.envTag)
		})
	}
}

func TestConfig_MultiTenantDefaults(t *testing.T) {
	cfg := &Config{}

	assert.False(t, cfg.MultiTenantEnabled,
		"MultiTenantEnabled must default to false (zero value)")
	assert.Empty(t, cfg.MultiTenantURL,
		"MultiTenantURL must default to empty string (zero value)")
	assert.Zero(t, cfg.MultiTenantTimeout,
		"MultiTenantTimeout must default to zero (zero value)")
	assert.Zero(t, cfg.MultiTenantIdleTimeoutSec,
		"MultiTenantIdleTimeoutSec must default to zero (zero value)")
	assert.Zero(t, cfg.MultiTenantMaxTenantPools,
		"MultiTenantMaxTenantPools must default to zero (zero value)")
	assert.Zero(t, cfg.MultiTenantCircuitBreakerThreshold,
		"MultiTenantCircuitBreakerThreshold must default to zero (zero value)")
	assert.Zero(t, cfg.MultiTenantCircuitBreakerTimeoutSec,
		"MultiTenantCircuitBreakerTimeoutSec must default to zero (zero value)")
}

func TestInitTenantMiddleware(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *Config
		expectNil    bool
		expectErrMsg string
	}{
		{
			name: "returns nil when multi-tenant disabled",
			cfg: &Config{
				MultiTenantEnabled: false,
			},
			expectNil: true,
		},
		{
			name: "returns error when enabled but URL is empty",
			cfg: &Config{
				MultiTenantEnabled: true,
				MultiTenantURL:     "",
			},
			expectNil:    true,
			expectErrMsg: "MULTI_TENANT_URL must not be blank",
		},
		{
			name: "returns error when enabled but URL is whitespace only",
			cfg: &Config{
				MultiTenantEnabled: true,
				MultiTenantURL:     "   ",
			},
			expectNil:    true,
			expectErrMsg: "MULTI_TENANT_URL must not be blank",
		},
		{
			name: "returns non-nil middleware when enabled with valid URL",
			cfg: &Config{
				MultiTenantEnabled:       true,
				EnvName:                  "development",
				MultiTenantURL:           "http://tenant-manager:8080",
				MultiTenantServiceAPIKey: "test-api-key",
				ApplicationName:          "ledger",
			},
			expectNil: false,
		},
		{
			name: "returns non-nil middleware with all config options set",
			cfg: &Config{
				MultiTenantEnabled:                 true,
				EnvName:                            "development",
				MultiTenantURL:                     "http://tenant-manager:8080",
				MultiTenantTimeout:                 30,
				MultiTenantIdleTimeoutSec:          300,
				MultiTenantCircuitBreakerThreshold: 3,
				MultiTenantServiceAPIKey:           "test-api-key",
				ApplicationName:                    "ledger",
			},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := newMockLogger()

			mw, _, err := initTenantMiddleware(tt.cfg, logger, nil)

			if tt.expectErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErrMsg)

				return
			}

			require.NoError(t, err)

			if tt.expectNil {
				assert.Nil(t, mw,
					"middleware must be nil when multi-tenant is disabled or URL is empty")
			} else {
				assert.NotNil(t, mw,
					"middleware must be non-nil when multi-tenant is enabled with valid URL")
			}
		})
	}
}

// =============================================================================
// Config Validation — Edge Cases
// =============================================================================

func TestInitTenantMiddleware_URLWhitespaceVariations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		url          string
		expectNil    bool
		expectErrMsg string
	}{
		{
			name:         "tab_only_url_returns_error",
			url:          "\t",
			expectNil:    true,
			expectErrMsg: "MULTI_TENANT_URL must not be blank",
		},
		{
			name:         "newline_only_url_returns_error",
			url:          "\n",
			expectNil:    true,
			expectErrMsg: "MULTI_TENANT_URL must not be blank",
		},
		{
			name:         "mixed_whitespace_url_returns_error",
			url:          " \t\n ",
			expectNil:    true,
			expectErrMsg: "MULTI_TENANT_URL must not be blank",
		},
		{
			name:      "url_with_leading_trailing_spaces_succeeds",
			url:       "  http://tenant-manager:8080  ",
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				MultiTenantEnabled:       true,
				EnvName:                  "development",
				MultiTenantURL:           tt.url,
				MultiTenantServiceAPIKey: "test-api-key",
				ApplicationName:          "ledger",
			}
			logger := newMockLogger()

			mw, _, err := initTenantMiddleware(cfg, logger, nil)

			if tt.expectErrMsg != "" {
				require.Error(t, err, "initTenantMiddleware should return error for whitespace-only URL")
				assert.Contains(t, err.Error(), tt.expectErrMsg)
				assert.Nil(t, mw, "middleware must be nil when URL validation fails")

				return
			}

			require.NoError(t, err, "initTenantMiddleware should not error for URL with content")

			if tt.expectNil {
				assert.Nil(t, mw, "middleware must be nil")
			} else {
				assert.NotNil(t, mw, "middleware must be non-nil for URL with content")
			}
		})
	}
}

func TestInitTenantMiddleware_DisabledIgnoresInvalidURL(t *testing.T) {
	t.Parallel()

	// When multi-tenant is disabled, invalid URLs should be ignored entirely.
	// The function should return nil without attempting URL validation.
	tests := []struct {
		name string
		url  string
	}{
		{
			name: "disabled_with_whitespace_url",
			url:  "   ",
		},
		{
			name: "disabled_with_tab_url",
			url:  "\t\n",
		},
		{
			name: "disabled_with_invalid_url",
			url:  "not-a-valid-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				MultiTenantEnabled: false,
				MultiTenantURL:     tt.url,
			}
			logger := newMockLogger()

			mw, _, err := initTenantMiddleware(cfg, logger, nil)

			require.NoError(t, err,
				"initTenantMiddleware must not error when disabled, regardless of URL value")
			assert.Nil(t, mw,
				"initTenantMiddleware must return nil when disabled")
		})
	}
}

// =============================================================================
// Metrics Instrumentation Tests
// =============================================================================

func TestInitTenantMiddleware_MetricsEmission(t *testing.T) {
	t.Parallel()

	t.Run("nil_telemetry_does_not_panic", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			MultiTenantEnabled:       true,
			EnvName:                  "development",
			MultiTenantURL:           "http://tenant-manager:8080",
			MultiTenantServiceAPIKey: "test-api-key",
			ApplicationName:          "ledger",
		}
		logger := newMockLogger()

		// Passing nil telemetry must not panic; metrics are no-op.
		mw, _, err := initTenantMiddleware(cfg, logger, nil)

		require.NoError(t, err)
		assert.NotNil(t, mw,
			"middleware must be non-nil even when telemetry is nil")
	})

	t.Run("nil_telemetry_with_disabled_does_not_panic", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			MultiTenantEnabled: false,
		}
		logger := newMockLogger()

		// When disabled, nil telemetry must be perfectly safe (early return path).
		mw, _, err := initTenantMiddleware(cfg, logger, nil)

		require.NoError(t, err)
		assert.Nil(t, mw,
			"middleware must be nil when multi-tenant is disabled")
	})

	t.Run("telemetry_with_nil_metrics_factory_does_not_panic", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			MultiTenantEnabled:       true,
			EnvName:                  "development",
			MultiTenantURL:           "http://tenant-manager:8080",
			MultiTenantServiceAPIKey: "test-api-key",
			ApplicationName:          "ledger",
		}
		logger := newMockLogger()

		// Telemetry struct with nil MetricsFactory must be safely guarded.
		telemetry := &libOpentelemetry.Telemetry{}

		mw, _, err := initTenantMiddleware(cfg, logger, telemetry)

		require.NoError(t, err)
		assert.NotNil(t, mw,
			"middleware must be non-nil even when MetricsFactory is nil")
	})
}

func TestBuildTenantClientOptions_RejectsHTTPOutsideDevelopment(t *testing.T) {
	t.Parallel()

	_, err := buildTenantClientOptions(&Config{EnvName: "production"}, "http://tenant-manager:8080")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must use https")
}

func TestBuildTenantClientOptions_AllowsHTTPInSafeEnvironments(t *testing.T) {
	t.Parallel()

	for _, envName := range []string{"local", "development", "dev", "test", "testing"} {
		t.Run(envName, func(t *testing.T) {
			t.Parallel()

			opts, err := buildTenantClientOptions(&Config{EnvName: envName}, "http://tenant-manager:8080")
			require.NoError(t, err)
			assert.NotEmpty(t, opts)
		})
	}
}

func TestBuildTenantClientOptions_InvalidURLReturnsError(t *testing.T) {
	t.Parallel()

	_, err := buildTenantClientOptions(&Config{EnvName: "development"}, "://bad-url")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid MULTI_TENANT_URL")
}

func TestBuildTenantClientOptions_RejectsRelativeURL(t *testing.T) {
	t.Parallel()

	_, err := buildTenantClientOptions(&Config{EnvName: "development"}, "tenant-manager:8080")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute URL")
}

func TestBuildTenantClientOptions_RejectsUnsupportedScheme(t *testing.T) {
	t.Parallel()

	_, err := buildTenantClientOptions(&Config{EnvName: "development"}, "ftp://tenant-manager:8080")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scheme must be http or https")
}

func TestAllowInsecureTenantManagerHTTP(t *testing.T) {
	t.Parallel()

	assert.True(t, allowInsecureTenantManagerHTTP(" local "))
	assert.True(t, allowInsecureTenantManagerHTTP("DEV"))
	assert.True(t, allowInsecureTenantManagerHTTP("testing"))
	assert.False(t, allowInsecureTenantManagerHTTP("production"))
	assert.True(t, allowInsecureTenantManagerHTTP("staging"))
}

func TestRedactedTenantManagerURL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "https://user:xxxxx@example.com", redactedTenantManagerURL("https://user:secret@example.com"))
	assert.Equal(t, "https://user:xxxxx@example.com/path", redactedTenantManagerURL("https://user:secret@example.com/path?token=secret#frag"))
	assert.Equal(t, "invalid-url", redactedTenantManagerURL("://bad-url"))
}

func TestWrapTenantMiddlewareWithMetrics_PreservesError(t *testing.T) {
	t.Parallel()

	expectedErr := fiber.ErrUnauthorized
	handler := wrapTenantMiddlewareWithMetrics(func(_ *fiber.Ctx) error { return expectedErr }, nil, newMockLogger())
	app := fiber.New()
	app.Get("/", handler)

	req := httptest.NewRequest(fiber.MethodGet, "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestResolveMongoURI_LegacySchemeValueBuildsFullURI(t *testing.T) {
	t.Parallel()

	uri, err := resolveMongoURI(&Config{
		MongoURI:        "mongodb",
		MongoDBHost:     "midaz-mongodb",
		MongoDBUser:     "midaz",
		MongoDBPassword: "lerian",
	}, "5703", "")
	require.NoError(t, err)
	assert.Contains(t, uri, "mongodb://")
	assert.Contains(t, uri, "midaz-mongodb")
	assert.NotContains(t, uri, "/crm")
}

func TestResolveMongoURI_InvalidLegacyValueReturnsError(t *testing.T) {
	t.Parallel()

	_, err := resolveMongoURI(&Config{MongoURI: "mongodb-invalid"}, "5703", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid MONGO_URI format")
}

// mockLogger implements libLog.Logger for testing.
type mockLogger struct{}

func newMockLogger() *mockLogger { return &mockLogger{} }

func (m *mockLogger) Log(_ context.Context, _ libLog.Level, _ string, _ ...libLog.Field) {}
func (m *mockLogger) With(_ ...libLog.Field) libLog.Logger                               { return m }
func (m *mockLogger) WithGroup(_ string) libLog.Logger                                   { return m }
func (m *mockLogger) Enabled(_ libLog.Level) bool                                        { return true }
func (m *mockLogger) Sync(_ context.Context) error                                       { return nil }
