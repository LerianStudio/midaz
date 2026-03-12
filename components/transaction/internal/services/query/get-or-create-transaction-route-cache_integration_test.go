//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"bytes"
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TEST INFRASTRUCTURE
// =============================================================================

// cacheQueryTestInfra holds both Redis and PostgreSQL infrastructure for cache query tests.
type cacheQueryTestInfra struct {
	pgContainer    *pgtestutil.ContainerResult
	redisContainer *redistestutil.ContainerResult
	uc             *UseCase
}

// setupCacheQueryTestInfra sets up Redis + PostgreSQL containers and creates a UseCase.
func setupCacheQueryTestInfra(t *testing.T) *cacheQueryTestInfra {
	t.Helper()

	// Setup PostgreSQL
	pgContainer := pgtestutil.SetupContainer(t)

	logger := libZap.InitializeLogger()
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	connStr := pgtestutil.BuildConnectionString(pgContainer.Host, pgContainer.Port, pgContainer.Config)

	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           pgContainer.Config.DBName,
		ReplicaDBName:           pgContainer.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	txRouteRepo := transactionroute.NewTransactionRoutePostgreSQLRepository(conn)

	// Setup Redis
	redisContainer := redistestutil.SetupContainer(t)
	redisConn := redistestutil.CreateConnection(t, redisContainer.Addr)

	redisRepo, err := redis.NewConsumerRedis(redisConn, false)
	require.NoError(t, err, "failed to create Redis repository")

	uc := &UseCase{
		TransactionRouteRepo: txRouteRepo,
		RedisRepo:            redisRepo,
	}

	return &cacheQueryTestInfra{
		pgContainer:    pgContainer,
		redisContainer: redisContainer,
		uc:             uc,
	}
}

// =============================================================================
// IS-1: Sentinel stored after DB miss
// When a transaction route does not exist in the database, the function must
// store a NOT_FOUND sentinel value in Redis with 60s TTL.
// =============================================================================

func TestIntegration_GetOrCreateTransactionRouteCache_SentinelStoredAfterDBMiss(t *testing.T) {
	infra := setupCacheQueryTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()
	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, nonExistentID)

	// Verify no cache exists before the call
	_, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	assert.ErrorIs(t, err, goredis.Nil, "key should not exist before first call")

	// Act - call for non-existent route should hit DB, get not-found, store sentinel
	_, err = infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, nonExistentID)

	// Assert - function returns ErrDatabaseItemNotFound
	require.Error(t, err, "should return error for non-existent route")
	assert.Equal(t, services.ErrDatabaseItemNotFound, err, "error should be ErrDatabaseItemNotFound")

	// Assert - sentinel value is now stored in Redis
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "sentinel should be stored in Redis after DB miss")
	assert.True(t, bytes.Equal(cachedBytes, []byte("NOT_FOUND")), "cached value should be NOT_FOUND sentinel bytes")

	// Assert - sentinel has a TTL (verify it expires, meaning TTL was set)
	ttl, err := infra.redisContainer.Client.TTL(ctx, internalKey).Result()
	require.NoError(t, err, "TTL query should not error")
	assert.Greater(t, ttl, time.Duration(0), "sentinel should have a positive TTL")
	assert.LessOrEqual(t, ttl, 60*time.Second, "sentinel TTL should be at most 60 seconds")
}

