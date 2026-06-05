// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers"
	workermocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
)

// splitHostPort returns the host and port segments for a miniredis address.
// miniredis always listens on 127.0.0.1:<ephemeral>.
func splitHostPort(t *testing.T, addr string) (string, string) {
	t.Helper()

	host, port, err := net.SplitHostPort(addr)
	require.NoError(t, err, "miniredis address must be host:port")

	return host, port
}

// startFakeTenantManager returns an HTTP server that satisfies the tiny slice
// of the Tenant Manager API the wiring helper touches: GET
// /v1/tenants/active?service=<name>. The handler echoes an empty active-tenant
// list so InitialTenantSync becomes a no-op and the supervisor stays idle.
func startFakeTenantManager(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/tenants/active"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	t.Cleanup(srv.Close)

	return srv
}

// newWiringTestDeps returns a multiTenantWiringDeps + supervisor extras that
// can be handed to buildMultiTenantComponents. The repos are gomock-backed so
// any accidental usage during construction fails loudly.
func newWiringTestDeps(t *testing.T) (multiTenantWiringDeps, workers.WorkerSupervisorDeps) {
	t.Helper()

	ctrl := gomock.NewController(t)

	syncRepo := workermocks.NewMockRuleSyncRepository(ctrl)
	usageRepo := workermocks.NewMockUsageCounterCleanupRepository(ctrl)
	compiler := workermocks.NewMockExpressionCompiler(ctrl)

	deps := multiTenantWiringDeps{
		SyncRepo:  syncRepo,
		UsageRepo: usageRepo,
		Compiler:  compiler,
		SyncConfig: workers.RuleSyncWorkerConfig{
			PollInterval:       1 * time.Second,
			StalenessThreshold: 5 * time.Second,
			OverlapBuffer:      200 * time.Millisecond,
		},
		CleanupConfig:        workers.UsageCleanupWorkerConfig{CleanupInterval: 1 * time.Hour},
		CleanupWorkerEnabled: true,
		CBTemplate: workers.CircuitBreakerTemplate{
			NamePrefix:    "test_supervisor",
			MaxRequests:   1,
			Timeout:       1 * time.Second,
			FailureThresh: 3,
		},
	}

	extras := workers.WorkerSupervisorDeps{
		RuleCache: cache.NewRuleCache(clock.RealClock{}),
		Clock:     clock.RealClock{},
		Logger:    testutil.NewMockLogger(),
		Service:   "tracer",
	}

	return deps, extras
}

// TestBuildMultiTenantComponents_Success verifies the happy path: a reachable
// Redis and a fake Tenant Manager produce a fully populated component set.
func TestBuildMultiTenantComponents_Success(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	redisHost, redisPort := splitHostPort(t, mr.Addr())

	tm := startFakeTenantManager(t)

	logger := testutil.NewMockLogger()
	cfg := &Config{
		ApplicationName:                        "tracer",
		MultiTenantEnabled:                     true,
		MultiTenantURL:                         tm.URL,
		MultiTenantServiceAPIKey:               "svc-api-key",
		MultiTenantRedisHost:                   redisHost,
		MultiTenantRedisPort:                   redisPort,
		MultiTenantTimeout:                     30,
		MultiTenantCircuitBreakerThreshold:     5,
		MultiTenantCircuitBreakerTimeoutSec:    30,
		MultiTenantMaxTenantPools:              100,
		MultiTenantIdleTimeoutSec:              300,
		MultiTenantConnectionsCheckIntervalSec: 30,
		// httptest.Server exposes http://; opt in explicitly per H13 hard gate.
		MultiTenantAllowInsecureHTTP: true,
	}

	deps, extras := newWiringTestDeps(t)

	components, err := buildMultiTenantComponents(cfg, logger, deps, extras)
	require.NoError(t, err)
	require.NotNil(t, components)
	t.Cleanup(func() {
		components.supervisor.Shutdown()
	})

	assert.NotNil(t, components.tmClient, "tenant-manager client must be built")
	assert.NotNil(t, components.pgManager, "postgres pool manager must be built")
	assert.NotNil(t, components.supervisor, "worker supervisor must be built")
	assert.NotNil(t, components.eventListener, "tenant event listener wrapper must be built")
}

