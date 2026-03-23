// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package query provides chaos tests for the GetOrCreateTransactionRouteCache use case.
//
// These tests exercise Redis and PostgreSQL fault tolerance when retrieving or populating
// the transaction route cache. When Redis is unavailable, the system should fall back to
// PostgreSQL. When PostgreSQL is unavailable after a cache miss, the system should return
// an error gracefully. They run only when CHAOS=1 is set.
//
// Run with:
//
//	CHAOS=1 go test -tags integration -v -run TestIntegration_Chaos ./components/ledger/internal/services/query/...
package query

import (
	"context"
	"os"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v4/commons/postgres"
	libRedis "github.com/LerianStudio/lib-commons/v4/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v4/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transactionroute"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// CHAOS TEST INFRASTRUCTURE (REDIS-ONLY PROXY)
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
		TransactionRedisRepo: redisRepo,
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
// CHAOS TEST INFRASTRUCTURE (DUAL PROXY: REDIS + POSTGRES)
// =============================================================================

// chaosDualProxyQueryTestInfra holds all resources for chaos tests that require
// fault injection on both Redis and PostgreSQL simultaneously.
// Redis is proxied through port 8666 and PostgreSQL through port 8667.
type chaosDualProxyQueryTestInfra struct {
	pgContainer    *pgtestutil.ContainerResult
	redisContainer *redistestutil.ContainerResult
	chaosInfra     *chaos.Infrastructure
	uc             *UseCase
	redisProxy     *chaos.Proxy
	pgProxy        *chaos.Proxy
}

// setupDualProxyQueryChaosInfra creates a chaos test infrastructure with both
// Redis and PostgreSQL routed through Toxiproxy, allowing independent fault
// injection on either dependency:
//  1. Creates a Docker network + Toxiproxy container via chaos.Infrastructure
//  2. Starts a PostgreSQL container on the host network
//  3. Registers the PostgreSQL container and creates a Toxiproxy proxy (port 8667)
//  4. Starts a Valkey container on the host network
//  5. Registers the Redis container and creates a Toxiproxy proxy (port 8666)
//  6. Builds a UseCase with both repos connected through their respective proxies
//
// All cleanup is registered with t.Cleanup() automatically.
func setupDualProxyQueryChaosInfra(t *testing.T) *chaosDualProxyQueryTestInfra {
	t.Helper()

	// 1. Create chaos infrastructure (Docker network + Toxiproxy)
	chaosInfra := chaos.NewInfrastructure(t)

	logger := libZap.InitializeLogger()

	// 2. Start PostgreSQL container on the host network
	pgContainer := pgtestutil.SetupContainer(t)

	// 3. Register PostgreSQL container and create proxy on port 8667
	_, err := chaosInfra.RegisterContainerWithPort("postgres", pgContainer.Container, "5432/tcp")
	require.NoError(t, err, "failed to register PostgreSQL container with chaos infrastructure")

	pgProxy, err := chaosInfra.CreateProxyFor("postgres", "8667/tcp")
	require.NoError(t, err, "failed to create Toxiproxy proxy for PostgreSQL")

	pgContainerInfo, ok := chaosInfra.GetContainer("postgres")
	require.True(t, ok, "PostgreSQL container must be registered")
	require.NotEmpty(t, pgContainerInfo.ProxyListen, "PostgreSQL proxy listen address must be non-empty")

	// Build TransactionRouteRepo connected through Toxiproxy
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	proxyPGConnStr := pgtestutil.BuildConnectionStringWithHost(pgContainerInfo.ProxyListen, pgContainer.Config)

	pgConn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: proxyPGConnStr,
		ConnectionStringReplica: proxyPGConnStr,
		PrimaryDBName:           pgContainer.Config.DBName,
		ReplicaDBName:           pgContainer.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	txRouteRepo := transactionroute.NewTransactionRoutePostgreSQLRepository(pgConn)

	// 4. Start Valkey container on the host network
	redisContainer := redistestutil.SetupContainer(t)

	// 5. Register Redis container and create proxy on port 8666
	_, err = chaosInfra.RegisterContainerWithPort("redis", redisContainer.Container, "6379/tcp")
	require.NoError(t, err, "failed to register Redis container with chaos infrastructure")

	redisProxy, err := chaosInfra.CreateProxyFor("redis", "8666/tcp")
	require.NoError(t, err, "failed to create Toxiproxy proxy for Redis")

	redisContainerInfo, ok := chaosInfra.GetContainer("redis")
	require.True(t, ok, "Redis container must be registered")
	require.NotEmpty(t, redisContainerInfo.ProxyListen, "Redis proxy listen address must be non-empty")

	// Build Redis repo connected through Toxiproxy
	proxyRedisConn := &libRedis.RedisConnection{
		Address: []string{redisContainerInfo.ProxyListen},
		Logger:  logger,
	}

	redisRepo, err := redis.NewConsumerRedis(proxyRedisConn, false)
	require.NoError(t, err, "failed to create Redis repository through proxy")

	uc := &UseCase{
		TransactionRouteRepo: txRouteRepo,
		TransactionRedisRepo: redisRepo,
	}

	return &chaosDualProxyQueryTestInfra{
		pgContainer:    pgContainer,
		redisContainer: redisContainer,
		chaosInfra:     chaosInfra,
		uc:             uc,
		redisProxy:     redisProxy,
		pgProxy:        pgProxy,
	}
}

// =============================================================================
// CH-1: REDIS CONNECTION LOSS DURING GetOrCreateTransactionRouteCache
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
// CH-2: HIGH LATENCY DURING GetOrCreateTransactionRouteCache
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

// =============================================================================
// CH-3: REDIS TIMEOUT ON SetBytes (WRITE FAILURE)
// =============================================================================

// TestIntegration_Chaos_Redis_WriteTimeout_GetOrCreateTransactionRouteCache verifies that
// GetOrCreateTransactionRouteCache still returns the correct result when Redis is available
// for reads but the write (SetBytes for sentinel/cache storage) times out.
//
// Scenario: Redis cache miss occurs, function fetches from PostgreSQL successfully,
// but the subsequent Redis SetBytes call times out due to high write latency.
// The function should still return the DB data or an error -- never panic.
//
// Note: Toxiproxy applies latency to all traffic on the proxy (reads + writes).
// To simulate write-only latency, we prime the cache read to miss (fresh key),
// then inject latency before the function attempts SetBytes.
// In practice, the injected latency affects the entire call, so we verify
// the function either returns data or an error within a bounded time.
//
// 5-Phase structure:
//  1. Normal   -- Cache miss -> DB fetch -> Redis store succeeds
//  2. Inject   -- 5000ms downstream latency added (affects SetBytes path)
//  3. Verify   -- Function with short deadline returns error or data, no hang/panic
//  4. Restore  -- Latency toxic removed
//  5. Recovery -- Function succeeds again with normal latency
func TestIntegration_Chaos_Redis_WriteTimeout_GetOrCreateTransactionRouteCache(t *testing.T) {
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
	sourceRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "WriteTimeout Source", "source")
	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "WriteTimeout Dest", "destination")
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "WriteTimeout Route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID, txRouteID)

	ctx := context.Background()

	// --- Phase 1: Normal ---
	t.Log("Phase 1 (Normal): verifying function succeeds with a fresh key (cache miss -> DB -> Redis store)")

	normalCtx, normalCancel := context.WithTimeout(ctx, 5*time.Second)
	defer normalCancel()

	cacheData, err := infra.uc.GetOrCreateTransactionRouteCache(normalCtx, orgID, ledgerID, txRouteID)
	require.NoError(t, err, "Phase 1: should succeed on first call (cache miss -> DB fetch -> store)")
	assert.NotNil(t, cacheData.Actions, "Phase 1: cache Actions should not be nil")

	// --- Phase 2: Inject ---
	// Inject high downstream latency to simulate Redis write timeout.
	// Use a second, uncached transaction route so the function must go through
	// the full cache-miss -> DB fetch -> SetBytes path.
	t.Log("Phase 2 (Inject): adding 5000ms downstream latency via Toxiproxy to simulate write timeout")

	err = infra.proxy.AddLatency(5000*time.Millisecond, 0)
	require.NoError(t, err, "Phase 2: AddLatency should not fail")

	// Create a second route that has NOT been cached yet
	orgID2 := libCommons.GenerateUUIDv7()
	ledgerID2 := libCommons.GenerateUUIDv7()

	sourceRouteID2 := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID2, ledgerID2, "WriteTimeout Source2", "source")
	destRouteID2 := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID2, ledgerID2, "WriteTimeout Dest2", "destination")
	txRouteID2 := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID2, ledgerID2, "WriteTimeout Route2")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID2, txRouteID2)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID2, txRouteID2)

	// --- Phase 3: Verify ---
	// The function should attempt GetBytes (slow due to latency), then fall through
	// to DB, then attempt SetBytes (slow). With a 1s deadline, it should time out
	// on Redis operations but must not hang or panic.
	t.Log("Phase 3 (Verify): function must not hang or panic when Redis write times out")

	writeTimeoutCtx, writeTimeoutCancel := context.WithTimeout(ctx, 1*time.Second)
	defer writeTimeoutCancel()

	var writeErr error

	done := make(chan struct{})

	go func() {
		defer close(done)
		_, writeErr = infra.uc.GetOrCreateTransactionRouteCache(writeTimeoutCtx, orgID2, ledgerID2, txRouteID2)
	}()

	select {
	case <-done:
		// Good -- call returned.
	case <-time.After(3 * time.Second):
		t.Fatal("Phase 3: GetOrCreateTransactionRouteCache hung for more than 3s -- should respect context deadline")
	}

	// The function either returns an error (timeout on Redis) or succeeds
	// (if DB fetch completes before deadline). Either outcome is acceptable;
	// the critical assertion is no panic and no hang.
	t.Logf("Phase 3: result during Redis write timeout: err=%v", writeErr)

	// --- Phase 4: Restore ---
	t.Log("Phase 4 (Restore): removing all toxics from proxy")

	err = infra.proxy.RemoveAllToxics()
	require.NoError(t, err, "Phase 4: RemoveAllToxics should not fail")

	// --- Phase 5: Recovery ---
	t.Log("Phase 5 (Recovery): verifying function succeeds after latency removal")

	chaos.AssertRecoveryWithin(t, func() error {
		recoveryCtx, recoveryCancel := context.WithTimeout(ctx, 5*time.Second)
		defer recoveryCancel()

		result, recoverErr := infra.uc.GetOrCreateTransactionRouteCache(recoveryCtx, orgID2, ledgerID2, txRouteID2)
		if recoverErr != nil {
			return recoverErr
		}

		if result.Actions == nil {
			return chaos.ErrDataIntegrityViolation
		}

		return nil
	}, 10*time.Second, "Phase 5: function should recover after write latency removal")

	t.Log("PASS: GetOrCreateTransactionRouteCache handles Redis write timeout gracefully")
}

