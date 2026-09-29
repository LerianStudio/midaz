// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"strings"
	"testing"

	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

const testPlatformProducers = `[{"service":"ledger","clientId":"ledger-m2m-client","certUri":"spiffe://example.test/service/ledger"}]`

// validContextReservationConfig is a local-mode configuration with plugin
// auth enabled, so the reservation routes authorize their caller.
func validContextReservationConfig() *Config {
	cfg := validContextPolicyConfig()
	cfg.DeploymentMode = "local"
	cfg.TracerPlatformProducers = testPlatformProducers
	cfg.ContextLimitMaxScopes = 10
	cfg.ContextLimitMaxScopeBytes = 4096
	cfg.ContextReserveMaxBodyBytes = 65536
	cfg.ContextReserveMaxLimits = 10
	cfg.ContextReserveMaxReservations = 100
	cfg.ContextPolicyCacheEntries = 10
	cfg.ContextPolicyMaxCompilations = 2

	return cfg
}

// withoutPluginAuth disables plugin auth, and with it the context policy
// administration that requires it.
func withoutPluginAuth(cfg *Config) *Config {
	cfg.PluginAuthEnabled = false
	cfg.ContextPolicyAdminEnabled = false

	return cfg
}

func loadTestContextReservationConfig(t *testing.T, cfg *Config) (*contextReservationConfig, error) {
	t.Helper()

	return loadContextReservationConfig(cfg, libLog.NewNop())
}

func TestContextReservationConfigRefusesInvalidProducerIdentity(t *testing.T) {
	t.Parallel()

	const pluginAuthRequired = "reservations require PLUGIN_AUTH_ENABLED=true unless DEPLOYMENT_MODE=local"

	for _, tc := range []struct {
		name   string
		mutate func(*Config)
		reason string
	}{
		{name: "plugin auth off in saas", mutate: func(c *Config) { withoutPluginAuth(c).DeploymentMode = "saas" }, reason: pluginAuthRequired},
		{name: "plugin auth off in byoc", mutate: func(c *Config) { withoutPluginAuth(c).DeploymentMode = "byoc" }, reason: pluginAuthRequired},
		{name: "plugin auth off with deployment mode unset", mutate: func(c *Config) { withoutPluginAuth(c).DeploymentMode = "" }, reason: pluginAuthRequired},
		{name: "plugin auth off with blank deployment mode", mutate: func(c *Config) { withoutPluginAuth(c).DeploymentMode = "   " }, reason: pluginAuthRequired},
		{name: "plugin auth off with unknown deployment mode", mutate: func(c *Config) { withoutPluginAuth(c).DeploymentMode = "localhost" }, reason: pluginAuthRequired},
		{name: "producer map lists no producer", mutate: func(c *Config) { c.TracerPlatformProducers = "[]" }, reason: "invalid TRACER_PLATFORM_PRODUCERS"},
		{name: "producer outside roster", mutate: func(c *Config) { c.TracerPlatformProducers = `[{"service":"fees","clientId":"fees"}]` }, reason: "invalid TRACER_PLATFORM_PRODUCERS"},
		{name: "producer map malformed", mutate: func(c *Config) { c.TracerPlatformProducers = `{"service":"ledger"}` }, reason: "invalid TRACER_PLATFORM_PRODUCERS"},
		{name: "gRPC without mtls", mutate: func(c *Config) { c.TracerGRPCPort = ":4021" }, reason: "TRACER_GRPC_PORT requires TRACER_TLS_MODE=mtls"},
		{name: "gRPC under mesh", mutate: func(c *Config) {
			c.TracerGRPCPort = ":4021"
			c.TracerTLSMode = "mesh"
		}, reason: "TRACER_GRPC_PORT requires TRACER_TLS_MODE=mtls"},
		{name: "gRPC without certificate mapping", mutate: func(c *Config) {
			c.TracerGRPCPort = ":4021"
			c.TracerTLSMode = "mtls"
			c.TracerPlatformProducers = `[{"service":"ledger","clientId":"ledger-m2m-client"}]`
		}, reason: "TRACER_GRPC_PORT requires a certUri in TRACER_PLATFORM_PRODUCERS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := validContextReservationConfig()
			tc.mutate(cfg)

			result, err := loadTestContextReservationConfig(t, cfg)
			require.ErrorContains(t, err, tc.reason)
			require.Nil(t, result)
		})
	}
}

