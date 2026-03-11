//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package command provides chaos tests for the ReloadOperationRouteCache use case.
//
// These tests exercise Redis fault tolerance during cache reload operations.
// ReloadOperationRouteCache reads from PostgreSQL and writes to Redis.
// When Redis is unavailable, individual cache write failures should be logged
// as warnings but the function should continue processing other routes
// (graceful degradation via continue pattern).
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

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	libRedis "github.com/LerianStudio/lib-commons/v3/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// CHAOS TEST INFRASTRUCTURE (RELOAD CACHE)
// =============================================================================

// chaosReloadTestInfra holds all resources for a reload cache chaos test scenario.
// It wraps chaos.Infrastructure (Toxiproxy + Docker network), a PostgreSQL container
// (direct, not proxied), and a Redis consumer repository connected through Toxiproxy.
type chaosReloadTestInfra struct {
	pgContainer    *pgtestutil.ContainerResult
	redisContainer *redistestutil.ContainerResult
	chaosInfra     *chaos.Infrastructure
	uc             *UseCase
	proxy          *chaos.Proxy
}

// setupReloadChaosInfra creates the full chaos test infrastructure for reload cache tests:
//  1. Creates a Docker network + Toxiproxy container via chaos.Infrastructure
//  2. Starts a PostgreSQL container (direct, not proxied)
//  3. Starts a Valkey container on the host network
//  4. Registers the Redis container with Infrastructure to get UpstreamAddr
//  5. Creates a Toxiproxy proxy that forwards through to Redis
//  6. Builds a UseCase with TransactionRouteRepo + OperationRouteRepo (direct PG)
//     and RedisRepo (through proxy)
//
// All cleanup is registered with t.Cleanup() automatically.
func setupReloadChaosInfra(t *testing.T) *chaosReloadTestInfra {
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
	opRouteRepo := operationroute.NewOperationRoutePostgreSQLRepository(pgConn)

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
		OperationRouteRepo:   opRouteRepo,
		RedisRepo:            redisRepo,
	}

	return &chaosReloadTestInfra{
		pgContainer:    pgContainer,
		redisContainer: redisContainer,
		chaosInfra:     chaosInfra,
		uc:             uc,
		proxy:          proxy,
	}
}

// =============================================================================
// REDIS CONNECTION LOSS DURING ReloadOperationRouteCache
// =============================================================================

// TestIntegration_Chaos_Redis_ConnectionLoss_ReloadOperationRouteCache verifies that
// ReloadOperationRouteCache handles Redis connection loss gracefully.
// The function reads from PostgreSQL (which remains available) and writes to Redis.
// When Redis is down, the cache write calls (CreateAccountingRouteCache) should fail
// but the function should continue processing remaining routes (continue pattern)
// and return nil (no error propagated for individual cache write failures).
//
// 5-Phase structure:
//  1. Normal   -- ReloadOperationRouteCache succeeds, cache is populated
//  2. Inject   -- Toxiproxy proxy is disabled (Redis connection loss)
//  3. Verify   -- ReloadOperationRouteCache completes without panic; cache writes fail silently
//  4. Restore  -- Toxiproxy proxy is re-enabled
//  5. Recovery -- ReloadOperationRouteCache succeeds, cache is repopulated
func TestIntegration_Chaos_Redis_ConnectionLoss_ReloadOperationRouteCache(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupReloadChaosInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create DB data: operation routes + transaction route + links
	sourceRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Chaos Source", "source")
	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Chaos Dest", "destination")
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Chaos Reload Route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID, txRouteID)

	ctx := context.Background()
	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, txRouteID)

	// --- Phase 1: Normal ---
	t.Log("Phase 1 (Normal): verifying ReloadOperationRouteCache succeeds through proxy")

	err := infra.uc.ReloadOperationRouteCache(ctx, orgID, ledgerID, sourceRouteID)
	require.NoError(t, err, "Phase 1: ReloadOperationRouteCache should succeed through proxy")

	// Verify cache was stored
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "Phase 1: cache should be readable after reload")
	assert.NotEmpty(t, cachedBytes, "Phase 1: cached bytes should not be empty")

	var cacheData mmodel.TransactionRouteCache
	err = cacheData.FromMsgpack(cachedBytes)
	require.NoError(t, err, "Phase 1: cache should deserialize correctly")
	assert.NotNil(t, cacheData.Actions, "Phase 1: cache should have Actions populated")

	// --- Phase 2: Inject ---
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate Redis connection loss")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	// ReloadOperationRouteCache reads from PG (direct, not affected) and writes to Redis (proxied).
	// With Redis down, CreateAccountingRouteCache will fail, but ReloadOperationRouteCache
	// uses a `continue` pattern -- it logs warnings and proceeds to the next route.
	// The function should return nil (no error) because it treats cache write failures
	// as non-fatal.
	t.Log("Phase 3 (Verify): ReloadOperationRouteCache should handle Redis outage gracefully")

	var chaosErr error

	require.NotPanics(t, func() {
		chaosErr = infra.uc.ReloadOperationRouteCache(ctx, orgID, ledgerID, sourceRouteID)
	}, "Phase 3: ReloadOperationRouteCache must not panic on Redis connection loss")

	// The function uses `continue` on cache write failures, so it returns nil.
	assert.NoError(t, chaosErr,
		"Phase 3: ReloadOperationRouteCache should return nil (cache write failures are non-fatal)")

	t.Logf("Phase 3: result during Redis outage: err=%v", chaosErr)

	// --- Phase 4: Restore ---
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	t.Log("Phase 5 (Recovery): verifying ReloadOperationRouteCache repopulates cache after proxy restoration")

	chaos.AssertRecoveryWithin(t, func() error {
		reloadErr := infra.uc.ReloadOperationRouteCache(ctx, orgID, ledgerID, sourceRouteID)
		if reloadErr != nil {
			return reloadErr
		}

		// Verify cache was actually repopulated
		bytes, getErr := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
		if getErr != nil {
			return getErr
		}

		if len(bytes) == 0 {
			return chaos.ErrDataIntegrityViolation
		}

		var cache mmodel.TransactionRouteCache
		if decErr := cache.FromMsgpack(bytes); decErr != nil {
			return decErr
		}

		if cache.Actions == nil {
			return chaos.ErrDataIntegrityViolation
		}

		return nil
	}, 10*time.Second, "Phase 5: ReloadOperationRouteCache should recover and repopulate cache")

	t.Log("PASS: ReloadOperationRouteCache handles Redis connection loss gracefully via continue pattern")
}