func TestIntegration_GetOrCreateTransactionRouteCache_SentinelStoredForDifferentNonExistentRoutes(t *testing.T) {
	infra := setupCacheQueryTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	nonExistent1 := libCommons.GenerateUUIDv7()
	nonExistent2 := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act - two different non-existent routes
	_, err1 := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, nonExistent1)
	_, err2 := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, nonExistent2)

	// Assert - both return not-found error
	require.Equal(t, services.ErrDatabaseItemNotFound, err1, "first call should return ErrDatabaseItemNotFound")
	require.Equal(t, services.ErrDatabaseItemNotFound, err2, "second call should return ErrDatabaseItemNotFound")

	// Assert - both have independent sentinel entries
	key1 := utils.AccountingRoutesInternalKey(orgID, ledgerID, nonExistent1)
	key2 := utils.AccountingRoutesInternalKey(orgID, ledgerID, nonExistent2)

	bytes1, err := infra.uc.RedisRepo.GetBytes(ctx, key1)
	require.NoError(t, err, "first sentinel should exist")
	assert.True(t, bytes.Equal(bytes1, []byte("NOT_FOUND")), "first key should have sentinel")

	bytes2, err := infra.uc.RedisRepo.GetBytes(ctx, key2)
	require.NoError(t, err, "second sentinel should exist")
	assert.True(t, bytes.Equal(bytes2, []byte("NOT_FOUND")), "second key should have sentinel")
}

// =============================================================================
// IS-2: Sentinel hit prevents DB call
// When Redis contains the NOT_FOUND sentinel, the function returns
// ErrDatabaseItemNotFound immediately without querying the database.
// =============================================================================

func TestIntegration_GetOrCreateTransactionRouteCache_SentinelHitReturnsCachedNotFound(t *testing.T) {
	infra := setupCacheQueryTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// First call stores sentinel in Redis
	_, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, nonExistentID)
	require.Equal(t, services.ErrDatabaseItemNotFound, err, "first call should return ErrDatabaseItemNotFound")

	// Act - second call should hit sentinel in Redis, return immediately
	result, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, nonExistentID)

	// Assert - same error returned from sentinel cache hit
	require.Error(t, err, "second call should also return error")
	assert.Equal(t, services.ErrDatabaseItemNotFound, err, "error should be ErrDatabaseItemNotFound from sentinel")
	assert.Equal(t, mmodel.TransactionRouteCache{}, result, "result should be zero-value cache struct")
}

func TestIntegration_GetOrCreateTransactionRouteCache_ManualSentinelInRedisReturnsCachedNotFound(t *testing.T) {
	infra := setupCacheQueryTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	arbitraryID := libCommons.GenerateUUIDv7()

	ctx := context.Background()
	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, arbitraryID)

	// Manually write sentinel to Redis (simulating prior DB miss without calling the function)
	err := infra.uc.RedisRepo.SetBytes(ctx, internalKey, []byte("NOT_FOUND"), time.Duration(60))
	require.NoError(t, err, "manual sentinel write should succeed")

	// Act - function should detect sentinel and return not-found without DB call
	result, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, arbitraryID)

	// Assert
	require.Error(t, err, "should return error when sentinel exists in Redis")
	assert.Equal(t, services.ErrDatabaseItemNotFound, err, "error should be ErrDatabaseItemNotFound")
	assert.Equal(t, mmodel.TransactionRouteCache{}, result, "result should be zero-value")
}

func TestIntegration_GetOrCreateTransactionRouteCache_MultipleSentinelHitsConsistent(t *testing.T) {
	infra := setupCacheQueryTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// First call stores sentinel
	_, _ = infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, nonExistentID)

	// Act - call multiple times; all should consistently return sentinel-based not-found
	for i := 0; i < 5; i++ {
		result, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, nonExistentID)
		assert.Equal(t, services.ErrDatabaseItemNotFound, err, "call %d should return ErrDatabaseItemNotFound", i+1)
		assert.Equal(t, mmodel.TransactionRouteCache{}, result, "call %d should return zero-value result", i+1)
	}
}

// =============================================================================
// IS-3: Sentinel TTL expiry
// After the sentinel TTL expires (60s), the next call should hit DB again.
// NOTE: We cannot wait 60s in a test, so we verify TTL behavior by setting a
// short-TTL sentinel manually and then checking behavior after expiry.
// =============================================================================

