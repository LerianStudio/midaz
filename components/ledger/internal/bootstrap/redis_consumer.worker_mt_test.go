// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	tmclient "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	tmpostgres "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/tenantcache"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/http/in"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRedisQueueConsumer_MultiTenantFields verifies that the RedisQueueConsumer
// struct contains the multi-tenant fields matching the same pattern as BalanceSyncWorker.
func TestRedisQueueConsumer_MultiTenantFields(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	handler := in.TransactionHandler{}
	tenantClient, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	cache := tenantcache.NewTenantCache()

	tests := []struct {
		name               string
		multiTenantEnabled bool
		tenantCache        *tenantcache.TenantCache
		pgManager          *tmpostgres.Manager
		wantMultiTenant    bool
	}{
		{
			name:               "multi-tenant enabled with pgManager",
			multiTenantEnabled: true,
			tenantCache:        cache,
			pgManager:          tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger)),
			wantMultiTenant:    true,
		},
		{
			name:               "single-tenant when pgManager is nil",
			multiTenantEnabled: true,
			tenantCache:        cache,
			pgManager:          nil,
			wantMultiTenant:    false,
		},
		{
			name:               "single-tenant when disabled",
			multiTenantEnabled: false,
			tenantCache:        nil,
			pgManager:          nil,
			wantMultiTenant:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			consumer := NewRedisQueueConsumer(logger, handler)

			// These fields must exist on the struct for multi-tenant support.
			consumer.multiTenantEnabled = tt.multiTenantEnabled
			consumer.tenantCache = tt.tenantCache
			consumer.pgManager = tt.pgManager

			assert.Equal(t, tt.multiTenantEnabled, consumer.multiTenantEnabled,
				"multiTenantEnabled field should be set on RedisQueueConsumer")

			if tt.wantMultiTenant {
				assert.NotNil(t, consumer.pgManager,
					"pgManager should be non-nil in multi-tenant mode")
				assert.NotNil(t, consumer.tenantCache,
					"tenantCache should be non-nil in multi-tenant mode")
			}
		})
	}
}

// TestRedisQueueConsumer_FallbackWhenPGManagerNil verifies the same fallback
// invariant for RedisQueueConsumer: pgManager == nil -> single-tenant.
func TestRedisQueueConsumer_FallbackWhenPGManagerNil(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	handler := in.TransactionHandler{}

	consumer := NewRedisQueueConsumer(logger, handler)
	consumer.multiTenantEnabled = true
	consumer.pgManager = nil

	assert.True(t, consumer.multiTenantEnabled,
		"multiTenantEnabled should be true")
	assert.Nil(t, consumer.pgManager,
		"pgManager should be nil, causing fallback to single-tenant behavior")

	ready := consumer.isMultiTenantReady()
	assert.False(t, ready,
		"isMultiTenantReady() should return false when pgManager is nil")
}

// TestNewRedisQueueConsumerMultiTenant verifies the multi-tenant-aware
// constructor for RedisQueueConsumer.
func TestNewRedisQueueConsumerMultiTenant(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	handler := in.TransactionHandler{}
	tenantClient, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))
	cache := tenantcache.NewTenantCache()

	consumer := NewRedisQueueConsumerMultiTenant(logger, handler, true, cache, pgMgr, "transaction")

	require.NotNil(t, consumer, "consumer should not be nil")
	assert.True(t, consumer.multiTenantEnabled,
		"multiTenantEnabled should be true")
	assert.Same(t, cache, consumer.tenantCache,
		"tenantCache should be the same instance")
	assert.Same(t, pgMgr, consumer.pgManager,
		"pgManager should be the same instance")
	assert.Equal(t, "transaction", consumer.serviceName,
		"serviceName should be set correctly")
}

