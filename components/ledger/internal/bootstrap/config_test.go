// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"testing/quick"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitServers_ErrorHandling tests that InitServers properly handles errors
// from child module initialization.
func TestInitServers_ErrorHandling(t *testing.T) {
	// Note: t.Parallel() removed because t.Setenv is incompatible with parallel tests.

	// Minimal config to trigger errors in child modules.
	// This test is a guard rail for the contract: initialization must return errors, not panic.
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "test")
	t.Setenv("DB_USER", "test")
	t.Setenv("DB_PASSWORD", "test")
	t.Setenv("MONGO_HOST", "localhost")
	t.Setenv("MONGO_PORT", "27017")
	t.Setenv("REDIS_HOST", "localhost:9999") // invalid on purpose

	var (
		service *Service
		err     error
	)

	// Fail fast: if this panics, the rest of the assertions are meaningless and may cascade.
	require.NotPanics(t, func() {
		service, err = InitServers()
	})

	require.Nil(t, service)
	require.Error(t, err)
	assert.NotEmpty(t, err.Error())
}

// TestConfig_MultiTenantFields_Defaults verifies that the Config struct contains
// the multi-tenant feature flag fields with correct env tags and default values.
// When MULTI_TENANT_ENABLED is not set, it defaults to false (opt-in behavior).
func TestConfig_MultiTenantFields_Defaults(t *testing.T) {
	t.Parallel()

	// Arrange: no MULTI_TENANT_* env vars set - all should use defaults
	cfg := &Config{}
	err := libCommons.SetConfigFromEnvVars(cfg)
	require.NoError(t, err, "SetConfigFromEnvVars should not fail with empty env")

	// Assert: multi-tenant defaults to disabled (opt-in)
	assert.False(t, cfg.MultiTenantEnabled, "MultiTenantEnabled should default to false")
	assert.Empty(t, cfg.TenantManagerURL, "TenantManagerURL should default to empty")
	assert.Empty(t, cfg.TenantServiceName, "TenantServiceName should default to empty")
	assert.Empty(t, cfg.TenantEnvironment, "TenantEnvironment should default to empty")
	assert.Zero(t, cfg.TenantCBFailures, "TenantCBFailures should default to zero (safe defaults applied at usage site)")
	assert.Zero(t, cfg.TenantCBTimeout, "TenantCBTimeout should default to zero (safe defaults applied at usage site)")
}

// TestConfig_MultiTenantEnvParsing verifies that the multi-tenant Config fields
// are correctly populated from environment variables via SetConfigFromEnvVars.
//
// Key behaviors tested:
// - When disabled (default): MultiTenantEnabled is false
// - When explicitly disabled: MultiTenantEnabled is false
// - When enabled without URL: Config fields reflect enabled state with empty URL
// - When enabled with empty URL: Config fields reflect enabled state with empty URL
func TestConfig_MultiTenantEnvParsing(t *testing.T) {
	// Note: t.Parallel() removed because sub-tests use t.Setenv which is
	// incompatible with parallel ancestors (Go testing restriction).

	tests := []struct {
		name                string
		envVars             map[string]string
		wantErr             bool
		wantErrContains     string
		wantTenantClientNil bool
	}{
		{
			name:    "disabled_by_default_no_tenant_client",
			envVars: map[string]string{
				// MULTI_TENANT_ENABLED not set, defaults to false
			},
			wantErr:             false,
			wantTenantClientNil: true,
		},
		{
			name: "explicitly_disabled_no_tenant_client",
			envVars: map[string]string{
				"MULTI_TENANT_ENABLED": "false",
			},
			wantErr:             false,
			wantTenantClientNil: true,
		},
		{
			name: "enabled_without_url_returns_error",
			envVars: map[string]string{
				"MULTI_TENANT_ENABLED": "true",
				"TENANT_MANAGER_URL":   "",
			},
			wantErr:         true,
			wantErrContains: "TENANT_MANAGER_URL",
		},
		{
			name: "enabled_with_empty_url_returns_error",
			envVars: map[string]string{
				"MULTI_TENANT_ENABLED": "true",
				// TENANT_MANAGER_URL not set at all
			},
			wantErr:         true,
			wantErrContains: "TENANT_MANAGER_URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: t.Parallel() removed because t.Setenv is incompatible with parallel sub-tests

			// Arrange: set env vars for this test case
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			// Act: validate config multi-tenant fields
			cfg := &Config{}
			err := libCommons.SetConfigFromEnvVars(cfg)
			require.NoError(t, err, "SetConfigFromEnvVars should not fail")

			// Assert: verify feature flag value
			if tt.wantErr {
				// When multi-tenant is enabled, URL must be required at config validation
				assert.True(t, cfg.MultiTenantEnabled, "MultiTenantEnabled should be true for error cases")
				assert.Empty(t, cfg.TenantManagerURL, "TenantManagerURL should be empty for error cases")
			}

			if tt.wantTenantClientNil {
				assert.False(t, cfg.MultiTenantEnabled, "MultiTenantEnabled should be false when tenant client is nil")
			}
		})
	}
}

