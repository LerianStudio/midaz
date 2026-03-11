//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TEST INFRASTRUCTURE
// =============================================================================

// cacheTestInfra holds the infrastructure needed for cache integration tests.
type cacheTestInfra struct {
	redisContainer *redistestutil.ContainerResult
	uc             *UseCase
}

// setupCacheTestInfra sets up Redis container and creates a UseCase with a real Redis repo.
func setupCacheTestInfra(t *testing.T) *cacheTestInfra {
	t.Helper()

	container := redistestutil.SetupContainer(t)
	conn := redistestutil.CreateConnection(t, container.Addr)

	redisRepo, err := redis.NewConsumerRedis(conn, false)
	require.NoError(t, err, "failed to create Redis repository")

	uc := &UseCase{
		RedisRepo: redisRepo,
	}

	return &cacheTestInfra{
		redisContainer: container,
		uc:             uc,
	}
}

// =============================================================================
// IS-1: Create transaction route -> verify action-aware cache stored in Redis
// =============================================================================

func TestIntegration_CreateAccountingRouteCache_ActionAwareCacheStored(t *testing.T) {
	infra := setupCacheTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()

	sourceRouteID := libCommons.GenerateUUIDv7()
	destRouteID := libCommons.GenerateUUIDv7()

	route := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Title:          "Settlement Route",
		Description:    "Route with action-aware cache",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:            sourceRouteID,
				OperationType: "source",
				Action:        "direct",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@cash_source",
				},
			},
			{
				ID:            destRouteID,
				OperationType: "destination",
				Action:        "direct",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@cash_destination",
				},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ctx := context.Background()

	// Act
	err := infra.uc.CreateAccountingRouteCache(ctx, route)

	// Assert
	require.NoError(t, err, "CreateAccountingRouteCache should not return error")

	// Verify cache was stored by reading it back
	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, routeID)
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "GetBytes should not return error for stored cache")
	assert.NotEmpty(t, cachedBytes, "cached bytes should not be empty")

	// Deserialize and verify structure
	var cacheData mmodel.TransactionRouteCache
	err = cacheData.FromMsgpack(cachedBytes)
	require.NoError(t, err, "FromMsgpack should not return error")

	// Verify action-aware grouping (the key assertion for IS-1)
	assert.NotNil(t, cacheData.Actions, "Actions map should not be nil")
	assert.Contains(t, cacheData.Actions, "direct", "Actions should contain 'direct' key")

	directAction := cacheData.Actions["direct"]
	assert.Len(t, directAction.Source, 1, "direct action should have 1 source")
	assert.Len(t, directAction.Destination, 1, "direct action should have 1 destination")
	assert.Contains(t, directAction.Source, sourceRouteID.String(), "direct action source should contain source route ID")
	assert.Contains(t, directAction.Destination, destRouteID.String(), "direct action destination should contain dest route ID")
}

func TestIntegration_CreateAccountingRouteCache_MultipleActions(t *testing.T) {
	infra := setupCacheTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()

	directSourceID := libCommons.GenerateUUIDv7()
	directDestID := libCommons.GenerateUUIDv7()
	holdSourceID := libCommons.GenerateUUIDv7()
	holdDestID := libCommons.GenerateUUIDv7()

	route := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Title:          "Multi-Action Route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:            directSourceID,
				OperationType: "source",
				Action:        "direct",
			},
			{
				ID:            directDestID,
				OperationType: "destination",
				Action:        "direct",
			},
			{
				ID:            holdSourceID,
				OperationType: "source",
				Action:        "hold",
			},
			{
				ID:            holdDestID,
				OperationType: "destination",
				Action:        "hold",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ctx := context.Background()

	// Act
	err := infra.uc.CreateAccountingRouteCache(ctx, route)

	// Assert
	require.NoError(t, err, "CreateAccountingRouteCache should not return error")

	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, routeID)
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "GetBytes should not return error")

	var cacheData mmodel.TransactionRouteCache
	err = cacheData.FromMsgpack(cachedBytes)
	require.NoError(t, err, "FromMsgpack should not return error")

	// Verify both actions exist
	assert.Len(t, cacheData.Actions, 2, "should have 2 action groups")
	assert.Contains(t, cacheData.Actions, "direct", "should contain 'direct' action")
	assert.Contains(t, cacheData.Actions, "hold", "should contain 'hold' action")

	// Verify routes grouped correctly per action
	assert.Contains(t, cacheData.Actions["direct"].Source, directSourceID.String())
	assert.Contains(t, cacheData.Actions["direct"].Destination, directDestID.String())
	assert.Contains(t, cacheData.Actions["hold"].Source, holdSourceID.String())
	assert.Contains(t, cacheData.Actions["hold"].Destination, holdDestID.String())
}