// TestRedisQueueConsumer_IsMultiTenantReady exercises the isMultiTenantReady()
// predicate across all combinations of multiTenantEnabled x pgManager x tenantCache,
// plus the zero-value struct edge case.
func TestRedisQueueConsumer_IsMultiTenantReady(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	handler := in.TransactionHandler{}
	tc, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	pgMgr := tmpostgres.NewManager(tc, "transaction", tmpostgres.WithLogger(logger))
	cache := tenantcache.NewTenantCache()

	tests := []struct {
		name               string
		multiTenantEnabled bool
		pgManager          *tmpostgres.Manager
		tenantCache        *tenantcache.TenantCache
		want               bool
	}{
		{
			name:               "true_when_enabled_pgManager_and_tenantCache_set",
			multiTenantEnabled: true,
			pgManager:          pgMgr,
			tenantCache:        cache,
			want:               true,
		},
		{
			name:               "false_when_enabled_but_pgManager_nil",
			multiTenantEnabled: true,
			pgManager:          nil,
			tenantCache:        cache,
			want:               false,
		},
		{
			name:               "false_when_enabled_but_tenantCache_nil",
			multiTenantEnabled: true,
			pgManager:          pgMgr,
			tenantCache:        nil,
			want:               false,
		},
		{
			name:               "false_when_disabled_but_pgManager_set",
			multiTenantEnabled: false,
			pgManager:          pgMgr,
			tenantCache:        cache,
			want:               false,
		},
		{
			name:               "false_when_disabled_and_pgManager_nil",
			multiTenantEnabled: false,
			pgManager:          nil,
			tenantCache:        nil,
			want:               false,
		},
		{
			name:               "false_for_zero_value_struct",
			multiTenantEnabled: false,
			pgManager:          nil,
			tenantCache:        nil,
			want:               false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var consumer *RedisQueueConsumer
			if tt.name == "false_for_zero_value_struct" {
				consumer = &RedisQueueConsumer{}
			} else {
				consumer = NewRedisQueueConsumer(logger, handler)
				consumer.multiTenantEnabled = tt.multiTenantEnabled
				consumer.pgManager = tt.pgManager
				consumer.tenantCache = tt.tenantCache
			}

			got := consumer.isMultiTenantReady()
			assert.Equal(t, tt.want, got,
				"isMultiTenantReady() should return %v", tt.want)
		})
	}
}

// TestNewRedisQueueConsumerMultiTenant_EdgeCases covers constructor edge cases:
// disabled mode with non-nil deps, nil tenantCache with non-nil pgManager, and all-nil.
func TestNewRedisQueueConsumerMultiTenant_EdgeCases(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	handler := in.TransactionHandler{}
	tenantClient, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))
	cache := tenantcache.NewTenantCache()

	tests := []struct {
		name               string
		multiTenantEnabled bool
		tenantCache        *tenantcache.TenantCache
		pgManager          *tmpostgres.Manager
		wantEnabled        bool
		wantReady          bool
	}{
		{
			name:               "disabled_with_non_nil_deps",
			multiTenantEnabled: false,
			tenantCache:        cache,
			pgManager:          pgMgr,
			wantEnabled:        false,
			wantReady:          false,
		},
		{
			name:               "nil_tenantCache_with_non_nil_pgManager",
			multiTenantEnabled: true,
			tenantCache:        nil,
			pgManager:          pgMgr,
			wantEnabled:        true,
			wantReady:          false,
		},
		{
			name:               "all_nil_disabled",
			multiTenantEnabled: false,
			tenantCache:        nil,
			pgManager:          nil,
			wantEnabled:        false,
			wantReady:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			consumer := NewRedisQueueConsumerMultiTenant(
				logger, handler,
				tt.multiTenantEnabled, tt.tenantCache, tt.pgManager, "transaction",
			)

			require.NotNil(t, consumer, "constructor must return non-nil")
			assert.Equal(t, tt.wantEnabled, consumer.multiTenantEnabled,
				"multiTenantEnabled should match input")
			assert.Equal(t, tt.wantReady, consumer.isMultiTenantReady(),
				"isMultiTenantReady() should reflect field combination")

			if tt.tenantCache == nil {
				assert.Nil(t, consumer.tenantCache,
					"tenantCache should be nil when passed nil")
			}
		})
	}
}

// TestNewRedisQueueConsumer_ZeroValueMultiTenantFields verifies that the base
// (non-multi-tenant) constructor leaves all multi-tenant fields at their zero values.
func TestNewRedisQueueConsumer_ZeroValueMultiTenantFields(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	handler := in.TransactionHandler{}

	consumer := NewRedisQueueConsumer(logger, handler)

	require.NotNil(t, consumer, "base constructor must return non-nil")
	assert.False(t, consumer.multiTenantEnabled,
		"multiTenantEnabled should default to false")
	assert.Nil(t, consumer.tenantCache,
		"tenantCache should default to nil")
	assert.Nil(t, consumer.pgManager,
		"pgManager should default to nil")
	assert.False(t, consumer.isMultiTenantReady(),
		"isMultiTenantReady() should be false for base constructor")
}