func TestContextReservationSurfaceDisabledWithoutProducers(t *testing.T) {
	t.Parallel()

	for _, producers := range []string{"", " \t "} {
		// Outside local mode, multi-tenant, without plugin auth or bounds: a
		// validations-only Tracer reads none of the reservation settings.
		cfg := &Config{TracerPlatformProducers: producers, DeploymentMode: "saas", MultiTenantEnabled: true}
		logger := testutil.NewMockLogger()

		result, err := loadContextReservationConfig(cfg, logger)
		require.NoError(t, err)
		require.Nil(t, result)
		require.Equal(t, 1, reservationDisabledInfos(logger))

		runtime, err := initContextReservation(cfg, nil, nil, nil, nil, nil, libLog.NewNop())
		require.NoError(t, err)
		require.Nil(t, runtime, "no reservation runtime is built, so no reservation route is mounted")
	}
}

func TestContextReservationSurfaceDisabledRefusesGRPCPort(t *testing.T) {
	t.Parallel()

	cfg := &Config{TracerGRPCPort: ":4021", TracerTLSMode: "mtls"}
	logger := testutil.NewMockLogger()

	result, err := loadContextReservationConfig(cfg, logger)
	require.ErrorContains(t, err, "TRACER_GRPC_PORT requires TRACER_PLATFORM_PRODUCERS")
	require.Nil(t, result)
	require.Zero(t, reservationDisabledInfos(logger))
}

func reservationDisabledInfos(logger *testutil.MockLogger) int {
	count := 0

	for _, call := range logger.Snapshot() {
		if call.Level == "info" && call.Message == "reservation integration disabled (TRACER_PLATFORM_PRODUCERS empty)" {
			count++
		}
	}

	return count
}

func TestContextReservationConfigLocalModeWithoutPluginAuthAttributesTheLedger(t *testing.T) {
	t.Parallel()

	result, err := loadTestContextReservationConfig(t, withoutPluginAuth(validContextReservationConfig()))
	require.NoError(t, err, "explicit local mode may serve reservations without plugin auth")
	require.True(t, result.unverifiedProducers)
}

func TestContextReservationConfigLoadsHTTPOnlyWithoutTLSMode(t *testing.T) {
	t.Parallel()

	cfg := validContextReservationConfig()

	result, err := loadTestContextReservationConfig(t, cfg)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.unverifiedProducers, "plugin auth verifies the caller even in local mode")

	producer, ok := result.producers.ByClientID("ledger-m2m-client")
	require.True(t, ok)
	require.Equal(t, producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaToken}, producer)
}

func TestContextReservationConfigLoadsHTTPAndGRPCUnderMTLS(t *testing.T) {
	t.Parallel()

	cfg := validContextReservationConfig()
	cfg.DeploymentMode = "saas"
	cfg.TracerGRPCPort = ":4021"
	cfg.TracerTLSMode = "MTLS"

	result, err := loadTestContextReservationConfig(t, cfg)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.unverifiedProducers)
	require.True(t, result.producers.HasCertificateMappings())
}

func TestContextReservationConfigVerifiesProducersWhenDeploymentModeIsUnset(t *testing.T) {
	t.Parallel()

	cfg := validContextReservationConfig()
	cfg.DeploymentMode = ""

	logger := testutil.NewMockLogger()

	result, err := loadContextReservationConfig(cfg, logger)
	require.NoError(t, err)
	require.False(t, result.unverifiedProducers, "plugin auth verifies the caller whatever the deployment mode")
	require.Zero(t, producerAuthDisabledWarnings(logger))
}