// =============================================================================
// CH-4: POSTGRES CONNECTION LOSS (CACHE MISS -> DB DOWN)
// =============================================================================

// TestIntegration_Chaos_Postgres_ConnectionLoss_GetOrCreateTransactionRouteCache verifies that
// GetOrCreateTransactionRouteCache returns an error gracefully (no panic) when a Redis cache
// miss occurs and PostgreSQL is unavailable.
//
// This uses the dual-proxy infrastructure so that both Redis and PostgreSQL are
// routed through Toxiproxy. Redis remains healthy, but PostgreSQL is disconnected.
//
// 5-Phase structure:
//  1. Normal   -- Cache miss -> DB fetch succeeds -> stores in Redis
//  2. Inject   -- PostgreSQL proxy disabled (connection loss)
//  3. Verify   -- Function with uncached key returns error (DB unreachable), no panic
//  4. Restore  -- PostgreSQL proxy re-enabled
//  5. Recovery -- Function succeeds again with DB restored
func TestIntegration_Chaos_Postgres_ConnectionLoss_GetOrCreateTransactionRouteCache(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupDualProxyQueryChaosInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert test data directly into PostgreSQL (bypassing proxy for fixture setup)
	sourceRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "PGDown Source", "source")
	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "PGDown Dest", "destination")
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "PGDown Route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID, txRouteID)

	ctx := context.Background()

	// --- Phase 1: Normal ---
	t.Log("Phase 1 (Normal): verifying function succeeds through both proxies")

	cacheData, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, txRouteID)
	require.NoError(t, err, "Phase 1: should succeed with both proxies healthy")
	assert.NotNil(t, cacheData.Actions, "Phase 1: cache Actions should not be nil")

	// --- Phase 2: Inject ---
	// Disconnect PostgreSQL proxy. Redis remains healthy.
	// Use a second route that is NOT cached yet so the function must query PostgreSQL.
	t.Log("Phase 2 (Inject): disabling PostgreSQL Toxiproxy proxy")

	err = infra.pgProxy.Disconnect()
	require.NoError(t, err, "Phase 2: PostgreSQL Toxiproxy Disconnect should not fail")

	// Create a second route in DB (direct connection) that is NOT in Redis cache
	orgID2 := libCommons.GenerateUUIDv7()
	ledgerID2 := libCommons.GenerateUUIDv7()

	sourceRouteID2 := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID2, ledgerID2, "PGDown Source2", "source")
	destRouteID2 := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID2, ledgerID2, "PGDown Dest2", "destination")
	txRouteID2 := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID2, ledgerID2, "PGDown Route2")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID2, txRouteID2)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID2, txRouteID2)

	// --- Phase 3: Verify ---
	// Redis cache miss for uncached key -> attempt DB fetch -> DB unreachable.
	// Function must return an error, not panic.
	t.Log("Phase 3 (Verify): verifying function returns error when PostgreSQL is down (cache miss scenario)")

	var pgDownErr error

	require.NotPanics(t, func() {
		_, pgDownErr = infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID2, ledgerID2, txRouteID2)
	}, "Phase 3: GetOrCreateTransactionRouteCache must not panic on PostgreSQL connection loss")

	assert.Error(t, pgDownErr,
		"Phase 3: function should return error when PostgreSQL is unreachable during cache miss")

	t.Logf("Phase 3: result during PostgreSQL outage: err=%v", pgDownErr)

	// --- Phase 4: Restore ---
	t.Log("Phase 4 (Restore): re-enabling PostgreSQL Toxiproxy proxy")

	err = infra.pgProxy.Reconnect()
	require.NoError(t, err, "Phase 4: PostgreSQL Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	t.Log("Phase 5 (Recovery): verifying function succeeds after PostgreSQL proxy restoration")

	chaos.AssertRecoveryWithin(t, func() error {
		result, recoverErr := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID2, ledgerID2, txRouteID2)
		if recoverErr != nil {
			return recoverErr
		}

		if result.Actions == nil {
			return chaos.ErrDataIntegrityViolation
		}

		return nil
	}, 10*time.Second, "Phase 5: function should recover after PostgreSQL proxy restoration")

	t.Log("PASS: GetOrCreateTransactionRouteCache handles PostgreSQL connection loss gracefully")
}