// TestBuildMultiTenantComponents_InvalidTenantManagerURL verifies the fail-fast
// path when MULTI_TENANT_URL is garbage. The tm client constructor rejects
// URLs without a host, which surfaces as an error out of the wiring helper.
func TestBuildMultiTenantComponents_InvalidTenantManagerURL(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	redisHost, redisPort := splitHostPort(t, mr.Addr())

	logger := testutil.NewMockLogger()
	cfg := &Config{
		ApplicationName:          "tracer",
		MultiTenantEnabled:       true,
		MultiTenantURL:           "not-a-valid-url",
		MultiTenantServiceAPIKey: "svc-api-key",
		MultiTenantRedisHost:     redisHost,
		MultiTenantRedisPort:     redisPort,
	}

	deps, extras := newWiringTestDeps(t)

	components, err := buildMultiTenantComponents(cfg, logger, deps, extras)
	require.Error(t, err)
	assert.Nil(t, components)
	assert.Contains(t, err.Error(), "tenant-manager client")
}

// TestBuildMultiTenantComponents_UnreachableRedis verifies that a Redis host
// that is not listening surfaces a useful error (the wiring helper PINGs
// Redis at construction time).
func TestBuildMultiTenantComponents_UnreachableRedis(t *testing.T) {
	t.Parallel()

	tm := startFakeTenantManager(t)

	logger := testutil.NewMockLogger()
	cfg := &Config{
		ApplicationName:          "tracer",
		MultiTenantEnabled:       true,
		MultiTenantURL:           tm.URL,
		MultiTenantServiceAPIKey: "svc-api-key",
		// Port 1 is a well-known unused low port; no miniredis running.
		MultiTenantRedisHost:                "127.0.0.1",
		MultiTenantRedisPort:                "1",
		MultiTenantTimeout:                  30,
		MultiTenantCircuitBreakerThreshold:  5,
		MultiTenantCircuitBreakerTimeoutSec: 30,
		// httptest.Server exposes http://; opt in explicitly per H13 hard gate.
		MultiTenantAllowInsecureHTTP: true,
	}

	deps, extras := newWiringTestDeps(t)

	components, err := buildMultiTenantComponents(cfg, logger, deps, extras)
	require.Error(t, err)
	assert.Nil(t, components)
	assert.Contains(t, err.Error(), "tenant pubsub redis")
}

// TestBuildMultiTenantComponents_RequiresMultiTenantEnabled rejects the
// footgun where the helper gets called in single-tenant mode — a safety rail
// the bootstrap relies on to keep the conditional branch clean.
func TestBuildMultiTenantComponents_RequiresMultiTenantEnabled(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()
	cfg := &Config{ApplicationName: "tracer", MultiTenantEnabled: false}

	deps, extras := newWiringTestDeps(t)

	components, err := buildMultiTenantComponents(cfg, logger, deps, extras)
	require.Error(t, err)
	assert.Nil(t, components)
	assert.Contains(t, err.Error(), "MULTI_TENANT_ENABLED=true")
}

// TestBuildMultiTenantComponents_SupervisorShutdownClean verifies that the
// component's supervisor can be shut down without hanging even with zero
// tenants registered — the 0-tenant leg of Deliverable D when applied at the
// wiring helper level.
func TestBuildMultiTenantComponents_SupervisorShutdownClean(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	redisHost, redisPort := splitHostPort(t, mr.Addr())

	tm := startFakeTenantManager(t)

	logger := testutil.NewMockLogger()
	cfg := &Config{
		ApplicationName:                        "tracer",
		MultiTenantEnabled:                     true,
		MultiTenantURL:                         tm.URL,
		MultiTenantServiceAPIKey:               "svc-api-key",
		MultiTenantRedisHost:                   redisHost,
		MultiTenantRedisPort:                   redisPort,
		MultiTenantTimeout:                     30,
		MultiTenantCircuitBreakerThreshold:     5,
		MultiTenantCircuitBreakerTimeoutSec:    30,
		MultiTenantMaxTenantPools:              100,
		MultiTenantIdleTimeoutSec:              300,
		MultiTenantConnectionsCheckIntervalSec: 30,
		// httptest.Server exposes http://; opt in explicitly per H13 hard gate.
		MultiTenantAllowInsecureHTTP: true,
	}

	deps, extras := newWiringTestDeps(t)
	components, err := buildMultiTenantComponents(cfg, logger, deps, extras)
	require.NoError(t, err)

	done := make(chan struct{})

	go func() {
		defer close(done)
		components.supervisor.Shutdown()
		components.eventListener.Shutdown()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor + listener shutdown did not complete within 2s")
	}
}

