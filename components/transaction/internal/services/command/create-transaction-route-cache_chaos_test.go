//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package command provides chaos tests for cache write operations.
//
// These tests exercise Redis fault tolerance during cache write operations
// (CreateAccountingRouteCache). When Redis is unavailable or has high latency,
// the system should return errors gracefully without panicking or hanging.
// They run only when CHAOS=1 is set.
//
// Run with:
//
//	CHAOS=1 go test -tags integration -v -run TestIntegration_Chaos ./components/transaction/internal/services/command/...
package command

import (
	"context"
	"os"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libRedis "github.com/LerianStudio/lib-commons/v4/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v4/commons/zap"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// CHAOS TEST INFRASTRUCTURE (CACHE WRITE)
// =============================================================================

// chaosCacheWriteTestInfra holds all resources for cache write chaos test scenarios.
// It wraps chaos.Infrastructure (Toxiproxy + Docker network) and a Redis consumer
// repository connected through the Toxiproxy proxy so that Redis faults can be
// injected/removed without restarting any container.
type chaosCacheWriteTestInfra struct {
	redisContainer *redistestutil.ContainerResult
	chaosInfra     *chaos.Infrastructure
	uc             *UseCase
	proxy          *chaos.Proxy
}

// setupCacheWriteChaosInfra creates the chaos test infrastructure for cache write tests:
//  1. Creates a Docker network + Toxiproxy container via chaos.Infrastructure
//  2. Starts a Valkey container on the host network
//  3. Registers the Redis container with Infrastructure to get UpstreamAddr
//  4. Creates a Toxiproxy proxy that forwards through to Redis
//  5. Builds a UseCase with RedisRepo connected through the proxy address
//
// All cleanup is registered with t.Cleanup() automatically.
func setupCacheWriteChaosInfra(t *testing.T) *chaosCacheWriteTestInfra {
	t.Helper()

	// 1. Create chaos infrastructure (Docker network + Toxiproxy)
	chaosInfra := chaos.NewInfrastructure(t)

	// 2. Start Valkey container on the host network
	redisContainer := redistestutil.SetupContainer(t)

	// 3. Register Redis container with chaos infrastructure
	_, err := chaosInfra.RegisterContainerWithPort("redis", redisContainer.Container, "6379/tcp")
	require.NoError(t, err, "failed to register Redis container with chaos infrastructure")

	// 4. Create a Toxiproxy proxy for Redis using port 8666
	proxy, err := chaosInfra.CreateProxyFor("redis", "8666/tcp")
	require.NoError(t, err, "failed to create Toxiproxy proxy for Redis")

	// 5. Resolve the proxy listen address
	containerInfo, ok := chaosInfra.GetContainer("redis")
	require.True(t, ok, "Redis container must be registered")
	require.NotEmpty(t, containerInfo.ProxyListen, "proxy listen address must be non-empty")

	proxyAddr := containerInfo.ProxyListen

	// 6. Build Redis repo connected through Toxiproxy
	logger := libZap.InitializeLogger()
	proxyConn := &libRedis.RedisConnection{
		Address: []string{proxyAddr},
		Logger:  logger,
	}

	redisRepo, err := redis.NewConsumerRedis(proxyConn, false)
	require.NoError(t, err, "failed to create Redis repository through proxy")

	uc := &UseCase{
		RedisRepo: redisRepo,
	}

	return &chaosCacheWriteTestInfra{
		redisContainer: redisContainer,
		chaosInfra:     chaosInfra,
		uc:             uc,
		proxy:          proxy,
	}
}

// makeTestRoute creates a TransactionRoute with operation routes for testing.
func makeTestRoute() *mmodel.TransactionRoute {
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()
	sourceID := libCommons.GenerateUUIDv7()
	destID := libCommons.GenerateUUIDv7()

	return &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Title:          "Chaos Test Route",
		Description:    "Route for chaos testing cache writes",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:                sourceID,
				OperationType:     "source",
				Action:            "direct",
				AccountingEntries: &mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{}},
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@chaos_source",
				},
			},
			{
				ID:                destID,
				OperationType:     "destination",
				Action:            "direct",
				AccountingEntries: &mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{}},
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@chaos_dest",
				},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// =============================================================================
// REDIS CONNECTION LOSS DURING CreateAccountingRouteCache
// =============================================================================

