// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libRedis "github.com/LerianStudio/lib-commons/v3/commons/redis"
	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	tmpostgres "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBalanceSyncWorker_MultiTenantFields verifies that the BalanceSyncWorker
// struct contains the multi-tenant fields required for per-tenant dispatching.
// These fields (multiTenantEnabled, tenantClient, pgManager) must exist on the
// struct so that Run() can dispatch to runMultiTenant() or runSingleTenant().
func TestBalanceSyncWorker_MultiTenantFields(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	conn := &libRedis.RedisConnection{}
	useCase := &command.UseCase{}
	tenantClient := tmclient.NewClient("http://localhost:0", logger)

	tests := []struct {
		name               string
		multiTenantEnabled bool
		tenantClient       *tmclient.Client
		pgManager          *tmpostgres.Manager
		wantMultiTenant    bool
	}{
		{
			name:               "multi-tenant enabled with pgManager",
			multiTenantEnabled: true,
			tenantClient:       tenantClient,
			pgManager:          tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger)),
			wantMultiTenant:    true,
		},
		{
			name:               "single-tenant when pgManager is nil",
			multiTenantEnabled: true,
			tenantClient:       tenantClient,
			pgManager:          nil,
			wantMultiTenant:    false,
		},
		{
			name:               "single-tenant when multiTenantEnabled is false",
			multiTenantEnabled: false,
			tenantClient:       nil,
			pgManager:          nil,
			wantMultiTenant:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			worker := NewBalanceSyncWorker(conn, logger, useCase, 5)

			// These fields must exist on the struct for multi-tenant support.
			// The test will fail to compile until the fields are added.
			worker.multiTenantEnabled = tt.multiTenantEnabled
			worker.tenantClient = tt.tenantClient
			worker.pgManager = tt.pgManager

			assert.Equal(t, tt.multiTenantEnabled, worker.multiTenantEnabled,
				"multiTenantEnabled field should be set on BalanceSyncWorker")

			if tt.wantMultiTenant {
				assert.NotNil(t, worker.pgManager,
					"pgManager should be non-nil in multi-tenant mode")
				assert.NotNil(t, worker.tenantClient,
					"tenantClient should be non-nil in multi-tenant mode")
			} else {
				if !tt.multiTenantEnabled {
					assert.Nil(t, worker.pgManager,
						"pgManager should be nil when multiTenantEnabled is false")
				}
			}
		})
	}
}

// TestBalanceSyncWorker_FallbackWhenPGManagerNil verifies the invariant that
// when pgManager is nil, the worker falls back to single-tenant behavior.
// This is tested by verifying the dispatch predicate: pgManager == nil means single-tenant.
func TestBalanceSyncWorker_FallbackWhenPGManagerNil(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	conn := &libRedis.RedisConnection{}
	useCase := &command.UseCase{}

	worker := NewBalanceSyncWorker(conn, logger, useCase, 5)

	// Set multiTenantEnabled = true but leave pgManager nil
	worker.multiTenantEnabled = true
	worker.pgManager = nil

	// The dispatch logic in Run() should use: pgManager != nil -> multi-tenant
	// This asserts the fallback invariant at the struct level.
	assert.True(t, worker.multiTenantEnabled,
		"multiTenantEnabled should be true")
	assert.Nil(t, worker.pgManager,
		"pgManager should be nil, causing fallback to single-tenant behavior")

	// Verify that isMultiTenantReady() returns false when pgManager is nil.
	// This method encapsulates the dispatch predicate.
	ready := worker.isMultiTenantReady()
	assert.False(t, ready,
		"isMultiTenantReady() should return false when pgManager is nil")
}

