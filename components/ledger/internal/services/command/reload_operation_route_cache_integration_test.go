// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v4/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v4/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transactionroute"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TEST INFRASTRUCTURE
// =============================================================================

// reloadCacheTestInfra holds PostgreSQL + Redis infrastructure for reload cache tests.
type reloadCacheTestInfra struct {
	pgContainer    *pgtestutil.ContainerResult
	redisContainer *redistestutil.ContainerResult
	uc             *UseCase
}

// setupReloadCacheTestInfra creates the test infrastructure with both PostgreSQL and Redis.
func setupReloadCacheTestInfra(t *testing.T) *reloadCacheTestInfra {
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
	opRouteRepo := operationroute.NewOperationRoutePostgreSQLRepository(conn)

	// Setup Redis
	redisContainer := redistestutil.SetupContainer(t)
	redisConn := redistestutil.CreateConnection(t, redisContainer.Addr)

	redisRepo, err := redis.NewConsumerRedis(redisConn, false)
	require.NoError(t, err, "failed to create Redis repository")

	uc := &UseCase{
		TransactionRouteRepo: txRouteRepo,
		OperationRouteRepo:   opRouteRepo,
		TransactionRedisRepo: redisRepo,
	}

	return &reloadCacheTestInfra{
		pgContainer:    pgContainer,
		redisContainer: redisContainer,
		uc:             uc,
	}
}

// =============================================================================
// IS-3: Reload operation route cache -> rebuilds with action grouping
// =============================================================================

func TestIntegration_ReloadOperationRouteCache_RebuildsSingleTransactionRoute(t *testing.T) {
	infra := setupReloadCacheTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create operation routes in DB
	sourceRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Source Route", "source")
	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Dest Route", "destination")

	// Create transaction route and links
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Reload Test Route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID, txRouteID)

	ctx := context.Background()

	// Act - reload cache for this operation route
	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, txRouteID)
	err := infra.uc.ReloadOperationRouteCache(ctx, orgID, ledgerID, sourceRouteID)

	// Assert
	require.NoError(t, err, "ReloadOperationRouteCache should not return error")

	// Verify cache was rebuilt
	freshBytes, err := infra.uc.TransactionRedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "GetBytes should not fail after reload")

	var freshCache mmodel.TransactionRouteCache
	err = freshCache.FromMsgpack(freshBytes)
	require.NoError(t, err, "FromMsgpack should not fail after reload")

	assert.NotNil(t, freshCache.Actions, "Actions should be populated after reload")
}

func TestIntegration_ReloadOperationRouteCache_NoTransactionRoutes(t *testing.T) {
	infra := setupReloadCacheTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create an operation route with no transaction route links
	opRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Unlinked Route", "source")

	ctx := context.Background()

	// Act
	err := infra.uc.ReloadOperationRouteCache(ctx, orgID, ledgerID, opRouteID)

	// Assert - should succeed without error (no-op)
	require.NoError(t, err, "ReloadOperationRouteCache should not return error for unlinked operation route")
}

func TestIntegration_ReloadOperationRouteCache_MultipleTransactionRoutes(t *testing.T) {
	infra := setupReloadCacheTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create a shared operation route
	sharedSourceID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Shared Source", "source")

	// Create two destination routes (one per transaction route)
	destID1 := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Dest 1", "destination")
	destID2 := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Dest 2", "destination")

	// Create two transaction routes, both linked to the shared source
	txRouteID1 := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "TX Route 1")
	txRouteID2 := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "TX Route 2")

	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sharedSourceID, txRouteID1)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destID1, txRouteID1)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sharedSourceID, txRouteID2)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destID2, txRouteID2)

	ctx := context.Background()

	// Act - reload for the shared operation route should rebuild both transaction route caches
	err := infra.uc.ReloadOperationRouteCache(ctx, orgID, ledgerID, sharedSourceID)

	// Assert
	require.NoError(t, err, "ReloadOperationRouteCache should not return error")

	// Verify both transaction route caches exist
	key1 := utils.AccountingRoutesInternalKey(orgID, ledgerID, txRouteID1)
	bytes1, err := infra.uc.TransactionRedisRepo.GetBytes(ctx, key1)
	require.NoError(t, err, "cache for txRoute1 should exist")
	assert.NotEmpty(t, bytes1, "cache for txRoute1 should not be empty")

	var cache1 mmodel.TransactionRouteCache
	err = cache1.FromMsgpack(bytes1)
	require.NoError(t, err)
	assert.NotNil(t, cache1.Actions, "txRoute1 cache should have Actions populated")

	key2 := utils.AccountingRoutesInternalKey(orgID, ledgerID, txRouteID2)
	bytes2, err := infra.uc.TransactionRedisRepo.GetBytes(ctx, key2)
	require.NoError(t, err, "cache for txRoute2 should exist")
	assert.NotEmpty(t, bytes2, "cache for txRoute2 should not be empty")

	var cache2 mmodel.TransactionRouteCache
	err = cache2.FromMsgpack(bytes2)
	require.NoError(t, err)
	assert.NotNil(t, cache2.Actions, "txRoute2 cache should have Actions populated")
}

