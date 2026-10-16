// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libRedis "github.com/LerianStudio/lib-commons/v7/commons/redis"
	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmmongo "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/tenantcache"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// tenantHealthProbeEnvKeys lists every env var read by the tenant health probe knobs.
var tenantHealthProbeEnvKeys = []string{
	"TENANT_HEALTH_PROBE_ENABLED",
	"TENANT_HEALTH_PROBE_INTERVAL_MS",
	"TENANT_HEALTH_PROBE_TIMEOUT_MS",
	"TENANT_HEALTH_PROBE_PASS_BUDGET_MS",
}

// tenantHealthWiring holds the per-tenant managers a multi-tenant ledger builds.
// None of them dials anything until a tenant connection is requested.
type tenantHealthWiring struct {
	client *tmclient.Client
	cache  *tenantcache.TenantCache
	redis  *libRedis.Client
	onbPG  *onboardingPostgresComponents
	txnPG  *transactionPostgresComponents
	onbMgo *onboardingMongoComponents
	txnMgo *transactionMongoComponents
	crmMgo *crmComponents
}

func newTenantHealthWiring(t *testing.T) tenantHealthWiring {
	t.Helper()

	logger := libLog.NewNop()

	client, err := tmclient.NewClient(
		"http://localhost:0", logger,
		tmclient.WithAllowInsecureHTTP(),
		tmclient.WithServiceAPIKey("test-api-key"),
	)
	require.NoError(t, err)

	server := miniredis.RunT(t)

	redisClient, err := libRedis.New(t.Context(), libRedis.Config{
		Topology: libRedis.Topology{Standalone: &libRedis.StandaloneTopology{Address: server.Addr()}},
		Logger:   logger,
	})
	require.NoError(t, err)

	t.Cleanup(func() { assert.NoError(t, redisClient.Close()) })

	return tenantHealthWiring{
		client: client,
		cache:  tenantcache.NewTenantCache(),
		redis:  redisClient,
		onbPG:  &onboardingPostgresComponents{pgManager: tmpostgres.NewManager(client, constant.ModuleOnboarding)},
		txnPG:  &transactionPostgresComponents{pgManager: tmpostgres.NewManager(client, constant.ModuleTransaction)},
		onbMgo: &onboardingMongoComponents{mongoManager: tmmongo.NewManager(client, constant.ModuleOnboarding)},
		txnMgo: &transactionMongoComponents{mongoManager: tmmongo.NewManager(client, constant.ModuleTransaction)},
		crmMgo: &crmComponents{mongoManager: tmmongo.NewManager(client, constant.ModuleCRM)},
	}
}

func enabledTenantHealthProbeConfig() *Config {
	return &Config{
		TenantHealthProbeEnabled:      true,
		TenantHealthProbeIntervalMs:   60000,
		TenantHealthProbeTimeoutMs:    2000,
		TenantHealthProbePassBudgetMs: 30000,
	}
}

func TestInitTenantHealthProberIsAbsentInSingleTenant(t *testing.T) {
	w := newTenantHealthWiring(t)

	prober := initTenantHealthProber(enabledTenantHealthProbeConfig(), libLog.NewNop(), w.redis, false,
		w.cache, w.client, "ledger", w.onbPG, w.txnPG, w.onbMgo, w.txnMgo, w.crmMgo, nil)

	assert.Nil(t, prober)
}

func TestInitTenantHealthProberHonorsTheEnabledFlag(t *testing.T) {
	w := newTenantHealthWiring(t)

	cfg := enabledTenantHealthProbeConfig()
	cfg.TenantHealthProbeEnabled = false

	prober := initTenantHealthProber(cfg, libLog.NewNop(), w.redis, true,
		w.cache, w.client, "ledger", w.onbPG, w.txnPG, w.onbMgo, w.txnMgo, w.crmMgo, nil)

	assert.Nil(t, prober)
}

func TestInitTenantHealthProberNeedsATenantSource(t *testing.T) {
	w := newTenantHealthWiring(t)
	logger := &proberCapturingLogger{Logger: libLog.NewNop()}

	prober := initTenantHealthProber(enabledTenantHealthProbeConfig(), logger, w.redis, true,
		nil, nil, "ledger", w.onbPG, w.txnPG, w.onbMgo, w.txnMgo, w.crmMgo, nil)

	assert.Nil(t, prober)
	assertLoggedAt(t, logger, libLog.LevelWarn)
}