func TestIntegration_GetOrCreateTransactionRouteCache_SentinelExpiryTriggersDBLookup(t *testing.T) {
	infra := setupCacheQueryTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()
	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, nonExistentID)

	// Manually set sentinel with a 1-second TTL to simulate near-expiry
	err := infra.uc.RedisRepo.SetBytes(ctx, internalKey, []byte("NOT_FOUND"), time.Duration(1))
	require.NoError(t, err, "manual sentinel write should succeed")

	// Verify sentinel exists
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "sentinel should exist initially")
	assert.True(t, bytes.Equal(cachedBytes, []byte("NOT_FOUND")), "should be sentinel value")

	// Wait for TTL to expire
	time.Sleep(2 * time.Second)

	// Act - after expiry, key should be gone, function hits DB again
	_, err = infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, nonExistentID)

	// Assert - route still doesn't exist in DB, so function stores a fresh sentinel
	require.Equal(t, services.ErrDatabaseItemNotFound, err, "should return ErrDatabaseItemNotFound after sentinel expiry")

	// Verify a new sentinel was stored (with the real 60s TTL this time)
	cachedBytes, err = infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "fresh sentinel should be stored after re-lookup")
	assert.True(t, bytes.Equal(cachedBytes, []byte("NOT_FOUND")), "new sentinel should be NOT_FOUND")

	ttl, err := infra.redisContainer.Client.TTL(ctx, internalKey).Result()
	require.NoError(t, err, "TTL query should not error")
	assert.Greater(t, ttl, 50*time.Second, "fresh sentinel should have TTL close to 60s")
}

// =============================================================================
// IS-4: Valid cache hit
// When a route exists in DB, the first call caches it in Redis; the second
// call returns from cache. Both calls return consistent data.
// =============================================================================

func TestIntegration_GetOrCreateTransactionRouteCache_FreshCacheReturned(t *testing.T) {
	infra := setupCacheQueryTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create operation routes in DB
	sourceRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Source Route", "source")
	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Dest Route", "destination")

	// Create transaction route in DB
	txRouteParams := pgtestutil.TransactionRouteParams{
		Title:       "Cached Route",
		Description: "Route to be cached",
	}
	txRouteID := pgtestutil.CreateTestTransactionRoute(t, infra.pgContainer.DB, orgID, ledgerID, txRouteParams)

	// Link operation routes
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID, txRouteID)

	ctx := context.Background()

	// Act - first call should fetch from DB and populate cache
	cacheData, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, txRouteID)

	// Assert
	require.NoError(t, err, "GetOrCreateTransactionRouteCache should not return error")
	assert.NotNil(t, cacheData.Actions, "fresh cache should have Actions populated")

	// Verify cache was stored in Redis
	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, txRouteID)
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "cache should exist in Redis after first call")
	assert.NotEmpty(t, cachedBytes, "cached bytes should not be empty")

	// Verify stored bytes are valid msgpack, not sentinel
	assert.False(t, bytes.Equal(cachedBytes, []byte("NOT_FOUND")), "cached bytes should not be sentinel value")
}

func TestIntegration_GetOrCreateTransactionRouteCache_HitReturnsCachedData(t *testing.T) {
	infra := setupCacheQueryTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create operation routes and transaction route in DB
	sourceRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Source", "source")
	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Dest", "destination")
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Cached Route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID, txRouteID)

	ctx := context.Background()

	// Populate cache with first call
	firstResult, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, txRouteID)
	require.NoError(t, err, "first call should not return error")

	// Act - second call should return from cache
	secondResult, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, txRouteID)

	// Assert
	require.NoError(t, err, "second call should not return error")
	assert.Equal(t, len(firstResult.Actions), len(secondResult.Actions), "cached result should match first result action count")
	assert.NotNil(t, secondResult.Actions, "cached result should have non-nil Actions")

	// Verify action keys are preserved across cache roundtrip
	for actionKey := range firstResult.Actions {
		assert.Contains(t, secondResult.Actions, actionKey, "second result should contain action key %q from first result", actionKey)
	}
}