func TestIntegration_ReloadOperationRouteCache_ActionGroupingVerified(t *testing.T) {
	infra := setupReloadCacheTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create operation routes with account rules
	ruleType := "alias"
	validIf := "@cash_account"
	sourceParams := pgtestutil.OperationRouteParams{
		Title:              "Source With Rule",
		Description:        "Has account rule",
		OperationType:      "source",
		AccountRuleType:    &ruleType,
		AccountRuleValidIf: &validIf,
	}
	sourceRouteID := pgtestutil.CreateTestOperationRoute(t, infra.pgContainer.DB, orgID, ledgerID, sourceParams)

	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Dest Route", "destination")

	// Create transaction route with links
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Action Grouping Route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID, txRouteID)

	ctx := context.Background()

	// Act
	err := infra.uc.ReloadOperationRouteCache(ctx, orgID, ledgerID, sourceRouteID)

	// Assert
	require.NoError(t, err, "ReloadOperationRouteCache should not return error")

	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, txRouteID)
	cachedBytes, err := infra.uc.TransactionRedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "GetBytes should not fail")

	var cacheData mmodel.TransactionRouteCache
	err = cacheData.FromMsgpack(cachedBytes)
	require.NoError(t, err, "FromMsgpack should not fail")

	// Verify action grouping
	assert.NotNil(t, cacheData.Actions, "Actions should be populated")
}

func TestIntegration_ReloadOperationRouteCache_ReplacesExistingCache(t *testing.T) {
	infra := setupReloadCacheTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	// Create DB data
	sourceRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Source", "source")
	destRouteID := pgtestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Dest", "destination")
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, infra.pgContainer.DB, orgID, ledgerID, "Replace Cache Route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, sourceRouteID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, infra.pgContainer.DB, destRouteID, txRouteID)

	ctx := context.Background()

	// Write an outdated cache with a fake route ID
	fakeRouteID := libCommons.GenerateUUIDv7()
	outdatedCache := mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{"direct": {Source: map[string]mmodel.OperationRouteCache{fakeRouteID.String(): {OperationType: "source"}}}},
	}
	outdatedBytes, err := outdatedCache.ToMsgpack()
	require.NoError(t, err)

	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, txRouteID)
	err = infra.uc.TransactionRedisRepo.SetBytes(ctx, internalKey, outdatedBytes, 0)
	require.NoError(t, err)

	// Act
	err = infra.uc.ReloadOperationRouteCache(ctx, orgID, ledgerID, sourceRouteID)

	// Assert
	require.NoError(t, err, "ReloadOperationRouteCache should not return error")

	freshBytes, err := infra.uc.TransactionRedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err)

	var freshCache mmodel.TransactionRouteCache
	err = freshCache.FromMsgpack(freshBytes)
	require.NoError(t, err)

	// The fake route ID should no longer be present in any action
	assert.NotNil(t, freshCache.Actions, "Actions should be populated after reload")
	for _, actionCache := range freshCache.Actions {
		assert.NotContains(t, actionCache.Source, fakeRouteID.String(), "outdated fake route should be gone")
	}
}