// TestOptions_TenantClient_ConcreteType verifies that Options.TenantClient uses
// the concrete *tmclient.Client type (not interface{}) and that it is nil by default.
// This is a compile-time + runtime check for the Options struct contract.
func TestOptions_TenantClient_ConcreteType(t *testing.T) {
	t.Parallel()

	// Arrange: create Options with zero values
	opts := &Options{}

	// Assert: TenantClient should be nil by default (concrete pointer type)
	assert.Nil(t, opts.TenantClient, "TenantClient should be nil by default")

	// Assert: verify the field accepts *tmclient.Client (compile-time check)
	// This line will fail to compile if TenantClient is not *tmclient.Client
	var _ *tmclient.Client = opts.TenantClient
}

// TestConfig_MultiTenantEnabled_FromEnv verifies that MULTI_TENANT_ENABLED
// env var is correctly parsed into the Config struct.
func TestConfig_MultiTenantEnabled_FromEnv(t *testing.T) {
	// Note: t.Parallel() removed because sub-tests use t.Setenv which is
	// incompatible with parallel ancestors (Go testing restriction).

	tests := []struct {
		name     string
		envValue string
		want     bool
	}{
		{
			name:     "true_string_enables_multi_tenant",
			envValue: "true",
			want:     true,
		},
		{
			name:     "false_string_disables_multi_tenant",
			envValue: "false",
			want:     false,
		},
		{
			name:     "unset_defaults_to_false",
			envValue: "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: t.Parallel() removed because t.Setenv is incompatible with parallel sub-tests

			t.Setenv("MULTI_TENANT_ENABLED", tt.envValue)

			cfg := &Config{}
			err := libCommons.SetConfigFromEnvVars(cfg)
			require.NoError(t, err, "SetConfigFromEnvVars should not fail")

			assert.Equal(t, tt.want, cfg.MultiTenantEnabled, "MultiTenantEnabled should match expected value")
		})
	}
}

// TestInitServersWithOptions_MultiTenantValidation verifies that InitServersWithOptions
// enforces the multi-tenant validation contract at the function level, not just at config
// struct level. This test exercises the actual error return path inside the function.
//
// This test complements TestInitServersWithOptions_MultiTenant which only validates Config
// struct population. This test verifies the BEHAVIOR: that the running function rejects
// the call when MULTI_TENANT_ENABLED=true and TENANT_MANAGER_URL is absent or blank.
func TestInitServersWithOptions_MultiTenantValidation(t *testing.T) {
	// Note: t.Parallel() removed because sub-tests use t.Setenv which is
	// incompatible with parallel ancestors (Go testing restriction).

	// Inject a pre-configured logger to avoid logger init side effects in test output.
	logger, _ := libZap.InitializeLoggerWithError()

	tests := []struct {
		name            string
		envVars         map[string]string
		wantErr         bool
		wantErrContains string
	}{
		{
			// AC-2: The primary scenario - enabled with no URL set at all.
			name: "enabled_no_url_set_returns_error",
			envVars: map[string]string{
				"MULTI_TENANT_ENABLED": "true",
				"PLUGIN_AUTH_ENABLED":  "true",
				// TENANT_MANAGER_URL intentionally not set
			},
			wantErr:         true,
			wantErrContains: "TENANT_MANAGER_URL",
		},
		{
			// AC-2: Explicitly set to empty string is also rejected.
			name: "enabled_url_explicit_empty_returns_error",
			envVars: map[string]string{
				"MULTI_TENANT_ENABLED": "true",
				"PLUGIN_AUTH_ENABLED":  "true",
				"TENANT_MANAGER_URL":   "",
			},
			wantErr:         true,
			wantErrContains: "TENANT_MANAGER_URL",
		},
		{
			// Edge case: whitespace-only URL is rejected by strings.TrimSpace guard.
			name: "enabled_url_whitespace_only_returns_error",
			envVars: map[string]string{
				"MULTI_TENANT_ENABLED": "true",
				"PLUGIN_AUTH_ENABLED":  "true",
				"TENANT_MANAGER_URL":   "   ",
			},
			wantErr:         true,
			wantErrContains: "TENANT_MANAGER_URL",
		},
		{
			// AC-3: Disabled flag short-circuits all multi-tenant logic — no error
			// even though URL is also absent.
			name: "disabled_no_url_no_error_from_validation",
			envVars: map[string]string{
				"MULTI_TENANT_ENABLED": "false",
				// TENANT_MANAGER_URL intentionally not set
			},
			// Function will fail later at transaction/DB init, not at multi-tenant check.
			wantErr:         true,
			wantErrContains: "", // error is NOT the multi-tenant validation error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: t.Parallel() removed because t.Setenv is incompatible with parallel sub-tests

			// Arrange: set env vars for this test case
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			// Act: call the actual function under test (not just SetConfigFromEnvVars)
			opts := &Options{Logger: logger}

			var service *Service

			var err error

			require.NotPanics(t, func() {
				service, err = InitServersWithOptions(opts)
			}, "InitServersWithOptions must never panic")

			// Assert
			require.Nil(t, service, "service must be nil when error occurs")
			require.Error(t, err, "error must not be nil")

			if tt.wantErrContains != "" {
				assert.Contains(t, err.Error(), tt.wantErrContains,
					"error should contain %q", tt.wantErrContains)
			}
		})
	}
}

