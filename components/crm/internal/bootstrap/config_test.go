// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"reflect"
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
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
			name:      "has MultiTenantCacheTTL int field",
			fieldName: "MultiTenantCacheTTL",
			fieldType: "int",
			envTag:    "MULTI_TENANT_CACHE_TTL",
		},
		{
			name:      "has MultiTenantCacheSize int field",
			fieldName: "MultiTenantCacheSize",
			fieldType: "int",
			envTag:    "MULTI_TENANT_CACHE_SIZE",
		},
		{
			name:      "has MultiTenantRetryMax int field",
			fieldName: "MultiTenantRetryMax",
			fieldType: "int",
			envTag:    "MULTI_TENANT_RETRY_MAX",
		},
		{
			name:      "has MultiTenantRetryDelay int field",
			fieldName: "MultiTenantRetryDelay",
			fieldType: "int",
			envTag:    "MULTI_TENANT_RETRY_DELAY",
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
	assert.Zero(t, cfg.MultiTenantCacheTTL,
		"MultiTenantCacheTTL must default to zero (zero value)")
	assert.Zero(t, cfg.MultiTenantCacheSize,
		"MultiTenantCacheSize must default to zero (zero value)")
	assert.Zero(t, cfg.MultiTenantRetryMax,
		"MultiTenantRetryMax must default to zero (zero value)")
	assert.Zero(t, cfg.MultiTenantRetryDelay,
		"MultiTenantRetryDelay must default to zero (zero value)")
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
				MultiTenantEnabled: true,
				MultiTenantURL:     "http://tenant-manager:8080",
			},
			expectNil: false,
		},
		{
			name: "returns non-nil middleware with all config options set",
			cfg: &Config{
				MultiTenantEnabled:  true,
				MultiTenantURL:      "http://tenant-manager:8080",
				MultiTenantTimeout:  30,
				MultiTenantCacheTTL: 300,
				MultiTenantRetryMax: 3,
			},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := newMockLogger()

			mw, err := initTenantMiddleware(tt.cfg, logger)

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
				MultiTenantEnabled: true,
				MultiTenantURL:     tt.url,
			}
			logger := newMockLogger()

			mw, err := initTenantMiddleware(cfg, logger)

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

			mw, err := initTenantMiddleware(cfg, logger)

			require.NoError(t, err,
				"initTenantMiddleware must not error when disabled, regardless of URL value")
			assert.Nil(t, mw,
				"initTenantMiddleware must return nil when disabled")
		})
	}
}

// mockLogger implements libLog.Logger for testing.
type mockLogger struct{}

func newMockLogger() *mockLogger { return &mockLogger{} }

func (m *mockLogger) Info(args ...any)                                  {}
func (m *mockLogger) Infof(format string, args ...any)                  {}
func (m *mockLogger) Infoln(args ...any)                                {}
func (m *mockLogger) Error(args ...any)                                 {}
func (m *mockLogger) Errorf(format string, args ...any)                 {}
func (m *mockLogger) Errorln(args ...any)                               {}
func (m *mockLogger) Warn(args ...any)                                  {}
func (m *mockLogger) Warnf(format string, args ...any)                  {}
func (m *mockLogger) Warnln(args ...any)                                {}
func (m *mockLogger) Debug(args ...any)                                 {}
func (m *mockLogger) Debugf(format string, args ...any)                 {}
func (m *mockLogger) Debugln(args ...any)                               {}
func (m *mockLogger) Fatal(args ...any)                                 {}
func (m *mockLogger) Fatalf(format string, args ...any)                 {}
func (m *mockLogger) Fatalln(args ...any)                               {}
func (m *mockLogger) WithFields(fields ...any) libLog.Logger            { return m }
func (m *mockLogger) WithDefaultMessageTemplate(s string) libLog.Logger { return m }
func (m *mockLogger) Sync() error                                       { return nil }