// TestRedisQueueConsumer_MultiTenantFields verifies that the RedisQueueConsumer
// struct contains the multi-tenant fields matching the same pattern as BalanceSyncWorker.
func TestRedisQueueConsumer_MultiTenantFields(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	handler := in.TransactionHandler{}
	tenantClient := tmclient.NewClient("http://localhost:0", logger)

	tests := []struct {
		name               string
		multiTenantEnabled bool
		tenantClient       *tmclient.Client
		pgManager          *tmpostgres.Manager
		wantMultiTenant    bool
	}{
		{
			name:               "multi-tenant enabled with pgManager",
			multiTenantEnabled: true,
			tenantClient:       tenantClient,
			pgManager:          tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger)),
			wantMultiTenant:    true,
		},
		{
			name:               "single-tenant when pgManager is nil",
			multiTenantEnabled: true,
			tenantClient:       tenantClient,
			pgManager:          nil,
			wantMultiTenant:    false,
		},
		{
			name:               "single-tenant when disabled",
			multiTenantEnabled: false,
			tenantClient:       nil,
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
			consumer.tenantClient = tt.tenantClient
			consumer.pgManager = tt.pgManager

			assert.Equal(t, tt.multiTenantEnabled, consumer.multiTenantEnabled,
				"multiTenantEnabled field should be set on RedisQueueConsumer")

			if tt.wantMultiTenant {
				assert.NotNil(t, consumer.pgManager,
					"pgManager should be non-nil in multi-tenant mode")
				assert.NotNil(t, consumer.tenantClient,
					"tenantClient should be non-nil in multi-tenant mode")
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

// TestNewBalanceSyncWorkerMultiTenant verifies that a multi-tenant-aware
// constructor correctly populates all multi-tenant fields.
func TestNewBalanceSyncWorkerMultiTenant(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	conn := &libRedis.RedisConnection{}
	useCase := &command.UseCase{}
	tenantClient := tmclient.NewClient("http://localhost:0", logger)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	worker := NewBalanceSyncWorkerMultiTenant(conn, logger, useCase, 5, true, tenantClient, pgMgr)

	require.NotNil(t, worker, "worker should not be nil")
	assert.True(t, worker.multiTenantEnabled,
		"multiTenantEnabled should be true")
	assert.Same(t, tenantClient, worker.tenantClient,
		"tenantClient should be the same instance")
	assert.Same(t, pgMgr, worker.pgManager,
		"pgManager should be the same instance")
	assert.Equal(t, 5, worker.maxWorkers,
		"maxWorkers should be set correctly")
}

// TestNewRedisQueueConsumerMultiTenant verifies the multi-tenant-aware
// constructor for RedisQueueConsumer.
func TestNewRedisQueueConsumerMultiTenant(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	handler := in.TransactionHandler{}
	tenantClient := tmclient.NewClient("http://localhost:0", logger)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	consumer := NewRedisQueueConsumerMultiTenant(logger, handler, true, tenantClient, pgMgr)

	require.NotNil(t, consumer, "consumer should not be nil")
	assert.True(t, consumer.multiTenantEnabled,
		"multiTenantEnabled should be true")
	assert.Same(t, tenantClient, consumer.tenantClient,
		"tenantClient should be the same instance")
	assert.Same(t, pgMgr, consumer.pgManager,
		"pgManager should be the same instance")
}

// TestRabbitMQConsumerHandlerReceivesPGManager verifies that the
// config.rabbitmq.go wireConsumer callback can access pgManager and mongoManager
// when creating multi-tenant consumer handlers. This test validates that the
// handler signature accepts the managers needed for per-tenant connection resolution.
func TestRabbitMQConsumerHandlerReceivesPGManager(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	tenantClient := tmclient.NewClient("http://localhost:0", logger)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	// Verify that rabbitMQComponents can carry pgManager and mongoManager
	// for the consumer handler to resolve per-tenant connections.
	rmqComponents := &rabbitMQComponents{
		pgManager: pgMgr,
	}

	assert.NotNil(t, rmqComponents.pgManager,
		"rabbitMQComponents should carry pgManager for consumer handler")
}

// TestBalanceSyncWorker_IsMultiTenantReady exercises the isMultiTenantReady()
// predicate across all combinations of multiTenantEnabled x pgManager x tenantClient,
// plus the zero-value struct edge case.
func TestBalanceSyncWorker_IsMultiTenantReady(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	conn := &libRedis.RedisConnection{}
	useCase := &command.UseCase{}
	tc := tmclient.NewClient("http://localhost:0", logger)
	pgMgr := tmpostgres.NewManager(tc, "transaction", tmpostgres.WithLogger(logger))

	tests := []struct {
		name               string
		multiTenantEnabled bool
		pgManager          *tmpostgres.Manager
		tenantClient       *tmclient.Client
		want               bool
	}{
		{
			name:               "true_when_enabled_pgManager_and_tenantClient_set",
			multiTenantEnabled: true,
			pgManager:          pgMgr,
			tenantClient:       tc,
			want:               true,
		},
		{
			name:               "false_when_enabled_but_pgManager_nil",
			multiTenantEnabled: true,
			pgManager:          nil,
			tenantClient:       tc,
			want:               false,
		},
		{
			name:               "false_when_enabled_but_tenantClient_nil",
			multiTenantEnabled: true,
			pgManager:          pgMgr,
			tenantClient:       nil,
			want:               false,
		},
		{
			name:               "false_when_disabled_but_pgManager_set",
			multiTenantEnabled: false,
			pgManager:          pgMgr,
			tenantClient:       tc,
			want:               false,
		},
		{
			name:               "false_when_disabled_and_pgManager_nil",
			multiTenantEnabled: false,
			pgManager:          nil,
			tenantClient:       nil,
			want:               false,
		},
		{
			name:               "false_for_zero_value_struct",
			multiTenantEnabled: false,
			pgManager:          nil,
			tenantClient:       nil,
			want:               false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var worker *BalanceSyncWorker
			if tt.name == "false_for_zero_value_struct" {
				worker = &BalanceSyncWorker{}
			} else {
				worker = NewBalanceSyncWorker(conn, logger, useCase, 5)
				worker.multiTenantEnabled = tt.multiTenantEnabled
				worker.pgManager = tt.pgManager
				worker.tenantClient = tt.tenantClient
			}

			got := worker.isMultiTenantReady()
			assert.Equal(t, tt.want, got,
				"isMultiTenantReady() should return %v", tt.want)
		})
	}
}

// TestRedisQueueConsumer_IsMultiTenantReady exercises the isMultiTenantReady()
// predicate across all combinations of multiTenantEnabled x pgManager x tenantClient,
// plus the zero-value struct edge case.
func TestRedisQueueConsumer_IsMultiTenantReady(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	handler := in.TransactionHandler{}
	tc := tmclient.NewClient("http://localhost:0", logger)
	pgMgr := tmpostgres.NewManager(tc, "transaction", tmpostgres.WithLogger(logger))

	tests := []struct {
		name               string
		multiTenantEnabled bool
		pgManager          *tmpostgres.Manager
		tenantClient       *tmclient.Client
		want               bool
	}{
		{
			name:               "true_when_enabled_pgManager_and_tenantClient_set",
			multiTenantEnabled: true,
			pgManager:          pgMgr,
			tenantClient:       tc,
			want:               true,
		},
		{
			name:               "false_when_enabled_but_pgManager_nil",
			multiTenantEnabled: true,
			pgManager:          nil,
			tenantClient:       tc,
			want:               false,
		},
		{
			name:               "false_when_enabled_but_tenantClient_nil",
			multiTenantEnabled: true,
			pgManager:          pgMgr,
			tenantClient:       nil,
			want:               false,
		},
		{
			name:               "false_when_disabled_but_pgManager_set",
			multiTenantEnabled: false,
			pgManager:          pgMgr,
			tenantClient:       tc,
			want:               false,
		},
		{
			name:               "false_when_disabled_and_pgManager_nil",
			multiTenantEnabled: false,
			pgManager:          nil,
			tenantClient:       nil,
			want:               false,
		},
		{
			name:               "false_for_zero_value_struct",
			multiTenantEnabled: false,
			pgManager:          nil,
			tenantClient:       nil,
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
				consumer.tenantClient = tt.tenantClient
			}

			got := consumer.isMultiTenantReady()
			assert.Equal(t, tt.want, got,
				"isMultiTenantReady() should return %v", tt.want)
		})
	}
}