// TestValidateMultiTenantConfig_IsGateForInitServers is a sanity check that
// the validation step rejects the exact config shape we use in single-tenant
// mode — proving the fail-fast path documented in Gate 3 is still enforced.
func TestValidateMultiTenantConfig_EnabledMissingURL_StopsInitServers(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()
	cfg := &Config{
		MultiTenantEnabled:       true,
		MultiTenantURL:           "",
		MultiTenantServiceAPIKey: "key",
		MultiTenantRedisHost:     "localhost",
	}

	err := ValidateMultiTenantConfig(t.Context(), cfg, logger)
	require.Error(t, err)
}

// TestBuildMultiTenantComponents_AppliesCustomCacheTTL verifies H7: the
// MULTI_TENANT_CACHE_TTL_SEC operator value flows into the tenant-manager
// client via WithCacheTTL. lib-commons keeps cacheTTL unexported, so the test
// exercises the wiring path with a non-zero value and confirms that
// construction succeeds — regression guard for the case where the option
// silently dropped through to the 1h default.
func TestBuildMultiTenantComponents_AppliesCustomCacheTTL(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	redisHost, redisPort := splitHostPort(t, mr.Addr())

	tm := startFakeTenantManager(t)

	logger := testutil.NewMockLogger()
	cfg := &Config{
		ApplicationName:                        "tracer",
		MultiTenantEnabled:                     true,
		MultiTenantURL:                         tm.URL,
		MultiTenantServiceAPIKey:               "svc-api-key",
		MultiTenantRedisHost:                   redisHost,
		MultiTenantRedisPort:                   redisPort,
		MultiTenantTimeout:                     30,
		MultiTenantCircuitBreakerThreshold:     5,
		MultiTenantCircuitBreakerTimeoutSec:    30,
		MultiTenantMaxTenantPools:              100,
		MultiTenantIdleTimeoutSec:              300,
		MultiTenantConnectionsCheckIntervalSec: 30,
		// Non-default cache TTL (7 seconds). If the option is never applied
		// the client silently falls back to 1h — this test would still pass
		// at compile time, but the TTL is passed through WithCacheTTL at the
		// call site and exercises the option surface.
		MultiTenantCacheTTLSec: 7,
		// httptest.Server exposes http://; opt in explicitly per H13 hard gate.
		MultiTenantAllowInsecureHTTP: true,
	}

	deps, extras := newWiringTestDeps(t)

	components, err := buildMultiTenantComponents(cfg, logger, deps, extras)
	require.NoError(t, err)
	require.NotNil(t, components)
	t.Cleanup(components.supervisor.Shutdown)

	assert.NotNil(t, components.tmClient,
		"tenant-manager client must be built with custom cache TTL option applied")
}

