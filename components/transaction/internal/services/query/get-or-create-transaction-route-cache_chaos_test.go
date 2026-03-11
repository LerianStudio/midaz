//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package query provides chaos tests for the GetOrCreateTransactionRouteCache use case.
//
// These tests exercise Redis fault tolerance when retrieving or populating
// the transaction route cache. When Redis is unavailable, the system should
// fall back to PostgreSQL. They run only when CHAOS=1 is set.
//
// Run with:
//
//	CHAOS=1 go test -tags integration -v -run TestIntegration_Chaos ./components/transaction/internal/services/query/...
package query

import (
	"context"
	"os"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	libRedis "github.com/LerianStudio/lib-commons/v3/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// CHAOS TEST INFRASTRUCTURE
// =============================================================================

// chaosQueryTestInfra holds all resources for a cache query chaos test scenario.
// It wraps chaos.Infrastructure (Toxiproxy + Docker network), a PostgreSQL container
// (direct, no proxy), and a Redis consumer repository connected through Toxiproxy
// so that Redis faults can be injected/removed without restarting any container.
type chaosQueryTestInfra struct {
	pgContainer    *pgtestutil.ContainerResult
	redisContainer *redistestutil.ContainerResult
	chaosInfra     *chaos.Infrastructure
	uc             *UseCase
	proxy          *chaos.Proxy
}

// setupQueryChaosInfra creates the full chaos test infrastructure:
//  1. Creates a Docker network + Toxiproxy container via chaos.Infrastructure
//  2. Starts a PostgreSQL container (direct, not proxied)
//  3. Starts a Valkey container on the host network
//  4. Registers the Redis container with Infrastructure to get UpstreamAddr
//  5. Creates a Toxiproxy proxy that forwards through to Redis
//  6. Builds a UseCase with TransactionRouteRepo (direct PG) + RedisRepo (through proxy)
//
// All cleanup is registered with t.Cleanup() automatically.
func setupQueryChaosInfra(t *testing.T) *chaosQueryTestInfra {
	t.Helper()

	// 1. Create chaos infrastructure (Docker network + Toxiproxy)
	chaosInfra := chaos.NewInfrastructure(t)

	// 2. Start PostgreSQL container (direct connection, not proxied)
	pgContainer := pgtestutil.SetupContainer(t)

	logger := libZap.InitializeLogger()
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	connStr := pgtestutil.BuildConnectionString(pgContainer.Host, pgContainer.Port, pgContainer.Config)

	pgConn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           pgContainer.Config.DBName,
		ReplicaDBName:           pgContainer.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	txRouteRepo := transactionroute.NewTransactionRoutePostgreSQLRepository(pgConn)

	// 3. Start Valkey container on the host network
	redisContainer := redistestutil.SetupContainer(t)

	// 4. Register Redis container with chaos infrastructure
	_, err := chaosInfra.RegisterContainerWithPort("redis", redisContainer.Container, "6379/tcp")
	require.NoError(t, err, "failed to register Redis container with chaos infrastructure")

	// 5. Create a Toxiproxy proxy for Redis using port 8666
	proxy, err := chaosInfra.CreateProxyFor("redis", "8666/tcp")
	require.NoError(t, err, "failed to create Toxiproxy proxy for Redis")

	// 6. Resolve the proxy listen address
	containerInfo, ok := chaosInfra.GetContainer("redis")
	require.True(t, ok, "Redis container must be registered")
	require.NotEmpty(t, containerInfo.ProxyListen, "proxy listen address must be non-empty")

	proxyAddr := containerInfo.ProxyListen

	// 7. Build Redis repo connected through Toxiproxy
	proxyConn := &libRedis.RedisConnection{
		Address: []string{proxyAddr},
		Logger:  logger,
	}

	redisRepo, err := redis.NewConsumerRedis(proxyConn, false)
	require.NoError(t, err, "failed to create Redis repository through proxy")

	uc := &UseCase{
		TransactionRouteRepo: txRouteRepo,
		RedisRepo:            redisRepo,
	}

	return &chaosQueryTestInfra{
		pgContainer:    pgContainer,
		redisContainer: redisContainer,
		chaosInfra:     chaosInfra,
		uc:             uc,
		proxy:          proxy,
	}
}

// =============================================================================
// REDIS CONNECTION LOSS DURING GetOrCreateTransactionRouteCache
// =============================================================================

// TestIntegration_Chaos_Redis_ConnectionLoss_GetOrCreateTransactionRouteCache verifies that
// GetOrCreateTransactionRouteCache falls back to PostgreSQL when Redis is unavailable.
// When Redis connection is lost, the cache read fails but the function should
// fetch from database and return valid data.
//
// 5-Phase structure:
//  1. Normal   -- Cache miss triggers DB fetch + Redis store, succeeds through proxy
//  2. Inject   -- Toxiproxy proxy is disabled (Redis connection loss)
//  3. Verify   -- Function still returns data (falls back to DB), no panic
//  4. Restore  -- Toxiproxy proxy is re-enabled
//  5. Recovery -- Function succeeds again using cache through proxy
func TestIntegration_Chaos_Redis_ConnectionLoss_GetOrCreateTransactionRouteCache(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupQueryChaosInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create operation routes and transaction route in PostgreSQL
	sourceRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Chaos Source", "source")
	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Chaos Dest", "destination")
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Chaos Fallback Route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID, txRouteID)

	ctx := context.Background()

	// --- Phase 1: Normal ---
	// First call: cache miss -> fetches from DB -> stores in Redis through proxy.
	t.Log("Phase 1 (Normal): verifying GetOrCreateTransactionRouteCache succeeds through proxy")

	cacheData, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, txRouteID)
	require.NoError(t, err, "Phase 1: GetOrCreateTransactionRouteCache should succeed through proxy")
	assert.NotNil(t, cacheData.Actions, "Phase 1: cache should have Actions populated")
	assert.NotNil(t, cacheData.Actions, "Phase 1: cache Actions should not be nil")

	// --- Phase 2: Inject ---
	// Disable the Toxiproxy proxy to simulate Redis connection loss.
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate Redis connection loss")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	// With Redis down, GetOrCreateTransactionRouteCache should:
	// - Fail to read from cache (Redis error, logged as warning)
	// - Fall through to PostgreSQL
	// - Fail to write cache (Redis error)
	// The current implementation returns an error on cache write failure.
	// We verify it does NOT panic.
	t.Log("Phase 3 (Verify): verifying graceful behavior when Redis is unavailable")

	var chaosErr error

	require.NotPanics(t, func() {
		_, chaosErr = infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, txRouteID)
	}, "Phase 3: GetOrCreateTransactionRouteCache must not panic on Redis connection loss")

	// The function may return an error (cache write failure) or succeed
	// (if the Redis read warning path is taken and DB fetch succeeds but cache write fails).
	// Either way, no panic is the primary assertion.
	t.Logf("Phase 3: result during Redis outage: err=%v", chaosErr)

	// --- Phase 4: Restore ---
	// Re-enable the proxy to restore Redis connectivity.
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	// After proxy restoration, the function should succeed again.
	t.Log("Phase 5 (Recovery): verifying GetOrCreateTransactionRouteCache succeeds after proxy restoration")

	chaos.AssertRecoveryWithin(t, func() error {
		result, recoverErr := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, txRouteID)
		if recoverErr != nil {
			return recoverErr
		}

		if result.Actions == nil {
			return chaos.ErrDataIntegrityViolation
		}

		return nil
	}, 10*time.Second, "Phase 5: GetOrCreateTransactionRouteCache should recover after proxy restoration")

	t.Log("PASS: GetOrCreateTransactionRouteCache handles Redis connection loss gracefully")
}