// TestNewBalanceSyncWorkerMultiTenant_EdgeCases covers constructor edge cases:
// disabled mode with non-nil deps, nil tenantClient with non-nil pgManager, and all-nil.
func TestNewBalanceSyncWorkerMultiTenant_EdgeCases(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	conn := &libRedis.RedisConnection{}
	useCase := &command.UseCase{}
	tenantClient := tmclient.NewClient("http://localhost:0", logger)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	tests := []struct {
		name               string
		multiTenantEnabled bool
		tenantClient       *tmclient.Client
		pgManager          *tmpostgres.Manager
		wantEnabled        bool
		wantReady          bool
	}{
		{
			name:               "disabled_with_non_nil_deps",
			multiTenantEnabled: false,
			tenantClient:       tenantClient,
			pgManager:          pgMgr,
			wantEnabled:        false,
			wantReady:          false,
		},
		{
			name:               "nil_tenantClient_with_non_nil_pgManager",
			multiTenantEnabled: true,
			tenantClient:       nil,
			pgManager:          pgMgr,
			wantEnabled:        true,
			wantReady:          false,
		},
		{
			name:               "all_nil_disabled",
			multiTenantEnabled: false,
			tenantClient:       nil,
			pgManager:          nil,
			wantEnabled:        false,
			wantReady:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			worker := NewBalanceSyncWorkerMultiTenant(
				conn, logger, useCase, 5,
				tt.multiTenantEnabled, tt.tenantClient, tt.pgManager,
			)

			require.NotNil(t, worker, "constructor must return non-nil")
			assert.Equal(t, tt.wantEnabled, worker.multiTenantEnabled,
				"multiTenantEnabled should match input")
			assert.Equal(t, tt.wantReady, worker.isMultiTenantReady(),
				"isMultiTenantReady() should reflect field combination")

			if tt.tenantClient == nil {
				assert.Nil(t, worker.tenantClient,
					"tenantClient should be nil when passed nil")
			}
		})
	}
}