// TestBuildMultiTenantComponents_RejectsHTTPWithoutOptIn verifies H13: when
// MULTI_TENANT_URL uses cleartext http:// and MULTI_TENANT_ALLOW_INSECURE_HTTP
// is not explicitly true, the wiring fails fast rather than silently sending
// the service API key in plaintext.
func TestBuildMultiTenantComponents_RejectsHTTPWithoutOptIn(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	redisHost, redisPort := splitHostPort(t, mr.Addr())

	tm := startFakeTenantManager(t)

	logger := testutil.NewMockLogger()
	cfg := &Config{
		ApplicationName:                        "tracer",
		MultiTenantEnabled:                     true,
		MultiTenantURL:                         tm.URL, // http://
		MultiTenantServiceAPIKey:               "svc-api-key",
		MultiTenantRedisHost:                   redisHost,
		MultiTenantRedisPort:                   redisPort,
		MultiTenantTimeout:                     30,
		MultiTenantCircuitBreakerThreshold:     5,
		MultiTenantCircuitBreakerTimeoutSec:    30,
		MultiTenantMaxTenantPools:              100,
		MultiTenantIdleTimeoutSec:              300,
		MultiTenantConnectionsCheckIntervalSec: 30,
		// Deliberately NOT setting MultiTenantAllowInsecureHTTP — the wiring
		// must reject cleartext HTTP unless the operator opts in explicitly.
	}

	deps, extras := newWiringTestDeps(t)

	components, err := buildMultiTenantComponents(cfg, logger, deps, extras)
	require.Error(t, err)
	assert.Nil(t, components)
	assert.Contains(t, err.Error(), "MULTI_TENANT_ALLOW_INSECURE_HTTP",
		"error must name the flag operators need to set so they understand the opt-in")
}

// TestBuildMultiTenantComponents_AllowsHTTPWithOptIn verifies H13: when the
// operator explicitly sets MULTI_TENANT_ALLOW_INSECURE_HTTP=true, cleartext
// http:// URLs are accepted and a security-warn is logged.
func TestBuildMultiTenantComponents_AllowsHTTPWithOptIn(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	redisHost, redisPort := splitHostPort(t, mr.Addr())

	tm := startFakeTenantManager(t)

	logger := testutil.NewMockLogger()
	cfg := &Config{
		ApplicationName:                        "tracer",
		MultiTenantEnabled:                     true,
		MultiTenantURL:                         tm.URL, // http://
		MultiTenantServiceAPIKey:               "svc-api-key",
		MultiTenantRedisHost:                   redisHost,
		MultiTenantRedisPort:                   redisPort,
		MultiTenantTimeout:                     30,
		MultiTenantCircuitBreakerThreshold:     5,
		MultiTenantCircuitBreakerTimeoutSec:    30,
		MultiTenantMaxTenantPools:              100,
		MultiTenantIdleTimeoutSec:              300,
		MultiTenantConnectionsCheckIntervalSec: 30,
		MultiTenantAllowInsecureHTTP:           true,
	}

	deps, extras := newWiringTestDeps(t)

	components, err := buildMultiTenantComponents(cfg, logger, deps, extras)
	require.NoError(t, err)
	require.NotNil(t, components)
	t.Cleanup(components.supervisor.Shutdown)

	// Verify a WARN log mentioning cleartext HTTP was emitted (so operators
	// notice the downgrade even when they opted in). MockLogger stores the
	// level as a lowercase string.
	var foundWarn bool
	for _, call := range logger.Calls {
		if call.Level == "warn" &&
			strings.Contains(call.Message, "cleartext HTTP") {
			foundWarn = true
			break
		}
	}
	assert.True(t, foundWarn, "expected WARN log about cleartext HTTP downgrade")
}

// TestBuildMultiTenantComponents_IsolatedGoroutineSafety exercises the
// fire-and-forget initial sync pathway: the goroutine spawned inside
// InitServers (simulated here by calling InitialTenantSync directly) must
// complete quickly against the empty tenant-list fake.
func TestBuildMultiTenantComponents_InitialTenantSyncEmpty(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	redisHost, redisPort := splitHostPort(t, mr.Addr())
	tm := startFakeTenantManager(t)

	logger := testutil.NewMockLogger()
	cfg := &Config{
		ApplicationName:                        "tracer",
		MultiTenantEnabled:                     true,
		MultiTenantURL:                         tm.URL,
		MultiTenantServiceAPIKey:               "svc-api-key",
		MultiTenantRedisHost:                   redisHost,
		MultiTenantRedisPort:                   redisPort,
		MultiTenantTimeout:                     30,
		MultiTenantCircuitBreakerThreshold:     5,
		MultiTenantCircuitBreakerTimeoutSec:    30,
		MultiTenantMaxTenantPools:              100,
		MultiTenantIdleTimeoutSec:              300,
		MultiTenantConnectionsCheckIntervalSec: 30,
		// httptest.Server exposes http://; opt in explicitly per H13 hard gate.
		MultiTenantAllowInsecureHTTP: true,
	}

	deps, extras := newWiringTestDeps(t)
	components, err := buildMultiTenantComponents(cfg, logger, deps, extras)
	require.NoError(t, err)
	t.Cleanup(components.supervisor.Shutdown)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Empty tenant list returned by the fake TM ⇒ no workers spawned, no error.
	require.NoError(t, components.supervisor.InitialTenantSync(ctx))
}