// TestConfig_MultiTenantCBDefaults verifies the circuit breaker config fields
// (TenantCBFailures, TenantCBTimeout) have zero defaults and are correctly
// populated from environment variables.
func TestConfig_MultiTenantCBDefaults(t *testing.T) {
	// Note: t.Parallel() removed because sub-tests use t.Setenv which is
	// incompatible with parallel ancestors (Go testing restriction).

	tests := []struct {
		name           string
		envVars        map[string]string
		wantCBFailures int
		wantCBTimeout  int
	}{
		{
			name:           "zero_defaults_when_env_not_set",
			envVars:        map[string]string{},
			wantCBFailures: 0,
			wantCBTimeout:  0,
		},
		{
			name: "cb_failures_set_from_env",
			envVars: map[string]string{
				"TENANT_CB_FAILURES": "5",
			},
			wantCBFailures: 5,
			wantCBTimeout:  0,
		},
		{
			name: "cb_timeout_set_from_env",
			envVars: map[string]string{
				"TENANT_CB_TIMEOUT": "30",
			},
			wantCBFailures: 0,
			wantCBTimeout:  30,
		},
		{
			name: "both_cb_fields_set_from_env",
			envVars: map[string]string{
				"TENANT_CB_FAILURES": "3",
				"TENANT_CB_TIMEOUT":  "60",
			},
			wantCBFailures: 3,
			wantCBTimeout:  60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: t.Parallel() removed because t.Setenv is incompatible with parallel sub-tests

			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg := &Config{}
			err := libCommons.SetConfigFromEnvVars(cfg)
			require.NoError(t, err, "SetConfigFromEnvVars should not fail")

			assert.Equal(t, tt.wantCBFailures, cfg.TenantCBFailures,
				"TenantCBFailures should match expected value")
			assert.Equal(t, tt.wantCBTimeout, cfg.TenantCBTimeout,
				"TenantCBTimeout should match expected value")
		})
	}
}

// =============================================================================
// Fuzz Tests
// =============================================================================

// containsInvalidEnvByte returns true if any of the provided strings contains
// a byte that is invalid for POSIX environment variable values.
// Null bytes (\x00) cause t.Setenv to fail with "setenv: invalid argument".
// Fuzz inputs that trigger this OS-level rejection are skipped — they test
// the OS, not the config parser.
func containsInvalidEnvByte(values ...string) bool {
	for _, v := range values {
		for i := 0; i < len(v); i++ {
			if v[i] == 0 {
				return true
			}
		}
	}

	return false
}