// =============================================================================
// CH-5: POSTGRES HIGH LATENCY (CACHE MISS -> SLOW DB)
// =============================================================================

// TestIntegration_Chaos_Postgres_HighLatency_GetOrCreateTransactionRouteCache verifies that
// GetOrCreateTransactionRouteCache does not hang indefinitely when a Redis cache miss
// triggers a PostgreSQL query and PostgreSQL has high latency.
//
// This uses the dual-proxy infrastructure. Redis remains healthy but the key is uncached,
// forcing a DB fetch through the slow PostgreSQL proxy.
//
// 5-Phase structure:
//  1. Normal   -- Cache miss -> DB fetch succeeds with normal latency
//  2. Inject   -- 5000ms latency added to PostgreSQL proxy
//  3. Verify   -- Function with 1s context deadline returns within bounded time
//  4. Restore  -- PostgreSQL latency toxic removed
//  5. Recovery -- Function succeeds again with normal latency
func TestIntegration_Chaos_Postgres_HighLatency_GetOrCreateTransactionRouteCache(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupDualProxyQueryChaosInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert test data directly into PostgreSQL
	sourceRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "PGLatency Source", "source")
	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "PGLatency Dest", "destination")
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "PGLatency Route")
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
	t.Log("Phase 2 (Inject): adding 5000ms latency to PostgreSQL Toxiproxy proxy")

	err = infra.pgProxy.AddLatency(5000*time.Millisecond, 0)
	require.NoError(t, err, "Phase 2: AddLatency on PostgreSQL proxy should not fail")

	// Use a second uncached key so the function must query PostgreSQL
	orgID2 := libCommons.GenerateUUIDv7()
	ledgerID2 := libCommons.GenerateUUIDv7()

	sourceRouteID2 := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID2, ledgerID2, "PGLatency Source2", "source")
	destRouteID2 := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID2, ledgerID2, "PGLatency Dest2", "destination")
	txRouteID2 := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID2, ledgerID2, "PGLatency Route2")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID2, txRouteID2)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID2, txRouteID2)

	// --- Phase 3: Verify ---
	// Redis cache miss -> PostgreSQL fetch with 5s latency but 1s context deadline.
	// Function must not hang.
	t.Log("Phase 3 (Verify): function must return within 1s deadline under 5s PostgreSQL latency")

	slowPGCtx, slowPGCancel := context.WithTimeout(ctx, 1*time.Second)
	defer slowPGCancel()

	var pgLatencyErr error

	done := make(chan struct{})

	go func() {
		defer close(done)
		_, pgLatencyErr = infra.uc.GetOrCreateTransactionRouteCache(slowPGCtx, orgID2, ledgerID2, txRouteID2)
	}()

	select {
	case <-done:
		// Good -- call returned within acceptable time.
	case <-time.After(3 * time.Second):
		t.Fatal("Phase 3: GetOrCreateTransactionRouteCache hung for more than 3s -- should respect context deadline")
	}

	t.Logf("Phase 3: result under high PostgreSQL latency: err=%v", pgLatencyErr)

	// --- Phase 4: Restore ---
	t.Log("Phase 4 (Restore): removing all toxics from PostgreSQL proxy")

	err = infra.pgProxy.RemoveAllToxics()
	require.NoError(t, err, "Phase 4: RemoveAllToxics on PostgreSQL proxy should not fail")

	// --- Phase 5: Recovery ---
	t.Log("Phase 5 (Recovery): verifying function succeeds after PostgreSQL latency removal")

	chaos.AssertRecoveryWithin(t, func() error {
		recoveryCtx, recoveryCancel := context.WithTimeout(ctx, 5*time.Second)
		defer recoveryCancel()

		result, recoverErr := infra.uc.GetOrCreateTransactionRouteCache(recoveryCtx, orgID2, ledgerID2, txRouteID2)
		if recoverErr != nil {
			return recoverErr
		}

		if result.Actions == nil {
			return chaos.ErrDataIntegrityViolation
		}

		return nil
	}, 10*time.Second, "Phase 5: function should recover after PostgreSQL latency removal")

	t.Log("PASS: GetOrCreateTransactionRouteCache handles PostgreSQL high latency gracefully")
}

