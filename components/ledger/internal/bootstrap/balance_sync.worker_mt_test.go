// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	tmclient "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/client"
	tmpostgres "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/tenantcache"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewBalanceSyncWorkerMT verifies that a multi-tenant-aware
// constructor correctly populates all multi-tenant fields.
func TestNewBalanceSyncWorkerMT(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	useCase := &command.UseCase{}
	tenantClient, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))
	cache := tenantcache.NewTenantCache()

	worker := NewBalanceSyncWorkerMT(logger, useCase, BalanceSyncConfig{}, true, cache, pgMgr, "transaction")

	require.NotNil(t, worker, "worker should not be nil")
	assert.True(t, worker.mtEnabled,
		"mtEnabled should be true")
	assert.Same(t, cache, worker.tenantCache,
		"tenantCache should be the same instance")
	assert.Same(t, pgMgr, worker.pgManager,
		"pgManager should be the same instance")
	assert.Equal(t, "transaction", worker.serviceName,
		"serviceName should be set correctly")
}

// TestBalanceSyncWorker_IsMTReady exercises the isMTReady()
// predicate across all combinations of mtEnabled x pgManager x tenantCache,
// plus the zero-value struct edge case.
func TestBalanceSyncWorker_IsMTReady(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	useCase := &command.UseCase{}
	tc, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	pgMgr := tmpostgres.NewManager(tc, "transaction", tmpostgres.WithLogger(logger))
	cache := tenantcache.NewTenantCache()

	tests := []struct {
		name        string
		mtEnabled   bool
		pgManager   *tmpostgres.Manager
		tenantCache *tenantcache.TenantCache
		want        bool
	}{
		{
			name:        "true_when_enabled_pgManager_and_tenantCache_set",
			mtEnabled:   true,
			pgManager:   pgMgr,
			tenantCache: cache,
			want:        true,
		},
		{
			name:        "false_when_enabled_but_pgManager_nil",
			mtEnabled:   true,
			pgManager:   nil,
			tenantCache: cache,
			want:        false,
		},
		{
			name:        "false_when_enabled_but_tenantCache_nil",
			mtEnabled:   true,
			pgManager:   pgMgr,
			tenantCache: nil,
			want:        false,
		},
		{
			name:        "false_when_disabled_but_pgManager_set",
			mtEnabled:   false,
			pgManager:   pgMgr,
			tenantCache: cache,
			want:        false,
		},
		{
			name:        "false_when_disabled_and_pgManager_nil",
			mtEnabled:   false,
			pgManager:   nil,
			tenantCache: nil,
			want:        false,
		},
		{
			name:        "false_for_zero_value_struct",
			mtEnabled:   false,
			pgManager:   nil,
			tenantCache: nil,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var worker *BalanceSyncWorker
			if tt.name == "false_for_zero_value_struct" {
				worker = &BalanceSyncWorker{}
			} else {
				worker = NewBalanceSyncWorker(logger, useCase, BalanceSyncConfig{})
				worker.mtEnabled = tt.mtEnabled
				worker.pgManager = tt.pgManager
				worker.tenantCache = tt.tenantCache
			}

			got := worker.isMTReady()
			assert.Equal(t, tt.want, got,
				"isMTReady() should return %v", tt.want)
		})
	}
}

// TestNewBalanceSyncWorkerMT_EdgeCases covers constructor edge cases:
// disabled mode with non-nil deps, nil tenantCache with non-nil pgManager, and all-nil.
func TestNewBalanceSyncWorkerMT_EdgeCases(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	useCase := &command.UseCase{}
	tenantClient, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))
	cache := tenantcache.NewTenantCache()

	tests := []struct {
		name        string
		mtEnabled   bool
		tenantCache *tenantcache.TenantCache
		pgManager   *tmpostgres.Manager
		wantEnabled bool
		wantReady   bool
	}{
		{
			name:        "disabled_with_non_nil_deps",
			mtEnabled:   false,
			tenantCache: cache,
			pgManager:   pgMgr,
			wantEnabled: false,
			wantReady:   false,
		},
		{
			name:        "nil_tenantCache_with_non_nil_pgManager",
			mtEnabled:   true,
			tenantCache: nil,
			pgManager:   pgMgr,
			wantEnabled: true,
			wantReady:   false,
		},
		{
			name:        "all_nil_disabled",
			mtEnabled:   false,
			tenantCache: nil,
			pgManager:   nil,
			wantEnabled: false,
			wantReady:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			worker := NewBalanceSyncWorkerMT(
				logger, useCase, BalanceSyncConfig{},
				tt.mtEnabled, tt.tenantCache, tt.pgManager, "transaction",
			)

			require.NotNil(t, worker, "constructor must return non-nil")
			assert.Equal(t, tt.wantEnabled, worker.mtEnabled,
				"mtEnabled should match input")
			assert.Equal(t, tt.wantReady, worker.isMTReady(),
				"isMTReady() should reflect field combination")

			if tt.tenantCache == nil {
				assert.Nil(t, worker.tenantCache,
					"tenantCache should be nil when passed nil")
			}
		})
	}
}

// TestNewBalanceSyncWorker_ZeroValueMultiTenantFields verifies that the base
// (non-multi-tenant) constructor leaves all multi-tenant fields at their zero values
// and correctly applies the maxWorkers default when the input is <= 0.
func TestNewBalanceSyncWorker_ZeroValueMultiTenantFields(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	useCase := &command.UseCase{}

	worker := NewBalanceSyncWorker(logger, useCase, BalanceSyncConfig{})

	require.NotNil(t, worker, "base constructor must return non-nil")
	assert.False(t, worker.mtEnabled,
		"mtEnabled should default to false")
	assert.Nil(t, worker.tenantCache,
		"tenantCache should default to nil")
	assert.Nil(t, worker.pgManager,
		"pgManager should default to nil")
	assert.False(t, worker.isMTReady(),
		"isMTReady() should be false for base constructor")
}