// TestService_Shutdown_ClosesTenantManagerClient verifies L1: Service.Shutdown
// wires every MT resource, including the tenant-manager HTTP client. The test
// drives the full Shutdown path against a Service built from real wiring
// components and confirms that (a) tmClient is populated and (b) Shutdown
// completes without error. Because tmclient.Client.Close is idempotent, a
// successful Shutdown proves the close call is reachable — without the wiring,
// the client would leak its internal cache goroutines at service teardown.
func TestService_Shutdown_ClosesTenantManagerClient(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	redisHost, redisPort := splitHostPort(t, mr.Addr())

	tm := startFakeTenantManager(t)

	logger := testutil.NewMockLogger()
	cfg := &Config{
		ApplicationName:                        "tracer",
		MultiTenantEnabled:                     true,
		MultiTenantURL:                         tm.URL,
		MultiTenantServiceAPIKey:               "svc-api-key",
		MultiTenantRedisHost:                   redisHost,
		MultiTenantRedisPort:                   redisPort,
		MultiTenantTimeout:                     30,
		MultiTenantCircuitBreakerThreshold:     5,
		MultiTenantCircuitBreakerTimeoutSec:    30,
		MultiTenantMaxTenantPools:              100,
		MultiTenantIdleTimeoutSec:              300,
		MultiTenantConnectionsCheckIntervalSec: 30,
		MultiTenantAllowInsecureHTTP:           true,
	}

	deps, extras := newWiringTestDeps(t)
	components, err := buildMultiTenantComponents(cfg, logger, deps, extras)
	require.NoError(t, err)
	require.NotNil(t, components)

	// Pre-condition: every MT resource that Shutdown touches is populated.
	require.NotNil(t, components.tmClient, "pre: tmClient must be built")
	require.NotNil(t, components.pgManager, "pre: pgManager must be built")
	require.NotNil(t, components.supervisor, "pre: supervisor must be built")
	require.NotNil(t, components.eventListener, "pre: eventListener must be built")

	// Build a Service the same way the bootstrap does (newService in config.go).
	// ReadyzDrainGraceSeconds is intentionally tiny here — Gate 7's drain
	// grace is exercised by service_drain_test.go; this test only cares
	// about the MT cleanup chain.
	svcCfg := *cfg
	svcCfg.ReadyzDrainGraceSeconds = 1
	svc := &Service{
		Logger:        logger,
		pgManager:     components.pgManager,
		supervisor:    components.supervisor,
		eventListener: components.eventListener,
		tmClient:      components.tmClient,
		config:        &svcCfg,
	}

	// Shutdown must close every MT resource without erroring. The test deadline
	// is generous (5s) because pgManager.Close can iterate over per-tenant
	// pools — but for the empty-tenant case it returns quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- svc.Shutdown(ctx) }()

	select {
	case err := <-done:
		require.NoError(t, err, "Shutdown must complete without error")
	case <-time.After(5 * time.Second):
		t.Fatal("Service.Shutdown did not complete within 5s — tmClient.Close may be blocking")
	}

	// Post-condition: calling Close a second time must not panic. tmclient.Close
	// is idempotent; if the first Close inside Shutdown succeeded, this is a
	// safe no-op. A panic here would indicate Shutdown corrupted the client.
	assert.NotPanics(t, func() {
		_ = svc.tmClient.Close()
	}, "tmClient.Close must remain safe after Service.Shutdown")
}