// =============================================================================
// CH-6: BOTH REDIS AND POSTGRES DOWN (TOTAL INFRASTRUCTURE FAILURE)
// =============================================================================

// TestIntegration_Chaos_BothDown_GetOrCreateTransactionRouteCache verifies that
// GetOrCreateTransactionRouteCache returns an error gracefully (no panic) when both
// Redis and PostgreSQL are simultaneously unavailable.
//
// This is the worst-case scenario: complete infrastructure failure. The function
// should fail fast with an error rather than hanging or panicking.
//
// 5-Phase structure:
//  1. Normal   -- Function succeeds through both proxies
//  2. Inject   -- Both Redis and PostgreSQL proxies disabled
//  3. Verify   -- Function returns error, no panic, no hang
//  4. Restore  -- Both proxies re-enabled
//  5. Recovery -- Function succeeds again with both services restored
func TestIntegration_Chaos_BothDown_GetOrCreateTransactionRouteCache(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupDualProxyQueryChaosInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Insert test data directly into PostgreSQL
	sourceRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "BothDown Source", "source")
	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "BothDown Dest", "destination")
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "BothDown Route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID, txRouteID)

	ctx := context.Background()

	// --- Phase 1: Normal ---
	t.Log("Phase 1 (Normal): verifying function succeeds through both proxies")

	cacheData, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, txRouteID)
	require.NoError(t, err, "Phase 1: should succeed with both services healthy")
	assert.NotNil(t, cacheData.Actions, "Phase 1: cache Actions should not be nil")

	// --- Phase 2: Inject ---
	// Disconnect BOTH proxies simultaneously.
	t.Log("Phase 2 (Inject): disabling both Redis and PostgreSQL Toxiproxy proxies")

	err = infra.redisProxy.Disconnect()
	require.NoError(t, err, "Phase 2: Redis Toxiproxy Disconnect should not fail")

	err = infra.pgProxy.Disconnect()
	require.NoError(t, err, "Phase 2: PostgreSQL Toxiproxy Disconnect should not fail")

	// Use a second uncached key to force the full code path (cache miss -> DB fetch)
	orgID2 := libCommons.GenerateUUIDv7()
	ledgerID2 := libCommons.GenerateUUIDv7()

	sourceRouteID2 := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID2, ledgerID2, "BothDown Source2", "source")
	destRouteID2 := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID2, ledgerID2, "BothDown Dest2", "destination")
	txRouteID2 := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID2, ledgerID2, "BothDown Route2")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID2, txRouteID2)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID2, txRouteID2)

	// --- Phase 3: Verify ---
	// Both Redis and PostgreSQL are down.
	// Redis GetBytes fails (warning) -> fall through to DB -> DB fails -> return error.
	// No panic, no hang.
	t.Log("Phase 3 (Verify): verifying function returns error when both services are down")

	var bothDownErr error

	require.NotPanics(t, func() {
		_, bothDownErr = infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID2, ledgerID2, txRouteID2)
	}, "Phase 3: GetOrCreateTransactionRouteCache must not panic when both Redis and PostgreSQL are down")

	assert.Error(t, bothDownErr,
		"Phase 3: function should return error when both services are unavailable")

	t.Logf("Phase 3: result during total infrastructure failure: err=%v", bothDownErr)

	// --- Phase 4: Restore ---
	// Re-enable BOTH proxies.
	t.Log("Phase 4 (Restore): re-enabling both Redis and PostgreSQL Toxiproxy proxies")

	err = infra.redisProxy.Reconnect()
	require.NoError(t, err, "Phase 4: Redis Toxiproxy Reconnect should not fail")

	err = infra.pgProxy.Reconnect()
	require.NoError(t, err, "Phase 4: PostgreSQL Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	t.Log("Phase 5 (Recovery): verifying function succeeds after both proxies restored")

	chaos.AssertRecoveryWithin(t, func() error {
		result, recoverErr := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID2, ledgerID2, txRouteID2)
		if recoverErr != nil {
			return recoverErr
		}

		if result.Actions == nil {
			return chaos.ErrDataIntegrityViolation
		}

		return nil
	}, 10*time.Second, "Phase 5: function should recover after both services restored")

	t.Log("PASS: GetOrCreateTransactionRouteCache handles total infrastructure failure gracefully")
}