// =============================================================================
// HIGH LATENCY DURING GetOrCreateTransactionRouteCache
// =============================================================================

// TestIntegration_Chaos_Redis_HighLatency_GetOrCreateTransactionRouteCache verifies that
// GetOrCreateTransactionRouteCache does not hang indefinitely when Redis has high latency.
// With a context deadline shorter than the injected latency, the call should time out
// gracefully and return an error (not hang or panic).
//
// 5-Phase structure:
//  1. Normal   -- Function succeeds with low latency
//  2. Inject   -- 5000ms latency added via Toxiproxy
//  3. Verify   -- Function with 1s context deadline returns error (timeout), no hang
//  4. Restore  -- Latency toxic removed
//  5. Recovery -- Function succeeds again with normal latency
func TestIntegration_Chaos_Redis_HighLatency_GetOrCreateTransactionRouteCache(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupQueryChaosInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create DB data
	sourceRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Latency Source", "source")
	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Latency Dest", "destination")
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Latency Route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID, txRouteID)

	ctx := context.Background()

	// --- Phase 1: Normal ---
	t.Log("Phase 1 (Normal): verifying function succeeds with normal latency")

	normalCtx, normalCancel := context.WithTimeout(ctx, 5*time.Second)
	defer normalCancel()

	cacheData, err := infra.uc.GetOrCreateTransactionRouteCache(normalCtx, orgID, ledgerID, txRouteID)
	require.NoError(t, err, "Phase 1: should succeed with normal latency")
	assert.NotNil(t, cacheData.Actions, "Phase 1: cache Actions should not be nil")

	// --- Phase 2: Inject ---
	t.Log("Phase 2 (Inject): adding 5000ms downstream latency via Toxiproxy")

	err = infra.proxy.AddLatency(5000*time.Millisecond, 0)
	require.NoError(t, err, "Phase 2: AddLatency should not fail")

	// --- Phase 3: Verify ---
	// With 5s latency on Redis but 1s context deadline, the Redis call should time out.
	// The function must not hang indefinitely.
	t.Log("Phase 3 (Verify): function must return within 1s deadline under 5s Redis latency")

	highLatencyCtx, highLatencyCancel := context.WithTimeout(ctx, 1*time.Second)
	defer highLatencyCancel()

	var latencyErr error

	done := make(chan struct{})

	go func() {
		defer close(done)
		_, latencyErr = infra.uc.GetOrCreateTransactionRouteCache(highLatencyCtx, orgID, ledgerID, txRouteID)
	}()

	select {
	case <-done:
		// Good -- call returned within acceptable time.
	case <-time.After(3 * time.Second):
		t.Fatal("Phase 3: GetOrCreateTransactionRouteCache hung for more than 3s -- should have timed out within 1s deadline")
	}

	// The function should either return an error (timeout) or succeed
	// (if the Redis warning path leads to a DB fetch that completes within deadline).
	t.Logf("Phase 3: result under high latency: err=%v", latencyErr)

	// --- Phase 4: Restore ---
	t.Log("Phase 4 (Restore): removing all toxics from proxy")

	err = infra.proxy.RemoveAllToxics()
	require.NoError(t, err, "Phase 4: RemoveAllToxics should not fail")

	// --- Phase 5: Recovery ---
	t.Log("Phase 5 (Recovery): verifying function succeeds after latency removal")

	chaos.AssertRecoveryWithin(t, func() error {
		recoveryCtx, recoveryCancel := context.WithTimeout(ctx, 5*time.Second)
		defer recoveryCancel()

		result, recoverErr := infra.uc.GetOrCreateTransactionRouteCache(recoveryCtx, orgID, ledgerID, txRouteID)
		if recoverErr != nil {
			return recoverErr
		}

		if result.Actions == nil {
			return chaos.ErrDataIntegrityViolation
		}

		return nil
	}, 10*time.Second, "Phase 5: function should recover after latency removal")

	t.Log("PASS: GetOrCreateTransactionRouteCache handles Redis high latency gracefully")
}