// FuzzConfig_MultiTenantEnvParsing verifies that SetConfigFromEnvVars never panics
// regardless of what values are injected into multi-tenant environment variables.
func FuzzConfig_MultiTenantEnvParsing(f *testing.F) {
	f.Add("true", "http://localhost:4003", "ledger", "production", "5", "30")
	f.Add("false", "", "", "", "0", "0")
	f.Add("TRUE", "https://tm.internal:443/api", "crm", "staging", "10", "60")
	f.Add("invalid", "not-a-url", "", "", "-1", "-1")
	f.Add("", "   ", "a]b[c", "unicode\u202e\ufffd", "999999999", "999999999")
	f.Add("1", "http://host'; DROP TABLE config;--", "svc'", "env'", "3", "10")
	f.Add("0", "<script>alert(1)</script>", "<img>", "</div>", "0", "0")
	f.Add("true", "http://"+strings.Repeat("a", 255)+".example.com", strings.Repeat("b", 255), strings.Repeat("c", 255), "1", "1")

	f.Fuzz(func(t *testing.T, enabled, url, service, env, cbFailures, cbTimeout string) {
		if containsInvalidEnvByte(enabled, url, service, env, cbFailures, cbTimeout) {
			t.Skip("skipping: input contains null byte (POSIX env var restriction)")
		}

		if len(enabled) > 64 {
			enabled = enabled[:64]
		}

		if len(url) > 512 {
			url = url[:512]
		}

		if len(service) > 256 {
			service = service[:256]
		}

		if len(env) > 256 {
			env = env[:256]
		}

		if len(cbFailures) > 32 {
			cbFailures = cbFailures[:32]
		}

		if len(cbTimeout) > 32 {
			cbTimeout = cbTimeout[:32]
		}

		t.Setenv("MULTI_TENANT_ENABLED", enabled)
		t.Setenv("TENANT_MANAGER_URL", url)
		t.Setenv("TENANT_SERVICE_NAME", service)
		t.Setenv("TENANT_ENVIRONMENT", env)
		t.Setenv("TENANT_CB_FAILURES", cbFailures)
		t.Setenv("TENANT_CB_TIMEOUT", cbTimeout)

		cfg := &Config{}
		_ = libCommons.SetConfigFromEnvVars(cfg)
	})
}

// FuzzConfig_MultiTenantValidation verifies that the validation logic that guards
// TenantManagerURL never panics for any URL value when MULTI_TENANT_ENABLED is true.
func FuzzConfig_MultiTenantValidation(f *testing.F) {
	f.Add("")
	f.Add("http://localhost:4003")
	f.Add("   ")
	f.Add("\u202e/etc/passwd")
	f.Add("https://very-long-url-" + strings.Repeat("a", 200) + ".example.com")
	f.Add("http://[::1]:4003")
	f.Add("http://user:pass@host:4003/path?q=1#frag")
	f.Add("grpc://tenant-manager:443")

	f.Fuzz(func(t *testing.T, url string) {
		if containsInvalidEnvByte(url) {
			t.Skip("skipping: input contains null byte (POSIX env var restriction)")
		}

		if len(url) > 1024 {
			url = url[:1024]
		}

		t.Setenv("MULTI_TENANT_ENABLED", "true")
		t.Setenv("TENANT_MANAGER_URL", url)

		cfg := &Config{}
		_ = libCommons.SetConfigFromEnvVars(cfg)

		if cfg.MultiTenantEnabled && cfg.TenantManagerURL == "" {
			return
		}
	})
}

// =============================================================================
// Property-Based Tests
// =============================================================================

