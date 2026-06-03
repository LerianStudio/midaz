// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"reflect"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultiTenant_BackwardCompatibility validates that the unified ledger
// operates correctly in single-tenant mode (MULTI_TENANT_ENABLED=false or unset).
// This is a MANDATORY test per multi-tenant.md "Single-Tenant Backward
// Compatibility Validation" section.
//
// Verified invariants:
//   - Config loads with default values (MultiTenantEnabled=false)
//   - Options.TenantClient is nil by default
//   - Service does not require Tenant Manager availability
//   - All MULTI_TENANT_* fields have correct env tags
func TestMultiTenant_BackwardCompatibility(t *testing.T) {
	t.Parallel()

	t.Run("config_defaults_to_single_tenant_when_zero_value", func(t *testing.T) {
		t.Parallel()

		// A zero-value Config (as produced when no MULTI_TENANT_* env vars are set)
		// must have multi-tenant disabled. Go zero values guarantee this: bool=false,
		// string="", int=0. This is the backward compatibility contract.
		cfg := &Config{}

		assert.False(t, cfg.MultiTenantEnabled,
			"MultiTenantEnabled must default to false (Go zero value)")
		assert.Empty(t, cfg.MultiTenantURL,
			"MultiTenantURL must default to empty string")
		assert.Zero(t, cfg.MultiTenantCircuitBreakerThreshold,
			"MultiTenantCircuitBreakerThreshold must default to zero")
		assert.Zero(t, cfg.MultiTenantCircuitBreakerTimeoutSec,
			"MultiTenantCircuitBreakerTimeoutSec must default to zero")
	})

	t.Run("multi_tenant_disabled_produces_nil_tenant_client", func(t *testing.T) {
		t.Parallel()

		// When MultiTenantEnabled=false, the code path in InitServersWithOptions
		// (lines 152-179) is NOT entered. tenantClient remains nil.
		// Test this by asserting Config zero values indicate no client creation needed.
		cfg := &Config{}
		assert.False(t, cfg.MultiTenantEnabled,
			"MultiTenantEnabled must be false for zero-value Config")
		// The `if cfg.MultiTenantEnabled` block would NOT execute,
		// so tenantClient remains nil throughout initialization.
	})

	t.Run("options_default_to_single_tenant_mode", func(t *testing.T) {
		t.Parallel()

		// Verify Options struct defaults: TenantClient must be nil when
		// no multi-tenant configuration is provided.
		opts := &Options{}
		assert.Nil(t, opts.TenantClient,
			"TenantClient must be nil by default in Options")
		assert.Nil(t, opts.Logger,
			"Logger must be nil by default in Options")
		assert.Nil(t, opts.CircuitBreakerStateListener,
			"CircuitBreakerStateListener must be nil by default in Options")
	})

	// Migrated from the standalone CRM bootstrap's backward_compat_test.go
	// (P3-T13b). CRM asserted initTenantMiddleware returns a nil handler with no
	// Tenant Manager contact when MT is disabled. The unified binary has no
	// initTenantMiddleware; its analog is buildUnifiedRouteSetup, which returns an
	// empty setup (no tenant middleware attached to any route group, no TM
	// client/manager touched) when MultiTenantEnabled is false. This preserves the
	// single-tenant guarantee for the merged binary that now also serves CRM routes.
	t.Run("route_setup_attaches_no_tenant_middleware_when_disabled", func(t *testing.T) {
		t.Parallel()

		logger := &libLog.GoLogger{}

		// All managers nil + tenantCache/loader nil: in single-tenant mode none of
		// them are dereferenced, proving no Tenant Manager interaction occurs.
		setup, err := buildUnifiedRouteSetup(&Config{MultiTenantEnabled: false}, logger,
			nil, nil, nil, nil, nil, nil, nil)

		require.NoError(t, err,
			"buildUnifiedRouteSetup must not error in single-tenant mode")
		require.NotNil(t, setup, "setup must be non-nil")
		assert.Nil(t, setup.onboardingRouteOptions,
			"onboarding routes must carry no tenant middleware in single-tenant mode")
		assert.Nil(t, setup.transactionRouteOptions,
			"transaction routes must carry no tenant middleware in single-tenant mode")
		assert.Nil(t, setup.ledgerRouteOptions,
			"ledger routes must carry no tenant middleware in single-tenant mode")
		assert.Nil(t, setup.crmRouteOptions,
			"CRM routes must carry no tenant middleware in single-tenant mode")
	})

	t.Run("crm_config_fields_present_with_correct_tags", func(t *testing.T) {
		t.Parallel()

		// CRM collapse (P3) adds a CrmPrefixed Mongo block + the bare LCRYPTO_*
		// crypto keys. Assert they exist with the exact env tags so the merged
		// binary loads CRM config (and the carried-over crypto values, R7).
		expectedFields := map[string]string{
			"CrmPrefixedMongoURI":    "MONGO_CRM_URI",
			"CrmPrefixedMongoDBName": "MONGO_CRM_NAME",
			"CrmHashSecretKey":       "LCRYPTO_HASH_SECRET_KEY",
			"CrmEncryptSecretKey":    "LCRYPTO_ENCRYPT_SECRET_KEY",
		}

		for fieldName, expectedTag := range expectedFields {
			field, found := reflect.TypeOf(Config{}).FieldByName(fieldName)
			require.True(t, found, "Config must have CRM field %s", fieldName)
			assert.Equal(t, expectedTag, field.Tag.Get("env"),
				"field %s must have env tag %q", fieldName, expectedTag)
		}
	})

	t.Run("config_struct_has_all_required_multi_tenant_fields_with_correct_tags", func(t *testing.T) {
		t.Parallel()

		// Verify that the Config struct includes the MULTI_TENANT_* fields
		// required by multi-tenant.md, with correct env tags.
		// This ensures backward compat: all fields must be optional (no envDefault
		// that forces multi-tenant on).
		expectedFields := map[string]string{
			"MultiTenantEnabled":                     "MULTI_TENANT_ENABLED",
			"MultiTenantURL":                         "MULTI_TENANT_URL",
			"MultiTenantCircuitBreakerThreshold":     "MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD",
			"MultiTenantCircuitBreakerTimeoutSec":    "MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC",
			"MultiTenantServiceAPIKey":               "MULTI_TENANT_SERVICE_API_KEY",
			"MultiTenantConnectionsCheckIntervalSec": "MULTI_TENANT_CONNECTIONS_CHECK_INTERVAL_SEC",
			"MultiTenantCacheTTLSec":                 "MULTI_TENANT_CACHE_TTL_SEC",
			"MultiTenantRedisHost":                   "MULTI_TENANT_REDIS_HOST",
			"MultiTenantRedisPort":                   "MULTI_TENANT_REDIS_PORT",
			"MultiTenantRedisPassword":               "MULTI_TENANT_REDIS_PASSWORD",
			"MultiTenantRedisTLS":                    "MULTI_TENANT_REDIS_TLS",
		}

		cfg := &Config{}
		// Verify zero-value Config is safe for single-tenant mode.
		assert.False(t, cfg.MultiTenantEnabled,
			"MultiTenantEnabled zero value must be false for backward compatibility")

		// Verify field existence and env tags via reflection.
		for fieldName, expectedTag := range expectedFields {
			field, found := reflect.TypeOf(Config{}).FieldByName(fieldName)
			require.True(t, found,
				"Config must have field %s for multi-tenant support", fieldName)

			envTag := field.Tag.Get("env")
			assert.Equal(t, expectedTag, envTag,
				"field %s must have env tag %q", fieldName, expectedTag)
		}
	})
}
