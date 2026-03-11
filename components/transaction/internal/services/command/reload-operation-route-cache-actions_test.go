// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestReloadOperationRouteCache_RebuildWithActionGrouping verifies AC-2: when ReloadOperationRouteCache
// rebuilds caches, the stored cache entries contain action-aware grouping in the Actions field.
func TestReloadOperationRouteCache_RebuildWithActionGrouping(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	operationRouteID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()
	sourceOpRouteID := libCommons.GenerateUUIDv7()
	destOpRouteID := libCommons.GenerateUUIDv7()

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo:   mockOperationRouteRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
		RedisRepo:            mockRedisRepo,
	}

	transactionRouteIDs := []uuid.UUID{transactionRouteID}

	// Transaction route with action-aware operation routes
	transactionRoute := &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Action-Aware Route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:            sourceOpRouteID,
				OperationType: "source",
				Action:        "direct",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@source_account",
				},
			},
			{
				ID:            destOpRouteID,
				OperationType: "destination",
				Action:        "direct",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@dest_account",
				},
			},
		},
	}

	mockOperationRouteRepo.EXPECT().
		FindTransactionRouteIDs(gomock.Any(), operationRouteID).
		Return(transactionRouteIDs, nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(transactionRoute, nil).
		Times(1)

	var capturedBytes []byte

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(0)).
		DoAndReturn(func(_ context.Context, _ string, bytes []byte, _ time.Duration) error {
			capturedBytes = bytes
			return nil
		}).
		Times(1)

	err := uc.ReloadOperationRouteCache(context.Background(), organizationID, ledgerID, operationRouteID)
	require.NoError(t, err)

	// Decode the captured bytes and verify action grouping
	var storedCache mmodel.TransactionRouteCache
	err = storedCache.FromMsgpack(capturedBytes)
	require.NoError(t, err, "stored bytes must be valid msgpack")

	require.NotNil(t, storedCache.Actions,
		"AC-2: rebuilt cache must contain non-nil Actions map")

	directAction, exists := storedCache.Actions["direct"]
	require.True(t, exists, "AC-2: rebuilt cache Actions must contain 'direct' key")
	assert.Len(t, directAction.Source, 1, "direct action should have 1 source route")
	assert.Len(t, directAction.Destination, 1, "direct action should have 1 destination route")
}

// TestReloadOperationRouteCache_MultipleTransactionRoutesWithActions verifies AC-2: when an operation
// route is associated with multiple transaction routes, all are rebuilt with correct action grouping.
func TestReloadOperationRouteCache_MultipleTransactionRoutesWithActions(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	operationRouteID := libCommons.GenerateUUIDv7()
	txRouteID1 := libCommons.GenerateUUIDv7()
	txRouteID2 := libCommons.GenerateUUIDv7()

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo:   mockOperationRouteRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
		RedisRepo:            mockRedisRepo,
	}

	txRoute1 := &mmodel.TransactionRoute{
		ID:             txRouteID1,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Route 1 - Direct",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:            libCommons.GenerateUUIDv7(),
				OperationType: "source",
				Action:        "direct",
			},
		},
	}

	txRoute2 := &mmodel.TransactionRoute{
		ID:             txRouteID2,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Route 2 - Hold",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:            libCommons.GenerateUUIDv7(),
				OperationType: "source",
				Action:        "hold",
			},
			{
				ID:            libCommons.GenerateUUIDv7(),
				OperationType: "destination",
				Action:        "hold",
			},
		},
	}

	mockOperationRouteRepo.EXPECT().
		FindTransactionRouteIDs(gomock.Any(), operationRouteID).
		Return([]uuid.UUID{txRouteID1, txRouteID2}, nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, txRouteID1).
		Return(txRoute1, nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, txRouteID2).
		Return(txRoute2, nil).
		Times(1)

	capturedBytesMap := make(map[int][]byte)
	callIndex := 0

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(0)).
		DoAndReturn(func(_ context.Context, _ string, bytes []byte, _ time.Duration) error {
			capturedBytesMap[callIndex] = bytes
			callIndex++
			return nil
		}).
		Times(2)

	err := uc.ReloadOperationRouteCache(context.Background(), organizationID, ledgerID, operationRouteID)
	require.NoError(t, err)

	assert.Len(t, capturedBytesMap, 2, "should have stored 2 cache entries")

	// Verify both stored caches have non-nil Actions
	for i, bytes := range capturedBytesMap {
		var cache mmodel.TransactionRouteCache
		err := cache.FromMsgpack(bytes)
		require.NoError(t, err, "cache %d must decode successfully", i)
		require.NotNil(t, cache.Actions,
			"AC-2: cache entry %d must have non-nil Actions", i)
	}
}