// TestNewRedisQueueConsumerMultiTenant_EdgeCases covers constructor edge cases:
// disabled mode with non-nil deps, nil tenantClient with non-nil pgManager, and all-nil.
func TestNewRedisQueueConsumerMultiTenant_EdgeCases(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	handler := in.TransactionHandler{}
	tenantClient := tmclient.NewClient("http://localhost:0", logger)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	tests := []struct {
		name               string
		multiTenantEnabled bool
		tenantClient       *tmclient.Client
		pgManager          *tmpostgres.Manager
		wantEnabled        bool
		wantReady          bool
	}{
		{
			name:               "disabled_with_non_nil_deps",
			multiTenantEnabled: false,
			tenantClient:       tenantClient,
			pgManager:          pgMgr,
			wantEnabled:        false,
			wantReady:          false,
		},
		{
			name:               "nil_tenantClient_with_non_nil_pgManager",
			multiTenantEnabled: true,
			tenantClient:       nil,
			pgManager:          pgMgr,
			wantEnabled:        true,
			wantReady:          false,
		},
		{
			name:               "all_nil_disabled",
			multiTenantEnabled: false,
			tenantClient:       nil,
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
				tt.multiTenantEnabled, tt.tenantClient, tt.pgManager,
			)

			require.NotNil(t, consumer, "constructor must return non-nil")
			assert.Equal(t, tt.wantEnabled, consumer.multiTenantEnabled,
				"multiTenantEnabled should match input")
			assert.Equal(t, tt.wantReady, consumer.isMultiTenantReady(),
				"isMultiTenantReady() should reflect field combination")

			if tt.tenantClient == nil {
				assert.Nil(t, consumer.tenantClient,
					"tenantClient should be nil when passed nil")
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
	conn := &libRedis.RedisConnection{}
	useCase := &command.UseCase{}

	tests := []struct {
		name           string
		maxWorkers     int
		wantMaxWorkers int
	}{
		{
			name:           "positive_maxWorkers_preserved",
			maxWorkers:     5,
			wantMaxWorkers: 5,
		},
		{
			name:           "zero_maxWorkers_defaults_to_5",
			maxWorkers:     0,
			wantMaxWorkers: 5,
		},
		{
			name:           "negative_maxWorkers_defaults_to_5",
			maxWorkers:     -1,
			wantMaxWorkers: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			worker := NewBalanceSyncWorker(conn, logger, useCase, tt.maxWorkers)

			require.NotNil(t, worker, "base constructor must return non-nil")
			assert.Equal(t, tt.wantMaxWorkers, worker.maxWorkers,
				"maxWorkers should be %d", tt.wantMaxWorkers)
			assert.False(t, worker.multiTenantEnabled,
				"multiTenantEnabled should default to false")
			assert.Nil(t, worker.tenantClient,
				"tenantClient should default to nil")
			assert.Nil(t, worker.pgManager,
				"pgManager should default to nil")
			assert.False(t, worker.isMultiTenantReady(),
				"isMultiTenantReady() should be false for base constructor")
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
	assert.Nil(t, consumer.tenantClient,
		"tenantClient should default to nil")
	assert.Nil(t, consumer.pgManager,
		"pgManager should default to nil")
	assert.False(t, consumer.isMultiTenantReady(),
		"isMultiTenantReady() should be false for base constructor")
}

// TestRabbitMQComponents_PGManagerField verifies that rabbitMQComponents
// correctly stores and exposes nil vs non-nil pgManager.
func TestRabbitMQComponents_PGManagerField(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	tenantClient := tmclient.NewClient("http://localhost:0", logger)
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

// TestBalanceSyncWorker_RunDispatchesBasedOnMultiTenantReady verifies that the
// Run() dispatch predicate (isMultiTenantReady) returns the correct value for
// single-tenant and multi-tenant configurations. Run() itself blocks, so we
// test the predicate that governs which branch Run() takes.
func TestBalanceSyncWorker_RunDispatchesBasedOnMultiTenantReady(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	conn := &libRedis.RedisConnection{}
	useCase := &command.UseCase{}
	tc := tmclient.NewClient("http://localhost:0", logger)
	pgMgr := tmpostgres.NewManager(tc, "transaction", tmpostgres.WithLogger(logger))

	tests := []struct {
		name               string
		multiTenantEnabled bool
		pgManager          *tmpostgres.Manager
		tenantClient       *tmclient.Client
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
			tenantClient:       tc,
			wantReady:          true,
		},
		{
			name:               "enabled_but_nil_pgManager_falls_back_to_single",
			multiTenantEnabled: true,
			pgManager:          nil,
			tenantClient:       tc,
			wantReady:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			worker := NewBalanceSyncWorker(conn, logger, useCase, 5)
			worker.multiTenantEnabled = tt.multiTenantEnabled
			worker.pgManager = tt.pgManager
			worker.tenantClient = tt.tenantClient

			got := worker.isMultiTenantReady()
			assert.Equal(t, tt.wantReady, got,
				"isMultiTenantReady() governs Run() dispatch: want %v", tt.wantReady)
		})
	}
}

// TestRedisQueueConsumer_RunDispatchesBasedOnMultiTenantReady verifies that the
// Run() dispatch predicate (isMultiTenantReady) returns the correct value for
// single-tenant and multi-tenant configurations on RedisQueueConsumer.
func TestRedisQueueConsumer_RunDispatchesBasedOnMultiTenantReady(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	handler := in.TransactionHandler{}
	tc := tmclient.NewClient("http://localhost:0", logger)
	pgMgr := tmpostgres.NewManager(tc, "transaction", tmpostgres.WithLogger(logger))

	tests := []struct {
		name               string
		multiTenantEnabled bool
		pgManager          *tmpostgres.Manager
		tenantClient       *tmclient.Client
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
			tenantClient:       tc,
			wantReady:          true,
		},
		{
			name:               "enabled_but_nil_pgManager_falls_back_to_single",
			multiTenantEnabled: true,
			pgManager:          nil,
			tenantClient:       tc,
			wantReady:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			consumer := NewRedisQueueConsumer(logger, handler)
			consumer.multiTenantEnabled = tt.multiTenantEnabled
			consumer.pgManager = tt.pgManager
			consumer.tenantClient = tt.tenantClient

			got := consumer.isMultiTenantReady()
			assert.Equal(t, tt.wantReady, got,
				"isMultiTenantReady() governs Run() dispatch: want %v", tt.wantReady)
		})
	}
}

// TestResolveTenantConnections_NoTenantID verifies that resolveTenantConnections
// returns the context unchanged and no error when there is no tenant ID in the
// context (single-tenant fallback path).
func TestResolveTenantConnections_NoTenantID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rmq := &rabbitMQComponents{}

	result, err := resolveTenantConnections(ctx, rmq)
	require.NoError(t, err)
	assert.Equal(t, ctx, result,
		"context should be unchanged when no tenant ID is present")
}