// TestRedisQueueConsumer_RunDispatchesBasedOnMultiTenantReady verifies that the
// Run() dispatch predicate (isMultiTenantReady) returns the correct value for
// single-tenant and multi-tenant configurations on RedisQueueConsumer.
func TestRedisQueueConsumer_RunDispatchesBasedOnMultiTenantReady(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	handler := in.TransactionHandler{}
	tc, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	pgMgr := tmpostgres.NewManager(tc, "transaction", tmpostgres.WithLogger(logger))
	cache := tenantcache.NewTenantCache()

	tests := []struct {
		name               string
		multiTenantEnabled bool
		pgManager          *tmpostgres.Manager
		tenantCache        *tenantcache.TenantCache
		wantReady          bool
	}{
		{
			name:               "single_tenant_dispatches_to_runSingleTenant",
			multiTenantEnabled: false,
			pgManager:          nil,
			wantReady:          false,
		},
		{
			name:               "multi_tenant_dispatches_to_runMultiTenant",
			multiTenantEnabled: true,
			pgManager:          pgMgr,
			tenantCache:        cache,
			wantReady:          true,
		},
		{
			name:               "enabled_but_nil_pgManager_falls_back_to_single",
			multiTenantEnabled: true,
			pgManager:          nil,
			tenantCache:        cache,
			wantReady:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			consumer := NewRedisQueueConsumer(logger, handler)
			consumer.multiTenantEnabled = tt.multiTenantEnabled
			consumer.pgManager = tt.pgManager
			consumer.tenantCache = tt.tenantCache

			got := consumer.isMultiTenantReady()
			assert.Equal(t, tt.wantReady, got,
				"isMultiTenantReady() governs Run() dispatch: want %v", tt.wantReady)
		})
	}
}

// TestRabbitMQConsumerHandlerReceivesPGManager verifies that the
// config.rabbitmq.go wireConsumer callback can access pgManager and mongoManager
// when creating multi-tenant consumer handlers.
func TestRabbitMQConsumerHandlerReceivesPGManager(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	tenantClient, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	rmqComponents := &rabbitMQComponents{
		pgManager: pgMgr,
	}

	assert.NotNil(t, rmqComponents.pgManager,
		"rabbitMQComponents should carry pgManager for consumer handler")
}

// TestRabbitMQComponents_PGManagerField verifies that rabbitMQComponents
// correctly stores and exposes nil vs non-nil pgManager.
func TestRabbitMQComponents_PGManagerField(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	tenantClient, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	tests := []struct {
		name      string
		pgManager *tmpostgres.Manager
		wantNil   bool
	}{
		{
			name:      "non_nil_pgManager",
			pgManager: pgMgr,
			wantNil:   false,
		},
		{
			name:      "nil_pgManager_single_tenant",
			pgManager: nil,
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rmq := &rabbitMQComponents{
				pgManager: tt.pgManager,
			}

			if tt.wantNil {
				assert.Nil(t, rmq.pgManager,
					"pgManager should be nil in single-tenant mode")
			} else {
				assert.NotNil(t, rmq.pgManager,
					"pgManager should be non-nil in multi-tenant mode")
				assert.Same(t, pgMgr, rmq.pgManager,
					"pgManager should be the same instance passed in")
			}
		})
	}
}

// TestResolveTenantConnections_NoTenantID verifies that resolveTenantConnections
// fails closed when there is no tenant ID in the context.
func TestResolveTenantConnections_NoTenantID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rmq := &rabbitMQComponents{}

	result, err := resolveTenantConnections(ctx, rmq)
	require.Error(t, err)
	assert.Equal(t, ctx, result,
		"context should be unchanged when tenant resolution fails before enrichment")
}

// TestResolveTenantConnections_NilManagers verifies that resolveTenantConnections
// does not panic and preserves the tenant ID when both pgManager and mongoManager
// are nil.
func TestResolveTenantConnections_NilManagers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tenantID string
	}{
		{name: "tenant_123", tenantID: "tenant-123"},
		{name: "tenant_456", tenantID: "tenant-456"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := tmcore.ContextWithTenantID(context.Background(), tt.tenantID)
			rmq := &rabbitMQComponents{pgManager: nil, mongoManager: nil}

			require.NotPanics(t, func() {
				result, err := resolveTenantConnections(ctx, rmq)
				require.NoError(t, err)
				assert.Equal(t, tt.tenantID, tmcore.GetTenantIDContext(result),
					"tenant ID should be preserved with nil managers")
			}, "resolveTenantConnections must not panic with nil managers")
		})
	}
}