func TestInitTenantHealthProberNeedsALockManager(t *testing.T) {
	w := newTenantHealthWiring(t)
	logger := &proberCapturingLogger{Logger: libLog.NewNop()}

	prober := initTenantHealthProber(enabledTenantHealthProbeConfig(), logger, nil, true,
		w.cache, w.client, "ledger", w.onbPG, w.txnPG, w.onbMgo, w.txnMgo, w.crmMgo, nil)

	assert.Nil(t, prober)
	assertLoggedAt(t, logger, libLog.LevelWarn)
}

func TestInitTenantHealthProberWiresEveryTenantDependency(t *testing.T) {
	w := newTenantHealthWiring(t)
	logger := &proberCapturingLogger{Logger: libLog.NewNop()}

	cfg := enabledTenantHealthProbeConfig()
	cfg.TenantHealthProbeIntervalMs = 10000
	cfg.TenantHealthProbePassBudgetMs = 4000

	prober := initTenantHealthProber(cfg, logger, w.redis, true,
		w.cache, w.client, "ledger", w.onbPG, w.txnPG, w.onbMgo, w.txnMgo, w.crmMgo, nil)
	require.NotNil(t, prober)

	assert.Equal(t, TenantHealthProbeConfig{
		Interval:   10 * time.Second,
		Timeout:    2 * time.Second,
		PassBudget: 4 * time.Second,
	}, prober.cfg)

	require.Len(t, prober.postgres, 2)
	assert.Equal(t, tenantDependencyPostgresOnboarding, prober.postgres[0].dependency)
	assert.Same(t, w.onbPG.pgManager, prober.postgres[0].resolver)
	assert.Equal(t, tenantDependencyPostgresTransaction, prober.postgres[1].dependency)
	assert.Same(t, w.txnPG.pgManager, prober.postgres[1].resolver)
	assert.Same(t, w.onbMgo.mongoManager, prober.mongo, "the onboarding manager is the first choice")

	assert.Same(t, w.cache, prober.tenants)
	assert.Same(t, w.client, prober.tenantLister)
	assert.Equal(t, "ledger", prober.tenantService)

	lease, ok := prober.lease.(*redisReconcilerLease)
	require.True(t, ok)
	assert.Equal(t, utils.TenantHealthProbeLockKey(), lease.lockKey)
	// Two intervals, the pass budget, the last tenant's three sequential probes
	// and the tenant-manager listing call.
	assert.Equal(t, 2*10*time.Second+4*time.Second+3*2*time.Second+30*time.Second, lease.expiry)

	configured := logger.withMessage("Tenant health prober configured")
	require.Len(t, configured, 1)
	assert.Equal(t, "tenant_manager", configured[0].fields["tenant_source"])
	assert.Equal(t, []string{tenantDependencyPostgresOnboarding, tenantDependencyPostgresTransaction, tenantDependencyMongo},
		configured[0].fields["dependencies"])
}

func TestInitTenantHealthProberKeepsTypedNilsOutOfItsInterfaces(t *testing.T) {
	w := newTenantHealthWiring(t)
	logger := &proberCapturingLogger{Logger: libLog.NewNop()}

	prober := initTenantHealthProber(enabledTenantHealthProbeConfig(), logger, w.redis, true,
		w.cache, nil, "ledger",
		&onboardingPostgresComponents{}, nil,
		nil, &transactionMongoComponents{}, w.crmMgo, nil)
	require.NotNil(t, prober)

	assert.Empty(t, prober.postgres)
	assert.True(t, prober.tenantLister == nil, "no tenant client must leave the lister unset")
	assert.Same(t, w.crmMgo.mongoManager, prober.mongo, "the first non-nil Mongo manager is used")

	configured := logger.withMessage("Tenant health prober configured")
	require.Len(t, configured, 1)
	assert.Equal(t, "cache", configured[0].fields["tenant_source"])
	assert.Equal(t, []string{tenantDependencyMongo}, configured[0].fields["dependencies"])

	noCache := initTenantHealthProber(enabledTenantHealthProbeConfig(), libLog.NewNop(), w.redis, true,
		nil, w.client, "ledger", nil, nil, nil, nil, nil, nil)
	require.NotNil(t, noCache)

	assert.True(t, noCache.tenants == nil, "no tenant cache must leave the cache unset")
	assert.True(t, noCache.mongo == nil, "no Mongo manager must leave the Mongo probe unset")
	assert.Empty(t, noCache.postgres)
}