// TestResolveTenantConnections_NilManagers verifies that resolveTenantConnections
// does not panic and preserves the tenant ID when both pgManager and mongoManager
// are nil. This covers the graceful degradation path where multi-tenant RabbitMQ
// is active but PG/Mongo managers have not been wired yet.
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
				assert.Equal(t, tt.tenantID, tmcore.GetTenantIDFromContext(result),
					"tenant ID should be preserved with nil managers")
			}, "resolveTenantConnections must not panic with nil managers")
		})
	}
}

// TestBalanceSyncWorker_MultiTenantConstructorPreservesRunBehavior verifies that
// NewBalanceSyncWorkerMultiTenant produces a worker where isMultiTenantReady()
// matches the expected value and the Run method is callable (compile-time check
// via interface satisfaction with *libCommons.Launcher).
func TestBalanceSyncWorker_MultiTenantConstructorPreservesRunBehavior(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	conn := &libRedis.RedisConnection{}
	useCase := &command.UseCase{}
	tenantClient := tmclient.NewClient("http://localhost:0", logger)
	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	tests := []struct {
		name               string
		multiTenantEnabled bool
		pgManager          *tmpostgres.Manager
		wantReady          bool
	}{
		{
			name:               "multi_tenant_ready",
			multiTenantEnabled: true,
			pgManager:          pgMgr,
			wantReady:          true,
		},
		{
			name:               "single_tenant_fallback",
			multiTenantEnabled: false,
			pgManager:          nil,
			wantReady:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			worker := NewBalanceSyncWorkerMultiTenant(
				conn, logger, useCase, 5,
				tt.multiTenantEnabled, tenantClient, tt.pgManager,
			)

			require.NotNil(t, worker, "constructor must return non-nil worker")
			assert.Equal(t, tt.wantReady, worker.isMultiTenantReady(),
				"isMultiTenantReady() should match expected dispatch path")

			// Compile-time verification that Run() accepts *libCommons.Launcher.
			// We assign to a func variable to prove the method exists without
			// actually invoking it (Run blocks on signal/Redis).
			var runFn func(*libCommons.Launcher) error = worker.Run
			assert.NotNil(t, runFn,
				"Run method must exist and accept *libCommons.Launcher")
		})
	}
}
