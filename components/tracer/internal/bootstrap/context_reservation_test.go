// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

const (
	testPlatformProducers = `[{"service":"ledger","clientId":"ledger-m2m-client","certUri":"spiffe://example.test/service/ledger"}]`
	testJWKSURL           = "https://access-manager.example.test/.well-known/jwks"
	testM2MIssuer         = "https://access-manager.example.test"
)

// serveTestJWKS publishes an empty key set on a loopback listener, so a test
// that starts the JWKS refresher never reaches the network. Loopback URLs are
// accepted without TLS by the JWKS key source.
func serveTestJWKS(t *testing.T) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if _, err := w.Write([]byte(`{"keys":[]}`)); err != nil {
			return
		}
	}))
	t.Cleanup(server.Close)

	return server.URL + "/.well-known/jwks"
}

// validContextReservationConfig is a local-mode configuration that loads
// without starting a JWKS refresher.
func validContextReservationConfig() *Config {
	cfg := validContextPolicyConfig()
	cfg.DeploymentMode = "local"
	cfg.ContextM2MJWKSURL = testJWKSURL
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

func loadTestContextReservationConfig(t *testing.T, cfg *Config) (*contextReservationConfig, error) {
	t.Helper()

	result, err := loadContextReservationConfig(cfg, libLog.NewNop())
	if result != nil {
		t.Cleanup(func() { require.NoError(t, result.close()) })
	}

	return result, err
}

func TestContextReservationConfigRefusesInvalidProducerIdentity(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		mutate func(*Config)
		reason string
	}{
		{name: "JWKS URL missing outside local", mutate: func(c *Config) {
			c.DeploymentMode = "byoc"
			c.ContextM2MIssuer = testM2MIssuer
			c.ContextM2MJWKSURL = ""
		}, reason: "CONTEXT_M2M_JWKS_URL is required"},
		{name: "JWKS URL blank with deployment mode unset", mutate: func(c *Config) {
			c.DeploymentMode = ""
			c.ContextM2MIssuer = testM2MIssuer
			c.ContextM2MJWKSURL = "  "
		}, reason: "CONTEXT_M2M_JWKS_URL is required"},
		{name: "issuer missing outside local", mutate: func(c *Config) { c.DeploymentMode = "saas" }, reason: "CONTEXT_M2M_ISSUER is required unless DEPLOYMENT_MODE=local"},
		{name: "issuer missing in byoc", mutate: func(c *Config) { c.DeploymentMode = "byoc" }, reason: "CONTEXT_M2M_ISSUER is required unless DEPLOYMENT_MODE=local"},
		{name: "issuer missing with deployment mode unset", mutate: func(c *Config) { c.DeploymentMode = "" }, reason: "CONTEXT_M2M_ISSUER is required unless DEPLOYMENT_MODE=local"},
		{name: "issuer missing with blank deployment mode", mutate: func(c *Config) { c.DeploymentMode = "   " }, reason: "CONTEXT_M2M_ISSUER is required unless DEPLOYMENT_MODE=local"},
		{name: "issuer missing with unknown deployment mode", mutate: func(c *Config) { c.DeploymentMode = "localhost" }, reason: "CONTEXT_M2M_ISSUER is required unless DEPLOYMENT_MODE=local"},
		{name: "plaintext JWKS outside local", mutate: func(c *Config) {
			c.DeploymentMode = "saas"
			c.ContextM2MIssuer = testM2MIssuer
			c.ContextM2MJWKSURL = "http://access-manager.internal/.well-known/jwks"
		}, reason: "invalid CONTEXT_M2M_JWKS_URL"},
		{name: "producer map missing", mutate: func(c *Config) { c.TracerPlatformProducers = "" }, reason: "invalid TRACER_PLATFORM_PRODUCERS"},
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

func TestContextReservationConfigLocalModeNeedsNoJWKS(t *testing.T) {
	t.Parallel()

	cfg := validContextReservationConfig()
	cfg.ContextM2MJWKSURL = ""

	result, err := loadTestContextReservationConfig(t, cfg)
	require.NoError(t, err, "local mode verifies no token, so it reads no JWKS")
	require.True(t, result.unverifiedProducers)
	require.Nil(t, result.keySource)
}

func TestContextReservationConfigLoadsHTTPOnlyWithoutTLSMode(t *testing.T) {
	t.Parallel()

	cfg := validContextReservationConfig()

	result, err := loadTestContextReservationConfig(t, cfg)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.m2m)
	require.Nil(t, result.keySource, "local mode must not start a JWKS refresher")
	require.True(t, result.unverifiedProducers, "explicit local mode attributes reservations without a token")

	producer, ok := result.producers.ByClientID("ledger-m2m-client")
	require.True(t, ok)
	require.Equal(t, producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaToken}, producer)
}

func TestContextReservationConfigLoadsHTTPAndGRPCUnderMTLS(t *testing.T) {
	t.Parallel()

	cfg := validContextReservationConfig()
	cfg.DeploymentMode = "saas"
	cfg.ContextM2MIssuer = testM2MIssuer
	cfg.ContextM2MJWKSURL = serveTestJWKS(t)
	cfg.TracerGRPCPort = ":4021"
	cfg.TracerTLSMode = "MTLS"

	result, err := loadTestContextReservationConfig(t, cfg)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.m2m)
	require.NotNil(t, result.keySource, "verification outside local mode must use the JWKS key source")
	require.False(t, result.unverifiedProducers)
	require.True(t, result.producers.HasCertificateMappings())
}

func TestContextReservationConfigVerifiesProducersWhenDeploymentModeIsUnset(t *testing.T) {
	t.Parallel()

	cfg := validContextReservationConfig()
	cfg.DeploymentMode = ""
	cfg.ContextM2MIssuer = testM2MIssuer
	cfg.ContextM2MJWKSURL = serveTestJWKS(t)

	logger := testutil.NewMockLogger()

	result, err := loadContextReservationConfig(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, result.close()) })

	require.False(t, result.unverifiedProducers)
	require.NotNil(t, result.keySource, "an unset DEPLOYMENT_MODE must verify producer tokens")
	require.Zero(t, producerAuthDisabledWarnings(logger))
}

func TestContextReservationConfigWarnsOnceWhenProducerVerificationIsDisabled(t *testing.T) {
	t.Parallel()

	cfg := validContextReservationConfig()
	cfg.DeploymentMode = " LOCAL "

	logger := testutil.NewMockLogger()

	result, err := loadContextReservationConfig(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, result.close()) })

	require.True(t, result.unverifiedProducers)
	require.Nil(t, result.keySource)
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

	cfg := validContextReservationConfig()
	cfg.MultiTenantEnabled = true

	result, err := loadTestContextReservationConfig(t, cfg)
	require.ErrorContains(t, err, "MULTI_TENANT_ENABLED=true requires producer token verification")
	require.Nil(t, result)

	cfg.DeploymentMode = "byoc"
	cfg.ContextM2MIssuer = testM2MIssuer
	cfg.ContextM2MJWKSURL = serveTestJWKS(t)

	result, err = loadTestContextReservationConfig(t, cfg)
	require.NoError(t, err, "multi-tenancy boots once producer tokens are verified")
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