func TestTenantHealthProbeClampReplacesInvalidSettings(t *testing.T) {
	logger := &proberCapturingLogger{Logger: libLog.NewNop()}

	clamped := clampTenantHealthProbeConfig(logger, TenantHealthProbeConfig{
		Interval:   0,
		Timeout:    -time.Second,
		PassBudget: 500 * time.Millisecond,
	})

	assert.Equal(t, TenantHealthProbeConfig{
		Interval:   60 * time.Second,
		Timeout:    2 * time.Second,
		PassBudget: time.Second,
	}, clamped)

	warnings := logger.withMessage("Invalid tenant health probe setting; using a valid value")
	require.Len(t, warnings, 3)

	replaced := map[string]any{}
	for _, warning := range warnings {
		assert.Equal(t, libLog.LevelWarn, warning.level)
		replaced[warning.fields["env"].(string)] = warning.fields["replacement_ms"]
	}

	assert.Equal(t, map[string]any{
		"TENANT_HEALTH_PROBE_INTERVAL_MS":    int64(60000),
		"TENANT_HEALTH_PROBE_TIMEOUT_MS":     int64(2000),
		"TENANT_HEALTH_PROBE_PASS_BUDGET_MS": int64(1000),
	}, replaced)
}

func TestTenantHealthProbeClampKeepsValidSettings(t *testing.T) {
	logger := &proberCapturingLogger{Logger: libLog.NewNop()}

	valid := TenantHealthProbeConfig{Interval: 5 * time.Second, Timeout: 100 * time.Millisecond, PassBudget: time.Second}

	assert.Equal(t, valid, clampTenantHealthProbeConfig(logger, valid))
	assert.Empty(t, logger.withMessage("Invalid tenant health probe setting; using a valid value"))
}

func TestTenantHealthProberLauncherRegistersTheProberOnlyWhenPresent(t *testing.T) {
	t.Parallel()

	prober := NewTenantHealthProber(libLog.NewNop(), &fakeReconcilerLease{}, TenantHealthProbeConfig{})

	assert.Contains(t, launcherAppNames(&Service{TenantHealthProber: prober}), "Tenant Health Prober")
	assert.NotContains(t, launcherAppNames(&Service{}), "Tenant Health Prober")
}

func TestTenantHealthProbeConfigDefaults(t *testing.T) {
	type want struct {
		enabled      bool
		intervalMs   int
		timeoutMs    int
		passBudgetMs int
	}

	defaults := want{enabled: true, intervalMs: 60000, timeoutMs: 2000, passBudgetMs: 30000}

	tests := []struct {
		name string
		env  map[string]string
		want want
	}{
		{name: "unset_uses_defaults", want: defaults},
		{
			name: "empty_uses_defaults",
			env: map[string]string{
				"TENANT_HEALTH_PROBE_ENABLED":        "",
				"TENANT_HEALTH_PROBE_INTERVAL_MS":    "",
				"TENANT_HEALTH_PROBE_TIMEOUT_MS":     "",
				"TENANT_HEALTH_PROBE_PASS_BUDGET_MS": "",
			},
			want: defaults,
		},
		{
			name: "explicit_values_are_read",
			env: map[string]string{
				"TENANT_HEALTH_PROBE_ENABLED":        "true",
				"TENANT_HEALTH_PROBE_INTERVAL_MS":    "30000",
				"TENANT_HEALTH_PROBE_TIMEOUT_MS":     "500",
				"TENANT_HEALTH_PROBE_PASS_BUDGET_MS": "10000",
			},
			want: want{enabled: true, intervalMs: 30000, timeoutMs: 500, passBudgetMs: 10000},
		},
		{
			name: "explicit_false_disables",
			env:  map[string]string{"TENANT_HEALTH_PROBE_ENABLED": "false"},
			want: want{enabled: false, intervalMs: 60000, timeoutMs: 2000, passBudgetMs: 30000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range tenantHealthProbeEnvKeys {
				t.Setenv(key, "")
				require.NoError(t, os.Unsetenv(key))
			}

			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			cfg := &Config{}
			require.NoError(t, libCommons.SetConfigFromEnvVars(cfg))

			applyConfigDefaults(cfg)

			assert.Equal(t, tt.want, want{
				enabled:      cfg.TenantHealthProbeEnabled,
				intervalMs:   cfg.TenantHealthProbeIntervalMs,
				timeoutMs:    cfg.TenantHealthProbeTimeoutMs,
				passBudgetMs: cfg.TenantHealthProbePassBudgetMs,
			})
		})
	}
}

func assertLoggedAt(t *testing.T, logger *proberCapturingLogger, level int) {
	t.Helper()

	logger.mu.Lock()
	defer logger.mu.Unlock()

	for _, entry := range logger.entries {
		if entry.level == level {
			return
		}
	}

	t.Fatalf("no log entry at level %d", level)
}
