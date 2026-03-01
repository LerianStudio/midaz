// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

var errRedisConnection = errors.New("redis connection error")

// TestDeleteTransactionRouteCache_Success tests successful cache deletion.
func TestDeleteTransactionRouteCache_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), expectedKey).
		Return(nil).
		Times(1)

	err := uc.DeleteTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.NoError(t, err)
}

// TestDeleteTransactionRouteCache_RedisError tests error handling when Redis Del fails.
func TestDeleteTransactionRouteCache_RedisError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), expectedKey).
		Return(errRedisConnection).
		Times(1)

	err := uc.DeleteTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	require.Error(t, err)
	assert.Equal(t, errRedisConnection, err)
}

// TestDeleteTransactionRouteCache_ContextCancelled tests error handling when context is cancelled.
func TestDeleteTransactionRouteCache_ContextCancelled(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), expectedKey).
		Return(context.Canceled).
		Times(1)

	err := uc.DeleteTransactionRouteCache(ctx, organizationID, ledgerID, transactionRouteID)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}
