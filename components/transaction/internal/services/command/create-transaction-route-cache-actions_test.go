// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestCreateAccountingRouteCache_StoresActionAwareCache verifies AC-1: the cache stored in Redis
// contains the Actions field with correct action grouping when operation routes have actions.
func TestCreateAccountingRouteCache_StoresActionAwareCache(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	routeID := uuid.Must(libCommons.GenerateUUIDv7())
	sourceRouteID := uuid.Must(libCommons.GenerateUUIDv7())
	destRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	route := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Action-Aware Route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:                sourceRouteID,
				OperationType:     "source",
				Action:            "direct",
				AccountingEntries: &mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{}},
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@cash_account",
				},
			},
			{
				ID:                destRouteID,
				OperationType:     "destination",
				Action:            "direct",
				AccountingEntries: &mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{}},
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@merchant_account",
				},
			},
		},
	}

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	var capturedBytes []byte

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(0)).
		DoAndReturn(func(_ context.Context, _ string, bytes []byte, _ time.Duration) error {
			capturedBytes = bytes
			return nil
		}).
		Times(1)

	err := uc.CreateAccountingRouteCache(context.Background(), route)
	require.NoError(t, err)

	// Decode the captured bytes and verify Actions field is populated
	var storedCache mmodel.TransactionRouteCache
	err = storedCache.FromMsgpack(capturedBytes)
	require.NoError(t, err, "stored bytes must be valid msgpack")

	require.NotNil(t, storedCache.Actions,
		"AC-1: stored cache must contain non-nil Actions map")
	assert.Len(t, storedCache.Actions, 1,
		"AC-1: stored cache should have exactly 1 action group (direct)")

	directAction, exists := storedCache.Actions["direct"]
	require.True(t, exists, "AC-1: Actions map must contain 'direct' key")
	assert.Len(t, directAction.Source, 1, "direct action should have 1 source route")
	assert.Len(t, directAction.Destination, 1, "direct action should have 1 destination route")
}

// TestCreateAccountingRouteCache_MultipleActions verifies AC-1: multiple actions are correctly
// grouped in the stored cache.
func TestCreateAccountingRouteCache_MultipleActions(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	routeID := uuid.Must(libCommons.GenerateUUIDv7())

	route := &mmodel.TransactionRoute{
		ID:             routeID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Multi-Action Route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:                uuid.Must(libCommons.GenerateUUIDv7()),
				OperationType:     "source",
				Action:            "direct",
				AccountingEntries: &mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{}},
			},
			{
				ID:                uuid.Must(libCommons.GenerateUUIDv7()),
				OperationType:     "destination",
				Action:            "direct",
				AccountingEntries: &mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{}},
			},
			{
				ID:                uuid.Must(libCommons.GenerateUUIDv7()),
				OperationType:     "source",
				Action:            "hold",
				AccountingEntries: &mmodel.AccountingEntries{Hold: &mmodel.AccountingEntry{}},
			},
			{
				ID:                uuid.Must(libCommons.GenerateUUIDv7()),
				OperationType:     "bidirectional",
				Action:            "hold",
				AccountingEntries: &mmodel.AccountingEntries{Hold: &mmodel.AccountingEntry{}},
			},
		},
	}

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	var capturedBytes []byte

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(0)).
		DoAndReturn(func(_ context.Context, _ string, bytes []byte, _ time.Duration) error {
			capturedBytes = bytes
			return nil
		}).
		Times(1)

	err := uc.CreateAccountingRouteCache(context.Background(), route)
	require.NoError(t, err)

	var storedCache mmodel.TransactionRouteCache
	err = storedCache.FromMsgpack(capturedBytes)
	require.NoError(t, err)

	require.NotNil(t, storedCache.Actions)
	assert.Len(t, storedCache.Actions, 2, "should have 2 action groups: direct and hold")

	directAction := storedCache.Actions["direct"]
	assert.Len(t, directAction.Source, 1)
	assert.Len(t, directAction.Destination, 1)

	holdAction := storedCache.Actions["hold"]
	assert.Len(t, holdAction.Source, 1)
	assert.Len(t, holdAction.Bidirectional, 1)
}