func TestContextReservationConfigWarnsOnceWhenProducerVerificationIsDisabled(t *testing.T) {
	t.Parallel()

	cfg := withoutPluginAuth(validContextReservationConfig())
	cfg.DeploymentMode = " LOCAL "

	logger := testutil.NewMockLogger()

	result, err := loadContextReservationConfig(cfg, logger)
	require.NoError(t, err)
	require.True(t, result.unverifiedProducers)
	require.Equal(t, 1, producerAuthDisabledWarnings(logger))
}

func producerAuthDisabledWarnings(logger *testutil.MockLogger) int {
	count := 0

	for _, call := range logger.Snapshot() {
		if call.Level == "warn" && strings.Contains(call.Message, "producer authentication is disabled") {
			count++
		}
	}

	return count
}

func TestContextReservationConfigRefusesUnverifiedProducersUnderMultiTenancy(t *testing.T) {
	t.Parallel()

	cfg := withoutPluginAuth(validContextReservationConfig())
	cfg.MultiTenantEnabled = true

	result, err := loadTestContextReservationConfig(t, cfg)
	require.ErrorContains(t, err, "MULTI_TENANT_ENABLED=true requires PLUGIN_AUTH_ENABLED=true for reservations")
	require.Nil(t, result)

	cfg.PluginAuthEnabled = true
	cfg.DeploymentMode = "byoc"

	result, err = loadTestContextReservationConfig(t, cfg)
	require.NoError(t, err, "multi-tenancy boots once the Access Manager authorizes the caller")
	require.False(t, result.unverifiedProducers)
}

func TestReservationTenantAuthorizerFollowsTenancy(t *testing.T) {
	t.Parallel()

	require.False(t, reservationTenantAuthorizer(nil).Active(), "single-tenant mode ignores the reservation tenant")
	require.False(t, reservationTenantAuthorizer(&componentsMT{}).Active())

	authorizer := producerauth.NewTenantAuthorizer(func(context.Context, string, string) error { return nil }, true)
	require.Same(t, authorizer, reservationTenantAuthorizer(&componentsMT{tenantAuthorizer: authorizer}))
}

func TestInitGRPCServerRequiresTheReservationRuntime(t *testing.T) {
	t.Parallel()

	server, err := initGRPCServer(&Config{}, nil, nil, libLog.NewNop(), nil, nil)
	require.NoError(t, err)
	require.Nil(t, server, "an unset TRACER_GRPC_PORT starts no gRPC server")

	server, err = initGRPCServer(&Config{TracerGRPCPort: ":4021", TracerTLSMode: "mtls"}, nil, nil, libLog.NewNop(), nil, nil)
	require.ErrorContains(t, err, "context reservation runtime is required")
	require.Nil(t, server)
}

func TestInitGRPCServerRefusesUnenforcedTenancyUnderMultiTenancy(t *testing.T) {
	t.Parallel()

	cfg := &Config{TracerGRPCPort: ":4021", TracerTLSMode: "mtls", MultiTenantEnabled: true}
	active := producerauth.NewTenantAuthorizer(func(context.Context, string, string) error { return nil }, true)
	pool := tmpostgres.NewManager(nil, "tracer")

	for name, row := range map[string]struct {
		pgManager *tmpostgres.Manager
		authz     *producerauth.TenantAuthorizer
	}{
		"no authorizer":       {pgManager: pool, authz: nil},
		"inactive authorizer": {pgManager: pool, authz: producerauth.NewTenantAuthorizer(nil, false)},
		"no pool manager":     {pgManager: nil, authz: active},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server, err := initGRPCServer(cfg, row.pgManager, row.authz, libLog.NewNop(), nil, nil)
			require.ErrorContains(t, err, "MULTI_TENANT_ENABLED requires the tenant authorizer and the tenant pool manager")
			require.Nil(t, server)
		})
	}

	t.Run("both enforcing", func(t *testing.T) {
		t.Parallel()

		server, err := initGRPCServer(cfg, pool, active, libLog.NewNop(), nil, nil)
		require.ErrorContains(t, err, "context reservation runtime is required", "the tenancy check passes")
		require.Nil(t, server)
	})
}
