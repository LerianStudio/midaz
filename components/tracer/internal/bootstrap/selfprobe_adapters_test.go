// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/http/in"
)

// TestPostgresSelfProbe_NilHealthChecker tolerates a nil checker without
// panicking — the adapter must surface a clear error so RunSelfProbe can
// log it.
func TestPostgresSelfProbe_NilHealthChecker(t *testing.T) {
	probe := newPostgresSelfProbe(nil)
	err := probe.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "health checker not wired")
}

// TestPostgresSelfProbe_NotConnected — when the underlying provider reports
// not connected, the probe surfaces that as the canonical "connection not
// established" error rather than letting GetDB blow up.
func TestPostgresSelfProbe_NotConnected(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockProvider := in.NewMockPostgresDBProvider(ctrl)
	mockProvider.EXPECT().IsConnected().Return(false)

	hc := in.NewTestableHealthChecker(mockProvider)
	probe := newPostgresSelfProbe(hc)

	err := probe.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection not established")
}

// TestPostgresSelfProbe_GetDBError surfaces underlying connection errors
// with the postgres self-probe prefix so log lines are searchable.
func TestPostgresSelfProbe_GetDBError(t *testing.T) {
	ctrl := gomock.NewController(t)

	getDBErr := errors.New("get db boom")
	mockProvider := in.NewMockPostgresDBProvider(ctrl)
	mockProvider.EXPECT().IsConnected().Return(true)
	mockProvider.EXPECT().GetDB(gomock.Any()).Return(nil, getDBErr)

	hc := in.NewTestableHealthChecker(mockProvider)
	probe := newPostgresSelfProbe(hc)

	err := probe.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get db")
	assert.ErrorIs(t, err, getDBErr)
}

// TestPostgresSelfProbe_PingFails captures ping-time failures.
func TestPostgresSelfProbe_PingFails(t *testing.T) {
	ctrl := gomock.NewController(t)

	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)

	defer db.Close()

	mock.ExpectPing().WillReturnError(errors.New("ping boom"))

	mockProvider := in.NewMockPostgresDBProvider(ctrl)
	mockProvider.EXPECT().IsConnected().Return(true)
	mockProvider.EXPECT().GetDB(gomock.Any()).Return(db, nil)

	hc := in.NewTestableHealthChecker(mockProvider)
	probe := newPostgresSelfProbe(hc)

	err = probe.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ping")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestPostgresSelfProbe_HappyPath returns nil when ping succeeds.
func TestPostgresSelfProbe_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)

	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)

	defer db.Close()

	mock.ExpectPing()

	mockProvider := in.NewMockPostgresDBProvider(ctrl)
	mockProvider.EXPECT().IsConnected().Return(true)
	mockProvider.EXPECT().GetDB(gomock.Any()).Return(db, nil)

	hc := in.NewTestableHealthChecker(mockProvider)
	probe := newPostgresSelfProbe(hc)

	require.NoError(t, probe.Check(context.Background()))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestRuleCacheSelfProbe_NilHealthChecker mirrors the postgres path — nil
// checker yields a clear error.
func TestRuleCacheSelfProbe_NilHealthChecker(t *testing.T) {
	probe := newRuleCacheSelfProbe(nil)
	err := probe.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "health checker not wired")
}

// TestRuleCacheSelfProbe_NoProvider — no SetCacheHealthProvider call ⇒ probe
// reports unconfigured.
func TestRuleCacheSelfProbe_NoProvider(t *testing.T) {
	hc := in.NewTestableHealthChecker(nil)
	probe := newRuleCacheSelfProbe(hc)

	err := probe.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider not configured")
}

// TestRuleCacheSelfProbe_NotReady — provider returns false ⇒ probe fails.
func TestRuleCacheSelfProbe_NotReady(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockCache := in.NewMockRuleCacheHealthProvider(ctrl)
	mockCache.EXPECT().IsReady(gomock.Any()).Return(false)

	hc := in.NewTestableHealthChecker(nil)
	hc.SetCacheHealthProvider(mockCache)

	probe := newRuleCacheSelfProbe(hc)
	err := probe.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache not ready")
}

// TestRuleCacheSelfProbe_HappyPath returns nil when IsReady is true.
func TestRuleCacheSelfProbe_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockCache := in.NewMockRuleCacheHealthProvider(ctrl)
	mockCache.EXPECT().IsReady(gomock.Any()).Return(true)

	hc := in.NewTestableHealthChecker(nil)
	hc.SetCacheHealthProvider(mockCache)

	probe := newRuleCacheSelfProbe(hc)
	require.NoError(t, probe.Check(context.Background()))
}

// TestBuildSelfProbeChecks_SingleTenant_IncludesPostgresAndRuleCache — in
// single-tenant mode the self-probe covers postgres + rule_cache (the cache
// is warmed at boot before the probe runs).
func TestBuildSelfProbeChecks_SingleTenant_IncludesPostgresAndRuleCache(t *testing.T) {
	hc := in.NewTestableHealthChecker(nil)
	checks := buildSelfProbeChecks(hc, false)

	_, hasPG := checks["postgres"]
	_, hasCache := checks["rule_cache"]
	_, hasTM := checks["tenant_manager"]
	_, hasPubsub := checks["tenant_pubsub"]

	assert.True(t, hasPG, "self-probe must include postgres")
	assert.True(t, hasCache, "self-probe must include rule_cache in single-tenant mode")
	assert.False(t, hasTM, "self-probe must NOT include tenant_manager — /readyz cycle is single-tenant")
	assert.False(t, hasPubsub, "self-probe must NOT include tenant_pubsub — /readyz cycle is single-tenant")
}

// TestBuildSelfProbeChecks_MultiTenant_OmitsRuleCache — in MT mode the empty
// bucket is intentionally NOT warmed; including rule_cache here would
// guarantee a startup probe failure and crashloop.
func TestBuildSelfProbeChecks_MultiTenant_OmitsRuleCache(t *testing.T) {
	hc := in.NewTestableHealthChecker(nil)
	checks := buildSelfProbeChecks(hc, true)

	_, hasPG := checks["postgres"]
	_, hasCache := checks["rule_cache"]

	assert.True(t, hasPG, "self-probe must include postgres in MT mode")
	assert.False(t, hasCache, "self-probe must NOT include rule_cache in MT mode (per-tenant readiness lives in metrics)")
}

// TestBuildSelfProbeChecks_NoHealthChecker — defensive: a nil checker yields
// an empty map.
func TestBuildSelfProbeChecks_NoHealthChecker(t *testing.T) {
	checks := buildSelfProbeChecks(nil, false)
	assert.Empty(t, checks, "nil hc ⇒ no deps to probe")
}