func TestIntegration_CreateAccountingRouteCache_BidirectionalRoute(t *testing.T) {
	infra := setupCacheTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()
	bidiRouteID := libCommons.GenerateUUIDv7()

	route := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Title:          "Bidirectional Route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:            bidiRouteID,
				OperationType: "bidirectional",
				Action:        "direct",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ctx := context.Background()

	// Act
	err := infra.uc.CreateAccountingRouteCache(ctx, route)

	// Assert
	require.NoError(t, err, "CreateAccountingRouteCache should not return error")

	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, routeID)
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "GetBytes should not return error")

	var cacheData mmodel.TransactionRouteCache
	err = cacheData.FromMsgpack(cachedBytes)
	require.NoError(t, err, "FromMsgpack should not return error")

	// Verify bidirectional in action grouping
	directAction := cacheData.Actions["direct"]
	assert.Len(t, directAction.Bidirectional, 1, "direct action should have 1 bidirectional")
	assert.Contains(t, directAction.Bidirectional, bidiRouteID.String())
}

func TestIntegration_CreateAccountingRouteCache_EmptyOperationRoutes(t *testing.T) {
	infra := setupCacheTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()

	route := &mmodel.TransactionRoute{
		ID:              routeID,
		OrganizationID:  orgID,
		LedgerID:        ledgerID,
		Title:           "Empty Route",
		OperationRoutes: []mmodel.OperationRoute{},
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	ctx := context.Background()

	// Act
	err := infra.uc.CreateAccountingRouteCache(ctx, route)

	// Assert
	require.NoError(t, err, "CreateAccountingRouteCache should not return error with empty routes")

	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, routeID)
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "GetBytes should not return error")

	var cacheData mmodel.TransactionRouteCache
	err = cacheData.FromMsgpack(cachedBytes)
	require.NoError(t, err, "FromMsgpack should not return error")

	assert.NotNil(t, cacheData.Actions, "Actions should be initialized but empty")
	assert.Empty(t, cacheData.Actions, "Actions should have no entries")
}

func TestIntegration_CreateAccountingRouteCache_OverwritesExistingKey(t *testing.T) {
	infra := setupCacheTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()

	// Write initial cache
	route1 := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Title:          "Route v1",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:            libCommons.GenerateUUIDv7(),
				OperationType: "source",
				Action:        "direct",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ctx := context.Background()
	err := infra.uc.CreateAccountingRouteCache(ctx, route1)
	require.NoError(t, err, "first CreateAccountingRouteCache should not fail")

	// Overwrite with updated route containing different operation routes
	newSourceID := libCommons.GenerateUUIDv7()
	newDestID := libCommons.GenerateUUIDv7()

	route2 := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Title:          "Route v2",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:            newSourceID,
				OperationType: "source",
				Action:        "hold",
			},
			{
				ID:            newDestID,
				OperationType: "destination",
				Action:        "hold",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Act
	err = infra.uc.CreateAccountingRouteCache(ctx, route2)

	// Assert
	require.NoError(t, err, "second CreateAccountingRouteCache should not fail")

	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, routeID)
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "GetBytes should not return error")

	var cacheData mmodel.TransactionRouteCache
	err = cacheData.FromMsgpack(cachedBytes)
	require.NoError(t, err, "FromMsgpack should not return error")

	// Should reflect route2 data, not route1
	assert.Contains(t, cacheData.Actions, "hold", "should contain 'hold' action from route2")
	assert.NotContains(t, cacheData.Actions, "direct", "should not contain 'direct' action from route1")
}