// TestIntegration_Chaos_Redis_ConnectionLoss_CreateAccountingRouteCache verifies that
// CreateAccountingRouteCache returns an error (and does not panic) when Redis
// connection is lost via Toxiproxy during a cache write operation.
//
// 5-Phase structure:
//  1. Normal   -- CreateAccountingRouteCache succeeds through proxy
//  2. Inject   -- Toxiproxy proxy is disabled (connection loss)
//  3. Verify   -- CreateAccountingRouteCache returns error; no panic
//  4. Restore  -- Toxiproxy proxy is re-enabled
//  5. Recovery -- CreateAccountingRouteCache succeeds again
func TestIntegration_Chaos_Redis_ConnectionLoss_CreateAccountingRouteCache(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupCacheWriteChaosInfra(t)

	route := makeTestRoute()
	ctx := context.Background()

	// --- Phase 1: Normal ---
	// Verify CreateAccountingRouteCache works through the proxy before any fault injection.
	t.Log("Phase 1 (Normal): verifying CreateAccountingRouteCache succeeds through proxy")

	err := infra.uc.CreateAccountingRouteCache(ctx, route)
	require.NoError(t, err, "Phase 1: CreateAccountingRouteCache should succeed through proxy")

	// Verify cache was stored
	internalKey := utils.AccountingRoutesInternalKey(route.OrganizationID, route.LedgerID, route.ID)
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "Phase 1: cache should be readable after write")
	assert.NotEmpty(t, cachedBytes, "Phase 1: cached bytes should not be empty")

	// --- Phase 2: Inject ---
	// Disable the Toxiproxy proxy to simulate Redis connection loss.
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate Redis connection loss")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	// CreateAccountingRouteCache must return an error (not panic) when Redis is down.
	t.Log("Phase 3 (Verify): CreateAccountingRouteCache must return error, not panic")

	newRoute := makeTestRoute()
	var chaosErr error

	require.NotPanics(t, func() {
		chaosErr = infra.uc.CreateAccountingRouteCache(ctx, newRoute)
	}, "Phase 3: CreateAccountingRouteCache must not panic on Redis connection loss")

	assert.Error(t, chaosErr,
		"Phase 3: CreateAccountingRouteCache must return error when Redis is unavailable")

	t.Logf("Phase 3: received expected error: %v", chaosErr)

	// --- Phase 4: Restore ---
	// Re-enable the proxy to restore Redis connectivity.
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	// After proxy restoration, CreateAccountingRouteCache should succeed again.
	t.Log("Phase 5 (Recovery): verifying CreateAccountingRouteCache succeeds after proxy restoration")

	recoveryRoute := makeTestRoute()

	chaos.AssertRecoveryWithin(t, func() error {
		return infra.uc.CreateAccountingRouteCache(ctx, recoveryRoute)
	}, 10*time.Second, "Phase 5: CreateAccountingRouteCache should recover after proxy restoration")

	// Verify the recovery route was actually stored
	recoveryKey := utils.AccountingRoutesInternalKey(recoveryRoute.OrganizationID, recoveryRoute.LedgerID, recoveryRoute.ID)
	recoveryBytes, err := infra.uc.RedisRepo.GetBytes(ctx, recoveryKey)
	require.NoError(t, err, "Phase 5: recovery cache should be readable")
	assert.NotEmpty(t, recoveryBytes, "Phase 5: recovery cache bytes should not be empty")

	t.Log("PASS: CreateAccountingRouteCache handles Redis connection loss gracefully")
}

// =============================================================================
// HIGH LATENCY DURING CreateAccountingRouteCache
// =============================================================================

// TestIntegration_Chaos_Redis_HighLatency_CreateAccountingRouteCache verifies that
// CreateAccountingRouteCache does not block indefinitely when Redis has high latency.
// With a context deadline shorter than the injected latency, the call should time out
// gracefully and return an error (not hang or panic).
//
// 5-Phase structure:
//  1. Normal   -- CreateAccountingRouteCache succeeds with low latency
//  2. Inject   -- 5000ms latency added via Toxiproxy
//  3. Verify   -- CreateAccountingRouteCache with 1s context deadline returns error (timeout)
//  4. Restore  -- Latency toxic removed
//  5. Recovery -- CreateAccountingRouteCache succeeds again with normal latency
func TestIntegration_Chaos_Redis_HighLatency_CreateAccountingRouteCache(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupCacheWriteChaosInfra(t)
	ctx := context.Background()

	// --- Phase 1: Normal ---
	t.Log("Phase 1 (Normal): verifying CreateAccountingRouteCache succeeds with normal latency")

	route := makeTestRoute()

	normalCtx, normalCancel := context.WithTimeout(ctx, 5*time.Second)
	defer normalCancel()

	err := infra.uc.CreateAccountingRouteCache(normalCtx, route)
	require.NoError(t, err, "Phase 1: should succeed with normal latency")

	// --- Phase 2: Inject ---
	t.Log("Phase 2 (Inject): adding 5000ms downstream latency via Toxiproxy")

	err = infra.proxy.AddLatency(5000*time.Millisecond, 0)
	require.NoError(t, err, "Phase 2: AddLatency should not fail")

	// --- Phase 3: Verify ---
	// With 5s latency on Redis but 1s context deadline, the Redis write should time out.
	t.Log("Phase 3 (Verify): CreateAccountingRouteCache must return within 1s deadline under 5s Redis latency")

	highLatencyRoute := makeTestRoute()
	highLatencyCtx, highLatencyCancel := context.WithTimeout(ctx, 1*time.Second)
	defer highLatencyCancel()

	var latencyErr error

	done := make(chan struct{})

	go func() {
		defer close(done)
		latencyErr = infra.uc.CreateAccountingRouteCache(highLatencyCtx, highLatencyRoute)
	}()

	select {
	case <-done:
		// Good -- call returned within acceptable time.
	case <-time.After(3 * time.Second):
		t.Fatal("Phase 3: CreateAccountingRouteCache hung for more than 3s -- should have timed out within 1s deadline")
	}

	assert.Error(t, latencyErr,
		"Phase 3: CreateAccountingRouteCache should return error under high latency with short deadline")

	t.Logf("Phase 3: result under high latency: err=%v", latencyErr)

	// --- Phase 4: Restore ---
	t.Log("Phase 4 (Restore): removing all toxics from proxy")

	err = infra.proxy.RemoveAllToxics()
	require.NoError(t, err, "Phase 4: RemoveAllToxics should not fail")

	// --- Phase 5: Recovery ---
	t.Log("Phase 5 (Recovery): verifying CreateAccountingRouteCache succeeds after latency removal")

	chaos.AssertRecoveryWithin(t, func() error {
		recoveryRoute := makeTestRoute()

		recoveryCtx, recoveryCancel := context.WithTimeout(ctx, 5*time.Second)
		defer recoveryCancel()

		return infra.uc.CreateAccountingRouteCache(recoveryCtx, recoveryRoute)
	}, 10*time.Second, "Phase 5: CreateAccountingRouteCache should recover after latency removal")

	t.Log("PASS: CreateAccountingRouteCache handles Redis high latency gracefully")
}