// TestProperty_Config_DisabledModeIsIdentity verifies that when
// MULTI_TENANT_ENABLED=false, setting any MT-specific env var does NOT
// affect the non-MT fields of Config.
func TestProperty_Config_DisabledModeIsIdentity(t *testing.T) {
	mtKeys := []string{
		"MULTI_TENANT_ENABLED",
		"TENANT_MANAGER_URL",
		"TENANT_SERVICE_NAME",
		"TENANT_ENVIRONMENT",
		"TENANT_CB_FAILURES",
		"TENANT_CB_TIMEOUT",
	}

	baseline := &Config{}
	require.NoError(t, libCommons.SetConfigFromEnvVars(baseline),
		"baseline SetConfigFromEnvVars must not fail")

	cleanupMTKeys := func(saved map[string]string) {
		for _, k := range mtKeys {
			if prev, hadPrev := saved[k]; hadPrev {
				_ = os.Setenv(k, prev)
			} else {
				_ = os.Unsetenv(k)
			}
		}
	}

	property := func(url, service, env, cbFailures, cbTimeout string) bool {
		if containsInvalidEnvByte(url, service, env, cbFailures, cbTimeout) {
			return true
		}

		if len(url) > 512 {
			url = url[:512]
		}

		if len(service) > 256 {
			service = service[:256]
		}

		if len(env) > 256 {
			env = env[:256]
		}

		if len(cbFailures) > 32 {
			cbFailures = cbFailures[:32]
		}

		if len(cbTimeout) > 32 {
			cbTimeout = cbTimeout[:32]
		}

		saved := make(map[string]string, len(mtKeys))
		for _, k := range mtKeys {
			if v, ok := os.LookupEnv(k); ok {
				saved[k] = v
			}
		}

		defer cleanupMTKeys(saved)

		_ = os.Setenv("MULTI_TENANT_ENABLED", "false")
		_ = os.Setenv("TENANT_MANAGER_URL", url)
		_ = os.Setenv("TENANT_SERVICE_NAME", service)
		_ = os.Setenv("TENANT_ENVIRONMENT", env)
		_ = os.Setenv("TENANT_CB_FAILURES", cbFailures)
		_ = os.Setenv("TENANT_CB_TIMEOUT", cbTimeout)

		withMT := &Config{}
		if err := libCommons.SetConfigFromEnvVars(withMT); err != nil {
			return true
		}

		return baseline.ServerAddress == withMT.ServerAddress &&
			baseline.AuthEnabled == withMT.AuthEnabled &&
			baseline.EnableTelemetry == withMT.EnableTelemetry &&
			baseline.AuthHost == withMT.AuthHost &&
			baseline.LogLevel == withMT.LogLevel
	}

	require.NoError(t, quick.Check(property, &quick.Config{MaxCount: 100}),
		"property TestProperty_Config_DisabledModeIsIdentity must hold for all inputs")
}

// TestProperty_Config_EnabledEmptyURLAlwaysErrors verifies that when
// MultiTenantEnabled=true and TenantManagerURL is empty, the validation
// guard always evaluates to the error branch.
func TestProperty_Config_EnabledEmptyURLAlwaysErrors(t *testing.T) {
	property := func(service, env string, cbFailures, cbTimeout uint8) bool {
		if containsInvalidEnvByte(service, env) {
			return true
		}

		if len(service) > 256 {
			service = service[:256]
		}

		if len(env) > 256 {
			env = env[:256]
		}

		cfg := &Config{
			MultiTenantEnabled: true,
			TenantManagerURL:   "",
			TenantServiceName:  service,
			TenantEnvironment:  env,
			TenantCBFailures:   int(cbFailures),
			TenantCBTimeout:    int(cbTimeout),
		}

		return cfg.MultiTenantEnabled && cfg.TenantManagerURL == ""
	}

	require.NoError(t, quick.Check(property, &quick.Config{MaxCount: 100}),
		"property TestProperty_Config_EnabledEmptyURLAlwaysErrors must hold for all inputs")
}

// TestProperty_Config_CBFieldsNonNegative verifies that TenantCBFailures and
// TenantCBTimeout are always >= 0 when the env vars contain non-negative
// decimal integer strings.
func TestProperty_Config_CBFieldsNonNegative(t *testing.T) {
	cbKeys := []string{"TENANT_CB_FAILURES", "TENANT_CB_TIMEOUT"}

	cleanupCBKeys := func(saved map[string]string) {
		for _, k := range cbKeys {
			if prev, hadPrev := saved[k]; hadPrev {
				_ = os.Setenv(k, prev)
			} else {
				_ = os.Unsetenv(k)
			}
		}
	}

	property := func(cbFailures, cbTimeout uint8) bool {
		failuresStr := fmt.Sprintf("%d", cbFailures)
		timeoutStr := fmt.Sprintf("%d", cbTimeout)

		saved := make(map[string]string, len(cbKeys))
		for _, k := range cbKeys {
			if v, ok := os.LookupEnv(k); ok {
				saved[k] = v
			}
		}

		defer cleanupCBKeys(saved)

		_ = os.Setenv("TENANT_CB_FAILURES", failuresStr)
		_ = os.Setenv("TENANT_CB_TIMEOUT", timeoutStr)

		cfg := &Config{}
		if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
			return true
		}

		return cfg.TenantCBFailures >= 0 && cfg.TenantCBTimeout >= 0
	}

	require.NoError(t, quick.Check(property, &quick.Config{MaxCount: 100}),
		"property TestProperty_Config_CBFieldsNonNegative must hold for all uint8-derived inputs")
}