func TestIntegration_GetOrCreateTransactionRouteCache_CacheMissPopulatesCache(t *testing.T) {
	infra := setupCacheQueryTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create DB data
	sourceRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Source", "source")
	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Dest", "destination")
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Miss Test Route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID, txRouteID)

	ctx := context.Background()

	// Verify no cache exists yet
	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, txRouteID)
	_, cacheErr := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	assert.ErrorIs(t, cacheErr, goredis.Nil, "key should not exist before first call")

	// Act
	result, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, txRouteID)

	// Assert
	require.NoError(t, err, "GetOrCreateTransactionRouteCache should not return error")
	assert.NotNil(t, result.Actions, "cache should have Actions populated")

	// Verify cache is now stored
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "cache should exist in Redis after miss")
	assert.NotEmpty(t, cachedBytes, "cached bytes should not be empty")

	// Verify stored value is valid msgpack (not sentinel)
	var decoded mmodel.TransactionRouteCache
	decodeErr := decoded.FromMsgpack(cachedBytes)
	assert.NoError(t, decodeErr, "cached bytes should be valid msgpack")
}

// =============================================================================
// IS-5: Cache invalidation flow
// After a sentinel is stored for a non-existent route, if the route is later
// created and its cache explicitly written, the sentinel is overwritten with
// valid msgpack data. Subsequent lookups return the valid cached route.
// =============================================================================

func TestIntegration_GetOrCreateTransactionRouteCache_SentinelOverwrittenByValidCache(t *testing.T) {
	infra := setupCacheQueryTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create route in DB first (we need a real route ID to test cache overwrite)
	sourceRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Source", "source")
	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Dest", "destination")
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Late Route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID, txRouteID)

	ctx := context.Background()
	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, txRouteID)

	// Step 1: Manually store sentinel (simulating a stale sentinel from before route existed)
	err := infra.uc.RedisRepo.SetBytes(ctx, internalKey, []byte("NOT_FOUND"), time.Duration(60))
	require.NoError(t, err, "sentinel write should succeed")

	// Verify sentinel is in place
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "sentinel should be readable")
	assert.True(t, bytes.Equal(cachedBytes, []byte("NOT_FOUND")), "should be sentinel value before overwrite")

	// Step 2: Overwrite sentinel by storing valid cache data via SetBytes
	// (this simulates what CreateAccountingRouteCache or a cache reload would do)
	validCache := mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			"direct": {
				Source:        map[string]mmodel.OperationRouteCache{sourceRouteID.String(): {OperationType: "source"}},
				Destination:   map[string]mmodel.OperationRouteCache{destRouteID.String(): {OperationType: "destination"}},
				Bidirectional: map[string]mmodel.OperationRouteCache{},
			},
		},
	}
	validBytes, err := validCache.ToMsgpack()
	require.NoError(t, err, "msgpack serialization should succeed")

	err = infra.uc.RedisRepo.SetBytes(ctx, internalKey, validBytes, 0)
	require.NoError(t, err, "overwriting sentinel with valid cache should succeed")

	// Act - function should now return valid cached data instead of sentinel
	result, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, txRouteID)

	// Assert
	require.NoError(t, err, "should not return error after sentinel is overwritten with valid cache")
	assert.NotNil(t, result.Actions, "result should have Actions populated")
	assert.Contains(t, result.Actions, "direct", "result should contain 'direct' action")
}

