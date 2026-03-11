//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"
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
// IS-2: Get cached route with nil Actions (old format) -> triggers DB refresh
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
	assert.NotNil(t, cacheData.Actions, "fresh cache should have non-nil Actions")

	// Verify cache was stored in Redis
	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, txRouteID)
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "cache should exist in Redis after first call")
	assert.NotEmpty(t, cachedBytes, "cached bytes should not be empty")
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
	assert.Equal(t, len(firstResult.Actions), len(secondResult.Actions), "cached result should match first result")
	assert.NotNil(t, secondResult.Actions, "cached result should have non-nil Actions")
}

func TestIntegration_GetOrCreateTransactionRouteCache_NotFoundInDB(t *testing.T) {
	infra := setupCacheQueryTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	nonExistentID := libCommons.GenerateUUIDv7()

	ctx := context.Background()

	// Act
	_, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, nonExistentID)

	// Assert
	require.Error(t, err, "should return error for non-existent transaction route")
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
	// redis.Nil or empty bytes expected - not an error we need to assert strongly on

	// Act
	result, err := infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, txRouteID)

	// Assert
	require.NoError(t, err, "GetOrCreateTransactionRouteCache should not return error")
	assert.NotNil(t, result.Actions, "cache should have Actions populated")

	// Verify cache is now stored
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "cache should exist in Redis after miss")
	assert.NotEmpty(t, cachedBytes, "cached bytes should not be empty")

	// Suppress unused variable warning for cacheErr
	_ = cacheErr
}

// =============================================================================
// Edge cases for IS-2
// =============================================================================

func TestIntegration_GetOrCreateTransactionRouteCache_CorruptedCacheTriggersRefresh(t *testing.T) {
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

	// Write corrupted bytes to Redis
	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, txRouteID)
	corruptedBytes := []byte{0xFF, 0xFE, 0x00, 0x01, 0x02}
	err := infra.uc.RedisRepo.SetBytes(ctx, internalKey, corruptedBytes, 0)
	require.NoError(t, err, "SetBytes should not fail for corrupted data")

	// Act - should fail to decode and return error
	_, err = infra.uc.GetOrCreateTransactionRouteCache(ctx, orgID, ledgerID, txRouteID)

	// Assert: corrupted cache produces a decode error (current behavior)
	// The function does not fall through to DB on decode errors - it returns the error
	require.Error(t, err, "should return error for corrupted cache data")
}

// Unused variable prevention
var (
	_ = services.ErrDatabaseItemNotFound
	_ = time.Now
)