func TestIntegration_CreateAccountingRouteCache_AccountRulePreserved(t *testing.T) {
	infra := setupCacheTestInfra(t)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()
	sourceID := libCommons.GenerateUUIDv7()

	route := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Title:          "Route With Account Rules",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:            sourceID,
				OperationType: "source",
				Action:        "direct",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@treasury",
				},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ctx := context.Background()

	// Act
	err := infra.uc.CreateAccountingRouteCache(ctx, route)

	// Assert
	require.NoError(t, err, "CreateAccountingRouteCache should not return error")

	internalKey := utils.AccountingRoutesInternalKey(orgID, ledgerID, routeID)
	cachedBytes, err := infra.uc.RedisRepo.GetBytes(ctx, internalKey)
	require.NoError(t, err, "GetBytes should not return error")

	var cacheData mmodel.TransactionRouteCache
	err = cacheData.FromMsgpack(cachedBytes)
	require.NoError(t, err, "FromMsgpack should not return error")

	// Verify account rule in action grouping
	directSource := cacheData.Actions["direct"].Source[sourceID.String()]
	require.NotNil(t, directSource.Account, "action source account rule should not be nil")
	assert.Equal(t, "alias", directSource.Account.RuleType, "action rule type should be preserved")
}

func TestIntegration_CreateAccountingRouteCache_DifferentOrgsSameRouteID(t *testing.T) {
	infra := setupCacheTestInfra(t)

	org1 := libCommons.GenerateUUIDv7()
	org2 := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	routeID := libCommons.GenerateUUIDv7()

	sourceID1 := libCommons.GenerateUUIDv7()
	sourceID2 := libCommons.GenerateUUIDv7()

	route1 := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: org1,
		LedgerID:       ledgerID,
		Title:          "Org1 Route",
		OperationRoutes: []mmodel.OperationRoute{
			{ID: sourceID1, OperationType: "source", Action: "direct"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	route2 := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: org2,
		LedgerID:       ledgerID,
		Title:          "Org2 Route",
		OperationRoutes: []mmodel.OperationRoute{
			{ID: sourceID2, OperationType: "source", Action: "hold"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ctx := context.Background()

	// Act
	err := infra.uc.CreateAccountingRouteCache(ctx, route1)
	require.NoError(t, err)
	err = infra.uc.CreateAccountingRouteCache(ctx, route2)
	require.NoError(t, err)

	// Assert - org1 cache is independent from org2
	key1 := utils.AccountingRoutesInternalKey(org1, ledgerID, routeID)
	bytes1, err := infra.uc.RedisRepo.GetBytes(ctx, key1)
	require.NoError(t, err)

	var cache1 mmodel.TransactionRouteCache
	err = cache1.FromMsgpack(bytes1)
	require.NoError(t, err)
	assert.Contains(t, cache1.Actions, "direct", "org1 should have 'direct' action")
	assert.NotContains(t, cache1.Actions, "hold", "org1 should not have 'hold' action")

	key2 := utils.AccountingRoutesInternalKey(org2, ledgerID, routeID)
	bytes2, err := infra.uc.RedisRepo.GetBytes(ctx, key2)
	require.NoError(t, err)

	var cache2 mmodel.TransactionRouteCache
	err = cache2.FromMsgpack(bytes2)
	require.NoError(t, err)
	assert.Contains(t, cache2.Actions, "hold", "org2 should have 'hold' action")
	assert.NotContains(t, cache2.Actions, "direct", "org2 should not have 'direct' action")
}

// Unused variable prevention for uuid import
var _ = uuid.Nil