func TestIntegration_GetOrCreateTransactionRouteCache_SentinelOverwrittenByDBFetch(t *testing.T) {
	infra := setupCacheQueryTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create route in DB first
	sourceRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Source", "source")
	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Dest", "destination")
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Overwrite Test Route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID, txRouteID)

	ctx := context.Background()
	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, txRouteID)

	// Manually inject sentinel (simulating stale sentinel from before route was created)
	err := infra.uc.RedisRepo.SetBytes(ctx, internalKey, []byte("NOT_FOUND"), time.Duration(60))
	require.NoError(t, err, "sentinel injection should succeed")

	// Act - function finds sentinel and returns ErrDatabaseItemNotFound
	// (sentinel takes priority - this is the current behavior)
	_, err = infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, txRouteID)
	assert.Equal(t, services.ErrDatabaseItemNotFound, err, "sentinel in Redis should cause not-found even if DB has the route")

	// Delete the sentinel to simulate TTL expiry
	delErr := infra.redisContainer.Client.Del(ctx, internalKey).Err()
	require.NoError(t, delErr, "deleting sentinel should succeed")

	// Act - after sentinel removal, function should fetch from DB and cache valid data
	result, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, txRouteID)
	require.NoError(t, err, "should succeed after sentinel removal when route exists in DB")
	assert.NotNil(t, result.Actions, "result should have Actions populated from DB")

	// Verify Redis now contains valid msgpack, not sentinel
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "valid cache should exist in Redis")
	assert.False(t, bytes.Equal(cachedBytes, []byte("NOT_FOUND")), "Redis should contain valid cache, not sentinel")

	var decoded mmodel.TransactionRouteCache
	decodeErr := decoded.FromMsgpack(cachedBytes)
	assert.NoError(t, decodeErr, "cached bytes should be valid msgpack after DB fetch")
}

// =============================================================================
// Edge cases: Corrupted cache fallback to DB
// =============================================================================

func TestIntegration_GetOrCreateTransactionRouteCache_CorruptedCacheFallsBackToDB(t *testing.T) {
	infra := setupCacheQueryTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create DB data
	sourceRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Source", "source")
	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Dest", "destination")
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Corrupt Cache Route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID, txRouteID)

	ctx := context.Background()

	// Write corrupted bytes to Redis (not sentinel, not valid msgpack)
	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, txRouteID)
	corruptedBytes := []byte{0xFF, 0xFE, 0x00, 0x01, 0x02}
	err := infra.uc.RedisRepo.SetBytes(ctx, internalKey, corruptedBytes, 0)
	require.NoError(t, err, "SetBytes should not fail for corrupted data")

	// Act - corrupted data should trigger DB fallback per implementation (lines 56-62)
	result, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, txRouteID)

	// Assert - implementation falls back to DB on decode error and caches fresh data
	require.NoError(t, err, "should fall back to DB successfully on corrupted cache")
	assert.NotNil(t, result.Actions, "result from DB fallback should have Actions populated")

	// Verify Redis now contains valid cache (corrupted data was replaced)
	freshBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "fresh cache should exist in Redis")
	assert.False(t, bytes.Equal(freshBytes, corruptedBytes), "corrupted bytes should be replaced")

	var decoded mmodel.TransactionRouteCache
	decodeErr := decoded.FromMsgpack(freshBytes)
	assert.NoError(t, decodeErr, "refreshed cache should be valid msgpack")
}

func TestIntegration_GetOrCreateTransactionRouteCache_CorruptedCacheForNonExistentRoute(t *testing.T) {
	infra := setupCacheQueryTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Write corrupted bytes for a route that does not exist in DB
	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, nonExistentID)
	corruptedBytes := []byte{0xAB, 0xCD, 0xEF}
	err := infra.uc.RedisRepo.SetBytes(ctx, internalKey, corruptedBytes, 0)
	require.NoError(t, err, "SetBytes should succeed")

	// Act - corrupted data triggers DB fallback, DB returns not-found, sentinel stored
	_, err = infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, nonExistentID)

	// Assert
	require.Equal(t, services.ErrDatabaseItemNotFound, err, "should return ErrDatabaseItemNotFound after DB fallback")

	// Verify sentinel replaced corrupted data
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "sentinel should exist after fallback")
	assert.True(t, bytes.Equal(cachedBytes, []byte("NOT_FOUND")), "corrupted data should be replaced with sentinel")
}
